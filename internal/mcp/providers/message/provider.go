// Package message provides the MCP message provider (send and list tools).
package message

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/channel"
	mcpgw "github.com/memohai/memoh/internal/mcp"
)

const (
	toolSend  = "send"
	toolReact = "react"
)

// Sender sends outbound messages through channel manager.
type Sender interface {
	Send(ctx context.Context, botID string, channelType channel.Type, req channel.SendRequest) error
}

// Reactor adds or removes emoji reactions through channel manager.
type Reactor interface {
	React(ctx context.Context, botID string, channelType channel.Type, req channel.ReactRequest) error
}

// ChannelTypeResolver parses platform name to channel type.
type ChannelTypeResolver interface {
	ParseChannelType(raw string) (channel.Type, error)
}

// Executor exposes send and react as MCP tools.
type Executor struct {
	sender   Sender
	reactor  Reactor
	resolver ChannelTypeResolver
	logger   *slog.Logger
}

// NewExecutor creates a message tool executor.
// reactor may be nil; the react tool will not be listed when reactor is unavailable.
func NewExecutor(log *slog.Logger, sender Sender, reactor Reactor, resolver ChannelTypeResolver) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{
		sender:   sender,
		reactor:  reactor,
		resolver: resolver,
		logger:   log.With(slog.String("provider", "message_tool")),
	}
}

// ListTools returns send and optionally react tool descriptors.
func (p *Executor) ListTools(_ context.Context, _ mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	var tools []mcpgw.ToolDescriptor
	if p.sender != nil && p.resolver != nil {
		tools = append(tools, mcpgw.ToolDescriptor{
			Name:        toolSend,
			Description: "Send a message to a channel or session. Supports text, structured messages, attachments, and replies.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"bot_id": map[string]any{
						"type":        "string",
						"description": "Bot ID, optional and defaults to current bot",
					},
					"platform": map[string]any{
						"type":        "string",
						"description": "Channel platform name",
					},
					"target": map[string]any{
						"type":        "string",
						"description": "Channel target (chat/group/thread ID)",
					},
					"channel_identity_id": map[string]any{
						"type":        "string",
						"description": "Target identity ID when direct target is absent",
					},
					"to_user_id": map[string]any{
						"type":        "string",
						"description": "Alias for channel_identity_id",
					},
					"text": map[string]any{
						"type":        "string",
						"description": "Message text shortcut when message object is omitted",
					},
					"reply_to": map[string]any{
						"type":        "string",
						"description": "Message ID to reply to. The reply will reference this message on the platform.",
					},
					"message": map[string]any{
						"type":        "object",
						"description": "Structured message payload with text/parts/attachments",
					},
				},
				"required": []string{},
			},
		})
	}
	if p.reactor != nil && p.resolver != nil {
		tools = append(tools, mcpgw.ToolDescriptor{
			Name:        toolReact,
			Description: "Add or remove an emoji reaction on a channel message",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"bot_id": map[string]any{
						"type":        "string",
						"description": "Bot ID, optional and defaults to current bot",
					},
					"platform": map[string]any{
						"type":        "string",
						"description": "Channel platform name. Defaults to current session platform.",
					},
					"target": map[string]any{
						"type":        "string",
						"description": "Channel target (chat/group ID). Defaults to current session reply target.",
					},
					"message_id": map[string]any{
						"type":        "string",
						"description": "The message ID to react to",
					},
					"emoji": map[string]any{
						"type":        "string",
						"description": "Emoji to react with (e.g. 👍, ❤️). Required when adding a reaction.",
					},
					"remove": map[string]any{
						"type":        "boolean",
						"description": "If true, remove the reaction instead of adding it. Default false.",
					},
				},
				"required": []string{"message_id"},
			},
		})
	}
	return tools, nil
}

// CallTool runs send or react; validates args and delegates to Sender/Reactor.
func (p *Executor) CallTool(ctx context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	switch toolName {
	case toolSend:
		return p.callSend(ctx, session, arguments)
	case toolReact:
		return p.callReact(ctx, session, arguments)
	default:
		return nil, mcpgw.ErrToolNotFound
	}
}

// --- send ---

