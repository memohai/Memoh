package pipeline

import "testing"

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
		Text:                     "old user and assistant summarized",
		CoveredMessageIDs:        []string{"external-old"},
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

func messageContents(messages []ContextMessage) []string {
	out := make([]string, 0, len(messages))
	for _, msg := range messages {
		out = append(out, msg.Content)
	}
	return out
}
