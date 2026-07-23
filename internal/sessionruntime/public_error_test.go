package sessionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
)

func TestRuntimeStateDoesNotExposePrivateRunErrors(t *testing.T) {
	t.Parallel()

	manager := testRuntimeManager(t, NewMemoryBackend(), "owner-public-error")
	if err := manager.StartRun(context.Background(), testBotID, testSessionID, testStreamID, make(chan struct{}, 1), func() {}, make(chan conversation.InjectMessage, 1)); err != nil {
		t.Fatalf("start run: %v", err)
	}
	handle := requireRunHandle(t, manager, testBotID, testSessionID, testStreamID)
	subscription, err := manager.Subscribe(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer subscription.Close()
	<-subscription.C

	const privateDetail = "SECRET postgres connection detail"
	if _, err := manager.HandleAgentEvent(context.Background(), handle, agentpkg.StreamEvent{Type: agentpkg.EventError, Error: privateDetail}); err != nil {
		t.Fatalf("handle error event: %v", err)
	}

	event := <-subscription.C
	snapshot, err := manager.Snapshot(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.CurrentRunView == nil {
		t.Fatal("snapshot has no current run")
	}
	if snapshot.CurrentRunView.ErrorCode != RuntimeErrorCodeRunFailed || snapshot.CurrentRunView.Error != runtimeRunFailedMessage {
		t.Fatalf("public run error = %#v", snapshot.CurrentRunView)
	}
	for name, value := range map[string]any{"snapshot": snapshot, "event": event} {
		data, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatalf("marshal %s: %v", name, marshalErr)
		}
		if strings.Contains(string(data), privateDetail) {
			t.Fatalf("%s leaked private runtime error: %s", name, data)
		}
	}
}

func TestRuntimeStateDoesNotExposePrivateSteerErrors(t *testing.T) {
	t.Parallel()

	snapshot := Snapshot{CurrentRunView: &CurrentRunView{
		Status: RunStatusRunning,
		Steer: &SteerState{
			Status: SteerStatusRejected,
			Error:  "SECRET redis publish detail",
		},
	}}
	sanitizeSnapshotErrors(&snapshot)
	if snapshot.CurrentRunView.Steer.ErrorCode != RuntimeErrorCodeCommandFailed || snapshot.CurrentRunView.Steer.Error != runtimeCommandFailedMessage {
		t.Fatalf("public steer error = %#v", snapshot.CurrentRunView.Steer)
	}
}

func TestRuntimeCommandResultDoesNotExposePrivateErrors(t *testing.T) {
	t.Parallel()

	result := newCommandResult(Command{ID: "command-1", Type: CommandAbort}, errors.New("SECRET redis command detail"))
	if result.ErrorCode != "runtime_command_failed" || result.Error != runtimeCommandFailedMessage {
		t.Fatalf("public command result = %#v", result)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "SECRET") {
		t.Fatalf("command result leaked private error: %s", data)
	}
}

func TestRuntimeErrorSanitizationPreservesExplicitClear(t *testing.T) {
	t.Parallel()

	status := RunStatusAborted
	code := ""
	message := ""
	event, err := sanitizeRuntimeEventErrors(Event{Delta: &RuntimeDelta{Run: &CurrentRunPatch{
		StreamID:  testStreamID,
		Status:    &status,
		ErrorCode: &code,
		Error:     &message,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if event.Delta == nil || event.Delta.Run == nil || event.Delta.Run.ErrorCode == nil || *event.Delta.Run.ErrorCode != "" || event.Delta.Run.Error == nil || *event.Delta.Run.Error != "" {
		t.Fatalf("explicit runtime error clear = %#v", event.Delta)
	}
}
