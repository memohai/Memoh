package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/conversation"
	dbpkg "github.com/memohai/memoh/internal/db"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

// ResolveRunConfigResult holds the output of ResolveRunConfig.
type ResolveRunConfigResult struct {
	RunConfig   agentpkg.RunConfig
	ModelID     string // database UUID of the selected model
	RuntimeType string
	// ContextTokenBudget is the chat model's declared context window, or 0
	// when the model does not declare one (or the runtime has no chat model).
	ContextTokenBudget int
}

// RunConfigResolver resolves a complete agent RunConfig and persists output
// rounds. Implemented by flow.Resolver.
type RunConfigResolver interface {
	ResolveRunConfig(ctx context.Context, botID, sessionID, channelIdentityID, currentPlatform, replyTarget, conversationType, chatToken string) (ResolveRunConfigResult, error)
	InlineImageAttachments(ctx context.Context, botID string, refs []ImageAttachmentRef) []sdk.ImagePart
	LoadContextHistoryProjection(ctx context.Context, botID, sessionID string) (ContextHistoryProjection, error)
	ScheduleCompaction(ctx context.Context, botID, sessionID, userID string, inputTokens, contextTokenBudget int)
	TrimDiscussContext(messages []ContextMessage, contextTokenBudget int, afterCursor int64) ([]ContextMessage, int)
	StoreRound(ctx context.Context, botID, sessionID, channelIdentityID, currentPlatform, persistedUserMessageID string, messages []sdk.Message, modelID string) error
	StoreRoundWithCursor(ctx context.Context, botID, sessionID, channelIdentityID, currentPlatform, persistedUserMessageID string, messages []sdk.Message, modelID string, cursor DiscussCursorCommit) (bool, error)
}

// discussStreamer abstracts the agent streaming capability for testability.
type discussStreamer interface {
	Stream(ctx context.Context, cfg agentpkg.RunConfig) <-chan agentpkg.StreamEvent
}

type discussRuntimeStreamer interface {
	StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error)
}

type DiscussCursorStore interface {
	GetDiscussCursor(ctx context.Context, sessionID, scopeKey string) (DiscussCursorPosition, error)
	UpsertDiscussCursor(ctx context.Context, sessionID, scopeKey, routeID, source string, position DiscussCursorPosition) error
}

type DiscussCursorPosition struct {
	SourceCursor int64
	EventCursor  int64
}

type DiscussCursorCommit struct {
	ScopeKey       string
	RouteID        string
	Source         string
	Position       DiscussCursorPosition
	DeliveryClaims []DeliveryClaim
}

type DeliveryClaim struct {
	EventID    string
	ClaimToken string
}

// DiscussStreamBroadcaster publishes stream events to local UI subscribers.
// Implemented by local.RouteHub.
type DiscussStreamBroadcaster interface {
	PublishEvent(routeKey string, event channel.StreamEvent)
}

// DiscussDriverDeps holds dependencies injected into the DiscussDriver.
type DiscussDriverDeps struct {
	Agent           discussStreamer
	Resolver        RunConfigResolver
	RuntimeStreamer discussRuntimeStreamer
	CursorStore     DiscussCursorStore
	Broadcaster     DiscussStreamBroadcaster
	Logger          *slog.Logger
}

// DiscussSessionConfig holds per-session configuration for discuss mode.
type DiscussSessionConfig struct {
	BotID                  string
	SessionID              string
	RouteID                string
	UserID                 string
	ChannelIdentityID      string
	ReplyTarget            string
	CurrentPlatform        string
	ConversationType       string
	ConversationName       string
	SessionToken           string //nolint:gosec // session credential material
	ChatToken              string //nolint:gosec // scoped chat routing token
	ToolHTTPURL            string
	PersistedUserMessageID string
	EventDelivery          *DiscussEventDelivery
}

type DiscussEventDelivery struct {
	EventID     string
	EventCursor int64
	Lease       *EventDeliveryLease
}

