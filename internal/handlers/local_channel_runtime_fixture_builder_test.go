package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/sessionruntime"
)

const (
	runtimeFixtureBotID         = "bot-1"
	runtimeFixtureSessionID     = "session-1"
	runtimeFixtureStreamID      = "stream-rich"
	runtimeFixtureInterruptedID = "stream-interrupted"
	runtimeFixtureOwnerID       = "owner-1"
	runtimeFixtureUpdateEnv     = "UPDATE_RUNTIME_CONTRACT_FIXTURES"
)

var runtimeFixtureStartTime = time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC)

func committedAbortAgentContractScript() []agentpkg.StreamEvent {
	return []agentpkg.StreamEvent{
		{Type: agentpkg.EventAgentStart},
		{Type: agentpkg.EventTextDelta, Delta: "partial output"},
		{Type: agentpkg.EventAgentAbort, HistoryCommitted: true},
	}
}

func runtimeFixtureRequestUserTurn(streamID string) *conversation.UITurn {
	return &conversation.UITurn{
		Role:              "user",
		Text:              "Inspect the workspace",
		Attachments:       []conversation.UIAttachment{{Type: "file", Name: "notes.txt", ContentHash: "sha256:notes"}},
		Timestamp:         runtimeFixtureStartTime,
		Platform:          "local",
		SenderUserID:      "user-1",
		ExternalMessageID: streamID,
	}
}

type runtimeContractFixture struct {
	Version                int                `json:"version"`
	Scenario               string             `json:"scenario"`
	RuntimeSnapshot        runtimeWireEvent   `json:"runtime_snapshot"`
	RuntimeStream          []runtimeWireEvent `json:"runtime_stream"`
	RuntimeTerminalStream  []runtimeWireEvent `json:"runtime_terminal_stream,omitempty"`
	RuntimeAbortStream     []runtimeWireEvent `json:"runtime_abort_stream,omitempty"`
	RuntimeAdmissionStream []runtimeWireEvent `json:"runtime_admission_stream,omitempty"`
	RuntimeResetStream     []runtimeWireEvent `json:"runtime_reset_stream,omitempty"`
	RuntimeSteerStream     []runtimeWireEvent `json:"runtime_steer_stream,omitempty"`
}

type runtimeReplacementContractFixture struct {
	Version       int              `json:"version"`
	RetrySnapshot runtimeWireEvent `json:"retry_snapshot"`
	EditSnapshot  runtimeWireEvent `json:"edit_snapshot"`
}

type runtimeGenerationReuseContractFixture struct {
	Version         int                `json:"version"`
	Scenario        string             `json:"scenario"`
	RuntimeSnapshot runtimeWireEvent   `json:"runtime_snapshot"`
	RuntimeStream   []runtimeWireEvent `json:"runtime_stream"`
}

type runtimeRecoveryContractFixture struct {
	Version           int              `json:"version"`
	Scenario          string           `json:"scenario"`
	RuntimeSnapshot   runtimeWireEvent `json:"runtime_snapshot"`
	GapDelta          runtimeWireEvent `json:"gap_delta"`
	DelayedDelta      runtimeWireEvent `json:"delayed_delta"`
	RuntimeCheckpoint runtimeWireEvent `json:"runtime_checkpoint"`
	PostRecoveryDelta runtimeWireEvent `json:"post_recovery_delta"`
}

type runtimeRecoveryFixtureBackend struct {
	sessionruntime.Backend

	mu       sync.Mutex
	record   bool
	dropNext bool
	events   chan sessionruntime.Event
}

func newRuntimeRecoveryFixtureBackend() *runtimeRecoveryFixtureBackend {
	return &runtimeRecoveryFixtureBackend{
		Backend: sessionruntime.NewMemoryBackend(),
		events:  make(chan sessionruntime.Event, 8),
	}
}

func (b *runtimeRecoveryFixtureBackend) armDroppedDelta() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.record = true
	b.dropNext = true
}

