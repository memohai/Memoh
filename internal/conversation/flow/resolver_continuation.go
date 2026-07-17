package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
)

// DeferredContinuation is the immutable row-allocation plan for resuming a
// tool call after its in-process waiter disappeared. The original request row
// remains the turn anchor, while ResponseRow and every later model row append
// to that same turn without allocating a second turn position.
type DeferredContinuation struct {
	turn          messagepkg.RuntimeTurnReservation
	responseRow   messagepkg.RuntimeRowReservation
	persistedRows []messagepkg.Message
}

// TurnReservation returns a detached copy suitable for runtime admission.
func (c DeferredContinuation) TurnReservation() messagepkg.RuntimeTurnReservation {
	return c.turn
}

// InitialRowLedger is the complete turn state known at admission: persisted
// request/tool-call rows plus the response row whose identity is already
// reserved for the decision being processed.
func (c DeferredContinuation) InitialRowLedger() []conversation.UIRowIdentity {
	rows := make([]conversation.UIRowIdentity, 0, len(c.persistedRows)+1)
	for _, message := range c.persistedRows {
		if strings.TrimSpace(message.ID) == "" {
			continue
		}
		rows = append(rows, conversation.UIRowIdentity{
			StableID: message.ID, Role: strings.ToLower(strings.TrimSpace(message.Role)),
			TurnID: message.TurnID, TurnPosition: message.TurnPosition, TurnMessageSeq: message.TurnMessageSeq,
		})
	}
	rows = append(rows, conversation.UIRowIdentity{
		StableID: c.responseRow.MessageID, Role: c.responseRow.Role,
		TurnID: c.responseRow.TurnID, TurnPosition: c.responseRow.TurnPosition, TurnMessageSeq: c.responseRow.TurnMessageSeq,
	})
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].TurnMessageSeq < rows[j].TurnMessageSeq
	})
	return rows
}

// NewDeferredContinuation validates a reconstructed continuation plan. The
// constructor keeps the plan immutable to callers while allowing runtime
// admission state to be restored by another process.
func NewDeferredContinuation(turn messagepkg.RuntimeTurnReservation, responseRow messagepkg.RuntimeRowReservation, persistedRows []messagepkg.Message) (DeferredContinuation, error) {
	if strings.TrimSpace(turn.TurnID) == "" || turn.TurnPosition <= 0 || strings.TrimSpace(turn.Request.MessageID) == "" {
		return DeferredContinuation{}, errors.New("deferred continuation turn is incomplete")
	}
	if !strings.EqualFold(strings.TrimSpace(turn.Request.Role), "user") || turn.Request.TurnID != turn.TurnID || turn.Request.TurnPosition != turn.TurnPosition || turn.Request.TurnMessageSeq != 1 {
		return DeferredContinuation{}, errors.New("deferred continuation request row does not match its turn")
	}
	if strings.TrimSpace(responseRow.MessageID) == "" || !strings.EqualFold(strings.TrimSpace(responseRow.Role), "tool") || responseRow.TurnID != turn.TurnID || responseRow.TurnPosition != turn.TurnPosition || responseRow.TurnMessageSeq <= 1 {
		return DeferredContinuation{}, errors.New("deferred continuation response row does not match its turn")
	}
	return DeferredContinuation{
		turn:          turn,
		responseRow:   responseRow,
		persistedRows: append([]messagepkg.Message(nil), persistedRows...),
	}, nil
}

