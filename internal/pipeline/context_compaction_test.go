package pipeline

import "testing"

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

func TestComposeContextWithArtifactsReplacesCoveredMessageAcrossClockDomains(t *testing.T) {
	t.Parallel()

	const sourceClockBase = int64(1_800_000_000_000)
	rc := RenderedContext{
		renderedText("before", sourceClockBase, "before covered message"),
		{
			MessageID:       "covered",
			ReceivedAtMs:    sourceClockBase + 100,
			EventCursor:     10,
			LastEventAtMs:   sourceClockBase + 100,
			LastEventCursor: 10,
			Content:         []RenderedContentPiece{{Type: "text", Text: messageXML("covered", "covered original")}},
		},
		renderedText("after", sourceClockBase+200, "after covered message"),
	}
	artifacts := []CompactionArtifact{{
		ID:            "artifact-a",
		Summary:       "covered summary",
		AnchorStartMs: 1_700_000_000_000,
		Sources: []CompactionSource{{
			ExternalMessageID: "covered",
			CreatedAtMs:       1_700_000_000_100,
		}},
	}}

	composed := ComposeContextWithArtifacts(rc, nil, artifacts)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	assertContextContents(t, composed.Messages, []string{
		messageXML("before", "before covered message"),
		"<summary>\ncovered summary\n</summary>",
		messageXML("after", "after covered message"),
	})
}

