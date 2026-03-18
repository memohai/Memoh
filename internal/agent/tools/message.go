package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/memohai/memoh/internal/channel"
	sdk "github.com/memohai/twilight-ai/sdk"
)

// MessageSender sends outbound messages through channel manager.
type MessageSender interface {
	Send(ctx context.Context, botID string, channelType channel.ChannelType, req channel.SendRequest) error
}

// MessageReactor adds or removes emoji reactions through channel manager.
type MessageReactor interface {
	React(ctx context.Context, botID string, channelType channel.ChannelType, req channel.ReactRequest) error
}

// MessageChannelResolver parses platform name to channel type.
type MessageChannelResolver interface {
	ParseChannelType(raw string) (channel.ChannelType, error)
}

// AssetMeta holds resolved metadata for a media asset.
type AssetMeta struct {
	ContentHash string
	Mime        string
	SizeBytes   int64
	StorageKey  string
}

// AssetResolver looks up persisted media assets by storage key.
type AssetResolver interface {
	GetByStorageKey(ctx context.Context, botID, storageKey string) (AssetMeta, error)
	IngestContainerFile(ctx context.Context, botID, containerPath string) (AssetMeta, error)
}

type MessageProvider struct {
	sender        MessageSender
	reactor       MessageReactor
	resolver      MessageChannelResolver
	assetResolver AssetResolver
	logger        *slog.Logger
}

func NewMessageProvider(log *slog.Logger, sender MessageSender, reactor MessageReactor, resolver MessageChannelResolver, assetResolver AssetResolver) *MessageProvider {
	if log == nil {
		log = slog.Default()
	}
	return &MessageProvider{
		sender: sender, reactor: reactor, resolver: resolver,
		assetResolver: assetResolver,
		logger:        log.With(slog.String("tool", "message")),
	}
}

func (p *MessageProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	var tools []sdk.Tool
	sess := session
	if p.sender != nil && p.resolver != nil {
		tools = append(tools, sdk.Tool{
			Name:        "send",
			Description: "Send a message to a DIFFERENT channel or person — NOT for replying to the current conversation. Use this only for cross-channel messaging, forwarding, or replying to inbox items.",
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
				return p.execSend(ctx, sess, inputAsMap(input))
			},
		})
	}
	if p.reactor != nil && p.resolver != nil {
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
				return p.execReact(ctx, sess, inputAsMap(input))
			},
		})
	}
	return tools, nil
}

func (p *MessageProvider) execSend(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	botID, err := resolveBotID(args, session)
	if err != nil {
		return nil, err
	}
	channelType, err := p.resolvePlatform(args, session)
	if err != nil {
		return nil, err
	}
	messageText := FirstStringArg(args, "text")
	outboundMessage, parseErr := parseOutboundMessage(args, messageText)
	if parseErr != nil {
		if rawAtt, ok := args["attachments"]; !ok || rawAtt == nil {
			return nil, parseErr
		}
		outboundMessage = channel.Message{Text: strings.TrimSpace(messageText)}
	}
	if rawAttachments, ok := args["attachments"]; ok && rawAttachments != nil {
		items := normalizeAttachmentInputs(rawAttachments)
		if items == nil {
			return nil, fmt.Errorf("attachments must be a string, object, or array")
		}
		if len(items) > 0 {
			resolved := p.resolveAttachments(ctx, botID, items)
			if len(resolved) == 0 {
				return nil, fmt.Errorf("attachments could not be resolved")
			}
			outboundMessage.Attachments = append(outboundMessage.Attachments, resolved...)
		}
	}
	if outboundMessage.IsEmpty() {
		return nil, fmt.Errorf("message or attachments required")
	}
	if replyTo := FirstStringArg(args, "reply_to"); replyTo != "" {
		outboundMessage.Reply = &channel.ReplyRef{MessageID: replyTo}
	}
	if outboundMessage.Format == "" && channel.ContainsMarkdown(outboundMessage.Text) {
		outboundMessage.Format = channel.MessageFormatMarkdown
	}
	target := FirstStringArg(args, "target")
	if target == "" {
		target = strings.TrimSpace(session.ReplyTarget)
	}
	if target == "" {
		return nil, fmt.Errorf("target is required")
	}
	if strings.EqualFold(channelType.String(), strings.TrimSpace(session.CurrentPlatform)) &&
		target == strings.TrimSpace(session.ReplyTarget) {
		return nil, fmt.Errorf(
			"You are trying to send a message to the SAME conversation you are already in. " +
				"Do NOT use the send tool for this. Instead, write your reply as plain text directly. " +
				"To include files, use the <attachments> block in your response (e.g. <attachments>[{\"type\":\"image\",\"path\":\"/data/media/file.jpg\"}]</attachments>).")
	}
	if err := p.sender.Send(ctx, botID, channelType, channel.SendRequest{Target: target, Message: outboundMessage}); err != nil {
		return nil, err
	}
	return map[string]any{
		"ok": true, "bot_id": botID, "platform": channelType.String(), "target": target,
		"instruction": "Message delivered successfully. You have completed your response. Please STOP now and do not call any more tools.",
	}, nil
}

