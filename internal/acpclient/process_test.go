package acpclient

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/memohai/memoh/internal/workspace/bridge"
	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
)

func TestBuildShellCommandQuotesCommandAndArgs(t *testing.T) {
	got := buildShellCommand("codex-acp", []string{"--flag", "value with spaces", "it's", "$HOME"})
	want := `codex-acp --flag 'value with spaces' 'it'\''s' '$HOME'`
	if got != want {
		t.Fatalf("buildShellCommand() = %q, want %q", got, want)
	}
}

func TestPrepareProcessEnvLocalPassThrough(t *testing.T) {
	client, server := newRecordingBridgeClient(t)
	env, cleanup, err := prepareProcessEnv(context.Background(), client, "/data", processOptions{
		Backend: WorkspaceBackendLocal,
		Env:     []string{"CUSTOM_FLAG=enabled"},
	})
	if err != nil {
		t.Fatalf("prepareProcessEnv() error = %v", err)
	}
	// Local processes inherit the host env and get our managed overrides
	// appended; HOME/PATH are never touched so the host toolchain keeps working.
	assertEnvHas(t, env, "CUSTOM_FLAG=enabled")
	if envHasKey(env, "HOME") || envHasKey(env, "PATH") {
		t.Fatalf("local env must not override HOME/PATH: %v", env)
	}
	if cleanup != nil {
		t.Fatalf("local cleanup should be nil")
	}
	if got := len(server.records()); got != 0 {
		t.Fatalf("local backend executed %d bridge commands, want 0", got)
	}
}

func TestPrepareProcessEnvLocalSelfHasNoOverrides(t *testing.T) {
	client, _ := newRecordingBridgeClient(t)
	env, cleanup, err := prepareProcessEnv(context.Background(), client, "/data", processOptions{
		Backend:       WorkspaceBackendLocal,
		AgentID:       "codex",
		SetupMode:     SetupModeSelf,
		WorkspaceRoot: "/home/user/ws",
	})
	if err != nil {
		t.Fatalf("prepareProcessEnv() error = %v", err)
	}
	if env != nil {
		t.Fatalf("local self env = %v, want nil (host login is used as-is)", env)
	}
	if cleanup != nil {
		t.Fatalf("local cleanup should be nil")
	}
}

func TestPrepareProcessEnvLocalCodexSetsScopedCodexHome(t *testing.T) {
	client, _ := newRecordingBridgeClient(t)
	env, cleanup, err := prepareProcessEnv(context.Background(), client, "/home/user/ws/project", processOptions{
		Backend:       WorkspaceBackendLocal,
		AgentID:       "codex",
		SetupMode:     SetupModeOAuth,
		WorkspaceRoot: "/home/user/ws",
	})
	if err != nil {
		t.Fatalf("prepareProcessEnv() error = %v", err)
	}
	if cleanup != nil {
		t.Fatalf("local cleanup should be nil")
	}
	if got := envValue(env, "CODEX_HOME"); got != "/home/user/ws/.codex" {
		t.Fatalf("local Codex CODEX_HOME = %q, want %q", got, "/home/user/ws/.codex")
	}
	if envHasKey(env, "HOME") {
		t.Fatalf("local Codex must not override HOME: %v", env)
	}
}

func TestPrepareProcessEnvLocalCodexRequiresWorkspaceRoot(t *testing.T) {
	client, _ := newRecordingBridgeClient(t)
	// Without a workspace root we cannot isolate CODEX_HOME, so BYOK Codex must
	// fail loudly rather than silently fall back to the user's real ~/.codex.
	_, _, err := prepareProcessEnv(context.Background(), client, "/home/user/ws/project", processOptions{
		Backend:   WorkspaceBackendLocal,
		AgentID:   "codex",
		SetupMode: SetupModeOAuth,
	})
	if err == nil {
		t.Fatalf("prepareProcessEnv() error = nil, want error for empty WorkspaceRoot")
	}
}

