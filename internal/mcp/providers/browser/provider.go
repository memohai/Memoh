package browser

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/browsercontexts"
	"github.com/memohai/memoh/internal/config"
	mcpgw "github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	toolBrowserAction  = "browser_action"
	toolBrowserObserve = "browser_observe"
)

type Executor struct {
	logger          *slog.Logger
	settings        *settings.Service
	browserContexts *browsercontexts.Service
	containers      bridge.Provider
	gatewayBaseURL  string
	httpClient      *http.Client
}

func NewExecutor(log *slog.Logger, settingsSvc *settings.Service, browserSvc *browsercontexts.Service, containers bridge.Provider, gatewayCfg config.BrowserGatewayConfig) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{
		logger:          log.With(slog.String("provider", "browser_tool")),
		settings:        settingsSvc,
		browserContexts: browserSvc,
		containers:      containers,
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
			Description: "Execute a browser action: navigate, click, double-click, focus, type, fill, press key, keyboard input, hover, select option, check/uncheck, scroll, drag-and-drop, upload files, go back/forward, reload, wait, or manage tabs (new/select/close).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":          map[string]any{"type": "string", "enum": []string{"navigate", "click", "dblclick", "focus", "type", "fill", "press", "keyboard_type", "keyboard_inserttext", "keydown", "keyup", "hover", "select", "check", "uncheck", "scroll", "scrollintoview", "drag", "upload", "wait", "go_back", "go_forward", "reload", "tab_new", "tab_select", "tab_close"}, "description": "The browser action to perform"},
					"url":             map[string]any{"type": "string", "description": "URL to navigate to (for navigate, tab_new)"},
					"selector":        map[string]any{"type": "string", "description": "CSS selector for the target element"},
					"text":            map[string]any{"type": "string", "description": "Text to type or fill (for type, fill, keyboard_type, keyboard_inserttext)"},
					"key":             map[string]any{"type": "string", "description": "Key to press (for press, keydown, keyup). Examples: Enter, Tab, Escape, Control+a"},
					"value":           map[string]any{"type": "string", "description": "Value to select (for select action)"},
					"target_selector": map[string]any{"type": "string", "description": "Target CSS selector (for drag action)"},
					"files":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "File paths to upload (for upload action)"},
					"tab_index":       map[string]any{"type": "integer", "description": "Tab index (for tab_select, tab_close)"},
					"direction":       map[string]any{"type": "string", "enum": []string{"up", "down", "left", "right"}, "description": "Scroll direction (for scroll)"},
					"amount":          map[string]any{"type": "integer", "description": "Scroll amount in pixels (for scroll, default 500)"},
					"timeout":         map[string]any{"type": "integer", "description": "Timeout in milliseconds"},
				},
				"required": []string{"action"},
			},
		},
		{
			Name:        toolBrowserObserve,
			Description: "Observe the current browser page: take screenshot (optionally annotated with numbered element labels or full-page), get accessibility tree snapshot, get text content, get HTML, evaluate JavaScript, get current URL, get page title, export PDF, or list open tabs.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"observe":   map[string]any{"type": "string", "enum": []string{"screenshot", "screenshot_annotate", "snapshot", "get_content", "get_html", "evaluate", "get_url", "get_title", "pdf", "tab_list"}, "description": "What to observe from the page"},
					"selector":  map[string]any{"type": "string", "description": "CSS selector to scope the observation"},
					"script":    map[string]any{"type": "string", "description": "JavaScript to evaluate (for evaluate)"},
					"full_page": map[string]any{"type": "boolean", "description": "Capture full page screenshot (for screenshot, default false)"},
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
		return e.callAction(ctx, botID, browserCtxID, arguments)
	case toolBrowserObserve:
		return e.callObserve(ctx, botID, browserCtxID, arguments)
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
	resp, err := e.httpClient.Do(req) //nolint:gosec // URL from internal gateway config
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
		"config": bc.Config,
	})
	createURL := fmt.Sprintf("%s/context", e.gatewayBaseURL)
	createReq, err := http.NewRequestWithContext(ctx, http.MethodPost, createURL, bytes.NewReader(createPayload))
	if err != nil {
		return err
	}
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := e.httpClient.Do(createReq) //nolint:gosec // URL from internal gateway config
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

