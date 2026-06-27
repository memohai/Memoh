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

func TestTrimContextSourcesByBudgetKeepsPostCompactMutationsOverFiller(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{
			MessageID:     "external-edited",
			ReceivedAtMs:  120,
			LastEventAtMs: 250,
			Content:       []RenderedContentPiece{{Type: "text", Text: `<message id="external-edited" edited="1970-01-01T00:04:10+00:00">edited after compact</message>`}},
		},
		{
			MessageID:     "external-deleted",
			ReceivedAtMs:  130,
			LastEventAtMs: 260,
			Content:       []RenderedContentPiece{{Type: "text", Text: `<message id="external-deleted"/>`}},
		},
		{
			MessageID:    "external-filler",
			ReceivedAtMs: 280,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-filler">` + strings.Repeat("budget filler ", 80) + `</message>`}},
		},
		{
			MessageID:    "external-new",
			ReceivedAtMs: 300,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-new">fresh user</message>`}},
		},
	}
	trs := []TurnResponseEntry{{
		SourceMessageID: "row-fresh-assistant",
		RequestedAtMs:   400,
		Role:            "assistant",
		Content:         "fresh assistant",
	}}
	summary := CompactSummary{
		Text:                   "covered old segment",
		CoveredMessageIDs:      []string{"external-edited", "external-deleted"},
		CoveredMessageCutoffMs: map[string]int64{"external-edited": 200, "external-deleted": 200},
	}

	trimmedRC, trimmedTRs := TrimContextSourcesByBudget(rc, trs, summary, 150)
	composed := ComposeContextWithSummary(trimmedRC, trimmedTRs, summary)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	joined := strings.Join(messageContents(composed.Messages), "\n")

	if strings.Contains(joined, "budget filler") {
		t.Fatalf("low-priority filler should be dropped: %q", joined)
	}
	for _, want := range []string{"[Conversation summary]\ncovered old segment", "edited after compact", `<message id="external-deleted"/>`, "fresh user", "fresh assistant"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("budget trim lost %q: %q", want, joined)
		}
	}
}

