package background

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

func TestStartSpawnTaskRegistersRunningSpawnTask(t *testing.T) {
	mgr := New(nil)
	var events []TaskEvent
	mgr.SetEventFunc(func(e TaskEvent) { events = append(events, e) })

	taskID, taskCtx, err := mgr.StartSpawnTask(context.Background(), "bot1", "sess1", "spawn: 2 tasks")
	if err != nil {
		t.Fatalf("StartSpawnTask failed: %v", err)
	}
	if taskID == "" {
		t.Fatal("expected non-empty task ID")
	}
	select {
	case <-taskCtx.Done():
		t.Fatal("expected task context to be alive")
	default:
	}

	task := mgr.GetForSession("bot1", "sess1", taskID)
	if task == nil {
		t.Fatal("expected task to be registered for the owning session")
	}
	snap := task.Snapshot()
	if snap.Kind != KindSpawn {
		t.Errorf("expected kind %s, got %q", KindSpawn, snap.Kind)
	}
	if snap.Status != TaskRunning {
		t.Errorf("expected status running, got %s", snap.Status)
	}
	if snap.Description != "spawn: 2 tasks" {
		t.Errorf("unexpected description: %q", snap.Description)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 task event, got %d", len(events))
	}
	if events[0].Event != TaskEventStarted || events[0].Kind != KindSpawn || events[0].TaskID != taskID {
		t.Errorf("unexpected started event: %+v", events[0])
	}
}

func TestCompleteSpawnTaskNotifiesJoinRecord(t *testing.T) {
	mgr := New(nil)
	var events []TaskEvent
	mgr.SetEventFunc(func(e TaskEvent) { events = append(events, e) })

	taskID, _, err := mgr.StartSpawnTask(context.Background(), "bot1", "sess1", "spawn: 2 tasks")
	if err != nil {
		t.Fatalf("StartSpawnTask failed: %v", err)
	}

	mgr.CompleteSpawnTask(taskID, []SpawnBranch{
		{Task: "research A", ChildSessionID: "child-a", Status: TaskCompleted, Report: "found A"},
		{Task: "research B", ChildSessionID: "child-b", Status: TaskCompleted, Report: "found B"},
	})

	notifications := waitDrain(t, mgr, "bot1", "sess1", 1)
	n := notifications[0]
	if n.TaskID != taskID {
		t.Errorf("expected task ID %s, got %s", taskID, n.TaskID)
	}
	if n.Kind != KindSpawn {
		t.Errorf("expected kind spawn, got %q", n.Kind)
	}
	if n.Status != TaskCompleted {
		t.Errorf("expected status completed, got %s", n.Status)
	}
	if len(n.Branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(n.Branches))
	}
	if n.Branches[0].ChildSessionID != "child-a" || n.Branches[0].Report != "found A" {
		t.Errorf("unexpected first branch: %+v", n.Branches[0])
	}

	task := mgr.Get(taskID)
	if task.Status != TaskCompleted {
		t.Errorf("expected task status completed, got %s", task.Status)
	}
	last := events[len(events)-1]
	if last.Event != TaskEventCompleted || last.Kind != KindSpawn {
		t.Errorf("unexpected final event: %+v", last)
	}
}

func TestCompleteSpawnTaskAnyBranchFailureMarksFailed(t *testing.T) {
	mgr := New(nil)

	taskID, _, err := mgr.StartSpawnTask(context.Background(), "bot1", "sess1", "spawn: 2 tasks")
	if err != nil {
		t.Fatalf("StartSpawnTask failed: %v", err)
	}

	mgr.CompleteSpawnTask(taskID, []SpawnBranch{
		{Task: "ok", ChildSessionID: "child-a", Status: TaskCompleted, Report: "fine"},
		{Task: "boom", Status: TaskFailed, Error: "all attempts failed"},
	})

	notifications := waitDrain(t, mgr, "bot1", "sess1", 1)
	if notifications[0].Status != TaskFailed {
		t.Errorf("expected status failed, got %s", notifications[0].Status)
	}
	if notifications[0].Branches[1].Error != "all attempts failed" {
		t.Errorf("expected branch error to be preserved, got %+v", notifications[0].Branches[1])
	}
}

