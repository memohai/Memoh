package agentpayload

import (
	"encoding/json"
	"testing"

	"github.com/memohai/memoh/internal/agent/background"
)

// TestBackgroundTaskHasTopLevelSessionID pins the wire shape the SSE
// per-session handler routes on. Removing or nesting `session_id` in the
// helper now breaks this test loudly without pinning the whole payload shape.
func TestBackgroundTaskHasTopLevelSessionID(t *testing.T) {
	t.Parallel()

	evt := background.TaskEvent{
		Event:     background.TaskEventStarted,
		TaskID:    "task-1",
		SessionID: "sess-1",
	}
	data, err := json.Marshal(BackgroundTask(evt))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["session_id"] != "sess-1" {
		t.Fatalf("session_id = %v, want sess-1", decoded["session_id"])
	}
}
