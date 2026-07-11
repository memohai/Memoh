package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

// WSStreamEvent represents a raw JSON event forwarded from the agent.
type WSStreamEvent = json.RawMessage

// terminalSnapshot captures the partial state extracted from a terminal
// agent event. It is used both for the success-path persistence and for the
// interrupted-path fallback so that real partial messages get saved instead
// of a synthetic placeholder.
type terminalSnapshot struct {
	sdkMessages    []sdk.Message
	usage          json.RawMessage
	deferredToolID string
	aborted        bool
	visibleOutput  bool
}

func hasVisibleAgentStreamOutput(event agentpkg.StreamEvent) bool {
	switch event.Type {
	case agentpkg.EventTextDelta,
		agentpkg.EventReasoningDelta:
		return strings.TrimSpace(event.Delta) != ""
	case agentpkg.EventToolCallInputStart,
		agentpkg.EventToolCallStart,
		agentpkg.EventToolCallProgress,
		agentpkg.EventToolCallEnd,
		agentpkg.EventToolApprovalRequest,
		agentpkg.EventUserInputRequest,
		agentpkg.EventReaction,
		agentpkg.EventSpeech:
		return true
	case agentpkg.EventAttachment:
		return len(event.Attachments) > 0
	default:
		return false
	}
}

func shouldForwardAgentStreamEvent(rc resolvedContext, event agentpkg.StreamEvent) bool {
	return event.Type != agentpkg.EventError || rc.promptMaterializationError() == nil
}

func finishStreamPostPersist(
	ctx context.Context,
	rc resolvedContext,
	persisted []messagepkg.Message,
	postPersistApplied bool,
	postPersist func(context.Context, []messagepkg.Message) error,
) error {
	if promptErr := rc.promptMaterializationError(); promptErr != nil {
		return promptErr
	}
	if postPersist == nil || postPersistApplied {
		return nil
	}
	return postPersist(context.WithoutCancel(ctx), persisted)
}

