package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

var ErrPersonalBotGuestAccessUnsupported = errors.New("personal bots do not support guest access")

func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "settings")),
	}
}

func (s *Service) GetBot(ctx context.Context, botID string) (Settings, error) {
	pgID, err := db.ParseUUID(botID)
	if err != nil {
		return Settings{}, err
	}
	row, err := s.queries.GetSettingsByBotID(ctx, pgID)
	if err != nil {
		return Settings{}, err
	}
	return normalizeBotSettingsReadRow(row), nil
}

func (s *Service) UpsertBot(ctx context.Context, botID string, req UpsertRequest) (Settings, error) {
	if s.queries == nil {
		return Settings{}, fmt.Errorf("settings queries not configured")
	}
	pgID, err := db.ParseUUID(botID)
	if err != nil {
		return Settings{}, err
	}
	botRow, err := s.queries.GetBotByID(ctx, pgID)
	if err != nil {
		return Settings{}, err
	}
	isPersonalBot := strings.EqualFold(strings.TrimSpace(botRow.Type), "personal")

	current := normalizeBotSetting(botRow.MaxContextLoadTime, botRow.Language, botRow.AllowGuest)
	if req.MaxContextLoadTime != nil && *req.MaxContextLoadTime > 0 {
		current.MaxContextLoadTime = *req.MaxContextLoadTime
	}
	if strings.TrimSpace(req.Language) != "" {
		current.Language = strings.TrimSpace(req.Language)
	}
	if isPersonalBot {
		if req.AllowGuest != nil && *req.AllowGuest {
			return Settings{}, ErrPersonalBotGuestAccessUnsupported
		}
		current.AllowGuest = false
	} else if req.AllowGuest != nil {
		current.AllowGuest = *req.AllowGuest
	}

	chatModelUUID := pgtype.UUID{}
	if value := strings.TrimSpace(req.ChatModelID); value != "" {
		modelID, err := s.resolveModelUUID(ctx, value)
		if err != nil {
			return Settings{}, err
		}
		chatModelUUID = modelID
	}
	memoryModelUUID := pgtype.UUID{}
	if value := strings.TrimSpace(req.MemoryModelID); value != "" {
		modelID, err := s.resolveModelUUID(ctx, value)
		if err != nil {
			return Settings{}, err
		}
		memoryModelUUID = modelID
	}
	embeddingModelUUID := pgtype.UUID{}
	if value := strings.TrimSpace(req.EmbeddingModelID); value != "" {
		modelID, err := s.resolveModelUUID(ctx, value)
		if err != nil {
			return Settings{}, err
		}
		embeddingModelUUID = modelID
	}

	updated, err := s.queries.UpsertBotSettings(ctx, sqlc.UpsertBotSettingsParams{
		ID:                 pgID,
		MaxContextLoadTime: int32(current.MaxContextLoadTime),
		Language:           current.Language,
		AllowGuest:         current.AllowGuest,
		ChatModelID:        chatModelUUID,
		MemoryModelID:      memoryModelUUID,
		EmbeddingModelID:   embeddingModelUUID,
	})
	if err != nil {
		return Settings{}, err
	}
	return normalizeBotSettingsWriteRow(updated), nil
}

func (s *Service) Delete(ctx context.Context, botID string) error {
	if s.queries == nil {
		return fmt.Errorf("settings queries not configured")
	}
	pgID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	return s.queries.DeleteSettingsByBotID(ctx, pgID)
}

func normalizeBotSetting(maxContextLoadTime int32, language string, allowGuest bool) Settings {
	settings := Settings{
		MaxContextLoadTime: int(maxContextLoadTime),
		Language:           strings.TrimSpace(language),
		AllowGuest:         allowGuest,
	}
	if settings.MaxContextLoadTime <= 0 {
		settings.MaxContextLoadTime = DefaultMaxContextLoadTime
	}
	if settings.Language == "" {
		settings.Language = DefaultLanguage
	}
	return settings
}

func normalizeBotSettingsReadRow(row sqlc.GetSettingsByBotIDRow) Settings {
	return normalizeBotSettingsFields(
		row.MaxContextLoadTime,
		row.Language,
		row.AllowGuest,
		row.ChatModelID,
		row.MemoryModelID,
		row.EmbeddingModelID,
	)
}

func normalizeBotSettingsWriteRow(row sqlc.UpsertBotSettingsRow) Settings {
	return normalizeBotSettingsFields(
		row.MaxContextLoadTime,
		row.Language,
		row.AllowGuest,
		row.ChatModelID,
		row.MemoryModelID,
		row.EmbeddingModelID,
	)
}

func normalizeBotSettingsFields(
	maxContextLoadTime int32,
	language string,
	allowGuest bool,
	chatModelID pgtype.Text,
	memoryModelID pgtype.Text,
	embeddingModelID pgtype.Text,
) Settings {
	settings := normalizeBotSetting(maxContextLoadTime, language, allowGuest)
	settings.ChatModelID = strings.TrimSpace(chatModelID.String)
	settings.MemoryModelID = strings.TrimSpace(memoryModelID.String)
	settings.EmbeddingModelID = strings.TrimSpace(embeddingModelID.String)
	return settings
}

func (s *Service) resolveModelUUID(ctx context.Context, modelID string) (pgtype.UUID, error) {
	if strings.TrimSpace(modelID) == "" {
		return pgtype.UUID{}, fmt.Errorf("model_id is required")
	}
	row, err := s.queries.GetModelByModelID(ctx, modelID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return row.ID, nil
}
