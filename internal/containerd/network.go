package containerd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containerd/containerd/v2/client"
	gocni "github.com/containerd/go-cni"
)

func setupCNINetwork(ctx context.Context, task client.Task, containerID string, cniBinDir string, cniConfDir string) (string, error) {
	if task == nil {
		return "", ErrInvalidArgument
	}
	if containerID == "" {
		containerID = task.ID()
	}
	if containerID == "" {
		return "", ErrInvalidArgument
	}

	pid := task.Pid()
	if pid == 0 {
		return "", fmt.Errorf("task pid not available for %s", containerID)
	}

	if _, err := os.Stat(cniConfDir); err != nil {
		return "", fmt.Errorf("cni config dir missing: %s: %w", cniConfDir, err)
	}
	if _, err := os.Stat(cniBinDir); err != nil {
		return "", fmt.Errorf("cni bin dir missing: %s: %w", cniBinDir, err)
	}
	netnsPath := filepath.Join("/proc", strconv.FormatUint(uint64(pid), 10), "ns", "net")
	if _, err := os.Stat(netnsPath); err != nil {
		return "", fmt.Errorf("netns not found: %s: %w", netnsPath, err)
	}

	cni, err := gocni.New(
		gocni.WithPluginDir([]string{cniBinDir}),
		gocni.WithPluginConfDir(cniConfDir),
	)
	if err != nil {
		return "", err
	}
	if err := cni.Load(gocni.WithLoNetwork, gocni.WithDefaultConf); err != nil {
		return "", err
	}
	result, err := cni.Setup(ctx, containerID, netnsPath)
	if err != nil {
		if !isDuplicateAllocationError(err) && !isVethExistsError(err) {
			return "", err
		}
		// Stale IPAM allocation or veth exists (e.g. after container restart with persisted
		// /var/lib/cni). Remove may fail if the previous iptables/veth state
		// is already gone; ignore the error so the retry Setup still runs.
		_ = cni.Remove(ctx, containerID, netnsPath)
		result, err = cni.Setup(ctx, containerID, netnsPath)
		if err != nil {
			return "", err
		}
	}
	ip := extractIP(result)
	if ip == "" {
		return "", fmt.Errorf("cni setup returned no usable IP for %s", containerID)
	}
	return ip, nil
}

func extractIP(result *gocni.Result) string {
	if result == nil {
		return ""
	}
	for _, cfg := range result.Interfaces {
		for _, ipCfg := range cfg.IPConfigs {
			if ipCfg.IP != nil {
				ip := ipCfg.IP.String()
				if ip != "" && ip != "127.0.0.1" && ip != "::1" {
					return ip
				}
			}
		}
	}
	return ""
}

func removeCNINetwork(ctx context.Context, task client.Task, containerID string, cniBinDir string, cniConfDir string) error {
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

	if _, err := os.Stat(cniConfDir); err != nil {
		return fmt.Errorf("cni config dir missing: %s: %w", cniConfDir, err)
	}
	if _, err := os.Stat(cniBinDir); err != nil {
		return fmt.Errorf("cni bin dir missing: %s: %w", cniBinDir, err)
	}

	netnsPath := filepath.Join("/proc", strconv.FormatUint(uint64(pid), 10), "ns", "net")
	if _, err := os.Stat(netnsPath); err != nil {
		return fmt.Errorf("netns not found: %s: %w", netnsPath, err)
	}

	cni, err := gocni.New(
		gocni.WithPluginDir([]string{cniBinDir}),
		gocni.WithPluginConfDir(cniConfDir),
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

// isVethExistsError returns true if the CNI setup failed because veth devices
// already exist (e.g. after container restart with stale network state).
func isVethExistsError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "already exists")
}
