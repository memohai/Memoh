package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	workspacepkg "github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
)

const readImageHint = "Also supports reading image files (PNG, JPEG, GIF, WebP)"

type containerTestBridgeProvider struct {
	client *bridge.Client
	err    error
	info   bridge.WorkspaceInfo
}

type containerTestTargetProvider struct {
	containerTestBridgeProvider
	targets       []workspacepkg.WorkspaceTarget
	resolved      workspacepkg.ResolvedWorkspaceTarget
	resolvedInput string
	resolveErr    error
	listCalls     int
}

func (p *containerTestTargetProvider) ResolveWorkspaceTarget(_ context.Context, _ string, targetID string) (workspacepkg.ResolvedWorkspaceTarget, error) {
	p.resolvedInput = targetID
	return p.resolved, p.resolveErr
}

func (p *containerTestTargetProvider) ListWorkspaceTargets(context.Context, string) ([]workspacepkg.WorkspaceTarget, error) {
	p.listCalls++
	return p.targets, nil
}

func (p containerTestBridgeProvider) MCPClient(context.Context, string) (*bridge.Client, error) {
	return p.client, p.err
}

func (p containerTestBridgeProvider) WorkspaceInfo(context.Context, string) (bridge.WorkspaceInfo, error) {
	return p.info, nil
}

type largeReadTestContainerService struct {
	pb.UnimplementedContainerServiceServer
	size int64
}

func (s *largeReadTestContainerService) Stat(_ context.Context, req *pb.StatRequest) (*pb.StatResponse, error) {
	return &pb.StatResponse{
		Entry: &pb.FileEntry{
			Path: req.GetPath(),
			Size: s.size,
		},
	}, nil
}

func newLargeReadTestClient(t *testing.T, size int64) *bridge.Client {
	t.Helper()

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	pb.RegisterContainerServiceServer(srv, &largeReadTestContainerService{size: size})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.Serve(lis)
	}()
	t.Cleanup(func() {
		srv.Stop()
		<-done
	})

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return bridge.NewClientFromConn(conn)
}

func readToolDescription(t *testing.T, supportsImageInput bool) string {
	t.Helper()
	provider := NewContainerProvider(nil, nil, nil, "")
	toolList, err := provider.Tools(context.Background(), SessionContext{
		BotID:              "bot-1",
		SupportsImageInput: supportsImageInput,
	})
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	for _, tool := range toolList {
		if tool.Name == "read" {
			return tool.Description
		}
	}
	t.Fatalf("read tool not found in container provider tools")
	return ""
}

func TestContainerReadDescriptionIncludesImageHintWhenImageInputSupported(t *testing.T) {
	t.Parallel()
	desc := readToolDescription(t, true)
	if !strings.Contains(desc, readImageHint) {
		t.Fatalf("expected read tool description to contain %q, got:\n%s", readImageHint, desc)
	}
}

func TestContainerReadDescriptionOmitsImageHintWhenImageInputUnsupported(t *testing.T) {
	t.Parallel()
	desc := readToolDescription(t, false)
	if strings.Contains(desc, readImageHint) {
		t.Fatalf("expected read tool description to NOT contain %q, got:\n%s", readImageHint, desc)
	}
}

func TestContainerApplyPatchDescriptionDoesNotReferenceSiblingTools(t *testing.T) {
	t.Parallel()

	provider := NewContainerProvider(nil, nil, nil, "")
	toolList, err := provider.Tools(context.Background(), SessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	for _, tool := range toolList {
		if tool.Name != ToolApplyPatch().String() {
			continue
		}
		for _, absent := range []string{"Use edit", "Use write", "`edit`", "`write`"} {
			if strings.Contains(tool.Description, absent) {
				t.Fatalf("apply_patch description references sibling tool %q:\n%s", absent, tool.Description)
			}
		}
		return
	}
	t.Fatalf("apply_patch tool not found")
}

func TestContainerProviderPreservesWorkspaceClientError(t *testing.T) {
	t.Parallel()

	cause := errors.New("runtime unavailable")
	provider := NewContainerProvider(nil, containerTestBridgeProvider{err: cause}, nil, "")
	_, err := provider.getClient(context.Background(), "bot-1")
	if !errors.Is(err, cause) {
		t.Fatalf("getClient() error = %v, want wrapped runtime cause", err)
	}
	if !strings.Contains(err.Error(), "workspace is not reachable") {
		t.Fatalf("getClient() error = %v, want Workspace-oriented context", err)
	}
}

func TestContainerExecDescriptionUsesRemoteWindowsCommands(t *testing.T) {
	t.Parallel()

	provider := NewContainerProvider(nil, containerTestBridgeProvider{info: bridge.WorkspaceInfo{
		Backend:        bridge.WorkspaceBackendRemote,
		OS:             "win32",
		DefaultWorkDir: `C:\Users\alice\Memoh`,
	}}, nil, "")
	toolList, err := provider.Tools(context.Background(), SessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	tool := toolByNameForTest(t, toolList, ToolExec())
	for _, expected := range []string{"cmd.exe", "run_in_background", "Do not use start"} {
		if !strings.Contains(tool.Description, expected) {
			t.Fatalf("Windows exec description does not contain %q:\n%s", expected, tool.Description)
		}
	}
	params, ok := tool.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("exec parameters = %T, want map[string]any", tool.Parameters)
	}
	properties, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("exec properties = %T, want map[string]any", params["properties"])
	}
	command, ok := properties["command"].(map[string]any)
	if !ok {
		t.Fatalf("command schema = %T, want map[string]any", properties["command"])
	}
	commandDescription, _ := command["description"].(string)
	if !strings.Contains(commandDescription, "dir") || strings.Contains(commandDescription, "ls -la") {
		t.Fatalf("Windows command description = %q", commandDescription)
	}
	description, ok := properties["description"].(map[string]any)
	if !ok {
		t.Fatalf("description schema = %T, want map[string]any", properties["description"])
	}
	descriptionText, _ := description["description"].(string)
	if strings.Contains(descriptionText, "ls -la") || strings.Contains(descriptionText, "curl") {
		t.Fatalf("Windows description guidance = %q", descriptionText)
	}
}

