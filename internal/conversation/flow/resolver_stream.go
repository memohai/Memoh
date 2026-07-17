package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

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
	runtimeRows    []messagepkg.RuntimeRowReservation
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

// IsCanceledStreamError reports whether err represents a canceled stream:
// context cancellation, a gRPC Canceled status, or a wrapped provider error
// that only exposes cancellation through its message.
func IsCanceledStreamError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || grpcstatus.Code(err) == codes.Canceled {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "context canceled") ||
		strings.Contains(message, "context cancelled") ||
		strings.Contains(message, "grpc canceled")
}

func isCanceledFlowError(ctx context.Context, err error) bool {
	return ctx.Err() != nil && IsCanceledStreamError(err)
}

type terminalEventDeliveryTimeoutContextKey struct{}

func WithTerminalEventDeliveryTimeout(ctx context.Context, timeout time.Duration) context.Context {
	if ctx == nil || timeout <= 0 {
		return ctx
	}
	return context.WithValue(ctx, terminalEventDeliveryTimeoutContextKey{}, timeout)
}

func sendWSAgentEvent(ctx context.Context, eventCh chan<- WSStreamEvent, event agentpkg.StreamEvent, data json.RawMessage) bool {
	if eventCh == nil {
		return false
	}
	deliveryCtx := ctx
	cancel := func() {}
	if event.IsTerminal() {
		if timeout, ok := ctx.Value(terminalEventDeliveryTimeoutContextKey{}).(time.Duration); ok && timeout > 0 {
			deliveryCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), timeout)
		}
	}
	defer cancel()

	select {
	case eventCh <- data:
		return true
	case <-deliveryCtx.Done():
		return false
	}
}

