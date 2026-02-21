package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/containerd/errdefs"
	"github.com/labstack/echo/v4"
	sdkjsonrpc "github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	ctr "github.com/memohai/memoh/internal/containerd"
	mcptools "github.com/memohai/memoh/internal/mcp"
)

func (h *ContainerdHandler) validateMCPContainer(ctx context.Context, containerID, botID string) error {
	if strings.TrimSpace(botID) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	info, err := h.service.GetContainer(ctx, containerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return echo.NewHTTPError(http.StatusNotFound, "container not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	labelBotID := strings.TrimSpace(info.Labels[mcptools.BotLabelKey])
	if labelBotID != "" && labelBotID != botID {
		return echo.NewHTTPError(http.StatusForbidden, "bot mismatch")
	}
	return nil
}

type mcpSession struct {
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	cmd       *exec.Cmd
	initMu    sync.Mutex
	initState mcpSessionInitState
	initWait  chan struct{}
	pendingMu sync.Mutex
	pending   map[string]chan *sdkjsonrpc.Response
	conn      sdkmcp.Connection
	closed    chan struct{}
	closeOnce sync.Once
	closeErr  error
	onClose   func()
}

type mcpSessionInitState uint8

const (
	mcpSessionInitStateNone mcpSessionInitState = iota
	mcpSessionInitStateInitializing
	mcpSessionInitStateInitialized
	mcpSessionInitStateReady
)

func (h *ContainerdHandler) getMCPSession(ctx context.Context, containerID string) (*mcpSession, error) {
	h.mcpMu.Lock()
	if sess, ok := h.mcpSess[containerID]; ok {
		h.mcpMu.Unlock()
		return sess, nil
	}
	h.mcpMu.Unlock()

	sess, err := h.startContainerdMCPSession(ctx, containerID)
	if err != nil {
		return nil, err
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
		Args:    []string{"/app/mcp"},
		FIFODir: h.mcpFIFODir(),
	})
	if err != nil {
		return nil, err
	}

	sess := &mcpSession{
		stdin:   execSession.Stdin,
		stdout:  execSession.Stdout,
		stderr:  execSession.Stderr,
		pending: make(map[string]chan *sdkjsonrpc.Response),
		closed:  make(chan struct{}),
	}
	transport := &sdkmcp.IOTransport{
		Reader: sess.stdout,
		Writer: sess.stdin,
	}
	conn, err := transport.Connect(ctx)
	if err != nil {
		sess.closeWithError(err)
		return nil, err
	}
	sess.conn = conn

	h.startMCPStderrLogger(execSession.Stderr, containerID)
	go sess.readLoop()
	go func() {
		_, err := execSession.Wait()
		if err != nil {
			if isBenignMCPSessionExit(err) {
				sess.closeWithError(io.EOF)
				return
			}
			h.logger.Error("mcp session exited", slog.Any("error", err), slog.String("container_id", containerID))
			sess.closeWithError(err)
			return
		}
		sess.closeWithError(io.EOF)
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
		s.pending = map[string]chan *sdkjsonrpc.Response{}
		s.pendingMu.Unlock()
		if s.conn != nil {
			_ = s.conn.Close()
		}
		if s.stdin != nil {
			_ = s.stdin.Close()
		}
		if s.stdout != nil {
			_ = s.stdout.Close()
		}
		if s.stderr != nil {
			_ = s.stderr.Close()
		}
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
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || strings.Contains(err.Error(), "closed pipe") {
				return
			}
			h.logger.Error("mcp stderr read failed", slog.Any("error", err), slog.String("container_id", containerID))
		}
	}()
}

func isBenignMCPSessionExit(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "code = canceled") || strings.Contains(msg, "context canceled") || strings.Contains(msg, "closed pipe")
}

func (h *ContainerdHandler) mcpFIFODir() string {
	if root := strings.TrimSpace(h.cfg.DataRoot); root != "" {
		return filepath.Join(root, ".containerd-fifo")
	}
	return "/tmp/memoh-containerd-fifo"
}

func (s *mcpSession) readLoop() {
	if s.conn == nil {
		s.closeWithError(io.EOF)
		return
	}
	for {
		msg, err := s.conn.Read(context.Background())
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.closeWithError(io.EOF)
				return
			}
			s.closeWithError(err)
			return
		}
		resp, ok := msg.(*sdkjsonrpc.Response)
		if !ok || !resp.ID.IsValid() {
			continue
		}
		id := sdkIDKey(resp.ID)
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
}

