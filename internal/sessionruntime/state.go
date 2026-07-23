package sessionruntime

import (
	"errors"
	"fmt"
	"strings"
	"time"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
)

func runtimeDeltaForAgentEvent(event agentpkg.StreamEvent, messages []conversation.UIMessage) (RuntimeDelta, bool) {
	switch event.Type {
	case agentpkg.EventAgentEnd, agentpkg.EventAgentAbort, agentpkg.EventError, agentpkg.EventHistoryCommit:
		return RuntimeDelta{MessageUpserts: append([]conversation.UIMessage(nil), messages...)}, true
	case agentpkg.EventRetry:
		return RuntimeDelta{ResetMessages: true}, true
	case agentpkg.EventTextDelta, agentpkg.EventReasoningDelta:
		if event.Delta == "" || len(messages) == 0 {
			return RuntimeDelta{}, false
		}
		message := messages[len(messages)-1]
		return RuntimeDelta{MessageAppends: []RuntimeMessageAppend{{
			ID:      message.ID,
			Type:    message.Type,
			Content: event.Delta,
		}}}, true
	case agentpkg.EventToolCallProgress:
		if len(messages) == 0 {
			return RuntimeDelta{}, false
		}
		message := messages[len(messages)-1]
		return RuntimeDelta{ProgressAppends: []RuntimeProgressAppend{{
			ID:       message.ID,
			Progress: event.Progress,
			Input:    event.Input,
		}}}, true
	default:
		if len(messages) == 0 {
			return RuntimeDelta{}, false
		}
		return RuntimeDelta{MessageUpserts: append([]conversation.UIMessage(nil), messages...)}, true
	}
}

func runtimeRunPatch(snapshot Snapshot, status, runError, steer, lease bool) RuntimeDelta {
	run := snapshot.CurrentRunView
	if run == nil {
		return RuntimeDelta{}
	}
	updatedAt := run.UpdatedAt
	patch := &CurrentRunPatch{
		StreamID:  run.StreamID,
		UpdatedAt: &updatedAt,
	}
	if status {
		value := run.Status
		patch.Status = &value
		historyCommitted := run.HistoryCommitted
		patch.HistoryCommitted = &historyCommitted
		if run.HistoryAssistantID != "" {
			value := run.HistoryAssistantID
			patch.HistoryAssistantID = &value
		}
		canonicalReady := run.CanonicalReady
		patch.CanonicalReady = &canonicalReady
	}
	if runError {
		if run.ErrorCode != "" {
			code := run.ErrorCode
			patch.ErrorCode = &code
		}
		value := run.Error
		patch.Error = &value
	}
	if steer && run.Steer != nil {
		value := *run.Steer
		patch.Steer = &value
	}
	if lease {
		value := time.Time{}
		if run.OwnerLeaseExpiresAt != nil {
			value = *run.OwnerLeaseExpiresAt
		}
		patch.OwnerLeaseExpiresAt = &value
	}
	return RuntimeDelta{Run: patch}
}

// leaseExpired is independent of process-local control. Once the backend
// lease expires, the old owner must not revive it even if its goroutine resumes.
func (*Manager) leaseExpired(run *CurrentRunView, now time.Time) bool {
	if run == nil || !isActiveRunStatus(run.Status) || run.OwnerLeaseExpiresAt == nil || now.Before(*run.OwnerLeaseExpiresAt) {
		return false
	}
	return true
}

func isActiveRunStatus(status string) bool {
	return strings.EqualFold(status, RunStatusAdmitting) || strings.EqualFold(status, RunStatusRunning) || strings.EqualFold(status, RunStatusAborting)
}

func isEventAcceptingRunStatus(status string) bool {
	return strings.EqualFold(status, RunStatusRunning) || strings.EqualFold(status, RunStatusAborting)
}

func (m *Manager) markLostIfExpired(snapshot *Snapshot, now time.Time) bool {
	if snapshot == nil || !m.leaseExpired(snapshot.CurrentRunView, now) {
		return false
	}
	run := snapshot.CurrentRunView
	snapshot.Seq++
	snapshot.UpdatedAt = now
	run.Status = RunStatusLost
	setRuntimeRunError(run, RunStatusLost)
	run.UpdatedAt = now
	run.OwnerLeaseExpiresAt = nil
	return true
}

func streamLeaseExpiry(snapshot Snapshot, streamID string, fallback time.Time) time.Time {
	if snapshot.CurrentRunView != nil && snapshot.CurrentRunView.StreamID == streamID && snapshot.CurrentRunView.OwnerLeaseExpiresAt != nil {
		return *snapshot.CurrentRunView.OwnerLeaseExpiresAt
	}
	return fallback
}

func nonNilQueue(queue []QueuedRunView) []QueuedRunView {
	if queue != nil {
		return queue
	}
	return []QueuedRunView{}
}

