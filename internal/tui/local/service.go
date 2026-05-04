package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// ManagedPid mirrors the JSON shape written by
// apps/desktop/src/main/daemon.ts so the desktop main process and the
// CLI can interchangeably manage the same memoh-server child.
type ManagedPid struct {
	Pid       int    `json:"pid"`
	Command   string `json:"command"`
	StartedAt string `json:"startedAt"`
}

// ReadPidFile parses local-server.pid.json. Missing file returns
// (nil, nil) so callers can treat "no server registered" without
// distinguishing IO from absence.
func ReadPidFile(path string) (*ManagedPid, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // path comes from UserDataDir
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read pid file: %w", err)
	}
	var info ManagedPid
	if err := json.Unmarshal(raw, &info); err != nil {
		return nil, fmt.Errorf("decode pid file: %w", err)
	}
	if info.Pid <= 0 {
		return nil, nil
	}
	return &info, nil
}

// WritePidFile atomically persists ManagedPid using the same JSON shape
// that desktop expects when it starts up and tries to recover an
// existing managed server.
func WritePidFile(path string, info ManagedPid) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create pid file dir: %w", err)
	}
	encoded, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("encode pid file: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	return os.Rename(tmp, path)
}

// RemovePidFile deletes the pid descriptor; missing-file is not an
// error.
func RemovePidFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// IsAlive reports whether the OS still has a process with the given
// pid. On Unix this is implemented via signal 0 (a permission probe);
// on Windows it consults the process snapshot.
func IsAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		// On Windows, FindProcess always succeeds — fall back to a
		// best-effort signal which translates to OpenProcess.
		return process.Signal(syscall.Signal(0)) == nil
	}
	return process.Signal(syscall.Signal(0)) == nil
}

// SpawnOptions controls how StartServer launches the bundled binary.
type SpawnOptions struct {
	// Binary defaults to BundledServerBinary().
	Binary string
	// ConfigPath defaults to ConfigPath().
	ConfigPath string
	// WorkingDir defaults to UserDataDir().
	WorkingDir string
	// LogPath defaults to LogPath(); the child's stdout/stderr are
	// appended to it.
	LogPath string
	// PidPath defaults to PidPath().
	PidPath string
	// Args overrides the default ["serve"] command-line.
	Args []string
	// ExtraEnv is merged onto the parent environment. CONFIG_PATH is
	// always set automatically.
	ExtraEnv []string
}

// StartServer launches the bundled memoh-server binary as a detached
// child, registers its pid in local-server.pid.json, and returns once
// the OS has accepted the spawn (it does NOT wait for the server to
// finish becoming HTTP-ready — callers should use the health package
// for that).
func StartServer(opts SpawnOptions) (*ManagedPid, error) {
	binary := opts.Binary
	if binary == "" {
		resolved, err := BundledServerBinary()
		if err != nil {
			return nil, err
		}
		binary = resolved
	}
	configPath := opts.ConfigPath
	if configPath == "" {
		resolved, err := ConfigPath()
		if err != nil {
			return nil, err
		}
		configPath = resolved
	}
	if _, err := os.Stat(configPath); err != nil {
		return nil, fmt.Errorf("config not found at %s; open Memoh.app once to initialize: %w", configPath, err)
	}
	workDir := opts.WorkingDir
	if workDir == "" {
		resolved, err := UserDataDir()
		if err != nil {
			return nil, err
		}
		workDir = resolved
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, fmt.Errorf("create working dir: %w", err)
	}
	logPath := opts.LogPath
	if logPath == "" {
		resolved, err := LogPath()
		if err != nil {
			return nil, err
		}
		logPath = resolved
	}
	pidPath := opts.PidPath
	if pidPath == "" {
		resolved, err := PidPath()
		if err != nil {
			return nil, err
		}
		pidPath = resolved
	}
	args := opts.Args
	if args == nil {
		args = []string{"serve"}
	}

	if existing, err := ReadPidFile(pidPath); err == nil && existing != nil && IsAlive(existing.Pid) {
		return existing, fmt.Errorf("server already running (pid=%d); use `memoh stop` first", existing.Pid)
	}

	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // logPath comes from UserDataDir
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	// Use context.Background() — the spawned server is detached and
	// must outlive the CLI invocation; we deliberately do not wire
	// it to a cancelable context.
	cmd := exec.CommandContext(context.Background(), binary, args...) //nolint:gosec // binary path is derived from os.Executable
	cmd.Dir = workDir
	env := append(os.Environ(), "CONFIG_PATH="+configPath)
	env = append(env, opts.ExtraEnv...)
	cmd.Env = env
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	detachAttrs(cmd)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("spawn memoh-server: %w", err)
	}

	// Detach: don't reap on exit; the child outlives this CLI invocation.
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}

	info := ManagedPid{
		Pid:       cmd.Process.Pid,
		Command:   binary + " " + joinArgs(args),
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := WritePidFile(pidPath, info); err != nil {
		return &info, fmt.Errorf("server started (pid=%d) but failed to write pid file: %w", info.Pid, err)
	}
	return &info, nil
}

// StopServer reads the pid file, sends SIGTERM, waits up to timeout
// for graceful exit, then escalates to SIGKILL. Returns true if a live
// process was actually killed; false if no managed server was running.
func StopServer(pidPath string, timeout time.Duration) (bool, error) {
	if pidPath == "" {
		resolved, err := PidPath()
		if err != nil {
			return false, err
		}
		pidPath = resolved
	}
	info, err := ReadPidFile(pidPath)
	if err != nil {
		return false, err
	}
	if info == nil || !IsAlive(info.Pid) {
		_ = RemovePidFile(pidPath)
		return false, nil
	}
	if err := terminate(info.Pid); err != nil {
		return false, fmt.Errorf("terminate pid=%d: %w", info.Pid, err)
	}
	if !waitExit(info.Pid, timeout) {
		if err := kill(info.Pid); err != nil {
			return false, fmt.Errorf("kill pid=%d: %w", info.Pid, err)
		}
		waitExit(info.Pid, timeout)
	}
	_ = RemovePidFile(pidPath)
	return true, nil
}

func waitExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !IsAlive(pid) {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return !IsAlive(pid)
}

func joinArgs(args []string) string {
	return strings.Join(args, " ")
}
