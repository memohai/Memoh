package runtimediagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/acpagent"
	"github.com/memohai/memoh/internal/acpclient"
	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/bots"
	dbstore "github.com/memohai/memoh/internal/db/store"
	displaypkg "github.com/memohai/memoh/internal/display"
	"github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

type WorkspaceProvider interface {
	bridge.Provider
	bridge.WorkspaceInfoProvider
	displaypkg.Workspace
	GetContainerInfo(ctx context.Context, botID string) (*workspace.ContainerStatus, error)
	GetContainerMetrics(ctx context.Context, botID string) (*workspace.ContainerMetricsResult, error)
}

type RuntimePool interface {
	RuntimeStatus(sessionID, agentID, projectPath string) acpagent.RuntimeStatus
}

type SessionLister interface {
	ListByBot(ctx context.Context, botID string) ([]session.Session, error)
}

type Service struct {
	logger         *slog.Logger
	workspace      WorkspaceProvider
	displayService *displaypkg.Service
	runtimes       RuntimePool
	sessions       SessionLister
	recorder       *Recorder
	now            func() time.Time
}

const DefaultHousekeepingInterval = 6 * time.Hour

func NewService(log *slog.Logger, workspaceProvider WorkspaceProvider, runtimes RuntimePool, sessions SessionLister, queries dbstore.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	s := &Service{
		logger:    log.With(slog.String("service", "runtime_diagnostics")),
		workspace: workspaceProvider,
		runtimes:  runtimes,
		sessions:  sessions,
		recorder:  NewRecorder(queries),
		now:       time.Now,
	}
	if workspaceProvider != nil {
		s.displayService = displaypkg.NewService(s.logger, workspaceProvider)
	}
	return s
}

func (s *Service) Recorder() *Recorder {
	if s == nil {
		return nil
	}
	return s.recorder
}

func (s *Service) StartHousekeeper(ctx context.Context, interval time.Duration) {
	if s == nil || s.recorder == nil {
		return
	}
	if interval <= 0 {
		interval = DefaultHousekeepingInterval
	}
	s.pruneEvents(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pruneEvents(ctx)
		}
	}
}

func (s *Service) pruneEvents(ctx context.Context) {
	if s == nil || s.recorder == nil {
		return
	}
	if err := s.recorder.Prune(ctx); err != nil {
		s.logger.Warn("prune runtime diagnostic events failed", slog.Any("error", err))
	}
}

func (s *Service) nowOrDefault() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Service) Get(ctx context.Context, bot bots.Bot) (Response, error) {
	checkedAt := s.nowOrDefault()
	resp := Response{
		CheckedAt: checkedAt,
		Workspace: WorkspaceDiagnostic{DiagnosticItem: DiagnosticItem{
			State: StateUnknown,
			Code:  "workspace_unknown",
			Label: "Workspace runtime",
		}},
		Container: ContainerDiagnostic{DiagnosticItem: DiagnosticItem{
			State: StateUnknown,
			Code:  "container_unknown",
			Label: "Container runtime",
		}},
		Display: DisplayDiagnostic{DiagnosticItem: DiagnosticItem{
			State: StateUnknown,
			Code:  "display_unknown",
			Label: "Workspace display",
		}},
	}

	workspaceInfo, client, clientErr := s.resolveWorkspace(ctx, bot.ID, &resp.Workspace)
	resp.Container = s.diagnoseContainer(ctx, bot.ID, workspaceInfo)
	resp.Display = s.diagnoseDisplay(ctx, bot.ID, workspaceInfo, client)

	recentEvents, err := s.recorder.ListRecent(ctx, bot.ID, 10)
	if err != nil {
		s.logger.Warn("list runtime diagnostic events failed", slog.String("bot_id", bot.ID), slog.Any("error", err))
	}
	resp.RecentEvents = recentEvents
	resp.ACPAgents = s.diagnoseACPAgents(ctx, bot, workspaceInfo, client, clientErr, recentEvents)
	resp.OverallState = overallState(resp)
	resp.Summary = summarize(resp)
	return resp, nil
}

