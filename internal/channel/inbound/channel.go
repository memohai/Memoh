package inbound

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/inbox"
	"github.com/memohai/memoh/internal/media"
	messagepkg "github.com/memohai/memoh/internal/message"
)

const (
	silentReplyToken        = "NO_REPLY"
	minDuplicateTextLength  = 10
	processingStatusTimeout = 60 * time.Second
)

var (
	whitespacePattern = regexp.MustCompile(`\s+`)
)

// RouteResolver resolves and manages channel routes.
type RouteResolver interface {
	ResolveConversation(ctx context.Context, input route.ResolveInput) (route.ResolveConversationResult, error)
}

type mediaIngestor interface {
	Ingest(ctx context.Context, input media.IngestInput) (media.Asset, error)
	// GetByStorageKey resolves an asset by reading its sidecar JSON.
	GetByStorageKey(ctx context.Context, botID, storageKey string) (media.Asset, error)
	// AccessPath returns a consumer-accessible reference for a persisted asset.
	AccessPath(asset media.Asset) string
	// IngestContainerFile reads a file from /data/ and ingests it into media store.
	IngestContainerFile(ctx context.Context, botID, containerPath string) (media.Asset, error)
}

// ChannelInboundProcessor routes channel inbound messages to the chat gateway.
type ChannelInboundProcessor struct {
	runner        flow.Runner
	routeResolver RouteResolver
	message       messagepkg.Writer
	mediaService  mediaIngestor
	inboxService  *inbox.Service
	registry      *channel.Registry
	logger        *slog.Logger
	jwtSecret     string
	tokenTTL      time.Duration
	identity      *IdentityResolver
	observer      channel.StreamObserver
}

// NewChannelInboundProcessor creates a processor with channel identity-based resolution.
func NewChannelInboundProcessor(
	log *slog.Logger,
	registry *channel.Registry,
	routeResolver RouteResolver,
	messageWriter messagepkg.Writer,
	runner flow.Runner,
	channelIdentityService ChannelIdentityService,
	memberService BotMemberService,
	policyService PolicyService,
	preauthService PreauthService,
	bindService BindService,
	jwtSecret string,
	tokenTTL time.Duration,
) *ChannelInboundProcessor {
	if log == nil {
		log = slog.Default()
	}
	if tokenTTL <= 0 {
		tokenTTL = 5 * time.Minute
	}
	identityResolver := NewIdentityResolver(log, registry, channelIdentityService, memberService, policyService, preauthService, bindService, "", "")
	return &ChannelInboundProcessor{
		runner:        runner,
		routeResolver: routeResolver,
		message:       messageWriter,
		registry:      registry,
		logger:        log.With(slog.String("component", "channel_router")),
		jwtSecret:     strings.TrimSpace(jwtSecret),
		tokenTTL:      tokenTTL,
		identity:      identityResolver,
	}
}

// IdentityMiddleware returns the identity resolution middleware.
func (p *ChannelInboundProcessor) IdentityMiddleware() channel.Middleware {
	if p == nil || p.identity == nil {
		return nil
	}
	return p.identity.Middleware()
}

// SetMediaService configures media ingestion support for inbound attachments.
func (p *ChannelInboundProcessor) SetMediaService(mediaService mediaIngestor) {
	if p == nil {
		return
	}
	p.mediaService = mediaService
}

// SetStreamObserver configures an observer that receives copies of all stream
// events produced for non-local channels (e.g. Telegram, Feishu). This enables
// cross-channel visibility in the WebUI without coupling adapters to the hub.
func (p *ChannelInboundProcessor) SetStreamObserver(observer channel.StreamObserver) {
	if p == nil {
		return
	}
	p.observer = observer
}

// SetInboxService configures the inbox service for storing non-mentioned
// group messages as inbox items.
func (p *ChannelInboundProcessor) SetInboxService(service *inbox.Service) {
	if p == nil {
		return
	}
	p.inboxService = service
}