func (b *runtimeRecoveryFixtureBackend) Publish(ctx context.Context, event sessionruntime.Event) error {
	b.mu.Lock()
	record := b.record && event.Type == sessionruntime.EventRuntimeDelta
	drop := record && b.dropNext
	if drop {
		b.dropNext = false
	}
	b.mu.Unlock()
	if record {
		b.events <- event
	}
	if drop {
		return nil
	}
	return b.Backend.Publish(ctx, event)
}

func runtimeFixtureWireEvent(t *testing.T, event sessionruntime.Event) runtimeWireEvent {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal runtime fixture event: %v", err)
	}
	var wire runtimeWireEvent
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("decode runtime fixture event: %v", err)
	}
	return wire
}

func receiveRuntimeRecoveryFixtureEvent(t *testing.T, events <-chan sessionruntime.Event) sessionruntime.Event {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runtime recovery fixture event")
		return sessionruntime.Event{}
	}
}

func buildRuntimeContractSnapshot(t *testing.T, streamID string, events []agentpkg.StreamEvent) (runtimeWireEvent, []runtimeWireEvent) {
	t.Helper()
	manager := sessionruntime.NewManager(sessionruntime.NewMemoryBackend(), runtimeFixtureManagerOptions(streamID))
	defer func() {
		if err := manager.Close(); err != nil {
			t.Fatalf("close fixture runtime manager: %v", err)
		}
	}()
	handle, err := manager.StartRunWithOptions(context.Background(), sessionruntime.RunStartOptions{
		BotID: runtimeContractBotID, SessionID: runtimeContractSessionID, StreamID: streamID,
		Admission: sessionruntime.RunAdmissionView{RequestUserTurn: runtimeFixtureRequestUserTurn(streamID)},
		AbortCh:   make(chan struct{}, 1), Cancel: func() {}, InjectCh: make(chan conversation.InjectMessage, 1),
	})
	if err != nil {
		t.Fatalf("start fixture runtime run: %v", err)
	}
	baselineEvent, client := captureRuntimeFixtureSnapshot(t, manager)

	handler := &LocalChannelHandler{logger: slog.Default(), sessionRuntime: manager}
	eventCh := make(chan flow.WSStreamEvent, len(events))
	for _, event := range events {
		eventCh <- rawRuntimeContractEvent(t, event)
	}
	close(eventCh)
	if err := handler.forwardRuntimeWSStreamEvents(
		context.Background(),
		context.Background(),
		handle,
		eventCh,
		nil,
		nil,
	); err != nil {
		t.Fatalf("forward fixture runtime events: %v", err)
	}

	snapshot, err := manager.Snapshot(context.Background(), runtimeContractBotID, runtimeContractSessionID)
	if err != nil {
		t.Fatalf("load fixture runtime snapshot: %v", err)
	}
	stream := []runtimeWireEvent{baselineEvent}
	for runtimeWireEventSeq(stream[len(stream)-1]) < snapshot.Seq {
		event := readRuntimeWireEventUntil(t, client, func(event runtimeWireEvent) bool {
			return runtimeWireEventType(event) == sessionruntime.EventRuntimeDelta && runtimeWireEventSeq(event) > runtimeWireEventSeq(baselineEvent)
		})
		normalizeRuntimeFixtureWireEvent(event)
		stream = append(stream, event)
	}
	finalEvent, _ := captureRuntimeFixtureSnapshot(t, manager)
	return finalEvent, stream
}

