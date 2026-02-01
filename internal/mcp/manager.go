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
	UserLabelKey    = "mcp.user_id"
	ContainerPrefix = "mcp-"
)

type ExecRequest struct {
	UserID   string
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

func NewManager(log *slog.Logger, service ctr.Service, cfg config.MCPConfig) *Manager {
	return &Manager{
		service: service,
		cfg:     cfg,
		logger:  log.With(slog.String("manager", "mcp")),
		containerID: func(userID string) string {
			return ContainerPrefix + userID
		},
	}
}

func (m *Manager) WithDB(db *pgxpool.Pool) *Manager {
	m.db = db
	m.queries = dbsqlc.New(db)
	return m
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

func (m *Manager) EnsureUser(ctx context.Context, userID string) error {
	if err := validateUserID(userID); err != nil {
		return err
	}

	dataDir, err := m.ensureUserDir(userID)
	if err != nil {
		return err
	}

	dataMount := m.cfg.DataMount
	if dataMount == "" {
		dataMount = config.DefaultDataMount
	}

	image := m.cfg.BusyboxImage
	if image == "" {
		image = config.DefaultBusyboxImg
	}

	specOpts := []oci.SpecOpts{
		oci.WithMounts([]specs.Mount{{
			Destination: dataMount,
			Type:        "bind",
			Source:      dataDir,
			Options:     []string{"rbind", "rw"},
		}}),
	}

	_, err = m.service.CreateContainer(ctx, ctr.CreateContainerRequest{
		ID:          m.containerID(userID),
		ImageRef:    image,
		Snapshotter: m.cfg.Snapshotter,
		Labels: map[string]string{
			UserLabelKey: userID,
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

func (m *Manager) ListUsers(ctx context.Context) ([]string, error) {
	containers, err := m.service.ListContainers(ctx)
	if err != nil {
		return nil, err
	}

	users := make([]string, 0, len(containers))
	for _, container := range containers {
		info, err := container.Info(ctx)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(info.ID, ContainerPrefix) {
			if userID, ok := info.Labels[UserLabelKey]; ok {
				users = append(users, userID)
			}
		}
	}
	return users, nil
}

func (m *Manager) Start(ctx context.Context, userID string) error {
	if err := m.EnsureUser(ctx, userID); err != nil {
		return err
	}

	_, err := m.service.StartTask(ctx, m.containerID(userID), &ctr.StartTaskOptions{
		UseStdio: false,
	})
	return err
}

func (m *Manager) Stop(ctx context.Context, userID string, timeout time.Duration) error {
	if err := validateUserID(userID); err != nil {
		return err
	}
	return m.service.StopTask(ctx, m.containerID(userID), &ctr.StopTaskOptions{
		Timeout: timeout,
		Force:   true,
	})
}

func (m *Manager) Delete(ctx context.Context, userID string) error {
	if err := validateUserID(userID); err != nil {
		return err
	}

	_ = m.service.DeleteTask(ctx, m.containerID(userID), &ctr.DeleteTaskOptions{Force: true})
	return m.service.DeleteContainer(ctx, m.containerID(userID), &ctr.DeleteContainerOptions{
		CleanupSnapshot: true,
	})
}

func (m *Manager) Exec(ctx context.Context, req ExecRequest) (*ExecResult, error) {
	if err := validateUserID(req.UserID); err != nil {
		return nil, err
	}
	if len(req.Command) == 0 {
		return nil, fmt.Errorf("%w: empty command", ctr.ErrInvalidArgument)
	}
	if m.queries == nil {
		return nil, fmt.Errorf("db is not configured")
	}

	startedAt := time.Now()
	if _, err := m.CreateVersion(ctx, req.UserID); err != nil {
		return nil, err
	}

	result, err := m.service.ExecTask(ctx, m.containerID(req.UserID), ctr.ExecTaskRequest{
		Args:     req.Command,
		Env:      req.Env,
		WorkDir:  req.WorkDir,
		Terminal: req.Terminal,
		UseStdio: req.UseStdio,
	})
	if err != nil {
		return nil, err
	}

	if err := m.insertEvent(ctx, m.containerID(req.UserID), "exec", map[string]any{
		"command":   req.Command,
		"work_dir":  req.WorkDir,
		"exit_code": result.ExitCode,
		"duration":  time.Since(startedAt).String(),
	}); err != nil {
		return nil, err
	}

	return &ExecResult{ExitCode: result.ExitCode}, nil
}

func (m *Manager) DataDir(userID string) (string, error) {
	if err := validateUserID(userID); err != nil {
		return "", err
	}

	root := m.cfg.DataRoot
	if root == "" {
		root = config.DefaultDataRoot
	}
	return filepath.Join(root, "users", userID), nil
}

func (m *Manager) ensureUserDir(userID string) (string, error) {
	root := m.cfg.DataRoot
	if root == "" {
		root = config.DefaultDataRoot
	}
	dir := filepath.Join(root, "users", userID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func validateUserID(userID string) error {
	return identity.ValidateUserID(userID)
}
