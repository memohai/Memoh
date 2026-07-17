package flow

import (
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	messagepkg "github.com/memohai/memoh/internal/message"
)

// runtimeRowTracker assigns database row identities at the first model-step
// event. Each SDK step has one assistant row and an optional aggregate tool
// row; reserving the adjacent odd sequence leaves room for a synthetic tool
// closure without renumbering a later assistant row.
type runtimeRowTracker struct {
	mu          sync.Mutex
	turn        *messagepkg.RuntimeTurnReservation
	nextStepSeq int64
	steps       []*runtimeStepRows
	active      *runtimeStepRows

	terminalSyntheticRows []messagepkg.RuntimeRowReservation
	auxiliaryRows         []messagepkg.RuntimeRowReservation
	pendingLedgerRows     []messagepkg.RuntimeRowReservation
}

type runtimeStepRows struct {
	assistant messagepkg.RuntimeRowReservation
	tool      *messagepkg.RuntimeRowReservation
	discarded bool
}

func newRuntimeRowTracker(turn *messagepkg.RuntimeTurnReservation) *runtimeRowTracker {
	if turn == nil || strings.TrimSpace(turn.TurnID) == "" || turn.TurnPosition <= 0 {
		return nil
	}
	return &runtimeRowTracker{turn: turn, nextStepSeq: 2}
}

func (t *runtimeRowTracker) annotate(event *agentpkg.StreamEvent) {
	if t == nil || event == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	switch event.Type {
	case agentpkg.EventModelStepStart:
		t.flushPendingLedgerLocked(event)
		step := t.startStepLocked()
		appendRuntimeLedgerRows(event, step.assistant)
		return
	case agentpkg.EventRetry:
		if t.active != nil {
			t.active.discarded = true
			t.active = nil
		}
		event.ResetLedger = true
		event.LedgerRows = runtimeRowIdentities(t.currentLedgerRowsLocked())
		t.pendingLedgerRows = nil
		return
	}
	if !runtimeEventBelongsToAssistantRow(event.Type) {
		t.flushPendingLedgerLocked(event)
		return
	}
	stepWasNew := t.active == nil
	step := t.ensureStepLocked()
	if step == nil {
		return
	}
	if stepWasNew {
		appendRuntimeLedgerRows(event, step.assistant)
	}
	if event.Type == agentpkg.EventToolCallEnd {
		toolWasNew := step.tool == nil
		tool := step.ensureTool(t.turn)
		applyRuntimeRowsToEvent(event, step.assistant, tool)
		if toolWasNew {
			appendRuntimeLedgerRows(event, tool)
		}
		t.flushPendingLedgerLocked(event)
		return
	}
	applyRuntimeRowsToEvent(event, step.assistant)
	t.flushPendingLedgerLocked(event)
}

func (t *runtimeRowTracker) startStepLocked() *runtimeStepRows {
	step := &runtimeStepRows{assistant: t.newRow("assistant", t.nextStepSeq)}
	t.nextStepSeq += 2
	t.steps = append(t.steps, step)
	t.active = step
	return step
}

func (t *runtimeRowTracker) ensureStepLocked() *runtimeStepRows {
	if t.active != nil {
		return t.active
	}
	return t.startStepLocked()
}

// reserveInjectedRow places a mid-turn user message in the same durable
// sequence as the assistant/tool rows around it. The agent invokes the
// injection recorder on its own goroutine, so allocation shares the tracker
// mutex with stream-event annotation.
func (t *runtimeRowTracker) reserveInjectedRow() *messagepkg.RuntimeRowReservation {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	row := t.newRow("user", t.nextStepSeq)
	t.nextStepSeq++
	t.auxiliaryRows = append(t.auxiliaryRows, row)
	t.pendingLedgerRows = append(t.pendingLedgerRows, row)
	return &row
}

func (t *runtimeRowTracker) reserveTerminalSyntheticRow(role string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	row := t.newRow(strings.ToLower(strings.TrimSpace(role)), t.nextStepSeq)
	t.nextStepSeq++
	t.terminalSyntheticRows = append(t.terminalSyntheticRows, row)
	t.auxiliaryRows = append(t.auxiliaryRows, row)
	t.pendingLedgerRows = append(t.pendingLedgerRows, row)
}

func (t *runtimeRowTracker) currentLedgerRowsLocked() []messagepkg.RuntimeRowReservation {
	rows := []messagepkg.RuntimeRowReservation{t.turn.Request}
	for _, step := range t.steps {
		if step.discarded {
			continue
		}
		rows = append(rows, step.assistant)
		if step.tool != nil {
			rows = append(rows, *step.tool)
		}
	}
	rows = append(rows, t.auxiliaryRows...)
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].TurnMessageSeq < rows[j].TurnMessageSeq
	})
	return rows
}

func (t *runtimeRowTracker) flushPendingLedgerLocked(event *agentpkg.StreamEvent) {
	if len(t.pendingLedgerRows) == 0 {
		return
	}
	appendRuntimeLedgerRows(event, t.pendingLedgerRows...)
	t.pendingLedgerRows = nil
}

