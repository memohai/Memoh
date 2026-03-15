package mcp

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
	containerID := m.containerID(botID)

	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("get container: %w", err)
	}

	backupPath := m.backupPath(botID)
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o750); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	mounts, mountErr := m.snapshotMounts(ctx, info)
	if errors.Is(mountErr, errMountNotSupported) {
		return m.preserveDataViaGRPC(ctx, botID, backupPath)
	}
	if mountErr != nil {
		return mountErr
	}

	f, err := os.Create(backupPath) //nolint:gosec // G304: operator-controlled path
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}

	writeErr := mount.WithReadonlyTempMount(ctx, mounts, func(root string) error {
		dataDir := mountedDataDir(root)
		if _, statErr := os.Stat(dataDir); statErr != nil {
			return nil // no /data to backup
		}
		return tarGzDir(f, dataDir)
	})

	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("export data: %w", writeErr)
	}
	return closeErr
}

// RestorePreservedData imports preserved data (backup tar.gz or legacy
// bind-mount directory) into a running container's /data.
func (m *Manager) RestorePreservedData(ctx context.Context, botID string) error {
	bp := m.backupPath(botID)
	if _, err := os.Stat(bp); err == nil {
		f, err := os.Open(bp) //nolint:gosec // G304: operator-controlled path
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		if err := m.ImportData(ctx, botID, f); err != nil {
			return err
		}
		return os.Remove(bp)
	}

	// Legacy bind-mount directory
	legacyDir := m.legacyDataDir(botID)
	migratedDir := legacyDir + migratedSuffix
	if _, err := os.Stat(migratedDir); err == nil {
		return nil // already imported previously
	}
	info, err := os.Stat(legacyDir)
	if err != nil || !info.IsDir() {
		return errors.New("no preserved data found")
	}

	return m.importLegacyDir(ctx, botID, legacyDir)
}

// HasPreservedData checks whether backup data exists for a bot, either as
// a tar.gz backup or a legacy bind-mount directory.
func (m *Manager) HasPreservedData(botID string) bool {
	if _, err := os.Stat(m.backupPath(botID)); err == nil {
		return true
	}
	legacyDir := m.legacyDataDir(botID)
	if _, err := os.Stat(legacyDir + migratedSuffix); err == nil {
		return false // already imported
	}
	info, err := os.Stat(legacyDir)
	return err == nil && info.IsDir()
}

// importLegacyDir copies a legacy bind-mount directory into the container
// via snapshot mount, then renames the source to .migrated.
func (m *Manager) importLegacyDir(ctx context.Context, botID, srcDir string) error {
	containerID := m.containerID(botID)

	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("get container: %w", err)
	}

	mounts, err := m.snapshotMounts(ctx, info)
	if errors.Is(err, errMountNotSupported) {
		return m.importLegacyDirViaGRPC(ctx, botID, srcDir)
	}
	if err != nil {
		return err
	}

	if err := m.safeStopTask(ctx, containerID); err != nil {
		return fmt.Errorf("stop container: %w", err)
	}
	defer m.restartContainer(context.WithoutCancel(ctx), botID, containerID)

	mountErr := mount.WithTempMount(ctx, mounts, func(root string) error {
		dataDir := mountedDataDir(root)
		if err := os.MkdirAll(dataDir, 0o750); err != nil {
			return err
		}
		return copyDirContents(srcDir, dataDir)
	})
	if mountErr != nil {
		return mountErr
	}

	if err := os.Rename(srcDir, srcDir+migratedSuffix); err != nil {
		m.logger.Warn("legacy import: rename failed",
			slog.String("src", srcDir), slog.Any("error", err))
	}
	return nil
}

// recoverOrphanedSnapshot detects a snapshot whose container was deleted
// (e.g. dev image rebuild, containerd metadata loss) and exports /data to a
// backup archive. The caller should invoke restorePreservedIntoSnapshot after
// creating the replacement container. Returns true when data was preserved.
func (m *Manager) recoverOrphanedSnapshot(ctx context.Context, botID string) bool {
	snapshotter := m.cfg.Snapshotter
	if snapshotter == "" {
		return false
	}

	snapshotKey := m.containerID(botID)
	raw, err := m.service.SnapshotMounts(ctx, snapshotter, snapshotKey)
	if err != nil {
		return false
	}

	mounts := make([]mount.Mount, len(raw))
	for i, r := range raw {
		mounts[i] = mount.Mount{Type: r.Type, Source: r.Source, Options: r.Options}
	}

	backupPath := m.backupPath(botID)
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o750); err != nil {
		m.logger.Warn("recover orphaned snapshot: mkdir failed",
			slog.String("bot_id", botID), slog.Any("error", err))
		return false
	}

	f, err := os.Create(backupPath) //nolint:gosec // G304: operator-controlled path
	if err != nil {
		m.logger.Warn("recover orphaned snapshot: create backup failed",
			slog.String("bot_id", botID), slog.Any("error", err))
		return false
	}

	writeErr := mount.WithReadonlyTempMount(ctx, mounts, func(root string) error {
		dataDir := mountedDataDir(root)
		if _, statErr := os.Stat(dataDir); statErr != nil {
			return nil
		}
		return tarGzDir(f, dataDir)
	})

	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(backupPath)
		m.logger.Warn("recover orphaned snapshot: export failed",
			slog.String("bot_id", botID), slog.Any("error", writeErr))
		return false
	}
	if closeErr != nil {
		_ = os.Remove(backupPath)
		return false
	}

	m.logger.Info("recovered data from orphaned snapshot",
		slog.String("bot_id", botID), slog.String("backup", backupPath))
	return true
}

// restorePreservedIntoSnapshot restores a preserved backup directly into
// the container's snapshot before the task is started. This avoids the
// stop/start cycle that RestorePreservedData (via ImportData) requires.
func (m *Manager) restorePreservedIntoSnapshot(ctx context.Context, botID string) error {
	bp := m.backupPath(botID)
	f, err := os.Open(bp) //nolint:gosec // G304: operator-controlled path
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	containerID := m.containerID(botID)
	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("get container: %w", err)
	}

	mounts, err := m.snapshotMounts(ctx, info)
	if err != nil {
		return err
	}

	if err := mount.WithTempMount(ctx, mounts, func(root string) error {
		dataDir := mountedDataDir(root)
		if err := os.MkdirAll(dataDir, 0o750); err != nil {
			return err
		}
		return untarGzDir(f, dataDir)
	}); err != nil {
		return err
	}

	_ = os.Remove(bp)
	m.logger.Info("restored preserved data into new container",
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
	netResult, err := m.service.SetupNetwork(ctx, ctr.NetworkSetupRequest{
		ContainerID: containerID,
		CNIBinDir:   m.cfg.CNIBinaryDir,
		CNIConfDir:  m.cfg.CNIConfigDir,
	})
	if err != nil {
		m.logger.Warn("network setup after restart failed",
			slog.String("container_id", containerID), slog.Any("error", err))
		return
	}
	m.SetContainerIP(botID, netResult.IP)
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
		info, err := d.Info()
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

		if d.IsDir() {
			return nil
		}

		f, err := os.Open(path) //nolint:gosec // G304: iterating operator-controlled data directory
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = io.Copy(tw, f)
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