func TestPrepareProcessEnvContainerAPIKey(t *testing.T) {
	client, server := newRecordingBridgeClient(t)
	env, cleanup, err := prepareProcessEnv(context.Background(), client, "/data", processOptions{
		Backend:   WorkspaceBackendContainer,
		AgentID:   "codex",
		SetupMode: SetupModeAPIKey,
		Env:       []string{"CUSTOM_FLAG=enabled", "HOME=/profile-home", "PATH=/profile-bin"},
	})
	if err != nil {
		t.Fatalf("prepareProcessEnv() error = %v", err)
	}
	if cleanup == nil {
		t.Fatalf("managed cleanup is nil")
	}
	assertEnvHas(t, env, "CUSTOM_FLAG=enabled")
	home := envValue(env, "HOME")
	if home != dataMountPath {
		t.Fatalf("api_key Codex HOME = %q, want %q", home, dataMountPath)
	}
	if got := envValue(env, "PATH"); got != defaultContainerPath {
		t.Fatalf("PATH = %q, want %q", got, defaultContainerPath)
	}
	if envHasKeyValue(env, "HOME", "/profile-home") || envHasKeyValue(env, "PATH", "/profile-bin") {
		t.Fatalf("container runtime HOME/PATH must not be overridden by profile env: %v", env)
	}
	if envHasKey(env, "CODEX_HOME") {
		t.Fatalf("api_key Codex runtime must not set CODEX_HOME: %v", env)
	}
	if writes := server.writes(); len(writes) != 0 {
		t.Fatalf("runtime prepare writes = %#v, want no config writes", writes)
	}
	dirs := server.dirs()
	if len(dirs) != 1 || dirs[0] != dataMountPath {
		t.Fatalf("prepare dirs = %#v, want workspace HOME %q", dirs, dataMountPath)
	}

	records := server.records()
	if len(records) != 0 {
		t.Fatalf("prepare records = %#v, want no shell command for temporary HOME", records)
	}

	cleanup()
	records = server.records()
	if len(records) != 0 {
		t.Fatalf("cleanup records = %#v, want no cleanup for workspace HOME", records)
	}
}

func TestPrepareProcessEnvContainerSelf(t *testing.T) {
	client, server := newRecordingBridgeClient(t)
	env, cleanup, err := prepareProcessEnv(context.Background(), client, "/data", processOptions{
		Backend:   WorkspaceBackendContainer,
		AgentID:   "codex",
		Env:       []string{"HOME=/profile-home", "PATH=/profile-bin"},
		SetupMode: SetupModeSelf,
	})
	if err != nil {
		t.Fatalf("prepareProcessEnv() error = %v", err)
	}
	if cleanup != nil {
		t.Fatalf("self cleanup should be nil")
	}
	if got := envValue(env, "HOME"); got != dataMountPath {
		t.Fatalf("self Codex HOME = %q, want %q", got, dataMountPath)
	}
	if got := envValue(env, "PATH"); got != defaultContainerPath {
		t.Fatalf("self PATH = %q, want %q", got, defaultContainerPath)
	}
	records := server.records()
	if len(records) != 0 {
		t.Fatalf("self records = %#v, want no HOME preparation", records)
	}
	if writes := server.writes(); len(writes) != 0 {
		t.Fatalf("self writes = %#v, want no managed config", writes)
	}
}

func TestPrepareProcessEnvContainerSelfGenericAgentDoesNotOverrideHome(t *testing.T) {
	client, _ := newRecordingBridgeClient(t)
	env, cleanup, err := prepareProcessEnv(context.Background(), client, "/data", processOptions{
		Backend:   WorkspaceBackendContainer,
		AgentID:   "other-acp",
		Env:       []string{"HOME=/profile-home", "PATH=/profile-bin"},
		SetupMode: SetupModeSelf,
	})
	if err != nil {
		t.Fatalf("prepareProcessEnv() error = %v", err)
	}
	if cleanup != nil {
		t.Fatalf("self cleanup should be nil")
	}
	if envHasKey(env, "HOME") {
		t.Fatalf("generic self env must not override workspace HOME: %v", env)
	}
	if got := envValue(env, "PATH"); got != defaultContainerPath {
		t.Fatalf("generic self PATH = %q, want %q", got, defaultContainerPath)
	}
}

func TestWriteCodexManagedConfigWritesFixedContainerConfig(t *testing.T) {
	client, server := newRecordingBridgeClient(t)
	err := WriteCodexManagedConfig(context.Background(), client, map[string]string{
		"api_key":  "sk-secret",
		"base_url": "https://proxy.example.com/v1",
	})
	if err != nil {
		t.Fatalf("WriteCodexManagedConfig() error = %v", err)
	}
	writes := server.writes()
	if len(writes) != 2 {
		t.Fatalf("managed writes len = %d, want config.toml + auth.json: %#v", len(writes), writes)
	}
	configWrite, ok := findWrite(writes, CodexManagedConfigDir+"/config.toml")
	if !ok {
		t.Fatalf("missing Codex config.toml write: %#v", writes)
	}
	config := string(configWrite.Content)
	for _, want := range []string{
		`model_provider = "OpenAI"`,
		`model_reasoning_effort = "xhigh"`,
		`model_reasoning_summary = "detailed"`,
		`model_supports_reasoning_summaries = true`,
		`hide_agent_reasoning = false`,
		`show_raw_agent_reasoning = false`,
		`[model_providers.OpenAI]`,
		`name = "OpenAI"`,
		`base_url = "https://proxy.example.com/v1"`,
		`wire_api = "responses"`,
		`requires_openai_auth = false`,
		`supports_websockets = false`,
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("Codex config missing %q:\n%s", want, config)
		}
	}
	if strings.Contains(config, "secret") {
		t.Fatalf("Codex config leaked API key:\n%s", config)
	}
	authWrite, ok := findWrite(writes, CodexManagedConfigDir+"/auth.json")
	if !ok {
		t.Fatalf("missing Codex auth.json write: %#v", writes)
	}
	auth := string(authWrite.Content)
	if !strings.Contains(auth, `"OPENAI_API_KEY": "sk-secret"`) {
		t.Fatalf("Codex auth missing API key:\n%s", auth)
	}
}

