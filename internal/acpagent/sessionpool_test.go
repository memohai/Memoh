package acpagent

import (
	"context"
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

	"github.com/memohai/memoh/internal/acpclient"
	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/mcp"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/workspace/bridge"
	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
	"github.com/memohai/memoh/internal/workspace/bridgesvc"
)

func TestSessionPoolKeyedBySessionIDReuseAndClose(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}

	client := newSessionPoolBridgeClient(t, root)
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeSessionPoolFakeAgentScript(t, binDir, "npx")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	runner := acpclient.NewRunner(nil, sessionPoolWorkspace{
		client: client,
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})
	pool := newSessionPool(nil, runner, fakeBotGetter{bot: enabledACPBot("bot-1", "api_key", nil)})
	pool.timeout = time.Hour
	contexts := mcp.NewToolSessionContextStore()
	pool.SetToolSessionContextStore(contexts)

	input := PromptInput{
		BotID:           "bot-1",
		SessionID:       "session-1",
		StreamID:        "stream-1",
		AgentID:         acpprofile.AgentCodexID,
		ProjectPath:     "/data/project",
		Prompt:          "first prompt",
		CurrentPlatform: "web",
	}
	result, err := pool.Prompt(context.Background(), input)
	if err != nil {
		t.Fatalf("Prompt(first) error = %v", err)
	}
	if !strings.Contains(result.Text, "session-pool-ok") {
		t.Fatalf("first result text = %q", result.Text)
	}
	firstSession := pool.sessions["session-1"].session
	if firstSession == nil {
		t.Fatalf("session was not stored")
	}
	merged := contexts.Merge(mcp.ToolSessionContext{BotID: "bot-1", SessionID: "session-1"})
	if merged.StreamID != "stream-1" || merged.CurrentPlatform != "web" {
		t.Fatalf("stored tool session = %#v", merged)
	}

	input.Prompt = "second prompt"
	if _, err := pool.Prompt(context.Background(), input); err != nil {
		t.Fatalf("Prompt(second) error = %v", err)
	}
	if got := pool.sessions["session-1"].session; got != firstSession {
		t.Fatalf("same session_id started a new ACP process")
	}

	input.SessionID = "session-2"
	input.Prompt = "third prompt"
	if _, err := pool.Prompt(context.Background(), input); err != nil {
		t.Fatalf("Prompt(third) error = %v", err)
	}
	if got := pool.sessions["session-2"].session; got == nil || got == firstSession {
		t.Fatalf("different session_id did not get an independent ACP session")
	}

	status := pool.RuntimeStatus("session-1", "", "")
	if status.State != "idle" || status.ACPSession == "" || status.ProjectPath != "/data/project" {
		t.Fatalf("RuntimeStatus() = %#v", status)
	}
	if err := pool.CloseSession("session-1"); err != nil {
		t.Fatalf("CloseSession() error = %v", err)
	}
	if _, ok := pool.sessions["session-1"]; ok {
		t.Fatalf("CloseSession did not remove the pooled session")
	}
	merged = contexts.Merge(mcp.ToolSessionContext{BotID: "bot-1", SessionID: "session-1"})
	if merged.StreamID != "" || merged.CurrentPlatform != "" {
		t.Fatalf("CloseSession did not clear stored tool session: %#v", merged)
	}
}

func TestSessionPoolEnsureStartsRuntimeAndReportsModels(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEMOH_ACP_SESSION_POOL_FAKE_AGENT_MODELS", "1")

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeSessionPoolFakeAgentScript(t, binDir, "npx")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	runner := acpclient.NewRunner(nil, sessionPoolWorkspace{
		client: newSessionPoolBridgeClient(t, root),
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})
	pool := newSessionPool(nil, runner, fakeBotGetter{bot: enabledACPBot("bot-1", "api_key", nil)})

	status, err := pool.Ensure(context.Background(), PromptInput{
		BotID:       "bot-1",
		SessionID:   "session-1",
		AgentID:     acpprofile.AgentCodexID,
		ProjectPath: "/data/project",
	})
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if status.State != "idle" || status.ACPSession == "" {
		t.Fatalf("Ensure() status = %#v, want idle runtime with ACP session id", status)
	}
	if status.Models == nil || !status.Models.Supported || status.Models.CurrentModelID != "gpt-5.1-codex" {
		t.Fatalf("Ensure() models = %#v, want protocol model state", status.Models)
	}
	if len(status.Models.Available) != 2 || status.Models.Available[0].ID != "gpt-5.1-codex" || status.Models.Available[1].ID != "gpt-5.1-codex-high" {
		t.Fatalf("Ensure() available models = %#v", status.Models.Available)
	}
}

