package background

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

func TestSpawnCompletesAndWaits(t *testing.T) {
	mgr := New(nil)
	taskID, outputFile := mgr.Spawn(
		context.Background(),
		"bot1",
		"sess1",
		"echo ok",
		"/data",
		"Say ok",
		func(context.Context, string, string, int32) (*bridge.ExecResult, error) {
			return &bridge.ExecResult{Stdout: "ok\n", ExitCode: 0}, nil
		},
		nil,
		nil,
	)
	if taskID == "" || outputFile == "" {
		t.Fatalf("Spawn returned taskID=%q outputFile=%q", taskID, outputFile)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	snap, err := mgr.WaitForSessionTask(ctx, "bot1", "sess1", taskID)
	if err != nil {
		t.Fatalf("WaitForSessionTask returned error: %v", err)
	}
	if snap.Status != TaskCompleted {
		t.Fatalf("status = %s, want completed", snap.Status)
	}
	if snap.OutputTail != "ok\n" {
		t.Fatalf("output tail = %q, want ok", snap.OutputTail)
	}
}

func TestSpawnFailurePreservesUnknownExitCode(t *testing.T) {
	mgr := New(nil)
	taskID, _ := mgr.Spawn(
		context.Background(),
		"bot1",
		"sess1",
		"broken",
		"/data",
		"Broken command",
		func(context.Context, string, string, int32) (*bridge.ExecResult, error) {
			return nil, errors.New("stream broke before exit")
		},
		nil,
		nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	snap, err := mgr.WaitForSessionTask(ctx, "bot1", "sess1", taskID)
	if err != nil {
		t.Fatalf("WaitForSessionTask returned error: %v", err)
	}
	if snap.Status != TaskFailed {
		t.Fatalf("status = %s, want failed", snap.Status)
	}
	if snap.ExitCode != -1 {
		t.Fatalf("exit code = %d, want -1", snap.ExitCode)
	}
}

func TestKillWakesWaiter(t *testing.T) {
	mgr := New(nil)
	taskID, _ := mgr.Spawn(
		context.Background(),
		"bot1",
		"sess1",
		"sleep 30",
		"/data",
		"Sleep",
		func(ctx context.Context, _, _ string, _ int32) (*bridge.ExecResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		nil,
		nil,
	)

	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan TaskSnapshot, 1)
	errCh := make(chan error, 1)
	go func() {
		snap, err := mgr.WaitForSessionTask(waitCtx, "bot1", "sess1", taskID)
		if err != nil {
			errCh <- err
			return
		}
		done <- snap
	}()

	if err := mgr.KillForSession("bot1", "sess1", taskID); err != nil {
		t.Fatalf("KillForSession returned error: %v", err)
	}
	select {
	case err := <-errCh:
		t.Fatalf("WaitForSessionTask returned error: %v", err)
	case snap := <-done:
		if snap.Status != TaskKilled {
			t.Fatalf("status = %s, want killed", snap.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for waiter to wake")
	}
}

func TestWaitForSessionTaskReturnsStalled(t *testing.T) {
	mgr := New(nil)
	taskID, _ := mgr.Spawn(
		context.Background(),
		"bot1",
		"sess1",
		"read -p password",
		"/data",
		"Prompt",
		func(ctx context.Context, _, _ string, _ int32) (*bridge.ExecResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		nil,
		nil,
	)
	task := mgr.Get(taskID)
	if task == nil {
		t.Fatal("task not found")
	}
	if !task.MarkStalled() {
		t.Fatal("expected MarkStalled to flip state")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	snap, err := mgr.WaitForSessionTask(ctx, "bot1", "sess1", taskID)
	if err != nil {
		t.Fatalf("WaitForSessionTask returned error: %v", err)
	}
	if !snap.Stalled || snap.Status != TaskRunning {
		t.Fatalf("snapshot = %+v, want running stalled task", snap)
	}
	_ = mgr.Kill(taskID)
}

func TestRunningTasksSummaryMentionsWaitTools(t *testing.T) {
	mgr := New(nil)
	taskID, _ := mgr.Spawn(
		context.Background(),
		"bot1",
		"sess1",
		"npm test",
		"/data",
		"Run tests",
		func(ctx context.Context, _, _ string, _ int32) (*bridge.ExecResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		nil,
		nil,
	)
	summary := mgr.RunningTasksSummary("bot1", "sess1")
	for _, want := range []string{taskID, "Run tests", "wait_until(task_id)", "get_background_status(task_id)"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
	_ = mgr.Kill(taskID)
}