func TestWriteCodexManagedConfigWritesOAuthAuth(t *testing.T) { //nolint:gosec // test fixture validates token-shaped Codex auth JSON.
	client, server := newRecordingBridgeClient(t)
	lastRefresh := time.Date(2026, 5, 28, 1, 2, 3, 0, time.UTC)
	err := WriteCodexManagedConfigWithAuth(context.Background(), client, CodexManagedConfig{
		Mode: SetupModeOAuth,
		OAuth: &CodexOAuthCredentials{ //nolint:gosec // test fixture token-shaped values
			AccessToken:  "access.jwt.token",
			IDToken:      "id.jwt.token",
			RefreshToken: "refresh-token",
			AccountID:    "account-123",
			BaseURL:      "https://chatgpt.com/backend-api",
			LastRefresh:  lastRefresh,
		},
	})
	if err != nil {
		t.Fatalf("WriteCodexManagedConfigWithAuth() error = %v", err)
	}
	writes := server.writes()
	if len(writes) != 2 {
		t.Fatalf("managed writes len = %d, want config.toml + auth.json: %#v", len(writes), writes)
	}
	configWrite, ok := findWrite(writes, CodexManagedConfigDir+"/config.toml")
	if !ok {
		t.Fatalf("missing Codex config.toml write: %#v", writes)
	}
	config := string(configWrite.Content)
	for _, want := range []string{
		`model_provider = "chatgpt-http"`,
		`model_reasoning_effort = "xhigh"`,
		`model_reasoning_summary = "detailed"`,
		`model_supports_reasoning_summaries = true`,
		`hide_agent_reasoning = false`,
		`show_raw_agent_reasoning = false`,
		`[model_providers.chatgpt-http]`,
		`name = "ChatGPT HTTP"`,
		`base_url = "https://chatgpt.com/backend-api/codex"`,
		`wire_api = "responses"`,
		`requires_openai_auth = true`,
		`supports_websockets = false`,
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("Codex OAuth config missing %q:\n%s", want, config)
		}
	}
	authWrite, ok := findWrite(writes, CodexManagedConfigDir+"/auth.json")
	if !ok {
		t.Fatalf("missing Codex auth.json write: %#v", writes)
	}
	var auth map[string]any
	if err := json.Unmarshal(authWrite.Content, &auth); err != nil {
		t.Fatalf("invalid auth json: %v\n%s", err, string(authWrite.Content))
	}
	if auth["auth_mode"] != "chatgpt" {
		t.Fatalf("auth_mode = %#v, want chatgpt", auth["auth_mode"])
	}
	tokens, ok := auth["tokens"].(map[string]any)
	if !ok {
		t.Fatalf("tokens missing from auth json: %#v", auth)
	}
	for key, want := range map[string]string{ //nolint:gosec // test fixture token-shaped values
		"id_token":      "id.jwt.token",
		"access_token":  "access.jwt.token",
		"refresh_token": "refresh-token",
		"account_id":    "account-123",
	} {
		if got := tokens[key]; got != want {
			t.Fatalf("tokens[%s] = %#v, want %q", key, got, want)
		}
	}
	if auth["last_refresh"] != lastRefresh.Format(time.RFC3339Nano) {
		t.Fatalf("last_refresh = %#v, want %q", auth["last_refresh"], lastRefresh.Format(time.RFC3339Nano))
	}
}

