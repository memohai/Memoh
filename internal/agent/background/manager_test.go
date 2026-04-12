package background

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

// waitDrain polls DrainNotifications until the expected count is reached or timeout.
func waitDrain(t *testing.T, mgr *Manager, botID, sessionID string, wantCount int) []Notification {
	t.Helper()
	deadline := time.After(5 * time.Second)
	var all []Notification
	for {
		all = append(all, mgr.DrainNotifications(botID, sessionID)...)
		if len(all) >= wantCount {
			return all
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d notifications, got %d", wantCount, len(all))
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestSpawnAndNotify(t *testing.T) {
	mgr := New(nil)

	called := make(chan struct{})
	execFn := func(_ context.Context, _, _ string, _ int32) (*bridge.ExecResult, error) {
		close(called)
		return &bridge.ExecResult{Stdout: "hello world\n", ExitCode: 0}, nil
	}

	taskID, outputFile := mgr.Spawn(context.Background(), "bot1", "sess1", "echo hello", "/data", "test echo", execFn, nil, nil)

	if taskID == "" {
		t.Fatal("expected non-empty task ID")
	}
	if outputFile == "" {
		t.Fatal("expected non-empty output file")
	}

	// Wait for exec to be called.
	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("execFn was not called within timeout")
	}

	// Wait for notification.
	notifications := waitDrain(t, mgr, "bot1", "sess1", 1)
	n := notifications[0]
	if n.TaskID != taskID {
		t.Errorf("expected task ID %s, got %s", taskID, n.TaskID)
	}
	if n.Status != TaskCompleted {
		t.Errorf("expected status completed, got %s", n.Status)
	}
	if n.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", n.ExitCode)
	}
	if n.BotID != "bot1" || n.SessionID != "sess1" {
		t.Errorf("unexpected bot/session: %s/%s", n.BotID, n.SessionID)
	}

	// Verify task state.
	task := mgr.Get(taskID)
	if task == nil {
		t.Fatal("task not found after completion")
	}
	if task.Status != TaskCompleted {
		t.Errorf("expected task status completed, got %s", task.Status)
	}
}

func TestSpawnFailedCommand(t *testing.T) {
	mgr := New(nil)

	execFn := func(_ context.Context, _, _ string, _ int32) (*bridge.ExecResult, error) {
		return &bridge.ExecResult{
			Stdout:   "some output\n",
			Stderr:   "error: not found\n",
			ExitCode: 1,
		}, nil
	}

	taskID, _ := mgr.Spawn(context.Background(), "bot1", "sess1", "false", "/data", "failing cmd", execFn, nil, nil)

	notifications := waitDrain(t, mgr, "bot1", "sess1", 1)
	n := notifications[0]
	if n.TaskID != taskID {
		t.Errorf("expected task ID %s, got %s", taskID, n.TaskID)
	}
	if n.Status != TaskFailed {
		t.Errorf("expected status failed, got %s", n.Status)
	}
	if n.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", n.ExitCode)
	}
}

func TestKillTask(t *testing.T) {
	mgr := New(nil)

	started := make(chan struct{})
	execFn := func(ctx context.Context, _, _ string, _ int32) (*bridge.ExecResult, error) {
		close(started)
		<-ctx.Done()
		return &bridge.ExecResult{ExitCode: -1}, ctx.Err()
	}

	taskID, _ := mgr.Spawn(context.Background(), "bot1", "sess1", "sleep 300", "/data", "long task", execFn, nil, nil)

	// Wait for the task to start.
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("task did not start within timeout")
	}

	if err := mgr.Kill(taskID); err != nil {
		t.Fatalf("kill failed: %v", err)
	}

	task := mgr.Get(taskID)
	if task == nil {
		t.Fatal("task not found")
	}
	if task.Status != TaskKilled {
		t.Errorf("expected status killed, got %s", task.Status)
	}

	// Killed tasks should not produce notifications.
	time.Sleep(50 * time.Millisecond) // give goroutine time to finish
	notifications := mgr.DrainNotifications("bot1", "sess1")
	if len(notifications) != 0 {
		t.Errorf("expected no notifications for killed task, got %d", len(notifications))
	}
}

func TestGetForSession(t *testing.T) {
	mgr := New(nil)

	taskID, _ := mgr.Spawn(context.Background(), "bot1", "sess1", "echo hello", "/data", "", func(_ context.Context, _, _ string, _ int32) (*bridge.ExecResult, error) {
		return &bridge.ExecResult{Stdout: "hello\n", ExitCode: 0}, nil
	}, nil, nil)

	if task := mgr.GetForSession("bot1", "sess1", taskID); task == nil {
		t.Fatal("expected task to be visible within the owning session")
	}
	if task := mgr.GetForSession("bot1", "sess2", taskID); task != nil {
		t.Fatal("expected task to be hidden from other sessions")
	}
}