func (s *Service) resolveWorkspace(ctx context.Context, botID string, out *WorkspaceDiagnostic) (bridge.WorkspaceInfo, *bridge.Client, error) {
	if s == nil || s.workspace == nil {
		out.State = StateError
		out.Code = "workspace_manager_missing"
		out.Detail = "Workspace manager is not configured."
		out.NextAction = "Check server workspace runtime configuration."
		return bridge.WorkspaceInfo{}, nil, errors.New("workspace manager not configured")
	}
	info, err := s.workspace.WorkspaceInfo(ctx, botID)
	if err != nil {
		out.State = StateError
		out.Code = "workspace_info_failed"
		out.Detail = err.Error()
		out.NextAction = "Check workspace backend configuration."
		return bridge.WorkspaceInfo{}, nil, err
	}
	info.Backend = normalizeWorkspaceBackend(info.Backend)
	out.Backend = info.Backend
	out.DefaultWorkDir = strings.TrimSpace(info.DefaultWorkDir)
	out.Evidence = map[string]any{
		"acp_tools_http_url": strings.TrimSpace(info.ACPToolsHTTPURL),
	}

	if info.Backend == bridge.WorkspaceBackendLocal {
		out.State = StateOK
		out.Code = "workspace_local"
		out.Detail = "Trusted local workspace is configured. Bridge probes are skipped to keep diagnostics read-only."
		out.Evidence["bridge_probe"] = "skipped_for_local_readonly"
		return info, nil, nil
	}

	client, err := s.workspace.MCPClient(ctx, botID)
	if err != nil {
		out.State = StateWarn
		out.Code = "bridge_unreachable"
		out.Detail = err.Error()
		out.NextAction = "Open the Workspace Runtime settings and start or recreate the workspace."
		return info, nil, err
	}
	out.State = StateOK
	out.Code = "workspace_reachable"
	out.Detail = "Workspace bridge is reachable."
	out.BridgeReachable = client != nil
	out.MCPReachable = client != nil
	return info, client, nil
}

func (s *Service) diagnoseContainer(ctx context.Context, botID string, info bridge.WorkspaceInfo) ContainerDiagnostic {
	diag := ContainerDiagnostic{DiagnosticItem: DiagnosticItem{
		Label: "Container runtime",
	}}
	backend := normalizeWorkspaceBackend(info.Backend)
	diag.WorkspaceBackend = backend
	if s == nil || s.workspace == nil {
		diag.State = StateUnknown
		diag.Code = "container_manager_missing"
		diag.Detail = "Workspace manager is not configured."
		return diag
	}
	if backend == bridge.WorkspaceBackendLocal {
		diag.State = StateNotApplicable
		diag.Code = "workspace_local"
		diag.Detail = "Bot uses a trusted local workspace; no isolated container is expected."
		return diag
	}
	status, err := s.workspace.GetContainerInfo(ctx, botID)
	if err != nil {
		if errors.Is(err, workspace.ErrContainerNotFound) {
			diag.State = StateWarn
			diag.Code = "runtime_cold_start_required"
			diag.Detail = "No workspace container exists for this bot yet."
			diag.NextAction = "Create or start the workspace from Container settings when you are ready."
			return diag
		}
		diag.State = StateError
		diag.Code = "container_status_failed"
		diag.Detail = err.Error()
		diag.NextAction = "Check the configured container runtime."
		return diag
	}
	diag.Exists = true
	diag.ContainerID = status.ContainerID
	diag.RuntimeBackend = status.RuntimeBackend
	diag.Status = status.Status
	diag.TaskRunning = status.TaskRunning
	diag.Evidence = map[string]any{
		"image":              status.Image,
		"namespace":          status.Namespace,
		"container_path":     status.ContainerPath,
		"has_preserved_data": status.HasPreservedData,
		"legacy":             status.Legacy,
	}
	if metrics, err := s.workspace.GetContainerMetrics(ctx, botID); err == nil && metrics != nil {
		diag.MetricsSupported = metrics.Supported
		diag.MetricsUnsupportedReason = metrics.UnsupportedReason
		if !metrics.SampledAt.IsZero() {
			sampledAt := metrics.SampledAt
			diag.SampledAt = &sampledAt
		}
	} else if err != nil {
		diag.Evidence["metrics_error"] = err.Error()
	}
	if status.TaskRunning {
		diag.State = StateOK
		diag.Code = "container_running"
		diag.Detail = "Workspace container task is running."
		return diag
	}
	diag.State = StateWarn
	diag.Code = "container_stopped"
	diag.Detail = "Workspace container exists but its task is not running."
	diag.NextAction = "Start the workspace from Container settings when runtime execution is needed."
	return diag
}

