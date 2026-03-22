package tools

import (
	"context"
	"log/slog"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/messaging"
)

type MessageProvider struct {
	exec *messaging.Executor
}

func NewMessageProvider(log *slog.Logger, sender messaging.Sender, reactor messaging.Reactor, resolver messaging.ChannelTypeResolver, assetResolver messaging.AssetResolver) *MessageProvider {
	if log == nil {
		log = slog.Default()
	}
	return &MessageProvider{
		exec: &messaging.Executor{
			Sender:        sender,
			Reactor:       reactor,
			Resolver:      resolver,
			AssetResolver: assetResolver,
			Logger:        log.With(slog.String("tool", "message")),
		},
	}
}

func (p *MessageProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	if session.IsSubagent {
		return nil, nil
	}
	var tools []sdk.Tool
	sess := session
	if p.exec.CanSend() {
		tools = append(tools, sdk.Tool{
			Name:        "send",
			Description: "Send a message to a DIFFERENT channel or person — NOT for replying to the current conversation. Use this only for cross-channel messaging or forwarding.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"bot_id":      map[string]any{"type": "string", "description": "Bot ID, optional and defaults to current bot"},
					"platform":    map[string]any{"type": "string", "description": "Channel platform name"},
					"target":      map[string]any{"type": "string", "description": "Channel target (chat/group/thread ID). Use get_contacts to find available targets."},
					"text":        map[string]any{"type": "string", "description": "Message text shortcut when message object is omitted"},
					"reply_to":    map[string]any{"type": "string", "description": "Message ID to reply to. The reply will reference this message on the platform."},
					"attachments": map[string]any{"type": "array", "description": "File paths or URLs to attach.", "items": map[string]any{"type": "string"}},
					"message":     map[string]any{"type": "object", "description": "Structured message payload with text/parts/attachments"},
				},
				"required": []string{},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execSend(ctx.Context, sess, inputAsMap(input))
			},
		})
	}
	if p.exec.CanReact() {
		tools = append(tools, sdk.Tool{
			Name:        "react",
			Description: "Add or remove an emoji reaction on a channel message",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"bot_id":     map[string]any{"type": "string", "description": "Bot ID, optional and defaults to current bot"},
					"platform":   map[string]any{"type": "string", "description": "Channel platform name. Defaults to current session platform."},
					"target":     map[string]any{"type": "string", "description": "Channel target (chat/group ID). Defaults to current session reply target."},
					"message_id": map[string]any{"type": "string", "description": "The message ID to react to"},
					"emoji":      map[string]any{"type": "string", "description": "Emoji to react with (e.g. 👍, ❤️). Required when adding a reaction."},
					"remove":     map[string]any{"type": "boolean", "description": "If true, remove the reaction instead of adding it. Default false."},
				},
				"required": []string{"message_id"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execReact(ctx.Context, sess, inputAsMap(input))
			},
		})
	}
	return tools, nil
}

func (p *MessageProvider) execSend(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	result, err := p.exec.Send(ctx, toMessagingSession(session), args)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok": true, "bot_id": result.BotID, "platform": result.Platform, "target": result.Target,
		"instruction": "Message delivered successfully. You have completed your response. Please STOP now and do not call any more tools.",
	}, nil
}

func (p *MessageProvider) execReact(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	result, err := p.exec.React(ctx, toMessagingSession(session), args)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok": true, "bot_id": result.BotID, "platform": result.Platform,
		"target": result.Target, "message_id": result.MessageID, "emoji": result.Emoji, "action": result.Action,
	}, nil
}

func toMessagingSession(s SessionContext) messaging.SessionContext {
	return messaging.SessionContext{
		BotID:           s.BotID,
		ChatID:          s.ChatID,
		CurrentPlatform: s.CurrentPlatform,
		ReplyTarget:     s.ReplyTarget,
	}
}
