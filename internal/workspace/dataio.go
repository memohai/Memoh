package workspace

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/errdefs"

	ctr "github.com/memohai/memoh/internal/containerd"
)

const (
	containerDataDir = "/data"
	backupsSubdir    = "backups"
	legacyBotsSubdir = "bots"
	migratedSuffix   = ".migrated"
)

// ExportData streams a tar.gz archive of the container's /data directory.
// The container is stopped during export and restarted afterwards.
// Caller must consume the returned reader before the context is cancelled.
func (m *Manager) ExportData(ctx context.Context, botID string) (io.ReadCloser, error) {
	containerID := m.containerID(botID)
	unlock := m.lockContainer(containerID)
	defer unlock()

	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("get container: %w", err)
	}

	mounts, err := m.snapshotMounts(ctx, info)
	if errors.Is(err, errMountNotSupported) {
		return m.exportDataViaGRPC(ctx, botID)
	}
	if err != nil {
		return nil, err
	}

	if err := m.safeStopTask(ctx, containerID); err != nil {
		return nil, fmt.Errorf("stop container: %w", err)
	}

	pr, pw := io.Pipe()

	go func() {
		var exportErr error
		defer func() {
			_ = pw.CloseWithError(exportErr)
			m.restartContainer(context.WithoutCancel(ctx), botID, containerID)
		}()

		exportErr = mount.WithReadonlyTempMount(ctx, mounts, func(root string) error {
			dataDir := mountedDataDir(root)
			if _, err := os.Stat(dataDir); err != nil {
				return nil // no /data, produce empty archive
			}
			return tarGzDir(pw, dataDir)
		})
	}()

	return pr, nil
}

// ImportData extracts a tar.gz archive into the container's /data directory.
// The container is stopped during import and restarted afterwards.
func (m *Manager) ImportData(ctx context.Context, botID string, r io.Reader) error {
	containerID := m.containerID(botID)
	unlock := m.lockContainer(containerID)
	defer unlock()

	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("get container: %w", err)
	}

	mounts, err := m.snapshotMounts(ctx, info)
	if errors.Is(err, errMountNotSupported) {
		return m.importDataViaGRPC(ctx, botID, r)
	}
	if err != nil {
		return err
	}

	if err := m.safeStopTask(ctx, containerID); err != nil {
		return fmt.Errorf("stop container: %w", err)
	}
	defer m.restartContainer(context.WithoutCancel(ctx), botID, containerID)

	return mount.WithTempMount(ctx, mounts, func(root string) error {
		dataDir := mountedDataDir(root)
		if err := os.MkdirAll(dataDir, 0o750); err != nil {
			return err
		}
		return untarGzDir(r, dataDir)
	})
}

