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
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	sessionpkg "github.com/memohai/memoh/internal/session"
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
	Evaluate(ctx context.Context, req acl.EvaluateRequest) (bool, error)
}

type mediaIngestor interface {
	channel.OutboundAttachmentStore
	channel.ContainerAttachmentIngester
}

// speechSynthesizer synthesizes text to speech audio.
type speechSynthesizer interface {
	Synthesize(ctx context.Context, modelID string, text string, overrideCfg map[string]any) ([]byte, string, error)
}

// speechModelResolver looks up the speech model ID configured for a bot.
type speechModelResolver interface {
	ResolveSpeechModelID(ctx context.Context, botID string) (string, error)
}

// TranscriptionResult is the minimal speech-to-text response shape needed by inbound routing.
type TranscriptionResult interface {
	GetText() string
}

// transcriptionRecognizer converts inbound audio to text using a configured model.
type transcriptionRecognizer interface {
	Transcribe(ctx context.Context, modelID string, audio []byte, filename string, contentType string, overrideCfg map[string]any) (TranscriptionResult, error)
}

// transcriptionModelResolver looks up the transcription model ID configured for a bot.
type transcriptionModelResolver interface {
	ResolveTranscriptionModelID(ctx context.Context, botID string) (string, error)
}

// SessionEnsurer resolves or creates an active session for a route.
type SessionEnsurer interface {
	EnsureActiveSession(ctx context.Context, botID, routeID, channelType string) (SessionResult, error)
	GetActiveSession(ctx context.Context, routeID string) (SessionResult, error)
	// CreateNewSession always creates a fresh session and sets it as the
	// active session for the given route, replacing any previous one.
	// sessionType defaults to "chat" if empty.
	CreateNewSession(ctx context.Context, botID, routeID, channelType, sessionType string) (SessionResult, error)
}

type ToolApprovalRunner interface {
	RespondToolApproval(ctx context.Context, input flow.ToolApprovalResponseInput, eventCh chan<- flow.WSStreamEvent) error
}

// IMDisplayOptionsReader exposes bot-level IM display preferences.
// Implementations typically adapt the settings service.
type IMDisplayOptionsReader interface {
	// ShowToolCallsInIM reports whether tool_call lifecycle events should
	// reach IM adapters for the given bot. Returns false by default when the
	// bot or its settings cannot be resolved.
	ShowToolCallsInIM(ctx context.Context, botID string) (bool, error)
}

// SessionResult carries the minimum fields needed from a session.
type SessionResult struct {
	ID   string
	Type string
}

