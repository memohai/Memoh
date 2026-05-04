package command

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/providers"
)

func (h *Handler) buildCompactGroup() *CommandGroup {
	g := newCommandGroup("compact", "Compact conversation context")
	g.DefaultAction = "run"
	g.Register(SubCommand{
		Name:    "run",
		Usage:   "run - Compact the current session's context immediately",
		IsWrite: true,
		Handler: func(cc CommandContext) (string, error) {
			if h.compactionService == nil {
				return "Compaction service is not available.", nil
			}
			sessionID := cc.SessionID
			if sessionID == "" {
				botUUID, err := db.ParseUUID(cc.BotID)
				if err != nil {
					return "", fmt.Errorf("invalid bot id: %w", err)
				}
				latestUUID, err := h.queries.GetLatestSessionIDByBot(cc.Ctx, botUUID)
				if err != nil {
					return "No active session found.", nil
				}
				sessionID = uuid.UUID(latestUUID.Bytes).String()
			}

			cfg, err := h.buildCompactConfig(cc, sessionID)
			if err != nil {
				return "", err
			}

			if err := h.compactionService.RunCompactionSync(cc.Ctx, cfg); err != nil {
				return "", fmt.Errorf("compaction failed: %w", err)
			}
			return "Context compaction completed successfully.", nil
		},
	})
	return g
}

func (h *Handler) buildCompactConfig(cc CommandContext, sessionID string) (compaction.TriggerConfig, error) {
	botSettings, err := h.settingsService.GetBot(cc.Ctx, cc.BotID)
	if err != nil {
		return compaction.TriggerConfig{}, fmt.Errorf("failed to load settings: %w", err)
	}
	modelID := botSettings.CompactionModelID
	if modelID == "" {
		modelID = botSettings.ChatModelID
	}
	if modelID == "" {
		return compaction.TriggerConfig{}, errors.New("no compaction or chat model configured for this bot")
	}

	compactModel, err := h.modelsService.GetByID(cc.Ctx, modelID)
	if err != nil {
		return compaction.TriggerConfig{}, fmt.Errorf("failed to load compaction model: %w", err)
	}
	compactProvider, err := models.FetchProviderByID(cc.Ctx, h.sqlcQueries, compactModel.ProviderID)
	if err != nil {
		return compaction.TriggerConfig{}, fmt.Errorf("failed to load provider: %w", err)
	}
	creds, err := h.providersService.ResolveModelCredentials(cc.Ctx, compactProvider)
	if err != nil {
		return compaction.TriggerConfig{}, fmt.Errorf("failed to resolve credentials: %w", err)
	}

	cfg := compaction.TriggerConfig{
		BotID:            cc.BotID,
		SessionID:        sessionID,
		ModelID:          compactModel.ModelID,
		ClientType:       compactProvider.ClientType,
		APIKey:           creds.APIKey,
		CodexAccountID:   creds.CodexAccountID,
		BaseURL:          providers.ProviderConfigString(compactProvider, "base_url"),
		Ratio:            100,
		TotalInputTokens: 1,
		PromptCacheTTL:   providers.ProviderConfigString(compactProvider, "prompt_cache_ttl"),
	}
	if compactModel.Config.ContextWindow != nil && *compactModel.Config.ContextWindow > 0 {
		cfg.MaxCompactTokens = *compactModel.Config.ContextWindow * 90 / 100
	}
	return cfg, nil
}
