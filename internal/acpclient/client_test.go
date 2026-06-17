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
	sdk "github.com/memohai/twilight-ai/sdk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/agent/event"
	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/toolapproval"
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
	var streamedEvents []event.StreamEvent
	startupCtx, cancelStartup := context.WithCancel(context.Background())
	sess, err := runner.StartSession(startupCtx, StartRequest{
		BotID:       "bot-1",
		ProjectPath: "/data/project",
		Command:     agentPath,
		Timeout:     10 * time.Second,
	}, EventSinkFunc(func(ev event.StreamEvent) {
		streamedMu.Lock()
		defer streamedMu.Unlock()
		streamedEvents = append(streamedEvents, ev)
		if ev.Type == event.TextDelta {
			streamed.WriteString(ev.Delta)
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
	streamedEventsSnapshot := append([]event.StreamEvent(nil), streamedEvents...)
	streamedMu.Unlock()
	if !strings.Contains(streamedText, "read: hello") {
		t.Fatalf("streamed text = %q", streamedText)
	}
	for _, want := range []string{"read", "write", "exec"} {
		if !hasStreamedToolEvent(streamedEventsSnapshot, event.ToolCallEnd, want) {
			t.Fatalf("streamed events missing %s tool end: %#v", want, streamedEventsSnapshot)
		}
	}
	for _, want := range []string{"read", "write", "exec"} {
		if !hasStreamedToolEvent(result.Events, event.ToolCallEnd, want) {
			t.Fatalf("result events missing %s tool end: %#v", want, result.Events)
		}
	}
	writeEvent := findStreamedToolEvent(streamedEventsSnapshot, event.ToolCallEnd, "write")
	if writeEvent == nil {
		t.Fatalf("streamed events missing write tool end: %#v", streamedEventsSnapshot)
	}
	writeInput, ok := writeEvent.Input.(map[string]any)
	if !ok {
		t.Fatalf("write input = %#v, want object", writeEvent.Input)
	}
	if writeInput["path"] == "" || writeInput["content"] != "written by fake agent\n" {
		t.Fatalf("write input = %#v, want path and content", writeInput)
	}
}

func TestWriteToolInputTruncatesLargeContent(t *testing.T) {
	content := strings.Repeat("a", maxWriteToolContentPreview+1) + "\n"
	input := writeToolInput("/data/large.txt", content)

	if input["path"] != "/data/large.txt" {
		t.Fatalf("path = %#v", input["path"])
	}
	if input["content_truncated"] != true {
		t.Fatalf("content_truncated = %#v, want true", input["content_truncated"])
	}
	if input["content_bytes"] != len(content) {
		t.Fatalf("content_bytes = %#v, want %d", input["content_bytes"], len(content))
	}
	if input["content_line_count"] != 2 {
		t.Fatalf("content_line_count = %#v, want 2", input["content_line_count"])
	}
	preview, ok := input["content"].(string)
	if !ok {
		t.Fatalf("content = %#v, want string", input["content"])
	}
	if len(preview) > maxWriteToolContentPreview {
		t.Fatalf("preview length = %d, want <= %d", len(preview), maxWriteToolContentPreview)
	}
	if preview == content {
		t.Fatalf("preview should be truncated")
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

func hasStreamedToolEvent(events []event.StreamEvent, typ event.StreamEventType, toolName string) bool {
	return findStreamedToolEvent(events, typ, toolName) != nil
}

func findStreamedToolEvent(events []event.StreamEvent, typ event.StreamEventType, toolName string) *event.StreamEvent {
	for i := range events {
		if events[i].Type == typ && events[i].ToolName == toolName {
			return &events[i]
		}
	}
	return nil
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
			Kind:      acp.Ptr(acp.ToolKindRead),
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
			Kind:      acp.Ptr(acp.ToolKindRead),
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

func TestRequestPermissionUsesMemohToolApproval(t *testing.T) {
	t.Parallel()

	approval := &fakeACPToolApproval{
		decision: toolapproval.Request{
			ID:      "approval-1",
			ShortID: 9,
			Status:  toolapproval.StatusApproved,
		},
	}
	callbacks := &clientCallbacks{
		root:        "/data",
		cwd:         "/data",
		virtualRoot: true,
		approval:    approval,
		baseSession: ToolSessionContext{
			BotID:             "bot-1",
			SessionID:         "session-1",
			StreamID:          "stream-1",
			ChannelIdentityID: "channel-1",
			CurrentPlatform:   "web",
			ConversationType:  "private",
		},
		events: &toolEventEmitter{},
	}
	collector := newEventCollector()
	callbacks.setPromptState(collector, nil, callbacks.baseSession)

	resp, err := callbacks.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		ToolCall: acp.ToolCallUpdate{
			ToolCallId: acp.ToolCallId("write-1"),
			Title:      acp.Ptr("Write /data/review.txt"),
			Kind:       acp.Ptr(acp.ToolKindEdit),
			Locations:  []acp.ToolCallLocation{{Path: "/data/review.txt"}},
			RawInput: map[string]any{
				"path":    "/data/review.txt",
				"content": "review me\n",
			},
		},
		Options: []acp.PermissionOption{
			{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow", OptionId: acp.PermissionOptionId("allow")},
			{Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject", OptionId: acp.PermissionOptionId("reject")},
		},
	})
	if err != nil {
		t.Fatalf("RequestPermission error = %v", err)
	}
	if resp.Outcome.Selected == nil || resp.Outcome.Selected.OptionId != acp.PermissionOptionId("allow") {
		t.Fatalf("permission outcome = %#v, want allow once", resp.Outcome)
	}
	if approval.created.ToolCallID != "write-1" || approval.created.ToolName != "write" {
		t.Fatalf("approval input = %#v", approval.created)
	}
	if approval.created.BotID != "bot-1" || approval.created.SessionID != "session-1" || approval.created.ChannelIdentityID != "channel-1" {
		t.Fatalf("approval context = %#v", approval.created)
	}
	events := collector.result().Events
	if len(events) != 2 {
		t.Fatalf("events = %#v, want pending and approved approval events", events)
	}
	for i, status := range []string{toolapproval.StatusPending, toolapproval.StatusApproved} {
		if events[i].Type != event.ToolApprovalRequest ||
			events[i].ToolCallID != "write-1" ||
			events[i].ApprovalID != "approval-1" ||
			events[i].Status != status {
			t.Fatalf("approval event %d = %#v, want status %q", i, events[i], status)
		}
	}
	approvalPayload, _ := events[1].Metadata["approval"].(map[string]any)
	if approvalPayload["can_approve"] != false {
		t.Fatalf("approval event = %#v", events[0])
	}
}

type fakeACPToolSource struct {
	tools []mcp.ToolDescriptor
}

func (s fakeACPToolSource) ListTools(context.Context, mcp.ToolSessionContext) ([]mcp.ToolDescriptor, error) {
	return append([]mcp.ToolDescriptor(nil), s.tools...), nil
}

func (fakeACPToolSource) CallTool(context.Context, mcp.ToolSessionContext, string, map[string]any) (map[string]any, error) {
	return mcp.BuildToolSuccessResult(map[string]any{"ok": true}), nil
}

func testACPToolGateway(toolNames ...string) *mcp.ToolGatewayService {
	tools := make([]mcp.ToolDescriptor, 0, len(toolNames))
	for _, name := range toolNames {
		tools = append(tools, mcp.ToolDescriptor{
			Name:        name,
			InputSchema: map[string]any{"type": "object"},
		})
	}
	return mcp.NewToolGatewayService(nil, []mcp.ToolSource{fakeACPToolSource{tools: tools}})
}

func TestMCPPermissionPreflightFromRequestShapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		request    acp.RequestPermissionRequest
		want       mcpPermissionPreflight
		wantParsed bool
	}{
		{
			name: "Codex generic title",
			request: acp.RequestPermissionRequest{
				ToolCall: acp.ToolCallUpdate{
					Title: acp.Ptr("Approve MCP tool call"),
					Kind:  acp.Ptr(acp.ToolKindOther),
					RawInput: map[string]any{
						"server_name": memohToolsMCPServerName,
						"method":      "tools/call",
						"params":      map[string]any{"name": "ask_user"},
					},
				},
			},
			want: mcpPermissionPreflight{
				toolName:        "ask_user",
				serverName:      memohToolsMCPServerName,
				hasToolName:     true,
				supportedMethod: true,
				shape:           mcpPermissionShapeGenericTitle,
			},
			wantParsed: true,
		},
		{
			name: "Claude Code structured title",
			request: acp.RequestPermissionRequest{
				ToolCall: acp.ToolCallUpdate{
					Title:    acp.Ptr("mcp__Memoh_Tools__ask_user"),
					Kind:     acp.Ptr(acp.ToolKindOther),
					RawInput: map[string]any{"questions": []any{}},
				},
			},
			want: mcpPermissionPreflight{
				toolName:        "ask_user",
				serverName:      memohToolsMCPServerSlug,
				hasToolName:     true,
				supportedMethod: true,
				shape:           mcpPermissionShapeStructuredTitle,
			},
			wantParsed: true,
		},
		{
			name: "plain unknown tool title",
			request: acp.RequestPermissionRequest{
				ToolCall: acp.ToolCallUpdate{
					Title:    acp.Ptr("some_custom_tool"),
					Kind:     acp.Ptr(acp.ToolKindOther),
					RawInput: map[string]any{"value": "ok"},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := mcpPermissionPreflightFromRequest(tc.request)
			if ok != tc.wantParsed {
				t.Fatalf("parsed = %v, want %v", ok, tc.wantParsed)
			}
			if !tc.wantParsed {
				return
			}
			if got != tc.want {
				t.Fatalf("preflight = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestRequestPermissionUnmappedToolAllowsWithoutApproval pins the native
// parity rule for permission requests that do not map to ACP client
// capabilities: harmless ACP protocol permissions are allowed directly, and
// MCP preflights are allowed only when their structured tools/call payload
// points at Memoh's actual ACP tool gateway.
func TestRequestPermissionUnmappedToolAllowsWithoutApproval(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		toolGateway *mcp.ToolGatewayService
		toolCall    acp.ToolCallUpdate
	}{
		{
			name:        "ACP MCP gateway tool",
			toolGateway: testACPToolGateway("native_tool"),
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("unknown-1"),
				Title:      acp.Ptr("Approve MCP tool call"),
				Kind:       acp.Ptr(acp.ToolKind("other")),
				RawInput: map[string]any{
					"server_name": memohToolsMCPServerName,
					"method":      "tools/call",
					"params": map[string]any{
						"name":      "native_tool",
						"arguments": map[string]any{"value": "ok"},
					},
				},
			},
		},
		{
			name:        "ACP MCP gateway wrapped Codex request",
			toolGateway: testACPToolGateway("native_tool"),
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("unknown-wrapped"),
				Title:      acp.Ptr("Approve MCP tool call"),
				Kind:       acp.Ptr(acp.ToolKindOther),
				RawInput: map[string]any{
					"id":          "approval-1",
					"turn_id":     "turn-1",
					"server_name": memohToolsMCPServerName,
					"request": map[string]any{
						"name":      "native_tool",
						"arguments": map[string]any{"value": "ok"},
					},
				},
			},
		},
		{
			name:        "ACP MCP gateway Codex request without tool name",
			toolGateway: testACPToolGateway("ask_user"),
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("unknown-codex-request"),
				Title:      acp.Ptr("Approve MCP tool call"),
				Kind:       acp.Ptr(acp.ToolKindOther),
				RawInput: map[string]any{
					"id":          "approval-2",
					"turn_id":     "turn-1",
					"server_name": memohToolsMCPServerSlug,
					"request": map[string]any{
						"_meta":            map[string]any{},
						"message":          "choose one",
						"mode":             "select",
						"requested_schema": map[string]any{"type": "object"},
					},
				},
			},
		},
		{
			name:        "ACP MCP gateway CallTool params",
			toolGateway: testACPToolGateway("native_tool"),
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("unknown-params"),
				Title:      acp.Ptr("Approve MCP tool call"),
				Kind:       acp.Ptr(acp.ToolKind("other")),
				RawInput: map[string]any{
					"server_name": memohToolsMCPServerName,
					"name":        "native_tool",
					"arguments":   map[string]any{"value": "ok"},
				},
			},
		},
		{
			name:        "Claude Code MCP title with direct tool args",
			toolGateway: testACPToolGateway("ask_user"),
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("claude-ask-user"),
				Title:      acp.Ptr("mcp__Memoh_Tools__ask_user"),
				Kind:       acp.Ptr(acp.ToolKindOther),
				RawInput: map[string]any{
					"questions": []any{
						map[string]any{"id": "answer", "question": "choose one"},
					},
				},
			},
		},
		{
			name: "agent mode switch",
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("unknown-2"),
				Title:      acp.Ptr("Exit plan mode"),
				Kind:       acp.Ptr(acp.ToolKind("switch_mode")),
				RawInput:   map[string]any{"description": "approve a custom action"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			approval := &fakeACPToolApproval{}
			callbacks := &clientCallbacks{
				root:        "/data",
				cwd:         "/data",
				virtualRoot: true,
				approval:    approval,
				toolGateway: tc.toolGateway,
				baseSession: ToolSessionContext{
					BotID:             "bot-1",
					SessionID:         "session-1",
					StreamID:          "stream-1",
					ChannelIdentityID: "channel-1",
				},
				events: &toolEventEmitter{},
			}
			callbacks.setPromptState(newEventCollector(), nil, callbacks.baseSession)

			resp, err := callbacks.RequestPermission(context.Background(), acp.RequestPermissionRequest{
				ToolCall: tc.toolCall,
				Options: []acp.PermissionOption{
					{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow", OptionId: acp.PermissionOptionId("allow")},
					{Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject", OptionId: acp.PermissionOptionId("reject")},
				},
			})
			if err != nil {
				t.Fatalf("RequestPermission error = %v", err)
			}
			if resp.Outcome.Selected == nil || resp.Outcome.Selected.OptionId != acp.PermissionOptionId("allow") {
				t.Fatalf("permission outcome = %#v, want allow once", resp.Outcome)
			}
			if got := approval.createdCount(); got != 0 {
				t.Fatalf("pending approvals created = %d, want 0 (unmapped permissions bypass like native)", got)
			}
		})
	}
}

func TestRequestPermissionUnknownUnmappedToolCancels(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		toolGateway *mcp.ToolGatewayService
		toolCall    acp.ToolCallUpdate
	}{
		{
			name: "unknown direct title",
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("unknown-danger"),
				Title:      acp.Ptr("new_dangerous_tool"),
				Kind:       acp.Ptr(acp.ToolKindOther),
				RawInput:   map[string]any{"path": "/data/output.txt"},
			},
		},
		{
			name:        "generic MCP preflight without structured name",
			toolGateway: testACPToolGateway("native_tool"),
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("mcp-no-name"),
				Title:      acp.Ptr("Approve MCP tool call"),
				Kind:       acp.Ptr(acp.ToolKindOther),
				RawInput:   map[string]any{"description": "please call native_tool"},
			},
		},
		{
			name:        "generic MCP preflight with free text raw input",
			toolGateway: testACPToolGateway("native_tool"),
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("mcp-free-text"),
				Title:      acp.Ptr("Approve MCP tool call"),
				Kind:       acp.Ptr(acp.ToolKindOther),
				RawInput:   "tools/call native_tool",
			},
		},
		{
			name:        "generic MCP preflight for unknown tool",
			toolGateway: testACPToolGateway("native_tool"),
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("mcp-unknown"),
				Title:      acp.Ptr("Approve MCP tool call"),
				Kind:       acp.Ptr(acp.ToolKindOther),
				RawInput: map[string]any{
					"method": "tools/call",
					"params": map[string]any{"name": "external_tool"},
				},
			},
		},
		{
			name:        "generic MCP preflight without server name",
			toolGateway: testACPToolGateway("native_tool"),
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("mcp-missing-server"),
				Title:      acp.Ptr("Approve MCP tool call"),
				Kind:       acp.Ptr(acp.ToolKindOther),
				RawInput: map[string]any{
					"method": "tools/call",
					"params": map[string]any{
						"name":      "native_tool",
						"arguments": map[string]any{"value": "ok"},
					},
				},
			},
		},
		{
			name:        "generic MCP preflight for non-Memoh server",
			toolGateway: testACPToolGateway("native_tool"),
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("mcp-external-server"),
				Title:      acp.Ptr("Approve MCP tool call"),
				Kind:       acp.Ptr(acp.ToolKindOther),
				RawInput: map[string]any{
					"id":          "approval-external",
					"turn_id":     "turn-1",
					"server_name": "External Tools",
					"request": map[string]any{
						"name":      "native_tool",
						"arguments": map[string]any{"value": "ok"},
					},
				},
			},
		},
		{
			name:        "generic MCP preflight for non-Memoh server without tool name",
			toolGateway: testACPToolGateway("native_tool"),
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("mcp-external-server-no-name"),
				Title:      acp.Ptr("Approve MCP tool call"),
				Kind:       acp.Ptr(acp.ToolKindOther),
				RawInput: map[string]any{
					"id":          "approval-external",
					"turn_id":     "turn-1",
					"server_name": "External Tools",
					"request": map[string]any{
						"message":          "choose one",
						"mode":             "select",
						"requested_schema": map[string]any{"type": "object"},
					},
				},
			},
		},
		{
			name:        "generic MCP preflight for unsupported method",
			toolGateway: testACPToolGateway("native_tool"),
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("mcp-list"),
				Title:      acp.Ptr("Approve MCP tool call"),
				Kind:       acp.Ptr(acp.ToolKindOther),
				RawInput: map[string]any{
					"method": "tools/list",
					"params": map[string]any{},
				},
			},
		},
		{
			name:        "generic MCP preflight for Memoh server unsupported method",
			toolGateway: testACPToolGateway("native_tool"),
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("mcp-memoh-list"),
				Title:      acp.Ptr("Approve MCP tool call"),
				Kind:       acp.Ptr(acp.ToolKindOther),
				RawInput: map[string]any{
					"server_name": "Memoh Tools",
					"method":      "tools/list",
					"params":      map[string]any{},
				},
			},
		},
		{
			name:        "Claude Code MCP title for unknown Memoh tool",
			toolGateway: testACPToolGateway("native_tool"),
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("claude-unknown-tool"),
				Title:      acp.Ptr("mcp__Memoh_Tools__external_tool"),
				Kind:       acp.Ptr(acp.ToolKindOther),
				RawInput:   map[string]any{"value": "ok"},
			},
		},
		{
			name:        "Claude Code MCP title for non-Memoh server",
			toolGateway: testACPToolGateway("native_tool"),
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("claude-external-server"),
				Title:      acp.Ptr("mcp__External_Tools__native_tool"),
				Kind:       acp.Ptr(acp.ToolKindOther),
				RawInput:   map[string]any{"value": "ok"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			approval := &fakeACPToolApproval{}
			callbacks := &clientCallbacks{
				root:        "/data",
				cwd:         "/data",
				virtualRoot: true,
				approval:    approval,
				toolGateway: tc.toolGateway,
				baseSession: ToolSessionContext{
					BotID:             "bot-1",
					SessionID:         "session-1",
					StreamID:          "stream-1",
					ChannelIdentityID: "channel-1",
				},
				events: &toolEventEmitter{},
			}
			callbacks.setPromptState(newEventCollector(), nil, callbacks.baseSession)

			resp, err := callbacks.RequestPermission(context.Background(), acp.RequestPermissionRequest{
				ToolCall: tc.toolCall,
				Options: []acp.PermissionOption{
					{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow", OptionId: acp.PermissionOptionId("allow")},
					{Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject", OptionId: acp.PermissionOptionId("reject")},
				},
			})
			if err != nil {
				t.Fatalf("RequestPermission error = %v", err)
			}
			if resp.Outcome.Cancelled == nil {
				t.Fatalf("permission outcome = %#v, want cancelled for unknown unmapped permission", resp.Outcome)
			}
			if got := approval.createdCount(); got != 0 {
				t.Fatalf("pending approvals created = %d, want 0 for unknown unmapped permission", got)
			}
		})
	}
}

