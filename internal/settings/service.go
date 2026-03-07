package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

var (
	ErrPersonalBotGuestAccessUnsupported = errors.New("personal bots do not support guest access")
	ErrModelIDAmbiguous                  = errors.New("model_id is ambiguous across providers")
	ErrInvalidModelRef                   = errors.New("invalid model reference")
)

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
		return Settings{}, errors.New("settings queries not configured")
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

	current := normalizeBotSetting(botRow.MaxContextLoadTime, botRow.MaxContextTokens, botRow.MaxInboxItems, botRow.Language, botRow.AllowGuest, botRow.ReasoningEnabled, botRow.ReasoningEffort, botRow.HeartbeatEnabled, botRow.HeartbeatInterval)
	if req.MaxContextLoadTime != nil && *req.MaxContextLoadTime > 0 {
		current.MaxContextLoadTime = *req.MaxContextLoadTime
	}
	if req.MaxContextTokens != nil && *req.MaxContextTokens >= 0 {
		current.MaxContextTokens = *req.MaxContextTokens
	}
	if req.MaxInboxItems != nil && *req.MaxInboxItems >= 0 {
		current.MaxInboxItems = *req.MaxInboxItems
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
	if req.ReasoningEnabled != nil {
		current.ReasoningEnabled = *req.ReasoningEnabled
	}
	if req.ReasoningEffort != nil && isValidReasoningEffort(*req.ReasoningEffort) {
		current.ReasoningEffort = *req.ReasoningEffort
	}
	if req.HeartbeatEnabled != nil {
		current.HeartbeatEnabled = *req.HeartbeatEnabled
	}
	if req.HeartbeatInterval != nil && *req.HeartbeatInterval > 0 {
		current.HeartbeatInterval = *req.HeartbeatInterval
	}
	chatModelUUID := pgtype.UUID{}
	if value := strings.TrimSpace(req.ChatModelID); value != "" {
		modelID, err := s.resolveModelUUID(ctx, value)
		if err != nil {
			return Settings{}, err
		}
		chatModelUUID = modelID
	}
	heartbeatModelUUID := pgtype.UUID{}
	if value := strings.TrimSpace(req.HeartbeatModelID); value != "" {
		modelID, err := s.resolveModelUUID(ctx, value)
		if err != nil {
			return Settings{}, err
		}
		heartbeatModelUUID = modelID
	}
	searchProviderUUID := pgtype.UUID{}
	if value := strings.TrimSpace(req.SearchProviderID); value != "" {
		providerID, err := db.ParseUUID(value)
		if err != nil {
			return Settings{}, err
		}
		searchProviderUUID = providerID
	}
	memoryProviderUUID := pgtype.UUID{}
	if value := strings.TrimSpace(req.MemoryProviderID); value != "" {
		providerID, err := db.ParseUUID(value)
		if err != nil {
			return Settings{}, err
		}
		memoryProviderUUID = providerID
	}
	ttsModelUUID := pgtype.UUID{}
	if value := strings.TrimSpace(req.TtsModelID); value != "" {
		modelID, err := db.ParseUUID(value)
		if err != nil {
			return Settings{}, err
		}
		ttsModelUUID = modelID
	}
	browserContextUUID := pgtype.UUID{}
	if value := strings.TrimSpace(req.BrowserContextID); value != "" {
		ctxID, err := db.ParseUUID(value)
		if err != nil {
			return Settings{}, err
		}
		browserContextUUID = ctxID
	}
	if current.MaxContextLoadTime < math.MinInt32 || current.MaxContextLoadTime > math.MaxInt32 ||
		current.MaxContextTokens < math.MinInt32 || current.MaxContextTokens > math.MaxInt32 ||
		current.MaxInboxItems < math.MinInt32 || current.MaxInboxItems > math.MaxInt32 ||
		current.HeartbeatInterval < math.MinInt32 || current.HeartbeatInterval > math.MaxInt32 {
		return Settings{}, errors.New("settings numeric value out of int32 range")
	}

	updated, err := s.queries.UpsertBotSettings(ctx, sqlc.UpsertBotSettingsParams{
		ID:                 pgID,
		MaxContextLoadTime: int32(current.MaxContextLoadTime),
		MaxContextTokens:   int32(current.MaxContextTokens),
		MaxInboxItems:      int32(current.MaxInboxItems),
		Language:           current.Language,
		AllowGuest:         current.AllowGuest,
		ReasoningEnabled:   current.ReasoningEnabled,
		ReasoningEffort:    current.ReasoningEffort,
		HeartbeatEnabled:   current.HeartbeatEnabled,
		HeartbeatInterval:  int32(current.HeartbeatInterval),
		HeartbeatPrompt:    "",
		ChatModelID:        chatModelUUID,
		HeartbeatModelID:   heartbeatModelUUID,
		SearchProviderID:   searchProviderUUID,
		MemoryProviderID:   memoryProviderUUID,
		TtsModelID:         ttsModelUUID,
		BrowserContextID:   browserContextUUID,
	})
	if err != nil {
		return Settings{}, err
	}
	return normalizeBotSettingsWriteRow(updated), nil
}

func (s *Service) Delete(ctx context.Context, botID string) error {
	if s.queries == nil {
		return errors.New("settings queries not configured")
	}
	pgID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	return s.queries.DeleteSettingsByBotID(ctx, pgID)
}

