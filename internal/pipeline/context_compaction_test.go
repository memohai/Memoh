package pipeline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestComposeContextWithArtifactsKeepsOrderedSummariesAtCoverageAnchors(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		renderedText("before", 50, "before compacted ranges"),
		renderedText("covered-a", 100, "covered by a"),
		renderedText("between", 250, "between summaries"),
		renderedText("covered-b", 300, "covered by b"),
		renderedText("after", 500, "after compacted ranges"),
	}
	trs := []TurnResponseEntry{
		{SourceMessageID: "assistant-a", RequestedAtMs: 150, Role: "assistant", Content: "covered assistant a"},
		{SourceMessageID: "assistant-b", RequestedAtMs: 350, Role: "assistant", Content: "covered assistant b"},
		{SourceMessageID: "assistant-new", RequestedAtMs: 550, Role: "assistant", Content: "new assistant"},
	}
	artifacts := []CompactionArtifact{
		{
			ID:            "artifact-a",
			Summary:       "summary a",
			AnchorStartMs: 100,
			Sources: []CompactionSource{
				{ExternalMessageID: "covered-a", CreatedAtMs: 200},
				{HistoryMessageID: "assistant-a", CreatedAtMs: 200},
			},
		},
		{
			ID:            "artifact-b",
			Summary:       "summary b",
			AnchorStartMs: 300,
			Sources: []CompactionSource{
				{ExternalMessageID: "covered-b", CreatedAtMs: 400},
				{HistoryMessageID: "assistant-b", CreatedAtMs: 400},
			},
		},
	}

	composed := ComposeContextWithArtifacts(rc, trs, artifacts)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	assertContextContents(t, composed.Messages, []string{
		messageXML("before", "before compacted ranges"),
		"<summary>\nsummary a\n</summary>",
		messageXML("between", "between summaries"),
		"<summary>\nsummary b\n</summary>",
		messageXML("after", "after compacted ranges"),
		"new assistant",
	})
}

func TestComposeContextWithArtifactsKeepsPostCompactionMutation(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		renderedText("before", 50, "before compact"),
		{
			MessageID:     "covered",
			ReceivedAtMs:  100,
			LastEventAtMs: 300,
			Content:       []RenderedContentPiece{{Type: "text", Text: messageXML("covered", "edited after compact")}},
		},
	}
	artifacts := []CompactionArtifact{{
		ID:            "artifact-a",
		Summary:       "original state",
		AnchorStartMs: 100,
		Sources:       []CompactionSource{{ExternalMessageID: "covered", CreatedAtMs: 200}},
	}}

	composed := ComposeContextWithArtifacts(rc, nil, artifacts)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	assertContextContents(t, composed.Messages, []string{
		messageXML("before", "before compact"),
		"<summary>\noriginal state\n</summary>",
		messageXML("covered", "edited after compact"),
	})
}

func TestComposeContextWithArtifactsUsesDurableAnchorInsteadOfCurrentReplayTime(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		renderedText("covered", 100, "covered source replayed early"),
		renderedText("between", 150, "persisted before artifact anchor"),
	}
	artifacts := []CompactionArtifact{{
		ID:            "artifact-a",
		Summary:       "durably anchored summary",
		AnchorStartMs: 200,
		Sources:       []CompactionSource{{ExternalMessageID: "covered", CreatedAtMs: 250}},
	}}

	composed := ComposeContextWithArtifacts(rc, nil, artifacts)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	assertContextContents(t, composed.Messages, []string{
		messageXML("between", "persisted before artifact anchor"),
		"<summary>\ndurably anchored summary\n</summary>",
	})
}

func TestComposeContextWithArtifactsKeepsRenderedMessageWithoutDurableCutoff(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{renderedText("covered", 100, "still live")}
	artifacts := []CompactionArtifact{{
		ID:      "artifact-a",
		Summary: "legacy summary",
		Sources: []CompactionSource{{ExternalMessageID: "covered"}},
	}}

	composed := ComposeContextWithArtifacts(rc, nil, artifacts)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	assertContextContents(t, composed.Messages, []string{
		"<summary>\nlegacy summary\n</summary>",
		messageXML("covered", "still live"),
	})
}

