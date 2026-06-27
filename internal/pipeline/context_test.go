package pipeline

import (
	"strings"
	"testing"
)

func TestComposeContextWithSummarySkipsCoveredReplay(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{
			MessageID:    "external-old",
			ReceivedAtMs: 100,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-old">old user</message>`}},
		},
		{
			MessageID:    "external-new",
			ReceivedAtMs: 300,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-new">new user</message>`}},
		},
	}
	trs := []TurnResponseEntry{
		{
			SourceMessageID: "row-old-assistant",
			RequestedAtMs:   200,
			Role:            "assistant",
			Content:         "old assistant",
		},
		{
			SourceMessageID: "row-new-assistant",
			RequestedAtMs:   400,
			Role:            "assistant",
			Content:         "new assistant",
		},
	}
	summary := CompactSummary{
		Text:                   "old user and assistant summarized",
		CoveredMessageIDs:      []string{"external-old"},
		CoveredMessageCutoffMs: map[string]int64{"external-old": 100},

		CoveredHistoryMessageIDs: []string{"row-old-assistant"},
	}

	composed := ComposeContextWithSummary(rc, trs, summary)
	if composed == nil {
		t.Fatal("expected composed context")
	}

	got := messageContents(composed.Messages)
	want := []string{
		"[Conversation summary]\nold user and assistant summarized",
		`<message id="external-new">new user</message>`,
		"new assistant",
	}
	if len(got) != len(want) {
		t.Fatalf("message count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}

func TestComposeContextWithSummaryReplacesCoveredGroupInPosition(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{
			MessageID:    "external-before",
			ReceivedAtMs: 50,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-before">before compact</message>`}},
		},
		{
			MessageID:    "external-covered",
			ReceivedAtMs: 100,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-covered">covered user</message>`}},
		},
		{
			MessageID:    "external-after",
			ReceivedAtMs: 300,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-after">after compact</message>`}},
		},
	}
	trs := []TurnResponseEntry{
		{
			SourceMessageID: "row-covered-assistant",
			RequestedAtMs:   200,
			Role:            "assistant",
			Content:         "covered assistant",
		},
	}
	summary := CompactSummary{
		Text:                   "covered user and assistant summarized",
		CoveredMessageIDs:      []string{"external-covered"},
		CoveredMessageCutoffMs: map[string]int64{"external-covered": 100},

		CoveredHistoryMessageIDs: []string{"row-covered-assistant"},
	}

	composed := ComposeContextWithSummary(rc, trs, summary)
	if composed == nil {
		t.Fatal("expected composed context")
	}

	got := messageContents(composed.Messages)
	want := []string{
		`<message id="external-before">before compact</message>`,
		"[Conversation summary]\ncovered user and assistant summarized",
		`<message id="external-after">after compact</message>`,
	}
	if len(got) != len(want) {
		t.Fatalf("message count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}

func TestComposeContextKeepsCoveredMessageAfterSummaryCutoff(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{
			MessageID:    "external-old",
			ReceivedAtMs: 250,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-old">edited after compact</message>`}},
		},
	}
	summary := CompactSummary{
		Text:                   "old user summarized",
		CoveredMessageIDs:      []string{"external-old"},
		CoveredMessageCutoffMs: map[string]int64{"external-old": 200},
	}

	composed := ComposeContextWithSummary(rc, nil, summary)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	got := messageContents(composed.Messages)
	want := []string{
		"[Conversation summary]\nold user summarized",
		`<message id="external-old">edited after compact</message>`,
	}
	if len(got) != len(want) {
		t.Fatalf("message count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}

func TestComposeContextAnchorsSummaryBeforePostCutoffMutation(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{
			MessageID:    "external-before",
			ReceivedAtMs: 50,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-before">before compact</message>`}},
		},
		{
			MessageID:     "external-old",
			ReceivedAtMs:  100,
			LastEventAtMs: 250,
			Content:       []RenderedContentPiece{{Type: "text", Text: `<message id="external-old">edited after compact</message>`}},
		},
	}
	summary := CompactSummary{
		Text:                   "old user summarized",
		CoveredMessageIDs:      []string{"external-old"},
		CoveredMessageCutoffMs: map[string]int64{"external-old": 200},
	}

	composed := ComposeContextWithSummary(rc, nil, summary)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	got := messageContents(composed.Messages)
	want := []string{
		`<message id="external-before">before compact</message>`,
		"[Conversation summary]\nold user summarized",
		`<message id="external-old">edited after compact</message>`,
	}
	if len(got) != len(want) {
		t.Fatalf("message count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}

func TestComposeContextKeepsCoveredMessageWithoutCutoff(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{
			MessageID:    "external-old",
			ReceivedAtMs: 100,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-old">old user</message>`}},
		},
	}
	summary := CompactSummary{
		Text:              "old user summarized",
		CoveredMessageIDs: []string{"external-old"},
	}

	composed := ComposeContextWithSummary(rc, nil, summary)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	got := messageContents(composed.Messages)
	want := []string{
		"[Conversation summary]\nold user summarized",
		`<message id="external-old">old user</message>`,
	}
	if len(got) != len(want) {
		t.Fatalf("message count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}

func TestComposeContextKeepsEditedCoveredMessageAfterSummaryCutoff(t *testing.T) {
	t.Parallel()

	ic := NewEmptyIC("sess-1")
	ic = Reduce(ic, MessageEvent{
		SessionID:    "sess-1",
		MessageID:    "external-old",
		ReceivedAtMs: 100,
		TimestampSec: 100,
		Content:      []ContentNode{{Type: "text", Text: "old"}},
		Conversation: ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})
	ic = Reduce(ic, EditEvent{
		SessionID:    "sess-1",
		MessageID:    "external-old",
		ReceivedAtMs: 250,
		TimestampSec: 250,
		Content:      []ContentNode{{Type: "text", Text: "edited after compact"}},
	})
	rc := Render(ic, RenderParams{})
	summary := CompactSummary{
		Text:                   "old summarized",
		CoveredMessageIDs:      []string{"external-old"},
		CoveredMessageCutoffMs: map[string]int64{"external-old": 200},
	}

	composed := ComposeContextWithSummary(rc, nil, summary)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	got := messageContents(composed.Messages)
	if len(got) != 2 {
		t.Fatalf("message count = %d, want summary plus edited message: %#v", len(got), got)
	}
	if !strings.Contains(got[1], "edited after compact") {
		t.Fatalf("edited message was not preserved: %#v", got)
	}
}

func TestComposeContextKeepsDeletedCoveredMessageAfterSummaryCutoff(t *testing.T) {
	t.Parallel()

	ic := NewEmptyIC("sess-1")
	ic = Reduce(ic, MessageEvent{
		SessionID:    "sess-1",
		MessageID:    "external-old",
		ReceivedAtMs: 100,
		TimestampSec: 100,
		Content:      []ContentNode{{Type: "text", Text: "old"}},
		Conversation: ConversationMeta{Channel: "telegram", ConversationType: "group"},
	})
	ic = Reduce(ic, DeleteEvent{
		SessionID:    "sess-1",
		MessageIDs:   []string{"external-old"},
		ReceivedAtMs: 250,
		TimestampSec: 250,
	})
	rc := Render(ic, RenderParams{})
	if got := LatestExternalEventMs(rc, 200); got != 250 {
		t.Fatalf("latest external event = %d, want delete event timestamp", got)
	}
	summary := CompactSummary{
		Text:                   "old summarized",
		CoveredMessageIDs:      []string{"external-old"},
		CoveredMessageCutoffMs: map[string]int64{"external-old": 200},
	}

	composed := ComposeContextWithSummary(rc, nil, summary)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	got := messageContents(composed.Messages)
	if len(got) != 2 {
		t.Fatalf("message count = %d, want summary plus deleted tombstone: %#v", len(got), got)
	}
	if !strings.Contains(got[1], `<message id="external-old"`) || !strings.Contains(got[1], "/>") {
		t.Fatalf("deleted tombstone was not preserved: %#v", got)
	}
}

func TestLatestExternalEventMsSkipsSelfSentSegments(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{ReceivedAtMs: 100, IsSelfSent: true},
		{ReceivedAtMs: 200, IsMyself: true},
		{ReceivedAtMs: 300},
	}

	if got := LatestExternalEventMs(rc, 50); got != 300 {
		t.Fatalf("latest external event = %d, want 300", got)
	}
	if got := LatestExternalEventMs(rc[:2], 50); got != 0 {
		t.Fatalf("self-sent segments should not count as external events, got %d", got)
	}
}

func TestComposeContextDoesNotDropTurnResponseByExternalIDCollision(t *testing.T) {
	t.Parallel()

	trs := []TurnResponseEntry{
		{
			SourceMessageID:   "assistant-row-1",
			ExternalMessageID: "external-old",
			RequestedAtMs:     200,
			Role:              "assistant",
			Content:           "assistant reply",
		},
	}
	summary := CompactSummary{
		Text:              "old user summarized",
		CoveredMessageIDs: []string{"external-old"},
	}

	composed := ComposeContextWithSummary(nil, trs, summary)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	got := messageContents(composed.Messages)
	want := []string{
		"[Conversation summary]\nold user summarized",
		"assistant reply",
	}
	if len(got) != len(want) {
		t.Fatalf("message count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}

func messageContents(messages []ContextMessage) []string {
	out := make([]string, 0, len(messages))
	for _, msg := range messages {
		out = append(out, msg.Content)
	}
	return out
}