// ChannelInboundProcessor routes channel inbound messages to the chat gateway.
type ChannelInboundProcessor struct {
	runner              flow.Runner
	routeResolver       RouteResolver
	message             messagepkg.Writer
	mediaService        mediaIngestor
	reactor             channelReactor
	commandHandler      *command.Handler
	registry            *channel.Registry
	logger              *slog.Logger
	jwtSecret           string
	tokenTTL            time.Duration
	identity            *IdentityResolver
	policy              PolicyService
	dispatcher          *RouteDispatcher
	acl                 chatACL
	observer            channel.StreamObserver
	speechService       speechSynthesizer
	speechModelResolver speechModelResolver
	transcriber         transcriptionRecognizer
	sttModelResolver    transcriptionModelResolver
	sessionEnsurer      SessionEnsurer
	pipeline            *pipelinepkg.Pipeline
	eventStore          *pipelinepkg.EventStore
	discussDriver       *pipelinepkg.DiscussDriver
	imDisplayOptions    IMDisplayOptionsReader

	// activeStreams maps "botID:routeID" to a context.CancelFunc for the
	// currently running agent stream. Used by /stop to abort generation
	// on external channels (Telegram, Discord, etc.).
	activeStreams sync.Map
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
	jwtSecret string,
	tokenTTL time.Duration,
) *ChannelInboundProcessor {
	if log == nil {
		log = slog.Default()
	}
	if tokenTTL <= 0 {
		tokenTTL = 5 * time.Minute
	}
	identityResolver := NewIdentityResolver(log, registry, channelIdentityService, policyService, "")
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

// SetSpeechService configures the speech synthesizer and settings reader for
// handling <speech> tag events (speech_delta) that require server-side audio synthesis.
func (p *ChannelInboundProcessor) SetSpeechService(synth speechSynthesizer, modelResolver speechModelResolver) {
	if p == nil {
		return
	}
	p.speechService = synth
	p.speechModelResolver = modelResolver
}

// SetTranscriptionService configures speech-to-text processing for inbound audio attachments.
func (p *ChannelInboundProcessor) SetTranscriptionService(recognizer transcriptionRecognizer, modelResolver transcriptionModelResolver) {
	if p == nil {
		return
	}
	p.transcriber = recognizer
	p.sttModelResolver = modelResolver
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

// SetPipeline configures the DCP pipeline, event store, and discuss driver.
func (p *ChannelInboundProcessor) SetPipeline(pipeline *pipelinepkg.Pipeline, store *pipelinepkg.EventStore, driver *pipelinepkg.DiscussDriver) {
	if p == nil {
		return
	}
	p.pipeline = pipeline
	p.eventStore = store
	p.discussDriver = driver
}

// SetDispatcher configures the per-route message dispatcher for inject/queue/parallel modes.
func (p *ChannelInboundProcessor) SetDispatcher(dispatcher *RouteDispatcher) {
	if p == nil {
		return
	}
	p.dispatcher = dispatcher
}

// SetIMDisplayOptions configures the reader used to gate IM-facing stream
// events (e.g. tool call lifecycle) on bot-level display preferences. When
// nil, tool call events are always dropped before reaching IM adapters.
func (p *ChannelInboundProcessor) SetIMDisplayOptions(reader IMDisplayOptionsReader) {
	if p == nil {
		return
	}
	p.imDisplayOptions = reader
}

// shouldShowToolCallsInIM reports whether tool_call_start / tool_call_end
// events should reach the IM adapter for the given bot. Failures and missing
// configuration default to false so tool calls remain hidden unless explicitly
// enabled.
func (p *ChannelInboundProcessor) shouldShowToolCallsInIM(ctx context.Context, botID string) bool {
	if p == nil || p.imDisplayOptions == nil {
		return false
	}
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return false
	}
	show, err := p.imDisplayOptions.ShowToolCallsInIM(ctx, botID)
	if err != nil {
		if p.logger != nil {
			p.logger.Debug(
				"show_tool_calls_in_im lookup failed, defaulting to hidden",
				slog.String("bot_id", botID),
				slog.Any("error", err),
			)
		}
		return false
	}
	return show
}

// HandleInbound processes an inbound channel message through identity resolution and chat gateway.
func (p *ChannelInboundProcessor) HandleInbound(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, sender channel.StreamReplySender) (retErr error) {
	if p.runner == nil {
		return errors.New("channel inbound processor not configured")
	}
	if sender == nil {
		return errors.New("reply sender not configured")
	}
	text := strings.TrimSpace(msg.Message.PlainText())
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

	// /new and /stop require route context, so they are handled separately
	// from the general command handler (which runs before route resolution).
	if isNewSessionCommand(cmdText) && isDirectedAtBot(msg) {
		return p.handleNewSessionCommand(ctx, cfg, msg, sender, identity)
	}
	if isStopCommand(cmdText) && isDirectedAtBot(msg) {
		return p.handleStopCommand(ctx, cfg, msg, sender, identity)
	}
	if isStatusCommand(cmdText) && isDirectedAtBot(msg) {
		return p.handleStatusCommand(ctx, cfg, msg, sender, identity)
	}

	// Skip generic command handler for mode-prefix commands (/btw, /now, /next)
	// so they pass through to mode detection below.
	if p.commandHandler != nil && p.commandHandler.IsCommand(cmdText) && !IsModeCommand(cmdText) && !isToolApprovalCommand(cmdText) && isDirectedAtBot(msg) {
		reply, err := p.commandHandler.ExecuteWithInput(ctx, command.ExecuteInput{
			BotID:             strings.TrimSpace(identity.BotID),
			ChannelIdentityID: strings.TrimSpace(identity.ChannelIdentityID),
			UserID:            strings.TrimSpace(identity.UserID),
			Text:              cmdText,
			ChannelType:       msg.Channel.String(),
			ConversationType:  strings.TrimSpace(msg.Conversation.Type),
			ConversationID:    strings.TrimSpace(msg.Conversation.ID),
			ThreadID:          extractThreadID(msg),
		})
		if err != nil {
			reply = "Error: " + err.Error()
		}
		return sender.Send(ctx, channel.OutboundMessage{
			Target:  strings.TrimSpace(msg.ReplyTarget),
			Message: channel.Message{Text: reply},
		})
	}

	resolvedAttachments := p.ingestInboundAttachments(ctx, cfg, msg, strings.TrimSpace(identity.BotID), msg.Message.Attachments)
	msg.Message.Attachments = resolvedAttachments
	if msg.Message.Reply != nil && len(msg.Message.Reply.Attachments) > 0 {
		msg.Message.Reply.Attachments = p.ingestInboundAttachments(ctx, cfg, msg, strings.TrimSpace(identity.BotID), msg.Message.Reply.Attachments)
	}
	hadVoiceAttachment := containsVoiceAttachment(resolvedAttachments)
	attachments := mapChannelToChatAttachments(resolvedAttachments)
	replyAttachments := mapChannelToChatAttachments(replyAttachmentsFromMessage(msg.Message.Reply))
	text = strings.TrimSpace(msg.Message.PlainText())

	// Detect inbound mode from message prefix (/btw, /now, /next).
	// Only applies to non-local channels; WebUI always uses the default flow.
	// Must run after buildInboundQuery so the prefix is stripped from the final text.
	inboundMode := ModeInject
	if !isLocalChannelType(msg.Channel) {
		inboundMode, text = DetectMode(text)
	}
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
		ChannelIdentityID: identity.ChannelIdentityID,
		ChannelConfigID:   identity.ChannelConfigID,
		ReplyTarget:       strings.TrimSpace(msg.ReplyTarget),
		Metadata:          routeMetadata,
	})
	if err != nil {
		return fmt.Errorf("resolve route conversation: %w", err)
	}

	// Resolve or auto-create the active session for this route.
	// Retry up to 3 times with short backoff to avoid persisting messages with NULL session_id.
	sessionID := ""
	sessionType := ""
	if p.sessionEnsurer != nil {
		for attempt := range 3 {
			sess, sessErr := p.sessionEnsurer.EnsureActiveSession(ctx, identity.BotID, resolved.RouteID, msg.Channel.String())
			if sessErr == nil {
				sessionID = sess.ID
				sessionType = sess.Type
				break
			}
			if p.logger != nil {
				p.logger.Warn("ensure active session failed",
					slog.Int("attempt", attempt+1),
					slog.Any("error", sessErr))
			}
			if attempt < 2 {
				time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			}
		}
	}

	// ACL gate: evaluate before events enter the pipeline. If denied, the
	// message is not persisted in the event store and not pushed into the
	// in-memory pipeline. This applies uniformly to chat and discuss modes.
	aclAllowed := true
	if p.acl != nil {
		allowed, aclErr := p.acl.Evaluate(ctx, acl.EvaluateRequest{
			BotID:             identity.BotID,
			ChannelIdentityID: identity.ChannelIdentityID,
			ChannelType:       msg.Channel.String(),
			SourceScope: acl.SourceScope{
				ConversationType: channel.NormalizeConversationType(msg.Conversation.Type),
				ConversationID:   strings.TrimSpace(msg.Conversation.ID),
				ThreadID:         threadID,
			},
		})
		if aclErr != nil {
			return fmt.Errorf("evaluate acl: %w", aclErr)
		}
		aclAllowed = allowed
	}

	if !aclAllowed {
		p.persistPassiveMessage(ctx, identity, msg, text, attachments, resolved.RouteID, sessionID, "")
		if p.logger != nil {
			p.logger.Info(
				"inbound denied by acl — event not ingested",
				slog.String("channel", msg.Channel.String()),
				slog.String("bot_id", strings.TrimSpace(identity.BotID)),
				slog.String("channel_identity_id", strings.TrimSpace(identity.ChannelIdentityID)),
				slog.String("conversation_type", strings.TrimSpace(msg.Conversation.Type)),
			)
		}
		return nil
	}

	if isToolApprovalCommand(cmdText) && isDirectedAtBot(msg) {
		return p.handleToolApprovalCommand(ctx, msg, sender, identity, resolved.RouteID, sessionID, cmdText)
	}

	// Push event into the DCP pipeline (persist + in-memory projection).
	// On first access for a session, replay persisted events to warm the pipeline.
	var latestRC pipelinepkg.RenderedContext
	var eventID string
	if p.pipeline != nil && sessionID != "" {
		if _, loaded := p.pipeline.GetIC(sessionID); !loaded {
			p.replayPipelineSession(ctx, sessionID)
		}
		pipelineMsg := msg
		pipelineMsg.Message = msg.Message
		pipelineMsg.Message.Attachments = resolvedAttachments
		event := pipelinepkg.AdaptInbound(pipelineMsg, sessionID, identity.ChannelIdentityID, identity.DisplayName)
		if p.eventStore != nil {
			eid, persistErr := p.eventStore.PersistEvent(ctx, identity.BotID, sessionID, event)
			if persistErr != nil {
				if p.logger != nil {
					p.logger.Warn("persist pipeline event failed", slog.Any("error", persistErr))
				}
			} else {
				eventID = eid
			}
		}
		latestRC = p.pipeline.PushEvent(sessionID, event)
	}

	// Discuss mode: dispatch to the discuss driver and return.
	// The discuss driver autonomously decides whether to call the LLM.
	if sessionType == sessionpkg.TypeDiscuss && p.discussDriver != nil && latestRC != nil {
		p.discussDriver.NotifyRC(ctx, sessionID, latestRC, pipelinepkg.DiscussSessionConfig{
			BotID:             identity.BotID,
			SessionID:         sessionID,
			ChannelIdentityID: identity.ChannelIdentityID,
			ReplyTarget:       strings.TrimSpace(msg.ReplyTarget),
			CurrentPlatform:   msg.Channel.String(),
			ConversationType:  strings.TrimSpace(msg.Conversation.Type),
			ConversationName:  strings.TrimSpace(msg.Conversation.Name),
		})
		p.persistPassiveMessage(ctx, identity, msg, text, attachments, resolved.RouteID, sessionID, eventID)
		return nil
	}

	// Bot-centric history container:
	// always persist channel traffic under bot_id so WebUI can view unified cross-platform history.
	activeChatID := strings.TrimSpace(identity.BotID)
	if activeChatID == "" {
		activeChatID = strings.TrimSpace(resolved.ChatID)
	}
	shouldTrigger := shouldTriggerAssistantResponse(msg) || identity.ForceReply

	if sessionType == sessionpkg.TypeDiscuss || shouldTrigger {
		if transcript := p.transcribeInboundAttachments(ctx, strings.TrimSpace(identity.BotID), resolvedAttachments); transcript != "" {
			labeledTranscript := formatInboundTranscript(transcript)
			if msg.Message.Metadata == nil {
				msg.Message.Metadata = make(map[string]any)
			}
			msg.Message.Metadata["transcript"] = transcript
			if plain := strings.TrimSpace(msg.Message.PlainText()); plain == "" {
				msg.Message.Text = labeledTranscript
			} else if !strings.Contains(plain, transcript) {
				msg.Message.Text = plain + "\n\n" + labeledTranscript
			}
		} else if hadVoiceAttachment && strings.TrimSpace(msg.Message.PlainText()) == "" {
			msg.Message.Text = formatVoiceTranscriptionUnavailableNotice(resolvedAttachments)
		}
		text = strings.TrimSpace(msg.Message.PlainText())
	}

	if !shouldTrigger {
		p.persistPassiveMessage(ctx, identity, msg, text, attachments, resolved.RouteID, sessionID, eventID)
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

	routeID := strings.TrimSpace(resolved.RouteID)

	// --- Dispatcher-based mode handling (inject / queue) ---
	// For non-parallel modes, when a route already has an active agent stream,
	// short-circuit here instead of starting a new stream.
	if p.dispatcher != nil && !isLocalChannelType(msg.Channel) && inboundMode != ModeParallel {
		if p.dispatcher.IsActive(routeID) {
			headerifiedText := flow.FormatUserHeader(flow.UserMessageHeaderInput{
				MessageID:         strings.TrimSpace(msg.Message.ID),
				ChannelIdentityID: strings.TrimSpace(identity.ChannelIdentityID),
				DisplayName:       strings.TrimSpace(identity.DisplayName),
				Channel:           msg.Channel.String(),
				ConversationType:  strings.TrimSpace(msg.Conversation.Type),
				ConversationName:  strings.TrimSpace(msg.Conversation.Name),
				Target:            strings.TrimSpace(msg.ReplyTarget),
				AttachmentPaths:   collectAttachmentPaths(attachments),
				Time:              time.Now().UTC(),
			}, text)

			switch inboundMode {
			case ModeInject:
				// Don't persist here — the injected message will be interleaved
				// at the correct position within the round by
				// interleaveInjectedMessages in storeRound.
				injected := p.dispatcher.Inject(routeID, InjectMessage{
					Text:            text,
					Attachments:     attachments,
					HeaderifiedText: headerifiedText,
				})
				if injected {
					p.sendModeConfirmation(ctx, sender, msg, identity, "inject")
				} else {
					if p.logger != nil {
						p.logger.Warn("inject failed (channel full), falling through to new stream",
							slog.String("route_id", routeID))
					}
					goto startStream
				}
				return nil

			case ModeQueue:
				p.persistPassiveMessage(ctx, identity, msg, text, attachments, routeID, sessionID, eventID)
				p.dispatcher.Enqueue(routeID, QueuedTask{
					Ctx:         ctx,
					Cfg:         cfg,
					Msg:         msg,
					Sender:      sender,
					Ident:       identity,
					Text:        text,
					Attachments: attachments,
				})
				p.sendModeConfirmation(ctx, sender, msg, identity, "queue")
				return nil
			}
		}
	}

startStream:

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
	streamClosed := false
	closeStream := func() error {
		if streamClosed {
			return nil
		}
		streamClosed = true
		return stream.Close(context.WithoutCancel(ctx))
	}
	defer func() {
		if streamClosed {
			return
		}
		if closeErr := closeStream(); closeErr != nil {
			if p.logger != nil {
				p.logger.Error(
					"reply stream close failed",
					slog.String("channel", msg.Channel.String()),
					slog.String("channel_identity_id", identity.ChannelIdentityID),
					slog.String("user_id", identity.UserID),
					slog.Any("error", closeErr),
				)
			}
			if retErr == nil {
				retErr = closeErr
			}
		}
	}()

	// For non-local channels (IM adapters), optionally drop tool_call events
	// before they reach the adapter when the bot's show_tool_calls_in_im
	// setting is off. The filter sits inside the TeeStream so WebUI
	// observers still receive the full event stream.
	if !isLocalChannelType(msg.Channel) && !p.shouldShowToolCallsInIM(ctx, identity.BotID) {
		stream = channel.NewToolCallDroppingStream(stream)
	}

	// For non-local channels, wrap the stream so events are mirrored to the
	// RouteHub (and thus to Web UI and other local subscribers).
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

	// Mark this route as active in the dispatcher so subsequent messages
	// can be injected or queued. Produces the inject channel for this stream.
	// Parallel mode (/now) skips the dispatcher entirely — it must not
	// interfere with the active flag or drain the queue of another stream.
	var injectCh <-chan conversation.InjectMessage
	if p.dispatcher != nil && !isLocalChannelType(msg.Channel) && inboundMode != ModeParallel {
		injectCh = p.dispatcher.MarkActive(routeID)
		defer func() {
			p.drainQueue(context.WithoutCancel(ctx), routeID)
		}()
	}

	chatReq := conversation.ChatRequest{
		BotID:                     identity.BotID,
		ChatID:                    activeChatID,
		SessionID:                 sessionID,
		Token:                     token,
		UserID:                    identity.UserID,
		SourceChannelIdentityID:   identity.ChannelIdentityID,
		DisplayName:               identity.DisplayName,
		RouteID:                   resolved.RouteID,
		ChatToken:                 chatToken,
		ExternalMessageID:         sourceMessageID,
		ReplyTarget:               target,
		ConversationType:          msg.Conversation.Type,
		ConversationName:          msg.Conversation.Name,
		SourceReplyToMessageID:    inboundReplyMessageID(msg.Message.Reply),
		ReplySender:               inboundReplySender(msg.Message.Reply),
		ReplyPreview:              inboundReplyPreview(msg.Message.Reply),
		ReplyAttachments:          replyAttachments,
		ForwardMessageID:          inboundForwardMessageID(msg.Message.Forward),
		ForwardFromUserID:         inboundForwardFromUserID(msg.Message.Forward),
		ForwardFromConversationID: inboundForwardFromConversationID(msg.Message.Forward),
		ForwardSender:             inboundForwardSender(msg.Message.Forward),
		ForwardDate:               inboundForwardDate(msg.Message.Forward),
		Query:                     text,
		CurrentChannel:            msg.Channel.String(),
		Channels:                  []string{msg.Channel.String()},
		UserMessagePersisted:      false,
		Attachments:               attachments,
		OutboundAssetCollector:    assetCollector,
		EventID:                   eventID,
	}
	if injectCh != nil {
		chatReq.InjectCh = injectCh
	}
	if mid, _ := msg.Metadata["model_id"].(string); strings.TrimSpace(mid) != "" {
		chatReq.Model = strings.TrimSpace(mid)
	}
	if re, _ := msg.Metadata["reasoning_effort"].(string); strings.TrimSpace(re) != "" {
		chatReq.ReasoningEffort = strings.TrimSpace(re)
	}
	// Create a cancellable context so /stop can abort the stream.
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	streamKey := strings.TrimSpace(identity.BotID) + ":" + strings.TrimSpace(resolved.RouteID)
	p.activeStreams.Store(streamKey, streamCancel)
	defer p.activeStreams.Delete(streamKey)

	chunkCh, streamErrCh := p.runner.StreamChat(streamCtx, chatReq)

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
					ingested := p.ingestOutboundAttachments(ctx, strings.TrimSpace(identity.BotID), msg.Channel, event.Attachments)
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
					p.synthesizeAndPushVoice(ctx, strings.TrimSpace(identity.BotID), msg.Channel, event.Speeches, stream, &outboundAssetRefs, &assetMu)
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
		if err := closeStream(); err != nil {
			if statusNotifier != nil {
				if notifyErr := p.notifyProcessingFailed(ctx, statusNotifier, cfg, msg, statusInfo, statusHandle, err); notifyErr != nil {
					p.logProcessingStatusError("processing_failed", msg, identity, notifyErr)
				}
			}
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
	if err := closeStream(); err != nil {
		if statusNotifier != nil {
			if notifyErr := p.notifyProcessingFailed(ctx, statusNotifier, cfg, msg, statusInfo, statusHandle, err); notifyErr != nil {
				p.logProcessingStatusError("processing_failed", msg, identity, notifyErr)
			}
		}
		return err
	}
	if statusNotifier != nil {
		if notifyErr := p.notifyProcessingCompleted(ctx, statusNotifier, cfg, msg, statusInfo, statusHandle); notifyErr != nil {
			p.logProcessingStatusError("processing_completed", msg, identity, notifyErr)
		}
	}
	return nil
}

// sendModeConfirmation sends a lightweight acknowledgement to the user when
// their message is injected or queued rather than triggering a new stream.
func (p *ChannelInboundProcessor) sendModeConfirmation(
	ctx context.Context,
	_ channel.StreamReplySender,
	msg channel.InboundMessage,
	identity InboundIdentity,
	mode string,
) {
	target := strings.TrimSpace(msg.ReplyTarget)
	sourceMessageID := strings.TrimSpace(msg.Message.ID)
	if target == "" || sourceMessageID == "" {
		return
	}
	if p.reactor != nil {
		emoji := "👀"
		if mode == "queue" {
			emoji = "📋"
		}
		_ = p.reactor.React(ctx, strings.TrimSpace(identity.BotID), msg.Channel, channel.ReactRequest{
			Target:    target,
			MessageID: sourceMessageID,
			Emoji:     emoji,
		})
	}
}

// drainQueue marks the route as done and processes any queued tasks.
func (p *ChannelInboundProcessor) drainQueue(ctx context.Context, routeID string) {
	if p.dispatcher == nil {
		return
	}
	result := p.dispatcher.MarkDone(routeID)

	for _, fn := range result.PendingPersists {
		fn(ctx)
	}

	for _, task := range result.QueuedTasks {
		if p.logger != nil {
			p.logger.Info("processing queued task",
				slog.String("route_id", routeID),
				slog.String("query", strings.TrimSpace(task.Text)),
			)
		}
		if err := p.HandleInbound(ctx, task.Cfg, task.Msg, task.Sender); err != nil { //nolint:contextcheck // ctx is already WithoutCancel from the defer caller
			if p.logger != nil {
				p.logger.Error("queued task processing failed",
					slog.String("route_id", routeID),
					slog.Any("error", err),
				)
			}
		}
	}
}

func collectAttachmentPaths(attachments []conversation.ChatAttachment) []string {
	if len(attachments) == 0 {
		return nil
	}
	paths := make([]string, 0, len(attachments))
	for _, att := range attachments {
		if p := strings.TrimSpace(att.Path); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
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

func inboundReplyMessageID(reply *channel.ReplyRef) string {
	if reply == nil {
		return ""
	}
	return strings.TrimSpace(reply.MessageID)
}

func inboundReplySender(reply *channel.ReplyRef) string {
	if reply == nil {
		return ""
	}
	return strings.TrimSpace(reply.Sender)
}

func inboundReplyPreview(reply *channel.ReplyRef) string {
	if reply == nil {
		return ""
	}
	return strings.TrimSpace(reply.Preview)
}

func replyAttachmentsFromMessage(reply *channel.ReplyRef) []channel.Attachment {
	if reply == nil {
		return nil
	}
	return reply.Attachments
}

func inboundForwardMessageID(forward *channel.ForwardRef) string {
	if forward == nil {
		return ""
	}
	return strings.TrimSpace(forward.MessageID)
}

func inboundForwardFromUserID(forward *channel.ForwardRef) string {
	if forward == nil {
		return ""
	}
	return strings.TrimSpace(forward.FromUserID)
}

func inboundForwardFromConversationID(forward *channel.ForwardRef) string {
	if forward == nil {
		return ""
	}
	return strings.TrimSpace(forward.FromConversationID)
}

func inboundForwardSender(forward *channel.ForwardRef) string {
	if forward == nil {
		return ""
	}
	return strings.TrimSpace(forward.Sender)
}

func inboundForwardDate(forward *channel.ForwardRef) int64 {
	if forward == nil {
		return 0
	}
	return forward.Date
}

func messageReplyMetadata(reply *channel.ReplyRef) map[string]any {
	if reply == nil {
		return nil
	}
	result := map[string]any{}
	if v := strings.TrimSpace(reply.MessageID); v != "" {
		result["message_id"] = v
	}
	if v := strings.TrimSpace(reply.Sender); v != "" {
		result["sender"] = v
	}
	if v := strings.TrimSpace(reply.Preview); v != "" {
		result["preview"] = v
	}
	if attachments := channelAttachmentMetadata(reply.Attachments); len(attachments) > 0 {
		result["attachments"] = attachments
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func channelAttachmentMetadata(attachments []channel.Attachment) []map[string]any {
	if len(attachments) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(attachments))
	for _, att := range attachments {
		item := channel.BundleFromAttachment(att).ToMap()
		if len(item) > 0 {
			result = append(result, item)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func messageForwardMetadata(forward *channel.ForwardRef) map[string]any {
	if forward == nil {
		return nil
	}
	result := map[string]any{}
	if v := strings.TrimSpace(forward.MessageID); v != "" {
		result["message_id"] = v
	}
	if v := strings.TrimSpace(forward.FromUserID); v != "" {
		result["from_user_id"] = v
	}
	if v := strings.TrimSpace(forward.FromConversationID); v != "" {
		result["from_conversation_id"] = v
	}
	if v := strings.TrimSpace(forward.Sender); v != "" {
		result["sender"] = v
	}
	if forward.Date > 0 {
		result["date"] = forward.Date
	}
	if len(result) == 0 {
		return nil
	}
	return result
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
	routeID, sessionID, eventID string,
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

	headerifiedText := flow.FormatUserHeader(flow.UserMessageHeaderInput{
		MessageID:         strings.TrimSpace(msg.Message.ID),
		ChannelIdentityID: strings.TrimSpace(ident.ChannelIdentityID),
		DisplayName:       strings.TrimSpace(ident.DisplayName),
		Channel:           msg.Channel.String(),
		ConversationType:  strings.TrimSpace(msg.Conversation.Type),
		ConversationName:  strings.TrimSpace(msg.Conversation.Name),
		Target:            strings.TrimSpace(msg.ReplyTarget),
		AttachmentPaths:   attachmentPaths,
		Time:              time.Now().UTC(),
	}, trimmedText)

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
	if reply := messageReplyMetadata(msg.Message.Reply); reply != nil {
		meta["reply"] = reply
	}
	if forward := messageForwardMetadata(msg.Message.Forward); forward != nil {
		meta["forward"] = forward
	}

	var assets []messagepkg.AssetRef
	for i, att := range attachments {
		ch := strings.TrimSpace(att.ContentHash)
		if ch == "" {
			continue
		}
		ref := messagepkg.AssetRef{
			ContentHash: ch,
			Role:        "attachment",
			Ordinal:     i,
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
		assets = append(assets, ref)
	}

	if _, err := p.message.Persist(ctx, messagepkg.PersistInput{
		BotID:                   botID,
		SessionID:               sessionID,
		SenderChannelIdentityID: strings.TrimSpace(ident.ChannelIdentityID),
		SenderUserID:            strings.TrimSpace(ident.UserID),
		ExternalMessageID:       strings.TrimSpace(msg.Message.ID),
		SourceReplyToMessageID:  inboundReplyMessageID(msg.Message.Reply),
		Role:                    "user",
		Content:                 serialized,
		Metadata:                meta,
		Assets:                  assets,
		EventID:                 eventID,
		DisplayText:             trimmedText,
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
	ApprovalID  string          `json:"approvalId"`
	ShortID     int             `json:"shortId"`
	Status      string          `json:"status"`
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
	case "tool_approval_request":
		return []channel.StreamEvent{
			{
				Type: channel.StreamEventToolCallStart,
				ToolCall: &channel.StreamToolCall{
					Name:       strings.TrimSpace(envelope.ToolName),
					CallID:     strings.TrimSpace(envelope.ToolCallID),
					Input:      parseRawJSON(envelope.Input),
					ApprovalID: strings.TrimSpace(envelope.ApprovalID),
					ShortID:    envelope.ShortID,
					Actions: []channel.Action{
						{Type: "tool_approval", Label: "Approve", Value: "approve:" + strings.TrimSpace(envelope.ApprovalID)},
						{Type: "tool_approval", Label: "Reject", Value: "reject:" + strings.TrimSpace(envelope.ApprovalID)},
					},
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

// requireIdentity resolves identity for the current message.
// It first checks whether the middleware chain already resolved and stored an
// IdentityState in the context (via IdentityResolver.Middleware), and reuses
// that result to avoid a redundant round-trip to the identity store. If no
// cached state is found it falls back to a fresh Resolve call.
func (p *ChannelInboundProcessor) requireIdentity(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage) (IdentityState, error) {
	if p.identity == nil {
		return IdentityState{}, errors.New("identity resolver not configured")
	}
	if state, ok := IdentityStateFromContext(ctx); ok {
		return state, nil
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
		item = channel.AttachmentFromBundle(channel.BundleFromAttachment(item).WithAssetAccess(
			botID,
			asset,
			p.mediaService.AccessPath(asset),
		))
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

func (p *ChannelInboundProcessor) transcribeInboundAttachments(ctx context.Context, botID string, attachments []channel.Attachment) string {
	if p == nil || p.transcriber == nil || p.sttModelResolver == nil || p.mediaService == nil || strings.TrimSpace(botID) == "" {
		return ""
	}
	modelID, err := p.sttModelResolver.ResolveTranscriptionModelID(ctx, botID)
	if err != nil || strings.TrimSpace(modelID) == "" {
		return ""
	}
	transcripts := make([]string, 0, len(attachments))
	for _, att := range attachments {
		if att.Type != channel.AttachmentAudio && att.Type != channel.AttachmentVoice {
			continue
		}
		if strings.TrimSpace(att.ContentHash) == "" {
			continue
		}
		reader, asset, err := p.mediaService.Open(ctx, botID, strings.TrimSpace(att.ContentHash))
		if err != nil {
			if p.logger != nil {
				p.logger.Warn("open inbound audio for transcription failed", slog.Any("error", err), slog.String("bot_id", botID), slog.String("content_hash", att.ContentHash))
			}
			continue
		}
		audio, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil || len(audio) == 0 {
			if p.logger != nil {
				p.logger.Warn("read inbound audio for transcription failed", slog.Any("error", readErr), slog.String("bot_id", botID), slog.String("content_hash", att.ContentHash))
			}
			continue
		}
		filename := strings.TrimSpace(att.Name)
		if filename == "" {
			filename = "audio" + filepath.Ext(asset.StorageKey)
		}
		contentType := strings.TrimSpace(att.Mime)
		if contentType == "" {
			contentType = strings.TrimSpace(asset.Mime)
		}
		result, txErr := p.transcriber.Transcribe(ctx, modelID, audio, filename, contentType, nil)
		if txErr != nil {
			if p.logger != nil {
				p.logger.Warn("inbound attachment transcription failed", slog.Any("error", txErr), slog.String("bot_id", botID), slog.String("content_hash", att.ContentHash))
			}
			continue
		}
		text := strings.TrimSpace(result.GetText())
		if text == "" {
			continue
		}
		transcripts = append(transcripts, text)
	}
	if len(transcripts) == 0 {
		return ""
	}
	return strings.Join(transcripts, "\n\n")
}

func formatInboundTranscript(transcript string) string {
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return ""
	}
	return "[Voice message transcription]\n" + transcript
}

func containsVoiceAttachment(attachments []channel.Attachment) bool {
	for _, att := range attachments {
		if att.Type == channel.AttachmentAudio || att.Type == channel.AttachmentVoice {
			return true
		}
	}
	return false
}

func formatVoiceTranscriptionUnavailableNotice(attachments []channel.Attachment) string {
	paths := make([]string, 0, len(attachments))
	for _, att := range attachments {
		if att.Type != channel.AttachmentAudio && att.Type != channel.AttachmentVoice {
			continue
		}
		if ref := strings.TrimSpace(att.URL); ref != "" {
			paths = append(paths, ref)
		}
	}
	if len(paths) == 0 {
		return "[User sent a voice message, but transcription is unavailable.]"
	}
	return "[User sent a voice message, but transcription is unavailable. Use transcribe_audio with one of these paths if needed: " + strings.Join(paths, ", ") + "]"
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
func (p *ChannelInboundProcessor) ingestOutboundAttachments(ctx context.Context, botID string, channelType channel.ChannelType, attachments []channel.Attachment) []channel.Attachment {
	if len(attachments) == 0 || p.mediaService == nil || strings.TrimSpace(botID) == "" {
		return attachments
	}
	prepared, err := channel.PrepareStreamEvent(ctx, p.mediaService, channel.ChannelConfig{
		BotID:       botID,
		ChannelType: channelType,
	}, channel.StreamEvent{
		Type:        channel.StreamEventAttachment,
		Attachments: attachments,
	})
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("prepare outbound attachments failed", slog.Any("error", err))
		}
		return attachments
	}
	return prepared.LogicalEvent().Attachments
}

func isDataURL(raw string) bool {
	return channel.IsDataURL(raw)
}

func isHTTPURL(raw string) bool {
	return channel.IsHTTPURL(raw)
}

// extractStorageKey derives the media storage key from a container-internal
// access path. The expected path format is /data/media/<storage_key>.
func extractStorageKey(accessPath string, _ string) string {
	return attachment.ExtractStorageKey(accessPath)
}

// isLocalChannelType returns true for channels that already publish to RouteHub
// natively (e.g. web). Wrapping these with a tee would cause duplicate events.
func isLocalChannelType(ct channel.ChannelType) bool {
	s := strings.ToLower(strings.TrimSpace(string(ct)))
	return s == "web" || s == "cli"
}

// replayPipelineSession loads persisted events from the DB and replays them
// into the pipeline. Called lazily on first access per session after cold start.
func (p *ChannelInboundProcessor) replayPipelineSession(ctx context.Context, sessionID string) {
	if p.eventStore == nil || p.pipeline == nil {
		return
	}
	events, err := p.eventStore.LoadEvents(ctx, sessionID)
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("pipeline replay failed", slog.String("session_id", sessionID), slog.Any("error", err))
		}
		return
	}
	if len(events) > 0 {
		p.pipeline.ReplaySession(sessionID, events)
		if p.logger != nil {
			p.logger.Info("pipeline session replayed", slog.String("session_id", sessionID), slog.Int("events", len(events)))
		}
	}
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
		Reply:       msg.Message.Reply,
		Forward:     msg.Message.Forward,
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
// for denormalized persistence. Only attachments with a non-empty ContentHash are
// included. StorageKey is extracted from Metadata when present.
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
			Name:        strings.TrimSpace(att.Name),
			Metadata:    att.Metadata,
		}
		ref.StorageKey = attachment.MetadataString(att.Metadata, attachment.MetadataKeyStorageKey)
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
		if att.Type == channel.AttachmentAudio || att.Type == channel.AttachmentVoice {
			continue
		}
		bundle := channel.BundleFromAttachment(att)
		ca := conversation.ChatAttachmentFromBundle(bundle)
		switch {
		case strings.TrimSpace(bundle.ContentHash) != "" && bundle.Path != "":
			ca.Path = bundle.Path
			ca.URL = ""
		case bundle.URL != "":
			ca.URL = bundle.URL
		case bundle.Path != "":
			ca.URL = bundle.Path
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
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	attachments := make([]channel.Attachment, 0, len(items))
	for _, item := range items {
		bundle := attachment.BundleFromMap(item)
		attachments = append(attachments, channel.AttachmentFromBundle(bundle))
	}
	return attachments
}

// synthesizeAndPushVoice handles speech_delta events by synthesizing audio
// and pushing the resulting voice attachment into the outbound stream.
func (p *ChannelInboundProcessor) synthesizeAndPushVoice(
	ctx context.Context,
	botID string,
	channelType channel.ChannelType,
	speeches []channel.SpeechRequest,
	stream channel.OutboundStream,
	outboundAssetRefs *[]conversation.OutboundAssetRef,
	assetMu *sync.Mutex,
) {
	if p.speechService == nil || p.speechModelResolver == nil {
		if p.logger != nil {
			p.logger.Warn("speech_delta received but TTS service not configured")
		}
		return
	}
	modelID, err := p.speechModelResolver.ResolveSpeechModelID(ctx, botID)
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
		audioData, contentType, synthErr := p.speechService.Synthesize(ctx, modelID, text, nil)
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
		ingested := p.ingestOutboundAttachments(ctx, botID, channelType, voiceEvent.Attachments)
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
		ref.StorageKey = attachment.MetadataString(att.Metadata, attachment.MetadataKeyStorageKey)
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
	if mentions, ok := msg.Metadata["mentions"]; ok && mentions != nil {
		m["mentions"] = mentions
	}
	if targets, ok := msg.Metadata["mentioned_targets"]; ok && targets != nil {
		m["mentioned_targets"] = targets
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
	if v := strings.TrimSpace(entry.Name); v != "" {
		meta["conversation_name"] = v
	}
	if v := strings.TrimSpace(entry.AvatarURL); v != "" {
		meta["conversation_avatar_url"] = v
	}
	if v := strings.TrimSpace(entry.Handle); v != "" {
		meta["conversation_handle"] = v
	}
}

// isStopCommand returns true when the command text is "/stop" (with
// optional Telegram-style @botname suffix and trailing whitespace).
func isStopCommand(cmdText string) bool {
	extracted := command.ExtractCommandText(cmdText)
	if extracted == "" {
		return false
	}
	parsed, err := command.Parse(extracted)
	if err != nil {
		return false
	}
	return parsed.Resource == "stop"
}

// handleStopCommand resolves the route for the current conversation and
// cancels any active agent stream, effectively aborting the generation.
func (p *ChannelInboundProcessor) handleStopCommand(
	ctx context.Context,
	cfg channel.ChannelConfig,
	msg channel.InboundMessage,
	sender channel.StreamReplySender,
	identity InboundIdentity,
) error {
	target := strings.TrimSpace(msg.ReplyTarget)
	if target == "" {
		return errors.New("reply target missing for /stop command")
	}

	if p.routeResolver == nil {
		return sender.Send(ctx, channel.OutboundMessage{
			Target:  target,
			Message: channel.Message{Text: "Error: route resolver not configured."},
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
		ChannelIdentityID: identity.ChannelIdentityID,
		ChannelConfigID:   identity.ChannelConfigID,
		ReplyTarget:       target,
		Metadata:          routeMetadata,
	})
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("resolve route for /stop command failed", slog.Any("error", err))
		}
		return sender.Send(ctx, channel.OutboundMessage{
			Target:  target,
			Message: channel.Message{Text: "Error: failed to resolve conversation route."},
		})
	}

	streamKey := strings.TrimSpace(identity.BotID) + ":" + strings.TrimSpace(resolved.RouteID)
	cancelVal, loaded := p.activeStreams.LoadAndDelete(streamKey)
	if !loaded {
		// No active stream — silently ignore.
		return nil
	}

	cancelFn, ok := cancelVal.(context.CancelFunc)
	if !ok {
		return nil
	}

	cancelFn()
	if p.logger != nil {
		p.logger.Info("agent stream aborted via /stop command",
			slog.String("bot_id", strings.TrimSpace(identity.BotID)),
			slog.String("route_id", strings.TrimSpace(resolved.RouteID)),
			slog.String("channel", msg.Channel.String()),
		)
	}
	return nil
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

func isStatusCommand(cmdText string) bool {
	extracted := command.ExtractCommandText(cmdText)
	if extracted == "" {
		return false
	}
	parsed, err := command.Parse(extracted)
	if err != nil {
		return false
	}
	return parsed.Resource == "status"
}

func isToolApprovalCommand(cmdText string) bool {
	extracted := command.ExtractCommandText(cmdText)
	if extracted == "" {
		return false
	}
	parsed, err := command.Parse(extracted)
	if err != nil {
		return false
	}
	return parsed.Resource == "approve" || parsed.Resource == "reject"
}

func (p *ChannelInboundProcessor) handleToolApprovalCommand(ctx context.Context, msg channel.InboundMessage, sender channel.StreamReplySender, identity InboundIdentity, routeID, sessionID, cmdText string) error {
	approvalRunner, ok := p.runner.(ToolApprovalRunner)
	if !ok {
		return sender.Send(ctx, channel.OutboundMessage{
			Target:  strings.TrimSpace(msg.ReplyTarget),
			Message: channel.Message{Text: "Tool approval is not configured."},
		})
	}
	extracted := command.ExtractCommandText(cmdText)
	parsed, err := command.Parse(extracted)
	if err != nil {
		return sender.Send(ctx, channel.OutboundMessage{
			Target:  strings.TrimSpace(msg.ReplyTarget),
			Message: channel.Message{Text: "Invalid approval command."},
		})
	}
	explicitID := ""
	reason := ""
	replyExternalID := ""
	if msg.Message.Reply != nil {
		replyExternalID = strings.TrimSpace(msg.Message.Reply.MessageID)
	}
	actionText := strings.TrimSpace(parsed.Action)
	if parsed.Resource == "reject" && replyExternalID != "" && actionText != "" && !looksLikeApprovalID(actionText) {
		reason = strings.TrimSpace(strings.Join(append([]string{actionText}, parsed.Args...), " "))
	} else {
		explicitID = actionText
		reason = strings.TrimSpace(strings.Join(parsed.Args, " "))
	}
	return p.streamToolApprovalCommand(ctx, msg, sender, identity, approvalRunner, flow.ToolApprovalResponseInput{
		BotID:                  strings.TrimSpace(identity.BotID),
		SessionID:              strings.TrimSpace(sessionID),
		ActorChannelIdentityID: strings.TrimSpace(identity.ChannelIdentityID),
		ExplicitID:             explicitID,
		ReplyExternalMessageID: replyExternalID,
		Decision:               parsed.Resource,
		Reason:                 reason,
		ChatToken:              p.issueChatToken(identity, routeID, msg),
	})
}

func (p *ChannelInboundProcessor) streamToolApprovalCommand(ctx context.Context, msg channel.InboundMessage, sender channel.StreamReplySender, identity InboundIdentity, approvalRunner ToolApprovalRunner, input flow.ToolApprovalResponseInput) error {
	target := strings.TrimSpace(msg.ReplyTarget)
	if target == "" {
		return errors.New("reply target missing")
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
			"conversation_type": msg.Conversation.Type,
		},
	})
	if err != nil {
		return err
	}
	streamClosed := false
	closeStream := func() error {
		if streamClosed {
			return nil
		}
		streamClosed = true
		return stream.Close(context.WithoutCancel(ctx))
	}
	defer func() { _ = closeStream() }()

	if !isLocalChannelType(msg.Channel) && !p.shouldShowToolCallsInIM(ctx, identity.BotID) {
		stream = channel.NewToolCallDroppingStream(stream)
	}
	if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventStatus, Status: channel.StreamStatusStarted}); err != nil {
		return err
	}

	eventCh := make(chan flow.WSStreamEvent, 64)
	errCh := make(chan error, 1)
	go func() {
		defer close(eventCh)
		errCh <- approvalRunner.RespondToolApproval(ctx, input, eventCh)
		close(errCh)
	}()

	var finalMessages []conversation.ModelMessage
	for eventCh != nil || errCh != nil {
		select {
		case chunk, ok := <-eventCh:
			if !ok {
				eventCh = nil
				continue
			}
			events, messages, parseErr := mapStreamChunkToChannelEvents(chunk)
			if parseErr != nil {
				if p.logger != nil {
					p.logger.Warn("approval stream chunk parse failed", slog.Any("error", parseErr))
				}
				continue
			}
			if len(messages) > 0 {
				finalMessages = messages
			}
			for _, event := range events {
				// Approval continuations should not flash transient "running"
				// tool messages in IM. If tool visibility is enabled, the
				// completed tool state may still be shown on tool_call_end.
				if event.Type == channel.StreamEventToolCallStart &&
					(event.ToolCall == nil || strings.TrimSpace(event.ToolCall.ApprovalID) == "") {
					continue
				}
				if event.Type == channel.StreamEventReaction && len(event.Reactions) > 0 {
					p.dispatchReactions(ctx, identity.BotID, msg.Channel, target, sourceMessageID, event.Reactions)
					continue
				}
				if err := stream.Push(ctx, event); err != nil {
					return err
				}
			}
		case runErr, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if runErr != nil {
				_ = stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventError, Error: runErr.Error()})
				return runErr
			}
		}
	}

	sentTexts, suppressReplies := collectMessageToolContext(p.registry, finalMessages, msg.Channel, target)
	if !suppressReplies {
		outputs := flow.ExtractAssistantOutputs(finalMessages)
		for _, output := range outputs {
			outMessage := buildChannelMessage(output, channel.ChannelCapabilities{Text: true, Markdown: true, Reply: true})
			if outMessage.IsEmpty() {
				continue
			}
			plainText := strings.TrimSpace(outMessage.PlainText())
			if isSilentReplyText(plainText) || isMessagingToolDuplicate(plainText, sentTexts) {
				continue
			}
			if outMessage.Reply == nil && sourceMessageID != "" {
				outMessage.Reply = &channel.ReplyRef{Target: target, MessageID: sourceMessageID}
			}
			if err := stream.Push(ctx, channel.StreamEvent{
				Type:  channel.StreamEventFinal,
				Final: &channel.StreamFinalizePayload{Message: outMessage},
			}); err != nil {
				return err
			}
		}
	}
	if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventStatus, Status: channel.StreamStatusCompleted}); err != nil {
		return err
	}
	return closeStream()
}

func (p *ChannelInboundProcessor) issueChatToken(identity InboundIdentity, routeID string, msg channel.InboundMessage) string {
	if p.jwtSecret == "" || strings.TrimSpace(msg.ReplyTarget) == "" {
		return ""
	}
	signed, _, err := auth.GenerateChatToken(auth.ChatToken{
		BotID:             identity.BotID,
		ChatID:            identity.BotID,
		RouteID:           strings.TrimSpace(routeID),
		UserID:            identity.UserID,
		ChannelIdentityID: identity.ChannelIdentityID,
	}, p.jwtSecret, p.tokenTTL)
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("issue approval chat token failed", slog.Any("error", err))
		}
		return ""
	}
	return signed
}

func looksLikeApprovalID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return strings.Contains(value, "-")
		}
	}
	return true
}

