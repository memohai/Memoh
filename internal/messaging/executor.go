package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	attachmentpkg "github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/media"
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
type AssetMeta = media.Asset

// AssetResolver looks up persisted media assets by storage key and can
// optionally ingest files from a bot's container filesystem.
type AssetResolver interface {
	channel.OutboundAttachmentStore
	channel.ContainerAttachmentIngester
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
	BotID     string
	Platform  string
	Target    string
	MessageID string
	// Local is true when the message targets the current conversation.
	// The caller should emit the resolved attachments as stream events.
	Local            bool
	LocalAttachments []channel.Attachment
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

type sendMode struct {
	name                   string
	allowLocalShortcut     bool
	requireTarget          bool
	promoteDataAttachments bool
}

type sendPlan struct {
	botID       string
	channelType channel.ChannelType
	target      string
	sameConv    bool
	message     channel.Message
}

// Send executes a send-message action. args are the tool call arguments.
func (e *Executor) Send(ctx context.Context, session SessionContext, args map[string]any) (*SendResult, error) {
	return e.sendWithMode(ctx, session, "", args, sendMode{
		name:               "send",
		allowLocalShortcut: true,
	})
}

// SendDirect sends a message via the channel adapter without the same-conversation
// local shortcut. Used by discuss mode where there is no active stream emitter.
func (e *Executor) SendDirect(ctx context.Context, session SessionContext, target string, args map[string]any) (*SendResult, error) {
	return e.sendWithMode(ctx, session, target, args, sendMode{
		name:                   "send direct",
		requireTarget:          true,
		promoteDataAttachments: true,
	})
}

func (e *Executor) sendWithMode(
	ctx context.Context,
	session SessionContext,
	explicitTarget string,
	args map[string]any,
	mode sendMode,
) (*SendResult, error) {
	if e.Sender == nil || e.Resolver == nil {
		return nil, errors.New("message service not available")
	}

	plan, err := e.prepareSendPlan(ctx, session, explicitTarget, args, mode)
	if err != nil {
		return nil, err
	}

	if mode.allowLocalShortcut && plan.sameConv {
		return &SendResult{
			BotID:            plan.botID,
			Platform:         plan.channelType.String(),
			Target:           plan.target,
			Local:            true,
			LocalAttachments: plan.message.Attachments,
		}, nil
	}

	if mode.promoteDataAttachments {
		e.promoteDataPathAttachmentsToAssets(ctx, plan.botID, plan.channelType, &plan.message)
	}

	if err := e.Sender.Send(ctx, plan.botID, plan.channelType, channel.SendRequest{
		Target:  plan.target,
		Message: plan.message,
	}); err != nil {
		if e.Logger != nil {
			e.Logger.Warn("outbound send failed",
				slog.String("mode", mode.name),
				slog.Any("error", err),
				slog.String("bot_id", plan.botID),
				slog.String("platform", string(plan.channelType)),
			)
		}
		return nil, err
	}

	return &SendResult{
		BotID:    plan.botID,
		Platform: plan.channelType.String(),
		Target:   plan.target,
	}, nil
}

func (e *Executor) prepareSendPlan(
	ctx context.Context,
	session SessionContext,
	explicitTarget string,
	args map[string]any,
	mode sendMode,
) (*sendPlan, error) {
	botID, err := e.resolveBotID(args, session)
	if err != nil {
		return nil, err
	}
	channelType, err := e.resolvePlatform(args, session)
	if err != nil {
		return nil, err
	}
	target := strings.TrimSpace(explicitTarget)
	if target == "" {
		target = firstStringArg(args, "target")
	}
	if target == "" {
		target = strings.TrimSpace(session.ReplyTarget)
	}

	sameConv := target == "" || IsSameConversation(session, channelType.String(), target)
	if mode.requireTarget && target == "" {
		return nil, errors.New("target is required")
	}
	if !mode.allowLocalShortcut && target == "" {
		return nil, errors.New("target is required for cross-conversation send")
	}

	msg, err := e.buildOutboundMessage(ctx, botID, session, channelType, target, args, sameConv)
	if err != nil {
		return nil, err
	}

	return &sendPlan{
		botID:       botID,
		channelType: channelType,
		target:      target,
		sameConv:    sameConv,
		message:     msg,
	}, nil
}

func (e *Executor) buildOutboundMessage(
	ctx context.Context,
	botID string,
	session SessionContext,
	channelType channel.ChannelType,
	target string,
	args map[string]any,
	isSameConv bool,
) (channel.Message, error) {
	messageText := firstStringArg(args, "text")
	outboundMessage, parseErr := ParseOutboundMessage(args, messageText)
	if parseErr != nil {
		if rawAtt, ok := args["attachments"]; !ok || rawAtt == nil {
			return channel.Message{}, parseErr
		}
		outboundMessage = channel.Message{Text: strings.TrimSpace(messageText)}
	}

	if rawAttachments, ok := args["attachments"]; ok && rawAttachments != nil {
		attachments, err := e.resolveOutboundAttachments(ctx, botID, session, channelType, target, rawAttachments, isSameConv)
		if err != nil {
			return channel.Message{}, err
		}
		outboundMessage.Attachments = append(outboundMessage.Attachments, attachments...)
	}
	if outboundMessage.IsEmpty() {
		return channel.Message{}, errors.New("message or attachments required")
	}
	if replyTo := firstStringArg(args, "reply_to"); replyTo != "" {
		outboundMessage.Reply = &channel.ReplyRef{MessageID: replyTo}
	}
	if outboundMessage.Format == "" && channel.ContainsMarkdown(outboundMessage.Text) {
		outboundMessage.Format = channel.MessageFormatMarkdown
	}
	return outboundMessage, nil
}

func (e *Executor) resolveOutboundAttachments(
	ctx context.Context,
	botID string,
	session SessionContext,
	channelType channel.ChannelType,
	target string,
	rawAttachments any,
	isSameConv bool,
) ([]channel.Attachment, error) {
	bundles, ok := attachmentpkg.ParseToolInputBundles(rawAttachments)
	if !ok {
		return nil, errors.New("attachments must be a string, object, or array")
	}
	if len(bundles) == 0 {
		return nil, nil
	}
	if isSameConv || IsSameConversation(session, channelType.String(), target) {
		return resolveSameConversationAttachments(bundles), nil
	}
	resolved := e.ResolveAttachments(ctx, botID, bundles)
	if len(resolved) == 0 {
		return nil, errors.New("attachments could not be resolved")
	}
	return resolved, nil
}

func resolveSameConversationAttachments(bundles []attachmentpkg.Bundle) []channel.Attachment {
	if len(bundles) == 0 {
		return nil
	}
	result := make([]channel.Attachment, 0, len(bundles))
	for _, bundle := range bundles {
		result = append(result, channel.AttachmentFromBundle(bundle))
	}
	return result
}

// promoteDataPathAttachmentsToAssets converts /data/* attachment references
// into content_hash-backed attachments before channel send.
// This avoids adapters treating local container paths as HTTP URLs.
func (e *Executor) promoteDataPathAttachmentsToAssets(ctx context.Context, botID string, channelType channel.ChannelType, msg *channel.Message) {
	if e.AssetResolver == nil || msg == nil || len(msg.Attachments) == 0 {
		return
	}
	prepared, err := channel.PrepareOutboundMessage(ctx, e.AssetResolver, channel.ChannelConfig{
		BotID:       botID,
		ChannelType: channelType,
	}, channel.OutboundMessage{
		Message: *msg,
	})
	if err != nil {
		return
	}
	msg.Attachments = prepared.Message.Message.Attachments
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

// IsSameConversation reports whether platform+target matches the session's
// current conversation.
func IsSameConversation(session SessionContext, platform, target string) bool {
	replyTarget := strings.TrimSpace(session.ReplyTarget)
	if replyTarget == "" {
		return false
	}
	if platform == "" {
		platform = strings.TrimSpace(session.CurrentPlatform)
	}
	if target == "" {
		target = replyTarget
	}
	return strings.EqualFold(platform, strings.TrimSpace(session.CurrentPlatform)) &&
		target == replyTarget
}

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
func (e *Executor) ResolveAttachments(ctx context.Context, botID string, bundles []attachmentpkg.Bundle) []channel.Attachment {
	var result []channel.Attachment
	for _, bundle := range bundles {
		if att := e.resolveAttachmentBundle(ctx, botID, bundle); att != nil {
			result = append(result, *att)
		}
	}
	return result
}

func (e *Executor) resolveAttachmentBundle(ctx context.Context, botID string, bundle attachmentpkg.Bundle) *channel.Attachment {
	att := channel.AttachmentFromBundle(bundle)
	if att.Type == "" {
		att.Type = channel.AttachmentFile
	}
	if strings.TrimSpace(att.ContentHash) != "" {
		return &att
	}
	if attachmentpkg.IsHTTPURL(bundle.URL) {
		att.URL = bundle.URL
		return &att
	}
	if bundle.Base64 != "" {
		att.Base64 = bundle.Base64
		return &att
	}
	ref := strings.TrimSpace(bundle.Path)
	if ref == "" {
		if strings.TrimSpace(att.PlatformKey) == "" {
			return nil
		}
		return &att
	}
	if storageKey := attachmentpkg.ExtractStorageKey(ref); storageKey != "" && e.AssetResolver != nil {
		asset, err := e.AssetResolver.GetByStorageKey(ctx, botID, storageKey)
		if err == nil {
			return AssetMetaToAttachment(asset, botID, string(att.Type), bundle.Name)
		}
	}
	if attachmentpkg.IsDataPath(ref) && e.AssetResolver != nil {
		asset, err := e.AssetResolver.IngestContainerFile(ctx, botID, ref)
		if err == nil {
			return AssetMetaToAttachment(asset, botID, string(att.Type), bundle.Name)
		}
		return nil
	}
	// att.Path is already set correctly by AttachmentFromBundle (bundle.Path = ref).
	return &att
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
func AssetMetaToAttachment(asset media.Asset, botID, attType, name string) *channel.Attachment {
	bundle := attachmentpkg.Bundle{
		Type: attType,
		Name: name,
	}.WithAsset(botID, asset)
	att := channel.AttachmentFromBundle(bundle)
	return &att
}

// InferAttachmentTypeFromMime infers attachment type from MIME type.
func InferAttachmentTypeFromMime(mime string) channel.AttachmentType {
	return channel.AttachmentType(attachmentpkg.InferTypeFromMime(mime))
}

// InferAttachmentTypeFromExt infers attachment type from file extension.
func InferAttachmentTypeFromExt(path string) channel.AttachmentType {
	return channel.AttachmentType(attachmentpkg.InferTypeFromExt(path))
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
