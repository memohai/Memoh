package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/errdefs"
	"github.com/labstack/echo/v4"

	ctr "github.com/memohai/memoh/internal/containerd"
	mcptools "github.com/memohai/memoh/internal/mcp"
)

// HandleMCPFS godoc
// @Summary MCP filesystem tools (JSON-RPC)
// @Description Forwards MCP JSON-RPC requests to the MCP server inside the container.
// @Description Required:
// @Description - container task is running
// @Description - container has data mount (default /data) bound to <data_root>/users/<user_id>
// @Description - container image contains the "mcp" binary
// @Description Auth: Bearer JWT is used to determine user_id (sub or user_id).
// @Description Paths must be relative (no leading slash) and must not contain "..".
// @Description
// @Description Example: tools/list
// @Description {"jsonrpc":"2.0","id":1,"method":"tools/list"}
// @Description
// @Description Example: tools/call (fs.read)
// @Description {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"fs.read","arguments":{"path":"notes.txt"}}}
// @Tags containerd
// @Param Authorization header string true "Bearer <token>"
// @Param bot_id path string true "Bot ID"
// @Param payload body object true "JSON-RPC request"
// @Success 200 {object} object "JSON-RPC response: {jsonrpc,id,result|error}"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/fs [post]
func (h *ContainerdHandler) HandleMCPFS(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	containerID, err := h.botContainerID(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "container not found for bot")
	}

	var req mcptools.JSONRPCRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		return c.JSON(http.StatusOK, mcptools.JSONRPCErrorResponse(req.ID, -32600, "invalid jsonrpc version"))
	}

	if err := h.validateMCPContainer(ctx, containerID, botID); err != nil {
		h.logger.Error("mcp fs validate failed", slog.Any("error", err), slog.String("bot_id", botID), slog.String("container_id", containerID))
		return c.JSON(http.StatusOK, mcptools.JSONRPCErrorResponse(req.ID, -32603, err.Error()))
	}
	if err := h.ensureTaskRunning(ctx, containerID); err != nil {
		h.logger.Error("mcp fs ensure task failed", slog.Any("error", err), slog.String("bot_id", botID), slog.String("container_id", containerID))
		return c.JSON(http.StatusOK, mcptools.JSONRPCErrorResponse(req.ID, -32603, err.Error()))
	}

	if strings.TrimSpace(req.Method) == "" {
		return c.JSON(http.StatusOK, mcptools.JSONRPCErrorResponse(req.ID, -32601, "method not found"))
	}
	if mcptools.IsNotification(req) {
		if err := h.notifyMCPServer(ctx, containerID, req); err != nil {
			h.logger.Error("mcp fs notify failed", slog.Any("error", err), slog.String("method", req.Method), slog.String("bot_id", botID), slog.String("container_id", containerID))
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		// MCP Streamable HTTP spec: notifications must be answered with 202 Accepted and no body.
		return c.NoContent(http.StatusAccepted)
	}
	payload, err := h.callMCPServer(ctx, containerID, req)
	if err != nil {
		h.logger.Error("mcp fs call failed", slog.Any("error", err), slog.String("method", req.Method), slog.String("bot_id", botID), slog.String("container_id", containerID))
		return c.JSON(http.StatusOK, mcptools.JSONRPCErrorResponse(req.ID, -32603, err.Error()))
	}
	return c.JSON(http.StatusOK, payload)
}