func TestContainerToolsDescribeOptionalExecutionLocationTarget(t *testing.T) {
	t.Parallel()

	targetProvider := &containerTestTargetProvider{targets: []workspacepkg.WorkspaceTarget{
		{TargetID: workspacepkg.WorkspaceTargetNative, Kind: workspacepkg.WorkspaceTargetNative, Name: "Native workspace", Status: workspacepkg.WorkspaceTargetStatusOnline},
		{TargetID: "target-1", Kind: workspacepkg.WorkspaceTargetRemote, Name: "Office PC", Primary: true, Status: workspacepkg.WorkspaceTargetStatusOffline},
	}}
	provider := NewContainerProvider(nil, targetProvider, nil, "")
	toolList, err := provider.Tools(context.Background(), SessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	if targetProvider.listCalls != 0 {
		t.Fatalf("Tools() listed execution locations %d times; schemas must stay static", targetProvider.listCalls)
	}
	_ = toolByNameForTest(t, toolList, ToolListExecutionLocations())
	for _, toolName := range []ToolName{ToolRead(), ToolWrite(), ToolList(), ToolEdit(), ToolApplyPatch(), ToolExec()} {
		tool := toolByNameForTest(t, toolList, toolName)
		params, ok := tool.Parameters.(map[string]any)
		if !ok {
			t.Fatalf("%s parameters = %T", tool.Name, tool.Parameters)
		}
		properties, _ := params["properties"].(map[string]any)
		target, _ := properties["target_id"].(map[string]any)
		description, _ := target["description"].(string)
		if !strings.Contains(description, "Exact target_id returned by list_execution_locations") ||
			!strings.Contains(description, "Do not pass a location name, type, or runtime ID") ||
			!strings.Contains(description, "Omit to use the default location") ||
			strings.Contains(description, "target-1") || strings.Contains(description, "Office PC") || strings.Contains(description, "offline") {
			t.Fatalf("%s target_id description = %q", tool.Name, description)
		}
	}
}

func TestListExecutionLocationsReadsCurrentBotTargetsAtExecutionTime(t *testing.T) {
	t.Parallel()

	targetProvider := &containerTestTargetProvider{targets: []workspacepkg.WorkspaceTarget{
		{
			TargetID:      workspacepkg.WorkspaceTargetNative,
			Kind:          workspacepkg.WorkspaceTargetNative,
			Name:          "Server Workspace",
			Primary:       true,
			Online:        true,
			Status:        workspacepkg.WorkspaceTargetStatusOnline,
			WorkspacePath: "/data",
		},
		{
			TargetID:      "target-1",
			Kind:          workspacepkg.WorkspaceTargetRemote,
			RuntimeID:     "runtime-id-must-not-leak",
			Name:          "Office PC",
			Status:        workspacepkg.WorkspaceTargetStatusOffline,
			WorkspacePath: "projects/memoh",
		},
	}}
	provider := NewContainerProvider(nil, targetProvider, nil, "")
	toolList, err := provider.Tools(context.Background(), SessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	if targetProvider.listCalls != 0 {
		t.Fatalf("Tools() listed execution locations %d times; want 0", targetProvider.listCalls)
	}

	tool := toolByNameForTest(t, toolList, ToolListExecutionLocations())
	raw, err := tool.Execute(&sdk.ToolExecContext{Context: context.Background()}, nil)
	if err != nil {
		t.Fatalf("list_execution_locations error = %v", err)
	}
	result, ok := raw.(listExecutionLocationsResult)
	if !ok {
		t.Fatalf("list_execution_locations result = %T", raw)
	}
	if targetProvider.listCalls != 1 || len(result.Locations) != 2 {
		t.Fatalf("list calls/locations = %d/%d, want 1/2", targetProvider.listCalls, len(result.Locations))
	}
	remote := result.Locations[1]
	if remote.TargetID != "target-1" || remote.Name != "Office PC" || remote.Type != "connected_computer" || remote.Default || remote.Available || remote.Status != workspacepkg.WorkspaceTargetStatusOffline || remote.StartingFolder != "projects/memoh" {
		t.Fatalf("remote execution location = %#v", remote)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal execution locations: %v", err)
	}
	if strings.Contains(string(encoded), "runtime-id-must-not-leak") || strings.Contains(string(encoded), "tool_approval") {
		t.Fatalf("private runtime fields leaked in result: %s", encoded)
	}

	targetProvider.targets[1].Online = true
	targetProvider.targets[1].Status = workspacepkg.WorkspaceTargetStatusOnline
	raw, err = tool.Execute(&sdk.ToolExecContext{Context: context.Background()}, nil)
	if err != nil {
		t.Fatalf("second list_execution_locations error = %v", err)
	}
	result = raw.(listExecutionLocationsResult)
	if targetProvider.listCalls != 2 || !result.Locations[1].Available {
		t.Fatalf("second call did not refresh live status: calls=%d location=%#v", targetProvider.listCalls, result.Locations[1])
	}
}

func TestContainerProviderResolvesOneCanonicalTargetPerInvocation(t *testing.T) {
	t.Parallel()

	targetProvider := &containerTestTargetProvider{resolved: workspacepkg.ResolvedWorkspaceTarget{
		TargetID: "canonical-target",
		Client:   &bridge.Client{},
		Info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendRemote,
			OS:             "win32",
			DefaultWorkDir: `C:\Users\alice\project`,
		},
	}}
	provider := NewContainerProvider(nil, targetProvider, nil, "")
	resolved, err := provider.resolveToolTarget(context.Background(), SessionContext{BotID: "bot-1"}, map[string]any{"target_id": "requested-target"})
	if err != nil {
		t.Fatalf("resolveToolTarget() error = %v", err)
	}
	if targetProvider.resolvedInput != "requested-target" || resolved.id != "canonical-target" {
		t.Fatalf("resolved input/id = %q/%q", targetProvider.resolvedInput, resolved.id)
	}
	if !resolved.workspace.windows || resolved.workspace.defaultWorkDir != `C:\Users\alice\project` {
		t.Fatalf("resolved workspace = %#v", resolved.workspace)
	}
}

func TestContainerProviderExplainsHowToRecoverFromMissingTarget(t *testing.T) {
	t.Parallel()

	targetProvider := &containerTestTargetProvider{resolveErr: workspacepkg.ErrWorkspaceTargetNotFound}
	provider := NewContainerProvider(nil, targetProvider, nil, "")
	_, err := provider.resolveToolTarget(context.Background(), SessionContext{BotID: "bot-1"}, map[string]any{"target_id": "server_workspace"})
	if err == nil {
		t.Fatal("resolveToolTarget() returned nil error")
	}
	if !errors.Is(err, workspacepkg.ErrWorkspaceTargetNotFound) {
		t.Fatalf("resolveToolTarget() error = %v, want workspace target not found", err)
	}
	for _, want := range []string{"target_id", ToolListExecutionLocations().String(), "retry"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("resolveToolTarget() error = %q, want %q", err, want)
		}
	}
}

