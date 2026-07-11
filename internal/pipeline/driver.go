package pipeline

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/conversation"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

// ResolveRunConfigResult holds the output of ResolveRunConfig.
type ResolveRunConfigResult struct {
	RunConfig   agentpkg.RunConfig
	ModelID     string // database UUID of the selected model
	RuntimeType string
	// ContextTokenBudget is the chat model's declared context window, or 0
	// when the model does not declare one (or the runtime has no chat model).
	ContextTokenBudget          int
	DirectDiscussPromptPreparer DirectDiscussPromptPreparer
}

// RunConfigResolver resolves a complete agent RunConfig and persists output
// rounds. Implemented by flow.Resolver.
type RunConfigResolver interface {
	ResolveRunConfig(ctx context.Context, botID, sessionID, channelIdentityID, currentPlatform, replyTarget, conversationType, chatToken string) (ResolveRunConfigResult, error)
	InlineImageAttachments(ctx context.Context, botID string, refs []ImageAttachmentRef) []sdk.ImagePart
	LoadContextHistoryProjection(ctx context.Context, botID, sessionID string) (ContextHistoryProjection, error)
	MaybeCompactSession(ctx context.Context, botID, sessionID, userID string, inputTokens, contextTokenBudget int)
	TrimDiscussContext(messages []ContextMessage, contextTokenBudget int) ([]ContextMessage, int)
	StoreRound(ctx context.Context, botID, sessionID, channelIdentityID, currentPlatform string, messages []sdk.Message, modelID string) error
}

// discussStreamer abstracts the agent streaming capability for testability.
type discussStreamer interface {
	Stream(ctx context.Context, cfg agentpkg.RunConfig) <-chan agentpkg.StreamEvent
}

type discussRuntimeStreamer interface {
	StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error)
}

type DiscussCursorStore interface {
	GetDiscussConsumedCursor(ctx context.Context, sessionID, scopeKey string) (int64, error)
	UpsertDiscussConsumedCursor(ctx context.Context, sessionID, scopeKey, routeID, source string, cursor int64) error
}

// DiscussStreamBroadcaster publishes stream events to local UI subscribers.
// Implemented by local.RouteHub.
type DiscussStreamBroadcaster interface {
	PublishEvent(routeKey string, event channel.StreamEvent)
}

// DiscussDriverDeps holds dependencies injected into the DiscussDriver.
type DiscussDriverDeps struct {
	Pipeline        *Pipeline
	EventStore      *EventStore
	Agent           *agentpkg.Agent
	Resolver        RunConfigResolver
	RuntimeStreamer discussRuntimeStreamer
	CursorStore     DiscussCursorStore
	Broadcaster     DiscussStreamBroadcaster
	Logger          *slog.Logger
}

// DiscussSessionConfig holds per-session configuration for discuss mode.
type DiscussSessionConfig struct {
	BotID             string
	SessionID         string
	RouteID           string
	UserID            string
	ChannelIdentityID string
	ReplyTarget       string
	CurrentPlatform   string
	ConversationType  string
	ConversationName  string
	SessionToken      string //nolint:gosec // session credential material
	ChatToken         string //nolint:gosec // scoped chat routing token
	ToolHTTPURL       string
}

// DiscussDriver manages discuss-mode sessions. It is goroutine-safe.
type DiscussDriver struct {
	deps     DiscussDriverDeps
	mu       sync.Mutex
	sessions map[string]*discussSession
	logger   *slog.Logger
}

type discussSession struct {
	config          DiscussSessionConfig
	rcCh            chan RenderedContext
	stopCh          chan struct{}
	cancel          context.CancelFunc
	lastProcessedMs int64
}

// NewDiscussDriver creates a new DiscussDriver.
func NewDiscussDriver(deps DiscussDriverDeps) *DiscussDriver {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &DiscussDriver{
		deps:     deps,
		sessions: make(map[string]*discussSession),
		logger:   logger.With(slog.String("service", "discuss_driver")),
	}
}

// NotifyRC pushes a new RenderedContext to the discuss session.
// If the session goroutine is not running, it starts one.
func (d *DiscussDriver) NotifyRC(_ context.Context, sessionID string, rc RenderedContext, config DiscussSessionConfig) {
	d.mu.Lock()
	sess, ok := d.sessions[sessionID]
	if !ok {
		sessCtx, cancel := context.WithCancel(context.Background()) //nolint:gosec // G118: cancel is stored in sess.cancel
		sess = &discussSession{
			config: config,
			rcCh:   make(chan RenderedContext, 16),
			stopCh: make(chan struct{}),
			cancel: cancel,
		}
		d.sessions[sessionID] = sess
		go d.runSession(sessCtx, sess) //nolint:contextcheck // long-lived goroutine; must outlive the inbound HTTP request
	} else {
		sess.config = config
	}
	d.mu.Unlock()

	select {
	case sess.rcCh <- rc:
	default:
		select {
		case <-sess.rcCh:
		default:
		}
		select {
		case sess.rcCh <- rc:
		default:
		}
	}
}

