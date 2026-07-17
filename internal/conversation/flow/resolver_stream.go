package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	dbstore "github.com/memohai/memoh/internal/db/store"
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
			if err := rejectACPWorkspaceTarget(streamReq); err != nil {
				errCh <- err
				return
			}
			r.streamACPAgentChunks(ctx, streamReq, chunkCh, errCh)
			return
		}
		streamCtx, preparedReq, prepareErr := r.prepareWorkspaceRequest(ctx, streamReq)
		if prepareErr != nil {
			errCh <- prepareErr
			return
		}
		streamReq = preparedReq

		doneTurn := r.enterSessionTurn(streamCtx, streamReq.BotID, streamReq.SessionID)
		defer doneTurn()

		if streamReq.RawQuery == "" {
			streamReq.RawQuery = strings.TrimSpace(streamReq.Query)
		}
		var err error
		if !streamReq.UserMessagePersisted {
			streamReq, err = r.applyUserMessageHook(streamCtx, streamReq)
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
		rc, err := r.resolve(streamCtx, streamReq)
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

		go r.maybeGenerateSessionTitle(context.WithoutCancel(streamCtx), streamReq, streamReq.RawQuery)

		cfg := rc.runConfig
		cfg.LiveToolStream = true
		cfg.CanRequestUserInput = r.canDeliverUserInputStream()
		cfg = r.prepareRunConfig(streamCtx, cfg)

		// Wrap with idle timeout: if no events arrive within the adaptive timeout, cancel the stream.
		idleCtx, idleCancel := withIdleTimeout(streamCtx, r.idleTimeoutOptions)
		defer idleCancel.Stop()

		eventCh := r.agent.Stream(idleCtx, cfg)
		stored := false
		clientGone := false
		var lastSnapshot terminalSnapshot
		var hasSnapshot bool
		var toolCallCount int
		var hasVisibleOutput bool
		var persistenceErr error
		var gate userInputStreamGate
		forwardChunk := func(chunk conversation.StreamChunk) {
			if clientGone || streamCtx.Err() != nil {
				clientGone = true
				return
			}
			select {
			case chunkCh <- chunk:
			case <-streamCtx.Done():
				clientGone = true
			}
		}
		for event := range eventCh {
			if event.IsTerminal() {
				idleCancel.Finish()
			} else {
				idleCancel.Reset()
			}

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
			held := gate.hold(event, data)
			if event.IsTerminal() && len(event.Messages) > 0 {
				if snap, ok := extractTerminalSnapshot(data); ok {
					snap.visibleOutput = hasVisibleOutput
					lastSnapshot = snap
					hasSnapshot = true
					if !stored {
						// Use WithoutCancel so persistence still succeeds even
						// when the parent ctx has already been cancelled by a
						// client disconnect or idle timeout.
						_, snapshotStored, storeErr := r.persistTerminalSnapshotWithStatus(
							context.WithoutCancel(streamCtx),
							streamReq,
							rc,
							snap,
						)
						stored = snapshotStored
						if storeErr != nil {
							if !stored {
								persistenceErr = storeErr
							}
							r.logger.Error("stream persist failed", slog.Any("error", storeErr))
						}
					}
				}
			}
			if held {
				continue
			}

			// Forward to the client unless the client is already gone. Once
			// the client disconnects we keep draining eventCh so the agent
			// goroutine can finish and the terminal event (with partial
			// messages) is captured for persistence above.
			forwardChunk(conversation.StreamChunk(data))
		}

		// Intermediate persistence on abort/error: persist only concrete
		// partial assistant/tool state. Failed sends without a terminal
		// snapshot are treated as unsent so the Web UI can restore the draft
		// without polluting history.
		if !stored {
			switch {
			case hasSnapshot && (persistenceErr == nil || dbstore.IsPersistenceRetrySafe(persistenceErr)):
				var persisted []messagepkg.Message
				persisted, persistenceErr = r.persistPartialResult(streamCtx, streamReq, rc, lastSnapshot, toolCallCount, idleCancel.DidFire(), hasVisibleOutput)
				stored = len(persisted) > 0
				if stored {
					persistenceErr = nil
				}
			default:
				r.logger.Info("skip persisting failed startup stream",
					slog.String("bot_id", streamReq.BotID),
					slog.String("chat_id", streamReq.ChatID),
				)
			}
		}
		if stored {
			notifyInjectedPersistence(rc.injectedRecords, nil)
		} else {
			injectErr := persistenceErr
			if injectErr == nil {
				injectErr = errors.New("agent stream ended before injected messages were persisted")
			}
			notifyInjectedPersistence(rc.injectedRecords, injectErr)
		}
		idleFired := idleCancel.DidFire()
		if gate.active {
			if stored {
				if !idleFired {
					_ = gate.release(func(data json.RawMessage) error {
						forwardChunk(data)
						return nil
					})
				}
			} else if persistenceErr == nil && !idleFired {
				persistenceErr = errors.New("ask_user terminal response was not persisted")
			}
		}

		var idleErr error
		if idleFired {
			idleErr = fmt.Errorf("stream timeout: no response from model provider (after %d tool calls)", toolCallCount)
			r.logger.Warn("agent stream aborted: idle timeout (no events from provider)",
				slog.String("bot_id", streamReq.BotID),
				slog.String("chat_id", streamReq.ChatID),
				slog.String("model_id", rc.model.ID),
				slog.Int("tool_calls", toolCallCount),
			)
		}
		if persistenceErr != nil {
			errCh <- fmt.Errorf("persist model stream result: %w", persistenceErr)
		} else if idleErr != nil {
			errCh <- idleErr
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
		if err := rejectACPWorkspaceTarget(req); err != nil {
			return nil, err
		}
		return nil, r.streamACPAgentWS(ctx, req, eventCh, abortCh)
	}
	var prepareErr error
	ctx, req, prepareErr = r.prepareWorkspaceRequest(ctx, req)
	if prepareErr != nil {
		return nil, prepareErr
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
	stored := false
	var injectionPersistenceErr error
	defer func() {
		if stored {
			notifyInjectedPersistence(rc.injectedRecords, nil)
			return
		}
		if injectionPersistenceErr == nil {
			injectionPersistenceErr = errors.New("agent stream ended before injected messages were persisted")
		}
		notifyInjectedPersistence(rc.injectedRecords, injectionPersistenceErr)
	}()

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
	idleCtx, idleCancel := withIdleTimeout(streamCtx, r.idleTimeoutOptions)
	defer idleCancel.Stop()

	agentEventCh := r.agent.Stream(idleCtx, cfg)
	modelID := rc.model.ID
	clientGone := false
	var lastSnapshot terminalSnapshot
	var hasSnapshot bool
	var toolCallCount int
	var hasVisibleOutput bool
	var persistedMessages []messagepkg.Message
	var persistenceErr error
	postPersistApplied := false
	var gate userInputStreamGate
	forwardEvent := func(data WSStreamEvent) {
		if clientGone || streamCtx.Err() != nil {
			clientGone = true
			return
		}
		select {
		case eventCh <- data:
		case <-streamCtx.Done():
			clientGone = true
		}
	}
	for event := range agentEventCh {
		if event.IsTerminal() {
			idleCancel.Finish()
		} else {
			idleCancel.Reset()
		}

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
		held := gate.hold(event, data)

		if event.IsTerminal() && len(event.Messages) > 0 {
			if snap, ok := extractTerminalSnapshot(data); ok {
				snap.visibleOutput = hasVisibleOutput
				lastSnapshot = snap
				hasSnapshot = true
				if !stored {
					persisted, snapshotStored, storeErr := r.persistTerminalSnapshotWithStatus(
						context.WithoutCancel(ctx),
						req,
						rc,
						snap,
					)
					persistedMessages = persisted
					stored = snapshotStored
					if storeErr != nil {
						if !stored {
							persistenceErr = storeErr
						}
						r.logger.Error("ws persist failed", slog.Any("error", storeErr))
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
		if held {
			continue
		}

		forwardEvent(WSStreamEvent(data))
	}

	// Intermediate persistence on abort/error
	if !stored {
		switch {
		case hasSnapshot && (persistenceErr == nil || dbstore.IsPersistenceRetrySafe(persistenceErr)):
			persistedMessages, persistenceErr = r.persistPartialResult(ctx, req, rc, lastSnapshot, toolCallCount, idleCancel.DidFire(), hasVisibleOutput)
			stored = len(persistedMessages) > 0
			if stored {
				persistenceErr = nil
			}
			if persistenceErr != nil && !stored {
				injectionPersistenceErr = persistenceErr
				return persistedMessages, fmt.Errorf("persist model stream result: %w", persistenceErr)
			}
		default:
			r.logger.Info("skip persisting failed startup ws stream",
				slog.String("bot_id", req.BotID),
				slog.String("chat_id", req.ChatID),
			)
		}
	}

	idleFired := idleCancel.DidFire()
	var idleErr error
	if idleFired {
		idleErr = fmt.Errorf("stream timeout: no response from model provider (after %d tool calls)", toolCallCount)
		r.logger.Warn("agent ws stream aborted: idle timeout (no events from provider)",
			slog.String("bot_id", req.BotID),
			slog.String("chat_id", req.ChatID),
			slog.String("model_id", modelID),
			slog.Int("tool_calls", toolCallCount),
		)
	}

	if postPersist != nil && !postPersistApplied {
		if err := postPersist(context.WithoutCancel(ctx), persistedMessages); err != nil {
			return persistedMessages, err
		}
	}
	if gate.active {
		if !stored {
			if persistenceErr != nil {
				return persistedMessages, fmt.Errorf("persist model stream result: %w", persistenceErr)
			}
			if idleErr != nil {
				return persistedMessages, idleErr
			}
			return persistedMessages, errors.New("ask_user terminal response was not persisted")
		}
		if !idleFired {
			_ = gate.release(func(data json.RawMessage) error {
				forwardEvent(data)
				return nil
			})
		}
	}
	if persistenceErr != nil {
		return persistedMessages, fmt.Errorf("persist model stream result: %w", persistenceErr)
	}
	if idleErr != nil {
		return persistedMessages, idleErr
	}

	return persistedMessages, nil
}

// persistTerminalSnapshot stores the SDK messages produced by an agent run
// (or partial run) into bot history. Triggers compaction when usage data
// indicates the context is large.
func (r *Resolver) persistTerminalSnapshot(ctx context.Context, req conversation.ChatRequest, rc resolvedContext, snap terminalSnapshot) error {
	_, _, err := r.persistTerminalSnapshotWithStatus(ctx, req, rc, snap)
	return err
}

func (r *Resolver) persistTerminalSnapshotWithStatus(
	ctx context.Context,
	req conversation.ChatRequest,
	rc resolvedContext,
	snap terminalSnapshot,
) ([]messagepkg.Message, bool, error) {
	persisted, err := r.persistTerminalSnapshotResult(ctx, req, rc, snap)
	return persisted, len(persisted) > 0, err
}

func (r *Resolver) persistTerminalSnapshotResult(ctx context.Context, req conversation.ChatRequest, rc resolvedContext, snap terminalSnapshot) ([]messagepkg.Message, error) {
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

	persisted, err := r.storeRoundWithOptionsResult(ctx, storeReq, roundMessages, rc.model.ID, storeRoundOptions{
		AllowPendingToolCalls: snap.deferredToolID != "",
	})
	if err != nil {
		return persisted, err
	}
	if len(persisted) > 0 {
		if err := r.persistSessionWorkspaceTarget(ctx, storeReq); err != nil {
			return persisted, err
		}
	}

	if inputTokens := extractInputTokensFromUsage(snap.usage); inputTokens > 0 {
		go r.maybeCompact(context.WithoutCancel(ctx), req, rc, inputTokens)
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
	snap terminalSnapshot,
	toolCallCount int,
	wasIdleTimeout bool,
	hasVisibleOutput bool,
) ([]messagepkg.Message, error) {
	persistCtx := context.WithoutCancel(ctx)

	if len(snap.sdkMessages) > 0 {
		// Ordinary interrupted runs close orphaned tool calls with synthetic
		// errors. A deferred request keeps its pending tool call open so a
		// transient first persistence failure cannot invalidate the prompt.
		snap.aborted = !hasVisibleOutput
		snap.visibleOutput = hasVisibleOutput
		persisted, err := r.persistTerminalSnapshotResult(persistCtx, req, rc, snap)
		if err == nil {
			r.logger.Info("persisted partial agent result",
				slog.String("bot_id", req.BotID),
				slog.Int("tool_calls", toolCallCount),
				slog.Int("partial_messages", len(snap.sdkMessages)),
				slog.Bool("idle_timeout", wasIdleTimeout),
			)
			// Trigger compaction on the failure path so that oversized
			// contexts don't deadlock (where the LLM can never succeed and
			// therefore compaction never fires).
			if rc.estimatedTokens > 0 && extractInputTokensFromUsage(snap.usage) == 0 {
				go r.maybeCompact(persistCtx, req, rc, rc.estimatedTokens)
			}
			return persisted, nil
		}
		r.logger.Error("failed to persist partial agent messages",
			slog.String("bot_id", req.BotID),
			slog.Any("error", err),
		)
		return persisted, err
	}

	r.logger.Info("skip persisting failed stream without terminal snapshot",
		slog.String("bot_id", req.BotID),
		slog.Int("tool_calls", toolCallCount),
		slog.Bool("idle_timeout", wasIdleTimeout),
		slog.Bool("visible_output", hasVisibleOutput),
	)

	if rc.estimatedTokens > 0 {
		go r.maybeCompact(persistCtx, req, rc, rc.estimatedTokens)
	}
	return nil, nil
}

// interleaveInjectedMessages inserts injected user messages at their correct
// positions within the round. Each record's AfterOutput value indicates how
// many output messages preceded the injection.
//
// round layout: [user_A, output_0, output_1, ..., output_N]
// AfterOutput=K → insert after round[K] (i.e. after the K-th output message).
func interleaveInjectedMessages(round []conversation.ModelMessage, injections []conversation.InjectedMessageRecord) []conversation.ModelMessage {
	if len(injections) == 0 {
		return round
	}
	injections = append([]conversation.InjectedMessageRecord(nil), injections...)
	sort.SliceStable(injections, func(i, j int) bool {
		if injections[i].AfterOutput != injections[j].AfterOutput {
			return injections[i].AfterOutput < injections[j].AfterOutput
		}
		return injections[i].Sequence < injections[j].Sequence
	})
	result := make([]conversation.ModelMessage, 0, len(round)+len(injections))
	injIdx := 0
	for i, msg := range round {
		result = append(result, msg)
		for injIdx < len(injections) && injections[injIdx].AfterOutput == i {
			injected := injections[injIdx].Message
			result = append(result, conversation.ModelMessage{
				Role:     "user",
				Content:  conversation.NewTextContent(injected.HeaderifiedText),
				Injected: &injected,
			})
			injIdx++
		}
	}
	for ; injIdx < len(injections); injIdx++ {
		injected := injections[injIdx].Message
		result = append(result, conversation.ModelMessage{
			Role:     "user",
			Content:  conversation.NewTextContent(injected.HeaderifiedText),
			Injected: &injected,
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