func TestContainerReadLargeFileErrorDoesNotReferenceSiblingTools(t *testing.T) {
	t.Parallel()

	client := newLargeReadTestClient(t, 17*1024*1024)
	provider := NewContainerProvider(nil, containerTestBridgeProvider{client: client}, nil, "")
	_, err := provider.execRead(context.Background(), SessionContext{BotID: "bot-1"}, map[string]any{
		"path": "/data/large.log",
	})
	if err == nil {
		t.Fatal("expected large file read to fail")
	}
	for _, absent := range []string{ToolExec().String(), "head/tail/sed", "line_offset", "n_lines"} {
		if strings.Contains(err.Error(), absent) {
			t.Fatalf("large file error should not reference unavailable or ineffective fallback %q: %v", absent, err)
		}
	}
	if !strings.Contains(err.Error(), "read tool cannot read files above this limit") {
		t.Fatalf("large file error should state the read limit directly, got %v", err)
	}
}

func TestDetectBlockedSleep(t *testing.T) {
	tests := []struct {
		command string
		blocked bool
	}{
		// Should block
		{"sleep 5", true},
		{"sleep 10", true},
		{"sleep 30", true},
		{"sleep 5 && echo done", true},
		{"sleep 5; echo done", true},

		// Should allow
		{"sleep 1", false},       // under 2 seconds
		{"sleep 0.5", false},     // under 2 seconds
		{"echo hello", false},    // not sleep
		{"npm install", false},   // not sleep
		{"echo sleep 5", false},  // sleep not at start
		{"cat sleep.txt", false}, // not the sleep command
	}

	for _, tt := range tests {
		result := detectBlockedSleep(tt.command)
		if tt.blocked && result == "" {
			t.Errorf("expected %q to be blocked, but it was allowed", tt.command)
		}
		if !tt.blocked && result != "" {
			t.Errorf("expected %q to be allowed, but got: %s", tt.command, result)
		}
	}
}
