package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	ctr "github.com/memohai/memoh/internal/containerd"
	mcptools "github.com/memohai/memoh/internal/mcp"
)

type MCPStdioRequest struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Cwd     string            `json:"cwd"`
}

type MCPStdioResponse struct {
	SessionID string   `json:"session_id"`
	URL       string   `json:"url"`
	Tools     []string `json:"tools,omitempty"`
}

type mcpStdioSession struct {
	id          string
	botID       string
	containerID string
	name        string
	createdAt   time.Time
	lastUsedAt  time.Time
	session     *mcpSession
}

// CreateMCPStdio godoc
// @Summary Create MCP stdio proxy
// @Description Start a stdio MCP process in the bot container and expose it as MCP HTTP endpoint.
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body MCPStdioRequest true "Stdio MCP payload"
// @Success 200 {object} MCPStdioResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/mcp-stdio [post]
func (h *ContainerdHandler) CreateMCPStdio(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	var req MCPStdioRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.Command) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "command is required")
	}
	ctx := c.Request().Context()
	containerID, err := h.botContainerID(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "container not found for bot")
	}
	if err := h.validateMCPContainer(ctx, containerID, botID); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := h.ensureTaskRunning(ctx, containerID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	sess, err := h.startContainerdMCPCommandSession(ctx, containerID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	tools := h.probeMCPTools(ctx, sess, botID, strings.TrimSpace(req.Name))
	sessionID := uuid.NewString()
	record := &mcpStdioSession{
		id:          sessionID,
		botID:       botID,
		containerID: containerID,
		name:        strings.TrimSpace(req.Name),
		createdAt:   time.Now().UTC(),
		lastUsedAt:  time.Now().UTC(),
		session:     sess,
	}
	sess.onClose = func() {
		h.mcpStdioMu.Lock()
		if current, ok := h.mcpStdioSess[sessionID]; ok && current == record {
			delete(h.mcpStdioSess, sessionID)
		}
		h.mcpStdioMu.Unlock()
	}
	h.mcpStdioMu.Lock()
	h.mcpStdioSess[sessionID] = record
	h.mcpStdioMu.Unlock()

	return c.JSON(http.StatusOK, MCPStdioResponse{
		SessionID: sessionID,
		URL:       fmt.Sprintf("/bots/%s/mcp-stdio/%s", botID, sessionID),
		Tools:     tools,
	})
}

// HandleMCPStdio godoc
// @Summary MCP stdio proxy (JSON-RPC)
// @Description Proxies MCP JSON-RPC requests to a stdio MCP process in the container.
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param session_id path string true "Session ID"
// @Param payload body object true "JSON-RPC request"
// @Success 200 {object} object "JSON-RPC response: {jsonrpc,id,result|error}"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/mcp-stdio/{session_id} [post]
func (h *ContainerdHandler) HandleMCPStdio(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session_id is required")
	}
	h.mcpStdioMu.Lock()
	session := h.mcpStdioSess[sessionID]
	h.mcpStdioMu.Unlock()
	if session == nil || session.session == nil || session.botID != botID {
		return echo.NewHTTPError(http.StatusNotFound, "mcp session not found")
	}
	select {
	case <-session.session.closed:
		return echo.NewHTTPError(http.StatusNotFound, "mcp session closed")
	default:
	}

	var req mcptools.JSONRPCRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		return c.JSON(http.StatusOK, mcptools.JSONRPCErrorResponse(req.ID, -32600, "invalid jsonrpc version"))
	}
	if strings.TrimSpace(req.Method) == "" {
		return c.JSON(http.StatusOK, mcptools.JSONRPCErrorResponse(req.ID, -32601, "method not found"))
	}
	session.lastUsedAt = time.Now().UTC()
	if mcptools.IsNotification(req) {
		if err := session.session.notify(c.Request().Context(), req); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.NoContent(http.StatusAccepted)
	}
	payload, err := session.session.call(c.Request().Context(), req)
	if err != nil {
		return c.JSON(http.StatusOK, mcptools.JSONRPCErrorResponse(req.ID, -32603, err.Error()))
	}
	return c.JSON(http.StatusOK, payload)
}

