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
	RunConfig agentpkg.RunConfig
	ModelID   string // database UUID of the selected model
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

// DiscussStreamBroadcaster publishes stream events to local UI subscribers.
// Implemented by local.RouteHub.
type DiscussStreamBroadcaster interface {
	PublishEvent(routeKey string, event channel.StreamEvent)
}

// DiscussDriverDeps holds dependencies injected into the DiscussDriver.
type DiscussDriverDeps struct {
	Pipeline       *Pipeline
	EventStore     *EventStore
	Agent          *agentpkg.Agent
	MessageService messagepkg.Service
	Resolver       RunConfigResolver
	Broadcaster    DiscussStreamBroadcaster
	Logger         *slog.Logger
}

// DiscussSessionConfig holds per-session configuration for discuss mode.
type DiscussSessionConfig struct {
	BotID             string
	SessionID         string
	ChannelIdentityID string
	ReplyTarget       string
	CurrentPlatform   string
	ConversationType  string
	ConversationName  string
	SessionToken      string //nolint:gosec // session credential material
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

const discussIdleTimeout = 10 * time.Minute

func (d *DiscussDriver) runSession(ctx context.Context, sess *discussSession) {
	sessionID := sess.config.SessionID
	log := d.logger.With(slog.String("session_id", sessionID), slog.String("bot_id", sess.config.BotID))
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
	cfg := sess.config

	trs := d.loadTurnResponses(ctx, cfg.SessionID)

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

	now := time.Now()

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

	sess.lastProcessedMs = now.UnixMilli()
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

func (d *DiscussDriver) loadTurnResponses(ctx context.Context, sessionID string) []TurnResponseEntry {
	if d.deps.MessageService == nil {
		return nil
	}

	since := time.Now().UTC().Add(-24 * time.Hour)
	msgs, err := d.deps.MessageService.ListActiveSinceBySession(ctx, sessionID, since)
	if err != nil {
		d.logger.Warn("load TRs failed", slog.String("session_id", sessionID), slog.Any("error", err))
		return nil
	}

	var trs []TurnResponseEntry
	for _, m := range msgs {
		if m.Role != "assistant" && m.Role != "tool" {
			continue
		}
		var mm conversation.ModelMessage
		if err := json.Unmarshal(m.Content, &mm); err != nil {
			continue
		}
		contentStr := mm.TextContent()
		if contentStr == "" {
			continue
		}
		trs = append(trs, TurnResponseEntry{
			RequestedAtMs: m.CreatedAt.UnixMilli(),
			Role:          m.Role,
			Content:       contentStr,
		})
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
		sb.WriteString("\n\nYou were mentioned or replied to. You should respond by calling the `send` tool now.")
	}

	return sb.String()
}

func contextMessagesToSDK(messages []ContextMessage) []sdk.Message {
	result := make([]sdk.Message, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "user":
			result = append(result, sdk.UserMessage(m.Content))
		case "assistant":
			result = append(result, sdk.AssistantMessage(m.Content))
		default:
			result = append(result, sdk.UserMessage(m.Content))
		}
	}
	return result
}