func (p *MessageProvider) execReact(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	botID, err := resolveBotID(args, session)
	if err != nil {
		return nil, err
	}
	channelType, err := p.resolvePlatform(args, session)
	if err != nil {
		return nil, err
	}
	target := FirstStringArg(args, "target")
	if target == "" {
		target = strings.TrimSpace(session.ReplyTarget)
	}
	if target == "" {
		return nil, fmt.Errorf("target is required")
	}
	messageID := FirstStringArg(args, "message_id")
	if messageID == "" {
		return nil, fmt.Errorf("message_id is required")
	}
	emoji := FirstStringArg(args, "emoji")
	remove, _, _ := BoolArg(args, "remove")
	if err := p.reactor.React(ctx, botID, channelType, channel.ReactRequest{
		Target: target, MessageID: messageID, Emoji: emoji, Remove: remove,
	}); err != nil {
		return nil, err
	}
	action := "added"
	if remove {
		action = "removed"
	}
	return map[string]any{
		"ok": true, "bot_id": botID, "platform": channelType.String(),
		"target": target, "message_id": messageID, "emoji": emoji, "action": action,
	}, nil
}

func (p *MessageProvider) resolvePlatform(args map[string]any, session SessionContext) (channel.ChannelType, error) {
	platform := FirstStringArg(args, "platform")
	if platform == "" {
		platform = strings.TrimSpace(session.CurrentPlatform)
	}
	if platform == "" {
		return "", errors.New("platform is required")
	}
	return p.resolver.ParseChannelType(platform)
}

func resolveBotID(args map[string]any, session SessionContext) (string, error) {
	botID := FirstStringArg(args, "bot_id")
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

func (p *MessageProvider) resolveAttachments(ctx context.Context, botID string, items []any) []channel.Attachment {
	var result []channel.Attachment
	for _, item := range items {
		switch v := item.(type) {
		case string:
			if att := p.resolveAttachmentRef(ctx, botID, strings.TrimSpace(v), "", ""); att != nil {
				result = append(result, *att)
			}
		case map[string]any:
			path := FirstStringArg(v, "path")
			urlVal := FirstStringArg(v, "url")
			attType := FirstStringArg(v, "type")
			name := FirstStringArg(v, "name")
			ref := path
			if ref == "" {
				ref = urlVal
			}
			if ref == "" {
				continue
			}
			if att := p.resolveAttachmentRef(ctx, botID, ref, attType, name); att != nil {
				result = append(result, *att)
			}
		}
	}
	return result
}

func normalizeAttachmentInputs(raw any) []any {
	switch v := raw.(type) {
	case nil:
		return nil
	case []any:
		if v == nil {
			return []any{}
		}
		return v
	case []string:
		items := make([]any, 0, len(v))
		for _, item := range v {
			items = append(items, item)
		}
		return items
	case string, map[string]any:
		return []any{v}
	default:
		return nil
	}
}

func (p *MessageProvider) resolveAttachmentRef(ctx context.Context, botID, ref, attType, name string) *channel.Attachment {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		t := channel.AttachmentType(attType)
		if t == "" {
			t = inferAttachmentTypeFromExt(ref)
		}
		return &channel.Attachment{Type: t, URL: ref, Name: name}
	}
	if strings.HasPrefix(lower, "data:") {
		t := channel.AttachmentType(attType)
		if t == "" {
			t = channel.AttachmentImage
		}
		return &channel.Attachment{Type: t, Base64: ref, Name: name}
	}
	if name == "" {
		name = filepath.Base(ref)
	}
	mediaMarker := filepath.Join("/data", "media")
	if !strings.HasSuffix(mediaMarker, "/") {
		mediaMarker += "/"
	}
	if idx := strings.Index(ref, mediaMarker); idx >= 0 && p.assetResolver != nil {
		storageKey := ref[idx+len(mediaMarker):]
		asset, err := p.assetResolver.GetByStorageKey(ctx, botID, storageKey)
		if err == nil {
			return assetMetaToAttachment(asset, botID, attType, name)
		}
	}
	dataPrefix := "/data/"
	if strings.HasPrefix(ref, dataPrefix) && p.assetResolver != nil {
		asset, err := p.assetResolver.IngestContainerFile(ctx, botID, ref)
		if err == nil {
			return assetMetaToAttachment(asset, botID, attType, name)
		}
		return nil
	}
	t := channel.AttachmentType(attType)
	if t == "" {
		t = inferAttachmentTypeFromExt(ref)
	}
	return &channel.Attachment{Type: t, URL: ref, Name: name}
}

func assetMetaToAttachment(asset AssetMeta, botID, attType, name string) *channel.Attachment {
	t := channel.AttachmentType(attType)
	if t == "" {
		t = inferAttachmentTypeFromMime(asset.Mime)
	}
	return &channel.Attachment{
		Type: t, ContentHash: asset.ContentHash, Mime: asset.Mime, Size: asset.SizeBytes, Name: name,
		Metadata: map[string]any{"bot_id": botID, "storage_key": asset.StorageKey},
	}
}

func inferAttachmentTypeFromMime(mime string) channel.AttachmentType {
	mime = strings.ToLower(strings.TrimSpace(mime))
	switch {
	case strings.HasPrefix(mime, "image/"):
		return channel.AttachmentImage
	case strings.HasPrefix(mime, "audio/"):
		return channel.AttachmentAudio
	case strings.HasPrefix(mime, "video/"):
		return channel.AttachmentVideo
	default:
		return channel.AttachmentFile
	}
}

func inferAttachmentTypeFromExt(path string) channel.AttachmentType {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg":
		return channel.AttachmentImage
	case ".mp3", ".wav", ".ogg", ".flac", ".aac":
		return channel.AttachmentAudio
	case ".mp4", ".webm", ".avi", ".mov":
		return channel.AttachmentVideo
	default:
		return channel.AttachmentFile
	}
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
