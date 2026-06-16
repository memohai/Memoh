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
	"github.com/memohai/memoh/internal/userinput"
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
	deferredKind   string
}

func isVisibleAgentStreamEvent(event agentpkg.StreamEvent) bool {
	switch event.Type {
	case agentpkg.EventTextStart,
		agentpkg.EventTextDelta,
		agentpkg.EventTextEnd,
		agentpkg.EventReasoningStart,
		agentpkg.EventReasoningDelta,
		agentpkg.EventReasoningEnd,
		agentpkg.EventToolCallInputStart,
		agentpkg.EventToolCallStart,
		agentpkg.EventToolCallProgress,
		agentpkg.EventToolCallEnd,
		agentpkg.EventToolApprovalRequest,
		agentpkg.EventUserInputRequest,
		agentpkg.EventAttachment,
		agentpkg.EventReaction,
		agentpkg.EventSpeech:
		return true
	default:
		return false
	}
}

// extractTerminalSnapshot decodes a terminal stream event payload into the
// raw SDK messages plus auxiliary metadata. Returns ok=false when the event
// has no usable messages.
func extractTerminalSnapshot(data []byte) (terminalSnapshot, bool) {
	var envelope struct {
		Type        string          `json:"type"`
		Messages    json.RawMessage `json:"messages"`
		Usage       json.RawMessage `json:"usage,omitempty"`
		ApprovalID  string          `json:"approvalId,omitempty"`
		UserInputID string          `json:"userInputId,omitempty"`
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
		deferredToolID: strings.TrimSpace(firstNonEmpty(envelope.UserInputID, envelope.ApprovalID)),
		deferredKind:   deferredKindFromTerminal(envelope.UserInputID, envelope.ApprovalID),
	}, true
}

func deferredKindFromTerminal(userInputID, approvalID string) string {
	if strings.TrimSpace(userInputID) != "" {
		return userinput.DeferredKind
	}
	if strings.TrimSpace(approvalID) != "" {
		return "tool_approval"
	}
	return ""
}

