package flow

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/agent/event"
	"github.com/memohai/memoh/internal/toolapproval"
)

func TestStreamChatACPFiltersFailedPendingAcrossTranscriptFlush(t *testing.T) {
	t.Parallel()

	streamed := []event.StreamEvent{
		{Type: event.ToolCallStart, ToolCallID: "write-1", ToolName: "write"},
		{
			Type:       event.ToolApprovalRequest,
			ToolCallID: "write-1",
			ToolName:   "write",
			ApprovalID: "approval-1",
			Status:     toolapproval.StatusPending,
		},
		{Type: event.ToolCallStart, ToolCallID: "read-1", ToolName: "read"},
		{Type: event.ToolCallEnd, ToolCallID: "read-1", ToolName: "read", Result: "done"},
		{
			Type:       event.ToolApprovalRequest,
			ToolCallID: "write-1",
			ToolName:   "write",
			ApprovalID: "approval-1",
			Status:     toolapproval.StatusRejected,
		},
		{Type: event.TextDelta, Delta: "finished"},
	}
	messages := &firstProjectionFailureMessageService{}
	projectionPersistCalls := 0
	pool := &acknowledgementRecordingACPPrompter{
		events: streamed,
		afterEvents: func() {
			projectionPersistCalls = messages.persistCalls
		},
	}
	resolver := &Resolver{
		messageService: messages,
		acpPool:        pool,
		botPermissions: allowWorkspaceExecFor("user-1"),
		sessionService: acpRuntimeSessionServiceForTest("user-1"),
		logger:         slog.New(slog.DiscardHandler),
	}
	req := persistedDiscussACPRequest()
	req.StreamID = "stream-1"
	eventCh := make(chan WSStreamEvent, 32)

	if err := resolver.streamACPAgentWS(context.Background(), req, eventCh, nil); err != nil {
		t.Fatalf("streamACPAgentWS() error = %v", err)
	}
	if projectionPersistCalls != 2 {
		t.Fatalf("projection persist calls = %d, want failed pending then rejected", projectionPersistCalls)
	}
	writeCalls := 0
	for _, persisted := range messages.persisted {
		for _, call := range extractAssistantToolCallParts(persistedModelMessage(t, persisted.Content)) {
			if call.ToolCallID != "write-1" {
				continue
			}
			writeCalls++
			if status := toolCallMetadataStatus(call, "approval"); status != toolapproval.StatusRejected {
				t.Fatalf("persisted write status = %q, want rejected", status)
			}
		}
	}
	if writeCalls != 1 {
		t.Fatalf("persisted write calls = %d, want only terminal projection", writeCalls)
	}
	close(eventCh)
	pendingPublished := false
	rejectedPublished := false
	for raw := range eventCh {
		var streamEvent agentpkg.StreamEvent
		if err := json.Unmarshal(raw, &streamEvent); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		if streamEvent.Type != agentpkg.EventToolApprovalRequest {
			continue
		}
		pendingPublished = pendingPublished || streamEvent.Status == toolapproval.StatusPending
		rejectedPublished = rejectedPublished || streamEvent.Status == toolapproval.StatusRejected
	}
	if pendingPublished || !rejectedPublished {
		t.Fatalf("pending/rejected published = %t/%t, want false/true", pendingPublished, rejectedPublished)
	}
}