func (s *Service) diagnoseDisplay(ctx context.Context, botID string, info bridge.WorkspaceInfo, client *bridge.Client) DisplayDiagnostic {
	diag := DisplayDiagnostic{DiagnosticItem: DiagnosticItem{
		Label: "Workspace display",
	}}
	if s == nil || s.workspace == nil || s.displayService == nil {
		diag.State = StateUnknown
		diag.Code = "display_manager_missing"
		diag.Detail = "Display service is not configured."
		return diag
	}
	status := s.displayService.Status(ctx, botID)
	diag.Enabled = status.Enabled
	diag.Available = status.Available
	diag.Running = status.Running
	diag.Transport = status.Transport
	diag.Encoder = status.Encoder
	diag.EncoderAvailable = status.EncoderAvailable
	diag.UnavailableReason = status.UnavailableReason
	if !status.Enabled {
		diag.State = StateDisabled
		diag.Code = "display_disabled"
		diag.Detail = "Workspace display is disabled for this bot."
		return diag
	}
	if client != nil {
		if probe, ok := probeDisplayRuntime(ctx, client); ok {
			diag.ToolkitAvailable = probe.ToolkitAvailable
			diag.PrepareSupported = probe.PrepareSupported
			diag.PrepareSystem = probe.PrepareSystem
			diag.DesktopAvailable = probe.DesktopAvailable
			diag.BrowserAvailable = probe.BrowserAvailable
			diag.A11yAvailable = probe.A11yAvailable
			if !diag.Running && !probe.VNCAvailable && diag.UnavailableReason == "" {
				diag.UnavailableReason = "display bundle unavailable"
			}
		}
	} else {
		probe := "workspace_bridge_unavailable"
		evidence := map[string]any{"probe": probe}
		if normalizeWorkspaceBackend(info.Backend) == bridge.WorkspaceBackendLocal {
			evidence = s.localDisplayEvidence(botID)
		}
		diag.Evidence = evidence
	}
	if status.Available {
		diag.State = StateOK
		diag.Code = "display_available"
		diag.Detail = "Workspace display is enabled and reachable."
		return diag
	}
	diag.State = StateWarn
	diag.Code = "display_unavailable"
	diag.Detail = fallback(status.UnavailableReason, "Workspace display is enabled but not currently reachable.")
	diag.NextAction = "Open Desktop settings to prepare or start display when visual tools are needed."
	return diag
}

func (s *Service) localDisplayEvidence(botID string) map[string]any {
	evidence := map[string]any{
		"probe": "local_display_socket_readonly",
	}
	if s == nil || s.workspace == nil {
		return evidence
	}
	socketPath := strings.TrimSpace(s.workspace.DisplaySocketPath(botID))
	evidence["socket_path_configured"] = socketPath != ""
	if socketPath == "" {
		return evidence
	}
	evidence["socket_path"] = socketPath
	info, err := os.Stat(socketPath)
	if err != nil {
		evidence["socket_exists"] = false
		if !os.IsNotExist(err) {
			evidence["socket_stat_error"] = err.Error()
		}
		return evidence
	}
	evidence["socket_exists"] = true
	evidence["socket_mode"] = info.Mode().String()
	return evidence
}

func (s *Service) diagnoseACPAgents(ctx context.Context, bot bots.Bot, workspaceInfo bridge.WorkspaceInfo, client *bridge.Client, clientErr error, recent []RuntimeEventSummary) []ACPAgentDiagnostic {
	profiles := acpprofile.List()
	out := make([]ACPAgentDiagnostic, 0, len(profiles))
	sessionsByAgent := s.acpSessionsByAgent(ctx, bot.ID)
	for _, public := range profiles {
		profile, ok := acpprofile.Lookup(public.ID)
		if !ok {
			continue
		}
		out = append(out, s.diagnoseACPAgent(ctx, bot, profile, workspaceInfo, client, clientErr, sessionsByAgent[profile.ID], recent))
	}
	return out
}

