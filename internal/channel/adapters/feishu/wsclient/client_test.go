package wsclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// fakeDispatcher records dispatched payloads and lets the test control the
// returned response/error per call.
type fakeDispatcher struct {
	mu       sync.Mutex
	payloads [][]byte
	respFn   func([]byte) (interface{}, error)
}

func (d *fakeDispatcher) Do(_ context.Context, payload []byte) (interface{}, error) {
	d.mu.Lock()
	d.payloads = append(d.payloads, payload)
	respFn := d.respFn
	d.mu.Unlock()
	if respFn == nil {
		return nil, nil
	}
	return respFn(payload)
}

func (d *fakeDispatcher) seen() [][]byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([][]byte, len(d.payloads))
	copy(out, d.payloads)
	return out
}

// mockGateway emulates the Feishu open-api gateway: HTTP endpoint
// discovery plus websocket upgrade, served by a single httptest.Server.
type mockGateway struct {
	t   testing.TB
	srv *httptest.Server

	mu          sync.Mutex
	conns       []*websocket.Conn
	upgradeHook func(*websocket.Conn)
	clientCfg   *larkws.ClientConfig
	endpointErr struct {
		code int // when non-zero, return as EndpointResp.Code
		msg  string
	}
	handshakeStatus    int
	handshakeAuthCode  int
	handshakeStatusMsg string
}

// mockGatewayOpt mutates an unstarted httptest.Server so callers can set
// Server.Config fields like ConnState. http.Server reads those from its
// own goroutine, so they must be set before Start.
type mockGatewayOpt func(*httptest.Server)

func newMockGateway(t testing.TB, opts ...mockGatewayOpt) *mockGateway {
	g := &mockGateway{t: t}
	mux := http.NewServeMux()
	mux.HandleFunc(larkws.GenEndpointUri, g.handleEndpoint)
	mux.HandleFunc("/ws", g.handleUpgrade)
	g.srv = httptest.NewUnstartedServer(mux)
	for _, opt := range opts {
		opt(g.srv)
	}
	g.srv.Start()
	t.Cleanup(g.close)
	return g
}

func (g *mockGateway) close() {
	g.mu.Lock()
	conns := g.conns
	g.conns = nil
	g.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
	g.srv.Close()
}

func (g *mockGateway) domain() string { return g.srv.URL }

func (g *mockGateway) handleEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(r.Body)
	defer func() { _ = r.Body.Close() }()
	var req map[string]string
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req["AppID"] == "" || req["AppSecret"] == "" {
		http.Error(w, "missing creds", http.StatusBadRequest)
		return
	}
	g.mu.Lock()
	endpointErr := g.endpointErr
	clientCfg := g.clientCfg
	g.mu.Unlock()

	if endpointErr.code != 0 {
		_ = json.NewEncoder(w).Encode(larkws.EndpointResp{
			Code: endpointErr.code,
			Msg:  endpointErr.msg,
		})
		return
	}

	wsURL := strings.Replace(g.srv.URL, "http://", "ws://", 1) + "/ws?service_id=42&device_id=test-conn"
	resp := larkws.EndpointResp{
		Code: larkws.OK,
		Data: &larkws.Endpoint{
			Url:          wsURL,
			ClientConfig: clientCfg,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// waitForConn blocks until the gateway has accepted at least n upgraded
// websocket connections, then returns the n-th one. The poll keeps tests
// race-free without sprinkling sleeps inline.
func (g *mockGateway) waitForConn(t testing.TB, n int, timeout time.Duration) *websocket.Conn {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		g.mu.Lock()
		if len(g.conns) >= n {
			c := g.conns[n-1]
			g.mu.Unlock()
			return c
		}
		g.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("websocket connection #%d not seen within %v", n, timeout)
	return nil
}

func (g *mockGateway) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	hsStatus := g.handshakeStatus
	hsAuth := g.handshakeAuthCode
	hsMsg := g.handshakeStatusMsg
	hook := g.upgradeHook
	g.mu.Unlock()

	if hsStatus != 0 {
		w.Header().Set(larkws.HeaderHandshakeStatus, strconv.Itoa(hsStatus))
		w.Header().Set(larkws.HeaderHandshakeMsg, hsMsg)
		if hsAuth != 0 {
			w.Header().Set(larkws.HeaderHandshakeAuthErrCode, strconv.Itoa(hsAuth))
		}
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		g.t.Errorf("upgrade failed: %v", err)
		return
	}
	g.mu.Lock()
	g.conns = append(g.conns, conn)
	g.mu.Unlock()
	if hook != nil {
		hook(conn)
	}
}

// pushEvent fabricates an event-type data frame and writes it to the given
// server-side conn.
func (*mockGateway) pushEvent(conn *websocket.Conn, msgID, eventJSON string) error {
	headers := larkws.Headers{}
	headers.Add(larkws.HeaderType, string(larkws.MessageTypeEvent))
	headers.Add(larkws.HeaderMessageID, msgID)
	headers.Add(larkws.HeaderTraceID, "trace-"+msgID)
	headers.Add(larkws.HeaderSum, "1")
	headers.Add(larkws.HeaderSeq, "0")
	frame := &larkws.Frame{
		Method:  int32(larkws.FrameTypeData),
		Headers: headers,
		Payload: []byte(eventJSON),
	}
	bs, err := frame.Marshal()
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, bs)
}

