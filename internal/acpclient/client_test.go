package acpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/workspace/bridge"
	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
	"github.com/memohai/memoh/internal/workspace/bridgesvc"
)

type testWorkspace struct {
	client *bridge.Client
	info   bridge.WorkspaceInfo
}

func (w testWorkspace) MCPClient(context.Context, string) (*bridge.Client, error) {
	return w.client, nil
}

func (w testWorkspace) WorkspaceInfo(context.Context, string) (bridge.WorkspaceInfo, error) {
	return w.info, nil
}

type rotatingTestWorkspace struct {
	info    bridge.WorkspaceInfo
	clients []*bridge.Client
	calls   int
}

func (w *rotatingTestWorkspace) MCPClient(context.Context, string) (*bridge.Client, error) {
	if w.calls >= len(w.clients) {
		return nil, errors.New("no more test clients")
	}
	client := w.clients[w.calls]
	w.calls++
	return client, nil
}

func (w *rotatingTestWorkspace) WorkspaceInfo(context.Context, string) (bridge.WorkspaceInfo, error) {
	return w.info, nil
}

func TestRunnerRunLocalWorkspaceFakeAgent(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "input.txt"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	client := newTestBridgeClient(t, root)
	agentPath := writeFakeAgentScript(t, root)
	runner := NewRunner(nil, testWorkspace{
		client: client,
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})

	result, err := runner.Run(context.Background(), RunRequest{
		BotID:       "bot-1",
		Task:        "touch the project",
		ProjectPath: "/data/project",
		Command:     agentPath,
		Timeout:     10 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(result.Text, "read: hello") {
		t.Fatalf("result text missing read content: %q", result.Text)
	}
	if !strings.Contains(result.Text, "term: terminal-ok") {
		t.Fatalf("result text missing terminal output: %q", result.Text)
	}
	if result.StopReason != string(acp.StopReasonEndTurn) {
		t.Fatalf("StopReason = %q, want %q", result.StopReason, acp.StopReasonEndTurn)
	}
	if got, err := os.ReadFile(filepath.Join(project, "output.txt")); err != nil { //nolint:gosec // test path is under t.TempDir.
		t.Fatalf("read output file: %v", err)
	} else if string(got) != "written by fake agent\n" {
		t.Fatalf("output file = %q", got)
	}
}

func TestRunnerRequiresACPCommand(t *testing.T) {
	root := t.TempDir()
	client := newTestBridgeClient(t, root)
	runner := NewRunner(nil, testWorkspace{
		client: client,
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})

	_, err := runner.Run(context.Background(), RunRequest{
		BotID:   "bot-1",
		Task:    "fix tests",
		Timeout: 2 * time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "ACP command is required") {
		t.Fatalf("Run() error = %v, want missing command error", err)
	}
}

func TestRunnerStartSessionStreamsEvents(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "input.txt"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	client := newTestBridgeClient(t, root)
	agentPath := writeFakeAgentScript(t, root)
	runner := NewRunner(nil, testWorkspace{
		client: client,
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})

	var streamedMu sync.Mutex
	var streamed strings.Builder
	var streamedEvents []StreamEvent
	startupCtx, cancelStartup := context.WithCancel(context.Background())
	sess, err := runner.StartSession(startupCtx, StartRequest{
		BotID:       "bot-1",
		ProjectPath: "/data/project",
		Command:     agentPath,
		Timeout:     10 * time.Second,
	}, EventSinkFunc(func(event StreamEvent) {
		streamedMu.Lock()
		defer streamedMu.Unlock()
		streamedEvents = append(streamedEvents, event)
		if event.Type == StreamEventTextDelta {
			streamed.WriteString(event.Delta)
		}
	}))
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	defer func() { _ = sess.Close() }()
	cancelStartup()

	result, err := sess.Prompt(context.Background(), "touch the project")
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if result.StopReason != string(acp.StopReasonEndTurn) {
		t.Fatalf("StopReason = %q, want %q", result.StopReason, acp.StopReasonEndTurn)
	}
	streamedMu.Lock()
	streamedText := streamed.String()
	streamedEventsSnapshot := append([]StreamEvent(nil), streamedEvents...)
	streamedMu.Unlock()
	if !strings.Contains(streamedText, "read: hello") {
		t.Fatalf("streamed text = %q", streamedText)
	}
	for _, want := range []string{"read", "write", "exec"} {
		if !hasStreamedToolEvent(streamedEventsSnapshot, StreamEventToolCallEnd, want) {
			t.Fatalf("streamed events missing %s tool end: %#v", want, streamedEventsSnapshot)
		}
		if !hasStreamedToolEvent(result.Events, StreamEventToolCallEnd, want) {
			t.Fatalf("result events missing %s tool end: %#v", want, result.Events)
		}
	}
}

func TestSessionPromptBuildsEmbeddedContextResource(t *testing.T) {
	t.Parallel()

	markdown := "# Memoh Context\n\nRemember the project preference."
	sess := &Session{embeddedContext: true}
	blocks := sess.promptBlocks("inspect the app", []PromptResource{{
		URI:      "memoh://context/current-turn",
		MimeType: "text/markdown",
		Text:     markdown,
	}})
	if len(blocks) != 2 {
		t.Fatalf("prompt blocks = %d, want text + resource", len(blocks))
	}
	if blocks[0].Text == nil || blocks[0].Text.Text != "inspect the app" {
		t.Fatalf("first block = %#v, want user text", blocks[0])
	}
	if blocks[1].Resource == nil || blocks[1].Resource.Resource.TextResourceContents == nil {
		t.Fatalf("second block = %#v, want embedded text resource", blocks[1])
	}
	resource := blocks[1].Resource.Resource.TextResourceContents
	if resource.Uri != "memoh://context/current-turn" || resource.MimeType == nil || *resource.MimeType != "text/markdown" || resource.Text != markdown {
		t.Fatalf("resource = %#v, want Memoh markdown context", resource)
	}
}

func TestSessionPromptFallsBackToTextContextWhenEmbeddedContextUnsupported(t *testing.T) {
	t.Parallel()

	sess := &Session{}
	blocks := sess.promptBlocks("inspect the app", []PromptResource{{
		URI:      "memoh://context/current-turn",
		MimeType: "text/markdown",
		Text:     "Memoh context",
	}})
	if len(blocks) != 1 || blocks[0].Text == nil {
		t.Fatalf("prompt blocks = %#v, want single text fallback", blocks)
	}
	text := blocks[0].Text.Text
	if !strings.Contains(text, `<context ref="memoh://context/current-turn">`) || !strings.Contains(text, "Memoh context") || !strings.Contains(text, "inspect the app") {
		t.Fatalf("fallback text = %q, want context and prompt", text)
	}
}

func TestStartMemohToolsBridgeRetriesClosingWorkspaceClient(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	stale := newTestBridgeClient(t, root)
	if err := stale.Close(); err != nil {
		t.Fatal(err)
	}
	fresh := newTestBridgeClient(t, root)
	workspace := &rotatingTestWorkspace{
		info: bridge.WorkspaceInfo{
			Backend:         bridge.WorkspaceBackendContainer,
			DefaultWorkDir:  "/data",
			ACPToolsHTTPURL: "http://127.0.0.1:18732/mcp",
		},
		clients: []*bridge.Client{fresh},
	}
	runner := NewRunner(nil, workspace)

	gotClient, stop, err := runner.startMemohToolsBridge(context.Background(), "bot-1", stale, "/mcp/test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	if err != nil {
		t.Fatalf("startMemohToolsBridge() error = %v", err)
	}
	defer stop()
	if gotClient != fresh {
		t.Fatalf("startMemohToolsBridge() client = %#v, want fresh client", gotClient)
	}
	if workspace.calls != 1 {
		t.Fatalf("workspace MCPClient calls = %d, want retry once", workspace.calls)
	}
}

func TestRunnerStartSessionSupportsReleaseTerminalWithoutWait(t *testing.T) {
	t.Setenv("MEMOH_ACP_FAKE_AGENT_RELEASE_TERMINAL_WITHOUT_WAIT", "1")
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}

	client := newTestBridgeClient(t, root)
	agentPath := writeFakeAgentScript(t, root)
	runner := NewRunner(nil, testWorkspace{
		client: client,
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})

	sess, err := runner.StartSession(context.Background(), StartRequest{
		BotID:       "bot-1",
		ProjectPath: "/data/project",
		Command:     agentPath,
		Timeout:     10 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	defer func() { _ = sess.Close() }()

	result, err := sess.Prompt(context.Background(), "check time")
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if !strings.Contains(result.Text, "term: terminal-ok") {
		t.Fatalf("result text = %q, want terminal output", result.Text)
	}
}

func hasStreamedToolEvent(events []StreamEvent, typ StreamEventType, toolName string) bool {
	for _, event := range events {
		if event.Type == typ && event.ToolName == toolName {
			return true
		}
	}
	return false
}

func TestRunnerStartSessionReadsProtocolModelsAndSetsModel(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEMOH_ACP_FAKE_AGENT_MODELS", "1")

	client := newTestBridgeClient(t, root)
	agentPath := writeFakeAgentScript(t, root)
	runner := NewRunner(nil, testWorkspace{
		client: client,
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})

	sess, err := runner.StartSession(context.Background(), StartRequest{
		BotID:       "bot-1",
		ProjectPath: "/data/project",
		Command:     agentPath,
		Timeout:     10 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	defer func() { _ = sess.Close() }()

	state := sess.ModelState()
	if !state.Supported || state.CurrentModelID != "gpt-5.1-codex" {
		t.Fatalf("ModelState() = %#v, want protocol model state", state)
	}
	if len(state.Available) != 2 || state.Available[1].ID != "gpt-5.1-codex-high" {
		t.Fatalf("available models = %#v", state.Available)
	}
	state.Available[0].Name = "mutated"
	if got := sess.ModelState().Available[0].Name; got == "mutated" {
		t.Fatalf("ModelState returned mutable slice")
	}

	state, err = sess.SetModel(context.Background(), "gpt-5.1-codex-high")
	if err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	if state.CurrentModelID != "gpt-5.1-codex-high" {
		t.Fatalf("SetModel state = %#v, want selected model", state)
	}
	if _, err := sess.SetModel(context.Background(), "gpt-5.1-codex-missing"); !errors.Is(err, ErrModelUnavailable) {
		t.Fatalf("SetModel(missing) error = %v, want ErrModelUnavailable", err)
	}
}

func TestRunnerStartSessionWithoutProtocolModelsDoesNotInventFallback(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}

	client := newTestBridgeClient(t, root)
	agentPath := writeFakeAgentScript(t, root)
	runner := NewRunner(nil, testWorkspace{
		client: client,
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})

	sess, err := runner.StartSession(context.Background(), StartRequest{
		BotID:       "bot-1",
		ProjectPath: "/data/project",
		Command:     agentPath,
		Timeout:     10 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	defer func() { _ = sess.Close() }()

	state := sess.ModelState()
	if state.Supported || state.CurrentModelID != "" || len(state.Available) != 0 {
		t.Fatalf("ModelState() = %#v, want unsupported with no fallback models", state)
	}
	if _, err := sess.SetModel(context.Background(), "gpt-5.1-codex"); !errors.Is(err, ErrModelSelectionUnsupported) {
		t.Fatalf("SetModel() error = %v, want ErrModelSelectionUnsupported", err)
	}
}

func TestRunnerStartSessionSendsNoMCPServers(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	capturePath := filepath.Join(root, "mcp-servers.json")
	t.Setenv("MEMOH_ACP_FAKE_AGENT_MCP_HTTP", "1")
	t.Setenv("MEMOH_ACP_FAKE_AGENT_CAPTURE_MCP_FILE", capturePath)

	client := newTestBridgeClient(t, root)
	agentPath := writeFakeAgentScript(t, root)
	runner := NewRunner(nil, testWorkspace{
		client: client,
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})

	sess, err := runner.StartSession(context.Background(), StartRequest{
		BotID:       "bot-1",
		ProjectPath: "/data/project",
		Command:     agentPath,
		Timeout:     10 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	defer func() { _ = sess.Close() }()

	raw, err := os.ReadFile(capturePath) //nolint:gosec // test path is under t.TempDir.
	if err != nil {
		t.Fatalf("read captured MCP servers: %v", err)
	}
	var servers []map[string]any
	if err := json.Unmarshal(raw, &servers); err != nil {
		t.Fatalf("decode captured MCP servers: %v", err)
	}
	if len(servers) != 0 {
		t.Fatalf("captured MCP servers = %#v, want none for basic ACP runtime", servers)
	}
}

func TestRunnerStartSessionInjectsHTTPToolServer(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	capturePath := filepath.Join(root, "mcp-servers.json")
	t.Setenv("MEMOH_ACP_FAKE_AGENT_MCP_HTTP", "1")
	t.Setenv("MEMOH_ACP_FAKE_AGENT_CAPTURE_MCP_FILE", capturePath)

	client := newTestBridgeClient(t, root)
	agentPath := writeFakeAgentScript(t, root)
	runner := NewRunner(nil, testWorkspace{
		client: client,
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})

	sess, err := runner.StartSession(context.Background(), StartRequest{
		BotID:       "bot-1",
		ProjectPath: "/data/project",
		Command:     agentPath,
		Timeout:     10 * time.Second,
		ToolHTTPURL: "http://memoh.test/bots/bot-1/tools",
		ToolHTTPHandler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{}}`))
		}),
		ToolSession: ToolSessionContext{
			BotID:             "bot-1",
			ChatID:            "chat-1",
			SessionID:         "session-1",
			StreamID:          "stream-1",
			SessionType:       "acp_agent",
			ChannelIdentityID: "user-1",
			SessionToken:      "token-1",
			ConversationType:  "private",
		},
	}, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	defer func() { _ = sess.Close() }()

	raw, err := os.ReadFile(capturePath) //nolint:gosec // test path is under t.TempDir.
	if err != nil {
		t.Fatalf("read captured MCP servers: %v", err)
	}
	var servers []map[string]any
	if err := json.Unmarshal(raw, &servers); err != nil {
		t.Fatalf("decode captured MCP servers: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("captured MCP servers = %#v, want one Memoh tools server", servers)
	}
	rawURL, _ := servers[0]["url"].(string)
	if servers[0]["type"] != "http" || !strings.HasPrefix(rawURL, "http://127.0.0.1:") || !strings.Contains(rawURL, "/mcp/") || servers[0]["name"] != "Memoh Tools" {
		t.Fatalf("captured MCP server = %#v", servers[0])
	}
	headers, _ := servers[0]["headers"].([]any)
	if hasCapturedHeaderName(headers, "Authorization") || hasCapturedHeaderName(headers, "X-Memoh-Session-Token") {
		t.Fatalf("captured credentials in MCP headers: %#v", headers)
	}
	if !hasCapturedHeader(headers, "X-Memoh-Session-Id", "session-1") {
		t.Fatalf("missing session id header in %#v", headers)
	}
	if !hasCapturedHeader(headers, "X-Memoh-Stream-Id", "stream-1") {
		t.Fatalf("missing stream id header in %#v", headers)
	}
	if !hasCapturedHeader(headers, "X-Memoh-Channel-Identity-Id", "user-1") {
		t.Fatalf("missing channel identity header in %#v", headers)
	}
	if !hasCapturedHeader(headers, "X-Memoh-Conversation-Type", "private") {
		t.Fatalf("missing conversation type header in %#v", headers)
	}
}

func TestRedactedToolHTTPURLHidesRouteSecrets(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "bot route",
			raw:  "http://memoh.test/bots/11111111-1111-1111-1111-111111111111/tools?token=secret#fragment",
			want: "http://memoh.test/bots/redacted/tools",
		},
		{
			name: "guard route",
			raw:  "http://127.0.0.1:12345/mcp/22222222-2222-2222-2222-222222222222",
			want: "http://127.0.0.1:12345/mcp/redacted",
		},
		{
			name: "non uuid route",
			raw:  "http://127.0.0.1:12345/mcp/local-secret",
			want: "http://127.0.0.1:12345/mcp/redacted",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactedToolHTTPURL(tc.raw); got != tc.want {
				t.Fatalf("redactedToolHTTPURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func hasCapturedHeader(headers []any, name, value string) bool {
	for _, raw := range headers {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if item["name"] == name && item["value"] == value {
			return true
		}
	}
	return false
}

func hasCapturedHeaderName(headers []any, name string) bool {
	for _, raw := range headers {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if item["name"] == name {
			return true
		}
	}
	return false
}

func TestSessionCloseCancelsActivePrompt(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	startedFile := filepath.Join(root, "prompt-started")
	cancelledFile := filepath.Join(root, "prompt-cancelled")
	t.Setenv("MEMOH_ACP_FAKE_AGENT_HANG_PROMPT", "1")
	t.Setenv("MEMOH_ACP_PROMPT_STARTED_FILE", startedFile)
	t.Setenv("MEMOH_ACP_PROMPT_CANCELLED_FILE", cancelledFile)

	client := newTestBridgeClient(t, root)
	agentPath := writeFakeAgentScript(t, root)
	runner := NewRunner(nil, testWorkspace{
		client: client,
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})

	sess, err := runner.StartSession(context.Background(), StartRequest{
		BotID:       "bot-1",
		ProjectPath: "/data/project",
		Command:     agentPath,
		Timeout:     10 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	promptErrCh := make(chan error, 1)
	go func() {
		_, err := sess.Prompt(context.Background(), "hang until close")
		promptErrCh <- err
	}()
	waitForFile(t, startedFile, 2*time.Second)

	closeErrCh := make(chan error, 1)
	go func() {
		closeErrCh <- sess.Close()
	}()
	select {
	case err := <-closeErrCh:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close blocked behind active Prompt")
	}
	waitForFile(t, cancelledFile, 2*time.Second)
	select {
	case err := <-promptErrCh:
		if err == nil {
			t.Fatal("Prompt returned nil error after Close cancelled it")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Prompt did not return after Close")
	}
}

//nolint:contextcheck // Close uses its own bounded cleanup context after startup cancellation.
func TestRunnerStartSessionCancellationStopsStartupProcess(t *testing.T) {
	root := t.TempDir()
	server := &startupCancelBridgeServer{
		processStarted:   make(chan struct{}),
		processCancelled: make(chan struct{}),
	}
	client := newStartupCancelBridgeClient(t, server)
	runner := NewRunner(nil, testWorkspace{
		client: client,
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		sess, err := runner.StartSession(ctx, StartRequest{
			BotID:       "bot-1",
			ProjectPath: "/data",
			Command:     "sh",
			Timeout:     time.Minute,
		}, nil)
		if sess != nil {
			_ = sess.Close()
		}
		errCh <- err
	}()

	select {
	case <-server.processStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("bridge process did not start")
	}
	cancel()
	select {
	case <-server.processCancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("bridge process context was not cancelled during startup")
	}
	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "initialize ACP agent") {
			t.Fatalf("StartSession() error = %v, want initialize failure after cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartSession did not return after startup cancellation")
	}
}

func TestRunnerRunContainerWorkspaceFakeAgent(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "input.txt"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	client := newTestBridgeClient(t, root)
	agentPath := writeFakeAgentScript(t, root)
	runner := NewRunner(nil, testWorkspace{
		client: client,
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendContainer,
			DefaultWorkDir: "/data",
		},
	})

	result, err := runner.Run(context.Background(), RunRequest{
		AgentID:     "codex",
		BotID:       "bot-1",
		Task:        "touch the project",
		ProjectPath: "/data/project",
		Command:     agentPath,
		SetupMode:   SetupModeAPIKey,
		Timeout:     10 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(result.Text, "read: hello") {
		t.Fatalf("result text missing read content: %q", result.Text)
	}
}

func TestRunnerMissingCommandIncludesStderr(t *testing.T) {
	root := t.TempDir()
	client := newTestBridgeClient(t, root)
	runner := NewRunner(nil, testWorkspace{
		client: client,
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})
	_, err := runner.Run(context.Background(), RunRequest{
		BotID:   "bot-1",
		Task:    "fix tests",
		Command: "memoh-definitely-missing-acp-command",
		Timeout: 2 * time.Second,
	})
	if err == nil {
		t.Fatal("expected missing command error")
	}
	if !strings.Contains(err.Error(), "memoh-definitely-missing-acp-command") {
		t.Fatalf("missing command error did not include stderr command detail: %v", err)
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Fatalf("missing command error is not actionable: %v", err)
	}
}

func TestRequestPermissionOnlyAutoAllowsOnce(t *testing.T) {
	callbacks := &clientCallbacks{root: "/data", cwd: "/data", virtualRoot: true}

	allowed, err := callbacks.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		ToolCall: acp.ToolCallUpdate{
			Locations: []acp.ToolCallLocation{{Path: "/data/output.txt"}},
			RawInput:  map[string]any{"path": "/data/output.txt", "cwd": "/data"},
		},
		Options: []acp.PermissionOption{
			{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow once", OptionId: acp.PermissionOptionId("once")},
		},
	})
	if err != nil {
		t.Fatalf("RequestPermission(allow_once) error = %v", err)
	}
	if allowed.Outcome.Selected == nil || allowed.Outcome.Selected.OptionId != acp.PermissionOptionId("once") {
		t.Fatalf("allow_once outcome = %#v, want selected once", allowed.Outcome)
	}

	always, err := callbacks.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		ToolCall: acp.ToolCallUpdate{
			Locations: []acp.ToolCallLocation{{Path: "/data/output.txt"}},
			RawInput:  map[string]any{"path": "/data/output.txt", "cwd": "/data"},
		},
		Options: []acp.PermissionOption{
			{Kind: acp.PermissionOptionKindAllowAlways, Name: "Allow always", OptionId: acp.PermissionOptionId("always")},
		},
	})
	if err != nil {
		t.Fatalf("RequestPermission(allow_always) error = %v", err)
	}
	if always.Outcome.Cancelled == nil {
		t.Fatalf("allow_always outcome = %#v, want cancelled because Memoh does not persist ACP permission grants", always.Outcome)
	}

	escaped, err := callbacks.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		ToolCall: acp.ToolCallUpdate{
			Locations: []acp.ToolCallLocation{{Path: "/outside.txt"}},
		},
		Options: []acp.PermissionOption{
			{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow once", OptionId: acp.PermissionOptionId("once")},
		},
	})
	if err != nil {
		t.Fatalf("RequestPermission(escaped) error = %v", err)
	}
	if escaped.Outcome.Cancelled == nil {
		t.Fatalf("escaped outcome = %#v, want cancelled", escaped.Outcome)
	}
}

func TestResolvePathUnderRootRejectsEscapeAndSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "app"), 0o750); err != nil {
		t.Fatal(err)
	}
	rootEval, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ResolvePathUnderRoot(root, "/data/app"); err != nil {
		t.Fatalf("ResolvePathUnderRoot(/data/app) error = %v", err)
	} else if got != filepath.Join(rootEval, "app") {
		t.Fatalf("ResolvePathUnderRoot(/data/app) = %q, want %q", got, filepath.Join(rootEval, "app"))
	}
	if _, err := ResolvePathUnderRoot(root, "../escape"); err == nil {
		t.Fatal("expected relative parent escape to be rejected")
	}

	link := filepath.Join(root, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := ResolvePathUnderRoot(root, filepath.Join(link, "file.txt")); err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}

func newTestBridgeClient(t *testing.T, root string) *bridge.Client {
	t.Helper()
	listener := bufconn.Listen(16 * 1024 * 1024)
	server := grpc.NewServer(
		grpc.MaxRecvMsgSize(16*1024*1024),
		grpc.MaxSendMsgSize(16*1024*1024),
	)
	pb.RegisterContainerServiceServer(server, bridgesvc.New(bridgesvc.Options{
		DefaultWorkDir:    root,
		WorkspaceRoot:     root,
		DataMount:         config.DefaultDataMount,
		AllowHostAbsolute: true,
	}))
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	conn, err := grpc.NewClient("passthrough:///acpclient-test",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(16*1024*1024),
			grpc.MaxCallSendMsgSize(16*1024*1024),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return bridge.NewClientFromConn(conn)
}

type startupCancelBridgeServer struct {
	pb.UnimplementedContainerServiceServer

	mu               sync.Mutex
	execs            int
	processStarted   chan struct{}
	processCancelled chan struct{}
}

func (s *startupCancelBridgeServer) Exec(stream grpc.BidiStreamingServer[pb.ExecInput, pb.ExecOutput]) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	s.mu.Lock()
	s.execs++
	execNumber := s.execs
	s.mu.Unlock()
	if execNumber == 1 {
		return stream.Send(&pb.ExecOutput{Stream: pb.ExecOutput_EXIT, ExitCode: 0})
	}
	close(s.processStarted)
	<-stream.Context().Done()
	close(s.processCancelled)
	return stream.Context().Err()
}

func newStartupCancelBridgeClient(t *testing.T, testServer *startupCancelBridgeServer) *bridge.Client {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterContainerServiceServer(server, testServer)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	conn, err := grpc.NewClient("passthrough:///acpclient-startup-cancel-test",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return bridge.NewClientFromConn(conn)
}

func writeFakeAgentScript(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "fake-acp-agent.sh")
	script := fmt.Sprintf("#!/bin/sh\nMEMOH_ACP_FAKE_AGENT=1 exec %s -test.run '^TestFakeACPAgentHelper$' --\n", escapeShellArg(os.Args[0]))
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil { //nolint:gosec // test helper must be executable.
		t.Fatal(err)
	}
	return path
}

func TestFakeACPAgentHelper(_ *testing.T) {
	if os.Getenv("MEMOH_ACP_FAKE_AGENT") != "1" {
		return
	}
	agent := &fakeACPAgent{}
	conn := acp.NewAgentSideConnection(agent, os.Stdout, os.Stdin)
	agent.conn = conn
	<-conn.Done()
	os.Exit(0)
}

type fakeACPAgent struct {
	conn *acp.AgentSideConnection
	cwd  string
}

func (*fakeACPAgent) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (*fakeACPAgent) Initialize(context.Context, acp.InitializeRequest) (acp.InitializeResponse, error) {
	capabilities := acp.AgentCapabilities{LoadSession: false}
	if os.Getenv("MEMOH_ACP_FAKE_AGENT_MCP_HTTP") == "1" {
		capabilities.McpCapabilities.Http = true
	}
	if os.Getenv("MEMOH_ACP_FAKE_AGENT_MCP_ACP") == "1" {
		capabilities.McpCapabilities.Acp = true
	}
	return acp.InitializeResponse{
		ProtocolVersion:   acp.ProtocolVersionNumber,
		AgentCapabilities: capabilities,
	}, nil
}

func (*fakeACPAgent) Cancel(context.Context, acp.CancelNotification) error {
	if path := os.Getenv("MEMOH_ACP_PROMPT_CANCELLED_FILE"); path != "" {
		_ = os.WriteFile(path, []byte("cancelled"), 0o600) //nolint:gosec // test helper writes to env-provided temp path.
	}
	return nil
}

func (*fakeACPAgent) CloseSession(context.Context, acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	return acp.CloseSessionResponse{}, nil
}

func (*fakeACPAgent) ListSessions(context.Context, acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
}

func (a *fakeACPAgent) NewSession(_ context.Context, p acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	a.cwd = p.Cwd
	if capturePath := os.Getenv("MEMOH_ACP_FAKE_AGENT_CAPTURE_MCP_FILE"); capturePath != "" {
		raw, err := json.Marshal(p.McpServers)
		if err != nil {
			return acp.NewSessionResponse{}, err
		}
		if err := os.WriteFile(capturePath, raw, 0o600); err != nil { //nolint:gosec // test helper writes to env-provided temp path.
			return acp.NewSessionResponse{}, err
		}
	}
	resp := acp.NewSessionResponse{SessionId: acp.SessionId("fake-session")}
	if os.Getenv("MEMOH_ACP_FAKE_AGENT_MODELS") == "1" {
		description := "Highest reasoning"
		resp.Models = &acp.SessionModelState{
			CurrentModelId: acp.ModelId("gpt-5.1-codex"),
			AvailableModels: []acp.ModelInfo{
				{ModelId: acp.ModelId("gpt-5.1-codex"), Name: "GPT-5.1 Codex"},
				{ModelId: acp.ModelId("gpt-5.1-codex-high"), Name: "GPT-5.1 Codex High", Description: &description},
			},
		}
	}
	return resp, nil
}

func (*fakeACPAgent) UnstableSetSessionModel(_ context.Context, p acp.UnstableSetSessionModelRequest) (acp.UnstableSetSessionModelResponse, error) {
	if p.SessionId != acp.SessionId("fake-session") {
		return acp.UnstableSetSessionModelResponse{}, fmt.Errorf("unexpected session id %q", p.SessionId)
	}
	if p.ModelId == "" {
		return acp.UnstableSetSessionModelResponse{}, errors.New("missing model id")
	}
	return acp.UnstableSetSessionModelResponse{}, nil
}

func (a *fakeACPAgent) Prompt(ctx context.Context, p acp.PromptRequest) (acp.PromptResponse, error) {
	if os.Getenv("MEMOH_ACP_FAKE_AGENT_HANG_PROMPT") == "1" {
		if path := os.Getenv("MEMOH_ACP_PROMPT_STARTED_FILE"); path != "" {
			_ = os.WriteFile(path, []byte("started"), 0o600) //nolint:gosec // test helper writes to env-provided temp path.
		}
		<-ctx.Done()
		if path := os.Getenv("MEMOH_ACP_PROMPT_CANCELLED_FILE"); path != "" {
			_ = os.WriteFile(path, []byte("cancelled"), 0o600) //nolint:gosec // test helper writes to env-provided temp path.
		}
		return acp.PromptResponse{}, ctx.Err()
	}
	if os.Getenv("MEMOH_ACP_FAKE_AGENT_RELEASE_TERMINAL_WITHOUT_WAIT") == "1" {
		return a.promptReleaseTerminalAfterOutput(ctx, p)
	}

	outputPath := filepath.Join(a.cwd, "output.txt")
	permission, err := a.conn.RequestPermission(ctx, acp.RequestPermissionRequest{
		SessionId: p.SessionId,
		ToolCall: acp.ToolCallUpdate{
			ToolCallId: acp.ToolCallId("write-output"),
			Title:      acp.Ptr("Write output file"),
			Kind:       acp.Ptr(acp.ToolKindEdit),
			Status:     acp.Ptr(acp.ToolCallStatusPending),
			Locations:  []acp.ToolCallLocation{{Path: outputPath}},
			RawInput:   map[string]any{"path": outputPath, "cwd": a.cwd},
		},
		Options: []acp.PermissionOption{
			{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow", OptionId: acp.PermissionOptionId("allow")},
			{Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject", OptionId: acp.PermissionOptionId("reject")},
		},
	})
	if err != nil {
		return acp.PromptResponse{}, err
	}
	if permission.Outcome.Selected == nil {
		return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
	}

	read, err := a.conn.ReadTextFile(ctx, acp.ReadTextFileRequest{
		SessionId: p.SessionId,
		Path:      filepath.Join(a.cwd, "input.txt"),
	})
	if err != nil {
		return acp.PromptResponse{}, err
	}
	if _, err := a.conn.WriteTextFile(ctx, acp.WriteTextFileRequest{
		SessionId: p.SessionId,
		Path:      outputPath,
		Content:   "written by fake agent\n",
	}); err != nil {
		return acp.PromptResponse{}, err
	}

	term, err := a.conn.CreateTerminal(ctx, acp.CreateTerminalRequest{
		SessionId: p.SessionId,
		Command:   "printf",
		Args:      []string{"terminal-ok"},
		Cwd:       &a.cwd,
	})
	if err != nil {
		return acp.PromptResponse{}, err
	}
	if _, err := a.conn.WaitForTerminalExit(ctx, acp.WaitForTerminalExitRequest{SessionId: p.SessionId, TerminalId: term.TerminalId}); err != nil {
		return acp.PromptResponse{}, err
	}
	termOut, err := a.conn.TerminalOutput(ctx, acp.TerminalOutputRequest{SessionId: p.SessionId, TerminalId: term.TerminalId})
	if err != nil {
		return acp.PromptResponse{}, err
	}
	_, _ = a.conn.ReleaseTerminal(ctx, acp.ReleaseTerminalRequest{SessionId: p.SessionId, TerminalId: term.TerminalId})
	_ = a.conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: p.SessionId,
		Update: acp.UpdateAgentMessageText(
			"read: " + strings.TrimSpace(read.Content) + " term: " + strings.TrimSpace(termOut.Output),
		),
	})
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func (a *fakeACPAgent) promptReleaseTerminalAfterOutput(ctx context.Context, p acp.PromptRequest) (acp.PromptResponse, error) {
	term, err := a.conn.CreateTerminal(ctx, acp.CreateTerminalRequest{
		SessionId: p.SessionId,
		Command:   "printf",
		Args:      []string{"terminal-ok"},
		Cwd:       &a.cwd,
	})
	if err != nil {
		return acp.PromptResponse{}, err
	}

	var termOut acp.TerminalOutputResponse
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		termOut, err = a.conn.TerminalOutput(ctx, acp.TerminalOutputRequest{SessionId: p.SessionId, TerminalId: term.TerminalId})
		if err != nil {
			return acp.PromptResponse{}, err
		}
		if strings.Contains(termOut.Output, "terminal-ok") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(termOut.Output, "terminal-ok") {
		return acp.PromptResponse{}, fmt.Errorf("terminal output = %q, want terminal-ok", termOut.Output)
	}
	if _, err := a.conn.ReleaseTerminal(ctx, acp.ReleaseTerminalRequest{SessionId: p.SessionId, TerminalId: term.TerminalId}); err != nil {
		return acp.PromptResponse{}, err
	}
	_ = a.conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: p.SessionId,
		Update:    acp.UpdateAgentMessageText("term: " + strings.TrimSpace(termOut.Output)),
	})
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("file %s was not created within %s", path, timeout)
}

func (*fakeACPAgent) ResumeSession(context.Context, acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, nil
}

func (*fakeACPAgent) SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}

func (*fakeACPAgent) SetSessionMode(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}