func (s *Service) diagnoseACPAgent(ctx context.Context, bot bots.Bot, profile acpprofile.Profile, workspaceInfo bridge.WorkspaceInfo, client *bridge.Client, clientErr error, sessions []session.Session, recent []RuntimeEventSummary) ACPAgentDiagnostic {
	backend := normalizeWorkspaceBackend(workspaceInfo.Backend)
	setup := acpprofile.ParseAgentSetup(bot.Metadata, profile.ID)
	mode := resolveSetupMode(setup, backend)
	profileDiag := ACPProfileDiagnostic{
		Registered:        true,
		BackendSupported:  slices.Contains(profile.SupportedBackends, backend),
		SessionModePin:    profile.SessionModeID,
		SessionConfigPins: copyStringMap(profile.SessionConfigValues),
	}
	cli := diagnoseCLI(ctx, client, profile, backend, strings.TrimSpace(workspaceInfo.DefaultWorkDir))
	auth := diagnoseAuth(ctx, client, profile, setup, mode, backend, strings.TrimSpace(workspaceInfo.DefaultWorkDir))
	resume := s.diagnoseSessionResume(profile.ID, setup.Enabled, sessions, cli.Available, auth, profileDiag)
	model := diagnoseModel(resume, s.runtimes)
	lastError := lastAgentError(profile.ID, recent)

	item := DiagnosticItem{
		Label:    profile.DisplayName,
		Evidence: map[string]any{"workspace_backend": backend},
	}
	switch {
	case !setup.Enabled:
		item.State = StateDisabled
		item.Code = "acp_agent_disabled"
		item.Detail = "ACP provider is disabled for this bot."
	case !profileDiag.BackendSupported:
		item.State = StateError
		item.Code = "unsupported_backend"
		item.Detail = fmt.Sprintf("%s does not support %s workspace backend.", profile.DisplayName, backend)
		item.NextAction = "Switch workspace backend or choose a provider that supports this backend."
	case clientErr != nil:
		item.State = StateWarn
		item.Code = "workspace_bridge_unreachable"
		item.Detail = clientErr.Error()
		item.NextAction = "Start or repair the workspace runtime before launching ACP."
	case !cli.Available:
		item.State = StateError
		item.Code = "cli_missing"
		item.Detail = fallback(cli.Error, "Provider CLI is not available in the selected workspace.")
		item.NextAction = "Install the provider CLI or adjust the provider command."
	case len(auth.MissingFields) > 0:
		item.State = StateError
		item.Code = "auth_missing"
		item.Detail = "Required provider authentication is missing."
		item.NextAction = "Open ACP provider settings and configure authentication."
	case auth.WarningCode != "":
		item.State = StateWarn
		item.Code = auth.WarningCode
		item.Detail = auth.WarningDetail
		item.NextAction = "Review the local auth file permissions before starting ACP."
	case resume.State == ACPSessionResumeStateWarmResumable:
		item.State = StateOK
		item.Code = "warm_resumable"
		item.Detail = "A matching ACP runtime is already warm and can resume this session."
	case resume.State == ACPSessionResumeStateColdStartRequired:
		item.State = StateWarn
		item.Code = "runtime_cold_start_required"
		item.Detail = "An ACP session exists, but no matching runtime is currently warm."
	default:
		item.State = StateUnknown
		item.Code = "no_acp_session"
		item.Detail = "No ACP session exists yet for this provider."
	}

	return ACPAgentDiagnostic{
		DiagnosticItem:   item,
		AgentID:          profile.ID,
		DisplayName:      profile.DisplayName,
		Enabled:          setup.Enabled,
		SetupMode:        string(mode),
		WorkspaceBackend: backend,
		CLI:              cli,
		Auth:             auth,
		Profile:          profileDiag,
		Model:            model,
		SessionResume:    resume,
		LastError:        lastError,
	}
}

