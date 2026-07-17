package pipeline

import "testing"

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
