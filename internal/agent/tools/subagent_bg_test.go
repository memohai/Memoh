package tools

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/background"
	messagepkg "github.com/memohai/memoh/internal/message"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

type fakeSpawnAgent struct {
	block   chan struct{}
	failFor map[string]string

	mu    sync.Mutex
	calls []SpawnRunConfig
}

func (f *fakeSpawnAgent) Generate(ctx context.Context, cfg SpawnRunConfig) (*SpawnResult, error) {
	return f.GenerateWithWatchdog(ctx, cfg, func() {})
}

func (f *fakeSpawnAgent) GenerateWithWatchdog(ctx context.Context, cfg SpawnRunConfig, _ func()) (*SpawnResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, cfg)
	f.mu.Unlock()

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
	return &SpawnResult{
		Text: "report for " + cfg.Query,
		Messages: []sdk.Message{{
			Role:    sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{sdk.TextPart{Text: "report for " + cfg.Query}},
		}},
	}, nil
}

func (f *fakeSpawnAgent) queries() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.calls))
	for _, call := range f.calls {
		out = append(out, call.Query)
	}
	return out
}

func (f *fakeSpawnAgent) callAt(i int) (SpawnRunConfig, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i < 0 || i >= len(f.calls) {
		return SpawnRunConfig{}, false
	}
	return f.calls[i], true
}

type fakeAgentSessionService struct {
	mu       sync.Mutex
	next     int
	sessions []sessionpkg.Session
}

func (s *fakeAgentSessionService) Create(_ context.Context, input sessionpkg.CreateInput) (sessionpkg.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	now := time.Unix(int64(s.next), 0).UTC()
	sess := sessionpkg.Session{
		ID:              "child_" + strconv.Itoa(s.next),
		BotID:           input.BotID,
		Type:            input.Type,
		Title:           input.Title,
		Metadata:        input.Metadata,
		ParentSessionID: input.ParentSessionID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	s.sessions = append(s.sessions, sess)
	return sess, nil
}

func (s *fakeAgentSessionService) ListSubagentsByParent(_ context.Context, parentSessionID string) ([]sessionpkg.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []sessionpkg.Session
	for _, sess := range s.sessions {
		if sess.ParentSessionID == parentSessionID {
			out = append(out, sess)
		}
	}
	return out, nil
}

func (s *fakeAgentSessionService) byAgent(parentSessionID, agentID string) (sessionpkg.Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.sessions {
		if sess.ParentSessionID == parentSessionID && sess.Metadata["agent_id"] == agentID {
			return sess, true
		}
	}
	return sessionpkg.Session{}, false
}

type fakeAgentMessageService struct {
	mu       sync.Mutex
	messages map[string][]messagepkg.Message
}

func newFakeAgentMessageService() *fakeAgentMessageService {
	return &fakeAgentMessageService{messages: make(map[string][]messagepkg.Message)}
}

func (s *fakeAgentMessageService) Persist(_ context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msg := messagepkg.Message{
		ID:        "msg_" + strconv.Itoa(len(s.messages[input.SessionID])+1),
		BotID:     input.BotID,
		SessionID: input.SessionID,
		Role:      input.Role,
		Content:   input.Content,
		Usage:     input.Usage,
		CreatedAt: time.Now().UTC(),
	}
	s.messages[input.SessionID] = append(s.messages[input.SessionID], msg)
	return msg, nil
}

func (s *fakeAgentMessageService) ListBySession(_ context.Context, sessionID string) ([]messagepkg.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]messagepkg.Message(nil), s.messages[sessionID]...), nil
}

func (*fakeAgentMessageService) List(context.Context, string) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*fakeAgentMessageService) ListSince(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*fakeAgentMessageService) ListActiveSince(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*fakeAgentMessageService) ListLatest(context.Context, string, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*fakeAgentMessageService) ListBefore(context.Context, string, time.Time, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*fakeAgentMessageService) ListSinceBySession(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*fakeAgentMessageService) ListActiveSinceBySession(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*fakeAgentMessageService) ListLatestBySession(context.Context, string, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*fakeAgentMessageService) ListBeforeBySession(context.Context, string, time.Time, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*fakeAgentMessageService) LocateByExternalIDBySession(context.Context, string, string, int32, int32) (messagepkg.LocateResult, error) {
	return messagepkg.LocateResult{}, nil
}