func captureRuntimeFixtureSnapshot(t *testing.T, manager *sessionruntime.Manager) (runtimeWireEvent, *websocket.Conn) {
	t.Helper()
	handler := runtimeContractLocalChannelHandler(manager)
	client := openLocalChannelTestWS(t, handler, runtimeContractBotID, runtimeContractUserID)
	if err := client.WriteJSON(map[string]any{
		"type":          "runtime_subscribe",
		"invocation_id": "fixture-runtime-subscribe",
		"session_id":    runtimeContractSessionID,
	}); err != nil {
		t.Fatalf("subscribe fixture runtime: %v", err)
	}
	event := readRuntimeWireEventUntil(t, client, func(event runtimeWireEvent) bool {
		return runtimeWireEventType(event) == sessionruntime.EventRuntimeSnapshot
	})
	if _, ok := event["snapshot"].(map[string]any); !ok {
		t.Fatalf("fixture runtime snapshot is missing: %#v", event)
	}
	normalizeRuntimeFixtureWireEvent(event)
	return event, client
}

func normalizeRuntimeFixtureWireEvent(event runtimeWireEvent) {
	event["bot_id"] = runtimeFixtureBotID
	event["session_id"] = runtimeFixtureSessionID
	updatedAt := runtimeFixtureStartTime.Add(10 * time.Second).Format(time.RFC3339)
	if _, ok := event["updated_at"]; ok {
		event["updated_at"] = updatedAt
	}
	if snapshot, ok := event["snapshot"].(map[string]any); ok {
		snapshot["bot_id"] = runtimeFixtureBotID
		snapshot["session_id"] = runtimeFixtureSessionID
		snapshot["updated_at"] = updatedAt
		if run, ok := snapshot["current_run_view"].(map[string]any); ok {
			run["started_at"] = runtimeFixtureStartTime.Format(time.RFC3339)
			run["updated_at"] = updatedAt
		}
	}
	if delta, ok := event["delta"].(map[string]any); ok {
		if run, ok := delta["current_run_view"].(map[string]any); ok {
			run["started_at"] = runtimeFixtureStartTime.Format(time.RFC3339)
			run["updated_at"] = updatedAt
			normalizeRuntimeFixtureSteer(run)
		}
		if run, ok := delta["run"].(map[string]any); ok {
			run["updated_at"] = updatedAt
			normalizeRuntimeFixtureSteer(run)
		}
	}
}

func normalizeRuntimeFixtureSteer(run map[string]any) {
	steer, ok := run["steer"].(map[string]any)
	if !ok {
		return
	}
	steer["id"] = "steer-runtime-contract"
	steer["created_at"] = runtimeFixtureStartTime.Format(time.RFC3339)
	steer["updated_at"] = runtimeFixtureStartTime.Add(10 * time.Second).Format(time.RFC3339)
}

func buildRuntimeAdmissionStream(t *testing.T) []runtimeWireEvent {
	t.Helper()
	manager := sessionruntime.NewManager(sessionruntime.NewMemoryBackend(), runtimeFixtureManagerOptions("stream-admission"))
	defer func() { _ = manager.Close() }()
	baseline, client := captureRuntimeFixtureSnapshot(t, manager)

	builderStarted := make(chan struct{})
	releaseBuilder := make(chan struct{})
	startDone := make(chan error, 1)
	go func() {
		_, err := manager.StartRunWithOptions(context.Background(), sessionruntime.RunStartOptions{
			BotID: runtimeContractBotID, SessionID: runtimeContractSessionID, StreamID: "stream-admission",
			AdmissionBuilder: func(context.Context, sessionruntime.RunHandle) (sessionruntime.RunAdmissionView, error) {
				close(builderStarted)
				<-releaseBuilder
				return sessionruntime.RunAdmissionView{RequestUserTurn: runtimeFixtureRequestUserTurn("stream-admission")}, nil
			},
			AbortCh: make(chan struct{}, 1), Cancel: func() {}, InjectCh: make(chan conversation.InjectMessage, 1),
		})
		startDone <- err
	}()
	<-builderStarted
	admitting := readRuntimeWireEventUntil(t, client, func(event runtimeWireEvent) bool {
		return runtimeWireEventType(event) == sessionruntime.EventRuntimeDelta && runtimeWireRunStatus(event) == sessionruntime.RunStatusAdmitting
	})
	close(releaseBuilder)
	if err := <-startDone; err != nil {
		t.Fatalf("activate admission fixture: %v", err)
	}
	running := readRuntimeWireEventUntil(t, client, func(event runtimeWireEvent) bool {
		return runtimeWireEventType(event) == sessionruntime.EventRuntimeDelta && runtimeWireRunStatus(event) == sessionruntime.RunStatusRunning
	})
	normalizeRuntimeFixtureWireEvent(admitting)
	normalizeRuntimeFixtureWireEvent(running)
	return []runtimeWireEvent{baseline, admitting, running}
}