// logCanceledAware logs err at Error level, degrading to Debug with the
// canceled message when the failure is just our own context cancellation.
func (r *Resolver) logCanceledAware(ctx context.Context, err error, failedMsg, canceledMsg string, args ...any) {
	log := r.logger.Error
	msg := failedMsg
	if isCanceledFlowError(ctx, err) {
		log = r.logger.Debug
		msg = canceledMsg
	}
	log(msg, append(args, slog.Any("error", err))...)
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
		if !streamReq.UserMessagePersisted && !streamReq.UserMessageHookApplied {
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
		idleCtx, idleCancel := withIdleTimeout(streamCtx)
		defer idleCancel.Stop()

		eventCh := r.agent.Stream(idleCtx, cfg)
		stored := false
		clientGone := false
		var lastSnapshot terminalSnapshot
		var hasSnapshot bool
		var toolCallCount int
		var hasVisibleOutput bool
		var persistenceErr error
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
						if storeErr := r.persistTerminalSnapshot(context.WithoutCancel(streamCtx), streamReq, rc, snap); storeErr != nil {
							persistenceErr = fmt.Errorf("persist terminal agent result: %w", storeErr)
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
			if !clientGone {
				select {
				case chunkCh <- conversation.StreamChunk(data):
				case <-streamCtx.Done():
					clientGone = true
				}
			}
		}
		if persistenceErr != nil {
			errCh <- persistenceErr
			return
		}

		// Intermediate persistence on abort/error: persist only concrete
		// partial assistant/tool state. Failed sends without a terminal
		// snapshot are treated as unsent so the Web UI can restore the draft
		// without polluting history.
		if !stored {
			switch {
			case hasSnapshot:
				if _, err := r.persistPartialResult(streamCtx, streamReq, rc, lastSnapshot, toolCallCount, idleCancel.DidFire(), hasVisibleOutput); err != nil {
					errCh <- fmt.Errorf("persist partial agent result: %w", err)
					return
				}
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
					case <-streamCtx.Done():
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
	_, err := r.streamChatWSResult(ctx, req, eventCh, abortCh)
	return err
}

type (
	persistenceGuardContextKey      struct{}
	terminalHookAuthorityContextKey struct{}
)

func WithTerminalHookAuthority(ctx context.Context, authority agentpkg.TerminalHookAuthority) context.Context {
	if ctx == nil || authority.Context == nil {
		return ctx
	}
	return context.WithValue(ctx, terminalHookAuthorityContextKey{}, authority)
}

// TerminalHookAuthorityFromContext returns the runtime ownership authority
// installed by the transport before agent execution begins.
func TerminalHookAuthorityFromContext(ctx context.Context) agentpkg.TerminalHookAuthority {
	if ctx == nil {
		return agentpkg.TerminalHookAuthority{}
	}
	authority, _ := ctx.Value(terminalHookAuthorityContextKey{}).(agentpkg.TerminalHookAuthority)
	return authority
}

// WithPersistenceGuard installs a fail-closed ownership check that runs
// immediately before terminal or partial agent output is written to history.
func WithPersistenceGuard(ctx context.Context, guard func(context.Context) error) context.Context {
	if guard == nil {
		return ctx
	}
	return context.WithValue(ctx, persistenceGuardContextKey{}, guard)
}

func persistenceGuardFromContext(ctx context.Context) func(context.Context) error {
	if ctx == nil {
		return nil
	}
	guard, _ := ctx.Value(persistenceGuardContextKey{}).(func(context.Context) error)
	return guard
}

func runPersistenceGuard(ctx context.Context) error {
	guard := persistenceGuardFromContext(ctx)
	if guard == nil {
		return nil
	}
	guardCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return guard(guardCtx)
}

func (r *Resolver) streamChatWSResult(
	ctx context.Context,
	req conversation.ChatRequest,
	eventCh chan<- WSStreamEvent,
	abortCh <-chan struct{},
) ([]messagepkg.Message, error) {
	return r.streamChatWSResultWithHooks(ctx, req, eventCh, abortCh, nil)
}

func (r *Resolver) streamChatWSResultWithHooks(
	ctx context.Context,
	req conversation.ChatRequest,
	eventCh chan<- WSStreamEvent,
	abortCh <-chan struct{},
	preflight func(context.Context) error,
) ([]messagepkg.Message, error) {
	return r.streamChatWSResultWithHooksAndTurn(ctx, req, eventCh, abortCh, preflight, false)
}

func (r *Resolver) streamChatWSResultWithHooksAndTurn(
	ctx context.Context,
	req conversation.ChatRequest,
	eventCh chan<- WSStreamEvent,
	abortCh <-chan struct{},
	preflight func(context.Context) error,
	sessionTurnHeld bool,
) ([]messagepkg.Message, error) {
	if err := rejectReservedSkillMetadataIfPresent(req); err != nil {
		return nil, err
	}
	if err := r.rejectRequestedSkillsIfUnsupportedContext(ctx, req); err != nil {
		return nil, err
	}
	if ok, err := r.isACPAgentSession(ctx, req); err != nil {
		r.logCanceledAware(ctx, err,
			"StreamChatWS: ACP session check failed",
			"StreamChatWS: ACP session check canceled",
			slog.String("bot_id", req.BotID),
			slog.String("session_id", req.SessionID),
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

	if !sessionTurnHeld {
		doneTurn := r.enterSessionTurn(ctx, req.BotID, req.SessionID)
		defer doneTurn()
	}

	if preflight != nil {
		if err := preflight(ctx); err != nil {
			return nil, err
		}
	}

	if req.RawQuery == "" {
		req.RawQuery = strings.TrimSpace(req.Query)
	}
	var err error
	if !req.UserMessagePersisted && !req.UserMessageHookApplied && !req.ReusePersistedUserMessage {
		req, err = r.applyUserMessageHook(ctx, req)
		if err != nil {
			r.logCanceledAware(ctx, err,
				"StreamChatWS: user message hook failed",
				"StreamChatWS: user message hook canceled",
				slog.String("bot_id", req.BotID),
			)
			return nil, err
		}
	}
	rc, err := r.resolve(ctx, req)
	if err != nil {
		r.logCanceledAware(ctx, err,
			"StreamChatWS: resolve failed",
			"StreamChatWS: resolve canceled",
			slog.String("bot_id", req.BotID),
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
	rowTracker := newRuntimeRowTracker(req.RuntimeTurn)
	bindRuntimeInjectedRecorder(&cfg, rc, rowTracker)
	bindRuntimeSyntheticRowRecorder(&cfg, rowTracker)
	canonicalSteps, err := r.beginCanonicalStepPersistence(streamCtx, req, rc)
	if err != nil {
		return nil, err
	}
	if canonicalSteps != nil {
		committed := agentpkg.StreamEvent{Type: agentpkg.EventHistoryCommit, HistoryCommitted: true}
		data, marshalErr := json.Marshal(committed)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal canonical history commit: %w", marshalErr)
		}
		select {
		case eventCh <- json.RawMessage(data):
		case <-streamCtx.Done():
			return nil, context.Cause(streamCtx)
		}
	}
	cfg = canonicalSteps.attachToRunConfig(cfg)

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
	canonicalFinalized := false
	for event := range agentEventCh {
		idleCancel.Reset() // each event resets the idle timer
		rowTracker.Annotate(&event)

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

		if event.IsTerminal() && canonicalSteps != nil {
			if !stored {
				if snap, ok := extractTerminalSnapshot(data); ok {
					snap.visibleOutput = hasVisibleOutput
					lastSnapshot = snap
					hasSnapshot = true
					if err := canonicalSteps.appendTerminalSnapshot(context.WithoutCancel(ctx), snap); err != nil {
						return persistedMessages, fmt.Errorf("persist terminal canonical step: %w", err)
					}
					r.finalizeCanonicalStepPersistence(context.WithoutCancel(ctx), req, rc, canonicalSteps, snap)
					canonicalFinalized = true
				}
				persistedMessages, event.HistoryCommitted = canonicalSteps.result()
				stored = true
				data, err = json.Marshal(event)
				if err != nil {
					return persistedMessages, fmt.Errorf("marshal terminal canonical result: %w", err)
				}
			}
		} else if event.IsTerminal() && len(event.Messages) > 0 {
			if snap, ok := extractTerminalSnapshot(data); ok {
				snap.visibleOutput = hasVisibleOutput
				snap.runtimeRows = rowTracker.bindTerminalRows(snap.sdkMessages)
				lastSnapshot = snap
				hasSnapshot = true
				if !stored {
					persisted, storeErr := r.persistTerminalSnapshotResult(context.WithoutCancel(ctx), req, rc, snap)
					if storeErr != nil {
						return persistedMessages, fmt.Errorf("persist terminal agent result: %w", storeErr)
					}
					persistedMessages = persisted
					if len(persisted) > 0 {
						if event.Metadata == nil {
							event.Metadata = make(map[string]any, 1)
						}
						event.Metadata[conversation.RuntimePersistedProjectionMetadataKey] = conversation.NewRuntimePersistedProjection(persisted)
						if remarshal, marshalErr := json.Marshal(event); marshalErr == nil {
							data = remarshal
						}
					}
					stored = true
				}
				if len(persistedMessages) > 0 {
					event.HistoryCommitted = true
					data, err = json.Marshal(event)
					if err != nil {
						return persistedMessages, fmt.Errorf("marshal terminal agent result: %w", err)
					}
				}
			}
		}

		if event.IsTerminal() {
			// Execution cancellation must not discard the terminal event that
			// acknowledges partial-history persistence.
			if !sendWSAgentEvent(ctx, eventCh, event, data) {
				clientGone = true
			}
		} else if !clientGone {
			clientGone = !sendWSAgentEvent(ctx, eventCh, event, data)
		}
	}

	// Intermediate persistence on abort/error
	if canonicalSteps != nil && !stored {
		if hasSnapshot {
			if err := canonicalSteps.appendTerminalSnapshot(context.WithoutCancel(ctx), lastSnapshot); err != nil {
				return persistedMessages, fmt.Errorf("persist final canonical step: %w", err)
			}
		}
		if !canonicalFinalized {
			r.finalizeCanonicalStepPersistence(context.WithoutCancel(ctx), req, rc, canonicalSteps, lastSnapshot)
		}
		persistedMessages, _ = canonicalSteps.result()
		stored = true
	}
	if !stored {
		switch {
		case hasSnapshot:
			if guardErr := runPersistenceGuard(ctx); guardErr != nil {
				return persistedMessages, fmt.Errorf("runtime ownership check before partial persistence: %w", guardErr)
			}
			var persistErr error
			persistedMessages, persistErr = r.persistPartialResult(ctx, req, rc, lastSnapshot, toolCallCount, idleCancel.DidFire(), hasVisibleOutput)
			if persistErr != nil {
				return persistedMessages, fmt.Errorf("persist partial agent result: %w", persistErr)
			}
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

	return persistedMessages, nil
}

func (r *Resolver) finalizeCanonicalStepPersistence(
	ctx context.Context,
	req conversation.ChatRequest,
	rc resolvedContext,
	state *canonicalStepPersistenceState,
	snap terminalSnapshot,
) {
	if state == nil {
		return
	}
	if err := runPersistenceGuard(ctx); err != nil {
		r.logger.Warn("skip canonical ancillary finalization after ownership loss",
			slog.String("bot_id", req.BotID),
			slog.String("session_id", req.SessionID),
			slog.Any("error", err),
		)
		return
	}
	if err := r.persistSessionWorkspaceTarget(ctx, req); err != nil {
		r.logger.Error("persist canonical workspace target failed",
			slog.String("bot_id", req.BotID),
			slog.String("session_id", req.SessionID),
			slog.Any("error", err),
		)
	}
	if state.hasAssistant() && !req.SkipMemoryExtraction {
		outputMessages := sdkMessagesToModelMessages(snap.sdkMessages)
		roundMessages := prependTurnUserMessage(req, outputMessages)
		if rc.injectedRecords != nil && len(*rc.injectedRecords) > 0 {
			roundMessages = interleaveInjectedMessages(roundMessages, *rc.injectedRecords)
		}
		go r.storeMemory(context.WithoutCancel(ctx), req, roundMessages)
	}
	if inputTokens := extractInputTokensFromUsage(snap.usage); inputTokens > 0 {
		go r.maybeCompact(context.WithoutCancel(ctx), req, rc, inputTokens)
	} else if snap.aborted && rc.estimatedTokens > 0 {
		go r.maybeCompact(context.WithoutCancel(ctx), req, rc, rc.estimatedTokens)
	}
}

// persistTerminalSnapshot stores the SDK messages produced by an agent run
// (or partial run) into bot history. Triggers compaction when usage data
// indicates the context is large.
func (r *Resolver) persistTerminalSnapshot(ctx context.Context, req conversation.ChatRequest, rc resolvedContext, snap terminalSnapshot) error {
	_, err := r.persistTerminalSnapshotResult(ctx, req, rc, snap)
	return err
}

func (r *Resolver) persistTerminalSnapshotResult(ctx context.Context, req conversation.ChatRequest, rc resolvedContext, snap terminalSnapshot) ([]messagepkg.Message, error) {
	if err := runPersistenceGuard(ctx); err != nil {
		return nil, fmt.Errorf("runtime ownership check before persistence: %w", err)
	}
	outputMessages := sdkMessagesToModelMessages(snap.sdkMessages)
	for i := range outputMessages {
		if i >= len(snap.runtimeRows) || strings.TrimSpace(snap.runtimeRows[i].MessageID) == "" {
			continue
		}
		row := snap.runtimeRows[i]
		outputMessages[i].RuntimeRow = &row
	}
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
		return nil, err
	}
	if len(persisted) > 0 {
		if err := r.persistSessionWorkspaceTarget(ctx, storeReq); err != nil {
			return nil, err
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

func bindRuntimeInjectedRecorder(cfg *agentpkg.RunConfig, rc resolvedContext, rowTracker *RuntimeRowTracker) {
	if cfg == nil || rowTracker == nil || rc.recordInjectedMessage == nil {
		return
	}
	cfg.InjectedRecorder = func(headerifiedText string, _ []sdk.ImagePart, insertAfter int) {
		rc.recordInjectedMessage(conversation.InjectedMessageRecord{
			HeaderifiedText: headerifiedText,
			InsertAfter:     insertAfter,
			RuntimeRow:      rowTracker.reserveInjectedRow(),
		})
	}
}

func bindRuntimeSyntheticRowRecorder(cfg *agentpkg.RunConfig, rowTracker *RuntimeRowTracker) {
	if cfg == nil || rowTracker == nil {
		return
	}
	cfg.SyntheticRowRecorder = rowTracker.reserveTerminalSyntheticRow
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
//
// Reachability note: both stream loops persist a captured terminal snapshot
// inline (or return on its error), so today hasSnapshot implies stored and
// this branch never fires. It stays as the defensive fallback for partial
// persistence and therefore must carry the full snapshot — including the row
// identities bound at capture time. Dropping runtimeRows would persist the
// user row with its reservation while assistant rows fall back to fresh
// identities, which breaks the turn's row-ledger ordering.
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
		// AllowPendingToolCalls=false → repairToolCallClosures will inject
		// synthetic error tool_results for any tool_calls that never received
		// a real result, preserving the assistant ↔ tool pairing required by
		// downstream provider serializers (especially Anthropic).
		persisted, err := r.persistTerminalSnapshotResult(persistCtx, req, rc, terminalSnapshot{
			sdkMessages:   snap.sdkMessages,
			runtimeRows:   snap.runtimeRows,
			aborted:       !hasVisibleOutput,
			visibleOutput: hasVisibleOutput,
		})
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
			if rc.estimatedTokens > 0 {
				go r.maybeCompact(persistCtx, req, rc, rc.estimatedTokens)
			}
			return persisted, nil
		}
		r.logger.Error("failed to persist partial agent messages",
			slog.String("bot_id", req.BotID),
			slog.Any("error", err),
		)
		return nil, err
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
				Role:       "user",
				Content:    conversation.NewTextContent(injections[injIdx].HeaderifiedText),
				RuntimeRow: injections[injIdx].RuntimeRow,
			})
			injIdx++
		}
	}
	for ; injIdx < len(injections); injIdx++ {
		result = append(result, conversation.ModelMessage{
			Role:       "user",
			Content:    conversation.NewTextContent(injections[injIdx].HeaderifiedText),
			RuntimeRow: injections[injIdx].RuntimeRow,
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
