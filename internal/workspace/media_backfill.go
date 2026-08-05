package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/containerd/containerd/v2/core/mount"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

const legacyMediaDir = "media"

// LegacyMediaObject describes an immutable object under /data/media. Open must
// be called synchronously from the visitor while WalkLegacyMedia is active.
type LegacyMediaObject struct {
	Key       string
	SizeBytes int64
	Open      func(context.Context) (io.ReadCloser, error)
}

// WalkLegacyMedia enumerates the media left in one native workspace before an
// S3 cutover. Running workspaces are read through the bridge. A stopped
// containerd workspace is read from its snapshot; backends without snapshot
// mounts are temporarily started and restored to the stopped state afterward.
func (m *Manager) WalkLegacyMedia(ctx context.Context, botID string, visit func(LegacyMediaObject) error) error {
	if visit == nil {
		return errors.New("legacy media visitor is required")
	}
	ref, err := m.loadLockedContainer(ctx, botID)
	if err != nil {
		return fmt.Errorf("load workspace: %w", err)
	}
	wasRunning := m.isTaskRunning(ctx, ref.containerID)
	if !wasRunning {
		mounts, mountErr := m.snapshotMounts(ctx, ref.info)
		if mountErr == nil {
			defer ref.Close()
			return mount.WithReadonlyTempMount(ctx, mounts, func(root string) error {
				return walkMountedLegacyMedia(ctx, root, botID, visit)
			})
		}
		if !errors.Is(mountErr, errMountNotSupported) {
			ref.Close()
			return fmt.Errorf("mount stopped workspace: %w", mountErr)
		}
	}
	ref.Close()
	return m.walkLegacyMediaViaBridge(ctx, botID, wasRunning, visit)
}

func walkMountedLegacyMedia(ctx context.Context, snapshotRoot, botID string, visit func(LegacyMediaObject) error) error {
	mediaRoot := filepath.Join(mountedDataDir(snapshotRoot), legacyMediaDir)
	err := filepath.WalkDir(mediaRoot, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(mediaRoot, filePath)
		if err != nil {
			return err
		}
		key := path.Join(botID, filepath.ToSlash(rel))
		return visit(LegacyMediaObject{
			Key:       key,
			SizeBytes: info.Size(),
			Open: func(context.Context) (io.ReadCloser, error) {
				return os.Open(filePath) //nolint:gosec // path is produced by WalkDir below the mounted media root
			},
		})
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("walk mounted media: %w", err)
	}
	return nil
}

func (m *Manager) walkLegacyMediaViaBridge(ctx context.Context, botID string, wasRunning bool, visit func(LegacyMediaObject) error) (retErr error) {
	if !wasRunning {
		m.logger.Info("media backfill temporarily starting stopped workspace", slog.String("bot_id", botID))
		if err := m.EnsureNativeRunning(ctx, botID); err != nil {
			return fmt.Errorf("temporarily start workspace: %w", err)
		}
		defer func() {
			stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			if err := m.StopBot(stopCtx, botID); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("restore stopped workspace state: %w", err))
				return
			}
			m.logger.Info("media backfill restored stopped workspace state", slog.String("bot_id", botID))
		}()
	}
	if err := m.WaitForWorkspaceReady(ctx, botID); err != nil {
		return err
	}
	client, err := m.nativeMCPClient(ctx, botID)
	if err != nil {
		return fmt.Errorf("connect workspace bridge: %w", err)
	}
	entries, err := client.ListDirAll(ctx, legacyMediaDir, true)
	if errors.Is(err, bridge.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list workspace media: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].GetPath() < entries[j].GetPath() })
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.GetIsDir() || isWorkspaceArchiveSymlinkMode(entry.GetMode()) {
			continue
		}
		rel := strings.TrimSpace(strings.ReplaceAll(entry.GetPath(), "\\", "/"))
		rel = strings.TrimPrefix(rel, "/")
		clean := path.Clean(rel)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != rel {
			return fmt.Errorf("workspace returned invalid media path %q", entry.GetPath())
		}
		containerPath := path.Join(legacyMediaDir, clean)
		if err := visit(LegacyMediaObject{
			Key:       path.Join(botID, clean),
			SizeBytes: entry.GetSize(),
			Open: func(openCtx context.Context) (io.ReadCloser, error) {
				return client.ReadRaw(openCtx, containerPath)
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