func diagnoseCLI(ctx context.Context, client *bridge.Client, profile acpprofile.Profile, backend string, workDir string) ACPCLIDiagnostic {
	effectiveCommand := strings.TrimSpace(profile.Command)
	effectiveArgs := append([]string(nil), profile.Args...)
	if backend == bridge.WorkspaceBackendLocal && strings.TrimSpace(profile.LocalCommand) != "" {
		effectiveCommand = strings.TrimSpace(profile.LocalCommand)
		effectiveArgs = append([]string(nil), profile.LocalArgs...)
	}
	diag := ACPCLIDiagnostic{
		ConfiguredCommand: strings.TrimSpace(profile.Command),
		ConfiguredArgs:    append([]string(nil), profile.Args...),
		EffectiveCommand:  effectiveCommand,
		EffectiveArgs:     effectiveArgs,
	}
	if client == nil {
		if backend == bridge.WorkspaceBackendLocal {
			applyLocalCommandDiagnostic(&diag, effectiveCommand)
			return diag
		}
		diag.Error = "workspace bridge client is unavailable"
		return diag
	}
	backendForProbe := acpclient.WorkspaceBackendContainer
	if backend == bridge.WorkspaceBackendLocal {
		backendForProbe = acpclient.WorkspaceBackendLocal
	}
	probe := acpclient.ResolveCommandDiagnostic(ctx, client, acpclient.CommandDiagnosticRequest{
		Command: effectiveCommand,
		WorkDir: workDir,
		Backend: backendForProbe,
	})
	diag.ResolvedPath = probe.ResolvedCommand
	diag.Source = probe.Source
	diag.Available = probe.Available
	diag.Error = probe.Error
	diag.Checks = append([]string(nil), probe.Checks...)
	return diag
}

func applyLocalCommandDiagnostic(diag *ACPCLIDiagnostic, command string) {
	if diag == nil {
		return
	}
	command = strings.TrimSpace(command)
	if command == "" {
		diag.Error = "ACP command is required"
		return
	}
	if !isPlainLocalCommand(command) {
		diag.Available = true
		diag.ResolvedPath = command
		diag.Source = "raw_command"
		return
	}
	if strings.Contains(command, "/") {
		diag.Source = "local_path"
		diag.Checks = append(diag.Checks, "test -x "+command)
		info, err := os.Stat(command)
		if err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			diag.Available = true
			diag.ResolvedPath = command
			return
		}
		diag.Error = localCommandNotAvailableError(command)
		return
	}
	diag.Source = "local_path"
	diag.Checks = append(diag.Checks, "exec.LookPath "+command)
	resolved, err := exec.LookPath(command)
	if err != nil {
		diag.Error = localCommandNotAvailableError(command)
		return
	}
	diag.Available = true
	diag.ResolvedPath = resolved
}

func diagnoseAuth(ctx context.Context, client *bridge.Client, profile acpprofile.Profile, setup acpprofile.AgentSetup, mode acpclient.SetupMode, backend string, workDir string) ACPAuthDiagnostic {
	auth := ACPAuthDiagnostic{
		Mode: string(mode),
	}
	switch mode {
	case acpclient.SetupModeSelf:
		auth.SelfManaged = true
		auth.Source = "workspace"
		return auth
	case acpclient.SetupModeOAuth:
		auth.Source = "managed"
		if profile.ID == acpprofile.AgentCodexID {
			auth.Source = "workspace_auth_file"
			if normalizeWorkspaceBackend(backend) == bridge.WorkspaceBackendLocal && client == nil {
				localAuth := localCodexOAuthStatus(workDir)
				auth.OAuthPresent = localAuth.present
				auth.WarningCode = localAuth.warningCode
				auth.WarningDetail = localAuth.warningDetail
			} else {
				auth.OAuthPresent = codexOAuthPresent(ctx, client)
			}
			if !auth.OAuthPresent {
				auth.RequiredFields = []string{"auth.json"}
				auth.MissingFields = []string{"auth.json"}
			}
			return auth
		}
		auth.OAuthPresent = strings.TrimSpace(setup.Managed["oauth_token"]) != ""
		if !auth.OAuthPresent {
			auth.RequiredFields = []string{"oauth_token"}
			auth.MissingFields = []string{"oauth_token"}
		}
		return auth
	default:
		auth.Source = "managed"
		auth.APIKeyPresent = strings.TrimSpace(setup.Managed["api_key"]) != ""
		if !auth.APIKeyPresent {
			auth.RequiredFields = []string{"api_key"}
			auth.MissingFields = []string{"api_key"}
		}
		return auth
	}
}