func (t *runtimeRowTracker) newRow(role string, seq int64) messagepkg.RuntimeRowReservation {
	return messagepkg.RuntimeRowReservation{
		MessageID:      uuid.NewString(),
		Role:           role,
		TurnID:         t.turn.TurnID,
		TurnPosition:   t.turn.TurnPosition,
		TurnMessageSeq: seq,
	}
}

func (s *runtimeStepRows) ensureTool(turn *messagepkg.RuntimeTurnReservation) messagepkg.RuntimeRowReservation {
	if s.tool == nil {
		row := messagepkg.RuntimeRowReservation{
			MessageID:      uuid.NewString(),
			Role:           "tool",
			TurnID:         turn.TurnID,
			TurnPosition:   turn.TurnPosition,
			TurnMessageSeq: s.assistant.TurnMessageSeq + 1,
		}
		s.tool = &row
	}
	return *s.tool
}

// bindTerminalRows maps the SDK's canonical assistant/tool row sequence back
// to the identities allocated while streaming. Missing tool results are the
// synthetic closure case and consume the slot reserved beside their step.
func (t *runtimeRowTracker) bindTerminalRows(messages []sdk.Message) []messagepkg.RuntimeRowReservation {
	if t == nil || len(messages) == 0 {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	rows := make([]messagepkg.RuntimeRowReservation, len(messages))
	stepIndex := 0
	syntheticIndex := 0
	for i, raw := range messages {
		role := strings.ToLower(strings.TrimSpace(string(raw.Role)))
		switch role {
		case "assistant":
			for stepIndex < len(t.steps) && t.steps[stepIndex].discarded {
				stepIndex++
			}
			var step *runtimeStepRows
			if stepIndex < len(t.steps) {
				step = t.steps[stepIndex]
			} else {
				step = t.startStepLocked()
			}
			rows[i] = step.assistant
			stepIndex++
		case "tool":
			idx := stepIndex - 1
			if idx >= 0 && idx < len(t.steps) && !t.steps[idx].discarded {
				rows[i] = t.steps[idx].ensureTool(t.turn)
			} else {
				rows[i] = t.newRow("tool", t.nextStepSeq)
				t.nextStepSeq++
			}
		case "user", "system":
			if syntheticIndex < len(t.terminalSyntheticRows) && t.terminalSyntheticRows[syntheticIndex].Role == role {
				rows[i] = t.terminalSyntheticRows[syntheticIndex]
				syntheticIndex++
			}
		}
	}
	return rows
}

func runtimeEventBelongsToAssistantRow(eventType agentpkg.StreamEventType) bool {
	switch eventType {
	case agentpkg.EventTextStart,
		agentpkg.EventTextDelta,
		agentpkg.EventTextEnd,
		agentpkg.EventReasoningStart,
		agentpkg.EventReasoningDelta,
		agentpkg.EventReasoningEnd,
		agentpkg.EventToolCallInputStart,
		agentpkg.EventToolCallStart,
		agentpkg.EventToolCallMetadata,
		agentpkg.EventToolCallProgress,
		agentpkg.EventToolCallEnd,
		agentpkg.EventToolApprovalRequest,
		agentpkg.EventUserInputRequest,
		agentpkg.EventAttachment:
		return true
	default:
		return false
	}
}

func applyRuntimeRowsToEvent(event *agentpkg.StreamEvent, rows ...messagepkg.RuntimeRowReservation) {
	if event == nil || len(rows) == 0 {
		return
	}
	primary := rows[0]
	event.StableID = primary.MessageID
	event.TurnID = primary.TurnID
	event.TurnPosition = primary.TurnPosition
	event.TurnMessageSeq = primary.TurnMessageSeq
	event.RowIdentities = make([]agentpkg.RowIdentity, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.MessageID) == "" {
			continue
		}
		event.RowIdentities = append(event.RowIdentities, agentpkg.RowIdentity{
			StableID:       row.MessageID,
			Role:           row.Role,
			TurnID:         row.TurnID,
			TurnPosition:   row.TurnPosition,
			TurnMessageSeq: row.TurnMessageSeq,
		})
	}
}

func appendRuntimeLedgerRows(event *agentpkg.StreamEvent, rows ...messagepkg.RuntimeRowReservation) {
	if event == nil || len(rows) == 0 {
		return
	}
	event.LedgerRows = append(event.LedgerRows, runtimeRowIdentities(rows)...)
}

func runtimeRowIdentities(rows []messagepkg.RuntimeRowReservation) []agentpkg.RowIdentity {
	identities := make([]agentpkg.RowIdentity, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.MessageID) == "" {
			continue
		}
		identities = append(identities, agentpkg.RowIdentity{
			StableID:       row.MessageID,
			Role:           row.Role,
			TurnID:         row.TurnID,
			TurnPosition:   row.TurnPosition,
			TurnMessageSeq: row.TurnMessageSeq,
		})
	}
	return identities
}
