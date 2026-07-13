package sessionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
)

const (
	testBotID     = "bot-runtime"
	testSessionID = "session-runtime"
	testStreamID  = "stream-runtime"
)

type runtimeBackendContractSuite struct {
	newBackend        func(t *testing.T) Backend
	newSharedBackends func(t *testing.T, count int) []Backend
}

func memoryRuntimeBackendSuite() runtimeBackendContractSuite {
	return runtimeBackendContractSuite{
		newBackend: func(t *testing.T) Backend {
			t.Helper()
			return NewMemoryBackend()
		},
		newSharedBackends: func(t *testing.T, count int) []Backend {
			t.Helper()
			backend := NewMemoryBackend()
			backends := make([]Backend, count)
			for i := range backends {
				backends[i] = backend
			}
			return backends
		},
	}
}

func testRuntimeManager(t *testing.T, backend Backend, ownerID string) *Manager {
	t.Helper()
	manager := NewManager(backend, Options{
		OwnerID:       ownerID,
		StateTTL:      time.Hour,
		OwnerLeaseTTL: 2 * time.Second,
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(func() {
		_ = manager.Close()
	})
	return manager
}

func testRuntimeManagerWithOptions(t *testing.T, backend Backend, opts Options) *Manager {
	t.Helper()
	manager := NewManager(backend, opts)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(func() {
		_ = manager.Close()
	})
	return manager
}

type gatedCommandSubscribeBackend struct {
	DistributedBackend
	entered  chan struct{}
	release  chan struct{}
	commands chan Command
	closed   chan struct{}
	once     sync.Once
}

func (b *gatedCommandSubscribeBackend) SubscribeCommands(context.Context, string) (CommandSubscription, error) {
	close(b.entered)
	<-b.release
	return CommandSubscription{C: b.commands, Close: b.closeSubscription}, nil
}

func (*gatedCommandSubscribeBackend) Close() error {
	return nil
}

func (b *gatedCommandSubscribeBackend) closeSubscription() {
	b.once.Do(func() {
		close(b.commands)
		close(b.closed)
	})
}

type activationGateBackend struct {
	Backend
	calls   atomic.Int64
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *activationGateBackend) Update(ctx context.Context, key Key, update SnapshotUpdate) (Snapshot, bool, error) {
	if b.calls.Add(1) == 2 {
		b.once.Do(func() { close(b.entered) })
		select {
		case <-b.release:
		case <-ctx.Done():
			return Snapshot{}, false, ctx.Err()
		}
	}
	return b.Backend.Update(ctx, key, update)
}

func richRuntimeAgentScript() []agentpkg.StreamEvent {
	return []agentpkg.StreamEvent{
		{Type: agentpkg.EventAgentStart},
		{Type: agentpkg.EventReasoningDelta, Delta: "I need to inspect the workspace."},
		{Type: agentpkg.EventTextDelta, Delta: "I will check the current state."},
		{Type: agentpkg.EventToolCallStart, ToolName: "exec", ToolCallID: "call-exec", Input: map[string]any{"command": "pwd"}},
		{Type: agentpkg.EventToolCallProgress, ToolName: "exec", ToolCallID: "call-exec", Progress: "queued"},
		{Type: agentpkg.EventToolCallEnd, ToolName: "exec", ToolCallID: "call-exec", Result: map[string]any{"stdout": "/workspace\n"}},
		{Type: agentpkg.EventToolApprovalRequest, ToolName: "exec", ToolCallID: "call-approval", Input: map[string]any{"command": "rm -rf build"}, ApprovalID: "approval-1", ShortID: 7, Status: "pending"},
		{Type: agentpkg.EventUserInputRequest, ToolName: "ask_user", ToolCallID: "call-ask", Input: map[string]any{"questions": []any{map[string]any{"text": "Continue?", "kind": "single_select"}}}, UserInputID: "input-1", ShortID: 8, Status: "pending", Metadata: map[string]any{"ui_payload": map[string]any{"version": 2, "questions": []any{map[string]any{"id": "q1", "text": "Continue?", "kind": "single_select"}}}}},
	}
}

func TestMemoryRuntimeManagerContract(t *testing.T) {
	t.Parallel()
	runCommonRuntimeManagerContract(t, memoryRuntimeBackendSuite())
}

func waitRuntimeEvent(t *testing.T, events <-chan Event, pred func(Event) bool) Event {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("runtime subscription closed")
			}
			if pred(event) {
				return event
			}
		case <-deadline:
			t.Fatal("timed out waiting for runtime event")
		}
	}
}

func receiveTestResult[T any](t *testing.T, label string, ch <-chan T) T {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
		var zero T
		return zero
	}
}

func TestRuntimeManagerRejectsRunAfterClose(t *testing.T) {
	manager := NewManager(NewMemoryBackend(), Options{})
	if err := manager.Close(); err != nil {
		t.Fatalf("close manager: %v", err)
	}
	err := manager.StartRun(
		context.Background(),
		testBotID,
		testSessionID,
		"stream-after-close",
		make(chan struct{}, 1),
		func() {},
		make(chan conversation.InjectMessage, 1),
	)
	if !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("start run after close error = %v, want ErrManagerClosed", err)
	}
	if ctrl := manager.localControl("stream-after-close"); ctrl != nil {
		t.Fatalf("closed manager retained run control: %#v", ctrl)
	}
}