func TestRequestPermissionRejectedByMemohToolApprovalSelectsRejectOption(t *testing.T) {
	t.Parallel()

	approval := &fakeACPToolApproval{
		decision: toolapproval.Request{
			ID:            "approval-2",
			ShortID:       10,
			Status:        toolapproval.StatusRejected,
			DecidedByUser: true,
		},
	}
	callbacks := &clientCallbacks{
		root:        "/data",
		cwd:         "/data",
		virtualRoot: true,
		approval:    approval,
		baseSession: ToolSessionContext{
			BotID:     "bot-1",
			SessionID: "session-1",
			StreamID:  "stream-1",
		},
		events: &toolEventEmitter{},
	}
	collector := newEventCollector()
	callbacks.setPromptState(collector, nil, callbacks.baseSession)

	resp, err := callbacks.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		ToolCall: acp.ToolCallUpdate{
			ToolCallId: acp.ToolCallId("exec-1"),
			Title:      acp.Ptr("Shell"),
			Kind:       acp.Ptr(acp.ToolKindExecute),
			RawInput:   map[string]any{"command": "rm -rf *"},
		},
		Options: []acp.PermissionOption{
			{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow", OptionId: acp.PermissionOptionId("allow")},
			{Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject", OptionId: acp.PermissionOptionId("reject")},
		},
	})
	if err != nil {
		t.Fatalf("RequestPermission error = %v", err)
	}
	if resp.Outcome.Selected == nil || resp.Outcome.Selected.OptionId != acp.PermissionOptionId("reject") {
		t.Fatalf("permission outcome = %#v, want reject once", resp.Outcome)
	}
	if approval.created.ToolName != "exec" {
		t.Fatalf("approval input = %#v", approval.created)
	}
	var sawRejected bool
	for _, ev := range collector.result().Events {
		if ev.Type == event.ToolApprovalRequest &&
			ev.ToolCallID == "exec-1" &&
			ev.ApprovalID == "approval-2" &&
			ev.Status == toolapproval.StatusRejected {
			sawRejected = true
		}
	}
	if !sawRejected {
		t.Fatalf("events = %#v, want rejected approval update", collector.result().Events)
	}
}