func (s *Service) diagnoseSessionResume(agentID string, enabled bool, sessions []session.Session, cliAvailable bool, auth ACPAuthDiagnostic, profile ACPProfileDiagnostic) ACPSessionResumeDiagnostic {
	if !enabled {
		return ACPSessionResumeDiagnostic{State: ACPSessionResumeStateDisabled, Detail: "ACP provider is disabled."}
	}
	if !profile.BackendSupported || !cliAvailable || len(auth.MissingFields) > 0 {
		return ACPSessionResumeDiagnostic{State: ACPSessionResumeStateBlocked, Detail: "Profile, CLI, or auth blocks runtime startup."}
	}
	if len(sessions) == 0 {
		return ACPSessionResumeDiagnostic{State: ACPSessionResumeStateNoACPSession, Detail: "No ACP session exists for this provider."}
	}
	for _, sess := range sessions {
		projectPath := metadataString(sess.Metadata, "project_path")
		status := acpagent.RuntimeStatus{}
		if s != nil && s.runtimes != nil {
			status = s.runtimes.RuntimeStatus(sess.ID, agentID, projectPath)
		}
		if strings.TrimSpace(status.RuntimeID) != "" && strings.TrimSpace(status.ACPSession) != "" {
			return ACPSessionResumeDiagnostic{
				State:      ACPSessionResumeStateWarmResumable,
				SessionID:  sess.ID,
				RuntimeID:  status.RuntimeID,
				ACPSession: status.ACPSession,
				Detail:     "A live runtime matches the ACP session.",
			}
		}
	}
	return ACPSessionResumeDiagnostic{
		State:     ACPSessionResumeStateColdStartRequired,
		SessionID: sessions[0].ID,
		Detail:    "ACP session exists, but no matching runtime process is warm.",
	}
}

func diagnoseModel(resume ACPSessionResumeDiagnostic, runtimes RuntimePool) ACPModelDiagnostic {
	if resume.State != ACPSessionResumeStateWarmResumable || runtimes == nil {
		return ACPModelDiagnostic{
			State:  ACPModelStateUnknownUntilRuntimeStart,
			Detail: "Model state is only available after the ACP runtime starts.",
		}
	}
	status := runtimes.RuntimeStatus(resume.SessionID, "", "")
	model := ACPModelDiagnostic{
		State:          ACPModelStateKnown,
		DefaultModelID: status.DefaultModelID,
	}
	if status.Models != nil {
		model.CurrentModelID = status.Models.CurrentModelID
		model.Available = append([]acpclient.ModelInfo(nil), status.Models.Available...)
		if !status.Models.Supported {
			model.State = ACPModelStateUnsupported
			model.Detail = "ACP runtime does not expose selectable models."
		}
	}
	if model.CurrentModelID == "" && model.DefaultModelID == "" && len(model.Available) == 0 {
		model.State = ACPModelStateUnknown
		model.Detail = "Warm runtime did not report model state."
	}
	return model
}

func (s *Service) acpSessionsByAgent(ctx context.Context, botID string) map[string][]session.Session {
	out := map[string][]session.Session{}
	if s == nil || s.sessions == nil {
		return out
	}
	sessions, err := s.sessions.ListByBot(ctx, botID)
	if err != nil {
		s.logger.Warn("list bot sessions for runtime diagnostics failed", slog.String("bot_id", botID), slog.Any("error", err))
		return out
	}
	for _, sess := range sessions {
		if sess.Type != session.TypeACPAgent {
			continue
		}
		agentID := acpprofile.NormalizeAgentID(metadataString(sess.Metadata, "acp_agent_id"))
		if agentID == "" {
			agentID = acpprofile.AgentCodexID
		}
		out[agentID] = append(out[agentID], sess)
	}
	return out
}

func overallState(resp Response) State {
	state := StateOK
	merge := func(next State) {
		switch next {
		case StateError:
			state = StateError
		case StateWarn:
			if state != StateError {
				state = StateWarn
			}
		case StateUnknown:
			if state == StateOK {
				state = StateUnknown
			}
		}
	}
	merge(resp.Workspace.State)
	merge(resp.Container.State)
	merge(resp.Display.State)
	for _, agent := range resp.ACPAgents {
		if !agent.Enabled {
			continue
		}
		merge(agent.State)
	}
	return state
}

