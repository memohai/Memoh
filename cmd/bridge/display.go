package main

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/logger"
)

const (
	displayEnabledEnv = "MEMOH_DISPLAY_ENABLED"
	xvncPath          = "/opt/memoh/toolkit/display/bin/Xvnc"
	xkbcompPath       = "/opt/memoh/toolkit/display/bin/xkbcomp"
	systemXkbcompPath = "/usr/bin/xkbcomp"
	xvncDisplay       = ":99"
	xvncGeometry      = "1280x800"
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
		cmd := exec.CommandContext(ctx, xvncPath, xvncDisplay, "-geometry", xvncGeometry, "-depth", "24", "-SecurityTypes", "None") //nolint:gosec // path is a fixed runtime bundle executable
		cmd.Env = withDisplayEnv(os.Environ())
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			logger.FromContext(ctx).Warn("failed to start Xvnc", slog.Any("error", err))
		} else {
			logger.FromContext(ctx).Info("Xvnc display started", slog.Int("pid", cmd.Process.Pid), slog.String("display", xvncDisplay))
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
