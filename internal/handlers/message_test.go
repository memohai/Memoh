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

func TestFilterUITurnPageKeepsCompleteTurns(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	turns := []conversation.UITurn{
		{ID: "turn-1", Role: "user", Timestamp: base.Add(time.Minute)},
		{ID: "turn-2", Role: "assistant", Timestamp: base.Add(2 * time.Minute)},
		{ID: "turn-3", Role: "user", Timestamp: base.Add(3 * time.Minute)},
		{ID: "turn-4", Role: "assistant", Timestamp: base.Add(4 * time.Minute)},
	}

	latest := filterUITurnPage(turns, time.Time{}, "", false, 2)
	if got := turnIDs(latest); strings.Join(got, ",") != "turn-3,turn-4" {
		t.Fatalf("latest IDs = %v", got)
	}

	before := filterUITurnPage(turns, turns[2].Timestamp, "turn-3", true, 2)
	if got := turnIDs(before); strings.Join(got, ",") != "turn-1,turn-2" {
		t.Fatalf("before IDs = %v", got)
	}

	beforeByTime := filterUITurnPage(turns, turns[2].Timestamp, "", true, 2)
	if got := turnIDs(beforeByTime); strings.Join(got, ",") != "turn-1,turn-2" {
		t.Fatalf("before by time IDs = %v", got)
	}

	beforeMissingID := filterUITurnPage(turns, turns[2].Timestamp, "missing-turn", true, 2)
	if got := turnIDs(beforeMissingID); strings.Join(got, ",") != "turn-1,turn-2" {
		t.Fatalf("before missing ID fallback IDs = %v", got)
	}
}

func turnIDs(turns []conversation.UITurn) []string {
	out := make([]string, 0, len(turns))
	for _, turn := range turns {
		out = append(out, turn.ID)
	}
	return out
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