func (*fakeAgentMessageService) DeleteByBot(context.Context, string) error {
	return nil
}

func (*fakeAgentMessageService) DeleteBySession(context.Context, string) error {
	return nil
}

func (*fakeAgentMessageService) LinkAssets(context.Context, string, []messagepkg.AssetRef) error {
	return nil
}

func newAgentControlProvider(t *testing.T, agent *fakeSpawnAgent) (*SpawnProvider, *background.Manager, *fakeAgentSessionService, *fakeAgentMessageService) {
	t.Helper()
	mgr := background.New(nil)
	sessionSvc := &fakeAgentSessionService{}
	messageSvc := newFakeAgentMessageService()
	p := NewSpawnProvider(nil, nil, nil, nil, nil, mgr)
	p.sessionService = sessionSvc
	p.SetAgent(agent)
	p.SetMessageService(messageSvc)
	p.modelResolver = func(context.Context, string) (*sdk.Model, string, string, error) {
		return &sdk.Model{}, "model-1", "", nil
	}
	return p, mgr, sessionSvc, messageSvc
}

func executeAgentTool(t *testing.T, p *SpawnProvider, session SessionContext, name string, args map[string]any) (any, error) {
	t.Helper()
	tools, err := p.Tools(context.Background(), session)
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}
	for _, tool := range tools {
		if tool.Name == name {
			return tool.Execute(&sdk.ToolExecContext{Context: context.Background()}, args)
		}
	}
	t.Fatalf("tool %q not found in %v", name, toolNames(tools))
	return nil, nil
}

func toolNames(tools []sdk.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

func waitUntil(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", value)
	}
	return m
}

func TestAgentControlToolsExposeSingleAgentSurface(t *testing.T) {
	p, _, _, _ := newAgentControlProvider(t, &fakeSpawnAgent{})
	session := SessionContext{BotID: "bot1", SessionID: "parent1"}

	tools, err := p.Tools(context.Background(), session)
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}
	got := toolNames(tools)
	want := []string{"spawn_agent", "send_message", "wait_agent", "list_agents"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected tools: got %v want %v", got, want)
	}

	subagentTools, err := p.Tools(context.Background(), SessionContext{BotID: "bot1", SessionID: "child", IsSubagent: true})
	if err != nil {
		t.Fatalf("subagent Tools failed: %v", err)
	}
	if len(subagentTools) != 0 {
		t.Fatalf("subagent should not see agent control tools, got %v", toolNames(subagentTools))
	}
}

func TestAgentControlToolSchemasDoNotReferenceSiblingTools(t *testing.T) {
	p, _, _, _ := newAgentControlProvider(t, &fakeSpawnAgent{})
	tools, err := p.Tools(context.Background(), SessionContext{BotID: "bot1", SessionID: "parent1"})
	if err != nil {
		t.Fatalf("Tools failed: %v", err)
	}
	for _, tool := range tools {
		raw, err := json.Marshal(tool.Parameters)
		if err != nil {
			t.Fatalf("marshal %s schema: %v", tool.Name, err)
		}
		schema := string(raw)
		switch tool.Name {
		case ToolSendMessage.String():
			for _, absent := range []string{ToolSpawnAgent.String(), ToolListAgents.String()} {
				if strings.Contains(schema, absent) {
					t.Fatalf("%s schema references sibling tool %s:\n%s", tool.Name, absent, schema)
				}
			}
		case ToolWaitAgent.String():
			for _, absent := range []string{ToolSpawnAgent.String(), ToolSendMessage.String()} {
				if strings.Contains(schema, absent) {
					t.Fatalf("%s schema references sibling tool %s:\n%s", tool.Name, absent, schema)
				}
			}
		}
	}
}

