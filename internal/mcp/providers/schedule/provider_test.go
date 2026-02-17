package schedule

import (
	"context"
	"errors"
	"testing"
	"time"

	mcpgw "github.com/memohai/memoh/internal/mcp"
	sched "github.com/memohai/memoh/internal/schedule"
)

type fakeScheduler struct {
	list      []sched.Schedule
	get       sched.Schedule
	getErr    error
	create    sched.Schedule
	createErr error
	update    sched.Schedule
	updateErr error
	deleteErr error
}

func (f *fakeScheduler) List(_ context.Context, _ string) ([]sched.Schedule, error) {
	return f.list, nil
}

func (f *fakeScheduler) Get(_ context.Context, _ string) (sched.Schedule, error) {
	if f.getErr != nil {
		return sched.Schedule{}, f.getErr
	}
	return f.get, nil
}

func (f *fakeScheduler) Create(_ context.Context, _ string, _ sched.CreateRequest) (sched.Schedule, error) {
	if f.createErr != nil {
		return sched.Schedule{}, f.createErr
	}
	return f.create, nil
}

func (f *fakeScheduler) Update(_ context.Context, _ string, _ sched.UpdateRequest) (sched.Schedule, error) {
	if f.updateErr != nil {
		return sched.Schedule{}, f.updateErr
	}
	return f.update, nil
}

func (f *fakeScheduler) Delete(_ context.Context, _ string) error {
	return f.deleteErr
}

func TestExecutor_ListTools_NilService(t *testing.T) {
	exec := NewExecutor(nil, nil)
	tools, err := exec.ListTools(context.Background(), mcpgw.ToolSessionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools when service nil, got %d", len(tools))
	}
}

func TestExecutor_ListTools(t *testing.T) {
	svc := &fakeScheduler{}
	exec := NewExecutor(nil, svc)
	tools, err := exec.ListTools(context.Background(), mcpgw.ToolSessionContext{})
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{toolScheduleList, toolScheduleGet, toolScheduleCreate, toolScheduleUpdate, toolScheduleDelete}
	if len(tools) != len(wantNames) {
		t.Fatalf("expected %d tools, got %d", len(wantNames), len(tools))
	}
	for i, name := range wantNames {
		if tools[i].Name != name {
			t.Errorf("tools[%d].Name = %q, want %q", i, tools[i].Name, name)
		}
	}
}

func TestExecutor_CallTool_NotFound(t *testing.T) {
	svc := &fakeScheduler{}
	exec := NewExecutor(nil, svc)
	_, err := exec.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: "bot1"}, "other_tool", nil)
	if !errors.Is(err, mcpgw.ErrToolNotFound) {
		t.Errorf("expected ErrToolNotFound, got %v", err)
	}
}

func TestExecutor_CallTool_NilService(t *testing.T) {
	exec := NewExecutor(nil, nil)
	result, err := exec.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: "bot1"}, toolScheduleList, nil)
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when service nil")
	}
}

func TestExecutor_CallTool_NoBotID(t *testing.T) {
	svc := &fakeScheduler{}
	exec := NewExecutor(nil, svc)
	result, err := exec.CallTool(context.Background(), mcpgw.ToolSessionContext{}, toolScheduleList, nil)
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when bot_id is missing")
	}
}

func TestExecutor_CallTool_List(t *testing.T) {
	svc := &fakeScheduler{
		list: []sched.Schedule{
			{ID: "id1", Name: "n1", BotID: "bot1"},
		},
	}
	exec := NewExecutor(nil, svc)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolScheduleList, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpgw.PayloadError(result); err != nil {
		t.Fatal(err)
	}
	content, _ := result["structuredContent"].(map[string]any)
	if content == nil {
		t.Fatal("no structuredContent")
	}
	items, _ := content["items"].([]sched.Schedule)
	if len(items) != 1 {
		t.Errorf("items length = %d", len(items))
	}
}

func TestExecutor_CallTool_Get_IdRequired(t *testing.T) {
	svc := &fakeScheduler{}
	exec := NewExecutor(nil, svc)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolScheduleGet, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when id is missing")
	}
}

func TestExecutor_CallTool_Get_BotMismatch(t *testing.T) {
	svc := &fakeScheduler{
		get: sched.Schedule{ID: "s1", BotID: "other-bot"},
	}
	exec := NewExecutor(nil, svc)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolScheduleGet, map[string]any{"id": "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when bot mismatch")
	}
}

func TestExecutor_CallTool_Get_Success(t *testing.T) {
	svc := &fakeScheduler{
		get: sched.Schedule{ID: "s1", Name: "job1", BotID: "bot1"},
	}
	exec := NewExecutor(nil, svc)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolScheduleGet, map[string]any{"id": "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpgw.PayloadError(result); err != nil {
		t.Fatal(err)
	}
	item, ok := result["structuredContent"].(sched.Schedule)
	if !ok {
		t.Fatal("structuredContent is not Schedule")
	}
	if item.ID != "s1" {
		t.Errorf("id = %v", item.ID)
	}
}

