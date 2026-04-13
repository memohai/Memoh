package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/browsercontexts"
	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

type BrowserProvider struct {
	logger          *slog.Logger
	settings        *settings.Service
	browserContexts *browsercontexts.Service
	containers      bridge.Provider
	gatewayBaseURL  string
	httpClient      *http.Client
}

func NewBrowserProvider(log *slog.Logger, settingsSvc *settings.Service, browserSvc *browsercontexts.Service, containers bridge.Provider, gatewayCfg config.BrowserGatewayConfig) *BrowserProvider {
	if log == nil {
		log = slog.Default()
	}
	return &BrowserProvider{
		logger:          log.With(slog.String("tool", "browser")),
		settings:        settingsSvc,
		browserContexts: browserSvc,
		containers:      containers,
		gatewayBaseURL:  strings.TrimRight(gatewayCfg.BaseURL(), "/"),
		httpClient:      &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *BrowserProvider) Tools(ctx context.Context, session SessionContext) ([]sdk.Tool, error) {
	if session.IsSubagent || p.settings == nil || p.browserContexts == nil {
		return nil, nil
	}
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return nil, nil
	}
	botSettings, err := p.settings.GetBot(ctx, botID)
	if err != nil {
		return nil, nil
	}
	if strings.TrimSpace(botSettings.BrowserContextID) == "" {
		return nil, nil
	}
	sess := session
	return []sdk.Tool{
		{
			Name:        "browser_action",
			Description: "Execute a browser action: navigate, click, double-click, focus, type, fill, press key, keyboard input, hover, select option, check/uncheck, scroll, drag-and-drop, upload files, go back/forward, reload, wait, or manage tabs (new/select/close).",
			Parameters: map[string]any{
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
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execAction(ctx.Context, sess, inputAsMap(input))
			},
		},
		{
			Name:        "browser_observe",
			Description: "Observe the current browser page: take screenshot (optionally annotated with numbered element labels or full-page), get accessibility tree snapshot, get text content, get HTML, evaluate JavaScript, get current URL, get page title, export PDF, or list open tabs.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"observe":   map[string]any{"type": "string", "enum": []string{"screenshot", "screenshot_annotate", "snapshot", "get_content", "get_html", "evaluate", "get_url", "get_title", "pdf", "tab_list"}, "description": "What to observe from the page"},
					"selector":  map[string]any{"type": "string", "description": "CSS selector to scope the observation"},
					"script":    map[string]any{"type": "string", "description": "JavaScript to evaluate (for evaluate)"},
					"full_page": map[string]any{"type": "boolean", "description": "Capture full page screenshot (for screenshot, default false)"},
				},
				"required": []string{"observe"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execObserve(ctx.Context, sess, inputAsMap(input))
			},
		},
		{
			Name:        "browser_remote_session",
			Description: "Manage a remote native Playwright session for full browser automation. Use 'create' to get a WebSocket endpoint that a Python Playwright client can connect to with full API access (including HttpOnly cookies, storage state, route interception, etc). Use 'close' to terminate a session. Use 'status' to check a session.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":        map[string]any{"type": "string", "enum": []string{"create", "close", "status"}, "description": "The session action to perform"},
					"session_id":    map[string]any{"type": "string", "description": "Session ID (required for close and status)"},
					"session_token": map[string]any{"type": "string", "description": "Session token (required for close and status, returned by create)"},
					"core":          map[string]any{"type": "string", "enum": []string{"chromium", "firefox"}, "description": "Browser core to use (for create, default: chromium)"},
				},
				"required": []string{"action"},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execRemoteSession(ctx.Context, sess, inputAsMap(input))
			},
		},
	}, nil
}

func (p *BrowserProvider) resolveContext(ctx context.Context, botID string) (string, browsercontexts.BrowserContext, error) {
	botSettings, err := p.settings.GetBot(ctx, botID)
	if err != nil {
		return "", browsercontexts.BrowserContext{}, err
	}
	browserCtxID := strings.TrimSpace(botSettings.BrowserContextID)
	if browserCtxID == "" {
		return "", browsercontexts.BrowserContext{}, errors.New("browser context not configured for this bot")
	}
	bcConfig, err := p.browserContexts.GetByID(ctx, browserCtxID)
	if err != nil {
		return "", browsercontexts.BrowserContext{}, fmt.Errorf("failed to load browser context config: %s", err.Error())
	}
	if err := p.ensureContext(ctx, botID, browserCtxID, bcConfig); err != nil {
		return "", browsercontexts.BrowserContext{}, fmt.Errorf("failed to ensure browser context: %s", err.Error())
	}
	return browserCtxID, bcConfig, nil
}

