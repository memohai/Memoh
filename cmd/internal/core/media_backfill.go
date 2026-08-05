package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/memohai/memoh/internal/config"
	storagebackfill "github.com/memohai/memoh/internal/storage/backfill"
	storagefactory "github.com/memohai/memoh/internal/storage/factory"
	"github.com/memohai/memoh/internal/workspace"
)

type mediaBackfill struct {
	enabled bool
	runner  *storagebackfill.Runner
	sources []storagebackfill.Source
	logger  *slog.Logger
}

func provideMediaBackfill(log *slog.Logger, manager *workspace.Manager, cfg config.Config) (*mediaBackfill, error) {
	backfill := &mediaBackfill{logger: log}
	if cfg.Storage.ProviderOrDefault() != config.StorageProviderS3 || !cfg.Storage.S3.BackfillOnStart {
		return backfill, nil
	}
	target, err := storagefactory.NewS3(cfg.Storage.S3)
	if err != nil {
		return nil, fmt.Errorf("configure media storage backfill target: %w", err)
	}
	runner, err := storagebackfill.New(target, log)
	if err != nil {
		return nil, err
	}
	dataRoot := strings.TrimSpace(cfg.Workspace.DataRoot)
	if dataRoot == "" {
		dataRoot = config.DefaultDataRoot
	}
	backfill.enabled = true
	backfill.runner = runner
	backfill.sources = []storagebackfill.Source{
		storagebackfill.NewFilesystemSource("host-filesystem", filepath.Join(dataRoot, "media")),
		&workspaceMediaBackfillSource{manager: manager},
	}
	return backfill, nil
}

func (b *mediaBackfill) Run(ctx context.Context) {
	if b == nil || !b.enabled || b.runner == nil {
		return
	}
	b.logger.Info("media storage backfill started")
	result, err := b.runner.Run(ctx, b.sources...)
	if err != nil {
		b.logger.Error("media storage backfill incomplete; rerun with backfill_on_start enabled",
			slog.Int64("scanned", result.Scanned),
			slog.Int64("copied", result.Copied),
			slog.Int64("existing", result.Existing),
			slog.Int64("failed", result.Failed),
			slog.Any("error", err),
		)
		return
	}
	b.logger.Info("media storage backfill completed; backfill_on_start can now be disabled",
		slog.Int64("scanned", result.Scanned),
		slog.Int64("copied", result.Copied),
		slog.Int64("existing", result.Existing),
		slog.Int64("ignored", result.Ignored),
		slog.Int64("bytes_copied", result.BytesCopied),
	)
}

type workspaceMediaBackfillSource struct {
	manager *workspace.Manager
}

func (*workspaceMediaBackfillSource) Name() string { return "workspace-filesystem" }

func (s *workspaceMediaBackfillSource) Walk(ctx context.Context, visit storagebackfill.Visitor) error {
	botIDs, err := s.manager.ListBots(ctx)
	if err != nil {
		return fmt.Errorf("list workspaces: %w", err)
	}
	sort.Strings(botIDs)
	var errs []error
	for _, botID := range botIDs {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := s.manager.WalkLegacyMedia(ctx, botID, func(object workspace.LegacyMediaObject) error {
			return visit(storagebackfill.Object{
				Key:       object.Key,
				SizeBytes: object.SizeBytes,
				Open:      object.Open,
			})
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("workspace %s: %w", botID, err))
		}
	}
	return errors.Join(errs...)
}