func TestComposeContextWithArtifactsIgnoresUnusableArtifactAtomically(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{renderedText("covered", 100, "must remain")}
	trs := []TurnResponseEntry{{SourceMessageID: "assistant", RequestedAtMs: 150, Role: "assistant", Content: "must also remain"}}
	artifacts := []CompactionArtifact{{
		ID:      "artifact-a",
		Sources: []CompactionSource{{ExternalMessageID: "covered", HistoryMessageID: "assistant", CreatedAtMs: 200}},
	}}

	composed := ComposeContextWithArtifacts(rc, trs, artifacts)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	assertContextContents(t, composed.Messages, []string{
		messageXML("covered", "must remain"),
		"must also remain",
	})
}

func TestComposeContextWithArtifactsPreservesIdentityForEqualSummaryText(t *testing.T) {
	t.Parallel()

	artifacts := []CompactionArtifact{
		{ID: "artifact-a", Summary: "same text", AnchorStartMs: 100},
		{ID: "artifact-b", Summary: "same text", AnchorStartMs: 200},
	}
	composed := ComposeContextWithArtifacts(nil, nil, artifacts)
	if composed == nil || len(composed.Messages) != 2 {
		t.Fatalf("composed context = %#v, want two summaries", composed)
	}
	if got := composed.Messages[0].CompactionArtifactID; got != "artifact-a" {
		t.Fatalf("first summary identity = %q, want artifact-a", got)
	}
	if got := composed.Messages[1].CompactionArtifactID; got != "artifact-b" {
		t.Fatalf("second summary identity = %q, want artifact-b", got)
	}
}

func TestComposeContextAtCursorSplitsOldAndCurrentRenderedSources(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		renderedText("old", 100, "old context"),
		renderedText("current-a", 300, "current context a"),
		renderedText("current-b", 400, "current context b"),
	}

	composed := composeContextWithArtifactsAtCursor(rc, nil, nil, 200)
	if composed == nil || len(composed.Messages) != 2 {
		t.Fatalf("composed context = %#v, want old and current sources", composed)
	}
	if composed.Messages[0].Current || !composed.Messages[1].Current {
		t.Fatalf("current source flags = %#v", composed.Messages)
	}
	if got := composed.Messages[0].RenderedMessageIDs; len(got) != 1 || got[0] != "old" {
		t.Fatalf("old source ids = %#v, want old", got)
	}
	if got := composed.Messages[1].RenderedMessageIDs; len(got) != 2 || got[0] != "current-a" || got[1] != "current-b" {
		t.Fatalf("current source ids = %#v", got)
	}
}

func TestComposeContextPreservesTurnResponseSourceIdentity(t *testing.T) {
	t.Parallel()

	composed := ComposeContext(nil, []TurnResponseEntry{{
		RequestedAtMs:   100,
		Role:            "assistant",
		Content:         "persisted response",
		SourceMessageID: "history-row",
	}})
	if composed == nil || len(composed.Messages) != 1 {
		t.Fatalf("composed context = %#v, want one turn response", composed)
	}
	if got := composed.Messages[0].SourceMessageID; got != "history-row" {
		t.Fatalf("turn response source id = %q, want history-row", got)
	}
}

func TestComposeContextAtCursorKeepsImageOnlyCurrentSource(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{{
		MessageID:    "image-only",
		ReceivedAtMs: 300,
		ImageRefs:    []ImageAttachmentRef{{ContentHash: "image-hash", Mime: "image/png"}},
	}}

	composed := composeContextWithArtifactsAtCursor(rc, nil, nil, 200)
	if composed == nil || len(composed.Messages) != 1 {
		t.Fatalf("composed context = %#v, want one image source", composed)
	}
	message := composed.Messages[0]
	if !message.Current || message.Role != "user" || message.Content != "" {
		t.Fatalf("image-only source = %#v", message)
	}
	if len(message.RenderedMessageIDs) != 1 || message.RenderedMessageIDs[0] != "image-only" {
		t.Fatalf("image-only source ids = %#v", message.RenderedMessageIDs)
	}
	if len(message.ImageRefs) != 1 || message.ImageRefs[0].ContentHash != "image-hash" {
		t.Fatalf("image-only source refs = %#v", message.ImageRefs)
	}
}

