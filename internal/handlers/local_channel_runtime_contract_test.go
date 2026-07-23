package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/acpfeedback"
	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/apperror"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/runtimefence"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/sessionruntime"
)

func (h *LocalChannelHandler) startWSStream(baseCtx, connCtx context.Context, activeStreams *wsStreamRegistry, writer *wsWriter, botID, sessionID, streamID, logLabel string, onFinish func(), runner wsStreamRunner) {
	h.startWSStreamWithAdmissionBuilder(baseCtx, connCtx, activeStreams, writer, botID, sessionID, streamID, logLabel, onFinish, runtimefence.ActivationOptions{}, func(context.Context) (sessionruntime.RunAdmissionView, error) {
		return sessionruntime.RunAdmissionView{}, nil
	}, runner)
}

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
	runtimeContractBotID          = "11111111-1111-1111-1111-111111111111"
	runtimeContractSessionID      = "22222222-2222-2222-2222-222222222222"
	runtimeContractOtherSessionID = "33333333-3333-3333-3333-333333333333"
	runtimeContractStreamID       = "stream-runtime-contract"
	runtimeContractUserID         = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	runtimeContractOtherUserID    = "cccccccc-cccc-cccc-cccc-cccccccccccc"
)

var handlerRuntimePrefixSequence atomic.Uint64

func requireHandlerRunHandle(t *testing.T, manager *sessionruntime.Manager, botID, sessionID, streamID string) sessionruntime.RunHandle {
	t.Helper()
	ref, ok, err := manager.StreamRef(context.Background(), botID, sessionID, streamID)
	if err != nil || !ok {
		t.Fatalf("load run handle for %q = ok:%v err:%v", streamID, ok, err)
	}
	if ref.BotID != botID || ref.SessionID != sessionID {
		t.Fatalf("run handle scope = %s/%s, want %s/%s", ref.BotID, ref.SessionID, botID, sessionID)
	}
	return sessionruntime.RunHandle{BotID: ref.BotID, SessionID: ref.SessionID, StreamID: ref.StreamID, Generation: ref.Generation}
}

func uniqueHandlerRuntimePrefix(scope string) string {
	return fmt.Sprintf("memoh:test:handler-runtime:%s:%d:%d:", scope, time.Now().UnixNano(), handlerRuntimePrefixSequence.Add(1))
}

func startHandlerRuntimeManager(t *testing.T, backend sessionruntime.Backend, options sessionruntime.Options) *sessionruntime.Manager {
	t.Helper()
	manager := sessionruntime.NewManager(backend, options)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start runtime manager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	return manager
}

func handlerRuntimeOptions(ownerID string) sessionruntime.Options {
	return sessionruntime.Options{
		OwnerID:       ownerID,
		StateTTL:      time.Hour,
		OwnerLeaseTTL: time.Minute,
		Logger:        slog.Default(),
	}
}

type runtimeWireEvent map[string]any

func receiveHandlerTestResult[T any](t *testing.T, label string, ch <-chan T) T {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(5 * time.Second):
		var zero T
		t.Fatalf("timed out waiting for %s", label)
		return zero
	}
}

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
	events := richActiveRunAgentContractScript()
	out := make([]flow.WSStreamEvent, 0, len(events))
	for _, event := range events {
		out = append(out, rawRuntimeContractEvent(t, event))
	}
	return out
}

func richActiveRunAgentContractScript() []agentpkg.StreamEvent {
	return []agentpkg.StreamEvent{
		{Type: agentpkg.EventAgentStart},
		{Type: agentpkg.EventReasoningDelta, Delta: "I need to inspect the workspace."},
		{Type: agentpkg.EventTextDelta, Delta: "I will check the current state."},
		{
			Type:       agentpkg.EventToolCallStart,
			ToolName:   "exec",
			ToolCallID: "call-exec",
			Input:      map[string]any{"command": "pwd"},
		},
		{
			Type:       agentpkg.EventToolCallProgress,
			ToolName:   "exec",
			ToolCallID: "call-exec",
			Progress:   "queued",
		},
		{
			Type:       agentpkg.EventToolCallProgress,
			ToolName:   "exec",
			ToolCallID: "call-exec",
			Progress:   map[string]any{"stdout": "/workspace\n"},
		},
		{
			Type:       agentpkg.EventToolCallEnd,
			ToolName:   "exec",
			ToolCallID: "call-exec",
			Result:     map[string]any{"structuredContent": map[string]any{"stdout": "/workspace\n"}},
		},
		{
			Type:       agentpkg.EventToolApprovalRequest,
			ToolName:   "exec",
			ToolCallID: "call-approval",
			Input:      map[string]any{"command": "rm -rf build"},
			ApprovalID: "approval-1",
			ShortID:    7,
			Status:     "pending",
		},
		{
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
		},
		{Type: agentpkg.EventAgentEnd, HistoryCommitted: true},
	}
}

func richActiveRunActiveAgentContractScript() []agentpkg.StreamEvent {
	events := richActiveRunAgentContractScript()
	return events[:len(events)-1]
}

func interruptedRunWSContractScript(t *testing.T) []flow.WSStreamEvent {
	t.Helper()
	events := interruptedRunAgentContractScript()
	out := make([]flow.WSStreamEvent, 0, len(events))
	for _, event := range events {
		out = append(out, rawRuntimeContractEvent(t, event))
	}
	return out
}

func interruptedRunAgentContractScript() []agentpkg.StreamEvent {
	return []agentpkg.StreamEvent{
		{Type: agentpkg.EventAgentStart},
		{Type: agentpkg.EventTextDelta, Delta: "partial output"},
		{Type: agentpkg.EventError, Error: "runtime interrupted"},
		{Type: agentpkg.EventAgentAbort},
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

		ctx := r.Context()
		(&LocalChannelHandler{logger: slog.Default()}).forwardWSStreamEvents(
			ctx,
			ctx,
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
	if events[2]["message"] != "The agent run failed." {
		t.Fatalf("error event = %#v", events[2])
	}
	feedback, _ := events[2]["feedback"].(map[string]any)
	if feedback["code"] != string(apperror.CodeSessionRuntimeRunFailed) {
		t.Fatalf("error feedback = %#v", feedback)
	}
}

func TestLocalChannelRuntimeManagedStreamDoesNotWriteLegacyFrames(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-single-runtime-path-owner"))
	if err := manager.StartRun(
		context.Background(),
		runtimeContractBotID,
		runtimeContractSessionID,
		runtimeContractStreamID,
		make(chan struct{}, 1),
		func() {},
		make(chan conversation.InjectMessage, 1),
	); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID)
	processed := make(chan struct{})
	closeWriter := make(chan struct{})
	var closeWriterOnce sync.Once
	defer closeWriterOnce.Do(func() { close(closeWriter) })
	events := []flow.WSStreamEvent{
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventAgentStart}),
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "runtime only"}),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		writer := newWSWriter(conn)
		eventCh := make(chan flow.WSStreamEvent, len(events))
		for _, event := range events {
			eventCh <- event
		}
		close(eventCh)
		handler := &LocalChannelHandler{logger: slog.Default(), sessionRuntime: manager}
		_ = handler.forwardRuntimeWSStreamEvents(r.Context(), r.Context(), handle, eventCh, nil, nil)
		close(processed)
		<-closeWriter
		writer.Close()
	}))
	defer server.Close()

	client, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer func() { _ = client.Close() }()
	select {
	case <-processed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runtime event processing")
	}
	if err := client.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	var event map[string]any
	if err := client.ReadJSON(&event); err == nil {
		t.Fatalf("runtime-managed stream wrote legacy frame: %#v", event)
	} else {
		var netErr net.Error
		if !errors.As(err, &netErr) || !netErr.Timeout() {
			t.Fatalf("read legacy frame: %v", err)
		}
	}
	closeWriterOnce.Do(func() { close(closeWriter) })

	snapshot, err := manager.Snapshot(context.Background(), runtimeContractBotID, runtimeContractSessionID)
	if err != nil {
		t.Fatalf("runtime snapshot: %v", err)
	}
	if !runtimeSnapshotHasMessage(snapshot, conversation.UIMessageText, "", "runtime only") {
		t.Fatalf("runtime snapshot messages = %#v", snapshot.CurrentRunView)
	}
}

func TestLocalChannelRuntimeContractAggregatesActiveRunSnapshot(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-contract-owner"))

	if err := manager.StartRun(
		context.Background(),
		runtimeContractBotID,
		runtimeContractSessionID,
		runtimeContractStreamID,
		make(chan struct{}, 1),
		func() {},
		make(chan conversation.InjectMessage, 1),
	); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID)

	handler := &LocalChannelHandler{logger: slog.Default(), sessionRuntime: manager}
	activeEvents := richActiveRunActiveAgentContractScript()
	eventCh := make(chan flow.WSStreamEvent, len(activeEvents))
	for _, event := range activeEvents {
		eventCh <- rawRuntimeContractEvent(t, event)
	}
	close(eventCh)
	if err := handler.forwardRuntimeWSStreamEvents(context.Background(), context.Background(), handle, eventCh, nil, nil); err != nil {
		t.Fatalf("forward runtime events: %v", err)
	}

	snapshot, err := manager.Snapshot(context.Background(), runtimeContractBotID, runtimeContractSessionID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.StreamID != runtimeContractStreamID || snapshot.CurrentRunView.Status != sessionruntime.RunStatusRunning {
		t.Fatalf("current run = %#v", snapshot.CurrentRunView)
	}
	var text, approval, askUser bool
	for _, block := range snapshot.CurrentRunView.Messages {
		if block.Type == "text" && block.Content == "I will check the current state." {
			text = true
		}
		if block.Type == "tool" && block.ToolCallID == "call-approval" && block.Approval != nil && block.Approval.CanApprove {
			approval = true
		}
		if block.Type == "tool" && block.ToolCallID == "call-ask" && block.UserInput != nil && block.UserInput.CanRespond {
			askUser = true
		}
	}
	if !text || !approval || !askUser {
		t.Fatalf("snapshot messages = %#v, want text, pending approval, and pending user input", snapshot.CurrentRunView.Messages)
	}
}

func TestLocalChannelStartWSStreamDrainsBufferedRuntimeEventsBeforeFinish(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-drain-owner"))

	const deltaCount = 128
	script := make([]flow.WSStreamEvent, 0, deltaCount+1)
	script = append(script, rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventAgentStart}))
	for range deltaCount {
		script = append(script, rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "x"}))
	}

	handler := &LocalChannelHandler{logger: slog.Default(), sessionRuntime: manager}
	handler.startWSStream(
		context.Background(),
		context.Background(),
		newWSStreamRegistry(),
		discardWSWriter(t),
		runtimeContractBotID,
		runtimeContractSessionID,
		runtimeContractStreamID,
		"runtime drain contract",
		nil,
		func(_ context.Context, eventCh chan<- flow.WSStreamEvent, _ <-chan struct{}, _ <-chan conversation.InjectMessage) error {
			for _, event := range script {
				eventCh <- event
			}
			return nil
		},
	)

	snapshot := waitHandlerRuntimeSnapshot(t, manager, func(snapshot sessionruntime.Snapshot) bool {
		return snapshot.CurrentRunView != nil && snapshot.CurrentRunView.Status == sessionruntime.RunStatusCompleted
	})
	var text string
	for _, block := range snapshot.CurrentRunView.Messages {
		if block.Type == conversation.UIMessageText {
			text = block.Content
			break
		}
	}
	if len(text) != deltaCount {
		t.Fatalf("text length = %d, want %d; content=%q", len(text), deltaCount, text)
	}
}

func TestLocalChannelStartWSStreamFinalizesCommittedPartialAbortAfterExecutionCancellation(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-partial-abort-owner"))

	handler := &LocalChannelHandler{logger: slog.Default(), sessionRuntime: manager}
	handler.startWSStream(
		context.Background(),
		context.Background(),
		newWSStreamRegistry(),
		discardWSWriter(t),
		runtimeContractBotID,
		runtimeContractSessionID,
		"stream-partial-abort",
		"runtime partial abort contract",
		nil,
		func(ctx context.Context, eventCh chan<- flow.WSStreamEvent, abortCh <-chan struct{}, _ <-chan conversation.InjectMessage) error {
			eventCh <- rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventAgentStart})
			eventCh <- rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "partial answer"})
			<-abortCh
			eventCh <- rawRuntimeContractEvent(t, agentpkg.StreamEvent{
				Type:             agentpkg.EventAgentAbort,
				HistoryCommitted: true,
			})
			<-ctx.Done()
			return nil
		},
	)

	waitHandlerRuntimeSnapshot(t, manager, func(snapshot sessionruntime.Snapshot) bool {
		return runtimeSnapshotHasMessage(snapshot, conversation.UIMessageText, "", "partial answer")
	})
	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, "stream-partial-abort")
	if aborted, err := manager.AbortRun(context.Background(), handle); err != nil || !aborted {
		t.Fatalf("abort partial runtime = %v, %v", aborted, err)
	}

	snapshot := waitHandlerRuntimeSnapshot(t, manager, func(snapshot sessionruntime.Snapshot) bool {
		return snapshot.CurrentRunView != nil &&
			snapshot.CurrentRunView.Status == sessionruntime.RunStatusAborted &&
			snapshot.CurrentRunView.HistoryCommitted
	})
	if !runtimeSnapshotHasMessage(snapshot, conversation.UIMessageText, "", "partial answer") {
		t.Fatalf("aborted runtime lost partial output: %#v", snapshot.CurrentRunView)
	}
}

func TestLocalChannelRuntimeTerminalUpdateSurvivesExecutionCancellationDuringCommit(t *testing.T) {
	t.Parallel()

	backend := &cancelDuringRuntimeUpdateBackend{
		Backend: sessionruntime.NewMemoryBackend(),
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	manager := startHandlerRuntimeManager(t, backend, handlerRuntimeOptions("handler-terminal-cancel-race-owner"))
	if err := manager.StartRun(
		context.Background(),
		runtimeContractBotID,
		runtimeContractSessionID,
		"stream-terminal-cancel-race",
		make(chan struct{}, 1),
		func() {},
		make(chan conversation.InjectMessage, 1),
	); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, "stream-terminal-cancel-race")

	executionCtx, cancelExecution := context.WithCancel(context.Background())
	defer cancelExecution()
	backend.Arm()
	eventCh := make(chan flow.WSStreamEvent, 1)
	eventCh <- rawRuntimeContractEvent(t, agentpkg.StreamEvent{
		Type:             agentpkg.EventAgentAbort,
		HistoryCommitted: true,
	})
	close(eventCh)
	forwardDone := make(chan error, 1)
	go func() {
		handler := &LocalChannelHandler{logger: slog.Default(), sessionRuntime: manager}
		forwardDone <- handler.forwardRuntimeWSStreamEvents(executionCtx, executionCtx, handle, eventCh, nil, nil)
	}()

	select {
	case <-backend.started:
	case <-time.After(2 * time.Second):
		t.Fatal("terminal runtime update did not start")
	}
	cancelExecution()
	close(backend.release)
	if err := <-forwardDone; err != nil {
		t.Fatalf("forward terminal event across execution cancellation: %v", err)
	}

	snapshot, err := manager.Snapshot(context.Background(), runtimeContractBotID, runtimeContractSessionID)
	if err != nil {
		t.Fatalf("load terminal runtime snapshot: %v", err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Status != sessionruntime.RunStatusAborted || !snapshot.CurrentRunView.HistoryCommitted {
		t.Fatalf("terminal runtime snapshot = %#v", snapshot.CurrentRunView)
	}
}

func TestLocalChannelRuntimeLegacyTerminalUsesFinalizationContext(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-legacy-finalization-owner"))
	if err := manager.StartRun(
		context.Background(),
		runtimeContractBotID,
		runtimeContractSessionID,
		"stream-legacy-finalization",
		make(chan struct{}, 1),
		func() {},
		make(chan conversation.InjectMessage, 1),
	); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, "stream-legacy-finalization")

	executionCtx, cancelExecution := context.WithCancel(context.Background())
	defer cancelExecution()
	eventCh := make(chan flow.WSStreamEvent, 1)
	eventCh <- rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventAgentAbort})
	close(eventCh)
	legacyCtx := make(chan context.Context, 1)
	releaseLegacy := make(chan struct{})
	forwardDone := make(chan error, 1)
	go func() {
		handler := &LocalChannelHandler{logger: slog.Default(), sessionRuntime: manager}
		forwardDone <- handler.forwardRuntimeWSStreamEvents(
			executionCtx,
			executionCtx,
			handle,
			eventCh,
			nil,
			func(ctx context.Context, _ agentpkg.StreamEvent) {
				legacyCtx <- ctx
				<-releaseLegacy
			},
		)
	}()

	deliveryCtx := <-legacyCtx
	cancelExecution()
	if err := deliveryCtx.Err(); err != nil {
		t.Fatalf("legacy terminal inherited execution cancellation: %v", err)
	}
	close(releaseLegacy)
	if err := <-forwardDone; err != nil {
		t.Fatalf("forward legacy terminal across execution cancellation: %v", err)
	}
}

