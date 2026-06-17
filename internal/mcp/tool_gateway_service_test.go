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

type sessionAwareGatewayTestProvider struct{}

func (*sessionAwareGatewayTestProvider) ListTools(_ context.Context, session ToolSessionContext) ([]ToolDescriptor, error) {
	if session.IsSubagent {
		return []ToolDescriptor{{Name: "subagent_tool", InputSchema: map[string]any{"type": "object"}}}, nil
	}
	return []ToolDescriptor{{Name: "agent_tool", InputSchema: map[string]any{"type": "object"}}}, nil
}

func (*sessionAwareGatewayTestProvider) CallTool(context.Context, ToolSessionContext, string, map[string]any) (map[string]any, error) {
	return nil, ErrToolNotFound
}

type countingGatewayTestProvider struct {
	calls int
}

func (p *countingGatewayTestProvider) ListTools(_ context.Context, _ ToolSessionContext) ([]ToolDescriptor, error) {
	p.calls++
	return []ToolDescriptor{{Name: "cached_tool", InputSchema: map[string]any{"type": "object"}}}, nil
}

func (*countingGatewayTestProvider) CallTool(context.Context, ToolSessionContext, string, map[string]any) (map[string]any, error) {
	return nil, ErrToolNotFound
}

type mutableGatewayTestProvider struct {
	tools []ToolDescriptor
}

func (p *mutableGatewayTestProvider) ListTools(_ context.Context, _ ToolSessionContext) ([]ToolDescriptor, error) {
	return append([]ToolDescriptor(nil), p.tools...), nil
}

func (*mutableGatewayTestProvider) CallTool(context.Context, ToolSessionContext, string, map[string]any) (map[string]any, error) {
	return nil, ErrToolNotFound
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

func TestToolGatewayServiceCacheIgnoresSessionID(t *testing.T) {
	provider := &countingGatewayTestProvider{}
	service := NewToolGatewayService(slog.Default(), []ToolSource{provider})
	session := ToolSessionContext{
		BotID:             "bot-1",
		SessionID:         "session-1",
		SessionType:       "acp_agent",
		ChannelIdentityID: "user-1",
	}

	if _, err := service.ListTools(context.Background(), session); err != nil {
		t.Fatalf("list tools failed: %v", err)
	}
	session.SessionID = "session-2"
	if _, err := service.ListTools(context.Background(), session); err != nil {
		t.Fatalf("list tools failed: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("ListTools calls = %d, want cache hit across session IDs", provider.calls)
	}
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
	service := NewToolGatewayService(slog.Default(), []ToolSource{providerA, providerB})

	tools, err := service.ListTools(context.Background(), ToolSessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("list tools failed: %v", err)
	}
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools after dedupe, got %d", len(tools))
	}
}

func TestToolGatewayServiceLookupTool(t *testing.T) {
	provider := &gatewayTestProvider{
		tools: []ToolDescriptor{
			{Name: "lookup_tool", Description: "Lookup", InputSchema: map[string]any{"type": "object"}},
		},
	}
	service := NewToolGatewayService(slog.Default(), []ToolSource{provider})

	desc, ok, err := service.LookupTool(context.Background(), ToolSessionContext{BotID: "bot-1"}, "lookup_tool")
	if err != nil {
		t.Fatalf("LookupTool error = %v", err)
	}
	if !ok || desc.Name != "lookup_tool" {
		t.Fatalf("LookupTool = (%#v, %v), want lookup_tool", desc, ok)
	}
	if _, ok, err := service.LookupTool(context.Background(), ToolSessionContext{BotID: "bot-1"}, "missing_tool"); err != nil || ok {
		t.Fatalf("LookupTool missing = ok %v err %v, want not found without error", ok, err)
	}
}

func TestToolGatewayServiceLookupToolRefreshesAfterCachedMiss(t *testing.T) {
	provider := &mutableGatewayTestProvider{}
	service := NewToolGatewayService(slog.Default(), []ToolSource{provider})
	session := ToolSessionContext{BotID: "bot-1"}

	if _, ok, err := service.LookupTool(context.Background(), session, "late_tool"); err != nil || ok {
		t.Fatalf("LookupTool before provider update = ok %v err %v, want cached miss", ok, err)
	}

	provider.tools = []ToolDescriptor{{Name: "late_tool", Description: "Late", InputSchema: map[string]any{"type": "object"}}}
	desc, ok, err := service.LookupTool(context.Background(), session, "late_tool")
	if err != nil {
		t.Fatalf("LookupTool after provider update error = %v", err)
	}
	if !ok || desc.Name != "late_tool" {
		t.Fatalf("LookupTool after provider update = (%#v, %v), want late_tool", desc, ok)
	}
}

func TestToolGatewayServiceCacheSeparatesSessionToolScopes(t *testing.T) {
	service := NewToolGatewayService(slog.Default(), []ToolSource{&sessionAwareGatewayTestProvider{}})

	agentTools, err := service.ListTools(context.Background(), ToolSessionContext{BotID: "bot-1", SessionID: "session-1"})
	if err != nil {
		t.Fatalf("list agent tools failed: %v", err)
	}
	if len(agentTools) != 1 || agentTools[0].Name != "agent_tool" {
		t.Fatalf("agent tools = %#v", agentTools)
	}

	subagentTools, err := service.ListTools(context.Background(), ToolSessionContext{BotID: "bot-1", SessionID: "session-1", IsSubagent: true})
	if err != nil {
		t.Fatalf("list subagent tools failed: %v", err)
	}
	if len(subagentTools) != 1 || subagentTools[0].Name != "subagent_tool" {
		t.Fatalf("subagent tools = %#v", subagentTools)
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
	service := NewToolGatewayService(slog.Default(), []ToolSource{provider})

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
	service := NewToolGatewayService(slog.Default(), []ToolSource{provider})

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
	service := NewToolGatewayService(slog.Default(), []ToolSource{provider})

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
