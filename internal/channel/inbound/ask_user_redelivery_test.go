package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/messageconv"
	"github.com/memohai/memoh/internal/pipeline"
	"github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/userinput"
)

func TestNonDiscussRedeliveryReplaysPendingAskUserAfterOutboundFailure(t *testing.T) {
	processor, queries, writer, gateway, pending, gatewayCalls := newPendingAskUserDelivery(t)

	firstSender := &failUserInputReplySender{}
	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), firstSender); err == nil {
		t.Fatal("first HandleInbound() error = nil, want outbound ask_user failure")
	}
	if *gatewayCalls != 1 || queries.deliveryIsCompleted() {
		t.Fatalf("first delivery = gateway:%d completed:%t, want 1/false", *gatewayCalls, queries.deliveryIsCompleted())
	}

	retrySender := &fakeReplySender{}
	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), retrySender); err != nil {
		t.Fatalf("redelivery HandleInbound() error = %v", err)
	}
	if *gatewayCalls != 1 {
		t.Fatalf("gateway calls after durable replay = %d, want 1", *gatewayCalls)
	}
	if gateway.pendingInputBotID != deliveryBotID || gateway.pendingInputSessionID != deliverySessionID ||
		gateway.pendingInputRequestID != pending.ID {
		t.Fatalf("pending lookup = bot:%q session:%q request:%q",
			gateway.pendingInputBotID, gateway.pendingInputSessionID, gateway.pendingInputRequestID)
	}
	replayed := firstUserInputToolCall(retrySender.events)
	if replayed == nil || replayed.CallID != pending.ToolCallID || replayed.ShortID != pending.ShortID {
		t.Fatalf("replayed ask_user = %#v", replayed)
	}
	input, _ := replayed.Input.(map[string]any)
	if input["user_input_id"] != pending.ID || len(userinput.PayloadFromStored(input["payload"]).Questions) != 1 {
		t.Fatalf("replayed ask_user input = %#v", input)
	}
	rendered := channel.RenderToolCallMessage(channel.BuildToolCallStart(replayed))
	if !strings.Contains(rendered, "Pick one") || !strings.Contains(rendered, "Alpha") {
		t.Fatalf("plain-text ask_user replay = %q", rendered)
	}
	if gateway.promptDeliveryCalls != 1 {
		t.Fatalf("prompt delivery marks = %d, want 1 after successful retry", gateway.promptDeliveryCalls)
	}
	if !queries.deliveryIsCompleted() || writer.completionCalls != 1 {
		t.Fatalf("completed delivery = %t, pending completions = %d", queries.deliveryIsCompleted(), writer.completionCalls)
	}
}

func TestNonDiscussRedeliveryDoesNotRepeatAskUserAfterLaterOutboundFailure(t *testing.T) {
	processor, queries, _, gateway, pending, gatewayCalls := newPendingAskUserDelivery(t)
	firstSender := &failAfterUserInputReplySender{}

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), firstSender); err == nil {
		t.Fatal("first HandleInbound() error = nil, want post-prompt status failure")
	}
	if firstUserInputToolCall(firstSender.events) == nil || gateway.promptDeliveryCalls != 1 {
		t.Fatalf("first prompt = %#v, marks = %d", firstSender.events, gateway.promptDeliveryCalls)
	}
	if gateway.pendingInputs[pending.ID].PromptDeliveredAt == nil {
		t.Fatal("successful prompt push was not durably marked")
	}

	retrySender := &fakeReplySender{}
	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), retrySender); err != nil {
		t.Fatalf("redelivery HandleInbound() error = %v", err)
	}
	if repeated := firstUserInputToolCall(retrySender.events); repeated != nil {
		t.Fatalf("ask_user repeated after successful delivery: %#v", repeated)
	}
	if *gatewayCalls != 1 || gateway.promptDeliveryCalls != 1 {
		t.Fatalf("calls = gateway:%d prompt marks:%d, want 1/1", *gatewayCalls, gateway.promptDeliveryCalls)
	}
	if !queries.deliveryIsCompleted() {
		t.Fatal("delivery was not completed after deduplicated retry")
	}
}