func buildRuntimeResetStream(t *testing.T) []runtimeWireEvent {
	t.Helper()
	manager := sessionruntime.NewManager(sessionruntime.NewMemoryBackend(), runtimeFixtureManagerOptions("stream-reset"))
	defer func() { _ = manager.Close() }()
	handle, err := manager.StartRunWithOptions(context.Background(), sessionruntime.RunStartOptions{
		BotID: runtimeContractBotID, SessionID: runtimeContractSessionID, StreamID: "stream-reset",
		AbortCh: make(chan struct{}, 1), Cancel: func() {}, InjectCh: make(chan conversation.InjectMessage, 1),
	})
	if err != nil {
		t.Fatalf("start reset fixture: %v", err)
	}
	baseline, client := captureRuntimeFixtureSnapshot(t, manager)
	events := []agentpkg.StreamEvent{
		{Type: agentpkg.EventTextDelta, Delta: "discarded draft"},
		{Type: agentpkg.EventRetry},
		{Type: agentpkg.EventTextDelta, Delta: "replacement draft"},
	}
	stream := make([]runtimeWireEvent, 0, len(events)+1)
	stream = append(stream, baseline)
	for _, agentEvent := range events {
		if _, err := manager.HandleAgentEvent(context.Background(), handle, agentEvent); err != nil {
			t.Fatalf("apply reset fixture event %s: %v", agentEvent.Type, err)
		}
		event := readRuntimeWireEventUntil(t, client, func(event runtimeWireEvent) bool {
			return runtimeWireEventType(event) == sessionruntime.EventRuntimeDelta
		})
		normalizeRuntimeFixtureWireEvent(event)
		stream = append(stream, event)
	}
	return stream
}

func buildRuntimeSteerStream(t *testing.T) []runtimeWireEvent {
	t.Helper()
	manager := sessionruntime.NewManager(sessionruntime.NewMemoryBackend(), runtimeFixtureManagerOptions("stream-steer"))
	defer func() { _ = manager.Close() }()
	injectCh := make(chan conversation.InjectMessage, 1)
	handle, err := manager.StartRunWithOptions(context.Background(), sessionruntime.RunStartOptions{
		BotID: runtimeContractBotID, SessionID: runtimeContractSessionID, StreamID: "stream-steer",
		AbortCh: make(chan struct{}, 1), Cancel: func() {}, InjectCh: injectCh,
	})
	if err != nil {
		t.Fatalf("start steer fixture: %v", err)
	}
	baseline, client := captureRuntimeFixtureSnapshot(t, manager)
	if _, err := manager.SteerRun(context.Background(), handle, "adjust course"); err != nil {
		t.Fatalf("steer fixture run: %v", err)
	}
	pending := readRuntimeWireEventUntil(t, client, func(event runtimeWireEvent) bool {
		return runtimeWireEventType(event) == sessionruntime.EventRuntimeDelta && runtimeWireSteerStatus(event) == sessionruntime.SteerStatusPending
	})
	queued := readRuntimeWireEventUntil(t, client, func(event runtimeWireEvent) bool {
		return runtimeWireEventType(event) == sessionruntime.EventRuntimeDelta && runtimeWireSteerStatus(event) == sessionruntime.SteerStatusQueued
	})
	select {
	case injected := <-injectCh:
		if injected.Applied == nil {
			t.Fatal("steer fixture injection has no applied callback")
		}
		injected.Applied()
	case <-time.After(2 * time.Second):
		t.Fatal("steer fixture injection timed out")
	}
	applied := readRuntimeWireEventUntil(t, client, func(event runtimeWireEvent) bool {
		return runtimeWireEventType(event) == sessionruntime.EventRuntimeDelta && runtimeWireSteerStatus(event) == sessionruntime.SteerStatusApplied
	})
	normalizeRuntimeFixtureWireEvent(pending)
	normalizeRuntimeFixtureWireEvent(queued)
	normalizeRuntimeFixtureWireEvent(applied)
	return []runtimeWireEvent{baseline, pending, queued, applied}
}