func (e *Executor) callAction(ctx context.Context, botID, contextID string, arguments map[string]any) (map[string]any, error) {
	action := mcpgw.StringArg(arguments, "action")
	if action == "" {
		return mcpgw.BuildToolErrorResult("action is required"), nil
	}
	payload := map[string]any{"action": action}
	for _, key := range []string{"url", "selector", "text", "key", "value", "target_selector", "direction"} {
		if v := mcpgw.StringArg(arguments, key); v != "" {
			payload[key] = v
		}
	}
	if v, ok, _ := mcpgw.IntArg(arguments, "timeout"); ok {
		payload["timeout"] = v
	}
	if v, ok, _ := mcpgw.IntArg(arguments, "amount"); ok {
		payload["amount"] = v
	}
	if v, ok, _ := mcpgw.IntArg(arguments, "tab_index"); ok {
		payload["tab_index"] = v
	}
	if files, ok := arguments["files"].([]any); ok && len(files) > 0 {
		payload["files"] = files
	}
	return e.doGatewayAction(ctx, botID, contextID, payload)
}

func (e *Executor) callObserve(ctx context.Context, botID, contextID string, arguments map[string]any) (map[string]any, error) {
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
	if v, ok := arguments["full_page"].(bool); ok {
		payload["full_page"] = v
	}
	return e.doGatewayAction(ctx, botID, contextID, payload)
}

func (e *Executor) doGatewayAction(ctx context.Context, botID, contextID string, payload map[string]any) (map[string]any, error) {
	body, _ := json.Marshal(payload)
	actionURL := fmt.Sprintf("%s/context/%s/action", e.gatewayBaseURL, contextID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, actionURL, bytes.NewReader(body))
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req) //nolint:gosec // URL from internal gateway config
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

	if b64, ok := gwResp.Data["screenshot"].(string); ok && b64 != "" {
		return e.buildScreenshotResult(ctx, botID, b64), nil
	}
	return mcpgw.BuildToolSuccessResult(gwResp.Data), nil
}

const screenshotContainerDir = "/data/browser-screenshots"

func (e *Executor) buildScreenshotResult(ctx context.Context, botID, base64Data string) map[string]any {
	mimeType := "image/png"

	imgBytes, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		e.logger.Warn("failed to decode screenshot base64", slog.Any("error", err))
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Screenshot captured (failed to decode for saving)"},
				{"type": "image", "data": base64Data, "mimeType": mimeType},
			},
		}
	}

	containerPath := fmt.Sprintf("%s/%d.png", screenshotContainerDir, time.Now().UnixMilli())

	client, clientErr := e.containers.MCPClient(ctx, botID)
	if clientErr != nil {
		e.logger.Warn("container not reachable for screenshot save", slog.String("bot_id", botID), slog.Any("error", clientErr))
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Screenshot captured (container not reachable, not saved to disk)"},
				{"type": "image", "data": base64Data, "mimeType": mimeType},
			},
		}
	}

	mkdirCmd := fmt.Sprintf("mkdir -p %s", screenshotContainerDir)
	_, _ = client.Exec(ctx, mkdirCmd, "/", 5)

	if writeErr := client.WriteFile(ctx, containerPath, imgBytes); writeErr != nil {
		e.logger.Warn("failed to write screenshot to container", slog.String("bot_id", botID), slog.Any("error", writeErr))
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": fmt.Sprintf("Screenshot captured (failed to save: %s)", writeErr.Error())},
				{"type": "image", "data": base64Data, "mimeType": mimeType},
			},
		}
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": fmt.Sprintf("Screenshot saved to %s", containerPath)},
			{"type": "image", "data": base64Data, "mimeType": mimeType},
		},
	}
}
