package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/mcp"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/settings"
)

// MemorySettingsReader returns bot settings for memory provider resolution.
type MemorySettingsReader interface {
	GetBot(ctx context.Context, botID string) (settings.Settings, error)
}

type MemoryProvider struct {
	registry *memprovider.Registry
	settings MemorySettingsReader
	logger   *slog.Logger
}

func NewMemoryProvider(log *slog.Logger, registry *memprovider.Registry, settingsSvc MemorySettingsReader) *MemoryProvider {
	if log == nil {
		log = slog.Default()
	}
	return &MemoryProvider{
		registry: registry,
		settings: settingsSvc,
		logger:   log.With(slog.String("tool", "memory")),
	}
}

func (p *MemoryProvider) Tools(ctx context.Context, session SessionContext) ([]sdk.Tool, error) {
	if session.IsSubagent {
		return nil, nil
	}
	provider := p.resolveProvider(ctx, session.BotID)
	if provider == nil {
		return nil, nil
	}
	mcpSession := toMCPSession(session)
	descriptors, err := provider.ListTools(ctx, mcpSession)
	if err != nil {
		return nil, nil
	}
	var tools []sdk.Tool
	for _, desc := range descriptors {
		desc := desc
		prov := provider
		sess := mcpSession
		tools = append(tools, sdk.Tool{
			Name:        desc.Name,
			Description: desc.Description,
			Parameters:  desc.InputSchema,
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				result, err := prov.CallTool(ctx.Context, sess, desc.Name, args)
				if err != nil {
					return nil, err
				}
				return normalizeToolResult(result), nil
			},
		})
	}
	return tools, nil
}

func (p *MemoryProvider) resolveProvider(ctx context.Context, botID string) memprovider.Provider {
	if p.registry == nil || p.settings == nil {
		return nil
	}
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return nil
	}
	botSettings, err := p.settings.GetBot(ctx, botID)
	if err != nil {
		return nil
	}
	providerID := strings.TrimSpace(botSettings.MemoryProviderID)
	if providerID == "" {
		return nil
	}
	prov, err := p.registry.Get(providerID)
	if err != nil {
		return nil
	}
	return prov
}

func toMCPSession(s SessionContext) mcp.ToolSessionContext {
	return mcp.ToolSessionContext{
		BotID:             s.BotID,
		ChatID:            s.ChatID,
		ChannelIdentityID: s.ChannelIdentityID,
		SessionToken:      s.SessionToken,
		CurrentPlatform:   s.CurrentPlatform,
		ReplyTarget:       s.ReplyTarget,
		IsSubagent:        s.IsSubagent,
	}
}

// normalizeToolResult extracts structuredContent from MCP-style results
// so the LLM sees clean data instead of the MCP wrapper.
func normalizeToolResult(result map[string]any) any {
	if result == nil {
		return map[string]any{"ok": true}
	}
	if sc, ok := result["structuredContent"]; ok && sc != nil {
		return sc
	}
	if content, ok := result["content"]; ok {
		if items, ok := content.([]map[string]any); ok && len(items) == 1 {
			if text, ok := items[0]["text"].(string); ok {
				var parsed any
				if json.Unmarshal([]byte(text), &parsed) == nil {
					return parsed
				}
				return text
			}
		}
	}
	return result
}
