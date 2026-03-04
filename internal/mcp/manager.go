package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/containerd"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/identity"
	"github.com/memohai/memoh/internal/mcp/mcpclient"
)

const (
	BotLabelKey     = "mcp.bot_id"
	ContainerPrefix = "mcp-"
)

type Manager struct {
	service         ctr.Service
	cfg             config.MCPConfig
	namespace       string
	containerID     func(string) string
	db              *pgxpool.Pool
	queries         *dbsqlc.Queries
	logger          *slog.Logger
	containerLockMu sync.Mutex
	containerLocks  map[string]*sync.Mutex
	mu              sync.RWMutex
	containerIPs    map[string]string
	grpcPool        *mcpclient.Pool
}

func NewManager(log *slog.Logger, service ctr.Service, cfg config.MCPConfig, namespace string, conn *pgxpool.Pool) *Manager {
	if namespace == "" {
		namespace = config.DefaultNamespace
	}
	m := &Manager{
		service:        service,
		cfg:            cfg,
		namespace:      namespace,
		db:             conn,
		queries:        dbsqlc.New(conn),
		logger:         log.With(slog.String("component", "mcp")),
		containerLocks: make(map[string]*sync.Mutex),
		containerIPs:   make(map[string]string),
		containerID: func(botID string) string {
			return ContainerPrefix + botID
		},
	}
	m.grpcPool = mcpclient.NewPool(m.ContainerIP)
	return m
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

// ContainerIP returns the cached IP address for a bot's container.
// If not cached, it attempts to recover the IP by re-running CNI setup.
func (m *Manager) ContainerIP(botID string) string {
	m.mu.RLock()
	if ip, ok := m.containerIPs[botID]; ok {
		m.mu.RUnlock()
		return ip
	}
	m.mu.RUnlock()

	// Cache miss - try to recover IP via CNI setup (idempotent)
	ip, err := m.recoverContainerIP(botID)
	if err != nil {
		m.logger.Warn("container IP recovery failed", slog.String("bot_id", botID), slog.Any("error", err))
		return ""
	}
	if ip != "" {
		m.mu.Lock()
		m.containerIPs[botID] = ip
		m.mu.Unlock()
		m.logger.Info("container IP recovered", slog.String("bot_id", botID), slog.String("ip", ip))
	}
	return ip
}

// SetContainerIP stores the container IP in the cache.
// If the IP changed, the stale gRPC connection is evicted from the pool.
func (m *Manager) SetContainerIP(botID, ip string) {
	if ip == "" {
		return
	}
	m.mu.Lock()
	old := m.containerIPs[botID]
	m.containerIPs[botID] = ip
	m.mu.Unlock()

	if old != "" && old != ip {
		m.grpcPool.Remove(botID)
		m.logger.Info("evicted stale gRPC connection", slog.String("bot_id", botID), slog.String("old_ip", old), slog.String("new_ip", ip))
	}
}

// recoverContainerIP attempts to restore the container IP by re-running CNI setup.
// CNI plugins are idempotent - calling Setup again returns the existing IP allocation.
func (m *Manager) recoverContainerIP(botID string) (string, error) {
	ctx := context.Background()
	containerID := m.containerID(botID)

	// First check if container exists and get basic info
	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		return "", err
	}

	// Check if IP is stored in labels (if we ever add label persistence)
	if ip, ok := info.Labels["mcp.container_ip"]; ok {
		return ip, nil
	}

	// Container exists but IP not cached - need to re-setup network to get IP
	// This happens after server restart when in-memory cache is lost
	netResult, err := m.service.SetupNetwork(ctx, ctr.NetworkSetupRequest{
		ContainerID: containerID,
		CNIBinDir:   m.cfg.CNIBinaryDir,
		CNIConfDir:  m.cfg.CNIConfigDir,
	})
	if err != nil {
		return "", fmt.Errorf("network setup for IP recovery: %w", err)
	}

	return netResult.IP, nil
}

// MCPClient returns a gRPC client for the given bot's container.
// Implements mcpclient.Provider.
func (m *Manager) MCPClient(ctx context.Context, botID string) (*mcpclient.Client, error) {
	return m.grpcPool.Get(ctx, botID)
}

func (m *Manager) Init(ctx context.Context) error {
	image := m.imageRef()

	if _, err := m.service.GetImage(ctx, image); err == nil {
		return nil
	}

	_, err := m.service.PullImage(ctx, image, &ctr.PullImageOptions{
		Unpack:      true,
		Snapshotter: m.cfg.Snapshotter,
	})
	return err
}