// pushEventFragment writes one fragment of a multi-fragment event.
func (*mockGateway) pushEventFragment(conn *websocket.Conn, msgID string, sum, seq int, payload []byte) error {
	headers := larkws.Headers{}
	headers.Add(larkws.HeaderType, string(larkws.MessageTypeEvent))
	headers.Add(larkws.HeaderMessageID, msgID)
	headers.Add(larkws.HeaderSum, strconv.Itoa(sum))
	headers.Add(larkws.HeaderSeq, strconv.Itoa(seq))
	frame := &larkws.Frame{
		Method:  int32(larkws.FrameTypeData),
		Headers: headers,
		Payload: payload,
	}
	bs, err := frame.Marshal()
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, bs)
}

// readResponseFrame reads a single frame from the server-side conn and
// returns its decoded representation.
func (*mockGateway) readResponseFrame(conn *websocket.Conn) (*larkws.Frame, error) {
	mt, raw, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	if mt != websocket.BinaryMessage {
		return nil, fmt.Errorf("unexpected message type %d", mt)
	}
	frame := &larkws.Frame{}
	if err := frame.Unmarshal(raw); err != nil {
		return nil, err
	}
	return frame, nil
}

func newTestClient(domain string, overrides ...func(*Config)) *Client {
	cfg := Config{
		AppID:            "app",
		AppSecret:        "secret",
		Domain:           domain,
		Logger:           slog.New(slog.DiscardHandler),
		PingInterval:     50 * time.Millisecond,
		HandshakeTimeout: 2 * time.Second,
		CloseGracePeriod: 200 * time.Millisecond,
	}
	for _, fn := range overrides {
		fn(&cfg)
	}
	return New(cfg)
}

// TestRunDispatchesEventAndAcks covers the happy path: connect, receive
// event, dispatch, ack.
func TestRunDispatchesEventAndAcks(t *testing.T) {
	t.Parallel()
	gw := newMockGateway(t)

	gw.upgradeHook = func(conn *websocket.Conn) {
		go func() {
			// give the client a moment to enter readLoop
			time.Sleep(10 * time.Millisecond)
			_ = gw.pushEvent(conn, "msg-1", `{"schema":"2.0","event":{}}`)
		}()
	}

	disp := &fakeDispatcher{}
	client := newTestClient(gw.domain())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(ctx, disp) }()

	conn := gw.waitForConn(t, 1, 2*time.Second)
	frame, err := gw.readResponseFrame(conn)
	if err != nil {
		t.Fatalf("read response frame: %v", err)
	}
	var ack larkws.Response
	if err := json.Unmarshal(frame.Payload, &ack); err != nil {
		t.Fatalf("decode ack payload: %v (raw=%q)", err, frame.Payload)
	}
	if ack.StatusCode != http.StatusOK {
		t.Errorf("ack status = %d, want 200", ack.StatusCode)
	}

	cancel()
	if err := <-runDone; err != nil {
		t.Errorf("Run returned error on cancel: %v", err)
	}

	if got := disp.seen(); len(got) != 1 {
		t.Fatalf("dispatcher saw %d payloads, want 1", len(got))
	}
}

// TestRunHonorsContextCancel covers the original bug: ctx cancel must
// close the TCP conn and return nil, leaving no goroutine inside Run.
func TestRunHonorsContextCancel(t *testing.T) {
	t.Parallel()
	gw := newMockGateway(t)

	disp := &fakeDispatcher{}
	client := newTestClient(gw.domain())

	ctx, cancel := context.WithCancel(context.Background())

	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(ctx, disp) }()

	conn := gw.waitForConn(t, 1, 2*time.Second)
	cancel()

	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("Run after cancel returned %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s of ctx cancel — connection leak suspected")
	}

	// Server-side ReadMessage must observe a close.
	_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Error("server still sees an open connection after client Run returned")
	}
}