// HandleInbound processes an inbound channel message through identity resolution and chat gateway.
func (p *ChannelInboundProcessor) HandleInbound(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, sender channel.StreamReplySender) error {
	if p.runner == nil {
		return fmt.Errorf("channel inbound processor not configured")
	}
	if sender == nil {
		return fmt.Errorf("reply sender not configured")
	}
	text := buildInboundQuery(msg.Message)
	if p.logger != nil {
		p.logger.Debug("inbound handle start",
			slog.String("channel", msg.Channel.String()),
			slog.String("message_id", strings.TrimSpace(msg.Message.ID)),
			slog.String("query", strings.TrimSpace(text)),
			slog.Int("attachments", len(msg.Message.Attachments)),
			slog.String("conversation_type", strings.TrimSpace(msg.Conversation.Type)),
			slog.String("conversation_id", strings.TrimSpace(msg.Conversation.ID)),
		)
	}
	if strings.TrimSpace(text) == "" && len(msg.Message.Attachments) == 0 {
		if p.logger != nil {
			p.logger.Debug("inbound dropped empty", slog.String("channel", msg.Channel.String()))
		}
		return nil
	}
	state, err := p.requireIdentity(ctx, cfg, msg)
	if err != nil {
		return err
	}
	if state.Decision != nil && state.Decision.Stop {
		if !state.Decision.Reply.IsEmpty() {
			return sender.Send(ctx, channel.OutboundMessage{
				Target:  strings.TrimSpace(msg.ReplyTarget),
				Message: state.Decision.Reply,
			})
		}
		if p.logger != nil {
			p.logger.Info(
				"inbound dropped by identity policy (no reply sent)",
				slog.String("channel", msg.Channel.String()),
				slog.String("bot_id", strings.TrimSpace(state.Identity.BotID)),
				slog.String("conversation_type", strings.TrimSpace(msg.Conversation.Type)),
				slog.String("conversation_id", strings.TrimSpace(msg.Conversation.ID)),
			)
		}
		return nil
	}

	identity := state.Identity
	resolvedAttachments := p.ingestInboundAttachments(ctx, cfg, msg, strings.TrimSpace(identity.BotID), msg.Message.Attachments)
	attachments := mapChannelToChatAttachments(resolvedAttachments)

	// Resolve or create the route via channel_routes.
	if p.routeResolver == nil {
		return fmt.Errorf("route resolver not configured")
	}
	routeMetadata := buildRouteMetadata(msg, identity)
	resolved, err := p.routeResolver.ResolveConversation(ctx, route.ResolveInput{
		BotID:             identity.BotID,
		Platform:          msg.Channel.String(),
		ConversationID:    msg.Conversation.ID,
		ThreadID:          extractThreadID(msg),
		ConversationType:  msg.Conversation.Type,
		ChannelIdentityID: identity.UserID,
		ChannelConfigID:   identity.ChannelConfigID,
		ReplyTarget:       strings.TrimSpace(msg.ReplyTarget),
		Metadata:          routeMetadata,
	})
	if err != nil {
		return fmt.Errorf("resolve route conversation: %w", err)
	}
	// Bot-centric history container:
	// always persist channel traffic under bot_id so WebUI can view unified cross-platform history.
	activeChatID := strings.TrimSpace(identity.BotID)
	if activeChatID == "" {
		activeChatID = strings.TrimSpace(resolved.ChatID)
	}
	// Determine inbox action: trigger (immediate response) or notify (passive).
	inboxAction := inbox.ActionNotify
	if shouldTriggerAssistantResponse(msg) || identity.ForceReply {
		inboxAction = inbox.ActionTrigger
	}

	// All messages go through inbox first.
	inboxItem := p.createInboxItem(ctx, identity, msg, text, attachments, resolved.RouteID, inboxAction)

	if inboxAction != inbox.ActionTrigger {
		if p.logger != nil {
			p.logger.Info(
				"inbound not triggering assistant (group trigger condition not met)",
				slog.String("channel", msg.Channel.String()),
				slog.String("bot_id", strings.TrimSpace(identity.BotID)),
				slog.String("route_id", strings.TrimSpace(resolved.RouteID)),
				slog.Bool("is_mentioned", metadataBool(msg.Metadata, "is_mentioned")),
				slog.Bool("is_reply_to_bot", metadataBool(msg.Metadata, "is_reply_to_bot")),
				slog.String("conversation_type", strings.TrimSpace(msg.Conversation.Type)),
				slog.String("query", strings.TrimSpace(text)),
				slog.Int("attachments", len(attachments)),
			)
		}
		if !strings.EqualFold(identity.BotType, "personal") {
			p.persistInboundUser(ctx, resolved.RouteID, identity, msg, text, attachments, "passive_sync")
		}
		return nil
	}

	// Mark the trigger inbox item as read immediately.
	if inboxItem.ID != "" {
		p.markInboxItemRead(ctx, inboxItem)
	}

	userMessagePersisted := p.persistInboundUser(ctx, resolved.RouteID, identity, msg, text, attachments, "active_chat")

	// Issue chat token for reply routing.
	chatToken := ""
	if p.jwtSecret != "" && strings.TrimSpace(msg.ReplyTarget) != "" {
		signed, _, err := auth.GenerateChatToken(auth.ChatToken{
			BotID:             identity.BotID,
			ChatID:            activeChatID,
			RouteID:           resolved.RouteID,
			UserID:            identity.UserID,
			ChannelIdentityID: identity.ChannelIdentityID,
		}, p.jwtSecret, p.tokenTTL)
		if err != nil {
			if p.logger != nil {
				p.logger.Warn("issue chat token failed", slog.Any("error", err))
			}
		} else {
			chatToken = signed
		}
	}

	// Issue user JWT for downstream calls (MCP, schedule, etc.). For guests use chat token as Bearer.
	token := ""
	if identity.UserID != "" && p.jwtSecret != "" {
		signed, _, err := auth.GenerateToken(identity.UserID, p.jwtSecret, p.tokenTTL)
		if err != nil {
			if p.logger != nil {
				p.logger.Warn("issue channel token failed", slog.Any("error", err))
			}
		} else {
			token = "Bearer " + signed
		}
	}
	if token == "" && chatToken != "" {
		token = "Bearer " + chatToken
	}

	var desc channel.Descriptor
	if p.registry != nil {
		desc, _ = p.registry.GetDescriptor(msg.Channel) //nolint:errcheck // descriptor lookup is best-effort
	}
	statusInfo := channel.ProcessingStatusInfo{
		BotID:             identity.BotID,
		ChatID:            activeChatID,
		RouteID:           resolved.RouteID,
		ChannelIdentityID: identity.ChannelIdentityID,
		UserID:            identity.UserID,
		Query:             text,
		ReplyTarget:       strings.TrimSpace(msg.ReplyTarget),
		SourceMessageID:   strings.TrimSpace(msg.Message.ID),
	}
	statusNotifier := p.resolveProcessingStatusNotifier(msg.Channel)
	statusHandle := channel.ProcessingStatusHandle{}
	if statusNotifier != nil {
		handle, notifyErr := p.notifyProcessingStarted(ctx, statusNotifier, cfg, msg, statusInfo)
		if notifyErr != nil {
			p.logProcessingStatusError("processing_started", msg, identity, notifyErr)
		} else {
			statusHandle = handle
		}
	}
	target := strings.TrimSpace(msg.ReplyTarget)
	if target == "" {
		err := fmt.Errorf("reply target missing")
		if statusNotifier != nil {
			if notifyErr := p.notifyProcessingFailed(ctx, statusNotifier, cfg, msg, statusInfo, statusHandle, err); notifyErr != nil {
				p.logProcessingStatusError("processing_failed", msg, identity, notifyErr)
			}
		}
		return err
	}
	sourceMessageID := strings.TrimSpace(msg.Message.ID)
	replyRef := &channel.ReplyRef{Target: target}
	if sourceMessageID != "" {
		replyRef.MessageID = sourceMessageID
	}
	stream, err := sender.OpenStream(ctx, target, channel.StreamOptions{
		Reply:           replyRef,
		SourceMessageID: sourceMessageID,
		Metadata: map[string]any{
			"route_id": resolved.RouteID,
		},
	})
	if err != nil {
		if statusNotifier != nil {
			if notifyErr := p.notifyProcessingFailed(ctx, statusNotifier, cfg, msg, statusInfo, statusHandle, err); notifyErr != nil {
				p.logProcessingStatusError("processing_failed", msg, identity, notifyErr)
			}
		}
		return err
	}
	defer func() {
		_ = stream.Close(context.WithoutCancel(ctx))
	}()

	// For non-local channels, wrap the stream so events are mirrored to the
	// RouteHub (and thus to WebUI/CLI subscribers).
	if p.observer != nil && !isLocalChannelType(msg.Channel) {
		stream = channel.NewTeeStream(stream, p.observer, strings.TrimSpace(identity.BotID), msg.Channel)
		// Broadcast the inbound user message so WebUI can display it.
		p.broadcastInboundMessage(ctx, strings.TrimSpace(identity.BotID), msg, text, identity, resolvedAttachments)
	}

	if err := stream.Push(ctx, channel.StreamEvent{
		Type:   channel.StreamEventStatus,
		Status: channel.StreamStatusStarted,
	}); err != nil {
		if statusNotifier != nil {
			if notifyErr := p.notifyProcessingFailed(ctx, statusNotifier, cfg, msg, statusInfo, statusHandle, err); notifyErr != nil {
				p.logProcessingStatusError("processing_failed", msg, identity, notifyErr)
			}
		}
		return err
	}

	// Mutex-protected collector for outbound asset refs. The resolver's
	// streaming goroutine calls OutboundAssetCollector at persist time.
	var (
		assetMu           sync.Mutex
		outboundAssetRefs []conversation.OutboundAssetRef
	)
	assetCollector := func() []conversation.OutboundAssetRef {
		assetMu.Lock()
		defer assetMu.Unlock()
		result := make([]conversation.OutboundAssetRef, len(outboundAssetRefs))
		copy(result, outboundAssetRefs)
		return result
	}

	chunkCh, streamErrCh := p.runner.StreamChat(ctx, conversation.ChatRequest{
		BotID:                   identity.BotID,
		ChatID:                  activeChatID,
		Token:                   token,
		UserID:                  identity.UserID,
		SourceChannelIdentityID: identity.ChannelIdentityID,
		DisplayName:             identity.DisplayName,
		RouteID:                 resolved.RouteID,
		ChatToken:               chatToken,
		ExternalMessageID:       sourceMessageID,
		ConversationType:        msg.Conversation.Type,
		ConversationName:        msg.Conversation.Name,
		Query:                   text,
		CurrentChannel:          msg.Channel.String(),
		Channels:                []string{msg.Channel.String()},
		UserMessagePersisted:    userMessagePersisted,
		Attachments:             attachments,
		OutboundAssetCollector:  assetCollector,
	})

	var (
		finalMessages       []conversation.ModelMessage
		outboundAttachments []channel.Attachment
		streamErr           error
	)
	for chunkCh != nil || streamErrCh != nil {
		select {
		case chunk, ok := <-chunkCh:
			if !ok {
				chunkCh = nil
				continue
			}
			events, messages, parseErr := mapStreamChunkToChannelEvents(chunk)
			if parseErr != nil {
				if p.logger != nil {
					p.logger.Warn(
						"stream chunk parse failed",
						slog.String("channel", msg.Channel.String()),
						slog.String("channel_identity_id", identity.ChannelIdentityID),
						slog.String("user_id", identity.UserID),
						slog.Any("error", parseErr),
					)
				}
				continue
			}
			for i, event := range events {
				if event.Type == channel.StreamEventAttachment && len(event.Attachments) > 0 {
					ingested := p.ingestOutboundAttachments(ctx, strings.TrimSpace(identity.BotID), event.Attachments)
					events[i].Attachments = ingested
					outboundAttachments = append(outboundAttachments, ingested...)
					assetMu.Lock()
					for _, att := range ingested {
						contentHash := strings.TrimSpace(att.ContentHash)
						if contentHash == "" {
							continue
						}
						ref := conversation.OutboundAssetRef{
							ContentHash: contentHash,
							Role:        "attachment",
							Ordinal:     len(outboundAssetRefs),
							Mime:        strings.TrimSpace(att.Mime),
							SizeBytes:   att.Size,
						}
						if att.Metadata != nil {
							if sk, ok := att.Metadata["storage_key"].(string); ok {
								ref.StorageKey = sk
							}
						}
						outboundAssetRefs = append(outboundAssetRefs, ref)
					}
					assetMu.Unlock()
				}
				if pushErr := stream.Push(ctx, events[i]); pushErr != nil {
					streamErr = pushErr
					break
				}
			}
			if len(messages) > 0 {
				finalMessages = messages
			}
		case err, ok := <-streamErrCh:
			if !ok {
				streamErrCh = nil
				continue
			}
			if err != nil {
				streamErr = err
			}
		}
		if streamErr != nil {
			break
		}
	}

	if streamErr != nil {
		if p.logger != nil {
			p.logger.Error(
				"chat gateway stream failed",
				slog.String("channel", msg.Channel.String()),
				slog.String("channel_identity_id", identity.ChannelIdentityID),
				slog.String("user_id", identity.UserID),
				slog.Any("error", streamErr),
			)
		}
		_ = stream.Push(ctx, channel.StreamEvent{
			Type:  channel.StreamEventError,
			Error: streamErr.Error(),
		})
		if statusNotifier != nil {
			if notifyErr := p.notifyProcessingFailed(ctx, statusNotifier, cfg, msg, statusInfo, statusHandle, streamErr); notifyErr != nil {
				p.logProcessingStatusError("processing_failed", msg, identity, notifyErr)
			}
		}
		return streamErr
	}

	sentTexts, suppressReplies := collectMessageToolContext(p.registry, finalMessages, msg.Channel, target)
	if suppressReplies {
		if err := stream.Push(ctx, channel.StreamEvent{
			Type:   channel.StreamEventStatus,
			Status: channel.StreamStatusCompleted,
		}); err != nil {
			return err
		}
		if statusNotifier != nil {
			if notifyErr := p.notifyProcessingCompleted(ctx, statusNotifier, cfg, msg, statusInfo, statusHandle); notifyErr != nil {
				p.logProcessingStatusError("processing_completed", msg, identity, notifyErr)
			}
		}
		return nil
	}

	outputs := flow.ExtractAssistantOutputs(finalMessages)
	attachmentsApplied := false
	for _, output := range outputs {
		outMessage := buildChannelMessage(output, desc.Capabilities)
		if outMessage.IsEmpty() && !(len(outboundAttachments) > 0 && !attachmentsApplied) {
			continue
		}
		plainText := strings.TrimSpace(outMessage.PlainText())
		if isSilentReplyText(plainText) {
			continue
		}
		if isMessagingToolDuplicate(plainText, sentTexts) {
			continue
		}
		if !attachmentsApplied && len(outboundAttachments) > 0 {
			outMessage.Attachments = append(outMessage.Attachments, outboundAttachments...)
			attachmentsApplied = true
		}
		if outMessage.Reply == nil && sourceMessageID != "" {
			outMessage.Reply = &channel.ReplyRef{
				Target:    target,
				MessageID: sourceMessageID,
			}
		}
		if err := stream.Push(ctx, channel.StreamEvent{
			Type: channel.StreamEventFinal,
			Final: &channel.StreamFinalizePayload{
				Message: outMessage,
			},
		}); err != nil {
			return err
		}
	}
	if !attachmentsApplied && len(outboundAttachments) > 0 {
		attachMsg := channel.Message{Attachments: outboundAttachments}
		if sourceMessageID != "" {
			attachMsg.Reply = &channel.ReplyRef{Target: target, MessageID: sourceMessageID}
		}
		if err := stream.Push(ctx, channel.StreamEvent{
			Type:  channel.StreamEventFinal,
			Final: &channel.StreamFinalizePayload{Message: attachMsg},
		}); err != nil {
			return err
		}
	}
	if err := stream.Push(ctx, channel.StreamEvent{
		Type:   channel.StreamEventStatus,
		Status: channel.StreamStatusCompleted,
	}); err != nil {
		return err
	}
	if statusNotifier != nil {
		if notifyErr := p.notifyProcessingCompleted(ctx, statusNotifier, cfg, msg, statusInfo, statusHandle); notifyErr != nil {
			p.logProcessingStatusError("processing_completed", msg, identity, notifyErr)
		}
	}
	return nil
}

