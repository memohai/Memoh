package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation/flow"
)

func TestLocalChannelHandlerBoundsRuntimeCommands(t *testing.T) {
	handler := &LocalChannelHandler{runtimeCommandSlots: make(chan struct{}, 1)}
	if !handler.tryAcquireRuntimeCommand() {
		t.Fatal("first runtime command slot was rejected")
	}
	if handler.tryAcquireRuntimeCommand() {
		t.Fatal("runtime command above the configured bound was accepted")
	}
	handler.releaseRuntimeCommand()
	if !handler.tryAcquireRuntimeCommand() {
		t.Fatal("released runtime command slot was not reusable")
	}
	handler.releaseRuntimeCommand()
}

const (
	runtimeContractBotID     = "11111111-1111-1111-1111-111111111111"
	runtimeContractSessionID = "22222222-2222-2222-2222-222222222222"
	runtimeContractStreamID  = "stream-runtime-contract"
)

func rawRuntimeContractEvent(t *testing.T, ev agentpkg.StreamEvent) flow.WSStreamEvent {
	t.Helper()
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal runtime event: %v", err)
	}
	return data
}

func richActiveRunWSContractScript(t *testing.T) []flow.WSStreamEvent {
	t.Helper()
	return []flow.WSStreamEvent{
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventAgentStart}),
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventReasoningDelta, Delta: "I need to inspect the workspace."}),
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "I will check the current state."}),
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{
			Type:       agentpkg.EventToolCallStart,
			ToolName:   "exec",
			ToolCallID: "call-exec",
			Input:      map[string]any{"command": "pwd"},
		}),
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{
			Type:       agentpkg.EventToolCallProgress,
			ToolName:   "exec",
			ToolCallID: "call-exec",
			Progress:   "queued",
		}),
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{
			Type:       agentpkg.EventToolCallProgress,
			ToolName:   "exec",
			ToolCallID: "call-exec",
			Progress:   map[string]any{"stdout": "/workspace\n"},
		}),
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{
			Type:       agentpkg.EventToolCallEnd,
			ToolName:   "exec",
			ToolCallID: "call-exec",
			Result:     map[string]any{"structuredContent": map[string]any{"stdout": "/workspace\n"}},
		}),
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{
			Type:       agentpkg.EventToolApprovalRequest,
			ToolName:   "exec",
			ToolCallID: "call-approval",
			Input:      map[string]any{"command": "rm -rf build"},
			ApprovalID: "approval-1",
			ShortID:    7,
			Status:     "pending",
		}),
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{
			Type:        agentpkg.EventUserInputRequest,
			ToolName:    "ask_user",
			ToolCallID:  "call-ask",
			Input:       map[string]any{"questions": []any{map[string]any{"text": "Continue?", "kind": "single_select"}}},
			UserInputID: "input-1",
			ShortID:     8,
			Status:      "pending",
			Metadata: map[string]any{
				"ui_payload": map[string]any{
					"version": 2,
					"questions": []any{map[string]any{
						"id":   "q1",
						"text": "Continue?",
						"kind": "single_select",
						"options": []any{
							map[string]any{"id": "yes", "label": "Yes"},
							map[string]any{"id": "no", "label": "No"},
						},
					}},
				},
			},
		}),
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventAgentEnd}),
	}
}

func interruptedRunWSContractScript(t *testing.T) []flow.WSStreamEvent {
	t.Helper()
	return []flow.WSStreamEvent{
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventAgentStart}),
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "partial output"}),
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventError, Error: "runtime interrupted"}),
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventAgentAbort}),
	}
}

