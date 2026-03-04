package mcp

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

type gatewayTestProvider struct {
	tools      []ToolDescriptor
	callResult map[string]map[string]any
	callErr    map[string]error
}

func (p *gatewayTestProvider) ListTools(_ context.Context, _ ToolSessionContext) ([]ToolDescriptor, error) {
	return p.tools, nil
}

func (p *gatewayTestProvider) CallTool(_ context.Context, _ ToolSessionContext, toolName string, _ map[string]any) (map[string]any, error) {
	if err, ok := p.callErr[toolName]; ok {
		return nil, err
	}
	if result, ok := p.callResult[toolName]; ok {
		return result, nil
	}
	return nil, ErrToolNotFound
}

func TestToolGatewayServiceListTools(t *testing.T) {
	providerA := &gatewayTestProvider{
		tools: []ToolDescriptor{
			{Name: "tool_a", InputSchema: map[string]any{"type": "object"}},
			{Name: "dup_tool", InputSchema: map[string]any{"type": "object"}},
		},
	}
	providerB := &gatewayTestProvider{
		tools: []ToolDescriptor{
			{Name: "tool_b", InputSchema: map[string]any{"type": "object"}},
			{Name: "dup_tool", InputSchema: map[string]any{"type": "object"}},
		},
	}
	service := NewToolGatewayService(slog.Default(), []ToolExecutor{providerA, providerB}, nil)

	tools, err := service.ListTools(context.Background(), ToolSessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("list tools failed: %v", err)
	}
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools after dedupe, got %d", len(tools))
	}
}

func TestToolGatewayServiceCallToolSuccess(t *testing.T) {
	provider := &gatewayTestProvider{
		tools: []ToolDescriptor{
			{Name: "echo_tool", InputSchema: map[string]any{"type": "object"}},
		},
		callResult: map[string]map[string]any{
			"echo_tool": {
				"content": []map[string]any{
					{"type": "text", "text": "ok"},
				},
			},
		},
		callErr: map[string]error{},
	}
	service := NewToolGatewayService(slog.Default(), []ToolExecutor{provider}, nil)

	result, err := service.CallTool(context.Background(), ToolSessionContext{BotID: "bot-1"}, ToolCallPayload{
		Name:      "echo_tool",
		Arguments: map[string]any{"value": "hello"},
	})
	if err != nil {
		t.Fatalf("call tool should not fail: %v", err)
	}
	if _, ok := result["content"]; !ok {
		t.Fatalf("expected content in tool result")
	}
}

func TestToolGatewayServiceCallToolNotFound(t *testing.T) {
	provider := &gatewayTestProvider{
		tools:      []ToolDescriptor{},
		callResult: map[string]map[string]any{},
		callErr:    map[string]error{},
	}
	service := NewToolGatewayService(slog.Default(), []ToolExecutor{provider}, nil)

	result, err := service.CallTool(context.Background(), ToolSessionContext{BotID: "bot-1"}, ToolCallPayload{
		Name:      "missing_tool",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call should return mcp error result instead of failing: %v", err)
	}
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Fatalf("expected isError=true for missing tool")
	}
}

func TestToolGatewayServiceCallToolProviderError(t *testing.T) {
	provider := &gatewayTestProvider{
		tools: []ToolDescriptor{
			{Name: "broken_tool", InputSchema: map[string]any{"type": "object"}},
		},
		callResult: map[string]map[string]any{},
		callErr: map[string]error{
			"broken_tool": errors.New("boom"),
		},
	}
	service := NewToolGatewayService(slog.Default(), []ToolExecutor{provider}, nil)

	result, err := service.CallTool(context.Background(), ToolSessionContext{BotID: "bot-1"}, ToolCallPayload{
		Name:      "broken_tool",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call should not return hard error: %v", err)
	}
	isErr, _ := result["isError"].(bool)
	if !isErr {
		t.Fatalf("expected isError=true for provider failure")
	}
}
