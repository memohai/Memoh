package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	sdkjsonrpc "github.com/modelcontextprotocol/go-sdk/jsonrpc"

	mcptools "github.com/memohai/memoh/internal/mcp"
)

type fakeMCPConnection struct {
	mu      sync.Mutex
	writes  []*sdkjsonrpc.Request
	readCh  chan sdkjsonrpc.Message
	closed  chan struct{}
	closeMu sync.Once
	onWrite func(req *sdkjsonrpc.Request) (*sdkjsonrpc.Response, error)
}

func newFakeMCPConnection(onWrite func(req *sdkjsonrpc.Request) (*sdkjsonrpc.Response, error)) *fakeMCPConnection {
	return &fakeMCPConnection{
		writes:  make([]*sdkjsonrpc.Request, 0, 16),
		readCh:  make(chan sdkjsonrpc.Message, 32),
		closed:  make(chan struct{}),
		onWrite: onWrite,
	}
}

func (c *fakeMCPConnection) Read(ctx context.Context) (sdkjsonrpc.Message, error) {
	select {
	case <-c.closed:
		return nil, io.EOF
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg, ok := <-c.readCh:
		if !ok {
			return nil, io.EOF
		}
		return msg, nil
	}
}

func (c *fakeMCPConnection) Write(ctx context.Context, msg sdkjsonrpc.Message) error {
	req, ok := msg.(*sdkjsonrpc.Request)
	if !ok {
		return fmt.Errorf("unsupported message type: %T", msg)
	}
	cloned := cloneJSONRPCRequest(req)
	c.mu.Lock()
	c.writes = append(c.writes, cloned)
	c.mu.Unlock()

	if c.onWrite == nil {
		return nil
	}
	resp, err := c.onWrite(cloned)
	if err != nil {
		return err
	}
	if resp == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		return io.EOF
	case c.readCh <- resp:
		return nil
	}
}

func (c *fakeMCPConnection) Close() error {
	c.closeMu.Do(func() {
		close(c.closed)
		close(c.readCh)
	})
	return nil
}

func (c *fakeMCPConnection) SessionID() string {
	return "test-session"
}

func cloneJSONRPCRequest(req *sdkjsonrpc.Request) *sdkjsonrpc.Request {
	if req == nil {
		return nil
	}
	params := append([]byte(nil), req.Params...)
	return &sdkjsonrpc.Request{
		ID:     req.ID,
		Method: req.Method,
		Params: params,
		Extra:  req.Extra,
	}
}

func jsonRPCSuccessResponse(id sdkjsonrpc.ID, payload map[string]any) *sdkjsonrpc.Response {
	body, _ := json.Marshal(payload)
	return &sdkjsonrpc.Response{
		ID:     id,
		Result: body,
	}
}

func newTestMCPSession(conn *fakeMCPConnection) *mcpSession {
	return &mcpSession{
		pending: map[string]chan *sdkjsonrpc.Response{},
		conn:    conn,
		closed:  make(chan struct{}),
	}
}

func TestMCPSessionRetriesInitializeAfterFailure(t *testing.T) {
	initCalls := 0
	conn := newFakeMCPConnection(func(req *sdkjsonrpc.Request) (*sdkjsonrpc.Response, error) {
		switch req.Method {
		case "initialize":
			initCalls++
			if initCalls == 1 {
				return &sdkjsonrpc.Response{
					ID: req.ID,
					Error: &sdkjsonrpc.Error{
						Code:    -32603,
						Message: "temporary init failure",
					},
				}, nil
			}
			return jsonRPCSuccessResponse(req.ID, map[string]any{
				"protocolVersion": "2025-06-18",
			}), nil
		case "tools/list":
			return jsonRPCSuccessResponse(req.ID, map[string]any{
				"tools": []any{},
			}), nil
		default:
			return nil, nil
		}
	})
	session := newTestMCPSession(conn)
	go session.readLoop()
	defer session.closeWithError(io.EOF)

	_, firstErr := session.call(context.Background(), mcptools.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      mcptools.RawStringID("1"),
		Method:  "tools/list",
	})
	if firstErr == nil {
		t.Fatalf("first call should fail when initialize fails")
	}

	secondPayload, secondErr := session.call(context.Background(), mcptools.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      mcptools.RawStringID("2"),
		Method:  "tools/list",
	})
	if secondErr != nil {
		t.Fatalf("second call should recover by retrying initialize: %v", secondErr)
	}
	if initCalls != 2 {
		t.Fatalf("initialize should be retried once, got calls: %d", initCalls)
	}
	result, ok := secondPayload["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing tools/list result: %#v", secondPayload)
	}
	if _, ok := result["tools"].([]any); !ok {
		t.Fatalf("missing tools field: %#v", result)
	}
}

func TestMCPSessionExplicitInitializeDoesNotDuplicateInitialize(t *testing.T) {
	initializeCalls := 0
	initializedNotifications := 0
	conn := newFakeMCPConnection(func(req *sdkjsonrpc.Request) (*sdkjsonrpc.Response, error) {
		switch req.Method {
		case "initialize":
			initializeCalls++
			return jsonRPCSuccessResponse(req.ID, map[string]any{
				"protocolVersion": "2025-06-18",
			}), nil
		case "notifications/initialized":
			initializedNotifications++
			return nil, nil
		case "tools/list":
			return jsonRPCSuccessResponse(req.ID, map[string]any{
				"tools": []any{},
			}), nil
		default:
			return nil, nil
		}
	})
	session := newTestMCPSession(conn)
	go session.readLoop()
	defer session.closeWithError(io.EOF)

	_, initErr := session.call(context.Background(), mcptools.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      mcptools.RawStringID("100"),
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"v1"}}`),
	})
	if initErr != nil {
		t.Fatalf("explicit initialize should succeed: %v", initErr)
	}

	_, listErr := session.call(context.Background(), mcptools.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      mcptools.RawStringID("101"),
		Method:  "tools/list",
	})
	if listErr != nil {
		t.Fatalf("tools/list after initialize should succeed: %v", listErr)
	}
	if initializeCalls != 1 {
		t.Fatalf("initialize should not be duplicated, got: %d", initializeCalls)
	}
	if initializedNotifications != 1 {
		t.Fatalf("should send exactly one notifications/initialized, got: %d", initializedNotifications)
	}
}

func TestMCPSessionRemovesPendingOnContextCancel(t *testing.T) {
	conn := newFakeMCPConnection(func(_ *sdkjsonrpc.Request) (*sdkjsonrpc.Response, error) {
		// Intentionally do not reply; caller should timeout.
		return nil, nil
	})
	session := newTestMCPSession(conn)
	session.initState = mcpSessionInitStateReady

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := session.call(ctx, mcptools.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      mcptools.RawStringID("200"),
		Method:  "tools/list",
	})
	if err == nil {
		t.Fatalf("call should fail on context timeout")
	}

	session.pendingMu.Lock()
	pendingCount := len(session.pending)
	session.pendingMu.Unlock()
	if pendingCount != 0 {
		t.Fatalf("pending map should be empty after cancellation, got: %d", pendingCount)
	}
}