func (p *BrowserProvider) execAction(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return nil, errors.New("bot_id is required")
	}
	contextID, _, err := p.resolveContext(ctx, botID)
	if err != nil {
		return nil, err
	}
	action := StringArg(args, "action")
	if action == "" {
		return nil, errors.New("action is required")
	}
	payload := map[string]any{"action": action}
	for _, key := range []string{"url", "selector", "text", "key", "value", "target_selector", "direction"} {
		if v := StringArg(args, key); v != "" {
			payload[key] = v
		}
	}
	if v, ok, _ := IntArg(args, "timeout"); ok {
		payload["timeout"] = v
	}
	if v, ok, _ := IntArg(args, "amount"); ok {
		payload["amount"] = v
	}
	if v, ok, _ := IntArg(args, "tab_index"); ok {
		payload["tab_index"] = v
	}
	if files, ok := args["files"].([]any); ok && len(files) > 0 {
		payload["files"] = files
	}
	return p.doGatewayAction(ctx, session, botID, contextID, payload)
}

func (p *BrowserProvider) execObserve(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return nil, errors.New("bot_id is required")
	}
	contextID, _, err := p.resolveContext(ctx, botID)
	if err != nil {
		return nil, err
	}
	observe := StringArg(args, "observe")
	if observe == "" {
		return nil, errors.New("observe is required")
	}
	payload := map[string]any{"action": observe}
	if v := StringArg(args, "selector"); v != "" {
		payload["selector"] = v
	}
	if v := StringArg(args, "script"); v != "" {
		payload["script"] = v
	}
	if v, ok := args["full_page"].(bool); ok {
		payload["full_page"] = v
	}
	return p.doGatewayAction(ctx, session, botID, contextID, payload)
}

func (p *BrowserProvider) ensureContext(ctx context.Context, botID, contextID string, bc browsercontexts.BrowserContext) error {
	existsURL := fmt.Sprintf("%s/context/%s/exists", p.gatewayBaseURL, contextID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, existsURL, nil)
	if err != nil {
		return err
	}
	resp, err := p.httpClient.Do(req) //nolint:gosec
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
	createPayload, _ := json.Marshal(map[string]any{"id": contextID, "name": bc.Name, "config": bc.Config, "bot_id": botID})
	createURL := fmt.Sprintf("%s/context", p.gatewayBaseURL)
	createReq, err := http.NewRequestWithContext(ctx, http.MethodPost, createURL, bytes.NewReader(createPayload))
	if err != nil {
		return err
	}
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := p.httpClient.Do(createReq) //nolint:gosec
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

func (p *BrowserProvider) doGatewayAction(ctx context.Context, session SessionContext, botID, contextID string, payload map[string]any) (any, error) {
	body, _ := json.Marshal(payload)
	actionURL := fmt.Sprintf("%s/context/%s/action", p.gatewayBaseURL, contextID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, actionURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient.Do(req) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("browser gateway request failed: %s", err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	var gwResp struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
		Error   string         `json:"error"`
	}
	if err := json.Unmarshal(respBody, &gwResp); err != nil {
		return nil, errors.New("invalid gateway response")
	}
	if !gwResp.Success {
		errMsg := gwResp.Error
		if errMsg == "" {
			errMsg = "browser action failed"
		}
		return nil, fmt.Errorf("%s", errMsg)
	}
	if b64, ok := gwResp.Data["screenshot"].(string); ok && b64 != "" {
		return p.buildScreenshotResult(ctx, botID, b64, session), nil
	}
	return gwResp.Data, nil
}

const browserScreenshotDir = "/data/browser-screenshots"

func (p *BrowserProvider) buildScreenshotResult(ctx context.Context, botID, base64Data string, session SessionContext) any {
	mimeType := "image/png"
	imgBytes, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Screenshot captured (failed to decode image data)"},
			},
		}
	}

	// Emit the screenshot as attachment for user delivery (once).
	p.emitScreenshotAttachment(session, base64Data, mimeType, int64(len(imgBytes)))

	containerPath := fmt.Sprintf("%s/%d.png", browserScreenshotDir, time.Now().UnixMilli())
	client, clientErr := p.containers.MCPClient(ctx, botID)
	if clientErr != nil {
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Screenshot captured and sent to user (container not reachable, not saved to disk)"},
			},
		}
	}
	mkdirCmd := fmt.Sprintf("mkdir -p %s", browserScreenshotDir)
	_, _ = client.Exec(ctx, mkdirCmd, "/", 5)
	if writeErr := client.WriteFile(ctx, containerPath, imgBytes); writeErr != nil {
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": fmt.Sprintf("Screenshot captured and sent to user (failed to save: %s)", writeErr.Error())},
			},
		}
	}
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": fmt.Sprintf("Screenshot saved to %s and sent to user", containerPath)},
		},
	}
}

