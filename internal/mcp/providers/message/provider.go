package message

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
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
	Send(ctx context.Context, botID string, channelType channel.ChannelType, req channel.SendRequest) error
}

// Reactor adds or removes emoji reactions through channel manager.
type Reactor interface {
	React(ctx context.Context, botID string, channelType channel.ChannelType, req channel.ReactRequest) error
}

// ChannelTypeResolver parses platform name to channel type.
type ChannelTypeResolver interface {
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
	// IngestContainerFile reads a file from the container's /data directory,
	// ingests it into the media store, and returns the resulting asset metadata.
	IngestContainerFile(ctx context.Context, botID, containerPath string) (AssetMeta, error)
}

// Executor exposes send and react as MCP tools.
type Executor struct {
	sender        Sender
	reactor       Reactor
	resolver      ChannelTypeResolver
	assetResolver AssetResolver
	logger        *slog.Logger
}

// NewExecutor creates a message tool executor.
// reactor and assetResolver may be nil.
func NewExecutor(log *slog.Logger, sender Sender, reactor Reactor, resolver ChannelTypeResolver, assetResolver AssetResolver) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{
		sender:        sender,
		reactor:       reactor,
		resolver:      resolver,
		assetResolver: assetResolver,
		logger:        log.With(slog.String("provider", "message_tool")),
	}
}