// EnsureBot creates the MCP container for a bot if it does not exist.
// Bot data lives in the container's writable layer (snapshot), not bind mounts.
func (m *Manager) EnsureBot(ctx context.Context, botID string) error {
	if err := validateBotID(botID); err != nil {
		return err
	}

	image := m.imageRef()
	resolvPath, err := ctr.ResolveConfSource(m.dataRoot())
	if err != nil {
		return err
	}

	mounts := []ctr.MountSpec{
		{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      resolvPath,
			Options:     []string{"rbind", "ro"},
		},
	}
	tzMounts, tzEnv := ctr.TimezoneSpec()
	mounts = append(mounts, tzMounts...)

	_, err = m.service.CreateContainer(ctx, ctr.CreateContainerRequest{
		ID:          m.containerID(botID),
		ImageRef:    image,
		Snapshotter: m.cfg.Snapshotter,
		Labels: map[string]string{
			BotLabelKey: botID,
		},
		Spec: ctr.ContainerSpec{
			Mounts: mounts,
			Env:    tzEnv,
		},
	})
	if err == nil {
		return nil
	}

	if !errdefs.IsAlreadyExists(err) {
		return err
	}

	return nil
}

// ListBots returns the bot IDs that have MCP containers.
func (m *Manager) ListBots(ctx context.Context) ([]string, error) {
	containers, err := m.service.ListContainers(ctx)
	if err != nil {
		return nil, err
	}

	botIDs := make([]string, 0, len(containers))
	for _, info := range containers {
		if strings.HasPrefix(info.ID, ContainerPrefix) {
			if botID, ok := info.Labels[BotLabelKey]; ok {
				botIDs = append(botIDs, botID)
			}
		}
	}
	return botIDs, nil
}

func (m *Manager) Start(ctx context.Context, botID string) error {
	if err := m.EnsureBot(ctx, botID); err != nil {
		return err
	}

	if err := m.service.StartContainer(ctx, m.containerID(botID), nil); err != nil {
		return err
	}
	netResult, err := m.service.SetupNetwork(ctx, ctr.NetworkSetupRequest{
		ContainerID: m.containerID(botID),
		CNIBinDir:   m.cfg.CNIBinaryDir,
		CNIConfDir:  m.cfg.CNIConfigDir,
	})
	if err != nil {
		if stopErr := m.service.StopContainer(ctx, m.containerID(botID), &ctr.StopTaskOptions{Force: true}); stopErr != nil {
			m.logger.Warn("cleanup: stop task failed", slog.String("container_id", m.containerID(botID)), slog.Any("error", stopErr))
		}
		return err
	}
	if netResult.IP != "" {
		m.mu.Lock()
		m.containerIPs[botID] = netResult.IP
		m.mu.Unlock()
		m.logger.Info("container network ready", slog.String("bot_id", botID), slog.String("ip", netResult.IP))

		// Run migration in the background so Start() returns immediately.
		// Migration uses its own context so it isn't cancelled when the
		// caller's HTTP request finishes.
		go m.migrateBindMountData(context.WithoutCancel(ctx), botID)
	}
	return nil
}

func (m *Manager) Stop(ctx context.Context, botID string, timeout time.Duration) error {
	if err := validateBotID(botID); err != nil {
		return err
	}
	return m.service.StopContainer(ctx, m.containerID(botID), &ctr.StopTaskOptions{
		Timeout: timeout,
		Force:   true,
	})
}

func (m *Manager) Delete(ctx context.Context, botID string) error {
	if err := validateBotID(botID); err != nil {
		return err
	}

	if err := m.service.RemoveNetwork(ctx, ctr.NetworkSetupRequest{
		ContainerID: m.containerID(botID),
		CNIBinDir:   m.cfg.CNIBinaryDir,
		CNIConfDir:  m.cfg.CNIConfigDir,
	}); err != nil {
		m.logger.Warn("cleanup: remove network failed", slog.String("container_id", m.containerID(botID)), slog.Any("error", err))
	}
	if err := m.service.DeleteTask(ctx, m.containerID(botID), &ctr.DeleteTaskOptions{Force: true}); err != nil {
		m.logger.Warn("cleanup: delete task failed", slog.String("container_id", m.containerID(botID)), slog.Any("error", err))
	}
	return m.service.DeleteContainer(ctx, m.containerID(botID), &ctr.DeleteContainerOptions{
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

func validateBotID(botID string) error {
	return identity.ValidateChannelIdentityID(botID)
}
