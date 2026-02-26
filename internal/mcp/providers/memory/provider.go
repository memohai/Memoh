package memory

import (
	"context"
	"log/slog"
	"strings"

	mcpgw "github.com/memohai/memoh/internal/mcp"
	memprovider "github.com/memohai/memoh/internal/memory/provider"
	"github.com/memohai/memoh/internal/settings"
)

// BotSettingsReader returns bot settings for provider resolution.
type BotSettingsReader interface {
	GetBot(ctx context.Context, botID string) (settings.Settings, error)
}

// Executor proxies MCP tool calls to the memory provider configured for each bot.
// If a bot has no memory provider, no tools are returned.
type Executor struct {
	registry        *memprovider.Registry
	settingsService BotSettingsReader
	logger          *slog.Logger
}

func NewExecutor(log *slog.Logger, registry *memprovider.Registry, settingsService BotSettingsReader) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{
		registry:        registry,
		settingsService: settingsService,
		logger:          log.With(slog.String("provider", "memory_tool")),
	}
}

func (e *Executor) resolveProvider(ctx context.Context, botID string) memprovider.Provider {
	if e.registry == nil || e.settingsService == nil {
		return nil
	}
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return nil
	}
	botSettings, err := e.settingsService.GetBot(ctx, botID)
	if err != nil {
		return nil
	}
	providerID := strings.TrimSpace(botSettings.MemoryProviderID)
	if providerID == "" {
		return nil
	}
	p, err := e.registry.Get(providerID)
	if err != nil {
		e.logger.Warn("memory provider lookup failed", slog.String("provider_id", providerID), slog.Any("error", err))
		return nil
	}
	return p
}

func (e *Executor) ListTools(ctx context.Context, session mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	p := e.resolveProvider(ctx, session.BotID)
	if p == nil {
		return []mcpgw.ToolDescriptor{}, nil
	}
	return p.ListTools(ctx, session)
}

func (e *Executor) CallTool(ctx context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	p := e.resolveProvider(ctx, session.BotID)
	if p == nil {
		return mcpgw.BuildToolErrorResult("memory not enabled for this bot"), nil
	}
	return p.CallTool(ctx, session, toolName, arguments)
}
