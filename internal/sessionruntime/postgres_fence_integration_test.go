package sessionruntime_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation/flow"
	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/dbtest"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	dbstore "github.com/memohai/memoh/internal/db/store"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/runtimefence"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/sessionruntime"
	"github.com/memohai/memoh/internal/toolapproval"
	"github.com/memohai/memoh/internal/userinput"
)

const (
	crossBackendCrashChildEnv   = "MEMOH_TEST_CROSS_BACKEND_CRASH_CHILD"
	crossBackendCrashRedisEnv   = "MEMOH_TEST_CROSS_BACKEND_CRASH_REDIS_URL"
	crossBackendCrashPrefixEnv  = "MEMOH_TEST_CROSS_BACKEND_CRASH_PREFIX"
	crossBackendCrashBotEnv     = "MEMOH_TEST_CROSS_BACKEND_CRASH_BOT_ID"
	crossBackendCrashSessionEnv = "MEMOH_TEST_CROSS_BACKEND_CRASH_SESSION_ID"
	crossBackendCrashStreamID   = "cross-backend-crash-stream"
	crossBackendCrashOwnerID    = "cross-backend-crash-owner"
	crossBackendCrashCommandTTL = 750 * time.Millisecond
	crossBackendCrashOwnerLease = 600 * time.Millisecond
)

type crossBackendCrashEvent struct {
	Type       string                   `json:"type"`
	InputID    string                   `json:"input_id,omitempty"`
	Generation string                   `json:"generation,omitempty"`
	Answer     userinput.QuestionAnswer `json:"answer,omitempty"`
}

var (
	crossBackendMigrationOnce sync.Once
	crossBackendMigrationErr  error
)