// emitScreenshotAttachment pushes the screenshot as an image attachment into the
// agent stream so it gets delivered to the user's chat.
func (*BrowserProvider) emitScreenshotAttachment(session SessionContext, base64Data, mimeType string, size int64) {
	if session.Emitter == nil {
		return
	}
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)
	session.Emitter(ToolStreamEvent{
		Type: StreamEventAttachment,
		Attachments: []Attachment{{
			Type: "image",
			URL:  dataURL,
			Mime: mimeType,
			Size: size,
		}},
	})
}

func (p *BrowserProvider) execRemoteSession(ctx context.Context, session SessionContext, args map[string]any) (any, error) {
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return nil, errors.New("bot_id is required")
	}
	// Same access gate as browser_action/browser_observe
	_, bcConfig, err := p.resolveContext(ctx, botID)
	if err != nil {
		return nil, err
	}

	action := StringArg(args, "action")
	switch action {
	case "create":
		return p.createRemoteSession(ctx, botID, bcConfig, args)
	case "close":
		sessionID := StringArg(args, "session_id")
		sessionToken := StringArg(args, "session_token")
		if sessionID == "" {
			return nil, errors.New("session_id is required for close")
		}
		if sessionToken == "" {
			return nil, errors.New("session_token is required for close")
		}
		return p.closeRemoteSession(ctx, sessionID, sessionToken)
	case "status":
		sessionID := StringArg(args, "session_id")
		sessionToken := StringArg(args, "session_token")
		if sessionID == "" {
			return nil, errors.New("session_id is required for status")
		}
		if sessionToken == "" {
			return nil, errors.New("session_token is required for status")
		}
		return p.getRemoteSessionStatus(ctx, sessionID, sessionToken)
	default:
		return nil, fmt.Errorf("unknown session action: %s", action)
	}
}

func (p *BrowserProvider) createRemoteSession(ctx context.Context, botID string, bcConfig browsercontexts.BrowserContext, args map[string]any) (any, error) {
	core := StringArg(args, "core")
	if core == "" {
		core = "chromium"
	}
	payload, _ := json.Marshal(map[string]any{
		"bot_id":         botID,
		"core":           core,
		"context_config": bcConfig.Config,
	})
	url := fmt.Sprintf("%s/session", p.gatewayBaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient.Do(req) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to create remote session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("create session failed (HTTP %d): %s", resp.StatusCode, string(body))
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.New("invalid session response")
	}
	return result, nil
}

func (p *BrowserProvider) closeRemoteSession(ctx context.Context, sessionID, sessionToken string) (any, error) {
	reqURL := fmt.Sprintf("%s/session/%s?token=%s", p.gatewayBaseURL, sessionID, sessionToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(req) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to close remote session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("close session failed (HTTP %d): %s", resp.StatusCode, string(body))
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.New("invalid session response")
	}
	return result, nil
}

func (p *BrowserProvider) getRemoteSessionStatus(ctx context.Context, sessionID, sessionToken string) (any, error) {
	reqURL := fmt.Sprintf("%s/session/%s?token=%s", p.gatewayBaseURL, sessionID, sessionToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(req) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to get remote session status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("get session status failed (HTTP %d): %s", resp.StatusCode, string(body))
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.New("invalid session response")
	}
	return result, nil
}
