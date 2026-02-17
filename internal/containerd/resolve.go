package containerd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Paths used when resolving /etc/resolv.conf source.
const (
	systemdResolvConf = "/run/systemd/resolve/resolv.conf"
	fallbackResolv    = "nameserver 1.1.1.1\nnameserver 8.8.8.8\n"
)

// ResolveConfSource returns a host path to mount as /etc/resolv.conf.
// If systemd-resolved config is available, use it. Otherwise write a fallback
// resolv.conf under dataDir and return that path.
func ResolveConfSource(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", ErrInvalidArgument
	}
	if runtime.GOOS == "darwin" {
		if ok, err := limaFileExists(systemdResolvConf); err != nil {
			return "", err
		} else if ok {
			return systemdResolvConf, nil
		}
	} else if _, err := os.Stat(systemdResolvConf); err == nil {
		return systemdResolvConf, nil
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", err
	}
	fallbackPath := filepath.Join(dataDir, "resolv.conf")
	if _, err := os.Stat(fallbackPath); err == nil {
		return fallbackPath, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.WriteFile(fallbackPath, []byte(fallbackResolv), 0o644); err != nil {
		return "", err
	}
	return fallbackPath, nil
}

func limaFileExists(path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, ErrInvalidArgument
	}
	cmd := exec.Command(
		"limactl",
		"shell",
		"--tty=false",
		"default",
		"--",
		"test",
		"-f",
		path,
	)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() == 1 {
			return false, nil
		}
	}
	return false, fmt.Errorf("lima test failed for %s: %w", path, err)
}
