// Package directory provides the MCP directory provider (channel user lookup).
package directory

import (
	"context"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/channel"
	mcpgw "github.com/memohai/memoh/internal/mcp"
)

const toolLookupChannelUser = "lookup_channel_user"

// ConfigResolver resolves effective channel config for a bot (used to call directory APIs).
type ConfigResolver interface {
	ResolveEffectiveConfig(ctx context.Context, botID string, channelType channel.Type) (channel.Config, error)
}

// ChannelTypeResolver parses platform name to channel type.
type ChannelTypeResolver interface {
	ParseChannelType(raw string) (channel.Type, error)
}

// Executor exposes channel directory lookup as an MCP tool for the LLM.
type Executor struct {
	registry       *channel.Registry
	configResolver ConfigResolver
	typeResolver   ChannelTypeResolver
	logger         *slog.Logger
}

// NewExecutor creates a directory tool executor.
func NewExecutor(log *slog.Logger, registry *channel.Registry, configResolver ConfigResolver, typeResolver ChannelTypeResolver) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{
		registry:       registry,
		configResolver: configResolver,
		typeResolver:   typeResolver,
		logger:         log.With(slog.String("provider", "directory_tool")),
	}
}

// ListTools returns the lookup_channel_user tool descriptor.
func (p *Executor) ListTools(_ context.Context, _ mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	if p.registry == nil || p.configResolver == nil || p.typeResolver == nil {
		return []mcpgw.ToolDescriptor{}, nil
	}
	return []mcpgw.ToolDescriptor{
		{
			Name:        toolLookupChannelUser,
			Description: "Look up a user or group on a channel by platform identifier (e.g. open_id, user_id, chat_id). Returns display name, handle, and id.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"platform": map[string]any{
						"type":        "string",
						"description": "Channel platform (e.g. feishu, telegram). Defaults to current session platform.",
					},
					"bot_id": map[string]any{
						"type":        "string",
						"description": "Bot ID. Defaults to current session bot.",
					},
					"input": map[string]any{
						"type":        "string",
						"description": "Platform-specific identifier: user id (feishu open_id/user_id, telegram chat_id for private), or \"chat_id:user_id\" for a user in a group (telegram).",
					},
					"kind": map[string]any{
						"type":        "string",
						"description": "Entry kind: \"user\" or \"group\". Default \"user\".",
						"enum":        []any{"user", "group"},
					},
				},
				"required": []string{"input"},
			},
		},
	}, nil
}

// CallTool runs lookup_channel_user.
func (p *Executor) CallTool(ctx context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if toolName != toolLookupChannelUser {
		return nil, mcpgw.ErrToolNotFound
	}
	if p.registry == nil || p.configResolver == nil || p.typeResolver == nil {
		return mcpgw.BuildToolErrorResult("directory lookup not available"), nil
	}

	botID := mcpgw.FirstStringArg(arguments, "bot_id")
	if botID == "" {
		botID = strings.TrimSpace(session.BotID)
	}
	if botID == "" {
		return mcpgw.BuildToolErrorResult("bot_id is required"), nil
	}
	if strings.TrimSpace(session.BotID) != "" && botID != strings.TrimSpace(session.BotID) {
		return mcpgw.BuildToolErrorResult("bot_id mismatch"), nil
	}

	platform := mcpgw.FirstStringArg(arguments, "platform")
	if platform == "" {
		platform = strings.TrimSpace(session.CurrentPlatform)
	}
	if platform == "" {
		return mcpgw.BuildToolErrorResult("platform is required"), nil
	}
	channelType, err := p.typeResolver.ParseChannelType(platform)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	dirAdapter, ok := p.registry.DirectoryAdapter(channelType)
	if !ok || dirAdapter == nil {
		return mcpgw.BuildToolErrorResult("channel does not support directory lookup"), nil
	}

	input := strings.TrimSpace(mcpgw.FirstStringArg(arguments, "input"))
	if input == "" {
		return mcpgw.BuildToolErrorResult("input is required"), nil
	}

	kindStr := strings.ToLower(strings.TrimSpace(mcpgw.FirstStringArg(arguments, "kind")))
	if kindStr == "" {
		kindStr = "user"
	}
	var kind channel.DirectoryEntryKind
	switch kindStr {
	case "user":
		kind = channel.DirectoryEntryUser
	case "group":
		kind = channel.DirectoryEntryGroup
	default:
		return mcpgw.BuildToolErrorResult("kind must be user or group"), nil
	}

	cfg, err := p.configResolver.ResolveEffectiveConfig(ctx, botID, channelType)
	if err != nil {
		p.logger.Warn("resolve config failed", slog.String("bot_id", botID), slog.String("platform", platform), slog.Any("error", err))
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	entry, err := dirAdapter.ResolveEntry(ctx, cfg, input, kind)
	if err != nil {
		p.logger.Warn("resolve entry failed", slog.String("input", input), slog.String("kind", kindStr), slog.Any("error", err))
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	payload := map[string]any{
		"ok":       true,
		"platform": channelType.String(),
		"kind":     string(entry.Kind),
		"id":       entry.ID,
		"name":     entry.Name,
		"handle":   entry.Handle,
		"metadata": entry.Metadata,
	}
	if entry.AvatarURL != "" {
		payload["avatar_url"] = entry.AvatarURL
	}
	return mcpgw.BuildToolSuccessResult(payload), nil
}