func runtimeEventEpoch(event Event) string {
	if epoch := strings.TrimSpace(event.Epoch); epoch != "" {
		return epoch
	}
	if event.Snapshot != nil {
		return strings.TrimSpace(event.Snapshot.Epoch)
	}
	return ""
}

func streamRefForRun(botID, sessionID string, run *CurrentRunView) StreamRef {
	if run == nil {
		return StreamRef{}
	}
	return StreamRef{
		BotID:      strings.TrimSpace(botID),
		SessionID:  strings.TrimSpace(sessionID),
		StreamID:   strings.TrimSpace(run.StreamID),
		OwnerID:    strings.TrimSpace(run.OwnerID),
		Generation: strings.TrimSpace(run.Generation),
	}
}

func runMatchesHandle(run *CurrentRunView, handle RunHandle) bool {
	handle = handle.normalized()
	return run != nil && run.StreamID == handle.StreamID && run.Generation == handle.Generation
}

func (m *Manager) streamRefForControl(ctrl *runControl) StreamRef {
	if ctrl == nil {
		return StreamRef{}
	}
	ownerID := ""
	if m != nil && m.distributed != nil {
		ownerID = m.ownerID
	}
	return StreamRef{
		BotID:      ctrl.botID,
		SessionID:  ctrl.sessionID,
		StreamID:   ctrl.streamID,
		OwnerID:    ownerID,
		Generation: ctrl.generation,
	}
}

func normalizeRunOperation(operation *RunOperationView) (*RunOperationView, error) {
	if operation == nil {
		return nil, nil
	}
	kind := strings.ToLower(strings.TrimSpace(operation.Kind))
	replaceFrom := strings.TrimSpace(operation.ReplaceFromMessageID)
	if replaceFrom == "" {
		return nil, errors.New("runtime operation replace_from_message_id is required")
	}
	if kind != RunOperationRetry && kind != RunOperationEdit {
		return nil, fmt.Errorf("unsupported runtime operation %q", operation.Kind)
	}
	clone := &RunOperationView{
		Kind:                 kind,
		ReplaceFromMessageID: replaceFrom,
	}
	if operation.ReplacementUserTurn != nil {
		turn := *operation.ReplacementUserTurn
		turn.Attachments = append([]conversation.UIAttachment(nil), turn.Attachments...)
		clone.ReplacementUserTurn = &turn
	}
	if kind == RunOperationEdit && (clone.ReplacementUserTurn == nil || !strings.EqualFold(strings.TrimSpace(clone.ReplacementUserTurn.Role), "user")) {
		return nil, errors.New("edit runtime operation requires a replacement user turn")
	}
	return clone, nil
}

func normalizeRunAdmission(admission RunAdmissionView) (RunAdmissionView, error) {
	operation, err := normalizeRunOperation(admission.Operation)
	if err != nil {
		return RunAdmissionView{}, err
	}
	requestUserTurn, err := normalizeRequestUserTurn(admission.RequestUserTurn)
	if err != nil {
		return RunAdmissionView{}, err
	}
	resolvedDecision, err := normalizeResolvedDecision(admission.ResolvedDecision)
	if err != nil {
		return RunAdmissionView{}, err
	}
	return RunAdmissionView{
		RequestUserTurn:  requestUserTurn,
		Operation:        operation,
		ResolvedDecision: resolvedDecision,
	}, nil
}

func normalizeResolvedDecision(decision *ResolvedDecisionView) (*ResolvedDecisionView, error) {
	if decision == nil {
		return nil, nil
	}
	kind := strings.TrimSpace(decision.Kind)
	id := strings.TrimSpace(decision.ID)
	status := strings.ToLower(strings.TrimSpace(decision.Status))
	switch kind {
	case "user_input", "tool_approval":
	default:
		return nil, fmt.Errorf("runtime resolved_decision kind %q is unsupported", decision.Kind)
	}
	if id == "" {
		return nil, errors.New("runtime resolved_decision requires an id")
	}
	switch status {
	case "submitted", "canceled", "approved", "rejected":
	default:
		return nil, fmt.Errorf("runtime resolved_decision status %q is unsupported", decision.Status)
	}
	if kind == "user_input" && status != "submitted" && status != "canceled" {
		return nil, fmt.Errorf("user_input resolved_decision status %q is unsupported", decision.Status)
	}
	if kind == "tool_approval" && status != "approved" && status != "rejected" {
		return nil, fmt.Errorf("tool_approval resolved_decision status %q is unsupported", decision.Status)
	}
	return &ResolvedDecisionView{Kind: kind, ID: id, Status: status}, nil
}

// ResolvedDecisionAdmission builds the admission payload for a deferred
// ask_user / tool-approval continuation run.
func ResolvedDecisionAdmission(kind, id, status string) RunAdmissionView {
	return RunAdmissionView{
		ResolvedDecision: &ResolvedDecisionView{
			Kind:   strings.TrimSpace(kind),
			ID:     strings.TrimSpace(id),
			Status: strings.ToLower(strings.TrimSpace(status)),
		},
	}
}