func TestRequestPermissionSystemRejectedByMemohToolApprovalCancels(t *testing.T) {
	t.Parallel()

	approval := &fakeACPToolApproval{
		decision: toolapproval.Request{
			ID:             "approval-system-reject",
			ShortID:        11,
			Status:         toolapproval.StatusRejected,
			DecisionReason: "tool approval timed out",
		},
	}
	callbacks := &clientCallbacks{
		root:        "/data",
		cwd:         "/data",
		virtualRoot: true,
		approval:    approval,
		baseSession: ToolSessionContext{
			BotID:     "bot-1",
			SessionID: "session-1",
			StreamID:  "stream-1",
		},
		events: &toolEventEmitter{},
	}
	collector := newEventCollector()
	callbacks.setPromptState(collector, nil, callbacks.baseSession)

	resp, err := callbacks.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		ToolCall: acp.ToolCallUpdate{
			ToolCallId: acp.ToolCallId("exec-1"),
			Title:      acp.Ptr("Shell"),
			Kind:       acp.Ptr(acp.ToolKindExecute),
			RawInput:   map[string]any{"command": "rm -rf *"},
		},
		Options: []acp.PermissionOption{
			{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow", OptionId: acp.PermissionOptionId("allow")},
			{Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject", OptionId: acp.PermissionOptionId("reject")},
		},
	})
	if err != nil {
		t.Fatalf("RequestPermission error = %v", err)
	}
	if resp.Outcome.Cancelled == nil || resp.Outcome.Selected != nil {
		t.Fatalf("permission outcome = %#v, want cancellation for system rejection", resp.Outcome)
	}
}