func TestNonDiscussAskUserPromptMarkerFailureDoesNotCompleteDelivery(t *testing.T) {
	processor, queries, _, gateway, _, gatewayCalls := newPendingAskUserDelivery(t)
	gateway.promptDeliveryErr = errors.New("prompt marker unavailable")
	sender := &fakeReplySender{}

	err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), sender)
	if err == nil || !strings.Contains(err.Error(), "prompt marker unavailable") {
		t.Fatalf("HandleInbound() error = %v, want marker failure", err)
	}
	if firstUserInputToolCall(sender.events) == nil {
		t.Fatal("ask_user prompt was not sent before marker failure")
	}
	if *gatewayCalls != 1 || gateway.promptDeliveryCalls != 1 || queries.deliveryIsCompleted() {
		t.Fatalf("marker failure = gateway:%d marks:%d completed:%t, want 1/1/false",
			*gatewayCalls, gateway.promptDeliveryCalls, queries.deliveryIsCompleted())
	}
}

func TestNonDiscussRedeliverySkipsAlreadyBoundAskUser(t *testing.T) {
	queries := seededDurableEventQueries(t, true)
	queries.deliveryCompleted = false
	queries.pending = true
	queries.response = true
	pending := pendingAskUserRequest()
	pending.PromptExternalMessageID = "already-delivered"
	pipelineState := pipeline.NewPipeline(pipeline.RenderParams{})
	writer := &durableHistoryWriter{
		queries: queries, pipeline: pipelineState,
		replayMessages: []message.Message{durableAskUserReplayMessage(t, pending)},
	}
	processor, _, gateway := newEventDeliveryProcessor(session.TypeChat, queries, writer, pipelineState)
	gateway.pendingInputs = map[string]userinput.Request{pending.ID: pending}
	gatewayCalls := 0
	gateway.onChat = func(conversation.ChatRequest) { gatewayCalls++ }
	sender := &fakeReplySender{}

	if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryMessage(), sender); err != nil {
		t.Fatalf("HandleInbound() error = %v", err)
	}
	if replayed := firstUserInputToolCall(sender.events); replayed != nil {
		t.Fatalf("already-bound ask_user was replayed: %#v", replayed)
	}
	if gatewayCalls != 0 || gateway.pendingInputCalls != 1 || gateway.promptDeliveryCalls != 0 {
		t.Fatalf("redelivery calls = gateway:%d pending:%d marks:%d, want 0/1/0",
			gatewayCalls, gateway.pendingInputCalls, gateway.promptDeliveryCalls)
	}
	if !queries.deliveryIsCompleted() {
		t.Fatal("delivery was not completed after skipping bound ask_user")
	}
}

func newPendingAskUserDelivery(t *testing.T) (
	*ChannelInboundProcessor,
	*durableEventQueries,
	*durableHistoryWriter,
	*fakeChatGateway,
	userinput.Request,
	*int,
) {
	t.Helper()
	queries := seededDurableEventQueries(t, false)
	pipelineState := pipeline.NewPipeline(pipeline.RenderParams{})
	writer := &durableHistoryWriter{queries: queries, pipeline: pipelineState}
	processor, _, gateway := newEventDeliveryProcessor(session.TypeChat, queries, writer, pipelineState)
	pending := pendingAskUserRequest()
	gateway.pendingInputs = map[string]userinput.Request{pending.ID: pending}
	writer.replayMessages = []message.Message{durableAskUserReplayMessage(t, pending)}
	gateway.streamChunks = []conversation.StreamChunk{conversation.StreamChunk([]byte(`{
		"type":"user_input_request",
		"toolName":"ask_user",
		"toolCallId":"ask-1",
		"userInputId":"66666666-6666-4666-8666-666666666666",
		"shortId":7,
		"status":"pending",
		"metadata":{"ui_payload":{"version":2,"questions":[{"id":"q1","text":"Pick one","kind":"single_select","options":[{"id":"q1.o1","label":"Alpha"}]}]}}
	}`))}
	gatewayCalls := 0
	gateway.onChat = func(conversation.ChatRequest) {
		gatewayCalls++
		queries.mu.Lock()
		queries.history = true
		queries.historyID = deliveryPGUUID("44444444-4444-4444-4444-444444444444")
		queries.pending = true
		queries.response = true
		queries.mu.Unlock()
	}
	return processor, queries, writer, gateway, pending, &gatewayCalls
}