func TestSessionPoolSetModelUpdatesRuntimeModel(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEMOH_ACP_SESSION_POOL_FAKE_AGENT_MODELS", "1")

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeSessionPoolFakeAgentScript(t, binDir, "npx")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	runner := acpclient.NewRunner(nil, sessionPoolWorkspace{
		client: newSessionPoolBridgeClient(t, root),
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})
	pool := newSessionPool(nil, runner, fakeBotGetter{bot: enabledACPBot("bot-1", "api_key", nil)})

	status, err := pool.SetModel(context.Background(), PromptInput{
		BotID:       "bot-1",
		SessionID:   "session-1",
		AgentID:     acpprofile.AgentCodexID,
		ProjectPath: "/data/project",
	}, "gpt-5.1-codex-high")
	if err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	if status.State != "idle" || status.ACPSession == "" {
		t.Fatalf("SetModel() status = %#v, want idle runtime with ACP session id", status)
	}
	if status.Models == nil || !status.Models.Supported || status.Models.CurrentModelID != "gpt-5.1-codex-high" {
		t.Fatalf("SetModel() models = %#v, want selected model", status.Models)
	}
}

func TestSessionPoolRuntimeStatusReportsActiveDuringColdStart(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	runner := &blockingRunner{
		info:    bridge.WorkspaceInfo{Backend: bridge.WorkspaceBackendContainer, DefaultWorkDir: "/data"},
		started: started,
		release: release,
	}
	pool := newSessionPool(
		nil,
		runner,
		fakeBotGetter{bot: enabledACPBot("bot-1", "api_key", map[string]any{"api_key": "sk-test"})},
	)

	errCh := make(chan error, 1)
	go func() {
		_, err := pool.Prompt(context.Background(), PromptInput{
			BotID:       "bot-1",
			SessionID:   "session-1",
			AgentID:     "codex",
			ProjectPath: "/data/project",
			Prompt:      "run",
		})
		errCh <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}

	status := pool.RuntimeStatus("session-1", "", "")
	if status.State != "active" || status.ACPSession != "" {
		t.Fatalf("RuntimeStatus during cold start = %#v, want active without ACP session id", status)
	}

	close(release)
	if err := <-errCh; err == nil || err.Error() != "released" {
		t.Fatalf("Prompt() error = %v, want released", err)
	}
	status = pool.RuntimeStatus("session-1", "codex", "/data/project")
	if status.State != "idle" || status.ACPSession != "" {
		t.Fatalf("RuntimeStatus after failed start = %#v, want idle without process", status)
	}
}

func TestSessionPoolCloseDuringColdStartPreventsReinsert(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	runner := &delayedStartRunner{
		info:    bridge.WorkspaceInfo{Backend: bridge.WorkspaceBackendContainer, DefaultWorkDir: "/data"},
		started: started,
		release: release,
		session: &acpclient.Session{},
	}
	pool := newSessionPool(
		nil,
		runner,
		fakeBotGetter{bot: enabledACPBot("bot-1", "api_key", map[string]any{"api_key": "sk-test"})},
	)

	type startResult struct {
		session *acpclient.Session
		err     error
	}
	resultCh := make(chan startResult, 1)
	go func() {
		sess, err := pool.getOrStart(context.Background(), PromptInput{
			BotID:       "bot-1",
			SessionID:   "session-1",
			AgentID:     "codex",
			ProjectPath: "/data/project",
		})
		resultCh <- startResult{session: sess, err: err}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}

	if err := pool.CloseSession("session-1"); err != nil {
		t.Fatalf("CloseSession() error = %v", err)
	}
	close(release)

	var result startResult
	select {
	case result = <-resultCh:
	case <-time.After(2 * time.Second):
		t.Fatal("getOrStart did not return")
	}
	if result.session != nil {
		t.Fatalf("getOrStart returned a session after CloseSession during startup")
	}
	if result.err == nil || !strings.Contains(result.err.Error(), "closed during startup") {
		t.Fatalf("getOrStart error = %v, want closed during startup", result.err)
	}
	if _, ok := pool.sessions["session-1"]; ok {
		t.Fatalf("closed cold-start session was reinserted into the pool")
	}
}

