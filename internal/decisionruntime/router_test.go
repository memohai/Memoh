package decisionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/runtimefence"
	"github.com/memohai/memoh/internal/sessionruntime"
)

const (
	decisionBotID     = "11111111-1111-1111-1111-111111111111"
	decisionSessionID = "22222222-2222-2222-2222-222222222222"
	decisionTargetID  = "33333333-3333-3333-3333-333333333333"
)

type routerTestResolver struct {
	prepared         runtimefence.PreservedDecision
	respondApproval  []flow.ToolApprovalResponseInput
	respondInput     []flow.UserInputResponseInput
	allocated        int
	activated        []runtimefence.ActivationOptions
	respondEvents    []agentpkg.StreamEvent
	respondFence     runtimefence.Fence
	reconcileHandled bool
	reconcileErr     error
	reconcileCalls   int
	commitErr        error
	lifecycle        []string
}

func (r *routerTestResolver) AllocateRuntimePersistenceFence(context.Context, string, string) (runtimefence.Fence, error) {
	r.allocated++
	return runtimefence.Fence{BotID: decisionBotID, SessionID: decisionSessionID, Token: 7}, nil
}

func (r *routerTestResolver) ActivateRuntimePersistenceFenceWithOptions(_ context.Context, _ runtimefence.Fence, options runtimefence.ActivationOptions) error {
	r.activated = append(r.activated, options)
	return nil
}

func (r *routerTestResolver) PrepareToolApprovalResponseTarget(context.Context, flow.ToolApprovalResponseInput) (runtimefence.PreservedDecision, error) {
	return r.prepared, nil
}

func (r *routerTestResolver) PrepareUserInputResponseTarget(context.Context, flow.UserInputResponseInput) (runtimefence.PreservedDecision, error) {
	return r.prepared, nil
}

func (r *routerTestResolver) RespondToolApproval(ctx context.Context, input flow.ToolApprovalResponseInput, eventCh chan<- flow.WSStreamEvent) error {
	r.respondApproval = append(r.respondApproval, input)
	r.respondFence, _ = runtimefence.FromContext(ctx)
	return emitRouterTestEvents(eventCh, r.respondEvents)
}

func (r *routerTestResolver) RespondUserInput(ctx context.Context, input flow.UserInputResponseInput, eventCh chan<- flow.WSStreamEvent) error {
	r.respondInput = append(r.respondInput, input)
	r.respondFence, _ = runtimefence.FromContext(ctx)
	return emitRouterTestEvents(eventCh, r.respondEvents)
}

func (r *routerTestResolver) CommitToolApprovalResponse(ctx context.Context, input flow.ToolApprovalResponseInput) (flow.CommittedToolApprovalResponse, error) {
	r.respondApproval = append(r.respondApproval, input)
	r.respondFence, _ = runtimefence.FromContext(ctx)
	r.lifecycle = append(r.lifecycle, "commit")
	return flow.CommittedToolApprovalResponse{}, r.commitErr
}

func (r *routerTestResolver) ContinueCommittedToolApprovalResponse(ctx context.Context, _ flow.CommittedToolApprovalResponse, eventCh chan<- flow.WSStreamEvent) error {
	r.respondFence, _ = runtimefence.FromContext(ctx)
	r.lifecycle = append(r.lifecycle, "continue")
	return emitRouterTestEvents(eventCh, r.respondEvents)
}

func (r *routerTestResolver) CommitUserInputResponse(ctx context.Context, input flow.UserInputResponseInput) (flow.CommittedUserInputResponse, error) {
	r.respondInput = append(r.respondInput, input)
	r.respondFence, _ = runtimefence.FromContext(ctx)
	r.lifecycle = append(r.lifecycle, "commit")
	return flow.CommittedUserInputResponse{}, r.commitErr
}

func (r *routerTestResolver) ContinueCommittedUserInputResponse(ctx context.Context, _ flow.CommittedUserInputResponse, eventCh chan<- flow.WSStreamEvent) error {
	r.respondFence, _ = runtimefence.FromContext(ctx)
	r.lifecycle = append(r.lifecycle, "continue")
	return emitRouterTestEvents(eventCh, r.respondEvents)
}