func TestSpawnAgentIDsAndDuplicateValidation(t *testing.T) {
	p, _, _, _ := newAgentControlProvider(t, &fakeSpawnAgent{})
	session := SessionContext{BotID: "bot1", SessionID: "parent1"}

	res := asMap(t, mustExecuteAgentTool(t, p, session, "spawn_agent", map[string]any{"task": "alpha"}))
	if res["agent_id"] != "agent_1" || res["status"] != "completed" {
		t.Fatalf("unexpected auto id result: %v", res)
	}

	res = asMap(t, mustExecuteAgentTool(t, p, session, "spawn_agent", map[string]any{"id": " Research_One ", "task": "beta"}))
	if res["agent_id"] != "research_one" {
		t.Fatalf("expected normalized custom id, got %v", res)
	}

	if _, err := executeAgentTool(t, p, session, "spawn_agent", map[string]any{"id": "research_one", "task": "again"}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
	if _, err := executeAgentTool(t, p, session, "spawn_agent", map[string]any{"id": "1bad", "task": "bad"}); err == nil || !strings.Contains(err.Error(), "invalid agent id") {
		t.Fatalf("expected invalid id error, got %v", err)
	}
}

func mustExecuteAgentTool(t *testing.T, p *SpawnProvider, session SessionContext, name string, args map[string]any) any {
	t.Helper()
	res, err := executeAgentTool(t, p, session, name, args)
	if err != nil {
		t.Fatalf("%s failed: %v", name, err)
	}
	return res
}

func TestSendMessageReusesSessionAndHistory(t *testing.T) {
	agent := &fakeSpawnAgent{}
	p, _, sessions, messages := newAgentControlProvider(t, agent)
	session := SessionContext{BotID: "bot1", SessionID: "parent1"}

	first := asMap(t, mustExecuteAgentTool(t, p, session, "spawn_agent", map[string]any{"id": "worker", "task": "first"}))
	second := asMap(t, mustExecuteAgentTool(t, p, session, "send_message", map[string]any{"id": "worker", "message": "second"}))
	if first["session_id"] == "" || first["session_id"] != second["session_id"] {
		t.Fatalf("expected send_message to reuse child session, first=%v second=%v", first, second)
	}

	call, ok := agent.callAt(1)
	if !ok {
		t.Fatal("expected second agent call")
	}
	if call.Identity.SessionID != first["session_id"] {
		t.Fatalf("expected reused identity session, got %q want %q", call.Identity.SessionID, first["session_id"])
	}
	if len(call.Messages) < 2 {
		t.Fatalf("expected persisted history loaded into second call, got %d messages", len(call.Messages))
	}
	rec, ok := sessions.byAgent("parent1", "worker")
	if !ok || rec.Metadata["agent_control_version"] != agentControlVersion {
		t.Fatalf("expected persisted agent metadata, got %+v", rec)
	}
	stored, _ := messages.ListBySession(context.Background(), first["session_id"].(string))
	if len(stored) != 4 {
		raw, _ := json.Marshal(stored)
		t.Fatalf("expected two user+assistant turns persisted, got %d: %s", len(stored), raw)
	}
}

func TestBusyAgentQueuesAndRunsFIFO(t *testing.T) {
	block := make(chan struct{})
	agent := &fakeSpawnAgent{block: block}
	p, _, _, _ := newAgentControlProvider(t, agent)
	session := SessionContext{BotID: "bot1", SessionID: "parent1"}

	first := asMap(t, mustExecuteAgentTool(t, p, session, "spawn_agent", map[string]any{
		"id":                "worker",
		"task":              "first",
		"run_in_background": true,
	}))
	if first["status"] != "background_started" {
		t.Fatalf("expected background_started, got %v", first)
	}
	if msg, _ := first["message"].(string); strings.Contains(msg, "get_background_status") || strings.Contains(msg, "kill_background") {
		t.Fatalf("background start message should not name tools that may be filtered out, got %q", msg)
	}
	second := asMap(t, mustExecuteAgentTool(t, p, session, "send_message", map[string]any{"id": "worker", "message": "second"}))
	third := asMap(t, mustExecuteAgentTool(t, p, session, "send_message", map[string]any{"id": "worker", "message": "third"}))
	if second["status"] != "queued" || second["queue_position"] != 1 {
		t.Fatalf("expected second queued at position 1, got %v", second)
	}
	if third["status"] != "queued" || third["queue_position"] != 2 {
		t.Fatalf("expected third queued at position 2, got %v", third)
	}

	close(block)
	waitUntil(t, 2*time.Second, func() bool {
		return reflect.DeepEqual(agent.queries(), []string{"first", "second", "third"})
	})
	waited := p.waitAgent(context.Background(), session, agentRecord{AgentID: "worker", SessionID: first["session_id"].(string)}, third["task_id"].(string), time.Second)
	if waited["status"] != "completed" {
		t.Fatalf("expected third task completed, got %v", waited)
	}
}

func TestWaitAgentTimeoutDoesNotCancelRunningTask(t *testing.T) {
	block := make(chan struct{})
	p, mgr, _, _ := newAgentControlProvider(t, &fakeSpawnAgent{block: block})
	session := SessionContext{BotID: "bot1", SessionID: "parent1"}
	started := asMap(t, mustExecuteAgentTool(t, p, session, "spawn_agent", map[string]any{
		"id":                "worker",
		"task":              "slow",
		"run_in_background": true,
	}))

	waited := p.waitAgent(context.Background(), session, agentRecord{AgentID: "worker", SessionID: started["session_id"].(string)}, "", 20*time.Millisecond)
	if waited["timed_out"] != true || waited["status"] != "running" {
		t.Fatalf("expected timed out running wait result, got %v", waited)
	}
	if task := mgr.GetForSession("bot1", "parent1", started["task_id"].(string)); task == nil || task.Snapshot().Status != background.TaskRunning {
		t.Fatalf("wait timeout should not cancel task, got %+v", task)
	}
	close(block)
}

func TestKillBackgroundCancelsRunningAndQueuedAgentTasks(t *testing.T) {
	block := make(chan struct{})
	agent := &fakeSpawnAgent{block: block}
	p, mgr, _, _ := newAgentControlProvider(t, agent)
	session := SessionContext{BotID: "bot1", SessionID: "parent1"}

	first := asMap(t, mustExecuteAgentTool(t, p, session, "spawn_agent", map[string]any{
		"id":                "worker",
		"task":              "first",
		"run_in_background": true,
	}))
	waitUntil(t, time.Second, func() bool {
		return reflect.DeepEqual(agent.queries(), []string{"first"})
	})
	second := asMap(t, mustExecuteAgentTool(t, p, session, "send_message", map[string]any{"id": "worker", "message": "second"}))

	if err := mgr.KillForSession("bot1", "parent1", second["task_id"].(string)); err != nil {
		t.Fatalf("kill queued task failed: %v", err)
	}
	if task := mgr.Get(second["task_id"].(string)); task == nil || task.Snapshot().Status != background.TaskKilled {
		t.Fatalf("expected queued task killed, got %+v", task)
	}

	if err := mgr.KillForSession("bot1", "parent1", first["task_id"].(string)); err != nil {
		t.Fatalf("kill running task failed: %v", err)
	}
	waitUntil(t, time.Second, func() bool {
		task := mgr.Get(first["task_id"].(string))
		return task != nil && task.Snapshot().Status == background.TaskKilled
	})
	if got := agent.queries(); !reflect.DeepEqual(got, []string{"first"}) {
		t.Fatalf("killed queued task should not run, got queries %v", got)
	}
}

func TestListAgentsScopedByCurrentSession(t *testing.T) {
	p, _, _, _ := newAgentControlProvider(t, &fakeSpawnAgent{})
	sessionA := SessionContext{BotID: "bot1", SessionID: "parent-a"}
	sessionB := SessionContext{BotID: "bot1", SessionID: "parent-b"}

	mustExecuteAgentTool(t, p, sessionA, "spawn_agent", map[string]any{"id": "alpha", "task": "a"})
	mustExecuteAgentTool(t, p, sessionB, "spawn_agent", map[string]any{"id": "beta", "task": "b"})

	listA := asMap(t, mustExecuteAgentTool(t, p, sessionA, "list_agents", map[string]any{}))
	agents := listA["agents"].([]map[string]any)
	if len(agents) != 1 || agents[0]["agent_id"] != "alpha" {
		t.Fatalf("expected only session A agent, got %v", listA)
	}
}
