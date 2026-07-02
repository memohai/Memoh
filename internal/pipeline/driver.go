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
	messagepkg "github.com/memohai/memoh/internal/message"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

// ResolveRunConfigResult holds the output of ResolveRunConfig.
type ResolveRunConfigResult struct {
	RunConfig   agentpkg.RunConfig
	ModelID     string // database UUID of the selected model
	RuntimeType string
}

// RunConfigResolver resolves a complete agent RunConfig and persists output
// rounds. Implemented by flow.Resolver.
type RunConfigResolver interface {
	ResolveRunConfig(ctx context.Context, botID, sessionID, channelIdentityID, currentPlatform, replyTarget, conversationType, chatToken string) (ResolveRunConfigResult, error)
	InlineImageAttachments(ctx context.Context, botID string, refs []ImageAttachmentRef) []sdk.ImagePart
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
	MessageService  messagepkg.Service
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

// SetResolver sets the RunConfigResolver after construction (breaks DI cycles).
func (d *DiscussDriver) SetResolver(r RunConfigResolver) {
	d.deps.Resolver = r
}

func (d *DiscussDriver) SetRuntimeStreamer(r discussRuntimeStreamer) {
	d.deps.RuntimeStreamer = r
}

// SetBroadcaster sets the stream broadcaster after construction so that
// discuss-mode agent events are forwarded to the Web UI in real time.
func (d *DiscussDriver) SetBroadcaster(b DiscussStreamBroadcaster) {
	d.deps.Broadcaster = b
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

	trs := d.loadTurnResponses(ctx, cfg.SessionID)

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
	if LatestExternalEventMs(rc, sess.lastProcessedMs) == 0 {
		return
	}

	composed := ComposeContext(rc, trs, "")
	if composed == nil {
		return
	}

	log.Info("triggering discuss LLM call",
		slog.Int("messages", len(composed.Messages)),
		slog.Int("estimated_tokens", composed.EstimatedTokens))

	if d.deps.Resolver == nil {
		log.Error("discuss driver: resolver not configured")
		return
	}
	resolved, err := d.deps.Resolver.ResolveRunConfig(ctx,
		cfg.BotID, cfg.SessionID, cfg.ChannelIdentityID,
		cfg.CurrentPlatform, cfg.ReplyTarget, cfg.ConversationType, cfg.SessionToken)
	if err != nil {
		log.Error("discuss: resolve run config failed", slog.Any("error", err))
		return
	}
	if strings.TrimSpace(resolved.RuntimeType) == sessionpkg.RuntimeACPAgent {
		isMentioned := wasRecentlyMentioned(rc, sess.lastProcessedMs)
		// A direct/1:1 conversation is always "addressed" (matching the inbound
		// layer's isDirectedAtBot/shouldTriggerAssistantResponse), so a DM
		// discuss-ACP session must reply even without an explicit @-mention or
		// reply-to. Without this, a `/new discuss codex` session in a DM would
		// be permanently silent.
		addressed := isMentioned || channel.IsPrivateConversationType(cfg.ConversationType)
		consumedMs := latestRCReceivedAtMs(rc)
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
		if d.streamDiscussACPRuntime(ctx, cfg, composed, addressed, log) {
			d.advanceDiscussCursor(ctx, sess, cfg, consumedMs, log)
		}
		return
	}
	runConfig := resolved.RunConfig

	runConfig.Messages = contextMessagesToSDK(composed.Messages)
	runConfig.SessionType = sessionpkg.TypeDiscuss
	runConfig.Query = ""

	// Inline image attachments from new RC segments so the model receives
	// them as native vision input (ImagePart) on the first encounter.
	// Subsequent turns only see the file path in the XML rendering.
	if runConfig.SupportsImageInput && d.deps.Resolver != nil {
		imageRefs := extractNewImageRefs(rc, sess.lastProcessedMs)
		if len(imageRefs) > 0 {
			imageParts := d.deps.Resolver.InlineImageAttachments(ctx, cfg.BotID, imageRefs)
			injectImagePartsIntoLastUserMessage(runConfig.Messages, imageParts)
		}
	}

	isMentioned := wasRecentlyMentioned(rc, sess.lastProcessedMs)
	lateBinding := buildLateBindingPrompt(isMentioned)
	runConfig.Messages = append(runConfig.Messages, sdk.UserMessage(lateBinding))
	runConfig = runConfig.RefreshContextFrag()

	eventCh := agent.Stream(ctx, runConfig)

	var finalMessages json.RawMessage
	for event := range eventCh {
		d.broadcastDiscussEvent(cfg.BotID, event)

		switch event.Type {
		case agentpkg.EventError:
			log.Error("discuss stream error", slog.String("error", event.Error))
		case agentpkg.EventAgentEnd, agentpkg.EventAgentAbort:
			finalMessages = event.Messages
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

	// Advance the cursor to the latest RC segment actually consumed in this
	// turn (not wall-clock time). Messages that arrive DURING LLM generation
	// will land in a newer RC with ReceivedAtMs > this cursor and correctly
	// trigger another round; wall-clock would wrongly mark them processed.
	consumedMs := latestRCReceivedAtMs(rc)
	d.advanceDiscussCursor(ctx, sess, cfg, consumedMs, log)
}

func (d *DiscussDriver) streamDiscussACPRuntime(ctx context.Context, cfg DiscussSessionConfig, composed *ComposeContextResult, isMentioned bool, log *slog.Logger) bool {
	if d.deps.RuntimeStreamer == nil {
		log.Error("discuss ACP runtime: streamer not configured")
		return false
	}
	prompt := discussACPFullContextPrompt(composed.Messages, buildLateBindingPrompt(isMentioned))
	if strings.TrimSpace(prompt) == "" {
		return false
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
			return false
		}
	}
	return streamed && terminal && !failed
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

// latestRCReceivedAtMs returns the maximum ReceivedAtMs across all segments
// in the given RC, or 0 if the RC is empty.
func latestRCReceivedAtMs(rc RenderedContext) int64 {
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
	cursor, err := d.deps.CursorStore.GetDiscussConsumedCursor(ctx, cfg.SessionID, discussCursorScope(cfg))
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
		cfg.SessionID,
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
func anchorFromTRs(trs []TurnResponseEntry) int64 {
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
func (d *DiscussDriver) broadcastDiscussEvent(botID string, event agentpkg.StreamEvent) {
	if d.deps.Broadcaster == nil {
		return
	}
	se, ok := agentEventToChannelEvent(event)
	if !ok {
		return
	}
	d.deps.Broadcaster.PublishEvent(botID, se)
}

func agentEventToChannelEvent(e agentpkg.StreamEvent) (channel.StreamEvent, bool) {
	switch e.Type {
	case agentpkg.EventAgentStart:
		return channel.StreamEvent{Type: channel.StreamEventAgentStart}, true
	case agentpkg.EventTextStart:
		return channel.StreamEvent{Type: channel.StreamEventPhaseStart, Phase: channel.StreamPhaseText}, true
	case agentpkg.EventTextDelta:
		return channel.StreamEvent{Type: channel.StreamEventDelta, Delta: e.Delta}, true
	case agentpkg.EventTextEnd:
		return channel.StreamEvent{Type: channel.StreamEventPhaseEnd, Phase: channel.StreamPhaseText}, true
	case agentpkg.EventReasoningStart:
		return channel.StreamEvent{Type: channel.StreamEventPhaseStart, Phase: channel.StreamPhaseReasoning}, true
	case agentpkg.EventReasoningDelta:
		return channel.StreamEvent{Type: channel.StreamEventDelta, Delta: e.Delta, Phase: channel.StreamPhaseReasoning}, true
	case agentpkg.EventReasoningEnd:
		return channel.StreamEvent{Type: channel.StreamEventPhaseEnd, Phase: channel.StreamPhaseReasoning}, true
	case agentpkg.EventToolCallStart:
		return channel.StreamEvent{
			Type:     channel.StreamEventToolCallStart,
			ToolCall: &channel.StreamToolCall{Name: e.ToolName, CallID: e.ToolCallID, Input: e.Input},
		}, true
	case agentpkg.EventToolCallEnd:
		return channel.StreamEvent{
			Type:     channel.StreamEventToolCallEnd,
			ToolCall: &channel.StreamToolCall{Name: e.ToolName, CallID: e.ToolCallID, Input: e.Input, Result: e.Result},
		}, true
	case agentpkg.EventToolApprovalRequest:
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
	case agentpkg.EventUserInputRequest:
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
	case agentpkg.EventAgentEnd:
		return channel.StreamEvent{Type: channel.StreamEventAgentEnd}, true
	case agentpkg.EventAgentAbort:
		return channel.StreamEvent{Type: channel.StreamEventAgentEnd}, true
	case agentpkg.EventError:
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
func (d *DiscussDriver) loadTurnResponses(ctx context.Context, sessionID string) []TurnResponseEntry {
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

	var trs []TurnResponseEntry
	for _, m := range msgs {
		entry, ok := DecodeTurnResponseEntry(m)
		if !ok {
			continue
		}
		trs = append(trs, entry)
	}
	return trs
}

// extractNewImageRefs collects ImageAttachmentRef entries from RC segments
// that arrived after afterMs (i.e. new since the last LLM call).
func extractNewImageRefs(rc RenderedContext, afterMs int64) []ImageAttachmentRef {
	var refs []ImageAttachmentRef
	for _, seg := range rc {
		if seg.ReceivedAtMs > afterMs && !seg.IsMyself {
			refs = append(refs, seg.ImageRefs...)
		}
	}
	return refs
}

// injectImagePartsIntoLastUserMessage appends ImageParts to the last user
// message in msgs so the model receives inline vision input.
func injectImagePartsIntoLastUserMessage(msgs []sdk.Message, parts []sdk.ImagePart) {
	if len(parts) == 0 {
		return
	}
	extra := make([]sdk.MessagePart, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p.Image) != "" {
			extra = append(extra, p)
		}
	}
	if len(extra) == 0 {
		return
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == sdk.MessageRoleUser {
			msgs[i].Content = append(msgs[i].Content, extra...)
			return
		}
	}
}

func wasRecentlyMentioned(rc RenderedContext, afterMs int64) bool {
	for _, seg := range rc {
		if seg.ReceivedAtMs > afterMs && (seg.MentionsMe || seg.RepliesToMe) {
			return true
		}
	}
	return false
}

func buildLateBindingPrompt(isMentioned bool) string {
	now := time.Now().Format(time.RFC3339)
	var sb strings.Builder
	sb.WriteString("Current time: ")
	sb.WriteString(now)
	sb.WriteString("\n\n")
	sb.WriteString("IMPORTANT: You MUST use the `send` tool to speak. Your text output is invisible to everyone — it is only internal monologue. ")
	sb.WriteString("If you want to say something, you MUST call the `send` tool. Writing text without a tool call means absolute silence — no one will see it.")

	if isMentioned {
		sb.WriteString("\n\nYou are being addressed directly. You should respond by calling the `send` tool now.")
	}

	return sb.String()
}

func contextMessagesToSDK(messages []ContextMessage) []sdk.Message {
	result := make([]sdk.Message, 0, len(messages))
	for _, m := range messages {
		if len(m.RawContent) > 0 {
			raw, err := json.Marshal(struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			}{
				Role:    m.Role,
				Content: m.RawContent,
			})
			if err == nil {
				var msg sdk.Message
				if json.Unmarshal(raw, &msg) == nil {
					result = append(result, msg)
					continue
				}
			}
		}
		switch m.Role {
		case "user":
			result = append(result, sdk.UserMessage(m.Content))
		case "assistant":
			result = append(result, sdk.AssistantMessage(m.Content))
		case "tool":
			result = append(result, sdk.UserMessage(m.Content))
		default:
			result = append(result, sdk.UserMessage(m.Content))
		}
	}
	return result
}