// resolveNewSessionType determines the session type for /new command.
// /new chat → chat, /new discuss → discuss, /new (no arg) → default by context.
// WebUI (local channel) always defaults to chat.
// Groups default to discuss, DMs default to chat.
func resolveNewSessionType(cmdText string, msg channel.InboundMessage) (string, error) {
	extracted := command.ExtractCommandText(cmdText)
	parsed, _ := command.Parse(extracted)

	explicit := strings.ToLower(strings.TrimSpace(parsed.Action))
	switch explicit {
	case "chat":
		return sessionpkg.TypeChat, nil
	case "discuss":
		if isLocalChannelType(msg.Channel) {
			return "", errors.New("discuss mode is not supported via WebUI — use a channel adapter (Telegram, Discord, etc.)")
		}
		return sessionpkg.TypeDiscuss, nil
	case "":
		// Default: local → chat, group → discuss, DM → chat.
		if isLocalChannelType(msg.Channel) {
			return sessionpkg.TypeChat, nil
		}
		if channel.IsPrivateConversationType(msg.Conversation.Type) {
			return sessionpkg.TypeChat, nil
		}
		return sessionpkg.TypeDiscuss, nil
	default:
		return "", fmt.Errorf("unknown session type %q — use /new, /new chat, or /new discuss", explicit)
	}
}

