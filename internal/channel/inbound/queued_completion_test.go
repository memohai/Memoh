package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

type orderedCompletionWriter struct {
	base     *durableHistoryWriter
	timeline *[]string
}

func (w *orderedCompletionWriter) Persist(ctx context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	return w.base.Persist(ctx, input)
}

func (w *orderedCompletionWriter) CompletePendingDelivery(ctx context.Context, messageID string) error {
	*w.timeline = append(*w.timeline, "complete")
	return w.base.CompletePendingDelivery(ctx, messageID)
}

type queuedLifecycleSender struct {
	timeline      *[]string
	finalTexts    []string
	failSend      bool
	failFinal     bool
	failCompleted bool
	failClose     bool
}

func (s *queuedLifecycleSender) Send(context.Context, channel.OutboundMessage) error {
	*s.timeline = append(*s.timeline, "send")
	if s.failSend {
		return errors.New("send failed")
	}
	return nil
}

func (s *queuedLifecycleSender) OpenStream(context.Context, string, channel.StreamOptions) (channel.OutboundStream, error) {
	return &queuedLifecycleStream{sender: s}, nil
}

type queuedLifecycleStream struct {
	sender *queuedLifecycleSender
}

func (s *queuedLifecycleStream) Push(_ context.Context, event channel.StreamEvent) error {
	switch {
	case event.Type == channel.StreamEventFinal:
		*s.sender.timeline = append(*s.sender.timeline, "final")
		if event.Final != nil {
			s.sender.finalTexts = append(s.sender.finalTexts, event.Final.Message.PlainText())
		}
		if s.sender.failFinal {
			return errors.New("final push failed")
		}
	case event.Type == channel.StreamEventStatus && event.Status == channel.StreamStatusCompleted:
		*s.sender.timeline = append(*s.sender.timeline, "status")
		if s.sender.failCompleted {
			return errors.New("completed status failed")
		}
	}
	return nil
}

func (s *queuedLifecycleStream) Close(context.Context) error {
	*s.sender.timeline = append(*s.sender.timeline, "close")
	if s.sender.failClose {
		return errors.New("stream close failed")
	}
	return nil
}

func TestDeliveryLifecycleFailureReplaysCompleteMixedRoundWithoutGateway(t *testing.T) {
	t.Parallel()

	mixedRound := []conversation.ModelMessage{
		{
			Role:    "assistant",
			Content: conversation.NewTextContent("calling read_media"),
			ToolCalls: []conversation.ToolCall{{
				ID:   "read-1",
				Type: "function",
				Function: conversation.ToolCallFunction{
					Name:      "read_media",
					Arguments: `{"path":"image.png"}`,
				},
			}},
		},
		{Role: "tool", Content: conversation.NewTextContent("read_media closed")},
		{Role: "user", Content: json.RawMessage(`[{"type":"image","url":"data:image/png;base64,aW1hZ2U="}]`)},
		{Role: "assistant", Content: conversation.NewTextContent("final answer after image")},
	}
	replayed := make([]messagepkg.Message, 0, 3)
	for i, modelMessage := range []conversation.ModelMessage{mixedRound[0], mixedRound[1], mixedRound[3]} {
		content, err := json.Marshal(modelMessage)
		if err != nil {
			t.Fatalf("marshal replay message %d: %v", i, err)
		}
		replayed = append(replayed, messagepkg.Message{Role: modelMessage.Role, Content: content})
	}

	for _, tc := range []struct {
		name      string
		configure func(*queuedLifecycleSender, bool)
	}{
		{
			name: "final",
			configure: func(sender *queuedLifecycleSender, fail bool) {
				sender.failFinal = fail
			},
		},
		{
			name: "completed status",
			configure: func(sender *queuedLifecycleSender, fail bool) {
				sender.failCompleted = fail
			},
		},
		{
			name: "close",
			configure: func(sender *queuedLifecycleSender, fail bool) {
				sender.failClose = fail
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			queries := &durableEventQueries{}
			pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
			writer := &durableHistoryWriter{queries: queries, pipeline: pipeline, replayMessages: replayed}
			processor, _, gateway := newEventDeliveryProcessor(sessionpkg.TypeChat, queries, writer, pipeline)
			gateway.resp.Messages = mixedRound
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
			timeline := make([]string, 0, 8)
			sender := &queuedLifecycleSender{timeline: &timeline}
			tc.configure(sender, true)

			if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), sender); err == nil {
				t.Fatal("first HandleInbound() error = nil, want outbound lifecycle failure")
			}
			tc.configure(sender, false)
			if err := processor.HandleInbound(deliveryContext(), deliveryConfig(), deliveryChatMessage(), sender); err != nil {
				t.Fatalf("redelivered HandleInbound() error = %v", err)
			}

			if gatewayCalls != 1 {
				t.Fatalf("gateway calls = %d, want 1 across failure and replay", gatewayCalls)
			}
			if writer.calls != 0 {
				t.Fatalf("inbound history writes = %d, want gateway-owned persistence only", writer.calls)
			}
			if !queries.deliveryIsCompleted() {
				t.Fatal("delivery was not completed after durable replay")
			}
			if len(sender.finalTexts) < 2 || sender.finalTexts[len(sender.finalTexts)-1] != "final answer after image" {
				t.Fatalf("final outputs = %#v, want replayed final answer", sender.finalTexts)
			}
		})
	}
}