func TestLocalChannelRuntimeTerminalCommitsBeforeLegacyDelivery(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-terminal-before-legacy-owner"))
	if err := manager.StartRun(
		context.Background(),
		runtimeContractBotID,
		runtimeContractSessionID,
		"stream-terminal-before-legacy",
		make(chan struct{}, 1),
		func() {},
		make(chan conversation.InjectMessage, 1),
	); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, "stream-terminal-before-legacy")

	eventCh := make(chan flow.WSStreamEvent, 1)
	eventCh <- rawRuntimeContractEvent(t, agentpkg.StreamEvent{
		Type:             agentpkg.EventAgentAbort,
		HistoryCommitted: true,
	})
	close(eventCh)
	legacyStarted := make(chan struct{})
	releaseLegacy := make(chan struct{})
	forwardDone := make(chan error, 1)
	go func() {
		handler := &LocalChannelHandler{logger: slog.Default(), sessionRuntime: manager}
		forwardDone <- handler.forwardRuntimeWSStreamEvents(
			context.Background(),
			context.Background(),
			handle,
			eventCh,
			nil,
			func(context.Context, agentpkg.StreamEvent) {
				close(legacyStarted)
				<-releaseLegacy
			},
		)
	}()

	select {
	case <-legacyStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("legacy terminal delivery did not start")
	}
	snapshot, err := manager.Snapshot(context.Background(), runtimeContractBotID, runtimeContractSessionID)
	if err != nil {
		t.Fatalf("load runtime snapshot while legacy delivery is blocked: %v", err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Status != sessionruntime.RunStatusAborted || !snapshot.CurrentRunView.HistoryCommitted {
		t.Fatalf("terminal runtime was not committed before legacy delivery: %#v", snapshot.CurrentRunView)
	}

	close(releaseLegacy)
	if err := <-forwardDone; err != nil {
		t.Fatalf("forward terminal event: %v", err)
	}
}

func TestLocalChannelRuntimeTerminalCommitSurvivesAssetLinkFailure(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), sessionruntime.Options{
		OwnerID: "handler-terminal-asset-failure-owner", StateTTL: time.Hour, OwnerLeaseTTL: time.Minute,
	})
	if err := manager.StartRun(
		context.Background(),
		runtimeContractBotID,
		runtimeContractSessionID,
		"stream-terminal-asset-failure",
		make(chan struct{}, 1),
		func() {},
		make(chan conversation.InjectMessage, 1),
	); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, "stream-terminal-asset-failure")
	assetErr := errors.New("injected asset link failure")
	handler := &LocalChannelHandler{
		logger:         slog.Default(),
		sessionRuntime: manager,
		resolver:       &failingAssetLinkResolver{Resolver: &flow.Resolver{}, err: assetErr},
	}
	eventCh := make(chan flow.WSStreamEvent, 2)
	eventCh <- rawRuntimeContractEvent(t, agentpkg.StreamEvent{
		Type: agentpkg.EventAttachment,
		Attachments: []agentpkg.FileAttachment{{
			Type: "file", Name: "report.txt", Mime: "text/plain", ContentHash: "asset-terminal-failure-hash",
		}},
	})
	eventCh <- rawRuntimeContractEvent(t, agentpkg.StreamEvent{
		Type: agentpkg.EventAgentEnd, HistoryCommitted: true,
	})
	close(eventCh)

	err := handler.forwardRuntimeWSStreamEvents(context.Background(), context.Background(), handle, eventCh, nil, nil)
	if !errors.Is(err, assetErr) {
		t.Fatalf("forward terminal with asset failure error = %v", err)
	}
	snapshot, snapshotErr := manager.Snapshot(context.Background(), runtimeContractBotID, runtimeContractSessionID)
	if snapshotErr != nil {
		t.Fatalf("load runtime snapshot: %v", snapshotErr)
	}
	if snapshot.CurrentRunView == nil ||
		snapshot.CurrentRunView.Status != sessionruntime.RunStatusErrored ||
		!snapshot.CurrentRunView.HistoryCommitted ||
		snapshot.CurrentRunView.CanonicalReady ||
		snapshot.CurrentRunView.ErrorCode != sessionruntime.RuntimeErrorCodeRunFailed ||
		snapshot.CurrentRunView.Error != "The agent run failed." {
		t.Fatalf("terminal runtime snapshot after asset failure = %#v", snapshot.CurrentRunView)
	}
}

func TestLocalChannelRuntimeWaitsForAssetLinkBeforeCanonicalTerminal(t *testing.T) {
	t.Parallel()

	manager := sessionruntime.NewManager(sessionruntime.NewMemoryBackend(), sessionruntime.Options{OwnerID: "handler-asset-order-owner"})
	if err := manager.StartRun(context.Background(), runtimeContractBotID, runtimeContractSessionID, "stream-asset-order", make(chan struct{}, 1), func() {}, make(chan conversation.InjectMessage, 1)); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	resolver := &orderedAssetLinkResolver{
		Resolver: &flow.Resolver{},
		started:  make(chan struct{}),
		release:  make(chan struct{}),
	}
	handler := &LocalChannelHandler{logger: slog.Default(), sessionRuntime: manager, resolver: resolver}
	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, "stream-asset-order")
	eventCh := make(chan flow.WSStreamEvent, 2)
	eventCh <- rawRuntimeContractEvent(t, agentpkg.StreamEvent{
		Type:        agentpkg.EventAttachment,
		Attachments: []agentpkg.FileAttachment{{Type: "file", Name: "report.txt", ContentHash: "asset-order-hash"}},
	})
	eventCh <- rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventAgentEnd, HistoryCommitted: true})
	close(eventCh)
	done := make(chan error, 1)
	go func() {
		done <- handler.forwardRuntimeWSStreamEvents(context.Background(), context.Background(), handle, eventCh, nil, nil)
	}()
	receiveHandlerTestResult(t, "asset link start", resolver.started)
	snapshot, err := manager.Snapshot(context.Background(), runtimeContractBotID, runtimeContractSessionID)
	if err != nil {
		t.Fatalf("load running snapshot: %v", err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Status != sessionruntime.RunStatusRunning {
		t.Fatalf("runtime terminalized before asset link: %#v", snapshot.CurrentRunView)
	}
	close(resolver.release)
	if err := receiveHandlerTestResult(t, "asset-ordered terminal", done); err != nil {
		t.Fatalf("forward terminal: %v", err)
	}
	snapshot, err = manager.Snapshot(context.Background(), runtimeContractBotID, runtimeContractSessionID)
	if err != nil || snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Status != sessionruntime.RunStatusCompleted || !snapshot.CurrentRunView.CanonicalReady {
		t.Fatalf("canonical terminal snapshot = %#v err=%v", snapshot.CurrentRunView, err)
	}
}

func TestLocalChannelRuntimeRetriesCompleteTerminalOutcome(t *testing.T) {
	t.Parallel()

	backend := &failRuntimeUpdateBackend{Backend: sessionruntime.NewMemoryBackend()}
	manager := sessionruntime.NewManager(backend, sessionruntime.Options{OwnerID: "handler-terminal-retry-owner"})
	if err := manager.StartRun(context.Background(), runtimeContractBotID, runtimeContractSessionID, "stream-terminal-retry", make(chan struct{}, 1), func() {}, make(chan conversation.InjectMessage, 1)); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	backend.FailNextUpdate()
	eventCh := make(chan flow.WSStreamEvent, 1)
	eventCh <- rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventAgentEnd, HistoryCommitted: true})
	close(eventCh)
	handler := &LocalChannelHandler{logger: slog.Default(), sessionRuntime: manager, resolver: &flow.Resolver{}}
	err := handler.forwardRuntimeWSStreamEvents(
		context.Background(),
		context.Background(),
		requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, "stream-terminal-retry"),
		eventCh,
		nil,
		nil,
	)
	if !errors.Is(err, sessionruntime.ErrTerminalCommitPending) {
		t.Fatalf("terminal retry error = %v", err)
	}
	snapshot := waitHandlerRuntimeSnapshot(t, manager, func(snapshot sessionruntime.Snapshot) bool {
		return snapshot.CurrentRunView != nil && snapshot.CurrentRunView.Status == sessionruntime.RunStatusCompleted
	})
	if !snapshot.CurrentRunView.HistoryCommitted || !snapshot.CurrentRunView.CanonicalReady || snapshot.CurrentRunView.Error != "" {
		t.Fatalf("retried terminal outcome = %#v", snapshot.CurrentRunView)
	}
}

func TestWSRuntimeFinalizationPreservesFenceAndSharesDeadline(t *testing.T) {
	t.Parallel()

	fence := runtimefence.Fence{
		BotID:     runtimeContractBotID,
		SessionID: runtimeContractSessionID,
		Token:     73,
	}
	runtimeSource, cancelRuntime := context.WithCancel(runtimefence.WithContext(context.Background(), fence))
	assetSource, cancelAsset := context.WithCancel(runtimefence.WithContext(context.Background(), fence))
	finalization := newWSRuntimeFinalization(runtimeSource, assetSource, time.Second)
	finalization.begin()
	defer finalization.close()
	cancelRuntime()
	cancelAsset()

	runtimeCtx := finalization.runtimeContext()
	assetCtx := finalization.assetContext()
	if got, ok := runtimefence.FromContext(runtimeCtx); !ok || got != fence {
		t.Fatalf("runtime finalization fence = %#v, %v", got, ok)
	}
	if got, ok := runtimefence.FromContext(assetCtx); !ok || got != fence {
		t.Fatalf("asset finalization fence = %#v, %v", got, ok)
	}
	if runtimeCtx.Err() != nil || assetCtx.Err() != nil {
		t.Fatalf("finalization inherited source cancellation: runtime=%v asset=%v", runtimeCtx.Err(), assetCtx.Err())
	}
	runtimeDeadline, runtimeOK := runtimeCtx.Deadline()
	assetDeadline, assetOK := assetCtx.Deadline()
	if !runtimeOK || !assetOK || !runtimeDeadline.Equal(assetDeadline) {
		t.Fatalf("finalization deadlines = runtime:%v/%v asset:%v/%v", runtimeDeadline, runtimeOK, assetDeadline, assetOK)
	}
}

func TestLocalChannelStartWSStreamMarksRuntimeErroredAfterClientDisconnect(t *testing.T) {
	t.Parallel()

	manager := sessionruntime.NewManager(sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-disconnect-error-owner"))

	connCtx, connCancel := context.WithCancel(context.Background())
	connCancel()

	handler := &LocalChannelHandler{logger: slog.Default(), sessionRuntime: manager}
	handler.startWSStream(
		context.Background(),
		connCtx,
		newWSStreamRegistry(),
		discardWSWriter(t),
		runtimeContractBotID,
		runtimeContractSessionID,
		runtimeContractStreamID,
		"runtime disconnect error contract",
		nil,
		func(_ context.Context, _ chan<- flow.WSStreamEvent, _ <-chan struct{}, _ <-chan conversation.InjectMessage) error {
			return errors.New("runner failed after client disconnected")
		},
	)

	snapshot := waitHandlerRuntimeSnapshot(t, manager, func(snapshot sessionruntime.Snapshot) bool {
		return snapshot.CurrentRunView != nil && snapshot.CurrentRunView.Status == sessionruntime.RunStatusErrored
	})
	if snapshot.CurrentRunView.ErrorCode != sessionruntime.RuntimeErrorCodeRunFailed || snapshot.CurrentRunView.Error != "The agent run failed." {
		t.Fatalf("runtime error = %#v", snapshot.CurrentRunView)
	}
}

func TestLocalChannelRuntimeUpdateFailureCancelsAndErrorsRun(t *testing.T) {
	t.Parallel()

	backend := &failRuntimeUpdateBackend{Backend: sessionruntime.NewMemoryBackend()}
	manager := startHandlerRuntimeManager(t, backend, handlerRuntimeOptions("handler-update-failure-owner"))

	runnerCanceled := make(chan struct{})
	runnerStarted := make(chan struct{})
	emitEvent := make(chan struct{})
	handler := &LocalChannelHandler{logger: slog.Default(), sessionRuntime: manager}
	handler.startWSStream(
		context.Background(),
		context.Background(),
		newWSStreamRegistry(),
		discardWSWriter(t),
		runtimeContractBotID,
		runtimeContractSessionID,
		"stream-update-failure",
		"runtime update failure contract",
		nil,
		func(ctx context.Context, eventCh chan<- flow.WSStreamEvent, _ <-chan struct{}, _ <-chan conversation.InjectMessage) error {
			close(runnerStarted)
			select {
			case <-emitEvent:
			case <-ctx.Done():
				return ctx.Err()
			}
			eventCh <- rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "lost delta"})
			<-ctx.Done()
			close(runnerCanceled)
			return ctx.Err()
		},
	)
	select {
	case <-runnerStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("runtime runner did not start")
	}
	backend.FailNextUpdate()
	close(emitEvent)

	snapshot := waitHandlerRuntimeSnapshot(t, manager, func(snapshot sessionruntime.Snapshot) bool {
		return snapshot.CurrentRunView != nil && snapshot.CurrentRunView.Status == sessionruntime.RunStatusErrored
	})
	if snapshot.CurrentRunView.ErrorCode != sessionruntime.RuntimeErrorCodeRunFailed || snapshot.CurrentRunView.Error != "The agent run failed." {
		t.Fatalf("runtime error = %#v", snapshot.CurrentRunView)
	}
	select {
	case <-runnerCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("runtime update failure did not cancel runner")
	}
}