func TestCreateTerminalUsesMemohToolApproval(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	client := newTestBridgeClient(t, root)
	approval := &fakeACPToolApproval{
		decision: toolapproval.Request{
			ID:      "approval-terminal",
			ShortID: 11,
			Status:  toolapproval.StatusApproved,
		},
	}
	callbacks := newClientCallbacks(
		context.Background(),
		client,
		"/data",
		"/data",
		time.Second,
		nil,
		nil,
		true,
		approval,
		nil,
		ToolSessionContext{
			BotID:             "bot-1",
			SessionID:         "session-1",
			StreamID:          "stream-1",
			ChannelIdentityID: "channel-1",
		},
		acpprofile.DefaultToolQuirks(),
	)
	collector := newEventCollector()
	callbacks.setPromptState(collector, nil, callbacks.baseSession)

	term, err := callbacks.CreateTerminal(context.Background(), acp.CreateTerminalRequest{
		Command: "printf",
		Args:    []string{"terminal-ok"},
	})
	if err != nil {
		t.Fatalf("CreateTerminal error = %v", err)
	}
	if approval.created.ToolCallID != "terminal-term-1" || approval.created.ToolName != "exec" {
		t.Fatalf("approval input = %#v", approval.created)
	}
	input, ok := approval.created.ToolInput.(map[string]any)
	if !ok || input["command"] != "printf terminal-ok" {
		t.Fatalf("approval command input = %#v", approval.created.ToolInput)
	}

	if _, err := callbacks.WaitForTerminalExit(context.Background(), acp.WaitForTerminalExitRequest{TerminalId: term.TerminalId}); err != nil {
		t.Fatalf("WaitForTerminalExit error = %v", err)
	}
	events := collector.result().Events
	var sawStart, sawApproval, sawEnd bool
	for _, ev := range events {
		if ev.ToolCallID != "terminal-term-1" {
			continue
		}
		switch ev.Type {
		case event.ToolCallStart:
			sawStart = ev.ToolName == "exec"
		case event.ToolApprovalRequest:
			sawApproval = ev.ApprovalID == "approval-terminal"
		case event.ToolCallEnd:
			sawEnd = ev.ToolName == "exec"
		}
	}
	if !sawStart || !sawApproval || !sawEnd {
		t.Fatalf("terminal events start=%v approval=%v end=%v events=%#v", sawStart, sawApproval, sawEnd, events)
	}
}

func TestCreateTerminalRejectedByMemohToolApprovalDoesNotStartTerminal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	client := newTestBridgeClient(t, root)
	approval := &fakeACPToolApproval{
		decision: toolapproval.Request{
			ID:            "approval-terminal-reject",
			ShortID:       12,
			Status:        toolapproval.StatusRejected,
			DecidedByUser: true,
		},
	}
	callbacks := newClientCallbacks(
		context.Background(),
		client,
		"/data",
		"/data",
		time.Second,
		nil,
		nil,
		true,
		approval,
		nil,
		ToolSessionContext{
			BotID:     "bot-1",
			SessionID: "session-1",
			StreamID:  "stream-1",
		},
		acpprofile.DefaultToolQuirks(),
	)
	collector := newEventCollector()
	callbacks.setPromptState(collector, nil, callbacks.baseSession)

	_, err := callbacks.CreateTerminal(context.Background(), acp.CreateTerminalRequest{
		Command: "pwd",
	})
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("CreateTerminal error = %v, want rejected", err)
	}
	result := collector.result()
	events := result.Events
	startIdx, endIdx := -1, -1
	for idx, ev := range events {
		if ev.ToolCallID != "terminal-term-1" {
			continue
		}
		switch ev.Type {
		case event.ToolCallStart:
			if startIdx < 0 {
				startIdx = idx
			}
		case event.ToolCallEnd:
			if strings.Contains(ev.Error, "rejected") {
				endIdx = idx
			}
		}
	}
	if startIdx < 0 || endIdx < 0 || startIdx > endIdx {
		t.Fatalf("events = %#v, want terminal tool_call_start before rejected tool_call_end", events)
	}
	if len(result.Output) != 2 {
		t.Fatalf("transcript output = %#v, want assistant tool call and tool result", result.Output)
	}
	if result.Output[0].Role != sdk.MessageRoleAssistant {
		t.Fatalf("first transcript message = %#v, want assistant tool call", result.Output[0])
	}
	if _, ok := result.Output[0].Content[0].(sdk.ToolCallPart); !ok {
		t.Fatalf("first transcript part = %#v, want tool call", result.Output[0].Content[0])
	}
	if _, ok := result.Output[1].Content[0].(sdk.ToolResultPart); !ok {
		t.Fatalf("second transcript part = %#v, want tool result", result.Output[1].Content[0])
	}
}

