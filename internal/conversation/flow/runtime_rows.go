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

// RuntimeRowTracker assigns database row identities at the first model-step
// event. Each SDK step has one assistant row and an optional aggregate tool
// row; reserving the adjacent odd sequence leaves room for a synthetic tool
// closure without renumbering a later assistant row.
type RuntimeRowTracker struct {
	mu          sync.Mutex
	turn        *messagepkg.RuntimeTurnReservation
	nextStepSeq int64
	steps       []*runtimeStepRows
	active      *runtimeStepRows

	// newMessageID allocates row identities; nil means uuid.NewString. It is
	// injectable so contract fixture generation can produce deterministic IDs
	// (random UUIDs would make every regenerated fixture drift).
	newMessageID func() string

	// implicitSteps selects transcript-derived step boundaries for event
	// streams that never emit EventModelStepStart (the ACP session bridge).
	// The boundary rules mirror acpclient.TranscriptRecorder's folding logic:
	// both must agree, because the live step an event is annotated with and
	// the terminal message bindTerminalRows settles it into are required to
	// be the same row.
	implicitSteps bool
	// implicit* mirror the recorder's pending-part state for the active step.
	implicitPendingText      bool
	implicitPendingReasoning bool
	implicitToolCallIDs      map[string]struct{}

	terminalSyntheticRows []messagepkg.RuntimeRowReservation
	auxiliaryRows         []messagepkg.RuntimeRowReservation
	pendingLedgerRows     []messagepkg.RuntimeRowReservation
}

type runtimeStepRows struct {
	assistant messagepkg.RuntimeRowReservation
	tool      *messagepkg.RuntimeRowReservation
	discarded bool
	// sealed marks a step whose assistant message the transcript has closed
	// (tool result emitted, or a folding flush). Only implicit-step streams
	// seal steps; the next content block must start a fresh step.
	sealed bool
}

func newRuntimeRowTracker(turn *messagepkg.RuntimeTurnReservation) *RuntimeRowTracker {
	if turn == nil || strings.TrimSpace(turn.TurnID) == "" || turn.TurnPosition <= 0 {
		return nil
	}
	return &RuntimeRowTracker{turn: turn, nextStepSeq: 2}
}

// newImplicitStepRuntimeRowTracker builds a tracker for event streams that
// never emit EventModelStepStart (the ACP session bridge). Step boundaries
// are derived from the same folding rules acpclient.TranscriptRecorder
// applies when it builds the terminal output, so a live block's stable_id is
// the identity bindTerminalRows settles the same content with.
func newImplicitStepRuntimeRowTracker(turn *messagepkg.RuntimeTurnReservation) *RuntimeRowTracker {
	tracker := newRuntimeRowTracker(turn)
	if tracker != nil {
		tracker.implicitSteps = true
	}
	return tracker
}

// NewRuntimeRowTracker builds a tracker with an explicit message-ID
// generator. It exists for consumers outside this package that replay the
// resolver's annotation step (the runtime contract fixtures in
// internal/handlers) and need deterministic row identities; production
// resolver paths keep using newRuntimeRowTracker, which defaults to
// uuid.NewString.
func NewRuntimeRowTracker(turn *messagepkg.RuntimeTurnReservation, newMessageID func() string) *RuntimeRowTracker {
	tracker := newRuntimeRowTracker(turn)
	if tracker == nil {
		return nil
	}
	if newMessageID != nil {
		tracker.newMessageID = newMessageID
	}
	return tracker
}

func (t *RuntimeRowTracker) allocMessageID() string {
	if t.newMessageID == nil {
		return uuid.NewString()
	}
	return t.newMessageID()
}

func (t *RuntimeRowTracker) Annotate(event *agentpkg.StreamEvent) {
	if t == nil || event == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	switch event.Type {
	case agentpkg.EventModelStepStart:
		t.resetImplicitPartsLocked()
		t.flushPendingLedgerLocked(event)
		step := t.startStepLocked()
		appendRuntimeLedgerRows(event, step.assistant)
		return
	case agentpkg.EventRetry:
		if t.active != nil {
			t.active.discarded = true
			t.active = nil
		}
		t.resetImplicitPartsLocked()
		event.ResetLedger = true
		event.LedgerRows = runtimeRowIdentities(t.currentLedgerRowsLocked())
		t.pendingLedgerRows = nil
		return
	}
	if !runtimeEventBelongsToAssistantRow(event.Type) {
		t.flushPendingLedgerLocked(event)
		return
	}
	step := t.active
	if t.implicitSteps && step != nil && t.implicitStepBoundaryLocked(step, event) {
		// The transcript folds everything before this event into a closed
		// assistant message, so the event must land on a fresh row.
		step.sealed = true
		step = nil
		t.resetImplicitPartsLocked()
	}
	if step == nil {
		step = t.startStepLocked()
		appendRuntimeLedgerRows(event, step.assistant)
	}
	if event.Type == agentpkg.EventToolCallEnd {
		toolWasNew := step.tool == nil
		tool := step.ensureTool(t.turn, t.allocMessageID)
		applyRuntimeRowsToEvent(event, step.assistant, tool)
		if toolWasNew {
			appendRuntimeLedgerRows(event, tool)
		}
		t.flushPendingLedgerLocked(event)
		t.applyImplicitEventLocked(step, event)
		return
	}
	applyRuntimeRowsToEvent(event, step.assistant)
	t.flushPendingLedgerLocked(event)
	t.applyImplicitEventLocked(step, event)
}

