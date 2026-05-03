package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/acl"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	netctl "github.com/memohai/memoh/internal/network"
	tzutil "github.com/memohai/memoh/internal/timezone"
)

type Service struct {
	queries dbstore.Queries
	acl     *acl.Service
	network *netctl.Service
	logger  *slog.Logger
}

var (
	ErrModelIDAmbiguous = errors.New("model_id is ambiguous across providers")
	ErrInvalidModelRef  = errors.New("invalid model reference")
)

func NewService(log *slog.Logger, queries dbstore.Queries, aclService *acl.Service, networkService *netctl.Service) *Service {
	return &Service{
		queries: queries,
		acl:     aclService,
		network: networkService,
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
	overlayBindingRow, err := s.queries.GetBotOverlayConfig(ctx, pgID)
	if err != nil {
		return Settings{}, err
	}
	previousOverlayConfig := settingsOverlayConfigFromRow(overlayBindingRow)
	if s.network != nil {
		if resolvedPrevious, resolveErr := s.network.GetBotConfig(ctx, botID); resolveErr == nil {
			previousOverlayConfig = resolvedPrevious
		}
	}
	aclDefaultEffect, err := s.getDefaultEffect(ctx, botID)
	if err != nil {
		return Settings{}, err
	}
	current := normalizeBotSetting(botRow.Language, aclDefaultEffect, botRow.ReasoningEnabled, botRow.ReasoningEffort, botRow.HeartbeatEnabled, botRow.HeartbeatInterval, botRow.CompactionEnabled, botRow.CompactionThreshold, botRow.CompactionRatio)
	if settingsRow, err := s.queries.GetSettingsByBotID(ctx, pgID); err == nil {
		current.ToolApprovalConfig = parseToolApprovalConfig(settingsRow.ToolApprovalConfig)
		current.DisplayEnabled = settingsRow.DisplayEnabled
	}
	current.OverlayEnabled = overlayBindingRow.OverlayEnabled
	current.OverlayProvider = strings.TrimSpace(overlayBindingRow.OverlayProvider)
	current.OverlayConfig = normalizeJSONObject(overlayBindingRow.OverlayConfig)
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
	if req.PersistFullToolResults != nil {
		current.PersistFullToolResults = *req.PersistFullToolResults
	}
	if req.ShowToolCallsInIM != nil {
		current.ShowToolCallsInIM = *req.ShowToolCallsInIM
	}
	if req.ToolApprovalConfig != nil {
		current.ToolApprovalConfig = NormalizeToolApprovalConfig(*req.ToolApprovalConfig)
	}
	if req.DisplayEnabled != nil {
		current.DisplayEnabled = *req.DisplayEnabled
	}
	timezoneValue := pgtype.Text{}
	if req.Timezone != nil {
		normalized, err := normalizeOptionalTimezone(*req.Timezone)
		if err != nil {
			return Settings{}, err
		}
		timezoneValue = normalized
	}
	if req.OverlayEnabled != nil {
		current.OverlayEnabled = *req.OverlayEnabled
	}
	if req.OverlayProvider != nil {
		current.OverlayProvider = strings.TrimSpace(*req.OverlayProvider)
	}
	if req.OverlayConfig != nil {
		current.OverlayConfig = req.OverlayConfig
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
	imageModelUUID := pgtype.UUID{}
	if value := strings.TrimSpace(req.ImageModelID); value != "" {
		modelID, err := s.resolveModelUUID(ctx, value)
		if err != nil {
			return Settings{}, err
		}
		imageModelUUID = modelID
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
	transcriptionModelUUID := pgtype.UUID{}
	if value := strings.TrimSpace(req.TranscriptionModelID); value != "" {
		modelID, err := db.ParseUUID(value)
		if err != nil {
			return Settings{}, err
		}
		transcriptionModelUUID = modelID
	}
	browserContextUUID := pgtype.UUID{}
	if value := strings.TrimSpace(req.BrowserContextID); value != "" {
		ctxID, err := db.ParseUUID(value)
		if err != nil {
			return Settings{}, err
		}
		browserContextUUID = ctxID
	}
	toolApprovalConfig, err := json.Marshal(current.ToolApprovalConfig)
	if err != nil {
		return Settings{}, err
	}

	normalizedNetwork, err := s.normalizeOverlayConfig(current)
	if err != nil {
		return Settings{}, err
	}
	nextOverlayConfig := settingsOverlayConfigFromSettings(normalizedNetwork)
	networkChanged := s.network != nil && !overlayConfigsEqual(previousOverlayConfig, nextOverlayConfig)
	rollbackNetworkChange := func(cause error) error {
		if !networkChanged {
			return cause
		}
		if rollbackErr := s.reconcileBotNetwork(ctx, botID, nextOverlayConfig, previousOverlayConfig); rollbackErr != nil {
			return errors.Join(cause, fmt.Errorf("network rollback failed: %w", rollbackErr))
		}
		return cause
	}
	if networkChanged {
		if err := s.reconcileBotNetwork(ctx, botID, previousOverlayConfig, nextOverlayConfig); err != nil {
			if rollbackErr := s.reconcileBotNetwork(ctx, botID, nextOverlayConfig, previousOverlayConfig); rollbackErr != nil {
				return Settings{}, errors.Join(fmt.Errorf("reconcile bot network: %w", err), fmt.Errorf("network rollback failed: %w", rollbackErr))
			}
			return Settings{}, err
		}
	}
	overlayConfigJSON, err := json.Marshal(normalizedNetwork.OverlayConfig)
	if err != nil {
		return Settings{}, rollbackNetworkChange(fmt.Errorf("marshal network config: %w", err))
	}
	updated, err := s.queries.UpsertBotSettings(ctx, sqlc.UpsertBotSettingsParams{
		ID:                     pgID,
		Timezone:               timezoneValue,
		Language:               current.Language,
		ReasoningEnabled:       current.ReasoningEnabled,
		ReasoningEffort:        current.ReasoningEffort,
		HeartbeatEnabled:       current.HeartbeatEnabled,
		HeartbeatInterval:      int32(current.HeartbeatInterval), //nolint:gosec // bounded by positive-only setter above
		HeartbeatPrompt:        "",
		CompactionEnabled:      current.CompactionEnabled,
		CompactionThreshold:    int32(current.CompactionThreshold), //nolint:gosec // bounded by non-negative setter above
		CompactionRatio:        int32(current.CompactionRatio),     //nolint:gosec // bounded 1-100 above
		ChatModelID:            chatModelUUID,
		HeartbeatModelID:       heartbeatModelUUID,
		CompactionModelID:      compactionModelUUID,
		TitleModelID:           titleModelUUID,
		ImageModelID:           imageModelUUID,
		SearchProviderID:       searchProviderUUID,
		MemoryProviderID:       memoryProviderUUID,
		TtsModelID:             ttsModelUUID,
		TranscriptionModelID:   transcriptionModelUUID,
		BrowserContextID:       browserContextUUID,
		PersistFullToolResults: current.PersistFullToolResults,
		ShowToolCallsInIm:      current.ShowToolCallsInIM,
		ToolApprovalConfig:     toolApprovalConfig,
		DisplayEnabled:         current.DisplayEnabled,
		OverlayProvider:        normalizedNetwork.OverlayProvider,
		OverlayEnabled:         normalizedNetwork.OverlayEnabled,
		OverlayConfig:          overlayConfigJSON,
	})
	if err != nil {
		return Settings{}, rollbackNetworkChange(err)
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
		ToolApprovalConfig:  DefaultToolApprovalConfig(),
	}
	if settings.Language == "" {
		settings.Language = DefaultLanguage
	}
	if settings.AclDefaultEffect == "" {
		settings.AclDefaultEffect = "allow"
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
	settings.OverlayConfig = map[string]any{}
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
		row.Timezone,
		row.ChatModelID,
		row.HeartbeatModelID,
		row.CompactionModelID,
		row.TitleModelID,
		row.ImageModelID,
		row.SearchProviderID,
		row.MemoryProviderID,
		row.TtsModelID,
		row.TranscriptionModelID,
		row.BrowserContextID,
		row.PersistFullToolResults,
		row.ShowToolCallsInIm,
		row.ToolApprovalConfig,
		row.DisplayEnabled,
		row.OverlayProvider,
		row.OverlayEnabled,
		row.OverlayConfig,
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
		row.Timezone,
		row.ChatModelID,
		row.HeartbeatModelID,
		row.CompactionModelID,
		row.TitleModelID,
		row.ImageModelID,
		row.SearchProviderID,
		row.MemoryProviderID,
		row.TtsModelID,
		row.TranscriptionModelID,
		row.BrowserContextID,
		row.PersistFullToolResults,
		row.ShowToolCallsInIm,
		row.ToolApprovalConfig,
		row.DisplayEnabled,
		row.OverlayProvider,
		row.OverlayEnabled,
		row.OverlayConfig,
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
	timezone pgtype.Text,
	chatModelID pgtype.UUID,
	heartbeatModelID pgtype.UUID,
	compactionModelID pgtype.UUID,
	titleModelID pgtype.UUID,
	imageModelID pgtype.UUID,
	searchProviderID pgtype.UUID,
	memoryProviderID pgtype.UUID,
	ttsModelID pgtype.UUID,
	transcriptionModelID pgtype.UUID,
	browserContextID pgtype.UUID,
	persistFullToolResults bool,
	showToolCallsInIM bool,
	toolApprovalConfig []byte,
	displayEnabled bool,
	overlayProvider string,
	overlayEnabled bool,
	overlayConfig []byte,
) Settings {
	settings := normalizeBotSetting(language, "", reasoningEnabled, reasoningEffort, heartbeatEnabled, heartbeatInterval, compactionEnabled, compactionThreshold, compactionRatio)
	if timezone.Valid {
		settings.Timezone = timezone.String
	}
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
	if imageModelID.Valid {
		settings.ImageModelID = uuid.UUID(imageModelID.Bytes).String()
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
	if transcriptionModelID.Valid {
		settings.TranscriptionModelID = uuid.UUID(transcriptionModelID.Bytes).String()
	}
	if browserContextID.Valid {
		settings.BrowserContextID = uuid.UUID(browserContextID.Bytes).String()
	}
	settings.PersistFullToolResults = persistFullToolResults
	settings.ShowToolCallsInIM = showToolCallsInIM
	settings.ToolApprovalConfig = parseToolApprovalConfig(toolApprovalConfig)
	settings.DisplayEnabled = displayEnabled
	settings.OverlayProvider = strings.TrimSpace(overlayProvider)
	settings.OverlayEnabled = overlayEnabled
	settings.OverlayConfig = normalizeJSONObject(overlayConfig)
	return settings
}

func parseToolApprovalConfig(raw []byte) ToolApprovalConfig {
	if len(raw) == 0 {
		return DefaultToolApprovalConfig()
	}
	var cfg ToolApprovalConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return DefaultToolApprovalConfig()
	}
	return NormalizeToolApprovalConfig(cfg)
}

func normalizeJSONObject(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func (s *Service) normalizeOverlayConfig(current Settings) (Settings, error) {
	current.OverlayProvider = strings.TrimSpace(current.OverlayProvider)
	current.OverlayConfig = cloneSettingsMap(current.OverlayConfig)
	if current.OverlayProvider == "" {
		if current.OverlayEnabled {
			return Settings{}, errors.New("network provider is required when network is enabled")
		}
		current.OverlayConfig = map[string]any{}
		return current, nil
	}
	if s.network == nil {
		return Settings{}, errors.New("network service not configured")
	}
	cfg, err := s.network.PrepareBotConfigForWrite(netctl.BotOverlayConfig{
		Enabled:  current.OverlayEnabled,
		Provider: current.OverlayProvider,
		Config:   current.OverlayConfig,
	})
	if err != nil {
		return Settings{}, err
	}
	current.OverlayEnabled = cfg.Enabled
	current.OverlayProvider = cfg.Provider
	current.OverlayConfig = cfg.Config
	return current, nil
}

func cloneSettingsMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func settingsOverlayConfigFromRow(row sqlc.GetBotOverlayConfigRow) netctl.BotOverlayConfig {
	return netctl.BotOverlayConfig{
		Enabled:  row.OverlayEnabled,
		Provider: strings.TrimSpace(row.OverlayProvider),
		Config:   normalizeJSONObject(row.OverlayConfig),
	}
}

func settingsOverlayConfigFromSettings(current Settings) netctl.BotOverlayConfig {
	return netctl.BotOverlayConfig{
		Enabled:  current.OverlayEnabled,
		Provider: strings.TrimSpace(current.OverlayProvider),
		Config:   cloneSettingsMap(current.OverlayConfig),
	}
}

func overlayConfigsEqual(left, right netctl.BotOverlayConfig) bool {
	if left.Enabled != right.Enabled || left.Provider != right.Provider {
		return false
	}
	return jsonEqual(left.Config, right.Config)
}

func (s *Service) reconcileBotNetwork(ctx context.Context, botID string, previous, next netctl.BotOverlayConfig) error {
	if s.network == nil || overlayConfigsEqual(previous, next) {
		return nil
	}
	return s.network.ReconcileBot(ctx, botID, previous, next)
}

func jsonEqual(left, right map[string]any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return string(leftJSON) == string(rightJSON)
}

func (s *Service) getDefaultEffect(ctx context.Context, botID string) (string, error) {
	if s.acl == nil {
		return "allow", nil
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

func normalizeOptionalTimezone(raw string) (pgtype.Text, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return pgtype.Text{}, nil
	}
	loc, _, err := tzutil.Resolve(normalized)
	if err != nil {
		return pgtype.Text{}, fmt.Errorf("invalid timezone: %w", err)
	}
	return pgtype.Text{String: loc.String(), Valid: true}, nil
}