func TestWriteTextFileUsesMemohToolApproval(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	client := newTestBridgeClient(t, root)
	approval := &fakeACPToolApproval{
		decision: toolapproval.Request{
			ID:      "approval-write",
			ShortID: 13,
			Status:  toolapproval.StatusApproved,
		},
	}
	callbacks := newClientCallbacks(
		context.Background(),
		client,
		"/data",
		"/data",
		time.Second,
		nil,
		nil,
		true,
		approval,
		nil,
		ToolSessionContext{
			BotID:             "bot-1",
			SessionID:         "session-1",
			StreamID:          "stream-1",
			ChannelIdentityID: "channel-1",
		},
		acpprofile.DefaultToolQuirks(),
	)
	collector := newEventCollector()
	callbacks.setPromptState(collector, nil, callbacks.baseSession)

	if _, err := callbacks.WriteTextFile(context.Background(), acp.WriteTextFileRequest{
		Path:    "/data/review.txt",
		Content: "review me\n",
	}); err != nil {
		t.Fatalf("WriteTextFile error = %v", err)
	}
	if approval.created.ToolName != "write" {
		t.Fatalf("approval input = %#v", approval.created)
	}
	input, ok := approval.created.ToolInput.(map[string]any)
	if !ok || input["path"] != "/data/review.txt" || input["content"] != "review me\n" {
		t.Fatalf("approval tool input = %#v", approval.created.ToolInput)
	}
	written, err := os.ReadFile(filepath.Join(root, "review.txt")) //nolint:gosec // reads from t.TempDir
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(written) != "review me\n" {
		t.Fatalf("written content = %q", written)
	}
	assertSingleApprovalWithStartEnd(t, collector.result().Events, approval.created.ToolCallID, "write", "approval-write")
}

func TestWriteTextFileWithoutToolSessionIsRejectedWhenApprovalEnabled(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	client := newTestBridgeClient(t, root)
	callbacks := newClientCallbacks(
		context.Background(),
		client,
		"/data",
		"/data",
		time.Second,
		nil,
		nil,
		true,
		&fakeACPToolApproval{decision: toolapproval.Request{Status: toolapproval.StatusApproved}},
		nil,
		ToolSessionContext{BotID: "bot-1"},
		acpprofile.DefaultToolQuirks(),
	)
	collector := newEventCollector()
	callbacks.setPromptState(collector, nil, callbacks.baseSession)

	_, err := callbacks.WriteTextFile(context.Background(), acp.WriteTextFileRequest{
		Path:    "/data/review.txt",
		Content: "review me\n",
	})
	// No session identity means nobody could be asked: a system outcome, so
	// the message must say "not approved", never "rejected by user".
	if err == nil || !strings.Contains(err.Error(), "not approved") {
		t.Fatalf("WriteTextFile error = %v, want not approved", err)
	}
	if _, err := os.Stat(filepath.Join(root, "review.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("written file stat error = %v, want not exist", err)
	}
	events := collector.result().Events
	var sawRejectedEnd bool
	for _, ev := range events {
		if ev.Type == event.ToolCallEnd && ev.ToolName == "write" && strings.Contains(ev.Error, "not approved") {
			sawRejectedEnd = true
		}
	}
	if !sawRejectedEnd {
		t.Fatalf("events = %#v, want not-approved tool_call_end", events)
	}
}

// TestRequestPermissionNonInteractiveCancels asserts that system-side
// rejections (no live stream to ask a user) cancel the permission request
// instead of reporting a user rejection the agent would keep retrying against.
func TestRequestPermissionNonInteractiveCancels(t *testing.T) {
	t.Parallel()

	approval := &fakeACPToolApproval{}
	callbacks := &clientCallbacks{
		root:        "/data",
		cwd:         "/data",
		virtualRoot: true,
		approval:    approval,
		baseSession: ToolSessionContext{
			BotID:             "bot-1",
			SessionID:         "session-1",
			ChannelIdentityID: "channel-1",
			// No StreamID: nobody can see or answer the approval.
		},
		events: &toolEventEmitter{},
	}
	callbacks.setPromptState(newEventCollector(), nil, callbacks.baseSession)

	resp, err := callbacks.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		ToolCall: acp.ToolCallUpdate{
			ToolCallId: acp.ToolCallId("exec-bg"),
			Title:      acp.Ptr("rm -rf /data/tmp"),
			Kind:       acp.Ptr(acp.ToolKindExecute),
			RawInput:   map[string]any{"command": "rm -rf /data/tmp"},
		},
		Options: []acp.PermissionOption{
			{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow", OptionId: acp.PermissionOptionId("allow")},
			{Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject", OptionId: acp.PermissionOptionId("reject")},
		},
	})
	if err != nil {
		t.Fatalf("RequestPermission error = %v", err)
	}
	if resp.Outcome.Cancelled == nil {
		t.Fatalf("permission outcome = %#v, want cancelled for system rejection", resp.Outcome)
	}
}

// TestRequestPermissionScopeRejectsOutOfRootPaths guards the pre-ask scope
// gate: every key the native extraction reads a path from (file_path et al.)
// and diff-carried paths must be confined to the workspace root. A thick
// agent executes the approved action itself, so this gate is the only chance
// to stop an out-of-root target before a user is asked to approve it.
func TestRequestPermissionScopeRejectsOutOfRootPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		toolCall acp.ToolCallUpdate
	}{
		{
			name: "unmapped path outside root",
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("unknown-out-of-scope"),
				Title:      acp.Ptr("Custom action"),
				RawInput:   map[string]any{"path": "/etc/passwd"},
			},
		},
		{
			name: "file_path key outside root",
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("write-escape"),
				Title:      acp.Ptr("Write file"),
				Kind:       acp.Ptr(acp.ToolKindEdit),
				RawInput:   map[string]any{"file_path": "/etc/passwd", "content": "x"},
			},
		},
		{
			name: "filePath key outside root",
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("write-escape-camel"),
				Title:      acp.Ptr("Write file"),
				Kind:       acp.Ptr(acp.ToolKindEdit),
				RawInput:   map[string]any{"filePath": "/etc/passwd", "content": "x"},
			},
		},
		{
			name: "diff path outside root",
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("edit-escape-diff"),
				Title:      acp.Ptr("Edit file"),
				Kind:       acp.Ptr(acp.ToolKindEdit),
				Content: []acp.ToolCallContent{{
					Diff: &acp.ToolCallContentDiff{Path: "/etc/cron.d/evil", NewText: "boom"},
				}},
			},
		},
		{
			// cwd/work_dir are the exec-escape keys: a command gated for the
			// workspace must not run with a working directory outside the root.
			name: "cwd key outside root",
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("exec-escape-cwd"),
				Title:      acp.Ptr("Run command"),
				Kind:       acp.Ptr(acp.ToolKindExecute),
				RawInput:   map[string]any{"command": "ls", "cwd": "/etc"},
			},
		},
		{
			name: "work_dir key outside root",
			toolCall: acp.ToolCallUpdate{
				ToolCallId: acp.ToolCallId("exec-escape-workdir"),
				Title:      acp.Ptr("Run command"),
				Kind:       acp.Ptr(acp.ToolKindExecute),
				RawInput:   map[string]any{"command": "ls", "work_dir": "/etc"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			approval := &fakeACPToolApproval{decision: toolapproval.Request{Status: toolapproval.StatusApproved}}
			callbacks := &clientCallbacks{
				root:        "/data",
				cwd:         "/data",
				virtualRoot: true,
				approval:    approval,
				baseSession: ToolSessionContext{
					BotID:             "bot-1",
					SessionID:         "session-1",
					StreamID:          "stream-1",
					ChannelIdentityID: "channel-1",
				},
				events: &toolEventEmitter{},
			}
			callbacks.setPromptState(newEventCollector(), nil, callbacks.baseSession)

			resp, err := callbacks.RequestPermission(context.Background(), acp.RequestPermissionRequest{
				ToolCall: tc.toolCall,
				Options: []acp.PermissionOption{
					{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow", OptionId: acp.PermissionOptionId("allow")},
				},
			})
			if err != nil {
				t.Fatalf("RequestPermission error = %v", err)
			}
			if resp.Outcome.Cancelled == nil {
				t.Fatalf("permission outcome = %#v, want cancelled for out-of-root path", resp.Outcome)
			}
			if got := approval.createdCount(); got != 0 {
				t.Fatalf("pending approvals created = %d, want 0 - the user must never be asked to approve an out-of-scope action", got)
			}
		})
	}
}

