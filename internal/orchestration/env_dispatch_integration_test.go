package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeEnvManager records every call the dispatch path makes so the assertions
// can pin both the order of invocation and the lease-fencing arguments. The
// fake intentionally does not lean on the real orchestrationenv package; the
// kernel only sees the EnvManager interface declared in types.go and the test
// substitutes a hand-rolled implementation behind that contract.
type fakeEnvManager struct {
	mu                sync.Mutex
	resource          EnvResourceRef
	getResourceCalls  []string
	acquireCalls      []EnvAcquireRequest
	createBindingArgs []EnvCreateBindingRequest
	releaseBinding    []EnvReleaseBindingRequest
	holdBinding       []EnvHoldBindingRequest
	releaseSession    []EnvReleaseSessionRequest
	nextSessionID     string
	nextLeaseToken    string
	nextLeaseEpoch    int64
	nextBindingID     string
}

func (f *fakeEnvManager) GetEnvResourceByName(_ context.Context, _, name string) (EnvResourceRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getResourceCalls = append(f.getResourceCalls, name)
	if name != f.resource.Name {
		return EnvResourceRef{}, errors.New("fake env: resource not found: " + name)
	}
	return f.resource, nil
}

func (f *fakeEnvManager) AcquireEnvSession(_ context.Context, req EnvAcquireRequest) (EnvSessionLease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acquireCalls = append(f.acquireCalls, req)
	return EnvSessionLease{
		SessionID:     f.nextSessionID,
		ResourceID:    f.resource.ID,
		ResourceKind:  f.resource.Kind,
		ResourceName:  f.resource.Name,
		LeaseToken:    f.nextLeaseToken,
		LeaseEpoch:    f.nextLeaseEpoch,
		RuntimeHandle: map[string]any{"socket": "/run/memoh/" + f.nextSessionID + ".sock"},
	}, nil
}

func (f *fakeEnvManager) CreateEnvBinding(_ context.Context, req EnvCreateBindingRequest) (EnvBindingHandle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createBindingArgs = append(f.createBindingArgs, req)
	return EnvBindingHandle{BindingID: f.nextBindingID}, nil
}

func (f *fakeEnvManager) ReleaseEnvBinding(_ context.Context, req EnvReleaseBindingRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releaseBinding = append(f.releaseBinding, req)
	return nil
}

func (f *fakeEnvManager) HoldEnvBinding(_ context.Context, req EnvHoldBindingRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.holdBinding = append(f.holdBinding, req)
	return nil
}

func (f *fakeEnvManager) ReleaseEnvSession(_ context.Context, req EnvReleaseSessionRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releaseSession = append(f.releaseSession, req)
	return nil
}

func newFakeEnvManager(resourceName, resourceID, kind string) *fakeEnvManager {
	return &fakeEnvManager{
		resource: EnvResourceRef{
			ID:       resourceID,
			Kind:     kind,
			Name:     resourceName,
			Status:   "active",
			Capacity: 4,
		},
		nextSessionID:  "session-" + uuid.NewString(),
		nextLeaseToken: "lease-" + uuid.NewString(),
		nextLeaseEpoch: 1,
		nextBindingID:  "binding-" + uuid.NewString(),
	}
}