func appendRuntimeFixtureDeltas(t *testing.T, manager *sessionruntime.Manager, client *websocket.Conn, stream []runtimeWireEvent) []runtimeWireEvent {
	t.Helper()
	snapshot, err := manager.Snapshot(context.Background(), runtimeContractBotID, runtimeContractSessionID)
	if err != nil {
		t.Fatalf("load runtime fixture checkpoint: %v", err)
	}
	for runtimeWireEventSeq(stream[len(stream)-1]) < snapshot.Seq {
		event := readRuntimeWireEventUntil(t, client, func(event runtimeWireEvent) bool {
			return runtimeWireEventType(event) == sessionruntime.EventRuntimeDelta &&
				runtimeWireEventSeq(event) > runtimeWireEventSeq(stream[len(stream)-1])
		})
		normalizeRuntimeFixtureWireEvent(event)
		stream = append(stream, event)
	}
	return stream
}

func runtimeGenerationReuseRequest(streamID, generation, text string, timestamp time.Time) *conversation.UITurn {
	request := runtimeFixtureRequestUserTurn(streamID)
	request.ID = "user-" + generation
	request.Text = text
	request.Timestamp = timestamp
	request.Attachments = nil
	return request
}

func buildRuntimeGenerationReuseContractFixture(t *testing.T) runtimeGenerationReuseContractFixture {
	t.Helper()
	const streamID = "stream-generation-reuse"
	generations := []string{"generation-a", "generation-b"}
	generationIndex := 0
	options := runtimeFixtureManagerOptions(streamID)
	options.RunGenerationGenerator = func() string {
		generation := generations[generationIndex]
		generationIndex++
		return generation
	}
	manager := sessionruntime.NewManager(sessionruntime.NewMemoryBackend(), options)
	defer func() { _ = manager.Close() }()
	baseline, client := captureRuntimeFixtureSnapshot(t, manager)
	stream := []runtimeWireEvent{baseline}

	start := func(generation, text string, timestamp time.Time) sessionruntime.RunHandle {
		handle, err := manager.StartRunWithOptions(context.Background(), sessionruntime.RunStartOptions{
			BotID: runtimeContractBotID, SessionID: runtimeContractSessionID, StreamID: streamID,
			Admission: sessionruntime.RunAdmissionView{RequestUserTurn: runtimeGenerationReuseRequest(streamID, generation, text, timestamp)},
			AbortCh:   make(chan struct{}, 1), Cancel: func() {}, InjectCh: make(chan conversation.InjectMessage, 1),
		})
		if err != nil {
			t.Fatalf("start reused-generation fixture %s: %v", generation, err)
		}
		return handle
	}

	first := start("generation-a", "old prompt", runtimeFixtureStartTime)
	stream = appendRuntimeFixtureDeltas(t, manager, client, stream)
	if _, err := manager.HandleAgentEvent(context.Background(), first, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "old answer"}); err != nil {
		t.Fatalf("append first reused-generation fixture output: %v", err)
	}
	stream = appendRuntimeFixtureDeltas(t, manager, client, stream)
	if err := manager.FinishRun(context.Background(), first, sessionruntime.RunStatusCompleted, ""); err != nil {
		t.Fatalf("finish first reused-generation fixture run: %v", err)
	}
	stream = appendRuntimeFixtureDeltas(t, manager, client, stream)

	second := start("generation-b", "new prompt", runtimeFixtureStartTime.Add(time.Minute))
	stream = appendRuntimeFixtureDeltas(t, manager, client, stream)
	if _, err := manager.HandleAgentEvent(context.Background(), second, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "new partial"}); err != nil {
		t.Fatalf("append second reused-generation fixture output: %v", err)
	}
	stream = appendRuntimeFixtureDeltas(t, manager, client, stream)
	snapshot, snapshotClient := captureRuntimeFixtureSnapshot(t, manager)
	if err := snapshotClient.Close(); err != nil {
		t.Fatalf("close reused-generation snapshot client: %v", err)
	}

	return runtimeGenerationReuseContractFixture{
		Version:         1,
		Scenario:        "generation_reuse",
		RuntimeSnapshot: snapshot,
		RuntimeStream:   stream,
	}
}

