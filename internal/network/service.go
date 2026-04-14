package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/jackc/pgx/v5"

	ctr "github.com/memohai/memoh/internal/containerd"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type statusRuntime interface {
	GetTaskInfo(ctx context.Context, containerID string) (ctr.TaskInfo, error)
}

var ErrWorkspaceContainerMissing = errors.New("workspace container is not created")

type Service struct {
	queries          *sqlc.Queries
	registry         *Registry
	runtime          statusRuntime
	controller       Controller
	runtimeKind      string
	cniBinDir        string
	cniConfDir       string
	networkStateRoot string
	logger           *slog.Logger
}

func NewService(log *slog.Logger, queries *sqlc.Queries, registry *Registry, runtime statusRuntime, runtimeKind, cniBinDir, cniConfDir, networkStateRoot string) *Service {
	return &Service{
		queries:          queries,
		registry:         registry,
		runtime:          runtime,
		runtimeKind:      normalizeKind(runtimeKind),
		cniBinDir:        cniBinDir,
		cniConfDir:       cniConfDir,
		networkStateRoot: networkStateRoot,
		logger:           log.With(slog.String("service", "network")),
	}
}

// SetController injects the network controller after construction to break
// the Service ↔ Controller circular dependency (Controller depends on Service
// as its BindingResolver).
func (s *Service) SetController(ctrl Controller) {
	s.controller = ctrl
}

func (s *Service) ListMeta(_ context.Context) []ProviderDescriptor {
	if s.registry == nil {
		return nil
	}
	return s.registry.ListDescriptors()
}

func (s *Service) Resolve(ctx context.Context, botID string) (BotNetworkConfig, error) {
	return s.GetBotConfig(ctx, botID)
}

func (s *Service) GetBotConfig(ctx context.Context, botID string) (BotNetworkConfig, error) {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return BotNetworkConfig{}, err
	}
	row, err := s.queries.GetBotNetworkConfig(ctx, pgBotID)
	if err != nil {
		return BotNetworkConfig{}, err
	}
	return s.normalizeBotConfig(
		row.NetworkEnabled,
		row.NetworkProvider,
		decodeJSONMap(row.NetworkConfig),
	)
}

func (s *Service) PrepareBotConfigForWrite(cfg BotNetworkConfig) (BotNetworkConfig, error) {
	normalized, err := s.normalizeBotConfig(
		cfg.Enabled,
		cfg.Provider,
		cfg.Config,
	)
	if err != nil {
		return BotNetworkConfig{}, err
	}
	if !normalized.Enabled || strings.TrimSpace(normalized.Provider) == "" {
		return normalized, nil
	}
	provider, err := s.requireProvider(normalized.Provider)
	if err != nil {
		return BotNetworkConfig{}, err
	}
	if _, err := provider.BuildDriver(normalized); err != nil {
		return BotNetworkConfig{}, err
	}
	return normalized, nil
}

func withWorkspace(st BotStatus, ws WorkspaceRuntimeStatus) BotStatus {
	w := ws
	st.Workspace = &w
	return st
}

func (s *Service) workspaceRuntimeStatus(ctx context.Context, botID string) WorkspaceRuntimeStatus {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return WorkspaceRuntimeStatus{State: "unknown", Message: "invalid bot id"}
	}
	row, err := s.queries.GetContainerByBotID(ctx, pgBotID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WorkspaceRuntimeStatus{State: "workspace_missing"}
		}
		return WorkspaceRuntimeStatus{State: "unknown", Message: err.Error()}
	}
	out := WorkspaceRuntimeStatus{
		State:       "task_stopped",
		ContainerID: strings.TrimSpace(row.ContainerID),
	}
	if s.runtime == nil {
		out.State = "runtime_unavailable"
		return out
	}
	task, err := s.runtime.GetTaskInfo(ctx, row.ContainerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			out.State = "task_stopped"
			return out
		}
		return WorkspaceRuntimeStatus{State: "unknown", Message: err.Error()}
	}
	out.TaskStatus = task.Status.String()
	out.PID = task.PID
	if task.Status == ctr.TaskStatusRunning && task.PID == 0 {
		out.State = "runtime_unavailable"
		if s.overlayRuntimeUnsupported() {
			out.Message = "current runtime backend does not expose a joinable network namespace"
		} else {
			out.Message = "workspace task is running but network namespace metadata is unavailable"
		}
		return out
	}
	if task.PID > 0 {
		out.State = "netns_ready"
		out.NetNSPath = filepath.Join("/proc", strconv.FormatUint(uint64(task.PID), 10), "ns", "net")
		if s.controller != nil {
			checkReq := AttachmentRequest{
				ContainerID: out.ContainerID,
				Runtime: RuntimeNetworkRequest{
					ContainerID: out.ContainerID,
					PID:         task.PID,
					NetNSPath:   out.NetNSPath,
					CNIBinDir:   s.cniBinDir,
					CNIConfDir:  s.cniConfDir,
				},
			}
			if st, err := s.controller.Status(ctx, checkReq); err == nil {
				out.NetworkAttached = st.Runtime.Attached
			}
		}
	} else {
		out.State = "task_stopped"
	}
	return out
}

