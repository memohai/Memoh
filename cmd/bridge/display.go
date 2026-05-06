package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/logger"
)

const (
	displayEnabledEnv     = "MEMOH_DISPLAY_ENABLED"
	displayRFBUnixPathEnv = "MEMOH_DISPLAY_RFB_UNIX_PATH"
	displayBrowserURLEnv  = "MEMOH_DISPLAY_BROWSER_URL"
	toolkitXvncPath       = "/opt/memoh/toolkit/display/bin/Xvnc"
	toolkitXkbcompPath    = "/opt/memoh/toolkit/display/bin/xkbcomp"
	toolkitXsetrootPath   = "/opt/memoh/toolkit/display/bin/xsetroot"
	toolkitTwmPath        = "/opt/memoh/toolkit/display/bin/twm"
	toolkitXtermPath      = "/opt/memoh/toolkit/display/bin/xterm"
	systemXkbcompPath     = "/usr/bin/xkbcomp"
	x11SocketDir          = "/tmp/.X11-unix"
	xvncDisplay           = ":99"
	xvncGeometry          = "1280x800"
	xvncSocketPath        = x11SocketDir + "/X99"
	xvncLockPath          = "/tmp/.X99-lock"
	defaultRFBUnixPath    = "/run/memoh/display.rfb.sock"
	displayReadyTimeout   = 30 * time.Second
)

func startDisplaySupervisor(ctx context.Context) {
	if !isTruthy(os.Getenv(displayEnabledEnv)) {
		return
	}
	go superviseXvnc(ctx)
}

func ensureDisplayRuntimeLinks(ctx context.Context, xkbcompPath string) {
	if _, err := os.Stat(systemXkbcompPath); err == nil {
		return
	}
	if strings.TrimSpace(xkbcompPath) == "" {
		logger.FromContext(ctx).Warn("display requested but xkbcomp is unavailable")
		return
	}
	if err := os.Symlink(xkbcompPath, systemXkbcompPath); err != nil && !os.IsExist(err) {
		logger.FromContext(ctx).Warn("failed to link xkbcomp for Xvnc", slog.String("target", xkbcompPath), slog.String("link", systemXkbcompPath), slog.Any("error", err))
	}
}

func superviseXvnc(ctx context.Context) {
	backoff := time.Second
	for {
		startedAt := time.Now()
		xvncPath := resolveDisplayCommand(toolkitXvncPath, "/usr/bin/Xvnc", "/usr/local/bin/Xvnc", "Xvnc")
		if xvncPath == "" {
			logger.FromContext(ctx).Warn("display requested but Xvnc is unavailable")
			if waitDisplayRetry(ctx, backoff) {
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		ensureDisplayRuntimeLinks(ctx, resolveDisplayCommand(toolkitXkbcompPath, "/usr/bin/xkbcomp", "/usr/local/bin/xkbcomp", "xkbcomp"))
		rfbUnixPath := displayRFBUnixPath()
		prepareX11SocketDir(ctx)
		if displaySocketReady(ctx, rfbUnixPath) {
			logger.FromContext(ctx).Info("Xvnc display already available", slog.String("display", xvncDisplay), slog.String("rfb_unix_path", rfbUnixPath))
			go startDisplaySession(ctx)
			if waitExistingDisplay(ctx, rfbUnixPath) {
				return
			}
			backoff = time.Second
			continue
		}
		if xvncProcessRunning() {
			stopXvncProcesses(ctx)
		}
		prepareDisplaySockets(ctx, rfbUnixPath)
		cmd := exec.CommandContext(ctx, xvncPath, //nolint:gosec // path is a fixed runtime bundle executable
			xvncDisplay,
			"-geometry", xvncGeometry,
			"-depth", "24",
			"-SecurityTypes", "None",
			"-rfbunixpath", rfbUnixPath,
			"-rfbunixmode", "0660",
			"-rfbport", "0",
		)
		cmd.Env = withDisplayEnv(os.Environ())
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			logger.FromContext(ctx).Warn("failed to start Xvnc", slog.Any("error", err))
		} else {
			logger.FromContext(ctx).Info("Xvnc display started", slog.Int("pid", cmd.Process.Pid), slog.String("display", xvncDisplay), slog.String("rfb_unix_path", rfbUnixPath))
			go startDisplaySession(ctx)
			waitErr := make(chan error, 1)
			go func() {
				waitErr <- cmd.Wait()
			}()
			select {
			case <-ctx.Done():
				_ = cmd.Process.Kill()
				<-waitErr
				return
			case err := <-waitErr:
				if ctx.Err() != nil {
					return
				}
				if err != nil {
					logger.FromContext(ctx).Warn("Xvnc exited", slog.Any("error", err))
				} else {
					logger.FromContext(ctx).Warn("Xvnc exited")
				}
			}
		}

		if time.Since(startedAt) > 30*time.Second {
			backoff = time.Second
		} else if backoff < 30*time.Second {
			backoff *= 2
		}
		if waitDisplayRetry(ctx, backoff) {
			return
		}
	}
}

func waitDisplayRetry(ctx context.Context, backoff time.Duration) bool {
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return true
	case <-timer.C:
		return false
	}
}

