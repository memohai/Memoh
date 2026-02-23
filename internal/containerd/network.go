package containerd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/v2/client"
	gocni "github.com/containerd/go-cni"
)

func setupCNINetwork(ctx context.Context, task client.Task, containerID string, CNIBinDir string, CNIConfDir string) error {
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

	if _, err := os.Stat(CNIConfDir); err != nil {
		return fmt.Errorf("cni config dir missing: %s: %w", CNIConfDir, err)
	}
	if _, err := os.Stat(CNIBinDir); err != nil {
		return fmt.Errorf("cni bin dir missing: %s: %w", CNIBinDir, err)
	}
	netnsPath := filepath.Join("/proc", fmt.Sprint(pid), "ns", "net")
	if _, err := os.Stat(netnsPath); err != nil {
		return fmt.Errorf("netns not found: %s: %w", netnsPath, err)
	}

	cni, err := gocni.New(
		gocni.WithPluginDir([]string{CNIBinDir}),
		gocni.WithPluginConfDir(CNIConfDir),
	)
	if err != nil {
		return err
	}
	if err := cni.Load(gocni.WithLoNetwork, gocni.WithDefaultConf); err != nil {
		return err
	}
	_, err = cni.Setup(ctx, containerID, netnsPath)
	if err != nil {
		if !isDuplicateAllocationError(err) {
			return err
		}
		if rmErr := cni.Remove(ctx, containerID, netnsPath); rmErr != nil {
			return rmErr
		}
		_, err = cni.Setup(ctx, containerID, netnsPath)
		if err != nil {
			return err
		}
	}
	return nil
}

func removeCNINetwork(ctx context.Context, task client.Task, containerID string, CNIBinDir string, CNIConfDir string) error {
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

	if _, err := os.Stat(CNIConfDir); err != nil {
		return fmt.Errorf("cni config dir missing: %s: %w", CNIConfDir, err)
	}
	if _, err := os.Stat(CNIBinDir); err != nil {
		return fmt.Errorf("cni bin dir missing: %s: %w", CNIBinDir, err)
	}

	netnsPath := filepath.Join("/proc", fmt.Sprint(pid), "ns", "net")
	if _, err := os.Stat(netnsPath); err != nil {
		return fmt.Errorf("netns not found: %s: %w", netnsPath, err)
	}

	cni, err := gocni.New(
		gocni.WithPluginDir([]string{CNIBinDir}),
		gocni.WithPluginConfDir(CNIConfDir),
	)
	if err != nil {
		return err
	}
	if err := cni.Load(gocni.WithLoNetwork, gocni.WithDefaultConf); err != nil {
		return err
	}
	return cni.Remove(ctx, containerID, netnsPath)
}

func isDuplicateAllocationError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "duplicate allocation")
}