func TestSessionPoolCloseDuringColdStartCancelsStartup(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	runner := &cancelAwareStartRunner{
		info:      bridge.WorkspaceInfo{Backend: bridge.WorkspaceBackendContainer, DefaultWorkDir: "/data"},
		started:   started,
		cancelled: cancelled,
	}
	pool := newSessionPool(
		nil,
		runner,
		fakeBotGetter{bot: enabledACPBot("bot-1", "api_key", map[string]any{"api_key": "sk-test"})},
	)

	type startResult struct {
		session *acpclient.Session
		err     error
	}
	resultCh := make(chan startResult, 1)
	go func() {
		sess, err := pool.getOrStart(context.Background(), PromptInput{
			BotID:       "bot-1",
			SessionID:   "session-1",
			AgentID:     "codex",
			ProjectPath: "/data/project",
		})
		resultCh <- startResult{session: sess, err: err}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}

	if err := pool.CloseSession("session-1"); err != nil {
		t.Fatalf("CloseSession() error = %v", err)
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("startup context was not cancelled")
	}

	var result startResult
	select {
	case result = <-resultCh:
	case <-time.After(2 * time.Second):
		t.Fatal("getOrStart did not return after startup cancellation")
	}
	if result.session != nil {
		t.Fatalf("getOrStart returned a session after startup cancellation")
	}
	if !errors.Is(result.err, context.Canceled) {
		t.Fatalf("getOrStart error = %v, want context.Canceled", result.err)
	}
	if _, ok := pool.sessions["session-1"]; ok {
		t.Fatalf("cancelled cold-start session remained in the pool")
	}
}

