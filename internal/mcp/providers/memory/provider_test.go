package memory

import (
	"context"
	"testing"

	mcpgw "github.com/memohai/memoh/internal/mcp"
	memprovider "github.com/memohai/memoh/internal/memory/provider"
	"github.com/memohai/memoh/internal/settings"
)

type fakeSettingsService struct {
	settings settings.Settings
	err      error
}

func (f *fakeSettingsService) GetBot(_ context.Context, _ string) (settings.Settings, error) {
	if f.err != nil {
		return settings.Settings{}, f.err
	}
	return f.settings, nil
}

type fakeProvider struct {
	tools    []mcpgw.ToolDescriptor
	callResp map[string]any
	callErr  error
}

func (f *fakeProvider) Type() string { return "fake" }
func (f *fakeProvider) OnBeforeChat(_ context.Context, _ memprovider.BeforeChatRequest) (*memprovider.BeforeChatResult, error) {
	return nil, nil
}
func (f *fakeProvider) OnAfterChat(_ context.Context, _ memprovider.AfterChatRequest) error {
	return nil
}
func (f *fakeProvider) ListTools(_ context.Context, _ mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	return f.tools, nil
}
func (f *fakeProvider) CallTool(_ context.Context, _ mcpgw.ToolSessionContext, _ string, _ map[string]any) (map[string]any, error) {
	return f.callResp, f.callErr
}
func (f *fakeProvider) Add(_ context.Context, _ memprovider.AddRequest) (memprovider.SearchResponse, error) {
	return memprovider.SearchResponse{}, nil
}
func (f *fakeProvider) Search(_ context.Context, _ memprovider.SearchRequest) (memprovider.SearchResponse, error) {
	return memprovider.SearchResponse{}, nil
}
func (f *fakeProvider) GetAll(_ context.Context, _ memprovider.GetAllRequest) (memprovider.SearchResponse, error) {
	return memprovider.SearchResponse{}, nil
}
func (f *fakeProvider) Update(_ context.Context, _ memprovider.UpdateRequest) (memprovider.MemoryItem, error) {
	return memprovider.MemoryItem{}, nil
}
func (f *fakeProvider) Delete(_ context.Context, _ string) (memprovider.DeleteResponse, error) {
	return memprovider.DeleteResponse{}, nil
}
func (f *fakeProvider) DeleteBatch(_ context.Context, _ []string) (memprovider.DeleteResponse, error) {
	return memprovider.DeleteResponse{}, nil
}
func (f *fakeProvider) DeleteAll(_ context.Context, _ memprovider.DeleteAllRequest) (memprovider.DeleteResponse, error) {
	return memprovider.DeleteResponse{}, nil
}
func (f *fakeProvider) Compact(_ context.Context, _ map[string]any, _ float64, _ int) (memprovider.CompactResult, error) {
	return memprovider.CompactResult{}, nil
}
func (f *fakeProvider) Usage(_ context.Context, _ map[string]any) (memprovider.UsageResponse, error) {
	return memprovider.UsageResponse{}, nil
}

func TestExecutor_ListTools_NoProvider(t *testing.T) {
	exec := NewExecutor(nil, nil, nil)
	tools, err := exec.ListTools(context.Background(), mcpgw.ToolSessionContext{BotID: "bot1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestExecutor_ListTools_WithProvider(t *testing.T) {
	registry := memprovider.NewRegistry(nil)
	fp := &fakeProvider{
		tools: []mcpgw.ToolDescriptor{{Name: "search_memory", Description: "test"}},
	}
	registry.Register("provider-1", fp)

	ss := &fakeSettingsService{
		settings: settings.Settings{MemoryProviderID: "provider-1"},
	}
	exec := NewExecutor(nil, registry, ss)

	tools, err := exec.ListTools(context.Background(), mcpgw.ToolSessionContext{BotID: "bot1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "search_memory" {
		t.Errorf("expected search_memory, got %s", tools[0].Name)
	}
}

func TestExecutor_CallTool_NoProvider(t *testing.T) {
	exec := NewExecutor(nil, nil, nil)
	result, err := exec.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: "bot1"}, "search_memory", nil)
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("expected error when no provider")
	}
}

func TestExecutor_CallTool_ProxiesToProvider(t *testing.T) {
	registry := memprovider.NewRegistry(nil)
	fp := &fakeProvider{
		callResp: mcpgw.BuildToolSuccessResult(map[string]any{"query": "test", "total": 1}),
	}
	registry.Register("provider-1", fp)

	ss := &fakeSettingsService{
		settings: settings.Settings{MemoryProviderID: "provider-1"},
	}
	exec := NewExecutor(nil, registry, ss)

	result, err := exec.CallTool(context.Background(), mcpgw.ToolSessionContext{BotID: "bot1"}, "search_memory", map[string]any{"query": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if isErr, _ := result["isError"].(bool); isErr {
		t.Error("unexpected error result")
	}
}