func (p *Executor) callSend(ctx context.Context, session mcpgw.ToolSessionContext, arguments map[string]any) (map[string]any, error) {
	if p.sender == nil || p.resolver == nil {
		return mcpgw.BuildToolErrorResult("message service not available"), nil
	}

	botID, err := p.resolveBotID(arguments, session)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	channelType, err := p.resolvePlatform(arguments, session)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	messageText := mcpgw.FirstStringArg(arguments, "text")
	outboundMessage, parseErr := parseOutboundMessage(arguments, messageText)
	if parseErr != nil {
		return mcpgw.BuildToolErrorResult(parseErr.Error()), nil
	}

	// Attach reply reference if reply_to is provided.
	if replyTo := mcpgw.FirstStringArg(arguments, "reply_to"); replyTo != "" {
		outboundMessage.Reply = &channel.ReplyRef{MessageID: replyTo}
	}

	target := mcpgw.FirstStringArg(arguments, "target")
	if target == "" {
		target = strings.TrimSpace(session.ReplyTarget)
	}
	channelIdentityID := mcpgw.FirstStringArg(arguments, "channel_identity_id", "to_user_id")
	if target == "" && channelIdentityID == "" {
		return mcpgw.BuildToolErrorResult("target or channel_identity_id is required"), nil
	}

	sendReq := channel.SendRequest{
		Target:            target,
		ChannelIdentityID: channelIdentityID,
		Message:           outboundMessage,
	}
	if err := p.sender.Send(ctx, botID, channelType, sendReq); err != nil {
		p.logger.Warn("send failed", slog.Any("error", err), slog.String("bot_id", botID), slog.String("platform", string(channelType)))
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	payload := map[string]any{
		"ok":                  true,
		"bot_id":              botID,
		"platform":            channelType.String(),
		"target":              target,
		"channel_identity_id": channelIdentityID,
		"instruction":         "Message delivered successfully. You have completed your response. Please STOP now and do not call any more tools.",
	}
	return mcpgw.BuildToolSuccessResult(payload), nil
}

// --- react ---

func (p *Executor) callReact(ctx context.Context, session mcpgw.ToolSessionContext, arguments map[string]any) (map[string]any, error) {
	if p.reactor == nil || p.resolver == nil {
		return mcpgw.BuildToolErrorResult("reaction service not available"), nil
	}

	botID, err := p.resolveBotID(arguments, session)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	channelType, err := p.resolvePlatform(arguments, session)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	target := mcpgw.FirstStringArg(arguments, "target")
	if target == "" {
		target = strings.TrimSpace(session.ReplyTarget)
	}
	if target == "" {
		return mcpgw.BuildToolErrorResult("target is required"), nil
	}

	messageID := mcpgw.FirstStringArg(arguments, "message_id")
	if messageID == "" {
		return mcpgw.BuildToolErrorResult("message_id is required"), nil
	}

	emoji := mcpgw.FirstStringArg(arguments, "emoji")
	remove, _, _ := mcpgw.BoolArg(arguments, "remove")

	reactReq := channel.ReactRequest{
		Target:    target,
		MessageID: messageID,
		Emoji:     emoji,
		Remove:    remove,
	}
	if err := p.reactor.React(ctx, botID, channelType, reactReq); err != nil {
		p.logger.Warn("react failed", slog.Any("error", err), slog.String("bot_id", botID), slog.String("platform", string(channelType)))
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	action := "added"
	if remove {
		action = "removed"
	}
	payload := map[string]any{
		"ok":         true,
		"bot_id":     botID,
		"platform":   channelType.String(),
		"target":     target,
		"message_id": messageID,
		"emoji":      emoji,
		"action":     action,
	}
	return mcpgw.BuildToolSuccessResult(payload), nil
}

// --- shared helpers ---

func (p *Executor) resolveBotID(arguments map[string]any, session mcpgw.ToolSessionContext) (string, error) {
	botID := mcpgw.FirstStringArg(arguments, "bot_id")
	if botID == "" {
		botID = strings.TrimSpace(session.BotID)
	}
	if botID == "" {
		return "", errors.New("bot_id is required")
	}
	if strings.TrimSpace(session.BotID) != "" && botID != strings.TrimSpace(session.BotID) {
		return "", errors.New("bot_id mismatch")
	}
	return botID, nil
}

func (p *Executor) resolvePlatform(arguments map[string]any, session mcpgw.ToolSessionContext) (channel.Type, error) {
	platform := mcpgw.FirstStringArg(arguments, "platform")
	if platform == "" {
		platform = strings.TrimSpace(session.CurrentPlatform)
	}
	if platform == "" {
		return "", errors.New("platform is required")
	}
	return p.resolver.ParseChannelType(platform)
}

func parseOutboundMessage(arguments map[string]any, fallbackText string) (channel.Message, error) {
	var msg channel.Message
	if raw, ok := arguments["message"]; ok && raw != nil {
		switch value := raw.(type) {
		case string:
			msg.Text = strings.TrimSpace(value)
		case map[string]any:
			data, err := json.Marshal(value)
			if err != nil {
				return channel.Message{}, err
			}
			if err := json.Unmarshal(data, &msg); err != nil {
				return channel.Message{}, err
			}
		default:
			return channel.Message{}, errors.New("message must be object or string")
		}
	}
	if msg.IsEmpty() && strings.TrimSpace(fallbackText) != "" {
		msg.Text = strings.TrimSpace(fallbackText)
	}
	if msg.IsEmpty() {
		return channel.Message{}, errors.New("message is required")
	}
	return msg, nil
}