// TestRunReturnsErrorOnServerClose verifies that a server-side disconnect is
// surfaced as an error so the caller knows to reconnect.
func TestRunReturnsErrorOnServerClose(t *testing.T) {
	t.Parallel()
	gw := newMockGateway(t)
	gw.upgradeHook = func(conn *websocket.Conn) {
		go func() {
			time.Sleep(20 * time.Millisecond)
			_ = conn.Close()
		}()
	}

	disp := &fakeDispatcher{}
	client := newTestClient(gw.domain())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := client.Run(ctx, disp)
	if err == nil {
		t.Fatal("Run returned nil on server-side close, want error")
	}
}

// TestRunHandshakeFailure verifies that a non-101 upgrade response is parsed
// into a typed lark error so callers can branch on terminal vs recoverable
// failures.
func TestRunHandshakeFailure(t *testing.T) {
	t.Parallel()
	gw := newMockGateway(t)
	gw.handshakeStatus = larkws.AuthFailed
	gw.handshakeAuthCode = larkws.ExceedConnLimit
	gw.handshakeStatusMsg = "exceed connection limit"

	disp := &fakeDispatcher{}
	client := newTestClient(gw.domain())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := client.Run(ctx, disp)
	if err == nil {
		t.Fatal("expected handshake error, got nil")
	}
	var ce *larkws.ClientError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *larkws.ClientError, got %T (%v)", err, err)
	}
	if ce.Code != larkws.AuthFailed {
		t.Errorf("ClientError code = %d, want %d", ce.Code, larkws.AuthFailed)
	}
}

// TestRunEndpointFailure verifies that a non-OK endpoint response surfaces
// as a typed error.
func TestRunEndpointFailure(t *testing.T) {
	t.Parallel()
	gw := newMockGateway(t)
	gw.endpointErr.code = 99999
	gw.endpointErr.msg = "bad app"

	disp := &fakeDispatcher{}
	client := newTestClient(gw.domain())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := client.Run(ctx, disp)
	if err == nil {
		t.Fatal("expected endpoint error, got nil")
	}
	var ce *larkws.ClientError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *larkws.ClientError, got %T (%v)", err, err)
	}
}

// TestRunReassemblesFragmentedEvent ensures the fragment cache stitches
// multi-fragment payloads back together before dispatch.
func TestRunReassemblesFragmentedEvent(t *testing.T) {
	t.Parallel()
	gw := newMockGateway(t)
	full := `{"schema":"2.0","event":{"text":"hello world"}}`
	mid := len(full) / 2
	gw.upgradeHook = func(conn *websocket.Conn) {
		go func() {
			time.Sleep(10 * time.Millisecond)
			_ = gw.pushEventFragment(conn, "frag-1", 2, 0, []byte(full[:mid]))
			time.Sleep(10 * time.Millisecond)
			_ = gw.pushEventFragment(conn, "frag-1", 2, 1, []byte(full[mid:]))
		}()
	}

	disp := &fakeDispatcher{}
	client := newTestClient(gw.domain())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(ctx, disp) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(disp.seen()) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-runDone

	got := disp.seen()
	if len(got) != 1 {
		t.Fatalf("dispatcher saw %d events, want 1", len(got))
	}
	if string(got[0]) != full {
		t.Errorf("reassembled payload = %q, want %q", got[0], full)
	}
}

