package discuss

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	agentevent "github.com/memohai/memoh/internal/agent/event"
	"github.com/memohai/memoh/internal/agent/turn"
	"github.com/memohai/memoh/internal/channel"
	messagepkg "github.com/memohai/memoh/internal/chat/message"
	sessionpkg "github.com/memohai/memoh/internal/chat/thread"
	"github.com/memohai/memoh/internal/chat/timeline"
)

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
	Pipeline       *timeline.Pipeline
	Turn           turn.Service
	MessageService messagepkg.Service
	CursorStore    DiscussCursorStore
	Broadcaster    DiscussStreamBroadcaster
	Logger         *slog.Logger
}

// DiscussSessionConfig holds per-thread configuration for discuss mode.
type DiscussSessionConfig struct {
	TeamID            string
	BotID             string
	ThreadID          string
	RouteID           string
	ChannelIdentityID string
	ReplyTarget       string
	CurrentPlatform   string
	ConversationType  string
	ConversationName  string
	SessionToken      string //nolint:gosec // session credential material
	ChatToken         string //nolint:gosec // scoped chat routing token
	ToolHTTPURL       string
}

// DiscussDriver manages discuss-mode thread workers. It is goroutine-safe.
type DiscussDriver struct {
	deps     DiscussDriverDeps
	mu       sync.Mutex
	sessions map[string]*discussSession
	logger   *slog.Logger
}

type discussSession struct {
	config          DiscussSessionConfig
	rcCh            chan timeline.RenderedContext
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
		logger:   logger.With(slog.String("service", "channel/discuss")),
	}
}

// SetTurnService sets the turn service after construction (breaks DI cycles).
func (d *DiscussDriver) SetTurnService(svc turn.Service) {
	d.deps.Turn = svc
}

// SetBroadcaster sets the stream broadcaster after construction so that
// discuss-mode agent events are forwarded to the Web UI in real time.
func (d *DiscussDriver) SetBroadcaster(b DiscussStreamBroadcaster) {
	d.deps.Broadcaster = b
}