func (t *RuntimeRowTracker) startStepLocked() *runtimeStepRows {
	step := &runtimeStepRows{assistant: t.newRow("assistant", t.nextStepSeq)}
	t.nextStepSeq += 2
	t.steps = append(t.steps, step)
	t.active = step
	return step
}

// implicitStepBoundaryLocked reports whether event begins a new assistant
// message under the transcript recorder's folding rules: text after a tool
// call part, reasoning after text or a tool call part, and an unmatched
// decision request while text or reasoning is pending all flush the current
// message. On a sealed step (a tool result already closed the message) any
// new content starts a fresh message — except a late parallel ToolCallEnd,
// whose transcript flush is a no-op and whose result joins the sealed
// message's aggregate tool row.
func (t *RuntimeRowTracker) implicitStepBoundaryLocked(step *runtimeStepRows, event *agentpkg.StreamEvent) bool {
	if step.sealed {
		switch event.Type {
		case agentpkg.EventTextDelta,
			agentpkg.EventReasoningDelta,
			agentpkg.EventToolCallStart,
			agentpkg.EventToolApprovalRequest,
			agentpkg.EventUserInputRequest:
			return true
		default:
			return false
		}
	}
	switch event.Type {
	case agentpkg.EventTextDelta:
		return len(t.implicitToolCallIDs) > 0
	case agentpkg.EventReasoningDelta:
		return t.implicitPendingText || len(t.implicitToolCallIDs) > 0
	case agentpkg.EventToolApprovalRequest, agentpkg.EventUserInputRequest:
		if _, ok := t.implicitToolCallIDs[strings.TrimSpace(event.ToolCallID)]; ok {
			return false
		}
		return t.implicitPendingText || t.implicitPendingReasoning
	default:
		return false
	}
}

// applyImplicitEventLocked advances the transcript-mirror state after an
// event has been annotated to step. ToolCallEnd closes the assistant message
// in the transcript (a tool result follows), so the step is sealed; the
// other updates track the recorder's pending text/reasoning buffers and the
// tool call parts accumulated into the current message.
func (t *RuntimeRowTracker) applyImplicitEventLocked(step *runtimeStepRows, event *agentpkg.StreamEvent) {
	if !t.implicitSteps {
		return
	}
	switch event.Type {
	case agentpkg.EventTextDelta:
		t.implicitPendingText = true
	case agentpkg.EventReasoningDelta:
		t.implicitPendingReasoning = true
	case agentpkg.EventToolCallStart:
		// A tool call folds pending text/reasoning into the current message;
		// the message itself stays open.
		t.implicitPendingText = false
		t.implicitPendingReasoning = false
		t.trackImplicitToolCallLocked(event.ToolCallID)
	case agentpkg.EventToolApprovalRequest, agentpkg.EventUserInputRequest:
		// An unmatched decision request adds a tool call part to the current
		// message, mirroring the recorder's metadata attachment.
		t.trackImplicitToolCallLocked(event.ToolCallID)
	case agentpkg.EventToolCallEnd:
		step.sealed = true
		t.resetImplicitPartsLocked()
	}
}

func (t *RuntimeRowTracker) trackImplicitToolCallLocked(toolCallID string) {
	if t.implicitToolCallIDs == nil {
		t.implicitToolCallIDs = map[string]struct{}{}
	}
	t.implicitToolCallIDs[strings.TrimSpace(toolCallID)] = struct{}{}
}

func (t *RuntimeRowTracker) resetImplicitPartsLocked() {
	t.implicitPendingText = false
	t.implicitPendingReasoning = false
	t.implicitToolCallIDs = nil
}

// reserveInjectedRow places a mid-turn user message in the same durable
// sequence as the assistant/tool rows around it. The agent invokes the
// injection recorder on its own goroutine, so allocation shares the tracker
// mutex with stream-event annotation.
func (t *RuntimeRowTracker) reserveInjectedRow() *messagepkg.RuntimeRowReservation {
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

func (t *RuntimeRowTracker) reserveTerminalSyntheticRow(role string) {
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

func (t *RuntimeRowTracker) currentLedgerRowsLocked() []messagepkg.RuntimeRowReservation {
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

func (t *RuntimeRowTracker) flushPendingLedgerLocked(event *agentpkg.StreamEvent) {
	if len(t.pendingLedgerRows) == 0 {
		return
	}
	appendRuntimeLedgerRows(event, t.pendingLedgerRows...)
	t.pendingLedgerRows = nil
}

func (t *RuntimeRowTracker) newRow(role string, seq int64) messagepkg.RuntimeRowReservation {
	return messagepkg.RuntimeRowReservation{
		MessageID:      t.allocMessageID(),
		Role:           role,
		TurnID:         t.turn.TurnID,
		TurnPosition:   t.turn.TurnPosition,
		TurnMessageSeq: seq,
	}
}

func (s *runtimeStepRows) ensureTool(turn *messagepkg.RuntimeTurnReservation, newMessageID func() string) messagepkg.RuntimeRowReservation {
	if s.tool == nil {
		row := messagepkg.RuntimeRowReservation{
			MessageID:      newMessageID(),
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
func (t *RuntimeRowTracker) bindTerminalRows(messages []sdk.Message) []messagepkg.RuntimeRowReservation {
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
				rows[i] = t.steps[idx].ensureTool(t.turn, t.allocMessageID)
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
