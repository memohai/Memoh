package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
)

// WSStreamEvent represents a raw JSON event forwarded from the agent.
type WSStreamEvent = json.RawMessage

// compactionRetryConfig controls the retry strategy after compaction when the
// agent stream fails. Matches the agent-level retry strategy: 10 total attempts,
// first 5 fast (no delay), last 5 with exponential backoff.
const (
	compactionMaxAttempts  = 10
	compactionFastAttempts = 5
	compactionBaseDelay    = 1 * time.Second
	compactionMaxDelay     = 30 * time.Second
)

// compactionRetryDelay returns the delay before the next compaction-retry attempt.
// First compactionFastAttempts are instant; after that, exponential backoff with jitter.
func compactionRetryDelay(attempt int) time.Duration {
	if attempt < compactionFastAttempts {
		return 0
	}
	backoffIdx := attempt - compactionFastAttempts
	if backoffIdx > 20 {
		backoffIdx = 20
	}
	delay := compactionBaseDelay * time.Duration(1<<uint(backoffIdx))
	delay = min(delay, compactionMaxDelay)
	jitter := time.Duration(rand.Int64N(int64(delay / 2))) //nolint:gosec // jitter does not need crypto/rand
	return delay/2 + jitter
}

// sendWSErrorEvent sends an EventError to the client via the WS event channel.
// Returns true if sent, false if context was cancelled.
func sendWSErrorEvent(ctx context.Context, eventCh chan<- WSStreamEvent, errMsg string) bool {
	evt := agentpkg.StreamEvent{
		Type:  agentpkg.EventError,
		Error: errMsg,
	}
	data, err := json.Marshal(evt)
	if err != nil {
		return false
	}
	select {
	case eventCh <- json.RawMessage(data):
		return true
	case <-ctx.Done():
		return false
	}
}