// PreserveData exports /data to a backup tar.gz on the host. Used before
// deleting a container when the user chooses to preserve data.
// For snapshot-mount backends the caller must stop the task first so the
// mounted snapshot is consistent; the Apple fallback uses gRPC and does not
// require a stop.
func (m *Manager) PreserveData(ctx context.Context, botID string) error {
	// Resolve the actual container ID — may be legacy "mcp-" or new "workspace-".
	containerID, err := m.ContainerID(ctx, botID)
	if err != nil {
		m.logger.Error("[MYDEBUG] PreserveData: ContainerID resolution failed",
			slog.String("bot_id", botID), slog.Any("error", err))
		return fmt.Errorf("resolve container for preserve: %w", err)
	}
	m.logger.Info("[MYDEBUG] PreserveData called",
		slog.String("bot_id", botID), slog.String("container_id", containerID))

	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		m.logger.Error("[MYDEBUG] PreserveData: GetContainer failed",
			slog.String("bot_id", botID), slog.Any("error", err))
		return fmt.Errorf("get container: %w", err)
	}

	backupPath := m.backupPath(botID)
	m.logger.Info("[MYDEBUG] PreserveData: backup target",
		slog.String("bot_id", botID), slog.String("backup_path", backupPath))
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o750); err != nil {
		m.logger.Error("[MYDEBUG] PreserveData: MkdirAll failed",
			slog.String("bot_id", botID), slog.Any("error", err))
		return fmt.Errorf("create backup dir: %w", err)
	}

	mounts, mountErr := m.snapshotMounts(ctx, info)
	if errors.Is(mountErr, errMountNotSupported) {
		m.logger.Info("[MYDEBUG] PreserveData: mounts not supported, falling back to gRPC",
			slog.String("bot_id", botID))
		return m.preserveDataViaGRPC(ctx, botID, backupPath)
	}
	if mountErr != nil {
		m.logger.Error("[MYDEBUG] PreserveData: snapshotMounts failed",
			slog.String("bot_id", botID), slog.Any("error", mountErr))
		return mountErr
	}
	m.logger.Info("[MYDEBUG] PreserveData: snapshot mounts obtained",
		slog.String("bot_id", botID), slog.Int("mount_count", len(mounts)))

	f, err := os.Create(backupPath) //nolint:gosec // G304: operator-controlled path
	if err != nil {
		m.logger.Error("[MYDEBUG] PreserveData: os.Create backup file failed",
			slog.String("bot_id", botID), slog.Any("error", err))
		return fmt.Errorf("create backup file: %w", err)
	}

	writeErr := mount.WithReadonlyTempMount(ctx, mounts, func(root string) error {
		dataDir := mountedDataDir(root)
		m.logger.Info("[MYDEBUG] PreserveData: mounted snapshot, checking /data",
			slog.String("bot_id", botID), slog.String("root", root),
			slog.String("data_dir", dataDir))
		stat, statErr := os.Stat(dataDir)
		if statErr != nil {
			m.logger.Warn("[MYDEBUG] PreserveData: /data does NOT exist in snapshot, nothing to backup",
				slog.String("bot_id", botID), slog.Any("stat_error", statErr))
			return nil // no /data to backup
		}
		m.logger.Info("[MYDEBUG] PreserveData: /data exists, starting tarGzDir",
			slog.String("bot_id", botID), slog.Bool("is_dir", stat.IsDir()))
		return tarGzDir(f, dataDir)
	})

	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(backupPath)
		m.logger.Error("[MYDEBUG] PreserveData: tarGzDir/mount FAILED, backup removed",
			slog.String("bot_id", botID), slog.Any("error", writeErr))
		return fmt.Errorf("export data: %w", writeErr)
	}
	if closeErr != nil {
		m.logger.Error("[MYDEBUG] PreserveData: file Close failed",
			slog.String("bot_id", botID), slog.Any("error", closeErr))
		return closeErr
	}

	// Log backup file size for verification
	if fi, err := os.Stat(backupPath); err == nil {
		m.logger.Info("[MYDEBUG] PreserveData: SUCCESS",
			slog.String("bot_id", botID),
			slog.String("backup_path", backupPath),
			slog.Int64("backup_size_bytes", fi.Size()))
	}
	return nil
}

// RestorePreservedData imports preserved data (backup tar.gz or legacy
// bind-mount directory) into a running container's /data.
func (m *Manager) RestorePreservedData(ctx context.Context, botID string) error {
	m.logger.Info("[MYDEBUG] RestorePreservedData called", slog.String("bot_id", botID))

	bp := m.backupPath(botID)
	m.logger.Info("[MYDEBUG] RestorePreservedData: checking backup tar.gz",
		slog.String("bot_id", botID), slog.String("backup_path", bp))
	if bpStat, err := os.Stat(bp); err == nil {
		m.logger.Info("[MYDEBUG] RestorePreservedData: backup tar.gz found",
			slog.String("bot_id", botID), slog.Int64("size_bytes", bpStat.Size()))
		f, err := os.Open(bp) //nolint:gosec // G304: operator-controlled path
		if err != nil {
			m.logger.Error("[MYDEBUG] RestorePreservedData: failed to open backup",
				slog.String("bot_id", botID), slog.Any("error", err))
			return err
		}
		defer func() { _ = f.Close() }()

		m.logger.Info("[MYDEBUG] RestorePreservedData: calling ImportData from tar.gz",
			slog.String("bot_id", botID))
		if err := m.ImportData(ctx, botID, f); err != nil {
			m.logger.Error("[MYDEBUG] RestorePreservedData: ImportData failed",
				slog.String("bot_id", botID), slog.Any("error", err))
			return err
		}
		m.logger.Info("[MYDEBUG] RestorePreservedData: ImportData succeeded, removing backup",
			slog.String("bot_id", botID))
		return os.Remove(bp)
	} else {
		m.logger.Info("[MYDEBUG] RestorePreservedData: no backup tar.gz, checking legacy dir",
			slog.String("bot_id", botID), slog.Any("stat_error", err))
	}

	// Legacy bind-mount directory
	legacyDir := m.legacyDataDir(botID)
	migratedDir := legacyDir + migratedSuffix
	m.logger.Info("[MYDEBUG] RestorePreservedData: checking legacy paths",
		slog.String("bot_id", botID),
		slog.String("legacy_dir", legacyDir),
		slog.String("migrated_dir", migratedDir))

	if _, err := os.Stat(migratedDir); err == nil {
		m.logger.Info("[MYDEBUG] RestorePreservedData: .migrated marker exists, already imported previously",
			slog.String("bot_id", botID))
		return nil // already imported previously
	}
	info, err := os.Stat(legacyDir)
	if err != nil || !info.IsDir() {
		m.logger.Error("[MYDEBUG] RestorePreservedData: no preserved data found anywhere",
			slog.String("bot_id", botID), slog.Any("stat_error", err))
		return errors.New("no preserved data found")
	}

	m.logger.Info("[MYDEBUG] RestorePreservedData: legacy dir found, calling importLegacyDir",
		slog.String("bot_id", botID), slog.String("legacy_dir", legacyDir))
	return m.importLegacyDir(ctx, botID, legacyDir)
}

