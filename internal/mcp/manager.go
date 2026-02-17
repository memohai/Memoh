package mcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// ExecWithCaptureResult holds stdout, stderr and exit code from container exec.
type ExecWithCaptureResult struct {
	Stdout   string
	Stderr   string
	ExitCode uint32
}

type Manager struct {
	service     ctr.Service
	cfg         config.MCPConfig
	namespace   string
	containerID func(string) string
	db          *pgxpool.Pool
	queries     *dbsqlc.Queries
	logger      *slog.Logger
}

func NewManager(log *slog.Logger, service ctr.Service, cfg config.MCPConfig, namespace string, conn *pgxpool.Pool) *Manager {
	if namespace == "" {
		namespace = config.DefaultNamespace
	}
	return &Manager{
		service:   service,
		cfg:       cfg,
		namespace: namespace,
		db:        conn,
		queries:   dbsqlc.New(conn),
		logger:    log.With(slog.String("component", "mcp")),
		containerID: func(botID string) string {
			return ContainerPrefix + botID
		},
	}
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

	specOpts := []oci.SpecOpts{
		oci.WithMounts([]specs.Mount{
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
	if err := ctr.SetupNetwork(ctx, task, m.containerID(botID), m.cfg.CNIBinaryDir, m.cfg.CNIConfigDir); err != nil {
		if stopErr := m.service.StopTask(ctx, m.containerID(botID), &ctr.StopTaskOptions{Force: true}); stopErr != nil {
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
		if err := ctr.RemoveNetwork(ctx, task, m.containerID(botID), m.cfg.CNIBinaryDir, m.cfg.CNIConfigDir); err != nil {
			m.logger.Warn("cleanup: remove network failed", slog.String("container_id", m.containerID(botID)), slog.Any("error", err))
		}
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
// On darwin, it uses Lima SSH to avoid virtiofs FIFO synchronization issues.
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

	if runtime.GOOS == "darwin" {
		return m.execWithCaptureLima(ctx, req)
	}
	return m.execWithCaptureContainerd(ctx, req)
}

// execWithCaptureLima runs exec through Lima SSH so that all FIFO I/O stays
// inside the VM, avoiding virtiofs FIFO synchronization issues on macOS.
func (m *Manager) execWithCaptureLima(ctx context.Context, req ExecRequest) (*ExecWithCaptureResult, error) {
	containerID := m.containerID(req.BotID)
	execID := fmt.Sprintf("exec-%d", time.Now().UnixNano())

	// Each element becomes a separate OS arg to limactl.  Lima/SSH joins
	// them with spaces and passes the result to the remote shell, so only
	// values that may contain shell-special characters need quoting.
	args := []string{"shell", "default", "--",
		"sudo", "ctr", "-n", m.namespace,
		"tasks", "exec", "--exec-id", execID,
	}
	if req.WorkDir != "" {
		args = append(args, "--cwd", req.WorkDir)
	}
	for _, e := range req.Env {
		args = append(args, "--env", e)
	}
	args = append(args, containerID)
	// Pass command args as-is; Lima shell-quotes each OS arg for the
	// remote SSH shell, preserving argument boundaries correctly.
	args = append(args, req.Command...)

	cmd := exec.CommandContext(ctx, "limactl", args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	exitCode := uint32(0)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = uint32(exitErr.ExitCode())
		} else {
			return nil, fmt.Errorf("lima exec: %w", err)
		}
	}

	// ctr tasks exec may write its own errors to stderr; separate them from
	// the container command's stderr output by checking for the ctr prefix.
	stderr := stderrBuf.String()
	if exitCode != 0 && strings.HasPrefix(stderr, "ctr:") {
		return nil, fmt.Errorf("container exec failed: %s", strings.TrimSpace(stderr))
	}

	return &ExecWithCaptureResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderr,
		ExitCode: exitCode,
	}, nil
}

// execWithCaptureContainerd uses the containerd ExecTask API with FIFO pipes.
// This works reliably on Linux where FIFO I/O stays on the same filesystem.
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
	if m.cfg.DataMount == "" {
		return config.DefaultDataMount
	}
	return m.cfg.DataMount
}

func (m *Manager) imageRef() string {
	if m.cfg.Image != "" {
		return m.cfg.Image
	}
	return config.DefaultMCPImage
}

func validateBotID(botID string) error {
	return identity.ValidateChannelIdentityID(botID)
}