func (s *Service) StatusBot(ctx context.Context, botID string) (BotStatus, error) {
	ws := s.workspaceRuntimeStatus(ctx, botID)
	cfg, err := s.GetBotConfig(ctx, botID)
	if err != nil {
		return BotStatus{}, err
	}
	if !cfg.Enabled {
		return withWorkspace(BotStatus{
			Provider:    cfg.Provider,
			State:       "disabled",
			Title:       "Disabled",
			Description: "Bot network is disabled.",
		}, ws), nil
	}
	if strings.TrimSpace(cfg.Provider) == "" {
		return withWorkspace(BotStatus{
			State:       string(StatusStateNeedsConfig),
			Title:       "Config Required",
			Description: "Select a network provider for this bot.",
		}, ws), nil
	}
	cfg, provider, err := s.getBotProvider(ctx, botID)
	if err != nil {
		return BotStatus{}, err
	}
	providerStatus, err := provider.Status(ctx, cfg)
	if err != nil {
		return BotStatus{}, err
	}
	if providerStatus.State != StatusStateReady {
		return withWorkspace(botStatusFromProviderStatus(cfg.Provider, providerStatus), ws), nil
	}
	if s.overlayRuntimeUnsupported() {
		return withWorkspace(composeBotStatus(cfg, unsupportedOverlayStatus(cfg)), ws), nil
	}
	overlayStatus, err := s.statusOverlayBot(ctx, botID, cfg)
	if err != nil {
		if errors.Is(err, ErrWorkspaceContainerMissing) {
			return withWorkspace(BotStatus{
				Provider:    cfg.Provider,
				State:       "workspace_missing",
				Title:       "Workspace Missing",
				Description: "Bot workspace container is not created yet. Start the bot once before checking overlay status.",
			}, ws), nil
		}
		return BotStatus{}, err
	}
	return withWorkspace(composeBotStatus(cfg, overlayStatus), ws), nil
}

func (s *Service) ListBotNodes(ctx context.Context, botID string) (NodeListResponse, error) {
	cfg, provider, err := s.getBotProvider(ctx, botID)
	if err != nil {
		return NodeListResponse{}, err
	}
	items, err := provider.ListNodes(ctx, botID, cfg)
	if err != nil {
		return NodeListResponse{
			Provider: cfg.Provider,
			Message:  err.Error(),
		}, nil
	}
	return NodeListResponse{
		Provider: cfg.Provider,
		Items:    items,
	}, nil
}

func (s *Service) ExecuteActionBot(ctx context.Context, botID, actionID string, input map[string]any) (ProviderActionExecution, error) {
	cfg, provider, err := s.getBotProvider(ctx, botID)
	if err != nil {
		return ProviderActionExecution{}, err
	}
	if actionID == "logout" {
		return s.executeLogout(ctx, botID, cfg)
	}
	if actionID != "test_connection" {
		return provider.ExecuteAction(ctx, cfg, actionID, input)
	}
	providerExec, err := provider.ExecuteAction(ctx, cfg, actionID, input)
	if err != nil {
		return providerExec, err
	}
	if providerExec.Status.State != StatusStateReady {
		return providerExec, nil
	}
	if _, ensureErr := s.ensureOverlayBot(ctx, botID, cfg); ensureErr != nil && !errors.Is(ensureErr, ErrWorkspaceContainerMissing) {
		return providerExec, ensureErr
	}
	botStatus, statusErr := s.StatusBot(ctx, botID)
	if statusErr != nil {
		return providerExec, statusErr
	}
	providerExec.Status = ProviderStatus{
		State:       StatusState(botStatus.State),
		Title:       botStatus.Title,
		Description: firstNonEmptyString(botStatus.Description, botStatus.Message),
		Details:     cloneMap(botStatus.Details),
	}
	return providerExec, nil
}