func TestRuntimeManagerDoesNotActivateAdmissionAfterClose(t *testing.T) {
	backend := NewMemoryBackend()
	manager := NewManager(backend, Options{})
	builderStarted := make(chan struct{})
	releaseBuilder := make(chan struct{})
	startDone := make(chan error, 1)
	go func() {
		startDone <- manager.StartRunWithAdmissionBuilder(
			context.Background(),
			testBotID,
			"session-close-during-admission",
			"stream-close-during-admission",
			func(context.Context) (RunAdmissionView, error) {
				close(builderStarted)
				<-releaseBuilder
				return RunAdmissionView{}, nil
			},
			make(chan struct{}, 1),
			func() {},
			make(chan conversation.InjectMessage, 1),
		)
	}()
	<-builderStarted

	shutdownCtx, cancelShutdown := context.WithCancel(context.Background())
	cancelShutdown()
	if err := manager.CloseContext(shutdownCtx); err != nil {
		t.Fatalf("close manager: %v", err)
	}
	close(releaseBuilder)
	if err := receiveTestResult(t, "closed admission", startDone); !errors.Is(err, ErrManagerClosed) && !errors.Is(err, ErrRunOwnershipLost) {
		t.Fatalf("closed admission error = %v, want manager closed or ownership lost", err)
	}
	snapshot, ok, err := backend.Load(context.Background(), Key{BotID: testBotID, SessionID: "session-close-during-admission"})
	if err != nil || !ok {
		t.Fatalf("load closed admission = ok:%v err:%v", ok, err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Status == RunStatusRunning {
		t.Fatalf("closed admission snapshot = %#v, must not activate", snapshot.CurrentRunView)
	}
}

func TestRuntimeManagerDoesNotStartCommandLoopAfterClose(t *testing.T) {
	backend := &gatedCommandSubscribeBackend{
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
		commands: make(chan Command),
		closed:   make(chan struct{}),
	}
	t.Cleanup(backend.closeSubscription)
	manager := NewManager(backend, Options{OwnerID: "owner-start-close-race"})
	t.Cleanup(func() { _ = manager.Close() })
	startDone := make(chan error, 1)
	go func() {
		startDone <- manager.Start(context.Background())
	}()
	<-backend.entered
	if err := manager.Close(); err != nil {
		t.Fatalf("close manager: %v", err)
	}
	close(backend.release)
	if err := receiveTestResult(t, "manager start after close", startDone); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("start after close error = %v, want ErrManagerClosed", err)
	}
	receiveTestResult(t, "late command subscription close", backend.closed)
}

func TestRuntimeManagerDoesNotTerminalizeAdmissionRejectedByClose(t *testing.T) {
	backend := &activationGateBackend{
		Backend: NewMemoryBackend(),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	manager := NewManager(backend, Options{})
	startDone := make(chan error, 1)
	go func() {
		startDone <- manager.StartRun(
			context.Background(), testBotID, "session-close-activation", "stream-close-activation",
			make(chan struct{}, 1), func() {}, make(chan conversation.InjectMessage, 1),
		)
	}()
	<-backend.entered
	closeDone := make(chan error, 1)
	go func() { closeDone <- manager.Close() }()
	deadline := time.After(time.Second)
	for !manager.isClosed() {
		select {
		case <-deadline:
			t.Fatal("manager close did not start")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(backend.release)
	if err := receiveTestResult(t, "activation rejected by close", startDone); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("start run error = %v, want ErrManagerClosed", err)
	}
	if err := receiveTestResult(t, "manager close after activation rejection", closeDone); err != nil {
		t.Fatalf("close manager: %v", err)
	}
	snapshot, ok, err := backend.Load(context.Background(), Key{BotID: testBotID, SessionID: "session-close-activation"})
	if err != nil || !ok {
		t.Fatalf("load rejected activation = ok:%v err:%v", ok, err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Status != RunStatusAdmitting {
		t.Fatalf("rejected activation = %#v, want admitting reservation", snapshot.CurrentRunView)
	}
}

func TestMemoryBackendExpiresSnapshots(t *testing.T) {
	backend := NewMemoryBackendWithTTL(30 * time.Millisecond)
	t.Cleanup(func() { _ = backend.Close() })
	key := Key{BotID: testBotID, SessionID: "session-memory-ttl"}
	if _, _, err := backend.Update(context.Background(), key, func(snapshot Snapshot, _ bool) (Snapshot, bool, error) {
		snapshot.BotID = key.BotID
		snapshot.SessionID = key.SessionID
		snapshot.Seq = 1
		snapshot.Queue = []QueuedRunView{}
		return snapshot, true, nil
	}); err != nil {
		t.Fatalf("start memory run: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if _, ok, err := backend.Load(context.Background(), key); err != nil || ok {
		t.Fatalf("expired snapshot = ok:%v err:%v", ok, err)
	}
}

func TestLocalRuntimeDoesNotUseOwnerLeases(t *testing.T) {
	t.Parallel()

	manager := testRuntimeManagerWithOptions(t, NewMemoryBackendWithTTL(20*time.Millisecond), Options{
		OwnerID:       "must-not-leak-into-local-state",
		StateTTL:      20 * time.Millisecond,
		OwnerLeaseTTL: 20 * time.Millisecond,
	})
	if err := manager.StartRun(context.Background(), testBotID, "session-local-no-lease", "stream-local-no-lease", make(chan struct{}, 1), func() {}, make(chan conversation.InjectMessage, 1)); err != nil {
		t.Fatalf("start local run: %v", err)
	}
	ctrl := manager.localControl("stream-local-no-lease")
	if ctrl == nil {
		t.Fatal("local run control missing")
	}
	if ctrl.leaseStop != nil || ctrl.leaseDone != nil || !ctrl.leaseValidUntil.IsZero() {
		t.Fatalf("local control unexpectedly has lease state: %#v", ctrl)
	}

	time.Sleep(3 * 20 * time.Millisecond)
	if err := manager.ValidateRunOwnership(context.Background(), testBotID, "session-local-no-lease", "stream-local-no-lease"); err != nil {
		t.Fatalf("local ownership expired after lease TTL: %v", err)
	}
	if _, err := manager.HandleAgentEvent(context.Background(), testBotID, "session-local-no-lease", "stream-local-no-lease", agentpkg.StreamEvent{Type: agentpkg.EventTextDelta, Delta: "still running"}); err != nil {
		t.Fatalf("handle local event after lease TTL: %v", err)
	}
	snapshot, err := manager.Snapshot(context.Background(), testBotID, "session-local-no-lease")
	if err != nil {
		t.Fatalf("load local snapshot: %v", err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Status != RunStatusRunning {
		t.Fatalf("local run = %#v, want running", snapshot.CurrentRunView)
	}
	if snapshot.CurrentRunView.OwnerID != "" || snapshot.CurrentRunView.OwnerLeaseExpiresAt != nil {
		t.Fatalf("local run leaked distributed ownership: %#v", snapshot.CurrentRunView)
	}
	assertRuntimeBlock(t, snapshot.CurrentRunView.Messages, conversation.UIMessageText, "", "still running")
}

func runCommonRuntimeManagerContract(t *testing.T, suite runtimeBackendContractSuite) {
	t.Helper()
	t.Run("snapshots rich active run and publishes deltas", func(t *testing.T) {
		t.Parallel()
		runRuntimeManagerSnapshotsRichActiveRunContract(t, suite)
	})
	t.Run("shares validated replacement operation snapshots", func(t *testing.T) {
		t.Parallel()
		runRuntimeManagerSharesReplacementOperationContract(t, suite)
	})
	t.Run("shares canonical request user turn snapshots", func(t *testing.T) {
		t.Parallel()
		runRuntimeManagerSharesRequestUserTurnContract(t, suite)
	})
	t.Run("keeps queued steer valid past command acknowledgement timeout", func(t *testing.T) {
		t.Parallel()
		runRuntimeManagerKeepsQueuedSteerPastAckTimeoutContract(t, suite)
	})
	t.Run("rejects queued steer when the run finishes", func(t *testing.T) {
		t.Parallel()
		runRuntimeManagerRejectsQueuedSteerOnFinishContract(t, suite)
	})
	t.Run("rejects queued steer when the agent sends a terminal event", func(t *testing.T) {
		t.Parallel()
		runRuntimeManagerRejectsQueuedSteerOnAgentTerminalContract(t, suite)
	})
	t.Run("signals subscriber buffer overflow", func(t *testing.T) {
		t.Parallel()
		runRuntimeManagerSignalsSubscriberOverflowContract(t, suite)
	})
	t.Run("keeps errored streams errored when abort terminal follows", func(t *testing.T) {
		t.Parallel()
		runRuntimeManagerKeepsErroredStreamErroredAfterAbortContract(t, suite)
	})
	t.Run("keeps errored streams errored when end terminal follows", func(t *testing.T) {
		t.Parallel()
		runRuntimeManagerKeepsErroredStreamErroredAfterEndContract(t, suite)
	})
	t.Run("serializes concurrent snapshot updates", func(t *testing.T) {
		t.Parallel()
		runRuntimeBackendSerializesConcurrentSnapshotUpdatesContract(t, suite)
	})
}

func runRuntimeManagerKeepsQueuedSteerPastAckTimeoutContract(t *testing.T, suite runtimeBackendContractSuite) {
	t.Helper()

	const commandAckTTL = 500 * time.Millisecond
	manager := testRuntimeManagerWithOptions(t, suite.newBackend(t), Options{
		OwnerID:       "owner-queued-steer",
		StateTTL:      time.Hour,
		OwnerLeaseTTL: time.Second,
		CommandAckTTL: commandAckTTL,
	})
	injectCh := make(chan conversation.InjectMessage, 1)
	if err := manager.StartRun(context.Background(), testBotID, testSessionID, testStreamID, make(chan struct{}, 1), func() {}, injectCh); err != nil {
		t.Fatalf("start run: %v", err)
	}
	steer, err := manager.Steer(context.Background(), testBotID, testSessionID, testStreamID, "wait for the next model step")
	if err != nil {
		t.Fatalf("steer: %v", err)
	}

	queued := waitRuntimeSnapshot(t, manager, testBotID, testSessionID, func(snapshot Snapshot) bool {
		return snapshot.CurrentRunView != nil &&
			snapshot.CurrentRunView.Steer != nil &&
			snapshot.CurrentRunView.Steer.ID == steer.ID &&
			snapshot.CurrentRunView.Steer.Status == SteerStatusQueued
	})
	if queued.CurrentRunView == nil || queued.CurrentRunView.Steer == nil {
		t.Fatal("queued steer is missing")
	}
	time.Sleep(2 * commandAckTTL)
	snapshot, err := manager.Snapshot(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("snapshot queued steer after acknowledgement timeout: %v", err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Steer == nil || snapshot.CurrentRunView.Steer.ID != steer.ID || snapshot.CurrentRunView.Steer.Status != SteerStatusQueued {
		t.Fatalf("steer after acknowledgement timeout = %#v, want queued", snapshot.CurrentRunView)
	}

	select {
	case injected := <-injectCh:
		if injected.Applied == nil {
			t.Fatal("queued steer is missing its applied acknowledgement")
		}
		injected.Applied()
	case <-time.After(time.Second):
		t.Fatal("queued steer was not delivered to the agent")
	}
	waitRuntimeSnapshot(t, manager, testBotID, testSessionID, func(snapshot Snapshot) bool {
		return snapshot.CurrentRunView != nil && snapshot.CurrentRunView.Steer != nil && snapshot.CurrentRunView.Steer.Status == SteerStatusApplied
	})
}

func TestRuntimeManagerPublishesAdmittingCheckpoint(t *testing.T) {
	backend := NewMemoryBackend()
	manager := testRuntimeManager(t, backend, "owner-admitting")
	sub, err := manager.Subscribe(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()

	builderStarted := make(chan struct{})
	releaseBuilder := make(chan struct{})
	startDone := make(chan error, 1)
	go func() {
		startDone <- manager.StartRunWithOperationBuilder(
			context.Background(),
			testBotID,
			testSessionID,
			"stream-admitting",
			func(context.Context) (*RunOperationView, error) {
				close(builderStarted)
				<-releaseBuilder
				return &RunOperationView{Kind: RunOperationRetry, ReplaceFromMessageID: "assistant-old"}, nil
			},
			make(chan struct{}, 1),
			func() {},
			make(chan conversation.InjectMessage, 1),
		)
	}()
	<-builderStarted

	snapshot, err := manager.Snapshot(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("snapshot reserved run: %v", err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Status != RunStatusAdmitting {
		t.Fatalf("reserved run = %#v, want admitting", snapshot.CurrentRunView)
	}
	event := waitRuntimeEvent(t, sub.C, func(event Event) bool {
		return event.Delta != nil && event.Delta.CurrentRunView != nil && event.Delta.CurrentRunView.Status == RunStatusAdmitting
	})
	if event.Snapshot != nil || event.Seq != snapshot.Seq {
		t.Fatalf("admitting checkpoint = %#v", event)
	}

	close(releaseBuilder)
	if err := <-startDone; err != nil {
		t.Fatalf("activate run: %v", err)
	}
	event = waitRuntimeEvent(t, sub.C, func(event Event) bool {
		return event.Delta != nil && event.Delta.CurrentRunView != nil && event.Delta.CurrentRunView.Status == RunStatusRunning
	})
	if event.Delta == nil || event.Delta.CurrentRunView == nil || event.Delta.CurrentRunView.Status != RunStatusRunning || event.Snapshot != nil {
		t.Fatalf("activated event = %#v", event)
	}
}

func TestRuntimeManagerBuilderFailureReleasesReservation(t *testing.T) {
	backend := NewMemoryBackend()
	manager := testRuntimeManager(t, backend, "owner-builder-failure")
	sub, err := manager.Subscribe(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()
	err = manager.StartRunWithOperationBuilder(
		context.Background(),
		testBotID,
		testSessionID,
		"stream-builder-failure",
		func(context.Context) (*RunOperationView, error) { return nil, errors.New("prepare operation failed") },
		make(chan struct{}, 1),
		func() {},
		make(chan conversation.InjectMessage, 1),
	)
	if err == nil || !strings.Contains(err.Error(), "prepare operation failed") {
		t.Fatalf("builder error = %v", err)
	}
	if _, ok, err := manager.StreamRef(context.Background(), "stream-builder-failure"); err != nil || ok {
		t.Fatalf("reservation stream ref = ok:%v err:%v", ok, err)
	}
	snapshot, err := manager.Snapshot(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Status != RunStatusErrored {
		t.Fatalf("failed admission snapshot = %#v", snapshot.CurrentRunView)
	}
	event := waitRuntimeEvent(t, sub.C, func(event Event) bool { return event.Seq == snapshot.Seq })
	if event.Delta == nil || event.Delta.CurrentRunView == nil || event.Delta.CurrentRunView.Status != RunStatusErrored {
		t.Fatalf("failed admission event = %#v, want self-contained terminal run", event)
	}
}

func runRuntimeManagerRejectsQueuedSteerOnFinishContract(t *testing.T, suite runtimeBackendContractSuite) {
	t.Helper()

	manager := testRuntimeManager(t, suite.newBackend(t), "owner-finished-steer")
	injectCh := make(chan conversation.InjectMessage, 1)
	if err := manager.StartRun(context.Background(), testBotID, testSessionID, testStreamID, make(chan struct{}, 1), func() {}, injectCh); err != nil {
		t.Fatalf("start run: %v", err)
	}
	steer, err := manager.Steer(context.Background(), testBotID, testSessionID, testStreamID, "adjust before finish")
	if err != nil {
		t.Fatalf("steer: %v", err)
	}
	waitRuntimeSnapshot(t, manager, testBotID, testSessionID, func(snapshot Snapshot) bool {
		return snapshot.CurrentRunView != nil && snapshot.CurrentRunView.Steer != nil &&
			snapshot.CurrentRunView.Steer.ID == steer.ID && snapshot.CurrentRunView.Steer.Status == SteerStatusQueued
	})
	if err := manager.FinishRun(context.Background(), testBotID, testSessionID, testStreamID, RunStatusCompleted, ""); err != nil {
		t.Fatalf("finish run: %v", err)
	}
	snapshot, err := manager.Snapshot(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Steer == nil || snapshot.CurrentRunView.Steer.Status != SteerStatusRejected {
		t.Fatalf("finished steer = %#v, want rejected", snapshot.CurrentRunView)
	}
	if snapshot.CurrentRunView.Steer.Error != steerRunFinishedError {
		t.Fatalf("finished steer error = %q", snapshot.CurrentRunView.Steer.Error)
	}
	select {
	case injected := <-injectCh:
		if injected.Applied != nil {
			injected.Applied()
		}
	default:
		t.Fatal("queued steer was not delivered")
	}
	snapshot, err = manager.Snapshot(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("snapshot after late apply: %v", err)
	}
	if snapshot.CurrentRunView.Steer.Status != SteerStatusRejected {
		t.Fatalf("late apply changed terminal steer = %#v", snapshot.CurrentRunView.Steer)
	}
}

func runRuntimeManagerRejectsQueuedSteerOnAgentTerminalContract(t *testing.T, suite runtimeBackendContractSuite) {
	t.Helper()

	for _, tc := range []struct {
		name       string
		event      agentpkg.StreamEvent
		wantStatus string
	}{
		{name: "end", event: agentpkg.StreamEvent{Type: agentpkg.EventAgentEnd}, wantStatus: RunStatusCompleted},
		{name: "abort", event: agentpkg.StreamEvent{Type: agentpkg.EventAgentAbort}, wantStatus: RunStatusAborted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := testRuntimeManager(t, suite.newBackend(t), "owner-agent-terminal-steer-"+tc.name)
			sub, err := manager.Subscribe(context.Background(), testBotID, testSessionID)
			if err != nil {
				t.Fatalf("subscribe: %v", err)
			}
			defer sub.Close()
			injectCh := make(chan conversation.InjectMessage, 1)
			if err := manager.StartRun(context.Background(), testBotID, testSessionID, testStreamID, make(chan struct{}, 1), func() {}, injectCh); err != nil {
				t.Fatalf("start run: %v", err)
			}
			steer, err := manager.Steer(context.Background(), testBotID, testSessionID, testStreamID, "adjust before agent "+tc.name)
			if err != nil {
				t.Fatalf("steer: %v", err)
			}
			select {
			case <-injectCh:
			case <-time.After(time.Second):
				t.Fatal("queued steer was not delivered")
			}
			waitRuntimeSnapshot(t, manager, testBotID, testSessionID, func(snapshot Snapshot) bool {
				return snapshot.CurrentRunView != nil && snapshot.CurrentRunView.Steer != nil &&
					snapshot.CurrentRunView.Steer.ID == steer.ID && snapshot.CurrentRunView.Steer.Status == SteerStatusQueued
			})
			if _, err := manager.HandleAgentEvent(context.Background(), testBotID, testSessionID, testStreamID, tc.event); err != nil {
				t.Fatalf("handle agent %s: %v", tc.name, err)
			}

			snapshot, err := manager.Snapshot(context.Background(), testBotID, testSessionID)
			if err != nil {
				t.Fatalf("snapshot: %v", err)
			}
			if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Steer == nil || snapshot.CurrentRunView.Steer.Status != SteerStatusRejected {
				t.Fatalf("agent-terminal steer = %#v, want rejected", snapshot.CurrentRunView)
			}
			if snapshot.CurrentRunView.Steer.Error != steerRunFinishedError {
				t.Fatalf("agent-terminal steer error = %q", snapshot.CurrentRunView.Steer.Error)
			}
			event := waitRuntimeEvent(t, sub.C, func(event Event) bool {
				return event.Delta != nil && event.Delta.Run != nil && event.Delta.Run.Status != nil && *event.Delta.Run.Status == tc.wantStatus
			})
			if event.Delta.Run.Steer == nil || event.Delta.Run.Steer.Status != SteerStatusRejected {
				t.Fatalf("agent-terminal delta = %#v, want rejected steer", event.Delta)
			}
		})
	}
}

func runRuntimeManagerSharesReplacementOperationContract(t *testing.T, suite runtimeBackendContractSuite) {
	t.Helper()

	backends := suite.newSharedBackends(t, 2)
	owner := testRuntimeManager(t, backends[0], "owner-operation")
	observer := testRuntimeManager(t, backends[1], "observer-operation")
	replacement := &conversation.UITurn{
		Role:      "user",
		Text:      "edited prompt",
		Timestamp: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC),
	}
	operation := &RunOperationView{
		Kind:                 RunOperationEdit,
		ReplaceFromMessageID: "user-old",
		ReplacementUserTurn:  replacement,
	}
	if err := owner.StartRunWithOperation(
		context.Background(),
		testBotID,
		testSessionID,
		testStreamID,
		operation,
		make(chan struct{}, 1),
		func() {},
		make(chan conversation.InjectMessage, 1),
	); err != nil {
		t.Fatalf("start replacement run: %v", err)
	}

	snapshot, err := observer.Snapshot(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("observer snapshot: %v", err)
	}
	got := snapshot.CurrentRunView
	if got == nil || got.Operation == nil {
		t.Fatalf("current run operation = %#v", got)
	}
	if got.Operation.Kind != RunOperationEdit || got.Operation.ReplaceFromMessageID != "user-old" {
		t.Fatalf("operation = %#v", got.Operation)
	}
	if got.Operation.ReplacementUserTurn == nil || got.Operation.ReplacementUserTurn.Text != "edited prompt" {
		t.Fatalf("replacement user turn = %#v", got.Operation.ReplacementUserTurn)
	}
}

func runRuntimeManagerSharesRequestUserTurnContract(t *testing.T, suite runtimeBackendContractSuite) {
	t.Helper()

	backends := suite.newSharedBackends(t, 2)
	owner := testRuntimeManager(t, backends[0], "owner-request-turn")
	observer := testRuntimeManager(t, backends[1], "observer-request-turn")
	requestTurn := &conversation.UITurn{
		Role:              "user",
		Text:              "inspect the workspace",
		Attachments:       []conversation.UIAttachment{{Type: "file", Name: "notes.txt", ContentHash: "sha256:notes"}},
		Timestamp:         time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC),
		Platform:          "local",
		SenderUserID:      "user-request-turn",
		ExternalMessageID: testStreamID,
	}
	if err := owner.StartRunWithAdmission(
		context.Background(),
		testBotID,
		testSessionID,
		testStreamID,
		RunAdmissionView{RequestUserTurn: requestTurn},
		make(chan struct{}, 1),
		func() {},
		make(chan conversation.InjectMessage, 1),
	); err != nil {
		t.Fatalf("start run with request user turn: %v", err)
	}

	snapshot, err := observer.Snapshot(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("observer snapshot: %v", err)
	}
	got := snapshot.CurrentRunView
	if got == nil || got.RequestUserTurn == nil {
		t.Fatalf("current run request user turn = %#v", got)
	}
	if got.RequestUserTurn.Text != requestTurn.Text || got.RequestUserTurn.ExternalMessageID != testStreamID {
		t.Fatalf("request user turn = %#v", got.RequestUserTurn)
	}
	if len(got.RequestUserTurn.Attachments) != 1 || got.RequestUserTurn.Attachments[0].ContentHash != "sha256:notes" {
		t.Fatalf("request user turn attachments = %#v", got.RequestUserTurn.Attachments)
	}

	requestTurn.Text = "mutated by caller"
	requestTurn.Attachments[0].Name = "mutated.txt"
	if got.RequestUserTurn.Text != "inspect the workspace" || got.RequestUserTurn.Attachments[0].Name != "notes.txt" {
		t.Fatalf("runtime request user turn aliases caller state: %#v", got.RequestUserTurn)
	}
}

func TestRuntimeManagerRejectsNonUserRequestTurn(t *testing.T) {
	t.Parallel()

	manager := testRuntimeManager(t, NewMemoryBackend(), "owner-invalid-request-turn")
	err := manager.StartRunWithAdmission(
		context.Background(),
		testBotID,
		testSessionID,
		"stream-invalid-request-turn",
		RunAdmissionView{RequestUserTurn: &conversation.UITurn{Role: "assistant", Text: "invalid"}},
		make(chan struct{}, 1),
		func() {},
		make(chan conversation.InjectMessage, 1),
	)
	if err == nil || !strings.Contains(err.Error(), "must have role user") {
		t.Fatalf("start run error = %v, want invalid request user turn", err)
	}
	snapshot, snapshotErr := manager.Snapshot(context.Background(), testBotID, testSessionID)
	if snapshotErr != nil {
		t.Fatalf("load errored snapshot: %v", snapshotErr)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Status != RunStatusErrored {
		t.Fatalf("invalid admission snapshot = %#v", snapshot.CurrentRunView)
	}
}

func runRuntimeManagerSnapshotsRichActiveRunContract(t *testing.T, suite runtimeBackendContractSuite) {
	t.Helper()

	manager := testRuntimeManager(t, suite.newBackend(t), "owner-1")
	sub, err := manager.Subscribe(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()

	abortCh := make(chan struct{}, 1)
	injectCh := make(chan conversation.InjectMessage, 1)
	if err := manager.StartRun(context.Background(), testBotID, testSessionID, testStreamID, abortCh, func() {}, injectCh); err != nil {
		t.Fatalf("start run: %v", err)
	}
	for _, event := range richRuntimeAgentScript() {
		if _, err := manager.HandleAgentEvent(context.Background(), testBotID, testSessionID, testStreamID, event); err != nil {
			t.Fatalf("handle event %s: %v", event.Type, err)
		}
	}

	snapshot, err := manager.Snapshot(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.StreamID != testStreamID || snapshot.CurrentRunView.Status != RunStatusRunning {
		t.Fatalf("current run = %#v", snapshot.CurrentRunView)
	}
	assertRuntimeBlock(t, snapshot.CurrentRunView.Messages, conversation.UIMessageReasoning, "", "I need to inspect the workspace.")
	assertRuntimeBlock(t, snapshot.CurrentRunView.Messages, conversation.UIMessageText, "", "I will check the current state.")
	assertRuntimeBlock(t, snapshot.CurrentRunView.Messages, conversation.UIMessageTool, "call-exec", "")
	assertRuntimeBlock(t, snapshot.CurrentRunView.Messages, conversation.UIMessageTool, "call-approval", "")
	assertRuntimeBlock(t, snapshot.CurrentRunView.Messages, conversation.UIMessageTool, "call-ask", "")
	if snapshot.Queue == nil {
		t.Fatal("queue must be an empty array, not nil")
	}

	var sawDelta bool
	deadline := time.After(2 * time.Second)
	for !sawDelta {
		select {
		case event := <-sub.C:
			sawDelta = event.Type == EventRuntimeDelta && event.Seq == snapshot.Seq && event.Delta != nil && event.Snapshot == nil
		case <-deadline:
			t.Fatal("timed out waiting for runtime delta")
		}
	}
}

func TestRuntimeManagerPublishesBoundedTextAppendPayloads(t *testing.T) {
	manager := testRuntimeManager(t, NewMemoryBackend(), "owner-bounded-delta")
	sub, err := manager.Subscribe(context.Background(), testBotID, "session-bounded-delta")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()
	if err := manager.StartRun(context.Background(), testBotID, "session-bounded-delta", "stream-bounded-delta", make(chan struct{}, 1), func() {}, make(chan conversation.InjectMessage, 1)); err != nil {
		t.Fatalf("start run: %v", err)
	}
	_ = waitRuntimeEvent(t, sub.C, func(event Event) bool {
		return event.Type == EventRuntimeDelta && event.Delta != nil && event.Delta.CurrentRunView != nil
	})

	chunk := strings.Repeat("x", 1024)
	var lastWireSize int
	for i := 0; i < 32; i++ {
		if _, err := manager.HandleAgentEvent(context.Background(), testBotID, "session-bounded-delta", "stream-bounded-delta", agentpkg.StreamEvent{
			Type:  agentpkg.EventTextDelta,
			Delta: chunk,
		}); err != nil {
			t.Fatalf("handle text delta %d: %v", i, err)
		}
		event := waitRuntimeEvent(t, sub.C, func(event Event) bool {
			return event.Type == EventRuntimeDelta && event.Delta != nil && len(event.Delta.MessageAppends) == 1
		})
		if event.Snapshot != nil || event.Delta.MessageAppends[0].Content != chunk {
			t.Fatalf("text delta %d = %#v", i, event)
		}
		wire, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal text delta %d: %v", i, err)
		}
		lastWireSize = len(wire)
		if lastWireSize > len(chunk)+512 {
			t.Fatalf("text delta %d wire size = %d, want bounded near chunk size %d", i, lastWireSize, len(chunk))
		}
	}

	snapshot, err := manager.Snapshot(context.Background(), testBotID, "session-bounded-delta")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	snapshotWire, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if lastWireSize*10 >= len(snapshotWire) {
		t.Fatalf("last delta size = %d, snapshot size = %d; delta appears to carry accumulated output", lastWireSize, len(snapshotWire))
	}
}

func TestRuntimeManagerRehydratesSnapshotWhenSequenceEpochResets(t *testing.T) {
	backend := NewMemoryBackend()
	manager := testRuntimeManagerWithOptions(t, backend, Options{
		OwnerID:       "observer-sequence-reset",
		StateTTL:      time.Hour,
		OwnerLeaseTTL: 100 * time.Millisecond,
	})
	key := Key{BotID: testBotID, SessionID: "session-sequence-reset"}
	firstUpdatedAt := time.Now().UTC()
	if _, _, err := backend.Update(context.Background(), key, func(Snapshot, bool) (Snapshot, bool, error) {
		return Snapshot{BotID: key.BotID, SessionID: key.SessionID, Seq: 100, Queue: []QueuedRunView{}, UpdatedAt: firstUpdatedAt}, true, nil
	}); err != nil {
		t.Fatalf("seed first epoch: %v", err)
	}
	sub, err := manager.Subscribe(context.Background(), key.BotID, key.SessionID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()
	_ = waitRuntimeEvent(t, sub.C, func(event Event) bool {
		return event.Type == EventRuntimeSnapshot && event.Seq == 100
	})

	secondUpdatedAt := firstUpdatedAt.Add(time.Second)
	if _, _, err := backend.Update(context.Background(), key, func(Snapshot, bool) (Snapshot, bool, error) {
		return Snapshot{BotID: key.BotID, SessionID: key.SessionID, Seq: 2, Queue: []QueuedRunView{}, UpdatedAt: secondUpdatedAt}, true, nil
	}); err != nil {
		t.Fatalf("seed second epoch: %v", err)
	}
	if err := backend.Publish(context.Background(), Event{
		Type:      EventRuntimeDelta,
		BotID:     key.BotID,
		SessionID: key.SessionID,
		Seq:       2,
		UpdatedAt: &secondUpdatedAt,
		Delta:     &RuntimeDelta{},
	}); err != nil {
		t.Fatalf("publish second epoch delta: %v", err)
	}
	event := waitRuntimeEvent(t, sub.C, func(event Event) bool {
		return event.Type == EventRuntimeSnapshot && event.Seq == 2
	})
	if event.Snapshot == nil || event.Delta != nil || event.Snapshot.Seq != 2 {
		t.Fatalf("sequence reset event = %#v", event)
	}
}

func runRuntimeManagerSignalsSubscriberOverflowContract(t *testing.T, suite runtimeBackendContractSuite) {
	t.Helper()

	backend := suite.newBackend(t)
	manager := testRuntimeManager(t, backend, "observer-overflow")
	sub, err := manager.Subscribe(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Close()
	for seq := int64(1); seq <= 256; seq++ {
		updatedAt := time.Now().UTC()
		if err := backend.Publish(context.Background(), Event{
			Type:      EventRuntimeDelta,
			BotID:     testBotID,
			SessionID: testSessionID,
			Seq:       seq,
			UpdatedAt: &updatedAt,
			Delta:     &RuntimeDelta{},
		}); err != nil {
			t.Fatalf("publish event %d: %v", seq, err)
		}
	}
	time.Sleep(50 * time.Millisecond)

	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-sub.C:
			if event.Type == EventRuntimeDropped {
				return
			}
		case <-deadline:
			t.Fatal("subscriber overflow did not produce runtime_dropped")
		}
	}
}

func runRuntimeManagerKeepsErroredStreamErroredAfterAbortContract(t *testing.T, suite runtimeBackendContractSuite) {
	t.Helper()

	manager := testRuntimeManager(t, suite.newBackend(t), "owner-error")
	if err := manager.StartRun(context.Background(), testBotID, testSessionID, testStreamID, make(chan struct{}, 1), func() {}, make(chan conversation.InjectMessage, 1)); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := manager.HandleAgentEvent(context.Background(), testBotID, testSessionID, testStreamID, agentpkg.StreamEvent{
		Type:  agentpkg.EventError,
		Error: "runtime interrupted",
	}); err != nil {
		t.Fatalf("handle error event: %v", err)
	}
	if _, err := manager.HandleAgentEvent(context.Background(), testBotID, testSessionID, testStreamID, agentpkg.StreamEvent{
		Type: agentpkg.EventAgentAbort,
	}); err != nil {
		t.Fatalf("handle abort terminal event: %v", err)
	}
	if err := manager.FinishRun(context.Background(), testBotID, testSessionID, testStreamID, "", ""); err != nil {
		t.Fatalf("finish errored run: %v", err)
	}

	snapshot, err := manager.Snapshot(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Status != RunStatusErrored {
		t.Fatalf("current run = %#v, want errored", snapshot.CurrentRunView)
	}
	if snapshot.CurrentRunView.Error != "runtime interrupted" {
		t.Fatalf("error = %q, want runtime interrupted", snapshot.CurrentRunView.Error)
	}
}

func runRuntimeManagerKeepsErroredStreamErroredAfterEndContract(t *testing.T, suite runtimeBackendContractSuite) {
	t.Helper()

	manager := testRuntimeManager(t, suite.newBackend(t), "owner-error-end")
	if err := manager.StartRun(context.Background(), testBotID, testSessionID, testStreamID, make(chan struct{}, 1), func() {}, make(chan conversation.InjectMessage, 1)); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := manager.HandleAgentEvent(context.Background(), testBotID, testSessionID, testStreamID, agentpkg.StreamEvent{
		Type:  agentpkg.EventError,
		Error: "provider failed",
	}); err != nil {
		t.Fatalf("handle error event: %v", err)
	}
	if _, err := manager.HandleAgentEvent(context.Background(), testBotID, testSessionID, testStreamID, agentpkg.StreamEvent{
		Type: agentpkg.EventAgentEnd,
	}); err != nil {
		t.Fatalf("handle end terminal event: %v", err)
	}

	snapshot, err := manager.Snapshot(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Status != RunStatusErrored || snapshot.CurrentRunView.Error != "provider failed" {
		t.Fatalf("current run = %#v, want errored provider failure", snapshot.CurrentRunView)
	}
}

func runRuntimeBackendSerializesConcurrentSnapshotUpdatesContract(t *testing.T, suite runtimeBackendContractSuite) {
	t.Helper()

	key := Key{BotID: testBotID, SessionID: "session-concurrent"}
	backends := suite.newSharedBackends(t, 4)
	for _, backend := range backends {
		backend := backend
		t.Cleanup(func() { _ = backend.Close() })
	}
	const updates = 40
	var wg sync.WaitGroup
	errCh := make(chan error, updates)
	for i := 0; i < updates; i++ {
		i := i
		backend := backends[i%len(backends)]
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := backend.Update(context.Background(), key, func(snapshot Snapshot, ok bool) (Snapshot, bool, error) {
				if !ok {
					snapshot = Snapshot{BotID: key.BotID, SessionID: key.SessionID, Queue: []QueuedRunView{}}
				}
				snapshot.Seq++
				snapshot.UpdatedAt = time.Now().UTC()
				snapshot.Queue = append(nonNilQueue(snapshot.Queue), QueuedRunView{StreamID: fmt.Sprintf("queued-%02d", i)})
				return snapshot, true, nil
			})
			if err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("update: %v", err)
		}
	}
	snapshot, ok, err := backends[0].Load(context.Background(), key)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !ok {
		t.Fatal("snapshot missing after concurrent updates")
	}
	if snapshot.Seq != updates {
		t.Fatalf("seq = %d, want %d", snapshot.Seq, updates)
	}
	if len(snapshot.Queue) != updates {
		t.Fatalf("queue len = %d, want %d", len(snapshot.Queue), updates)
	}
}

func assertRuntimeBlock(t *testing.T, messages []conversation.UIMessage, kind conversation.UIMessageType, toolCallID, content string) {
	t.Helper()
	for _, message := range messages {
		if message.Type != kind {
			continue
		}
		if toolCallID != "" && message.ToolCallID != toolCallID {
			continue
		}
		if content != "" && message.Content != content {
			continue
		}
		return
	}
	data, _ := json.Marshal(messages)
	t.Fatalf("missing runtime block kind=%s tool=%q content=%q in %s", kind, toolCallID, content, data)
}

func waitRuntimeSnapshot(t *testing.T, manager *Manager, botID, sessionID string, pred func(Snapshot) bool) Snapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := manager.Snapshot(context.Background(), botID, sessionID)
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		if pred(snapshot) {
			return snapshot
		}
		time.Sleep(10 * time.Millisecond)
	}
	snapshot, _ := manager.Snapshot(context.Background(), botID, sessionID)
	t.Fatalf("timed out waiting for snapshot, last=%#v", snapshot)
	return Snapshot{}
}