func buildRuntimeRecoveryContractFixture(t *testing.T) runtimeRecoveryContractFixture {
	t.Helper()
	const streamID = "stream-recovery"
	backend := newRuntimeRecoveryFixtureBackend()
	manager := sessionruntime.NewManager(backend, runtimeFixtureManagerOptions(streamID))
	defer func() {
		if err := manager.Close(); err != nil {
			t.Fatalf("close recovery fixture runtime manager: %v", err)
		}
	}()
	handle, err := manager.StartRunWithOptions(context.Background(), sessionruntime.RunStartOptions{
		BotID: runtimeContractBotID, SessionID: runtimeContractSessionID, StreamID: streamID,
		Admission: sessionruntime.RunAdmissionView{RequestUserTurn: runtimeFixtureRequestUserTurn(streamID)},
		AbortCh:   make(chan struct{}, 1), Cancel: func() {}, InjectCh: make(chan conversation.InjectMessage, 1),
	})
	if err != nil {
		t.Fatalf("start recovery fixture runtime run: %v", err)
	}
	baseline, client := captureRuntimeFixtureSnapshot(t, manager)
	defer func() { _ = client.Close() }()

	backend.armDroppedDelta()
	if _, err := manager.HandleAgentEvent(context.Background(), handle, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "missing "}); err != nil {
		t.Fatalf("append delayed recovery fixture delta: %v", err)
	}
	delayedEvent := receiveRuntimeRecoveryFixtureEvent(t, backend.events)
	if _, err := manager.HandleAgentEvent(context.Background(), handle, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "checkpoint"}); err != nil {
		t.Fatalf("append gap recovery fixture delta: %v", err)
	}
	gapEvent := receiveRuntimeRecoveryFixtureEvent(t, backend.events)
	if delayedEvent.Seq != runtimeWireEventSeq(baseline)+1 || gapEvent.Seq != delayedEvent.Seq+1 {
		t.Fatalf("recovery fixture sequence = baseline:%d delayed:%d gap:%d", runtimeWireEventSeq(baseline), delayedEvent.Seq, gapEvent.Seq)
	}

	checkpoint := readRuntimeWireEventUntil(t, client, func(event runtimeWireEvent) bool {
		return runtimeWireEventSeq(event) >= gapEvent.Seq &&
			(runtimeWireEventType(event) == sessionruntime.EventRuntimeSnapshot ||
				runtimeWireEventType(event) == sessionruntime.EventRuntimeDelta ||
				runtimeWireEventType(event) == sessionruntime.EventRuntimeDropped)
	})
	if runtimeWireEventType(checkpoint) != sessionruntime.EventRuntimeSnapshot || runtimeWireEventSeq(checkpoint) != gapEvent.Seq {
		t.Fatalf("manager gap recovery event = %#v, want checkpoint at seq %d", checkpoint, gapEvent.Seq)
	}

	if _, err := manager.HandleAgentEvent(context.Background(), handle, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: " continued"}); err != nil {
		t.Fatalf("append post-recovery fixture delta: %v", err)
	}
	postRecovery := readRuntimeWireEventUntil(t, client, func(event runtimeWireEvent) bool {
		return runtimeWireEventSeq(event) >= gapEvent.Seq+1 &&
			(runtimeWireEventType(event) == sessionruntime.EventRuntimeSnapshot ||
				runtimeWireEventType(event) == sessionruntime.EventRuntimeDelta ||
				runtimeWireEventType(event) == sessionruntime.EventRuntimeDropped)
	})
	if runtimeWireEventType(postRecovery) != sessionruntime.EventRuntimeDelta || runtimeWireEventSeq(postRecovery) != gapEvent.Seq+1 {
		t.Fatalf("post-recovery event = %#v, want continuous delta at seq %d", postRecovery, gapEvent.Seq+1)
	}

	delayed := runtimeFixtureWireEvent(t, delayedEvent)
	gap := runtimeFixtureWireEvent(t, gapEvent)
	for _, event := range []runtimeWireEvent{delayed, gap, checkpoint, postRecovery} {
		normalizeRuntimeFixtureWireEvent(event)
	}
	return runtimeRecoveryContractFixture{
		Version:           1,
		Scenario:          "gap_checkpoint_recovery",
		RuntimeSnapshot:   baseline,
		GapDelta:          gap,
		DelayedDelta:      delayed,
		RuntimeCheckpoint: checkpoint,
		PostRecoveryDelta: postRecovery,
	}
}