func (h *ContainerdHandler) validateMCPContainer(ctx context.Context, containerID, botID string) error {
	if strings.TrimSpace(botID) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	container, err := h.service.GetContainer(ctx, containerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return echo.NewHTTPError(http.StatusNotFound, "container not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	infoCtx := ctx
	if strings.TrimSpace(h.namespace) != "" {
		infoCtx = namespaces.WithNamespace(ctx, h.namespace)
	}
	info, err := container.Info(infoCtx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	labelBotID := strings.TrimSpace(info.Labels[mcptools.BotLabelKey])
	if labelBotID != "" && labelBotID != botID {
		return echo.NewHTTPError(http.StatusForbidden, "bot mismatch")
	}
	return nil
}

func (h *ContainerdHandler) callMCPServer(ctx context.Context, containerID string, req mcptools.JSONRPCRequest) (map[string]any, error) {
	session, err := h.getMCPSession(ctx, containerID)
	if err != nil {
		return nil, err
	}
	return session.call(ctx, req)
}

func (h *ContainerdHandler) notifyMCPServer(ctx context.Context, containerID string, req mcptools.JSONRPCRequest) error {
	session, err := h.getMCPSession(ctx, containerID)
	if err != nil {
		return err
	}
	return session.notify(ctx, req)
}

type mcpSession struct {
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	cmd       *exec.Cmd
	initOnce  sync.Once
	writeMu   sync.Mutex
	pendingMu sync.Mutex
	pending   map[string]chan mcptools.JSONRPCResponse
	closed    chan struct{}
	closeOnce sync.Once
	closeErr  error
	onClose   func()
}

func (h *ContainerdHandler) getMCPSession(ctx context.Context, containerID string) (*mcpSession, error) {
	h.mcpMu.Lock()
	if sess, ok := h.mcpSess[containerID]; ok {
		h.mcpMu.Unlock()
		return sess, nil
	}
	h.mcpMu.Unlock()

	var sess *mcpSession
	var err error
	if runtime.GOOS == "darwin" {
		sess, err = h.startLimaMCPSession(containerID)
	}
	if err != nil || sess == nil {
		sess, err = h.startContainerdMCPSession(ctx, containerID)
		if err != nil {
			return nil, err
		}
	}

	h.mcpMu.Lock()
	h.mcpSess[containerID] = sess
	h.mcpMu.Unlock()

	sess.onClose = func() {
		h.mcpMu.Lock()
		if current, ok := h.mcpSess[containerID]; ok && current == sess {
			delete(h.mcpSess, containerID)
		}
		h.mcpMu.Unlock()
	}

	return sess, nil
}

func (h *ContainerdHandler) startContainerdMCPSession(ctx context.Context, containerID string) (*mcpSession, error) {
	execSession, err := h.service.ExecTaskStreaming(ctx, containerID, ctr.ExecTaskRequest{
		Args: []string{"/app/mcp"},
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
			h.logger.Error("mcp session exited", slog.Any("error", err), slog.String("container_id", containerID))
			sess.closeWithError(err)
		} else {
			sess.closeWithError(io.EOF)
		}
	}()

	return sess, nil
}

func (h *ContainerdHandler) startLimaMCPSession(containerID string) (*mcpSession, error) {
	execID := fmt.Sprintf("mcp-%d", time.Now().UnixNano())
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
		"/app/mcp",
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
			h.logger.Error("mcp session exited", slog.Any("error", err), slog.String("container_id", containerID))
			sess.closeWithError(err)
		} else {
			sess.closeWithError(io.EOF)
		}
	}()

	return sess, nil
}

func (s *mcpSession) closeWithError(err error) {
	s.closeOnce.Do(func() {
		s.closeErr = err
		close(s.closed)
		s.pendingMu.Lock()
		for _, ch := range s.pending {
			close(ch)
		}
		s.pending = map[string]chan mcptools.JSONRPCResponse{}
		s.pendingMu.Unlock()
		_ = s.stdin.Close()
		_ = s.stdout.Close()
		_ = s.stderr.Close()
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		if s.onClose != nil {
			s.onClose()
		}
	})
}

func (h *ContainerdHandler) startMCPStderrLogger(stderr io.ReadCloser, containerID string) {
	if stderr == nil {
		return
	}
	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			h.logger.Warn("mcp stderr", slog.String("container_id", containerID), slog.String("message", line))
		}
		if err := scanner.Err(); err != nil {
			h.logger.Error("mcp stderr read failed", slog.Any("error", err), slog.String("container_id", containerID))
		}
	}()
}

func (s *mcpSession) readLoop() {
	scanner := bufio.NewScanner(s.stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var resp mcptools.JSONRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}
		id := strings.TrimSpace(string(resp.ID))
		if id == "" {
			continue
		}
		s.pendingMu.Lock()
		ch, ok := s.pending[id]
		if ok {
			delete(s.pending, id)
		}
		s.pendingMu.Unlock()
		if ok {
			ch <- resp
			close(ch)
		}
	}
	if err := scanner.Err(); err != nil {
		s.closeWithError(err)
	} else {
		s.closeWithError(io.EOF)
	}
}

func (s *mcpSession) call(ctx context.Context, req mcptools.JSONRPCRequest) (map[string]any, error) {
	payloads, targetID, err := mcptools.BuildPayloads(req, &s.initOnce)
	if err != nil {
		return nil, err
	}
	target := strings.TrimSpace(string(targetID))
	if target == "" {
		return nil, fmt.Errorf("missing request id")
	}

	respCh := make(chan mcptools.JSONRPCResponse, 1)
	s.pendingMu.Lock()
	s.pending[target] = respCh
	s.pendingMu.Unlock()

	if err := s.writePayloads(payloads); err != nil {
		return nil, err
	}

	select {
	case resp, ok := <-respCh:
		if !ok {
			if s.closeErr != nil {
				return nil, s.closeErr
			}
			return nil, io.EOF
		}
		if resp.Error != nil {
			return map[string]any{
				"jsonrpc": "2.0",
				"id":      resp.ID,
				"error": map[string]any{
					"code":    resp.Error.Code,
					"message": resp.Error.Message,
				},
			}, nil
		}
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      resp.ID,
			"result":  resp.Result,
		}, nil
	case <-s.closed:
		if s.closeErr != nil {
			return nil, s.closeErr
		}
		return nil, io.EOF
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *mcpSession) notify(ctx context.Context, req mcptools.JSONRPCRequest) error {
	payloads, err := mcptools.BuildNotificationPayloads(req)
	if err != nil {
		return err
	}
	return s.writePayloads(payloads)
}

func (s *mcpSession) writePayloads(payloads []string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	for _, payload := range payloads {
		if _, err := s.stdin.Write([]byte(payload + "\n")); err != nil {
			return err
		}
	}
	return nil
}