func TestLocalChannelRuntimeForwarderBatchesAdjacentTextDeltas(t *testing.T) {
	t.Parallel()

	backend := &countRuntimeUpdateBackend{Backend: sessionruntime.NewMemoryBackend()}
	manager := startHandlerRuntimeManager(t, backend, handlerRuntimeOptions("handler-batch-owner"))
	defer func() { _ = manager.Close() }()
	if err := manager.StartRun(context.Background(), runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID, make(chan struct{}, 1), func() {}, make(chan conversation.InjectMessage, 1)); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	baselineUpdates := backend.UpdateCount()

	eventCh := make(chan flow.WSStreamEvent, 100)
	for range 100 {
		eventCh <- rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "x"})
	}
	close(eventCh)
	handler := &LocalChannelHandler{logger: slog.Default(), sessionRuntime: manager, resolver: &flow.Resolver{}}
	if err := handler.forwardRuntimeWSStreamEvents(context.Background(), context.Background(), requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID), eventCh, nil, nil); err != nil {
		t.Fatalf("forward batched runtime deltas: %v", err)
	}
	if updates := backend.UpdateCount() - baselineUpdates; updates <= 0 || updates >= 100 {
		t.Fatalf("runtime update count = %d, want batching to reduce 100 text deltas", updates)
	}
	snapshot, err := manager.Snapshot(context.Background(), runtimeContractBotID, runtimeContractSessionID)
	if err != nil {
		t.Fatalf("load batched snapshot: %v", err)
	}
	if !runtimeSnapshotHasMessage(snapshot, conversation.UIMessageText, "", strings.Repeat("x", 100)) {
		t.Fatalf("batched snapshot = %#v", snapshot.CurrentRunView)
	}
}

func TestLocalChannelPersistentRuntimePublishFailureReconcilesWithoutCancelingRun(t *testing.T) {
	t.Parallel()

	backend := &failRuntimePublishBackend{Backend: sessionruntime.NewMemoryBackend(), succeed: 1}
	manager := sessionruntime.NewManager(backend, handlerRuntimeOptions("handler-publish-failure-owner"))

	runnerCompleted := make(chan struct{})
	handler := &LocalChannelHandler{logger: slog.Default(), sessionRuntime: manager}
	handler.startWSStream(
		context.Background(),
		context.Background(),
		newWSStreamRegistry(),
		discardWSWriter(t),
		runtimeContractBotID,
		runtimeContractSessionID,
		"stream-publish-failure",
		"runtime publish failure contract",
		nil,
		func(_ context.Context, eventCh chan<- flow.WSStreamEvent, _ <-chan struct{}, _ <-chan conversation.InjectMessage) error {
			eventCh <- rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "committed delta"})
			close(runnerCompleted)
			return nil
		},
	)

	snapshot := waitHandlerRuntimeSnapshot(t, manager, func(snapshot sessionruntime.Snapshot) bool {
		return snapshot.CurrentRunView != nil && snapshot.CurrentRunView.Status == sessionruntime.RunStatusCompleted
	})
	if !runtimeSnapshotHasMessage(snapshot, conversation.UIMessageText, "", "committed delta") {
		t.Fatalf("runtime snapshot = %#v, want committed delta", snapshot.CurrentRunView)
	}
	select {
	case <-runnerCompleted:
	case <-time.After(2 * time.Second):
		t.Fatal("runtime publish failure blocked runner completion")
	}
	if _, ok, err := manager.StreamRef(context.Background(), runtimeContractBotID, runtimeContractSessionID, "stream-publish-failure"); err != nil || ok {
		t.Fatalf("terminalized publish failure stream ref = ok:%v err:%v", ok, err)
	}
}

func TestLocalChannelHandleWebSocketRuntimeSubscribeReturnsSnapshotAndAgentDeltas(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-ws-owner"))

	injectCh := make(chan conversation.InjectMessage, 1)
	if err := manager.StartRun(
		context.Background(),
		runtimeContractBotID,
		runtimeContractSessionID,
		runtimeContractStreamID,
		make(chan struct{}, 1),
		func() {},
		injectCh,
	); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	for _, event := range richActiveRunAgentContractScript()[:3] {
		if _, err := manager.HandleAgentEvent(context.Background(), requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID), event); err != nil {
			t.Fatalf("handle seeded event %s: %v", event.Type, err)
		}
	}

	handler := runtimeContractLocalChannelHandler(manager)
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type":       "runtime_subscribe",
		"session_id": runtimeContractSessionID,
	}); err != nil {
		t.Fatalf("write runtime_subscribe: %v", err)
	}

	snapshotEvent := readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeSnapshot
	})
	if snapshotEvent.Snapshot == nil {
		t.Fatalf("runtime_snapshot missing snapshot: %#v", snapshotEvent)
	}
	snapshot := snapshotEvent.Snapshot
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.StreamID != runtimeContractStreamID || snapshot.CurrentRunView.Status != sessionruntime.RunStatusRunning {
		t.Fatalf("current run = %#v", snapshot.CurrentRunView)
	}
	if !runtimeSnapshotHasMessage(*snapshot, conversation.UIMessageReasoning, "", "I need to inspect the workspace.") {
		t.Fatalf("snapshot messages = %#v, want reasoning block", snapshot.CurrentRunView.Messages)
	}
	if !runtimeSnapshotHasMessage(*snapshot, conversation.UIMessageText, "", "I will check the current state.") {
		t.Fatalf("snapshot messages = %#v, want text block", snapshot.CurrentRunView.Messages)
	}

	// The next command is a synchronization barrier: the handler can only process
	// it after the runtime subscription has been installed in the same WS loop.
	if err := client.WriteJSON(map[string]any{
		"type":       "steer_current_run",
		"stream_id":  runtimeContractStreamID,
		"session_id": runtimeContractSessionID,
		"generation": snapshot.CurrentRunView.Generation,
		"text":       "sync subscription",
	}); err != nil {
		t.Fatalf("write steer sync command: %v", err)
	}
	select {
	case injected := <-injectCh:
		if injected.Text != "sync subscription" {
			t.Fatalf("sync steer injected text = %q", injected.Text)
		}
		if injected.Applied == nil {
			t.Fatal("sync steer injection missing applied acknowledgement")
		}
		injected.Applied()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sync steer injection")
	}
	appliedSteerEvent := readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeDelta &&
			runtimeDeltaSteerStatus(event) == sessionruntime.SteerStatusApplied
	})

	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID)
	if _, err := manager.HandleAgentEvent(context.Background(), handle, agentpkg.StreamEvent{
		Type:  agentpkg.EventTextDelta,
		Delta: " Late delta.",
	}); err != nil {
		t.Fatalf("handle late agent delta: %v", err)
	}
	deltaEvent := readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeDelta &&
			event.Seq > appliedSteerEvent.Seq &&
			runtimeDeltaHasMessageAppend(event, conversation.UIMessageText, " Late delta.")
	})
	if deltaEvent.StreamID != runtimeContractStreamID {
		t.Fatalf("delta stream id = %q, want %q", deltaEvent.StreamID, runtimeContractStreamID)
	}
}

func TestLocalChannelHandleWebSocketSecondClientAttachesActiveRunAndReceivesDeltas(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-ws-second-client-owner"))

	operation := &sessionruntime.RunOperationView{
		Kind:                 sessionruntime.RunOperationRetry,
		ReplaceFromMessageID: "assistant-old",
	}
	if _, err := manager.StartRunWithOptions(context.Background(), sessionruntime.RunStartOptions{
		BotID: runtimeContractBotID, SessionID: runtimeContractSessionID, StreamID: runtimeContractStreamID,
		Admission: sessionruntime.RunAdmissionView{Operation: operation},
		AbortCh:   make(chan struct{}, 1), Cancel: func() {}, InjectCh: make(chan conversation.InjectMessage, 1),
	}); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID)
	if _, err := manager.HandleAgentEvent(context.Background(), handle, agentpkg.StreamEvent{
		Type:  agentpkg.EventTextDelta,
		Delta: "Initial output.",
	}); err != nil {
		t.Fatalf("seed runtime event: %v", err)
	}

	handler := runtimeContractLocalChannelHandler(manager)
	first := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	second := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	for _, client := range []*websocket.Conn{first, second} {
		if err := client.WriteJSON(map[string]any{
			"type":       "runtime_subscribe",
			"session_id": runtimeContractSessionID,
		}); err != nil {
			t.Fatalf("write runtime_subscribe: %v", err)
		}
		snapshotEvent := readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
			return event.Type == sessionruntime.EventRuntimeSnapshot
		})
		if snapshotEvent.Snapshot == nil || !runtimeSnapshotHasMessage(*snapshotEvent.Snapshot, conversation.UIMessageText, "", "Initial output.") {
			t.Fatalf("snapshot = %#v, want active run text", snapshotEvent.Snapshot)
		}
		gotOperation := snapshotEvent.Snapshot.CurrentRunView.Operation
		if gotOperation == nil || gotOperation.Kind != sessionruntime.RunOperationRetry || gotOperation.ReplaceFromMessageID != "assistant-old" {
			t.Fatalf("snapshot operation = %#v", gotOperation)
		}
	}

	if _, err := manager.HandleAgentEvent(context.Background(), requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID), agentpkg.StreamEvent{
		Type:  agentpkg.EventTextDelta,
		Delta: " Delta for both clients.",
	}); err != nil {
		t.Fatalf("handle shared delta: %v", err)
	}
	for _, client := range []*websocket.Conn{first, second} {
		deltaEvent := readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
			return event.Type == sessionruntime.EventRuntimeDelta &&
				runtimeDeltaHasMessageAppend(event, conversation.UIMessageText, " Delta for both clients.")
		})
		if deltaEvent.StreamID != runtimeContractStreamID {
			t.Fatalf("delta stream id = %q, want %q", deltaEvent.StreamID, runtimeContractStreamID)
		}
	}
}

func TestLocalChannelHandleWebSocketOrdinarySendPublishesRequestTurnToAllSubscribers(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-ws-request-turn-owner"))

	releaseRun := make(chan struct{})
	resolver := &scriptedOrdinaryResolver{
		Resolver: flow.NewResolver(slog.Default(), nil, nil, nil, nil, nil, nil, nil, time.UTC, time.Second),
		started:  make(chan conversation.ChatRequest, 1),
		release:  releaseRun,
	}
	handler := runtimeContractLocalChannelHandler(manager)
	handler.SetResolver(resolver)
	initiator := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	observer := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	for _, client := range []*websocket.Conn{initiator, observer} {
		if err := client.WriteJSON(map[string]any{
			"type":       "runtime_subscribe",
			"session_id": runtimeContractSessionID,
		}); err != nil {
			t.Fatalf("subscribe runtime: %v", err)
		}
		_ = readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
			return event.Type == sessionruntime.EventRuntimeSnapshot
		})
	}

	if err := initiator.WriteJSON(map[string]any{
		"type":       "message",
		"stream_id":  "stream-request-turn",
		"session_id": runtimeContractSessionID,
		"text":       "canonical prompt",
	}); err != nil {
		t.Fatalf("send ordinary message: %v", err)
	}

	for _, client := range []*websocket.Conn{initiator, observer} {
		event := readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
			return runtimeDeltaCurrentRun(event) != nil &&
				runtimeDeltaCurrentRun(event).Status == sessionruntime.RunStatusRunning &&
				runtimeDeltaCurrentRun(event).RequestUserTurn != nil
		})
		turn := runtimeDeltaCurrentRun(event).RequestUserTurn
		if turn.Role != "user" || turn.Text != "canonical prompt" {
			t.Fatalf("runtime request user turn = %#v", turn)
		}
		if turn.ExternalMessageID != "stream-request-turn" || turn.Platform != "local" {
			t.Fatalf("runtime request user identity = %#v", turn)
		}
	}

	select {
	case req := <-resolver.started:
		if req.ExternalMessageID != "stream-request-turn" || !req.UserMessageHookApplied {
			t.Fatalf("prepared stream request = %#v", req)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ordinary stream runner")
	}
	close(releaseRun)
}

func TestLocalChannelRuntimeAbortCancelsOutboundAssetLinking(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), sessionruntime.Options{OwnerID: "handler-asset-cancel-owner"})

	resolver := &blockingAssetResolver{
		scriptedOrdinaryResolver: &scriptedOrdinaryResolver{
			Resolver: flow.NewResolver(slog.Default(), nil, nil, nil, nil, nil, nil, nil, time.UTC, time.Second),
		},
		linkStarted:  make(chan struct{}),
		linkCanceled: make(chan struct{}),
		forceRelease: make(chan struct{}),
	}
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(resolver.forceRelease) }) })
	handler := runtimeContractLocalChannelHandler(manager)
	handler.SetResolver(resolver)
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	const streamID = "stream-asset-link-cancel"
	if err := client.WriteJSON(map[string]any{
		"type": "message", "stream_id": streamID, "session_id": runtimeContractSessionID, "text": "create an asset",
	}); err != nil {
		t.Fatalf("send asset-producing message: %v", err)
	}
	select {
	case <-resolver.linkStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for outbound asset linking")
	}
	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, streamID)
	if aborted, err := manager.AbortRun(context.Background(), handle); err != nil || !aborted {
		t.Fatalf("abort asset-producing run = aborted:%v err:%v", aborted, err)
	}
	select {
	case <-resolver.linkCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for outbound asset linking cancellation")
	}
}