// NotifyRC pushes a new timeline.RenderedContext to the discuss thread worker.
// If the worker goroutine is not running, it starts one.
func (d *DiscussDriver) NotifyRC(_ context.Context, sessionID string, rc timeline.RenderedContext, config DiscussSessionConfig) {
	d.mu.Lock()
	sess, ok := d.sessions[sessionID]
	if !ok {
		sessCtx, cancel := context.WithCancel(context.Background()) //nolint:gosec // G118: cancel is stored in sess.cancel
		sess = &discussSession{
			config: config,
			rcCh:   make(chan timeline.RenderedContext, 16),
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

// StopSession stops a discuss thread worker.
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

// HasSession returns true if a discuss thread worker is running.
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
	sessionID := initialConfig.ThreadID
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

	var latestRC timeline.RenderedContext

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

		if timeline.LatestExternalEventMs(latestRC, sess.lastProcessedMs) == 0 {
			continue
		}

		d.handleReply(ctx, sess, latestRC, log)
	}
}

func (d *DiscussDriver) handleReply(ctx context.Context, sess *discussSession, rc timeline.RenderedContext, log *slog.Logger) {
	d.handleReplyWithTurn(ctx, sess, rc, log, d.deps.Turn)
}

func (d *DiscussDriver) handleReplyWithTurn(ctx context.Context, sess *discussSession, rc timeline.RenderedContext, log *slog.Logger, turnSvc turn.Service) {
	cfg := d.sessionConfigSnapshot(sess)

	trs := d.loadTurnResponses(ctx, cfg.ThreadID)

	// Cold-start / post-idle initialisation: if we haven't processed anything
	// in this goroutine's lifetime yet, anchor `lastProcessedMs` to the most
	// recent TR's requested_at. Any RC segment strictly older than that has
	// already been "seen" by a prior LLM call (whose response is in the TR
	// stream), so it should not retrigger a reply. Without this anchor, every
	// idle-timeout restart would treat the entire session history as brand
	// new external traffic and re-answer it.
	if sess.lastProcessedMs == 0 {
		sess.lastProcessedMs = maxInt64(anchorFromTRs(trs), d.loadDiscussCursor(ctx, cfg, log))
	}

	// Re-evaluate the trigger condition now that lastProcessedMs is anchored.
	// The outer loop used lastProcessedMs=0 to allow first-time dispatch into
	// this function; after initialisation, we must verify there's actually a
	// new external event past the anchor before spending an LLM call.
	if timeline.LatestExternalEventMs(rc, sess.lastProcessedMs) == 0 {
		return
	}

	composed := timeline.ComposeContext(rc, trs, "")
	if composed == nil {
		return
	}

	log.Info("triggering discuss LLM call",
		slog.Int("messages", len(composed.Messages)),
		slog.Int("estimated_tokens", composed.EstimatedTokens))

	if turnSvc == nil {
		log.Error("discuss driver: turn service not configured")
		return
	}

	isMentioned := wasRecentlyMentioned(rc, sess.lastProcessedMs)
	// A direct/1:1 conversation is always "addressed" (matching the inbound
	// layer's isDirectedAtBot/shouldTriggerAssistantResponse), so a DM
	// discuss-ACP session must reply even without an explicit @-mention or
	// reply-to. Expensive external runtimes use this as a participation gate.
	addressed := isMentioned || channel.IsPrivateConversationType(cfg.ConversationType)
	consumedMs := latestRCReceivedAtMs(rc)

	msgs := make([]turn.DiscussMessage, 0, len(composed.Messages))
	for _, m := range composed.Messages {
		msgs = append(msgs, turn.DiscussMessage{Role: m.Role, Content: m.Content, RawContent: m.RawContent})
	}
	var imageRefs []turn.DiscussImageRef
	for _, r := range extractNewImageRefs(rc, sess.lastProcessedMs) {
		imageRefs = append(imageRefs, turn.DiscussImageRef{ContentHash: r.ContentHash, Mime: r.Mime})
	}

	handle, err := turnSvc.StartTurn(ctx, turn.StartTurnCommand{
		SchemaVersion:           1,
		TeamID:                  cfg.TeamID,
		Mode:                    turn.ModeDiscuss,
		BotID:                   cfg.BotID,
		ThreadID:                cfg.ThreadID,
		RouteID:                 cfg.RouteID,
		SourceChannelIdentityID: cfg.ChannelIdentityID,
		CurrentChannel:          cfg.CurrentPlatform,
		ReplyTarget:             cfg.ReplyTarget,
		ConversationType:        cfg.ConversationType,
		ConversationName:        cfg.ConversationName,
		SessionToken:            cfg.SessionToken,
		ChatToken:               cfg.ChatToken,
		ToolHTTPURL:             cfg.ToolHTTPURL,
		DiscussMessages:         msgs,
		DiscussImageRefs:        imageRefs,
		DiscussMentioned:        isMentioned,
		DiscussAddressed:        addressed,
	})
	if err != nil {
		log.Error("discuss: start turn failed", slog.Any("error", err))
		return
	}

	var (
		runtimeType string
		streamed    bool
		terminal    bool
		failed      bool
		skipped     bool
	)
	events, errsCh := handle.Events(), handle.Errs()
	for events != nil || errsCh != nil {
		select {
		case e, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			switch e.Kind {
			case turn.DiscussEventRunResolved:
				var payload turn.DiscussRunResolvedPayload
				if json.Unmarshal(e.Payload, &payload) == nil {
					runtimeType = payload.RuntimeType
				}
			case turn.DiscussEventSkipped:
				skipped = true
			default:
				var se agentevent.StreamEvent
				if decodeErr := json.Unmarshal(e.Payload, &se); decodeErr != nil {
					log.Warn("discuss: decode stream event failed", slog.Any("error", decodeErr))
					failed = true
					continue
				}
				streamed = true
				if se.Type == agentevent.Error {
					failed = true
					log.Error("discuss stream error", slog.String("error", se.Error))
				}
				if se.Type == agentevent.AgentEnd || se.Type == agentevent.AgentAbort {
					terminal = true
				}
				d.broadcastDiscussEvent(cfg.BotID, se)
			}
		case streamErr, ok := <-errsCh:
			if !ok {
				errsCh = nil
				continue
			}
			if streamErr != nil {
				log.Error("discuss turn failed", slog.Any("error", streamErr))
				failed = true
			}
		case <-ctx.Done():
			log.Warn("discuss turn cancelled", slog.Any("error", ctx.Err()))
			return
		}
	}

	if runtimeType == "" {
		// Run config never resolved; leave the cursor untouched so the same
		// batch retriggers on the next RC (pre-port semantics).
		return
	}

	// Cursor advance preserves the pre-port semantics: ACP runs advance when
	// the participation gate skipped them or after a clean terminal stream;
	// the cheap native runtime advances unconditionally so a failed LLM call
	// is not endlessly re-answered.
	if strings.TrimSpace(runtimeType) == sessionpkg.RuntimeACPAgent {
		if skipped || (streamed && terminal && !failed) {
			d.advanceDiscussCursor(ctx, sess, cfg, consumedMs, log)
		}
		return
	}
	d.advanceDiscussCursor(ctx, sess, cfg, consumedMs, log)
}

// latestRCReceivedAtMs returns the maximum ReceivedAtMs across all segments
// in the given RC, or 0 if the RC is empty.
func latestRCReceivedAtMs(rc timeline.RenderedContext) int64 {
	var latest int64
	for _, seg := range rc {
		if seg.ReceivedAtMs > latest {
			latest = seg.ReceivedAtMs
		}
	}
	return latest
}

func (d *DiscussDriver) loadDiscussCursor(ctx context.Context, cfg DiscussSessionConfig, log *slog.Logger) int64 {
	if d.deps.CursorStore == nil {
		return 0
	}
	cursor, err := d.deps.CursorStore.GetDiscussConsumedCursor(ctx, cfg.ThreadID, discussCursorScope(cfg))
	if err != nil {
		log.Warn("discuss cursor load failed", slog.Any("error", err))
		return 0
	}
	return cursor
}

func (d *DiscussDriver) advanceDiscussCursor(ctx context.Context, sess *discussSession, cfg DiscussSessionConfig, cursor int64, log *slog.Logger) {
	if cursor <= sess.lastProcessedMs {
		return
	}
	sess.lastProcessedMs = cursor
	if d.deps.CursorStore == nil {
		return
	}
	if err := d.deps.CursorStore.UpsertDiscussConsumedCursor(ctx,
		cfg.ThreadID,
		discussCursorScope(cfg),
		strings.TrimSpace(cfg.RouteID),
		strings.TrimSpace(cfg.CurrentPlatform),
		cursor,
	); err != nil {
		log.Warn("discuss cursor persist failed", slog.Any("error", err), slog.Int64("cursor", cursor))
	}
}

func discussCursorScope(cfg DiscussSessionConfig) string {
	if routeID := strings.TrimSpace(cfg.RouteID); routeID != "" {
		return "route:" + routeID
	}
	platform := strings.TrimSpace(cfg.CurrentPlatform)
	identityID := strings.TrimSpace(cfg.ChannelIdentityID)
	switch {
	case platform != "" && identityID != "":
		return "source:" + platform + ":" + identityID
	case platform != "":
		return "source:" + platform
	default:
		return "default"
	}
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// anchorFromTRs returns the maximum RequestedAtMs across a TR slice. Used
// by the cold-start initialisation of `lastProcessedMs` so that RC segments
// older than the latest persisted bot response are not re-answered.
func anchorFromTRs(trs []timeline.TurnResponseEntry) int64 {
	var latest int64
	for _, tr := range trs {
		if tr.RequestedAtMs > latest {
			latest = tr.RequestedAtMs
		}
	}
	return latest
}

// broadcastDiscussEvent forwards an agent stream event to the RouteHub so the
// Web UI can display thinking, tool calls, and text deltas in real time.
func (d *DiscussDriver) broadcastDiscussEvent(botID string, event agentevent.StreamEvent) {
	if d.deps.Broadcaster == nil {
		return
	}
	se, ok := agentEventToChannelEvent(event)
	if !ok {
		return
	}
	d.deps.Broadcaster.PublishEvent(botID, se)
}

func agentEventToChannelEvent(e agentevent.StreamEvent) (channel.StreamEvent, bool) {
	switch e.Type {
	case agentevent.AgentStart:
		return channel.StreamEvent{Type: channel.StreamEventAgentStart}, true
	case agentevent.TextStart:
		return channel.StreamEvent{Type: channel.StreamEventPhaseStart, Phase: channel.StreamPhaseText}, true
	case agentevent.TextDelta:
		return channel.StreamEvent{Type: channel.StreamEventDelta, Delta: e.Delta}, true
	case agentevent.TextEnd:
		return channel.StreamEvent{Type: channel.StreamEventPhaseEnd, Phase: channel.StreamPhaseText}, true
	case agentevent.ReasoningStart:
		return channel.StreamEvent{Type: channel.StreamEventPhaseStart, Phase: channel.StreamPhaseReasoning}, true
	case agentevent.ReasoningDelta:
		return channel.StreamEvent{Type: channel.StreamEventDelta, Delta: e.Delta, Phase: channel.StreamPhaseReasoning}, true
	case agentevent.ReasoningEnd:
		return channel.StreamEvent{Type: channel.StreamEventPhaseEnd, Phase: channel.StreamPhaseReasoning}, true
	case agentevent.ToolCallStart:
		return channel.StreamEvent{
			Type:     channel.StreamEventToolCallStart,
			ToolCall: &channel.StreamToolCall{Name: e.ToolName, CallID: e.ToolCallID, Input: e.Input},
		}, true
	case agentevent.ToolCallEnd:
		return channel.StreamEvent{
			Type:     channel.StreamEventToolCallEnd,
			ToolCall: &channel.StreamToolCall{Name: e.ToolName, CallID: e.ToolCallID, Input: e.Input, Result: e.Result},
		}, true
	case agentevent.ToolApprovalRequest:
		return channel.StreamEvent{
			Type: channel.StreamEventToolCallStart,
			ToolCall: &channel.StreamToolCall{
				Name:       strings.TrimSpace(e.ToolName),
				CallID:     strings.TrimSpace(e.ToolCallID),
				Input:      e.Input,
				ApprovalID: strings.TrimSpace(e.ApprovalID),
				ShortID:    e.ShortID,
				Actions: []channel.Action{
					{Type: "tool_approval", Label: "Approve", Value: "approve:" + strings.TrimSpace(e.ApprovalID)},
					{Type: "tool_approval", Label: "Reject", Value: "reject:" + strings.TrimSpace(e.ApprovalID)},
				},
			},
		}, true
	case agentevent.UserInputRequest:
		userInputID := strings.TrimSpace(e.UserInputID)
		if userInputID == "" {
			userInputID = strings.TrimSpace(e.ApprovalID)
		}
		return channel.StreamEvent{
			Type: channel.StreamEventToolCallStart,
			ToolCall: &channel.StreamToolCall{
				Name:   strings.TrimSpace(e.ToolName),
				CallID: strings.TrimSpace(e.ToolCallID),
				Input: map[string]any{
					"user_input_id": userInputID,
					"short_id":      e.ShortID,
					"status":        strings.TrimSpace(e.Status),
					"payload":       e.Input,
				},
				ShortID: e.ShortID,
				Actions: []channel.Action{
					{Type: "user_input", Label: "Respond", Value: "respond:" + userInputID},
				},
			},
		}, true
	case agentevent.AgentEnd:
		return channel.StreamEvent{Type: channel.StreamEventAgentEnd}, true
	case agentevent.AgentAbort:
		return channel.StreamEvent{Type: channel.StreamEventAgentEnd}, true
	case agentevent.Error:
		return channel.StreamEvent{Type: channel.StreamEventError, Error: e.Error}, true
	default:
		return channel.StreamEvent{}, false
	}
}

// loadTurnResponses loads every assistant/tool message ever persisted for
// this session and decodes them into TR entries. There is no time-based cut
// off on purpose: truncating TRs while RC is replayed in full from the events
// table creates an asymmetric context (user messages visible, the bot's own
// earlier replies missing) that confuses both the LLM and loop-detection.
// Any size-bound trimming should happen later via compaction, not here.
func (d *DiscussDriver) loadTurnResponses(ctx context.Context, sessionID string) []timeline.TurnResponseEntry {
	if d.deps.MessageService == nil {
		return nil
	}

	// time.Unix(0, 0) is the Unix epoch; the underlying query uses
	// `created_at >= $1`, so this effectively loads every session message.
	msgs, err := d.deps.MessageService.ListActiveSinceBySession(ctx, sessionID, time.Unix(0, 0).UTC())
	if err != nil {
		d.logger.Warn("load TRs failed", slog.String("session_id", sessionID), slog.Any("error", err))
		return nil
	}

	var trs []timeline.TurnResponseEntry
	for _, m := range msgs {
		entry, ok := timeline.DecodeTurnResponseEntry(m)
		if !ok {
			continue
		}
		trs = append(trs, entry)
	}
	return trs
}

// extractNewImageRefs collects timeline.ImageAttachmentRef entries from RC segments
// that arrived after afterMs (i.e. new since the last LLM call).
func extractNewImageRefs(rc timeline.RenderedContext, afterMs int64) []timeline.ImageAttachmentRef {
	var refs []timeline.ImageAttachmentRef
	for _, seg := range rc {
		if seg.ReceivedAtMs > afterMs && !seg.IsMyself {
			refs = append(refs, seg.ImageRefs...)
		}
	}
	return refs
}

func wasRecentlyMentioned(rc timeline.RenderedContext, afterMs int64) bool {
	for _, seg := range rc {
		if seg.ReceivedAtMs > afterMs && (seg.MentionsMe || seg.RepliesToMe) {
			return true
		}
	}
	return false
}
