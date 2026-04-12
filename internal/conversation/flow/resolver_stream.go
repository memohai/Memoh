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
)

// WSStreamEvent represents a raw JSON event forwarded from the agent.
type WSStreamEvent = json.RawMessage

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
		if streamReq.RawQuery == "" {
			streamReq.RawQuery = strings.TrimSpace(streamReq.Query)
		}
		streamReq.Query = rc.query

		go r.maybeGenerateSessionTitle(context.WithoutCancel(ctx), streamReq, streamReq.Query)

		cfg := rc.runConfig
		cfg = r.prepareRunConfig(ctx, cfg)

		// Wrap with idle timeout: if no events arrive within the adaptive timeout, cancel the stream.
		idleCtx, idleCancel := withIdleTimeout(ctx)
		defer idleCancel.Stop()

		eventCh := r.agent.Stream(idleCtx, cfg)
		stored := false
		var toolCallCount int
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
				return
			}
		}

		// Intermediate persistence on abort/error: if stream ended without
		// storing results, persist a synthetic message so the user can see
		// what happened and ask the bot to continue.
		if !stored {
			r.persistPartialResult(ctx, streamReq, rc, toolCallCount, idleCancel.DidFire())
		}

		if idleCancel.DidFire() {
			r.logger.Warn("agent stream aborted: idle timeout (no events from provider)",
				slog.String("bot_id", streamReq.BotID),
				slog.String("chat_id", streamReq.ChatID),
				slog.String("model_id", rc.model.ID),
				slog.Int("tool_calls", toolCallCount),
			)
			// Notify the client that the stream was terminated due to idle timeout.
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

	rc, err := r.resolve(ctx, req)
	if err != nil {
		r.logger.Error("StreamChatWS: resolve failed",
			slog.String("bot_id", req.BotID),
			slog.Any("error", err),
		)
		return fmt.Errorf("resolve: %w", err)
	}
	if req.RawQuery == "" {
		req.RawQuery = strings.TrimSpace(req.Query)
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
	cfg = r.prepareRunConfig(streamCtx, cfg)

	// Wrap with idle timeout: if no events arrive within the adaptive timeout, cancel the stream.
	idleCtx, idleCancel := withIdleTimeout(streamCtx)
	defer idleCancel.Stop()

	agentEventCh := r.agent.Stream(idleCtx, cfg)
	modelID := rc.model.ID
	stored := false
	var toolCallCount int
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
			return ctx.Err()
		}
	}

	// Intermediate persistence on abort/error
	if !stored {
		r.persistPartialResult(ctx, req, rc, toolCallCount, idleCancel.DidFire())
	}

	if idleCancel.DidFire() {
		r.logger.Warn("agent ws stream aborted: idle timeout (no events from provider)",
			slog.String("bot_id", req.BotID),
			slog.String("chat_id", req.ChatID),
			slog.String("model_id", modelID),
			slog.Int("tool_calls", toolCallCount),
		)
		// Notify the client that the stream was terminated due to idle timeout.
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

// persistPartialResult stores a synthetic assistant message when the agent
// stream was interrupted (error, abort, idle timeout) after completing tool
// calls but before producing a final response. This preserves intermediate
// progress so the user can see what was accomplished and ask the bot to continue.
func (r *Resolver) persistPartialResult(ctx context.Context, req conversation.ChatRequest, rc resolvedContext, toolCallCount int, wasIdleTimeout bool) {
	reason := "provider error"
	if wasIdleTimeout {
		reason = "provider idle timeout"
	}
	syntheticMsg := fmt.Sprintf("[Agent interrupted after %d tool calls: %s. Partial results saved — ask the bot to continue.]", toolCallCount, reason)

	roundMessages := prependUserMessage(req.Query, []conversation.ModelMessage{
		{Role: "assistant", Content: conversation.NewTextContent(syntheticMsg)},
	})

	if err := r.storeRound(context.WithoutCancel(ctx), req, roundMessages, rc.model.ID); err != nil {
		r.logger.Error("failed to persist partial result",
			slog.String("bot_id", req.BotID),
			slog.Any("error", err),
		)
	}

	// Trigger compaction on failure path so that oversized contexts don't
	// create a deadlock where the LLM can never succeed (and therefore
	// compaction never fires). Use the estimated token count from resolve.
	if rc.estimatedTokens > 0 {
		r.maybeCompact(context.WithoutCancel(ctx), req, rc, rc.estimatedTokens)
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
