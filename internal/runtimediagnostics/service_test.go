package runtimediagnostics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

func TestGetLocalWorkspaceDoesNotCreateBridgeClient(t *testing.T) {
	provider := &diagnosticWorkspaceFake{
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: t.TempDir(),
		},
		mcpErr: errors.New("MCPClient mutates local workspace state"),
	}
	service := NewService(nil, provider, nil, nil, nil)

	resp, err := service.Get(context.Background(), bots.Bot{
		ID:       "bot-1",
		Metadata: enabledSelfManagedACPMetadata(acpprofile.AgentCodexID),
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if provider.mcpCalls != 0 {
		t.Fatalf("MCPClient called %d time(s), want 0 for read-only local diagnostics", provider.mcpCalls)
	}
	if resp.Workspace.State != StateOK {
		t.Fatalf("workspace state = %q, want ok; detail=%q", resp.Workspace.State, resp.Workspace.Detail)
	}
}

func TestGetBridgeUnreachableDoesNotReportCLIMissing(t *testing.T) {
	provider := &diagnosticWorkspaceFake{
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendContainer,
			DefaultWorkDir: "/data",
		},
		mcpErr: errors.New("bridge dial failed"),
	}
	service := NewService(nil, provider, nil, nil, nil)

	resp, err := service.Get(context.Background(), bots.Bot{
		ID:       "bot-1",
		Metadata: enabledSelfManagedACPMetadata(acpprofile.AgentCodexID),
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if resp.Workspace.BridgeReachable {
		t.Fatalf("bridge_reachable = true, want false when MCPClient fails")
	}
	codex := findAgentDiagnostic(resp.ACPAgents, acpprofile.AgentCodexID)
	if codex == nil {
		t.Fatalf("codex diagnostic not found in %#v", resp.ACPAgents)
	}
	if codex.Code != "workspace_bridge_unreachable" {
		t.Fatalf("codex code = %q, want workspace_bridge_unreachable; detail=%q cli_error=%q", codex.Code, codex.Detail, codex.CLI.Error)
	}
}

func TestDiagnoseCLILocalWithoutBridgeUsesHostPath(t *testing.T) {
	command, err := os.Executable()
	if err != nil {
		t.Fatalf("current executable: %v", err)
	}

	diag := diagnoseCLI(context.Background(), nil, acpprofile.Profile{
		Command:      "container-agent-cli",
		LocalCommand: command,
	}, bridge.WorkspaceBackendLocal, t.TempDir())

	if !diag.Available {
		t.Fatalf("diagnostic available = false, error = %q", diag.Error)
	}
	if diag.ResolvedPath != command {
		t.Fatalf("resolved path = %q, want %q", diag.ResolvedPath, command)
	}
	if diag.Source != "local_path" {
		t.Fatalf("source = %q, want local_path", diag.Source)
	}
}

func TestGetLocalCodexOAuthReadsHostWorkspaceAuthWithoutBridge(t *testing.T) {
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, ".codex"), 0o700); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	authJSON := []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id-token","access_token":"access-token","refresh_token":"refresh-token","account_id":"account-id"}}`)
	if err := os.WriteFile(filepath.Join(workDir, ".codex", "auth.json"), authJSON, 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}

	command, err := os.Executable()
	if err != nil {
		t.Fatalf("current executable: %v", err)
	}
	provider := &diagnosticWorkspaceFake{
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: workDir,
		},
		mcpErr: errors.New("MCPClient mutates local workspace state"),
	}
	service := NewService(nil, provider, nil, nil, nil)
	resp, err := service.Get(context.Background(), bots.Bot{
		ID:       "bot-1",
		Metadata: enabledOAuthACPMetadata(acpprofile.AgentCodexID, command),
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if provider.mcpCalls != 0 {
		t.Fatalf("MCPClient called %d time(s), want 0 for read-only local diagnostics", provider.mcpCalls)
	}
	codex := findAgentDiagnostic(resp.ACPAgents, acpprofile.AgentCodexID)
	if codex == nil {
		t.Fatalf("codex diagnostic not found in %#v", resp.ACPAgents)
	}
	if !codex.Auth.OAuthPresent {
		t.Fatalf("OAuthPresent = false, want true; auth=%#v", codex.Auth)
	}
	if len(codex.Auth.MissingFields) != 0 {
		t.Fatalf("MissingFields = %#v, want none", codex.Auth.MissingFields)
	}
	if codex.SessionResume.State == "blocked" {
		t.Fatalf("session resume state = blocked, want not blocked when local Codex OAuth auth.json is present; auth=%#v", codex.Auth)
	}
}

func TestGetLocalCodexOAuthWarnsOnPermissiveAuthFile(t *testing.T) {
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, ".codex"), 0o700); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	authJSON := []byte(`{"auth_mode":"chatgpt","tokens":{"id_token":"id-token","access_token":"access-token","refresh_token":"refresh-token","account_id":"account-id"}}`)
	authPath := filepath.Join(workDir, ".codex", "auth.json")
	if err := os.WriteFile(authPath, authJSON, 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.Chmod(authPath, 0o644); err != nil { //nolint:gosec // test fixture intentionally creates an overly permissive auth file.
		t.Fatalf("chmod auth.json: %v", err)
	}

	command, err := os.Executable()
	if err != nil {
		t.Fatalf("current executable: %v", err)
	}
	provider := &diagnosticWorkspaceFake{
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: workDir,
		},
		mcpErr: errors.New("MCPClient mutates local workspace state"),
	}
	service := NewService(nil, provider, nil, nil, nil)
	resp, err := service.Get(context.Background(), bots.Bot{
		ID:       "bot-1",
		Metadata: enabledOAuthACPMetadata(acpprofile.AgentCodexID, command),
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	codex := findAgentDiagnostic(resp.ACPAgents, acpprofile.AgentCodexID)
	if codex == nil {
		t.Fatalf("codex diagnostic not found in %#v", resp.ACPAgents)
	}
	if codex.Auth.WarningCode != "auth_file_permissive" {
		t.Fatalf("auth warning code = %q, want auth_file_permissive; auth=%#v", codex.Auth.WarningCode, codex.Auth)
	}
	if codex.Code != "auth_file_permissive" || codex.State != StateWarn {
		t.Fatalf("codex state/code = %s/%q, want warn/auth_file_permissive; detail=%q", codex.State, codex.Code, codex.Detail)
	}
	if codex.Auth.OAuthPresent != true || len(codex.Auth.MissingFields) != 0 {
		t.Fatalf("permissive auth file should still count as present without missing fields; auth=%#v", codex.Auth)
	}
}

func TestGetLocalDisplayProbeIsReadonlyWithoutBridge(t *testing.T) {
	provider := &diagnosticWorkspaceFake{
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: t.TempDir(),
		},
		displayEnabled: true,
		mcpErr:         errors.New("MCPClient mutates local workspace state"),
	}
	service := NewService(nil, provider, nil, nil, nil)
	resp, err := service.Get(context.Background(), bots.Bot{
		ID:       "bot-1",
		Metadata: enabledSelfManagedACPMetadata(acpprofile.AgentCodexID),
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if provider.mcpCalls != 0 {
		t.Fatalf("MCPClient called %d time(s), want 0 for read-only local diagnostics", provider.mcpCalls)
	}
	if resp.Display.Evidence["probe"] != "local_display_socket_readonly" {
		t.Fatalf("display probe evidence = %#v, want probe local_display_socket_readonly", resp.Display.Evidence)
	}
	if resp.Display.Evidence["socket_path_configured"] != false {
		t.Fatalf("display socket_path_configured evidence = %#v, want false", resp.Display.Evidence["socket_path_configured"])
	}
}

func TestGetLocalDisplayProbeIncludesSocketEvidence(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "display.sock")
	if err := os.WriteFile(socketPath, []byte("not a socket"), 0o600); err != nil {
		t.Fatalf("write fake display socket marker: %v", err)
	}
	provider := &diagnosticWorkspaceFake{
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: t.TempDir(),
		},
		displayEnabled: true,
		displaySocket:  socketPath,
		mcpErr:         errors.New("MCPClient mutates local workspace state"),
	}
	service := NewService(nil, provider, nil, nil, nil)
	resp, err := service.Get(context.Background(), bots.Bot{
		ID:       "bot-1",
		Metadata: enabledSelfManagedACPMetadata(acpprofile.AgentCodexID),
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if resp.Display.Evidence["probe"] != "local_display_socket_readonly" {
		t.Fatalf("display probe evidence = %#v, want local_display_socket_readonly", resp.Display.Evidence)
	}
	if resp.Display.Evidence["socket_path"] != socketPath {
		t.Fatalf("display socket_path evidence = %#v, want %q", resp.Display.Evidence["socket_path"], socketPath)
	}
	if resp.Display.Evidence["socket_exists"] != true {
		t.Fatalf("display socket_exists evidence = %#v, want true", resp.Display.Evidence["socket_exists"])
	}
}

func TestTypedACPDiagnosticStates(t *testing.T) {
	var model ACPModelDiagnostic
	model.State = ACPModelStateKnown
	if model.State != ACPModelStateKnown {
		t.Fatalf("model state = %q, want known", model.State)
	}

	var resume ACPSessionResumeDiagnostic
	resume.State = ACPSessionResumeStateWarmResumable
	if resume.State != ACPSessionResumeStateWarmResumable {
		t.Fatalf("session resume state = %q, want warm_resumable", resume.State)
	}
}

func enabledSelfManagedACPMetadata(agentID string) map[string]any {
	return map[string]any{
		acpprofile.MetadataKeyACP: map[string]any{
			"agents": map[string]any{
				agentID: map[string]any{
					"enabled":    true,
					"setup_mode": "self",
				},
			},
		},
	}
}

func enabledOAuthACPMetadata(agentID string, localCommand string) map[string]any {
	return map[string]any{
		acpprofile.MetadataKeyACP: map[string]any{
			"agents": map[string]any{
				agentID: map[string]any{
					"enabled":       true,
					"setup_mode":    "oauth",
					"local_command": localCommand,
				},
			},
		},
	}
}

func findAgentDiagnostic(agents []ACPAgentDiagnostic, agentID string) *ACPAgentDiagnostic {
	for i := range agents {
		if agents[i].AgentID == agentID {
			return &agents[i]
		}
	}
	return nil
}

type diagnosticWorkspaceFake struct {
	info           bridge.WorkspaceInfo
	mcpErr         error
	mcpCalls       int
	displayEnabled bool
	displaySocket  string
}

func (w *diagnosticWorkspaceFake) WorkspaceInfo(context.Context, string) (bridge.WorkspaceInfo, error) {
	return w.info, nil
}

func (w *diagnosticWorkspaceFake) MCPClient(context.Context, string) (*bridge.Client, error) {
	w.mcpCalls++
	return nil, w.mcpErr
}

func (w *diagnosticWorkspaceFake) BotDisplayEnabled(context.Context, string) bool {
	return w.displayEnabled
}

func (w *diagnosticWorkspaceFake) DisplaySocketPath(string) string {
	return w.displaySocket
}

func (*diagnosticWorkspaceFake) GetContainerInfo(context.Context, string) (*workspace.ContainerStatus, error) {
	return nil, workspace.ErrContainerNotFound
}

func (*diagnosticWorkspaceFake) GetContainerMetrics(context.Context, string) (*workspace.ContainerMetricsResult, error) {
	return &workspace.ContainerMetricsResult{Supported: true}, nil
}