// TestIntegrationDispatchAcquiresEnvAndReleasesAfterCompletion drives a single
// child task that declares env_preconditions.required=true through the full
// scheduler/attempt lifecycle and asserts that the kernel:
//
//  1. resolves the resource_name on the env manager,
//  2. acquires a session whose fencing tuple is echoed into the input manifest,
//  3. creates a binding that points at the acquired attempt, and
//  4. releases both the binding and the session after the attempt completes.
func TestIntegrationDispatchAcquiresEnvAndReleasesAfterCompletion(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	envManager := newFakeEnvManager("shared-shell", "env-resource-"+uuid.NewString(), EnvPreconditionsKindContainer)
	svc.SetEnvManager(envManager)
	svc.SetEnvLeaseTTL(45 * time.Second)

	svc.SetStartRunPlanner(fixedStartRunPlanner{
		result: &StartRunPlanningResult{
			Summary: "single env-bound task",
			ChildTasks: []PlannedTaskSpec{
				{
					Alias:         "exec",
					Kind:          "step",
					Goal:          "run shell command in shared shell",
					WorkerProfile: DefaultRootWorkerProfile,
					EnvPreconditions: EnvPreconditions{
						Required:     true,
						Kind:         EnvPreconditionsKindContainer,
						ResourceName: "shared-shell",
						Mode:         "read_write",
						EffectClass:  EnvPreconditionsEffectInternal,
					},
				},
			},
		},
	})

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-env-" + uuid.NewString(),
		Subject:  "subject-env-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "run command via env-bound task",
		BotID:          "bot-" + uuid.NewString(),
		IdempotencyKey: "start-env-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := dispatchNextReadyTaskForTest(ctx, svc)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}

	if got := len(envManager.getResourceCalls); got != 1 {
		t.Fatalf("GetEnvResourceByName call count = %d, want 1", got)
	}
	if envManager.getResourceCalls[0] != "shared-shell" {
		t.Fatalf("GetEnvResourceByName name = %q, want %q", envManager.getResourceCalls[0], "shared-shell")
	}
	if got := len(envManager.acquireCalls); got != 1 {
		t.Fatalf("AcquireEnvSession call count = %d, want 1", got)
	}
	acquired := envManager.acquireCalls[0]
	if acquired.LeaseHolderKind != EnvLeaseHolderWorker {
		t.Fatalf("acquire lease holder kind = %q, want %q", acquired.LeaseHolderKind, EnvLeaseHolderWorker)
	}
	if acquired.LeaseTTL != 45*time.Second {
		t.Fatalf("acquire lease TTL = %v, want 45s", acquired.LeaseTTL)
	}
	if got := len(envManager.createBindingArgs); got != 1 {
		t.Fatalf("CreateEnvBinding call count = %d, want 1", got)
	}
	binding := envManager.createBindingArgs[0]
	if binding.SessionID != envManager.nextSessionID {
		t.Fatalf("binding session_id = %q, want %q", binding.SessionID, envManager.nextSessionID)
	}
	if binding.LeaseToken != envManager.nextLeaseToken || binding.LeaseEpoch != envManager.nextLeaseEpoch {
		t.Fatalf("binding lease tuple = (%s,%d), want (%s,%d)", binding.LeaseToken, binding.LeaseEpoch, envManager.nextLeaseToken, envManager.nextLeaseEpoch)
	}
	if binding.Purpose != EnvBindingPurposePrimary {
		t.Fatalf("binding purpose = %q, want %q", binding.Purpose, EnvBindingPurposePrimary)
	}

	// The captured manifest must echo the acquired session so a verifier
	// replay or a delayed release can fence stale callers.
	captured := decodeManifestEnvPayload(t, loadInputManifestEnvPayload(t, ctx, svc, handle.RunID))
	if required, _ := captured["required"].(bool); !required {
		t.Fatalf("captured_env_preconditions.required = false, want true; payload=%v", captured)
	}
	if got := stringFrom(captured, "session_id"); got != envManager.nextSessionID {
		t.Fatalf("captured_env_preconditions.session_id = %q, want %q", got, envManager.nextSessionID)
	}
	if got := stringFrom(captured, "lease_token"); got != envManager.nextLeaseToken {
		t.Fatalf("captured_env_preconditions.lease_token = %q, want %q", got, envManager.nextLeaseToken)
	}
	if got := stringFrom(captured, "binding_id"); got != envManager.nextBindingID {
		t.Fatalf("captured_env_preconditions.binding_id = %q, want %q", got, envManager.nextBindingID)
	}

	// Drive the attempt to a successful terminal state and verify the
	// env manager observes a release for both the binding and the session.
	attempt, err := svc.ClaimNextAttempt(ctx, AttemptClaim{
		WorkerID:        "worker-" + uuid.NewString(),
		ExecutorID:      DefaultWorkerExecutorID,
		WorkerProfiles:  []string{DefaultRootWorkerProfile},
		LeaseTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("ClaimNextAttempt() error = %v", err)
	}
	if _, err := svc.StartAttempt(ctx, attempt.ID, attempt.ClaimToken); err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := svc.CompleteAttempt(ctx, AttemptCompletion{
		AttemptID:        attempt.ID,
		ClaimToken:       attempt.ClaimToken,
		Status:           TaskAttemptStatusCompleted,
		Summary:          "shell command produced 0",
		StructuredOutput: map[string]any{"exit": 0},
	}); err != nil {
		t.Fatalf("CompleteAttempt() error = %v", err)
	}

	if got := len(envManager.releaseBinding); got != 1 {
		t.Fatalf("ReleaseEnvBinding call count = %d, want 1", got)
	}
	if got := envManager.releaseBinding[0].BindingID; got != envManager.nextBindingID {
		t.Fatalf("release binding id = %q, want %q", got, envManager.nextBindingID)
	}
	if reason := envManager.releaseBinding[0].Reason; !strings.HasPrefix(reason, "attempt_") {
		t.Fatalf("release binding reason = %q, want attempt_*", reason)
	}
	if got := len(envManager.releaseSession); got != 1 {
		t.Fatalf("ReleaseEnvSession call count = %d, want 1", got)
	}
	if got := envManager.releaseSession[0].SessionID; got != envManager.nextSessionID {
		t.Fatalf("release session id = %q, want %q", got, envManager.nextSessionID)
	}
}

// TestIntegrationDispatchSkipsEnvManagerWhenPreconditionsNotRequired pins the
// fast path: pure-LLM tasks never call into the env manager and the manifest
// retains the column default sentinel rather than a synthesized envelope.
func TestIntegrationDispatchSkipsEnvManagerWhenPreconditionsNotRequired(t *testing.T) {
	svc, pool, cleanup := setupOrchestrationIntegrationTest(t)
	defer cleanup()

	envManager := newFakeEnvManager("unused", "env-resource-unused", EnvPreconditionsKindContainer)
	svc.SetEnvManager(envManager)

	ctx := context.Background()
	caller := ControlIdentity{
		TenantID: "tenant-env-skip-" + uuid.NewString(),
		Subject:  "subject-env-skip-" + uuid.NewString(),
	}

	handle, err := svc.StartRun(ctx, caller, StartRunRequest{
		Goal:           "no env required",
		IdempotencyKey: "start-env-skip-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}
	processRunPlanningIntent(t, ctx, svc)
	defer cleanupOrchestrationIntegrationRun(t, ctx, pool, handle.RunID)
	defer cleanupOrchestrationIntegrationIdempotency(t, ctx, pool, caller.TenantID, caller.Subject)

	dispatched, err := dispatchNextReadyTaskForTest(ctx, svc)
	if err != nil {
		t.Fatalf("DispatchNextReadyTask() error = %v", err)
	}
	if !dispatched {
		t.Fatal("DispatchNextReadyTask() = false, want true")
	}

	if got := len(envManager.getResourceCalls); got != 0 {
		t.Fatalf("GetEnvResourceByName call count = %d, want 0", got)
	}
	if got := len(envManager.acquireCalls); got != 0 {
		t.Fatalf("AcquireEnvSession call count = %d, want 0", got)
	}
	if got := len(envManager.createBindingArgs); got != 0 {
		t.Fatalf("CreateEnvBinding call count = %d, want 0", got)
	}

	captured := decodeManifestEnvPayload(t, loadInputManifestEnvPayload(t, ctx, svc, handle.RunID))
	if required, _ := captured["required"].(bool); required {
		t.Fatalf("captured_env_preconditions.required = true, want false; payload=%v", captured)
	}
}

func decodeManifestEnvPayload(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	if len(raw) == 0 {
		return map[string]any{}
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal captured_env_preconditions: %v", err)
	}
	return parsed
}

// loadInputManifestEnvPayload looks up the run's single attempt and returns
// the captured_env_preconditions JSONB column on its input manifest. The
// helper assumes the test produced exactly one attempt; assertions on count
// stay in the caller so the failure message points at the right line.
func loadInputManifestEnvPayload(t *testing.T, ctx context.Context, svc *Service, runID string) []byte {
	t.Helper()
	pgRunID := mustParsePGUUID(t, runID)
	attempts, err := svc.queries.ListCurrentOrchestrationTaskAttemptsByRun(ctx, pgRunID)
	if err != nil {
		t.Fatalf("ListCurrentOrchestrationTaskAttemptsByRun() error = %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempt count = %d, want 1", len(attempts))
	}
	manifest, err := svc.queries.GetOrchestrationInputManifestByID(ctx, attempts[0].InputManifestID)
	if err != nil {
		t.Fatalf("GetOrchestrationInputManifestByID() error = %v", err)
	}
	return manifest.CapturedEnvPreconditions
}