func TestSessionPoolCloseSessionWaitsForInFlightSessionLock(t *testing.T) {
	pool := newSessionPool(nil, nil, nil)
	unlock := pool.lockSession("session-1")

	closed := make(chan error, 1)
	go func() {
		closed <- pool.CloseSession("session-1")
	}()

	select {
	case err := <-closed:
		t.Fatalf("CloseSession returned before in-flight session lock released: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	unlock()

	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("CloseSession() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CloseSession did not unblock after releasing the session lock")
	}
}

func TestSessionPoolSerializesColdStartForSameSessionID(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeSessionPoolFakeAgentScript(t, binDir, "npx")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	startLog := filepath.Join(root, "starts.log")
	t.Setenv("MEMOH_ACP_START_LOG", startLog)

	runner := acpclient.NewRunner(nil, sessionPoolWorkspace{
		client: newSessionPoolBridgeClient(t, root),
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})
	pool := newSessionPool(nil, runner, fakeBotGetter{bot: enabledACPBot("bot-1", "api_key", nil)})

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := pool.Prompt(context.Background(), PromptInput{
				BotID:       "bot-1",
				SessionID:   "session-1",
				AgentID:     "codex",
				ProjectPath: "/data/project",
				Prompt:      "same session",
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Prompt() error = %v", err)
		}
	}

	raw, err := os.ReadFile(startLog) //nolint:gosec // test path under t.TempDir.
	if err != nil {
		t.Fatalf("read start log: %v", err)
	}
	if starts := strings.Count(string(raw), "start\n"); starts != 1 {
		t.Fatalf("fake ACP process starts = %d, want 1; log=%q", starts, string(raw))
	}
}

func TestSessionPoolSetupModeResolution(t *testing.T) {
	missingAPIKey := newSessionPool(nil, &recordingRunner{
		info: bridge.WorkspaceInfo{Backend: bridge.WorkspaceBackendContainer, DefaultWorkDir: "/data"},
	}, fakeBotGetter{bot: enabledACPBot("bot-1", "api_key", nil)})
	_, err := missingAPIKey.Prompt(context.Background(), PromptInput{
		BotID:       "bot-1",
		SessionID:   "session-1",
		AgentID:     "codex",
		ProjectPath: "/data/project",
		Prompt:      "run",
	})
	if err == nil || !strings.Contains(err.Error(), "api_key required") {
		t.Fatalf("container api_key missing key error = %v", err)
	}

	apiKeyRunner := &recordingRunner{
		info:     bridge.WorkspaceInfo{Backend: bridge.WorkspaceBackendContainer, DefaultWorkDir: "/data", ACPToolsHTTPURL: "http://127.0.0.1:18732/mcp"},
		startErr: errors.New("started"),
	}
	apiKeyPool := newSessionPool(nil, apiKeyRunner, fakeBotGetter{bot: enabledACPBot("bot-1", "api_key", map[string]any{"api_key": "sk-test", "base_url": "https://proxy.example.com/v1"})})
	_, err = apiKeyPool.Prompt(context.Background(), PromptInput{
		BotID:       "bot-1",
		SessionID:   "session-1",
		AgentID:     "codex",
		ProjectPath: "/data/project",
		Prompt:      "run",
	})
	if err == nil || err.Error() != "started" {
		t.Fatalf("container api_key error = %v, want runner start error", err)
	}
	if apiKeyRunner.req.SetupMode != acpclient.SetupModeAPIKey {
		t.Fatalf("api_key setup mode = %q", apiKeyRunner.req.SetupMode)
	}
	if len(apiKeyRunner.req.Env) != 0 {
		t.Fatalf("api_key mode must use Codex files, not credential env: %v", apiKeyRunner.req.Env)
	}

	oauthRunner := &recordingRunner{
		info:     bridge.WorkspaceInfo{Backend: bridge.WorkspaceBackendContainer, DefaultWorkDir: "/data"},
		startErr: errors.New("started"),
	}
	oauthPool := newSessionPool(nil, oauthRunner, fakeBotGetter{bot: enabledACPBot("bot-1", "oauth", map[string]any{"provider_id": "provider-1"})})
	_, err = oauthPool.Prompt(context.Background(), PromptInput{
		BotID:       "bot-1",
		SessionID:   "session-1",
		AgentID:     "codex",
		ProjectPath: "/data/project",
		Prompt:      "run",
	})
	if err == nil || err.Error() != "started" {
		t.Fatalf("container oauth error = %v, want runner start error", err)
	}
	if oauthRunner.req.SetupMode != acpclient.SetupModeOAuth {
		t.Fatalf("oauth setup mode = %q", oauthRunner.req.SetupMode)
	}

	selfRunner := &recordingRunner{
		info:     bridge.WorkspaceInfo{Backend: bridge.WorkspaceBackendContainer, DefaultWorkDir: "/data"},
		startErr: errors.New("started"),
	}
	selfPool := newSessionPool(nil, selfRunner, fakeBotGetter{bot: enabledACPBot("bot-1", "self", nil)})
	_, err = selfPool.Prompt(context.Background(), PromptInput{
		BotID:       "bot-1",
		SessionID:   "session-1",
		AgentID:     "codex",
		ProjectPath: "/data/project",
		Prompt:      "run",
	})
	if err == nil || err.Error() != "started" {
		t.Fatalf("container self error = %v, want runner start error", err)
	}
	if selfRunner.req.SetupMode != acpclient.SetupModeSelf {
		t.Fatalf("self setup mode = %q", selfRunner.req.SetupMode)
	}
	if len(selfRunner.req.Env) != 0 {
		t.Fatalf("self mode injected credential env: %v", selfRunner.req.Env)
	}
	if got := selfPool.RuntimeStatus("session-1", "codex", "/data/project"); got.State != "idle" || got.ACPSession != "" {
		t.Fatalf("RuntimeStatus after failed start = %#v, want idle without process", got)
	}

	localRunner := &recordingRunner{
		info:     bridge.WorkspaceInfo{Backend: bridge.WorkspaceBackendLocal, DefaultWorkDir: t.TempDir()},
		startErr: errors.New("started"),
	}
	localPool := newSessionPool(nil, localRunner, fakeBotGetter{bot: enabledACPBot("bot-1", "api_key", nil)})
	_, err = localPool.Prompt(context.Background(), PromptInput{
		BotID:       "bot-1",
		SessionID:   "session-1",
		AgentID:     "codex",
		ProjectPath: "",
		Prompt:      "run",
	})
	if err == nil || err.Error() != "started" {
		t.Fatalf("local missing key error = %v, want runner start error", err)
	}
	if len(localRunner.req.Env) != 0 {
		t.Fatalf("local backend injected env: %v", localRunner.req.Env)
	}
	if localRunner.req.LocalCommand != "npx" || len(localRunner.req.LocalArgs) == 0 {
		t.Fatalf("local command not passed through: %#v", localRunner.req)
	}

	claudeRunner := &recordingRunner{
		info:     bridge.WorkspaceInfo{Backend: bridge.WorkspaceBackendContainer, DefaultWorkDir: "/data"},
		startErr: errors.New("started"),
	}
	claudePool := newSessionPool(nil, claudeRunner, fakeBotGetter{bot: enabledACPAgentBot("bot-1", acpprofile.AgentClaudeCodeID, "api_key", map[string]any{
		"api_key":  "sk-ant-test",
		"base_url": "https://anthropic-proxy.example.com",
	})})
	_, err = claudePool.Prompt(context.Background(), PromptInput{
		BotID:       "bot-1",
		SessionID:   "session-1",
		AgentID:     acpprofile.AgentClaudeCodeID,
		ProjectPath: "/data/project",
		Prompt:      "run",
	})
	if err == nil || err.Error() != "started" {
		t.Fatalf("container Claude Code api_key error = %v, want runner start error", err)
	}
	if claudeRunner.req.Command != "claude-agent-acp" {
		t.Fatalf("Claude Code command = %q", claudeRunner.req.Command)
	}
	if !startRequestEnvHas(claudeRunner.req.Env, "ANTHROPIC_API_KEY", "sk-ant-test") ||
		!startRequestEnvHas(claudeRunner.req.Env, "ANTHROPIC_BASE_URL", "https://anthropic-proxy.example.com") {
		t.Fatalf("Claude Code env = %#v, want Anthropic managed env", claudeRunner.req.Env)
	}
	if !startRequestEnvHas(claudeRunner.req.Env, "ANTHROPIC_AUTH_TOKEN", "") ||
		!startRequestEnvHas(claudeRunner.req.Env, "CLAUDE_CODE_OAUTH_TOKEN", "") {
		t.Fatalf("Claude Code api_key env = %#v, want conflicting auth env cleared", claudeRunner.req.Env)
	}

	claudeOAuthRunner := &recordingRunner{
		info:     bridge.WorkspaceInfo{Backend: bridge.WorkspaceBackendContainer, DefaultWorkDir: "/data"},
		startErr: errors.New("started"),
	}
	claudeOAuthManaged := map[string]any{ //nolint:gosec // Test fixture token, not a real credential.
		"oauth_token": "fake-claude-oauth-token",
		"base_url":    "https://anthropic-proxy.example.com",
	}
	claudeOAuthPool := newSessionPool(nil, claudeOAuthRunner, fakeBotGetter{bot: enabledACPAgentBot("bot-1", acpprofile.AgentClaudeCodeID, "oauth", claudeOAuthManaged)})
	_, err = claudeOAuthPool.Prompt(context.Background(), PromptInput{
		BotID:       "bot-1",
		SessionID:   "session-1",
		AgentID:     acpprofile.AgentClaudeCodeID,
		ProjectPath: "/data/project",
		Prompt:      "run",
	})
	if err == nil || err.Error() != "started" {
		t.Fatalf("container Claude Code oauth error = %v, want runner start error", err)
	}
	if !startRequestEnvHas(claudeOAuthRunner.req.Env, "CLAUDE_CODE_OAUTH_TOKEN", "fake-claude-oauth-token") ||
		!startRequestEnvHas(claudeOAuthRunner.req.Env, "ANTHROPIC_BASE_URL", "https://anthropic-proxy.example.com") {
		t.Fatalf("Claude Code oauth env = %#v, want Claude managed oauth env", claudeOAuthRunner.req.Env)
	}
	if !startRequestEnvHas(claudeOAuthRunner.req.Env, "ANTHROPIC_API_KEY", "") ||
		!startRequestEnvHas(claudeOAuthRunner.req.Env, "ANTHROPIC_AUTH_TOKEN", "") {
		t.Fatalf("Claude Code oauth env = %#v, want conflicting auth env cleared", claudeOAuthRunner.req.Env)
	}
}

func TestSessionPoolUsesSessionMetadataAsRuntimeTruth(t *testing.T) {
	runner := &recordingRunner{
		info:     bridge.WorkspaceInfo{Backend: bridge.WorkspaceBackendContainer, DefaultWorkDir: "/data", ACPToolsHTTPURL: "http://127.0.0.1:18732/mcp"},
		startErr: errors.New("started"),
	}
	pool := newSessionPool(
		nil,
		runner,
		fakeBotGetter{bot: enabledACPBot("bot-1", "api_key", map[string]any{"api_key": "sk-test"})},
		fakeSessionGetter{session: sessionpkg.Session{
			ID:    "session-1",
			BotID: "bot-1",
			Type:  sessionpkg.TypeACPAgent,
			Metadata: map[string]any{
				"acp_agent_id": "codex",
				"project_path": "/data/from-session",
			},
		}},
	)

	_, err := pool.Prompt(context.Background(), PromptInput{
		BotID:       "bot-1",
		SessionID:   "session-1",
		AgentID:     "wrong-agent",
		ProjectPath: "/data/from-caller",
		Prompt:      "run",
	})
	if err == nil || err.Error() != "started" {
		t.Fatalf("Prompt() error = %v, want runner start error", err)
	}
	if runner.req.AgentID != "codex" {
		t.Fatalf("runner agent_id = %q, want session metadata codex", runner.req.AgentID)
	}
	if runner.req.ProjectPath != "/data/from-session" {
		t.Fatalf("runner project_path = %q, want session metadata project path", runner.req.ProjectPath)
	}
}

func TestSessionPoolPassesToolHTTPURLAndSessionContext(t *testing.T) {
	runner := &recordingRunner{
		info:     bridge.WorkspaceInfo{Backend: bridge.WorkspaceBackendContainer, DefaultWorkDir: "/data", ACPToolsHTTPURL: "http://127.0.0.1:18732/mcp"},
		startErr: errors.New("started"),
	}
	pool := newSessionPool(
		nil,
		runner,
		fakeBotGetter{bot: enabledACPBot("bot-1", "api_key", map[string]any{"api_key": "sk-test"})},
	)
	pool.SetToolGateway(mcp.NewToolGatewayService(nil, nil))
	contexts := mcp.NewToolSessionContextStore()
	pool.SetToolSessionContextStore(contexts)

	_, err := pool.Prompt(context.Background(), PromptInput{
		BotID:             "bot-1",
		ChatID:            "chat-1",
		SessionID:         "session-1",
		StreamID:          "stream-1",
		RouteID:           "route-1",
		AgentID:           "codex",
		ProjectPath:       "/data/project",
		Prompt:            "run",
		ChannelIdentityID: "user-1",
		SessionToken:      "token-1",
		CurrentPlatform:   "web",
		ReplyTarget:       "reply-1",
		ConversationType:  "private",
	})
	if err == nil || err.Error() != "started" {
		t.Fatalf("Prompt() error = %v, want runner start error", err)
	}
	if runner.req.ToolHTTPURL != "http://127.0.0.1:18732/mcp" {
		t.Fatalf("ToolHTTPURL = %q", runner.req.ToolHTTPURL)
	}
	if runner.req.ToolSession.BotID != "bot-1" || runner.req.ToolSession.ChatID != "chat-1" || runner.req.ToolSession.SessionID != "session-1" || runner.req.ToolSession.RouteID != "route-1" {
		t.Fatalf("ToolSession ids = %#v", runner.req.ToolSession)
	}
	if runner.req.ToolSession.StreamID != "stream-1" || runner.req.ToolSession.ChannelIdentityID != "user-1" || runner.req.ToolSession.SessionToken != "token-1" || runner.req.ToolSession.CurrentPlatform != "web" || runner.req.ToolSession.ReplyTarget != "reply-1" || runner.req.ToolSession.ConversationType != "private" {
		t.Fatalf("ToolSession metadata = %#v", runner.req.ToolSession)
	}
	merged := contexts.Merge(mcp.ToolSessionContext{BotID: "bot-1", SessionID: "session-1"})
	if merged.StreamID != "" || merged.ConversationType != "" {
		t.Fatalf("failed start did not clear stored tool session: %#v", merged)
	}
}

func TestSessionPoolUsesRequestToolURLForLocalWorkspace(t *testing.T) {
	pool := newSessionPool(nil, nil, nil)
	pool.SetToolGateway(mcp.NewToolGatewayService(nil, nil))

	got, err := pool.resolveToolHTTPURL(context.Background(), PromptInput{
		BotID:       "bot-1",
		ToolHTTPURL: "http://127.0.0.1:18731/bots/bot-1/tools",
	}, bridge.WorkspaceInfo{Backend: bridge.WorkspaceBackendLocal})
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:18731/bots/bot-1/tools" {
		t.Fatalf("local ToolHTTPURL = %q", got)
	}
}

