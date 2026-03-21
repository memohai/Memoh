package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

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
	r.logger.Info("agent stream start",
		slog.String("bot_id", req.BotID),
		slog.String("chat_id", req.ChatID),
	)

	go func() {
		defer close(chunkCh)
		defer close(errCh)

		streamReq := req
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
		cfg = r.prepareRunConfig(ctx, cfg)

		eventCh := r.agent.Stream(ctx, cfg)
		stored := false
		for event := range eventCh {
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
				if _, storeErr := r.tryStoreStream(ctx, streamReq, data, rc.model.ID); storeErr != nil {
					r.logger.Error("stream persist failed", slog.Any("error", storeErr))
				} else {
					stored = true
				}
			}
			chunkCh <- conversation.StreamChunk(data)
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
	rc, err := r.resolve(ctx, req)
	if err != nil {
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
	cfg = r.prepareRunConfig(streamCtx, cfg)

	agentEventCh := r.agent.Stream(streamCtx, cfg)
	modelID := rc.model.ID
	stored := false
	for event := range agentEventCh {
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
			if _, storeErr := r.tryStoreStream(ctx, req, data, modelID); storeErr != nil {
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

	return nil
}

// tryStoreStream attempts to extract final messages from a stream event and persist them.
func (r *Resolver) tryStoreStream(ctx context.Context, req conversation.ChatRequest, data []byte, modelID string) (bool, error) {
	var envelope struct {
		Type     string          `json:"type"`
		Messages json.RawMessage `json:"messages"`
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

	return true, r.storeRound(ctx, req, roundMessages, modelID)
}
