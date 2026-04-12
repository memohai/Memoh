package tools

import (
	"context"
	"log/slog"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/channel"
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
			Description: "Send a message, file, or attachment. When target is omitted, delivers to the current conversation as an inline attachment/message. When target is specified, sends to that channel/person.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"bot_id":      map[string]any{"type": "string", "description": "Bot ID, optional and defaults to current bot"},
					"platform":    map[string]any{"type": "string", "description": "Channel platform name. Defaults to current session platform."},
					"target":      map[string]any{"type": "string", "description": "Channel target (chat/group/thread ID). Optional — omit to send in the current conversation. Use get_contacts to find targets for other conversations."},
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
			Description: "Add or remove an emoji reaction on a message. When target/platform are omitted, reacts in the current conversation.",
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
	// Discuss mode: same-conversation sends must go through the channel adapter
	// directly — there is no active stream to emit into.
	if result.Local && session.SessionType == "discuss" {
		sendResult, err := p.exec.SendDirect(ctx, toMessagingSession(session), result.Target, args)
		if err != nil {
			return nil, err
		}
		resp := map[string]any{
			"ok": true, "bot_id": sendResult.BotID, "platform": sendResult.Platform, "target": sendResult.Target,
			"delivered": "current_conversation",
		}
		if sendResult.MessageID != "" {
			resp["message_id"] = sendResult.MessageID
		}
		return resp, nil
	}
	if result.Local && session.Emitter != nil {
		atts := channelAttachmentsToToolAttachments(result.LocalAttachments)
		if len(atts) > 0 {
			session.Emitter(ToolStreamEvent{
				Type:        StreamEventAttachment,
				Attachments: atts,
			})
		}
		resp := map[string]any{
			"ok":          true,
			"delivered":   "current_conversation",
			"attachments": len(atts),
		}
		if result.MessageID != "" {
			resp["message_id"] = result.MessageID
		}
		return resp, nil
	}
	resp := map[string]any{
		"ok": true, "bot_id": result.BotID, "platform": result.Platform, "target": result.Target,
	}
	if result.MessageID != "" {
		resp["message_id"] = result.MessageID
	}
	return resp, nil
}

func channelAttachmentsToToolAttachments(atts []channel.Attachment) []Attachment {
	if len(atts) == 0 {
		return nil
	}
	result := make([]Attachment, 0, len(atts))
	for _, a := range atts {
		result = append(result, Attachment{
			Type:        string(a.Type),
			URL:         a.URL,
			Mime:        a.Mime,
			Name:        a.Name,
			ContentHash: a.ContentHash,
			Size:        a.Size,
			Metadata:    a.Metadata,
		})
	}
	return result
}

func (p *MessageProvider) execReact(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	// Check same-conversation before delegating to executor.
	platform := FirstStringArg(args, "platform")
	if platform == "" {
		platform = strings.TrimSpace(session.CurrentPlatform)
	}
	target := FirstStringArg(args, "target")
	if target == "" {
		target = strings.TrimSpace(session.ReplyTarget)
	}
	if session.IsSameConversation(platform, target) && session.Emitter != nil {
		messageID := FirstStringArg(args, "message_id")
		emoji := FirstStringArg(args, "emoji")
		remove, _, _ := BoolArg(args, "remove")
		if messageID == "" {
			return nil, nil
		}
		session.Emitter(ToolStreamEvent{
			Type: StreamEventReaction,
			Reactions: []Reaction{{
				Emoji:     emoji,
				MessageID: messageID,
				Remove:    remove,
			}},
		})
		action := "added"
		if remove {
			action = "removed"
		}
		return map[string]any{
			"ok": true, "emoji": emoji, "action": action,
			"delivered": "current_conversation",
		}, nil
	}
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