func TestKillSpawnTaskCancelsContextAndSuppressesNotification(t *testing.T) {
	mgr := New(nil)

	taskID, taskCtx, err := mgr.StartSpawnTask(context.Background(), "bot1", "sess1", "spawn: 1 task")
	if err != nil {
		t.Fatalf("StartSpawnTask failed: %v", err)
	}

	if err := mgr.Kill(taskID); err != nil {
		t.Fatalf("kill failed: %v", err)
	}
	select {
	case <-taskCtx.Done():
	default:
		t.Fatal("expected task context to be canceled by Kill")
	}

	mgr.CompleteSpawnTask(taskID, []SpawnBranch{
		{Task: "interrupted", Status: TaskFailed, Error: "parent cancelled"},
	})

	task := mgr.Get(taskID)
	if task.Status != TaskKilled {
		t.Errorf("expected status killed, got %s", task.Status)
	}
	if got := len(task.Snapshot().Branches); got != 1 {
		t.Errorf("expected branch outcomes recorded on killed task, got %d", got)
	}
	if notifications := mgr.DrainNotifications("bot1", "sess1"); len(notifications) != 0 {
		t.Errorf("expected no notifications for killed task, got %d", len(notifications))
	}
}

func TestStartSpawnTaskEnforcesRunningCap(t *testing.T) {
	mgr := New(nil)

	ids := make([]string, 0, MaxRunningSpawnTasks)
	for range MaxRunningSpawnTasks {
		id, _, err := mgr.StartSpawnTask(context.Background(), "bot1", "sess1", "spawn batch")
		if err != nil {
			t.Fatalf("StartSpawnTask under cap failed: %v", err)
		}
		ids = append(ids, id)
	}

	if _, _, err := mgr.StartSpawnTask(context.Background(), "bot1", "sess1", "over cap"); err == nil {
		t.Fatal("expected error when running spawn cap is reached")
	}
	if _, _, err := mgr.StartSpawnTask(context.Background(), "bot1", "sess2", "other session"); err != nil {
		t.Fatalf("expected other session to be unaffected by cap: %v", err)
	}

	mgr.CompleteSpawnTask(ids[0], nil)
	if _, _, err := mgr.StartSpawnTask(context.Background(), "bot1", "sess1", "after slot freed"); err != nil {
		t.Fatalf("expected start to succeed after a slot freed: %v", err)
	}
}

func TestSpawnNotificationFormat(t *testing.T) {
	n := Notification{
		TaskID:      "bg_test_3",
		Kind:        KindSpawn,
		Status:      TaskCompleted,
		Description: "spawn: 2 tasks",
		Branches: []SpawnBranch{
			{Task: "research A", ChildSessionID: "child-a", Status: TaskCompleted, Report: "found A"},
			{Task: "research B", ChildSessionID: "child-b", Status: TaskFailed, Error: "watchdog timed out"},
		},
		Duration: 90 * time.Second,
	}

	text := n.FormatForAgent()
	for _, want := range []string{
		"<task-notification>",
		"<kind>spawn</kind>",
		"bg_test_3",
		"<status>completed</status>",
		"spawn: 2 tasks",
		"child-a",
		"research A",
		"found A",
		"child-b",
		"watchdog timed out",
		"search_messages",
		"</task-notification>",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("spawn notification missing %q:\n%s", want, text)
		}
	}
	for _, reject := range []string{"exit-code", "output-file", "<command>"} {
		if strings.Contains(text, reject) {
			t.Errorf("spawn notification should not contain %q:\n%s", reject, text)
		}
	}
	if !strings.HasPrefix(n.MessageText(), "A background task completed:") {
		t.Errorf("unexpected message lead: %q", n.MessageText())
	}

	// Exec notifications keep their existing format, without a kind tag.
	execN := Notification{TaskID: "bg_test_4", Kind: KindExec, Status: TaskCompleted, Command: "npm install"}
	if strings.Contains(execN.FormatForAgent(), "<kind>") {
		t.Errorf("exec notification format must stay unchanged:\n%s", execN.FormatForAgent())
	}
}

