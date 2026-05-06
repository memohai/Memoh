package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/logger"
)

const (
	displayEnabledEnv     = "MEMOH_DISPLAY_ENABLED"
	displayRFBUnixPathEnv = "MEMOH_DISPLAY_RFB_UNIX_PATH"
	xvncPath              = "/opt/memoh/toolkit/display/bin/Xvnc"
	xkbcompPath           = "/opt/memoh/toolkit/display/bin/xkbcomp"
	xsetrootPath          = "/opt/memoh/toolkit/display/bin/xsetroot"
	twmPath               = "/opt/memoh/toolkit/display/bin/twm"
	xtermPath             = "/opt/memoh/toolkit/display/bin/xterm"
	systemXkbcompPath     = "/usr/bin/xkbcomp"
	x11SocketDir          = "/tmp/.X11-unix"
	xvncDisplay           = ":99"
	xvncGeometry          = "1280x800"
	xvncSocketPath        = x11SocketDir + "/X99"
	defaultRFBUnixPath    = "/run/memoh/display.rfb.sock"
	displayReadyTimeout   = 30 * time.Second
)

func startDisplaySupervisor(ctx context.Context) {
	if !isTruthy(os.Getenv(displayEnabledEnv)) {
		return
	}
	info, err := os.Stat(xvncPath)
	if err != nil {
		logger.FromContext(ctx).Warn("display requested but Xvnc is unavailable", slog.String("path", xvncPath), slog.Any("error", err))
		return
	}
	if info.Mode().Perm()&0o111 == 0 {
		logger.FromContext(ctx).Warn("display requested but Xvnc is not executable", slog.String("path", xvncPath))
		return
	}
	ensureDisplayRuntimeLinks(ctx)

	go superviseXvnc(ctx)
}

func ensureDisplayRuntimeLinks(ctx context.Context) {
	if _, err := os.Stat(systemXkbcompPath); err == nil {
		return
	}
	if _, err := os.Stat(xkbcompPath); err != nil {
		logger.FromContext(ctx).Warn("display requested but xkbcomp wrapper is unavailable", slog.String("path", xkbcompPath), slog.Any("error", err))
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
		rfbUnixPath := displayRFBUnixPath()
		prepareX11SocketDir(ctx)
		prepareRFBUnixSocket(ctx, rfbUnixPath)
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
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func displayRFBUnixPath() string {
	path := strings.TrimSpace(os.Getenv(displayRFBUnixPathEnv))
	if path == "" {
		return defaultRFBUnixPath
	}
	return filepath.Clean(path)
}

func prepareRFBUnixSocket(ctx context.Context, path string) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		logger.FromContext(ctx).Warn("failed to create display socket directory", slog.String("dir", dir), slog.Any("error", err))
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logger.FromContext(ctx).Warn("failed to remove stale display socket", slog.String("path", path), slog.Any("error", err))
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
	runDisplayCommand(ctx, xsetrootPath, "-solid", "#315f7d")
	startDisplayCommand(ctx, "window manager", twmPath)
	startDisplayCommand(ctx, "terminal", xtermPath,
		"-geometry", "100x30+28+28",
		"-title", "Memoh Workspace",
		"-e", "/bin/sh", "-c", "cd /data 2>/dev/null || cd /; exec /bin/sh",
	)
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
	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec // path is a fixed runtime bundle executable
	cmd.Env = withDisplayEnv(os.Environ())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logger.FromContext(ctx).Warn("display helper failed", slog.String("path", path), slog.Any("error", err))
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