func collectRuntimeContractWSEvents(t *testing.T, script []flow.WSStreamEvent, stopAt string) []map[string]any {
	t.Helper()

	closeWriter := make(chan struct{})
	var closeWriterOnce sync.Once
	handlerDone := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			handlerDone <- err
			return
		}
		defer func() { _ = conn.Close() }()

		writer := newWSWriter(conn)
		eventCh := make(chan flow.WSStreamEvent, len(script))
		for _, event := range script {
			eventCh <- event
		}
		close(eventCh)

		(&LocalChannelHandler{logger: slog.Default()}).forwardWSStreamEvents(
			r.Context(),
			r.Context(),
			writer,
			runtimeContractBotID,
			runtimeContractSessionID,
			runtimeContractStreamID,
			eventCh,
		)

		<-closeWriter
		writer.Close()
		handlerDone <- nil
	}))
	defer server.Close()
	defer closeWriterOnce.Do(func() { close(closeWriter) })

	client, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer func() { _ = client.Close() }()

	var events []map[string]any
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := client.SetReadDeadline(deadline); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}
		var event map[string]any
		if err := client.ReadJSON(&event); err != nil {
			t.Fatalf("read ws event: %v; events=%#v", err, events)
		}
		events = append(events, event)
		if event["type"] == stopAt {
			break
		}
	}

	closeWriterOnce.Do(func() { close(closeWriter) })
	select {
	case err := <-handlerDone:
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handler")
	}

	return events
}

func TestLocalChannelRuntimeContractForwardsRichActiveRunUIState(t *testing.T) {
	t.Parallel()

	events := collectRuntimeContractWSEvents(t, richActiveRunWSContractScript(t), "end")
	if len(events) < 8 {
		t.Fatalf("got %d events, want rich stream events: %#v", len(events), events)
	}
	if events[0]["type"] != "start" {
		t.Fatalf("first event = %#v, want start", events[0])
	}

	var reasoning, text, execTool, approvalTool, askUserTool map[string]any
	for _, event := range events {
		data, _ := event["data"].(map[string]any)
		switch {
		case data["type"] == "reasoning":
			reasoning = data
		case data["type"] == "text":
			text = data
		case data["type"] == "tool" && data["tool_call_id"] == "call-exec":
			execTool = data
		case data["type"] == "tool" && data["tool_call_id"] == "call-approval":
			approvalTool = data
		case data["type"] == "tool" && data["tool_call_id"] == "call-ask":
			askUserTool = data
		}
	}

	if reasoning["content"] != "I need to inspect the workspace." {
		t.Fatalf("reasoning = %#v", reasoning)
	}
	if text["content"] != "I will check the current state." {
		t.Fatalf("text = %#v", text)
	}
	if execTool["running"] != false {
		t.Fatalf("exec tool = %#v, want completed running=false", execTool)
	}
	if progress, _ := execTool["progress"].([]any); len(progress) != 2 {
		t.Fatalf("exec progress = %#v, want two entries", execTool["progress"])
	}
	approval, _ := approvalTool["approval"].(map[string]any)
	if approval["approval_id"] != "approval-1" || approval["can_approve"] != true {
		t.Fatalf("approval tool = %#v", approvalTool)
	}
	userInput, _ := askUserTool["user_input"].(map[string]any)
	if userInput["user_input_id"] != "input-1" || userInput["can_respond"] != true {
		t.Fatalf("ask_user tool = %#v", askUserTool)
	}
	if events[len(events)-1]["type"] != "end" {
		t.Fatalf("last event = %#v, want end", events[len(events)-1])
	}
}

func TestLocalChannelRuntimeContractForwardsInterruptedRunError(t *testing.T) {
	t.Parallel()

	events := collectRuntimeContractWSEvents(t, interruptedRunWSContractScript(t), "end")
	if len(events) != 4 {
		t.Fatalf("events = %#v, want start, partial message, error, end", events)
	}
	if events[0]["type"] != "start" || events[1]["type"] != "message" || events[2]["type"] != "error" || events[3]["type"] != "end" {
		t.Fatalf("unexpected interrupted event sequence: %#v", events)
	}
	if events[2]["message"] != "runtime interrupted" {
		t.Fatalf("error event = %#v", events[2])
	}
}