func buildRichActiveRunContractFixture(t *testing.T) runtimeContractFixture {
	t.Helper()
	snapshotEvent, stream := buildRuntimeContractSnapshot(t, runtimeFixtureStreamID, richActiveRunActiveAgentContractScript())
	terminalSnapshotEvent, fullStream := buildRuntimeContractSnapshot(t, runtimeFixtureStreamID, richActiveRunAgentContractScript())
	if runtimeWireRunStatus(terminalSnapshotEvent) != sessionruntime.RunStatusCompleted {
		t.Fatalf("rich terminal fixture status = %#v, want completed", terminalSnapshotEvent)
	}
	terminalStream := make([]runtimeWireEvent, 0, len(fullStream))
	for _, event := range fullStream {
		if runtimeWireEventSeq(event) > runtimeWireEventSeq(snapshotEvent) {
			terminalStream = append(terminalStream, event)
		}
	}
	if len(terminalStream) == 0 {
		t.Fatal("rich terminal fixture is missing terminal deltas")
	}
	return runtimeContractFixture{
		Version:                6,
		Scenario:               "rich_active_run",
		RuntimeSnapshot:        snapshotEvent,
		RuntimeStream:          stream,
		RuntimeTerminalStream:  terminalStream,
		RuntimeAdmissionStream: buildRuntimeAdmissionStream(t),
		RuntimeResetStream:     buildRuntimeResetStream(t),
		RuntimeSteerStream:     buildRuntimeSteerStream(t),
	}
}

func buildInterruptedRunContractFixture(t *testing.T) runtimeContractFixture {
	t.Helper()
	snapshotEvent, stream := buildRuntimeContractSnapshot(t, runtimeFixtureInterruptedID, interruptedRunAgentContractScript())
	_, abortStream := buildRuntimeContractSnapshot(t, runtimeFixtureInterruptedID, committedAbortAgentContractScript())
	return runtimeContractFixture{
		Version:            5,
		Scenario:           "interrupted_run",
		RuntimeSnapshot:    snapshotEvent,
		RuntimeStream:      stream,
		RuntimeAbortStream: abortStream,
	}
}

func buildRuntimeReplacementContractFixture(t *testing.T) runtimeReplacementContractFixture {
	t.Helper()
	return runtimeReplacementContractFixture{
		Version: 2,
		RetrySnapshot: buildRuntimeOperationEvent(t, "stream-retry-operation", &sessionruntime.RunOperationView{
			Kind:                 sessionruntime.RunOperationRetry,
			ReplaceFromMessageID: "assistant-old",
		}),
		EditSnapshot: buildRuntimeOperationEvent(t, "stream-edit-operation", &sessionruntime.RunOperationView{
			Kind:                 sessionruntime.RunOperationEdit,
			ReplaceFromMessageID: "user-old",
			ReplacementUserTurn: &conversation.UITurn{
				Role:      "user",
				Text:      "edited prompt",
				Platform:  "local",
				Timestamp: runtimeFixtureStartTime,
			},
		}),
	}
}

