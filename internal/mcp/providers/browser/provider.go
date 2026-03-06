package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	mcpgw "github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/browsercontexts"
	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/settings"
)

const (
	toolBrowserAction  = "browser_action"
	toolBrowserObserve = "browser_observe"
)

type Executor struct {
	logger          *slog.Logger
	settings        *settings.Service
	browserContexts *browsercontexts.Service
	gatewayBaseURL  string
	httpClient      *http.Client
}

func NewExecutor(log *slog.Logger, settingsSvc *settings.Service, browserSvc *browsercontexts.Service, gatewayCfg config.BrowserGatewayConfig) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{
		logger:          log.With(slog.String("provider", "browser_tool")),
		settings:        settingsSvc,
		browserContexts: browserSvc,
		gatewayBaseURL:  strings.TrimRight(gatewayCfg.BaseURL(), "/"),
		httpClient:      &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *Executor) ListTools(ctx context.Context, session mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	if e.settings == nil || e.browserContexts == nil {
		return []mcpgw.ToolDescriptor{}, nil
	}
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return []mcpgw.ToolDescriptor{}, nil
	}
	botSettings, err := e.settings.GetBot(ctx, botID)
	if err != nil {
		return []mcpgw.ToolDescriptor{}, nil
	}
	if strings.TrimSpace(botSettings.BrowserContextID) == "" {
		return []mcpgw.ToolDescriptor{}, nil
	}
	return []mcpgw.ToolDescriptor{
		{
			Name:        toolBrowserAction,
			Description: "Execute a browser action: navigate to URL, click element, type text, scroll, go back/forward, reload, or wait.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":    map[string]any{"type": "string", "enum": []string{"navigate", "click", "type", "scroll", "go_back", "go_forward", "reload", "wait"}, "description": "The browser action to perform"},
					"url":       map[string]any{"type": "string", "description": "URL to navigate to (for navigate action)"},
					"selector":  map[string]any{"type": "string", "description": "CSS selector for the target element"},
					"text":      map[string]any{"type": "string", "description": "Text to type (for type action)"},
					"direction": map[string]any{"type": "string", "enum": []string{"up", "down", "left", "right"}, "description": "Scroll direction"},
					"timeout":   map[string]any{"type": "integer", "description": "Timeout in milliseconds"},
				},
				"required": []string{"action"},
			},
		},
		{
			Name:        toolBrowserObserve,
			Description: "Observe the current browser page: take screenshot, get text content, get HTML, evaluate JavaScript, get current URL, or get page title.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"observe":  map[string]any{"type": "string", "enum": []string{"screenshot", "get_content", "get_html", "evaluate", "get_url", "get_title"}, "description": "What to observe from the page"},
					"selector": map[string]any{"type": "string", "description": "CSS selector to scope the observation"},
					"script":   map[string]any{"type": "string", "description": "JavaScript to evaluate (for evaluate observation)"},
				},
				"required": []string{"observe"},
			},
		},
	}, nil
}

func (e *Executor) CallTool(ctx context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if e.settings == nil || e.browserContexts == nil {
		return mcpgw.BuildToolErrorResult("browser tools are not available"), nil
	}
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return mcpgw.BuildToolErrorResult("bot_id is required"), nil
	}

	botSettings, err := e.settings.GetBot(ctx, botID)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	browserCtxID := strings.TrimSpace(botSettings.BrowserContextID)
	if browserCtxID == "" {
		return mcpgw.BuildToolErrorResult("browser context not configured for this bot"), nil
	}

	bcConfig, err := e.browserContexts.GetByID(ctx, browserCtxID)
	if err != nil {
		return mcpgw.BuildToolErrorResult("failed to load browser context config: " + err.Error()), nil
	}

	if err := e.ensureContext(ctx, browserCtxID, bcConfig); err != nil {
		return mcpgw.BuildToolErrorResult("failed to ensure browser context: " + err.Error()), nil
	}

	switch toolName {
	case toolBrowserAction:
		return e.callAction(ctx, browserCtxID, arguments)
	case toolBrowserObserve:
		return e.callObserve(ctx, browserCtxID, arguments)
	default:
		return nil, mcpgw.ErrToolNotFound
	}
}