func TestLocalChannelDistributedAdmissionCarriesPersistenceFence(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("MEMOH_TEST_REDIS_URL"))
	if redisURL == "" {
		redisURL = strings.TrimSpace(os.Getenv("MEMOH_TEST_VALKEY_URL"))
	}
	if redisURL == "" {
		if os.Getenv("MEMOH_TEST_DISTRIBUTED_REQUIRED") == "1" {
			t.Fatal("distributed persistence fence contract requires Redis or Valkey")
		}
		t.Skip("set MEMOH_TEST_REDIS_URL or MEMOH_TEST_VALKEY_URL to run distributed persistence fence contract")
	}
	backend, err := sessionruntime.NewRedisBackend(context.Background(), sessionruntime.RedisOptions{
		URL:       redisURL,
		KeyPrefix: uniqueHandlerRuntimePrefix("persistence-fence"),
		StateTTL:  time.Minute,
	})
	if err != nil {
		t.Fatalf("create runtime backend: %v", err)
	}
	manager := startHandlerRuntimeManager(t, backend, sessionruntime.Options{
		OwnerID:       "handler-persistence-fence-owner",
		StateTTL:      time.Minute,
		OwnerLeaseTTL: 5 * time.Second,
	})

	releaseRun := make(chan struct{})
	wantFence := runtimefence.Fence{BotID: runtimeContractBotID, SessionID: runtimeContractSessionID, Token: 17}
	resolver := &scriptedOrdinaryResolver{
		Resolver:     flow.NewResolver(slog.Default(), nil, nil, nil, nil, nil, nil, nil, time.UTC, time.Second),
		started:      make(chan conversation.ChatRequest, 1),
		release:      releaseRun,
		fence:        wantFence,
		prepareFence: make(chan runtimefence.Fence, 1),
		streamFence:  make(chan runtimefence.Fence, 1),
		authority:    make(chan agentpkg.TerminalHookAuthority, 1),
	}
	handler := runtimeContractLocalChannelHandler(manager)
	handler.SetResolver(resolver)
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type":       "message",
		"stream_id":  "stream-persistence-fence",
		"session_id": runtimeContractSessionID,
		"text":       "fenced prompt",
	}); err != nil {
		t.Fatalf("send distributed message: %v", err)
	}

	for stage, observed := range map[string]<-chan runtimefence.Fence{
		"admission": resolver.prepareFence,
		"runner":    resolver.streamFence,
	} {
		select {
		case got := <-observed:
			if got != wantFence {
				t.Fatalf("%s fence = %#v, want %#v", stage, got, wantFence)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %s fence", stage)
		}
	}
	var authority agentpkg.TerminalHookAuthority
	select {
	case authority = <-resolver.authority:
		if authority.Context == nil || authority.Validate == nil {
			t.Fatalf("terminal hook authority = %#v", authority)
		}
		if err := authority.Validate(context.Background()); err != nil {
			t.Fatalf("validate admitted run authority: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for terminal hook authority")
	}
	close(releaseRun)
	waitHandlerRuntimeSnapshot(t, manager, func(snapshot sessionruntime.Snapshot) bool {
		return snapshot.CurrentRunView != nil && snapshot.CurrentRunView.Status == sessionruntime.RunStatusCompleted
	})
	select {
	case <-authority.Context.Done():
		if !errors.Is(context.Cause(authority.Context), context.Canceled) {
			t.Fatalf("terminal hook authority cause = %v", context.Cause(authority.Context))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("completed run did not revoke terminal hook authority")
	}
}

func TestLocalChannelHandleWebSocketReplacementCommandsUseRuntimeProtocolAfterSubscribe(t *testing.T) {
	tests := []struct {
		name            string
		command         map[string]any
		wantKind        string
		wantReplaceFrom string
		wantReplacement string
	}{
		{
			name: "retry",
			command: map[string]any{
				"type": "retry_message", "stream_id": "stream-command-retry", "session_id": runtimeContractSessionID, "message_id": "assistant-old",
			},
			wantKind:        sessionruntime.RunOperationRetry,
			wantReplaceFrom: "assistant-old",
		},
		{
			name: "edit",
			command: map[string]any{
				"type": "edit_message", "stream_id": "stream-command-edit", "session_id": runtimeContractSessionID, "message_id": "user-old", "text": "edited prompt",
			},
			wantKind:        sessionruntime.RunOperationEdit,
			wantReplaceFrom: "user-old",
			wantReplacement: "edited prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-command-"+tt.name))

			handler := runtimeContractLocalChannelHandler(manager)
			handler.resolver = newScriptedReplacementResolver(tt.name, []flow.WSStreamEvent{
				rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventAgentStart}),
				rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "replacement output"}),
				rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventAgentEnd}),
			})
			client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
			if err := client.WriteJSON(map[string]any{"type": "runtime_subscribe", "session_id": runtimeContractSessionID}); err != nil {
				t.Fatalf("subscribe runtime: %v", err)
			}
			_ = readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
				return event.Type == sessionruntime.EventRuntimeSnapshot
			})
			if err := client.WriteJSON(tt.command); err != nil {
				t.Fatalf("write replacement command: %v", err)
			}

			var operation *sessionruntime.RunOperationView
			deadline := time.Now().Add(2 * time.Second)
			for {
				if err := client.SetReadDeadline(deadline); err != nil {
					t.Fatalf("set read deadline: %v", err)
				}
				var raw map[string]any
				if err := client.ReadJSON(&raw); err != nil {
					t.Fatalf("read replacement runtime event: %v", err)
				}
				eventType, _ := raw["type"].(string)
				if eventType == "start" || eventType == "message" || eventType == "end" {
					t.Fatalf("replacement command emitted legacy frame: %#v", raw)
				}
				data, err := json.Marshal(raw)
				if err != nil {
					t.Fatalf("marshal runtime event: %v", err)
				}
				var event sessionruntime.Event
				if err := json.Unmarshal(data, &event); err != nil {
					t.Fatalf("decode runtime event: %v", err)
				}
				if run := runtimeDeltaCurrentRun(event); run != nil && run.Operation != nil {
					operation = run.Operation
				}
				if runtimeDeltaRunStatus(event) == sessionruntime.RunStatusCompleted {
					break
				}
			}
			if operation == nil || operation.Kind != tt.wantKind || operation.ReplaceFromMessageID != tt.wantReplaceFrom {
				t.Fatalf("runtime operation = %#v", operation)
			}
			if tt.wantReplacement != "" && (operation.ReplacementUserTurn == nil || operation.ReplacementUserTurn.Text != tt.wantReplacement) {
				t.Fatalf("replacement user turn = %#v", operation.ReplacementUserTurn)
			}
		})
	}
}

func TestLocalChannelHandleWebSocketLegacyClientReceivesFramesWithoutRuntimeSubscribe(t *testing.T) {
	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-legacy-client"))

	handler := runtimeContractLocalChannelHandler(manager)
	handler.resolver = newScriptedReplacementResolver(sessionruntime.RunOperationRetry, []flow.WSStreamEvent{
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventAgentStart}),
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "legacy output"}),
		rawRuntimeContractEvent(t, agentpkg.StreamEvent{Type: agentpkg.EventAgentEnd}),
	})
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type": "retry_message", "stream_id": "stream-legacy-client", "session_id": runtimeContractSessionID, "message_id": "assistant-old",
	}); err != nil {
		t.Fatalf("write retry command: %v", err)
	}

	legacyFrames := map[string]bool{}
	deadline := time.Now().Add(2 * time.Second)
	for !legacyFrames["end"] {
		if err := client.SetReadDeadline(deadline); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}
		var raw map[string]any
		if err := client.ReadJSON(&raw); err != nil {
			t.Fatalf("read legacy frame: %v", err)
		}
		if eventType, _ := raw["type"].(string); eventType == "start" || eventType == "message" || eventType == "end" {
			legacyFrames[eventType] = true
		}
	}
	for _, eventType := range []string{"start", "message", "end"} {
		if !legacyFrames[eventType] {
			t.Fatalf("legacy client did not receive %s: %#v", eventType, legacyFrames)
		}
	}
	waitHandlerRuntimeSnapshot(t, manager, func(snapshot sessionruntime.Snapshot) bool {
		return snapshot.CurrentRunView != nil && snapshot.CurrentRunView.Status == sessionruntime.RunStatusCompleted
	})
}

func TestLocalChannelHandleWebSocketCanAbortBlockedReplacementAdmission(t *testing.T) {
	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-blocked-admission"))

	validationStarted := make(chan struct{})
	validationCanceled := make(chan struct{})
	handler := runtimeContractLocalChannelHandler(manager)
	handler.resolver = &blockingReplacementResolver{
		scriptedReplacementResolver: newScriptedReplacementResolver(sessionruntime.RunOperationRetry, nil),
		validationStarted:           validationStarted,
		validationCanceled:          validationCanceled,
	}
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type": "retry_message", "stream_id": "stream-blocked-admission", "session_id": runtimeContractSessionID, "message_id": "assistant-old",
	}); err != nil {
		t.Fatalf("write replacement command: %v", err)
	}
	select {
	case <-validationStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("replacement validation did not start")
	}
	if err := client.WriteJSON(map[string]any{
		"type": "abort", "stream_id": "stream-blocked-admission",
	}); err != nil {
		t.Fatalf("write abort command: %v", err)
	}
	select {
	case <-validationCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("abort did not cancel blocked replacement validation")
	}
	snapshot := waitHandlerRuntimeSnapshot(t, manager, func(snapshot sessionruntime.Snapshot) bool {
		return snapshot.CurrentRunView != nil && snapshot.CurrentRunView.Status == sessionruntime.RunStatusAborted
	})
	if snapshot.CurrentRunView.StreamID != "stream-blocked-admission" {
		t.Fatalf("aborted stream = %#v", snapshot.CurrentRunView)
	}
}

func TestLocalChannelHandleWebSocketFinalizesCanceledRunWithoutTerminalEvent(t *testing.T) {
	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-canceled-without-terminal"))

	started := make(chan struct{})
	handler := runtimeContractLocalChannelHandler(manager)
	handler.resolver = &cancelBlockingReplacementResolver{
		scriptedReplacementResolver: newScriptedReplacementResolver(sessionruntime.RunOperationRetry, nil),
		started:                     started,
	}
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type": "retry_message", "stream_id": "stream-canceled-without-terminal", "session_id": runtimeContractSessionID, "message_id": "assistant-old",
	}); err != nil {
		t.Fatalf("write replacement command: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("replacement stream did not start")
	}
	requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, "stream-canceled-without-terminal")
	if err := client.WriteJSON(map[string]any{
		"type": "abort", "stream_id": "stream-canceled-without-terminal", "session_id": runtimeContractSessionID,
	}); err != nil {
		t.Fatalf("write generationless abort command: %v", err)
	}

	snapshot := waitHandlerRuntimeSnapshot(t, manager, func(snapshot sessionruntime.Snapshot) bool {
		return snapshot.CurrentRunView != nil && snapshot.CurrentRunView.Status == sessionruntime.RunStatusAborted
	})
	if snapshot.CurrentRunView.StreamID != "stream-canceled-without-terminal" {
		t.Fatalf("aborted stream = %#v", snapshot.CurrentRunView)
	}
}

func TestLocalChannelHandleWebSocketKeepsMultipleSessionRuntimeSubscriptions(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-ws-multi-session-owner"))

	streams := map[string]string{
		runtimeContractSessionID:      runtimeContractStreamID,
		runtimeContractOtherSessionID: runtimeContractStreamID + "-other",
	}
	for sessionID, streamID := range streams {
		if err := manager.StartRun(
			context.Background(),
			runtimeContractBotID,
			sessionID,
			streamID,
			make(chan struct{}, 1),
			func() {},
			make(chan conversation.InjectMessage, 1),
		); err != nil {
			t.Fatalf("start runtime run for session %s: %v", sessionID, err)
		}
	}

	handler := runtimeContractLocalChannelHandlerWithSessions(manager, map[string]sqlc.BotSession{
		runtimeContractSessionID:      runtimeContractSessionRow(runtimeContractSessionID),
		runtimeContractOtherSessionID: runtimeContractSessionRow(runtimeContractOtherSessionID),
	})
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	for _, sessionID := range []string{runtimeContractSessionID, runtimeContractOtherSessionID} {
		if err := client.WriteJSON(map[string]any{
			"type":       "runtime_subscribe",
			"session_id": sessionID,
		}); err != nil {
			t.Fatalf("subscribe session %s: %v", sessionID, err)
		}
		snapshotEvent := readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
			return event.Type == sessionruntime.EventRuntimeSnapshot && event.SessionID == sessionID
		})
		if snapshotEvent.Snapshot == nil || snapshotEvent.Snapshot.CurrentRunView == nil || snapshotEvent.Snapshot.CurrentRunView.StreamID != streams[sessionID] {
			t.Fatalf("session %s snapshot = %#v", sessionID, snapshotEvent.Snapshot)
		}
	}

	for _, sessionID := range []string{runtimeContractSessionID, runtimeContractOtherSessionID} {
		text := "delta for " + sessionID
		if _, err := manager.HandleAgentEvent(context.Background(), requireHandlerRunHandle(t, manager, runtimeContractBotID, sessionID, streams[sessionID]), agentpkg.StreamEvent{
			Type:  agentpkg.EventTextDelta,
			Delta: text,
		}); err != nil {
			t.Fatalf("handle delta for session %s: %v", sessionID, err)
		}
		deltaEvent := readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
			return event.Type == sessionruntime.EventRuntimeDelta &&
				event.SessionID == sessionID &&
				runtimeDeltaHasMessageAppend(event, conversation.UIMessageText, text)
		})
		if deltaEvent.StreamID != streams[sessionID] {
			t.Fatalf("session %s delta stream id = %q, want %q", sessionID, deltaEvent.StreamID, streams[sessionID])
		}
	}
}

func TestLocalChannelHandleWebSocketRuntimeSubscribeDoesNotReplaySnapshotDeltas(t *testing.T) {
	t.Parallel()

	baseBackend := sessionruntime.NewMemoryBackend()
	hookBackend := &runtimeSubscribeHookBackend{Backend: baseBackend}
	manager := startHandlerRuntimeManager(t, hookBackend, handlerRuntimeOptions("handler-subscribe-gap-owner"))

	if err := manager.StartRun(
		context.Background(),
		runtimeContractBotID,
		runtimeContractSessionID,
		runtimeContractStreamID,
		make(chan struct{}, 1),
		func() {},
		make(chan conversation.InjectMessage, 1),
	); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID)
	if _, err := manager.HandleAgentEvent(context.Background(), handle, agentpkg.StreamEvent{
		Type:  agentpkg.EventTextDelta,
		Delta: "Before subscribe.",
	}); err != nil {
		t.Fatalf("seed runtime event: %v", err)
	}

	var once sync.Once
	hookBackend.onSubscribe = func() {
		once.Do(func() {
			if _, err := manager.HandleAgentEvent(context.Background(), handle, agentpkg.StreamEvent{
				Type:  agentpkg.EventTextDelta,
				Delta: " Gap delta.",
			}); err != nil {
				t.Errorf("gap runtime event: %v", err)
			}
		})
	}

	handler := runtimeContractLocalChannelHandler(manager)
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type":       "runtime_subscribe",
		"session_id": runtimeContractSessionID,
	}); err != nil {
		t.Fatalf("write runtime_subscribe: %v", err)
	}
	snapshotEvent := readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeSnapshot
	})
	if snapshotEvent.Snapshot == nil || !runtimeSnapshotHasMessage(*snapshotEvent.Snapshot, conversation.UIMessageText, "", "Gap delta.") {
		t.Fatalf("snapshot = %#v, want gap delta", snapshotEvent.Snapshot)
	}
	assertNoRuntimeContractEvent(t, client, 200*time.Millisecond, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeDelta && event.Seq <= snapshotEvent.Seq
	})
}

func TestLocalChannelHandleWebSocketRuntimeCommandsRouteThroughManager(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-command-owner"))

	abortCh := make(chan struct{}, 1)
	injectCh := make(chan conversation.InjectMessage, 1)
	canceled := make(chan struct{})
	var cancelOnce sync.Once
	if err := manager.StartRun(
		context.Background(),
		runtimeContractBotID,
		runtimeContractSessionID,
		runtimeContractStreamID,
		abortCh,
		func() { cancelOnce.Do(func() { close(canceled) }) },
		injectCh,
	); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID)

	handler := runtimeContractLocalChannelHandler(manager)
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type":       "runtime_subscribe",
		"session_id": runtimeContractSessionID,
	}); err != nil {
		t.Fatalf("write runtime_subscribe: %v", err)
	}
	readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeSnapshot
	})

	if err := client.WriteJSON(map[string]any{
		"type":       "steer_current_run",
		"stream_id":  runtimeContractStreamID,
		"session_id": runtimeContractSessionID,
		"generation": handle.Generation,
		"text":       "adjust course",
	}); err != nil {
		t.Fatalf("write steer command: %v", err)
	}
	select {
	case injected := <-injectCh:
		if injected.Text != "adjust course" {
			t.Fatalf("steer injected text = %q", injected.Text)
		}
		if injected.Applied == nil {
			t.Fatal("steer injection missing applied acknowledgement")
		}
		injected.Applied()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for steer injection")
	}
	steerEvent := readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeDelta &&
			runtimeDeltaSteerStatus(event) == sessionruntime.SteerStatusApplied
	})
	if steerEvent.Delta.Run.Steer.Text != "adjust course" {
		t.Fatalf("steer state = %#v", steerEvent.Delta.Run.Steer)
	}

	if err := client.WriteJSON(map[string]any{
		"type":       "abort",
		"stream_id":  runtimeContractStreamID,
		"session_id": runtimeContractSessionID,
		"generation": handle.Generation,
	}); err != nil {
		t.Fatalf("write abort command: %v", err)
	}
	select {
	case <-abortCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for abort signal")
	}
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cancel")
	}
	abortEvent := readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeDelta &&
			runtimeDeltaRunStatus(event) == sessionruntime.RunStatusAborting
	})
	if abortEvent.Delta.Run.Error != nil {
		t.Fatalf("abort delta unexpectedly changed error: %#v", abortEvent.Delta.Run)
	}
	if err := manager.FinishRun(context.Background(), requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID), sessionruntime.RunStatusAborted, ""); err != nil {
		t.Fatalf("finish aborted runtime: %v", err)
	}
	readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeDelta &&
			runtimeDeltaRunStatus(event) == sessionruntime.RunStatusAborted
	})
}