// sendChunkErrorEvent sends an EventError to the client via the chunk channel.
// Returns true if sent, false if context was cancelled.
func sendChunkErrorEvent(ctx context.Context, chunkCh chan<- conversation.StreamChunk, errMsg string) bool {
	evt := agentpkg.StreamEvent{
		Type:  agentpkg.EventError,
		Error: errMsg,
	}
	data, err := json.Marshal(evt)
	if err != nil {
		return false
	}
	select {
	case chunkCh <- conversation.StreamChunk(data):
		return true
	case <-ctx.Done():
		return false
	}
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

		if streamReq.RawQuery == "" {
			streamReq.RawQuery = strings.TrimSpace(streamReq.Query)
		}

		var titleGenerated bool
		var lastIdleTimeout bool
		var lastToolCallCount int
		var lastRC resolvedContext

		for attempt := 0; attempt < compactionMaxAttempts; attempt++ {
			rc, err := r.resolve(ctx, streamReq)
			if err != nil {
				errMsg := fmt.Sprintf("resolve failed (attempt %d/%d): %v", attempt+1, compactionMaxAttempts, err)
				r.logger.Error("agent stream resolve failed",
					slog.String("bot_id", streamReq.BotID),
					slog.String("chat_id", streamReq.ChatID),
					slog.Int("attempt", attempt+1),
					slog.Any("error", err),
				)
				if attempt == 0 {
					errCh <- err
				} else {
					sendChunkErrorEvent(ctx, chunkCh, errMsg)
				}
				return
			}
			streamReq.Query = rc.query
			lastRC = rc

			if !titleGenerated {
				go r.maybeGenerateSessionTitle(context.WithoutCancel(ctx), streamReq, streamReq.Query)
				titleGenerated = true
			}

			cfg := rc.runConfig
			cfg = r.prepareRunConfig(ctx, cfg)

			// Wrap with idle timeout: if no events arrive within the adaptive timeout, cancel the stream.
			idleCtx, idleCancel := withIdleTimeout(ctx)

			eventCh := r.agent.Stream(idleCtx, cfg)
			stored := false
			hadError := false
			var toolCallCount int
			for event := range eventCh {
				idleCancel.Reset() // each event resets the idle timer

				// Track tool calls for adaptive idle timeout and progress events
				if event.Type == agentpkg.EventToolCallStart {
					toolCallCount++
					idleCancel.RecordToolCall()
				}

				if event.Type == agentpkg.EventError {
					hadError = true
					r.logger.Error("agent stream error",
						slog.String("bot_id", streamReq.BotID),
						slog.String("chat_id", streamReq.ChatID),
						slog.String("model_id", rc.model.ID),
						slog.String("error", event.Error),
					)
				}

				data, err := json.Marshal(event)
				if err != nil {
					continue
				}
				if !stored && event.IsTerminal() && len(event.Messages) > 0 {
					if _, storeErr := r.tryStoreStream(ctx, streamReq, data, rc.model.ID, rc); storeErr != nil {
						r.logger.Error("stream persist failed", slog.Any("error", storeErr))
					} else {
						stored = true
					}
				}
				select {
				case chunkCh <- conversation.StreamChunk(data):
				case <-ctx.Done():
					r.logger.Info("stream chat: context cancelled during event relay",
						slog.String("bot_id", streamReq.BotID),
						slog.String("session_id", streamReq.SessionID),
						slog.Bool("stored", stored),
						slog.Bool("had_error", hadError),
					)
					idleCancel.Stop()
					return
				}
			}

			lastIdleTimeout = idleCancel.DidFire()
			lastToolCallCount = toolCallCount
			idleCancel.Stop()

			// If the stream produced a stored response and no error was
			// observed, the round completed successfully.
			if stored && !hadError {
				r.logger.Info("stream chat: round completed",
					slog.String("bot_id", streamReq.BotID),
					slog.String("session_id", streamReq.SessionID),
					slog.Int("tool_calls", toolCallCount),
					slog.Int("attempt", attempt+1),
				)
				return
			}

			// The stream stored a partial response but also emitted an
			// error. Treat it as a failed round: persist a final error
			// so the user sees something in history, then retry.
			if stored && hadError {
				r.logger.Warn("stream chat: stored partial response but stream had error, will retry",
					slog.String("bot_id", streamReq.BotID),
					slog.String("session_id", streamReq.SessionID),
					slog.Int("tool_calls", toolCallCount),
					slog.Int("attempt", attempt+1),
				)
				r.persistFinalError(context.WithoutCancel(ctx), streamReq, rc, toolCallCount, lastIdleTimeout)
			}

			// Stream ended without a clean store (or stored partial + error).
			// Run compaction to shrink context, then retry.
			if !r.runCompactionSyncWithResult(context.WithoutCancel(ctx), streamReq, rc.estimatedTokens) {
				// Compaction didn't run or failed. No point retrying without it.
				reason := "provider error"
				if lastIdleTimeout {
					reason = "provider idle timeout"
				}
				r.logger.Warn("stream chat: compaction unavailable, giving up",
					slog.String("bot_id", streamReq.BotID),
					slog.String("session_id", streamReq.SessionID),
					slog.String("reason", reason),
					slog.Int("attempt", attempt+1),
				)
				r.persistFinalError(context.WithoutCancel(ctx), streamReq, rc, lastToolCallCount, lastIdleTimeout)
				sendChunkErrorEvent(ctx, chunkCh, fmt.Sprintf("stream failed after %d tool calls: %s (compaction unavailable)", lastToolCallCount, reason))
				return
			}

			// Compaction ran. Notify client and retry.
			r.logger.Info("retrying agent stream after compaction",
				slog.String("bot_id", streamReq.BotID),
				slog.Int("attempt", attempt+2),
				slog.Int("max_attempts", compactionMaxAttempts),
			)
			sendChunkErrorEvent(ctx, chunkCh, fmt.Sprintf("context compacted, retrying (attempt %d/%d)", attempt+2, compactionMaxAttempts))

			if delay := compactionRetryDelay(attempt); delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					return
				}
			}
		}

		// All retries exhausted. Persist a final error for the record.
		r.persistFinalError(context.WithoutCancel(ctx), streamReq, lastRC, lastToolCallCount, lastIdleTimeout)
		sendChunkErrorEvent(ctx, chunkCh, fmt.Sprintf("all %d attempts exhausted after compaction", compactionMaxAttempts))
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
	doneTurn := r.enterSessionTurn(ctx, req.BotID, req.SessionID)
	defer doneTurn()

	if req.RawQuery == "" {
		req.RawQuery = strings.TrimSpace(req.Query)
	}

	var titleGenerated bool
	var lastIdleTimeout bool
	var lastToolCallCount int
	var lastRC resolvedContext

	for attempt := 0; attempt < compactionMaxAttempts; attempt++ {
		rc, err := r.resolve(ctx, req)
		if err != nil {
			errMsg := fmt.Sprintf("resolve failed (attempt %d/%d): %v", attempt+1, compactionMaxAttempts, err)
			r.logger.Error("StreamChatWS: resolve failed",
				slog.String("bot_id", req.BotID),
				slog.Int("attempt", attempt+1),
				slog.Any("error", err),
			)
			sendWSErrorEvent(ctx, eventCh, errMsg)
			if attempt == 0 {
				return fmt.Errorf("resolve: %w", err)
			}
			return fmt.Errorf("resolve retry %d: %w", attempt+1, err)
		}
		req.Query = rc.query
		lastRC = rc

		if !titleGenerated {
			go r.maybeGenerateSessionTitle(context.WithoutCancel(ctx), req, req.Query)
			titleGenerated = true
		}

		streamCtx, cancel := context.WithCancel(ctx)

		abortDone := make(chan struct{})
		go func() {
			defer close(abortDone)
			select {
			case <-abortCh:
				cancel()
			case <-streamCtx.Done():
			}
		}()

		cfg := rc.runConfig
		cfg = r.prepareRunConfig(streamCtx, cfg)

		// Wrap with idle timeout: if no events arrive within the adaptive timeout, cancel the stream.
		idleCtx, idleCancel := withIdleTimeout(streamCtx)

		agentEventCh := r.agent.Stream(idleCtx, cfg)
		modelID := rc.model.ID
		stored := false
		hadError := false
		var toolCallCount int
		for event := range agentEventCh {
			idleCancel.Reset() // each event resets the idle timer

			// Track tool calls for adaptive idle timeout
			if event.Type == agentpkg.EventToolCallStart {
				toolCallCount++
				idleCancel.RecordToolCall()
			}

			if event.Type == agentpkg.EventError {
				hadError = true
				r.logger.Error("agent stream error",
					slog.String("bot_id", req.BotID),
					slog.String("chat_id", req.ChatID),
					slog.String("model_id", modelID),
					slog.String("error", event.Error),
				)
			}

			data, err := json.Marshal(event)
			if err != nil {
				continue
			}

			if !stored && event.IsTerminal() && len(event.Messages) > 0 {
				if _, storeErr := r.tryStoreStream(ctx, req, data, modelID, rc); storeErr != nil {
					r.logger.Error("ws persist failed", slog.Any("error", storeErr))
				} else {
					stored = true
				}
			}

			select {
			case eventCh <- json.RawMessage(data):
			case <-ctx.Done():
				idleCancel.Stop()
				cancel()
				<-abortDone // wait for abort goroutine to finish
				return ctx.Err()
			}
		}

		lastIdleTimeout = idleCancel.DidFire()
		lastToolCallCount = toolCallCount
		idleCancel.Stop()
		cancel()
		<-abortDone // wait for abort goroutine to finish

		if stored && !hadError {
			r.logger.Info("stream ws: round completed",
				slog.String("bot_id", req.BotID),
				slog.String("session_id", req.SessionID),
				slog.Int("tool_calls", toolCallCount),
				slog.Int("attempt", attempt+1),
			)
			return nil
		}

		// Stored partial response but stream had error. Persist error and retry.
		if stored && hadError {
			r.logger.Warn("stream ws: stored partial response but stream had error, will retry",
				slog.String("bot_id", req.BotID),
				slog.String("session_id", req.SessionID),
				slog.Int("tool_calls", toolCallCount),
				slog.Int("attempt", attempt+1),
			)
			r.persistFinalError(context.WithoutCancel(ctx), req, rc, toolCallCount, lastIdleTimeout)
		}

		// Stream ended without a clean store (or stored partial + error).
		// Run compaction to shrink context, then retry.
		if !r.runCompactionSyncWithResult(context.WithoutCancel(ctx), req, rc.estimatedTokens) {
			// Compaction didn't run or failed. No point retrying without it.
			reason := "provider error"
			if lastIdleTimeout {
				reason = "provider idle timeout"
			}
			r.logger.Warn("stream ws: compaction unavailable, giving up",
				slog.String("bot_id", req.BotID),
				slog.String("session_id", req.SessionID),
				slog.String("reason", reason),
				slog.Int("attempt", attempt+1),
			)
			r.persistFinalError(context.WithoutCancel(ctx), req, rc, lastToolCallCount, lastIdleTimeout)
			sendWSErrorEvent(ctx, eventCh, fmt.Sprintf("stream failed after %d tool calls: %s", lastToolCallCount, reason))
			return nil
		}

		// Compaction ran. Notify client and retry.
		r.logger.Info("retrying ws agent stream after compaction",
			slog.String("bot_id", req.BotID),
			slog.Int("attempt", attempt+2),
			slog.Int("max_attempts", compactionMaxAttempts),
		)
		sendWSErrorEvent(ctx, eventCh, fmt.Sprintf("context compacted, retrying (attempt %d/%d)", attempt+2, compactionMaxAttempts))

		if delay := compactionRetryDelay(attempt); delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
		}
	}

	// All retries exhausted. Persist a final error for the record.
	r.persistFinalError(context.WithoutCancel(ctx), req, lastRC, lastToolCallCount, lastIdleTimeout)
	sendWSErrorEvent(ctx, eventCh, fmt.Sprintf("all %d attempts exhausted after compaction", compactionMaxAttempts))
	return nil
}