func TestStartBridgeProcessCanRunWithoutBridgeHardTimeout(t *testing.T) {
	client, server := newRecordingBridgeClient(t)
	proc, err := startBridgeProcess(context.Background(), client, "codex-acp", nil, "/data", time.Minute, processOptions{
		Backend:   WorkspaceBackendContainer,
		AgentID:   "codex",
		SetupMode: SetupModeAPIKey,
		Env:       []string{"TRACE_ID=trace-1"},
		NoTimeout: true,
	})
	if err != nil {
		t.Fatalf("startBridgeProcess() error = %v", err)
	}

	// The exec stream input is sent asynchronously; wait until the bridge
	// recording server has received it before closing the process and
	// reading records.
	server.waitForRecordWithTimeout(t, -1, 2*time.Second)
	_ = proc.Close()

	records := server.records()
	if len(records) < 2 {
		t.Fatalf("records len = %d, want at least command check + process exec: %#v", len(records), records)
	}
	processRecord, ok := findRecordWithTimeout(records, -1)
	if !ok {
		t.Fatalf("expected a record with NoTimeout (-1); got %#v", records)
	}
	if processRecord.Timeout != -1 {
		t.Fatalf("process timeout = %d, want -1 no bridge hard timeout", processRecord.Timeout)
	}
	if strings.Contains(processRecord.Command, "TRACE_ID=trace-1") {
		t.Fatalf("process command leaked env var: %q", processRecord.Command)
	}
	assertEnvHas(t, processRecord.Env, "TRACE_ID=trace-1")
	if envHasKey(processRecord.Env, "CODEX_HOME") {
		t.Fatalf("process env must not set CODEX_HOME: %v", processRecord.Env)
	}
}

func TestStartBridgeProcessUsesContainerToolkitFallback(t *testing.T) {
	client, server := newRecordingBridgeClient(t)
	server.setExitCode("command -v codex-acp >/dev/null 2>&1", 127)

	proc, err := startBridgeProcess(context.Background(), client, "codex-acp", nil, "/data", time.Minute, processOptions{
		Backend:   WorkspaceBackendContainer,
		AgentID:   "codex",
		SetupMode: SetupModeAPIKey,
	})
	if err != nil {
		t.Fatalf("startBridgeProcess() error = %v", err)
	}
	server.waitForRecordWithTimeout(t, int32(time.Minute.Seconds()), 2*time.Second)
	_ = proc.Close()

	processRecord, ok := findRecordWithTimeout(server.records(), int32(time.Minute.Seconds()))
	if !ok {
		t.Fatalf("missing process exec record: %#v", server.records())
	}
	want := containerToolkitBin + "/codex-acp"
	if processRecord.Command != want {
		t.Fatalf("process command = %q, want %q", processRecord.Command, want)
	}
}

func TestStartBridgeProcessRetriesTransientMissingCommand(t *testing.T) {
	oldWindow := commandResolveWindow
	oldDelay := commandResolveDelay
	commandResolveWindow = time.Second
	commandResolveDelay = time.Millisecond
	t.Cleanup(func() {
		commandResolveWindow = oldWindow
		commandResolveDelay = oldDelay
	})

	client, server := newRecordingBridgeClient(t)
	server.setExitCodes("command -v codex-acp >/dev/null 2>&1", 127, 0)
	server.setExitCode("test -x "+containerToolkitBin+"/codex-acp", 1)

	proc, err := startBridgeProcess(context.Background(), client, "codex-acp", nil, "/data", time.Minute, processOptions{
		Backend:   WorkspaceBackendContainer,
		AgentID:   "codex",
		SetupMode: SetupModeAPIKey,
	})
	if err != nil {
		t.Fatalf("startBridgeProcess() error = %v", err)
	}
	server.waitForRecordWithTimeout(t, int32(time.Minute.Seconds()), 2*time.Second)
	_ = proc.Close()

	var checks int
	for _, record := range server.records() {
		if record.Command == "command -v codex-acp >/dev/null 2>&1" {
			checks++
		}
	}
	if checks < 2 {
		t.Fatalf("command checks = %d, want retry; records=%#v", checks, server.records())
	}
	processRecord, ok := findRecordWithTimeout(server.records(), int32(time.Minute.Seconds()))
	if !ok || processRecord.Command != "codex-acp" {
		t.Fatalf("process record = %#v, ok=%v", processRecord, ok)
	}
}

func TestStartBridgeProcessReportsToolkitFallbackFailure(t *testing.T) {
	oldWindow := commandResolveWindow
	commandResolveWindow = 0
	t.Cleanup(func() { commandResolveWindow = oldWindow })

	client, server := newRecordingBridgeClient(t)
	server.setExitCode("command -v codex-acp >/dev/null 2>&1", 127)
	server.setExitCode("test -x "+containerToolkitBin+"/codex-acp", 1)

	_, err := startBridgeProcess(context.Background(), client, "codex-acp", nil, "/data", time.Minute, processOptions{
		Backend:   WorkspaceBackendContainer,
		AgentID:   "codex",
		SetupMode: SetupModeAPIKey,
	})
	if err == nil {
		t.Fatalf("startBridgeProcess() error = nil, want missing command error")
	}
	msg := err.Error()
	for _, want := range []string{"codex-acp", "workspace PATH", containerToolkitBin} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}