func shouldTriggerAssistantResponse(msg channel.InboundMessage) bool {
	if isDirectConversationType(msg.Conversation.Type) {
		return true
	}
	if metadataBool(msg.Metadata, "is_mentioned") {
		return true
	}
	if metadataBool(msg.Metadata, "is_reply_to_bot") {
		return true
	}
	return hasCommandPrefix(msg.Message.PlainText(), msg.Metadata)
}

func isDirectConversationType(conversationType string) bool {
	ct := strings.ToLower(strings.TrimSpace(conversationType))
	return ct == "" || ct == "p2p" || ct == "private" || ct == "direct"
}

func hasCommandPrefix(text string, metadata map[string]any) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	prefixes := []string{"/"}
	if metadata != nil {
		if raw, ok := metadata["command_prefix"]; ok {
			if value := strings.TrimSpace(fmt.Sprint(raw)); value != "" {
				prefixes = []string{value}
			}
		}
		if raw, ok := metadata["command_prefixes"]; ok {
			if parsed := parseCommandPrefixes(raw); len(parsed) > 0 {
				prefixes = parsed
			}
		}
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func parseCommandPrefixes(raw any) []string {
	if items, ok := raw.([]string); ok {
		result := make([]string, 0, len(items))
		for _, item := range items {
			value := strings.TrimSpace(item)
			if value == "" {
				continue
			}
			result = append(result, value)
		}
		return result
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(fmt.Sprint(item))
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	return result
}

func metadataBool(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	raw, ok := metadata[key]
	if !ok {
		return false
	}
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func (p *ChannelInboundProcessor) persistInboundUser(
	ctx context.Context,
	routeID string,
	identity InboundIdentity,
	msg channel.InboundMessage,
	query string,
	attachments []conversation.ChatAttachment,
	triggerMode string,
) bool {
	if p.message == nil {
		return false
	}
	botID := strings.TrimSpace(identity.BotID)
	if botID == "" {
		return false
	}
	var attachmentPaths []string
	for _, att := range attachments {
		if ap := strings.TrimSpace(att.Path); ap != "" {
			attachmentPaths = append(attachmentPaths, ap)
		}
	}
	headerifiedQuery := flow.FormatUserHeader(
		strings.TrimSpace(msg.Message.ID),
		strings.TrimSpace(identity.ChannelIdentityID),
		strings.TrimSpace(identity.DisplayName),
		msg.Channel.String(),
		strings.TrimSpace(msg.Conversation.Type),
		strings.TrimSpace(msg.Conversation.Name),
		attachmentPaths,
		query,
	)
	payload, err := json.Marshal(conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(headerifiedQuery),
	})
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("marshal inbound user message failed", slog.Any("error", err))
		}
		return false
	}
	meta := map[string]any{
		"route_id":     strings.TrimSpace(routeID),
		"platform":     msg.Channel.String(),
		"trigger_mode": strings.TrimSpace(triggerMode),
	}
	if _, err := p.message.Persist(ctx, messagepkg.PersistInput{
		BotID:                   botID,
		RouteID:                 strings.TrimSpace(routeID),
		SenderChannelIdentityID: strings.TrimSpace(identity.ChannelIdentityID),
		SenderUserID:            strings.TrimSpace(identity.UserID),
		Platform:                msg.Channel.String(),
		ExternalMessageID:       strings.TrimSpace(msg.Message.ID),
		Role:                    "user",
		Content:                 payload,
		Metadata:                meta,
		Assets:                  chatAttachmentsToAssetRefs(attachments),
	}); err != nil && p.logger != nil {
		p.logger.Warn("persist inbound user message failed", slog.Any("error", err))
		return false
	}
	return true
}