func waitExistingDisplay(ctx context.Context, rfbUnixPath string) bool {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return true
		case <-ticker.C:
			if !displaySocketReady(ctx, rfbUnixPath) {
				return false
			}
		}
	}
}

func displaySocketReady(ctx context.Context, path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	dialCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	dialer := net.Dialer{Timeout: 300 * time.Millisecond}
	conn, err := dialer.DialContext(dialCtx, "unix", filepath.Clean(path))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func displayRFBUnixPath() string {
	path := strings.TrimSpace(os.Getenv(displayRFBUnixPathEnv))
	if path == "" {
		return defaultRFBUnixPath
	}
	return filepath.Clean(path)
}

func prepareDisplaySockets(ctx context.Context, path string) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		logger.FromContext(ctx).Warn("failed to create display socket directory", slog.String("dir", dir), slog.Any("error", err))
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logger.FromContext(ctx).Warn("failed to remove stale display socket", slog.String("path", path), slog.Any("error", err))
	}
	if xvncProcessRunning() {
		return
	}
	for _, stalePath := range []string{xvncSocketPath, xvncLockPath} {
		if err := os.Remove(stalePath); err != nil && !os.IsNotExist(err) {
			logger.FromContext(ctx).Warn("failed to remove stale Xvnc file", slog.String("path", stalePath), slog.Any("error", err))
		}
	}
}

func prepareX11SocketDir(ctx context.Context) {
	if err := os.MkdirAll(x11SocketDir, 0o1777); err != nil { //nolint:gosec // X11 socket dir must be world-writable with sticky bit.
		logger.FromContext(ctx).Warn("failed to create X11 socket directory", slog.String("dir", x11SocketDir), slog.Any("error", err))
		return
	}
	if err := os.Chmod(x11SocketDir, 0o1777); err != nil { //nolint:gosec // X11 socket dir must be world-writable with sticky bit.
		logger.FromContext(ctx).Warn("failed to set X11 socket directory permissions", slog.String("dir", x11SocketDir), slog.Any("error", err))
	}
}

func startDisplaySession(ctx context.Context) {
	if err := waitForDisplaySocket(ctx, displayReadyTimeout); err != nil {
		logger.FromContext(ctx).Warn("display session skipped; X socket not ready", slog.Any("error", err))
		return
	}
	if err := sleepWithContext(ctx, 300*time.Millisecond); err != nil {
		return
	}
	if xsetroot := resolveDisplayCommand(toolkitXsetrootPath, "/usr/bin/xsetroot", "/usr/local/bin/xsetroot", "xsetroot"); xsetroot != "" {
		runDisplayCommand(ctx, xsetroot, "-solid", "#315f7d")
	}
	startDesktopSession(ctx)
	startDisplayTerminal(ctx)
	startDisplayBrowser(ctx)
}

func waitForDisplaySocket(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(xvncSocketPath); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return os.ErrDeadlineExceeded
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func runDisplayCommand(ctx context.Context, path string, args ...string) {
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		return
	}
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, path, args...) //nolint:gosec // path is a fixed runtime bundle executable
	cmd.Env = withDisplayEnv(os.Environ())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logger.FromContext(ctx).Warn("display helper failed", slog.String("path", path), slog.Any("error", err))
	}
}

func startDesktopSession(ctx context.Context) {
	if displayProcessRunning(ctx, "xfce4-session", "xfwm4", "twm") {
		return
	}
	if desktop := resolveDisplayCommand("startxfce4"); desktop != "" {
		startDisplayCommand(ctx, "desktop", desktop)
		return
	}
	if desktop := resolveDisplayCommand("xfce4-session"); desktop != "" {
		startDisplayCommand(ctx, "desktop", desktop)
		return
	}
	if windowManager := resolveDisplayCommand("xfwm4"); windowManager != "" {
		startDisplayCommand(ctx, "window manager", windowManager)
		return
	}
	if windowManager := resolveDisplayCommand(toolkitTwmPath); windowManager != "" {
		startDisplayCommand(ctx, "window manager", windowManager)
		return
	}
	logger.FromContext(ctx).Warn("display desktop session unavailable")
}