func TestKillForSession(t *testing.T) {
	mgr := New(nil)

	started := make(chan struct{})
	execFn := func(ctx context.Context, _, _ string, _ int32) (*bridge.ExecResult, error) {
		close(started)
		<-ctx.Done()
		return &bridge.ExecResult{ExitCode: -1}, ctx.Err()
	}

	taskID, _ := mgr.Spawn(context.Background(), "bot1", "sess1", "sleep 300", "/data", "long task", execFn, nil, nil)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("task did not start within timeout")
	}

	if err := mgr.KillForSession("bot1", "sess2", taskID); err == nil {
		t.Fatal("expected kill from another session to fail")
	}
	if err := mgr.KillForSession("bot1", "sess1", taskID); err != nil {
		t.Fatalf("expected kill from owning session to succeed: %v", err)
	}
}

func TestListForSession(t *testing.T) {
	mgr := New(nil)

	started := make(chan struct{}, 2)
	execFn := func(ctx context.Context, _, _ string, _ int32) (*bridge.ExecResult, error) {
		started <- struct{}{}
		<-ctx.Done()
		return &bridge.ExecResult{ExitCode: -1}, ctx.Err()
	}

	mgr.Spawn(context.Background(), "bot1", "sess1", "cmd1", "/data", "d1", execFn, nil, nil)
	mgr.Spawn(context.Background(), "bot1", "sess1", "cmd2", "/data", "d2", execFn, nil, nil)
	mgr.Spawn(context.Background(), "bot2", "sess2", "cmd3", "/data", "d3", execFn, nil, nil)

	// Wait for all to start.
	for range 3 {
		<-started
	}

	tasks := mgr.ListForSession("bot1", "sess1")
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks for bot1/sess1, got %d", len(tasks))
	}

	tasks = mgr.ListForSession("bot2", "sess2")
	if len(tasks) != 1 {
		t.Errorf("expected 1 task for bot2/sess2, got %d", len(tasks))
	}
}

func TestDrainNotifications(t *testing.T) {
	mgr := New(nil)

	done := make(chan struct{}, 3)
	execFn := func(_ context.Context, _, _ string, _ int32) (*bridge.ExecResult, error) {
		defer func() { done <- struct{}{} }()
		return &bridge.ExecResult{Stdout: "ok\n", ExitCode: 0}, nil
	}

	mgr.Spawn(context.Background(), "bot1", "sess1", "echo 1", "/data", "", execFn, nil, nil)
	mgr.Spawn(context.Background(), "bot1", "sess2", "echo 2", "/data", "", execFn, nil, nil)
	mgr.Spawn(context.Background(), "bot2", "sess1", "echo 3", "/data", "", execFn, nil, nil)

	// Wait for all tasks to complete.
	for range 3 {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("task did not complete within timeout")
		}
	}

	// Drain only bot1/sess1.
	notifications := waitDrain(t, mgr, "bot1", "sess1", 1)
	if len(notifications) != 1 {
		t.Errorf("expected 1 notification for bot1/sess1, got %d", len(notifications))
	}

	// The other two should still be pending.
	notifications = waitDrain(t, mgr, "bot1", "sess2", 1)
	if len(notifications) != 1 {
		t.Errorf("expected 1 notification for bot1/sess2, got %d", len(notifications))
	}
}

func TestMarkNotifiedPreventsDoubleNotification(t *testing.T) {
	mgr := New(nil)

	// Simulate two goroutines racing to complete/notify.
	execFn := func(_ context.Context, _, _ string, _ int32) (*bridge.ExecResult, error) {
		return &bridge.ExecResult{Stdout: "ok\n", ExitCode: 0}, nil
	}

	taskID, _ := mgr.Spawn(context.Background(), "bot1", "sess1", "echo hi", "/data", "", execFn, nil, nil)
	notifications := waitDrain(t, mgr, "bot1", "sess1", 1)
	if len(notifications) != 1 {
		t.Fatalf("expected exactly 1 notification, got %d", len(notifications))
	}
	if notifications[0].TaskID != taskID {
		t.Errorf("unexpected task ID: %s", notifications[0].TaskID)
	}

	// Calling MarkNotified again should return false (already notified).
	task := mgr.Get(taskID)
	if task.MarkNotified() {
		t.Error("MarkNotified should return false on second call")
	}

	// No additional notifications should appear.
	extra := mgr.DrainNotifications("bot1", "sess1")
	if len(extra) != 0 {
		t.Errorf("expected no extra notifications, got %d", len(extra))
	}
}

