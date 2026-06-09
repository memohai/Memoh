package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/background"
)

// fakeSpawnAgent satisfies SpawnAgent. When block is non-nil it waits for the
// channel to close or the context to cancel before returning.
type fakeSpawnAgent struct {
	block   chan struct{}
	failFor map[string]string // query -> non-retryable error message
}

func (f *fakeSpawnAgent) Generate(ctx context.Context, cfg SpawnRunConfig) (*SpawnResult, error) {
	return f.GenerateWithWatchdog(ctx, cfg, func() {})
}

func (f *fakeSpawnAgent) GenerateWithWatchdog(ctx context.Context, cfg SpawnRunConfig, _ func()) (*SpawnResult, error) {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if msg, ok := f.failFor[cfg.Query]; ok {
		return nil, errors.New(msg)
	}
	return &SpawnResult{Text: "report for " + cfg.Query}, nil
}

func newBgSpawnProvider(t *testing.T, agent SpawnAgent) (*SpawnProvider, *background.Manager) {
	t.Helper()
	mgr := background.New(nil)
	p := NewSpawnProvider(nil, nil, nil, nil, nil, mgr)
	p.SetAgent(agent)
	p.modelResolver = func(context.Context, string) (*sdk.Model, string, string, error) {
		return &sdk.Model{}, "model-1", "", nil
	}
	return p, mgr
}

func spawnExecuteFn(t *testing.T, p *SpawnProvider, session SessionContext) func(args map[string]any) (any, error) {
	t.Helper()
	toolList, err := p.Tools(context.Background(), session)
	if err != nil || len(toolList) != 1 {
		t.Fatalf("expected one spawn tool, got %d (err=%v)", len(toolList), err)
	}
	return func(args map[string]any) (any, error) {
		return toolList[0].Execute(&sdk.ToolExecContext{Context: context.Background()}, args)
	}
}

func executeWithin(t *testing.T, fn func(args map[string]any) (any, error), args map[string]any, timeout time.Duration) (any, error) {
	t.Helper()
	type outcome struct {
		result any
		err    error
	}
	resCh := make(chan outcome, 1)
	go func() {
		r, err := fn(args)
		resCh <- outcome{r, err}
	}()
	select {
	case out := <-resCh:
		return out.result, out.err
	case <-time.After(timeout):
		t.Fatalf("spawn execute did not return within %s", timeout)
		return nil, nil
	}
}