func (r *routerTestResolver) ReconcileToolApprovalResponse(context.Context, flow.ToolApprovalResponseInput) (bool, error) {
	r.reconcileCalls++
	return r.reconcileHandled, r.reconcileErr
}

func (r *routerTestResolver) ReconcileUserInputResponse(context.Context, flow.UserInputResponseInput) (bool, error) {
	r.reconcileCalls++
	return r.reconcileHandled, r.reconcileErr
}

func (*routerTestResolver) DeferSessionCompaction(string, string, string) func() {
	return func() {}
}

func emitRouterTestEvents(eventCh chan<- flow.WSStreamEvent, events []agentpkg.StreamEvent) error {
	for _, event := range events {
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}
		eventCh <- raw
	}
	return nil
}

type routerTestManager struct {
	distributed       bool
	dispatchHandled   bool
	dispatchErr       error
	invokeHandler     bool
	commandHandler    func(context.Context, sessionruntime.Command) error
	commandReconciler func(context.Context, sessionruntime.Command) (bool, error)
	starts            int
	validations       int
	handledEvents     []agentpkg.StreamEvent
	finalizedEvents   []agentpkg.StreamEvent
	canonicalReady    []bool
	finalizeErr       error
	finishes          int
	ownershipCancel   context.CancelCauseFunc
	admissions        []sessionruntime.RunAdmissionView
}

func (m *routerTestManager) SetCommandHandler(handler func(context.Context, sessionruntime.Command) error) {
	m.commandHandler = handler
}

func (m *routerTestManager) SetCommandReconciler(reconciler func(context.Context, sessionruntime.Command) (bool, error)) {
	m.commandReconciler = reconciler
}

func (m *routerTestManager) DispatchActiveCommand(ctx context.Context, botID, sessionID, commandType, targetID string, payload []byte) (bool, error) {
	if !m.dispatchHandled || !m.invokeHandler || m.commandHandler == nil {
		return m.dispatchHandled, m.dispatchErr
	}
	err := m.commandHandler(ctx, sessionruntime.Command{
		Type: commandType, BotID: botID, SessionID: sessionID, StreamID: "active-stream",
		Generation: "active-generation", TargetID: targetID, Payload: payload,
	})
	return true, err
}

func (m *routerTestManager) IsDistributed() bool { return m.distributed }

func (m *routerTestManager) ValidateRunOwnership(context.Context, sessionruntime.RunHandle) error {
	m.validations++
	return nil
}

func (m *routerTestManager) StartRunWithOptions(ctx context.Context, options sessionruntime.RunStartOptions) (sessionruntime.RunHandle, error) {
	m.starts++
	m.ownershipCancel = options.OwnershipCancel
	handle := sessionruntime.RunHandle{
		BotID: options.BotID, SessionID: options.SessionID, StreamID: options.StreamID, Generation: "generation-1",
	}
	if options.AdmissionBuilder != nil {
		admission, err := options.AdmissionBuilder(ctx, handle)
		if err != nil {
			return sessionruntime.RunHandle{}, err
		}
		m.admissions = append(m.admissions, admission)
	}
	return handle, nil
}

func (m *routerTestManager) HandleAgentEvent(_ context.Context, _ sessionruntime.RunHandle, event agentpkg.StreamEvent) ([]conversation.UIMessage, error) {
	m.handledEvents = append(m.handledEvents, event)
	return nil, nil
}

func (m *routerTestManager) FinalizeAgentEvent(_ context.Context, _ sessionruntime.RunHandle, event agentpkg.StreamEvent, canonicalReady bool, _ string) ([]conversation.UIMessage, error) {
	m.finalizedEvents = append(m.finalizedEvents, event)
	m.canonicalReady = append(m.canonicalReady, canonicalReady)
	return nil, m.finalizeErr
}

func (m *routerTestManager) FinishRun(context.Context, sessionruntime.RunHandle, string, string) error {
	m.finishes++
	if m.ownershipCancel != nil {
		m.ownershipCancel(sessionruntime.ErrRunOwnershipLost)
	}
	return nil
}