func TestStalledNotificationFormat(t *testing.T) {
	n := Notification{
		TaskID:     "bg_test_2",
		Status:     TaskRunning,
		Command:    "apt install -q foo",
		OutputFile: "/tmp/memoh-bg/bg_test_2.log",
		OutputTail: "Do you want to continue? [Y/n]",
		Duration:   50 * time.Second,
		Stalled:    true,
	}

	text := n.FormatForAgent()
	for _, want := range []string{
		"<status>stalled</status>",
		"<suggestion>",
		"non-interactive",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("stalled notification missing %q:\n%s", want, text)
		}
	}
	// Stalled notifications should NOT have exit-code.
	if strings.Contains(text, "exit-code") {
		t.Errorf("stalled notification should not contain exit-code:\n%s", text)
	}
}

func TestRunningTasksSummary(t *testing.T) {
	mgr := New(nil)

	started := make(chan struct{})
	execFn := func(ctx context.Context, _, _ string, _ int32) (*bridge.ExecResult, error) {
		close(started)
		<-ctx.Done()
		return &bridge.ExecResult{ExitCode: -1}, ctx.Err()
	}

	mgr.Spawn(context.Background(), "bot1", "sess1", "npm test", "/data", "Run tests", execFn, nil, nil)
	<-started

	summary := mgr.RunningTasksSummary("bot1", "sess1")
	if !strings.Contains(summary, "Run tests") {
		t.Errorf("summary should mention description, got: %s", summary)
	}
	if !strings.Contains(summary, "Currently running background tasks:") {
		t.Errorf("summary should have header, got: %s", summary)
	}

	// No tasks for other session.
	if s := mgr.RunningTasksSummary("bot2", "sess2"); s != "" {
		t.Errorf("expected empty summary for other session, got: %s", s)
	}
}

func TestCleanupRemovesOnlyOldCompletedTasks(t *testing.T) {
	mgr := New(nil)
	now := time.Now()

	mgr.tasks["old_done"] = &Task{
		ID:          "old_done",
		BotID:       "bot1",
		SessionID:   "sess1",
		Status:      TaskCompleted,
		CompletedAt: now.Add(-2 * time.Hour),
	}
	mgr.tasks["recent_done"] = &Task{
		ID:          "recent_done",
		BotID:       "bot1",
		SessionID:   "sess1",
		Status:      TaskCompleted,
		CompletedAt: now.Add(-10 * time.Minute),
	}
	mgr.tasks["running"] = &Task{
		ID:        "running",
		BotID:     "bot1",
		SessionID: "sess1",
		Status:    TaskRunning,
		StartedAt: now.Add(-2 * time.Hour),
	}

	mgr.Cleanup(time.Hour)

	if mgr.Get("old_done") != nil {
		t.Fatal("expected old completed task to be cleaned up")
	}
	if mgr.Get("recent_done") == nil {
		t.Fatal("expected recent completed task to be retained")
	}
	if mgr.Get("running") == nil {
		t.Fatal("expected running task to be retained")
	}
}

func TestSpawnUsesRestartSafeTaskIDs(t *testing.T) {
	mgr1 := New(nil)
	mgr2 := New(nil)

	execFn := func(_ context.Context, _, _ string, _ int32) (*bridge.ExecResult, error) {
		return &bridge.ExecResult{Stdout: "ok\n", ExitCode: 0}, nil
	}

	taskID1, outputFile1 := mgr1.Spawn(context.Background(), "bot123456789", "sess1", "echo one", "/data", "", execFn, nil, nil)
	taskID2, outputFile2 := mgr2.Spawn(context.Background(), "bot123456789", "sess1", "echo two", "/data", "", execFn, nil, nil)

	if taskID1 == taskID2 {
		t.Fatalf("expected distinct task IDs across fresh managers, got %q", taskID1)
	}
	if outputFile1 == outputFile2 {
		t.Fatalf("expected distinct output files across fresh managers, got %q", outputFile1)
	}
	if !strings.HasPrefix(taskID1, "bg_bot12345_") {
		t.Fatalf("unexpected task ID format: %q", taskID1)
	}
	if !strings.HasPrefix(taskID2, "bg_bot12345_") {
		t.Fatalf("unexpected task ID format: %q", taskID2)
	}
}

func TestNotificationFormat(t *testing.T) {
	n := Notification{
		TaskID:      "bg_test_1",
		Status:      TaskCompleted,
		Command:     "npm install",
		Description: "Install dependencies",
		ExitCode:    0,
		OutputFile:  "/tmp/memoh-bg/bg_test_1.log",
		OutputTail:  "added 1337 packages\n",
		Duration:    45 * time.Second,
	}

	text := n.FormatForAgent()
	if text == "" {
		t.Fatal("expected non-empty notification text")
	}
	for _, want := range []string{
		"<task-notification>",
		"bg_test_1",
		"completed",
		"npm install",
		"Install dependencies",
		"/tmp/memoh-bg/bg_test_1.log",
		"added 1337 packages",
		"</task-notification>",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("notification text missing %q:\n%s", want, text)
		}
	}
}