func TestQueuedDeliveryCompletesAfterSuccessfulReplyLifecycle(t *testing.T) {
	assistant := []conversation.ModelMessage{{
		Role:    "assistant",
		Content: conversation.NewTextContent("done"),
	}}
	tests := []struct {
		name        string
		messages    []conversation.ModelMessage
		gatewayErr  error
		configure   func(*queuedLifecycleSender)
		wantPending bool
		wantEvents  []string
	}{
		{
			name:        "durable response failure keeps pending",
			gatewayErr:  errors.New("persist model response"),
			wantPending: true,
			wantEvents:  []string{"close"},
		},
		{
			name:     "final failure keeps pending",
			messages: assistant,
			configure: func(sender *queuedLifecycleSender) {
				sender.failFinal = true
			},
			wantPending: true,
			wantEvents:  []string{"final", "close"},
		},
		{
			name:     "completed status failure keeps pending",
			messages: assistant,
			configure: func(sender *queuedLifecycleSender) {
				sender.failCompleted = true
			},
			wantPending: true,
			wantEvents:  []string{"final", "status", "close"},
		},
		{
			name:     "close failure keeps pending",
			messages: assistant,
			configure: func(sender *queuedLifecycleSender) {
				sender.failClose = true
			},
			wantPending: true,
			wantEvents:  []string{"final", "status", "close"},
		},
		{
			name:        "output success completes last",
			messages:    assistant,
			wantPending: false,
			wantEvents:  []string{"final", "status", "close", "complete"},
		},
		{
			name:        "no output success completes explicitly",
			wantPending: false,
			wantEvents:  []string{"status", "close", "complete"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries := &durableEventQueries{}
			pipeline := pipelinepkg.NewPipeline(pipelinepkg.RenderParams{})
			baseWriter := &durableHistoryWriter{queries: queries, pipeline: pipeline}
			timeline := make([]string, 0, 4)
			writer := &orderedCompletionWriter{base: baseWriter, timeline: &timeline}
			processor, dispatcher, gateway := newQueuedEventDeliveryProcessor(t, queries, writer, pipeline)
			gateway.resp.Messages = tt.messages
			gateway.err = tt.gatewayErr
			sender := &queuedLifecycleSender{timeline: &timeline}
			if tt.configure != nil {
				tt.configure(sender)
			}
			dispatcher.MarkActive("route")

			ctx := deliveryContext()
			if err := processor.HandleInbound(ctx, deliveryConfig(), queuedDeliveryMessage(), sender); err != nil {
				t.Fatalf("enqueue HandleInbound() error = %v", err)
			}
			processor.drainQueue(ctx, "route")

			pending, _ := queries.pendingHistoryState()
			if pending != tt.wantPending {
				t.Fatalf("pending delivery = %t, want %t; events = %#v", pending, tt.wantPending, timeline)
			}
			if !reflect.DeepEqual(timeline, tt.wantEvents) {
				t.Fatalf("reply lifecycle = %#v, want %#v", timeline, tt.wantEvents)
			}
		})
	}
}
