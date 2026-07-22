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
	tracker.Annotate(&firstDelta)
	if len(firstDelta.LedgerRows) != 1 || firstDelta.LedgerRows[0].StableID != firstDelta.StableID || firstDelta.LedgerRows[0].TurnMessageSeq != 2 {
		t.Fatalf("first assistant ledger rows = %#v", firstDelta.LedgerRows)
	}
	toolEnd := agentpkg.StreamEvent{Type: agentpkg.EventToolCallEnd}
	tracker.Annotate(&toolEnd)
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
	tracker.Annotate(&secondStep)
	if len(secondStep.LedgerRows) != 1 || secondStep.LedgerRows[0].TurnMessageSeq != 4 {
		t.Fatalf("second step ledger rows = %#v", secondStep.LedgerRows)
	}
	secondDelta := agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "done"}
	tracker.Annotate(&secondDelta)

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
	tracker.Annotate(&discarded)
	retry := agentpkg.StreamEvent{Type: agentpkg.EventRetry}
	tracker.Annotate(&retry)
	if !retry.ResetLedger || len(retry.LedgerRows) != 1 || retry.LedgerRows[0].Role != "user" || retry.LedgerRows[0].TurnMessageSeq != 1 {
		t.Fatalf("retry ledger reset = %#v", retry)
	}
	kept := agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "kept"}
	tracker.Annotate(&kept)
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
	tracker.Annotate(&agentpkg.StreamEvent{Type: agentpkg.EventModelStepStart})
	firstToolEnd := agentpkg.StreamEvent{Type: agentpkg.EventToolCallEnd}
	tracker.Annotate(&firstToolEnd)

	injected := tracker.reserveInjectedRow()
	if injected == nil || injected.Role != "user" || injected.TurnMessageSeq != 4 {
		t.Fatalf("injected row = %#v, want user sequence 4", injected)
	}

	secondStep := agentpkg.StreamEvent{Type: agentpkg.EventModelStepStart}
	tracker.Annotate(&secondStep)
	if len(secondStep.LedgerRows) != 2 || secondStep.LedgerRows[0].Role != "user" || secondStep.LedgerRows[0].TurnMessageSeq != 4 || secondStep.LedgerRows[1].TurnMessageSeq != 5 {
		t.Fatalf("injected user and next assistant ledger rows = %#v", secondStep.LedgerRows)
	}
	secondDelta := agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "done"}
	tracker.Annotate(&secondDelta)
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
	tracker.Annotate(&agentpkg.StreamEvent{Type: agentpkg.EventModelStepStart})
	tracker.Annotate(&agentpkg.StreamEvent{Type: agentpkg.EventToolCallEnd})
	tracker.reserveTerminalSyntheticRow("user")
	secondStep := agentpkg.StreamEvent{Type: agentpkg.EventModelStepStart}
	tracker.Annotate(&secondStep)
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