func drainSpawnNotifications(t *testing.T, mgr *background.Manager, botID, sessionID string, want int) []background.Notification {
	t.Helper()
	deadline := time.After(5 * time.Second)
	var all []background.Notification
	for {
		all = append(all, mgr.DrainNotifications(botID, sessionID)...)
		if len(all) >= want {
			return all
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d notifications, got %d", want, len(all))
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestSpawnRunInBackgroundReturnsImmediatelyAndNotifiesJoinRecord(t *testing.T) {
	fake := &fakeSpawnAgent{block: make(chan struct{})}
	p, mgr := newBgSpawnProvider(t, fake)
	session := SessionContext{BotID: "bot1", SessionID: "sess1"}
	execute := spawnExecuteFn(t, p, session)

	result, err := executeWithin(t, execute, map[string]any{
		"tasks":             []any{"alpha", "beta"},
		"run_in_background": true,
	}, 2*time.Second)
	if err != nil {
		t.Fatalf("background spawn failed: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["status"] != "background_started" {
		t.Fatalf("expected background_started, got %v", m["status"])
	}
	taskID, _ := m["task_id"].(string)
	if taskID == "" {
		t.Fatal("expected non-empty task_id")
	}
	if desc, _ := m["description"].(string); !strings.Contains(desc, "alpha") {
		t.Errorf("expected description carrying the task list, got %q", desc)
	}
	if task := mgr.GetForSession("bot1", "sess1", taskID); task == nil || task.Status != background.TaskRunning {
		t.Fatalf("expected running spawn task registered for session, got %+v", task)
	}

	close(fake.block)
	n := drainSpawnNotifications(t, mgr, "bot1", "sess1", 1)[0]
	if n.Kind != background.KindSpawn || n.TaskID != taskID {
		t.Errorf("unexpected notification identity: %+v", n)
	}
	if n.Status != background.TaskCompleted {
		t.Errorf("expected completed join, got %s", n.Status)
	}
	if len(n.Branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(n.Branches))
	}
	if n.Branches[0].Task != "alpha" || n.Branches[0].Report != "report for alpha" {
		t.Errorf("unexpected first branch: %+v", n.Branches[0])
	}
	if n.Branches[1].Task != "beta" || n.Branches[1].Status != background.TaskCompleted {
		t.Errorf("unexpected second branch: %+v", n.Branches[1])
	}
}

func TestSpawnRunInBackgroundRecordsBranchFailure(t *testing.T) {
	fake := &fakeSpawnAgent{failFor: map[string]string{"beta": "invalid task configuration"}}
	p, mgr := newBgSpawnProvider(t, fake)
	session := SessionContext{BotID: "bot1", SessionID: "sess1"}
	execute := spawnExecuteFn(t, p, session)

	if _, err := executeWithin(t, execute, map[string]any{
		"tasks":             []any{"alpha", "beta"},
		"run_in_background": true,
	}, 2*time.Second); err != nil {
		t.Fatalf("background spawn failed: %v", err)
	}

	n := drainSpawnNotifications(t, mgr, "bot1", "sess1", 1)[0]
	if n.Status != background.TaskFailed {
		t.Errorf("expected failed join when a branch fails, got %s", n.Status)
	}
	if n.Branches[0].Status != background.TaskCompleted {
		t.Errorf("expected alpha branch completed, got %+v", n.Branches[0])
	}
	if n.Branches[1].Status != background.TaskFailed || !strings.Contains(n.Branches[1].Error, "invalid task configuration") {
		t.Errorf("expected beta branch failure preserved, got %+v", n.Branches[1])
	}
}

func TestSpawnBackgroundKillCancelsBranchesAndSuppressesNotification(t *testing.T) {
	fake := &fakeSpawnAgent{block: make(chan struct{})}
	p, mgr := newBgSpawnProvider(t, fake)
	session := SessionContext{BotID: "bot1", SessionID: "sess1"}
	execute := spawnExecuteFn(t, p, session)

	result, err := executeWithin(t, execute, map[string]any{
		"tasks":             []any{"alpha"},
		"run_in_background": true,
	}, 2*time.Second)
	if err != nil {
		t.Fatalf("background spawn failed: %v", err)
	}
	taskID, _ := result.(map[string]any)["task_id"].(string)
	if taskID == "" {
		t.Fatal("expected non-empty task_id")
	}

	if err := mgr.KillForSession("bot1", "sess1", taskID); err != nil {
		t.Fatalf("kill failed: %v", err)
	}

	// The join still records branch outcomes once cancellation propagates.
	deadline := time.After(5 * time.Second)
	for len(mgr.Get(taskID).Snapshot().Branches) != 1 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for branch outcomes after kill")
		case <-time.After(10 * time.Millisecond):
		}
	}

	task := mgr.Get(taskID)
	if task.Status != background.TaskKilled {
		t.Errorf("expected killed status, got %s", task.Status)
	}
	if snap := task.Snapshot(); snap.Branches[0].Status != background.TaskFailed {
		t.Errorf("expected killed branch recorded as failed, got %+v", snap.Branches[0])
	}
	if n := mgr.DrainNotifications("bot1", "sess1"); len(n) != 0 {
		t.Errorf("expected no notifications for killed spawn task, got %d", len(n))
	}
}

func TestSpawnBackgroundHonorsManagerRunningCapAcrossRuns(t *testing.T) {
	fake := &fakeSpawnAgent{block: make(chan struct{})}
	p, mgr := newBgSpawnProvider(t, fake)
	session := SessionContext{BotID: "bot1", SessionID: "sess1"}

	firstRun := spawnExecuteFn(t, p, session)
	for range background.MaxRunningSpawnTasks {
		result, err := executeWithin(t, firstRun, map[string]any{
			"tasks":             []any{"task"},
			"run_in_background": true,
		}, 2*time.Second)
		if err != nil {
			t.Fatalf("background spawn under cap failed: %v", err)
		}
		if result.(map[string]any)["status"] != "background_started" {
			t.Fatalf("expected background_started under cap, got %v", result)
		}
	}

	// A fresh agent run gets a fresh per-run closure, but the manager cap
	// still applies across runs.
	secondRun := spawnExecuteFn(t, p, session)
	result, err := executeWithin(t, secondRun, map[string]any{
		"tasks":             []any{"over cap"},
		"run_in_background": true,
	}, 2*time.Second)
	if err != nil {
		t.Fatalf("expected structured limit result, got error: %v", err)
	}
	m := result.(map[string]any)
	if m["isError"] != true {
		t.Fatalf("expected isError result at cap, got %v", m)
	}
	text := m["content"].([]map[string]any)[0]["text"].(string)
	if !strings.Contains(text, "spawn limit") {
		t.Errorf("expected limit message, got %q", text)
	}

	close(fake.block)
	drainSpawnNotifications(t, mgr, "bot1", "sess1", background.MaxRunningSpawnTasks)
}

func TestSpawnForegroundPathUnchangedAndSchemaExposesBackgroundFlag(t *testing.T) {
	fake := &fakeSpawnAgent{}
	p, _ := newBgSpawnProvider(t, fake)
	session := SessionContext{BotID: "bot1", SessionID: "sess1"}

	toolList, err := p.Tools(context.Background(), session)
	if err != nil || len(toolList) != 1 {
		t.Fatalf("expected one spawn tool, got %d (err=%v)", len(toolList), err)
	}
	props := toolList[0].Parameters.(map[string]any)["properties"].(map[string]any)
	if _, ok := props["run_in_background"]; !ok {
		t.Error("expected run_in_background in spawn tool schema")
	}

	result, err := toolList[0].Execute(&sdk.ToolExecContext{Context: context.Background()}, map[string]any{
		"tasks": []any{"alpha"},
	})
	if err != nil {
		t.Fatalf("foreground spawn failed: %v", err)
	}
	results, ok := result.(map[string]any)["results"].([]spawnResult)
	if !ok || len(results) != 1 {
		t.Fatalf("expected one foreground result, got %v", result)
	}
	if !results[0].Success || results[0].Text != "report for alpha" {
		t.Errorf("unexpected foreground result: %+v", results[0])
	}
}
