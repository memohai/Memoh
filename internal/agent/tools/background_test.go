package tools

import (
	"context"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/agent/background"
)

func TestBackgroundProviderWaitAndInspectAgentResult(t *testing.T) {
	mgr := background.New(nil)
	p := NewBackgroundProvider(nil, mgr)
	session := SessionContext{BotID: "bot1", SessionID: "sess1"}

	taskID, _, err := mgr.StartAgentTask(context.Background(), "bot1", "sess1", "worker", "child-1", "input task", "worker: input task", false)
	if err != nil {
		t.Fatalf("StartAgentTask failed: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(10 * time.Millisecond)
		mgr.CompleteAgentTask(taskID, background.AgentTaskResult{
			AgentID:        "worker",
			AgentSessionID: "child-1",
			Message:        "input task",
			Status:         background.TaskCompleted,
			Report:         "final assistant text",
		})
	}()

	waitRes, err := p.execWaitUntil(context.Background(), session, map[string]any{"task_id": taskID})
	if err != nil {
		t.Fatalf("wait_until failed: %v", err)
	}
	if waitRes.(map[string]any)["status"] != "completed" {
		t.Fatalf("wait_until payload = %v, want completed", waitRes)
	}
	<-done

	statusRes, err := p.execGetBackgroundStatus(context.Background(), session, map[string]any{"task_id": taskID})
	if err != nil {
		t.Fatalf("get_background_status failed: %v", err)
	}
	sm := statusRes.(map[string]any)
	if sm["input"] != "input task" {
		t.Fatalf("input = %v, want original task", sm["input"])
	}
	if sm["result"] != "final assistant text" {
		t.Fatalf("result = %v, want final assistant text", sm["result"])
	}
	if _, ok := sm["message"]; ok {
		t.Fatalf("status payload should not use message for agent result: %v", sm)
	}
}

func TestBackgroundProviderSpawnResultShape(t *testing.T) {
	mgr := background.New(nil)
	p := NewBackgroundProvider(nil, mgr)
	session := SessionContext{BotID: "bot1", SessionID: "sess1"}

	taskID, _, err := mgr.StartSpawnTask(context.Background(), "bot1", "sess1", "spawn 2 task(s): alpha | beta")
	if err != nil {
		t.Fatalf("StartSpawnTask failed: %v", err)
	}
	mgr.CompleteSpawnTask(taskID, []background.SpawnBranch{
		{Task: "alpha", ChildSessionID: "child-a", Status: background.TaskCompleted, Report: "found A"},
		{Task: "beta", Status: background.TaskFailed, Error: "boom"},
	})

	statusRes, err := p.execGetBackgroundStatus(context.Background(), session, map[string]any{"task_id": taskID})
	if err != nil {
		t.Fatalf("get_background_status failed: %v", err)
	}
	sm := statusRes.(map[string]any)
	if sm["kind"] != "spawn" || sm["status"] != "failed" {
		t.Errorf("unexpected spawn status payload: %v", sm)
	}
	result := sm["result"].(map[string]any)
	branches := result["branches"].([]map[string]any)
	if len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(branches))
	}
	if branches[0]["session_id"] != "child-a" || branches[0]["result"] != "found A" {
		t.Errorf("unexpected first branch payload: %v", branches[0])
	}
	if branches[1]["error"] != "boom" {
		t.Errorf("unexpected second branch payload: %v", branches[1])
	}
	if _, ok := branches[0]["report"]; ok {
		t.Errorf("branch payload should use result instead of report: %v", branches[0])
	}
}

func TestBackgroundProviderListKillAndWait(t *testing.T) {
	mgr := background.New(nil)
	p := NewBackgroundProvider(nil, mgr)
	session := SessionContext{BotID: "bot1", SessionID: "sess1"}

	taskID, _, err := mgr.StartSpawnTask(context.Background(), "bot1", "sess1", "long spawn")
	if err != nil {
		t.Fatalf("StartSpawnTask failed: %v", err)
	}
	listRes, err := p.execListBackground(context.Background(), session, nil)
	if err != nil {
		t.Fatalf("list_background failed: %v", err)
	}
	entries := listRes.(map[string]any)["tasks"].([]map[string]any)
	if len(entries) != 1 || entries[0]["task_id"] != taskID {
		t.Fatalf("unexpected list payload: %v", listRes)
	}

	if _, err := p.execKillBackground(context.Background(), session, map[string]any{"task_id": taskID}); err != nil {
		t.Fatalf("kill_background failed: %v", err)
	}
	waitRes, err := p.execWaitUntil(context.Background(), session, map[string]any{"task_id": taskID})
	if err != nil {
		t.Fatalf("wait_until failed: %v", err)
	}
	if waitRes.(map[string]any)["status"] != "killed" {
		t.Fatalf("wait_until payload = %v, want killed", waitRes)
	}
	if _, err := p.execWait(context.Background(), session, map[string]any{"duration": 0.001}); err != nil {
		t.Fatalf("wait failed: %v", err)
	}
}