// ACP event streams never emit EventModelStepStart, so the implicit-step
// tracker must open a new assistant row exactly where the transcript recorder
// closes a message. Each scenario replays a live event sequence and then
// binds the terminal transcript the recorder would fold from it; the
// identities must match row by row.
func TestImplicitStepRuntimeRowTrackerAlignsLiveAndTerminalIdentities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// events are annotated in order (Annotate mutates them in place).
		events []agentpkg.StreamEvent
		// terminal is the transcript the same event sequence folds into.
		terminal []sdk.Message
		// wantSeq is the expected TurnMessageSeq per terminal row; messages
		// sharing a step's aggregate tool row repeat its sequence.
		wantSeq []int64
		// liveRows maps an event index to the terminal row whose MessageID
		// must equal that event's StableID.
		liveRows map[int]int
		// liveToolRows maps a ToolCallEnd event index to the terminal row
		// whose MessageID must equal the event's tool-row identity.
		liveToolRows map[int]int
		wantSteps    int
	}{
		{
			name: "multi segment tool turns",
			events: []agentpkg.StreamEvent{
				{Type: agentpkg.EventTextDelta, Delta: "first"},
				{Type: agentpkg.EventToolCallStart, ToolCallID: "call-1", ToolName: "read"},
				{Type: agentpkg.EventToolCallEnd, ToolCallID: "call-1", ToolName: "read"},
				{Type: agentpkg.EventTextDelta, Delta: "second"},
				{Type: agentpkg.EventToolCallStart, ToolCallID: "call-2", ToolName: "exec"},
				{Type: agentpkg.EventToolCallEnd, ToolCallID: "call-2", ToolName: "exec"},
				{Type: agentpkg.EventTextDelta, Delta: "third"},
			},
			terminal: []sdk.Message{
				{Role: sdk.MessageRoleAssistant},
				{Role: sdk.MessageRoleTool},
				{Role: sdk.MessageRoleAssistant},
				{Role: sdk.MessageRoleTool},
				{Role: sdk.MessageRoleAssistant},
			},
			wantSeq:      []int64{2, 3, 4, 5, 6},
			liveRows:     map[int]int{0: 0, 3: 2, 6: 4},
			liveToolRows: map[int]int{2: 1, 5: 3},
			wantSteps:    3,
		},
		{
			name: "reasoning after text splits the message",
			events: []agentpkg.StreamEvent{
				{Type: agentpkg.EventTextDelta, Delta: "answer"},
				{Type: agentpkg.EventReasoningDelta, Delta: "thinking"},
				{Type: agentpkg.EventTextDelta, Delta: "more"},
			},
			terminal: []sdk.Message{
				{Role: sdk.MessageRoleAssistant},
				{Role: sdk.MessageRoleAssistant},
			},
			wantSeq:   []int64{2, 4},
			liveRows:  map[int]int{0: 0, 1: 1, 2: 1},
			wantSteps: 2,
		},
		{
			name: "approval before tool start flushes pending text",
			events: []agentpkg.StreamEvent{
				{Type: agentpkg.EventTextDelta, Delta: "let me check"},
				{Type: agentpkg.EventToolApprovalRequest, ToolCallID: "call-1", ToolName: "exec"},
				{Type: agentpkg.EventToolCallStart, ToolCallID: "call-1", ToolName: "exec"},
				{Type: agentpkg.EventToolCallEnd, ToolCallID: "call-1", ToolName: "exec"},
				{Type: agentpkg.EventTextDelta, Delta: "done"},
			},
			terminal: []sdk.Message{
				{Role: sdk.MessageRoleAssistant},
				{Role: sdk.MessageRoleAssistant},
				{Role: sdk.MessageRoleTool},
				{Role: sdk.MessageRoleAssistant},
			},
			wantSeq:      []int64{2, 4, 5, 6},
			liveRows:     map[int]int{0: 0, 1: 1, 2: 1, 4: 3},
			liveToolRows: map[int]int{3: 2},
			wantSteps:    3,
		},
		{
			name: "parallel tool calls share the aggregate tool row",
			events: []agentpkg.StreamEvent{
				{Type: agentpkg.EventToolCallStart, ToolCallID: "call-1", ToolName: "read"},
				{Type: agentpkg.EventToolCallStart, ToolCallID: "call-2", ToolName: "exec"},
				{Type: agentpkg.EventToolCallEnd, ToolCallID: "call-1", ToolName: "read"},
				{Type: agentpkg.EventToolCallEnd, ToolCallID: "call-2", ToolName: "exec"},
				{Type: agentpkg.EventTextDelta, Delta: "both done"},
			},
			terminal: []sdk.Message{
				{Role: sdk.MessageRoleAssistant},
				{Role: sdk.MessageRoleTool},
				{Role: sdk.MessageRoleTool},
				{Role: sdk.MessageRoleAssistant},
			},
			wantSeq:      []int64{2, 3, 3, 4},
			liveRows:     map[int]int{0: 0, 1: 0, 4: 3},
			liveToolRows: map[int]int{2: 1, 3: 2},
			wantSteps:    2,
		},
		{
			name: "text end marker does not mint a phantom step",
			events: []agentpkg.StreamEvent{
				{Type: agentpkg.EventTextDelta, Delta: "answer"},
				{Type: agentpkg.EventToolCallStart, ToolCallID: "call-1", ToolName: "read"},
				{Type: agentpkg.EventToolCallEnd, ToolCallID: "call-1", ToolName: "read"},
				{Type: agentpkg.EventTextEnd},
			},
			terminal: []sdk.Message{
				{Role: sdk.MessageRoleAssistant},
				{Role: sdk.MessageRoleTool},
			},
			wantSeq:      []int64{2, 3},
			liveRows:     map[int]int{0: 0, 3: 0},
			liveToolRows: map[int]int{2: 1},
			wantSteps:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tracker := newImplicitStepRuntimeRowTracker(runtimeRowTestTurn())
			events := make([]agentpkg.StreamEvent, len(tt.events))
			for i, ev := range tt.events {
				events[i] = ev
				tracker.Annotate(&events[i])
			}

			rows := tracker.bindTerminalRows(tt.terminal)
			if len(rows) != len(tt.terminal) {
				t.Fatalf("terminal rows = %d, want %d", len(rows), len(tt.terminal))
			}
			if len(tracker.steps) != tt.wantSteps {
				t.Fatalf("tracker steps = %d, want %d", len(tracker.steps), tt.wantSteps)
			}
			for i, row := range rows {
				if row.MessageID == "" || row.TurnMessageSeq != tt.wantSeq[i] {
					t.Fatalf("terminal row %d = %#v, want sequence %d", i, row, tt.wantSeq[i])
				}
			}
			for eventIdx, rowIdx := range tt.liveRows {
				if events[eventIdx].StableID == "" || events[eventIdx].StableID != rows[rowIdx].MessageID {
					t.Fatalf("event %d stable ID = %q, want terminal row %d identity %q",
						eventIdx, events[eventIdx].StableID, rowIdx, rows[rowIdx].MessageID)
				}
			}
			for eventIdx, rowIdx := range tt.liveToolRows {
				identities := events[eventIdx].RowIdentities
				if len(identities) != 2 || identities[1].Role != "tool" || identities[1].StableID != rows[rowIdx].MessageID {
					t.Fatalf("event %d tool identity = %#v, want terminal row %d identity %q",
						eventIdx, identities, rowIdx, rows[rowIdx].MessageID)
				}
			}
		})
	}
}