// StreamChat runs a streaming chat via the internal agent.
func (r *Resolver) StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
	chunkCh := make(chan conversation.StreamChunk)
	errCh := make(chan error, 1)
	go func() {
		defer close(chunkCh)
		defer close(errCh)
		streamReq := req
		doneTurn := r.enterSessionTurn(ctx, streamReq.BotID, streamReq.SessionID)
		defer doneTurn()
		var err error
		streamReq, err = r.pinPersistTurn(ctx, streamReq)
		if err != nil {
			errCh <- err
			return
		}
		defer r.cleanupEmptyPersistTurn(context.WithoutCancel(ctx), streamReq)

		if streamReq.RawQuery == "" {
			streamReq.RawQuery = strings.TrimSpace(streamReq.Query)
		}
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

		go r.maybeGenerateSessionTitle(context.WithoutCancel(ctx), streamReq, streamReq.Query)

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
			if isVisibleAgentStreamEvent(event) {
				hasVisibleOutput = true
			}

			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if event.IsTerminal() && len(event.Messages) > 0 {
				if snap, ok := extractTerminalSnapshot(data); ok {
					lastSnapshot = snap
					hasSnapshot = true
					if !stored {
						// Use WithoutCancel so persistence still succeeds even
						// when the parent ctx has already been cancelled by a
						// client disconnect or idle timeout.
						if didStore, storeErr := r.persistTerminalSnapshot(context.WithoutCancel(ctx), streamReq, rc, snap); storeErr != nil {
							r.logger.Error("stream persist failed", slog.Any("error", storeErr))
						} else if didStore {
							stored = true
						}
					}
				}
			}

			// Forward to the client unless the client is already gone. Once
			// the client disconnects we keep draining eventCh so the agent
			// goroutine can finish and the terminal event (with partial
			// messages) is captured for persistence above.
			if !clientGone {
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
			switch {
			case hasSnapshot:
				r.persistPartialResult(ctx, streamReq, rc, lastSnapshot.sdkMessages, toolCallCount, idleCancel.DidFire(), hasVisibleOutput)
			default:
				r.logger.Info("skip persisting failed startup stream",
					slog.String("bot_id", streamReq.BotID),
					slog.String("chat_id", streamReq.ChatID),
				)
			}
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
	if ok, err := r.isACPAgentSession(ctx, req); err != nil {
		r.logger.Error("StreamChatWS: ACP session check failed",
			slog.String("bot_id", req.BotID),
			slog.String("session_id", req.SessionID),
			slog.Any("error", err),
		)
		return err
	} else if ok {
		return r.streamACPAgentWS(ctx, req, eventCh, abortCh)
	}

	doneTurn := r.enterSessionTurn(ctx, req.BotID, req.SessionID)
	defer doneTurn()
	var err error
	req, err = r.pinPersistTurn(ctx, req)
	if err != nil {
		return err
	}
	defer r.cleanupEmptyPersistTurn(context.WithoutCancel(ctx), req)

	if req.RawQuery == "" {
		req.RawQuery = strings.TrimSpace(req.Query)
	}
	req, err = r.applyUserMessageHook(ctx, req)
	if err != nil {
		r.logger.Error("StreamChatWS: user message hook failed",
			slog.String("bot_id", req.BotID),
			slog.Any("error", err),
		)
		return err
	}
	rc, err := r.resolve(ctx, req)
	if err != nil {
		r.logger.Error("StreamChatWS: resolve failed",
			slog.String("bot_id", req.BotID),
			slog.Any("error", err),
		)
		return fmt.Errorf("resolve: %w", err)
	}
	req.Query = rc.query

	go r.maybeGenerateSessionTitle(context.WithoutCancel(ctx), req, req.Query)

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
		if isVisibleAgentStreamEvent(event) {
			hasVisibleOutput = true
		}

		data, err := json.Marshal(event)
		if err != nil {
			continue
		}

		if event.IsTerminal() && len(event.Messages) > 0 {
			if snap, ok := extractTerminalSnapshot(data); ok {
				lastSnapshot = snap
				hasSnapshot = true
				if !stored {
					if didStore, storeErr := r.persistTerminalSnapshot(context.WithoutCancel(ctx), req, rc, snap); storeErr != nil {
						r.logger.Error("ws persist failed", slog.Any("error", storeErr))
					} else if didStore {
						stored = true
					}
				}
			}
		}

		if !clientGone {
			select {
			case eventCh <- json.RawMessage(data):
			case <-ctx.Done():
				clientGone = true
			}
		}
	}

	// Intermediate persistence on abort/error
	if !stored {
		switch {
		case hasSnapshot:
			r.persistPartialResult(ctx, req, rc, lastSnapshot.sdkMessages, toolCallCount, idleCancel.DidFire(), hasVisibleOutput)
		default:
			r.logger.Info("skip persisting failed startup ws stream",
				slog.String("bot_id", req.BotID),
				slog.String("chat_id", req.ChatID),
			)
		}
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

	return nil
}

// persistTerminalSnapshot stores the SDK messages produced by an agent run
// (or partial run) into bot history. Triggers compaction when usage data
// indicates the context is large.
func (r *Resolver) persistTerminalSnapshot(ctx context.Context, req conversation.ChatRequest, rc resolvedContext, snap terminalSnapshot) (bool, error) {
	outputMessages := sdkMessagesToModelMessages(snap.sdkMessages)
	if !hasPersistableAssistantOutput(outputMessages) {
		r.logger.Info("skip persisting terminal snapshot without assistant output",
			slog.String("bot_id", req.BotID),
			slog.String("chat_id", req.ChatID),
			slog.Int("messages", len(outputMessages)),
		)
		return false, nil
	}

	roundMessages := prependUserMessage(req.Query, outputMessages)

	if rc.injectedRecords != nil && len(*rc.injectedRecords) > 0 {
		roundMessages = interleaveInjectedMessages(roundMessages, *rc.injectedRecords)
	}

	stored, err := r.storeRoundWithContext(ctx, req, roundMessages, rc.model.ID, storeRoundOptions{
		AllowPendingToolCalls: snap.deferredToolID != "",
	})
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(stored.TurnID) == "" {
		return false, nil
	}
	r.updateDeferredPersistContext(ctx, snap, stored)

	if inputTokens := extractInputTokensFromUsage(snap.usage); inputTokens > 0 {
		go r.maybeCompact(context.WithoutCancel(ctx), req, rc, inputTokens)
	}

	return true, nil
}

func (r *Resolver) updateDeferredPersistContext(ctx context.Context, snap terminalSnapshot, stored storedRoundContext) {
	id := strings.TrimSpace(snap.deferredToolID)
	branchID := strings.TrimSpace(stored.BranchID)
	turnID := strings.TrimSpace(stored.TurnID)
	if id == "" || branchID == "" {
		return
	}
	switch snap.deferredKind {
	case userinput.DeferredKind:
		if svc, ok := r.userInput.(interface {
			UpdatePersistContext(context.Context, string, string, string) (userinput.Request, error)
		}); ok {
			if _, err := svc.UpdatePersistContext(ctx, id, branchID, turnID); err != nil && r.logger != nil {
				r.logger.Warn("update user input persist context failed", slog.String("request_id", id), slog.Any("error", err))
			}
		}
	default:
		if r.toolApproval != nil {
			if _, err := r.toolApproval.UpdatePersistContext(ctx, id, branchID, turnID); err != nil && r.logger != nil {
				r.logger.Warn("update tool approval persist context failed", slog.String("approval_id", id), slog.Any("error", err))
			}
		}
	}
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
) {
	persistCtx := context.WithoutCancel(ctx)

	if len(partialMessages) > 0 {
		// AllowPendingToolCalls=false → repairToolCallClosures will inject
		// synthetic error tool_results for any tool_calls that never received
		// a real result, preserving the assistant ↔ tool pairing required by
		// downstream provider serializers (especially Anthropic).
		didStore, err := r.persistTerminalSnapshot(persistCtx, req, rc, terminalSnapshot{
			sdkMessages: partialMessages,
		})
		if err == nil && didStore {
			r.logger.Info("persisted partial agent result",
				slog.String("bot_id", req.BotID),
				slog.Int("tool_calls", toolCallCount),
				slog.Int("partial_messages", len(partialMessages)),
				slog.Bool("idle_timeout", wasIdleTimeout),
			)
			// Trigger compaction on the failure path so that oversized
			// contexts don't deadlock (where the LLM can never succeed and
			// therefore compaction never fires).
			if rc.estimatedTokens > 0 {
				r.maybeCompact(persistCtx, req, rc, rc.estimatedTokens)
			}
			return
		}
		if err == nil {
			return
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

	if rc.estimatedTokens > 0 {
		r.maybeCompact(persistCtx, req, rc, rc.estimatedTokens)
	}
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