// HasPreservedData checks whether backup data exists for a bot, either as
// a tar.gz backup or a legacy bind-mount directory.
func (m *Manager) HasPreservedData(botID string) bool {
	bp := m.backupPath(botID)
	if _, err := os.Stat(bp); err == nil {
		m.logger.Info("[MYDEBUG] HasPreservedData: backup tar.gz exists",
			slog.String("bot_id", botID), slog.String("backup_path", bp))
		return true
	}
	legacyDir := m.legacyDataDir(botID)
	if _, err := os.Stat(legacyDir + migratedSuffix); err == nil {
		m.logger.Info("[MYDEBUG] HasPreservedData: .migrated marker found, already imported → false",
			slog.String("bot_id", botID))
		return false // already imported
	}
	info, err := os.Stat(legacyDir)
	result := err == nil && info.IsDir()
	m.logger.Info("[MYDEBUG] HasPreservedData: checked legacy dir",
		slog.String("bot_id", botID), slog.String("legacy_dir", legacyDir),
		slog.Bool("exists_and_is_dir", result), slog.Any("stat_error", err))
	return result
}

// importLegacyDir copies a legacy bind-mount directory into the container
// via snapshot mount, then renames the source to .migrated.
func (m *Manager) importLegacyDir(ctx context.Context, botID, srcDir string) error {
	containerID := m.containerID(botID)
	m.logger.Info("[MYDEBUG] importLegacyDir called",
		slog.String("bot_id", botID), slog.String("src_dir", srcDir),
		slog.String("container_id", containerID))

	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		m.logger.Error("[MYDEBUG] importLegacyDir: GetContainer failed",
			slog.String("bot_id", botID), slog.Any("error", err))
		return fmt.Errorf("get container: %w", err)
	}

	mounts, err := m.snapshotMounts(ctx, info)
	if errors.Is(err, errMountNotSupported) {
		m.logger.Info("[MYDEBUG] importLegacyDir: mounts not supported, using gRPC fallback",
			slog.String("bot_id", botID))
		return m.importLegacyDirViaGRPC(ctx, botID, srcDir)
	}
	if err != nil {
		m.logger.Error("[MYDEBUG] importLegacyDir: snapshotMounts failed",
			slog.String("bot_id", botID), slog.Any("error", err))
		return err
	}

	m.logger.Info("[MYDEBUG] importLegacyDir: stopping task for import",
		slog.String("bot_id", botID))
	if err := m.safeStopTask(ctx, containerID); err != nil {
		m.logger.Error("[MYDEBUG] importLegacyDir: safeStopTask failed",
			slog.String("bot_id", botID), slog.Any("error", err))
		return fmt.Errorf("stop container: %w", err)
	}
	defer m.restartContainer(context.WithoutCancel(ctx), botID, containerID)

	m.logger.Info("[MYDEBUG] importLegacyDir: mounting snapshot and copying dir contents",
		slog.String("bot_id", botID))
	mountErr := mount.WithTempMount(ctx, mounts, func(root string) error {
		dataDir := mountedDataDir(root)
		m.logger.Info("[MYDEBUG] importLegacyDir: mounted, creating /data and copying",
			slog.String("bot_id", botID), slog.String("data_dir", dataDir))
		if err := os.MkdirAll(dataDir, 0o750); err != nil {
			return err
		}
		return copyDirContents(srcDir, dataDir)
	})
	if mountErr != nil {
		m.logger.Error("[MYDEBUG] importLegacyDir: mount/copy FAILED",
			slog.String("bot_id", botID), slog.Any("error", mountErr))
		return mountErr
	}

	m.logger.Info("[MYDEBUG] importLegacyDir: copy succeeded, renaming to .migrated",
		slog.String("bot_id", botID))
	if err := os.Rename(srcDir, srcDir+migratedSuffix); err != nil {
		m.logger.Warn("[MYDEBUG] importLegacyDir: rename to .migrated failed (non-fatal)",
			slog.String("src", srcDir), slog.Any("error", err))
	}
	m.logger.Info("[MYDEBUG] importLegacyDir: SUCCESS", slog.String("bot_id", botID))
	return nil
}