type execRecord struct {
	Command string
	WorkDir string
	Env     []string
	Timeout int32
}

type writeRecord struct {
	Path    string
	Content []byte
}

type recordingBridgeServer struct {
	pb.UnimplementedContainerServiceServer

	mu    sync.Mutex
	execs []execRecord
	files []writeRecord
	dirs_ []string
	exits map[string]int32
	seqs  map[string][]int32
}

func (s *recordingBridgeServer) Exec(stream grpc.BidiStreamingServer[pb.ExecInput, pb.ExecOutput]) error {
	input, err := stream.Recv()
	if err != nil {
		return err
	}
	s.mu.Lock()
	exitCode := s.exits[input.GetCommand()]
	if len(s.seqs[input.GetCommand()]) > 0 {
		exitCode = s.seqs[input.GetCommand()][0]
		s.seqs[input.GetCommand()] = s.seqs[input.GetCommand()][1:]
	}
	s.execs = append(s.execs, execRecord{
		Command: input.GetCommand(),
		WorkDir: input.GetWorkDir(),
		Env:     append([]string(nil), input.GetEnv()...),
		Timeout: input.GetTimeoutSeconds(),
	})
	s.mu.Unlock()
	if err := stream.Send(&pb.ExecOutput{Stream: pb.ExecOutput_EXIT, ExitCode: exitCode}); err != nil {
		return err
	}
	return nil
}

func (s *recordingBridgeServer) setExitCodes(command string, codes ...int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seqs == nil {
		s.seqs = make(map[string][]int32)
	}
	s.seqs[command] = append([]int32(nil), codes...)
}

func (s *recordingBridgeServer) setExitCode(command string, code int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exits == nil {
		s.exits = make(map[string]int32)
	}
	s.exits[command] = code
}

func (s *recordingBridgeServer) WriteFile(_ context.Context, req *pb.WriteFileRequest) (*pb.WriteFileResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = append(s.files, writeRecord{
		Path:    req.GetPath(),
		Content: append([]byte(nil), req.GetContent()...),
	})
	return &pb.WriteFileResponse{}, nil
}

func (s *recordingBridgeServer) Mkdir(_ context.Context, req *pb.MkdirRequest) (*pb.MkdirResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dirs_ = append(s.dirs_, req.GetPath())
	return &pb.MkdirResponse{}, nil
}

func (s *recordingBridgeServer) records() []execRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]execRecord, len(s.execs))
	copy(out, s.execs)
	return out
}

func (s *recordingBridgeServer) writes() []writeRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]writeRecord, len(s.files))
	copy(out, s.files)
	return out
}

func (s *recordingBridgeServer) dirs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.dirs_))
	copy(out, s.dirs_)
	return out
}

// waitForRecordWithTimeout polls until a record with the given timeout value
// has been recorded, or the deadline elapses. It is used to bridge the gap
// between the async ExecStreamWithEnv input send and the server-side Recv.
func (s *recordingBridgeServer) waitForRecordWithTimeout(t *testing.T, want int32, deadline time.Duration) {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if _, ok := findRecordWithTimeout(s.records(), want); ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func findRecordWithTimeout(records []execRecord, want int32) (execRecord, bool) {
	for _, rec := range records {
		if rec.Timeout == want {
			return rec, true
		}
	}
	return execRecord{}, false
}

func findWrite(writes []writeRecord, path string) (writeRecord, bool) {
	for _, write := range writes {
		if write.Path == path {
			return write, true
		}
	}
	return writeRecord{}, false
}

func newRecordingBridgeClient(t *testing.T) (*bridge.Client, *recordingBridgeServer) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	recorder := &recordingBridgeServer{}
	pb.RegisterContainerServiceServer(server, recorder)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	conn, err := grpc.NewClient("passthrough:///acpclient-process-test",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return bridge.NewClientFromConn(conn), recorder
}

func assertEnvHas(t *testing.T, env []string, want string) {
	t.Helper()
	for _, item := range env {
		if item == want {
			return
		}
	}
	t.Fatalf("env %v missing %q", env, want)
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return ""
}

func envHasKeyValue(env []string, key, value string) bool {
	want := key + "=" + value
	for _, item := range env {
		if item == want {
			return true
		}
	}
	return false
}

func envHasKey(env []string, key string) bool {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}