func TestRequestPermissionGrantDedupesWriteTextFileApproval(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	client := newTestBridgeClient(t, root)
	approval := &fakeACPToolApproval{
		decision: toolapproval.Request{
			ID:      "approval-write",
			ShortID: 14,
			Status:  toolapproval.StatusApproved,
		},
	}
	callbacks := newClientCallbacks(
		context.Background(),
		client,
		"/data",
		"/data",
		time.Second,
		nil,
		nil,
		true,
		approval,
		nil,
		ToolSessionContext{
			BotID:             "bot-1",
			SessionID:         "session-1",
			StreamID:          "stream-1",
			ChannelIdentityID: "channel-1",
		},
		acpprofile.DefaultToolQuirks(),
	)
	collector := newEventCollector()
	callbacks.setPromptState(collector, nil, callbacks.baseSession)

	permission, err := callbacks.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		ToolCall: acp.ToolCallUpdate{
			ToolCallId: acp.ToolCallId("write-1"),
			Title:      acp.Ptr("Write /data/review.txt"),
			Kind:       acp.Ptr(acp.ToolKindEdit),
			RawInput: map[string]any{
				"path":    "/data/review.txt",
				"content": "review me\n",
			},
		},
		Options: []acp.PermissionOption{
			{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow", OptionId: acp.PermissionOptionId("allow")},
			{Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject", OptionId: acp.PermissionOptionId("reject")},
		},
	})
	if err != nil {
		t.Fatalf("RequestPermission error = %v", err)
	}
	if permission.Outcome.Selected == nil {
		t.Fatalf("permission outcome = %#v, want selected", permission.Outcome)
	}
	if _, err := callbacks.WriteTextFile(context.Background(), acp.WriteTextFileRequest{
		Path:    "/data/review.txt",
		Content: "review me\n",
	}); err != nil {
		t.Fatalf("WriteTextFile error = %v", err)
	}
	if got := approval.createdCount(); got != 1 {
		t.Fatalf("approval create count = %d, want 1", got)
	}
	assertSingleApprovalWithStartEnd(t, collector.result().Events, "write-1", "write", "approval-write")
}

func TestRequestPermissionGrantDedupesCreateTerminalApproval(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	client := newTestBridgeClient(t, root)
	approval := &fakeACPToolApproval{
		decision: toolapproval.Request{
			ID:      "approval-exec",
			ShortID: 15,
			Status:  toolapproval.StatusApproved,
		},
	}
	callbacks := newClientCallbacks(
		context.Background(),
		client,
		"/data",
		"/data",
		time.Second,
		nil,
		nil,
		true,
		approval,
		nil,
		ToolSessionContext{
			BotID:             "bot-1",
			SessionID:         "session-1",
			StreamID:          "stream-1",
			ChannelIdentityID: "channel-1",
		},
		acpprofile.DefaultToolQuirks(),
	)
	collector := newEventCollector()
	callbacks.setPromptState(collector, nil, callbacks.baseSession)

	if _, err := callbacks.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		ToolCall: acp.ToolCallUpdate{
			ToolCallId: acp.ToolCallId("exec-1"),
			Title:      acp.Ptr("Shell"),
			Kind:       acp.Ptr(acp.ToolKindExecute),
			RawInput:   map[string]any{"command": "pwd"},
		},
		Options: []acp.PermissionOption{
			{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow", OptionId: acp.PermissionOptionId("allow")},
			{Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject", OptionId: acp.PermissionOptionId("reject")},
		},
	}); err != nil {
		t.Fatalf("RequestPermission error = %v", err)
	}
	term, err := callbacks.CreateTerminal(context.Background(), acp.CreateTerminalRequest{Command: "pwd"})
	if err != nil {
		t.Fatalf("CreateTerminal error = %v", err)
	}
	if _, err := callbacks.WaitForTerminalExit(context.Background(), acp.WaitForTerminalExitRequest{TerminalId: term.TerminalId}); err != nil {
		t.Fatalf("WaitForTerminalExit error = %v", err)
	}
	if got := approval.createdCount(); got != 1 {
		t.Fatalf("approval create count = %d, want 1", got)
	}
	assertSingleApprovalWithStartEnd(t, collector.result().Events, "exec-1", "exec", "approval-exec")
}