func TestRouterRoutesActiveApprovalToCanonicalOwner(t *testing.T) {
	resolver := &routerTestResolver{prepared: runtimefence.PreservedDecision{
		Kind: runtimefence.DecisionToolApproval, ID: decisionTargetID, BotID: decisionBotID, SessionID: decisionSessionID,
	}}
	manager := &routerTestManager{dispatchHandled: true, invokeHandler: true}
	router := newRouter(slog.New(slog.DiscardHandler), manager, resolver)

	err := router.RespondToolApproval(context.Background(), flow.ToolApprovalResponseInput{
		BotID: decisionBotID, ApprovalID: decisionTargetID, Decision: "approve", ChatToken: "secret",
	}, nil)
	if err != nil {
		t.Fatalf("RespondToolApproval() error = %v", err)
	}
	if manager.starts != 0 || resolver.allocated != 0 {
		t.Fatalf("active response started fallback: starts=%d allocated=%d", manager.starts, resolver.allocated)
	}
	if len(resolver.respondApproval) != 1 {
		t.Fatalf("owner response calls = %d, want 1", len(resolver.respondApproval))
	}
	got := resolver.respondApproval[0]
	if !got.ResolveOnly || got.BotID != decisionBotID || got.SessionID != decisionSessionID || got.ExplicitID != decisionTargetID {
		t.Fatalf("owner response input = %#v", got)
	}
	if got.ChatToken != "" {
		t.Fatal("routed command retained transport credential")
	}
}

func TestRouterMemoryFallbackUsesRuntimeLifecycleWithoutFence(t *testing.T) {
	resolver := &routerTestResolver{prepared: runtimefence.PreservedDecision{
		Kind: runtimefence.DecisionUserInput, ID: decisionTargetID, BotID: decisionBotID, SessionID: decisionSessionID,
	}}
	manager := &routerTestManager{}
	router := newRouter(slog.New(slog.DiscardHandler), manager, resolver)

	if err := router.RespondUserInput(context.Background(), flow.UserInputResponseInput{UserInputID: decisionTargetID, TextAnswer: "yes"}, nil); err != nil {
		t.Fatalf("RespondUserInput() error = %v", err)
	}
	if len(resolver.respondInput) != 1 || resolver.respondInput[0].SessionID != decisionSessionID {
		t.Fatalf("local response inputs = %#v", resolver.respondInput)
	}
	if resolver.respondFence.Valid() || resolver.allocated != 0 || manager.starts != 1 {
		t.Fatalf("memory continuation lifecycle = fence:%#v allocated:%d starts:%d", resolver.respondFence, resolver.allocated, manager.starts)
	}
	if len(manager.admissions) != 1 || manager.admissions[0].ResolvedDecision == nil || manager.admissions[0].ResolvedDecision.Status != "submitted" {
		t.Fatalf("memory continuation admission = %#v", manager.admissions)
	}
	if got := strings.Join(resolver.lifecycle, ","); got != "commit,continue" {
		t.Fatalf("memory decision lifecycle = %q, want commit,continue", got)
	}
}

func TestRouterCommitFailureDoesNotPublishOrContinue(t *testing.T) {
	wantErr := errors.New("durable decision commit failed")
	resolver := &routerTestResolver{
		prepared: runtimefence.PreservedDecision{
			Kind: runtimefence.DecisionToolApproval, ID: decisionTargetID, BotID: decisionBotID, SessionID: decisionSessionID,
		},
		commitErr: wantErr,
	}
	manager := &routerTestManager{}
	router := newRouter(slog.New(slog.DiscardHandler), manager, resolver)

	err := router.RespondToolApproval(context.Background(), flow.ToolApprovalResponseInput{
		ApprovalID: decisionTargetID,
		Decision:   "approve",
	}, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("RespondToolApproval() error = %v, want %v", err, wantErr)
	}
	if got := strings.Join(resolver.lifecycle, ","); got != "commit" {
		t.Fatalf("failed decision lifecycle = %q, want commit", got)
	}
	if len(manager.admissions) != 0 || len(manager.handledEvents) != 0 || len(manager.finalizedEvents) != 0 {
		t.Fatalf("failed decision was projected: admissions=%#v handled=%#v finalized=%#v", manager.admissions, manager.handledEvents, manager.finalizedEvents)
	}
}

