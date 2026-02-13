package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
	mcpgw "github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/memory"
)

type fakeSearcher struct {
	resp memory.SearchResponse
	err  error
}

func (f *fakeSearcher) Search(ctx context.Context, req memory.SearchRequest) (memory.SearchResponse, error) {
	if f.err != nil {
		return memory.SearchResponse{}, f.err
	}
	return f.resp, nil
}

type fakeChatAccessor struct {
	chat           conversation.Conversation
	getErr         error
	participant    bool
	participantErr error
}

func (f *fakeChatAccessor) Get(ctx context.Context, conversationID string) (conversation.Conversation, error) {
	if f.getErr != nil {
		return conversation.Conversation{}, f.getErr
	}
	return f.chat, nil
}

func (f *fakeChatAccessor) IsParticipant(ctx context.Context, conversationID, channelIdentityID string) (bool, error) {
	if f.participantErr != nil {
		return false, f.participantErr
	}
	return f.participant, nil
}

func (f *fakeChatAccessor) GetReadAccess(ctx context.Context, conversationID, channelIdentityID string) (conversation.ConversationReadAccess, error) {
	return conversation.ConversationReadAccess{}, nil
}

type fakeAdminChecker struct {
	admin bool
	err   error
}

func (f *fakeAdminChecker) IsAdmin(ctx context.Context, channelIdentityID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.admin, nil
}

func TestExecutor_ListTools_NilDeps(t *testing.T) {
	exec := NewExecutor(nil, nil, nil, nil)
	tools, err := exec.ListTools(context.Background(), mcpgw.ToolSessionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools when deps nil, got %d", len(tools))
	}
}

func TestExecutor_ListTools(t *testing.T) {
	searcher := &fakeSearcher{}
	accessor := &fakeChatAccessor{}
	exec := NewExecutor(nil, searcher, accessor, nil)
	tools, err := exec.ListTools(context.Background(), mcpgw.ToolSessionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != toolSearchMemory {
		t.Errorf("tool name = %q, want %q", tools[0].Name, toolSearchMemory)
	}
}

func TestExecutor_CallTool_NotFound(t *testing.T) {
	searcher := &fakeSearcher{}
	accessor := &fakeChatAccessor{}
	exec := NewExecutor(nil, searcher, accessor, nil)
	_, err := exec.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: "bot1"}, "other_tool", nil)
	if err != mcpgw.ErrToolNotFound {
		t.Errorf("expected ErrToolNotFound, got %v", err)
	}
}

func TestExecutor_CallTool_NilDeps(t *testing.T) {
	exec := NewExecutor(nil, nil, nil, nil)
	result, err := exec.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: "bot1"}, toolSearchMemory, map[string]any{"query": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error result when deps nil")
	}
}

func TestExecutor_CallTool_NoQuery(t *testing.T) {
	searcher := &fakeSearcher{}
	accessor := &fakeChatAccessor{}
	exec := NewExecutor(nil, searcher, accessor, nil)
	result, err := exec.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: "bot1"}, toolSearchMemory, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when query is empty")
	}
}

func TestExecutor_CallTool_NoBotID(t *testing.T) {
	searcher := &fakeSearcher{}
	accessor := &fakeChatAccessor{}
	exec := NewExecutor(nil, searcher, accessor, nil)
	result, err := exec.CallTool(context.Background(), mcpgw.ToolSessionContext{}, toolSearchMemory, map[string]any{"query": "q"})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when bot_id is missing")
	}
}

func TestExecutor_CallTool_Success_BotScope(t *testing.T) {
	searcher := &fakeSearcher{
		resp: memory.SearchResponse{
			Results: []memory.MemoryItem{
				{ID: "id1", Memory: "mem1", Score: 0.9},
			},
		},
	}
	accessor := &fakeChatAccessor{}
	exec := NewExecutor(nil, searcher, accessor, nil)
	ctx := context.Background()
	session := mcpgw.ToolSessionContext{BotID: "bot1", ChatID: "bot1"}
	result, err := exec.CallTool(ctx, session, toolSearchMemory, map[string]any{"query": "test"})
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
	if content["query"] != "test" {
		t.Errorf("query = %v", content["query"])
	}
	if content["total"] != 1 {
		t.Errorf("total = %v", content["total"])
	}
}

