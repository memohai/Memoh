package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/settings"
)

func (r *Resolver) loadBotSettings(ctx context.Context, botID string) (settings.Settings, error) {
	if r.settingsService == nil {
		return settings.Settings{}, errors.New("settings service not configured")
	}
	return r.settingsService.GetBot(ctx, botID)
}

func (r *Resolver) loadBotLoopDetectionEnabled(ctx context.Context, botID string) bool {
	if r.queries == nil {
		return false
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return false
	}
	row, err := r.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		r.logger.Debug("failed to load bot metadata for loop detection",
			slog.String("bot_id", botID),
			slog.Any("error", err),
		)
		return false
	}
	return parseLoopDetectionEnabledFromMetadata(row.Metadata)
}

func parseLoopDetectionEnabledFromMetadata(payload []byte) bool {
	if len(payload) == 0 {
		return false
	}
	var metadata map[string]any
	if err := json.Unmarshal(payload, &metadata); err != nil || metadata == nil {
		return false
	}
	features, ok := metadata["features"].(map[string]any)
	if !ok {
		return false
	}
	loopDetection, ok := features["loop_detection"].(map[string]any)
	if !ok {
		return false
	}
	enabled, ok := loopDetection["enabled"].(bool)
	if !ok {
		return false
	}
	return enabled
}
