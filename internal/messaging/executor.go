package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

// SessionContext carries request-scoped identity for tool execution.
type SessionContext struct {
	BotID           string
	ChatID          string
	CurrentPlatform string
	ReplyTarget     string
}

// Sender sends outbound messages through a channel manager.
type Sender interface {
	Send(ctx context.Context, botID string, channelType channel.ChannelType, req channel.SendRequest) error
}

// Reactor adds or removes emoji reactions through a channel manager.
type Reactor interface {
	React(ctx context.Context, botID string, channelType channel.ChannelType, req channel.ReactRequest) error
}

// ChannelTypeResolver parses a platform name to a channel type.
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
	IngestContainerFile(ctx context.Context, botID, containerPath string) (AssetMeta, error)
}

// Executor provides send and react operations for channel messaging.
type Executor struct {
	Sender        Sender
	Reactor       Reactor
	Resolver      ChannelTypeResolver
	AssetResolver AssetResolver
	Logger        *slog.Logger
}

// SendResult is the success payload returned after sending a message.
type SendResult struct {
	BotID    string
	Platform string
	Target   string
}

// ReactResult is the success payload returned after reacting.
type ReactResult struct {
	BotID     string
	Platform  string
	Target    string
	MessageID string
	Emoji     string
	Action    string // "added" or "removed"
}

// Send executes a send-message action. args are the tool call arguments.
func (e *Executor) Send(ctx context.Context, session SessionContext, args map[string]any) (*SendResult, error) {
	if e.Sender == nil || e.Resolver == nil {
		return nil, errors.New("message service not available")
	}
	botID, err := e.resolveBotID(args, session)
	if err != nil {
		return nil, err
	}
	channelType, err := e.resolvePlatform(args, session)
	if err != nil {
		return nil, err
	}
	messageText := firstStringArg(args, "text")
	outboundMessage, parseErr := ParseOutboundMessage(args, messageText)
	if parseErr != nil {
		if rawAtt, ok := args["attachments"]; !ok || rawAtt == nil {
			return nil, parseErr
		}
		outboundMessage = channel.Message{Text: strings.TrimSpace(messageText)}
	}
	if rawAttachments, ok := args["attachments"]; ok && rawAttachments != nil {
		items := NormalizeAttachmentInputs(rawAttachments)
		if items == nil {
			return nil, errors.New("attachments must be a string, object, or array")
		}
		if len(items) > 0 {
			resolved := e.ResolveAttachments(ctx, botID, items)
			if len(resolved) == 0 {
				return nil, errors.New("attachments could not be resolved")
			}
			outboundMessage.Attachments = append(outboundMessage.Attachments, resolved...)
		}
	}
	if outboundMessage.IsEmpty() {
		return nil, errors.New("message or attachments required")
	}
	if replyTo := firstStringArg(args, "reply_to"); replyTo != "" {
		outboundMessage.Reply = &channel.ReplyRef{MessageID: replyTo}
	}
	if outboundMessage.Format == "" && channel.ContainsMarkdown(outboundMessage.Text) {
		outboundMessage.Format = channel.MessageFormatMarkdown
	}
	target := firstStringArg(args, "target")
	if target == "" {
		target = strings.TrimSpace(session.ReplyTarget)
	}
	if target == "" {
		return nil, errors.New("target is required")
	}
	if strings.EqualFold(channelType.String(), strings.TrimSpace(session.CurrentPlatform)) &&
		target == strings.TrimSpace(session.ReplyTarget) {
		return nil, errors.New("you are trying to send a message to the same conversation you are already in. " +
			"Do not use the send tool for this. Instead, write your reply as plain text directly. " +
			"To include files, use the <attachments> block in your response (e.g. <attachments>[{\"type\":\"image\",\"path\":\"/data/media/file.jpg\"}]</attachments>)")
	}
	if err := e.Sender.Send(ctx, botID, channelType, channel.SendRequest{Target: target, Message: outboundMessage}); err != nil {
		if e.Logger != nil {
			e.Logger.Warn("send failed", slog.Any("error", err), slog.String("bot_id", botID), slog.String("platform", string(channelType)))
		}
		return nil, err
	}
	return &SendResult{BotID: botID, Platform: channelType.String(), Target: target}, nil
}

// React executes a react action. args are the tool call arguments.
func (e *Executor) React(ctx context.Context, session SessionContext, args map[string]any) (*ReactResult, error) {
	if e.Reactor == nil || e.Resolver == nil {
		return nil, errors.New("reaction service not available")
	}
	botID, err := e.resolveBotID(args, session)
	if err != nil {
		return nil, err
	}
	channelType, err := e.resolvePlatform(args, session)
	if err != nil {
		return nil, err
	}
	target := firstStringArg(args, "target")
	if target == "" {
		target = strings.TrimSpace(session.ReplyTarget)
	}
	if target == "" {
		return nil, errors.New("target is required")
	}
	messageID := firstStringArg(args, "message_id")
	if messageID == "" {
		return nil, errors.New("message_id is required")
	}
	emoji := firstStringArg(args, "emoji")
	remove, _, _ := boolArg(args, "remove")
	if err := e.Reactor.React(ctx, botID, channelType, channel.ReactRequest{
		Target: target, MessageID: messageID, Emoji: emoji, Remove: remove,
	}); err != nil {
		if e.Logger != nil {
			e.Logger.Warn("react failed", slog.Any("error", err), slog.String("bot_id", botID), slog.String("platform", string(channelType)))
		}
		return nil, err
	}
	action := "added"
	if remove {
		action = "removed"
	}
	return &ReactResult{
		BotID: botID, Platform: channelType.String(), Target: target,
		MessageID: messageID, Emoji: emoji, Action: action,
	}, nil
}

