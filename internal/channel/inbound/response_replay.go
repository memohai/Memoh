package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/messageconv"
	"github.com/memohai/memoh/internal/userinput"
)

type pendingUserInputResolver interface {
	ResolvePendingUserInput(ctx context.Context, botID, sessionID, requestID string) (userinput.Request, error)
}

type userInputPromptDeliveryMarker interface {
	MarkUserInputPromptDelivered(ctx context.Context, requestID string) (userinput.Request, error)
}

func (p *ChannelInboundProcessor) loadPersistedTurnResponse(
	ctx context.Context,
	sessionID string,
	requestMessageID string,
	required bool,
) ([]conversation.ModelMessage, bool, error) {
	reader, ok := p.message.(messagepkg.TurnResponseReplayReader)
	if !ok {
		if required {
			return nil, false, errors.New("durable turn response reader is not configured")
		}
		return nil, false, nil
	}
	stored, err := reader.ListVisibleTurnResponsesByRequest(ctx, sessionID, requestMessageID)
	if err != nil {
		return nil, false, fmt.Errorf("load durable turn response: %w", err)
	}
	if len(stored) == 0 {
		if required {
			return nil, false, errors.New("durable turn response is missing")
		}
		return nil, false, nil
	}
	messages := make([]conversation.ModelMessage, 0, len(stored))
	for _, persisted := range stored {
		var message conversation.ModelMessage
		if err := json.Unmarshal(persisted.Content, &message); err != nil {
			return nil, false, fmt.Errorf("decode durable turn response %s: %w", persisted.ID, err)
		}
		if strings.TrimSpace(message.Role) == "" {
			message.Role = strings.TrimSpace(persisted.Role)
		}
		messages = append(messages, message)
	}
	return messages, true, nil
}

func (p *ChannelInboundProcessor) persistedUserInputEvents(
	ctx context.Context,
	botID string,
	sessionID string,
	messages []conversation.ModelMessage,
) ([]channel.StreamEvent, error) {
	seen := make(map[string]struct{})
	var events []channel.StreamEvent
	for _, message := range messages {
		if !strings.EqualFold(strings.TrimSpace(message.Role), "assistant") {
			continue
		}
		for _, part := range messageconv.ModelMessageToSDKMessage(message).Content {
			call, ok := part.(sdk.ToolCallPart)
			if !ok || !strings.EqualFold(strings.TrimSpace(call.ToolName), userinput.ToolNameAskUser) {
				continue
			}
			metadata, ok := call.ProviderMetadata["user_input"].(map[string]any)
			if !ok {
				continue
			}
			requestID := strings.TrimSpace(metadataString(metadata, "user_input_id"))
			if requestID == "" {
				return nil, errors.New("durable ask_user response is missing user_input_id")
			}
			if _, duplicate := seen[requestID]; duplicate {
				continue
			}
			seen[requestID] = struct{}{}
			resolver, ok := p.runner.(pendingUserInputResolver)
			if !ok {
				return nil, errors.New("pending user input resolver is not configured")
			}
			req, err := resolver.ResolvePendingUserInput(ctx, botID, sessionID, requestID)
			if errors.Is(err, userinput.ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("resolve durable ask_user request %s: %w", requestID, err)
			}
			if req.ID != requestID || req.SessionID != sessionID || req.BotID != botID ||
				req.Status != userinput.StatusPending ||
				!strings.EqualFold(strings.TrimSpace(req.ToolName), userinput.ToolNameAskUser) ||
				strings.TrimSpace(req.ToolCallID) != strings.TrimSpace(call.ToolCallID) {
				return nil, fmt.Errorf("durable ask_user request %s does not match persisted response", requestID)
			}
			if userInputPromptDelivered(req) {
				continue
			}
			payload, err := json.Marshal(req.UIPayload)
			if err != nil {
				return nil, fmt.Errorf("encode durable ask_user payload %s: %w", requestID, err)
			}
			event := buildUserInputStreamEvent(
				req.ToolName,
				req.ToolCallID,
				req.ID,
				req.ShortID,
				req.Status,
				json.RawMessage(payload),
			)
			event.ToolCall.Locale = p.localizer(ctx, botID).Locale()
			events = append(events, event)
		}
	}
	return events, nil
}

func (p *ChannelInboundProcessor) pushStreamEvent(
	ctx context.Context,
	botID string,
	sessionID string,
	stream channel.OutboundStream,
	event channel.StreamEvent,
) error {
	if !isUserInputEvent(&event) {
		return stream.Push(ctx, event)
	}
	input, _ := event.ToolCall.Input.(map[string]any)
	requestID := strings.TrimSpace(metadataString(input, "user_input_id"))
	if requestID == "" {
		return errors.New("ask_user stream event is missing user_input_id")
	}
	resolver, ok := p.runner.(pendingUserInputResolver)
	if !ok {
		return errors.New("pending user input resolver is not configured")
	}
	req, err := resolver.ResolvePendingUserInput(ctx, botID, sessionID, requestID)
	if errors.Is(err, userinput.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve ask_user delivery %s: %w", requestID, err)
	}
	if req.ID != requestID || strings.TrimSpace(req.ToolCallID) != strings.TrimSpace(event.ToolCall.CallID) ||
		!strings.EqualFold(strings.TrimSpace(req.ToolName), strings.TrimSpace(event.ToolCall.Name)) {
		return fmt.Errorf("ask_user delivery %s does not match its stream event", requestID)
	}
	if userInputPromptDelivered(req) {
		return nil
	}
	if err := stream.Push(ctx, event); err != nil {
		return err
	}
	marker, ok := p.runner.(userInputPromptDeliveryMarker)
	if !ok {
		return errors.New("user input prompt delivery marker is not configured")
	}
	marked, err := marker.MarkUserInputPromptDelivered(context.WithoutCancel(ctx), requestID)
	if errors.Is(err, userinput.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("mark ask_user prompt %s delivered: %w", requestID, err)
	}
	if marked.ID != requestID || marked.PromptDeliveredAt == nil {
		return fmt.Errorf("mark ask_user prompt %s delivered: invalid durable evidence", requestID)
	}
	return nil
}

func userInputPromptDelivered(req userinput.Request) bool {
	return req.PromptDeliveredAt != nil || strings.TrimSpace(req.PromptExternalMessageID) != ""
}