func TestPostgresValkeyMultiprocessCommandReconciliationChild(t *testing.T) {
	if os.Getenv(crossBackendCrashChildEnv) != "1" {
		t.Skip("cross-backend crash child helper")
	}
	ctx := context.Background()
	redisURL := strings.TrimSpace(os.Getenv(crossBackendCrashRedisEnv))
	prefix := strings.TrimSpace(os.Getenv(crossBackendCrashPrefixEnv))
	botID := strings.TrimSpace(os.Getenv(crossBackendCrashBotEnv))
	sessionID := strings.TrimSpace(os.Getenv(crossBackendCrashSessionEnv))
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if redisURL == "" || prefix == "" || botID == "" || sessionID == "" || dsn == "" {
		t.Fatal("cross-backend crash child configuration is incomplete")
	}

	pool, err := dbpkg.OpenPostgresDSN(ctx, dsn)
	if err != nil {
		t.Fatalf("create child postgres pool: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping child postgres: %v", err)
	}
	store := postgresstore.NewQueriesWithPool(pool, dbsqlc.New(pool))
	inputs := userinput.NewService(nil, store)
	resolver := &flow.Resolver{}
	resolver.SetUserInputService(inputs)

	backend, err := sessionruntime.NewRedisBackend(ctx, sessionruntime.RedisOptions{
		URL: redisURL, KeyPrefix: prefix, StateTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("create child runtime backend: %v", err)
	}
	manager := sessionruntime.NewManager(backend, sessionruntime.Options{
		OwnerID: crossBackendCrashOwnerID, StateTTL: time.Minute,
		OwnerLeaseTTL: crossBackendCrashOwnerLease, CommandAckTTL: crossBackendCrashCommandTTL,
	})
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("start child runtime manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	queries := dbsqlc.New(pool)
	token, err := queries.NextSessionRuntimeFenceToken(ctx)
	if err != nil {
		t.Fatalf("allocate child runtime fence: %v", err)
	}
	fence := runtimefence.Fence{BotID: botID, SessionID: sessionID, Token: token}
	runCtx := runtimefence.WithContext(ctx, fence)
	handle, err := manager.StartRunWithOptions(runCtx, sessionruntime.RunStartOptions{
		BotID: botID, SessionID: sessionID, StreamID: crossBackendCrashStreamID,
		AdmissionBuilder: func(admissionCtx context.Context, _ sessionruntime.RunHandle) (sessionruntime.RunAdmissionView, error) {
			return sessionruntime.RunAdmissionView{}, runtimefence.Activate(admissionCtx, store, fence)
		},
		AbortCh: make(chan struct{}, 1), Cancel: func() {},
	})
	if err != nil {
		t.Fatalf("start child runtime run: %v", err)
	}
	request := createCrossBackendPendingInput(t, runCtx, inputs, fence, "cross-backend-crash-input")
	answer := userInputAnswer(request)[0]
	if _, err := manager.HandleAgentEvent(runCtx, handle, agentpkg.StreamEvent{
		Type: agentpkg.EventUserInputRequest, ToolName: userinput.ToolNameAskUser,
		ToolCallID: request.ToolCallID, UserInputID: request.ID, Status: userinput.StatusPending,
	}); err != nil {
		t.Fatalf("publish child pending input: %v", err)
	}

	encoder := json.NewEncoder(os.Stdout)
	var outputMu sync.Mutex
	emit := func(event crossBackendCrashEvent) error {
		outputMu.Lock()
		defer outputMu.Unlock()
		return encoder.Encode(event)
	}
	manager.SetCommandHandler(func(commandCtx context.Context, command sessionruntime.Command) error {
		var input flow.UserInputResponseInput
		if err := json.Unmarshal(command.Payload, &input); err != nil {
			return err
		}
		input.BotID = command.BotID
		input.SessionID = command.SessionID
		input.UserInputID = command.TargetID
		input.ExplicitID = command.TargetID
		input.ResolveOnly = true
		if err := resolver.RespondUserInput(commandCtx, input, nil); err != nil {
			return err
		}
		if err := emit(crossBackendCrashEvent{Type: "committed", InputID: command.TargetID}); err != nil {
			return err
		}
		select {}
	})
	if err := emit(crossBackendCrashEvent{Type: "ready", InputID: request.ID, Generation: handle.Generation, Answer: answer}); err != nil {
		t.Fatalf("emit child readiness: %v", err)
	}
	select {}
}

func TestPostgresValkeyMultiprocessCrashReconcilesCommittedCommand(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("MEMOH_TEST_REDIS_URL"))
	if redisURL == "" {
		redisURL = strings.TrimSpace(os.Getenv("MEMOH_TEST_VALKEY_URL"))
	}
	if redisURL == "" {
		if os.Getenv("MEMOH_TEST_CROSS_BACKEND_REQUIRED") == "1" {
			t.Fatal("cross-backend crash reconciliation requires Redis or Valkey")
		}
		t.Skip("set MEMOH_TEST_REDIS_URL or MEMOH_TEST_VALKEY_URL to run cross-backend crash reconciliation")
	}
	ctx := context.Background()
	pool := openCrossBackendPostgresPool(t, ctx)
	botID, sessionID := createCrossBackendFixtures(t, ctx, pool)
	prefix := "memoh:test:runtime-crash:" + uuid.NewString() + ":"

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	childCtx, cancelChild := context.WithTimeout(ctx, 15*time.Second)
	defer cancelChild()
	cmd := exec.CommandContext(childCtx, executable, //nolint:gosec // Re-executes this test binary as an isolated owner process.
		"-test.run=^TestPostgresValkeyMultiprocessCommandReconciliationChild$", "-test.timeout=12s")
	cmd.Env = crossBackendCrashProcessEnv(
		crossBackendCrashChildEnv+"=1",
		crossBackendCrashRedisEnv+"="+redisURL,
		crossBackendCrashPrefixEnv+"="+prefix,
		crossBackendCrashBotEnv+"="+botID.String(),
		crossBackendCrashSessionEnv+"="+sessionID.String(),
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("open child stdout: %v", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start crash owner child: %v", err)
	}
	childStopped := false
	t.Cleanup(func() {
		if !childStopped && cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	decoder := json.NewDecoder(bufio.NewReader(stdout))
	var ready crossBackendCrashEvent
	if err := decoder.Decode(&ready); err != nil {
		t.Fatalf("read crash owner readiness: %v\nstderr:\n%s", err, stderr.String())
	}
	if ready.Type != "ready" || ready.InputID == "" || ready.Generation == "" || ready.Answer.QuestionID == "" {
		t.Fatalf("crash owner readiness = %#v", ready)
	}

	backend, err := sessionruntime.NewRedisBackend(ctx, sessionruntime.RedisOptions{
		URL: redisURL, KeyPrefix: prefix, StateTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("create observer runtime backend: %v", err)
	}
	observer := sessionruntime.NewManager(backend, sessionruntime.Options{
		OwnerID: "cross-backend-crash-observer", StateTTL: time.Minute,
		OwnerLeaseTTL: crossBackendCrashOwnerLease, CommandAckTTL: crossBackendCrashCommandTTL,
	})
	if err := observer.Start(ctx); err != nil {
		t.Fatalf("start observer runtime manager: %v", err)
	}
	t.Cleanup(func() { _ = observer.Close() })
	inputs := userinput.NewService(nil, postgresstore.NewQueriesWithPool(pool, dbsqlc.New(pool)))
	reconciler := &flow.Resolver{}
	reconciler.SetUserInputService(inputs)
	observer.SetCommandReconciler(func(reconcileCtx context.Context, command sessionruntime.Command) (bool, error) {
		var input flow.UserInputResponseInput
		if err := json.Unmarshal(command.Payload, &input); err != nil {
			return true, err
		}
		input.BotID = command.BotID
		input.SessionID = command.SessionID
		input.UserInputID = command.TargetID
		input.ExplicitID = command.TargetID
		input.ResolveOnly = true
		return reconciler.ReconcileUserInputResponse(reconcileCtx, input)
	})
	payload, err := json.Marshal(flow.UserInputResponseInput{Answers: []userinput.QuestionAnswer{ready.Answer}})
	if err != nil {
		t.Fatalf("marshal crash command payload: %v", err)
	}
	type dispatchResult struct {
		handled bool
		err     error
	}
	dispatched := make(chan dispatchResult, 1)
	go func() {
		handled, dispatchErr := observer.DispatchActiveCommand(ctx, botID.String(), sessionID.String(), sessionruntime.CommandUserInputResponse, ready.InputID, payload)
		dispatched <- dispatchResult{handled: handled, err: dispatchErr}
	}()
	var committed crossBackendCrashEvent
	if err := decoder.Decode(&committed); err != nil {
		t.Fatalf("read committed child event: %v\nstderr:\n%s", err, stderr.String())
	}
	if committed.Type != "committed" || committed.InputID != ready.InputID {
		t.Fatalf("committed child event = %#v", committed)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("crash runtime owner child: %v", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("runtime owner child exited cleanly after forced crash")
	}
	childStopped = true

	select {
	case result := <-dispatched:
		if !result.handled || result.err != nil {
			t.Fatalf("dispatch after owner crash = handled:%v err:%v", result.handled, result.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("dispatch did not reconcile the PostgreSQL decision after owner crash")
	}
	resolved, err := inputs.Get(ctx, ready.InputID)
	if err != nil {
		t.Fatalf("load reconciled user input: %v", err)
	}
	if resolved.Status != userinput.StatusSubmitted {
		t.Fatalf("reconciled user input status = %q, want %q", resolved.Status, userinput.StatusSubmitted)
	}
	handled, err := observer.DispatchActiveCommand(ctx, botID.String(), sessionID.String(), sessionruntime.CommandUserInputResponse, ready.InputID, payload)
	if !handled || err != nil {
		t.Fatalf("idempotent command replay = handled:%v err:%v", handled, err)
	}
}

func crossBackendCrashProcessEnv(overrides ...string) []string {
	replaced := make(map[string]struct{}, len(overrides))
	for _, override := range overrides {
		key, _, _ := strings.Cut(override, "=")
		replaced[key] = struct{}{}
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, ok := replaced[key]; !ok {
			environment = append(environment, entry)
		}
	}
	return append(environment, overrides...)
}

func TestPostgresRuntimeFenceAcrossValkeyOwners(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("MEMOH_TEST_REDIS_URL"))
	if redisURL == "" {
		redisURL = strings.TrimSpace(os.Getenv("MEMOH_TEST_VALKEY_URL"))
	}
	if redisURL == "" {
		if os.Getenv("MEMOH_TEST_CROSS_BACKEND_REQUIRED") == "1" {
			t.Fatal("cross-backend runtime fencing is required, but Redis or Valkey is not configured")
		}
		t.Skip("set MEMOH_TEST_REDIS_URL or MEMOH_TEST_VALKEY_URL to run cross-backend fencing")
	}

	ctx := context.Background()
	pool := openCrossBackendPostgresPool(t, ctx)
	botID, sessionID := createCrossBackendFixtures(t, ctx, pool)
	queries := dbsqlc.New(pool)
	store := postgresstore.NewQueriesWithPool(pool, queries)
	service := messagepkg.NewService(nil, store)
	prefix := "memoh:test:runtime-fence:" + uuid.NewString() + ":"

	newManager := func(ownerID string) *sessionruntime.Manager {
		backend, err := sessionruntime.NewRedisBackend(ctx, sessionruntime.RedisOptions{
			URL:       redisURL,
			KeyPrefix: prefix,
			StateTTL:  time.Minute,
		})
		if err != nil {
			t.Fatalf("create %s runtime backend: %v", ownerID, err)
		}
		manager := sessionruntime.NewManager(backend, sessionruntime.Options{
			OwnerID:       ownerID,
			StateTTL:      time.Minute,
			OwnerLeaseTTL: 150 * time.Millisecond,
			Logger:        slog.Default(),
		})
		if err := manager.Start(ctx); err != nil {
			t.Fatalf("start %s runtime manager: %v", ownerID, err)
		}
		return manager
	}

	startOwner := func(manager *sessionruntime.Manager, streamID string, fence runtimefence.Fence) {
		runCtx := runtimefence.WithContext(ctx, fence)
		_, err := manager.StartRunWithOptions(runCtx, sessionruntime.RunStartOptions{
			BotID: fence.BotID, SessionID: fence.SessionID, StreamID: streamID,
			AdmissionBuilder: func(admissionCtx context.Context, handle sessionruntime.RunHandle) (sessionruntime.RunAdmissionView, error) {
				if err := manager.ValidateRunOwnership(admissionCtx, handle); err != nil {
					return sessionruntime.RunAdmissionView{}, err
				}
				if err := runtimefence.Activate(admissionCtx, store, fence); err != nil {
					return sessionruntime.RunAdmissionView{}, err
				}
				if err := manager.ValidateRunOwnership(admissionCtx, handle); err != nil {
					return sessionruntime.RunAdmissionView{}, err
				}
				return sessionruntime.RunAdmissionView{}, nil
			},
			AbortCh: make(chan struct{}, 1), Cancel: func() {},
		})
		if err != nil {
			t.Fatalf("start owner %s: %v", streamID, err)
		}
	}

	managerA := newManager("postgres-fence-owner-a")
	tokenA, err := queries.NextSessionRuntimeFenceToken(ctx)
	if err != nil {
		t.Fatalf("allocate owner A token: %v", err)
	}
	fenceA := runtimefence.Fence{BotID: botID.String(), SessionID: sessionID.String(), Token: tokenA}
	startOwner(managerA, "stream-owner-a", fenceA)
	if _, err := service.Persist(runtimefence.WithContext(ctx, fenceA), messagepkg.PersistInput{
		BotID: botID.String(), SessionID: sessionID.String(), Role: "assistant",
		Content: []byte(`{"role":"assistant","content":"owner a"}`), SkipHistoryTurn: true,
	}); err != nil {
		t.Fatalf("persist owner A message: %v", err)
	}
	if err := managerA.Close(); err != nil {
		t.Fatalf("close owner A: %v", err)
	}

	time.Sleep(350 * time.Millisecond)
	managerB := newManager("postgres-fence-owner-b")
	t.Cleanup(func() { _ = managerB.Close() })
	tokenB, err := queries.NextSessionRuntimeFenceToken(ctx)
	if err != nil {
		t.Fatalf("allocate owner B token: %v", err)
	}
	fenceB := runtimefence.Fence{BotID: botID.String(), SessionID: sessionID.String(), Token: tokenB}
	startOwner(managerB, "stream-owner-b", fenceB)

	if _, err := service.Persist(runtimefence.WithContext(ctx, fenceA), messagepkg.PersistInput{
		BotID: botID.String(), SessionID: sessionID.String(), Role: "assistant",
		Content: []byte(`{"role":"assistant","content":"stale owner a"}`), SkipHistoryTurn: true,
	}); !errors.Is(err, runtimefence.ErrStale) {
		t.Fatalf("stale owner A persistence error = %v, want ErrStale", err)
	}
	if _, err := service.Persist(runtimefence.WithContext(ctx, fenceB), messagepkg.PersistInput{
		BotID: botID.String(), SessionID: sessionID.String(), Role: "assistant",
		Content: []byte(`{"role":"assistant","content":"owner b"}`), SkipHistoryTurn: true,
	}); err != nil {
		t.Fatalf("persist owner B message: %v", err)
	}
}

func TestPostgresRuntimeFenceProtectsPendingDecisionsFromStaleCleanup(t *testing.T) {
	ctx := context.Background()
	pool := openCrossBackendPostgresPool(t, ctx)
	botID, sessionID := createCrossBackendFixtures(t, ctx, pool)
	queries := dbsqlc.New(pool)
	store := postgresstore.NewQueriesWithPool(pool, queries)
	approvals := toolapproval.NewService(nil, store, nil)
	inputs := userinput.NewService(nil, store)

	tokenA, err := queries.NextSessionRuntimeFenceToken(ctx)
	if err != nil {
		t.Fatalf("allocate owner A token: %v", err)
	}
	fenceA := runtimefence.Fence{BotID: botID.String(), SessionID: sessionID.String(), Token: tokenA}
	if err := runtimefence.Activate(ctx, store, fenceA); err != nil {
		t.Fatalf("activate owner A token: %v", err)
	}
	ownerA := runtimefence.WithContext(ctx, fenceA)
	oldApproval, err := approvals.CreatePending(ownerA, toolapproval.CreatePendingInput{
		BotID: fenceA.BotID, SessionID: fenceA.SessionID, ToolCallID: "approval-owner-a", ToolName: "write",
		ToolInput: map[string]any{"path": "owner-a.txt"},
	})
	if err != nil {
		t.Fatalf("create owner A approval: %v", err)
	}
	oldInput, err := inputs.CreatePending(ownerA, userinput.CreatePendingInput{
		BotID: fenceA.BotID, SessionID: fenceA.SessionID, ToolCallID: "input-owner-a", ToolName: userinput.ToolNameAskUser,
		Input: map[string]any{"questions": []any{map[string]any{
			"text": "Old question", "kind": userinput.QuestionKindSingleSelect,
			"options": []any{map[string]any{"label": "A"}, map[string]any{"label": "B"}},
		}}},
	})
	if err != nil {
		t.Fatalf("create owner A user input: %v", err)
	}

	tokenB, err := queries.NextSessionRuntimeFenceToken(ctx)
	if err != nil {
		t.Fatalf("allocate owner B token: %v", err)
	}
	fenceB := runtimefence.Fence{BotID: botID.String(), SessionID: sessionID.String(), Token: tokenB}
	if err := runtimefence.Activate(ctx, store, fenceB); err != nil {
		t.Fatalf("activate owner B token: %v", err)
	}
	gotOldApproval, err := approvals.Get(ctx, oldApproval.ID)
	if err != nil || gotOldApproval.Status != toolapproval.StatusCancelled {
		t.Fatalf("owner A approval after takeover = (%#v, %v)", gotOldApproval, err)
	}
	gotOldInput, err := inputs.Get(ctx, oldInput.ID)
	if err != nil || gotOldInput.Status != userinput.StatusCanceled {
		t.Fatalf("owner A user input after takeover = (%#v, %v)", gotOldInput, err)
	}

	ownerB := runtimefence.WithContext(ctx, fenceB)

	approval, err := approvals.CreatePending(ownerB, toolapproval.CreatePendingInput{
		BotID: fenceB.BotID, SessionID: fenceB.SessionID, ToolCallID: "approval-owner-b", ToolName: "write",
		ToolInput: map[string]any{"path": "owner-b.txt"},
	})
	if err != nil {
		t.Fatalf("create owner B approval: %v", err)
	}
	input, err := inputs.CreatePending(ownerB, userinput.CreatePendingInput{
		BotID: fenceB.BotID, SessionID: fenceB.SessionID, ToolCallID: "input-owner-b", ToolName: userinput.ToolNameAskUser,
		Input: map[string]any{"questions": []any{map[string]any{
			"text": "Choose", "kind": userinput.QuestionKindSingleSelect,
			"options": []any{map[string]any{"label": "A"}, map[string]any{"label": "B"}},
		}}},
	})
	if err != nil {
		t.Fatalf("create owner B user input: %v", err)
	}
	if err := runtimefence.Activate(ctx, store, fenceB); err != nil {
		t.Fatalf("reactivate owner B token: %v", err)
	}

	if _, err := approvals.CreatePending(ownerA, toolapproval.CreatePendingInput{
		BotID: fenceA.BotID, SessionID: fenceA.SessionID, ToolCallID: "approval-owner-a-stale", ToolName: "write",
		ToolInput: map[string]any{"path": "stale-owner-a.txt"},
	}); !errors.Is(err, runtimefence.ErrStale) {
		t.Fatalf("stale approval creation error = %v, want ErrStale", err)
	}
	if _, err := inputs.CreatePending(ownerA, userinput.CreatePendingInput{
		BotID: fenceA.BotID, SessionID: fenceA.SessionID, ToolCallID: "input-owner-a-stale", ToolName: userinput.ToolNameAskUser,
		Input: map[string]any{"questions": []any{map[string]any{
			"text": "Stale question", "kind": userinput.QuestionKindSingleSelect,
			"options": []any{map[string]any{"label": "A"}, map[string]any{"label": "B"}},
		}}},
	}); !errors.Is(err, runtimefence.ErrStale) {
		t.Fatalf("stale user input creation error = %v, want ErrStale", err)
	}
	if _, err := approvals.CancelPendingForSession(ownerA, fenceA.BotID, fenceA.SessionID, "stale ACP cleanup"); !errors.Is(err, runtimefence.ErrStale) {
		t.Fatalf("stale approval cleanup error = %v, want ErrStale", err)
	}
	if _, err := inputs.CancelPendingForSession(ownerA, fenceA.BotID, fenceA.SessionID, "stale ACP cleanup"); !errors.Is(err, runtimefence.ErrStale) {
		t.Fatalf("stale user input cleanup error = %v, want ErrStale", err)
	}
	gotApproval, err := approvals.Get(ctx, approval.ID)
	if err != nil || gotApproval.Status != toolapproval.StatusPending {
		t.Fatalf("owner B approval after stale cleanup = (%#v, %v)", gotApproval, err)
	}
	gotInput, err := inputs.Get(ctx, input.ID)
	if err != nil || gotInput.Status != userinput.StatusPending {
		t.Fatalf("owner B user input after stale cleanup = (%#v, %v)", gotInput, err)
	}
}

func TestPostgresRuntimeFenceActivationPreservesResponseTarget(t *testing.T) {
	t.Run("tool approval", func(t *testing.T) {
		ctx := context.Background()
		pool := openCrossBackendPostgresPool(t, ctx)
		botID, sessionID := createCrossBackendFixtures(t, ctx, pool)
		queries := dbsqlc.New(pool)
		store := postgresstore.NewQueriesWithPool(pool, queries)
		approvals := toolapproval.NewService(nil, store, nil)
		inputs := userinput.NewService(nil, store)

		fenceA := activateNextPostgresRuntimeFence(t, ctx, queries, store, botID, sessionID, runtimefence.ActivationOptions{})
		ownerA := runtimefence.WithContext(ctx, fenceA)
		target, err := approvals.CreatePending(ownerA, toolapproval.CreatePendingInput{
			BotID: fenceA.BotID, SessionID: fenceA.SessionID, ToolCallID: "approval-preserved", ToolName: "write",
			ToolInput: map[string]any{"path": "preserved.txt"},
		})
		if err != nil {
			t.Fatalf("create preserved approval: %v", err)
		}
		siblingApproval, err := approvals.CreatePending(ownerA, toolapproval.CreatePendingInput{
			BotID: fenceA.BotID, SessionID: fenceA.SessionID, ToolCallID: "approval-sibling", ToolName: "write",
			ToolInput: map[string]any{"path": "sibling.txt"},
		})
		if err != nil {
			t.Fatalf("create sibling approval: %v", err)
		}
		siblingInput := createCrossBackendPendingInput(t, ownerA, inputs, fenceA, "input-sibling")

		fenceB := activateNextPostgresRuntimeFence(t, ctx, queries, store, botID, sessionID, runtimefence.ActivationOptions{
			PreserveDecision: &runtimefence.PreservedDecision{Kind: runtimefence.DecisionToolApproval, ID: target.ID},
		})
		gotTarget, err := approvals.Get(ctx, target.ID)
		if err != nil || gotTarget.Status != toolapproval.StatusPending {
			t.Fatalf("preserved approval after activation = (%#v, %v)", gotTarget, err)
		}
		gotSiblingApproval, err := approvals.Get(ctx, siblingApproval.ID)
		if err != nil || gotSiblingApproval.Status != toolapproval.StatusCancelled {
			t.Fatalf("sibling approval after activation = (%#v, %v)", gotSiblingApproval, err)
		}
		gotSiblingInput, err := inputs.Get(ctx, siblingInput.ID)
		if err != nil || gotSiblingInput.Status != userinput.StatusCanceled {
			t.Fatalf("sibling user input after activation = (%#v, %v)", gotSiblingInput, err)
		}
		approved, err := approvals.Approve(runtimefence.WithContext(ctx, fenceB), target.ID, "", "approved after takeover")
		if err != nil || approved.Status != toolapproval.StatusApproved {
			t.Fatalf("resolve preserved approval = (%#v, %v)", approved, err)
		}
	})

	t.Run("user input", func(t *testing.T) {
		ctx := context.Background()
		pool := openCrossBackendPostgresPool(t, ctx)
		botID, sessionID := createCrossBackendFixtures(t, ctx, pool)
		queries := dbsqlc.New(pool)
		store := postgresstore.NewQueriesWithPool(pool, queries)
		approvals := toolapproval.NewService(nil, store, nil)
		inputs := userinput.NewService(nil, store)

		fenceA := activateNextPostgresRuntimeFence(t, ctx, queries, store, botID, sessionID, runtimefence.ActivationOptions{})
		ownerA := runtimefence.WithContext(ctx, fenceA)
		target := createCrossBackendPendingInput(t, ownerA, inputs, fenceA, "input-preserved")
		siblingInput := createCrossBackendPendingInput(t, ownerA, inputs, fenceA, "input-sibling")
		siblingApproval, err := approvals.CreatePending(ownerA, toolapproval.CreatePendingInput{
			BotID: fenceA.BotID, SessionID: fenceA.SessionID, ToolCallID: "approval-sibling", ToolName: "write",
			ToolInput: map[string]any{"path": "sibling.txt"},
		})
		if err != nil {
			t.Fatalf("create sibling approval: %v", err)
		}

		fenceB := activateNextPostgresRuntimeFence(t, ctx, queries, store, botID, sessionID, runtimefence.ActivationOptions{
			PreserveDecision: &runtimefence.PreservedDecision{Kind: runtimefence.DecisionUserInput, ID: target.ID},
		})
		gotTarget, err := inputs.Get(ctx, target.ID)
		if err != nil || gotTarget.Status != userinput.StatusPending {
			t.Fatalf("preserved user input after activation = (%#v, %v)", gotTarget, err)
		}
		gotSiblingInput, err := inputs.Get(ctx, siblingInput.ID)
		if err != nil || gotSiblingInput.Status != userinput.StatusCanceled {
			t.Fatalf("sibling user input after activation = (%#v, %v)", gotSiblingInput, err)
		}
		gotSiblingApproval, err := approvals.Get(ctx, siblingApproval.ID)
		if err != nil || gotSiblingApproval.Status != toolapproval.StatusCancelled {
			t.Fatalf("sibling approval after activation = (%#v, %v)", gotSiblingApproval, err)
		}
		question := target.UIPayload.Questions[0]
		option := question.Options[0]
		submitted, err := inputs.Submit(runtimefence.WithContext(ctx, fenceB), userinput.SubmitInput{
			RequestID: target.ID,
			Answers:   []userinput.QuestionAnswer{{QuestionID: question.ID, OptionIDs: []string{option.ID}}},
		})
		if err != nil || submitted.Status != userinput.StatusSubmitted {
			t.Fatalf("resolve preserved user input = (%#v, %v)", submitted, err)
		}
	})
}

func TestPostgresRuntimeFenceResponseWinsPreservedDecisionClaimRace(t *testing.T) {
	ctx := context.Background()
	pool := openCrossBackendPostgresPool(t, ctx)
	botID, sessionID := createCrossBackendFixtures(t, ctx, pool)
	queries := dbsqlc.New(pool)
	store := postgresstore.NewQueriesWithPool(pool, queries)
	approvals := toolapproval.NewService(nil, store, nil)
	inputs := userinput.NewService(nil, store)

	fenceA := activateNextPostgresRuntimeFence(t, ctx, queries, store, botID, sessionID, runtimefence.ActivationOptions{})
	ownerA := runtimefence.WithContext(ctx, fenceA)
	target, err := approvals.CreatePending(ctx, toolapproval.CreatePendingInput{
		BotID: fenceA.BotID, SessionID: fenceA.SessionID, ToolCallID: "approval-resolved-before-activation", ToolName: "write",
		ToolInput: map[string]any{"path": "target.txt"},
	})
	if err != nil {
		t.Fatalf("create response target: %v", err)
	}
	siblingApproval, err := approvals.CreatePending(ownerA, toolapproval.CreatePendingInput{
		BotID: fenceA.BotID, SessionID: fenceA.SessionID, ToolCallID: "approval-must-survive", ToolName: "write",
		ToolInput: map[string]any{"path": "sibling.txt"},
	})
	if err != nil {
		t.Fatalf("create sibling approval: %v", err)
	}
	siblingInput := createCrossBackendPendingInput(t, ownerA, inputs, fenceA, "input-must-survive")

	writer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin response transaction: %v", err)
	}
	defer func() { _ = writer.Rollback(ctx) }()
	if _, err := writer.Exec(ctx, "SELECT id FROM tool_approval_requests WHERE id = $1 FOR UPDATE", target.ID); err != nil {
		t.Fatalf("lock response target: %v", err)
	}
	writerApprovals := toolapproval.NewService(nil, postgresstore.NewQueries(dbsqlc.New(writer)), nil)

	tokenB, err := queries.NextSessionRuntimeFenceToken(ctx)
	if err != nil {
		t.Fatalf("allocate successor fence: %v", err)
	}
	fenceB := runtimefence.Fence{BotID: fenceA.BotID, SessionID: fenceA.SessionID, Token: tokenB}
	activationPID := make(chan int32, 1)
	activationStore := &signalingPostgresTransactionStore{Queries: store, pool: pool, pid: activationPID}
	activationDone := make(chan error, 1)
	go func() {
		activationDone <- runtimefence.ActivateWithOptions(ctx, activationStore, fenceB, runtimefence.ActivationOptions{
			PreserveDecision: &runtimefence.PreservedDecision{Kind: runtimefence.DecisionToolApproval, ID: target.ID},
		})
	}()
	var activationBackendPID int32
	select {
	case activationBackendPID = <-activationPID:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out starting successor activation")
	}
	waitForPostgresLockWait(t, ctx, pool, activationBackendPID)
	if _, err := writerApprovals.Approve(ctx, target.ID, "", "won the response race"); err != nil {
		t.Fatalf("resolve target while activation waits: %v", err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatalf("commit winning response: %v", err)
	}
	select {
	case err := <-activationDone:
		if !errors.Is(err, runtimefence.ErrPreservedDecisionUnavailable) {
			t.Fatalf("successor activation error = %v, want ErrPreservedDecisionUnavailable", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for rejected successor activation")
	}
	var currentToken int64
	if err := pool.QueryRow(ctx, "SELECT runtime_fencing_token FROM bot_sessions WHERE id = $1", sessionID).Scan(&currentToken); err != nil {
		t.Fatalf("load runtime fence after rejected activation: %v", err)
	}
	if currentToken != fenceA.Token {
		t.Fatalf("runtime fence advanced to %d, want %d", currentToken, fenceA.Token)
	}
	gotSiblingApproval, err := approvals.Get(ctx, siblingApproval.ID)
	if err != nil || gotSiblingApproval.Status != toolapproval.StatusPending {
		t.Fatalf("sibling approval after rejected activation = (%#v, %v)", gotSiblingApproval, err)
	}
	gotSiblingInput, err := inputs.Get(ctx, siblingInput.ID)
	if err != nil || gotSiblingInput.Status != userinput.StatusPending {
		t.Fatalf("sibling user input after rejected activation = (%#v, %v)", gotSiblingInput, err)
	}
}

func TestPostgresRuntimeFenceActivationRejectsExpiredPreservedInput(t *testing.T) {
	ctx := context.Background()
	pool := openCrossBackendPostgresPool(t, ctx)
	botID, sessionID := createCrossBackendFixtures(t, ctx, pool)
	queries := dbsqlc.New(pool)
	store := postgresstore.NewQueriesWithPool(pool, queries)
	approvals := toolapproval.NewService(nil, store, nil)
	inputs := userinput.NewService(nil, store)

	fenceA := activateNextPostgresRuntimeFence(t, ctx, queries, store, botID, sessionID, runtimefence.ActivationOptions{})
	ownerA := runtimefence.WithContext(ctx, fenceA)
	expiredAt := time.Now().Add(-time.Minute)
	target, err := inputs.CreatePending(ownerA, userinput.CreatePendingInput{
		BotID: fenceA.BotID, SessionID: fenceA.SessionID, ToolCallID: "expired-preserved-input", ToolName: userinput.ToolNameAskUser,
		Input: map[string]any{"questions": []any{map[string]any{
			"text": "Expired question", "kind": userinput.QuestionKindSingleSelect,
			"options": []any{map[string]any{"label": "A"}, map[string]any{"label": "B"}},
		}}},
		ExpiresAt: &expiredAt,
	})
	if err != nil {
		t.Fatalf("create expired response target: %v", err)
	}
	siblingInput := createCrossBackendPendingInput(t, ownerA, inputs, fenceA, "input-survives-expired-target")
	siblingApproval, err := approvals.CreatePending(ownerA, toolapproval.CreatePendingInput{
		BotID: fenceA.BotID, SessionID: fenceA.SessionID, ToolCallID: "approval-survives-expired-target", ToolName: "write",
		ToolInput: map[string]any{"path": "sibling.txt"},
	})
	if err != nil {
		t.Fatalf("create sibling approval: %v", err)
	}

	tokenB, err := queries.NextSessionRuntimeFenceToken(ctx)
	if err != nil {
		t.Fatalf("allocate successor fence: %v", err)
	}
	fenceB := runtimefence.Fence{BotID: fenceA.BotID, SessionID: fenceA.SessionID, Token: tokenB}
	err = runtimefence.ActivateWithOptions(ctx, store, fenceB, runtimefence.ActivationOptions{
		PreserveDecision: &runtimefence.PreservedDecision{Kind: runtimefence.DecisionUserInput, ID: target.ID},
	})
	if !errors.Is(err, runtimefence.ErrPreservedDecisionUnavailable) {
		t.Fatalf("successor activation error = %v, want ErrPreservedDecisionUnavailable", err)
	}
	assertPostgresRuntimeFenceToken(t, ctx, pool, sessionID, fenceA.Token)
	if got, getErr := inputs.Get(ctx, siblingInput.ID); getErr != nil || got.Status != userinput.StatusPending {
		t.Fatalf("sibling input after rejected activation = (%#v, %v)", got, getErr)
	}
	if got, getErr := approvals.Get(ctx, siblingApproval.ID); getErr != nil || got.Status != toolapproval.StatusPending {
		t.Fatalf("sibling approval after rejected activation = (%#v, %v)", got, getErr)
	}
}

func TestPostgresRuntimeFenceClaimWinsUnfencedResponseRace(t *testing.T) {
	t.Run("tool approval", func(t *testing.T) {
		ctx := context.Background()
		pool := openCrossBackendPostgresPool(t, ctx)
		botID, sessionID := createCrossBackendFixtures(t, ctx, pool)
		queries := dbsqlc.New(pool)
		store := postgresstore.NewQueriesWithPool(pool, queries)
		approvals := toolapproval.NewService(nil, store, nil)

		fenceA := activateNextPostgresRuntimeFence(t, ctx, queries, store, botID, sessionID, runtimefence.ActivationOptions{})
		target, err := approvals.CreatePending(ctx, toolapproval.CreatePendingInput{
			BotID: fenceA.BotID, SessionID: fenceA.SessionID, ToolCallID: "approval-claim-wins", ToolName: "write",
			ToolInput: map[string]any{"path": "claimed.txt"},
		})
		if err != nil {
			t.Fatalf("create approval target: %v", err)
		}
		fenceB, activationDone, claimed, resume := startPausedPostgresActivation(
			t, ctx, pool, queries, store, fenceA, runtimefence.DecisionToolApproval, target.ID,
		)
		<-claimed

		writerConn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("acquire unfenced writer: %v", err)
		}
		defer writerConn.Release()
		writerPID := postgresBackendPID(t, ctx, writerConn)
		writerApprovals := toolapproval.NewService(nil, postgresstore.NewQueries(dbsqlc.New(writerConn)), nil)
		writerDone := make(chan error, 1)
		go func() {
			_, writerErr := writerApprovals.Approve(ctx, target.ID, "", "stale response")
			writerDone <- writerErr
		}()
		waitForPostgresLockWait(t, ctx, pool, writerPID)
		close(resume)
		awaitPostgresActivation(t, activationDone)
		select {
		case writerErr := <-writerDone:
			if !errors.Is(writerErr, toolapproval.ErrAlreadyDecided) {
				t.Fatalf("unfenced approval response error = %v, want ErrAlreadyDecided", writerErr)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for unfenced approval response")
		}
		canceled, err := approvals.CancelPendingForSession(ctx, fenceB.BotID, fenceB.SessionID, "stale unfenced cleanup")
		if err != nil {
			t.Fatalf("unfenced approval cleanup: %v", err)
		}
		if len(canceled) != 0 {
			t.Fatalf("unfenced cleanup canceled claimed approvals: %#v", canceled)
		}
		if _, err := approvals.Approve(runtimefence.WithContext(ctx, fenceB), target.ID, "", "current response"); err != nil {
			t.Fatalf("resolve claimed approval with current fence: %v", err)
		}
	})

	t.Run("user input", func(t *testing.T) {
		ctx := context.Background()
		pool := openCrossBackendPostgresPool(t, ctx)
		botID, sessionID := createCrossBackendFixtures(t, ctx, pool)
		queries := dbsqlc.New(pool)
		store := postgresstore.NewQueriesWithPool(pool, queries)
		inputs := userinput.NewService(nil, store)

		fenceA := activateNextPostgresRuntimeFence(t, ctx, queries, store, botID, sessionID, runtimefence.ActivationOptions{})
		target := createCrossBackendPendingInput(t, ctx, inputs, fenceA, "input-claim-wins")
		fenceB, activationDone, claimed, resume := startPausedPostgresActivation(
			t, ctx, pool, queries, store, fenceA, runtimefence.DecisionUserInput, target.ID,
		)
		<-claimed

		writerConn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("acquire unfenced writer: %v", err)
		}
		defer writerConn.Release()
		writerPID := postgresBackendPID(t, ctx, writerConn)
		writerInputs := userinput.NewService(nil, postgresstore.NewQueries(dbsqlc.New(writerConn)))
		answer := userInputAnswer(target)
		writerDone := make(chan error, 1)
		go func() {
			_, writerErr := writerInputs.Submit(ctx, userinput.SubmitInput{RequestID: target.ID, Answers: answer})
			writerDone <- writerErr
		}()
		waitForPostgresLockWait(t, ctx, pool, writerPID)
		close(resume)
		awaitPostgresActivation(t, activationDone)
		select {
		case writerErr := <-writerDone:
			if !errors.Is(writerErr, userinput.ErrAlreadyDecided) {
				t.Fatalf("unfenced input response error = %v, want ErrAlreadyDecided", writerErr)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for unfenced input response")
		}
		canceled, err := inputs.CancelPendingForSession(ctx, fenceB.BotID, fenceB.SessionID, "stale unfenced cleanup")
		if err != nil {
			t.Fatalf("unfenced input cleanup: %v", err)
		}
		if len(canceled) != 0 {
			t.Fatalf("unfenced cleanup canceled claimed inputs: %#v", canceled)
		}
		if _, err := inputs.Fail(ctx, target.ID, map[string]any{"status": "failed"}); !errors.Is(err, userinput.ErrAlreadyDecided) {
			t.Fatalf("unfenced fail error = %v, want ErrAlreadyDecided", err)
		}
		if _, err := inputs.Submit(runtimefence.WithContext(ctx, fenceB), userinput.SubmitInput{RequestID: target.ID, Answers: answer}); err != nil {
			t.Fatalf("resolve claimed input with current fence: %v", err)
		}
	})
}

func TestPostgresRuntimeFenceDecisionPayloadIsImmutable(t *testing.T) {
	t.Run("tool approval", func(t *testing.T) {
		ctx := context.Background()
		pool := openCrossBackendPostgresPool(t, ctx)
		botID, sessionID := createCrossBackendFixtures(t, ctx, pool)
		store := postgresstore.NewQueriesWithPool(pool, dbsqlc.New(pool))
		approvals := toolapproval.NewService(nil, store, nil)
		input := toolapproval.CreatePendingInput{
			BotID: botID.String(), SessionID: sessionID.String(), ToolCallID: "immutable-approval", ToolName: "write",
			ToolInput: map[string]any{"path": "approved.txt"},
		}
		first, err := approvals.CreatePending(ctx, input)
		if err != nil {
			t.Fatalf("create approval: %v", err)
		}
		if duplicate, err := approvals.CreatePending(ctx, input); err != nil || duplicate.ID != first.ID {
			t.Fatalf("idempotent approval create = (%#v, %v)", duplicate, err)
		}
		input.ToolInput = map[string]any{"path": "changed.txt"}
		if _, err := approvals.CreatePending(ctx, input); !errors.Is(err, toolapproval.ErrAlreadyDecided) {
			t.Fatalf("changed approval payload error = %v, want ErrAlreadyDecided", err)
		}
		got, err := approvals.Get(ctx, first.ID)
		if err != nil || got.ToolInput["path"] != "approved.txt" {
			t.Fatalf("stored approval after changed duplicate = (%#v, %v)", got, err)
		}
	})

	t.Run("user input", func(t *testing.T) {
		ctx := context.Background()
		pool := openCrossBackendPostgresPool(t, ctx)
		botID, sessionID := createCrossBackendFixtures(t, ctx, pool)
		store := postgresstore.NewQueriesWithPool(pool, dbsqlc.New(pool))
		inputs := userinput.NewService(nil, store)
		input := userinput.CreatePendingInput{
			BotID: botID.String(), SessionID: sessionID.String(), ToolCallID: "immutable-input", ToolName: userinput.ToolNameAskUser,
			Input: map[string]any{"questions": []any{map[string]any{
				"text": "Approved question", "kind": userinput.QuestionKindSingleSelect,
				"options": []any{map[string]any{"label": "A"}, map[string]any{"label": "B"}},
			}}},
		}
		first, err := inputs.CreatePending(ctx, input)
		if err != nil {
			t.Fatalf("create user input: %v", err)
		}
		if duplicate, err := inputs.CreatePending(ctx, input); err != nil || duplicate.ID != first.ID {
			t.Fatalf("idempotent user input create = (%#v, %v)", duplicate, err)
		}
		input.Input = map[string]any{"questions": []any{map[string]any{
			"text": "Changed question", "kind": userinput.QuestionKindText,
		}}}
		if _, err := inputs.CreatePending(ctx, input); !errors.Is(err, userinput.ErrAlreadyDecided) {
			t.Fatalf("changed user input payload error = %v, want ErrAlreadyDecided", err)
		}
		got, err := inputs.Get(ctx, first.ID)
		if err != nil || len(got.UIPayload.Questions) != 1 || got.UIPayload.Questions[0].Text != "Approved question" {
			t.Fatalf("stored user input after changed duplicate = (%#v, %v)", got, err)
		}
	})
}

func activateNextPostgresRuntimeFence(t *testing.T, ctx context.Context, queries *dbsqlc.Queries, store dbstore.Queries, botID, sessionID pgtype.UUID, options runtimefence.ActivationOptions) runtimefence.Fence {
	t.Helper()
	token, err := queries.NextSessionRuntimeFenceToken(ctx)
	if err != nil {
		t.Fatalf("allocate runtime fence token: %v", err)
	}
	fence := runtimefence.Fence{BotID: uuid.UUID(botID.Bytes).String(), SessionID: uuid.UUID(sessionID.Bytes).String(), Token: token}
	if err := runtimefence.ActivateWithOptions(ctx, store, fence, options); err != nil {
		t.Fatalf("activate runtime fence: %v", err)
	}
	return fence
}

func createCrossBackendPendingInput(t *testing.T, ctx context.Context, inputs *userinput.Service, fence runtimefence.Fence, toolCallID string) userinput.Request {
	t.Helper()
	input, err := inputs.CreatePending(ctx, userinput.CreatePendingInput{
		BotID: fence.BotID, SessionID: fence.SessionID, ToolCallID: toolCallID, ToolName: userinput.ToolNameAskUser,
		Input: map[string]any{"questions": []any{map[string]any{
			"text": "Choose", "kind": userinput.QuestionKindSingleSelect,
			"options": []any{map[string]any{"label": "A"}, map[string]any{"label": "B"}},
		}}},
	})
	if err != nil {
		t.Fatalf("create pending user input %s: %v", toolCallID, err)
	}
	return input
}

func userInputAnswer(request userinput.Request) []userinput.QuestionAnswer {
	question := request.UIPayload.Questions[0]
	return []userinput.QuestionAnswer{{QuestionID: question.ID, OptionIDs: []string{question.Options[0].ID}}}
}

func startPausedPostgresActivation(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	queries *dbsqlc.Queries,
	store dbstore.Queries,
	current runtimefence.Fence,
	decisionKind string,
	decisionID string,
) (runtimefence.Fence, <-chan error, <-chan struct{}, chan struct{}) {
	t.Helper()
	token, err := queries.NextSessionRuntimeFenceToken(ctx)
	if err != nil {
		t.Fatalf("allocate successor fence: %v", err)
	}
	fence := runtimefence.Fence{BotID: current.BotID, SessionID: current.SessionID, Token: token}
	claimed := make(chan struct{})
	resume := make(chan struct{})
	done := make(chan error, 1)
	activationStore := &pausingClaimPostgresTransactionStore{
		Queries: store,
		pool:    pool,
		kind:    decisionKind,
		claimed: claimed,
		resume:  resume,
	}
	go func() {
		done <- runtimefence.ActivateWithOptions(ctx, activationStore, fence, runtimefence.ActivationOptions{
			PreserveDecision: &runtimefence.PreservedDecision{Kind: decisionKind, ID: decisionID},
		})
	}()
	return fence, done, claimed, resume
}

func awaitPostgresActivation(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("activate successor fence: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for successor activation")
	}
}

func assertPostgresRuntimeFenceToken(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sessionID pgtype.UUID, want int64) {
	t.Helper()
	var got int64
	if err := pool.QueryRow(ctx, "SELECT runtime_fencing_token FROM bot_sessions WHERE id = $1", sessionID).Scan(&got); err != nil {
		t.Fatalf("load runtime fence: %v", err)
	}
	if got != want {
		t.Fatalf("runtime fence token = %d, want %d", got, want)
	}
}

func postgresBackendPID(t *testing.T, ctx context.Context, conn *pgxpool.Conn) int32 {
	t.Helper()
	var pid int32
	if err := conn.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatalf("load PostgreSQL backend PID: %v", err)
	}
	return pid
}

func TestPostgresRuntimeFenceActivationCleansDecisionCommittedWhileWaiting(t *testing.T) {
	ctx := context.Background()
	pool := openCrossBackendPostgresPool(t, ctx)
	botID, sessionID := createCrossBackendFixtures(t, ctx, pool)
	queries := dbsqlc.New(pool)
	store := postgresstore.NewQueriesWithPool(pool, queries)

	tokenA, err := queries.NextSessionRuntimeFenceToken(ctx)
	if err != nil {
		t.Fatalf("allocate owner A token: %v", err)
	}
	fenceA := runtimefence.Fence{BotID: botID.String(), SessionID: sessionID.String(), Token: tokenA}
	if err := runtimefence.Activate(ctx, store, fenceA); err != nil {
		t.Fatalf("activate owner A token: %v", err)
	}

	writer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin owner A decision writer: %v", err)
	}
	defer func() { _ = writer.Rollback(ctx) }()
	writerStore := &existingPostgresTransactionStore{Queries: postgresstore.NewQueries(dbsqlc.New(writer))}
	approvals := toolapproval.NewService(nil, writerStore, nil)
	inputs := userinput.NewService(nil, writerStore)
	ownerA := runtimefence.WithContext(ctx, fenceA)
	approval, err := approvals.CreatePending(ownerA, toolapproval.CreatePendingInput{
		BotID: fenceA.BotID, SessionID: fenceA.SessionID, ToolCallID: "approval-waiting-writer", ToolName: "write",
		ToolInput: map[string]any{"path": "waiting.txt"},
	})
	if err != nil {
		t.Fatalf("create waiting owner approval: %v", err)
	}
	input, err := inputs.CreatePending(ownerA, userinput.CreatePendingInput{
		BotID: fenceA.BotID, SessionID: fenceA.SessionID, ToolCallID: "input-waiting-writer", ToolName: userinput.ToolNameAskUser,
		Input: map[string]any{"questions": []any{map[string]any{
			"text": "Waiting question", "kind": userinput.QuestionKindSingleSelect,
			"options": []any{map[string]any{"label": "A"}, map[string]any{"label": "B"}},
		}}},
	})
	if err != nil {
		t.Fatalf("create waiting owner user input: %v", err)
	}

	tokenB, err := queries.NextSessionRuntimeFenceToken(ctx)
	if err != nil {
		t.Fatalf("allocate owner B token: %v", err)
	}
	fenceB := runtimefence.Fence{BotID: botID.String(), SessionID: sessionID.String(), Token: tokenB}
	activationPID := make(chan int32, 1)
	activationStore := &signalingPostgresTransactionStore{Queries: store, pool: pool, pid: activationPID}
	activationDone := make(chan error, 1)
	go func() {
		activationDone <- runtimefence.Activate(ctx, activationStore, fenceB)
	}()

	var pid int32
	select {
	case pid = <-activationPID:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out starting owner B activation")
	}
	waitForPostgresLockWait(t, ctx, pool, pid)
	if err := writer.Commit(ctx); err != nil {
		t.Fatalf("commit owner A decisions: %v", err)
	}
	select {
	case err := <-activationDone:
		if err != nil {
			t.Fatalf("activate owner B after writer commit: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for owner B activation")
	}

	readApprovals := toolapproval.NewService(nil, store, nil)
	readInputs := userinput.NewService(nil, store)
	gotApproval, err := readApprovals.Get(ctx, approval.ID)
	if err != nil || gotApproval.Status != toolapproval.StatusCancelled {
		t.Fatalf("waiting approval after takeover = (%#v, %v)", gotApproval, err)
	}
	gotInput, err := readInputs.Get(ctx, input.ID)
	if err != nil || gotInput.Status != userinput.StatusCanceled {
		t.Fatalf("waiting user input after takeover = (%#v, %v)", gotInput, err)
	}
}

type existingPostgresTransactionStore struct {
	*postgresstore.Queries
}

func (*existingPostgresTransactionStore) SupportsTransactions() bool { return true }

func (s *existingPostgresTransactionStore) InTx(_ context.Context, fn func(dbstore.Queries) error) error {
	return fn(s)
}

type pausingClaimPostgresTransactionStore struct {
	dbstore.Queries
	pool    *pgxpool.Pool
	kind    string
	claimed chan struct{}
	resume  <-chan struct{}
}

func (*pausingClaimPostgresTransactionStore) SupportsTransactions() bool { return true }

func (s *pausingClaimPostgresTransactionStore) InTx(ctx context.Context, fn func(dbstore.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := &pausingClaimPostgresQueries{
		Queries: postgresstore.NewQueries(dbsqlc.New(tx)),
		kind:    s.kind,
		claimed: s.claimed,
		resume:  s.resume,
	}
	if err := fn(queries); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type pausingClaimPostgresQueries struct {
	*postgresstore.Queries
	kind    string
	claimed chan struct{}
	resume  <-chan struct{}
	once    sync.Once
}

func (q *pausingClaimPostgresQueries) ClaimToolApprovalRequestForRuntime(ctx context.Context, arg dbsqlc.ClaimToolApprovalRequestForRuntimeParams) (dbsqlc.ToolApprovalRequest, error) {
	row, err := q.Queries.ClaimToolApprovalRequestForRuntime(ctx, arg)
	if err == nil && q.kind == runtimefence.DecisionToolApproval {
		q.pauseAfterClaim(ctx)
	}
	return row, err
}

func (q *pausingClaimPostgresQueries) ClaimUserInputRequestForRuntime(ctx context.Context, arg dbsqlc.ClaimUserInputRequestForRuntimeParams) (dbsqlc.UserInputRequest, error) {
	row, err := q.Queries.ClaimUserInputRequestForRuntime(ctx, arg)
	if err == nil && q.kind == runtimefence.DecisionUserInput {
		q.pauseAfterClaim(ctx)
	}
	return row, err
}

func (q *pausingClaimPostgresQueries) pauseAfterClaim(ctx context.Context) {
	q.once.Do(func() { close(q.claimed) })
	select {
	case <-q.resume:
	case <-ctx.Done():
	}
}

type signalingPostgresTransactionStore struct {
	dbstore.Queries
	pool *pgxpool.Pool
	pid  chan<- int32
}

func (*signalingPostgresTransactionStore) SupportsTransactions() bool { return true }

func (s *signalingPostgresTransactionStore) InTx(ctx context.Context, fn func(dbstore.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var pid int32
	if err := tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		return err
	}
	s.pid <- pid
	if err := fn(postgresstore.NewQueries(dbsqlc.New(tx))); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func waitForPostgresLockWait(t *testing.T, ctx context.Context, pool *pgxpool.Pool, pid int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if err := pool.QueryRow(ctx, "SELECT COALESCE(wait_event_type = 'Lock', false) FROM pg_stat_activity WHERE pid = $1", pid).Scan(&blocked); err != nil {
			t.Fatalf("inspect activation lock wait: %v", err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("owner B activation never waited for the owner A writer lock")
}

func TestPostgresRuntimeFenceProtectsSessionUpdates(t *testing.T) {
	ctx := context.Background()
	pool := openCrossBackendPostgresPool(t, ctx)
	botID, sessionID := createCrossBackendFixtures(t, ctx, pool)
	queries := dbsqlc.New(pool)
	store := postgresstore.NewQueriesWithPool(pool, queries)
	service := sessionpkg.NewService(nil, store, nil)

	tokenA, err := queries.NextSessionRuntimeFenceToken(ctx)
	if err != nil {
		t.Fatalf("allocate owner A token: %v", err)
	}
	fenceA := runtimefence.Fence{BotID: botID.String(), SessionID: sessionID.String(), Token: tokenA}
	if err := runtimefence.Activate(ctx, store, fenceA); err != nil {
		t.Fatalf("activate owner A token: %v", err)
	}
	tokenB, err := queries.NextSessionRuntimeFenceToken(ctx)
	if err != nil {
		t.Fatalf("allocate owner B token: %v", err)
	}
	fenceB := runtimefence.Fence{BotID: botID.String(), SessionID: sessionID.String(), Token: tokenB}
	if err := runtimefence.Activate(ctx, store, fenceB); err != nil {
		t.Fatalf("activate owner B token: %v", err)
	}

	ownerB := runtimefence.WithContext(ctx, fenceB)
	if _, err := service.UpdateTitle(ownerB, fenceB.SessionID, "owner B title"); err != nil {
		t.Fatalf("update owner B title: %v", err)
	}
	if _, err := service.UpdateMetadata(ownerB, fenceB.SessionID, map[string]any{"owner": "B"}); err != nil {
		t.Fatalf("update owner B metadata: %v", err)
	}
	ownerA := runtimefence.WithContext(ctx, fenceA)
	if _, err := service.UpdateTitle(ownerA, fenceA.SessionID, "stale owner A title"); !errors.Is(err, runtimefence.ErrStale) {
		t.Fatalf("stale title update error = %v, want ErrStale", err)
	}
	if _, err := service.UpdateMetadata(ownerA, fenceA.SessionID, map[string]any{"owner": "A"}); !errors.Is(err, runtimefence.ErrStale) {
		t.Fatalf("stale metadata update error = %v, want ErrStale", err)
	}
	row, err := queries.GetSessionByID(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session after stale updates: %v", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
		t.Fatalf("decode session metadata: %v", err)
	}
	if row.Title != "owner B title" || metadata["owner"] != "B" {
		t.Fatalf("session after stale updates = title:%q metadata:%s", row.Title, row.Metadata)
	}
}

func openCrossBackendPostgresPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		if os.Getenv("MEMOH_TEST_CROSS_BACKEND_REQUIRED") == "1" {
			t.Fatal("cross-backend runtime fencing is required, but TEST_POSTGRES_DSN is not set")
		}
		t.Skip("set TEST_POSTGRES_DSN to run cross-backend fencing")
	}
	pool, err := dbpkg.OpenPostgresDSN(ctx, dsn)
	if err != nil {
		t.Fatalf("create cross-backend postgres pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("cross-backend postgres ping: %v", err)
	}
	if os.Getenv("TEST_POSTGRES_BOOTSTRAP_SCHEMA") == "1" {
		crossBackendMigrationOnce.Do(func() {
			crossBackendMigrationErr = dbtest.MigratePostgresUp(dsn)
		})
		if crossBackendMigrationErr != nil {
			t.Fatalf("migrate PostgreSQL test database: %v", crossBackendMigrationErr)
		}
	}
	return pool
}

func createCrossBackendFixtures(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (pgtype.UUID, pgtype.UUID) {
	t.Helper()
	userID := uuid.New()
	botID := uuid.New()
	sessionID := uuid.New()
	name := fmt.Sprintf("rtf-%s", uuid.NewString()[:12])
	if _, err := pool.Exec(ctx, `
		WITH created_user AS (
			INSERT INTO users (id, username, is_active)
			VALUES ($1, $2, true)
			RETURNING id
		)
		INSERT INTO team_members (user_id, role)
		SELECT id, 'admin' FROM created_user`, userID, name); err != nil {
		t.Fatalf("create cross-backend user: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO bots (id, owner_user_id, name) VALUES ($1, $2, $3)", botID, userID, name); err != nil {
		t.Fatalf("create cross-backend bot: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO bot_sessions (id, bot_id, channel_type) VALUES ($1, $2, 'local')", sessionID, botID); err != nil {
		t.Fatalf("create cross-backend session: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM bots WHERE id = $1", botID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})
	pgBotID, err := dbpkg.ParseUUID(botID.String())
	if err != nil {
		t.Fatalf("parse cross-backend bot id: %v", err)
	}
	pgSessionID, err := dbpkg.ParseUUID(sessionID.String())
	if err != nil {
		t.Fatalf("parse cross-backend session id: %v", err)
	}
	return pgBotID, pgSessionID
}
