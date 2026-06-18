package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolGatewayMiddlewareScopesRuntimeToolCallsToActivePrompts(t *testing.T) {
	provider := &gatewayTestProvider{
		tools:      []ToolDescriptor{{Name: "echo_tool", InputSchema: map[string]any{"type": "object"}}},
		callResult: map[string]map[string]any{"echo_tool": BuildToolSuccessResult(map[string]any{"ok": true})},
		callErr:    map[string]error{},
	}
	service := NewToolGatewayService(nil, []ToolSource{provider})
	idle := ToolGatewayMiddleware(service, nil, ToolSessionContext{BotID: "bot-1", RuntimeID: "rt_idle"})(nil)

	result, err := idle(context.Background(), "tools/list", &sdkmcp.ServerRequest[*sdkmcp.ListToolsParams]{Params: &sdkmcp.ListToolsParams{}})
	if err != nil {
		t.Fatalf("idle runtime tools/list should succeed: %v", err)
	}
	list, ok := result.(*sdkmcp.ListToolsResult)
	if !ok || len(list.Tools) != 1 || list.Tools[0].Name != "echo_tool" {
		t.Fatalf("tools/list result = %#v", result)
	}

	if _, err := idle(context.Background(), "tools/call", callToolRequest("echo_tool")); err == nil || !strings.Contains(err.Error(), "not processing a prompt") {
		t.Fatalf("idle runtime tools/call error = %v", err)
	}

	active := ToolGatewayMiddleware(service, nil, ToolSessionContext{BotID: "bot-1", RuntimeID: "rt_active", RuntimeActive: true})(nil)
	result, err = active(context.Background(), "tools/call", callToolRequest("echo_tool"))
	if err != nil {
		t.Fatalf("active runtime tools/call should succeed: %v", err)
	}
	if _, ok := result.(*sdkmcp.CallToolResult); !ok {
		t.Fatalf("tools/call result = %#v", result)
	}
}

func TestToolSessionContextFromHTTPParsesSupportsImageInput(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.test/tools", nil)
	if err != nil {
		t.Fatalf("NewRequest error = %v", err)
	}
	req.Header.Set(ToolHeaderSupportsImageInput, "true")

	session := ToolSessionContextFromHTTP(req, "bot-1")
	if !session.SupportsImageInput {
		t.Fatalf("SupportsImageInput = false, want true")
	}
}

func callToolRequest(name string) *sdkmcp.ServerRequest[*sdkmcp.CallToolParamsRaw] {
	return &sdkmcp.ServerRequest[*sdkmcp.CallToolParamsRaw]{
		Params: &sdkmcp.CallToolParamsRaw{
			Name:      name,
			Arguments: json.RawMessage(`{}`),
		},
	}
}
