package mcp

import (
	"bytes"
	"context"
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
)

const (
	BotLabelKey     = "mcp.bot_id"
	ContainerPrefix = "mcp-"
)

type ExecRequest struct {
	BotID    string
	Command  []string
	Env      []string
	WorkDir  string
	Terminal bool
	UseStdio bool
}

type ExecResult struct {
	ExitCode uint32
}

// ExecWithCaptureResult holds stdout, stderr and exit code from container exec.
type ExecWithCaptureResult struct {
	Stdout   string
	Stderr   string
	ExitCode uint32
}

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
}

func NewManager(log *slog.Logger, service ctr.Service, cfg config.MCPConfig, namespace string, conn *pgxpool.Pool) *Manager {
	if namespace == "" {
		namespace = config.DefaultNamespace
	}
	return &Manager{
		service:        service,
		cfg:            cfg,
		namespace:      namespace,
		db:             conn,
		queries:        dbsqlc.New(conn),
		logger:         log.With(slog.String("component", "mcp")),
		containerLocks: make(map[string]*sync.Mutex),
		containerID: func(botID string) string {
			return ContainerPrefix + botID
		},
	}
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

func (m *Manager) Init(ctx context.Context) error {
	image := m.imageRef()

	_, err := m.service.PullImage(ctx, image, &ctr.PullImageOptions{
		Unpack:      true,
		Snapshotter: m.cfg.Snapshotter,
	})
	return err
}

// EnsureBot creates the MCP container for a bot if it does not exist.
func (m *Manager) EnsureBot(ctx context.Context, botID string) error {
	if err := validateBotID(botID); err != nil {
		return err
	}

	dataDir, err := m.ensureBotDir(botID)
	if err != nil {
		return err
	}

	dataMount := m.dataMount()
	image := m.imageRef()
	resolvPath, err := ctr.ResolveConfSource(dataDir)
	if err != nil {
		return err
	}

	mounts := []ctr.MountSpec{
		{
			Destination: dataMount,
			Type:        "bind",
			Source:      dataDir,
			Options:     []string{"rbind", "rw"},
		},
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

	if err := m.service.StartContainer(ctx, m.containerID(botID), &ctr.StartTaskOptions{
		UseStdio: false,
	}); err != nil {
		return err
	}
	if err := m.service.SetupNetwork(ctx, ctr.NetworkSetupRequest{
		ContainerID: m.containerID(botID),
		CNIBinDir:   m.cfg.CNIBinaryDir,
		CNIConfDir:  m.cfg.CNIConfigDir,
	}); err != nil {
		if stopErr := m.service.StopContainer(ctx, m.containerID(botID), &ctr.StopTaskOptions{Force: true}); stopErr != nil {
			m.logger.Warn("cleanup: stop task failed", slog.String("container_id", m.containerID(botID)), slog.Any("error", stopErr))
		}
		return err
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

func (m *Manager) Exec(ctx context.Context, req ExecRequest) (*ExecResult, error) {
	if err := validateBotID(req.BotID); err != nil {
		return nil, err
	}
	if len(req.Command) == 0 {
		return nil, fmt.Errorf("%w: empty command", ctr.ErrInvalidArgument)
	}
	if m.queries == nil {
		return nil, fmt.Errorf("db is not configured")
	}

	startedAt := time.Now()
	if _, err := m.CreateVersion(ctx, req.BotID); err != nil {
		return nil, err
	}

	result, err := m.service.ExecTask(ctx, m.containerID(req.BotID), ctr.ExecTaskRequest{
		Args:     req.Command,
		Env:      req.Env,
		WorkDir:  req.WorkDir,
		Terminal: req.Terminal,
		UseStdio: req.UseStdio,
	})
	if err != nil {
		return nil, err
	}

	if err := m.insertEvent(ctx, m.containerID(req.BotID), "exec", map[string]any{
		"bot_id":    req.BotID,
		"command":   req.Command,
		"work_dir":  req.WorkDir,
		"exit_code": result.ExitCode,
		"duration":  time.Since(startedAt).String(),
	}); err != nil {
		return nil, err
	}

	return &ExecResult{ExitCode: result.ExitCode}, nil
}

// ExecWithCapture runs a command in the bot container and returns stdout, stderr and exit code.
// Use this when the caller needs command output (e.g. MCP exec tool).
// The container must already be running; use Start(botID) or the container/start API to start it.
func (m *Manager) ExecWithCapture(ctx context.Context, req ExecRequest) (*ExecWithCaptureResult, error) {
	if err := validateBotID(req.BotID); err != nil {
		return nil, err
	}
	if len(req.Command) == 0 {
		return nil, fmt.Errorf("%w: empty command", ctr.ErrInvalidArgument)
	}
	if m.queries == nil {
		return nil, fmt.Errorf("db is not configured")
	}
	return m.execWithCaptureContainerd(ctx, req)
}

func (m *Manager) execWithCaptureContainerd(ctx context.Context, req ExecRequest) (*ExecWithCaptureResult, error) {
	fifoDir, err := os.MkdirTemp(m.dataRoot(), "exec-fifo-")
	if err != nil {
		return nil, fmt.Errorf("create fifo dir: %w", err)
	}
	defer os.RemoveAll(fifoDir)

	var stdoutBuf, stderrBuf bytes.Buffer
	result, err := m.service.ExecTask(ctx, m.containerID(req.BotID), ctr.ExecTaskRequest{
		Args:    req.Command,
		Env:     req.Env,
		WorkDir: req.WorkDir,
		Stderr:  &stderrBuf,
		Stdout:  &stdoutBuf,
		FIFODir: fifoDir,
	})
	if err != nil {
		return nil, err
	}
	return &ExecWithCaptureResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: result.ExitCode,
	}, nil
}

// DataDir returns the host data directory for a bot.
func (m *Manager) DataDir(botID string) (string, error) {
	if err := validateBotID(botID); err != nil {
		return "", err
	}

	return filepath.Join(m.dataRoot(), "bots", botID), nil
}

func (m *Manager) ensureBotDir(botID string) (string, error) {
	dir := filepath.Join(m.dataRoot(), "bots", botID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func (m *Manager) dataRoot() string {
	if m.cfg.DataRoot == "" {
		return config.DefaultDataRoot
	}
	return m.cfg.DataRoot
}

func (m *Manager) dataMount() string {
	return config.DefaultDataMount
}

func (m *Manager) imageRef() string {
	return m.cfg.ImageRef()
}

func validateBotID(botID string) error {
	return identity.ValidateChannelIdentityID(botID)
}