func TestComposeContextAtCursorDropsImageOnlyHistoricalPlaceholder(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{
			MessageID:    "old-image",
			ReceivedAtMs: 100,
			ImageRefs:    []ImageAttachmentRef{{ContentHash: "old-image-hash", Mime: "image/png"}},
		},
		renderedText("current", 300, "current context"),
	}

	composed := composeContextWithArtifactsAtCursor(rc, nil, nil, 200)
	if composed == nil || len(composed.Messages) != 1 {
		t.Fatalf("composed context = %#v, want only current text source", composed)
	}
	if !composed.Messages[0].Current || strings.Contains(composed.Messages[0].Content, "old-image") {
		t.Fatalf("historical image placeholder leaked into prompt: %#v", composed.Messages)
	}
}

func TestComposeContextAtCursorPreservesSeparatedCurrentSources(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		renderedText("current-a", 300, "current context a"),
		{
			MessageID:    "self",
			ReceivedAtMs: 400,
			IsSelfSent:   true,
			Content:      []RenderedContentPiece{{Type: "text", Text: "self context"}},
		},
		renderedText("current-b", 500, "current context b"),
	}
	trs := []TurnResponseEntry{{RequestedAtMs: 350, Role: "assistant", Content: "turn response"}}

	composed := composeContextWithArtifactsAtCursor(rc, trs, nil, 200)
	if composed == nil || len(composed.Messages) != 4 {
		t.Fatalf("composed context = %#v, want two current sources separated by TR and self context", composed)
	}
	if !composed.Messages[0].Current || composed.Messages[1].Current || composed.Messages[2].Current || !composed.Messages[3].Current {
		t.Fatalf("separated current source flags = %#v", composed.Messages)
	}
}

func TestLatestExternalEventMsUsesMutationTimeAndSkipsSelfSent(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{ReceivedAtMs: 100, LastEventAtMs: 400, IsSelfSent: true},
		{ReceivedAtMs: 200, LastEventAtMs: 500, IsMyself: true},
		{ReceivedAtMs: 300, LastEventAtMs: 600},
	}

	if got := LatestExternalEventMs(rc, 350); got != 600 {
		t.Fatalf("latest external event = %d, want 600", got)
	}
	if got := LatestExternalEventMs(rc[:2], 350); got != 0 {
		t.Fatalf("self-originated mutations must not count as external events, got %d", got)
	}
}

func TestEstimateMessageTokensUsesSharedCharsPerToken(t *testing.T) {
	t.Parallel()

	if got := estimateMessageTokens(ContextMessage{Role: "user", Content: "abc"}); got != 1 {
		t.Fatalf("estimateMessageTokens() = %d, want 1", got)
	}
}

func TestEstimateMessageTokensIncludesToolProviderMetadata(t *testing.T) {
	t.Parallel()

	content := func(signature string) json.RawMessage {
		raw, err := json.Marshal([]map[string]any{{
			"type":       "tool-call",
			"toolCallId": "call-1",
			"toolName":   "lookup",
			"input":      map[string]any{"query": "weather"},
			"providerMetadata": map[string]any{
				"google": map[string]any{"thoughtSignature": signature},
			},
		}})
		if err != nil {
			t.Fatalf("marshal canonical content: %v", err)
		}
		return raw
	}
	small := estimateMessageTokens(ContextMessage{Role: "assistant", RawContent: content("sig")})
	large := estimateMessageTokens(ContextMessage{Role: "assistant", RawContent: content(strings.Repeat("sig", 100))})
	if large <= small {
		t.Fatalf("metadata estimates = small:%d large:%d, want growth", small, large)
	}
}

func renderedText(id string, atMs int64, text string) RenderedSegment {
	return RenderedSegment{
		MessageID:    id,
		ReceivedAtMs: atMs,
		Content:      []RenderedContentPiece{{Type: "text", Text: messageXML(id, text)}},
	}
}

func messageXML(id, text string) string {
	return `<message id="` + id + `">` + text + `</message>`
}

func assertContextContents(t *testing.T, messages []ContextMessage, want []string) {
	t.Helper()
	got := make([]string, 0, len(messages))
	for _, message := range messages {
		got = append(got, message.Content)
	}
	if len(got) != len(want) {
		t.Fatalf("message count = %d, want %d:\ngot  %#v\nwant %#v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}