func (r *Resolver) tryStoreStream(ctx context.Context, req conversation.ChatRequest, data []byte, modelID string, rc resolvedContext) (bool, error) {
	var envelope struct {
		Type     string          `json:"type"`
		Messages json.RawMessage `json:"messages"`
		Usage    json.RawMessage `json:"usage,omitempty"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return false, nil
	}
	if len(envelope.Messages) == 0 {
		return false, nil
	}

	var sdkMsgs []sdk.Message
	if err := json.Unmarshal(envelope.Messages, &sdkMsgs); err != nil || len(sdkMsgs) == 0 {
		return false, nil
	}
	outputMessages := sdkMessagesToModelMessages(sdkMsgs)
	roundMessages := prependUserMessage(req.Query, outputMessages)

	if rc.injectedRecords != nil && len(*rc.injectedRecords) > 0 {
		roundMessages = interleaveInjectedMessages(roundMessages, *rc.injectedRecords)
	}

	if err := r.storeRound(ctx, req, roundMessages, modelID); err != nil {
		return false, err
	}

	if inputTokens := extractInputTokensFromUsage(envelope.Usage); inputTokens > 0 {
		go r.maybeCompact(context.WithoutCancel(ctx), req, rc, inputTokens)
	}

	return true, nil
}

// runCompactionSyncWithResult runs synchronous compaction and returns true
// if compaction actually ran and succeeded.
func (r *Resolver) runCompactionSyncWithResult(ctx context.Context, req conversation.ChatRequest, estimatedTokens int) bool {
	if estimatedTokens <= 0 {
		return false
	}
	return r.runCompactionSync(ctx, req, estimatedTokens)
}

// persistFinalError stores a synthetic assistant message as a last resort when
// all retry attempts have been exhausted or compaction is unavailable.
func (r *Resolver) persistFinalError(ctx context.Context, req conversation.ChatRequest, rc resolvedContext, toolCallCount int, wasIdleTimeout bool) {
	if rc.model.ID == "" {
		return
	}
	reason := "provider error"
	if wasIdleTimeout {
		reason = "provider idle timeout"
	}
	syntheticMsg := fmt.Sprintf("[Agent interrupted after %d tool calls: %s. Partial results saved — ask the bot to continue.]", toolCallCount, reason)

	roundMessages := prependUserMessage(req.Query, []conversation.ModelMessage{
		{Role: "assistant", Content: conversation.NewTextContent(syntheticMsg)},
	})

	if err := r.storeRound(ctx, req, roundMessages, rc.model.ID); err != nil {
		r.logger.Error("failed to persist partial result",
			slog.String("bot_id", req.BotID),
			slog.Any("error", err),
		)
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