// recoverOrphanedSnapshot detects a snapshot whose container was deleted
// (e.g. dev image rebuild, containerd metadata loss) and exports /data to a
// backup archive. The caller should invoke restorePreservedIntoSnapshot after
// creating the replacement container. Returns true when data was preserved.
func (m *Manager) recoverOrphanedSnapshot(ctx context.Context, botID string) bool {
	m.logger.Info("[MYDEBUG] recoverOrphanedSnapshot called", slog.String("bot_id", botID))

	snapshotter := m.cfg.Snapshotter
	if snapshotter == "" {
		m.logger.Info("[MYDEBUG] recoverOrphanedSnapshot: no snapshotter configured, skip",
			slog.String("bot_id", botID))
		return false
	}

	snapshotKey := m.containerID(botID)
	m.logger.Info("[MYDEBUG] recoverOrphanedSnapshot: querying snapshot mounts",
		slog.String("bot_id", botID), slog.String("snapshotter", snapshotter),
		slog.String("snapshot_key", snapshotKey))
	raw, err := m.service.SnapshotMounts(ctx, snapshotter, snapshotKey)
	if err != nil {
		m.logger.Info("[MYDEBUG] recoverOrphanedSnapshot: SnapshotMounts failed (no orphan)",
			slog.String("bot_id", botID), slog.Any("error", err))
		return false
	}
	m.logger.Info("[MYDEBUG] recoverOrphanedSnapshot: orphaned snapshot FOUND",
		slog.String("bot_id", botID), slog.Int("mount_count", len(raw)))

	mounts := make([]mount.Mount, len(raw))
	for i, r := range raw {
		mounts[i] = mount.Mount{Type: r.Type, Source: r.Source, Options: r.Options}
	}

	backupPath := m.backupPath(botID)
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o750); err != nil {
		m.logger.Warn("[MYDEBUG] recoverOrphanedSnapshot: mkdir failed",
			slog.String("bot_id", botID), slog.Any("error", err))
		return false
	}

	f, err := os.Create(backupPath) //nolint:gosec // G304: operator-controlled path
	if err != nil {
		m.logger.Warn("[MYDEBUG] recoverOrphanedSnapshot: create backup file failed",
			slog.String("bot_id", botID), slog.Any("error", err))
		return false
	}

	writeErr := mount.WithReadonlyTempMount(ctx, mounts, func(root string) error {
		dataDir := mountedDataDir(root)
		m.logger.Info("[MYDEBUG] recoverOrphanedSnapshot: mounted, checking /data",
			slog.String("bot_id", botID), slog.String("data_dir", dataDir))
		if _, statErr := os.Stat(dataDir); statErr != nil {
			m.logger.Warn("[MYDEBUG] recoverOrphanedSnapshot: /data does not exist in orphan",
				slog.String("bot_id", botID), slog.Any("stat_error", statErr))
			return nil
		}
		m.logger.Info("[MYDEBUG] recoverOrphanedSnapshot: /data exists, exporting",
			slog.String("bot_id", botID))
		return tarGzDir(f, dataDir)
	})

	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(backupPath)
		m.logger.Warn("[MYDEBUG] recoverOrphanedSnapshot: export FAILED",
			slog.String("bot_id", botID), slog.Any("error", writeErr))
		return false
	}
	if closeErr != nil {
		_ = os.Remove(backupPath)
		m.logger.Warn("[MYDEBUG] recoverOrphanedSnapshot: file close FAILED",
			slog.String("bot_id", botID), slog.Any("error", closeErr))
		return false
	}

	if fi, err := os.Stat(backupPath); err == nil {
		m.logger.Info("[MYDEBUG] recoverOrphanedSnapshot: SUCCESS",
			slog.String("bot_id", botID), slog.String("backup", backupPath),
			slog.Int64("backup_size_bytes", fi.Size()))
	}
	return true
}