func TestLocalChannelHandleWebSocketActiveRunResponsesAreSidebandCommands(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-sideband-owner"))
	if err := manager.StartRun(
		context.Background(),
		runtimeContractBotID,
		runtimeContractSessionID,
		runtimeContractStreamID,
		make(chan struct{}, 1),
		func() {},
		make(chan conversation.InjectMessage, 1),
	); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	for _, event := range []agentpkg.StreamEvent{
		{Type: agentpkg.EventToolApprovalRequest, ToolName: "exec", ToolCallID: "call-approval", ApprovalID: "approval-1", Status: "pending"},
		{Type: agentpkg.EventUserInputRequest, ToolName: "ask_user", ToolCallID: "call-ask", UserInputID: "input-1", Status: "pending"},
	} {
		if _, err := manager.HandleAgentEvent(context.Background(), requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID), event); err != nil {
			t.Fatalf("record active response target: %v", err)
		}
	}

	resolver := &sidebandResponseResolver{
		Resolver:  &flow.Resolver{},
		approvals: make(chan flow.ToolApprovalResponseInput, 1),
		inputs:    make(chan flow.UserInputResponseInput, 1),
	}
	handler := runtimeContractLocalChannelHandler(manager)
	handler.SetResolver(resolver)
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)

	if err := client.WriteJSON(map[string]any{
		"type":        "tool_approval_response",
		"stream_id":   "approval-response-stream",
		"session_id":  runtimeContractSessionID,
		"approval_id": "approval-1",
		"decision":    "approve",
	}); err != nil {
		t.Fatalf("write approval response: %v", err)
	}
	select {
	case input := <-resolver.approvals:
		if input.ApprovalID != "approval-1" || !input.SuppressActivePromptAttach || !input.ResolveOnly {
			t.Fatalf("approval input = %#v", input)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for approval response")
	}
	approvalResult := readCommandEventUntil(t, client, "approval-response-stream")
	if approvalResult.Type != "command_result" || approvalResult.ActionID != "tool_approval_response" {
		t.Fatalf("approval result = %#v", approvalResult)
	}

	if err := client.WriteJSON(map[string]any{
		"type":          "user_input_response",
		"stream_id":     "user-input-response-stream",
		"session_id":    runtimeContractSessionID,
		"user_input_id": "input-1",
		"answers":       []map[string]any{{"question_id": "q1", "option_ids": []string{"yes"}}},
	}); err != nil {
		t.Fatalf("write user input response: %v", err)
	}
	select {
	case input := <-resolver.inputs:
		if input.UserInputID != "input-1" || !input.SuppressActivePromptAttach || !input.ResolveOnly {
			t.Fatalf("user input = %#v", input)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for user input response")
	}
	inputResult := readCommandEventUntil(t, client, "user-input-response-stream")
	if inputResult.Type != "command_result" || inputResult.ActionID != "user_input_response" {
		t.Fatalf("user input result = %#v", inputResult)
	}

	snapshot, err := manager.Snapshot(context.Background(), runtimeContractBotID, runtimeContractSessionID)
	if err != nil {
		t.Fatalf("load runtime snapshot: %v", err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.StreamID != runtimeContractStreamID || snapshot.CurrentRunView.Status != sessionruntime.RunStatusRunning {
		t.Fatalf("active runtime replaced by sideband response: %#v", snapshot.CurrentRunView)
	}
}

func TestLocalChannelDistributedInactiveDurableResponseAdmission(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("MEMOH_TEST_REDIS_URL"))
	if redisURL == "" {
		redisURL = strings.TrimSpace(os.Getenv("MEMOH_TEST_VALKEY_URL"))
	}
	if redisURL == "" {
		if os.Getenv("MEMOH_TEST_DISTRIBUTED_REQUIRED") == "1" {
			t.Fatal("distributed durable response admission requires Redis or Valkey")
		}
		t.Skip("set MEMOH_TEST_REDIS_URL or MEMOH_TEST_VALKEY_URL to run distributed durable response admission")
	}

	tests := []struct {
		name      string
		command   map[string]any
		preserved runtimefence.PreservedDecision
	}{
		{
			name: "tool approval",
			command: map[string]any{
				"type": "tool_approval_response", "stream_id": "approval-deferred-stream", "session_id": runtimeContractSessionID,
				"approval_id": "33333333-3333-3333-3333-333333333333", "decision": "approve",
			},
			preserved: runtimefence.PreservedDecision{Kind: runtimefence.DecisionToolApproval, ID: "33333333-3333-3333-3333-333333333333"},
		},
		{
			name: "user input",
			command: map[string]any{
				"type": "user_input_response", "stream_id": "input-deferred-stream", "session_id": runtimeContractSessionID,
				"user_input_id": "44444444-4444-4444-4444-444444444444",
				"answers":       []map[string]any{{"question_id": "q1", "option_ids": []string{"q1.o1"}}},
			},
			preserved: runtimefence.PreservedDecision{Kind: runtimefence.DecisionUserInput, ID: "44444444-4444-4444-4444-444444444444"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := sessionruntime.NewRedisBackend(context.Background(), sessionruntime.RedisOptions{
				URL: redisURL, KeyPrefix: uniqueHandlerRuntimePrefix("durable-response-" + strings.ReplaceAll(tt.name, " ", "-")), StateTTL: time.Minute,
			})
			if err != nil {
				t.Fatalf("create runtime backend: %v", err)
			}
			manager := startHandlerRuntimeManager(t, backend, sessionruntime.Options{
				OwnerID: "handler-durable-response-" + strings.ReplaceAll(tt.name, " ", "-"), StateTTL: time.Minute, OwnerLeaseTTL: 5 * time.Second,
			})

			streamID := tt.command["stream_id"].(string)
			wantFence := runtimefence.Fence{BotID: runtimeContractBotID, SessionID: runtimeContractSessionID, Token: 73}
			resolver := &deferredResponseResolver{
				Resolver:  &flow.Resolver{},
				fence:     wantFence,
				preserved: tt.preserved,
				stages:    make(chan deferredResponseStage, 3),
			}
			handler := runtimeContractLocalChannelHandler(manager)
			handler.SetResolver(resolver)
			client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
			if err := client.WriteJSON(tt.command); err != nil {
				t.Fatalf("write deferred response: %v", err)
			}

			wantStages := []string{"prepare", "activate", "respond"}
			for _, wantStage := range wantStages {
				select {
				case stage := <-resolver.stages:
					if stage.name != wantStage {
						t.Fatalf("response stage = %q, want %q", stage.name, wantStage)
					}
					if stage.name == "activate" {
						if stage.fence != wantFence || stage.options.PreserveDecision == nil || *stage.options.PreserveDecision != tt.preserved {
							t.Fatalf("activation = fence:%#v options:%#v", stage.fence, stage.options)
						}
					}
					if stage.name == "respond" && (stage.fence != wantFence || stage.resolveOnly) {
						t.Fatalf("response ran with fence:%#v resolveOnly:%v", stage.fence, stage.resolveOnly)
					}
				case <-time.After(2 * time.Second):
					t.Fatalf("timed out waiting for %s stage", wantStage)
				}
			}
			snapshot := waitHandlerRuntimeSnapshot(t, manager, func(snapshot sessionruntime.Snapshot) bool {
				return snapshot.CurrentRunView != nil && snapshot.CurrentRunView.Status == sessionruntime.RunStatusCompleted
			})
			if snapshot.CurrentRunView.StreamID != streamID {
				t.Fatalf("continuation stream = %#v", snapshot.CurrentRunView)
			}
		})
	}
}

func TestLocalChannelDistributedInactiveDurableResponseDoesNotReplaceActiveRun(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("MEMOH_TEST_REDIS_URL"))
	if redisURL == "" {
		redisURL = strings.TrimSpace(os.Getenv("MEMOH_TEST_VALKEY_URL"))
	}
	if redisURL == "" {
		if os.Getenv("MEMOH_TEST_DISTRIBUTED_REQUIRED") == "1" {
			t.Fatal("distributed durable response collision contract requires Redis or Valkey")
		}
		t.Skip("set MEMOH_TEST_REDIS_URL or MEMOH_TEST_VALKEY_URL to run durable response collision contract")
	}
	tests := []struct {
		name      string
		command   map[string]any
		preserved runtimefence.PreservedDecision
	}{
		{
			name: "tool approval",
			command: map[string]any{
				"type": "tool_approval_response", "stream_id": "approval-collision-stream", "session_id": runtimeContractSessionID,
				"approval_id": "55555555-5555-5555-5555-555555555555", "decision": "approve",
			},
			preserved: runtimefence.PreservedDecision{Kind: runtimefence.DecisionToolApproval, ID: "55555555-5555-5555-5555-555555555555"},
		},
		{
			name: "user input",
			command: map[string]any{
				"type": "user_input_response", "stream_id": "input-collision-stream", "session_id": runtimeContractSessionID,
				"user_input_id": "66666666-6666-6666-6666-666666666666",
				"answers":       []map[string]any{{"question_id": "q1", "option_ids": []string{"q1.o1"}}},
			},
			preserved: runtimefence.PreservedDecision{Kind: runtimefence.DecisionUserInput, ID: "66666666-6666-6666-6666-666666666666"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := sessionruntime.NewRedisBackend(context.Background(), sessionruntime.RedisOptions{
				URL: redisURL, KeyPrefix: uniqueHandlerRuntimePrefix("durable-response-collision-" + strings.ReplaceAll(tt.name, " ", "-")), StateTTL: time.Minute,
			})
			if err != nil {
				t.Fatalf("create runtime backend: %v", err)
			}
			manager := startHandlerRuntimeManager(t, backend, sessionruntime.Options{
				OwnerID: "handler-durable-response-collision-" + strings.ReplaceAll(tt.name, " ", "-"), StateTTL: time.Minute, OwnerLeaseTTL: 5 * time.Second,
			})
			if err := manager.StartRun(context.Background(), runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID, make(chan struct{}, 1), func() {}, make(chan conversation.InjectMessage, 1)); err != nil {
				t.Fatalf("start existing runtime run: %v", err)
			}

			resolver := &deferredResponseResolver{
				Resolver:  &flow.Resolver{},
				fence:     runtimefence.Fence{BotID: runtimeContractBotID, SessionID: runtimeContractSessionID, Token: 74},
				preserved: tt.preserved,
				stages:    make(chan deferredResponseStage, 3),
			}
			handler := runtimeContractLocalChannelHandler(manager)
			handler.SetResolver(resolver)
			client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
			if err := client.WriteJSON(tt.command); err != nil {
				t.Fatalf("write colliding response: %v", err)
			}
			select {
			case stage := <-resolver.stages:
				if stage.name != "prepare" {
					t.Fatalf("first response stage = %q", stage.name)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for response preparation")
			}
			errorEvent := readRuntimeWireEventUntil(t, client, func(event runtimeWireEvent) bool { return event["type"] == "error" })
			feedback, _ := errorEvent["feedback"].(map[string]any)
			if errorEvent["message"] != "The agent run failed." || feedback["code"] != string(apperror.CodeSessionRuntimeRunFailed) {
				t.Fatalf("collision error = %#v", errorEvent)
			}
			if strings.Contains(fmt.Sprint(errorEvent), "already has an active runtime run") {
				t.Fatalf("collision error leaked private runtime detail: %#v", errorEvent)
			}
			select {
			case stage := <-resolver.stages:
				t.Fatalf("unexpected response stage after rejected admission: %q", stage.name)
			default:
			}
			snapshot, err := manager.Snapshot(context.Background(), runtimeContractBotID, runtimeContractSessionID)
			if err != nil {
				t.Fatalf("load active runtime snapshot: %v", err)
			}
			if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.StreamID != runtimeContractStreamID || snapshot.CurrentRunView.Status != sessionruntime.RunStatusRunning {
				t.Fatalf("existing runtime was replaced: %#v", snapshot.CurrentRunView)
			}
		})
	}
}

func TestLocalChannelInvalidUserInputResponseFailsBeforeRunAdmission(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), sessionruntime.Options{
		OwnerID: "handler-invalid-user-input", StateTTL: time.Hour, OwnerLeaseTTL: time.Minute,
	})
	resolver := &deferredResponseResolver{
		Resolver:   &flow.Resolver{},
		prepareErr: errors.New("question q1 has no option unknown"),
		stages:     make(chan deferredResponseStage, 3),
	}
	handler := runtimeContractLocalChannelHandler(manager)
	handler.SetResolver(resolver)
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type": "user_input_response", "stream_id": "input-invalid-stream", "session_id": runtimeContractSessionID,
		"user_input_id": "77777777-7777-7777-7777-777777777777",
		"answers":       []map[string]any{{"question_id": "q1", "option_ids": []string{"unknown"}}},
	}); err != nil {
		t.Fatalf("write invalid user input response: %v", err)
	}
	select {
	case stage := <-resolver.stages:
		if stage.name != "prepare" {
			t.Fatalf("first response stage = %q", stage.name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for response preparation")
	}
	result := readCommandEventUntil(t, client, "input-invalid-stream")
	if result.Type != "command_error" || result.ActionID != sessionruntime.CommandUserInputResponse {
		t.Fatalf("invalid response result = %#v", result)
	}
	select {
	case stage := <-resolver.stages:
		t.Fatalf("unexpected response stage after invalid prepare: %q", stage.name)
	default:
	}
	snapshot, err := manager.Snapshot(context.Background(), runtimeContractBotID, runtimeContractSessionID)
	if err != nil {
		t.Fatalf("load runtime snapshot: %v", err)
	}
	if snapshot.CurrentRunView != nil {
		t.Fatalf("invalid response admitted a runtime run: %#v", snapshot.CurrentRunView)
	}
}

func TestLocalChannelSidebandResponseCanFinishRunWithoutDeferredStream(t *testing.T) {
	tests := []struct {
		name       string
		target     agentpkg.StreamEvent
		command    map[string]any
		invocation string
		actionID   string
		calls      func(*sidebandResponseResolver) <-chan struct{}
	}{
		{
			name: "approval",
			target: agentpkg.StreamEvent{
				Type: agentpkg.EventToolApprovalRequest, ToolName: "exec", ToolCallID: "call-approval-terminal",
				ApprovalID: "approval-terminal", Status: "pending",
			},
			command: map[string]any{
				"type": "tool_approval_response", "stream_id": "approval-terminal-response", "session_id": runtimeContractSessionID,
				"approval_id": "approval-terminal", "decision": "approve",
			},
			invocation: "approval-terminal-response",
			actionID:   sessionruntime.CommandToolApprovalResponse,
			calls:      func(r *sidebandResponseResolver) <-chan struct{} { return r.approvalCalls },
		},
		{
			name: "user_input",
			target: agentpkg.StreamEvent{
				Type: agentpkg.EventUserInputRequest, ToolName: "ask_user", ToolCallID: "call-input-terminal",
				UserInputID: "input-terminal", Status: "pending",
			},
			command: map[string]any{
				"type": "user_input_response", "stream_id": "input-terminal-response", "session_id": runtimeContractSessionID,
				"user_input_id": "input-terminal", "answers": []map[string]any{{"question_id": "q1", "option_ids": []string{"yes"}}},
			},
			invocation: "input-terminal-response",
			actionID:   sessionruntime.CommandUserInputResponse,
			calls:      func(r *sidebandResponseResolver) <-chan struct{} { return r.inputCalls },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), sessionruntime.Options{
				OwnerID: "handler-sideband-terminal-" + tt.name, StateTTL: time.Hour, OwnerLeaseTTL: time.Minute, Logger: slog.Default(),
			})
			if err := manager.StartRun(context.Background(), runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID,
				make(chan struct{}, 1), func() {}, make(chan conversation.InjectMessage, 1)); err != nil {
				t.Fatalf("start runtime run: %v", err)
			}
			handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID)
			if _, err := manager.HandleAgentEvent(context.Background(), handle, tt.target); err != nil {
				t.Fatalf("record response target: %v", err)
			}

			resolver := &sidebandResponseResolver{
				Resolver:      &flow.Resolver{},
				approvals:     make(chan flow.ToolApprovalResponseInput, 2),
				inputs:        make(chan flow.UserInputResponseInput, 2),
				approvalCalls: make(chan struct{}, 2),
				inputCalls:    make(chan struct{}, 2),
				finish: func(ctx context.Context) error {
					return manager.FinishRun(context.WithoutCancel(ctx), handle, sessionruntime.RunStatusCompleted, "")
				},
			}
			handler := runtimeContractLocalChannelHandler(manager)
			handler.SetResolver(resolver)
			client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
			if err := client.WriteJSON(tt.command); err != nil {
				t.Fatalf("write sideband response: %v", err)
			}

			select {
			case <-tt.calls(resolver):
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for resolver call")
			}
			result := readCommandEventUntil(t, client, tt.invocation)
			if result.Type != "command_result" || result.ActionID != tt.actionID {
				t.Fatalf("command result = %#v", result)
			}
			snapshot := waitHandlerRuntimeSnapshot(t, manager, func(snapshot sessionruntime.Snapshot) bool {
				return snapshot.CurrentRunView != nil && snapshot.CurrentRunView.Status == sessionruntime.RunStatusCompleted
			})
			if snapshot.CurrentRunView.StreamID != runtimeContractStreamID {
				t.Fatalf("sideband response started a deferred stream: %#v", snapshot.CurrentRunView)
			}
			select {
			case <-tt.calls(resolver):
				t.Fatal("resolver called more than once")
			default:
			}
		})
	}
}