func (s *mcpSession) call(ctx context.Context, req mcptools.JSONRPCRequest) (map[string]any, error) {
	method := strings.TrimSpace(req.Method)
	if method == "initialize" {
		return s.callInitialize(ctx, req)
	}
	if method != "notifications/initialized" {
		if err := s.ensureInitialized(ctx); err != nil {
			return nil, err
		}
	}

	targetID, err := parseRawJSONRPCID(req.ID)
	if err != nil {
		return nil, err
	}
	target := sdkIDKey(targetID)
	if target == "" {
		return nil, fmt.Errorf("missing request id")
	}
	if s.conn == nil {
		return nil, io.EOF
	}

	respCh := make(chan *sdkjsonrpc.Response, 1)
	s.pendingMu.Lock()
	s.pending[target] = respCh
	s.pendingMu.Unlock()

	callReq := &sdkjsonrpc.Request{
		ID:     targetID,
		Method: method,
		Params: req.Params,
	}
	if err := s.conn.Write(ctx, callReq); err != nil {
		s.removePending(target)
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
		if method == "notifications/initialized" {
			s.setInitStateAtLeast(mcpSessionInitStateReady)
		}
		return sdkResponsePayload(resp)
	case <-s.closed:
		if s.closeErr != nil {
			return nil, s.closeErr
		}
		return nil, io.EOF
	case <-ctx.Done():
		s.removePending(target)
		return nil, ctx.Err()
	}
}

