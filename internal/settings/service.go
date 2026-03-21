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

	"github.com/memohai/memoh/internal/acl"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type Service struct {
	queries *sqlc.Queries
	acl     *acl.Service
	logger  *slog.Logger
}

var (
	ErrModelIDAmbiguous = errors.New("model_id is ambiguous across providers")
	ErrInvalidModelRef  = errors.New("invalid model reference")
)

func NewService(log *slog.Logger, queries *sqlc.Queries, aclService *acl.Service) *Service {
	return &Service{
		queries: queries,
		acl:     aclService,
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
	settings := normalizeBotSettingsReadRow(row)
	allowGuest, err := s.allowGuestEnabled(ctx, botID)
	if err != nil {
		return Settings{}, err
	}
	settings.AllowGuest = allowGuest
	return settings, nil
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
	allowGuest, err := s.allowGuestEnabled(ctx, botID)
	if err != nil {
		return Settings{}, err
	}
	current := normalizeBotSetting(botRow.MaxContextLoadTime, botRow.MaxContextTokens, botRow.Language, allowGuest, botRow.ReasoningEnabled, botRow.ReasoningEffort, botRow.HeartbeatEnabled, botRow.HeartbeatInterval)
	if req.MaxContextLoadTime != nil && *req.MaxContextLoadTime > 0 {
		current.MaxContextLoadTime = *req.MaxContextLoadTime
	}
	if req.MaxContextTokens != nil && *req.MaxContextTokens >= 0 {
		current.MaxContextTokens = *req.MaxContextTokens
	}
	if strings.TrimSpace(req.Language) != "" {
		current.Language = strings.TrimSpace(req.Language)
	}
	if req.AllowGuest != nil {
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
	titleModelUUID := pgtype.UUID{}
	if value := strings.TrimSpace(req.TitleModelID); value != "" {
		modelID, err := s.resolveModelUUID(ctx, value)
		if err != nil {
			return Settings{}, err
		}
		titleModelUUID = modelID
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
		current.HeartbeatInterval < math.MinInt32 || current.HeartbeatInterval > math.MaxInt32 {
		return Settings{}, errors.New("settings numeric value out of int32 range")
	}

	updated, err := s.queries.UpsertBotSettings(ctx, sqlc.UpsertBotSettingsParams{
		ID:                 pgID,
		MaxContextLoadTime: int32(current.MaxContextLoadTime), //nolint:gosec // range validated above
		MaxContextTokens:   int32(current.MaxContextTokens),
		Language:           current.Language,
		ReasoningEnabled:   current.ReasoningEnabled,
		ReasoningEffort:    current.ReasoningEffort,
		HeartbeatEnabled:   current.HeartbeatEnabled,
		HeartbeatInterval:  int32(current.HeartbeatInterval),
		HeartbeatPrompt:    "",
		ChatModelID:        chatModelUUID,
		HeartbeatModelID:   heartbeatModelUUID,
		TitleModelID:       titleModelUUID,
		SearchProviderID:   searchProviderUUID,
		MemoryProviderID:   memoryProviderUUID,
		TtsModelID:         ttsModelUUID,
		BrowserContextID:   browserContextUUID,
	})
	if err != nil {
		return Settings{}, err
	}
	createdByUserID := ""
	if botRow.OwnerUserID.Valid {
		createdByUserID = uuid.UUID(botRow.OwnerUserID.Bytes).String()
	}
	if err := s.setAllowGuest(ctx, botID, createdByUserID, current.AllowGuest); err != nil {
		return Settings{}, err
	}
	settings := normalizeBotSettingsWriteRow(updated)
	settings.AllowGuest = current.AllowGuest
	return settings, nil
}

func (s *Service) Delete(ctx context.Context, botID string) error {
	if s.queries == nil {
		return errors.New("settings queries not configured")
	}
	pgID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	if err := s.queries.DeleteSettingsByBotID(ctx, pgID); err != nil {
		return err
	}
	return s.setAllowGuest(ctx, botID, "", false)
}

func normalizeBotSetting(maxContextLoadTime int32, maxContextTokens int32, language string, allowGuest bool, reasoningEnabled bool, reasoningEffort string, heartbeatEnabled bool, heartbeatInterval int32) Settings {
	settings := Settings{
		MaxContextLoadTime: int(maxContextLoadTime),
		MaxContextTokens:   int(maxContextTokens),
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
		row.Language,
		row.ReasoningEnabled,
		row.ReasoningEffort,
		row.HeartbeatEnabled,
		row.HeartbeatInterval,
		row.ChatModelID,
		row.HeartbeatModelID,
		row.TitleModelID,
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
		row.Language,
		row.ReasoningEnabled,
		row.ReasoningEffort,
		row.HeartbeatEnabled,
		row.HeartbeatInterval,
		row.ChatModelID,
		row.HeartbeatModelID,
		row.TitleModelID,
		row.SearchProviderID,
		row.MemoryProviderID,
		row.TtsModelID,
		row.BrowserContextID,
	)
}

func normalizeBotSettingsFields(
	maxContextLoadTime int32,
	maxContextTokens int32,
	language string,
	reasoningEnabled bool,
	reasoningEffort string,
	heartbeatEnabled bool,
	heartbeatInterval int32,
	chatModelID pgtype.UUID,
	heartbeatModelID pgtype.UUID,
	titleModelID pgtype.UUID,
	searchProviderID pgtype.UUID,
	memoryProviderID pgtype.UUID,
	ttsModelID pgtype.UUID,
	browserContextID pgtype.UUID,
) Settings {
	settings := normalizeBotSetting(maxContextLoadTime, maxContextTokens, language, false, reasoningEnabled, reasoningEffort, heartbeatEnabled, heartbeatInterval)
	if chatModelID.Valid {
		settings.ChatModelID = uuid.UUID(chatModelID.Bytes).String()
	}
	if heartbeatModelID.Valid {
		settings.HeartbeatModelID = uuid.UUID(heartbeatModelID.Bytes).String()
	}
	if titleModelID.Valid {
		settings.TitleModelID = uuid.UUID(titleModelID.Bytes).String()
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

func (s *Service) allowGuestEnabled(ctx context.Context, botID string) (bool, error) {
	if s.acl == nil {
		return false, nil
	}
	return s.acl.AllowGuestEnabled(ctx, botID)
}

func (s *Service) setAllowGuest(ctx context.Context, botID, createdByUserID string, enabled bool) error {
	if s.acl == nil {
		return nil
	}
	return s.acl.SetAllowGuest(ctx, botID, createdByUserID, enabled)
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