func TestLocalChannelHandleWebSocketRoutesActiveResponseToRemoteOwner(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("MEMOH_TEST_REDIS_URL"))
	if redisURL == "" {
		redisURL = strings.TrimSpace(os.Getenv("MEMOH_TEST_VALKEY_URL"))
	}
	if redisURL == "" {
		if os.Getenv("MEMOH_TEST_DISTRIBUTED_REQUIRED") == "1" {
			t.Fatal("cross-server handler routing is required, but neither MEMOH_TEST_REDIS_URL nor MEMOH_TEST_VALKEY_URL is set")
		}
		t.Skip("set MEMOH_TEST_REDIS_URL or MEMOH_TEST_VALKEY_URL to run cross-server handler routing")
	}
	prefix := uniqueHandlerRuntimePrefix(strings.ReplaceAll(t.Name(), "/", ":"))
	ownerBackend, err := sessionruntime.NewRedisBackend(context.Background(), sessionruntime.RedisOptions{URL: redisURL, KeyPrefix: prefix, StateTTL: time.Minute})
	if err != nil {
		t.Fatalf("create owner runtime backend: %v", err)
	}
	remoteBackend, err := sessionruntime.NewRedisBackend(context.Background(), sessionruntime.RedisOptions{URL: redisURL, KeyPrefix: prefix, StateTTL: time.Minute})
	if err != nil {
		_ = ownerBackend.Close()
		t.Fatalf("create remote runtime backend: %v", err)
	}
	owner := sessionruntime.NewManager(ownerBackend, sessionruntime.Options{
		OwnerID:       "handler-response-owner",
		StateTTL:      time.Hour,
		OwnerLeaseTTL: time.Minute,
	})
	remote := sessionruntime.NewManager(remoteBackend, sessionruntime.Options{
		OwnerID:       "handler-response-remote",
		StateTTL:      time.Hour,
		OwnerLeaseTTL: time.Minute,
	})
	if err := owner.Start(context.Background()); err != nil {
		t.Fatalf("start owner manager: %v", err)
	}
	if err := remote.Start(context.Background()); err != nil {
		t.Fatalf("start remote manager: %v", err)
	}
	t.Cleanup(func() {
		_ = remote.Close()
		_ = owner.Close()
	})
	ownerFence := runtimefence.Fence{BotID: runtimeContractBotID, SessionID: runtimeContractSessionID, Token: 41}
	if err := owner.StartRun(runtimefence.WithContext(context.Background(), ownerFence), runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID, make(chan struct{}, 1), func() {}, make(chan conversation.InjectMessage, 1)); err != nil {
		t.Fatalf("start owner run: %v", err)
	}
	if _, err := owner.HandleAgentEvent(context.Background(), requireHandlerRunHandle(t, owner, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID), agentpkg.StreamEvent{
		Type:       agentpkg.EventToolApprovalRequest,
		ToolName:   "exec",
		ToolCallID: "call-approval",
		ApprovalID: "approval-remote",
		Status:     "pending",
	}); err != nil {
		t.Fatalf("record remote approval target: %v", err)
	}

	ownerResolver := &sidebandResponseResolver{
		Resolver:       &flow.Resolver{},
		approvals:      make(chan flow.ToolApprovalResponseInput, 1),
		inputs:         make(chan flow.UserInputResponseInput, 1),
		approvalFences: make(chan runtimefence.Fence, 1),
	}
	ownerHandler := runtimeContractLocalChannelHandler(owner)
	ownerHandler.SetResolver(ownerResolver)
	remoteHandler := runtimeContractLocalChannelHandler(remote)
	client := openLocalChannelTestWS(t, remoteHandler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type":        "tool_approval_response",
		"stream_id":   "approval-remote-response",
		"session_id":  runtimeContractSessionID,
		"approval_id": "approval-remote",
		"decision":    "approve",
	}); err != nil {
		t.Fatalf("write remote approval response: %v", err)
	}
	select {
	case input := <-ownerResolver.approvals:
		if input.ApprovalID != "approval-remote" || input.ExplicitID != "approval-remote" || input.BotID != runtimeContractBotID || input.SessionID != runtimeContractSessionID || !input.ResolveOnly {
			t.Fatalf("owner approval input = %#v", input)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for owner approval resolver")
	}
	select {
	case fence := <-ownerResolver.approvalFences:
		if fence != ownerFence {
			t.Fatalf("owner command fence = %#v, want %#v", fence, ownerFence)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for owner command fence")
	}
	result := readCommandEventUntil(t, client, "approval-remote-response")
	if result.Type != "command_result" || result.ActionID != "tool_approval_response" {
		t.Fatalf("remote approval result = %#v", result)
	}
}

func TestLocalChannelHandleWebSocketRuntimeUnsubscribeStopsSessionEvents(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-unsubscribe-owner"))
	for sessionID, streamID := range map[string]string{
		runtimeContractSessionID:      runtimeContractStreamID,
		runtimeContractOtherSessionID: runtimeContractStreamID + "-other",
	} {
		if err := manager.StartRun(context.Background(), runtimeContractBotID, sessionID, streamID, make(chan struct{}, 1), func() {}, make(chan conversation.InjectMessage, 1)); err != nil {
			t.Fatalf("start runtime run for %s: %v", sessionID, err)
		}
	}

	handler := runtimeContractLocalChannelHandlerWithSessions(manager, map[string]sqlc.BotSession{
		runtimeContractSessionID:      runtimeContractSessionRow(runtimeContractSessionID),
		runtimeContractOtherSessionID: runtimeContractSessionRow(runtimeContractOtherSessionID),
	})
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{"type": "runtime_subscribe", "session_id": runtimeContractSessionID}); err != nil {
		t.Fatalf("subscribe runtime: %v", err)
	}
	readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeSnapshot && event.SessionID == runtimeContractSessionID
	})
	if err := client.WriteJSON(map[string]any{
		"type": "runtime_unsubscribe", "session_id": runtimeContractSessionID, "invocation_id": "unsubscribe-runtime",
	}); err != nil {
		t.Fatalf("unsubscribe runtime: %v", err)
	}
	result := readCommandEventUntil(t, client, "unsubscribe-runtime")
	if result.Type != "command_result" || result.ActionID != "runtime_unsubscribe" {
		t.Fatalf("unsubscribe result = %#v, want command_result", result)
	}

	if _, err := manager.HandleAgentEvent(context.Background(), requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID), agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "must not arrive"}); err != nil {
		t.Fatalf("publish unsubscribed runtime delta: %v", err)
	}
	assertNoRuntimeContractEvent(t, client, 200*time.Millisecond, func(event sessionruntime.Event) bool {
		return event.SessionID == runtimeContractSessionID && event.Type == sessionruntime.EventRuntimeDelta
	})
}

func TestLocalChannelHandleWebSocketRuntimeResubscribeOrdersOldFramesBeforeCheckpoint(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-resubscribe-owner"))
	if err := manager.StartRun(context.Background(), runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID,
		make(chan struct{}, 1), func() {}, make(chan conversation.InjectMessage, 1)); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}

	handler := runtimeContractLocalChannelHandler(manager)
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type": "runtime_subscribe", "session_id": runtimeContractSessionID, "invocation_id": "initial-runtime-subscribe",
	}); err != nil {
		t.Fatalf("subscribe runtime: %v", err)
	}
	readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeSnapshot && event.SessionID == runtimeContractSessionID
	})
	readCommandEventUntil(t, client, "initial-runtime-subscribe")

	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID)
	if _, err := manager.HandleAgentEvent(context.Background(), handle, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "before checkpoint"}); err != nil {
		t.Fatalf("publish pre-checkpoint delta: %v", err)
	}
	expected, err := manager.Snapshot(context.Background(), runtimeContractBotID, runtimeContractSessionID)
	if err != nil {
		t.Fatalf("load expected checkpoint: %v", err)
	}
	if err := client.WriteJSON(map[string]any{
		"type": "runtime_subscribe", "session_id": runtimeContractSessionID, "invocation_id": "replacement-runtime-subscribe",
	}); err != nil {
		t.Fatalf("resubscribe runtime: %v", err)
	}

	sawCheckpoint := false
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := client.SetReadDeadline(deadline); err != nil {
			t.Fatalf("set resubscribe read deadline: %v", err)
		}
		var event runtimeWireEvent
		if err := client.ReadJSON(&event); err != nil {
			t.Fatalf("read resubscribe event: %v", err)
		}
		if event["invocation_id"] == "replacement-runtime-subscribe" {
			if event["type"] != "command_result" {
				t.Fatalf("replacement subscription result = %#v", event)
			}
			break
		}
		if event["session_id"] != runtimeContractSessionID {
			continue
		}
		switch runtimeWireEventType(event) {
		case sessionruntime.EventRuntimeSnapshot:
			if sawCheckpoint {
				t.Fatalf("replacement subscription emitted multiple checkpoints: %#v", event)
			}
			if runtimeWireEventSeq(event) != expected.Seq {
				t.Fatalf("replacement checkpoint seq = %d, want %d", runtimeWireEventSeq(event), expected.Seq)
			}
			sawCheckpoint = true
		case sessionruntime.EventRuntimeDelta:
			if sawCheckpoint && runtimeWireEventSeq(event) <= expected.Seq {
				t.Fatalf("old subscription delta arrived after replacement checkpoint: %#v", event)
			}
		}
	}
	if !sawCheckpoint {
		t.Fatal("replacement subscription did not emit a checkpoint")
	}

	if _, err := manager.HandleAgentEvent(context.Background(), handle, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: " after checkpoint"}); err != nil {
		t.Fatalf("publish post-checkpoint delta: %v", err)
	}
	next := readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeDelta && event.SessionID == runtimeContractSessionID
	})
	if next.Seq != expected.Seq+1 {
		t.Fatalf("post-checkpoint delta seq = %d, want %d", next.Seq, expected.Seq+1)
	}
}

func TestLocalChannelHandleWebSocketRuntimeUnsubscribeAfterAccessRevocation(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), sessionruntime.Options{OwnerID: "handler-revoked-unsubscribe-owner"})
	if err := manager.StartRun(context.Background(), runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID, make(chan struct{}, 1), func() {}, make(chan conversation.InjectMessage, 1)); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}

	var revoked atomic.Bool
	handler := runtimeContractLocalChannelHandlerWithAuth(manager, map[string]sqlc.BotSession{
		runtimeContractSessionID:      runtimeContractSessionRow(runtimeContractSessionID),
		runtimeContractOtherSessionID: runtimeContractSessionRow(runtimeContractOtherSessionID),
	}, func(context.Context, sqlc.ListBotUserGrantsForUserParams) ([]sqlc.ListBotUserGrantsForUserRow, error) {
		if revoked.Load() {
			return nil, nil
		}
		return runtimeContractManageGrants(), nil
	})
	handler.runtimeAuthInterval = time.Hour
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{"type": "runtime_subscribe", "session_id": runtimeContractSessionID, "invocation_id": "subscribe-before-revoke"}); err != nil {
		t.Fatalf("subscribe runtime: %v", err)
	}
	readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeSnapshot && event.SessionID == runtimeContractSessionID
	})
	readCommandEventUntil(t, client, "subscribe-before-revoke")

	revoked.Store(true)
	if err := client.WriteJSON(map[string]any{
		"type": "runtime_unsubscribe", "session_id": runtimeContractSessionID, "invocation_id": "unsubscribe-after-revoke",
	}); err != nil {
		t.Fatalf("unsubscribe after revoke: %v", err)
	}
	result := readCommandEventUntil(t, client, "unsubscribe-after-revoke")
	if result.Type != "command_result" || result.ActionID != "runtime_unsubscribe" {
		t.Fatalf("revoked unsubscribe result = %#v, want command_result", result)
	}

	if _, err := manager.HandleAgentEvent(context.Background(), requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID), agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "must not arrive after revoked unsubscribe"}); err != nil {
		t.Fatalf("publish unsubscribed runtime delta: %v", err)
	}
	assertNoRuntimeContractEvent(t, client, 200*time.Millisecond, func(event sessionruntime.Event) bool {
		return event.SessionID == runtimeContractSessionID && event.Type == sessionruntime.EventRuntimeDelta
	})
}

func TestLocalChannelHandleWebSocketRuntimeUnsubscribeCancelsBlockedSetup(t *testing.T) {
	t.Parallel()

	backend := &blockingRuntimeSubscribeBackend{
		Backend:  sessionruntime.NewMemoryBackend(),
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
	}
	manager := startHandlerRuntimeManager(t, backend, sessionruntime.Options{OwnerID: "handler-blocked-subscribe-owner"})
	handler := runtimeContractLocalChannelHandler(manager)
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type": "runtime_subscribe", "session_id": runtimeContractSessionID, "invocation_id": "blocked-subscribe",
	}); err != nil {
		t.Fatalf("subscribe runtime: %v", err)
	}
	select {
	case <-backend.started:
	case <-time.After(2 * time.Second):
		t.Fatal("runtime subscribe setup did not block")
	}
	if err := client.WriteJSON(map[string]any{
		"type": "runtime_unsubscribe", "session_id": runtimeContractSessionID, "invocation_id": "cancel-blocked-subscribe",
	}); err != nil {
		t.Fatalf("unsubscribe blocked runtime setup: %v", err)
	}
	result := readCommandEventUntil(t, client, "cancel-blocked-subscribe")
	if result.Type != "command_result" {
		t.Fatalf("unsubscribe blocked setup result = %#v, want command_result", result)
	}
	select {
	case <-backend.canceled:
	case <-time.After(time.Second):
		t.Fatal("runtime unsubscribe did not cancel blocked subscribe setup")
	}
}