func TestSessionPoolUsesWorkspaceACPToolsEndpointForContainer(t *testing.T) {
	pool := newSessionPool(nil, nil, nil)
	pool.SetToolGateway(mcp.NewToolGatewayService(nil, nil))

	got, err := pool.resolveToolHTTPURL(context.Background(), PromptInput{
		BotID: "bot-1",
	}, bridge.WorkspaceInfo{
		Backend:         bridge.WorkspaceBackendContainer,
		ACPToolsHTTPURL: "http://127.0.0.1:18732/mcp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:18732/mcp" {
		t.Fatalf("container ToolHTTPURL = %q", got)
	}
}

func TestTrustedToolSessionContextForcesBaseIdentity(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://memoh.test/mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(mcp.ToolHeaderBotID, "spoofed-bot")
	req.Header.Set(mcp.ToolHeaderSessionID, "spoofed-session")
	req.Header.Set(mcp.ToolHeaderStreamID, "spoofed-stream")
	req.Header.Set(mcp.ToolHeaderIsSubagent, "true")

	got := trustedToolSessionContext(req, acpclient.ToolSessionContext{
		BotID:       "bot-1",
		SessionID:   "session-1",
		StreamID:    "stream-1",
		SessionType: "acp_agent",
	})
	if got.BotID != "bot-1" || got.SessionID != "session-1" || got.StreamID != "stream-1" || got.SessionType != "acp_agent" {
		t.Fatalf("trusted context = %#v", got)
	}
	if got.IsSubagent {
		t.Fatalf("trusted context should force is_subagent=false: %#v", got)
	}
}

func TestPromptToolEventSinkPreservesACPAndHTTPToolEventOrder(t *testing.T) {
	sink := newPromptToolEventSink(nil)
	sink.EmitACPEvent(acpclient.StreamEvent{Type: acpclient.StreamEventTextDelta, Delta: "before"})
	sink.EmitToolStreamEvent(mcp.ToolStreamEvent{
		Type:       "tool_call_start",
		ToolCallID: "call-1",
		ToolName:   "schedule_list",
	})
	sink.EmitToolStreamEvent(mcp.ToolStreamEvent{
		Type:       "tool_call_end",
		ToolCallID: "call-1",
		ToolName:   "schedule_list",
		Result:     map[string]any{"ok": true},
	})
	sink.EmitACPEvent(acpclient.StreamEvent{Type: acpclient.StreamEventTextDelta, Delta: "after"})

	events := sink.Events()
	if len(events) != 4 {
		t.Fatalf("events = %#v", events)
	}
	if events[0].Type != acpclient.StreamEventTextDelta || events[1].Type != acpclient.StreamEventToolCallStart || events[2].Type != acpclient.StreamEventToolCallEnd || events[3].Type != acpclient.StreamEventTextDelta {
		t.Fatalf("events order = %#v", events)
	}
}

func TestSessionPoolReapIdle(t *testing.T) {
	pool := newSessionPool(nil, nil, nil)
	pool.timeout = time.Minute
	now := time.Now()
	pool.sessions["idle"] = &pooledSession{status: "idle", lastActive: now.Add(-2 * time.Minute)}
	pool.sessions["active"] = &pooledSession{status: "active", lastActive: now.Add(-2 * time.Minute)}
	pool.sessions["fresh"] = &pooledSession{status: "idle", lastActive: now.Add(-30 * time.Second)}

	if got := pool.reapIdle(now); got != 1 {
		t.Fatalf("reapIdle() = %d, want 1", got)
	}
	if _, ok := pool.sessions["idle"]; ok {
		t.Fatalf("stale idle session was not removed")
	}
	if _, ok := pool.sessions["active"]; !ok {
		t.Fatalf("active session should not be reaped")
	}
	if _, ok := pool.sessions["fresh"]; !ok {
		t.Fatalf("fresh idle session should not be reaped")
	}
}

type fakeBotGetter struct {
	bot bots.Bot
	err error
}

func (g fakeBotGetter) Get(context.Context, string) (bots.Bot, error) {
	return g.bot, g.err
}

type fakeSessionGetter struct {
	session sessionpkg.Session
	err     error
}

func (g fakeSessionGetter) Get(context.Context, string) (sessionpkg.Session, error) {
	return g.session, g.err
}

type recordingRunner struct {
	info     bridge.WorkspaceInfo
	req      acpclient.StartRequest
	startErr error
}

type blockingRunner struct {
	info    bridge.WorkspaceInfo
	started chan struct{}
	release chan struct{}
}

type delayedStartRunner struct {
	info    bridge.WorkspaceInfo
	started chan struct{}
	release chan struct{}
	session *acpclient.Session
}

type cancelAwareStartRunner struct {
	info      bridge.WorkspaceInfo
	started   chan struct{}
	cancelled chan struct{}
}

func (r *blockingRunner) WorkspaceInfo(context.Context, string) (bridge.WorkspaceInfo, error) {
	return r.info, nil
}

func (r *blockingRunner) StartSession(context.Context, acpclient.StartRequest, acpclient.EventSink) (*acpclient.Session, error) {
	close(r.started)
	<-r.release
	return nil, errors.New("released")
}

func (r *delayedStartRunner) WorkspaceInfo(context.Context, string) (bridge.WorkspaceInfo, error) {
	return r.info, nil
}

func (r *delayedStartRunner) StartSession(context.Context, acpclient.StartRequest, acpclient.EventSink) (*acpclient.Session, error) {
	close(r.started)
	<-r.release
	return r.session, nil
}

func (r *cancelAwareStartRunner) WorkspaceInfo(context.Context, string) (bridge.WorkspaceInfo, error) {
	return r.info, nil
}

func (r *cancelAwareStartRunner) StartSession(ctx context.Context, _ acpclient.StartRequest, _ acpclient.EventSink) (*acpclient.Session, error) {
	close(r.started)
	<-ctx.Done()
	close(r.cancelled)
	return nil, ctx.Err()
}

func (r *recordingRunner) WorkspaceInfo(context.Context, string) (bridge.WorkspaceInfo, error) {
	return r.info, nil
}

func (r *recordingRunner) StartSession(_ context.Context, req acpclient.StartRequest, _ acpclient.EventSink) (*acpclient.Session, error) {
	r.req = req
	return nil, r.startErr
}

type sessionPoolWorkspace struct {
	client *bridge.Client
	info   bridge.WorkspaceInfo
}

func (w sessionPoolWorkspace) MCPClient(context.Context, string) (*bridge.Client, error) {
	return w.client, nil
}

func (w sessionPoolWorkspace) WorkspaceInfo(context.Context, string) (bridge.WorkspaceInfo, error) {
	return w.info, nil
}

func enabledACPBot(id, mode string, managed map[string]any) bots.Bot {
	return enabledACPAgentBot(id, acpprofile.AgentCodexID, mode, managed)
}

func enabledACPAgentBot(id, agentID, mode string, managed map[string]any) bots.Bot {
	if managed == nil {
		managed = map[string]any{}
	}
	return bots.Bot{
		ID: id,
		Metadata: map[string]any{
			"acp": map[string]any{
				"agents": map[string]any{
					agentID: map[string]any{
						"enabled":    true,
						"setup_mode": mode,
						"managed":    managed,
					},
				},
			},
		},
	}
}

func startRequestEnvHas(env []string, key, want string) bool {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix) == want
		}
	}
	return false
}