// TestRunSendsPing exercises the ping cadence: the server should observe a
// binary ping frame within roughly the configured interval.
func TestRunSendsPing(t *testing.T) {
	t.Parallel()
	gw := newMockGateway(t)

	pingSeen := make(chan struct{}, 1)
	gw.upgradeHook = func(conn *websocket.Conn) {
		go func() {
			for {
				mt, raw, err := conn.ReadMessage()
				if err != nil {
					return
				}
				if mt != websocket.BinaryMessage {
					continue
				}
				frame := &larkws.Frame{}
				if err := frame.Unmarshal(raw); err != nil {
					continue
				}
				headers := larkws.Headers(frame.Headers)
				if larkws.MessageType(headers.GetString(larkws.HeaderType)) == larkws.MessageTypePing {
					select {
					case pingSeen <- struct{}{}:
					default:
					}
					return
				}
			}
		}()
	}

	disp := &fakeDispatcher{}
	client := newTestClient(gw.domain(), func(c *Config) {
		c.PingInterval = 30 * time.Millisecond
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(ctx, disp) }()

	select {
	case <-pingSeen:
	case <-time.After(1 * time.Second):
		t.Fatal("did not observe ping frame within 1s")
	}
	cancel()
	<-runDone
}

// TestRunDispatcherErrorBecomesNon200 verifies the ack reflects dispatch
// failures so the platform can retry.
func TestRunDispatcherErrorBecomesNon200(t *testing.T) {
	t.Parallel()
	gw := newMockGateway(t)

	gw.upgradeHook = func(conn *websocket.Conn) {
		go func() {
			time.Sleep(10 * time.Millisecond)
			_ = gw.pushEvent(conn, "msg-err", `{"schema":"2.0","event":{}}`)
		}()
	}

	disp := &fakeDispatcher{
		respFn: func([]byte) (interface{}, error) {
			return nil, errors.New("dispatch boom")
		},
	}
	client := newTestClient(gw.domain())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(ctx, disp) }()

	conn := gw.waitForConn(t, 1, 2*time.Second)
	frame, err := gw.readResponseFrame(conn)
	if err != nil {
		t.Fatalf("read response frame: %v", err)
	}
	var ack larkws.Response
	if err := json.Unmarshal(frame.Payload, &ack); err != nil {
		t.Fatalf("decode ack payload: %v", err)
	}
	if ack.StatusCode != http.StatusInternalServerError {
		t.Errorf("ack status = %d, want %d", ack.StatusCode, http.StatusInternalServerError)
	}
	cancel()
	<-runDone
}