func TestRequestPermissionGrantDedupesTerminalWithCwdAndArgs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	client := newTestBridgeClient(t, root)
	approval := &fakeACPToolApproval{
		decision: toolapproval.Request{
			ID:      "approval-exec-cwd",
			ShortID: 16,
			Status:  toolapproval.StatusApproved,
		},
	}
	callbacks := newClientCallbacks(
		context.Background(),
		client,
		"/data",
		"/data",
		time.Second,
		nil,
		nil,
		true,
		approval,
		nil,
		ToolSessionContext{
			BotID:             "bot-1",
			SessionID:         "session-1",
			StreamID:          "stream-1",
			ChannelIdentityID: "channel-1",
		},
		acpprofile.DefaultToolQuirks(),
	)
	collector := newEventCollector()
	callbacks.setPromptState(collector, nil, callbacks.baseSession)

	// The permission request carries the raw command with loose spacing and no
	// cwd; the terminal create rebuilds it from Command+Args and adds a cwd.
	// The one-shot grant must still match.
	if _, err := callbacks.RequestPermission(context.Background(), acp.RequestPermissionRequest{
		ToolCall: acp.ToolCallUpdate{
			ToolCallId: acp.ToolCallId("exec-cwd-1"),
			Title:      acp.Ptr("Shell"),
			Kind:       acp.Ptr(acp.ToolKindExecute),
			RawInput:   map[string]any{"command": "printf  grant-ok"},
		},
		Options: []acp.PermissionOption{
			{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow", OptionId: acp.PermissionOptionId("allow")},
			{Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject", OptionId: acp.PermissionOptionId("reject")},
		},
	}); err != nil {
		t.Fatalf("RequestPermission error = %v", err)
	}
	term, err := callbacks.CreateTerminal(context.Background(), acp.CreateTerminalRequest{
		Command: "printf",
		Args:    []string{"grant-ok"},
		Cwd:     acp.Ptr("/data"),
	})
	if err != nil {
		t.Fatalf("CreateTerminal error = %v", err)
	}
	if _, err := callbacks.WaitForTerminalExit(context.Background(), acp.WaitForTerminalExitRequest{TerminalId: term.TerminalId}); err != nil {
		t.Fatalf("WaitForTerminalExit error = %v", err)
	}
	if got := approval.createdCount(); got != 1 {
		t.Fatalf("approval create count = %d, want 1 (grant should dedupe despite cwd/spacing)", got)
	}
	assertSingleApprovalWithStartEnd(t, collector.result().Events, "exec-cwd-1", "exec", "approval-exec-cwd")
}

func TestPermissionNativeToolMapsClaudeCodeShapes(t *testing.T) {
	t.Parallel()

	// Shapes mirror what @agentclientprotocol/claude-agent-acp sends via
	// toolInfoFromToolUse: Bash -> execute + {command}, Write/Edit -> edit +
	// {file_path, ...}.
	cases := []struct {
		name     string
		request  acp.RequestPermissionRequest
		wantTool string
		wantKey  string
		wantVal  string
	}{
		{
			name: "bash",
			request: acp.RequestPermissionRequest{
				ToolCall: acp.ToolCallUpdate{
					ToolCallId: acp.ToolCallId("toolu_bash"),
					Title:      acp.Ptr("npm test"),
					Kind:       acp.Ptr(acp.ToolKindExecute),
					RawInput:   map[string]any{"command": "npm test", "description": "Run tests"},
				},
			},
			wantTool: "exec",
			wantKey:  "command",
			wantVal:  "npm test",
		},
		{
			name: "write",
			request: acp.RequestPermissionRequest{
				ToolCall: acp.ToolCallUpdate{
					ToolCallId: acp.ToolCallId("toolu_write"),
					Title:      acp.Ptr("Write foo.txt"),
					Kind:       acp.Ptr(acp.ToolKindEdit),
					RawInput:   map[string]any{"file_path": "/data/foo.txt", "content": "hello"},
					Locations:  []acp.ToolCallLocation{{Path: "/data/foo.txt"}},
				},
			},
			wantTool: "write",
			wantKey:  "path",
			wantVal:  "/data/foo.txt",
		},
		{
			name: "edit",
			request: acp.RequestPermissionRequest{
				ToolCall: acp.ToolCallUpdate{
					ToolCallId: acp.ToolCallId("toolu_edit"),
					Title:      acp.Ptr("Edit foo.txt"),
					Kind:       acp.Ptr(acp.ToolKindEdit),
					RawInput: map[string]any{
						"file_path":  "/data/foo.txt",
						"old_string": "hello",
						"new_string": "world",
					},
					Locations: []acp.ToolCallLocation{{Path: "/data/foo.txt"}},
				},
			},
			wantTool: "edit",
			wantKey:  "path",
			wantVal:  "/data/foo.txt",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			toolCallID, toolName, input, ok := permissionNativeTool(tc.request, acpprofile.DefaultToolQuirks())
			if !ok {
				t.Fatalf("permissionNativeTool() failed to map %s request", tc.name)
			}
			if toolCallID != strings.TrimSpace(string(tc.request.ToolCall.ToolCallId)) {
				t.Fatalf("toolCallID = %q, want %q", toolCallID, tc.request.ToolCall.ToolCallId)
			}
			if toolName != tc.wantTool {
				t.Fatalf("toolName = %q, want %q", toolName, tc.wantTool)
			}
			if got := stringFromAny(input[tc.wantKey]); got != tc.wantVal {
				t.Fatalf("input[%s] = %q, want %q (input=%#v)", tc.wantKey, got, tc.wantVal, input)
			}
		})
	}
}

func TestPinSessionModeSkipsWhenNotNeeded(t *testing.T) {
	t.Parallel()

	modes := &acp.SessionModeState{
		CurrentModeId: acp.SessionModeId("default"),
		AvailableModes: []acp.SessionMode{
			{Id: acp.SessionModeId("default"), Name: "Always Ask"},
			{Id: acp.SessionModeId("acceptEdits"), Name: "Accept Edits"},
		},
	}
	// nil conn proves these paths never issue a set_mode call.
	if err := pinSessionMode(context.Background(), nil, acp.SessionId("s1"), modes, "", nil, "claude-code"); err != nil {
		t.Fatalf("empty desired mode: %v", err)
	}
	if err := pinSessionMode(context.Background(), nil, acp.SessionId("s1"), nil, "default", nil, "claude-code"); err != nil {
		t.Fatalf("nil modes: %v", err)
	}
	if err := pinSessionMode(context.Background(), nil, acp.SessionId("s1"), modes, "default", nil, "claude-code"); err != nil {
		t.Fatalf("already in desired mode: %v", err)
	}
	if err := pinSessionMode(context.Background(), nil, acp.SessionId("s1"), modes, "nonexistent", nil, "claude-code"); err != nil {
		t.Fatalf("unavailable mode should be skipped, got %v", err)
	}
}