func TestRouterDistributedFallbackClaimsFenceAndProjectsTerminal(t *testing.T) {
	resolver := &routerTestResolver{
		prepared: runtimefence.PreservedDecision{
			Kind: runtimefence.DecisionToolApproval, ID: decisionTargetID, BotID: decisionBotID, SessionID: decisionSessionID,
		},
		respondEvents: []agentpkg.StreamEvent{
			{Type: agentpkg.EventAgentStart},
			{Type: agentpkg.EventTextDelta, Delta: "continued"},
			{Type: agentpkg.EventAgentEnd, HistoryCommitted: true},
		},
	}
	manager := &routerTestManager{distributed: true}
	router := newRouter(slog.New(slog.DiscardHandler), manager, resolver)
	output := make(chan flow.WSStreamEvent, len(resolver.respondEvents))

	if err := router.RespondToolApproval(context.Background(), flow.ToolApprovalResponseInput{ApprovalID: decisionTargetID, Decision: "approve"}, output); err != nil {
		t.Fatalf("RespondToolApproval() error = %v", err)
	}
	if manager.starts != 1 || resolver.allocated != 1 || len(resolver.activated) != 1 {
		t.Fatalf("distributed admission = starts:%d allocated:%d activated:%d", manager.starts, resolver.allocated, len(resolver.activated))
	}
	if preserved := resolver.activated[0].PreserveDecision; preserved == nil || *preserved != resolver.prepared {
		t.Fatalf("preserved decision = %#v", preserved)
	}
	if resolver.respondFence.Token != 7 {
		t.Fatalf("continuation fence = %#v", resolver.respondFence)
	}
	if len(manager.handledEvents) != 2 || len(manager.finalizedEvents) != 1 || !manager.canonicalReady[0] {
		t.Fatalf("runtime projection = handled:%#v finalized:%#v canonical:%#v", manager.handledEvents, manager.finalizedEvents, manager.canonicalReady)
	}
	if manager.finishes != 1 || len(output) != len(resolver.respondEvents) {
		t.Fatalf("completion = finishes:%d output:%d", manager.finishes, len(output))
	}
}

func TestRouterLeavesPendingTerminalForManagerRetry(t *testing.T) {
	resolver := &routerTestResolver{
		prepared: runtimefence.PreservedDecision{
			Kind: runtimefence.DecisionUserInput, ID: decisionTargetID, BotID: decisionBotID, SessionID: decisionSessionID,
		},
		respondEvents: []agentpkg.StreamEvent{{Type: agentpkg.EventAgentEnd, HistoryCommitted: true}},
	}
	manager := &routerTestManager{distributed: true, finalizeErr: sessionruntime.ErrTerminalCommitPending}
	router := newRouter(slog.New(slog.DiscardHandler), manager, resolver)

	err := router.RespondUserInput(context.Background(), flow.UserInputResponseInput{UserInputID: decisionTargetID, TextAnswer: "yes"}, nil)
	if !errors.Is(err, sessionruntime.ErrTerminalCommitPending) {
		t.Fatalf("terminal error = %v, want ErrTerminalCommitPending", err)
	}
	if manager.finishes != 0 {
		t.Fatalf("pending terminal was overwritten by %d FinishRun call(s)", manager.finishes)
	}
}

func TestRouterPropagatesActiveCommandResult(t *testing.T) {
	wantErr := errors.New("decision conflict")
	resolver := &routerTestResolver{prepared: runtimefence.PreservedDecision{
		Kind: runtimefence.DecisionToolApproval, ID: decisionTargetID, BotID: decisionBotID, SessionID: decisionSessionID,
	}}
	manager := &routerTestManager{dispatchHandled: true, dispatchErr: wantErr}
	router := newRouter(slog.New(slog.DiscardHandler), manager, resolver)

	err := router.RespondToolApproval(context.Background(), flow.ToolApprovalResponseInput{ApprovalID: decisionTargetID, Decision: "approve"}, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("RespondToolApproval() error = %v, want %v", err, wantErr)
	}
}