// ResolvedDecisionFromCommand derives a continuation admission anchor from a
// routed decision command payload.
func ResolvedDecisionFromCommand(kind, id, commandType string, payload []byte) *ResolvedDecisionView {
	status, _, ok := commandDecisionStatus(Command{Type: commandType, Payload: payload, TargetID: id})
	if !ok {
		return nil
	}
	decision, err := normalizeResolvedDecision(&ResolvedDecisionView{Kind: kind, ID: id, Status: status})
	if err != nil {
		return nil
	}
	return decision
}

func normalizeRequestUserTurn(turn *conversation.UITurn) (*conversation.UITurn, error) {
	if turn == nil {
		return nil, nil
	}
	if !strings.EqualFold(strings.TrimSpace(turn.Role), "user") {
		return nil, errors.New("runtime request_user_turn must have role user")
	}
	clone := *turn
	clone.Role = "user"
	clone.Attachments = append([]conversation.UIAttachment(nil), turn.Attachments...)
	clone.Messages = append([]conversation.UIMessage(nil), turn.Messages...)
	return &clone, nil
}

// ApprovalDecisionStatus maps a tool-approval response decision onto the
// resolved_decision status published to runtime subscribers.
func ApprovalDecisionStatus(decision string) string {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "approve", "approved":
		return "approved"
	case "reject", "rejected":
		return "rejected"
	default:
		return ""
	}
}

func leaseRenewInterval(ttl time.Duration) time.Duration {
	interval := ttl / 3
	if interval <= 0 {
		return 100 * time.Millisecond
	}
	if interval < 10*time.Millisecond {
		return 10 * time.Millisecond
	}
	return interval
}

func runtimeReconcileInterval(ownerLeaseTTL time.Duration) time.Duration {
	interval := ownerLeaseTTL / 3
	if interval < 50*time.Millisecond {
		return 50 * time.Millisecond
	}
	if interval > 2*time.Second {
		return 2 * time.Second
	}
	return interval
}

func enqueueRuntimeEvent(ch chan Event, event Event) {
	select {
	case ch <- event:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- Event{
		Type:      EventRuntimeDropped,
		BotID:     event.BotID,
		SessionID: event.SessionID,
		Epoch:     event.Epoch,
		StreamID:  event.StreamID,
		Seq:       event.Seq,
		Message:   "runtime subscriber buffer overflow",
	}:
	default:
	}
}

func upsertUIMessage(messages []conversation.UIMessage, incoming conversation.UIMessage) []conversation.UIMessage {
	if i := uiMessageIndex(messages, incoming); i >= 0 {
		messages[i] = preserveTerminalDecision(messages[i], incoming)
		return messages
	}
	return append(messages, incoming)
}

func uiMessageIndex(messages []conversation.UIMessage, incoming conversation.UIMessage) int {
	for i := range messages {
		if incoming.Type == conversation.UIMessageTool && strings.TrimSpace(incoming.ToolCallID) != "" && messages[i].Type == conversation.UIMessageTool && messages[i].ToolCallID == incoming.ToolCallID {
			return i
		}
		if messages[i].ID == incoming.ID {
			return i
		}
	}
	return -1
}

func preserveTerminalDecision(existing, incoming conversation.UIMessage) conversation.UIMessage {
	if existing.Type != conversation.UIMessageTool || incoming.Type != conversation.UIMessageTool {
		return incoming
	}
	if existing.Approval != nil && terminalDecisionStatus(existing.Approval.Status) &&
		(incoming.Approval == nil || (sameDecisionID(existing.Approval.ApprovalID, incoming.Approval.ApprovalID) && !terminalDecisionStatus(incoming.Approval.Status))) {
		approval := *existing.Approval
		incoming.Approval = &approval
	}
	if existing.UserInput != nil && terminalDecisionStatus(existing.UserInput.Status) &&
		(incoming.UserInput == nil || (sameDecisionID(existing.UserInput.UserInputID, incoming.UserInput.UserInputID) && !terminalDecisionStatus(incoming.UserInput.Status))) {
		userInput := *existing.UserInput
		incoming.UserInput = &userInput
	}
	return incoming
}

func terminalDecisionStatus(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	return status != "" && status != "pending"
}

func sameDecisionID(left, right string) bool {
	left = strings.TrimSpace(left)
	return left != "" && left == strings.TrimSpace(right)
}

func resolvedUIMessageUpserts(run *CurrentRunView, requested []conversation.UIMessage) []conversation.UIMessage {
	if run == nil || len(requested) == 0 {
		return nil
	}
	resolved := make([]conversation.UIMessage, 0, len(requested))
	for _, message := range requested {
		if i := uiMessageIndex(run.Messages, message); i >= 0 {
			resolved = append(resolved, run.Messages[i])
		}
	}
	return resolved
}
