package tools

import (
	"context"
	"testing"

	"github.com/memohai/memoh/internal/agent/background"
)

func TestBgStatusListsAndInspectsSpawnTasks(t *testing.T) {
	mgr := background.New(nil)
	p := NewContainerProvider(nil, nil, mgr, "")
	session := SessionContext{BotID: "bot1", SessionID: "sess1"}

	taskID, _, err := mgr.StartSpawnTask(context.Background(), "bot1", "sess1", "spawn 2 task(s): alpha | beta")
	if err != nil {
		t.Fatalf("StartSpawnTask failed: %v", err)
	}

	listRes, err := p.execBgStatus(context.Background(), session, map[string]any{"action": "list"})
	if err != nil {
		t.Fatalf("background.status list failed: %v", err)
	}
	entries := listRes.(map[string]any)["tasks"].([]map[string]any)
	if len(entries) != 1 {
		t.Fatalf("expected 1 task entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry["kind"] != "spawn" {
		t.Errorf("expected kind spawn in list entry, got %v", entry["kind"])
	}
	if entry["description"] != "spawn 2 task(s): alpha | beta" {
		t.Errorf("unexpected description: %v", entry["description"])
	}
	if _, ok := entry["command"]; ok {
		t.Error("spawn list entry should not carry an exec command field")
	}
	if _, ok := entry["output_file"]; ok {
		t.Error("spawn list entry should not carry an exec output_file field")
	}

	mgr.CompleteSpawnTask(taskID, []background.SpawnBranch{
		{Task: "alpha", ChildSessionID: "child-a", Status: background.TaskCompleted, Report: "found A"},
		{Task: "beta", Status: background.TaskFailed, Error: "boom"},
	})

	statusRes, err := p.execBgStatus(context.Background(), session, map[string]any{"action": "status", "task_id": taskID})
	if err != nil {
		t.Fatalf("background.status status failed: %v", err)
	}
	sm := statusRes.(map[string]any)
	if sm["kind"] != "spawn" || sm["status"] != "failed" {
		t.Errorf("unexpected spawn status payload: %v", sm)
	}
	if _, ok := sm["exit_code"]; ok {
		t.Error("spawn status should not carry an exec exit_code field")
	}
	branches, ok := sm["branches"].([]map[string]any)
	if !ok || len(branches) != 2 {
		t.Fatalf("expected 2 branches in spawn status, got %v", sm["branches"])
	}
	if branches[0]["session_id"] != "child-a" || branches[0]["report"] != "found A" {
		t.Errorf("unexpected first branch payload: %v", branches[0])
	}
	if branches[1]["error"] != "boom" {
		t.Errorf("unexpected second branch payload: %v", branches[1])
	}

	mgr.DrainNotifications("bot1", "sess1")
}