func (p *ChannelInboundProcessor) createInboxItem(
	ctx context.Context,
	ident InboundIdentity,
	msg channel.InboundMessage,
	text string,
	attachments []conversation.ChatAttachment,
	routeID string,
	action string,
) inbox.Item {
	if p.inboxService == nil {
		return inbox.Item{}
	}
	botID := strings.TrimSpace(ident.BotID)
	if botID == "" {
		return inbox.Item{}
	}
	trimmedText := strings.TrimSpace(text)
	if trimmedText == "" && len(attachments) == 0 {
		return inbox.Item{}
	}
	displayName := strings.TrimSpace(ident.DisplayName)
	if displayName == "" {
		displayName = "Unknown"
	}

	var attachmentPaths []string
	for _, att := range attachments {
		if p := strings.TrimSpace(att.Path); p != "" {
			attachmentPaths = append(attachmentPaths, p)
		}
	}

	meta := flow.BuildUserMessageMeta(
		strings.TrimSpace(msg.Message.ID),
		strings.TrimSpace(ident.ChannelIdentityID),
		displayName,
		msg.Channel.String(),
		strings.TrimSpace(msg.Conversation.Type),
		strings.TrimSpace(msg.Conversation.Name),
		attachmentPaths,
	)
	header := meta.ToMap()
	header["route_id"] = strings.TrimSpace(routeID)

	item, err := p.inboxService.Create(ctx, inbox.CreateRequest{
		BotID:   botID,
		Source:  msg.Channel.String(),
		Header:  header,
		Content: trimmedText,
		Action:  action,
	})
	if err != nil && p.logger != nil {
		p.logger.Warn("create inbox item failed", slog.Any("error", err), slog.String("bot_id", botID))
	}
	return item
}

func (p *ChannelInboundProcessor) markInboxItemRead(ctx context.Context, item inbox.Item) {
	if p.inboxService == nil || item.ID == "" {
		return
	}
	if err := p.inboxService.MarkRead(ctx, item.BotID, []string{item.ID}); err != nil && p.logger != nil {
		p.logger.Warn("mark inbox item read failed", slog.Any("error", err), slog.String("item_id", item.ID))
	}
}

func buildChannelMessage(output conversation.AssistantOutput, capabilities channel.ChannelCapabilities) channel.Message {
	msg := channel.Message{}
	if strings.TrimSpace(output.Content) != "" {
		msg.Text = strings.TrimSpace(output.Content)
		if containsMarkdown(msg.Text) && (capabilities.Markdown || capabilities.RichText) {
			msg.Format = channel.MessageFormatMarkdown
		}
	}
	if len(output.Parts) == 0 {
		return msg
	}
	if capabilities.RichText {
		parts := make([]channel.MessagePart, 0, len(output.Parts))
		for _, part := range output.Parts {
			if !contentPartHasValue(part) {
				continue
			}
			partType := normalizeContentPartType(part.Type)
			parts = append(parts, channel.MessagePart{
				Type:              partType,
				Text:              part.Text,
				URL:               part.URL,
				Styles:            normalizeContentPartStyles(part.Styles),
				Language:          part.Language,
				ChannelIdentityID: part.ChannelIdentityID,
				Emoji:             part.Emoji,
			})
		}
		if len(parts) > 0 {
			msg.Parts = parts
			msg.Format = channel.MessageFormatRich
		}
		return msg
	}
	textParts := make([]string, 0, len(output.Parts))
	for _, part := range output.Parts {
		if !contentPartHasValue(part) {
			continue
		}
		textParts = append(textParts, strings.TrimSpace(contentPartText(part)))
	}
	if len(textParts) > 0 {
		msg.Text = strings.Join(textParts, "\n")
		if msg.Format == "" && containsMarkdown(msg.Text) && (capabilities.Markdown || capabilities.RichText) {
			msg.Format = channel.MessageFormatMarkdown
		}
	}
	return msg
}

func containsMarkdown(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	patterns := []string{
		`\\*\\*[^*]+\\*\\*`,
		`\\*[^*]+\\*`,
		`~~[^~]+~~`,
		"`[^`]+`",
		"```[\\s\\S]*```",
		`\\[.+\\]\\(.+\\)`,
		`(?m)^#{1,6}\\s`,
		`(?m)^[-*]\\s`,
		`(?m)^\\d+\\.\\s`,
	}
	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			return true
		}
	}
	return false
}

func contentPartHasValue(part conversation.ContentPart) bool {
	if strings.TrimSpace(part.Text) != "" {
		return true
	}
	if strings.TrimSpace(part.URL) != "" {
		return true
	}
	if strings.TrimSpace(part.Emoji) != "" {
		return true
	}
	return false
}

func contentPartText(part conversation.ContentPart) string {
	if strings.TrimSpace(part.Text) != "" {
		return part.Text
	}
	if strings.TrimSpace(part.URL) != "" {
		return part.URL
	}
	if strings.TrimSpace(part.Emoji) != "" {
		return part.Emoji
	}
	return ""
}

type gatewayStreamEnvelope struct {
	Type     string                      `json:"type"`
	Delta    string                      `json:"delta"`
	Error    string                      `json:"error"`
	Message  string                      `json:"message"`
	Image    string                      `json:"image"`
	Data     json.RawMessage             `json:"data"`
	Messages []conversation.ModelMessage `json:"messages"`

	ToolName    string          `json:"toolName"`
	ToolCallID  string          `json:"toolCallId"`
	Input       json.RawMessage `json:"input"`
	Result      json.RawMessage `json:"result"`
	Attachments json.RawMessage `json:"attachments"`
}

type gatewayStreamDoneData struct {
	Messages []conversation.ModelMessage `json:"messages"`
}