func (e *Executor) ensureContext(ctx context.Context, contextID string, bc browsercontexts.BrowserContext) error {
	existsURL := fmt.Sprintf("%s/context/%s/exists", e.gatewayBaseURL, contextID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, existsURL, nil)
	if err != nil {
		return err
	}
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("browser gateway unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	var existsResp struct {
		Exists bool `json:"exists"`
	}
	if err := json.Unmarshal(body, &existsResp); err != nil {
		return fmt.Errorf("invalid exists response: %w", err)
	}
	if existsResp.Exists {
		return nil
	}

	createPayload, _ := json.Marshal(map[string]any{
		"id":     contextID,
		"name":   bc.Name,
		"config": json.RawMessage(bc.Config),
	})
	createURL := fmt.Sprintf("%s/context", e.gatewayBaseURL)
	createReq, err := http.NewRequestWithContext(ctx, http.MethodPost, createURL, bytes.NewReader(createPayload))
	if err != nil {
		return err
	}
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := e.httpClient.Do(createReq)
	if err != nil {
		return fmt.Errorf("failed to create browser context: %w", err)
	}
	defer func() { _ = createResp.Body.Close() }()
	if createResp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(createResp.Body)
		return fmt.Errorf("create context failed (HTTP %d): %s", createResp.StatusCode, string(errBody))
	}
	return nil
}

func (e *Executor) callAction(ctx context.Context, contextID string, arguments map[string]any) (map[string]any, error) {
	action := mcpgw.StringArg(arguments, "action")
	if action == "" {
		return mcpgw.BuildToolErrorResult("action is required"), nil
	}
	payload := map[string]any{"action": action}
	if v := mcpgw.StringArg(arguments, "url"); v != "" {
		payload["url"] = v
	}
	if v := mcpgw.StringArg(arguments, "selector"); v != "" {
		payload["selector"] = v
	}
	if v := mcpgw.StringArg(arguments, "text"); v != "" {
		payload["text"] = v
	}
	if v := mcpgw.StringArg(arguments, "direction"); v != "" {
		payload["direction"] = v
	}
	if v, ok, _ := mcpgw.IntArg(arguments, "timeout"); ok {
		payload["timeout"] = v
	}
	return e.doGatewayAction(ctx, contextID, payload)
}

func (e *Executor) callObserve(ctx context.Context, contextID string, arguments map[string]any) (map[string]any, error) {
	observe := mcpgw.StringArg(arguments, "observe")
	if observe == "" {
		return mcpgw.BuildToolErrorResult("observe is required"), nil
	}
	payload := map[string]any{"action": observe}
	if v := mcpgw.StringArg(arguments, "selector"); v != "" {
		payload["selector"] = v
	}
	if v := mcpgw.StringArg(arguments, "script"); v != "" {
		payload["script"] = v
	}
	return e.doGatewayAction(ctx, contextID, payload)
}

func (e *Executor) doGatewayAction(ctx context.Context, contextID string, payload map[string]any) (map[string]any, error) {
	body, _ := json.Marshal(payload)
	actionURL := fmt.Sprintf("%s/context/%s/action", e.gatewayBaseURL, contextID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, actionURL, bytes.NewReader(body))
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return mcpgw.BuildToolErrorResult("browser gateway request failed: " + err.Error()), nil
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	var gwResp struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
		Error   string         `json:"error"`
	}
	if err := json.Unmarshal(respBody, &gwResp); err != nil {
		return mcpgw.BuildToolErrorResult("invalid gateway response"), nil
	}
	if !gwResp.Success {
		errMsg := gwResp.Error
		if errMsg == "" {
			errMsg = "browser action failed"
		}
		return mcpgw.BuildToolErrorResult(errMsg), nil
	}
	return mcpgw.BuildToolSuccessResult(gwResp.Data), nil
}