func (s *mcpSession) callInitialize(ctx context.Context, req mcptools.JSONRPCRequest) (map[string]any, error) {
	payload, err := s.callRaw(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := mcptools.PayloadError(payload); err != nil {
		return payload, nil
	}
	s.setInitStateAtLeast(mcpSessionInitStateInitialized)
	return payload, nil
}

func (s *mcpSession) callRaw(ctx context.Context, req mcptools.JSONRPCRequest) (map[string]any, error) {
	method := strings.TrimSpace(req.Method)
	targetID, err := parseRawJSONRPCID(req.ID)
	if err != nil {
		return nil, err
	}
	target := sdkIDKey(targetID)
	if target == "" {
		return nil, fmt.Errorf("missing request id")
	}
	if s.conn == nil {
		return nil, io.EOF
	}

	respCh := make(chan *sdkjsonrpc.Response, 1)
	s.pendingMu.Lock()
	s.pending[target] = respCh
	s.pendingMu.Unlock()

	callReq := &sdkjsonrpc.Request{
		ID:     targetID,
		Method: method,
		Params: req.Params,
	}
	if err := s.conn.Write(ctx, callReq); err != nil {
		s.removePending(target)
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
		return sdkResponsePayload(resp)
	case <-s.closed:
		if s.closeErr != nil {
			return nil, s.closeErr
		}
		return nil, io.EOF
	case <-ctx.Done():
		s.removePending(target)
		return nil, ctx.Err()
	}
}

func (s *mcpSession) notify(ctx context.Context, req mcptools.JSONRPCRequest) error {
	if s.conn == nil {
		return io.EOF
	}
	method := strings.TrimSpace(req.Method)
	notification := &sdkjsonrpc.Request{
		Method: method,
		Params: req.Params,
	}
	if err := s.conn.Write(ctx, notification); err != nil {
		return err
	}
	if method == "notifications/initialized" {
		s.setInitStateAtLeast(mcpSessionInitStateReady)
	}
	return nil
}

func (s *mcpSession) ensureInitialized(ctx context.Context) error {
	for {
		s.initMu.Lock()
		switch s.initState {
		case mcpSessionInitStateReady:
			s.initMu.Unlock()
			return nil
		case mcpSessionInitStateInitializing:
			waitCh := s.initWait
			s.initMu.Unlock()
			if waitCh == nil {
				continue
			}
			select {
			case <-waitCh:
				continue
			case <-ctx.Done():
				return ctx.Err()
			case <-s.closed:
				if s.closeErr != nil {
					return s.closeErr
				}
				return io.EOF
			}
		case mcpSessionInitStateInitialized:
			waitCh := make(chan struct{})
			s.initState = mcpSessionInitStateInitializing
			s.initWait = waitCh
			s.initMu.Unlock()

			err := s.sendInitializedNotification(ctx)

			s.initMu.Lock()
			if err == nil {
				s.initState = mcpSessionInitStateReady
			} else {
				s.initState = mcpSessionInitStateInitialized
			}
			s.initWait = nil
			close(waitCh)
			s.initMu.Unlock()

			if err != nil {
				return err
			}
			return nil
		default:
			waitCh := make(chan struct{})
			s.initState = mcpSessionInitStateInitializing
			s.initWait = waitCh
			s.initMu.Unlock()

			nextState, err := s.initializeHandshake(ctx)

			s.initMu.Lock()
			s.initState = nextState
			s.initWait = nil
			close(waitCh)
			s.initMu.Unlock()

			if err != nil {
				return err
			}
			if nextState == mcpSessionInitStateReady {
				return nil
			}
		}
	}
}

func (s *mcpSession) initializeHandshake(ctx context.Context) (mcpSessionInitState, error) {
	params, err := json.Marshal(map[string]any{
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
	})
	if err != nil {
		return mcpSessionInitStateNone, err
	}
	initID, err := sdkjsonrpc.MakeID("init-1")
	if err != nil {
		return mcpSessionInitStateNone, err
	}
	initResp, err := s.invokeCall(ctx, &sdkjsonrpc.Request{
		ID:     initID,
		Method: "initialize",
		Params: params,
	})
	if err != nil {
		return mcpSessionInitStateNone, err
	}
	if initResp.Error != nil {
		return mcpSessionInitStateNone, initResp.Error
	}
	if err := s.sendInitializedNotification(ctx); err != nil {
		return mcpSessionInitStateInitialized, err
	}
	return mcpSessionInitStateReady, nil
}

func (s *mcpSession) sendInitializedNotification(ctx context.Context) error {
	if s.conn == nil {
		return io.EOF
	}
	return s.conn.Write(ctx, &sdkjsonrpc.Request{
		Method: "notifications/initialized",
	})
}

func (s *mcpSession) invokeCall(ctx context.Context, req *sdkjsonrpc.Request) (*sdkjsonrpc.Response, error) {
	if s.conn == nil {
		return nil, io.EOF
	}
	if req == nil || !req.ID.IsValid() {
		return nil, fmt.Errorf("missing request id")
	}
	key := sdkIDKey(req.ID)
	if key == "" {
		return nil, fmt.Errorf("invalid request id")
	}

	respCh := make(chan *sdkjsonrpc.Response, 1)
	s.pendingMu.Lock()
	s.pending[key] = respCh
	s.pendingMu.Unlock()

	if err := s.conn.Write(ctx, req); err != nil {
		s.removePending(key)
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
		return resp, nil
	case <-s.closed:
		if s.closeErr != nil {
			return nil, s.closeErr
		}
		return nil, io.EOF
	case <-ctx.Done():
		s.removePending(key)
		return nil, ctx.Err()
	}
}

func (s *mcpSession) removePending(key string) {
	if strings.TrimSpace(key) == "" {
		return
	}
	s.pendingMu.Lock()
	delete(s.pending, key)
	s.pendingMu.Unlock()
}

func (s *mcpSession) setInitStateAtLeast(next mcpSessionInitState) {
	s.initMu.Lock()
	if s.initState != mcpSessionInitStateInitializing && s.initState < next {
		s.initState = next
	}
	s.initMu.Unlock()
}

func parseRawJSONRPCID(raw json.RawMessage) (sdkjsonrpc.ID, error) {
	if len(raw) == 0 {
		return sdkjsonrpc.ID{}, fmt.Errorf("missing request id")
	}
	var idValue any
	if err := json.Unmarshal(raw, &idValue); err != nil {
		return sdkjsonrpc.ID{}, err
	}
	id, err := sdkjsonrpc.MakeID(idValue)
	if err != nil {
		return sdkjsonrpc.ID{}, err
	}
	if !id.IsValid() {
		return sdkjsonrpc.ID{}, fmt.Errorf("missing request id")
	}
	return id, nil
}

func sdkIDKey(id sdkjsonrpc.ID) string {
	if !id.IsValid() {
		return ""
	}
	raw, err := json.Marshal(id.Raw())
	if err != nil {
		return ""
	}
	return string(raw)
}

func sdkIDRaw(id sdkjsonrpc.ID) json.RawMessage {
	if !id.IsValid() {
		return nil
	}
	raw, err := json.Marshal(id.Raw())
	if err != nil {
		return nil
	}
	return json.RawMessage(raw)
}

func sdkResponsePayload(resp *sdkjsonrpc.Response) (map[string]any, error) {
	if resp == nil {
		return nil, io.EOF
	}
	if resp.Error != nil {
		code := int64(-32603)
		message := strings.TrimSpace(resp.Error.Error())
		if wireErr, ok := resp.Error.(*sdkjsonrpc.Error); ok {
			code = wireErr.Code
			message = strings.TrimSpace(wireErr.Message)
		}
		if message == "" {
			message = "internal error"
		}
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      sdkIDRaw(resp.ID),
			"error": map[string]any{
				"code":    code,
				"message": message,
			},
		}, nil
	}
	var result any
	if len(resp.Result) > 0 {
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      sdkIDRaw(resp.ID),
		"result":  result,
	}, nil
}