// handleNewSessionCommand resolves the route for the current message and
// creates a brand-new active session, effectively starting a fresh
// conversation in the same IM thread/chat.
// Supports: /new (default), /new chat, /new discuss.
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

	cmdText := rawTextForCommand(msg, "")
	sessType, err := resolveNewSessionType(cmdText, msg)
	if err != nil {
		return sender.Send(ctx, channel.OutboundMessage{
			Target:  target,
			Message: channel.Message{Text: "Error: " + err.Error()},
		})
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
		ChannelIdentityID: identity.ChannelIdentityID,
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

	sess, err := p.sessionEnsurer.CreateNewSession(ctx, identity.BotID, resolved.RouteID, msg.Channel.String(), sessType)
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("create new session via /new command failed", slog.Any("error", err))
		}
		return sender.Send(ctx, channel.OutboundMessage{
			Target:  target,
			Message: channel.Message{Text: "Error: failed to create new session."},
		})
	}

	modeLabel := "chat"
	if sess.Type == sessionpkg.TypeDiscuss {
		modeLabel = "discuss"
	}
	if p.logger != nil {
		p.logger.Info("new session created via /new command",
			slog.String("bot_id", strings.TrimSpace(identity.BotID)),
			slog.String("route_id", strings.TrimSpace(resolved.RouteID)),
			slog.String("session_id", strings.TrimSpace(sess.ID)),
			slog.String("session_type", sess.Type),
			slog.String("channel", msg.Channel.String()),
		)
	}
	return sender.Send(ctx, channel.OutboundMessage{
		Target:  target,
		Message: channel.Message{Text: fmt.Sprintf("New %s conversation started.", modeLabel)},
	})
}