func (h *ContainerdHandler) startContainerdMCPCommandSession(ctx context.Context, containerID string, req MCPStdioRequest) (*mcpSession, error) {
	if runtime.GOOS == "darwin" {
		return h.startLimaMCPCommandSession(containerID, req)
	}
	args := append([]string{strings.TrimSpace(req.Command)}, req.Args...)
	env := buildEnvPairs(req.Env)
	execSession, err := h.service.ExecTaskStreaming(ctx, containerID, ctr.ExecTaskRequest{
		Args:    args,
		Env:     env,
		WorkDir: strings.TrimSpace(req.Cwd),
	})
	if err != nil {
		return nil, err
	}

	sess := &mcpSession{
		stdin:   execSession.Stdin,
		stdout:  execSession.Stdout,
		stderr:  execSession.Stderr,
		pending: make(map[string]chan mcptools.JSONRPCResponse),
		closed:  make(chan struct{}),
	}
	h.startMCPStderrLogger(execSession.Stderr, containerID)
	go sess.readLoop()
	go func() {
		_, err := execSession.Wait()
		if err != nil {
			h.logger.Error("mcp stdio session exited", slog.Any("error", err), slog.String("container_id", containerID))
			sess.closeWithError(err)
		} else {
			sess.closeWithError(io.EOF)
		}
	}()
	return sess, nil
}

func buildEnvPairs(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		if strings.TrimSpace(k) != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s=%s", k, env[k]))
	}
	return out
}

func (h *ContainerdHandler) probeMCPTools(ctx context.Context, sess *mcpSession, botID, name string) []string {
	if sess == nil {
		return nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	payload, err := sess.call(probeCtx, mcptools.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      mcptools.RawStringID("probe-tools"),
		Method:  "tools/list",
	})
	if err != nil {
		h.logger.Warn("mcp stdio tools probe failed",
			slog.String("bot_id", botID),
			slog.String("name", name),
			slog.Any("error", err),
		)
		return nil
	}
	tools := extractToolNames(payload)
	if len(tools) == 0 {
		h.logger.Warn("mcp stdio tools empty",
			slog.String("bot_id", botID),
			slog.String("name", name),
		)
	} else {
		h.logger.Info("mcp stdio tools loaded",
			slog.String("bot_id", botID),
			slog.String("name", name),
			slog.Int("count", len(tools)),
		)
	}
	return tools
}

func extractToolNames(payload map[string]any) []string {
	result, ok := payload["result"].(map[string]any)
	if !ok {
		return nil
	}
	rawTools, ok := result["tools"].([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(rawTools))
	for _, raw := range rawTools {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := item["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (h *ContainerdHandler) startLimaMCPCommandSession(containerID string, req MCPStdioRequest) (*mcpSession, error) {
	execID := fmt.Sprintf("mcp-stdio-%d", time.Now().UnixNano())
	cmdline := buildShellCommand(req)
	cmd := exec.Command(
		"limactl",
		"shell",
		"--tty=false",
		"default",
		"--",
		"sudo",
		"-n",
		"ctr",
		"-n",
		"default",
		"tasks",
		"exec",
		"--exec-id",
		execID,
		containerID,
		"/bin/sh",
		"-lc",
		cmdline,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}

	sess := &mcpSession{
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		cmd:     cmd,
		pending: make(map[string]chan mcptools.JSONRPCResponse),
		closed:  make(chan struct{}),
	}

	h.startMCPStderrLogger(stderr, containerID)
	go sess.readLoop()
	go func() {
		if err := cmd.Wait(); err != nil {
			h.logger.Error("mcp stdio session exited", slog.Any("error", err), slog.String("container_id", containerID))
			sess.closeWithError(err)
		} else {
			sess.closeWithError(io.EOF)
		}
	}()

	return sess, nil
}

func buildShellCommand(req MCPStdioRequest) string {
	cmd := strings.TrimSpace(req.Command)
	if cmd == "" {
		return ""
	}
	parts := make([]string, 0, len(req.Args)+1)
	parts = append(parts, escapeShellArg(cmd))
	for _, arg := range req.Args {
		parts = append(parts, escapeShellArg(arg))
	}
	command := strings.Join(parts, " ")

	assignments := []string{}
	for _, pair := range buildEnvPairs(req.Env) {
		assignments = append(assignments, escapeShellArg(pair))
	}
	if len(assignments) > 0 {
		command = strings.Join(assignments, " ") + " " + command
	}
	if strings.TrimSpace(req.Cwd) != "" {
		command = "cd " + escapeShellArg(req.Cwd) + " && " + command
	}
	return command
}

func escapeShellArg(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n'\"\\$&;|<>*?()[]{}!`") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
