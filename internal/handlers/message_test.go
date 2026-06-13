package handlers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/toolapproval"
)

type testFlusher struct{}

func (*testFlusher) Flush() {}

func TestParseSinceParam(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	parsed, ok, err := parseSinceParam(now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("parse RFC3339 failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected parseSinceParam ok=true")
	}
	if !parsed.Equal(now) {
		t.Fatalf("expected parsed time %s, got %s", now, parsed)
	}

	parsedEpoch, ok, err := parseSinceParam("1735689600000")
	if err != nil {
		t.Fatalf("parse epoch millis failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected epoch parse ok=true")
	}
	if parsedEpoch.UnixMilli() != 1735689600000 {
		t.Fatalf("expected parsed epoch millis 1735689600000, got %d", parsedEpoch.UnixMilli())
	}

	if _, _, err := parseSinceParam("invalid-time"); err == nil {
		t.Fatalf("expected invalid since parameter error")
	}
}

func TestParseBeforeParam(t *testing.T) {
	t.Parallel()

	if _, ok := parseBeforeParam(""); ok {
		t.Fatalf("expected empty before value to be ignored")
	}
	parsed, ok := parseBeforeParam("1735689600000")
	if !ok {
		t.Fatalf("expected epoch millis before value to parse")
	}
	if parsed.UnixMilli() != 1735689600000 {
		t.Fatalf("expected parsed epoch millis 1735689600000, got %d", parsed.UnixMilli())
	}
}

func TestMergeToolApprovalsUsesCanApproveFunction(t *testing.T) {
	t.Parallel()

	turns := []conversation.UITurn{
		{
			Role: "assistant",
			Messages: []conversation.UIMessage{
				{
					Type:       conversation.UIMessageTool,
					ToolCallID: "call-1",
				},
			},
		},
	}
	approvals := []toolapproval.Request{
		{
			ID:         "approval-1",
			ToolCallID: "call-1",
			ShortID:    7,
			Status:     toolapproval.StatusPending,
		},
	}

	mergeToolApprovals(turns, approvals, func(req toolapproval.Request) bool {
		return req.ID == "approval-2"
	})

	approval := turns[0].Messages[0].Approval
	if approval == nil {
		t.Fatal("approval metadata was not merged")
	}
	if approval.Status != toolapproval.StatusPending {
		t.Fatalf("approval status = %q, want pending", approval.Status)
	}
	if approval.CanApprove {
		t.Fatal("mergeToolApprovals ignored injected canApprove function")
	}
}

func TestWriteSSEJSON(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	writer := bufio.NewWriter(&output)
	flusher := &testFlusher{}

	if err := writeSSEJSON(writer, flusher, map[string]any{"type": "ping"}); err != nil {
		t.Fatalf("writeSSEJSON failed: %v", err)
	}
	raw := output.String()
	if !strings.HasPrefix(raw, "data: ") {
		t.Fatalf("expected SSE data prefix, got %q", raw)
	}
	if !strings.HasSuffix(raw, "\n\n") {
		t.Fatalf("expected SSE payload suffix, got %q", raw)
	}
	payloadText := strings.TrimSuffix(strings.TrimPrefix(raw, "data: "), "\n\n")
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloadText), &payload); err != nil {
		t.Fatalf("decode SSE payload failed: %v", err)
	}
	if payload["type"] != "ping" {
		t.Fatalf("expected payload type ping, got %#v", payload["type"])
	}
}