// TestEndpointURLConstruction is a sanity test for the gateway helper to
// keep upgrade routing aligned with the production handshake URL shape.
func TestEndpointURLConstruction(t *testing.T) {
	t.Parallel()
	gw := newMockGateway(t)
	body, _ := json.Marshal(map[string]string{"AppID": "a", "AppSecret": "b"})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, gw.domain()+larkws.GenEndpointUri, strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // G107: test request to httptest.Server
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	parsed := &larkws.EndpointResp{}
	if err := json.Unmarshal(raw, parsed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if parsed.Code != larkws.OK {
		t.Fatalf("got code=%d msg=%s", parsed.Code, parsed.Msg)
	}
	u, err := url.Parse(parsed.Data.Url)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if u.Query().Get(larkws.ServiceID) == "" || u.Query().Get(larkws.DeviceID) == "" {
		t.Errorf("endpoint url missing service_id/device_id: %s", parsed.Data.Url)
	}
}

// TestBuildDialerInheritsDefaults verifies the dialer keeps
// http.ProxyFromEnvironment from websocket.DefaultDialer and does not mutate
// the package-level default.
func TestBuildDialerInheritsDefaults(t *testing.T) {
	t.Parallel()
	c := newTestClient("https://example.com")
	d := c.buildDialer()
	if d.Proxy == nil {
		t.Errorf("dialer.Proxy is nil; expected http.ProxyFromEnvironment")
	}
	if d.HandshakeTimeout != c.cfg.HandshakeTimeout {
		t.Errorf("dialer.HandshakeTimeout = %v, want %v", d.HandshakeTimeout, c.cfg.HandshakeTimeout)
	}
	if websocket.DefaultDialer.HandshakeTimeout == c.cfg.HandshakeTimeout {
		t.Errorf("buildDialer mutated websocket.DefaultDialer; must work on a copy")
	}
}

// TestRunHandshakeFailureDoesNotLeakConnections asserts that repeated
// rejected handshakes do not leave live TCP conns piling up on the server
// side, which is what would happen if a gorilla upgrade ever stopped
// closing the netConn for us.
func TestRunHandshakeFailureDoesNotLeakConnections(t *testing.T) {
	t.Parallel()
	var live int32
	gw := newMockGateway(t, func(srv *httptest.Server) {
		srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
			switch state {
			case http.StateNew:
				atomic.AddInt32(&live, 1)
			case http.StateClosed, http.StateHijacked:
				atomic.AddInt32(&live, -1)
			}
		}
	})
	gw.handshakeStatus = larkws.AuthFailed
	gw.handshakeAuthCode = larkws.ExceedConnLimit
	gw.handshakeStatusMsg = "exceed connection limit"

	disp := &fakeDispatcher{}
	const attempts = 30
	for i := 0; i < attempts; i++ {
		client := newTestClient(gw.domain())
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := client.Run(ctx, disp)
		cancel()
		if err == nil {
			t.Fatalf("attempt %d: expected handshake error, got nil", i)
		}
	}

	// Tolerance covers in-flight conns whose StateClosed callback has not
	// fired yet (typically 0-1); a real leak would be near `attempts`.
	const tolerance = int32(3)
	deadline := time.Now().Add(5 * time.Second)
	var last int32
	for time.Now().Before(deadline) {
		last = atomic.LoadInt32(&live)
		if last <= tolerance {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("after %d failed handshakes the test server still has %d live conns (tolerance=%d); resp.Body is leaking", attempts, last, tolerance)
}

// TestFragmentCacheReassembles is a focused unit test for the fragment
// cache, independent of the websocket plumbing.
func TestFragmentCacheReassembles(t *testing.T) {
	t.Parallel()
	c := newFragmentCache(5 * time.Second)
	defer c.stop()

	if got := c.add("k", 3, 0, []byte("foo")); got != nil {
		t.Errorf("expected nil while incomplete, got %q", got)
	}
	if got := c.add("k", 3, 2, []byte("baz")); got != nil {
		t.Errorf("expected nil while incomplete, got %q", got)
	}
	got := c.add("k", 3, 1, []byte("bar"))
	if string(got) != "foobarbaz" {
		t.Errorf("reassembled = %q, want foobarbaz", got)
	}
	// After completion the entry should be evicted, so the next add for
	// the same id behaves as a brand-new sequence.
	if got := c.add("k", 1, 0, []byte("solo")); string(got) != "solo" {
		t.Errorf("post-eviction add = %q, want solo", got)
	}
}

// TestRunDispatchesEventsConcurrently asserts a slow dispatcher does not
// stall the read loop: a second event must be ack'd while the first one is
// still inside dispatcher.Do.
func TestRunDispatchesEventsConcurrently(t *testing.T) {
	t.Parallel()
	gw := newMockGateway(t)
	// Disable client pings so the server only sees ACK frames. With
	// pings on, one arriving while slow dispatch is blocked would look
	// like the fast ack.
	client := newTestClient(gw.domain(), func(c *Config) {
		c.PingInterval = 1 * time.Hour
	})

	gw.upgradeHook = func(conn *websocket.Conn) {
		go func() {
			time.Sleep(10 * time.Millisecond)
			_ = gw.pushEvent(conn, "msg-slow", `{"schema":"2.0","event":{"slow":true}}`)
			_ = gw.pushEvent(conn, "msg-fast", `{"schema":"2.0","event":{"slow":false}}`)
		}()
	}

	released := make(chan struct{})
	slowEntered := make(chan struct{})
	var firstFlag atomic.Bool
	disp := &fakeDispatcher{
		respFn: func(payload []byte) (interface{}, error) {
			if strings.Contains(string(payload), `"slow":true`) {
				if firstFlag.CompareAndSwap(false, true) {
					close(slowEntered)
				}
				<-released
			}
			return nil, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- client.Run(ctx, disp) }()

	conn := gw.waitForConn(t, 1, 2*time.Second)
	<-slowEntered

	// While slow dispatch is still blocked, the fast event's ack must
	// already be readable.
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	frame, err := gw.readResponseFrame(conn)
	if err != nil {
		close(released)
		t.Fatalf("fast ack did not arrive while slow dispatch was blocked: %v", err)
	}
	if got := larkws.Headers(frame.Headers).GetString(larkws.HeaderMessageID); got != "msg-fast" {
		close(released)
		t.Fatalf("first ack message_id = %q, want msg-fast", got)
	}
	_ = conn.SetReadDeadline(time.Time{})

	close(released)
	frame, err = gw.readResponseFrame(conn)
	if err != nil {
		t.Fatalf("slow ack did not arrive after release: %v", err)
	}
	if got := larkws.Headers(frame.Headers).GetString(larkws.HeaderMessageID); got != "msg-slow" {
		t.Fatalf("second ack message_id = %q, want msg-slow", got)
	}

	cancel()
	if err := <-runDone; err != nil {
		t.Errorf("Run returned error on cancel: %v", err)
	}
}

// TestFragmentCacheTTLEviction confirms incomplete fragments are not
// retained forever.
func TestFragmentCacheTTLEviction(t *testing.T) {
	t.Parallel()
	c := newFragmentCache(20 * time.Millisecond)
	defer c.stop()
	if got := c.add("k", 2, 0, []byte("foo")); got != nil {
		t.Errorf("expected nil while incomplete, got %q", got)
	}
	// Wait for the janitor to evict.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		_, exists := c.items["k"]
		c.mu.Unlock()
		if !exists {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("expired fragment was not evicted")
}
