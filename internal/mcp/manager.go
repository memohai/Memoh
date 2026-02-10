package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opencontainers/runtime-spec/specs-go"

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

type Manager struct {
	service     ctr.Service
	cfg         config.MCPConfig
	containerID func(string) string
	db          *pgxpool.Pool
	queries     *dbsqlc.Queries
	logger      *slog.Logger
}

func NewManager(log *slog.Logger, service ctr.Service, cfg config.Config, db *pgxpool.Pool) *Manager {
	return &Manager{
		db:      db,
		queries: dbsqlc.New(db),
		service: service,
		cfg:     cfg.MCP,
		logger:  log.With(slog.String("component", "mcp")),
		containerID: func(botID string) string {
			return ContainerPrefix + botID
		},
	}
}

func (m *Manager) Init(ctx context.Context) error {
	image := m.cfg.BusyboxImage
	if image == "" {
		image = config.DefaultBusyboxImg
	}

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

	specOpts := []oci.SpecOpts{
		oci.WithMounts([]specs.Mount{
			{
				Destination: dataMount,
				Type:        "bind",
				Source:      dataDir,
				Options:     []string{"rbind", "rw"},
			},
			{
				Destination: "/app",
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
		}),
	}

	_, err = m.service.CreateContainer(ctx, ctr.CreateContainerRequest{
		ID:          m.containerID(botID),
		ImageRef:    image,
		Snapshotter: m.cfg.Snapshotter,
		Labels: map[string]string{
			BotLabelKey: botID,
		},
		SpecOpts: specOpts,
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
	for _, container := range containers {
		info, err := container.Info(ctx)
		if err != nil {
			return nil, err
		}
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

	task, err := m.service.StartTask(ctx, m.containerID(botID), &ctr.StartTaskOptions{
		UseStdio: false,
	})
	if err != nil {
		return err
	}
	if err := ctr.SetupNetwork(ctx, task, m.containerID(botID)); err != nil {
		_ = m.service.StopTask(ctx, m.containerID(botID), &ctr.StopTaskOptions{Force: true})
		return err
	}
	return nil
}

func (m *Manager) Stop(ctx context.Context, botID string, timeout time.Duration) error {
	if err := validateBotID(botID); err != nil {
		return err
	}
	return m.service.StopTask(ctx, m.containerID(botID), &ctr.StopTaskOptions{
		Timeout: timeout,
		Force:   true,
	})
}

func (m *Manager) Delete(ctx context.Context, botID string) error {
	if err := validateBotID(botID); err != nil {
		return err
	}

	if task, taskErr := m.service.GetTask(ctx, m.containerID(botID)); taskErr == nil {
		_ = ctr.RemoveNetwork(ctx, task, m.containerID(botID))
	}
	_ = m.service.DeleteTask(ctx, m.containerID(botID), &ctr.DeleteTaskOptions{Force: true})
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
	if m.cfg.DataMount == "" {
		return config.DefaultDataMount
	}
	return m.cfg.DataMount
}

func (m *Manager) imageRef() string {
	if m.cfg.BusyboxImage == "" {
		return config.DefaultBusyboxImg
	}
	return m.cfg.BusyboxImage
}

func validateBotID(botID string) error {
	return identity.ValidateUserID(botID)
}
