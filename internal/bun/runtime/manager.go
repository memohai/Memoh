package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/embedded"
)

type Manager struct {
	log      *slog.Logger
	cfg      config.Config
	host     string
	port     int
	workdir  string
	cmd      *exec.Cmd
	stopOnce sync.Once
}

const (
	defaultGatewayHost      = "127.0.0.1"
	defaultGatewayPort      = 8081
	agentConfigFileName     = "config.toml"
	agentBinName            = "agent-bin"
	agentUnavailableMarker  = "UNAVAILABLE"
	healthCheckTimeout      = 30 * time.Second
	healthCheckRetryBackoff = 400 * time.Millisecond
	processStopTimeout      = 5 * time.Second
)

func NewManager(log *slog.Logger, cfg config.Config) *Manager {
	host := cfg.AgentGateway.Host
	if host == "" {
		host = defaultGatewayHost
	}
	port := cfg.AgentGateway.Port
	if port == 0 {
		port = defaultGatewayPort
	}
	return &Manager{
		log:  log.With(slog.String("component", "agent-runtime")),
		cfg:  cfg,
		host: host,
		port: port,
	}
}

func (m *Manager) Start(ctx context.Context) error {
	workdir, err := os.MkdirTemp("", "memoh-agent-runtime-*")
	if err != nil {
		return fmt.Errorf("create runtime temp dir: %w", err)
	}
	m.workdir = workdir

	agentFS, err := embedded.AgentFS()
	if err != nil {
		return err
	}

	agentDir := filepath.Join(workdir, "agent")
	if err := extractFS(agentFS, agentDir); err != nil {
		return fmt.Errorf("extract agent assets: %w", err)
	}

	agentBinPath := filepath.Join(agentDir, agentBinaryNameForRuntime())
	if _, err := os.Stat(agentBinPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			markerPath := filepath.Join(agentDir, agentUnavailableMarker)
			if _, markerErr := os.Stat(markerPath); markerErr == nil {
				m.log.Warn("bundled agent binary unavailable for current platform; falling back to configured agent gateway", slog.String("platform", runtimePlatform()))
				return nil
			}
		}
		return fmt.Errorf("agent binary missing: %w", err)
	}
	if err := os.Chmod(agentBinPath, 0o755); err != nil { //nolint:gosec // G302: executable binary requires execute bit; 0600 would make it non-executable
		return fmt.Errorf("chmod agent binary: %w", err)
	}
	agentConfigPath := filepath.Join(agentDir, agentConfigFileName)
	if err := writeAgentConfig(agentConfigPath, m.cfg); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, agentBinPath) //nolint:gosec // G204: path is constructed internally from an embedded asset, not user input
	cmd.Dir = agentDir
	cmd.Env = append(
		os.Environ(),
		"MEMOH_CONFIG_PATH="+agentConfigPath,
		"CONFIG_PATH="+agentConfigPath,
	)
	cmd.Stdout = &logWriter{log: m.log, level: slog.LevelInfo}
	cmd.Stderr = &logWriter{log: m.log, level: slog.LevelError}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start bundled agent runtime: %w", err)
	}
	m.cmd = cmd

	m.log.Info("bundled agent runtime started", slog.Int("pid", cmd.Process.Pid), slog.String("addr", m.address()))
	if err := m.waitHealthy(ctx); err != nil {
		return err
	}
	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	var retErr error
	m.stopOnce.Do(func() {
		if m.cmd == nil || m.cmd.Process == nil {
			return
		}

		_ = m.cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() {
			done <- m.cmd.Wait()
		}()

		select {
		case err := <-done:
			if err != nil && !errors.Is(err, syscall.EINTR) {
				retErr = err
			}
		case <-ctx.Done():
			_ = m.cmd.Process.Kill()
			retErr = ctx.Err()
		case <-time.After(processStopTimeout):
			_ = m.cmd.Process.Kill()
			<-done
		}

		if m.workdir != "" {
			_ = os.RemoveAll(m.workdir)
		}
	})
	return retErr
}

func (m *Manager) waitHealthy(ctx context.Context) error {
	client := &http.Client{Timeout: 2 * time.Second}
	healthURL := fmt.Sprintf("http://%s/health", m.address())
	deadline := time.Now().Add(healthCheckTimeout)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		resp, err := client.Do(req) //nolint:gosec // G704: URL is constructed from operator-configured host/port, not from user input
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}
		time.Sleep(healthCheckRetryBackoff)
	}
	return fmt.Errorf("bundled agent runtime health check timeout: %s", healthURL)
}

func (m *Manager) address() string {
	return fmt.Sprintf("%s:%d", m.host, m.port)
}

func extractFS(src fs.FS, targetDir string) error {
	if err := os.MkdirAll(targetDir, 0o750); err != nil {
		return err
	}
	return fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		target := filepath.Join(targetDir, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		r, err := src.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = r.Close() }()

		w, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) //nolint:gosec // G304: target is derived from an embedded FS walk within a process-owned temp dir
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, r); err != nil {
			_ = w.Close()
			return err
		}
		return w.Close()
	})
}

func writeAgentConfig(path string, cfg config.Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	f, err := os.Create(path) //nolint:gosec // G304: path is constructed internally from a process-owned temp dir
	if err != nil {
		return fmt.Errorf("create agent config: %w", err)
	}
	defer func() { _ = f.Close() }()
	return toml.NewEncoder(f).Encode(cfg)
}

type logWriter struct {
	log   *slog.Logger
	level slog.Level
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	msg = trimTrailingNewline(msg)
	if msg != "" {
		w.log.LogAttrs(context.Background(), w.level, "runtime process output", slog.String("detail", msg))
	}
	return len(p), nil
}

func trimTrailingNewline(s string) string {
	for len(s) > 0 {
		last := s[len(s)-1]
		if last != '\n' && last != '\r' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

func runtimePlatform() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

func agentBinaryNameForRuntime() string {
	if runtime.GOOS == "windows" {
		return agentBinName + ".exe"
	}
	return agentBinName
}