func (r *Resolver) prepareDeferredContinuation(ctx context.Context, sessionID, toolCallID string) (DeferredContinuation, error) {
	if r == nil || r.messageService == nil {
		return DeferredContinuation{}, errors.New("message service not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	toolCallID = strings.TrimSpace(toolCallID)
	if sessionID == "" || toolCallID == "" {
		return DeferredContinuation{}, errors.New("deferred continuation scope is incomplete")
	}

	messages, err := r.messageService.ListBySession(ctx, sessionID)
	if err != nil {
		return DeferredContinuation{}, fmt.Errorf("load deferred continuation history: %w", err)
	}
	assistantID := findAssistantMessageForToolCall(messages, toolCallID)
	if assistantID == "" {
		return DeferredContinuation{}, fmt.Errorf("assistant row for deferred tool call %q was not found", toolCallID)
	}

	var assistant messagepkg.Message
	for _, message := range messages {
		if message.ID == assistantID {
			assistant = message
			break
		}
	}
	if strings.TrimSpace(assistant.TurnID) == "" || assistant.TurnPosition <= 0 || assistant.TurnMessageSeq <= 0 {
		return DeferredContinuation{}, errors.New("deferred tool-call row has no runtime turn coordinates")
	}

	turnRows := make([]messagepkg.Message, 0, 3)
	var request messagepkg.Message
	var tail messagepkg.Message
	for _, message := range messages {
		if message.TurnID != assistant.TurnID {
			continue
		}
		turnRows = append(turnRows, message)
		if message.TurnMessageSeq == 1 && strings.EqualFold(strings.TrimSpace(message.Role), "user") {
			request = message
		}
		if tail.TurnMessageSeq < message.TurnMessageSeq {
			tail = message
		}
	}
	if strings.TrimSpace(request.ID) == "" {
		return DeferredContinuation{}, errors.New("deferred continuation turn has no request row")
	}
	if tail.ID != assistant.ID {
		return DeferredContinuation{}, errors.New("deferred tool-call row is no longer the turn tail")
	}
	sort.SliceStable(turnRows, func(i, j int) bool {
		return turnRows[i].TurnMessageSeq < turnRows[j].TurnMessageSeq
	})

	turn := messagepkg.RuntimeTurnReservation{
		TurnID:       assistant.TurnID,
		TurnPosition: assistant.TurnPosition,
		Request:      runtimeReservationForPersistedMessage(request),
	}
	return NewDeferredContinuation(turn, messagepkg.RuntimeRowReservation{
		MessageID:      uuid.NewString(),
		Role:           "tool",
		TurnID:         assistant.TurnID,
		TurnPosition:   assistant.TurnPosition,
		TurnMessageSeq: assistant.TurnMessageSeq + 1,
	}, turnRows)
}

func runtimeReservationForPersistedMessage(message messagepkg.Message) messagepkg.RuntimeRowReservation {
	return messagepkg.RuntimeRowReservation{
		MessageID:      message.ID,
		Role:           strings.ToLower(strings.TrimSpace(message.Role)),
		TurnID:         message.TurnID,
		TurnPosition:   message.TurnPosition,
		TurnMessageSeq: message.TurnMessageSeq,
	}
}

func (r *Resolver) persistDeferredResponse(
	ctx context.Context,
	req conversation.ChatRequest,
	messages []conversation.ModelMessage,
	continuation DeferredContinuation,
) (DeferredContinuation, error) {
	if len(messages) != 1 || !strings.EqualFold(strings.TrimSpace(messages[0].Role), "tool") {
		return DeferredContinuation{}, errors.New("deferred continuation requires exactly one tool response row")
	}
	row := continuation.responseRow
	messages[0].RuntimeRow = &row
	turn := continuation.turn
	req.RuntimeTurn = &turn
	persisted, err := r.storeRoundWithOptionsResult(ctx, req, messages, "", storeRoundOptions{AllowPendingToolCalls: true})
	if err != nil {
		return DeferredContinuation{}, err
	}
	if len(persisted) != 1 {
		return DeferredContinuation{}, fmt.Errorf("persisted deferred response rows = %d, want 1", len(persisted))
	}
	response := persisted[0]
	if response.ID != row.MessageID || response.TurnID != row.TurnID || response.TurnPosition != row.TurnPosition || response.TurnMessageSeq != row.TurnMessageSeq {
		return DeferredContinuation{}, errors.New("persisted deferred response changed its runtime row identity")
	}
	continuation.persistedRows = append(append([]messagepkg.Message(nil), continuation.persistedRows...), response)
	return continuation, nil
}

func newDeferredContinuationRowTracker(continuation DeferredContinuation) *runtimeRowTracker {
	turn := continuation.turn
	tracker := &runtimeRowTracker{
		turn:        &turn,
		nextStepSeq: continuation.responseRow.TurnMessageSeq + 1,
	}
	for _, message := range continuation.persistedRows {
		if message.ID == turn.Request.MessageID {
			continue
		}
		row := runtimeReservationForPersistedMessage(message)
		tracker.auxiliaryRows = append(tracker.auxiliaryRows, row)
		tracker.pendingLedgerRows = append(tracker.pendingLedgerRows, row)
	}
	return tracker
}

func (r *Resolver) continueDeferredSession(
	ctx context.Context,
	continuation DeferredContinuation,
	cfg agent.RunConfig,
	req conversation.ChatRequest,
	rc resolvedContext,
	eventCh chan<- WSStreamEvent,
) error {
	turn := continuation.turn
	req.RuntimeTurn = &turn
	rowTracker := newDeferredContinuationRowTracker(continuation)
	bindRuntimeSyntheticRowRecorder(&cfg, rowTracker)

	stream := r.agent.Stream(ctx, cfg)
	stored := false
	for event := range stream {
		rowTracker.annotate(&event)
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		if !stored && event.IsTerminal() && len(event.Messages) > 0 {
			if snap, ok := extractTerminalSnapshot(data); ok {
				snap.runtimeRows = rowTracker.bindTerminalRows(snap.sdkMessages)
				persisted, storeErr := r.persistTerminalSnapshotResult(context.WithoutCancel(ctx), req, rc, snap)
				if storeErr != nil {
					return storeErr
				}
				allPersisted := append(append([]messagepkg.Message(nil), continuation.persistedRows...), persisted...)
				if len(allPersisted) > 0 {
					if event.Metadata == nil {
						event.Metadata = make(map[string]any, 1)
					}
					event.Metadata[conversation.RuntimePersistedProjectionMetadataKey] = conversation.NewRuntimePersistedProjection(allPersisted)
					if remarshal, marshalErr := json.Marshal(event); marshalErr == nil {
						data = remarshal
					}
				}
				stored = true
			}
		}
		if eventCh != nil {
			select {
			case eventCh <- json.RawMessage(data):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return nil
}

func (r *Resolver) prepareContinuationRunConfig(
	ctx context.Context,
	base agent.RunConfig,
	fallback historyfrag.ScopeFallback,
	summaryScope contextfrag.Scope,
	eventCh chan<- WSStreamEvent,
) (agent.RunConfig, error) {
	loaded, err := r.loadHistoryRecords(ctx, fallback, summaryScope.SessionID, defaultMaxContextMinutes)
	if err != nil {
		return agent.RunConfig{}, err
	}
	loaded = pruneHistoryForGateway(loaded)
	loaded, err = r.replaceCompactedMessages(ctx, summaryScope.SessionID, summaryScope, loaded, compactionArtifactBoundary{})
	if err != nil {
		return agent.RunConfig{}, err
	}
	messages, retained, _ := trimMessagesAndRecordsByTokens(r.logger, loaded, 0)
	messages = sanitizeMessages(messages)

	base.ContextFrags = historyContextFragsForMessages(messages, retained)
	// Close any tool call left open by an interrupted turn before the transcript
	// reaches providers that enforce strict assistant-tool adjacency. A process
	// restart can orphan a deferred ask_user / tool-approval call while a later
	// request still completes normally; repairing here (not in ContextFrags)
	// keeps the fragments faithful to history while the outgoing messages stay
	// provider-valid. Applies to every continuation path that resumes after a
	// deferred tool call.
	base.Messages = modelMessagesToSDKMessages(repairToolCallClosures(nonNilModelMessages(messages), syntheticToolClosureError))
	base.Query = ""
	base.LiveToolStream = eventCh != nil
	base.CanRequestUserInput = r.canDeliverUserInputWS(eventCh)
	return r.prepareRunConfig(ctx, base), nil
}