func newSessionPoolBridgeClient(t *testing.T, root string) *bridge.Client {
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

	conn, err := grpc.NewClient("passthrough:///acpagent-sessionpool-test",
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

func writeSessionPoolFakeAgentScript(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := fmt.Sprintf("#!/bin/sh\nif [ -n \"${MEMOH_ACP_START_LOG:-}\" ]; then printf 'start\\n' >> \"$MEMOH_ACP_START_LOG\"; fi\nMEMOH_ACP_SESSION_POOL_FAKE_AGENT=1 exec %s -test.run '^TestSessionPoolFakeAgentHelper$' --\n", sessionPoolShellArg(os.Args[0]))
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil { //nolint:gosec // test helper must be executable.
		t.Fatal(err)
	}
	return path
}

func TestSessionPoolFakeAgentHelper(_ *testing.T) {
	if os.Getenv("MEMOH_ACP_SESSION_POOL_FAKE_AGENT") != "1" {
		return
	}
	agent := &sessionPoolFakeAgent{}
	conn := acp.NewAgentSideConnection(agent, os.Stdout, os.Stdin)
	agent.conn = conn
	<-conn.Done()
	os.Exit(0)
}

type sessionPoolFakeAgent struct {
	conn *acp.AgentSideConnection
}

func (*sessionPoolFakeAgent) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (*sessionPoolFakeAgent) Initialize(context.Context, acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		ProtocolVersion:   acp.ProtocolVersionNumber,
		AgentCapabilities: acp.AgentCapabilities{LoadSession: false},
	}, nil
}

func (*sessionPoolFakeAgent) Cancel(context.Context, acp.CancelNotification) error { return nil }

func (*sessionPoolFakeAgent) CloseSession(context.Context, acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	return acp.CloseSessionResponse{}, nil
}

func (*sessionPoolFakeAgent) ListSessions(context.Context, acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
}

func (*sessionPoolFakeAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	resp := acp.NewSessionResponse{SessionId: acp.SessionId("session-pool-fake-session")}
	if os.Getenv("MEMOH_ACP_SESSION_POOL_FAKE_AGENT_MODELS") == "1" {
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

func (*sessionPoolFakeAgent) UnstableSetSessionModel(_ context.Context, p acp.UnstableSetSessionModelRequest) (acp.UnstableSetSessionModelResponse, error) {
	if p.SessionId != acp.SessionId("session-pool-fake-session") {
		return acp.UnstableSetSessionModelResponse{}, fmt.Errorf("unexpected session id %q", p.SessionId)
	}
	if p.ModelId == "" {
		return acp.UnstableSetSessionModelResponse{}, errors.New("missing model id")
	}
	return acp.UnstableSetSessionModelResponse{}, nil
}

func (a *sessionPoolFakeAgent) Prompt(ctx context.Context, p acp.PromptRequest) (acp.PromptResponse, error) {
	_ = a.conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: p.SessionId,
		Update:    acp.UpdateAgentMessageText("session-pool-ok"),
	})
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func (*sessionPoolFakeAgent) ResumeSession(context.Context, acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, nil
}

func (*sessionPoolFakeAgent) SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}

func (*sessionPoolFakeAgent) SetSessionMode(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}

func sessionPoolShellArg(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