func discussEventDeliveryClaims(deliveries []DiscussEventDelivery) ([]DeliveryClaim, error) {
	if len(deliveries) == 0 {
		return nil, nil
	}
	claims := make([]DeliveryClaim, 0, len(deliveries))
	seen := make(map[string]struct{}, len(deliveries))
	for i, delivery := range deliveries {
		if delivery.Lease == nil {
			return nil, fmt.Errorf("discuss event delivery %d has no lease", i)
		}
		eventID, err := dbpkg.ParseUUID(strings.TrimSpace(delivery.EventID))
		if err != nil {
			return nil, fmt.Errorf("invalid discuss event delivery %d id: %w", i, err)
		}
		if !delivery.Lease.eventID.Valid || eventID != delivery.Lease.eventID {
			return nil, fmt.Errorf("discuss event delivery %d does not match its lease", i)
		}
		if !delivery.Lease.claimToken.Valid {
			return nil, fmt.Errorf("discuss event delivery %d has no claim token", i)
		}
		canonicalEventID := eventID.String()
		if _, ok := seen[canonicalEventID]; ok {
			return nil, fmt.Errorf("duplicate discuss event delivery %q", canonicalEventID)
		}
		seen[canonicalEventID] = struct{}{}
		claims = append(claims, DeliveryClaim{
			EventID:    canonicalEventID,
			ClaimToken: delivery.Lease.claimToken.String(),
		})
	}
	return claims, nil
}

func discussCursorCommitWithClaims(
	cfg DiscussSessionConfig,
	position DiscussCursorPosition,
	claims []DeliveryClaim,
) DiscussCursorCommit {
	commit := discussCursorCommit(cfg, position)
	commit.DeliveryClaims = append([]DeliveryClaim(nil), claims...)
	return commit
}

type discussReplyResult uint8

const (
	discussReplyRetry discussReplyResult = iota
	discussReplyComplete
	discussReplyCursorPending
)

// DiscussDriver manages discuss-mode sessions. It is goroutine-safe.
type DiscussDriver struct {
	deps             DiscussDriverDeps
	mu               sync.Mutex
	sessions         map[string]*discussSession
	logger           *slog.Logger
	idleTimeout      time.Duration
	cursorRetryDelay time.Duration
}

type discussSession struct {
	config              DiscussSessionConfig
	rcCh                chan discussNotification
	stopCh              chan struct{}
	cancel              context.CancelFunc
	lastProcessedCursor int64
	pendingCursor       *pendingDiscussCursor
}

type pendingDiscussCursor struct {
	config     DiscussSessionConfig
	position   DiscussCursorPosition
	deliveries []DiscussEventDelivery
}

type discussNotification struct {
	rc         RenderedContext
	config     DiscussSessionConfig
	deliveries []DiscussEventDelivery
}

// NewDiscussDriver creates a new DiscussDriver.
func NewDiscussDriver(deps DiscussDriverDeps) *DiscussDriver {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &DiscussDriver{
		deps:             deps,
		sessions:         make(map[string]*discussSession),
		logger:           logger.With(slog.String("service", "discuss_driver")),
		idleTimeout:      discussIdleTimeout,
		cursorRetryDelay: time.Second,
	}
}

// NotifyRC pushes a new RenderedContext to the discuss session.
// If the session goroutine is not running, it starts one.
func (d *DiscussDriver) NotifyRC(ctx context.Context, sessionID string, rc RenderedContext, config DiscussSessionConfig) {
	notification := discussNotification{rc: rc, config: config}
	if config.EventDelivery != nil {
		notification.deliveries = []DiscussEventDelivery{*config.EventDelivery}
	}
	d.mu.Lock()
	sess, ok := d.sessions[sessionID]
	if !ok {
		sessCtx, cancel := context.WithCancel(context.WithoutCancel(ctx)) //nolint:gosec // G118: cancel is stored in sess.cancel
		sess = &discussSession{
			config: config,
			rcCh:   make(chan discussNotification, 16),
			stopCh: make(chan struct{}),
			cancel: cancel,
		}
		d.sessions[sessionID] = sess
		go d.runSession(sessCtx, sess) //nolint:contextcheck // long-lived goroutine; must outlive the inbound HTTP request
	} else {
		sess.config = config
	}
	discarded := enqueueDiscussNotification(sess.rcCh, notification)
	d.mu.Unlock()
	releaseDiscussEventDeliveries(ctx, discarded, d.logger)
}

