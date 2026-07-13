package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/sessionruntime"
)

const (
	runtimeSmokeChildEnv  = "MEMOH_TEST_RUNTIME_SMOKE_CHILD"
	runtimeSmokeRedisEnv  = "MEMOH_TEST_RUNTIME_SMOKE_REDIS_URL"
	runtimeSmokePrefixEnv = "MEMOH_TEST_RUNTIME_SMOKE_PREFIX"
	runtimeSmokeOwnerEnv  = "MEMOH_TEST_RUNTIME_SMOKE_OWNER_ID"
	runtimeSmokeLeaseTTL  = 600 * time.Millisecond
)

type runtimeSmokeState struct {
	mu sync.Mutex

	started                 bool
	handle                  sessionruntime.RunHandle
	streamID                string
	steers                  []string
	aborted                 bool
	canceled                bool
	approvalID              string
	approvalDecision        string
	approvalResolvedByOwner bool
}

type runtimeSmokeStateView struct {
	StreamID                string   `json:"stream_id,omitempty"`
	Steers                  []string `json:"steers"`
	Aborted                 bool     `json:"aborted"`
	Canceled                bool     `json:"canceled"`
	ApprovalID              string   `json:"approval_id,omitempty"`
	ApprovalDecision        string   `json:"approval_decision,omitempty"`
	ApprovalResolvedByOwner bool     `json:"approval_resolved_by_owner"`
}

type runtimeSmokeResolver struct {
	*flow.Resolver
	state *runtimeSmokeState
}

func (r *runtimeSmokeResolver) RespondToolApproval(_ context.Context, input flow.ToolApprovalResponseInput, _ chan<- flow.WSStreamEvent) error {
	r.state.mu.Lock()
	r.state.approvalID = input.ApprovalID
	r.state.approvalDecision = input.Decision
	r.state.approvalResolvedByOwner = input.ResolveOnly
	r.state.mu.Unlock()
	return nil
}

func (s *runtimeSmokeState) startRun(manager *sessionruntime.Manager, streamID, extraText string, processDone <-chan struct{}) error {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return errors.New("stream_id is required")
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("runtime smoke process already owns a run")
	}
	s.started = true
	s.streamID = streamID
	s.mu.Unlock()

	runCtx, runCancel := context.WithCancel(context.Background())
	abortCh := make(chan struct{}, 1)
	injectCh := make(chan conversation.InjectMessage, 4)
	cancelRun := func() {
		runCancel()
		s.mu.Lock()
		s.canceled = true
		s.mu.Unlock()
	}
	handle, err := manager.StartRunHandle(runCtx, runtimeContractBotID, runtimeContractSessionID, streamID, abortCh, cancelRun, injectCh)
	if err != nil {
		runCancel()
		s.mu.Lock()
		s.started = false
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	s.handle = handle
	s.mu.Unlock()
	go func() {
		select {
		case <-abortCh:
			s.mu.Lock()
			s.aborted = true
			s.mu.Unlock()
		case <-processDone:
		}
	}()
	go func() {
		for {
			select {
			case injected, ok := <-injectCh:
				if !ok {
					return
				}
				s.mu.Lock()
				s.steers = append(s.steers, injected.Text)
				s.mu.Unlock()
				if injected.Applied != nil {
					injected.Applied()
				}
			case <-processDone:
				return
			}
		}
	}()

	for _, event := range richActiveRunActiveAgentContractScript() {
		if _, err := manager.HandleAgentEvent(context.Background(), handle, event); err != nil {
			return fmt.Errorf("handle %s event: %w", event.Type, err)
		}
	}
	if extraText != "" {
		if _, err := manager.HandleAgentEvent(context.Background(), handle, agentpkg.StreamEvent{
			Type:  agentpkg.EventTextDelta,
			Delta: extraText,
		}); err != nil {
			return fmt.Errorf("handle extra text event: %w", err)
		}
	}
	return nil
}

func (s *runtimeSmokeState) view() runtimeSmokeStateView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return runtimeSmokeStateView{
		StreamID:                s.streamID,
		Steers:                  append([]string(nil), s.steers...),
		Aborted:                 s.aborted,
		Canceled:                s.canceled,
		ApprovalID:              s.approvalID,
		ApprovalDecision:        s.approvalDecision,
		ApprovalResolvedByOwner: s.approvalResolvedByOwner,
	}
}