// CanSend returns true if the executor has a sender and resolver configured.
func (e *Executor) CanSend() bool { return e.Sender != nil && e.Resolver != nil }

// CanReact returns true if the executor has a reactor and resolver configured.
func (e *Executor) CanReact() bool { return e.Reactor != nil && e.Resolver != nil }

func (*Executor) resolveBotID(args map[string]any, session SessionContext) (string, error) {
	botID := firstStringArg(args, "bot_id")
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

func (e *Executor) resolvePlatform(args map[string]any, session SessionContext) (channel.ChannelType, error) {
	platform := firstStringArg(args, "platform")
	if platform == "" {
		platform = strings.TrimSpace(session.CurrentPlatform)
	}
	if platform == "" {
		return "", errors.New("platform is required")
	}
	return e.Resolver.ParseChannelType(platform)
}

// ResolveAttachments converts raw attachment arguments into channel.Attachment values.
func (e *Executor) ResolveAttachments(ctx context.Context, botID string, items []any) []channel.Attachment {
	var result []channel.Attachment
	for _, item := range items {
		switch v := item.(type) {
		case string:
			if att := e.resolveAttachmentRef(ctx, botID, strings.TrimSpace(v), "", ""); att != nil {
				result = append(result, *att)
			}
		case map[string]any:
			path := firstStringArg(v, "path")
			urlVal := firstStringArg(v, "url")
			attType := firstStringArg(v, "type")
			name := firstStringArg(v, "name")
			ref := path
			if ref == "" {
				ref = urlVal
			}
			if ref == "" {
				continue
			}
			if att := e.resolveAttachmentRef(ctx, botID, ref, attType, name); att != nil {
				result = append(result, *att)
			}
		}
	}
	return result
}

func (e *Executor) resolveAttachmentRef(ctx context.Context, botID, ref, attType, name string) *channel.Attachment {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		t := channel.AttachmentType(attType)
		if t == "" {
			t = InferAttachmentTypeFromExt(ref)
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
	if idx := strings.Index(ref, mediaMarker); idx >= 0 && e.AssetResolver != nil {
		storageKey := ref[idx+len(mediaMarker):]
		asset, err := e.AssetResolver.GetByStorageKey(ctx, botID, storageKey)
		if err == nil {
			return AssetMetaToAttachment(asset, botID, attType, name)
		}
	}
	dataPrefix := "/data/"
	if strings.HasPrefix(ref, dataPrefix) && e.AssetResolver != nil {
		asset, err := e.AssetResolver.IngestContainerFile(ctx, botID, ref)
		if err == nil {
			return AssetMetaToAttachment(asset, botID, attType, name)
		}
		return nil
	}
	t := channel.AttachmentType(attType)
	if t == "" {
		t = InferAttachmentTypeFromExt(ref)
	}
	return &channel.Attachment{Type: t, URL: ref, Name: name}
}

// NormalizeAttachmentInputs normalizes raw attachment argument into a slice.
func NormalizeAttachmentInputs(raw any) []any {
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

// ParseOutboundMessage parses a message from tool call arguments.
func ParseOutboundMessage(arguments map[string]any, fallbackText string) (channel.Message, error) {
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

// AssetMetaToAttachment converts an AssetMeta to a channel.Attachment.
func AssetMetaToAttachment(asset AssetMeta, botID, attType, name string) *channel.Attachment {
	t := channel.AttachmentType(attType)
	if t == "" {
		t = InferAttachmentTypeFromMime(asset.Mime)
	}
	return &channel.Attachment{
		Type: t, ContentHash: asset.ContentHash, Mime: asset.Mime, Size: asset.SizeBytes, Name: name,
		Metadata: map[string]any{"bot_id": botID, "storage_key": asset.StorageKey},
	}
}

// InferAttachmentTypeFromMime infers attachment type from MIME type.
func InferAttachmentTypeFromMime(mime string) channel.AttachmentType {
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

// InferAttachmentTypeFromExt infers attachment type from file extension.
func InferAttachmentTypeFromExt(path string) channel.AttachmentType {
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

func firstStringArg(args map[string]any, keys ...string) string {
	for _, key := range keys {
		if args == nil {
			continue
		}
		raw, ok := args[key]
		if !ok {
			continue
		}
		if s, ok := raw.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func boolArg(args map[string]any, key string) (bool, bool, error) {
	if args == nil {
		return false, false, nil
	}
	raw, ok := args[key]
	if !ok || raw == nil {
		return false, false, nil
	}
	value, ok := raw.(bool)
	if !ok {
		return false, true, errors.New(key + " must be a boolean")
	}
	return value, true, nil
}