func mapStreamChunkToChannelEvents(chunk conversation.StreamChunk) ([]channel.StreamEvent, []conversation.ModelMessage, error) {
	if len(chunk) == 0 {
		return nil, nil, nil
	}
	var envelope gatewayStreamEnvelope
	if err := json.Unmarshal(chunk, &envelope); err != nil {
		return nil, nil, err
	}
	finalMessages := make([]conversation.ModelMessage, 0, len(envelope.Messages))
	finalMessages = append(finalMessages, envelope.Messages...)
	if len(finalMessages) == 0 && len(envelope.Data) > 0 {
		var done gatewayStreamDoneData
		if err := json.Unmarshal(envelope.Data, &done); err == nil && len(done.Messages) > 0 {
			finalMessages = append(finalMessages, done.Messages...)
		}
	}
	eventType := strings.ToLower(strings.TrimSpace(envelope.Type))
	switch eventType {
	case "text_delta":
		if envelope.Delta == "" {
			return nil, finalMessages, nil
		}
		return []channel.StreamEvent{
			{
				Type:  channel.StreamEventDelta,
				Delta: envelope.Delta,
				Phase: channel.StreamPhaseText,
			},
		}, finalMessages, nil
	case "reasoning_delta":
		if envelope.Delta == "" {
			return nil, finalMessages, nil
		}
		return []channel.StreamEvent{
			{
				Type:  channel.StreamEventDelta,
				Delta: envelope.Delta,
				Phase: channel.StreamPhaseReasoning,
			},
		}, finalMessages, nil
	case "tool_call_start":
		return []channel.StreamEvent{
			{
				Type: channel.StreamEventToolCallStart,
				ToolCall: &channel.StreamToolCall{
					Name:   strings.TrimSpace(envelope.ToolName),
					CallID: strings.TrimSpace(envelope.ToolCallID),
					Input:  parseRawJSON(envelope.Input),
				},
			},
		}, finalMessages, nil
	case "tool_call_end":
		return []channel.StreamEvent{
			{
				Type: channel.StreamEventToolCallEnd,
				ToolCall: &channel.StreamToolCall{
					Name:   strings.TrimSpace(envelope.ToolName),
					CallID: strings.TrimSpace(envelope.ToolCallID),
					Input:  parseRawJSON(envelope.Input),
					Result: parseRawJSON(envelope.Result),
				},
			},
		}, finalMessages, nil
	case "reasoning_start":
		return []channel.StreamEvent{
			{Type: channel.StreamEventPhaseStart, Phase: channel.StreamPhaseReasoning},
		}, finalMessages, nil
	case "reasoning_end":
		return []channel.StreamEvent{
			{Type: channel.StreamEventPhaseEnd, Phase: channel.StreamPhaseReasoning},
		}, finalMessages, nil
	case "text_start":
		return []channel.StreamEvent{
			{Type: channel.StreamEventPhaseStart, Phase: channel.StreamPhaseText},
		}, finalMessages, nil
	case "text_end":
		return []channel.StreamEvent{
			{Type: channel.StreamEventPhaseEnd, Phase: channel.StreamPhaseText},
		}, finalMessages, nil
	case "attachment_delta":
		attachments := parseAttachmentDelta(envelope.Attachments)
		if len(attachments) == 0 {
			return nil, finalMessages, nil
		}
		return []channel.StreamEvent{
			{Type: channel.StreamEventAttachment, Attachments: attachments},
		}, finalMessages, nil
	case "agent_start":
		return []channel.StreamEvent{
			{
				Type: channel.StreamEventAgentStart,
				Metadata: map[string]any{
					"input": parseRawJSON(envelope.Input),
					"data":  parseRawJSON(envelope.Data),
				},
			},
		}, finalMessages, nil
	case "agent_end":
		return []channel.StreamEvent{
			{
				Type: channel.StreamEventAgentEnd,
				Metadata: map[string]any{
					"result": parseRawJSON(envelope.Result),
					"data":   parseRawJSON(envelope.Data),
				},
			},
		}, finalMessages, nil
	case "processing_started":
		return []channel.StreamEvent{
			{Type: channel.StreamEventProcessingStarted},
		}, finalMessages, nil
	case "processing_completed":
		return []channel.StreamEvent{
			{Type: channel.StreamEventProcessingCompleted},
		}, finalMessages, nil
	case "processing_failed":
		streamError := strings.TrimSpace(envelope.Error)
		if streamError == "" {
			streamError = strings.TrimSpace(envelope.Message)
		}
		return []channel.StreamEvent{
			{
				Type:  channel.StreamEventProcessingFailed,
				Error: streamError,
			},
		}, finalMessages, nil
	case "error":
		streamError := strings.TrimSpace(envelope.Error)
		if streamError == "" {
			streamError = strings.TrimSpace(envelope.Message)
		}
		if streamError == "" {
			streamError = "stream error"
		}
		return []channel.StreamEvent{
			{
				Type:  channel.StreamEventError,
				Error: streamError,
			},
		}, finalMessages, nil
	default:
		return nil, finalMessages, nil
	}
}

func buildInboundQuery(message channel.Message) string {
	text := strings.TrimSpace(message.PlainText())
	if text != "" {
		return text
	}
	if len(message.Attachments) == 0 {
		return ""
	}
	count := len(message.Attachments)
	if count == 1 {
		return "[User sent 1 attachment]"
	}
	return fmt.Sprintf("[User sent %d attachments]", count)
}

func normalizeContentPartType(raw string) channel.MessagePartType {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "link":
		return channel.MessagePartLink
	case "code_block":
		return channel.MessagePartCodeBlock
	case "mention":
		return channel.MessagePartMention
	case "emoji":
		return channel.MessagePartEmoji
	default:
		return channel.MessagePartText
	}
}

func normalizeContentPartStyles(styles []string) []channel.MessageTextStyle {
	if len(styles) == 0 {
		return nil
	}
	result := make([]channel.MessageTextStyle, 0, len(styles))
	for _, style := range styles {
		switch strings.TrimSpace(strings.ToLower(style)) {
		case "bold":
			result = append(result, channel.MessageStyleBold)
		case "italic":
			result = append(result, channel.MessageStyleItalic)
		case "strikethrough", "lineThrough":
			result = append(result, channel.MessageStyleStrikethrough)
		case "code":
			result = append(result, channel.MessageStyleCode)
		default:
			continue
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

type sendMessageToolArgs struct {
	Platform          string           `json:"platform"`
	Target            string           `json:"target"`
	ChannelIdentityID string           `json:"channel_identity_id"`
	Text              string           `json:"text"`
	Message           *channel.Message `json:"message"`
}

func collectMessageToolContext(registry *channel.Registry, messages []conversation.ModelMessage, channelType channel.ChannelType, replyTarget string) ([]string, bool) {
	if len(messages) == 0 {
		return nil, false
	}
	var sentTexts []string
	suppressReplies := false
	for _, msg := range messages {
		for _, tc := range msg.ToolCalls {
			if tc.Function.Name != "send" && tc.Function.Name != "send_message" {
				continue
			}
			var args sendMessageToolArgs
			if !parseToolArguments(tc.Function.Arguments, &args) {
				continue
			}
			if text := strings.TrimSpace(extractSendMessageText(args)); text != "" {
				sentTexts = append(sentTexts, text)
			}
			if shouldSuppressForToolCall(registry, args, channelType, replyTarget) {
				suppressReplies = true
			}
		}
	}
	return sentTexts, suppressReplies
}

func parseToolArguments(raw string, out any) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	if err := json.Unmarshal([]byte(raw), out); err == nil {
		return true
	}
	var decoded string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return false
	}
	if strings.TrimSpace(decoded) == "" {
		return false
	}
	return json.Unmarshal([]byte(decoded), out) == nil
}

func extractSendMessageText(args sendMessageToolArgs) string {
	if strings.TrimSpace(args.Text) != "" {
		return strings.TrimSpace(args.Text)
	}
	if args.Message == nil {
		return ""
	}
	return strings.TrimSpace(args.Message.PlainText())
}

func shouldSuppressForToolCall(registry *channel.Registry, args sendMessageToolArgs, channelType channel.ChannelType, replyTarget string) bool {
	platform := strings.TrimSpace(args.Platform)
	if platform == "" {
		platform = string(channelType)
	}
	if !strings.EqualFold(platform, string(channelType)) {
		return false
	}
	target := strings.TrimSpace(args.Target)
	if target == "" && strings.TrimSpace(args.ChannelIdentityID) == "" {
		target = replyTarget
	}
	if strings.TrimSpace(target) == "" || strings.TrimSpace(replyTarget) == "" {
		return false
	}
	normalizedTarget := normalizeReplyTarget(registry, channelType, target)
	normalizedReply := normalizeReplyTarget(registry, channelType, replyTarget)
	if normalizedTarget == "" || normalizedReply == "" {
		return false
	}
	return normalizedTarget == normalizedReply
}

func normalizeReplyTarget(registry *channel.Registry, channelType channel.ChannelType, target string) string {
	if registry == nil {
		return strings.TrimSpace(target)
	}
	normalized, ok := registry.NormalizeTarget(channelType, target)
	if ok && strings.TrimSpace(normalized) != "" {
		return strings.TrimSpace(normalized)
	}
	return strings.TrimSpace(target)
}

func isSilentReplyText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	token := []rune(silentReplyToken)
	value := []rune(trimmed)
	if len(value) < len(token) {
		return false
	}
	if hasTokenPrefix(value, token) {
		return true
	}
	if hasTokenSuffix(value, token) {
		return true
	}
	return false
}

