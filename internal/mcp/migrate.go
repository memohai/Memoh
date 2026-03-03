package mcp

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/memohai/memoh/internal/mcp/mcpclient"
)

const migratedSuffix = ".migrated"

// migrateBindMountData copies bot data from the old host bind-mount directory
// into the container via gRPC, then renames the source to prevent re-migration.
// This is a one-time operation for bots that were created before the switch
// from bind mounts to container-local storage.
func (m *Manager) migrateBindMountData(ctx context.Context, botID string) {
	srcDir := filepath.Join(m.dataRoot(), "bots", botID)
	migratedDir := srcDir + migratedSuffix

	if _, err := os.Stat(migratedDir); err == nil {
		return // already migrated
	}
	info, err := os.Stat(srcDir)
	if err != nil || !info.IsDir() {
		return // no old data
	}

	// Quick check: is the directory empty?
	entries, err := os.ReadDir(srcDir)
	if err != nil || len(entries) == 0 {
		return
	}

	client, err := m.grpcPool.Get(ctx, botID)
	if err != nil {
		m.logger.Warn("migrate: cannot connect to container",
			slog.String("bot_id", botID), slog.Any("error", err))
		return
	}

	m.logger.Info("migrating bind-mount data into container",
		slog.String("bot_id", botID), slog.String("src", srcDir))

	var migrated, failed int
	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// A directory walk error means the entire subtree is skipped by
			// WalkDir. Count it as a failure so the src dir is NOT renamed
			// and migration is retried on next start.
			m.logger.Warn("migrate: walk error",
				slog.String("path", path), slog.Any("error", walkErr))
			failed++
			return nil
		}
		rel, relErr := filepath.Rel(srcDir, path)
		if relErr != nil || rel == "." {
			return nil
		}
		if d.IsDir() {
			return nil // dirs are created implicitly by WriteFile
		}

		if err := copyFileToContainer(ctx, client, path, rel); err != nil {
			m.logger.Warn("migrate: copy failed",
				slog.String("file", rel), slog.Any("error", err))
			failed++
			return nil
		}
		migrated++
		return nil
	})
	if err != nil {
		m.logger.Warn("migrate: walk failed", slog.String("bot_id", botID), slog.Any("error", err))
	}

	m.logger.Info("migration complete",
		slog.String("bot_id", botID),
		slog.Int("migrated", migrated),
		slog.Int("failed", failed))

	if failed == 0 {
		if renameErr := os.Rename(srcDir, migratedDir); renameErr != nil {
			m.logger.Warn("migrate: rename src dir failed",
				slog.String("src", srcDir), slog.Any("error", renameErr))
		}
	}
}

func copyFileToContainer(ctx context.Context, client *mcpclient.Client, hostPath, containerRelPath string) error {
	f, err := os.Open(hostPath)
	if err != nil {
		return err
	}
	defer f.Close()

	containerRelPath = strings.ReplaceAll(containerRelPath, string(filepath.Separator), "/")
	_, err = client.WriteRaw(ctx, containerRelPath, f)
	return err
}