// restorePreservedIntoSnapshot restores a preserved backup directly into
// the container's snapshot before the task is started. This avoids the
// stop/start cycle that RestorePreservedData (via ImportData) requires.
func (m *Manager) restorePreservedIntoSnapshot(ctx context.Context, botID string) error {
	bp := m.backupPath(botID)
	m.logger.Info("[MYDEBUG] restorePreservedIntoSnapshot called",
		slog.String("bot_id", botID), slog.String("backup_path", bp))

	bpStat, statErr := os.Stat(bp)
	if statErr != nil {
		m.logger.Error("[MYDEBUG] restorePreservedIntoSnapshot: backup file does NOT exist",
			slog.String("bot_id", botID), slog.Any("error", statErr))
	} else {
		m.logger.Info("[MYDEBUG] restorePreservedIntoSnapshot: backup file found",
			slog.String("bot_id", botID), slog.Int64("size_bytes", bpStat.Size()))
	}

	f, err := os.Open(bp) //nolint:gosec // G304: operator-controlled path
	if err != nil {
		m.logger.Error("[MYDEBUG] restorePreservedIntoSnapshot: os.Open FAILED",
			slog.String("bot_id", botID), slog.Any("error", err))
		return err
	}
	defer func() { _ = f.Close() }()

	containerID := m.containerID(botID)
	m.logger.Info("[MYDEBUG] restorePreservedIntoSnapshot: getting container info",
		slog.String("bot_id", botID), slog.String("container_id", containerID))
	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		m.logger.Error("[MYDEBUG] restorePreservedIntoSnapshot: GetContainer FAILED",
			slog.String("bot_id", botID), slog.Any("error", err))
		return fmt.Errorf("get container: %w", err)
	}

	m.logger.Info("[MYDEBUG] restorePreservedIntoSnapshot: getting snapshot mounts",
		slog.String("bot_id", botID),
		slog.String("snapshotter", info.Snapshotter),
		slog.String("snapshot_key", info.SnapshotKey))
	mounts, err := m.snapshotMounts(ctx, info)
	if err != nil {
		m.logger.Error("[MYDEBUG] restorePreservedIntoSnapshot: snapshotMounts FAILED",
			slog.String("bot_id", botID), slog.Any("error", err))
		return err
	}
	m.logger.Info("[MYDEBUG] restorePreservedIntoSnapshot: mounting snapshot and untarring",
		slog.String("bot_id", botID), slog.Int("mount_count", len(mounts)))

	if err := mount.WithTempMount(ctx, mounts, func(root string) error {
		dataDir := mountedDataDir(root)
		m.logger.Info("[MYDEBUG] restorePreservedIntoSnapshot: mounted at root, creating /data",
			slog.String("bot_id", botID), slog.String("root", root),
			slog.String("data_dir", dataDir))
		if err := os.MkdirAll(dataDir, 0o750); err != nil {
			m.logger.Error("[MYDEBUG] restorePreservedIntoSnapshot: MkdirAll /data FAILED",
				slog.String("bot_id", botID), slog.Any("error", err))
			return err
		}
		m.logger.Info("[MYDEBUG] restorePreservedIntoSnapshot: calling untarGzDir",
			slog.String("bot_id", botID))
		return untarGzDir(f, dataDir)
	}); err != nil {
		m.logger.Error("[MYDEBUG] restorePreservedIntoSnapshot: WithTempMount/untarGzDir FAILED",
			slog.String("bot_id", botID), slog.Any("error", err))
		return err
	}

	_ = os.Remove(bp)
	m.logger.Info("[MYDEBUG] restorePreservedIntoSnapshot: SUCCESS, backup removed",
		slog.String("bot_id", botID))
	return nil
}