func hasTokenPrefix(value []rune, token []rune) bool {
	if len(value) < len(token) {
		return false
	}
	for i := range token {
		if value[i] != token[i] {
			return false
		}
	}
	if len(value) == len(token) {
		return true
	}
	return !isWordChar(value[len(token)])
}

func hasTokenSuffix(value []rune, token []rune) bool {
	if len(value) < len(token) {
		return false
	}
	start := len(value) - len(token)
	for i := range token {
		if value[start+i] != token[i] {
			return false
		}
	}
	if start == 0 {
		return true
	}
	return !isWordChar(value[start-1])
}

func isWordChar(value rune) bool {
	return value == '_' || unicode.IsLetter(value) || unicode.IsDigit(value)
}

func normalizeTextForComparison(text string) string {
	trimmed := strings.TrimSpace(strings.ToLower(text))
	if trimmed == "" {
		return ""
	}
	return strings.TrimSpace(whitespacePattern.ReplaceAllString(trimmed, " "))
}

func isMessagingToolDuplicate(text string, sentTexts []string) bool {
	if len(sentTexts) == 0 {
		return false
	}
	normalized := normalizeTextForComparison(text)
	if len(normalized) < minDuplicateTextLength {
		return false
	}
	for _, sent := range sentTexts {
		sentNormalized := normalizeTextForComparison(sent)
		if len(sentNormalized) < minDuplicateTextLength {
			continue
		}
		if strings.Contains(normalized, sentNormalized) || strings.Contains(sentNormalized, normalized) {
			return true
		}
	}
	return false
}

// requireIdentity resolves identity for the current message. Always resolves from msg so each sender is identified correctly (no reuse of context state across messages).
func (p *ChannelInboundProcessor) requireIdentity(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage) (IdentityState, error) {
	if p.identity == nil {
		return IdentityState{}, fmt.Errorf("identity resolver not configured")
	}
	return p.identity.Resolve(ctx, cfg, msg)
}

func (p *ChannelInboundProcessor) resolveProcessingStatusNotifier(channelType channel.ChannelType) channel.ProcessingStatusNotifier {
	if p == nil || p.registry == nil {
		return nil
	}
	notifier, ok := p.registry.GetProcessingStatusNotifier(channelType)
	if !ok {
		return nil
	}
	return notifier
}

func (p *ChannelInboundProcessor) notifyProcessingStarted(
	ctx context.Context,
	notifier channel.ProcessingStatusNotifier,
	cfg channel.ChannelConfig,
	msg channel.InboundMessage,
	info channel.ProcessingStatusInfo,
) (channel.ProcessingStatusHandle, error) {
	if notifier == nil {
		return channel.ProcessingStatusHandle{}, nil
	}
	statusCtx, cancel := context.WithTimeout(ctx, processingStatusTimeout)
	defer cancel()
	return notifier.ProcessingStarted(statusCtx, cfg, msg, info)
}

func (p *ChannelInboundProcessor) notifyProcessingCompleted(
	ctx context.Context,
	notifier channel.ProcessingStatusNotifier,
	cfg channel.ChannelConfig,
	msg channel.InboundMessage,
	info channel.ProcessingStatusInfo,
	handle channel.ProcessingStatusHandle,
) error {
	if notifier == nil {
		return nil
	}
	statusCtx, cancel := context.WithTimeout(ctx, processingStatusTimeout)
	defer cancel()
	return notifier.ProcessingCompleted(statusCtx, cfg, msg, info, handle)
}

func (p *ChannelInboundProcessor) notifyProcessingFailed(
	ctx context.Context,
	notifier channel.ProcessingStatusNotifier,
	cfg channel.ChannelConfig,
	msg channel.InboundMessage,
	info channel.ProcessingStatusInfo,
	handle channel.ProcessingStatusHandle,
	cause error,
) error {
	if notifier == nil {
		return nil
	}
	statusCtx, cancel := context.WithTimeout(ctx, processingStatusTimeout)
	defer cancel()
	return notifier.ProcessingFailed(statusCtx, cfg, msg, info, handle, cause)
}

func (p *ChannelInboundProcessor) logProcessingStatusError(
	stage string,
	msg channel.InboundMessage,
	identity InboundIdentity,
	err error,
) {
	if p == nil || p.logger == nil || err == nil {
		return
	}
	p.logger.Warn(
		"processing status notify failed",
		slog.String("stage", stage),
		slog.String("channel", msg.Channel.String()),
		slog.String("channel_identity_id", identity.ChannelIdentityID),
		slog.String("user_id", identity.UserID),
		slog.Any("error", err),
	)
}

// parseRawJSON converts raw JSON bytes to a typed value for StreamToolCall fields.
func parseRawJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}

func (p *ChannelInboundProcessor) ingestInboundAttachments(
	ctx context.Context,
	cfg channel.ChannelConfig,
	msg channel.InboundMessage,
	botID string,
	attachments []channel.Attachment,
) []channel.Attachment {
	if len(attachments) == 0 || p == nil || p.mediaService == nil || strings.TrimSpace(botID) == "" {
		return attachments
	}
	result := make([]channel.Attachment, 0, len(attachments))
	for _, att := range attachments {
		item := att
		if strings.TrimSpace(item.ContentHash) != "" {
			result = append(result, item)
			continue
		}
		payload, err := p.loadInboundAttachmentPayload(ctx, cfg, msg, item)
		if err != nil {
			if p.logger != nil {
				p.logger.Warn(
					"inbound attachment ingest skipped",
					slog.Any("error", err),
					slog.String("attachment_type", strings.TrimSpace(string(item.Type))),
					slog.String("attachment_url", strings.TrimSpace(item.URL)),
					slog.String("platform_key", strings.TrimSpace(item.PlatformKey)),
				)
			}
			result = append(result, item)
			continue
		}
		sourceMime := attachment.NormalizeMime(item.Mime)
		if sourceMime == "" {
			sourceMime = attachment.NormalizeMime(payload.mime)
		}
		if strings.TrimSpace(item.Name) == "" {
			item.Name = strings.TrimSpace(payload.name)
		}
		if item.Size == 0 && payload.size > 0 {
			item.Size = payload.size
		}
		mediaType := attachment.MapMediaType(string(item.Type))
		preparedReader, finalMime, err := attachment.PrepareReaderAndMime(payload.reader, mediaType, sourceMime)
		if err != nil {
			if payload.reader != nil {
				_ = payload.reader.Close()
			}
			if p.logger != nil {
				p.logger.Warn(
					"inbound attachment mime prepare failed",
					slog.Any("error", err),
					slog.String("attachment_type", strings.TrimSpace(string(item.Type))),
					slog.String("attachment_url", strings.TrimSpace(item.URL)),
					slog.String("platform_key", strings.TrimSpace(item.PlatformKey)),
				)
			}
			result = append(result, item)
			continue
		}
		item.Mime = finalMime
		maxBytes := media.MaxAssetBytes
		asset, err := p.mediaService.Ingest(ctx, media.IngestInput{
			BotID:    botID,
			Mime:     strings.TrimSpace(item.Mime),
			Reader:   preparedReader,
			MaxBytes: maxBytes,
		})
		if payload.reader != nil {
			_ = payload.reader.Close()
		}
		if err != nil {
			if p.logger != nil {
				p.logger.Warn(
					"inbound attachment ingest failed",
					slog.Any("error", err),
					slog.String("attachment_type", strings.TrimSpace(string(item.Type))),
					slog.String("attachment_url", strings.TrimSpace(item.URL)),
					slog.String("platform_key", strings.TrimSpace(item.PlatformKey)),
				)
			}
			result = append(result, item)
			continue
		}
		item.ContentHash = asset.ContentHash
		item.URL = p.mediaService.AccessPath(asset)
		item.PlatformKey = ""
		item.Base64 = ""
		if item.Metadata == nil {
			item.Metadata = make(map[string]any)
		}
		item.Metadata["bot_id"] = botID
		item.Metadata["storage_key"] = asset.StorageKey
		if strings.TrimSpace(item.Mime) == "" {
			item.Mime = attachment.NormalizeMime(asset.Mime)
		}
		if item.Size == 0 && asset.SizeBytes > 0 {
			item.Size = asset.SizeBytes
		}
		result = append(result, item)
	}
	return result
}

