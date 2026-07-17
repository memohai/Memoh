package pipeline

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

func TestAdaptInboundLeavesCursorAllocationToDurableIngest(t *testing.T) {
	t.Parallel()

	event := AdaptInbound(channel.InboundMessage{
		Message: channel.Message{ID: "first", Text: "one"},
	}, "session", "user", "Alice").(MessageEvent)

	if event.EventCursor != 0 {
		t.Fatalf("adapted event cursor = %d, want durable ingest to allocate it", event.EventCursor)
	}
}

func TestRenderedMessagePreservesLatestMutationCursor(t *testing.T) {
	t.Parallel()

	ic := Reduce(NewEmptyIC("session"), MessageEvent{
		SessionID:    "session",
		MessageID:    "message",
		ReceivedAtMs: 1_000,
		EventCursor:  10,
		Content:      []ContentNode{{Type: "text", Text: "before"}},
	})
	ic = Reduce(ic, EditEvent{
		SessionID:    "session",
		MessageID:    "message",
		ReceivedAtMs: 1_000,
		EventCursor:  20,
		Content:      []ContentNode{{Type: "text", Text: "after"}},
	})

	rendered := Render(ic, RenderParams{})
	if len(rendered) != 1 || rendered[0].EventCursor != 10 || rendered[0].LastEventCursor != 20 {
		t.Fatalf("rendered cursors = %#v, want creation 10 and mutation 20", rendered)
	}
}

func TestMergeContextKeepsCreationOrderAfterEqualTimeEdit(t *testing.T) {
	t.Parallel()

	ic := NewEmptyIC("session")
	ic = Reduce(ic, MessageEvent{
		SessionID: "session", MessageID: "first", ReceivedAtMs: 1_000, EventCursor: 10,
		Content: []ContentNode{{Type: "text", Text: "first before"}},
	})
	ic = Reduce(ic, MessageEvent{
		SessionID: "session", MessageID: "second", ReceivedAtMs: 1_000, EventCursor: 20,
		Content: []ContentNode{{Type: "text", Text: "second"}},
	})
	ic = Reduce(ic, EditEvent{
		SessionID: "session", MessageID: "first", ReceivedAtMs: 1_000, EventCursor: 30,
		Content: []ContentNode{{Type: "text", Text: "first after"}},
	})

	messages := MergeContext(Render(ic, RenderParams{}), nil)
	if len(messages) != 1 {
		t.Fatalf("merged messages = %#v, want one user message", messages)
	}
	first := strings.Index(messages[0].Content, "first after")
	second := strings.Index(messages[0].Content, "second")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("equal-time edit moved the first message after its peer: %s", messages[0].Content)
	}
}

func TestMergeContextCarriesLatestExternalEventCursorWithRenderedUserMessage(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{
			MessageID:       "edited-old",
			ReceivedAtMs:    1_000,
			EventCursor:     10,
			LastEventCursor: 30,
			Content:         []RenderedContentPiece{{Type: "text", Text: "edited old message"}},
		},
		{
			MessageID:       "newer",
			ReceivedAtMs:    2_000,
			EventCursor:     20,
			LastEventCursor: 20,
			Content:         []RenderedContentPiece{{Type: "text", Text: "newer message"}},
		},
	}
	messages := MergeContext(rc, []TurnResponseEntry{{RequestedAtMs: 1_500, Role: "assistant", Content: "reply"}})

	if len(messages) != 3 {
		t.Fatalf("messages = %#v, want old user, assistant, newer user", messages)
	}
	if messages[0].LatestExternalEventCursor != 30 || messages[2].LatestExternalEventCursor != 20 {
		t.Fatalf("latest external event cursors = %d and %d, want 30 and 20", messages[0].LatestExternalEventCursor, messages[2].LatestExternalEventCursor)
	}
}

func TestMergeContextTriggerCursorIgnoresSelfSentEvents(t *testing.T) {
	t.Parallel()

	messages := MergeContext(RenderedContext{
		{
			MessageID:       "external",
			ReceivedAtMs:    1_000,
			LastEventCursor: 30,
			Content:         []RenderedContentPiece{{Type: "text", Text: "external message"}},
		},
		{
			MessageID:       "self",
			ReceivedAtMs:    1_100,
			LastEventCursor: 40,
			IsSelfSent:      true,
			Content:         []RenderedContentPiece{{Type: "text", Text: "bot echo"}},
		},
	}, nil)

	if len(messages) != 1 || messages[0].LatestExternalEventCursor != 30 {
		t.Fatalf("trigger cursor = %#v, want latest external cursor 30", messages)
	}
}

func TestReplaySessionOrdersEqualTimeMutationByEventCursor(t *testing.T) {
	t.Parallel()

	p := NewPipeline(RenderParams{})
	rendered := p.ReplaySession("session", []CanonicalEvent{
		EditEvent{
			SessionID: "session", MessageID: "message", ReceivedAtMs: 1_000, EventCursor: 20,
			Content: []ContentNode{{Type: "text", Text: "after"}},
		},
		MessageEvent{
			SessionID: "session", MessageID: "message", ReceivedAtMs: 1_000, EventCursor: 10,
			Content: []ContentNode{{Type: "text", Text: "before"}},
		},
	})

	if len(rendered) != 1 || !strings.Contains(rendered[0].Content[0].Text, "after") {
		t.Fatalf("replayed mutation was lost: %#v", rendered)
	}
}

func TestLatestExternalEventCursorDistinguishesEqualConversationTimes(t *testing.T) {
	t.Parallel()

	rendered := RenderedContext{
		{MessageID: "first", ReceivedAtMs: 1_000, LastEventCursor: 10},
		{MessageID: "second", ReceivedAtMs: 1_000, LastEventCursor: 20},
	}
	if got := LatestExternalEventCursor(rendered[:1], 0); got != 10 {
		t.Fatalf("first cursor = %d, want 10", got)
	}
	if got := LatestExternalEventCursor(rendered, 10); got != 20 {
		t.Fatalf("equal-time second event cursor = %d, want 20", got)
	}
}