func normalizeBotSetting(maxContextLoadTime int32, maxContextTokens int32, maxInboxItems int32, language string, allowGuest bool, reasoningEnabled bool, reasoningEffort string, heartbeatEnabled bool, heartbeatInterval int32) Settings {
	settings := Settings{
		MaxContextLoadTime: int(maxContextLoadTime),
		MaxContextTokens:   int(maxContextTokens),
		MaxInboxItems:      int(maxInboxItems),
		Language:           strings.TrimSpace(language),
		AllowGuest:         allowGuest,
		ReasoningEnabled:   reasoningEnabled,
		ReasoningEffort:    strings.TrimSpace(reasoningEffort),
		HeartbeatEnabled:   heartbeatEnabled,
		HeartbeatInterval:  int(heartbeatInterval),
	}
	if settings.MaxContextLoadTime <= 0 {
		settings.MaxContextLoadTime = DefaultMaxContextLoadTime
	}
	if settings.MaxContextTokens < 0 {
		settings.MaxContextTokens = 0
	}
	if settings.MaxInboxItems <= 0 {
		settings.MaxInboxItems = DefaultMaxInboxItems
	}
	if settings.Language == "" {
		settings.Language = DefaultLanguage
	}
	if !isValidReasoningEffort(settings.ReasoningEffort) {
		settings.ReasoningEffort = DefaultReasoningEffort
	}
	if settings.HeartbeatInterval <= 0 {
		settings.HeartbeatInterval = DefaultHeartbeatInterval
	}
	return settings
}

func isValidReasoningEffort(effort string) bool {
	switch effort {
	case "low", "medium", "high":
		return true
	default:
		return false
	}
}

func normalizeBotSettingsReadRow(row sqlc.GetSettingsByBotIDRow) Settings {
	return normalizeBotSettingsFields(
		row.MaxContextLoadTime,
		row.MaxContextTokens,
		row.MaxInboxItems,
		row.Language,
		row.AllowGuest,
		row.ReasoningEnabled,
		row.ReasoningEffort,
		row.HeartbeatEnabled,
		row.HeartbeatInterval,
		row.ChatModelID,
		row.HeartbeatModelID,
		row.SearchProviderID,
		row.MemoryProviderID,
		row.TtsModelID,
		row.BrowserContextID,
	)
}

func normalizeBotSettingsWriteRow(row sqlc.UpsertBotSettingsRow) Settings {
	return normalizeBotSettingsFields(
		row.MaxContextLoadTime,
		row.MaxContextTokens,
		row.MaxInboxItems,
		row.Language,
		row.AllowGuest,
		row.ReasoningEnabled,
		row.ReasoningEffort,
		row.HeartbeatEnabled,
		row.HeartbeatInterval,
		row.ChatModelID,
		row.HeartbeatModelID,
		row.SearchProviderID,
		row.MemoryProviderID,
		row.TtsModelID,
		row.BrowserContextID,
	)
}

func normalizeBotSettingsFields(
	maxContextLoadTime int32,
	maxContextTokens int32,
	maxInboxItems int32,
	language string,
	allowGuest bool,
	reasoningEnabled bool,
	reasoningEffort string,
	heartbeatEnabled bool,
	heartbeatInterval int32,
	chatModelID pgtype.UUID,
	heartbeatModelID pgtype.UUID,
	searchProviderID pgtype.UUID,
	memoryProviderID pgtype.UUID,
	ttsModelID pgtype.UUID,
	browserContextID pgtype.UUID,
) Settings {
	settings := normalizeBotSetting(maxContextLoadTime, maxContextTokens, maxInboxItems, language, allowGuest, reasoningEnabled, reasoningEffort, heartbeatEnabled, heartbeatInterval)
	if chatModelID.Valid {
		settings.ChatModelID = uuid.UUID(chatModelID.Bytes).String()
	}
	if heartbeatModelID.Valid {
		settings.HeartbeatModelID = uuid.UUID(heartbeatModelID.Bytes).String()
	}
	if searchProviderID.Valid {
		settings.SearchProviderID = uuid.UUID(searchProviderID.Bytes).String()
	}
	if memoryProviderID.Valid {
		settings.MemoryProviderID = uuid.UUID(memoryProviderID.Bytes).String()
	}
	if ttsModelID.Valid {
		settings.TtsModelID = uuid.UUID(ttsModelID.Bytes).String()
	}
	if browserContextID.Valid {
		settings.BrowserContextID = uuid.UUID(browserContextID.Bytes).String()
	}
	return settings
}

func (s *Service) resolveModelUUID(ctx context.Context, modelID string) (pgtype.UUID, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return pgtype.UUID{}, fmt.Errorf("%w: model_id is required", ErrInvalidModelRef)
	}

	// Preferred path: when caller already passes the model UUID.
	if parsed, err := db.ParseUUID(modelID); err == nil {
		if _, err := s.queries.GetModelByID(ctx, parsed); err == nil {
			return parsed, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, err
		}
	}

	rows, err := s.queries.ListModelsByModelID(ctx, modelID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if len(rows) == 0 {
		return pgtype.UUID{}, fmt.Errorf("%w: model not found: %s", ErrInvalidModelRef, modelID)
	}
	if len(rows) > 1 {
		return pgtype.UUID{}, fmt.Errorf("%w: %s", ErrModelIDAmbiguous, modelID)
	}
	return rows[0].ID, nil
}
