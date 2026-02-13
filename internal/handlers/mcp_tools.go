package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/memohai/memoh/internal/auth"
	mcpgw "github.com/memohai/memoh/internal/mcp"
)

const (
	headerChannelIdentityID = "X-Memoh-Channel-Identity-Id"
	headerSessionToken      = "X-Memoh-Session-Token"
	headerCurrentPlatform   = "X-Memoh-Current-Platform"
	headerReplyTarget       = "X-Memoh-Reply-Target"
)

func (h *ContainerdHandler) SetToolGatewayService(service *mcpgw.ToolGatewayService) {
	h.toolGateway = service
}

// HandleMCPTools godoc
// @Summary Unified MCP tools gateway
// @Description MCP endpoint for tool discovery and invocation.
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body object true "JSON-RPC request"
// @Success 200 {object} object "JSON-RPC response: {jsonrpc,id,result|error}"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/tools [post]
func (h *ContainerdHandler) HandleMCPTools(c echo.Context) error {
	if h.toolGateway == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "tool gateway not configured")
	}
	botID, err := h.requireBotAccessWithGuest(c)
	if err != nil {
		return err
	}
	return h.handleMCPToolsWithBotID(c, botID)
}

func (h *ContainerdHandler) handleMCPToolsWithBotID(c echo.Context, botID string) error {
	session := h.buildToolSessionContext(c, botID)

	req := c.Request()
	ensureStreamableAcceptHeader(req)
	ctx := context.WithValue(req.Context(), toolSessionContextKey{}, session)
	req = req.WithContext(ctx)

	handler := sdkmcp.NewStreamableHTTPHandler(
		func(r *http.Request) *sdkmcp.Server {
			return h.buildToolMCPServer(r.Context())
		},
		&sdkmcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
			Logger:       h.logger,
		},
	)
	handler.ServeHTTP(c.Response().Writer, req)
	return nil
}

func ensureStreamableAcceptHeader(req *http.Request) {
	if req == nil {
		return
	}
	acceptValues := req.Header.Values("Accept")
	joined := strings.ToLower(strings.Join(acceptValues, ","))
	hasJSON := strings.Contains(joined, "application/json") || strings.Contains(joined, "application/*") || strings.Contains(joined, "*/*")
	hasStream := strings.Contains(joined, "text/event-stream") || strings.Contains(joined, "text/*") || strings.Contains(joined, "*/*")
	if hasJSON && hasStream {
		return
	}

	base := strings.TrimSpace(strings.Join(acceptValues, ","))
	parts := make([]string, 0, 3)
	if base != "" {
		parts = append(parts, base)
	}
	if !hasJSON {
		parts = append(parts, "application/json")
	}
	if !hasStream {
		parts = append(parts, "text/event-stream")
	}
	if len(parts) == 0 {
		parts = append(parts, "application/json", "text/event-stream")
	}
	req.Header.Set("Accept", strings.Join(parts, ", "))
}

type toolSessionContextKey struct{}

func (h *ContainerdHandler) buildToolMCPServer(ctx context.Context) *sdkmcp.Server {
	if h.toolGateway == nil {
		return nil
	}
	session, ok := ctx.Value(toolSessionContextKey{}).(mcpgw.ToolSessionContext)
	if !ok {
		return nil
	}

	server := sdkmcp.NewServer(
		&sdkmcp.Implementation{
			Name:    "memoh-tools-gateway",
			Version: "1.0.0",
		},
		&sdkmcp.ServerOptions{
			Capabilities: &sdkmcp.ServerCapabilities{
				Tools: &sdkmcp.ToolCapabilities{
					ListChanged: false,
				},
			},
		},
	)
	server.AddReceivingMiddleware(h.toolGatewayMiddleware(session))
	return server
}

func (h *ContainerdHandler) toolGatewayMiddleware(session mcpgw.ToolSessionContext) sdkmcp.Middleware {
	return func(next sdkmcp.MethodHandler) sdkmcp.MethodHandler {
		return func(ctx context.Context, method string, req sdkmcp.Request) (sdkmcp.Result, error) {
			switch strings.TrimSpace(method) {
			case "tools/list":
				tools, err := h.toolGateway.ListTools(ctx, session)
				if err != nil {
					return nil, err
				}
				return &sdkmcp.ListToolsResult{
					Tools: convertGatewayToolsToSDK(tools),
				}, nil
			case "tools/call":
				callReq, ok := req.(*sdkmcp.ServerRequest[*sdkmcp.CallToolParamsRaw])
				if !ok || callReq == nil || callReq.Params == nil {
					return nil, fmt.Errorf("tools/call params is required")
				}
				payload, err := buildToolCallPayloadFromRaw(callReq.Params)
				if err != nil {
					return nil, err
				}
				result, err := h.toolGateway.CallTool(ctx, session, payload)
				if err != nil {
					return nil, err
				}
				return convertGatewayCallResultToSDK(result)
			default:
				return next(ctx, method, req)
			}
		}
	}
}

func buildToolCallPayloadFromRaw(params *sdkmcp.CallToolParamsRaw) (mcpgw.ToolCallPayload, error) {
	if params == nil {
		return mcpgw.ToolCallPayload{}, fmt.Errorf("tools/call params is required")
	}
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return mcpgw.ToolCallPayload{}, fmt.Errorf("tools/call name is required")
	}
	arguments := map[string]any{}
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &arguments); err != nil {
			return mcpgw.ToolCallPayload{}, err
		}
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	return mcpgw.ToolCallPayload{
		Name:      name,
		Arguments: arguments,
	}, nil
}

func convertGatewayToolsToSDK(items []mcpgw.ToolDescriptor) []*sdkmcp.Tool {
	if len(items) == 0 {
		return []*sdkmcp.Tool{}
	}
	tools := make([]*sdkmcp.Tool, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		inputSchema := item.InputSchema
		if inputSchema == nil {
			inputSchema = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}
		tools = append(tools, &sdkmcp.Tool{
			Name:        name,
			Description: strings.TrimSpace(item.Description),
			InputSchema: inputSchema,
		})
	}
	return tools
}

func convertGatewayCallResultToSDK(result map[string]any) (*sdkmcp.CallToolResult, error) {
	if result == nil {
		result = mcpgw.BuildToolSuccessResult(map[string]any{"ok": true})
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	var out sdkmcp.CallToolResult
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (h *ContainerdHandler) buildToolSessionContext(c echo.Context, botID string) mcpgw.ToolSessionContext {
	channelIdentityID := strings.TrimSpace(c.Request().Header.Get(headerChannelIdentityID))
	if channelIdentityID == "" {
		if ctxIdentityID, err := auth.UserIDFromContext(c); err == nil {
			channelIdentityID = strings.TrimSpace(ctxIdentityID)
		}
	}
	return mcpgw.ToolSessionContext{
		BotID:             strings.TrimSpace(botID),
		ChatID:            strings.TrimSpace(botID),
		ChannelIdentityID: channelIdentityID,
		SessionToken:      strings.TrimSpace(c.Request().Header.Get(headerSessionToken)),
		CurrentPlatform:   strings.TrimSpace(c.Request().Header.Get(headerCurrentPlatform)),
		ReplyTarget:       strings.TrimSpace(c.Request().Header.Get(headerReplyTarget)),
	}
}
