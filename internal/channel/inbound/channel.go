package inbound

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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

	"github.com/memohai/memoh/internal/acl"
	"github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/command"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/media"
	messagepkg "github.com/memohai/memoh/internal/message"
)

var base64Std = base64.StdEncoding

const (
	silentReplyToken        = "NO_REPLY"
	minDuplicateTextLength  = 10
	processingStatusTimeout = 60 * time.Second
)

var whitespacePattern = regexp.MustCompile(`\s+`)

// RouteResolver resolves and manages channel routes.
type RouteResolver interface {
	ResolveConversation(ctx context.Context, input route.ResolveInput) (route.ResolveConversationResult, error)
}

type channelReactor interface {
	React(ctx context.Context, botID string, channelType channel.ChannelType, req channel.ReactRequest) error
}

type chatACL interface {
	CanPerformChatTrigger(ctx context.Context, req acl.ChatTriggerRequest) (bool, error)
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

// ttsSynthesizer synthesizes text to speech audio.
type ttsSynthesizer interface {
	Synthesize(ctx context.Context, modelID string, text string, overrideCfg map[string]any) ([]byte, string, error)
}

// ttsModelResolver looks up the TTS model ID configured for a bot.
type ttsModelResolver interface {
	ResolveTtsModelID(ctx context.Context, botID string) (string, error)
}

// SessionEnsurer resolves or creates an active session for a route.
type SessionEnsurer interface {
	EnsureActiveSession(ctx context.Context, botID, routeID, channelType string) (SessionResult, error)
	// CreateNewSession always creates a fresh session and sets it as the
	// active session for the given route, replacing any previous one.
	CreateNewSession(ctx context.Context, botID, routeID, channelType string) (SessionResult, error)
}

// SessionResult carries the minimum fields needed from a session.
type SessionResult struct {
	ID string
}

// ChannelInboundProcessor routes channel inbound messages to the chat gateway.
type ChannelInboundProcessor struct {
	runner           flow.Runner
	routeResolver    RouteResolver
	message          messagepkg.Writer
	mediaService     mediaIngestor
	reactor          channelReactor
	commandHandler   *command.Handler
	registry         *channel.Registry
	logger           *slog.Logger
	jwtSecret        string
	tokenTTL         time.Duration
	identity         *IdentityResolver
	policy           PolicyService
	acl              chatACL
	observer         channel.StreamObserver
	ttsService       ttsSynthesizer
	ttsModelResolver ttsModelResolver
	sessionEnsurer   SessionEnsurer
}

// NewChannelInboundProcessor creates a processor with channel identity-based resolution.
func NewChannelInboundProcessor(
	log *slog.Logger,
	registry *channel.Registry,
	routeResolver RouteResolver,
	messageWriter messagepkg.Writer,
	runner flow.Runner,
	channelIdentityService ChannelIdentityService,
	policyService PolicyService,
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
	identityResolver := NewIdentityResolver(log, registry, channelIdentityService, policyService, bindService, "")
	return &ChannelInboundProcessor{
		runner:        runner,
		routeResolver: routeResolver,
		message:       messageWriter,
		registry:      registry,
		logger:        log.With(slog.String("component", "channel_router")),
		jwtSecret:     strings.TrimSpace(jwtSecret),
		tokenTTL:      tokenTTL,
		identity:      identityResolver,
		policy:        policyService,
	}
}

func (p *ChannelInboundProcessor) SetACLService(service chatACL) {
	if p == nil {
		return
	}
	p.acl = service
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

// SetReactor configures the channel reactor for handling inline emoji reactions.
func (p *ChannelInboundProcessor) SetReactor(reactor channelReactor) {
	if p == nil {
		return
	}
	p.reactor = reactor
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

// SetTtsService configures the TTS synthesizer and settings reader for handling
// <speech> tag events (speech_delta) that require server-side audio synthesis.
func (p *ChannelInboundProcessor) SetTtsService(synth ttsSynthesizer, modelResolver ttsModelResolver) {
	if p == nil {
		return
	}
	p.ttsService = synth
	p.ttsModelResolver = modelResolver
}

// SetSessionEnsurer configures the session ensurer for auto-creating sessions on routes.
func (p *ChannelInboundProcessor) SetSessionEnsurer(ensurer SessionEnsurer) {
	if p == nil {
		return
	}
	p.sessionEnsurer = ensurer
}

// SetCommandHandler configures the slash command handler for intercepting
// /command messages before they reach the LLM.
func (p *ChannelInboundProcessor) SetCommandHandler(handler *command.Handler) {
	if p == nil {
		return
	}
	p.commandHandler = handler
}

// HandleInbound processes an inbound channel message through identity resolution and chat gateway.
func (p *ChannelInboundProcessor) HandleInbound(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, sender channel.StreamReplySender) error {
	if p.runner == nil {
		return errors.New("channel inbound processor not configured")
	}
	if sender == nil {
		return errors.New("reply sender not configured")
	}
	text := buildInboundQuery(msg.Message, nil)
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
	if strings.TrimSpace(msg.Message.PlainText()) == "" && len(msg.Message.Attachments) == 0 {
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

	// Intercept slash commands before they reach the LLM.
	// Use raw_text (without prepended quote/forward context) so that
	// quoted content like "[Reply to Bot: /fs list]\n hello" doesn't
	// accidentally match a command.
	// In group chats, only process if the message is directed at this bot
	// (via @mention or reply) to avoid all bots responding to the same command.
	cmdText := rawTextForCommand(msg, text)

	// /new requires route context, so it is handled separately from the
	// general command handler (which runs before route resolution).
	if isNewSessionCommand(cmdText) && isDirectedAtBot(msg) {
		return p.handleNewSessionCommand(ctx, cfg, msg, sender, identity)
	}

	if p.commandHandler != nil && p.commandHandler.IsCommand(cmdText) && isDirectedAtBot(msg) {
		reply, err := p.commandHandler.Execute(ctx, strings.TrimSpace(identity.BotID), strings.TrimSpace(identity.ChannelIdentityID), cmdText)
		if err != nil {
			reply = "Error: " + err.Error()
		}
		return sender.Send(ctx, channel.OutboundMessage{
			Target:  strings.TrimSpace(msg.ReplyTarget),
			Message: channel.Message{Text: reply},
		})
	}

	resolvedAttachments := p.ingestInboundAttachments(ctx, cfg, msg, strings.TrimSpace(identity.BotID), msg.Message.Attachments)
	attachments := mapChannelToChatAttachments(resolvedAttachments)
	text = buildInboundQuery(msg.Message, attachments)
	threadID := extractThreadID(msg)

	// Resolve or create the route via channel_routes.
	if p.routeResolver == nil {
		return errors.New("route resolver not configured")
	}
	routeMetadata := buildRouteMetadata(msg, identity)
	p.enrichConversationAvatar(ctx, cfg, msg, routeMetadata)
	resolved, err := p.routeResolver.ResolveConversation(ctx, route.ResolveInput{
		BotID:             identity.BotID,
		Platform:          msg.Channel.String(),
		ConversationID:    msg.Conversation.ID,
		ThreadID:          threadID,
		ConversationType:  msg.Conversation.Type,
		ChannelIdentityID: identity.UserID,
		ChannelConfigID:   identity.ChannelConfigID,
		ReplyTarget:       strings.TrimSpace(msg.ReplyTarget),
		Metadata:          routeMetadata,
	})
	if err != nil {
		return fmt.Errorf("resolve route conversation: %w", err)
	}

	// Resolve or auto-create the active session for this route.
	sessionID := ""
	if p.sessionEnsurer != nil {
		sess, sessErr := p.sessionEnsurer.EnsureActiveSession(ctx, identity.BotID, resolved.RouteID, msg.Channel.String())
		if sessErr != nil {
			if p.logger != nil {
				p.logger.Warn("ensure active session failed", slog.Any("error", sessErr))
			}
		} else {
			sessionID = sess.ID
		}
	}

	// Bot-centric history container:
	// always persist channel traffic under bot_id so WebUI can view unified cross-platform history.
	activeChatID := strings.TrimSpace(identity.BotID)
	if activeChatID == "" {
		activeChatID = strings.TrimSpace(resolved.ChatID)
	}
	shouldTrigger := shouldTriggerAssistantResponse(msg) || identity.ForceReply
	if shouldTrigger && p.acl != nil {
		allowed, err := p.acl.CanPerformChatTrigger(ctx, acl.ChatTriggerRequest{
			BotID:             identity.BotID,
			UserID:            identity.UserID,
			ChannelIdentityID: identity.ChannelIdentityID,
			SourceScope: acl.SourceScope{
				Channel:          msg.Channel.String(),
				ConversationType: channel.NormalizeConversationType(msg.Conversation.Type),
				ConversationID:   strings.TrimSpace(msg.Conversation.ID),
				ThreadID:         threadID,
			},
		})
		if err != nil {
			return fmt.Errorf("authorize chat trigger: %w", err)
		}
		if !allowed {
			shouldTrigger = false
			if p.logger != nil {
				p.logger.Info(
					"inbound trigger denied by acl",
					slog.String("channel", msg.Channel.String()),
					slog.String("bot_id", strings.TrimSpace(identity.BotID)),
					slog.String("user_id", strings.TrimSpace(identity.UserID)),
					slog.String("channel_identity_id", strings.TrimSpace(identity.ChannelIdentityID)),
					slog.String("conversation_type", strings.TrimSpace(msg.Conversation.Type)),
				)
			}
		}
	}

	if !shouldTrigger {
		p.persistPassiveMessage(ctx, identity, msg, text, attachments, resolved.RouteID, sessionID)
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
		return nil
	}

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

	// Issue bot-owner JWT for downstream calls (MCP tools, schedule, etc.).
	// The agent uses this token to call back into the server's container/MCP
	// endpoints which require bot-owner or admin access. Using the chatting
	// user's identity would cause 403 for non-owner users.
	token := ""
	if p.jwtSecret != "" {
		tokenUserID := strings.TrimSpace(identity.UserID)
		if p.policy != nil {
			if ownerID, err := p.policy.BotOwnerUserID(ctx, identity.BotID); err == nil && ownerID != "" {
				tokenUserID = ownerID
			} else if p.logger != nil {
				p.logger.Warn("resolve bot owner for token failed, falling back to caller identity",
					slog.String("bot_id", identity.BotID), slog.Any("error", err))
			}
		}
		if tokenUserID != "" {
			signed, _, err := auth.GenerateToken(tokenUserID, p.jwtSecret, p.tokenTTL)
			if err != nil {
				if p.logger != nil {
					p.logger.Warn("issue channel token failed", slog.Any("error", err))
				}
			} else {
				token = "Bearer " + signed
			}
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
		err := errors.New("reply target missing")
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
			"route_id":          resolved.RouteID,
			"conversation_type": msg.Conversation.Type,
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
		SessionID:               sessionID,
		Token:                   token,
		UserID:                  identity.UserID,
		SourceChannelIdentityID: identity.ChannelIdentityID,
		DisplayName:             identity.DisplayName,
		RouteID:                 resolved.RouteID,
		ChatToken:               chatToken,
		ExternalMessageID:       sourceMessageID,
		ReplyTarget:             target,
		ConversationType:        msg.Conversation.Type,
		ConversationName:        msg.Conversation.Name,
		Query:                   text,
		CurrentChannel:          msg.Channel.String(),
		Channels:                []string{msg.Channel.String()},
		UserMessagePersisted:    false,
		Attachments:             attachments,
		OutboundAssetCollector:  assetCollector,
	})

	var (
		finalMessages []conversation.ModelMessage
		streamErr     error
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
					assetMu.Lock()
					outboundAssetRefs = append(outboundAssetRefs, buildAssetRefs(ingested, len(outboundAssetRefs))...)
					assetMu.Unlock()
				}
				if event.Type == channel.StreamEventReaction && len(event.Reactions) > 0 {
					p.dispatchReactions(ctx, identity.BotID, msg.Channel, target, sourceMessageID, event.Reactions)
					continue
				}
				if event.Type == channel.StreamEventSpeech && len(event.Speeches) > 0 {
					p.synthesizeAndPushVoice(ctx, strings.TrimSpace(identity.BotID), event.Speeches, stream, &outboundAssetRefs, &assetMu)
					continue
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
	for _, output := range outputs {
		outMessage := buildChannelMessage(output, desc.Capabilities)
		if outMessage.IsEmpty() {
			continue
		}
		plainText := strings.TrimSpace(outMessage.PlainText())
		if isSilentReplyText(plainText) {
			continue
		}
		if isMessagingToolDuplicate(plainText, sentTexts) {
			continue
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
	return false
}

// isDirectedAtBot reports whether the message is explicitly directed at this bot,
// either because it's a direct conversation, the bot is @mentioned, or it's a reply
// to this bot's message.
func isDirectedAtBot(msg channel.InboundMessage) bool {
	if isDirectConversationType(msg.Conversation.Type) {
		return true
	}
	return metadataBool(msg.Metadata, "is_mentioned") || metadataBool(msg.Metadata, "is_reply_to_bot")
}

// rawTextForCommand returns the original user text (without prepended
// quote/forward context) for slash-command detection. Adapters store the
// undecorated text as metadata["raw_text"]; this helper falls back to the
// full decorated text when the key is absent (e.g. direct messages or
// adapters that don't prepend context).
func rawTextForCommand(msg channel.InboundMessage, fallback string) string {
	if raw, ok := msg.Metadata["raw_text"].(string); ok && strings.TrimSpace(raw) != "" {
		return raw
	}
	return fallback
}

func isDirectConversationType(conversationType string) bool {
	return channel.IsPrivateConversationType(conversationType)
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

// persistPassiveMessage writes a user message directly into bot_history_messages
// for group conversations where the bot was not @mentioned. This replaces the
// old inbox system — the message is stored in the route's active session so it
// becomes part of the conversation history the next time the agent is triggered.
func (p *ChannelInboundProcessor) persistPassiveMessage(
	ctx context.Context,
	ident InboundIdentity,
	msg channel.InboundMessage,
	text string,
	attachments []conversation.ChatAttachment,
	routeID, sessionID string,
) {
	if p.message == nil {
		return
	}
	botID := strings.TrimSpace(ident.BotID)
	if botID == "" {
		return
	}
	trimmedText := strings.TrimSpace(text)
	if trimmedText == "" && len(attachments) == 0 {
		return
	}

	var attachmentPaths []string
	for _, att := range attachments {
		if ap := strings.TrimSpace(att.Path); ap != "" {
			attachmentPaths = append(attachmentPaths, ap)
		}
	}

	headerifiedText := flow.FormatUserHeader(
		strings.TrimSpace(msg.Message.ID),
		strings.TrimSpace(ident.ChannelIdentityID),
		strings.TrimSpace(ident.DisplayName),
		msg.Channel.String(),
		strings.TrimSpace(msg.Conversation.Type),
		strings.TrimSpace(msg.Conversation.Name),
		attachmentPaths,
		trimmedText,
	)

	modelMsg := conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent(headerifiedText)}
	serialized, err := json.Marshal(modelMsg)
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("marshal passive message failed", slog.Any("error", err))
		}
		return
	}

	meta := map[string]any{
		"route_id": strings.TrimSpace(routeID),
		"platform": msg.Channel.String(),
	}

	var assets []messagepkg.AssetRef
	for i, att := range attachments {
		ch := strings.TrimSpace(att.ContentHash)
		if ch == "" {
			continue
		}
		assets = append(assets, messagepkg.AssetRef{
			ContentHash: ch,
			Role:        "attachment",
			Ordinal:     i,
			Mime:        strings.TrimSpace(att.Mime),
			SizeBytes:   att.Size,
			Name:        strings.TrimSpace(att.Name),
			Metadata:    att.Metadata,
		})
	}

	if _, err := p.message.Persist(ctx, messagepkg.PersistInput{
		BotID:                   botID,
		SessionID:               sessionID,
		SenderChannelIdentityID: strings.TrimSpace(ident.ChannelIdentityID),
		SenderUserID:            strings.TrimSpace(ident.UserID),
		ExternalMessageID:       strings.TrimSpace(msg.Message.ID),
		Role:                    "user",
		Content:                 serialized,
		Metadata:                meta,
		Assets:                  assets,
	}); err != nil && p.logger != nil {
		p.logger.Warn("persist passive message failed", slog.Any("error", err), slog.String("bot_id", botID))
	}
}

func buildChannelMessage(output conversation.AssistantOutput, capabilities channel.ChannelCapabilities) channel.Message {
	msg := channel.Message{}
	if strings.TrimSpace(output.Content) != "" {
		msg.Text = strings.TrimSpace(output.Content)
		if channel.ContainsMarkdown(msg.Text) && (capabilities.Markdown || capabilities.RichText) {
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
		if msg.Format == "" && channel.ContainsMarkdown(msg.Text) && (capabilities.Markdown || capabilities.RichText) {
			msg.Format = channel.MessageFormatMarkdown
		}
	}
	return msg
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

// agentStreamEnvelope is the JSON shape produced by internal/agent.StreamEvent.
type agentStreamEnvelope struct {
	Type     string                      `json:"type"`
	Delta    string                      `json:"delta"`
	Error    string                      `json:"error"`
	Message  string                      `json:"message"`
	Data     json.RawMessage             `json:"data"`
	Messages []conversation.ModelMessage `json:"messages"`

	ToolName    string          `json:"toolName"`
	ToolCallID  string          `json:"toolCallId"`
	Input       json.RawMessage `json:"input"`
	Result      json.RawMessage `json:"result"`
	Attachments json.RawMessage `json:"attachments"`
	Reactions   json.RawMessage `json:"reactions"`
	Speeches    json.RawMessage `json:"speeches"`
}

func mapStreamChunkToChannelEvents(chunk conversation.StreamChunk) ([]channel.StreamEvent, []conversation.ModelMessage, error) {
	if len(chunk) == 0 {
		return nil, nil, nil
	}
	var envelope agentStreamEnvelope
	if err := json.Unmarshal(chunk, &envelope); err != nil {
		return nil, nil, err
	}
	finalMessages := make([]conversation.ModelMessage, 0, len(envelope.Messages))
	finalMessages = append(finalMessages, envelope.Messages...)
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
	case "reaction_delta":
		reactions := parseReactionDelta(envelope.Reactions)
		if len(reactions) == 0 {
			return nil, finalMessages, nil
		}
		return []channel.StreamEvent{
			{Type: channel.StreamEventReaction, Reactions: reactions},
		}, finalMessages, nil
	case "speech_delta":
		speeches := parseSpeechDelta(envelope.Speeches)
		if len(speeches) == 0 {
			return nil, finalMessages, nil
		}
		return []channel.StreamEvent{
			{Type: channel.StreamEventSpeech, Speeches: speeches},
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

func buildInboundQuery(message channel.Message, attachments []conversation.ChatAttachment) string {
	text := strings.TrimSpace(message.PlainText())
	if text != "" {
		return text
	}
	if len(message.Attachments) == 0 {
		return ""
	}
	count := len(message.Attachments)
	fallback := fmt.Sprintf("[User sent %d attachments]", count)
	if count == 1 {
		fallback = "[User sent 1 attachment]"
	}
	refs := collectContainerAttachmentRefs(attachments)
	if len(refs) == 0 {
		return fallback
	}
	var sb strings.Builder
	sb.WriteString(fallback)
	sb.WriteString("\n[Attachment refs: container paths]\n")
	for _, ref := range refs {
		sb.WriteString("- ")
		sb.WriteString(ref)
		sb.WriteByte('\n')
	}
	return strings.TrimSpace(sb.String())
}

func collectContainerAttachmentRefs(attachments []conversation.ChatAttachment) []string {
	if len(attachments) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(attachments))
	refs := make([]string, 0, len(attachments))
	for _, att := range attachments {
		ref := strings.TrimSpace(att.Path)
		if ref == "" {
			continue
		}
		if _, exists := seen[ref]; exists {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
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
		return IdentityState{}, errors.New("identity resolver not configured")
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

func (*ChannelInboundProcessor) notifyProcessingStarted(
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

func (*ChannelInboundProcessor) notifyProcessingCompleted(
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

func (*ChannelInboundProcessor) notifyProcessingFailed(
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
			BotID:       botID,
			Mime:        strings.TrimSpace(item.Mime),
			Reader:      preparedReader,
			MaxBytes:    maxBytes,
			OriginalExt: filepath.Ext(strings.TrimSpace(item.Name)),
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
		return inboundAttachmentPayload{}, errors.New("attachment has no ingestible payload")
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
		return inboundAttachmentPayload{}, errors.New("resolved attachment reader is nil")
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
	resp, err := client.Do(req) //nolint:gosec // G704: URL is an attachment URL provided by the inbound channel adapter
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
		sourceURL := item.URL
		item.ContentHash = asset.ContentHash
		item.URL = ""
		item.Base64 = ""
		if item.Metadata == nil {
			item.Metadata = make(map[string]any)
		}
		item.Metadata["bot_id"] = botID
		item.Metadata["storage_key"] = asset.StorageKey
		if n := strings.TrimSpace(item.Name); n != "" {
			item.Metadata["name"] = n
		}
		if su := strings.TrimSpace(sourceURL); su != "" && !isDataURL(su) {
			item.Metadata["source_url"] = su
		}
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
	sourcePath := accessPath

	// Try media marker lookup first.
	storageKey := extractStorageKey(accessPath, botID)
	if storageKey != "" {
		asset, err := p.mediaService.GetByStorageKey(ctx, botID, storageKey)
		if err == nil {
			applyAssetToAttachment(asset, botID, item, sourcePath)
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
		applyAssetToAttachment(asset, botID, item, sourcePath)
		return true
	}

	return false
}

func applyAssetToAttachment(asset media.Asset, botID string, item *channel.Attachment, sourcePath string) {
	sourceURL := item.URL
	item.ContentHash = asset.ContentHash
	item.URL = ""
	if item.Metadata == nil {
		item.Metadata = make(map[string]any)
	}
	item.Metadata["bot_id"] = botID
	item.Metadata["storage_key"] = asset.StorageKey
	if n := strings.TrimSpace(item.Name); n != "" {
		item.Metadata["name"] = n
	}
	if sp := strings.TrimSpace(sourcePath); sp != "" {
		item.Metadata["source_path"] = sp
	}
	if su := strings.TrimSpace(sourceURL); su != "" && !isDataURL(su) {
		item.Metadata["source_url"] = su
	}
	if strings.TrimSpace(item.Mime) == "" {
		item.Mime = attachment.NormalizeMime(asset.Mime)
	}
	if item.Size == 0 && asset.SizeBytes > 0 {
		item.Size = asset.SizeBytes
	}
	if item.Type == channel.AttachmentFile || item.Type == "" {
		item.Type = inferAttachmentTypeFromMime(strings.TrimSpace(item.Mime))
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

// extractStorageKey derives the media storage key from a container-internal
// access path. The expected path format is /data/media/<storage_key>.
func extractStorageKey(accessPath string, _ string) string {
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
		name := strings.TrimSpace(item.Name)
		if name == "" && url != "" && !isDataURL(url) {
			name = filepath.Base(url)
		}
		attachments = append(attachments, channel.Attachment{
			Type:        channel.AttachmentType(strings.TrimSpace(item.Type)),
			URL:         url,
			PlatformKey: strings.TrimSpace(item.PlatformKey),
			ContentHash: strings.TrimSpace(item.ContentHash),
			Name:        name,
			Mime:        strings.TrimSpace(item.Mime),
			Size:        item.Size,
		})
	}
	return attachments
}

// synthesizeAndPushVoice handles speech_delta events by synthesizing audio
// and pushing the resulting voice attachment into the outbound stream.
func (p *ChannelInboundProcessor) synthesizeAndPushVoice(
	ctx context.Context,
	botID string,
	speeches []channel.SpeechRequest,
	stream channel.OutboundStream,
	outboundAssetRefs *[]conversation.OutboundAssetRef,
	assetMu *sync.Mutex,
) {
	if p.ttsService == nil || p.ttsModelResolver == nil {
		if p.logger != nil {
			p.logger.Warn("speech_delta received but TTS service not configured")
		}
		return
	}
	modelID, err := p.ttsModelResolver.ResolveTtsModelID(ctx, botID)
	if err != nil || strings.TrimSpace(modelID) == "" {
		if p.logger != nil {
			p.logger.Warn("speech_delta: bot has no TTS model configured", slog.String("bot_id", botID))
		}
		return
	}
	for _, speech := range speeches {
		text := strings.TrimSpace(speech.Text)
		if text == "" {
			continue
		}
		audioData, contentType, synthErr := p.ttsService.Synthesize(ctx, modelID, text, nil)
		if synthErr != nil {
			if p.logger != nil {
				p.logger.Warn("speech synthesis failed", slog.String("bot_id", botID), slog.Any("error", synthErr))
			}
			continue
		}
		dataURL := encodeDataURL(contentType, audioData)
		voiceEvent := channel.StreamEvent{
			Type: channel.StreamEventAttachment,
			Attachments: []channel.Attachment{
				{
					Type: channel.AttachmentVoice,
					URL:  dataURL,
					Mime: contentType,
					Size: int64(len(audioData)),
				},
			},
		}
		ingested := p.ingestOutboundAttachments(ctx, botID, voiceEvent.Attachments)
		voiceEvent.Attachments = ingested
		assetMu.Lock()
		*outboundAssetRefs = append(*outboundAssetRefs, buildAssetRefs(ingested, len(*outboundAssetRefs))...)
		assetMu.Unlock()
		if pushErr := stream.Push(ctx, voiceEvent); pushErr != nil {
			if p.logger != nil {
				p.logger.Warn("push voice attachment failed", slog.String("bot_id", botID), slog.Any("error", pushErr))
			}
			return
		}
	}
}

// parseSpeechDelta converts raw JSON speech data to SpeechRequest values.
func parseSpeechDelta(raw json.RawMessage) []channel.SpeechRequest {
	if len(raw) == 0 {
		return nil
	}
	var items []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	speeches := make([]channel.SpeechRequest, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		speeches = append(speeches, channel.SpeechRequest{Text: text})
	}
	return speeches
}

func buildAssetRefs(attachments []channel.Attachment, startOrdinal int) []conversation.OutboundAssetRef {
	var refs []conversation.OutboundAssetRef
	for _, att := range attachments {
		contentHash := strings.TrimSpace(att.ContentHash)
		if contentHash == "" {
			continue
		}
		ref := conversation.OutboundAssetRef{
			ContentHash: contentHash,
			Role:        "attachment",
			Ordinal:     startOrdinal + len(refs),
			Mime:        strings.TrimSpace(att.Mime),
			SizeBytes:   att.Size,
			Name:        strings.TrimSpace(att.Name),
			Metadata:    att.Metadata,
		}
		if att.Metadata != nil {
			if sk, ok := att.Metadata["storage_key"].(string); ok {
				ref.StorageKey = sk
			}
		}
		refs = append(refs, ref)
	}
	return refs
}

func encodeDataURL(mime string, data []byte) string {
	encoded := base64Encode(data)
	return "data:" + mime + ";base64," + encoded
}

func base64Encode(data []byte) string {
	return base64Std.EncodeToString(data)
}

// parseReactionDelta converts raw JSON reaction data to channel ReactRequests.
func parseReactionDelta(raw json.RawMessage) []channel.ReactRequest {
	if len(raw) == 0 {
		return nil
	}
	var items []struct {
		Emoji string `json:"emoji"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	reactions := make([]channel.ReactRequest, 0, len(items))
	for _, item := range items {
		emoji := strings.TrimSpace(item.Emoji)
		if emoji == "" {
			continue
		}
		reactions = append(reactions, channel.ReactRequest{
			Emoji: emoji,
		})
	}
	return reactions
}

// dispatchReactions sends emoji reactions to the channel for the source message.
func (p *ChannelInboundProcessor) dispatchReactions(
	ctx context.Context,
	botID string,
	channelType channel.ChannelType,
	target string,
	sourceMessageID string,
	reactions []channel.ReactRequest,
) {
	if p.reactor == nil {
		return
	}
	target = strings.TrimSpace(target)
	sourceMessageID = strings.TrimSpace(sourceMessageID)
	if target == "" || sourceMessageID == "" {
		if p.logger != nil {
			p.logger.Warn("cannot dispatch reactions: missing target or source message ID",
				slog.String("bot_id", botID),
				slog.String("channel", channelType.String()),
			)
		}
		return
	}
	for _, reaction := range reactions {
		req := channel.ReactRequest{
			Target:    target,
			MessageID: sourceMessageID,
			Emoji:     reaction.Emoji,
		}
		if err := p.reactor.React(ctx, strings.TrimSpace(botID), channelType, req); err != nil {
			if p.logger != nil {
				p.logger.Warn("inline reaction failed",
					slog.String("bot_id", botID),
					slog.String("channel", channelType.String()),
					slog.String("emoji", reaction.Emoji),
					slog.String("message_id", sourceMessageID),
					slog.Any("error", err),
				)
			}
		}
	}
}

// buildRouteMetadata extracts user/conversation information for route metadata persistence.
func buildRouteMetadata(msg channel.InboundMessage, identity InboundIdentity) map[string]any {
	m := make(map[string]any)

	if v := strings.TrimSpace(identity.DisplayName); v != "" {
		m["sender_display_name"] = v
	}
	if v := strings.TrimSpace(identity.AvatarURL); v != "" {
		m["sender_avatar_url"] = v
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
		if k == "username" {
			m["sender_username"] = v
		}
	}

	return m
}

// enrichConversationAvatar resolves group-level metadata (avatar, handle) via
// the directory adapter and stores them in the route metadata map.
func (p *ChannelInboundProcessor) enrichConversationAvatar(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, meta map[string]any) {
	convType := strings.TrimSpace(msg.Conversation.Type)
	if convType != "group" && convType != "supergroup" && convType != "channel" {
		return
	}
	if p.registry == nil {
		return
	}
	directoryAdapter, ok := p.registry.DirectoryAdapter(msg.Channel)
	if !ok || directoryAdapter == nil {
		return
	}
	convID := strings.TrimSpace(msg.Conversation.ID)
	if convID == "" {
		return
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	entry, err := directoryAdapter.ResolveEntry(lookupCtx, cfg, convID, channel.DirectoryEntryGroup)
	if err != nil {
		if p.logger != nil {
			p.logger.Debug("resolve conversation directory entry failed",
				slog.String("channel", msg.Channel.String()),
				slog.String("conversation_id", convID),
				slog.Any("error", err),
			)
		}
		return
	}
	if v := strings.TrimSpace(entry.AvatarURL); v != "" {
		meta["conversation_avatar_url"] = v
	}
	if v := strings.TrimSpace(entry.Handle); v != "" {
		meta["conversation_handle"] = v
	}
}

// isNewSessionCommand returns true when the command text is "/new" (with
// optional Telegram-style @botname suffix and trailing whitespace).
func isNewSessionCommand(cmdText string) bool {
	extracted := command.ExtractCommandText(cmdText)
	if extracted == "" {
		return false
	}
	parsed, err := command.Parse(extracted)
	if err != nil {
		return false
	}
	return parsed.Resource == "new"
}

// handleNewSessionCommand resolves the route for the current message and
// creates a brand-new active session, effectively starting a fresh
// conversation in the same IM thread/chat.
func (p *ChannelInboundProcessor) handleNewSessionCommand(
	ctx context.Context,
	cfg channel.ChannelConfig,
	msg channel.InboundMessage,
	sender channel.StreamReplySender,
	identity InboundIdentity,
) error {
	target := strings.TrimSpace(msg.ReplyTarget)
	if target == "" {
		return errors.New("reply target missing for /new command")
	}

	if p.routeResolver == nil {
		return sender.Send(ctx, channel.OutboundMessage{
			Target:  target,
			Message: channel.Message{Text: "Error: route resolver not configured."},
		})
	}
	if p.sessionEnsurer == nil {
		return sender.Send(ctx, channel.OutboundMessage{
			Target:  target,
			Message: channel.Message{Text: "Error: session service not configured."},
		})
	}

	threadID := extractThreadID(msg)
	routeMetadata := buildRouteMetadata(msg, identity)
	p.enrichConversationAvatar(ctx, cfg, msg, routeMetadata)
	resolved, err := p.routeResolver.ResolveConversation(ctx, route.ResolveInput{
		BotID:             identity.BotID,
		Platform:          msg.Channel.String(),
		ConversationID:    msg.Conversation.ID,
		ThreadID:          threadID,
		ConversationType:  msg.Conversation.Type,
		ChannelIdentityID: identity.UserID,
		ChannelConfigID:   identity.ChannelConfigID,
		ReplyTarget:       target,
		Metadata:          routeMetadata,
	})
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("resolve route for /new command failed", slog.Any("error", err))
		}
		return sender.Send(ctx, channel.OutboundMessage{
			Target:  target,
			Message: channel.Message{Text: "Error: failed to resolve conversation route."},
		})
	}

	sess, err := p.sessionEnsurer.CreateNewSession(ctx, identity.BotID, resolved.RouteID, msg.Channel.String())
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("create new session via /new command failed", slog.Any("error", err))
		}
		return sender.Send(ctx, channel.OutboundMessage{
			Target:  target,
			Message: channel.Message{Text: "Error: failed to create new session."},
		})
	}

	if p.logger != nil {
		p.logger.Info("new session created via /new command",
			slog.String("bot_id", strings.TrimSpace(identity.BotID)),
			slog.String("route_id", strings.TrimSpace(resolved.RouteID)),
			slog.String("session_id", strings.TrimSpace(sess.ID)),
			slog.String("channel", msg.Channel.String()),
		)
	}
	return sender.Send(ctx, channel.OutboundMessage{
		Target:  target,
		Message: channel.Message{Text: "New conversation started."},
	})
}
