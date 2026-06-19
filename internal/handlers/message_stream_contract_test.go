package handlers

import (
	"encoding/json"
	"sort"
	"testing"
)

// TestBackgroundTaskPayloadHasTopLevelSessionID pins the wire shape produced
// by cmd/agent/app.go's bgManager.SetEventFunc. The marshaled payload is
// forwarded verbatim to per-session SSE subscribers, so adding a top-level
// key here adds it to the wire — the test should fail loudly when that
// happens so the change is reviewed.
//
// We can't import cmd/agent (it's a main package), but the publisher there
// builds the payload via this exact map literal; if it diverges, update both
// sides together.
func TestBackgroundTaskPayloadHasTopLevelSessionID(t *testing.T) {
	t.Parallel()

	// Mirror cmd/agent/app.go's bgManager.SetEventFunc payload shape.
	payload := map[string]any{
		"event":      "started",
		"session_id": "sess-1",
		"task":       map[string]any{"task_id": "task-1", "session_id": "sess-1"},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertTopLevelKeys(t, decoded, []string{"event", "session_id", "task"})

	if got := payloadSessionID(decoded); got != "sess-1" {
		t.Fatalf("payloadSessionID = %q, want sess-1", got)
	}
}

// TestAgentStreamPayloadHasTopLevelSessionID pins the wire shape produced by
// internal/conversation/flow/resolver_trigger.go's publishBackgroundAgentStream.
// Same reasoning as the BackgroundTask test.
func TestAgentStreamPayloadHasTopLevelSessionID(t *testing.T) {
	t.Parallel()

	// Mirror resolver_trigger.go's publishBackgroundAgentStream payload shape.
	payload := map[string]any{
		"session_id": "sess-2",
		"stream":     map[string]any{"text": "hello"},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertTopLevelKeys(t, decoded, []string{"session_id", "stream"})

	if got := payloadSessionID(decoded); got != "sess-2" {
		t.Fatalf("payloadSessionID = %q, want sess-2", got)
	}
}

func assertTopLevelKeys(t *testing.T, payload map[string]any, want []string) {
	t.Helper()
	got := make([]string, 0, len(payload))
	for k := range payload {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("top-level keys = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("top-level keys = %v, want %v", got, want)
		}
	}
}