func (s *runtimeSmokeState) runHandle() sessionruntime.RunHandle {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handle
}

// TestLocalChannelRuntimeMultiprocessServer is re-executed as a child test
// binary so each smoke server owns independent Go memory and goroutines. This
// is a runtime/handler process-boundary smoke test, not full cmd/agent wiring.
func TestLocalChannelRuntimeMultiprocessServer(t *testing.T) {
	if os.Getenv(runtimeSmokeChildEnv) != "1" {
		t.Skip("runtime smoke child helper")
	}
	redisURL := strings.TrimSpace(os.Getenv(runtimeSmokeRedisEnv))
	prefix := strings.TrimSpace(os.Getenv(runtimeSmokePrefixEnv))
	ownerID := strings.TrimSpace(os.Getenv(runtimeSmokeOwnerEnv))
	if redisURL == "" || prefix == "" || ownerID == "" {
		t.Fatal("runtime smoke child configuration is incomplete")
	}

	backend, err := sessionruntime.NewRedisBackend(context.Background(), sessionruntime.RedisOptions{
		URL:       redisURL,
		KeyPrefix: prefix,
		StateTTL:  time.Minute,
	})
	if err != nil {
		t.Fatalf("create runtime smoke backend: %v", err)
	}
	manager := sessionruntime.NewManager(backend, sessionruntime.Options{
		OwnerID:       ownerID,
		StateTTL:      time.Minute,
		OwnerLeaseTTL: runtimeSmokeLeaseTTL,
		CommandAckTTL: 2 * time.Second,
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start runtime smoke manager: %v", err)
	}
	defer func() { _ = manager.Close() }()

	state := &runtimeSmokeState{}
	resolver := &runtimeSmokeResolver{Resolver: &flow.Resolver{}, state: state}
	handler := runtimeContractLocalChannelHandler(manager)
	handler.SetResolver(resolver)
	shutdown := make(chan struct{})

	e := echo.New()
	e.GET("/bots/:bot_id/local/ws", func(c echo.Context) error {
		c.Set("user", &jwt.Token{
			Valid: true,
			Claims: jwt.MapClaims{
				"sub":     runtimeContractUserID,
				"user_id": runtimeContractUserID,
			},
		})
		return handler.HandleWebSocket(c)
	})
	e.POST("/control/start", func(c echo.Context) error {
		var request struct {
			StreamID string `json:"stream_id"`
			Text     string `json:"text"`
		}
		if err := c.Bind(&request); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		// The control seam scripts resolver output while all runtime ownership,
		// aggregation, Redis routing, and WS observation use production paths.
		if err := state.startRun(manager, request.StreamID, request.Text, shutdown); err != nil {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return c.JSON(http.StatusOK, state.view())
	})
	e.POST("/control/delta", func(c echo.Context) error {
		var request struct {
			Text string `json:"text"`
		}
		if err := c.Bind(&request); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		view := state.view()
		if view.StreamID == "" {
			return echo.NewHTTPError(http.StatusConflict, "runtime smoke run is not active")
		}
		if _, err := manager.HandleAgentEvent(context.Background(), state.runHandle(), agentpkg.StreamEvent{
			Type:  agentpkg.EventTextDelta,
			Delta: request.Text,
		}); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.GET("/control/state", func(c echo.Context) error {
		return c.JSON(http.StatusOK, state.view())
	})
	var shutdownOnce sync.Once
	e.POST("/control/shutdown", func(c echo.Context) error {
		shutdownOnce.Do(func() { close(shutdown) })
		return c.JSON(http.StatusOK, map[string]string{"status": "stopping"})
	})

	server := httptest.NewServer(e)
	defer server.Close()
	if err := json.NewEncoder(os.Stdout).Encode(map[string]string{"url": server.URL}); err != nil {
		t.Fatalf("write runtime smoke readiness: %v", err)
	}
	<-shutdown
}

type runtimeSmokeProcess struct {
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	stdout   io.ReadCloser
	url      string
	stderr   bytes.Buffer
	output   bytes.Buffer
	waitDone chan struct{}
	scanDone chan struct{}
	waitErr  error
	stopOnce sync.Once
	stopErr  error
}

type runtimeSmokeReady struct {
	url string
	err error
}

func startRuntimeSmokeProcess(t *testing.T, redisURL, prefix, ownerID string) *runtimeSmokeProcess {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	processCtx, processCancel := context.WithTimeout(context.Background(), 35*time.Second)
	cmd := exec.CommandContext(processCtx, executable, //nolint:gosec // Re-execute this test binary as the isolated server process.
		"-test.run=^TestLocalChannelRuntimeMultiprocessServer$",
		"-test.timeout=30s",
	)
	cmd.Env = runtimeSmokeProcessEnv(
		runtimeSmokeChildEnv+"=1",
		runtimeSmokeRedisEnv+"="+redisURL,
		runtimeSmokePrefixEnv+"="+prefix,
		runtimeSmokeOwnerEnv+"="+ownerID,
		"GORACE=halt_on_error=1",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		processCancel()
		t.Fatalf("open runtime smoke stdout: %v", err)
	}
	process := &runtimeSmokeProcess{cmd: cmd, cancel: processCancel, stdout: stdout, waitDone: make(chan struct{}), scanDone: make(chan struct{})}
	cmd.Stderr = &process.stderr
	readyCh := make(chan runtimeSmokeReady, 1)
	if err := cmd.Start(); err != nil {
		processCancel()
		t.Fatalf("start runtime smoke process %s: %v", ownerID, err)
	}
	go func() {
		defer close(process.scanDone)
		scanner := bufio.NewScanner(stdout)
		readySent := false
		for scanner.Scan() {
			line := scanner.Text()
			process.output.WriteString(line)
			process.output.WriteByte('\n')
			var ready map[string]string
			if !readySent && json.Unmarshal([]byte(line), &ready) == nil && strings.TrimSpace(ready["url"]) != "" {
				readySent = true
				readyCh <- runtimeSmokeReady{url: strings.TrimSpace(ready["url"])}
			}
		}
		if !readySent {
			scanErr := scanner.Err()
			if scanErr == nil {
				scanErr = io.EOF
			}
			readyCh <- runtimeSmokeReady{err: fmt.Errorf("runtime smoke process exited before readiness: %w", scanErr)}
		}
	}()
	go func() {
		<-process.scanDone
		process.waitErr = cmd.Wait()
		close(process.waitDone)
	}()

	select {
	case ready := <-readyCh:
		if ready.err != nil {
			_ = process.stop(false)
			t.Fatalf("start runtime smoke process %s: %v\nstdout:\n%s\nstderr:\n%s", ownerID, ready.err, process.output.String(), process.stderr.String())
		}
		process.url = ready.url
	case <-time.After(10 * time.Second):
		_ = process.stop(false)
		t.Fatalf("timed out starting runtime smoke process %s\nstdout:\n%s\nstderr:\n%s", ownerID, process.output.String(), process.stderr.String())
	}
	t.Cleanup(func() {
		if err := process.stop(false); err != nil {
			t.Errorf("stop runtime smoke process %s: %v", ownerID, err)
		}
	})
	return process
}

func runtimeSmokeProcessEnv(overrides ...string) []string {
	keys := make(map[string]struct{}, len(overrides))
	for _, override := range overrides {
		key, _, _ := strings.Cut(override, "=")
		keys[key] = struct{}{}
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := keys[key]; !replaced {
			environment = append(environment, entry)
		}
	}
	return append(environment, overrides...)
}

func (p *runtimeSmokeProcess) stop(graceful bool) error {
	p.stopOnce.Do(func() {
		defer p.cancel()
		if graceful {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url+"/control/shutdown", nil)
			if err == nil {
				client := &http.Client{Timeout: 2 * time.Second}
				var response *http.Response
				response, err = client.Do(request) //nolint:gosec // p.url is emitted by this test's local httptest server.
				if response != nil {
					_, _ = io.Copy(io.Discard, response.Body)
					_ = response.Body.Close()
				}
			}
			if err != nil {
				p.stopErr = fmt.Errorf("request graceful shutdown: %w", err)
				_ = p.cmd.Process.Kill()
			}
		} else {
			select {
			case <-p.waitDone:
				if p.waitErr == nil {
					p.stopErr = fmt.Errorf("runtime smoke process exited cleanly before forced stop\nstderr:\n%s", p.stderr.String())
				} else {
					p.stopErr = fmt.Errorf("runtime smoke process exited before forced stop\nstderr:\n%s: %w", p.stderr.String(), p.waitErr)
				}
				return
			default:
			}
			if err := p.cmd.Process.Kill(); err != nil {
				p.stopErr = fmt.Errorf("force-stop runtime smoke process: %w", err)
			}
		}
		select {
		case <-p.waitDone:
		case <-time.After(5 * time.Second):
			_ = p.cmd.Process.Kill()
			_ = p.stdout.Close()
			select {
			case <-p.waitDone:
			case <-time.After(2 * time.Second):
				if p.stopErr == nil {
					p.stopErr = errors.New("runtime smoke process did not stop after kill")
				}
				return
			}
		}
		if graceful && p.waitErr != nil && p.stopErr == nil {
			p.stopErr = fmt.Errorf("runtime smoke process exit: %w\nstderr:\n%s", p.waitErr, p.stderr.String())
		}
		if !graceful && p.stopErr == nil {
			stderr := p.stderr.String()
			if p.waitErr == nil {
				p.stopErr = errors.New("runtime smoke process exited cleanly during forced stop")
			} else if strings.Contains(stderr, "WARNING: DATA RACE") || strings.Contains(stderr, "panic:") {
				p.stopErr = fmt.Errorf("runtime smoke process failed before forced stop\nstderr:\n%s", stderr)
			}
		}
	})
	return p.stopErr
}

func runtimeSmokeRedisURL(t *testing.T) string {
	t.Helper()
	redisURL := strings.TrimSpace(os.Getenv("MEMOH_TEST_REDIS_URL"))
	if redisURL == "" {
		redisURL = strings.TrimSpace(os.Getenv("MEMOH_TEST_VALKEY_URL"))
	}
	if redisURL == "" {
		if os.Getenv("MEMOH_TEST_DISTRIBUTED_REQUIRED") == "1" {
			t.Fatal("multiprocess runtime smoke test is required, but neither MEMOH_TEST_REDIS_URL nor MEMOH_TEST_VALKEY_URL is set")
		}
		t.Skip("set MEMOH_TEST_REDIS_URL or MEMOH_TEST_VALKEY_URL to run the multiprocess runtime smoke test")
	}
	return redisURL
}

func postRuntimeSmokeControl(t *testing.T, process *runtimeSmokeProcess, path string, payload any) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal runtime smoke request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, process.url+path, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("create runtime smoke request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request) //nolint:gosec // process.url is emitted by this test's local httptest server.
	if err != nil {
		t.Fatalf("post runtime smoke control %s: %v", path, err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read runtime smoke control %s: %v", path, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("runtime smoke control %s returned %s: %s", path, response.Status, body)
	}
}

func readRuntimeSmokeState(t *testing.T, process *runtimeSmokeProcess) runtimeSmokeStateView {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, process.url+"/control/state", nil)
	if err != nil {
		t.Fatalf("create runtime smoke state request: %v", err)
	}
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request) //nolint:gosec // process.url is emitted by this test's local httptest server.
	if err != nil {
		t.Fatalf("read runtime smoke state: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("runtime smoke state returned %s: %s", response.Status, body)
	}
	var state runtimeSmokeStateView
	if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
		t.Fatalf("decode runtime smoke state: %v", err)
	}
	return state
}

func waitRuntimeSmokeState(t *testing.T, process *runtimeSmokeProcess, pred func(runtimeSmokeStateView) bool) runtimeSmokeStateView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var state runtimeSmokeStateView
	for time.Now().Before(deadline) {
		state = readRuntimeSmokeState(t, process)
		if pred(state) {
			return state
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for runtime smoke state: %#v", state)
	return runtimeSmokeStateView{}
}

func dialRuntimeSmokeWS(t *testing.T, process *runtimeSmokeProcess) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(process.url, "http") + "/bots/" + runtimeContractBotID + "/local/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = 5 * time.Second
	client, response, err := dialer.DialContext(ctx, wsURL, nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial runtime smoke websocket: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func writeRuntimeSmokeWS(t *testing.T, client *websocket.Conn, label string, payload any) {
	t.Helper()
	if err := client.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set %s write deadline: %v", label, err)
	}
	if err := client.WriteJSON(payload); err != nil {
		t.Fatalf("%s: %v", label, err)
	}
}

func readRuntimeSmokeEventUntil(t *testing.T, client *websocket.Conn, pred func(sessionruntime.Event) bool) sessionruntime.Event {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var events []sessionruntime.Event
	for {
		if err := client.SetReadDeadline(deadline); err != nil {
			t.Fatalf("set runtime smoke read deadline: %v", err)
		}
		var event sessionruntime.Event
		if err := client.ReadJSON(&event); err != nil {
			t.Fatalf("read runtime smoke event: %v; events=%#v", err, events)
		}
		events = append(events, event)
		if pred(event) {
			return event
		}
	}
}

func TestLocalChannelRuntimeMultiprocessSmoke(t *testing.T) {
	redisURL := runtimeSmokeRedisURL(t)
	prefix := uniqueHandlerRuntimePrefix("multiprocess")
	serverA := startRuntimeSmokeProcess(t, redisURL, prefix, "runtime-smoke-server-a")
	serverB := startRuntimeSmokeProcess(t, redisURL, prefix, "runtime-smoke-server-b")

	postRuntimeSmokeControl(t, serverB, "/control/start", map[string]string{
		"stream_id": runtimeContractStreamID,
		"text":      " Owner B initial output.",
	})
	clientA := dialRuntimeSmokeWS(t, serverA)
	writeRuntimeSmokeWS(t, clientA, "subscribe to runtime through server A", map[string]any{
		"type":          "runtime_subscribe",
		"session_id":    runtimeContractSessionID,
		"invocation_id": "subscribe-on-a",
	})
	snapshotEvent := readRuntimeSmokeEventUntil(t, clientA, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeSnapshot
	})
	if snapshotEvent.Snapshot == nil || snapshotEvent.Snapshot.CurrentRunView == nil {
		t.Fatalf("server A runtime snapshot = %#v", snapshotEvent)
	}
	initialRun := snapshotEvent.Snapshot.CurrentRunView
	if initialRun.StreamID != runtimeContractStreamID || initialRun.OwnerID != "runtime-smoke-server-b" || initialRun.Status != sessionruntime.RunStatusRunning {
		t.Fatalf("server A attached run = %#v", initialRun)
	}
	if !runtimeSnapshotHasMessage(*snapshotEvent.Snapshot, conversation.UIMessageText, "", "Owner B initial output.") {
		t.Fatalf("server A snapshot messages = %#v", initialRun.Messages)
	}

	postRuntimeSmokeControl(t, serverB, "/control/delta", map[string]string{"text": " Cross-process delta."})
	readRuntimeSmokeEventUntil(t, clientA, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeDelta && runtimeDeltaHasMessageAppend(event, conversation.UIMessageText, " Cross-process delta.")
	})

	writeRuntimeSmokeWS(t, clientA, "steer owner B through server A", map[string]any{
		"type":       "steer_current_run",
		"stream_id":  runtimeContractStreamID,
		"session_id": runtimeContractSessionID,
		"generation": initialRun.Generation,
		"text":       "steer from server A",
	})
	waitRuntimeSmokeState(t, serverB, func(state runtimeSmokeStateView) bool {
		return len(state.Steers) == 1 && state.Steers[0] == "steer from server A"
	})
	readRuntimeSmokeEventUntil(t, clientA, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeDelta && runtimeDeltaSteerStatus(event) == sessionruntime.SteerStatusApplied
	})

	writeRuntimeSmokeWS(t, clientA, "approve owner B tool through server A", map[string]any{
		"type":          "tool_approval_response",
		"stream_id":     "approval-through-a",
		"session_id":    runtimeContractSessionID,
		"invocation_id": "approval-through-a",
		"approval_id":   "approval-1",
		"decision":      "approve",
	})
	approvalResult := readCommandEventUntil(t, clientA, "approval-through-a")
	if approvalResult.Type != "command_result" || approvalResult.ActionID != sessionruntime.CommandToolApprovalResponse {
		t.Fatalf("cross-process approval result = %#v", approvalResult)
	}
	waitRuntimeSmokeState(t, serverB, func(state runtimeSmokeStateView) bool {
		return state.ApprovalID == "approval-1" && state.ApprovalDecision == "approve" && state.ApprovalResolvedByOwner
	})

	if err := serverB.stop(false); err != nil {
		t.Fatalf("crash owner server B: %v", err)
	}
	readRuntimeSmokeEventUntil(t, clientA, func(event sessionruntime.Event) bool {
		if event.Snapshot != nil && event.Snapshot.CurrentRunView != nil {
			return event.Snapshot.CurrentRunView.Status == sessionruntime.RunStatusLost
		}
		return event.Type == sessionruntime.EventRuntimeDelta && runtimeDeltaRunStatus(event) == sessionruntime.RunStatusLost
	})

	restartedB := startRuntimeSmokeProcess(t, redisURL, prefix, "runtime-smoke-server-b-restarted")
	postRuntimeSmokeControl(t, restartedB, "/control/start", map[string]string{
		"stream_id": "stream-after-owner-restart",
		"text":      " Owner B restarted output.",
	})
	readRuntimeSmokeEventUntil(t, clientA, func(event sessionruntime.Event) bool {
		return event.StreamID == "stream-after-owner-restart" && runtimeDeltaRunStatus(event) == sessionruntime.RunStatusRunning
	})

	if err := clientA.Close(); err != nil {
		t.Fatalf("close server A websocket before restart: %v", err)
	}
	if err := serverA.stop(true); err != nil {
		t.Fatalf("restart observer server A: %v", err)
	}
	restartedA := startRuntimeSmokeProcess(t, redisURL, prefix, "runtime-smoke-server-a")
	clientAfterRestart := dialRuntimeSmokeWS(t, restartedA)
	defer func() { _ = clientAfterRestart.Close() }()
	writeRuntimeSmokeWS(t, clientAfterRestart, "subscribe after server A restart", map[string]any{
		"type":          "runtime_subscribe",
		"session_id":    runtimeContractSessionID,
		"invocation_id": "subscribe-after-a-restart",
	})
	rehydrated := readRuntimeSmokeEventUntil(t, clientAfterRestart, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeSnapshot
	})
	if rehydrated.Snapshot == nil || rehydrated.Snapshot.CurrentRunView == nil {
		t.Fatalf("rehydrated runtime snapshot = %#v", rehydrated)
	}
	rehydratedRun := rehydrated.Snapshot.CurrentRunView
	if rehydratedRun.StreamID != "stream-after-owner-restart" || rehydratedRun.OwnerID != "runtime-smoke-server-b-restarted" || rehydratedRun.Status != sessionruntime.RunStatusRunning {
		t.Fatalf("rehydrated current run = %#v", rehydratedRun)
	}
	if !runtimeSnapshotHasMessage(*rehydrated.Snapshot, conversation.UIMessageText, "", "Owner B restarted output.") {
		t.Fatalf("rehydrated messages = %#v", rehydratedRun.Messages)
	}

	postRuntimeSmokeControl(t, restartedB, "/control/delta", map[string]string{"text": " Delta after server A restart."})
	readRuntimeSmokeEventUntil(t, clientAfterRestart, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeDelta && runtimeDeltaHasMessageAppend(event, conversation.UIMessageText, " Delta after server A restart.")
	})
	writeRuntimeSmokeWS(t, clientAfterRestart, "abort restarted owner B through restarted server A", map[string]any{
		"type":       "abort",
		"stream_id":  "stream-after-owner-restart",
		"session_id": runtimeContractSessionID,
		"generation": rehydratedRun.Generation,
	})
	waitRuntimeSmokeState(t, restartedB, func(state runtimeSmokeStateView) bool {
		return state.Aborted && state.Canceled
	})
	readRuntimeSmokeEventUntil(t, clientAfterRestart, func(event sessionruntime.Event) bool {
		return event.Type == sessionruntime.EventRuntimeDelta && runtimeDeltaRunStatus(event) == sessionruntime.RunStatusAborting
	})
	if err := clientAfterRestart.Close(); err != nil {
		t.Fatalf("close restarted server A websocket: %v", err)
	}
	if err := restartedA.stop(true); err != nil {
		t.Fatalf("stop restarted observer server A: %v", err)
	}
	if err := restartedB.stop(true); err != nil {
		t.Fatalf("stop restarted owner server B: %v", err)
	}
}