func TestLocalChannelHandleWebSocketRuntimeSubscribeTimeoutClosesConnection(t *testing.T) {
	t.Parallel()

	backend := &blockingRuntimeSubscribeBackend{
		Backend:  sessionruntime.NewMemoryBackend(),
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
	}
	manager := startHandlerRuntimeManager(t, backend, sessionruntime.Options{OwnerID: "handler-timeout-subscribe-owner"})
	handler := runtimeContractLocalChannelHandler(manager)
	handler.runtimeSetupTimeout = 25 * time.Millisecond
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type": "runtime_subscribe", "session_id": runtimeContractSessionID, "invocation_id": "timed-out-subscribe",
	}); err != nil {
		t.Fatalf("subscribe runtime: %v", err)
	}
	select {
	case <-backend.started:
	case <-time.After(2 * time.Second):
		t.Fatal("runtime subscribe setup did not block")
	}
	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set runtime subscribe timeout read deadline: %v", err)
	}
	if _, _, err := client.ReadMessage(); err == nil {
		t.Fatal("runtime subscribe setup timeout left the websocket connected")
	}
	select {
	case <-backend.canceled:
	case <-time.After(time.Second):
		t.Fatal("runtime subscribe setup timeout did not cancel the backend subscription")
	}
}

func TestLocalChannelHandleWebSocketRuntimeUnsubscribeCancelsInFlightAuthorization(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), sessionruntime.Options{OwnerID: "handler-cancel-auth-owner"})

	var blockAuth atomic.Bool
	authStarted := make(chan struct{})
	authCanceled := make(chan struct{})
	var startOnce sync.Once
	var cancelOnce sync.Once
	handler := runtimeContractLocalChannelHandlerWithAuth(manager, map[string]sqlc.BotSession{
		runtimeContractSessionID: runtimeContractSessionRow(runtimeContractSessionID),
	}, func(ctx context.Context, _ sqlc.ListBotUserGrantsForUserParams) ([]sqlc.ListBotUserGrantsForUserRow, error) {
		if !blockAuth.Load() {
			return runtimeContractManageGrants(), nil
		}
		startOnce.Do(func() { close(authStarted) })
		<-ctx.Done()
		cancelOnce.Do(func() { close(authCanceled) })
		return nil, ctx.Err()
	})
	handler.runtimeAuthInterval = 200 * time.Millisecond
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type": "runtime_subscribe", "session_id": runtimeContractSessionID, "invocation_id": "subscribe-before-block",
	}); err != nil {
		t.Fatalf("subscribe runtime: %v", err)
	}
	readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeSnapshot && event.SessionID == runtimeContractSessionID
	})
	readCommandEventUntil(t, client, "subscribe-before-block")

	blockAuth.Store(true)
	select {
	case <-authStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("periodic runtime authorization did not start")
	}
	if err := client.WriteJSON(map[string]any{
		"type": "runtime_unsubscribe", "session_id": runtimeContractSessionID, "invocation_id": "unsubscribe-during-auth",
	}); err != nil {
		t.Fatalf("unsubscribe runtime: %v", err)
	}
	result := readCommandEventUntil(t, client, "unsubscribe-during-auth")
	if result.Type != "command_result" {
		t.Fatalf("unsubscribe result = %#v, want command_result", result)
	}
	select {
	case <-authCanceled:
	case <-time.After(time.Second):
		t.Fatal("runtime unsubscribe did not cancel in-flight authorization")
	}
}

func TestLocalChannelHandleWebSocketClosesRuntimeSubscriptionAfterAccessRevocation(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), sessionruntime.Options{OwnerID: "handler-revoked-subscription-owner"})

	var revoked atomic.Bool
	handler := runtimeContractLocalChannelHandlerWithAuth(manager, map[string]sqlc.BotSession{
		runtimeContractSessionID: runtimeContractSessionRow(runtimeContractSessionID),
	}, func(context.Context, sqlc.ListBotUserGrantsForUserParams) ([]sqlc.ListBotUserGrantsForUserRow, error) {
		if revoked.Load() {
			return nil, nil
		}
		return runtimeContractManageGrants(), nil
	})
	handler.runtimeAuthInterval = 20 * time.Millisecond
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{"type": "runtime_subscribe", "session_id": runtimeContractSessionID}); err != nil {
		t.Fatalf("subscribe runtime: %v", err)
	}
	readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeSnapshot && event.SessionID == runtimeContractSessionID
	})

	revoked.Store(true)
	if err := client.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	for {
		var raw json.RawMessage
		if err := client.ReadJSON(&raw); err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				t.Fatalf("runtime websocket remained open after access revocation: %v", err)
			}
			return
		}
	}
}

func TestLocalChannelHandleWebSocketAbortRequiresOwningSession(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-abort-auth-owner"))

	abortCh := make(chan struct{}, 1)
	canceled := make(chan struct{})
	var cancelOnce sync.Once
	if err := manager.StartRun(
		context.Background(),
		runtimeContractBotID,
		runtimeContractSessionID,
		runtimeContractStreamID,
		abortCh,
		func() { cancelOnce.Do(func() { close(canceled) }) },
		make(chan conversation.InjectMessage, 1),
	); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID)

	handler := runtimeContractLocalChannelHandlerWithSessions(manager, map[string]sqlc.BotSession{
		runtimeContractSessionID:      runtimeContractSessionRow(runtimeContractSessionID),
		runtimeContractOtherSessionID: runtimeContractSessionRow(runtimeContractOtherSessionID),
	})
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type":       "abort",
		"stream_id":  runtimeContractStreamID,
		"session_id": runtimeContractOtherSessionID,
		"generation": handle.Generation,
	}); err != nil {
		t.Fatalf("write abort command: %v", err)
	}
	errorEvent := readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == "error"
	})
	if errorEvent.Message != "The agent run is no longer active." {
		t.Fatalf("abort error = %#v", errorEvent)
	}
	select {
	case <-abortCh:
		t.Fatal("abort signal delivered for wrong session")
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case <-canceled:
		t.Fatal("stream canceled for wrong session")
	default:
	}
}

func TestLocalChannelHandleWebSocketRuntimeAccessRejectsNonOwnerACPSession(t *testing.T) {
	t.Parallel()

	manager := startHandlerRuntimeManager(t, sessionruntime.NewMemoryBackend(), handlerRuntimeOptions("handler-acp-owner-auth-owner"))

	abortCh := make(chan struct{}, 1)
	injectCh := make(chan conversation.InjectMessage, 1)
	canceled := make(chan struct{})
	var cancelOnce sync.Once
	if err := manager.StartRun(
		context.Background(),
		runtimeContractBotID,
		runtimeContractSessionID,
		runtimeContractStreamID,
		abortCh,
		func() { cancelOnce.Do(func() { close(canceled) }) },
		injectCh,
	); err != nil {
		t.Fatalf("start runtime run: %v", err)
	}
	handle := requireHandlerRunHandle(t, manager, runtimeContractBotID, runtimeContractSessionID, runtimeContractStreamID)

	handler := runtimeContractLocalChannelHandlerWithSessions(manager, map[string]sqlc.BotSession{
		runtimeContractSessionID: runtimeContractACPSessionRow(runtimeContractSessionID, runtimeContractOtherUserID),
	})
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type":          "runtime_subscribe",
		"invocation_id": "runtime-subscribe-owner-check",
		"session_id":    runtimeContractSessionID,
	}); err != nil {
		t.Fatalf("write runtime_subscribe: %v", err)
	}
	subscribeError := readCommandEventUntil(t, client, "runtime-subscribe-owner-check")
	if subscribeError.Type != "command_error" || subscribeError.Error == nil || subscribeError.Error.Code != acpfeedback.CodeNoWorkspaceExec || subscribeError.Error.Message != "This ACP runtime belongs to another user." {
		t.Fatalf("runtime_subscribe error = %#v, want ACP runtime owner mismatch", subscribeError)
	}

	if err := client.WriteJSON(map[string]any{
		"type":       "abort",
		"stream_id":  runtimeContractStreamID,
		"session_id": runtimeContractSessionID,
		"generation": handle.Generation,
	}); err != nil {
		t.Fatalf("write abort command: %v", err)
	}
	errorEvent := readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == "error"
	})
	if errorEvent.Message != "This ACP runtime belongs to another user." {
		t.Fatalf("abort error = %#v, want ACP runtime owner mismatch", errorEvent)
	}
	select {
	case <-abortCh:
		t.Fatal("abort signal delivered for non-owner ACP session")
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case <-canceled:
		t.Fatal("stream canceled for non-owner ACP session")
	default:
	}

	if err := client.WriteJSON(map[string]any{
		"type":       "steer_current_run",
		"stream_id":  runtimeContractStreamID,
		"session_id": runtimeContractSessionID,
		"generation": handle.Generation,
		"text":       "take over this run",
	}); err != nil {
		t.Fatalf("write steer command: %v", err)
	}
	errorEvent = readRuntimeContractEventUntil(t, client, func(event sessionruntime.Event) bool {
		return event.Type == "error"
	})
	if errorEvent.Message != "This ACP runtime belongs to another user." {
		t.Fatalf("steer error = %#v, want ACP runtime owner mismatch", errorEvent)
	}
	select {
	case injected := <-injectCh:
		t.Fatalf("steer injected for non-owner ACP session: %#v", injected)
	case <-time.After(100 * time.Millisecond):
	}
}

func runtimeContractLocalChannelHandler(manager *sessionruntime.Manager) *LocalChannelHandler {
	return runtimeContractLocalChannelHandlerWithSessions(manager, map[string]sqlc.BotSession{
		runtimeContractSessionID: runtimeContractSessionRow(runtimeContractSessionID),
	})
}

type runtimeSubscribeHookBackend struct {
	sessionruntime.Backend
	onSubscribe func()
}

type blockingRuntimeSubscribeBackend struct {
	sessionruntime.Backend
	started  chan struct{}
	canceled chan struct{}
	once     sync.Once
}

func (b *blockingRuntimeSubscribeBackend) Subscribe(ctx context.Context, _ sessionruntime.Key) (sessionruntime.Subscription, error) {
	b.once.Do(func() { close(b.started) })
	<-ctx.Done()
	close(b.canceled)
	return sessionruntime.Subscription{}, ctx.Err()
}

type scriptedReplacementResolver struct {
	*flow.Resolver
	events []flow.WSStreamEvent
}

type scriptedOrdinaryResolver struct {
	*flow.Resolver
	started      chan conversation.ChatRequest
	release      <-chan struct{}
	fence        runtimefence.Fence
	prepareFence chan runtimefence.Fence
	streamFence  chan runtimefence.Fence
	authority    chan agentpkg.TerminalHookAuthority
}

type blockingAssetResolver struct {
	*scriptedOrdinaryResolver
	linkStarted  chan struct{}
	linkCanceled chan struct{}
	forceRelease chan struct{}
}

type failingAssetLinkResolver struct {
	*flow.Resolver
	err error
}