func TestExecutor_CallTool_ChatNotFound(t *testing.T) {
	searcher := &fakeSearcher{}
	accessor := &fakeChatAccessor{getErr: errors.New("not found")}
	exec := NewExecutor(nil, searcher, accessor, nil)
	session := mcpgw.ToolSessionContext{BotID: "bot1", ChatID: "chat-other"}
	result, err := exec.CallTool(context.Background(), session, toolSearchMemory, map[string]any{"query": "q"})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when chat not found")
	}
}

func TestExecutor_CallTool_BotMismatch(t *testing.T) {
	accessor := &fakeChatAccessor{
		chat: conversation.Conversation{BotID: "other-bot", ID: "c1"},
	}
	searcher := &fakeSearcher{}
	exec := NewExecutor(nil, searcher, accessor, nil)
	session := mcpgw.ToolSessionContext{BotID: "bot1", ChatID: "c1"}
	result, err := exec.CallTool(context.Background(), session, toolSearchMemory, map[string]any{"query": "q"})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when bot mismatch")
	}
}

func TestExecutor_CallTool_NotParticipant(t *testing.T) {
	accessor := &fakeChatAccessor{
		chat:        conversation.Conversation{BotID: "bot1", ID: "c1"},
		participant: false,
	}
	searcher := &fakeSearcher{}
	exec := NewExecutor(nil, searcher, accessor, &fakeAdminChecker{admin: false})
	session := mcpgw.ToolSessionContext{BotID: "bot1", ChatID: "c1", ChannelIdentityID: "user1"}
	result, err := exec.CallTool(context.Background(), session, toolSearchMemory, map[string]any{"query": "q"})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when not participant")
	}
}

func TestExecutor_CallTool_AdminBypass(t *testing.T) {
	searcher := &fakeSearcher{
		resp: memory.SearchResponse{Results: []memory.MemoryItem{{ID: "id1", Memory: "m1", Score: 0.8}}},
	}
	accessor := &fakeChatAccessor{
		chat:        conversation.Conversation{BotID: "bot1", ID: "c1"},
		participant: false,
	}
	admin := &fakeAdminChecker{admin: true}
	exec := NewExecutor(nil, searcher, accessor, admin)
	session := mcpgw.ToolSessionContext{BotID: "bot1", ChatID: "c1", ChannelIdentityID: "admin1"}
	result, err := exec.CallTool(context.Background(), session, toolSearchMemory, map[string]any{"query": "q"})
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
	if v, ok := content["total"].(int); !ok || v != 1 {
		t.Errorf("total = %v", content["total"])
	}
}

func TestExecutor_CallTool_SearchError(t *testing.T) {
	searcher := &fakeSearcher{err: errors.New("search failed")}
	accessor := &fakeChatAccessor{}
	exec := NewExecutor(nil, searcher, accessor, nil)
	session := mcpgw.ToolSessionContext{BotID: "bot1"}
	result, err := exec.CallTool(context.Background(), session, toolSearchMemory, map[string]any{"query": "q"})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when search fails")
	}
}

func TestDeduplicateMemoryItems(t *testing.T) {
	tests := []struct {
		name    string
		items   []memory.MemoryItem
		wantLen int
	}{
		{"empty", nil, 0},
		{"single", []memory.MemoryItem{{ID: "a", Memory: "m", Score: 1}}, 1},
		{"dedup by id", []memory.MemoryItem{
			{ID: "a", Memory: "m1", Score: 1},
			{ID: "a", Memory: "m2", Score: 0.9},
		}, 1},
		{"dedup by memory when id empty", []memory.MemoryItem{
			{ID: "", Memory: "same", Score: 1},
			{ID: "", Memory: "same", Score: 0.9},
		}, 1},
		{"no dedup", []memory.MemoryItem{
			{ID: "a", Memory: "m1", Score: 1},
			{ID: "b", Memory: "m2", Score: 0.9},
		}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deduplicateMemoryItems(tt.items)
			if len(got) != tt.wantLen {
				t.Errorf("deduplicateMemoryItems() length = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}