type inboundAttachmentPayload struct {
	reader io.ReadCloser
	mime   string
	name   string
	size   int64
}

func (p *ChannelInboundProcessor) loadInboundAttachmentPayload(
	ctx context.Context,
	cfg channel.ChannelConfig,
	msg channel.InboundMessage,
	att channel.Attachment,
) (inboundAttachmentPayload, error) {
	rawURL := strings.TrimSpace(att.URL)
	if rawURL != "" {
		payload, err := openInboundAttachmentURL(ctx, rawURL)
		if err == nil {
			if strings.TrimSpace(att.Mime) != "" {
				payload.mime = strings.TrimSpace(att.Mime)
			}
			if strings.TrimSpace(payload.name) == "" {
				payload.name = strings.TrimSpace(att.Name)
			}
			return payload, nil
		}
		// When URL download fails and no other source exists, return URL error.
		if strings.TrimSpace(att.PlatformKey) == "" && strings.TrimSpace(att.Base64) == "" {
			return inboundAttachmentPayload{}, err
		}
	}
	rawBase64 := strings.TrimSpace(att.Base64)
	if rawBase64 != "" {
		decoded, err := attachment.DecodeBase64(rawBase64, media.MaxAssetBytes)
		if err != nil {
			return inboundAttachmentPayload{}, fmt.Errorf("decode attachment base64: %w", err)
		}
		mimeType := strings.TrimSpace(att.Mime)
		if mimeType == "" {
			mimeType = strings.TrimSpace(attachment.MimeFromDataURL(rawBase64))
		}
		return inboundAttachmentPayload{
			reader: io.NopCloser(decoded),
			mime:   mimeType,
			name:   strings.TrimSpace(att.Name),
		}, nil
	}
	platformKey := strings.TrimSpace(att.PlatformKey)
	if platformKey == "" {
		return inboundAttachmentPayload{}, fmt.Errorf("attachment has no ingestible payload")
	}
	resolver := p.resolveAttachmentResolver(msg.Channel)
	if resolver == nil {
		return inboundAttachmentPayload{}, fmt.Errorf("attachment resolver not supported for channel: %s", msg.Channel.String())
	}
	resolved, err := resolver.ResolveAttachment(ctx, cfg, att)
	if err != nil {
		return inboundAttachmentPayload{}, fmt.Errorf("resolve attachment by platform key: %w", err)
	}
	if resolved.Reader == nil {
		return inboundAttachmentPayload{}, fmt.Errorf("resolved attachment reader is nil")
	}
	mime := strings.TrimSpace(att.Mime)
	if mime == "" {
		mime = strings.TrimSpace(resolved.Mime)
	}
	name := strings.TrimSpace(att.Name)
	if name == "" {
		name = strings.TrimSpace(resolved.Name)
	}
	return inboundAttachmentPayload{
		reader: resolved.Reader,
		mime:   mime,
		name:   name,
		size:   resolved.Size,
	}, nil
}

func openInboundAttachmentURL(ctx context.Context, rawURL string) (inboundAttachmentPayload, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return inboundAttachmentPayload{}, fmt.Errorf("build request: %w", err)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return inboundAttachmentPayload{}, fmt.Errorf("download attachment: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_ = resp.Body.Close()
		return inboundAttachmentPayload{}, fmt.Errorf("download attachment status: %d", resp.StatusCode)
	}
	maxBytes := media.MaxAssetBytes
	if resp.ContentLength > maxBytes {
		_ = resp.Body.Close()
		return inboundAttachmentPayload{}, fmt.Errorf("%w: max %d bytes", media.ErrAssetTooLarge, maxBytes)
	}
	mime := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if idx := strings.Index(mime, ";"); idx >= 0 {
		mime = strings.TrimSpace(mime[:idx])
	}
	return inboundAttachmentPayload{
		reader: resp.Body,
		mime:   mime,
		size:   resp.ContentLength,
	}, nil
}

func (p *ChannelInboundProcessor) resolveAttachmentResolver(channelType channel.ChannelType) channel.AttachmentResolver {
	if p == nil || p.registry == nil {
		return nil
	}
	resolver, ok := p.registry.GetAttachmentResolver(channelType)
	if !ok {
		return nil
	}
	return resolver
}

// ingestOutboundAttachments persists LLM-generated attachment data URLs via the
// media service, replacing ephemeral data URLs with stable asset references.
// For container-internal paths (non-HTTP), it attempts to resolve the existing
// asset by matching the storage key extracted from the path.
func (p *ChannelInboundProcessor) ingestOutboundAttachments(ctx context.Context, botID string, attachments []channel.Attachment) []channel.Attachment {
	if len(attachments) == 0 || p.mediaService == nil || strings.TrimSpace(botID) == "" {
		return attachments
	}
	result := make([]channel.Attachment, 0, len(attachments))
	for _, att := range attachments {
		item := att
		rawURL := strings.TrimSpace(item.URL)
		if strings.TrimSpace(item.ContentHash) != "" {
			result = append(result, item)
			continue
		}
		// Non-data-URL, non-HTTP path: try to resolve as an existing asset via storage key.
		if rawURL != "" && !isDataURL(rawURL) && !isHTTPURL(rawURL) {
			if resolved := p.resolveContainerPathAsset(ctx, botID, rawURL, &item); resolved {
				result = append(result, item)
				continue
			}
			result = append(result, item)
			continue
		}
		if !isDataURL(rawURL) {
			result = append(result, item)
			continue
		}
		decoded, err := attachment.DecodeBase64(rawURL, media.MaxAssetBytes)
		if err != nil {
			if p.logger != nil {
				p.logger.Warn("decode outbound attachment data url failed", slog.Any("error", err))
			}
			result = append(result, item)
			continue
		}
		mimeType := attachment.NormalizeMime(item.Mime)
		if mimeType == "" {
			mimeType = attachment.MimeFromDataURL(rawURL)
		}
		asset, err := p.mediaService.Ingest(ctx, media.IngestInput{
			BotID:    botID,
			Mime:     mimeType,
			Reader:   decoded,
			MaxBytes: media.MaxAssetBytes,
		})
		if err != nil {
			if p.logger != nil {
				p.logger.Warn("ingest outbound attachment failed", slog.Any("error", err))
			}
			result = append(result, item)
			continue
		}
		item.ContentHash = asset.ContentHash
		item.URL = ""
		item.Base64 = ""
		if item.Metadata == nil {
			item.Metadata = make(map[string]any)
		}
		item.Metadata["bot_id"] = botID
		item.Metadata["storage_key"] = asset.StorageKey
		if strings.TrimSpace(item.Mime) == "" {
			item.Mime = attachment.NormalizeMime(asset.Mime)
		}
		if item.Size == 0 && asset.SizeBytes > 0 {
			item.Size = asset.SizeBytes
		}
		result = append(result, item)
	}
	return result
}

func isDataURL(raw string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "data:")
}