func startDisplayTerminal(ctx context.Context) {
	xterm := resolveDisplayCommand(toolkitXtermPath, "/usr/bin/xterm", "/usr/local/bin/xterm", "xterm")
	if xterm == "" {
		return
	}
	startDisplayCommand(ctx, "terminal", xterm,
		"-geometry", "100x30+28+28",
		"-title", "Memoh Workspace",
		"-e", "/bin/sh", "-c", "cd /data 2>/dev/null || cd /; exec /bin/sh",
	)
}

func startDisplayBrowser(ctx context.Context) {
	if displayProcessRunning(ctx, "google-chrome", "chromium") {
		return
	}
	browser := resolveDisplayCommand("google-chrome-stable", "google-chrome", "chromium", "chromium-browser")
	if browser == "" {
		logger.FromContext(ctx).Warn("display browser unavailable")
		return
	}
	url := strings.TrimSpace(os.Getenv(displayBrowserURLEnv))
	if url == "" {
		url = "about:blank"
	}
	startDisplayCommand(ctx, "browser", browser,
		"--no-sandbox",
		"--disable-dev-shm-usage",
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--user-data-dir=/tmp/memoh-display-browser",
		url,
	)
}

func displayProcessRunning(ctx context.Context, patterns ...string) bool {
	pgrep := resolveDisplayCommand("pgrep")
	if pgrep == "" {
		return false
	}
	for _, pattern := range patterns {
		cmd := exec.CommandContext(ctx, pgrep, "-f", pattern) //nolint:gosec // patterns are controlled by this package.
		if cmd.Run() == nil {
			return true
		}
	}
	return false
}

func xvncProcessRunning() bool {
	return len(xvncProcessIDs()) > 0
}

func xvncProcessIDs() []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var pids []int
	for _, entry := range entries {
		name := entry.Name()
		if name == "" || name[0] < '0' || name[0] > '9' {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", name, "cmdline")) //nolint:gosec // /proc entries are kernel-provided.
		if err != nil || len(cmdline) == 0 {
			continue
		}
		pid, err := strconv.Atoi(name)
		if err != nil {
			continue
		}
		parts := strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")
		hasXvnc := false
		hasDisplay := false
		for _, arg := range parts {
			if filepath.Base(arg) == "Xvnc" {
				hasXvnc = true
			}
			if arg == xvncDisplay {
				hasDisplay = true
			}
		}
		if hasXvnc && hasDisplay {
			pids = append(pids, pid)
		}
	}
	return pids
}

func stopXvncProcesses(ctx context.Context) {
	pids := xvncProcessIDs()
	for _, pid := range pids {
		process, err := os.FindProcess(pid)
		if err == nil {
			_ = process.Kill()
		}
	}
	if len(pids) == 0 {
		return
	}
	if err := sleepWithContext(ctx, 300*time.Millisecond); err != nil {
		return
	}
}

func startDisplayCommand(ctx context.Context, name, path string, args ...string) {
	info, err := os.Stat(path)
	if err != nil {
		logger.FromContext(ctx).Warn("display helper unavailable", slog.String("name", name), slog.String("path", path), slog.Any("error", err))
		return
	}
	if info.Mode().Perm()&0o111 == 0 {
		logger.FromContext(ctx).Warn("display helper is not executable", slog.String("name", name), slog.String("path", path))
		return
	}
	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec // path is a fixed runtime bundle executable
	cmd.Env = withDisplayEnv(os.Environ())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		logger.FromContext(ctx).Warn("failed to start display helper", slog.String("name", name), slog.Any("error", err))
		return
	}
	logger.FromContext(ctx).Info("display helper started", slog.String("name", name), slog.Int("pid", cmd.Process.Pid))
	go func() {
		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			logger.FromContext(ctx).Warn("display helper exited", slog.String("name", name), slog.Any("error", err))
		}
	}()
}

func resolveDisplayCommand(candidates ...string) string {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.Contains(candidate, "/") {
			info, err := os.Stat(candidate)
			if err == nil && info.Mode().Perm()&0o111 != 0 {
				return candidate
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return ""
}

func withDisplayEnv(env []string) []string {
	out := make([]string, 0, len(env)+1)
	hasDisplay := false
	for _, item := range env {
		switch {
		case strings.HasPrefix(item, "DISPLAY="):
			out = append(out, "DISPLAY="+xvncDisplay)
			hasDisplay = true
		default:
			out = append(out, item)
		}
	}
	if !hasDisplay {
		out = append(out, "DISPLAY="+xvncDisplay)
	}
	return out
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "t", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