// executeLogout stops and removes the overlay sidecar and then wipes the
// provider's state directory so that the next EnsureAttached triggers a fresh
// authentication flow.
func (s *Service) executeLogout(ctx context.Context, botID string, cfg BotNetworkConfig) (ProviderActionExecution, error) {
	if err := s.detachOverlayBot(ctx, botID, cfg); err != nil && !errors.Is(err, ErrWorkspaceContainerMissing) {
		return ProviderActionExecution{}, err
	}
	if s.networkStateRoot != "" && strings.TrimSpace(cfg.Provider) != "" {
		stateDir := filepath.Join(s.networkStateRoot, "network", botID, cfg.Provider)
		if err := os.RemoveAll(stateDir); err != nil {
			s.logger.Warn("failed to wipe network state directory on logout",
				slog.String("bot_id", botID),
				slog.String("provider", cfg.Provider),
				slog.Any("error", err))
		}
	}
	return ProviderActionExecution{
		ActionID: "logout",
		Status: ProviderStatus{
			State:       StatusStateNeedsConfig,
			Title:       "Logged Out",
			Description: "SD-WAN overlay has been stopped and state cleared.",
		},
	}, nil
}

func (s *Service) ReconcileBot(ctx context.Context, botID string, previous, next BotNetworkConfig) error {
	if previous.Enabled && strings.TrimSpace(previous.Provider) != "" {
		s.logger.Info("detach previous overlay", slog.String("bot_id", botID), slog.String("provider", previous.Provider))
		if err := s.detachOverlayBot(ctx, botID, previous); err != nil && !errors.Is(err, ErrWorkspaceContainerMissing) {
			return err
		}
	}
	if !next.Enabled || strings.TrimSpace(next.Provider) == "" {
		return nil
	}
	s.logger.Info("attach current overlay", slog.String("bot_id", botID), slog.String("provider", next.Provider))
	_, ensureErr := s.ensureOverlayBot(ctx, botID, next)
	if errors.Is(ensureErr, ErrWorkspaceContainerMissing) {
		s.logger.Info("skip overlay attach because workspace container is missing", slog.String("bot_id", botID))
		return nil
	}
	return ensureErr
}

func (s *Service) normalizeBotConfig(enabled bool, provider string, config map[string]any) (BotNetworkConfig, error) {
	cfg := BotNetworkConfig{
		Enabled:  enabled,
		Provider: strings.TrimSpace(provider),
		Config:   cloneMap(config),
	}
	if !cfg.Enabled {
		if cfg.Provider == "" {
			cfg.Config = map[string]any{}
		}
		return cfg, nil
	}
	if cfg.Provider == "" {
		cfg.Config = map[string]any{}
		return cfg, nil
	}
	providerImpl, err := s.requireProvider(cfg.Provider)
	if err != nil {
		return BotNetworkConfig{}, err
	}
	cfg.Config, err = providerImpl.NormalizeConfig(cfg.Config)
	if err != nil {
		return BotNetworkConfig{}, err
	}
	return cfg, nil
}

func (s *Service) getBotProvider(ctx context.Context, botID string) (BotNetworkConfig, Provider, error) {
	cfg, err := s.GetBotConfig(ctx, botID)
	if err != nil {
		return BotNetworkConfig{}, nil, err
	}
	if strings.TrimSpace(cfg.Provider) == "" {
		return BotNetworkConfig{}, nil, fmt.Errorf("network provider is not configured for bot %s", botID)
	}
	provider, err := s.requireProvider(cfg.Provider)
	if err != nil {
		return BotNetworkConfig{}, nil, err
	}
	return cfg, provider, nil
}

func (s *Service) statusOverlayBot(ctx context.Context, botID string, cfg BotNetworkConfig) (OverlayStatus, error) {
	req, err := s.buildAttachmentRequest(ctx, botID, cfg)
	if err != nil {
		return OverlayStatus{}, err
	}
	if s.overlayRuntimeUnsupported() {
		return unsupportedOverlayStatus(cfg), nil
	}
	attachmentStatus, err := s.controller.Status(ctx, req)
	if err != nil {
		return OverlayStatus{}, err
	}
	return attachmentStatus.Overlay, nil
}

func (s *Service) buildAttachmentRequest(ctx context.Context, botID string, cfg BotNetworkConfig) (AttachmentRequest, error) {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return AttachmentRequest{}, err
	}
	row, err := s.queries.GetContainerByBotID(ctx, pgBotID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AttachmentRequest{}, ErrWorkspaceContainerMissing
		}
		return AttachmentRequest{}, err
	}
	req := AttachmentRequest{
		BotID:       botID,
		ContainerID: row.ContainerID,
		Runtime: RuntimeNetworkRequest{
			ContainerID: row.ContainerID,
			CNIBinDir:   s.cniBinDir,
			CNIConfDir:  s.cniConfDir,
		},
		Overlay: cfg,
	}
	if s.runtime == nil {
		return req, nil
	}
	task, err := s.runtime.GetTaskInfo(ctx, row.ContainerID)
	if err == nil && task.PID > 0 {
		req.Runtime.PID = task.PID
		req.Runtime.NetNSPath = filepath.Join("/proc", strconv.FormatUint(uint64(task.PID), 10), "ns", "net")
		return req, nil
	}
	if err != nil && !errdefs.IsNotFound(err) {
		return AttachmentRequest{}, err
	}
	return req, nil
}