func TestCompleteSpawnTaskClampsBranchReports(t *testing.T) {
	mgr := New(nil)

	taskID, _, err := mgr.StartSpawnTask(context.Background(), "bot1", "sess1", "spawn: 1 task")
	if err != nil {
		t.Fatalf("StartSpawnTask failed: %v", err)
	}

	longReport := strings.Repeat("x", spawnReportMaxBytes) + "FINDINGS-TAIL"
	longTask := "TASK-HEAD" + strings.Repeat("y", spawnBranchTaskMaxBytes)
	longError := "ERROR-HEAD" + strings.Repeat("z", spawnBranchErrorMaxBytes)
	mgr.CompleteSpawnTask(taskID, []SpawnBranch{
		{Task: longTask, ChildSessionID: "child-a", Status: TaskFailed, Report: longReport, Error: longError},
	})

	n := waitDrain(t, mgr, "bot1", "sess1", 1)[0]
	br := n.Branches[0]
	if got := len(br.Report); got > spawnReportMaxBytes {
		t.Errorf("expected report clamped to %d bytes, got %d", spawnReportMaxBytes, got)
	}
	if !strings.HasSuffix(br.Report, "FINDINGS-TAIL") {
		t.Error("expected clamping to keep the report tail")
	}
	if got := len(br.Task); got > spawnBranchTaskMaxBytes+len("...") {
		t.Errorf("expected task clamped to ~%d bytes, got %d", spawnBranchTaskMaxBytes, got)
	}
	if !strings.HasPrefix(br.Task, "TASK-HEAD") {
		t.Error("expected clamping to keep the task head")
	}
	if got := len(br.Error); got > spawnBranchErrorMaxBytes+len("...") {
		t.Errorf("expected error clamped to ~%d bytes, got %d", spawnBranchErrorMaxBytes, got)
	}
	if !strings.HasPrefix(br.Error, "ERROR-HEAD") {
		t.Error("expected clamping to keep the error head")
	}
	if snap := mgr.Get(taskID).Snapshot(); len(snap.Branches[0].Report) > spawnReportMaxBytes ||
		len(snap.Branches[0].Task) > spawnBranchTaskMaxBytes+len("...") ||
		len(snap.Branches[0].Error) > spawnBranchErrorMaxBytes+len("...") {
		t.Error("expected recorded branch fields to be clamped as well")
	}
}

func TestRunningTasksSummaryIncludesSpawnTasksWithoutOutputFile(t *testing.T) {
	mgr := New(nil)

	if _, _, err := mgr.StartSpawnTask(context.Background(), "bot1", "sess1", "spawn: 3 research tasks"); err != nil {
		t.Fatalf("StartSpawnTask failed: %v", err)
	}

	summary := mgr.RunningTasksSummary("bot1", "sess1")
	if !strings.Contains(summary, "spawn: 3 research tasks") {
		t.Errorf("summary should mention spawn task description, got: %s", summary)
	}
	if strings.Contains(summary, "output:") {
		t.Errorf("summary should omit output file for spawn tasks, got: %s", summary)
	}
}

func TestExecTasksCarryExecKind(t *testing.T) {
	mgr := New(nil)
	execFn := func(_ context.Context, _, _ string, _ int32) (*bridge.ExecResult, error) {
		return &bridge.ExecResult{Stdout: "ok\n", ExitCode: 0}, nil
	}

	taskID, _ := mgr.Spawn(context.Background(), "bot1", "sess1", "echo hi", "/data", "", execFn, nil, nil)
	if snap := mgr.Get(taskID).Snapshot(); snap.Kind != KindExec {
		t.Errorf("expected exec kind, got %q", snap.Kind)
	}
	_ = waitDrain(t, mgr, "bot1", "sess1", 1)
}