func buildRuntimeOperationEvent(t *testing.T, streamID string, operation *sessionruntime.RunOperationView) runtimeWireEvent {
	t.Helper()
	manager := sessionruntime.NewManager(sessionruntime.NewMemoryBackend(), runtimeFixtureManagerOptions(streamID))
	defer func() { _ = manager.Close() }()
	if _, err := manager.StartRunWithOptions(context.Background(), sessionruntime.RunStartOptions{
		BotID: runtimeContractBotID, SessionID: runtimeContractSessionID, StreamID: streamID,
		Admission: sessionruntime.RunAdmissionView{Operation: operation},
		AbortCh:   make(chan struct{}, 1), Cancel: func() {}, InjectCh: make(chan conversation.InjectMessage, 1),
	}); err != nil {
		t.Fatalf("start operation fixture run: %v", err)
	}
	event, _ := captureRuntimeFixtureSnapshot(t, manager)
	if snapshot, ok := event["snapshot"].(map[string]any); ok {
		snapshot["updated_at"] = runtimeFixtureStartTime.Format(time.RFC3339)
		if run, ok := snapshot["current_run_view"].(map[string]any); ok {
			run["started_at"] = runtimeFixtureStartTime.Format(time.RFC3339)
			run["updated_at"] = runtimeFixtureStartTime.Format(time.RFC3339)
		}
	}
	return event
}

func runtimeFixtureManagerOptions(streamID string) sessionruntime.Options {
	return sessionruntime.Options{
		OwnerID:                runtimeFixtureOwnerID,
		StateTTL:               time.Hour,
		OwnerLeaseTTL:          time.Minute,
		Logger:                 slog.Default(),
		EpochGenerator:         func() string { return "epoch-runtime-contract-v1" },
		RunGenerationGenerator: func() string { return "generation-" + streamID },
	}
}

func runtimeWireRunStatus(event runtimeWireEvent) string {
	if snapshot, ok := event["snapshot"].(map[string]any); ok {
		if run, ok := snapshot["current_run_view"].(map[string]any); ok {
			status, _ := run["status"].(string)
			return status
		}
	}
	if delta, ok := event["delta"].(map[string]any); ok {
		if run, ok := delta["current_run_view"].(map[string]any); ok {
			status, _ := run["status"].(string)
			return status
		}
		if run, ok := delta["run"].(map[string]any); ok {
			status, _ := run["status"].(string)
			return status
		}
	}
	return ""
}

func runtimeWireSteerStatus(event runtimeWireEvent) string {
	delta, _ := event["delta"].(map[string]any)
	if run, ok := delta["current_run_view"].(map[string]any); ok {
		if steer, ok := run["steer"].(map[string]any); ok {
			status, _ := steer["status"].(string)
			return status
		}
	}
	if run, ok := delta["run"].(map[string]any); ok {
		if steer, ok := run["steer"].(map[string]any); ok {
			status, _ := steer["status"].(string)
			return status
		}
	}
	return ""
}

func readRuntimeWireEventUntil(t *testing.T, client *websocket.Conn, pred func(runtimeWireEvent) bool) runtimeWireEvent {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var events []runtimeWireEvent
	for {
		if err := client.SetReadDeadline(deadline); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}
		_, raw, err := client.ReadMessage()
		if err != nil {
			t.Fatalf("read runtime wire event: %v; events=%#v", err, events)
		}
		var event runtimeWireEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("decode runtime wire event: %v; raw=%s", err, raw)
		}
		events = append(events, event)
		if pred(event) {
			return event
		}
	}
}