func TestTrimContextSourcesByBudgetPreservesSummaryAnchor(t *testing.T) {
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
			MessageID:    "external-filler",
			ReceivedAtMs: 200,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-filler">` + strings.Repeat("budget filler ", 80) + `</message>`}},
		},
		{
			MessageID:    "external-after",
			ReceivedAtMs: 300,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-after">after compact</message>`}},
		},
	}
	summary := CompactSummary{
		Text:                   "covered user summarized",
		CoveredMessageIDs:      []string{"external-covered"},
		CoveredMessageCutoffMs: map[string]int64{"external-covered": 100},
	}

	trimmedRC, trimmedTRs := TrimContextSourcesByBudget(rc, nil, summary, 150)
	composed := ComposeContextWithSummary(trimmedRC, trimmedTRs, summary)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	got := messageContents(composed.Messages)
	want := []string{
		`<message id="external-before">before compact</message>`,
		"[Conversation summary]\ncovered user summarized",
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

func TestTrimContextSourcesByBudgetKeepsTurnResponseToolPair(t *testing.T) {
	t.Parallel()

	trs := []TurnResponseEntry{
		{
			RequestedAtMs: 100,
			Role:          "assistant",
			Content:       "tool call",
			RawContent:    []byte(`[{"type":"tool-call","toolCallId":"call-1","toolName":"lookup","input":{"q":"memoh"}},{"type":"tool-call","toolCallId":"call-2","toolName":"lookup","input":{"q":"budget"}}]`),
		},
		{
			RequestedAtMs: 110,
			Role:          "tool",
			Content:       "tool result",
			RawContent:    []byte(`[{"type":"tool-result","toolCallId":"call-1","toolName":"lookup","output":{"ok":true}}]`),
		},
		{
			RequestedAtMs: 120,
			Role:          "tool",
			Content:       "second tool result",
			RawContent:    []byte(`[{"type":"tool-result","toolCallId":"call-2","toolName":"lookup","output":{"ok":true}}]`),
		},
		{
			RequestedAtMs: 200,
			Role:          "assistant",
			Content:       strings.Repeat("newer overflow ", 80),
		},
	}

	_, trimmedTRs := TrimContextSourcesByBudget(nil, trs, CompactSummary{}, 220)
	if len(trimmedTRs) != 3 {
		t.Fatalf("expected paired assistant/tool retained, got %#v", trimmedTRs)
	}
	if trimmedTRs[0].Role != "assistant" || trimmedTRs[1].Role != "tool" || trimmedTRs[2].Role != "tool" {
		t.Fatalf("unexpected retained roles: %#v", trimmedTRs)
	}
}

func TestTrimContextSourcesByBudgetIgnoresUnmatchedToolResultWhenKeepingPair(t *testing.T) {
	t.Parallel()

	trs := []TurnResponseEntry{
		{
			RequestedAtMs: 100,
			Role:          "assistant",
			Content:       "tool call",
			RawContent:    []byte(`[{"type":"tool-call","toolCallId":"call-1","toolName":"lookup","input":{"q":"memoh"}}]`),
		},
		{
			RequestedAtMs: 110,
			Role:          "tool",
			Content:       "matching tool result",
			RawContent:    []byte(`[{"type":"tool-result","toolCallId":"call-1","toolName":"lookup","output":{"ok":true}}]`),
		},
		{
			RequestedAtMs: 120,
			Role:          "tool",
			Content:       strings.Repeat("unmatched orphan tool result ", 80),
			RawContent:    []byte(`[{"type":"tool-result","toolCallId":"call-old","toolName":"lookup","output":{"ok":true}}]`),
		},
		{
			RequestedAtMs: 200,
			Role:          "assistant",
			Content:       strings.Repeat("newer overflow ", 800),
		},
	}

	_, trimmedTRs := TrimContextSourcesByBudget(nil, trs, CompactSummary{}, 500)
	if len(trimmedTRs) != 2 {
		t.Fatalf("expected matching assistant/tool pair only, got %#v", trimmedTRs)
	}
	if trimmedTRs[0].Role != "assistant" || trimmedTRs[1].Role != "tool" {
		t.Fatalf("unexpected retained roles: %#v", trimmedTRs)
	}
	if strings.Contains(trimmedTRs[1].Content, "unmatched orphan") {
		t.Fatalf("unmatched orphan tool result was retained: %#v", trimmedTRs)
	}
}

func TestDropOrphanTurnResponseToolsDropsDuplicateToolResultForConsumedCallID(t *testing.T) {
	t.Parallel()

	trs := []TurnResponseEntry{
		{
			RequestedAtMs: 100,
			Role:          "assistant",
			Content:       "tool call",
			RawContent:    []byte(`[{"type":"tool-call","toolCallId":"call-1","toolName":"lookup","input":{"q":"memoh"}}]`),
		},
		{
			RequestedAtMs: 110,
			Role:          "tool",
			Content:       "matching tool result",
			RawContent:    []byte(`[{"type":"tool-result","toolCallId":"call-1","toolName":"lookup","output":{"ok":true}}]`),
		},
		{
			RequestedAtMs: 120,
			Role:          "tool",
			Content:       "duplicate stale tool result",
			RawContent:    []byte(`[{"type":"tool-result","toolCallId":"call-1","toolName":"lookup","output":{"ok":false}}]`),
		},
	}

	trimmedTRs := dropOrphanTurnResponseTools(trs)
	if len(trimmedTRs) != 2 {
		t.Fatalf("expected assistant plus first matching tool only, got %#v", trimmedTRs)
	}
	if trimmedTRs[0].Role != "assistant" || trimmedTRs[1].Role != "tool" {
		t.Fatalf("unexpected retained roles: %#v", trimmedTRs)
	}
	if strings.Contains(trimmedTRs[1].Content, "duplicate stale") {
		t.Fatalf("duplicate tool result was retained: %#v", trimmedTRs)
	}
}

func TestTrimContextSourcesByBudgetForceKeepsOversizedPostCompactMutation(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{
			MessageID:     "external-edited",
			ReceivedAtMs:  120,
			LastEventAtMs: 250,
			Content:       []RenderedContentPiece{{Type: "text", Text: `<message id="external-edited">` + strings.Repeat("edited after compact ", 80) + `</message>`}},
		},
	}
	summary := CompactSummary{
		Text:                   "covered old segment",
		CoveredMessageIDs:      []string{"external-edited"},
		CoveredMessageCutoffMs: map[string]int64{"external-edited": 200},
	}

	trimmedRC, trimmedTRs := TrimContextSourcesByBudget(rc, nil, summary, 20)
	composed := ComposeContextWithSummary(trimmedRC, trimmedTRs, summary)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	joined := strings.Join(messageContents(composed.Messages), "\n")
	if !strings.Contains(joined, "edited after compact") {
		t.Fatalf("oversized post-compact mutation should be force-kept: %q", joined)
	}
}