func TestPinSessionConfigValuesSkipsWhenNotNeeded(t *testing.T) {
	t.Parallel()

	effort := acp.SessionConfigOption{
		Select: &acp.SessionConfigOptionSelect{
			Id:           acp.SessionConfigId("effort"),
			CurrentValue: acp.SessionConfigValueId("high"),
			Options: acp.SessionConfigSelectOptions{
				Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{
					{Value: acp.SessionConfigValueId("default"), Name: "Default"},
					{Value: acp.SessionConfigValueId("high"), Name: "High"},
				},
			},
		},
	}
	// nil conn proves these paths never issue a set_config_option call:
	// no desired entry, already at desired value, and unadvertised value.
	pinSessionConfigValues(context.Background(), nil, acp.SessionId("s1"), []acp.SessionConfigOption{effort}, nil, nil, "claude-code")
	pinSessionConfigValues(context.Background(), nil, acp.SessionId("s1"), []acp.SessionConfigOption{effort}, map[string]string{"effort": "high"}, nil, "claude-code")
	pinSessionConfigValues(context.Background(), nil, acp.SessionId("s1"), []acp.SessionConfigOption{effort}, map[string]string{"effort": "ultra"}, nil, "claude-code")
	pinSessionConfigValues(context.Background(), nil, acp.SessionId("s1"), nil, map[string]string{"effort": "high"}, nil, "claude-code")
}

func TestApprovalGrantsAreClearedBetweenPrompts(t *testing.T) {
	t.Parallel()

	callbacks := &clientCallbacks{}
	input := writeToolInput("/data/review.txt", "review me\n")
	callbacks.rememberApprovalGrant("write-1", "write", input)
	if got, ok := callbacks.consumeApprovalGrant("write", input); !ok || got != "write-1" {
		t.Fatalf("consume grant = %q, %v; want write-1, true", got, ok)
	}

	callbacks.rememberApprovalGrant("write-1", "write", input)
	callbacks.setPromptState(nil, nil, ToolSessionContext{})
	if got, ok := callbacks.consumeApprovalGrant("write", input); ok || got != "" {
		t.Fatalf("stale grant survived prompt reset: %q, %v", got, ok)
	}
}

func TestWriteApprovalGrantKeyIncludesFullContentHashWhenPreviewTruncated(t *testing.T) {
	t.Parallel()

	prefix := strings.Repeat("a", maxWriteToolContentPreview)
	first := writeToolInput("/data/large.txt", prefix+"b")
	second := writeToolInput("/data/large.txt", prefix+"c")
	if first["content"] != second["content"] || first["content_bytes"] != second["content_bytes"] {
		t.Fatalf("test setup expected same preview and byte count: %#v %#v", first, second)
	}
	if first["content_sha256"] == "" || first["content_sha256"] == second["content_sha256"] {
		t.Fatalf("content hashes should distinguish truncated writes: %#v %#v", first["content_sha256"], second["content_sha256"])
	}
	if approvalGrantKey("write", first) == approvalGrantKey("write", second) {
		t.Fatal("grant keys for distinct truncated writes should differ")
	}
}

func assertSingleApprovalWithStartEnd(t *testing.T, events []event.StreamEvent, toolCallID, toolName, approvalID string) {
	t.Helper()
	var pendingApprovals, approvedApprovals, starts, ends int
	for _, ev := range events {
		if ev.ToolCallID != toolCallID {
			continue
		}
		switch ev.Type {
		case event.ToolApprovalRequest:
			if ev.ApprovalID != approvalID {
				t.Fatalf("approval event = %#v, want approval id %q", ev, approvalID)
			}
			switch ev.Status {
			case toolapproval.StatusPending:
				pendingApprovals++
			case toolapproval.StatusApproved:
				approvedApprovals++
			default:
				t.Fatalf("approval event = %#v, want pending or approved", ev)
			}
		case event.ToolCallStart:
			starts++
			if ev.ToolName != toolName {
				t.Fatalf("start event = %#v, want tool %q", ev, toolName)
			}
		case event.ToolCallEnd:
			ends++
			if ev.ToolName != toolName || ev.Error != "" {
				t.Fatalf("end event = %#v, want successful %q", ev, toolName)
			}
		}
	}
	if pendingApprovals != 1 || approvedApprovals != 1 || starts != 1 || ends != 1 {
		t.Fatalf("events for %s pending/approved/start/end = %d/%d/%d/%d, events=%#v", toolCallID, pendingApprovals, approvedApprovals, starts, ends, events)
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

type fakeACPToolApproval struct {
	mu          sync.Mutex
	created     toolapproval.CreatePendingInput
	createCount int
	decision    toolapproval.Request
	evaluation  toolapproval.Evaluation
	waiters     int
}

func (f *fakeACPToolApproval) EvaluatePolicy(context.Context, toolapproval.CreatePendingInput) (toolapproval.Evaluation, error) {
	if strings.TrimSpace(f.evaluation.Decision) != "" {
		return f.evaluation, nil
	}
	return toolapproval.Evaluation{Decision: toolapproval.DecisionNeedsApproval}, nil
}

func (f *fakeACPToolApproval) CreatePending(_ context.Context, input toolapproval.CreatePendingInput) (toolapproval.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = input
	f.createCount++
	req := toolapproval.Request{
		ID:                "approval-1",
		BotID:             input.BotID,
		SessionID:         input.SessionID,
		ChannelIdentityID: input.ChannelIdentityID,
		ToolCallID:        input.ToolCallID,
		ToolName:          input.ToolName,
		ToolInput:         copyInputMap(input.ToolInput),
		ShortID:           1,
		Status:            toolapproval.StatusPending,
		SourcePlatform:    input.SourcePlatform,
		ConversationType:  input.ConversationType,
	}
	if strings.TrimSpace(f.decision.ID) != "" {
		req.ID = f.decision.ID
	}
	if f.decision.ShortID != 0 {
		req.ShortID = f.decision.ShortID
	}
	return req, nil
}

func (*fakeACPToolApproval) Reject(context.Context, string, string, string) (toolapproval.Request, error) {
	return toolapproval.Request{Status: toolapproval.StatusRejected}, nil
}

func (f *fakeACPToolApproval) Get(_ context.Context, approvalID string) (toolapproval.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	decision := f.decision
	if strings.TrimSpace(decision.ID) == "" {
		decision.ID = approvalID
	}
	return decision, nil
}

func (f *fakeACPToolApproval) WaitForDecision(_ context.Context, approvalID string) (toolapproval.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	decision := f.decision
	if strings.TrimSpace(decision.ID) == "" {
		decision.ID = approvalID
	}
	if strings.TrimSpace(decision.Status) == "" {
		decision.Status = toolapproval.StatusApproved
	}
	return decision, nil
}

func (f *fakeACPToolApproval) RegisterWaiter(string) func() {
	f.mu.Lock()
	f.waiters++
	f.mu.Unlock()
	return func() {
		f.mu.Lock()
		f.waiters--
		f.mu.Unlock()
	}
}

func (f *fakeACPToolApproval) createdCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.createCount
}

func copyInputMap(input any) map[string]any {
	out := map[string]any{}
	if m, ok := input.(map[string]any); ok {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}