func TestExecutor_CallTool_Create_RequiredFields(t *testing.T) {
	svc := &fakeScheduler{}
	exec := NewExecutor(nil, svc)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolScheduleCreate, map[string]any{
		"name": "n", "description": "d", "pattern": "* * * * *",
	})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when command is missing")
	}
}

func TestExecutor_CallTool_Create_Success(t *testing.T) {
	svc := &fakeScheduler{
		create: sched.Schedule{
			ID: "new1", Name: "n1", Description: "d1", Pattern: "* * * * *", Command: "echo",
			BotID: "bot1", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		},
	}
	exec := NewExecutor(nil, svc)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolScheduleCreate, map[string]any{
		"name": "n1", "description": "d1", "pattern": "* * * * *", "command": "echo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpgw.PayloadError(result); err != nil {
		t.Fatal(err)
	}
	item, ok := result["structuredContent"].(sched.Schedule)
	if !ok {
		t.Fatal("structuredContent is not Schedule")
	}
	if item.ID != "new1" {
		t.Errorf("id = %v", item.ID)
	}
}

func TestExecutor_CallTool_Update_IdRequired(t *testing.T) {
	svc := &fakeScheduler{}
	exec := NewExecutor(nil, svc)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolScheduleUpdate, map[string]any{"name": "n"})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when id is missing")
	}
}

func TestExecutor_CallTool_Update_Success(t *testing.T) {
	svc := &fakeScheduler{
		update: sched.Schedule{ID: "s1", Name: "updated", BotID: "bot1"},
	}
	exec := NewExecutor(nil, svc)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolScheduleUpdate, map[string]any{
		"id": "s1", "name": "updated",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpgw.PayloadError(result); err != nil {
		t.Fatal(err)
	}
}

func TestExecutor_CallTool_Delete_IdRequired(t *testing.T) {
	svc := &fakeScheduler{}
	exec := NewExecutor(nil, svc)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolScheduleDelete, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when id is missing")
	}
}

func TestExecutor_CallTool_Delete_BotMismatch(t *testing.T) {
	svc := &fakeScheduler{
		get: sched.Schedule{ID: "s1", BotID: "other-bot"},
	}
	exec := NewExecutor(nil, svc)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolScheduleDelete, map[string]any{"id": "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when bot mismatch on delete")
	}
}

func TestExecutor_CallTool_Delete_Success(t *testing.T) {
	svc := &fakeScheduler{
		get: sched.Schedule{ID: "s1", BotID: "bot1"},
	}
	exec := NewExecutor(nil, svc)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolScheduleDelete, map[string]any{"id": "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpgw.PayloadError(result); err != nil {
		t.Fatal(err)
	}
	content, _ := result["structuredContent"].(map[string]any)
	if content == nil {
		t.Fatal("no structuredContent")
	}
	if success, _ := content["success"].(bool); !success {
		t.Errorf("success = %v", content["success"])
	}
}

func TestExecutor_CallTool_Get_ServiceError(t *testing.T) {
	svc := &fakeScheduler{getErr: errors.New("not found")}
	exec := NewExecutor(nil, svc)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolScheduleGet, map[string]any{"id": "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when Get fails")
	}
}

func TestParseNullableIntArg(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		key     string
		wantSet bool
		wantVal *int
		wantErr bool
	}{
		{"nil args", nil, "x", false, nil, false},
		{"missing key", map[string]any{}, "x", false, nil, false},
		{"null value", map[string]any{"x": nil}, "x", true, nil, false},
		{"int value", map[string]any{"x": 5}, "x", true, intPtr(5), false},
		{"invalid type", map[string]any{"x": "bad"}, "x", false, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNullableIntArg(tt.args, tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseNullableIntArg() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.Set != tt.wantSet {
				t.Errorf("Set = %v, want %v", got.Set, tt.wantSet)
			}
			if tt.wantVal == nil {
				if got.Value != nil {
					t.Errorf("Value = %v, want nil", got.Value)
				}
			} else {
				if got.Value == nil || *got.Value != *tt.wantVal {
					t.Errorf("Value = %v, want %v", got.Value, tt.wantVal)
				}
			}
		})
	}
}

func TestEmptyObjectSchema(t *testing.T) {
	m := emptyObjectSchema()
	if m["type"] != "object" {
		t.Errorf("type = %v", m["type"])
	}
	if m["properties"] == nil {
		t.Error("properties should be non-nil")
	}
}

func intPtr(n int) *int {
	return &n
}