func (p *ChannelInboundProcessor) handleStatusCommand(
	ctx context.Context,
	cfg channel.ChannelConfig,
	msg channel.InboundMessage,
	sender channel.StreamReplySender,
	identity InboundIdentity,
) error {
	target := strings.TrimSpace(msg.ReplyTarget)
	if target == "" {
		return errors.New("reply target missing for /status command")
	}
	if p.routeResolver == nil {
		return sender.Send(ctx, channel.OutboundMessage{
			Target:  target,
			Message: channel.Message{Text: "Error: route resolver not configured."},
		})
	}
	if p.commandHandler == nil {
		return sender.Send(ctx, channel.OutboundMessage{
			Target:  target,
			Message: channel.Message{Text: "Error: command handler not configured."},
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
		ChannelIdentityID: identity.ChannelIdentityID,
		ChannelConfigID:   identity.ChannelConfigID,
		ReplyTarget:       target,
		Metadata:          routeMetadata,
	})
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("resolve route for /status command failed", slog.Any("error", err))
		}
		return sender.Send(ctx, channel.OutboundMessage{
			Target:  target,
			Message: channel.Message{Text: "Error: failed to resolve conversation route."},
		})
	}

	sessionID := ""
	if p.sessionEnsurer != nil {
		sess, sessErr := p.sessionEnsurer.GetActiveSession(ctx, resolved.RouteID)
		if sessErr == nil {
			sessionID = strings.TrimSpace(sess.ID)
		} else if p.logger != nil {
			p.logger.Debug("resolve active session for /status command failed", slog.Any("error", sessErr))
		}
	}

	reply, execErr := p.commandHandler.ExecuteWithInput(ctx, command.ExecuteInput{
		BotID:             strings.TrimSpace(identity.BotID),
		ChannelIdentityID: strings.TrimSpace(identity.ChannelIdentityID),
		UserID:            strings.TrimSpace(identity.UserID),
		Text:              rawTextForCommand(msg, strings.TrimSpace(msg.Message.PlainText())),
		ChannelType:       msg.Channel.String(),
		ConversationType:  strings.TrimSpace(msg.Conversation.Type),
		ConversationID:    strings.TrimSpace(msg.Conversation.ID),
		ThreadID:          threadID,
		RouteID:           strings.TrimSpace(resolved.RouteID),
		SessionID:         sessionID,
	})
	if execErr != nil {
		reply = "Error: " + execErr.Error()
	}

	return sender.Send(ctx, channel.OutboundMessage{
		Target:  target,
		Message: channel.Message{Text: reply},
	})
}