// StopSession stops a discuss session goroutine.
func (d *DiscussDriver) StopSession(sessionID string) {
	d.mu.Lock()
	sess, ok := d.sessions[sessionID]
	if ok {
		sess.cancel()
		close(sess.stopCh)
		delete(d.sessions, sessionID)
	}
	d.mu.Unlock()
}

// StopAll stops all discuss session goroutines.
func (d *DiscussDriver) StopAll() {
	d.mu.Lock()
	for id, sess := range d.sessions {
		sess.cancel()
		close(sess.stopCh)
		delete(d.sessions, id)
	}
	d.mu.Unlock()
}

// HasSession returns true if a discuss session goroutine is running.
func (d *DiscussDriver) HasSession(sessionID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.sessions[sessionID]
	return ok
}

func (d *DiscussDriver) sessionConfigSnapshot(sess *discussSession) DiscussSessionConfig {
	d.mu.Lock()
	defer d.mu.Unlock()
	return sess.config
}

const discussIdleTimeout = 10 * time.Minute

func (d *DiscussDriver) runSession(ctx context.Context, sess *discussSession) {
	initialConfig := d.sessionConfigSnapshot(sess)
	sessionID := initialConfig.SessionID
	log := d.logger.With(slog.String("session_id", sessionID), slog.String("bot_id", initialConfig.BotID))
	log.Info("discuss session started")
	defer func() {
		log.Info("discuss session stopped")
		d.mu.Lock()
		if cur, ok := d.sessions[sessionID]; ok && cur == sess {
			delete(d.sessions, sessionID)
		}
		d.mu.Unlock()
	}()

	idle := time.NewTimer(discussIdleTimeout)
	defer idle.Stop()

	var latestRC RenderedContext

	for {
		select {
		case <-sess.stopCh:
			return
		case <-idle.C:
			log.Info("discuss session idle timeout, exiting")
			return
		case rc := <-sess.rcCh:
			latestRC = rc
			idle.Reset(discussIdleTimeout)
		}

	drain:
		for {
			select {
			case rc := <-sess.rcCh:
				latestRC = rc
			default:
				break drain
			}
		}

		if len(latestRC) == 0 {
			continue
		}

		if LatestExternalEventMs(latestRC, sess.lastProcessedMs) == 0 {
			continue
		}

		d.handleReply(ctx, sess, latestRC, log)
	}
}

func (d *DiscussDriver) handleReply(ctx context.Context, sess *discussSession, rc RenderedContext, log *slog.Logger) {
	d.handleReplyWithAgent(ctx, sess, rc, log, d.deps.Agent)
}

