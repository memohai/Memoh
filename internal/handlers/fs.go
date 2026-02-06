package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
// @Param id path string true "Container ID"
// @Param payload body object true "JSON-RPC request"
// @Success 200 {object} object "JSON-RPC response: {jsonrpc,id,result|error}"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /container/fs/{id} [post]
func (h *ContainerdHandler) HandleMCPFS(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	containerID := strings.TrimSpace(c.Param("id"))
	if containerID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "container id is required")
	}

	var req mcptools.JSONRPCRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		return c.JSON(http.StatusOK, mcptools.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcptools.JSONRPCError{Code: -32600, Message: "invalid jsonrpc version"},
		})
	}

	if err := h.validateMCPContainer(c.Request().Context(), containerID, botID); err != nil {
		return err
	}
	if err := h.ensureTaskRunning(c.Request().Context(), containerID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	switch req.Method {
	case "tools/list":
		payload, err := h.callMCPServer(c.Request().Context(), containerID, req)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, payload)
	case "tools/call":
		payload, err := h.callMCPServer(c.Request().Context(), containerID, req)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, payload)
	default:
		return c.JSON(http.StatusOK, mcptools.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcptools.JSONRPCError{Code: -32601, Message: "method not found"},
		})
	}
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
		Args: []string{"/mcp"},
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

	go sess.readLoop()
	go func() {
		_, err := execSession.Wait()
		if err != nil {
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
		"/mcp",
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

	go sess.readLoop()
	go func() {
		if err := cmd.Wait(); err != nil {
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
	payloads, targetID, err := buildMCPPayloads(req, &s.initOnce)
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

	s.writeMu.Lock()
	for _, payload := range payloads {
		if _, err := s.stdin.Write([]byte(payload + "\n")); err != nil {
			s.writeMu.Unlock()
			return nil, err
		}
	}
	s.writeMu.Unlock()

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

func buildMCPPayloads(req mcptools.JSONRPCRequest, initOnce *sync.Once) ([]string, json.RawMessage, error) {
	if req.JSONRPC == "" {
		req.JSONRPC = "2.0"
	}
	targetID := req.ID
	payloads := []string{}
	shouldInit := req.Method != "initialize" && req.Method != "notifications/initialized"
	if initOnce != nil {
		ran := false
		initOnce.Do(func() {
			ran = true
		})
		if ran {
			// This is the first call on the session.
		} else {
			shouldInit = false
		}
	}
	if shouldInit {
		initReq := map[string]any{
			"jsonrpc": "2.0",
			"id":      "init-1",
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities": map[string]any{
					"roots": map[string]any{
						"listChanged": false,
					},
				},
				"clientInfo": map[string]any{
					"name":    "memoh-http-proxy",
					"version": "v0",
				},
			},
		}
		initBytes, err := json.Marshal(initReq)
		if err != nil {
			return nil, nil, err
		}
		payloads = append(payloads, string(initBytes))

		initialized := map[string]any{
			"jsonrpc": "2.0",
			"method":  "notifications/initialized",
		}
		initializedBytes, err := json.Marshal(initialized)
		if err != nil {
			return nil, nil, err
		}
		payloads = append(payloads, string(initializedBytes))
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, nil, err
	}
	payloads = append(payloads, string(reqBytes))
	return payloads, targetID, nil
}