func TestRouterReconcilesCommittedDuplicateBeforeStartingContinuation(t *testing.T) {
	resolver := &routerTestResolver{
		prepared: runtimefence.PreservedDecision{
			Kind: runtimefence.DecisionToolApproval, ID: decisionTargetID, BotID: decisionBotID, SessionID: decisionSessionID,
		},
		reconcileHandled: true,
	}
	manager := &routerTestManager{distributed: true}
	router := newRouter(slog.New(slog.DiscardHandler), manager, resolver)

	if err := router.RespondToolApproval(context.Background(), flow.ToolApprovalResponseInput{ApprovalID: decisionTargetID, Decision: "approve"}, nil); err != nil {
		t.Fatalf("duplicate response error = %v", err)
	}
	if resolver.reconcileCalls != 1 || resolver.allocated != 0 || manager.starts != 0 || len(resolver.respondApproval) != 0 {
		t.Fatalf("duplicate routing = reconcile:%d allocated:%d starts:%d responses:%d", resolver.reconcileCalls, resolver.allocated, manager.starts, len(resolver.respondApproval))
	}
}

func TestRouterReturnsCommittedPayloadConflict(t *testing.T) {
	wantErr := errors.New("payload conflict")
	resolver := &routerTestResolver{
		prepared: runtimefence.PreservedDecision{
			Kind: runtimefence.DecisionUserInput, ID: decisionTargetID, BotID: decisionBotID, SessionID: decisionSessionID,
		},
		reconcileHandled: true,
		reconcileErr:     wantErr,
	}
	router := newRouter(slog.New(slog.DiscardHandler), &routerTestManager{distributed: true}, resolver)

	err := router.RespondUserInput(context.Background(), flow.UserInputResponseInput{UserInputID: decisionTargetID, TextAnswer: "different"}, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("conflicting duplicate error = %v, want %v", err, wantErr)
	}
}

func TestRouterRoutesDecisionAcrossRedisOwnersOptional(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("MEMOH_TEST_REDIS_URL"))
	if redisURL == "" {
		redisURL = strings.TrimSpace(os.Getenv("MEMOH_TEST_VALKEY_URL"))
	}
	if redisURL == "" {
		t.Skip("set MEMOH_TEST_REDIS_URL or MEMOH_TEST_VALKEY_URL to run cross-owner decision routing")
	}
	prefix := fmt.Sprintf("memoh:test:decision_runtime:%d:", time.Now().UnixNano())
	newBackend := func() *sessionruntime.RedisBackend {
		backend, err := sessionruntime.NewRedisBackend(context.Background(), sessionruntime.RedisOptions{
			URL: redisURL, KeyPrefix: prefix, StateTTL: time.Minute,
		})
		if err != nil {
			t.Fatalf("new Redis backend: %v", err)
		}
		return backend
	}
	newManager := func(ownerID string) *sessionruntime.Manager {
		manager := sessionruntime.NewManager(newBackend(), sessionruntime.Options{
			OwnerID: ownerID, StateTTL: time.Minute, OwnerLeaseTTL: 5 * time.Second, CommandAckTTL: 2 * time.Second,
		})
		if err := manager.Start(context.Background()); err != nil {
			t.Fatalf("start manager %s: %v", ownerID, err)
		}
		return manager
	}
	owner := newManager("decision-owner")
	remote := newManager("decision-remote")
	t.Cleanup(func() {
		if err := remote.Close(); err != nil {
			t.Errorf("close remote manager: %v", err)
		}
		if err := owner.Close(); err != nil {
			t.Errorf("close owner manager: %v", err)
		}
	})
	prepared := runtimefence.PreservedDecision{
		Kind: runtimefence.DecisionToolApproval, ID: decisionTargetID, BotID: decisionBotID, SessionID: decisionSessionID,
	}
	ownerResolver := &routerTestResolver{prepared: prepared}
	remoteResolver := &routerTestResolver{prepared: prepared}
	newRouter(slog.New(slog.DiscardHandler), owner, ownerResolver)
	remoteRouter := newRouter(slog.New(slog.DiscardHandler), remote, remoteResolver)

	handle, err := owner.StartRunWithOptions(context.Background(), sessionruntime.RunStartOptions{
		BotID: decisionBotID, SessionID: decisionSessionID, StreamID: "active-decision-run",
	})
	if err != nil {
		t.Fatalf("start active decision run: %v", err)
	}
	if _, err := owner.HandleAgentEvent(context.Background(), handle, agentpkg.StreamEvent{
		Type: agentpkg.EventToolApprovalRequest, ApprovalID: decisionTargetID, ToolCallID: "call-1", ToolName: "exec", Status: "pending",
	}); err != nil {
		t.Fatalf("project active decision: %v", err)
	}

	if err := remoteRouter.RespondToolApproval(context.Background(), flow.ToolApprovalResponseInput{
		BotID: decisionBotID, ApprovalID: decisionTargetID, Decision: "approve",
	}, nil); err != nil {
		t.Fatalf("route remote approval: %v", err)
	}
	if len(ownerResolver.respondApproval) != 1 || !ownerResolver.respondApproval[0].ResolveOnly {
		t.Fatalf("owner responses = %#v", ownerResolver.respondApproval)
	}
	if len(remoteResolver.respondApproval) != 0 {
		t.Fatalf("remote server executed owner response: %#v", remoteResolver.respondApproval)
	}
}

