package workspace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/container"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/identity"
	netctl "github.com/memohai/memoh/internal/network"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

const (
	BotLabelKey                 = "memoh.bot_id"
	WorkspaceLabelKey           = "memoh.workspace"
	WorkspaceLabelValue         = "v3"
	WorkspaceCDIDevicesLabelKey = "memoh.workspace.cdi_devices"
	ContainerPrefix             = "workspace-"
	LegacyContainerPrefix       = "mcp-"
	DisplayRFBSocketName        = "display.rfb.sock"

	legacyGRPCPort = 9090
)

// ErrContainerNotFound is returned when no container exists for a bot.
var ErrContainerNotFound = errors.New("container not found for bot")

// ContainerStatus combines DB records with live containerd state.
type ContainerStatus struct {
	ContainerID      string    `json:"container_id"`
	WorkspaceBackend string    `json:"workspace_backend"`
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

type ContainerMetricsStatus struct {
	Exists      bool `json:"exists"`
	TaskRunning bool `json:"task_running"`
}

type ContainerStorageMetrics struct {
	Path      string `json:"path"`
	UsedBytes uint64 `json:"used_bytes"`
}

type ContainerMetricsResult struct {
	Supported         bool
	UnsupportedReason string
	Status            ContainerMetricsStatus
	SampledAt         time.Time
	CPU               *ctr.CPUMetrics
	Memory            *ctr.MemoryMetrics
	Storage           *ContainerStorageMetrics
}

type Manager struct {
	service           runtimeService
	networkController netctl.Controller
	cfg               config.WorkspaceConfig
	namespace         string
	db                *pgxpool.Pool
	queries           dbstore.Queries
	logger            *slog.Logger
	containerLockMu   sync.Mutex
	containerLocks    map[string]*sync.Mutex
	grpcPool          *bridge.Pool
	legacyMu          sync.RWMutex
	legacyIPs         map[string]string // botID → IP for pre-bridge containers
}

type WorkspaceStartConfig struct {
	Backend            string
	LocalWorkspacePath string
}

func NewManager(log *slog.Logger, service runtimeService, networkController netctl.Controller, cfg config.WorkspaceConfig, namespace string, conn *pgxpool.Pool, queryOverride ...dbstore.Queries) *Manager {
	if namespace == "" {
		namespace = config.DefaultNamespace
	}
	var queries dbstore.Queries
	if len(queryOverride) > 0 {
		queries = queryOverride[0]
	} else if conn != nil {
		queries = postgresstore.NewQueries(dbsqlc.New(conn))
	}
	m := &Manager{
		service:           service,
		networkController: networkController,
		cfg:               cfg,
		namespace:         namespace,
		db:                conn,
		queries:           queries,
		logger:            log.With(slog.String("component", "workspace")),
		containerLocks:    make(map[string]*sync.Mutex),
		legacyIPs:         make(map[string]string),
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

// DisplaySocketPath returns the host-side path to the workspace display RFB
// Unix socket. The directory is mounted into the container at /run/memoh.
func (m *Manager) DisplaySocketPath(botID string) string {
	return filepath.Join(m.socketDir(botID), DisplayRFBSocketName)
}

// dialTarget returns the gRPC dial target for a bot. Legacy containers
// (pre-bridge) are reached via TCP; bridge containers use UDS.
func (m *Manager) dialTarget(botID string) string {
	if targeter, ok := m.service.(interface{ BridgeTarget(string) string }); ok {
		if target := strings.TrimSpace(targeter.BridgeTarget(botID)); target != "" {
			return target
		}
	}
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
	if provider, ok := m.service.(bridge.Provider); ok {
		client, err := provider.MCPClient(ctx, botID)
		if err == nil {
			return client, nil
		}
		if !errors.Is(err, ctr.ErrNotSupported) && !ctr.IsNotFound(err) {
			return nil, err
		}
	}
	return m.grpcPool.Get(ctx, botID)
}

func (m *Manager) WorkspaceInfo(ctx context.Context, botID string) (bridge.WorkspaceInfo, error) {
	if provider, ok := m.service.(bridge.WorkspaceInfoProvider); ok {
		info, err := provider.WorkspaceInfo(ctx, botID)
		if err == nil {
			return info, nil
		}
		if !errors.Is(err, ctr.ErrNotSupported) && !ctr.IsNotFound(err) {
			return bridge.WorkspaceInfo{}, err
		}
	}
	return bridge.WorkspaceInfo{
		Backend:        bridge.WorkspaceBackendContainer,
		DefaultWorkDir: config.DefaultDataMount,
	}, nil
}

func (m *Manager) Init(ctx context.Context) error {
	image := m.imageRef()
	result, err := m.PrepareImageForCreate(ctx, image, &ctr.PullImageOptions{
		Unpack:        true,
		StorageDriver: m.cfg.Snapshotter,
	})
	if err != nil {
		m.logger.Warn("base image preparation failed", slog.String("image", image), slog.Any("error", err))
		return err
	}
	if result.Mode == ImagePrepareDelegated {
		m.logger.Info("base image pull delegated to container backend", slog.String("image", image))
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

func (m *Manager) buildWorkspaceContainerSpec(ctx context.Context, botID string, gpu WorkspaceGPUConfig) (ctr.ContainerSpec, error) {
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

	skillRoots, err := m.ResolveWorkspaceSkillDiscoveryRoots(ctx, botID)
	if err != nil {
		return ctr.ContainerSpec{}, err
	}
	skillEnv := skillset.ContainerEnv(skillRoots)
	env := make([]string, 0, len(tzEnv)+1+len(skillEnv))
	env = append(env, tzEnv...)
	env = append(env, "BRIDGE_SOCKET_PATH=/run/memoh/bridge.sock")
	if m.botDisplayEnabled(ctx, botID) {
		env = append(env,
			"MEMOH_DISPLAY_ENABLED=true",
			"MEMOH_DISPLAY_RFB_TCP_ADDR=127.0.0.1:5999",
			"DISPLAY=:99",
		)
	}
	env = append(env, skillEnv...)

	return ctr.ContainerSpec{
		Cmd:        []string{"/opt/memoh/bridge"},
		Mounts:     mounts,
		Env:        env,
		CDIDevices: normalizeWorkspaceGPUDevices(gpu.Devices),
	}, nil
}

func (m *Manager) botDisplayEnabled(ctx context.Context, botID string) bool {
	if m.queries == nil {
		return false
	}
	id, err := db.ParseUUID(botID)
	if err != nil {
		return false
	}
	row, err := m.queries.GetSettingsByBotID(ctx, id)
	if err != nil {
		return false
	}
	return row.DisplayEnabled
}

func (m *Manager) BotDisplayEnabled(ctx context.Context, botID string) bool {
	return m.botDisplayEnabled(ctx, botID)
}

func (m *Manager) DisplayDialContext(ctx context.Context, botID, network, address string) (net.Conn, error) {
	client, err := m.MCPClient(ctx, botID)
	if err != nil {
		return nil, err
	}
	return client.DialContext(ctx, network, address)
}

func (m *Manager) ensureBotWithImage(ctx context.Context, botID, image string, gpu WorkspaceGPUConfig) error {
	if err := validateBotID(botID); err != nil {
		return err
	}
	spec, err := m.buildWorkspaceContainerSpec(ctx, botID, gpu)
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
		ID:              ContainerPrefix + botID,
		ImageRef:        image,
		ImagePullPolicy: m.cfg.EffectiveImagePullPolicy(),
		StorageRef:      ctr.StorageRef{Driver: m.cfg.Snapshotter, Kind: "active"},
		Labels:          labels,
		Spec:            spec,
	})
	if err == nil {
		return nil
	}

	if !ctr.IsAlreadyExists(err) {
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

func (m *Manager) StartWithWorkspaceConfig(ctx context.Context, botID, image string, gpu WorkspaceGPUConfig, workspaceCfg WorkspaceStartConfig) error {
	switch strings.ToLower(strings.TrimSpace(workspaceCfg.Backend)) {
	case "", bridge.WorkspaceBackendContainer:
		return m.StartWithResolvedConfig(ctx, botID, image, gpu)
	case bridge.WorkspaceBackendLocal:
		return m.startWithLocalConfig(ctx, botID, image, workspaceCfg.LocalWorkspacePath)
	default:
		return fmt.Errorf("unsupported workspace backend %q", workspaceCfg.Backend)
	}
}

func (m *Manager) startWithLocalConfig(ctx context.Context, botID, image, workspacePath string) error {
	if err := validateBotID(botID); err != nil {
		return err
	}
	if checker, ok := m.service.(interface{ LocalEnabled() bool }); !ok || !checker.LocalEnabled() {
		return ctr.ErrNotSupported
	}
	containerID := LocalContainerPrefix + botID
	path := strings.TrimSpace(workspacePath)
	if path == "" {
		path = m.defaultLocalWorkspacePath(ctx, botID)
	}
	labels := map[string]string{
		BotLabelKey:       botID,
		WorkspaceLabelKey: WorkspaceLabelValue,
	}
	if strings.TrimSpace(image) == "" {
		image = "local"
	}
	if _, err := m.service.CreateContainer(ctx, ctr.CreateContainerRequest{
		ID:              containerID,
		ImageRef:        image,
		ImagePullPolicy: config.ImagePullPolicyNever,
		StorageRef:      ctr.StorageRef{Driver: localRuntimeName, Key: path, Kind: "directory"},
		Labels:          labels,
	}); err != nil && !ctr.IsAlreadyExists(err) {
		return err
	}
	if err := m.startTaskAndEnsureNetwork(ctx, botID, containerID); err != nil {
		return err
	}
	m.upsertContainerRecord(ctx, botID, containerID, "running", image)
	return nil
}

func (m *Manager) startWithResolvedConfig(ctx context.Context, botID, image string, gpu WorkspaceGPUConfig) error {
	containerID := m.resolveContainerID(ctx, botID)

	// Before creating a new container, check for an orphaned snapshot
	// (container deleted but snapshot with /data survived). Export /data
	// to a backup so it can be restored after EnsureBot creates a fresh
	// container. This covers dev image rebuilds, containerd metadata loss,
	// and manual container deletion.
	if _, err := m.service.GetContainer(ctx, containerID); ctr.IsNotFound(err) {
		m.recoverOrphanedSnapshot(ctx, botID)
	}

	if err := m.ensureBotWithImage(ctx, botID, image, gpu); err != nil {
		return err
	}

	// Restore preserved data (from orphaned snapshot recovery or a previous
	// CleanupBotContainer with preserveData) into the fresh snapshot before
	// starting the task when the backend exposes snapshot mounts. Backends
	// without mount support restore through the bridge after the task starts.
	restoreAfterStart := false
	if m.HasPreservedData(botID) {
		if err := m.restorePreservedIntoSnapshot(ctx, botID); err != nil {
			if errors.Is(err, errMountNotSupported) {
				restoreAfterStart = true
			} else {
				return fmt.Errorf("restore preserved data: %w", err)
			}
		}
	}

	// Start the task and restore the container network so workspace processes
	// regain outbound connectivity. Server communication still uses UDS.
	if err := m.startTaskAndEnsureNetwork(ctx, botID, containerID); err != nil {
		if stopErr := m.service.StopContainer(ctx, containerID, &ctr.StopTaskOptions{Force: true}); stopErr != nil {
			m.logger.Warn("cleanup: stop task failed", slog.String("container_id", containerID), slog.Any("error", stopErr))
		}
		return err
	}
	if restoreAfterStart {
		if err := m.RestorePreservedData(ctx, botID); err != nil {
			return fmt.Errorf("restore preserved data through bridge: %w", err)
		}
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

	if preserveData {
		if err := m.preserveDataBeforeDelete(ctx, botID); err != nil {
			return fmt.Errorf("preserve data: %w", err)
		}
	}

	m.clearLegacyRoute(botID)

	if err := m.removeContainerNetwork(ctx, botID, containerID); err != nil {
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
	return m.cfg.DataRootPath()
}

func (m *Manager) imageRef() string {
	return m.cfg.ImageRef()
}

func (m *Manager) defaultLocalWorkspacePath(ctx context.Context, botID string) string {
	displayName := botID
	if m.queries != nil {
		if pgBotID, err := db.ParseUUID(botID); err == nil {
			if row, err := m.queries.GetBotByID(ctx, pgBotID); err == nil && row.DisplayName.Valid && strings.TrimSpace(row.DisplayName.String) != "" {
				displayName = row.DisplayName.String
			}
		}
	}
	if resolver, ok := m.service.(interface {
		DefaultLocalWorkspacePath(string, string) string
	}); ok {
		if path := strings.TrimSpace(resolver.DefaultLocalWorkspacePath(botID, displayName)); path != "" {
			return path
		}
	}
	return filepath.Join(config.LocalConfig{}.WorkspaceParent(), displayName)
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
