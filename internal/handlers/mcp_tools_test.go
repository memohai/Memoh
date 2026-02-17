package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	mcpgw "github.com/memohai/memoh/internal/mcp"
)

func TestBuildToolCallPayloadFromRaw(t *testing.T) {
	params := &sdkmcp.CallToolParamsRaw{
		Name:      " tool_a ",
		Arguments: json.RawMessage(`{"x":1}`),
	}
	payload, err := buildToolCallPayloadFromRaw(params)
	if err != nil {
		t.Fatalf("valid payload should parse: %v", err)
	}
	if payload.Name != "tool_a" {
		t.Fatalf("unexpected tool name: %s", payload.Name)
	}
	if _, ok := payload.Arguments["x"]; !ok {
		t.Fatalf("expected argument x")
	}

	invalid := &sdkmcp.CallToolParamsRaw{
		Name:      "",
		Arguments: json.RawMessage(`{}`),
	}
	if _, err := buildToolCallPayloadFromRaw(invalid); err == nil {
		t.Fatalf("empty tool name should fail")
	}
}

func TestHandleMCPToolsWithoutGateway(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/bots/bot-1/tools", strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"tools/list"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/bots/:bot_id/tools")
	c.SetParamNames("bot_id")
	c.SetParamValues("bot-1")

	handler := &ContainerdHandler{}
	err := handler.HandleMCPTools(c)
	if err == nil {
		t.Fatalf("expected service unavailable error")
	}
	httpErr := &echo.HTTPError{}
	ok := errors.As(err, &httpErr)
	if !ok {
		t.Fatalf("expected echo HTTP error, got %T", err)
	}
	if httpErr.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status code: %d", httpErr.Code)
	}
}

type mcpToolsTestExecutor struct {
	lastSession mcpgw.ToolSessionContext
}

func (e *mcpToolsTestExecutor) ListTools(_ context.Context, session mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	e.lastSession = session
	return []mcpgw.ToolDescriptor{
		{
			Name:        "echo_tool",
			Description: "echo input",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{"type": "string"},
				},
			},
		},
	}, nil
}

func (e *mcpToolsTestExecutor) CallTool(_ context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	e.lastSession = session
	if strings.TrimSpace(toolName) != "echo_tool" {
		return nil, mcpgw.ErrToolNotFound
	}
	return mcpgw.BuildToolSuccessResult(map[string]any{
		"ok":                  true,
		"echo":                mcpgw.StringArg(arguments, "input"),
		"chat_id":             session.ChatID,
		"channel_identity_id": session.ChannelIdentityID,
	}), nil
}

func TestHandleMCPToolsWithGatewayAcceptCompatibility(t *testing.T) {
	e := echo.New()
	executor := &mcpToolsTestExecutor{}
	toolGateway := mcpgw.NewToolGatewayService(slog.Default(), []mcpgw.ToolExecutor{executor}, nil)
	handler := &ContainerdHandler{
		logger:      slog.Default(),
		toolGateway: toolGateway,
	}

	listReq := httptest.NewRequest(http.MethodPost, "/bots/bot-1/tools", strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"tools/list"}`))
	listReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	listReq.Header.Set("Accept", "application/json")
	listReq.Header.Set("X-Memoh-Channel-Identity-Id", "user-1")
	listRec := httptest.NewRecorder()
	listCtx := e.NewContext(listReq, listRec)

	if err := handler.handleMCPToolsWithBotID(listCtx, "bot-1"); err != nil {
		t.Fatalf("list tools should succeed: %v", err)
	}
	if listRec.Code != http.StatusOK {
		t.Fatalf("unexpected list status: %d body=%s", listRec.Code, listRec.Body.String())
	}
	if !strings.Contains(strings.ToLower(listReq.Header.Get("Accept")), "text/event-stream") {
		t.Fatalf("accept header should include text/event-stream: %s", listReq.Header.Get("Accept"))
	}

	var listPayload map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list payload failed: %v", err)
	}
	result, _ := listPayload["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("expected one tool, got: %#v", result["tools"])
	}

	callReq := httptest.NewRequest(http.MethodPost, "/bots/bot-1/tools", strings.NewReader(`{"jsonrpc":"2.0","id":"2","method":"tools/call","params":{"name":"echo_tool","arguments":{"input":"hello"}}}`))
	callReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	callReq.Header.Set("Accept", "application/json")
	callReq.Header.Set("X-Memoh-Channel-Identity-Id", "user-1")
	callRec := httptest.NewRecorder()
	callCtx := e.NewContext(callReq, callRec)

	if err := handler.handleMCPToolsWithBotID(callCtx, "bot-1"); err != nil {
		t.Fatalf("call tool should succeed: %v", err)
	}
	if callRec.Code != http.StatusOK {
		t.Fatalf("unexpected call status: %d body=%s", callRec.Code, callRec.Body.String())
	}

	var callPayload map[string]any
	if err := json.Unmarshal(callRec.Body.Bytes(), &callPayload); err != nil {
		t.Fatalf("decode call payload failed: %v", err)
	}
	callResult, _ := callPayload["result"].(map[string]any)
	structured, _ := callResult["structuredContent"].(map[string]any)
	if echoValue := strings.TrimSpace(mcpgw.StringArg(structured, "echo")); echoValue != "hello" {
		t.Fatalf("unexpected echo value: %#v", structured["echo"])
	}
	if strings.TrimSpace(mcpgw.StringArg(structured, "chat_id")) != "bot-1" {
		t.Fatalf("unexpected chat id: %#v", structured["chat_id"])
	}
	if strings.TrimSpace(mcpgw.StringArg(structured, "channel_identity_id")) != "user-1" {
		t.Fatalf("unexpected channel identity id: %#v", structured["channel_identity_id"])
	}
}
