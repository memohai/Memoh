package workspace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/containerd"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/identity"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	BotLabelKey                 = "memoh.bot_id"
	WorkspaceLabelKey           = "memoh.workspace"
	WorkspaceLabelValue         = "v3"
	WorkspaceCDIDevicesLabelKey = "memoh.workspace.cdi_devices"
	ContainerPrefix             = "workspace-"
	LegacyContainerPrefix       = "mcp-"

	legacyGRPCPort = 9090
)

// ErrContainerNotFound is returned when no container exists for a bot.
var ErrContainerNotFound = errors.New("container not found for bot")

// ContainerStatus combines DB records with live containerd state.
type ContainerStatus struct {
	ContainerID      string    `json:"container_id"`
	Image            string    `json:"image"`
	Status           string    `json:"status"`
	Namespace        string    `json:"namespace"`
	ContainerPath    string    `json:"container_path"`
	CDIDevices       []string  `json:"cdi_devices,omitempty"`
	TaskRunning      bool      `json:"task_running"`
	HasPreservedData bool      `json:"has_preserved_data"`
	Legacy           bool      `json:"legacy"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Manager struct {
	service         ctr.Service
	cfg             config.WorkspaceConfig
	namespace       string
	db              *pgxpool.Pool
	queries         *dbsqlc.Queries
	logger          *slog.Logger
	containerLockMu sync.Mutex
	containerLocks  map[string]*sync.Mutex
	grpcPool        *bridge.Pool
	legacyMu        sync.RWMutex
	legacyIPs       map[string]string // botID → IP for pre-bridge containers
}

func NewManager(log *slog.Logger, service ctr.Service, cfg config.WorkspaceConfig, namespace string, conn *pgxpool.Pool) *Manager {
	if namespace == "" {
		namespace = config.DefaultNamespace
	}
	m := &Manager{
		service:        service,
		cfg:            cfg,
		namespace:      namespace,
		db:             conn,
		queries:        dbsqlc.New(conn),
		logger:         log.With(slog.String("component", "workspace")),
		containerLocks: make(map[string]*sync.Mutex),
		legacyIPs:      make(map[string]string),
	}
	m.grpcPool = bridge.NewPool(m.dialTarget)
	return m
}

// resolveContainerID resolves the actual containerd container ID for a bot.
// This is the SINGLE point of container ID resolution for all lookup operations.
// It delegates to ContainerID (DB → label → scan) and falls back to the
// new-style prefix if no container exists yet.
func (m *Manager) resolveContainerID(ctx context.Context, botID string) string {
	id, err := m.ContainerID(ctx, botID)
	if err != nil {
		return ContainerPrefix + botID
	}
	return id
}

func (m *Manager) lockContainer(containerID string) func() {
	m.containerLockMu.Lock()
	lock, ok := m.containerLocks[containerID]
	if !ok {
		lock = &sync.Mutex{}
		m.containerLocks[containerID] = lock
	}
	m.containerLockMu.Unlock()

	lock.Lock()
	return lock.Unlock
}

// socketDir returns the host-side directory that is bind-mounted into the
// container at /run/memoh, holding the UDS socket file.
func (m *Manager) socketDir(botID string) string {
	return filepath.Join(m.dataRoot(), "run", botID)
}

// socketPath returns the path to the UDS socket file for a bot's container.
func (m *Manager) socketPath(botID string) string {
	return filepath.Join(m.socketDir(botID), "bridge.sock")
}

// dialTarget returns the gRPC dial target for a bot. Legacy containers
// (pre-bridge) are reached via TCP; bridge containers use UDS.
func (m *Manager) dialTarget(botID string) string {
	m.legacyMu.RLock()
	ip, legacy := m.legacyIPs[botID]
	m.legacyMu.RUnlock()
	if legacy {
		return fmt.Sprintf("%s:%d", ip, legacyGRPCPort)
	}
	return "unix://" + m.socketPath(botID)
}

// SetLegacyIP records the IP address of a legacy (pre-bridge) container
// so the gRPC pool can reach it via TCP.
func (m *Manager) SetLegacyIP(botID, ip string) {
	m.legacyMu.Lock()
	m.legacyIPs[botID] = ip
	m.legacyMu.Unlock()
}

// ClearLegacyIP removes a cached legacy IP (e.g. when the container is deleted).
func (m *Manager) ClearLegacyIP(botID string) {
	m.legacyMu.Lock()
	delete(m.legacyIPs, botID)
	m.legacyMu.Unlock()
}

// clearLegacyRoute evicts any stale TCP fallback state for a bot so future
// gRPC dials use the bridge container's Unix socket.
func (m *Manager) clearLegacyRoute(botID string) {
	m.ClearLegacyIP(botID)
	m.grpcPool.Remove(botID)
}

// MCPClient returns a gRPC client for the given bot's container.
// Implements bridge.Provider.
func (m *Manager) MCPClient(ctx context.Context, botID string) (*bridge.Client, error) {
	return m.grpcPool.Get(ctx, botID)
}

func (m *Manager) Init(ctx context.Context) error {
	image := m.imageRef()

	// Pre-pull the default base image so container creation doesn't block
	// on a network download. If the image is already present, this is a no-op.
	if _, err := m.service.GetImage(ctx, image); err != nil {
		m.logger.Info("pulling base image for workspace containers", slog.String("image", image))
		if _, pullErr := m.service.PullImage(ctx, image, &ctr.PullImageOptions{
			Unpack:      true,
			Snapshotter: m.cfg.Snapshotter,
		}); pullErr != nil {
			m.logger.Warn("base image pull failed", slog.String("image", image), slog.Any("error", pullErr))
			return pullErr
		}
	}
	return nil
}

// EnsureBot creates the workspace container for a bot if it does not exist.
// Bot data lives in the container's writable layer (snapshot), not bind mounts.
// The Memoh runtime (bridge binary + toolkit) is injected via read-only bind mount.
// If imageOverride is non-empty, it is used instead of the configured default.
func (m *Manager) EnsureBot(ctx context.Context, botID, imageOverride string) error {
	image := m.imageRef()
	if imageOverride != "" {
		image = config.NormalizeImageRef(imageOverride)
	}
	gpu, err := m.resolveWorkspaceGPU(ctx, botID)
	if err != nil {
		return err
	}
	return m.ensureBotWithImage(ctx, botID, image, gpu)
}

func workspaceCDIDevicesLabelValue(devices []string) string {
	devices = normalizeWorkspaceGPUDevices(devices)
	return strings.Join(devices, ",")
}

func workspaceCDIDevicesFromLabels(labels map[string]string) []string {
	if len(labels) == 0 {
		return nil
	}
	value := strings.TrimSpace(labels[WorkspaceCDIDevicesLabelKey])
	if value == "" {
		return nil
	}
	return normalizeWorkspaceGPUDevices(strings.Split(value, ","))
}

func (m *Manager) buildWorkspaceContainerSpec(botID string, gpu WorkspaceGPUConfig) (ctr.ContainerSpec, error) {
	resolvPath, err := ctr.ResolveConfSource(m.dataRoot())
	if err != nil {
		return ctr.ContainerSpec{}, err
	}

	runtimeDir := m.cfg.RuntimePath()
	sockDir := m.socketDir(botID)
	if err := os.MkdirAll(sockDir, 0o750); err != nil {
		return ctr.ContainerSpec{}, fmt.Errorf("create socket dir: %w", err)
	}

	mounts := []ctr.MountSpec{
		{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      resolvPath,
			Options:     []string{"rbind", "ro"},
		},
		{
			Destination: "/opt/memoh",
			Type:        "bind",
			Source:      runtimeDir,
			Options:     []string{"rbind", "ro"},
		},
		{
			Destination: "/run/memoh",
			Type:        "bind",
			Source:      sockDir,
			Options:     []string{"rbind", "rw"},
		},
	}
	tzMounts, tzEnv := ctr.TimezoneSpec()
	mounts = append(mounts, tzMounts...)

	env := make([]string, 0, len(tzEnv)+1)
	env = append(env, tzEnv...)
	env = append(env, "BRIDGE_SOCKET_PATH=/run/memoh/bridge.sock")

	return ctr.ContainerSpec{
		Cmd:        []string{"/opt/memoh/bridge"},
		Mounts:     mounts,
		Env:        env,
		CDIDevices: normalizeWorkspaceGPUDevices(gpu.Devices),
	}, nil
}

func (m *Manager) ensureBotWithImage(ctx context.Context, botID, image string, gpu WorkspaceGPUConfig) error {
	if err := validateBotID(botID); err != nil {
		return err
	}
	spec, err := m.buildWorkspaceContainerSpec(botID, gpu)
	if err != nil {
		return err
	}

	labels := map[string]string{
		BotLabelKey:       botID,
		WorkspaceLabelKey: WorkspaceLabelValue,
	}
	if value := workspaceCDIDevicesLabelValue(gpu.Devices); value != "" {
		labels[WorkspaceCDIDevicesLabelKey] = value
	}

	_, err = m.service.CreateContainer(ctx, ctr.CreateContainerRequest{
		ID:          ContainerPrefix + botID,
		ImageRef:    image,
		Snapshotter: m.cfg.Snapshotter,
		Labels:      labels,
		Spec:        spec,
	})
	if err == nil {
		return nil
	}

	if !errdefs.IsAlreadyExists(err) {
		return err
	}

	return nil
}

// ListBots returns the bot IDs that have workspace containers.
func (m *Manager) ListBots(ctx context.Context) ([]string, error) {
	containers, err := m.service.ListContainers(ctx)
	if err != nil {
		return nil, err
	}

	botIDs := make([]string, 0, len(containers))
	for _, info := range containers {
		if botID, ok := BotIDFromContainerInfo(info); ok {
			botIDs = append(botIDs, botID)
		}
	}
	return botIDs, nil
}

func (m *Manager) Start(ctx context.Context, botID string) error {
	image, err := m.resolveWorkspaceImage(ctx, botID)
	if err != nil {
		return err
	}
	gpu, err := m.resolveWorkspaceGPU(ctx, botID)
	if err != nil {
		return err
	}
	return m.startWithResolvedConfig(ctx, botID, image, gpu)
}

// StartWithImage creates and starts the MCP container for a bot.
// If imageOverride is non-empty, it is used as the base image instead of the
// configured default. The override only applies when creating a new container.
func (m *Manager) StartWithImage(ctx context.Context, botID, imageOverride string) error {
	image := strings.TrimSpace(imageOverride)
	if image == "" {
		return m.Start(ctx, botID)
	}
	gpu, err := m.resolveWorkspaceGPU(ctx, botID)
	if err != nil {
		return err
	}
	return m.startWithResolvedConfig(ctx, botID, config.NormalizeImageRef(image), gpu)
}

// StartWithResolvedImage creates and starts the workspace container for a bot
// using an explicit image reference.
func (m *Manager) StartWithResolvedImage(ctx context.Context, botID, image string) error {
	image = strings.TrimSpace(image)
	if image == "" {
		return errors.New("image is required")
	}
	gpu, err := m.resolveWorkspaceGPU(ctx, botID)
	if err != nil {
		return err
	}
	return m.startWithResolvedConfig(ctx, botID, image, gpu)
}

func (m *Manager) StartWithResolvedConfig(ctx context.Context, botID, image string, gpu WorkspaceGPUConfig) error {
	image = strings.TrimSpace(image)
	if image == "" {
		return errors.New("image is required")
	}
	return m.startWithResolvedConfig(ctx, botID, image, gpu)
}

func (m *Manager) startWithResolvedConfig(ctx context.Context, botID, image string, gpu WorkspaceGPUConfig) error {
	containerID := m.resolveContainerID(ctx, botID)

	// Before creating a new container, check for an orphaned snapshot
	// (container deleted but snapshot with /data survived). Export /data
	// to a backup so it can be restored after EnsureBot creates a fresh
	// container. This covers dev image rebuilds, containerd metadata loss,
	// and manual container deletion.
	if _, err := m.service.GetContainer(ctx, containerID); errdefs.IsNotFound(err) {
		m.recoverOrphanedSnapshot(ctx, botID)
	}

	if err := m.ensureBotWithImage(ctx, botID, image, gpu); err != nil {
		return err
	}

	// Restore preserved data (from orphaned snapshot recovery or a previous
	// CleanupBotContainer with preserveData) into the fresh snapshot before
	// starting the task, avoiding a redundant stop/start cycle.
	if m.HasPreservedData(botID) {
		if err := m.restorePreservedIntoSnapshot(ctx, botID); err != nil {
			return fmt.Errorf("restore preserved data: %w", err)
		}
	}

	if err := m.service.StartContainer(ctx, containerID, nil); err != nil {
		return err
	}

	// CNI network setup (for outbound connectivity — container processes
	// may need to download packages). Server communicates via UDS, not IP.
	if _, err := m.service.SetupNetwork(ctx, ctr.NetworkSetupRequest{
		ContainerID: containerID,
		CNIBinDir:   m.cfg.CNIBinaryDir,
		CNIConfDir:  m.cfg.CNIConfigDir,
	}); err != nil {
		if stopErr := m.service.StopContainer(ctx, containerID, &ctr.StopTaskOptions{Force: true}); stopErr != nil {
			m.logger.Warn("cleanup: stop task failed", slog.String("container_id", containerID), slog.Any("error", stopErr))
		}
		return err
	}
	if !m.IsLegacyContainer(ctx, containerID) {
		m.clearLegacyRoute(botID)
	}
	return nil
}

func (m *Manager) Stop(ctx context.Context, botID string, timeout time.Duration) error {
	if err := validateBotID(botID); err != nil {
		return err
	}
	return m.service.StopContainer(ctx, m.resolveContainerID(ctx, botID), &ctr.StopTaskOptions{
		Timeout: timeout,
		Force:   true,
	})
}

func (m *Manager) Delete(ctx context.Context, botID string, preserveData bool) error {
	if err := validateBotID(botID); err != nil {
		return err
	}

	containerID := m.resolveContainerID(ctx, botID)

	stoppedForPreserve := false

	if preserveData {
		info, err := m.service.GetContainer(ctx, containerID)
		if err != nil {
			return fmt.Errorf("get container for preserve: %w", err)
		}

		if _, err := m.snapshotMounts(ctx, info); errors.Is(err, errMountNotSupported) {
			// Apple backend fallback uses gRPC against a running container.
		} else if err != nil {
			return err
		} else {
			if err := m.safeStopTask(ctx, containerID); err != nil {
				return fmt.Errorf("stop for data preserve: %w", err)
			}
			stoppedForPreserve = true
		}

		if err := m.PreserveData(ctx, botID); err != nil {
			// Export failed — restart only if we stopped the task, and abort
			// deletion to prevent data loss.
			if stoppedForPreserve {
				m.restartContainer(ctx, botID, containerID)
			}
			return fmt.Errorf("preserve data: %w", err)
		}
	}

	m.clearLegacyRoute(botID)

	if err := m.service.RemoveNetwork(ctx, ctr.NetworkSetupRequest{
		ContainerID: containerID,
		CNIBinDir:   m.cfg.CNIBinaryDir,
		CNIConfDir:  m.cfg.CNIConfigDir,
	}); err != nil {
		m.logger.Warn("delete: remove network failed",
			slog.String("container_id", containerID), slog.Any("error", err))
	}
	if err := m.service.DeleteTask(ctx, containerID, &ctr.DeleteTaskOptions{Force: true}); err != nil {
		m.logger.Warn("delete: delete task failed",
			slog.String("container_id", containerID), slog.Any("error", err))
	}
	return m.service.DeleteContainer(ctx, containerID, &ctr.DeleteContainerOptions{
		CleanupSnapshot: true,
	})
}

func (m *Manager) dataRoot() string {
	if m.cfg.DataRoot == "" {
		return config.DefaultDataRoot
	}
	return m.cfg.DataRoot
}

func (m *Manager) imageRef() string {
	return m.cfg.ImageRef()
}

// IsLegacyContainer returns true if the container was created before the
// bridge runtime injection architecture (uses the legacy "mcp-" prefix).
// Legacy containers are functional but unreachable from the server (they
// use TCP gRPC instead of UDS). Users should delete and recreate them.
func (*Manager) IsLegacyContainer(_ context.Context, containerID string) bool {
	return strings.HasPrefix(containerID, LegacyContainerPrefix)
}

func validateBotID(botID string) error {
	return identity.ValidateChannelIdentityID(botID)
}