func (p *Executor) ListTools(ctx context.Context, session mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	var tools []mcpgw.ToolDescriptor
	if p.sender != nil && p.resolver != nil {
		tools = append(tools, mcpgw.ToolDescriptor{
			Name:        toolSend,
			Description: "Send a message to a channel or session. Supports text, attachments, and replies.",
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
						"description": "Channel target (chat/group/thread ID). Use get_contacts to find available targets.",
					},
					"text": map[string]any{
						"type":        "string",
						"description": "Message text shortcut when message object is omitted",
					},
					"reply_to": map[string]any{
						"type":        "string",
						"description": "Message ID to reply to. The reply will reference this message on the platform.",
					},
					"attachments": map[string]any{
						"type":        "array",
						"description": "File paths or URLs to attach. Each item is a container path (e.g. /data/media/ab/file.jpg), an HTTP URL, or an object with {path, url, type, name}.",
						"items":       map[string]any{},
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
		// Allow empty message when attachments are provided.
		if rawAtt, ok := arguments["attachments"]; !ok || rawAtt == nil {
			return mcpgw.BuildToolErrorResult(parseErr.Error()), nil
		}
		outboundMessage = channel.Message{Text: strings.TrimSpace(messageText)}
	}

	// Resolve top-level attachments parameter.
	if rawAttachments, ok := arguments["attachments"]; ok && rawAttachments != nil {
		if arr, ok := rawAttachments.([]any); ok && len(arr) > 0 {
			resolved := p.resolveAttachments(ctx, botID, arr)
			outboundMessage.Attachments = append(outboundMessage.Attachments, resolved...)
		}
	}

	if outboundMessage.IsEmpty() {
		return mcpgw.BuildToolErrorResult("message or attachments required"), nil
	}

	if replyTo := mcpgw.FirstStringArg(arguments, "reply_to"); replyTo != "" {
		outboundMessage.Reply = &channel.ReplyRef{MessageID: replyTo}
	}

	target := mcpgw.FirstStringArg(arguments, "target")
	if target == "" {
		target = strings.TrimSpace(session.ReplyTarget)
	}
	if target == "" {
		return mcpgw.BuildToolErrorResult("target is required"), nil
	}

	sendReq := channel.SendRequest{
		Target:  target,
		Message: outboundMessage,
	}
	if err := p.sender.Send(ctx, botID, channelType, sendReq); err != nil {
		p.logger.Warn("send failed", slog.Any("error", err), slog.String("bot_id", botID), slog.String("platform", string(channelType)))
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	payload := map[string]any{
		"ok":          true,
		"bot_id":      botID,
		"platform":    channelType.String(),
		"target":      target,
		"instruction": "Message delivered successfully. You have completed your response. Please STOP now and do not call any more tools.",
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
		return "", fmt.Errorf("bot_id is required")
	}
	if strings.TrimSpace(session.BotID) != "" && botID != strings.TrimSpace(session.BotID) {
		return "", fmt.Errorf("bot_id mismatch")
	}
	return botID, nil
}

func (p *Executor) resolvePlatform(arguments map[string]any, session mcpgw.ToolSessionContext) (channel.ChannelType, error) {
	platform := mcpgw.FirstStringArg(arguments, "platform")
	if platform == "" {
		platform = strings.TrimSpace(session.CurrentPlatform)
	}
	if platform == "" {
		return "", fmt.Errorf("platform is required")
	}
	return p.resolver.ParseChannelType(platform)
}

// --- attachment resolution ---

// resolveAttachments converts raw attachment arguments (strings or objects)
// into channel.Attachment values, resolving container media paths when possible.
func (p *Executor) resolveAttachments(ctx context.Context, botID string, items []any) []channel.Attachment {
	var result []channel.Attachment
	for _, item := range items {
		switch v := item.(type) {
		case string:
			if att := p.resolveAttachmentRef(ctx, botID, strings.TrimSpace(v), "", ""); att != nil {
				result = append(result, *att)
			}
		case map[string]any:
			path := mcpgw.FirstStringArg(v, "path")
			url := mcpgw.FirstStringArg(v, "url")
			attType := mcpgw.FirstStringArg(v, "type")
			name := mcpgw.FirstStringArg(v, "name")
			ref := path
			if ref == "" {
				ref = url
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

// resolveAttachmentRef resolves a single path or URL to a channel.Attachment.
func (p *Executor) resolveAttachmentRef(ctx context.Context, botID, ref, attType, name string) *channel.Attachment {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	lower := strings.ToLower(ref)

	// HTTP/HTTPS URL — pass through.
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		t := channel.AttachmentType(attType)
		if t == "" {
			t = inferAttachmentTypeFromExt(ref)
		}
		return &channel.Attachment{
			Type: t,
			URL:  ref,
			Name: name,
		}
	}

	// Data URL — pass through.
	if strings.HasPrefix(lower, "data:") {
		t := channel.AttachmentType(attType)
		if t == "" {
			t = channel.AttachmentImage
		}
		return &channel.Attachment{
			Type:   t,
			Base64: ref,
			Name:   name,
		}
	}

	// Default name from the original path basename when not specified.
	if name == "" {
		name = filepath.Base(ref)
	}

	// Container media path — resolve via asset storage.
	const mediaMarker = "/data/media/"
	if idx := strings.Index(ref, mediaMarker); idx >= 0 && p.assetResolver != nil {
		storageKey := ref[idx+len(mediaMarker):]
		asset, err := p.assetResolver.GetByStorageKey(ctx, botID, storageKey)
		if err == nil {
			return assetMetaToAttachment(asset, botID, attType, name)
		}
		if p.logger != nil {
			p.logger.Warn("resolve media path failed", slog.String("path", ref), slog.Any("error", err))
		}
	}

	// Other container /data/ path — ingest into media store first.
	const dataPrefix = "/data/"
	if strings.HasPrefix(ref, dataPrefix) && p.assetResolver != nil {
		asset, err := p.assetResolver.IngestContainerFile(ctx, botID, ref)
		if err == nil {
			return assetMetaToAttachment(asset, botID, attType, name)
		}
		if p.logger != nil {
			p.logger.Warn("ingest container file failed", slog.String("path", ref), slog.Any("error", err))
		}
		return nil
	}

	// Unknown path — pass through as URL (may fail for non-HTTP paths).
	t := channel.AttachmentType(attType)
	if t == "" {
		t = inferAttachmentTypeFromExt(ref)
	}
	return &channel.Attachment{
		Type: t,
		URL:  ref,
		Name: name,
	}
}

func assetMetaToAttachment(asset AssetMeta, botID, attType, name string) *channel.Attachment {
	t := channel.AttachmentType(attType)
	if t == "" {
		t = inferAttachmentTypeFromMime(asset.Mime)
	}
	return &channel.Attachment{
		Type:        t,
		ContentHash: asset.ContentHash,
		Mime:        asset.Mime,
		Size:        asset.SizeBytes,
		Name:        name,
		Metadata: map[string]any{
			"bot_id":      botID,
			"storage_key": asset.StorageKey,
		},
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
			return channel.Message{}, fmt.Errorf("message must be object or string")
		}
	}
	if msg.IsEmpty() && strings.TrimSpace(fallbackText) != "" {
		msg.Text = strings.TrimSpace(fallbackText)
	}
	if msg.IsEmpty() {
		return channel.Message{}, fmt.Errorf("message is required")
	}
	return msg, nil
}