func TestTrimContextSourcesByBudgetForceKeepsLatestExternalTrigger(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{
			MessageID:    "external-old",
			ReceivedAtMs: 100,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-old">old filler</message>`}},
		},
		{
			MessageID:    "external-trigger",
			ReceivedAtMs: 300,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-trigger">` + strings.Repeat("current trigger ", 80) + `</message>`}},
		},
	}

	trimmedRC, trimmedTRs := TrimContextSourcesByBudget(rc, nil, CompactSummary{}, 1)
	composed := ComposeContextWithSummary(trimmedRC, trimmedTRs, CompactSummary{})
	if composed == nil {
		t.Fatal("expected composed context")
	}
	joined := strings.Join(messageContents(composed.Messages), "\n")
	if !strings.Contains(joined, "current trigger") {
		t.Fatalf("latest external trigger should be force-kept: %q", joined)
	}
	if strings.Contains(joined, "old filler") {
		t.Fatalf("old filler should be dropped before trigger: %q", joined)
	}
}

func TestTrimContextSourcesByBudgetForceKeepsLatestExternalTriggerByOrderOnTimestampTie(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{
			MessageID:    "external-old",
			ReceivedAtMs: 300,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-old">old same timestamp</message>`}},
		},
		{
			MessageID:    "external-trigger",
			ReceivedAtMs: 300,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="external-trigger">` + strings.Repeat("current trigger ", 80) + `</message>`}},
		},
	}

	trimmedRC, trimmedTRs := TrimContextSourcesByBudget(rc, nil, CompactSummary{}, 1)
	composed := ComposeContextWithSummary(trimmedRC, trimmedTRs, CompactSummary{})
	if composed == nil {
		t.Fatal("expected composed context")
	}
	joined := strings.Join(messageContents(composed.Messages), "\n")
	if !strings.Contains(joined, "current trigger") {
		t.Fatalf("latest same-timestamp trigger should be force-kept: %q", joined)
	}
	if strings.Contains(joined, "old same timestamp") {
		t.Fatalf("older same-timestamp message should be dropped before trigger: %q", joined)
	}
}

func TestTrimContextSourcesByBudgetAccountsForMergedRCSeparators(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{
			MessageID:    "external-old-1",
			ReceivedAtMs: 100,
			Content:      []RenderedContentPiece{{Type: "text", Text: "aa"}},
		},
		{
			MessageID:    "external-old-2",
			ReceivedAtMs: 200,
			Content:      []RenderedContentPiece{{Type: "text", Text: "bb"}},
		},
		{
			MessageID:    "external-trigger",
			ReceivedAtMs: 300,
			Content:      []RenderedContentPiece{{Type: "text", Text: "cc"}},
		},
	}

	trimmedRC, trimmedTRs := TrimContextSourcesByBudget(rc, nil, CompactSummary{}, 2)
	composed := ComposeContextWithSummary(trimmedRC, trimmedTRs, CompactSummary{})
	if composed == nil {
		t.Fatal("expected composed context")
	}
	got := messageContents(composed.Messages)
	if len(got) != 1 || got[0] != "cc" {
		t.Fatalf("budget should only retain latest trigger after RC merge overhead, got %#v", got)
	}
	if composed.EstimatedTokens > 2 {
		t.Fatalf("composed tokens = %d, want within budget", composed.EstimatedTokens)
	}
}

func messageContents(messages []ContextMessage) []string {
	out := make([]string, 0, len(messages))
	for _, msg := range messages {
		out = append(out, msg.Content)
	}
	return out
}
