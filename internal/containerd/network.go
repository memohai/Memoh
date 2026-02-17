package containerd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/containerd/containerd/v2/client"
	gocni "github.com/containerd/go-cni"
)

// Default CNI config and binary directories on Linux.
const (
	defaultCNIConfDir = "/etc/cni/net.d"
	defaultCNIBinDir  = "/opt/cni/bin"
)

// SetupNetwork attaches CNI networking to a running task.
func SetupNetwork(ctx context.Context, task client.Task, containerID string) error {
	if task == nil {
		return ErrInvalidArgument
	}
	if containerID == "" {
		containerID = task.ID()
	}
	if containerID == "" {
		return ErrInvalidArgument
	}

	pid := task.Pid()
	if pid == 0 {
		return fmt.Errorf("task pid not available for %s", containerID)
	}
	if runtime.GOOS == "darwin" {
		return setupNetworkWithCLI(ctx, containerID, pid)
	}

	if _, err := os.Stat(defaultCNIConfDir); err != nil {
		return fmt.Errorf("cni config dir missing: %s: %w", defaultCNIConfDir, err)
	}
	if _, err := os.Stat(defaultCNIBinDir); err != nil {
		return fmt.Errorf("cni bin dir missing: %s: %w", defaultCNIBinDir, err)
	}
	netnsPath := filepath.Join("/proc", strconv.FormatUint(uint64(pid), 10), "ns", "net")
	if _, err := os.Stat(netnsPath); err != nil {
		return fmt.Errorf("netns not found: %s: %w", netnsPath, err)
	}

	cni, err := gocni.New(
		gocni.WithPluginDir([]string{defaultCNIBinDir}),
		gocni.WithPluginConfDir(defaultCNIConfDir),
	)
	if err != nil {
		return err
	}
	if err := cni.Load(gocni.WithLoNetwork, gocni.WithDefaultConf); err != nil {
		return err
	}
	_, err = cni.Setup(ctx, containerID, netnsPath)
	if err == nil {
		return nil
	}
	if !isDuplicateAllocationError(err) {
		return err
	}
	if rmErr := cni.Remove(ctx, containerID, netnsPath); rmErr != nil {
		return rmErr
	}
	_, err = cni.Setup(ctx, containerID, netnsPath)
	return err
}

func setupNetworkWithCLI(ctx context.Context, containerID string, pid uint32) error {
	args := []string{
		"shell",
		"--tty=false",
		"default",
		"--",
		"sudo",
		"-n",
		"memoh-cli",
		"cni-setup",
		"--id", containerID,
		"--pid", strconv.FormatUint(uint64(pid), 10),
		"--conf-dir", defaultCNIConfDir,
		"--bin-dir", defaultCNIBinDir,
	}
	cmd := exec.CommandContext(ctx, "limactl", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			cniErr := fmt.Errorf("cni cli failed: %s", strings.TrimSpace(stderr.String()))
			if !isDuplicateAllocationError(cniErr) {
				return cniErr
			}
		} else if !isDuplicateAllocationError(err) {
			return err
		}
		if rmErr := removeNetworkWithCLI(ctx, containerID, pid); rmErr != nil {
			return rmErr
		}
		cmd = exec.CommandContext(ctx, "limactl", args...)
		stderr.Reset()
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			if stderr.Len() > 0 {
				return fmt.Errorf("cni cli failed: %s", strings.TrimSpace(stderr.String()))
			}
			return err
		}
	}
	return nil
}

// RemoveNetwork detaches CNI networking for a running task.
func RemoveNetwork(ctx context.Context, task client.Task, containerID string) error {
	if task == nil {
		return ErrInvalidArgument
	}
	if containerID == "" {
		containerID = task.ID()
	}
	if containerID == "" {
		return ErrInvalidArgument
	}

	pid := task.Pid()
	if pid == 0 {
		return fmt.Errorf("task pid not available for %s", containerID)
	}
	if runtime.GOOS == "darwin" {
		return removeNetworkWithCLI(ctx, containerID, pid)
	}

	if _, err := os.Stat(defaultCNIConfDir); err != nil {
		return fmt.Errorf("cni config dir missing: %s: %w", defaultCNIConfDir, err)
	}
	if _, err := os.Stat(defaultCNIBinDir); err != nil {
		return fmt.Errorf("cni bin dir missing: %s: %w", defaultCNIBinDir, err)
	}

	netnsPath := filepath.Join("/proc", strconv.FormatUint(uint64(pid), 10), "ns", "net")
	if _, err := os.Stat(netnsPath); err != nil {
		return fmt.Errorf("netns not found: %s: %w", netnsPath, err)
	}

	cni, err := gocni.New(
		gocni.WithPluginDir([]string{defaultCNIBinDir}),
		gocni.WithPluginConfDir(defaultCNIConfDir),
	)
	if err != nil {
		return err
	}
	if err := cni.Load(gocni.WithLoNetwork, gocni.WithDefaultConf); err != nil {
		return err
	}
	return cni.Remove(ctx, containerID, netnsPath)
}

func removeNetworkWithCLI(ctx context.Context, containerID string, pid uint32) error {
	args := []string{
		"shell",
		"--tty=false",
		"default",
		"--",
		"sudo",
		"-n",
		"memoh-cli",
		"cni-remove",
		"--id", containerID,
		"--pid", strconv.FormatUint(uint64(pid), 10),
		"--conf-dir", defaultCNIConfDir,
		"--bin-dir", defaultCNIBinDir,
	}
	cmd := exec.CommandContext(ctx, "limactl", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("cni cli failed: %s", strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

func isDuplicateAllocationError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "duplicate allocation")
}