func (d *DiscussDriver) handleReplyWithAgent(ctx context.Context, sess *discussSession, rc RenderedContext, log *slog.Logger, agent discussStreamer) {
	cfg := d.sessionConfigSnapshot(sess)
	if d.deps.Resolver == nil {
		log.Error("discuss driver: resolver not configured")
		return
	}

	history, err := d.deps.Resolver.LoadContextHistoryProjection(ctx, cfg.BotID, cfg.SessionID)
	if err != nil {
		log.Error("discuss: load context history projection failed", slog.Any("error", err))
		return
	}
	trs := history.TurnResponses
	artifacts := history.CompactionArtifacts
	activeRC := ActiveRenderedContext(rc, artifacts)

	// Cold-start / post-idle initialisation: if we haven't processed anything
	// in this goroutine's lifetime yet, anchor `lastProcessedMs` to the most
	// recent TR's requested_at. Any RC segment strictly older than that has
	// already been "seen" by a prior LLM call (whose response is in the TR
	// stream), so it should not retrigger a reply. Without this anchor, every
	// idle-timeout restart would treat the entire session history as brand
	// new external traffic and re-answer it.
	if sess.lastProcessedMs == 0 {
		sess.lastProcessedMs = maxInt64(history.LatestTurnResponseAtMs, d.loadDiscussCursor(ctx, cfg, log))
	}

	// Re-evaluate the trigger condition now that lastProcessedMs is anchored.
	// The outer loop used lastProcessedMs=0 to allow first-time dispatch into
	// this function; after initialisation, we must verify there's actually a
	// new external event past the anchor before spending an LLM call.
	if LatestExternalEventMs(activeRC, sess.lastProcessedMs) == 0 {
		if LatestExternalEventMs(rc, sess.lastProcessedMs) > 0 {
			d.advanceDiscussCursor(ctx, sess, cfg, latestRCEventAtMs(rc), log)
		}
		return
	}

	composed := ComposeContextWithArtifacts(rc, trs, artifacts)
	if composed == nil {
		return
	}

	log.Info("triggering discuss LLM call",
		slog.Int("messages", len(composed.Messages)),
		slog.Int("estimated_tokens", composed.EstimatedTokens))

	resolved, err := d.deps.Resolver.ResolveRunConfig(ctx,
		cfg.BotID, cfg.SessionID, cfg.ChannelIdentityID,
		cfg.CurrentPlatform, cfg.ReplyTarget, cfg.ConversationType, cfg.SessionToken)
	if err != nil {
		log.Error("discuss: resolve run config failed", slog.Any("error", err))
		return
	}
	if strings.TrimSpace(resolved.RuntimeType) == sessionpkg.RuntimeACPAgent {
		isMentioned := wasRecentlyMentioned(activeRC, sess.lastProcessedMs)
		// A direct/1:1 conversation is always "addressed" (matching the inbound
		// layer's isDirectedAtBot/shouldTriggerAssistantResponse), so a DM
		// discuss-ACP session must reply even without an explicit @-mention or
		// reply-to. Without this, a `/new discuss codex` session in a DM would
		// be permanently silent.
		addressed := isMentioned || channel.IsPrivateConversationType(cfg.ConversationType)
		consumedMs := latestRCEventAtMs(rc)
		// Participation gate for external ACP runtimes. Unlike the cheap model
		// runtime, entering the ACP path spins up Codex/Claude Code, so we must
		// not pre-warm it for passive group chatter. Only start the runtime when
		// the bot was actually addressed. When it is not, advance the consumed
		// cursor without starting a runtime so the same batch is not
		// re-evaluated; those messages remain covered as context on the next
		// addressed turn because the reset/full-context prompt re-composes from
		// the RC window.
		if !addressed {
			d.advanceDiscussCursor(ctx, sess, cfg, consumedMs, log)
			return
		}
		attempted, completed := d.streamDiscussACPRuntime(ctx, cfg, composed, addressed, log)
		if attempted {
			d.maybeCompactDiscussContext(ctx, cfg, composed.EstimatedTokens, resolved.ContextTokenBudget)
		}
		if completed {
			d.advanceDiscussCursor(ctx, sess, cfg, consumedMs, log)
		}
		return
	}
	runConfig := resolved.RunConfig

	composedMessages, estimatedTokens := composed.Messages, composed.EstimatedTokens
	if resolved.ContextTokenBudget > 0 {
		composedMessages, estimatedTokens = d.deps.Resolver.TrimDiscussContext(composedMessages, resolved.ContextTokenBudget)
	}
	contextEntries := repairSDKContextToolClosures(contextMessagesToSDKEntries(composedMessages))
	runConfig.Messages = sdkMessagesFromContextEntries(contextEntries)
	runConfig.ContextFrags = append(
		runConfig.ContextFrags,
		compactionSummaryContextFrags(contextEntries, artifacts, runConfig.ContextScope)...,
	)
	runConfig.SessionType = sessionpkg.TypeDiscuss
	runConfig.Query = ""

	// Inline image attachments from new RC segments so the model receives
	// them as native vision input (ImagePart) on the first encounter.
	// Subsequent turns only see the file path in the XML rendering.
	if runConfig.SupportsImageInput && d.deps.Resolver != nil {
		imageRefs := extractNewImageRefs(activeRC, sess.lastProcessedMs)
		if len(imageRefs) > 0 {
			imageParts := d.deps.Resolver.InlineImageAttachments(ctx, cfg.BotID, imageRefs)
			injectImagePartsIntoLastUserMessage(runConfig.Messages, imageParts)
		}
	}

	isMentioned := wasRecentlyMentioned(activeRC, sess.lastProcessedMs)
	lateBinding := buildLateBindingPrompt(isMentioned)
	runConfig.Messages = append(runConfig.Messages, sdk.UserMessage(lateBinding))
	runConfig = runConfig.RefreshContextFrag()

	eventCh := agent.Stream(ctx, runConfig)

	var finalMessages json.RawMessage
	var finalUsage json.RawMessage
	terminalReceived := false
	for event := range eventCh {
		d.broadcastDiscussEvent(cfg.BotID, event)

		switch event.Type {
		case agentpkg.EventError:
			log.Error("discuss stream error", slog.String("error", event.Error))
		case agentpkg.EventAgentEnd, agentpkg.EventAgentAbort:
			terminalReceived = true
			finalMessages = event.Messages
			finalUsage = event.Usage
		}
	}

	if d.deps.Resolver != nil && len(finalMessages) > 0 {
		var sdkMsgs []sdk.Message
		if json.Unmarshal(finalMessages, &sdkMsgs) == nil && len(sdkMsgs) > 0 {
			if storeErr := d.deps.Resolver.StoreRound(ctx,
				cfg.BotID, cfg.SessionID, cfg.ChannelIdentityID, cfg.CurrentPlatform,
				sdkMsgs, resolved.ModelID,
			); storeErr != nil {
				log.Error("discuss: store round failed", slog.Any("error", storeErr))
			}
		}
	}
	inputTokens := usageInputTokens(finalUsage)
	if inputTokens <= 0 {
		inputTokens = estimatedTokens
	}
	d.maybeCompactDiscussContext(ctx, cfg, inputTokens, resolved.ContextTokenBudget)
	if !terminalReceived {
		return
	}

	// Advance the cursor to the latest RC segment actually consumed in this
	// turn (not wall-clock time). Messages that arrive DURING LLM generation
	// will land in a newer RC event past this cursor and correctly
	// trigger another round; wall-clock would wrongly mark them processed.
	consumedMs := latestRCEventAtMs(rc)
	d.advanceDiscussCursor(ctx, sess, cfg, consumedMs, log)
}

