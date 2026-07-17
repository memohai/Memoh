package flow

import (
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	messagepkg "github.com/memohai/memoh/internal/message"
)

func TestRuntimeRowTrackerKeepsLiveAndTerminalIdentitiesAligned(t *testing.T) {
	tracker := newRuntimeRowTracker(runtimeRowTestTurn())
	firstDelta := agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "working"}
	tracker.annotate(&firstDelta)
	if len(firstDelta.LedgerRows) != 1 || firstDelta.LedgerRows[0].StableID != firstDelta.StableID || firstDelta.LedgerRows[0].TurnMessageSeq != 2 {
		t.Fatalf("first assistant ledger rows = %#v", firstDelta.LedgerRows)
	}
	toolEnd := agentpkg.StreamEvent{Type: agentpkg.EventToolCallEnd}
	tracker.annotate(&toolEnd)
	if len(toolEnd.RowIdentities) != 2 || toolEnd.RowIdentities[0].Role != "assistant" || toolEnd.RowIdentities[1].Role != "tool" {
		t.Fatalf("live tool row identities = %#v", toolEnd.RowIdentities)
	}
	if toolEnd.RowIdentities[0].StableID != firstDelta.StableID || toolEnd.RowIdentities[0].TurnMessageSeq != 2 || toolEnd.RowIdentities[1].TurnMessageSeq != 3 {
		t.Fatalf("live tool row coordinates = %#v", toolEnd.RowIdentities)
	}
	if len(toolEnd.LedgerRows) != 1 || toolEnd.LedgerRows[0].StableID != toolEnd.RowIdentities[1].StableID {
		t.Fatalf("tool ledger rows = %#v", toolEnd.LedgerRows)
	}
	secondStep := agentpkg.StreamEvent{Type: agentpkg.EventModelStepStart}
	tracker.annotate(&secondStep)
	if len(secondStep.LedgerRows) != 1 || secondStep.LedgerRows[0].TurnMessageSeq != 4 {
		t.Fatalf("second step ledger rows = %#v", secondStep.LedgerRows)
	}
	secondDelta := agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "done"}
	tracker.annotate(&secondDelta)

	rows := tracker.bindTerminalRows([]sdk.Message{
		{Role: sdk.MessageRoleAssistant},
		{Role: sdk.MessageRoleTool},
		{Role: sdk.MessageRoleAssistant},
	})
	if len(rows) != 3 {
		t.Fatalf("terminal rows = %d, want 3", len(rows))
	}
	if rows[0].MessageID != firstDelta.StableID || rows[0].TurnMessageSeq != 2 {
		t.Fatalf("first assistant row = %#v, live event = %#v", rows[0], firstDelta)
	}
	if rows[1].Role != "tool" || rows[1].TurnMessageSeq != 3 {
		t.Fatalf("tool row = %#v", rows[1])
	}
	if rows[1].MessageID != toolEnd.RowIdentities[1].StableID {
		t.Fatalf("terminal tool row %q != live tool row %q", rows[1].MessageID, toolEnd.RowIdentities[1].StableID)
	}
	if rows[2].MessageID != secondDelta.StableID || rows[2].TurnMessageSeq != 4 {
		t.Fatalf("second assistant row = %#v, live event = %#v", rows[2], secondDelta)
	}
	for _, row := range rows {
		if row.TurnID != "11111111-1111-1111-1111-111111111111" || row.TurnPosition != 7 {
			t.Fatalf("row coordinates = %#v", row)
		}
	}
}

func TestRuntimeRowTrackerDoesNotRecycleRetriedStepIdentity(t *testing.T) {
	tracker := newRuntimeRowTracker(runtimeRowTestTurn())
	discarded := agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "discarded"}
	tracker.annotate(&discarded)
	retry := agentpkg.StreamEvent{Type: agentpkg.EventRetry}
	tracker.annotate(&retry)
	if !retry.ResetLedger || len(retry.LedgerRows) != 1 || retry.LedgerRows[0].Role != "user" || retry.LedgerRows[0].TurnMessageSeq != 1 {
		t.Fatalf("retry ledger reset = %#v", retry)
	}
	kept := agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "kept"}
	tracker.annotate(&kept)
	if len(kept.LedgerRows) != 1 || kept.LedgerRows[0].TurnMessageSeq != 4 {
		t.Fatalf("kept step ledger rows = %#v", kept.LedgerRows)
	}

	rows := tracker.bindTerminalRows([]sdk.Message{{Role: sdk.MessageRoleAssistant}})
	if len(rows) != 1 || rows[0].MessageID != kept.StableID || rows[0].TurnMessageSeq != 4 {
		t.Fatalf("retried terminal rows = %#v, kept event = %#v", rows, kept)
	}
	if rows[0].MessageID == discarded.StableID {
		t.Fatalf("retried step recycled discarded identity %q", discarded.StableID)
	}
}