func summarize(resp Response) string {
	enabled := 0
	blocked := 0
	warm := 0
	for _, agent := range resp.ACPAgents {
		if !agent.Enabled {
			continue
		}
		enabled++
		if agent.State == StateError {
			blocked++
		}
		if agent.SessionResume.State == ACPSessionResumeStateWarmResumable {
			warm++
		}
	}
	return fmt.Sprintf("Workspace %s, container %s, display %s. ACP providers: %d enabled, %d blocked, %d warm resumable.",
		resp.Workspace.State, resp.Container.State, resp.Display.State, enabled, blocked, warm)
}

func resolveSetupMode(setup acpprofile.AgentSetup, backend string) acpclient.SetupMode {
	mode := acpclient.SetupMode(setup.Mode)
	if !setup.ModeSet {
		if backend == bridge.WorkspaceBackendLocal {
			return acpclient.SetupModeSelf
		}
		return acpclient.SetupModeAPIKey
	}
	switch mode {
	case acpclient.SetupModeOAuth, acpclient.SetupModeSelf:
		return mode
	default:
		return acpclient.SetupModeAPIKey
	}
}

func normalizeWorkspaceBackend(backend string) string {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case bridge.WorkspaceBackendLocal:
		return bridge.WorkspaceBackendLocal
	default:
		return bridge.WorkspaceBackendContainer
	}
}

func isPlainLocalCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	return !strings.ContainsAny(command, " \t\n'\"\\$&;|<>*?()[]{}!`")
}

func localCommandNotAvailableError(command string) string {
	return fmt.Sprintf("ACP command %q is not available to the local workspace process. Install the ACP agent command and restart Memoh Desktop/local server so PATH is inherited", command)
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func lastAgentError(agentID string, events []RuntimeEventSummary) *RuntimeEventSummary {
	for i := range events {
		event := events[i]
		if event.Scope == "acp" && event.AgentID == agentID && event.Severity == "error" {
			return &event
		}
	}
	return nil
}

func codexOAuthPresent(ctx context.Context, client *bridge.Client) bool {
	if client == nil {
		return false
	}
	resp, err := client.ReadFile(ctx, path.Join(acpclient.CodexManagedConfigDir, "auth.json"), 0, 0)
	if err != nil || resp == nil {
		return false
	}
	return codexOAuthPresentFromJSON([]byte(resp.Content))
}

type localCodexOAuthDiagnostic struct {
	present       bool
	warningCode   string
	warningDetail string
}

func localCodexOAuthStatus(workDir string) localCodexOAuthDiagnostic {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return localCodexOAuthDiagnostic{}
	}
	authPath := filepath.Join(workDir, ".codex", "auth.json")
	info, statErr := os.Stat(authPath)
	content, err := os.ReadFile(authPath) //nolint:gosec // read-only presence check for the bot-scoped local workspace auth file.
	if err != nil {
		return localCodexOAuthDiagnostic{}
	}
	diag := localCodexOAuthDiagnostic{present: codexOAuthPresentFromJSON(content)}
	if statErr == nil && info != nil && info.Mode().Perm()&0o077 != 0 {
		diag.warningCode = "auth_file_permissive"
		diag.warningDetail = "Local Codex auth.json is readable by group or other users."
	}
	return diag
}

func codexOAuthPresentFromJSON(content []byte) bool {
	var auth struct {
		AuthMode string `json:"auth_mode"`
		Tokens   struct {
			IDToken      string `json:"id_token"`
			AccessToken  string `json:"access_token"`  //nolint:gosec // read-only presence check; value is never returned.
			RefreshToken string `json:"refresh_token"` //nolint:gosec // read-only presence check; value is never returned.
			AccountID    string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(content, &auth); err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(auth.AuthMode), "chatgpt") &&
		strings.TrimSpace(auth.Tokens.IDToken) != "" &&
		strings.TrimSpace(auth.Tokens.AccessToken) != "" &&
		strings.TrimSpace(auth.Tokens.RefreshToken) != "" &&
		strings.TrimSpace(auth.Tokens.AccountID) != "" &&
		auth.Tokens.IDToken != auth.Tokens.AccessToken
}
