package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	aclDefaultEffect, err := s.getDefaultEffect(ctx, botID)
	if err != nil {
		return Settings{}, err
	}
	settings.AclDefaultEffect = aclDefaultEffect
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
	aclDefaultEffect, err := s.getDefaultEffect(ctx, botID)
	if err != nil {
		return Settings{}, err
	}
	current := normalizeBotSetting(botRow.Language, aclDefaultEffect, botRow.ReasoningEnabled, botRow.ReasoningEffort, botRow.HeartbeatEnabled, botRow.HeartbeatInterval, botRow.CompactionEnabled, botRow.CompactionThreshold, botRow.CompactionRatio)
	if strings.TrimSpace(req.Language) != "" {
		current.Language = strings.TrimSpace(req.Language)
	}
	if effect := strings.TrimSpace(req.AclDefaultEffect); effect != "" {
		current.AclDefaultEffect = effect
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
	if req.CompactionEnabled != nil {
		current.CompactionEnabled = *req.CompactionEnabled
	}
	if req.CompactionThreshold != nil && *req.CompactionThreshold >= 0 {
		current.CompactionThreshold = *req.CompactionThreshold
	}
	if req.CompactionRatio != nil && *req.CompactionRatio >= 1 && *req.CompactionRatio <= 100 {
		current.CompactionRatio = *req.CompactionRatio
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
	compactionModelUUID := pgtype.UUID{}
	if req.CompactionModelID != nil {
		if value := strings.TrimSpace(*req.CompactionModelID); value != "" {
			modelID, err := s.resolveModelUUID(ctx, value)
			if err != nil {
				return Settings{}, err
			}
			compactionModelUUID = modelID
		}
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
	updated, err := s.queries.UpsertBotSettings(ctx, sqlc.UpsertBotSettingsParams{
		ID:                  pgID,
		Language:            current.Language,
		ReasoningEnabled:    current.ReasoningEnabled,
		ReasoningEffort:     current.ReasoningEffort,
		HeartbeatEnabled:    current.HeartbeatEnabled,
		HeartbeatInterval:   int32(current.HeartbeatInterval), //nolint:gosec // bounded by positive-only setter above
		HeartbeatPrompt:     "",
		CompactionEnabled:   current.CompactionEnabled,
		CompactionThreshold: int32(current.CompactionThreshold), //nolint:gosec // bounded by non-negative setter above
		CompactionRatio:     int32(current.CompactionRatio),     //nolint:gosec // bounded 1-100 above
		ChatModelID:         chatModelUUID,
		HeartbeatModelID:    heartbeatModelUUID,
		CompactionModelID:   compactionModelUUID,
		TitleModelID:        titleModelUUID,
		SearchProviderID:    searchProviderUUID,
		MemoryProviderID:    memoryProviderUUID,
		TtsModelID:          ttsModelUUID,
		BrowserContextID:    browserContextUUID,
	})
	if err != nil {
		return Settings{}, err
	}
	createdByUserID := ""
	if botRow.OwnerUserID.Valid {
		createdByUserID = uuid.UUID(botRow.OwnerUserID.Bytes).String()
	}
	_ = createdByUserID
	if err := s.setDefaultEffect(ctx, botID, current.AclDefaultEffect); err != nil {
		return Settings{}, err
	}
	settings := normalizeBotSettingsWriteRow(updated)
	settings.AclDefaultEffect = current.AclDefaultEffect
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
	return nil
}

func normalizeBotSetting(language string, aclDefaultEffect string, reasoningEnabled bool, reasoningEffort string, heartbeatEnabled bool, heartbeatInterval int32, compactionEnabled bool, compactionThreshold int32, compactionRatio int32) Settings {
	settings := Settings{
		Language:            strings.TrimSpace(language),
		AclDefaultEffect:    strings.TrimSpace(aclDefaultEffect),
		ReasoningEnabled:    reasoningEnabled,
		ReasoningEffort:     strings.TrimSpace(reasoningEffort),
		HeartbeatEnabled:    heartbeatEnabled,
		HeartbeatInterval:   int(heartbeatInterval),
		CompactionEnabled:   compactionEnabled,
		CompactionThreshold: int(compactionThreshold),
		CompactionRatio:     int(compactionRatio),
	}
	if settings.Language == "" {
		settings.Language = DefaultLanguage
	}
	if settings.AclDefaultEffect == "" {
		settings.AclDefaultEffect = "deny"
	}
	if !isValidReasoningEffort(settings.ReasoningEffort) {
		settings.ReasoningEffort = DefaultReasoningEffort
	}
	if settings.HeartbeatInterval <= 0 {
		settings.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if settings.CompactionThreshold < 0 {
		settings.CompactionThreshold = 0
	}
	if settings.CompactionRatio < 1 || settings.CompactionRatio > 100 {
		settings.CompactionRatio = 80
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
		row.Language,
		row.ReasoningEnabled,
		row.ReasoningEffort,
		row.HeartbeatEnabled,
		row.HeartbeatInterval,
		row.CompactionEnabled,
		row.CompactionThreshold,
		row.CompactionRatio,
		row.ChatModelID,
		row.HeartbeatModelID,
		row.CompactionModelID,
		row.TitleModelID,
		row.SearchProviderID,
		row.MemoryProviderID,
		row.TtsModelID,
		row.BrowserContextID,
	)
}

func normalizeBotSettingsWriteRow(row sqlc.UpsertBotSettingsRow) Settings {
	return normalizeBotSettingsFields(
		row.Language,
		row.ReasoningEnabled,
		row.ReasoningEffort,
		row.HeartbeatEnabled,
		row.HeartbeatInterval,
		row.CompactionEnabled,
		row.CompactionThreshold,
		row.CompactionRatio,
		row.ChatModelID,
		row.HeartbeatModelID,
		row.CompactionModelID,
		row.TitleModelID,
		row.SearchProviderID,
		row.MemoryProviderID,
		row.TtsModelID,
		row.BrowserContextID,
	)
}

func normalizeBotSettingsFields(
	language string,
	reasoningEnabled bool,
	reasoningEffort string,
	heartbeatEnabled bool,
	heartbeatInterval int32,
	compactionEnabled bool,
	compactionThreshold int32,
	compactionRatio int32,
	chatModelID pgtype.UUID,
	heartbeatModelID pgtype.UUID,
	compactionModelID pgtype.UUID,
	titleModelID pgtype.UUID,
	searchProviderID pgtype.UUID,
	memoryProviderID pgtype.UUID,
	ttsModelID pgtype.UUID,
	browserContextID pgtype.UUID,
) Settings {
	settings := normalizeBotSetting(language, "", reasoningEnabled, reasoningEffort, heartbeatEnabled, heartbeatInterval, compactionEnabled, compactionThreshold, compactionRatio)
	if chatModelID.Valid {
		settings.ChatModelID = uuid.UUID(chatModelID.Bytes).String()
	}
	if heartbeatModelID.Valid {
		settings.HeartbeatModelID = uuid.UUID(heartbeatModelID.Bytes).String()
	}
	if compactionModelID.Valid {
		settings.CompactionModelID = uuid.UUID(compactionModelID.Bytes).String()
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

func (s *Service) getDefaultEffect(ctx context.Context, botID string) (string, error) {
	if s.acl == nil {
		return "deny", nil
	}
	return s.acl.GetDefaultEffect(ctx, botID)
}

func (s *Service) setDefaultEffect(ctx context.Context, botID, effect string) error {
	if s.acl == nil {
		return nil
	}
	if effect == "" {
		return nil
	}
	return s.acl.SetDefaultEffect(ctx, botID, effect)
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