func (d *DiscussDriver) streamDiscussACPRuntime(
	ctx context.Context,
	cfg DiscussSessionConfig,
	composed *ComposeContextResult,
	isMentioned bool,
	log *slog.Logger,
) (bool, bool) {
	if d.deps.RuntimeStreamer == nil {
		log.Error("discuss ACP runtime: streamer not configured")
		return false, false
	}
	prompt := discussACPFullContextPrompt(composed.Messages, buildLateBindingPrompt(isMentioned))
	if strings.TrimSpace(prompt) == "" {
		return false, false
	}
	chunks, errs := d.deps.RuntimeStreamer.StreamChat(ctx, conversation.ChatRequest{
		BotID:                   cfg.BotID,
		ChatID:                  cfg.BotID,
		SessionID:               cfg.SessionID,
		RouteID:                 cfg.RouteID,
		SourceChannelIdentityID: cfg.ChannelIdentityID,
		CurrentChannel:          cfg.CurrentPlatform,
		ReplyTarget:             cfg.ReplyTarget,
		ConversationType:        cfg.ConversationType,
		Token:                   cfg.SessionToken,
		ChatToken:               cfg.ChatToken,
		ToolHTTPURL:             cfg.ToolHTTPURL,
		Query:                   prompt,
		RawQuery:                prompt,
		UserMessagePersisted:    true,
		SkipMemoryExtraction:    true,
		ForceFreshRuntime:       true,
	})
	streamed := false
	terminal := false
	failed := false
	for chunks != nil || errs != nil {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				chunks = nil
				continue
			}
			var event agentpkg.StreamEvent
			if err := json.Unmarshal(chunk, &event); err != nil {
				log.Warn("discuss ACP runtime: decode stream event failed", slog.Any("error", err))
				failed = true
				continue
			}
			streamed = true
			if event.Type == agentpkg.EventError {
				failed = true
			}
			if event.Type == agentpkg.EventAgentEnd || event.Type == agentpkg.EventAgentAbort {
				terminal = true
			}
			d.broadcastDiscussEvent(cfg.BotID, event)
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil {
				log.Error("discuss ACP runtime failed", slog.Any("error", err))
				failed = true
			}
		case <-ctx.Done():
			log.Warn("discuss ACP runtime cancelled", slog.Any("error", ctx.Err()))
			return true, false
		}
	}
	return true, streamed && terminal && !failed
}

func discussACPFullContextPrompt(messages []ContextMessage, lateBinding string) string {
	var b strings.Builder
	b.WriteString("You are replying in a discuss-mode conversation. The runtime is reset each turn, so use the complete context below as the source of truth.\n\n")
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "user"
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		b.WriteString("[")
		b.WriteString(role)
		b.WriteString("]\n")
		b.WriteString(content)
		b.WriteString("\n\n")
	}
	b.WriteString("Reply to the latest user-visible message when a response is appropriate.")
	if strings.TrimSpace(lateBinding) != "" {
		b.WriteString("\n\n")
		b.WriteString(strings.TrimSpace(lateBinding))
	}
	return strings.TrimSpace(b.String())
}