func isHTTPURL(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// resolveContainerPathAsset attempts to match a container-internal file path
// to an existing media asset by extracting the storage key from the path.
// For non-media-marker paths, it ingests the file into the media store first.
// Returns true if the asset was resolved and item was updated.
func (p *ChannelInboundProcessor) resolveContainerPathAsset(ctx context.Context, botID, accessPath string, item *channel.Attachment) bool {
	// Try media marker lookup first.
	storageKey := extractStorageKey(accessPath, botID)
	if storageKey != "" {
		asset, err := p.mediaService.GetByStorageKey(ctx, botID, storageKey)
		if err == nil {
			applyAssetToAttachment(asset, botID, item)
			return true
		}
	}

	// For any path starting with data mount, ingest the file into media store.
	dataPrefix := "/data"
	if !strings.HasSuffix(dataPrefix, "/") {
		dataPrefix += "/"
	}
	if strings.HasPrefix(accessPath, dataPrefix) {
		asset, err := p.mediaService.IngestContainerFile(ctx, botID, accessPath)
		if err != nil {
			if p.logger != nil {
				p.logger.Warn("ingest container file for stream failed", slog.String("path", accessPath), slog.Any("error", err))
			}
			return false
		}
		applyAssetToAttachment(asset, botID, item)
		return true
	}

	return false
}

func applyAssetToAttachment(asset media.Asset, botID string, item *channel.Attachment) {
	item.ContentHash = asset.ContentHash
	item.URL = ""
	if item.Metadata == nil {
		item.Metadata = make(map[string]any)
	}
	item.Metadata["bot_id"] = botID
	item.Metadata["storage_key"] = asset.StorageKey
	if strings.TrimSpace(item.Mime) == "" {
		item.Mime = attachment.NormalizeMime(asset.Mime)
	}
	if item.Size == 0 && asset.SizeBytes > 0 {
		item.Size = asset.SizeBytes
	}
}

// extractStorageKey derives the media storage key from a container-internal
// access path. The expected path format is /data/media/<storage_key>.
func extractStorageKey(accessPath string, botID string) string {
	marker := filepath.Join("/data", "media")
	if !strings.HasSuffix(marker, "/") {
		marker += "/"
	}
	idx := strings.Index(accessPath, marker)
	if idx < 0 {
		return ""
	}
	return accessPath[idx+len(marker):]
}

// isLocalChannelType returns true for channels that already publish to RouteHub
// natively (Web, CLI). Wrapping these with a tee would cause duplicate events.
func isLocalChannelType(ct channel.ChannelType) bool {
	s := strings.ToLower(strings.TrimSpace(string(ct)))
	return s == "web" || s == "cli"
}

// broadcastInboundMessage notifies the observer about the user's inbound
// message so WebUI subscribers see the full conversation, not just the bot reply.
func (p *ChannelInboundProcessor) broadcastInboundMessage(
	ctx context.Context,
	botID string,
	msg channel.InboundMessage,
	text string,
	identity InboundIdentity,
	resolvedAttachments []channel.Attachment,
) {
	if p.observer == nil || strings.TrimSpace(botID) == "" {
		return
	}
	inboundMsg := channel.Message{
		Text:        text,
		Attachments: resolvedAttachments,
		Metadata: map[string]any{
			"external_message_id": strings.TrimSpace(msg.Message.ID),
			"sender_display_name": strings.TrimSpace(identity.DisplayName),
		},
	}
	p.observer.OnStreamEvent(ctx, botID, msg.Channel, channel.StreamEvent{
		Type: channel.StreamEventFinal,
		Final: &channel.StreamFinalizePayload{
			Message: inboundMsg,
		},
		Metadata: map[string]any{
			"source_channel": string(msg.Channel),
			"role":           "user",
			"sender_user_id": strings.TrimSpace(identity.UserID),
		},
	})
}

// channelAttachmentsToAssetRefs converts channel Attachments to message AssetRefs
// with full metadata for denormalized persistence.
func channelAttachmentsToAssetRefs(attachments []channel.Attachment, role string) []messagepkg.AssetRef {
	if len(attachments) == 0 {
		return nil
	}
	refs := make([]messagepkg.AssetRef, 0, len(attachments))
	for idx, att := range attachments {
		contentHash := strings.TrimSpace(att.ContentHash)
		if contentHash == "" {
			continue
		}
		ref := messagepkg.AssetRef{
			ContentHash: contentHash,
			Role:        role,
			Ordinal:     idx,
			Mime:        strings.TrimSpace(att.Mime),
			SizeBytes:   att.Size,
		}
		if att.Metadata != nil {
			if sk, ok := att.Metadata["storage_key"].(string); ok {
				ref.StorageKey = sk
			}
		}
		refs = append(refs, ref)
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func chatAttachmentsToAssetRefs(attachments []conversation.ChatAttachment) []messagepkg.AssetRef {
	if len(attachments) == 0 {
		return nil
	}
	refs := make([]messagepkg.AssetRef, 0, len(attachments))
	for idx, att := range attachments {
		contentHash := strings.TrimSpace(att.ContentHash)
		if contentHash == "" {
			continue
		}
		ref := messagepkg.AssetRef{
			ContentHash: contentHash,
			Role:        "attachment",
			Ordinal:     idx,
			Mime:        strings.TrimSpace(att.Mime),
			SizeBytes:   att.Size,
		}
		if att.Metadata != nil {
			if sk, ok := att.Metadata["storage_key"].(string); ok {
				ref.StorageKey = sk
			}
		}
		refs = append(refs, ref)
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func mapChannelToChatAttachments(attachments []channel.Attachment) []conversation.ChatAttachment {
	if len(attachments) == 0 {
		return nil
	}
	result := make([]conversation.ChatAttachment, 0, len(attachments))
	for _, att := range attachments {
		ca := conversation.ChatAttachment{
			Type:        string(att.Type),
			PlatformKey: att.PlatformKey,
			ContentHash: att.ContentHash,
			Name:        att.Name,
			Mime:        attachment.NormalizeMime(att.Mime),
			Size:        att.Size,
			Metadata:    att.Metadata,
			Base64:      attachment.NormalizeBase64DataURL(att.Base64, attachment.NormalizeMime(att.Mime)),
		}
		if strings.TrimSpace(att.ContentHash) != "" {
			ca.Path = att.URL
		} else {
			ca.URL = att.URL
		}
		result = append(result, ca)
	}
	return result
}

// parseAttachmentDelta converts raw JSON attachment data to channel Attachments.
func parseAttachmentDelta(raw json.RawMessage) []channel.Attachment {
	if len(raw) == 0 {
		return nil
	}
	var items []struct {
		Type        string `json:"type"`
		URL         string `json:"url"`
		Path        string `json:"path"`
		PlatformKey string `json:"platform_key"`
		ContentHash string `json:"content_hash"`
		Name        string `json:"name"`
		Mime        string `json:"mime"`
		Size        int64  `json:"size"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	attachments := make([]channel.Attachment, 0, len(items))
	for _, item := range items {
		url := strings.TrimSpace(item.URL)
		if url == "" {
			url = strings.TrimSpace(item.Path)
		}
		attachments = append(attachments, channel.Attachment{
			Type:        channel.AttachmentType(strings.TrimSpace(item.Type)),
			URL:         url,
			PlatformKey: strings.TrimSpace(item.PlatformKey),
			ContentHash: strings.TrimSpace(item.ContentHash),
			Name:        strings.TrimSpace(item.Name),
			Mime:        strings.TrimSpace(item.Mime),
			Size:        item.Size,
		})
	}
	return attachments
}

// buildRouteMetadata extracts user/conversation information for route metadata persistence.
func buildRouteMetadata(msg channel.InboundMessage, identity InboundIdentity) map[string]any {
	m := make(map[string]any)

	if v := strings.TrimSpace(identity.DisplayName); v != "" {
		m["sender_display_name"] = v
	}
	if v := strings.TrimSpace(msg.Sender.SubjectID); v != "" {
		m["sender_id"] = v
	}
	if v := strings.TrimSpace(msg.Conversation.Name); v != "" {
		m["conversation_name"] = v
	}

	for k, v := range msg.Sender.Attributes {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		switch k {
		case "username":
			m["sender_username"] = v
		}
	}

	return m
}