// extractTerminalSnapshot decodes a terminal stream event payload into the
// raw SDK messages plus auxiliary metadata. Returns ok=false when the event
// has no usable messages.
func extractTerminalSnapshot(data []byte) (terminalSnapshot, bool) {
	var envelope struct {
		Type       string          `json:"type"`
		Messages   json.RawMessage `json:"messages"`
		Usage      json.RawMessage `json:"usage,omitempty"`
		ApprovalID string          `json:"approvalId,omitempty"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return terminalSnapshot{}, false
	}
	if len(envelope.Messages) == 0 {
		return terminalSnapshot{}, false
	}
	var sdkMsgs []sdk.Message
	if err := json.Unmarshal(envelope.Messages, &sdkMsgs); err != nil || len(sdkMsgs) == 0 {
		return terminalSnapshot{}, false
	}
	return terminalSnapshot{
		sdkMessages:    sdkMsgs,
		usage:          envelope.Usage,
		deferredToolID: strings.TrimSpace(envelope.ApprovalID),
		aborted:        envelope.Type == string(agentpkg.EventAgentAbort),
	}, true
}

// StreamChat runs a streaming chat via the internal agent.
func (r *Resolver) StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
	chunkCh := make(chan conversation.StreamChunk)
	errCh := make(chan error, 1)
	go func() {
		defer close(chunkCh)
		defer close(errCh)
		streamReq := req
		if streamReq.RawQuery == "" {
			streamReq.RawQuery = strings.TrimSpace(streamReq.Query)
		}
		if err := rejectReservedSkillMetadataIfPresent(streamReq); err != nil {
			errCh <- err
			return
		}
		if err := r.rejectRequestedSkillsIfUnsupportedContext(ctx, streamReq); err != nil {
			errCh <- err
			return
		}
		if ok, err := r.isACPAgentSession(ctx, streamReq); err != nil {
			r.logger.Error("StreamChat: ACP session check failed",
				slog.String("bot_id", streamReq.BotID),
				slog.String("session_id", streamReq.SessionID),
				slog.Any("error", err),
			)
			errCh <- err
			return
		} else if ok {
			r.streamACPAgentChunks(ctx, streamReq, chunkCh, errCh)
			return
		}

		doneTurn := r.enterSessionTurn(ctx, streamReq.BotID, streamReq.SessionID)
		defer doneTurn()

		if streamReq.RawQuery == "" {
			streamReq.RawQuery = strings.TrimSpace(streamReq.Query)
		}
		var err error
		if !streamReq.UserMessagePersisted {
			streamReq, err = r.applyUserMessageHook(ctx, streamReq)
			if err != nil {
				r.logger.Error("agent stream user message hook failed",
					slog.String("bot_id", streamReq.BotID),
					slog.String("chat_id", streamReq.ChatID),
					slog.Any("error", err),
				)
				errCh <- err
				return
			}
		}
		rc, err := r.resolve(ctx, streamReq)
		if err != nil {
			r.logger.Error("agent stream resolve failed",
				slog.String("bot_id", streamReq.BotID),
				slog.String("chat_id", streamReq.ChatID),
				slog.Any("error", err),
			)
			errCh <- err
			return
		}
		streamReq.Query = rc.query

		go r.maybeGenerateSessionTitle(context.WithoutCancel(ctx), streamReq, streamReq.RawQuery)

		cfg := rc.runConfig
		cfg.LiveToolStream = true
		cfg.CanRequestUserInput = r.canDeliverUserInputStream()
		cfg = r.prepareRunConfig(ctx, cfg)

		// Wrap with idle timeout: if no events arrive within the adaptive timeout, cancel the stream.
		idleCtx, idleCancel := withIdleTimeout(ctx)
		defer idleCancel.Stop()

		eventCh := r.agent.Stream(idleCtx, cfg)
		stored := false
		clientGone := false
		var lastSnapshot terminalSnapshot
		var hasSnapshot bool
		var toolCallCount int
		var hasVisibleOutput bool
		for event := range eventCh {
			idleCancel.Reset() // each event resets the idle timer

			// Track tool calls for adaptive idle timeout and progress events
			if event.Type == agentpkg.EventToolCallStart {
				toolCallCount++
				idleCancel.RecordToolCall()
			}

			if event.Type == agentpkg.EventError {
				r.logger.Error("agent stream error",
					slog.String("bot_id", streamReq.BotID),
					slog.String("chat_id", streamReq.ChatID),
					slog.String("model_id", rc.model.ID),
					slog.String("error", event.Error),
				)
			}
			if hasVisibleAgentStreamOutput(event) {
				hasVisibleOutput = true
			}

			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if event.IsTerminal() && len(event.Messages) > 0 {
				if snap, ok := extractTerminalSnapshot(data); ok {
					snap.visibleOutput = hasVisibleOutput
					lastSnapshot = snap
					hasSnapshot = true
					if !stored {
						// Use WithoutCancel so persistence still succeeds even
						// when the parent ctx has already been cancelled by a
						// client disconnect or idle timeout.
						if storeErr := r.persistTerminalSnapshot(context.WithoutCancel(ctx), streamReq, rc, snap); storeErr != nil {
							r.logger.Error("stream persist failed", slog.Any("error", storeErr))
						} else {
							stored = true
						}
					}
				}
			}

			// Forward to the client unless the client is already gone. Once
			// the client disconnects we keep draining eventCh so the agent
			// goroutine can finish and the terminal event (with partial
			// messages) is captured for persistence above.
			if !clientGone && shouldForwardAgentStreamEvent(rc, event) {
				select {
				case chunkCh <- conversation.StreamChunk(data):
				case <-ctx.Done():
					clientGone = true
				}
			}
		}

		// Intermediate persistence on abort/error: persist only concrete
		// partial assistant/tool state. Failed sends without a terminal
		// snapshot are treated as unsent so the Web UI can restore the draft
		// without polluting history.
		if !stored {
			var partialMessages []sdk.Message
			if hasSnapshot {
				partialMessages = lastSnapshot.sdkMessages
			}
			_ = r.persistPartialResult(ctx, streamReq, rc, partialMessages, toolCallCount, idleCancel.DidFire(), hasVisibleOutput)
		}

		if idleCancel.DidFire() {
			r.logger.Warn("agent stream aborted: idle timeout (no events from provider)",
				slog.String("bot_id", streamReq.BotID),
				slog.String("chat_id", streamReq.ChatID),
				slog.String("model_id", rc.model.ID),
				slog.Int("tool_calls", toolCallCount),
			)
			// Notify the client that the stream was terminated due to idle timeout.
			if !clientGone {
				timeoutEvent := agentpkg.StreamEvent{
					Type:  agentpkg.EventError,
					Error: fmt.Sprintf("stream timeout: no response from model provider (after %d tool calls)", toolCallCount),
				}
				if data, err := json.Marshal(timeoutEvent); err == nil {
					select {
					case chunkCh <- conversation.StreamChunk(data):
					case <-ctx.Done():
					}
				}
			}
		}

		if promptErr := rc.promptMaterializationError(); promptErr != nil {
			errCh <- promptErr
		}
	}()
	return chunkCh, errCh
}

// StreamChatWS resolves the agent context and streams agent events.
// Events are sent on eventCh. When abortCh is closed, the context is cancelled.
func (r *Resolver) StreamChatWS(
	ctx context.Context,
	req conversation.ChatRequest,
	eventCh chan<- WSStreamEvent,
	abortCh <-chan struct{},
) error {
	_, err := r.streamChatWSResult(ctx, req, eventCh, abortCh)
	return err
}

func (r *Resolver) streamChatWSResult(
	ctx context.Context,
	req conversation.ChatRequest,
	eventCh chan<- WSStreamEvent,
	abortCh <-chan struct{},
) ([]messagepkg.Message, error) {
	return r.streamChatWSResultWithHooks(ctx, req, eventCh, abortCh, nil, nil)
}

func (r *Resolver) streamChatWSResultWithHooks(
	ctx context.Context,
	req conversation.ChatRequest,
	eventCh chan<- WSStreamEvent,
	abortCh <-chan struct{},
	preflight func(context.Context) error,
	postPersist func(context.Context, []messagepkg.Message) error,
) ([]messagepkg.Message, error) {
	if err := rejectReservedSkillMetadataIfPresent(req); err != nil {
		return nil, err
	}
	if err := r.rejectRequestedSkillsIfUnsupportedContext(ctx, req); err != nil {
		return nil, err
	}
	if ok, err := r.isACPAgentSession(ctx, req); err != nil {
		r.logger.Error("StreamChatWS: ACP session check failed",
			slog.String("bot_id", req.BotID),
			slog.String("session_id", req.SessionID),
			slog.Any("error", err),
		)
		return nil, err
	} else if ok {
		return nil, r.streamACPAgentWS(ctx, req, eventCh, abortCh)
	}

	doneTurn := r.enterSessionTurn(ctx, req.BotID, req.SessionID)
	defer doneTurn()

	if preflight != nil {
		if err := preflight(ctx); err != nil {
			return nil, err
		}
	}

	if req.RawQuery == "" {
		req.RawQuery = strings.TrimSpace(req.Query)
	}
	var err error
	if !req.UserMessagePersisted && !req.ReusePersistedUserMessage {
		req, err = r.applyUserMessageHook(ctx, req)
		if err != nil {
			r.logger.Error("StreamChatWS: user message hook failed",
				slog.String("bot_id", req.BotID),
				slog.Any("error", err),
			)
			return nil, err
		}
	}
	rc, err := r.resolve(ctx, req)
	if err != nil {
		r.logger.Error("StreamChatWS: resolve failed",
			slog.String("bot_id", req.BotID),
			slog.Any("error", err),
		)
		return nil, fmt.Errorf("resolve: %w", err)
	}
	req.Query = rc.query

	go r.maybeGenerateSessionTitle(context.WithoutCancel(ctx), req, req.RawQuery)

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		select {
		case <-abortCh:
			cancel()
		case <-streamCtx.Done():
		}
	}()

	cfg := rc.runConfig
	cfg.LiveToolStream = true
	cfg.CanRequestUserInput = r.canDeliverUserInputWS(eventCh)
	cfg = r.prepareRunConfig(streamCtx, cfg)

	// Wrap with idle timeout: if no events arrive within the adaptive timeout, cancel the stream.
	idleCtx, idleCancel := withIdleTimeout(streamCtx)
	defer idleCancel.Stop()

	agentEventCh := r.agent.Stream(idleCtx, cfg)
	modelID := rc.model.ID
	stored := false
	clientGone := false
	var lastSnapshot terminalSnapshot
	var hasSnapshot bool
	var toolCallCount int
	var hasVisibleOutput bool
	var persistedMessages []messagepkg.Message
	postPersistApplied := false
	for event := range agentEventCh {
		idleCancel.Reset() // each event resets the idle timer

		// Track tool calls for adaptive idle timeout
		if event.Type == agentpkg.EventToolCallStart {
			toolCallCount++
			idleCancel.RecordToolCall()
		}

		if event.Type == agentpkg.EventError {
			r.logger.Error("agent stream error",
				slog.String("bot_id", req.BotID),
				slog.String("chat_id", req.ChatID),
				slog.String("model_id", modelID),
				slog.String("error", event.Error),
			)
		}
		if hasVisibleAgentStreamOutput(event) {
			hasVisibleOutput = true
		}

		data, err := json.Marshal(event)
		if err != nil {
			continue
		}

		if event.IsTerminal() && len(event.Messages) > 0 {
			if snap, ok := extractTerminalSnapshot(data); ok {
				snap.visibleOutput = hasVisibleOutput
				lastSnapshot = snap
				hasSnapshot = true
				if !stored {
					persisted, storeErr := r.persistTerminalSnapshotResult(context.WithoutCancel(ctx), req, rc, snap)
					if storeErr != nil {
						r.logger.Error("ws persist failed", slog.Any("error", storeErr))
					} else {
						persistedMessages = persisted
						stored = true
					}
				}
			}
		}

		if event.IsTerminal() && postPersist != nil && !postPersistApplied {
			if err := postPersist(context.WithoutCancel(ctx), persistedMessages); err != nil {
				return persistedMessages, err
			}
			postPersistApplied = true
		}

		if !clientGone && shouldForwardAgentStreamEvent(rc, event) {
			select {
			case eventCh <- json.RawMessage(data):
			case <-ctx.Done():
				clientGone = true
			}
		}
	}

	// Intermediate persistence on abort/error
	if !stored {
		var partialMessages []sdk.Message
		if hasSnapshot {
			partialMessages = lastSnapshot.sdkMessages
		}
		persistedMessages = r.persistPartialResult(ctx, req, rc, partialMessages, toolCallCount, idleCancel.DidFire(), hasVisibleOutput)
	}

	if idleCancel.DidFire() {
		r.logger.Warn("agent ws stream aborted: idle timeout (no events from provider)",
			slog.String("bot_id", req.BotID),
			slog.String("chat_id", req.ChatID),
			slog.String("model_id", modelID),
			slog.Int("tool_calls", toolCallCount),
		)
		// Notify the client that the stream was terminated due to idle timeout.
		if !clientGone {
			timeoutEvent := agentpkg.StreamEvent{
				Type:  agentpkg.EventError,
				Error: fmt.Sprintf("stream timeout: no response from model provider (after %d tool calls)", toolCallCount),
			}
			if data, err := json.Marshal(timeoutEvent); err == nil {
				select {
				case eventCh <- json.RawMessage(data):
				case <-ctx.Done():
				}
			}
		}
	}

	if err := finishStreamPostPersist(ctx, rc, persistedMessages, postPersistApplied, postPersist); err != nil {
		return persistedMessages, err
	}

	return persistedMessages, nil
}

// persistTerminalSnapshot stores the SDK messages produced by an agent run
// (or partial run) into bot history and evaluates raw history pressure.
func (r *Resolver) persistTerminalSnapshot(ctx context.Context, req conversation.ChatRequest, rc resolvedContext, snap terminalSnapshot) error {
	_, err := r.persistTerminalSnapshotResult(ctx, req, rc, snap)
	return err
}

func (r *Resolver) persistTerminalSnapshotResult(
	ctx context.Context,
	req conversation.ChatRequest,
	rc resolvedContext,
	snap terminalSnapshot,
) (persisted []messagepkg.Message, err error) {
	defer func() {
		if err != nil {
			return
		}
		providerInputTokens := extractInputTokensFromUsage(snap.usage)
		if pressure := rc.compactionPressure(providerInputTokens); pressure > 0 {
			go r.maybeCompact(context.WithoutCancel(ctx), req, rc, pressure)
		}
	}()

	outputMessages := sdkMessagesToModelMessages(snap.sdkMessages)
	if snap.aborted && !snap.visibleOutput {
		r.logger.Info("skip persisting aborted terminal snapshot before visible output",
			slog.String("bot_id", req.BotID),
			slog.String("chat_id", req.ChatID),
			slog.Int("messages", len(outputMessages)),
		)
		return nil, nil
	}
	if !hasPersistableAssistantOutput(outputMessages) {
		r.logger.Info("skip persisting terminal snapshot without assistant output",
			slog.String("bot_id", req.BotID),
			slog.String("chat_id", req.ChatID),
			slog.Int("messages", len(outputMessages)),
		)
		return nil, nil
	}

	storeReq := req
	if req.ReusePersistedUserMessage {
		storeReq.UserMessagePersisted = true
	}
	roundMessages := prependTurnUserMessage(storeReq, outputMessages)

	if rc.injectedRecords != nil && len(*rc.injectedRecords) > 0 {
		roundMessages = interleaveInjectedMessages(roundMessages, *rc.injectedRecords)
	}

	persisted, err = r.storeRoundWithOptionsResult(ctx, storeReq, roundMessages, rc.model.ID, storeRoundOptions{
		AllowPendingToolCalls: snap.deferredToolID != "",
	})
	if err != nil {
		return nil, err
	}

	return persisted, nil
}

func hasPersistableAssistantOutput(messages []conversation.ModelMessage) bool {
	for _, msg := range messages {
		if strings.EqualFold(strings.TrimSpace(msg.Role), "assistant") && !isEmptyAssistantMessage(msg) {
			return true
		}
	}
	return false
}

// persistPartialResult is the interrupt-path fallback. When the agent stream
// was interrupted (provider error, user abort, idle timeout) and partial SDK
// messages are available, those are persisted via the normal pipeline so
// orphaned tool_calls get repaired with synthetic error tool_results, keeping
// the conversation coherent for "ask the bot to continue".
//
// When no partial messages are available, failures are not persisted. The UI
// can show temporary errors without committing a user-only history row for a
// send that did not successfully produce an assistant turn.
func (r *Resolver) persistPartialResult(
	ctx context.Context,
	req conversation.ChatRequest,
	rc resolvedContext,
	partialMessages []sdk.Message,
	toolCallCount int,
	wasIdleTimeout bool,
	hasVisibleOutput bool,
) []messagepkg.Message {
	persistCtx := context.WithoutCancel(ctx)

	if len(partialMessages) > 0 {
		// AllowPendingToolCalls=false → repairToolCallClosures will inject
		// synthetic error tool_results for any tool_calls that never received
		// a real result, preserving the assistant ↔ tool pairing required by
		// downstream provider serializers (especially Anthropic).
		persisted, err := r.persistTerminalSnapshotResult(persistCtx, req, rc, terminalSnapshot{
			sdkMessages:   partialMessages,
			aborted:       !hasVisibleOutput,
			visibleOutput: hasVisibleOutput,
		})
		if err == nil {
			r.logger.Info("persisted partial agent result",
				slog.String("bot_id", req.BotID),
				slog.Int("tool_calls", toolCallCount),
				slog.Int("partial_messages", len(partialMessages)),
				slog.Bool("idle_timeout", wasIdleTimeout),
			)
			return persisted
		}
		r.logger.Error("failed to persist partial agent messages",
			slog.String("bot_id", req.BotID),
			slog.Any("error", err),
		)
	}

	r.logger.Info("skip persisting failed stream without terminal snapshot",
		slog.String("bot_id", req.BotID),
		slog.Int("tool_calls", toolCallCount),
		slog.Bool("idle_timeout", wasIdleTimeout),
		slog.Bool("visible_output", hasVisibleOutput),
	)

	// Trigger compaction on the failure path so oversized raw history cannot
	// deadlock a session before the provider emits a persistable snapshot.
	if pressure := rc.compactionPressure(0); pressure > 0 {
		r.maybeCompact(persistCtx, req, rc, pressure)
	}
	return nil
}

// interleaveInjectedMessages inserts injected user messages at their correct
// positions within the round. Each record's InsertAfter value indicates how
// many output messages preceded the injection.
//
// round layout: [user_A, output_0, output_1, ..., output_N]
// InsertAfter=K → insert after round[K] (i.e. after the K-th output message).
func interleaveInjectedMessages(round []conversation.ModelMessage, injections []conversation.InjectedMessageRecord) []conversation.ModelMessage {
	if len(injections) == 0 {
		return round
	}
	result := make([]conversation.ModelMessage, 0, len(round)+len(injections))
	injIdx := 0
	for i, msg := range round {
		result = append(result, msg)
		for injIdx < len(injections) && injections[injIdx].InsertAfter == i {
			result = append(result, conversation.ModelMessage{
				Role:    "user",
				Content: conversation.NewTextContent(injections[injIdx].HeaderifiedText),
			})
			injIdx++
		}
	}
	for ; injIdx < len(injections); injIdx++ {
		result = append(result, conversation.ModelMessage{
			Role:    "user",
			Content: conversation.NewTextContent(injections[injIdx].HeaderifiedText),
		})
	}
	return result
}

func extractInputTokensFromUsage(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var u struct {
		InputTokens int `json:"inputTokens"`
	}
	if json.Unmarshal(raw, &u) != nil {
		return 0
	}
	return u.InputTokens
}