func pendingAskUserRequest() userinput.Request {
	return userinput.Request{
		ID:         "66666666-6666-4666-8666-666666666666",
		BotID:      deliveryBotID,
		SessionID:  deliverySessionID,
		ToolCallID: "ask-1",
		ToolName:   userinput.ToolNameAskUser,
		ShortID:    7,
		Status:     userinput.StatusPending,
		Input: map[string]any{
			"questions": []any{map[string]any{"text": "Pick one"}},
		},
		UIPayload: userinput.UIPayload{
			Version: userinput.PayloadVersion,
			Questions: []userinput.UIQuestion{{
				ID: "q1", Text: "Pick one", Kind: userinput.QuestionKindSingleSelect,
				Options: []userinput.UIOption{{ID: "q1.o1", Label: "Alpha"}},
			}},
		},
	}
}

func durableAskUserReplayMessage(t *testing.T, req userinput.Request) message.Message {
	t.Helper()
	model := messageconv.SDKMessagesToModelMessages([]sdk.Message{{
		Role: sdk.MessageRoleAssistant,
		Content: []sdk.MessagePart{sdk.ToolCallPart{
			ToolCallID: req.ToolCallID,
			ToolName:   req.ToolName,
			Input:      req.Input,
			ProviderMetadata: map[string]any{
				"user_input": map[string]any{
					"user_input_id": req.ID,
					"short_id":      req.ShortID,
					"status":        req.Status,
					"ui_payload":    req.UIPayload,
				},
			},
		}},
	}})[0]
	content, err := json.Marshal(model)
	if err != nil {
		t.Fatalf("marshal replay model: %v", err)
	}
	return message.Message{
		ID: "77777777-7777-4777-8777-777777777777", Role: "assistant", Content: content,
	}
}

func firstUserInputToolCall(events []channel.StreamEvent) *channel.StreamToolCall {
	for i := range events {
		if isUserInputEvent(&events[i]) {
			return events[i].ToolCall
		}
	}
	return nil
}

type failUserInputReplySender struct {
	fakeReplySender
}

func (s *failUserInputReplySender) OpenStream(_ context.Context, target string, _ channel.StreamOptions) (channel.OutboundStream, error) {
	return &failUserInputOutboundStream{
		delegate: &fakeOutboundStream{sender: &s.fakeReplySender, target: target},
	}, nil
}

type failUserInputOutboundStream struct {
	delegate channel.OutboundStream
}

func (s *failUserInputOutboundStream) Push(ctx context.Context, event channel.StreamEvent) error {
	if isUserInputEvent(&event) {
		return errors.New("ask_user outbound unavailable")
	}
	return s.delegate.Push(ctx, event)
}

func (s *failUserInputOutboundStream) Close(ctx context.Context) error {
	return s.delegate.Close(ctx)
}

type failAfterUserInputReplySender struct {
	fakeReplySender
}

func (s *failAfterUserInputReplySender) OpenStream(_ context.Context, target string, _ channel.StreamOptions) (channel.OutboundStream, error) {
	return &failAfterUserInputOutboundStream{
		delegate: &fakeOutboundStream{sender: &s.fakeReplySender, target: target},
	}, nil
}

type failAfterUserInputOutboundStream struct {
	delegate      channel.OutboundStream
	userInputSent bool
}

func (s *failAfterUserInputOutboundStream) Push(ctx context.Context, event channel.StreamEvent) error {
	if isUserInputEvent(&event) {
		s.userInputSent = true
	}
	if s.userInputSent && event.Type == channel.StreamEventStatus && event.Status == channel.StreamStatusCompleted {
		return errors.New("status completion unavailable")
	}
	return s.delegate.Push(ctx, event)
}

func (s *failAfterUserInputOutboundStream) Close(ctx context.Context) error {
	return s.delegate.Close(ctx)
}