func TestRouterRunsFencedContinuationWithRedisOptional(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("MEMOH_TEST_REDIS_URL"))
	if redisURL == "" {
		redisURL = strings.TrimSpace(os.Getenv("MEMOH_TEST_VALKEY_URL"))
	}
	if redisURL == "" {
		t.Skip("set MEMOH_TEST_REDIS_URL or MEMOH_TEST_VALKEY_URL to run fenced decision continuation")
	}
	backend, err := sessionruntime.NewRedisBackend(context.Background(), sessionruntime.RedisOptions{
		URL: redisURL, KeyPrefix: fmt.Sprintf("memoh:test:decision_continuation:%d:", time.Now().UnixNano()), StateTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("new Redis backend: %v", err)
	}
	manager := sessionruntime.NewManager(backend, sessionruntime.Options{
		OwnerID: "decision-continuation-owner", StateTTL: time.Minute, OwnerLeaseTTL: 5 * time.Second, CommandAckTTL: 2 * time.Second,
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Errorf("close manager: %v", err)
		}
	})
	resolver := &routerTestResolver{
		prepared: runtimefence.PreservedDecision{
			Kind: runtimefence.DecisionUserInput, ID: decisionTargetID, BotID: decisionBotID, SessionID: decisionSessionID,
		},
		respondEvents: []agentpkg.StreamEvent{
			{Type: agentpkg.EventAgentStart},
			{Type: agentpkg.EventTextDelta, Delta: "continued"},
			{Type: agentpkg.EventAgentEnd, HistoryCommitted: true},
		},
	}
	router := newRouter(slog.New(slog.DiscardHandler), manager, resolver)

	if err := router.RespondUserInput(context.Background(), flow.UserInputResponseInput{
		BotID: decisionBotID, UserInputID: decisionTargetID, TextAnswer: "continue",
	}, nil); err != nil {
		t.Fatalf("run fenced continuation: %v", err)
	}
	if resolver.allocated != 1 || len(resolver.activated) != 1 || resolver.respondFence.Token != 7 {
		t.Fatalf("fenced continuation = allocated:%d activated:%d fence:%#v", resolver.allocated, len(resolver.activated), resolver.respondFence)
	}
	snapshot, err := manager.Snapshot(context.Background(), decisionBotID, decisionSessionID)
	if err != nil {
		t.Fatalf("load continuation snapshot: %v", err)
	}
	if snapshot.CurrentRunView == nil || snapshot.CurrentRunView.Status != sessionruntime.RunStatusCompleted || !snapshot.CurrentRunView.HistoryCommitted || !snapshot.CurrentRunView.CanonicalReady {
		t.Fatalf("continuation snapshot = %#v", snapshot.CurrentRunView)
	}
}