func enqueueDiscussNotification(ch chan discussNotification, notification discussNotification) []DiscussEventDelivery {
	select {
	case ch <- notification:
		return nil
	default:
		latest := notification
		var discarded []DiscussEventDelivery
		for {
			select {
			case queued := <-ch:
				var dropped []DiscussEventDelivery
				latest, dropped = newerDiscussNotification(queued, latest)
				discarded = append(discarded, dropped...)
			default:
				ch <- latest
				return discarded
			}
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
		d.releaseDiscussSessionDeliveries(ctx, sess)
		log.Info("discuss session stopped")
		d.mu.Lock()
		if cur, ok := d.sessions[sessionID]; ok && cur == sess {
			delete(d.sessions, sessionID)
		}
		d.mu.Unlock()
	}()

	idle := time.NewTimer(d.idleTimeout)
	defer idle.Stop()

	for {
		if sess.pendingCursor != nil {
			pending := sess.pendingCursor
			retryDelay := d.cursorRetryDelay
			if retryDelay <= 0 {
				retryDelay = time.Second
			}
			retry := time.NewTimer(retryDelay)
			select {
			case <-ctx.Done():
				retry.Stop()
				return
			case <-sess.stopCh:
				retry.Stop()
				return
			case <-retry.C:
			}
			switch d.retryPendingDiscussCursor(ctx, sess, log) {
			case pendingDiscussCursorRetryPending:
				continue
			case pendingDiscussCursorRetryReconciled, pendingDiscussCursorRetryAbandoned:
				releaseDiscussEventDeliveries(ctx, pending.deliveries, log)
				continue
			}
			completeDiscussEventDeliveries(ctx, pending.deliveries, log)
		}
		var latest discussNotification
		select {
		case <-sess.stopCh:
			return
		case <-idle.C:
			log.Info("discuss session idle timeout reached")
			var ok bool
			latest, ok = d.takeQueuedNotificationOrRetire(ctx, sessionID, sess)
			if !ok {
				return
			}
			idle.Reset(d.idleTimeout)
		case notification := <-sess.rcCh:
			latest = notification
			idle.Reset(d.idleTimeout)
		}

		var discarded []DiscussEventDelivery
		latest, discarded = drainLatestDiscussNotification(sess.rcCh, latest)
		releaseDiscussEventDeliveries(ctx, discarded, log)

		if len(latest.rc) == 0 {
			releaseDiscussEventDeliveries(ctx, latest.deliveries, log)
			continue
		}

		currentTriggers := newDiscussCurrentTriggerSelection(latest.deliveries)
		if !hasExternalCurrentTrigger(latest.rc, currentTriggers, sess.lastProcessedCursor) {
			completeDiscussEventDeliveries(ctx, latest.deliveries, log)
			continue
		}
		claims, err := discussEventDeliveryClaims(latest.deliveries)
		if err != nil {
			log.Error("discuss event delivery ownership is invalid", slog.Any("error", err))
			releaseDiscussEventDeliveries(ctx, latest.deliveries, log)
			continue
		}

		replyCtx, cancelReply, active := bindDiscussEventDeliveries(ctx, latest.deliveries)
		if !active {
			cancelReply()
			releaseDiscussEventDeliveries(ctx, latest.deliveries, log)
			continue
		}
		result := d.handleReply(replyCtx, sess, latest.rc, latest.config, currentTriggers, claims, log)
		cancelReply()
		switch result {
		case discussReplyComplete:
			completeDiscussEventDeliveries(ctx, latest.deliveries, log)
		case discussReplyCursorPending:
			if sess.pendingCursor == nil {
				releaseDiscussEventDeliveries(ctx, latest.deliveries, log)
				continue
			}
			merged, discarded := mergeDiscussEventDeliveries(
				sess.pendingCursor.deliveries,
				latest.deliveries,
			)
			sess.pendingCursor.deliveries = merged
			releaseDiscussEventDeliveries(ctx, discarded, log)
		default:
			releaseDiscussEventDeliveries(ctx, latest.deliveries, log)
		}
	}
}

func (d *DiscussDriver) takeQueuedNotificationOrRetire(
	ctx context.Context,
	sessionID string,
	sess *discussSession,
) (discussNotification, bool) {
	d.mu.Lock()

	if current, ok := d.sessions[sessionID]; !ok || current != sess {
		d.mu.Unlock()
		return discussNotification{}, false
	}
	select {
	case notification := <-sess.rcCh:
		latest, discarded := drainLatestDiscussNotification(sess.rcCh, notification)
		d.mu.Unlock()
		releaseDiscussEventDeliveries(ctx, discarded, d.logger)
		return latest, true
	default:
		delete(d.sessions, sessionID)
		d.mu.Unlock()
		return discussNotification{}, false
	}
}

func drainLatestDiscussNotification(ch <-chan discussNotification, latest discussNotification) (discussNotification, []DiscussEventDelivery) {
	var discarded []DiscussEventDelivery
	for {
		select {
		case notification := <-ch:
			var dropped []DiscussEventDelivery
			latest, dropped = newerDiscussNotification(latest, notification)
			discarded = append(discarded, dropped...)
		default:
			return latest, discarded
		}
	}
}

func newerDiscussNotification(a, b discussNotification) (discussNotification, []DiscussEventDelivery) {
	deliveries, discarded := mergeDiscussEventDeliveries(a.deliveries, b.deliveries)
	b.deliveries = deliveries
	return b, discarded
}

func (d *DiscussDriver) handleReply(
	ctx context.Context,
	sess *discussSession,
	rc RenderedContext,
	cfg DiscussSessionConfig,
	currentTriggers discussCurrentTriggerSelection,
	claims []DeliveryClaim,
	log *slog.Logger,
) discussReplyResult {
	return d.handleReplyWithAgentConfigAndTriggers(ctx, sess, rc, cfg, currentTriggers, claims, log, d.deps.Agent)
}

func (d *DiscussDriver) handleReplyWithAgent(ctx context.Context, sess *discussSession, rc RenderedContext, log *slog.Logger, agent discussStreamer) discussReplyResult {
	cfg := d.sessionConfigSnapshot(sess)
	return d.handleReplyWithAgentConfig(ctx, sess, rc, cfg, log, agent)
}

func (d *DiscussDriver) handleReplyWithAgentConfig(
	ctx context.Context,
	sess *discussSession,
	rc RenderedContext,
	cfg DiscussSessionConfig,
	log *slog.Logger,
	agent discussStreamer,
) discussReplyResult {
	return d.handleReplyWithAgentConfigAndTriggers(
		ctx,
		sess,
		rc,
		cfg,
		newDiscussCurrentTriggerSelection(eventDeliveriesFromConfig(cfg)),
		nil,
		log,
		agent,
	)
}

func (d *DiscussDriver) handleReplyWithAgentConfigAndTriggers(
	ctx context.Context,
	sess *discussSession,
	rc RenderedContext,
	cfg DiscussSessionConfig,
	currentTriggers discussCurrentTriggerSelection,
	claims []DeliveryClaim,
	log *slog.Logger,
	agent discussStreamer,
) discussReplyResult {
	if d.deps.Resolver == nil {
		log.Error("discuss driver: resolver not configured")
		return discussReplyRetry
	}

	history, err := d.deps.Resolver.LoadContextHistoryProjection(ctx, cfg.BotID, cfg.SessionID)
	if err != nil {
		log.Error("discuss: load context history projection failed", slog.Any("error", err))
		return discussReplyRetry
	}
	artifacts := history.CompactionArtifacts
	triggerRC := markCurrentTriggerSegments(rc, currentTriggers)
	recentRC := recentRenderedContextWithCurrentTriggers(triggerRC, history.WindowStartAtMs)
	activeRC := ActiveRenderedContext(recentRC, artifacts)

	// Cold-start / post-idle initialisation trusts the durable cursor when one
	// exists. Timestamp-based TR mapping is only a legacy fallback: using it to
	// advance an exact cursor can hide a delayed event that arrived after the
	// response with an older source timestamp.
	if sess.lastProcessedCursor == 0 {
		persisted := d.loadDiscussCursor(ctx, cfg, log)
		if persisted.EventCursor > 0 {
			sess.lastProcessedCursor = persisted.EventCursor
		} else {
			legacyBoundary := persisted.SourceCursor
			if history.LatestTurnResponseAtMs > legacyBoundary {
				legacyBoundary = history.LatestTurnResponseAtMs
			}
			seeded := historyDiscussCursor(rc, legacyBoundary)
			if !d.advanceDiscussCursor(ctx, sess, cfg, seeded, log) {
				return discussReplyCursorPending
			}
		}
	}

	// Re-evaluate the trigger condition now that lastProcessedCursor is anchored.
	// The outer loop used lastProcessedCursor=0 to allow first-time dispatch into
	// this function; after initialisation, we must verify there's actually a
	// new external event past the anchor before spending an LLM call.
	if !hasExternalCurrentTrigger(activeRC, currentTriggers, sess.lastProcessedCursor) {
		if LatestExternalEventCursor(rc, sess.lastProcessedCursor) > 0 {
			if !d.advanceDiscussCursor(ctx, sess, cfg, renderedContextDiscussCursor(rc), log) {
				return discussReplyCursorPending
			}
		}
		return discussReplyComplete
	}

	composed := ComposeContextProjection(recentRC, history)
	if composed == nil {
		return discussReplyRetry
	}

	log.Info("triggering discuss LLM call",
		slog.Int("messages", len(composed.Messages)),
		slog.Int("estimated_tokens", composed.EstimatedTokens))

	resolved, err := d.deps.Resolver.ResolveRunConfig(ctx,
		cfg.BotID, cfg.SessionID, cfg.ChannelIdentityID,
		cfg.CurrentPlatform, cfg.ReplyTarget, cfg.ConversationType, cfg.SessionToken)
	if err != nil {
		log.Error("discuss: resolve run config failed", slog.Any("error", err))
		return discussReplyRetry
	}
	if strings.TrimSpace(resolved.RuntimeType) == sessionpkg.RuntimeACPAgent {
		isMentioned := wasRecentlyMentionedWithTriggers(activeRC, currentTriggers, sess.lastProcessedCursor)
		// A direct/1:1 conversation is always "addressed" (matching the inbound
		// layer's isDirectedAtBot/shouldTriggerAssistantResponse), so a DM
		// discuss-ACP session must reply even without an explicit @-mention or
		// reply-to. Without this, a `/new discuss codex` session in a DM would
		// be permanently silent.
		addressed := isMentioned || channel.IsPrivateConversationType(cfg.ConversationType)
		consumedCursor := renderedContextDiscussCursor(rc)
		// Participation gate for external ACP runtimes. Unlike the cheap model
		// runtime, entering the ACP path spins up Codex/Claude Code, so we must
		// not pre-warm it for passive group chatter. Only start the runtime when
		// the bot was actually addressed. When it is not, advance the consumed
		// cursor without starting a runtime so the same batch is not
		// re-evaluated; those messages remain covered as context on the next
		// addressed turn because the reset/full-context prompt re-composes from
		// the RC window.
		if !addressed {
			if !d.advanceDiscussCursor(ctx, sess, cfg, consumedCursor, log) {
				return discussReplyCursorPending
			}
			return discussReplyComplete
		}
		attempted, terminal, cursorCommitted := d.streamDiscussACPRuntime(
			ctx,
			cfg,
			composed,
			sess.lastProcessedCursor,
			addressed,
			discussCursorCommitWithClaims(cfg, consumedCursor, claims),
			log,
		)
		if attempted {
			d.maybeCompactDiscussContext(ctx, cfg, composed.EstimatedTokens, resolved.ContextTokenBudget)
		}
		if cursorCommitted {
			if consumedCursor.EventCursor > sess.lastProcessedCursor {
				sess.lastProcessedCursor = consumedCursor.EventCursor
			}
			sess.pendingCursor = nil
			return discussReplyComplete
		}
		if terminal {
			if !d.advanceDiscussCursor(ctx, sess, cfg, consumedCursor, log) {
				return discussReplyCursorPending
			}
			return discussReplyComplete
		}
		return discussReplyRetry
	}
	runConfig := resolved.RunConfig

	composedMessages, estimatedTokens := composed.Messages, composed.EstimatedTokens
	if resolved.ContextTokenBudget > 0 {
		composedMessages, estimatedTokens = d.deps.Resolver.TrimDiscussContext(
			composedMessages,
			resolved.ContextTokenBudget,
			sess.lastProcessedCursor,
		)
	}
	contextEntries := repairSDKContextToolClosures(contextMessagesToSDKEntries(composedMessages))
	runConfig.SessionType = sessionpkg.TypeDiscuss
	runConfig.Query = ""

	// Inline image attachments from new RC segments so the model receives
	// them as native vision input (ImagePart) on the first encounter.
	// Subsequent turns only see the file path in the XML rendering.
	if runConfig.SupportsImageInput && d.deps.Resolver != nil {
		for _, target := range extractNewImageTargets(activeRC, currentTriggers, sess.lastProcessedCursor) {
			imageParts := d.deps.Resolver.InlineImageAttachments(ctx, cfg.BotID, target.refs)
			injectImagePartsIntoRenderedUserMessage(
				contextEntries,
				target.messageID,
				target.eventCursor,
				imageParts,
			)
		}
	}
	runConfig.Messages = sdkMessagesFromContextEntries(contextEntries)
	runConfig.ContextFrags = append(
		runConfig.ContextFrags,
		compactionSummaryContextFrags(contextEntries, artifacts, runConfig.ContextScope)...,
	)

	isMentioned := wasRecentlyMentionedWithTriggers(activeRC, currentTriggers, sess.lastProcessedCursor)
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

	consumedCursor := renderedContextDiscussCursor(rc)
	roundCursorPersisted := false
	if len(finalMessages) > 0 {
		var sdkMsgs []sdk.Message
		if err := json.Unmarshal(finalMessages, &sdkMsgs); err != nil {
			log.Error("discuss: decode final messages failed", slog.Any("error", err))
			return discussReplyRetry
		}
		if len(sdkMsgs) > 0 {
			var storeErr error
			if terminalReceived {
				roundCursorPersisted, storeErr = d.deps.Resolver.StoreRoundWithCursor(ctx,
					cfg.BotID, cfg.SessionID, cfg.ChannelIdentityID, cfg.CurrentPlatform, cfg.PersistedUserMessageID,
					sdkMsgs, resolved.ModelID,
					discussCursorCommitWithClaims(cfg, consumedCursor, claims),
				)
			} else {
				storeErr = d.deps.Resolver.StoreRound(ctx,
					cfg.BotID, cfg.SessionID, cfg.ChannelIdentityID, cfg.CurrentPlatform, cfg.PersistedUserMessageID,
					sdkMsgs, resolved.ModelID,
				)
			}
			if storeErr != nil {
				log.Error("discuss: store round failed", slog.Any("error", storeErr))
				return discussReplyRetry
			}
		}
	}
	inputTokens := usageInputTokens(finalUsage)
	if inputTokens <= 0 {
		inputTokens = estimatedTokens
	}
	d.maybeCompactDiscussContext(ctx, cfg, inputTokens, resolved.ContextTokenBudget)
	if !terminalReceived {
		return discussReplyRetry
	}

	// Advance the cursor to the latest RC segment actually consumed in this
	// turn (not wall-clock time). Messages that arrive DURING LLM generation
	// will land in a newer RC event past this cursor and correctly
	// trigger another round; wall-clock would wrongly mark them processed.
	if roundCursorPersisted {
		if consumedCursor.EventCursor > sess.lastProcessedCursor {
			sess.lastProcessedCursor = consumedCursor.EventCursor
		}
		sess.pendingCursor = nil
		return discussReplyComplete
	}
	if !d.advanceDiscussCursor(ctx, sess, cfg, consumedCursor, log) {
		return discussReplyCursorPending
	}
	return discussReplyComplete
}