// errMountNotSupported indicates the backend doesn't support snapshot mounts
// (e.g. Apple Virtualization). Callers fall back to gRPC-based data operations.
var errMountNotSupported = errors.New("snapshot mount not supported on this backend")

func (m *Manager) snapshotMounts(ctx context.Context, info ctr.ContainerInfo) ([]mount.Mount, error) {
	raw, err := m.service.SnapshotMounts(ctx, info.Snapshotter, info.SnapshotKey)
	if err != nil {
		if errors.Is(err, ctr.ErrNotSupported) {
			return nil, errMountNotSupported
		}
		return nil, fmt.Errorf("get snapshot mounts: %w", err)
	}
	mounts := make([]mount.Mount, len(raw))
	for i, r := range raw {
		mounts[i] = mount.Mount{
			Type:    r.Type,
			Source:  r.Source,
			Options: r.Options,
		}
	}
	return mounts, nil
}

func (m *Manager) restartContainer(ctx context.Context, botID, containerID string) {
	m.grpcPool.Remove(botID)
	if err := m.service.DeleteTask(ctx, containerID, &ctr.DeleteTaskOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
		m.logger.Warn("cleanup stale task after data operation failed",
			slog.String("container_id", containerID), slog.Any("error", err))
		return
	}
	if err := m.service.StartContainer(ctx, containerID, nil); err != nil {
		m.logger.Warn("restart after data operation failed",
			slog.String("container_id", containerID), slog.Any("error", err))
		return
	}
	// CNI network setup — outbound connectivity is required for package
	// downloads and other network-dependent operations in the container.
	if _, err := m.service.SetupNetwork(ctx, ctr.NetworkSetupRequest{
		ContainerID: containerID,
		CNIBinDir:   m.cfg.CNIBinaryDir,
		CNIConfDir:  m.cfg.CNIConfigDir,
	}); err != nil {
		m.logger.Error("network setup after restart failed",
			slog.String("container_id", containerID), slog.Any("error", err))
		return
	}
}

func mountedDataDir(root string) string {
	return filepath.Join(root, strings.TrimPrefix(containerDataDir, string(filepath.Separator)))
}

func (m *Manager) backupPath(botID string) string {
	return filepath.Join(m.dataRoot(), backupsSubdir, botID+".tar.gz")
}

func (m *Manager) legacyDataDir(botID string) string {
	return filepath.Join(m.dataRoot(), legacyBotsSubdir, botID)
}

// ---------------------------------------------------------------------------
// gRPC fallback (Apple backend / no mount support)
// ---------------------------------------------------------------------------

func (m *Manager) exportDataViaGRPC(ctx context.Context, botID string) (io.ReadCloser, error) {
	client, err := m.grpcPool.Get(ctx, botID)
	if err != nil {
		return nil, fmt.Errorf("grpc connect: %w", err)
	}

	entries, err := client.ListDir(ctx, containerDataDir, true)
	if err != nil {
		return nil, fmt.Errorf("list dir: %w", err)
	}

	pr, pw := io.Pipe()
	go func() {
		gw := gzip.NewWriter(pw)
		tw := tar.NewWriter(gw)
		var writeErr error
		defer func() {
			_ = tw.Close()
			_ = gw.Close()
			_ = pw.CloseWithError(writeErr)
		}()

		for _, entry := range entries {
			if entry.GetIsDir() {
				continue
			}
			relPath := entry.GetPath()
			absPath := containerDataDir + "/" + strings.TrimPrefix(relPath, "/")

			r, readErr := client.ReadRaw(ctx, absPath)
			if readErr != nil {
				writeErr = fmt.Errorf("read %s: %w", absPath, readErr)
				return
			}
			hdr := &tar.Header{
				Name: relPath,
				Size: entry.GetSize(),
				Mode: 0o644,
			}
			if writeErr = tw.WriteHeader(hdr); writeErr != nil {
				_ = r.Close()
				return
			}
			if _, writeErr = io.Copy(tw, r); writeErr != nil {
				_ = r.Close()
				return
			}
			_ = r.Close()
		}
	}()

	return pr, nil
}

func (m *Manager) preserveDataViaGRPC(ctx context.Context, botID, backupPath string) error {
	reader, err := m.exportDataViaGRPC(ctx, botID)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()

	f, err := os.Create(backupPath) //nolint:gosec // G304: operator-controlled path
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}
	if _, err := io.Copy(f, reader); err != nil {
		_ = f.Close()
		_ = os.Remove(backupPath)
		return err
	}
	return f.Close()
}