func composeBotStatus(cfg BotNetworkConfig, overlay OverlayStatus) BotStatus {
	status := BotStatus{
		Provider:     cfg.Provider,
		Attached:     overlay.Attached,
		State:        overlay.State,
		Message:      overlay.Message,
		NetworkIP:    overlay.NetworkIP,
		ProxyAddress: overlay.ProxyAddress,
		Details:      cloneMap(overlay.Details),
	}
	switch overlay.State {
	case "ready":
		status.Title = "Connected"
		status.Description = "SD-WAN overlay is running and attached."
	case "needs_login":
		status.Title = "Login Required"
		status.Description = "Open the authentication URL to complete login."
	case "starting":
		status.Title = "Starting"
		status.Description = firstNonEmptyString(overlay.Message, "SD-WAN overlay is starting.")
	case "degraded":
		status.Title = "Degraded"
		status.Description = firstNonEmptyString(overlay.Message, "SD-WAN overlay is running but reported health issues.")
	case "stopped":
		status.Title = "Stopped"
		status.Description = "SD-WAN overlay exists but is not running."
	case "missing":
		status.Title = "Not Attached"
		status.Description = "SD-WAN overlay is not attached yet. Start the bot workspace first."
	case "unsupported":
		status.Title = "Unsupported"
		status.Description = firstNonEmptyString(overlay.Message, "Provider-backed network is not supported by the current runtime backend.")
	case "disabled":
		status.Title = "Disabled"
		status.Description = "Bot network is disabled."
	default:
		status.Title = strings.ToUpper(firstNonEmptyString(overlay.State, "unknown"))
		status.Description = firstNonEmptyString(overlay.Message, "Overlay status is unavailable.")
	}
	return status
}

func (s *Service) ensureOverlayBot(ctx context.Context, botID string, cfg BotNetworkConfig) (OverlayStatus, error) {
	req, err := s.buildAttachmentRequest(ctx, botID, cfg)
	if err != nil {
		return OverlayStatus{}, err
	}
	if s.overlayRuntimeUnsupported() {
		s.logger.Info("skip overlay ensure because current runtime backend does not support provider sidecars", slog.String("bot_id", botID), slog.String("provider", cfg.Provider), slog.String("runtime", s.runtimeKind))
		return unsupportedOverlayStatus(cfg), nil
	}
	if strings.TrimSpace(req.Runtime.NetNSPath) == "" {
		s.logger.Info("skip overlay ensure because workspace task is not running", slog.String("bot_id", botID), slog.String("provider", cfg.Provider))
		attachmentStatus, statusErr := s.controller.Status(ctx, req)
		if statusErr != nil {
			return OverlayStatus{}, statusErr
		}
		return attachmentStatus.Overlay, nil
	}
	req.OverlayOnly = true
	attachmentStatus, err := s.controller.EnsureAttached(ctx, req)
	if err != nil {
		return OverlayStatus{}, err
	}
	return attachmentStatus.Overlay, nil
}

func (s *Service) detachOverlayBot(ctx context.Context, botID string, cfg BotNetworkConfig) error {
	req, err := s.buildAttachmentRequest(ctx, botID, cfg)
	if err != nil {
		return err
	}
	req.OverlayOnly = true
	return s.controller.Detach(ctx, req)
}

func (s *Service) overlayRuntimeUnsupported() bool {
	return s.runtimeKind == "apple"
}

func unsupportedOverlayStatus(cfg BotNetworkConfig) OverlayStatus {
	return OverlayStatus{
		Provider: cfg.Provider,
		State:    "unsupported",
		Message:  "Provider-backed network sidecars are not supported on the current runtime backend.",
	}
}

func botStatusFromProviderStatus(provider string, status ProviderStatus) BotStatus {
	return BotStatus{
		Provider:    provider,
		State:       string(status.State),
		Title:       status.Title,
		Description: status.Description,
		Details:     cloneMap(status.Details),
	}
}

func (s *Service) requireProvider(kind string) (Provider, error) {
	if s.registry == nil {
		return nil, errors.New("network provider registry is not configured")
	}
	provider, ok := s.registry.Get(kind)
	if !ok {
		return nil, fmt.Errorf("unsupported network provider: %s", kind)
	}
	return provider, nil
}

func decodeJSONMap(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}