type orderedAssetLinkResolver struct {
	*flow.Resolver
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *orderedAssetLinkResolver) LinkOutboundAssets(ctx context.Context, _ string, _ string, _ []messagepkg.AssetRef) error {
	r.once.Do(func() { close(r.started) })
	select {
	case <-r.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *failingAssetLinkResolver) LinkOutboundAssets(context.Context, string, string, []messagepkg.AssetRef) error {
	return r.err
}

func (*blockingAssetResolver) StreamChatWS(ctx context.Context, _ conversation.ChatRequest, eventCh chan<- flow.WSStreamEvent, _ <-chan struct{}) error {
	event, err := json.Marshal(agentpkg.StreamEvent{
		Type:        agentpkg.EventAttachment,
		Attachments: []agentpkg.FileAttachment{{Type: "file", Name: "report.txt", Mime: "text/plain", ContentHash: "asset-link-cancel-hash"}},
	})
	if err != nil {
		return err
	}
	select {
	case eventCh <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *blockingAssetResolver) LinkOutboundAssets(ctx context.Context, _, _ string, _ []messagepkg.AssetRef) error {
	close(r.linkStarted)
	select {
	case <-ctx.Done():
		close(r.linkCanceled)
		return ctx.Err()
	case <-r.forceRelease:
		return nil
	}
}

type sidebandResponseResolver struct {
	*flow.Resolver
	approvals      chan flow.ToolApprovalResponseInput
	inputs         chan flow.UserInputResponseInput
	approvalFences chan runtimefence.Fence
	inputFences    chan runtimefence.Fence
	approvalCalls  chan struct{}
	inputCalls     chan struct{}
	finish         func(context.Context) error
}

type deferredResponseStage struct {
	name        string
	fence       runtimefence.Fence
	options     runtimefence.ActivationOptions
	resolveOnly bool
}

type deferredResponseResolver struct {
	*flow.Resolver
	fence      runtimefence.Fence
	preserved  runtimefence.PreservedDecision
	prepareErr error
	stages     chan deferredResponseStage
}

func (r *deferredResponseResolver) AllocateRuntimePersistenceFence(context.Context, string, string) (runtimefence.Fence, error) {
	return r.fence, nil
}

func (r *deferredResponseResolver) ActivateRuntimePersistenceFenceWithOptions(_ context.Context, fence runtimefence.Fence, options runtimefence.ActivationOptions) error {
	r.stages <- deferredResponseStage{name: "activate", fence: fence, options: options}
	return nil
}

func (r *deferredResponseResolver) PrepareToolApprovalResponse(context.Context, flow.ToolApprovalResponseInput) (runtimefence.PreservedDecision, error) {
	r.stages <- deferredResponseStage{name: "prepare"}
	return r.preserved, r.prepareErr
}

func (r *deferredResponseResolver) PrepareUserInputResponse(context.Context, flow.UserInputResponseInput) (runtimefence.PreservedDecision, error) {
	r.stages <- deferredResponseStage{name: "prepare"}
	return r.preserved, r.prepareErr
}

func (r *deferredResponseResolver) RespondToolApproval(ctx context.Context, input flow.ToolApprovalResponseInput, _ chan<- flow.WSStreamEvent) error {
	fence, _ := runtimefence.FromContext(ctx)
	r.stages <- deferredResponseStage{name: "respond", fence: fence, resolveOnly: input.ResolveOnly}
	return nil
}

func (r *deferredResponseResolver) RespondUserInput(ctx context.Context, input flow.UserInputResponseInput, _ chan<- flow.WSStreamEvent) error {
	fence, _ := runtimefence.FromContext(ctx)
	r.stages <- deferredResponseStage{name: "respond", fence: fence, resolveOnly: input.ResolveOnly}
	return nil
}

func (r *sidebandResponseResolver) RespondToolApproval(ctx context.Context, input flow.ToolApprovalResponseInput, _ chan<- flow.WSStreamEvent) error {
	if r.approvalFences != nil {
		fence, _ := runtimefence.FromContext(ctx)
		r.approvalFences <- fence
	}
	r.approvals <- input
	if r.approvalCalls != nil {
		r.approvalCalls <- struct{}{}
	}
	if r.finish != nil {
		return r.finish(ctx)
	}
	return nil
}

func (r *sidebandResponseResolver) RespondUserInput(ctx context.Context, input flow.UserInputResponseInput, _ chan<- flow.WSStreamEvent) error {
	if r.inputFences != nil {
		fence, _ := runtimefence.FromContext(ctx)
		r.inputFences <- fence
	}
	r.inputs <- input
	if r.inputCalls != nil {
		r.inputCalls <- struct{}{}
	}
	if r.finish != nil {
		return r.finish(ctx)
	}
	return nil
}

func (r *scriptedOrdinaryResolver) AllocateRuntimePersistenceFence(ctx context.Context, botID, sessionID string) (runtimefence.Fence, error) {
	if r.fence.Valid() {
		if r.fence.BotID != botID || r.fence.SessionID != sessionID {
			return runtimefence.Fence{}, runtimefence.ErrStale
		}
		return r.fence, nil
	}
	return r.Resolver.AllocateRuntimePersistenceFence(ctx, botID, sessionID)
}

func (r *scriptedOrdinaryResolver) ActivateRuntimePersistenceFenceWithOptions(ctx context.Context, fence runtimefence.Fence, options runtimefence.ActivationOptions) error {
	if r.fence.Valid() {
		if fence != r.fence {
			return runtimefence.ErrStale
		}
		if options.PreserveDecision != nil {
			return errors.New("ordinary runtime admission unexpectedly preserved a decision")
		}
		return nil
	}
	return r.Resolver.ActivateRuntimePersistenceFenceWithOptions(ctx, fence, options)
}

func (r *scriptedOrdinaryResolver) PrepareUserMessageWS(ctx context.Context, req conversation.ChatRequest) (conversation.ChatRequest, error) {
	if r.prepareFence != nil {
		fence, _ := runtimefence.FromContext(ctx)
		r.prepareFence <- fence
	}
	req.UserMessageHookApplied = true
	return req, nil
}

func (r *scriptedOrdinaryResolver) StreamChatWS(ctx context.Context, req conversation.ChatRequest, _ chan<- flow.WSStreamEvent, _ <-chan struct{}) error {
	if r.streamFence != nil {
		fence, _ := runtimefence.FromContext(ctx)
		r.streamFence <- fence
	}
	if r.authority != nil {
		r.authority <- flow.TerminalHookAuthorityFromContext(ctx)
	}
	select {
	case r.started <- req:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-r.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type blockingReplacementResolver struct {
	*scriptedReplacementResolver
	validationStarted  chan struct{}
	validationCanceled chan struct{}
}

type cancelBlockingReplacementResolver struct {
	*scriptedReplacementResolver
	started chan struct{}
}

func (r *cancelBlockingReplacementResolver) StreamPreparedReplacementWS(ctx context.Context, _ flow.PreparedReplacementWS, _ chan<- flow.WSStreamEvent, _ <-chan struct{}) error {
	close(r.started)
	<-ctx.Done()
	return ctx.Err()
}

func (r *blockingReplacementResolver) ValidatePreparedReplacementWS(ctx context.Context, _ flow.PreparedReplacementWS) error {
	close(r.validationStarted)
	<-ctx.Done()
	close(r.validationCanceled)
	return ctx.Err()
}

type replacementContractMessageService struct {
	messagepkg.Service
	messages map[string]messagepkg.Message
	turn     messagepkg.HistoryTurn
}

func (s *replacementContractMessageService) GetByIDBySession(_ context.Context, _ string, messageID string) (messagepkg.Message, error) {
	message, ok := s.messages[messageID]
	if !ok {
		return messagepkg.Message{}, errors.New("replacement contract message not found")
	}
	return message, nil
}

func (s *replacementContractMessageService) GetVisibleTurnByMessage(context.Context, string, string) (messagepkg.HistoryTurn, error) {
	return s.turn, nil
}

func (s *replacementContractMessageService) GetLatestVisibleTurnBySession(context.Context, string) (messagepkg.HistoryTurn, error) {
	return s.turn, nil
}

func newScriptedReplacementResolver(kind string, events []flow.WSStreamEvent) *scriptedReplacementResolver {
	messages := &replacementContractMessageService{}
	switch kind {
	case sessionruntime.RunOperationEdit:
		messages.messages = map[string]messagepkg.Message{
			"user-old": {ID: "user-old", Role: "user"},
		}
		messages.turn = messagepkg.HistoryTurn{ID: "turn-old", RequestMessageID: "user-old", AssistantMessageID: "assistant-old"}
	default:
		messages.messages = map[string]messagepkg.Message{
			"assistant-old": {ID: "assistant-old", Role: "assistant"},
			"user-request":  {ID: "user-request", Role: "user", DisplayContent: "original prompt"},
		}
		messages.turn = messagepkg.HistoryTurn{ID: "turn-old", RequestMessageID: "user-request", AssistantMessageID: "assistant-old"}
	}
	resolver := flow.NewResolver(slog.Default(), nil, nil, nil, messages, nil, nil, nil, time.UTC, time.Second)
	return &scriptedReplacementResolver{Resolver: resolver, events: events}
}

func (r *scriptedReplacementResolver) StreamPreparedReplacementWS(ctx context.Context, _ flow.PreparedReplacementWS, eventCh chan<- flow.WSStreamEvent, _ <-chan struct{}) error {
	for _, event := range r.events {
		select {
		case eventCh <- event:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

type failRuntimeUpdateBackend struct {
	sessionruntime.Backend
	mu       sync.Mutex
	failNext bool
}

type failRuntimePublishBackend struct {
	sessionruntime.Backend
	mu      sync.Mutex
	count   int
	succeed int
}

type countRuntimeUpdateBackend struct {
	sessionruntime.Backend
	mu      sync.Mutex
	updates int
}

type cancelDuringRuntimeUpdateBackend struct {
	sessionruntime.Backend
	mu      sync.Mutex
	armed   bool
	started chan struct{}
	release chan struct{}
}

func (b *cancelDuringRuntimeUpdateBackend) Arm() {
	b.mu.Lock()
	b.armed = true
	b.mu.Unlock()
}

func (b *cancelDuringRuntimeUpdateBackend) Update(ctx context.Context, key sessionruntime.Key, update sessionruntime.SnapshotUpdate) (sessionruntime.Snapshot, bool, error) {
	b.mu.Lock()
	block := b.armed
	b.armed = false
	b.mu.Unlock()
	if block {
		close(b.started)
		select {
		case <-b.release:
		case <-ctx.Done():
			return sessionruntime.Snapshot{}, false, ctx.Err()
		}
	}
	return b.Backend.Update(ctx, key, update)
}

func (b *countRuntimeUpdateBackend) Update(ctx context.Context, key sessionruntime.Key, update sessionruntime.SnapshotUpdate) (sessionruntime.Snapshot, bool, error) {
	b.mu.Lock()
	b.updates++
	b.mu.Unlock()
	return b.Backend.Update(ctx, key, update)
}

func (b *countRuntimeUpdateBackend) UpdateCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.updates
}

func (b *failRuntimePublishBackend) Publish(ctx context.Context, event sessionruntime.Event) error {
	b.mu.Lock()
	b.count++
	shouldFail := b.count > b.succeed
	b.mu.Unlock()
	if shouldFail {
		return errors.New("injected persistent runtime publish failure")
	}
	return b.Backend.Publish(ctx, event)
}

func (b *failRuntimeUpdateBackend) Update(ctx context.Context, key sessionruntime.Key, update sessionruntime.SnapshotUpdate) (sessionruntime.Snapshot, bool, error) {
	b.mu.Lock()
	shouldFail := b.failNext
	b.failNext = false
	b.mu.Unlock()
	if shouldFail {
		return sessionruntime.Snapshot{}, false, errors.New("injected runtime update failure")
	}
	return b.Backend.Update(ctx, key, update)
}

func (b *failRuntimeUpdateBackend) FailNextUpdate() {
	b.mu.Lock()
	b.failNext = true
	b.mu.Unlock()
}

func (b *runtimeSubscribeHookBackend) Subscribe(ctx context.Context, key sessionruntime.Key) (sessionruntime.Subscription, error) {
	sub, err := b.Backend.Subscribe(ctx, key)
	if err != nil {
		return sub, err
	}
	if b.onSubscribe != nil {
		onSubscribe := b.onSubscribe
		b.onSubscribe = nil
		onSubscribe()
	}
	return sub, nil
}

func runtimeContractLocalChannelHandlerWithSessions(manager *sessionruntime.Manager, sessions map[string]sqlc.BotSession) *LocalChannelHandler {
	return runtimeContractLocalChannelHandlerWithAuth(manager, sessions, func(context.Context, sqlc.ListBotUserGrantsForUserParams) ([]sqlc.ListBotUserGrantsForUserRow, error) {
		return runtimeContractManageGrants(), nil
	})
}

func runtimeContractManageGrants() []sqlc.ListBotUserGrantsForUserRow {
	return []sqlc.ListBotUserGrantsForUserRow{{
		ID:          testUUID("dddddddd-dddd-dddd-dddd-dddddddddddd"),
		BotID:       testUUID(runtimeContractBotID),
		SubjectType: bots.GrantSubjectUser,
		UserID:      testUUID(runtimeContractUserID),
		Permissions: []byte(`["manage"]`),
	}}
}

func runtimeContractLocalChannelHandlerWithAuth(manager *sessionruntime.Manager, sessions map[string]sqlc.BotSession, listGrants func(context.Context, sqlc.ListBotUserGrantsForUserParams) ([]sqlc.ListBotUserGrantsForUserRow, error)) *LocalChannelHandler {
	queries := localChannelSessionAuthQueries{
		bot:     testBotRow(runtimeContractBotID, map[string]any{}),
		session: runtimeContractSessionRow(runtimeContractSessionID),
		getSessionByID: func(_ context.Context, id pgtype.UUID) (sqlc.BotSession, error) {
			for sessionID, session := range sessions {
				if id == testUUID(sessionID) {
					return session, nil
				}
			}
			return sqlc.BotSession{}, errors.New("session not found")
		},
		listGrants: listGrants,
	}
	handler := &LocalChannelHandler{
		channelType:    channel.ChannelTypeLocal,
		botService:     bots.NewService(nil, queries),
		accountService: accounts.NewService(nil, testAdminAccountStore{role: "user"}),
		sessionService: sessionpkg.NewService(nil, queries, nil),
		resolver:       &flow.Resolver{},
		sessionRuntime: manager,
		logger:         slog.Default(),
	}
	handler.bindSessionRuntimeCommandHandler()
	return handler
}

func runtimeContractSessionRow(sessionID string) sqlc.BotSession {
	return sqlc.BotSession{
		ID:              testUUID(sessionID),
		BotID:           testUUID(runtimeContractBotID),
		Type:            sessionpkg.TypeChat,
		SessionMode:     sessionpkg.TypeChat,
		RuntimeType:     sessionpkg.RuntimeModel,
		CreatedByUserID: testUUID(runtimeContractUserID),
		Metadata:        []byte(`{}`),
		CreatedAt:       pgtype.Timestamptz{Valid: true},
		UpdatedAt:       pgtype.Timestamptz{Valid: true},
	}
}

func runtimeContractACPSessionRow(sessionID, runtimeOwnerID string) sqlc.BotSession {
	row := runtimeContractSessionRow(sessionID)
	row.Type = sessionpkg.TypeChat
	row.SessionMode = sessionpkg.TypeChat
	row.RuntimeType = sessionpkg.RuntimeACPAgent
	row.RuntimeMetadata = testJSON(map[string]any{
		"acp_agent_id":             "codex",
		"project_path":             "/data",
		"runtime_owner_account_id": runtimeOwnerID,
	})
	return row
}

func runtimeSnapshotHasMessage(snapshot sessionruntime.Snapshot, kind conversation.UIMessageType, toolCallID, content string) bool {
	if snapshot.CurrentRunView == nil {
		return false
	}
	for _, message := range snapshot.CurrentRunView.Messages {
		if message.Type != kind {
			continue
		}
		if toolCallID != "" && message.ToolCallID != toolCallID {
			continue
		}
		if content != "" && !strings.Contains(message.Content, content) {
			continue
		}
		return true
	}
	return false
}

func runtimeDeltaCurrentRun(event sessionruntime.Event) *sessionruntime.CurrentRunView {
	if event.Delta == nil {
		return nil
	}
	return event.Delta.CurrentRunView
}

func runtimeDeltaRunStatus(event sessionruntime.Event) string {
	if run := runtimeDeltaCurrentRun(event); run != nil {
		return run.Status
	}
	if event.Delta == nil || event.Delta.Run == nil || event.Delta.Run.Status == nil {
		return ""
	}
	return *event.Delta.Run.Status
}

func runtimeDeltaSteerStatus(event sessionruntime.Event) string {
	if run := runtimeDeltaCurrentRun(event); run != nil && run.Steer != nil {
		return run.Steer.Status
	}
	if event.Delta == nil || event.Delta.Run == nil || event.Delta.Run.Steer == nil {
		return ""
	}
	return event.Delta.Run.Steer.Status
}

func runtimeDeltaHasMessageAppend(event sessionruntime.Event, kind conversation.UIMessageType, content string) bool {
	if event.Delta == nil {
		return false
	}
	for _, append := range event.Delta.MessageAppends {
		if append.Type == kind && append.Content == content {
			return true
		}
	}
	return false
}

func runtimeWireEventType(event runtimeWireEvent) string {
	value, _ := event["type"].(string)
	return value
}

func runtimeWireEventSeq(event runtimeWireEvent) int64 {
	switch value := event["seq"].(type) {
	case float64:
		return int64(value)
	case json.Number:
		seq, _ := value.Int64()
		return seq
	case int64:
		return value
	default:
		return 0
	}
}

func readRuntimeContractEventUntil(t *testing.T, client *websocket.Conn, pred func(sessionruntime.Event) bool) sessionruntime.Event {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var events []sessionruntime.Event
	for {
		if err := client.SetReadDeadline(deadline); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}
		var event sessionruntime.Event
		if err := client.ReadJSON(&event); err != nil {
			t.Fatalf("read runtime ws event: %v; events=%#v", err, events)
		}
		events = append(events, event)
		if pred(event) {
			return event
		}
	}
}

func readRuntimeWireEventUntil(t *testing.T, client *websocket.Conn, pred func(runtimeWireEvent) bool) runtimeWireEvent {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var events []runtimeWireEvent
	for {
		if err := client.SetReadDeadline(deadline); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}
		var event runtimeWireEvent
		if err := client.ReadJSON(&event); err != nil {
			t.Fatalf("read runtime wire event: %v; events=%#v", err, events)
		}
		events = append(events, event)
		if pred(event) {
			return event
		}
	}
}

func readCommandEventUntil(t *testing.T, client *websocket.Conn, invocationID string) CommandEventResponse {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := client.SetReadDeadline(deadline); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}
		var raw json.RawMessage
		if err := client.ReadJSON(&raw); err != nil {
			t.Fatalf("read command ws event: %v", err)
		}
		var event CommandEventResponse
		if err := json.Unmarshal(raw, &event); err != nil {
			continue
		}
		if event.InvocationID == invocationID && (event.Type == "command_result" || event.Type == "command_error") {
			return event
		}
	}
}

func assertNoRuntimeContractEvent(t *testing.T, client *websocket.Conn, wait time.Duration, pred func(sessionruntime.Event) bool) {
	t.Helper()
	deadline := time.Now().Add(wait)
	var events []sessionruntime.Event
	for time.Now().Before(deadline) {
		if err := client.SetReadDeadline(deadline); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}
		var event sessionruntime.Event
		if err := client.ReadJSON(&event); err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return
			}
			t.Fatalf("read runtime ws event: %v; events=%#v", err, events)
		}
		events = append(events, event)
		if pred(event) {
			t.Fatalf("unexpected runtime ws event: %#v; events=%#v", event, events)
		}
	}
}

func waitHandlerRuntimeSnapshot(t *testing.T, manager *sessionruntime.Manager, pred func(sessionruntime.Snapshot) bool) sessionruntime.Snapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var last sessionruntime.Snapshot
	for time.Now().Before(deadline) {
		snapshot, err := manager.Snapshot(context.Background(), runtimeContractBotID, runtimeContractSessionID)
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		last = snapshot
		if pred(snapshot) {
			return snapshot
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for runtime snapshot; last=%#v", last)
	return sessionruntime.Snapshot{}
}

func discardWSWriter(t *testing.T) *wsWriter {
	t.Helper()
	closeWriter := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
		<-closeWriter
		_ = conn.Close()
		<-done
	}))
	t.Cleanup(func() {
		close(closeWriter)
		server.Close()
	})
	client, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if resp != nil && resp.Body != nil {
		t.Cleanup(func() { _ = resp.Body.Close() })
	}
	if err != nil {
		t.Fatalf("dial discard websocket: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	writer := newWSWriter(client)
	t.Cleanup(writer.Close)
	return writer
}