func (m *Manager) importDataViaGRPC(ctx context.Context, botID string, r io.Reader) error {
	client, err := m.grpcPool.Get(ctx, botID)
	if err != nil {
		return fmt.Errorf("grpc connect: %w", err)
	}

	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		absPath := containerDataDir + "/" + strings.TrimPrefix(header.Name, "/")
		if _, err := client.WriteRaw(ctx, absPath, io.LimitReader(tr, header.Size)); err != nil {
			return fmt.Errorf("write %s: %w", absPath, err)
		}
	}
}

func (m *Manager) importLegacyDirViaGRPC(ctx context.Context, botID, srcDir string) error {
	client, err := m.grpcPool.Get(ctx, botID)
	if err != nil {
		return fmt.Errorf("grpc connect: %w", err)
	}

	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(srcDir, path)
		if relErr != nil || rel == "." || d.IsDir() {
			return relErr
		}
		f, openErr := os.Open(path) //nolint:gosec // G304: operator-controlled legacy data path
		if openErr != nil {
			return openErr
		}
		defer func() { _ = f.Close() }()

		containerPath := containerDataDir + "/" + filepath.ToSlash(rel)
		_, copyErr := client.WriteRaw(ctx, containerPath, f)
		return copyErr
	})
	if err != nil {
		return err
	}

	if err := os.Rename(srcDir, srcDir+migratedSuffix); err != nil {
		m.logger.Warn("legacy import: rename failed",
			slog.String("src", srcDir), slog.Any("error", err))
	}
	return nil
}

// ---------------------------------------------------------------------------
// tar.gz helpers
// ---------------------------------------------------------------------------

// tarGzDir writes a gzip-compressed tar archive of all files under dir to w.
// Paths inside the archive are relative to dir.
func tarGzDir(w io.Writer, dir string) error {
	gw := gzip.NewWriter(w)
	defer func() { _ = gw.Close() }()
	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil || rel == "." {
			return err
		}

		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(rel)
			return tw.WriteHeader(header)
		}

		// For regular files: open first, then Fstat on the same fd so that
		// the size in the tar header is guaranteed to match the content we
		// read. This avoids race conditions and overlayfs size mismatches
		// that cause "archive/tar: write too long".
		f, err := os.Open(path) //nolint:gosec // G304: iterating operator-controlled data directory
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		info, err := f.Stat()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		_, err = io.Copy(tw, io.LimitReader(f, info.Size()))
		return err
	})
}

// untarGzDir extracts a gzip-compressed tar archive into dst.
func untarGzDir(r io.Reader, dst string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer func() { _ = gr.Close() }()
	tr := tar.NewReader(gr)
	root, err := os.OpenRoot(dst)
	if err != nil {
		return fmt.Errorf("open root: %w", err)
	}
	defer func() { _ = root.Close() }()

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}

		target, err := sanitizeArchivePath(header.Name)
		if err != nil {
			return err
		}
		if target == "" {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			mode := header.FileInfo().Mode().Perm()
			if err := root.MkdirAll(target, mode); err != nil {
				return err
			}
		case tar.TypeReg:
			mode := header.FileInfo().Mode().Perm()
			parent := filepath.Dir(target)
			if parent != "." && parent != "" {
				if err := root.MkdirAll(parent, 0o750); err != nil {
					return err
				}
			}
			f, err := root.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil { //nolint:gosec // G110: decompression bomb not a concern for operator archives
				_ = f.Close()
				return err
			}
			_ = f.Close()
		}
	}
}

// copyDirContents copies all files from src into dst (both must be directories).
func copyDirContents(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || rel == "." {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}

		in, err := os.Open(path) //nolint:gosec // G304: copying operator-controlled migration data
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()

		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		out, err := os.Create(target) //nolint:gosec // G304: target within mounted snapshot
		if err != nil {
			return err
		}
		defer func() { _ = out.Close() }()

		_, err = io.Copy(out, in)
		return err
	})
}

// sanitizeArchivePath converts a tar header path into a safe relative path.
// Empty or "." paths are ignored.
func sanitizeArchivePath(name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || clean == "" {
		return "", nil
	}
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("tar absolute path is not allowed: %s", name)
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("tar path traversal: %s", name)
	}
	return clean, nil
}