func TestRuntimeRowTrackerPlacesInjectedRowsBetweenModelSteps(t *testing.T) {
	tracker := newRuntimeRowTracker(runtimeRowTestTurn())
	tracker.annotate(&agentpkg.StreamEvent{Type: agentpkg.EventModelStepStart})
	firstToolEnd := agentpkg.StreamEvent{Type: agentpkg.EventToolCallEnd}
	tracker.annotate(&firstToolEnd)

	injected := tracker.reserveInjectedRow()
	if injected == nil || injected.Role != "user" || injected.TurnMessageSeq != 4 {
		t.Fatalf("injected row = %#v, want user sequence 4", injected)
	}

	secondStep := agentpkg.StreamEvent{Type: agentpkg.EventModelStepStart}
	tracker.annotate(&secondStep)
	if len(secondStep.LedgerRows) != 2 || secondStep.LedgerRows[0].Role != "user" || secondStep.LedgerRows[0].TurnMessageSeq != 4 || secondStep.LedgerRows[1].TurnMessageSeq != 5 {
		t.Fatalf("injected user and next assistant ledger rows = %#v", secondStep.LedgerRows)
	}
	secondDelta := agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "done"}
	tracker.annotate(&secondDelta)
	rows := tracker.bindTerminalRows([]sdk.Message{
		{Role: sdk.MessageRoleAssistant},
		{Role: sdk.MessageRoleTool},
		{Role: sdk.MessageRoleAssistant},
	})

	if len(rows) != 3 {
		t.Fatalf("terminal rows = %d, want 3", len(rows))
	}
	wantSeq := []int64{2, 3, 5}
	for i, row := range rows {
		if row.TurnMessageSeq != wantSeq[i] {
			t.Fatalf("terminal row %d sequence = %d, want %d", i, row.TurnMessageSeq, wantSeq[i])
		}
	}
	if firstToolEnd.RowIdentities[1].StableID != rows[1].MessageID {
		t.Fatalf("tool identity changed between live and terminal rows")
	}
	if secondDelta.StableID != rows[2].MessageID {
		t.Fatalf("second assistant identity changed between live and terminal rows")
	}
}

func TestRuntimeRowTrackerPlacesTerminalSyntheticUserBetweenModelSteps(t *testing.T) {
	tracker := newRuntimeRowTracker(runtimeRowTestTurn())
	tracker.annotate(&agentpkg.StreamEvent{Type: agentpkg.EventModelStepStart})
	tracker.annotate(&agentpkg.StreamEvent{Type: agentpkg.EventToolCallEnd})
	tracker.reserveTerminalSyntheticRow("user")
	secondStep := agentpkg.StreamEvent{Type: agentpkg.EventModelStepStart}
	tracker.annotate(&secondStep)
	if len(secondStep.LedgerRows) != 2 || secondStep.LedgerRows[0].Role != "user" || secondStep.LedgerRows[0].TurnMessageSeq != 4 || secondStep.LedgerRows[1].TurnMessageSeq != 5 {
		t.Fatalf("synthetic user and next assistant ledger rows = %#v", secondStep.LedgerRows)
	}

	rows := tracker.bindTerminalRows([]sdk.Message{
		{Role: sdk.MessageRoleAssistant},
		{Role: sdk.MessageRoleTool},
		{Role: sdk.MessageRoleUser},
		{Role: sdk.MessageRoleAssistant},
	})
	if len(rows) != 4 {
		t.Fatalf("terminal rows = %d, want 4", len(rows))
	}
	wantRoles := []string{"assistant", "tool", "user", "assistant"}
	wantSeq := []int64{2, 3, 4, 5}
	for i, row := range rows {
		if row.Role != wantRoles[i] || row.TurnMessageSeq != wantSeq[i] || row.MessageID == "" {
			t.Fatalf("terminal row %d = %#v, want %s sequence %d", i, row, wantRoles[i], wantSeq[i])
		}
	}
}

func runtimeRowTestTurn() *messagepkg.RuntimeTurnReservation {
	return &messagepkg.RuntimeTurnReservation{
		TurnID:       "11111111-1111-1111-1111-111111111111",
		TurnPosition: 7,
		Request: messagepkg.RuntimeRowReservation{
			MessageID:      "22222222-2222-2222-2222-222222222222",
			Role:           "user",
			TurnID:         "11111111-1111-1111-1111-111111111111",
			TurnPosition:   7,
			TurnMessageSeq: 1,
		},
	}
}