func TestComposeContextWithArtifactsKeepsPostCoverageMutationsAcrossClockDomains(t *testing.T) {
	t.Parallel()

	const sourceClockBase = int64(1_800_000_000_000)
	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "edit", content: messageXML("covered", "edited after coverage")},
		{name: "delete", content: `<message id="covered"/>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rc := RenderedContext{
				renderedText("before", sourceClockBase, "before covered message"),
				{
					MessageID:       "covered",
					ReceivedAtMs:    sourceClockBase + 100,
					EventCursor:     10,
					LastEventAtMs:   1_600_000_000_000,
					LastEventCursor: 30,
					Content:         []RenderedContentPiece{{Type: "text", Text: tc.content}},
				},
				renderedText("after", sourceClockBase+200, "after covered message"),
			}
			artifacts := []CompactionArtifact{{
				ID:            "artifact-a",
				Summary:       "state before mutation",
				AnchorStartMs: 1_700_000_000_000,
				Sources: []CompactionSource{{
					ExternalMessageID: "covered",
					CreatedAtMs:       1_700_000_000_100,
				}},
			}}

			composed := ComposeContextWithArtifacts(rc, nil, artifacts)
			if composed == nil {
				t.Fatal("expected composed context")
			}
			assertContextContents(t, composed.Messages, []string{
				messageXML("before", "before covered message"),
				"<summary>\nstate before mutation\n</summary>",
				tc.content + "\n" + messageXML("after", "after covered message"),
			})
		})
	}
}

func TestComposeContextWithArtifactsKeepsEqualTimeSummaryAfterCurrentUser(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{renderedText("current-user", 100, "current instruction")}
	trs := []TurnResponseEntry{
		{SourceMessageID: "middle", RequestedAtMs: 100, Role: "assistant", Content: "middle tool work"},
		{SourceMessageID: "tail", RequestedAtMs: 100, Role: "assistant", Content: "latest tail"},
	}
	artifacts := []CompactionArtifact{{
		ID:            "artifact-middle",
		Summary:       "middle summary",
		AnchorStartMs: 100,
		Sources:       []CompactionSource{{HistoryMessageID: "middle", CreatedAtMs: 100}},
	}}

	composed := ComposeContextWithArtifacts(rc, trs, artifacts)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	assertContextContents(t, composed.Messages, []string{
		messageXML("current-user", "current instruction"),
		"<summary>\nmiddle summary\n</summary>",
		"latest tail",
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

func TestMergeContextKeepsEditedMessageAtOriginalConversationPosition(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{{
		MessageID:     "question",
		ReceivedAtMs:  100,
		LastEventAtMs: 300,
		Content:       []RenderedContentPiece{{Type: "text", Text: messageXML("question", "edited question")}},
	}}
	turnResponses := []TurnResponseEntry{{RequestedAtMs: 200, Role: "assistant", Content: "answer"}}

	assertContextContents(t, MergeContext(rc, turnResponses), []string{
		messageXML("question", "edited question"),
		"answer",
	})
}

func TestComposeContextProjectionUsesDurableMidTurnInjectionPosition(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		renderedText("original", 100, "original question"),
		renderedText("injected", 150, "injected correction"),
	}
	projection := ContextHistoryProjection{
		TurnResponses: []TurnResponseEntry{
			{
				SourceMessageID: "assistant-before",
				RequestedAtMs:   300,
				Role:            "assistant",
				Content:         "assistant before injection",
				HistoryPosition: HistoryPosition{TurnPosition: 1, MessageSequence: 2},
			},
			{
				SourceMessageID: "assistant-after",
				RequestedAtMs:   301,
				Role:            "assistant",
				Content:         "assistant after injection",
				HistoryPosition: HistoryPosition{TurnPosition: 2, MessageSequence: 2},
			},
		},
		ExternalMessagePositions: map[string]HistoryPosition{
			"original": {TurnPosition: 1, MessageSequence: 1},
			"injected": {TurnPosition: 2, MessageSequence: 1},
		},
	}

	composed := ComposeContextProjection(rc, projection)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	assertContextContents(t, composed.Messages, []string{
		messageXML("original", "original question"),
		"assistant before injection",
		messageXML("injected", "injected correction"),
		"assistant after injection",
	})
}

func TestComposeContextProjectionPrefixesPartialCoverageWithoutAnchorPosition(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{renderedText("between", 200, "between covered sources")}
	projection := ContextHistoryProjection{
		CompactionArtifacts: []CompactionArtifact{{
			ID:            "artifact-partial",
			Summary:       "partial coverage summary",
			AnchorStartMs: 100,
			Sources: []CompactionSource{
				{ExternalMessageID: "covered-before-window", CreatedAtMs: 100},
				{ExternalMessageID: "covered-inside-window", CreatedAtMs: 300},
			},
		}},
		ExternalMessagePositions: map[string]HistoryPosition{
			"between":               {TurnPosition: 2, MessageSequence: 1},
			"covered-inside-window": {TurnPosition: 3, MessageSequence: 1},
		},
		WindowStartAtMs: 150,
	}

	composed := ComposeContextProjection(rc, projection)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	assertContextContents(t, composed.Messages, []string{
		"<summary>\npartial coverage summary\n</summary>",
		messageXML("between", "between covered sources"),
	})
}

func TestComposeContextProjectionReplacesInjectedArtifactTwinsOnce(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		renderedText("original", 100, "original question"),
		renderedText("injected", 150, "injected correction"),
	}
	projection := ContextHistoryProjection{
		TurnResponses: []TurnResponseEntry{
			{
				SourceMessageID: "assistant-before",
				RequestedAtMs:   300,
				Role:            "assistant",
				Content:         "assistant before injection",
				HistoryPosition: HistoryPosition{TurnPosition: 1, MessageSequence: 2},
			},
			{
				SourceMessageID: "assistant-after",
				RequestedAtMs:   301,
				Role:            "assistant",
				Content:         "assistant after injection",
				HistoryPosition: HistoryPosition{TurnPosition: 2, MessageSequence: 2},
			},
		},
		CompactionArtifacts: []CompactionArtifact{{
			ID:      "artifact-injected",
			Summary: "injected exchange summary",
			Sources: []CompactionSource{
				{HistoryMessageID: "injected-history", ExternalMessageID: "injected", CreatedAtMs: 400},
				{HistoryMessageID: "assistant-after", CreatedAtMs: 401},
			},
		}},
		HistoryMessagePositions: map[string]HistoryPosition{
			"assistant-before": {TurnPosition: 1, MessageSequence: 2},
			"injected-history": {TurnPosition: 2, MessageSequence: 1},
			"assistant-after":  {TurnPosition: 2, MessageSequence: 2},
		},
		ExternalMessagePositions: map[string]HistoryPosition{
			"original": {TurnPosition: 1, MessageSequence: 1},
			"injected": {TurnPosition: 2, MessageSequence: 1},
		},
	}

	composed := ComposeContextProjection(rc, projection)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	assertContextContents(t, composed.Messages, []string{
		messageXML("original", "original question"),
		"assistant before injection",
		"<summary>\ninjected exchange summary\n</summary>",
	})
}

func TestComposeContextWithArtifactsUsesCoveredIdentityInsteadOfCrossDomainAnchor(t *testing.T) {
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
		"<summary>\ndurably anchored summary\n</summary>",
		messageXML("between", "persisted before artifact anchor"),
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

func TestLatestExternalEventCursorUsesMutationTimeAndSkipsSelfSent(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		{ReceivedAtMs: 100, LastEventAtMs: 400, IsSelfSent: true},
		{ReceivedAtMs: 200, LastEventAtMs: 500, IsMyself: true},
		{ReceivedAtMs: 300, LastEventAtMs: 600},
	}

	if got := LatestExternalEventCursor(rc, 350); got != 600 {
		t.Fatalf("latest external event = %d, want 600", got)
	}
	if got := LatestExternalEventCursor(rc[:2], 350); got != 0 {
		t.Fatalf("self-originated mutations must not count as external events, got %d", got)
	}
}

func TestComposeContextUsesExactCoveredEventCursorForMutations(t *testing.T) {
	t.Parallel()

	const sourceClockBase = int64(1_800_000_000_000)
	tests := []struct {
		name            string
		eventCursor     int64
		lastEventCursor int64
		lastEventAtMs   int64
		coveredCursors  []int64
		content         string
		wantRaw         bool
	}{
		{
			name:            "exact latest mutation is covered",
			eventCursor:     10,
			lastEventCursor: 20,
			lastEventAtMs:   1_600_000_000_000,
			coveredCursors:  []int64{10, 20},
			content:         messageXML("covered", "edit included in summary"),
		},
		{
			name:            "later mutation is not covered",
			eventCursor:     10,
			lastEventCursor: 20,
			lastEventAtMs:   1_600_000_000_000,
			coveredCursors:  []int64{10},
			content:         messageXML("covered", "edit after summary"),
			wantRaw:         true,
		},
		{
			name:            "higher unrelated cursor is not a cutoff",
			eventCursor:     10,
			lastEventCursor: 20,
			lastEventAtMs:   1_600_000_000_000,
			coveredCursors:  []int64{30},
			content:         messageXML("covered", "out of order edit"),
			wantRaw:         true,
		},
		{
			name:            "legacy coverage cursor fails open",
			eventCursor:     10,
			lastEventCursor: 20,
			lastEventAtMs:   1_600_000_000_000,
			coveredCursors:  []int64{0},
			content:         messageXML("covered", "edit with legacy summary"),
			wantRaw:         true,
		},
		{
			name:           "timestamp only mutation fails open",
			lastEventAtMs:  1_600_000_000_000,
			coveredCursors: []int64{1_600_000_000_000},
			content:        messageXML("covered", "edit without durable cursor"),
			wantRaw:        true,
		},
		{
			name:            "unmutated legacy source remains covered",
			eventCursor:     10,
			lastEventCursor: 10,
			lastEventAtMs:   sourceClockBase,
			coveredCursors:  []int64{0},
			content:         messageXML("covered", "original state"),
		},
	}
	composers := []struct {
		name    string
		compose func(RenderedContext, CompactionArtifact) *ComposeContextResult
	}{
		{
			name: "time ordered",
			compose: func(rc RenderedContext, artifact CompactionArtifact) *ComposeContextResult {
				return ComposeContextWithArtifacts(rc, nil, []CompactionArtifact{artifact})
			},
		},
		{
			name: "durably positioned",
			compose: func(rc RenderedContext, artifact CompactionArtifact) *ComposeContextResult {
				return ComposeContextProjection(rc, ContextHistoryProjection{
					CompactionArtifacts: []CompactionArtifact{artifact},
					ExternalMessagePositions: map[string]HistoryPosition{
						"covered": {TurnPosition: 1, MessageSequence: 1},
					},
				})
			},
		},
	}
	for _, tc := range tests {
		for _, composer := range composers {
			t.Run(tc.name+"/"+composer.name, func(t *testing.T) {
				t.Parallel()

				sources := make([]CompactionSource, 0, len(tc.coveredCursors))
				for i, cursor := range tc.coveredCursors {
					sources = append(sources, CompactionSource{
						ExternalMessageID: "covered",
						CreatedAtMs:       1_700_000_000_000 + int64(i),
						EventCursor:       cursor,
					})
				}
				rc := RenderedContext{{
					MessageID:       "covered",
					ReceivedAtMs:    sourceClockBase,
					EventCursor:     tc.eventCursor,
					LastEventAtMs:   tc.lastEventAtMs,
					LastEventCursor: tc.lastEventCursor,
					Content:         []RenderedContentPiece{{Type: "text", Text: tc.content}},
				}}
				artifact := CompactionArtifact{
					ID:            "artifact-a",
					Summary:       "covered state",
					AnchorStartMs: 1_700_000_000_000,
					Sources:       sources,
				}

				composed := composer.compose(rc, artifact)
				if composed == nil {
					t.Fatal("expected composed context")
				}
				want := []string{"<summary>\ncovered state\n</summary>"}
				if tc.wantRaw {
					want = append(want, tc.content)
				}
				assertContextContents(t, composed.Messages, want)
			})
		}
	}
}

func TestComposeContextPlacesCoveredMutationSummaryAfterEqualTimeCurrentUser(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{
		renderedText("current", 100, "current instruction"),
		{
			MessageID:       "covered",
			ReceivedAtMs:    100,
			EventCursor:     10,
			LastEventAtMs:   50,
			LastEventCursor: 20,
			Content:         []RenderedContentPiece{{Type: "text", Text: messageXML("covered", "covered edit")}},
		},
	}
	artifacts := []CompactionArtifact{{
		ID:            "artifact-a",
		Summary:       "covered state",
		AnchorStartMs: 50,
		Sources: []CompactionSource{
			{ExternalMessageID: "covered", CreatedAtMs: 50, EventCursor: 10},
			{ExternalMessageID: "covered", CreatedAtMs: 51, EventCursor: 20},
		},
	}}

	composed := ComposeContextWithArtifacts(rc, nil, artifacts)
	if composed == nil {
		t.Fatal("expected composed context")
	}
	assertContextContents(t, composed.Messages, []string{
		messageXML("current", "current instruction"),
		"<summary>\ncovered state\n</summary>",
	})
}

func TestComposeContextKeepsFrontierOrderWhenOneArtifactCoversSharedMutation(t *testing.T) {
	t.Parallel()

	rc := RenderedContext{{
		MessageID:       "covered",
		ReceivedAtMs:    100,
		EventCursor:     10,
		LastEventAtMs:   200,
		LastEventCursor: 20,
		Content:         []RenderedContentPiece{{Type: "text", Text: messageXML("covered", "covered edit")}},
	}}
	artifacts := []CompactionArtifact{
		{ID: "artifact-a", Summary: "summary a", Sources: []CompactionSource{{ExternalMessageID: "covered", CreatedAtMs: 100, EventCursor: 20}}},
		{ID: "artifact-b", Summary: "summary b", Sources: []CompactionSource{{ExternalMessageID: "covered", CreatedAtMs: 100, EventCursor: 30}}},
	}
	composed := []*ComposeContextResult{
		ComposeContextWithArtifacts(rc, nil, artifacts),
		ComposeContextProjection(rc, ContextHistoryProjection{
			CompactionArtifacts: artifacts,
			ExternalMessagePositions: map[string]HistoryPosition{
				"covered": {TurnPosition: 1, MessageSequence: 1},
			},
		}),
	}
	for i, result := range composed {
		if result == nil {
			t.Fatalf("composer %d returned no context", i)
		}
		assertContextContents(t, result.Messages, []string{
			"<summary>\nsummary a\n</summary>",
			"<summary>\nsummary b\n</summary>",
		})
		if result.Messages[0].CompactionArtifactID != "artifact-a" ||
			result.Messages[1].CompactionArtifactID != "artifact-b" {
			t.Fatalf("composer %d artifact order = %q, %q; want artifact-a, artifact-b", i,
				result.Messages[0].CompactionArtifactID, result.Messages[1].CompactionArtifactID)
		}
	}
}
