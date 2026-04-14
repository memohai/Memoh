package network

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/errdefs"

	ctr "github.com/memohai/memoh/internal/containerd"
)

const (
	overlayLabelManaged    = "memoh.network.managed"
	overlayLabelBotID      = "memoh.network.bot_id"
	overlayLabelKind       = "memoh.network.provider_kind"
	overlayLabelConfigHash = "memoh.network.config_hash"
)

type nativeClientRuntime interface {
	CreateContainer(ctx context.Context, req ctr.CreateContainerRequest) (ctr.ContainerInfo, error)
	GetContainer(ctx context.Context, id string) (ctr.ContainerInfo, error)
	DeleteContainer(ctx context.Context, id string, opts *ctr.DeleteContainerOptions) error
	StartContainer(ctx context.Context, containerID string, opts *ctr.StartTaskOptions) error
	StopContainer(ctx context.Context, containerID string, opts *ctr.StopTaskOptions) error
	DeleteTask(ctx context.Context, containerID string, opts *ctr.DeleteTaskOptions) error
	GetTaskInfo(ctx context.Context, containerID string) (ctr.TaskInfo, error)
}

type nativeClientDriver struct {
	kind      string
	config    BotNetworkConfig
	runtime   nativeClientRuntime
	stateRoot string
}

type nativeSidecarSpec struct {
	image        string
	cmd          []string
	env          []string
	mounts       []ctr.MountSpec
	addedCaps    []string
	proxyAddress string
	details      map[string]any
}

func (d *nativeClientDriver) Kind() string { return d.kind }

func (d *nativeClientDriver) EnsureAttached(ctx context.Context, req AttachmentRequest) (OverlayStatus, error) {
	if !req.Overlay.Enabled || !d.config.Enabled {
		return OverlayStatus{
			Provider: d.kind,
			State:    "disabled",
		}, nil
	}
	if d.runtime == nil {
		return OverlayStatus{}, fmt.Errorf("%s overlay runtime is not configured", d.kind)
	}
	if strings.TrimSpace(req.Runtime.NetNSPath) == "" {
		return OverlayStatus{}, fmt.Errorf("%s overlay requires workspace network namespace path", d.kind)
	}
	spec, err := d.buildSpec(req)
	if err != nil {
		return OverlayStatus{}, err
	}
	sidecarID := d.sidecarID(req)
	if err := d.ensureSidecarContainer(ctx, sidecarID, req, spec); err != nil {
		return OverlayStatus{}, err
	}
	return d.Status(ctx, req)
}

func (d *nativeClientDriver) Detach(ctx context.Context, req AttachmentRequest) error {
	if d.runtime == nil {
		return nil
	}
	sidecarID := d.sidecarID(req)
	if err := d.runtime.StopContainer(ctx, sidecarID, &ctr.StopTaskOptions{
		Signal:  syscall.SIGTERM,
		Timeout: 10 * time.Second,
		Force:   true,
	}); err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	if err := d.runtime.DeleteTask(ctx, sidecarID, &ctr.DeleteTaskOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	if err := d.runtime.DeleteContainer(ctx, sidecarID, &ctr.DeleteContainerOptions{CleanupSnapshot: true}); err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	return nil
}

func (d *nativeClientDriver) Status(ctx context.Context, req AttachmentRequest) (OverlayStatus, error) {
	if !req.Overlay.Enabled || !d.config.Enabled {
		return OverlayStatus{
			Provider: d.kind,
			State:    "disabled",
		}, nil
	}
	sidecarID := d.sidecarID(req)
	task, err := d.runtime.GetTaskInfo(ctx, sidecarID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return OverlayStatus{
				Provider: d.kind,
				State:    "missing",
				Message:  "overlay sidecar is not created",
			}, nil
		}
		return OverlayStatus{}, err
	}
	attached := task.Status == ctr.TaskStatusRunning
	state := "stopped"
	if attached {
		state = "ready"
	}
	status := OverlayStatus{
		Provider: d.kind,
		Attached: attached,
		State:    state,
		Details: map[string]any{
			"sidecar_id": sidecarID,
			"pid":        task.PID,
		},
	}
	spec, specErr := d.buildSpec(req)
	if specErr == nil {
		status.ProxyAddress = spec.proxyAddress
		status.Details = mergeStatusDetails(status.Details, spec.details)
	}
	if d.kind == "tailscale" && attached {
		d.enrichTailscaleStatus(ctx, req, &status)
	}
	return status, nil
}

func (d *nativeClientDriver) ensureSidecarContainer(ctx context.Context, sidecarID string, req AttachmentRequest, spec nativeSidecarSpec) error {
	configHash := specConfigHash(spec, req.Runtime.NetNSPath)
	needsCreate := false

	existing, err := d.runtime.GetContainer(ctx, sidecarID)
	if err != nil {
		if !errdefs.IsNotFound(err) {
			return err
		}
		needsCreate = true
	} else if existing.Labels[overlayLabelConfigHash] != configHash {
		// Config changed — tear down the old sidecar so it gets recreated.
		_ = d.runtime.StopContainer(ctx, sidecarID, &ctr.StopTaskOptions{Signal: syscall.SIGTERM, Timeout: 5 * time.Second, Force: true})
		_ = d.runtime.DeleteTask(ctx, sidecarID, &ctr.DeleteTaskOptions{Force: true})
		if err := d.runtime.DeleteContainer(ctx, sidecarID, &ctr.DeleteContainerOptions{CleanupSnapshot: true}); err != nil && !errdefs.IsNotFound(err) {
			return err
		}
		needsCreate = true
	}

	if needsCreate {
		if _, err := d.runtime.CreateContainer(ctx, ctr.CreateContainerRequest{
			ID:       sidecarID,
			ImageRef: spec.image,
			Labels: map[string]string{
				overlayLabelManaged:    "true",
				overlayLabelBotID:      req.BotID,
				overlayLabelKind:       d.kind,
				overlayLabelConfigHash: configHash,
			},
			Spec: ctr.ContainerSpec{
				Cmd:                  spec.cmd,
				Env:                  spec.env,
				User:                 "0",
				Mounts:               spec.mounts,
				NetworkNamespacePath: req.Runtime.NetNSPath,
				AddedCapabilities:    spec.addedCaps,
			},
		}); err != nil {
			return err
		}
	}

	task, err := d.runtime.GetTaskInfo(ctx, sidecarID)
	if err == nil && task.Status == ctr.TaskStatusRunning {
		return nil
	}
	if err == nil {
		if err := d.runtime.DeleteTask(ctx, sidecarID, &ctr.DeleteTaskOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
			return err
		}
	}
	return d.runtime.StartContainer(ctx, sidecarID, nil)
}

// specConfigHash computes a stable fingerprint of the sidecar spec so we can
// detect when the container needs to be recreated after a config change.
// netNSPath is included because the OCI spec embeds it at creation time; when
// the workspace task restarts with a new PID the old path becomes stale and
// the sidecar must be rebuilt.
func specConfigHash(spec nativeSidecarSpec, netNSPath string) string {
	h := sha256.New()
	h.Write([]byte(spec.image))
	h.Write([]byte{0})
	for _, c := range spec.cmd {
		h.Write([]byte(c))
		h.Write([]byte{0})
	}
	env := append([]string(nil), spec.env...)
	sort.Strings(env)
	for _, e := range env {
		h.Write([]byte(e))
		h.Write([]byte{0})
	}
	for _, cap := range spec.addedCaps {
		h.Write([]byte(cap))
		h.Write([]byte{0})
	}
	h.Write([]byte{0})
	h.Write([]byte(netNSPath))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func (d *nativeClientDriver) sidecarID(req AttachmentRequest) string {
	return fmt.Sprintf("%s-net-%s", req.ContainerID, d.kind)
}

func (d *nativeClientDriver) buildSpec(req AttachmentRequest) (nativeSidecarSpec, error) {
	switch d.kind {
	case "tailscale":
		return d.buildTailscaleSpec(req)
	case "netbird":
		return d.buildNetbirdSpec(req)
	default:
		return nativeSidecarSpec{}, fmt.Errorf("unsupported native driver %q", d.kind)
	}
}

func (d *nativeClientDriver) buildTailscaleSpec(req AttachmentRequest) (nativeSidecarSpec, error) {
	cfg := d.config.Config
	authKey := stringConfig(cfg, "auth_key")
	userspace := boolConfig(cfg, "userspace")
	stateDir := d.tailscaleStateDir(req)
	socketDir := d.tailscaleSocketDir(req)
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		return nativeSidecarSpec{}, err
	}
	if err := os.MkdirAll(socketDir, 0o750); err != nil {
		return nativeSidecarSpec{}, err
	}
	proxyAddress := ""
	env := []string{
		"TS_STATE_DIR=/var/lib/tailscale",
		"TS_SOCKET=/var/run/tailscale/tailscaled.sock",
		"TS_USERSPACE=" + strconv.FormatBool(userspace),
		"TS_ENABLE_HEALTH_CHECK=true",
		"TS_LOCAL_ADDR_PORT=127.0.0.1:9002",
		"TS_AUTH_ONCE=true",
	}
	// auth_key is optional: when empty tailscaled starts in interactive-login
	// mode and exposes an AuthURL via LocalAPI for the user to complete OAuth.
	if authKey != "" {
		env = append(env, "TS_AUTHKEY="+authKey)
	}
	if hostname := firstNonEmptyString(stringConfig(cfg, "hostname"), req.BotID); hostname != "" {
		env = append(env, "TS_HOSTNAME="+hostname)
	}
	if routes := stringConfig(cfg, "advertise_routes"); routes != "" {
		env = append(env, "TS_ROUTES="+routes)
	}
	if boolConfig(cfg, "accept_dns", true) {
		env = append(env, "TS_ACCEPT_DNS=true")
	}
	extraArgs := make([]string, 0, 4)
	if controlURL := stringConfig(cfg, "control_url"); controlURL != "" {
		extraArgs = append(extraArgs, "--login-server="+controlURL)
	}
	extraArgs = append(extraArgs, "--accept-routes="+strconv.FormatBool(boolConfig(cfg, "accept_routes")))
	exitNode := stringConfig(req.Overlay.Config, "exit_node")
	if exitNode != "" {
		extraArgs = append(extraArgs, "--exit-node="+exitNode)
	}
	if more := strings.TrimSpace(stringConfig(cfg, "extra_args")); more != "" {
		extraArgs = append(extraArgs, more)
	}
	if len(extraArgs) > 0 {
		env = append(env, "TS_EXTRA_ARGS="+strings.Join(extraArgs, " "))
	}
	tailscaledExtraArgs := make([]string, 0, 4)
	if userspace {
		tailscaledExtraArgs = append(tailscaledExtraArgs, "--tun=userspace-networking")
	}
	if boolConfig(cfg, "socks5_enabled") {
		port := intConfig(cfg, "socks5_port", 1055)
		env = append(env, "TS_SOCKS5_SERVER=:"+strconv.Itoa(port))
		proxyAddress = "socks5://127.0.0.1:" + strconv.Itoa(port)
	}
	if boolConfig(cfg, "http_proxy_enabled") {
		port := intConfig(cfg, "http_proxy_port", 1056)
		env = append(env, "TS_OUTBOUND_HTTP_PROXY_LISTEN=:"+strconv.Itoa(port))
		if proxyAddress == "" {
			proxyAddress = "http://127.0.0.1:" + strconv.Itoa(port)
		}
	}
	if len(tailscaledExtraArgs) > 0 {
		env = append(env, "TS_TAILSCALED_EXTRA_ARGS="+strings.Join(tailscaledExtraArgs, " "))
	}
	mounts := []ctr.MountSpec{{
		Destination: "/var/lib/tailscale",
		Type:        "bind",
		Source:      stateDir,
		Options:     []string{"rbind", "rw"},
	}, {
		Destination: "/var/run/tailscale",
		Type:        "bind",
		Source:      socketDir,
		Options:     []string{"rbind", "rw"},
	}}
	addedCaps := []string(nil)
	if !userspace {
		mounts = append(mounts, ctr.MountSpec{
			Destination: "/dev/net/tun",
			Type:        "bind",
			Source:      "/dev/net/tun",
			Options:     []string{"rbind", "rw"},
		})
		addedCaps = append(addedCaps, "CAP_NET_ADMIN")
	}
	return nativeSidecarSpec{
		image:        "tailscale/tailscale:stable",
		env:          env,
		mounts:       mounts,
		addedCaps:    addedCaps,
		proxyAddress: proxyAddress,
		details: map[string]any{
			"userspace":                 userspace,
			"state_dir":                 stateDir,
			"localapi_socket_host_path": d.tailscaleSocketPath(req),
			"healthcheck_url":           "http://127.0.0.1:9002/healthz",
			"configured_exit_node":      exitNode,
		},
	}, nil
}

func (d *nativeClientDriver) buildNetbirdSpec(req AttachmentRequest) (nativeSidecarSpec, error) {
	cfg := d.config.Config
	setupKey := stringConfig(cfg, "setup_key")
	userspace := boolConfig(cfg, "userspace")
	stateDir := filepath.Join(d.stateRoot, req.BotID, "netbird")
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		return nativeSidecarSpec{}, err
	}
	env := []string{
		"NB_LOG_LEVEL=info",
	}
	// setup_key is optional: when empty NetBird starts without pre-auth and
	// may require interactive SSO login via management UI.
	if setupKey != "" {
		env = append(env, "NB_SETUP_KEY="+setupKey)
	}
	if hostname := firstNonEmptyString(stringConfig(cfg, "hostname"), req.BotID); hostname != "" {
		env = append(env, "NB_HOSTNAME="+hostname)
	}
	if managementURL := stringConfig(cfg, "management_url"); managementURL != "" {
		env = append(env, "NB_MANAGEMENT_URL="+managementURL)
	}
	if adminURL := stringConfig(cfg, "admin_url"); adminURL != "" {
		env = append(env, "NB_ADMIN_URL="+adminURL)
	}
	if boolConfig(cfg, "disable_dns") {
		env = append(env, "NB_DISABLE_DNS=true")
	}
	if extraArgs := stringConfig(cfg, "extra_args"); extraArgs != "" {
		env = append(env, "NB_EXTRA_ARGS="+extraArgs)
	}
	mounts := []ctr.MountSpec{{
		Destination: "/var/lib/netbird",
		Type:        "bind",
		Source:      stateDir,
		Options:     []string{"rbind", "rw"},
	}}
	addedCaps := []string(nil)
	if !userspace {
		mounts = append(mounts, ctr.MountSpec{
			Destination: "/dev/net/tun",
			Type:        "bind",
			Source:      "/dev/net/tun",
			Options:     []string{"rbind", "rw"},
		})
		addedCaps = append(addedCaps, "CAP_NET_ADMIN")
	}
	return nativeSidecarSpec{
		image:     "netbirdio/netbird:latest",
		env:       env,
		mounts:    mounts,
		addedCaps: addedCaps,
		details: map[string]any{
			"userspace": userspace,
			"state_dir": stateDir,
		},
	}, nil
}

func stringConfig(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	value, ok := config[key]
	if !ok {
		return ""
	}
	s, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func boolConfig(config map[string]any, key string, defaults ...bool) bool {
	if config != nil {
		if value, ok := config[key]; ok {
			if b, ok := value.(bool); ok {
				return b
			}
		}
	}
	if len(defaults) > 0 {
		return defaults[0]
	}
	return false
}

func intConfig(config map[string]any, key string, defaultValue int) int {
	if config != nil {
		if value, ok := config[key]; ok {
			switch typed := value.(type) {
			case float64:
				return int(typed)
			case int:
				return typed
			}
		}
	}
	return defaultValue
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (d *nativeClientDriver) tailscaleStateDir(req AttachmentRequest) string {
	return filepath.Join(d.stateRoot, req.BotID, "tailscale")
}

func (d *nativeClientDriver) tailscaleSocketDir(req AttachmentRequest) string {
	return filepath.Join(d.tailscaleStateDir(req), "run")
}

func (d *nativeClientDriver) tailscaleSocketPath(req AttachmentRequest) string {
	return filepath.Join(d.tailscaleSocketDir(req), "tailscaled.sock")
}

func (d *nativeClientDriver) enrichTailscaleStatus(ctx context.Context, req AttachmentRequest, status *OverlayStatus) {
	localStatus, err := readTailscaleLocalStatus(ctx, d.tailscaleSocketPath(req))
	if err != nil {
		status.Details = mergeStatusDetails(status.Details, map[string]any{
			"localapi_error": err.Error(),
		})
		status.State = "starting"
		status.Message = "tailscaled is running but LocalAPI is not ready yet"
		return
	}
	// NeedsLogin: tailscaled is running but waiting for interactive auth.
	if localStatus.BackendState == "NeedsLogin" || localStatus.AuthURL != "" {
		status.State = "needs_login"
		status.Message = "Open the authentication URL to complete login."
		if localStatus.AuthURL != "" {
			status.Details = mergeStatusDetails(status.Details, map[string]any{
				"auth_url":      localStatus.AuthURL,
				"backend_state": localStatus.BackendState,
			})
		}
		return
	}
	if localStatus.Self != nil {
		if len(localStatus.Self.TailscaleIPs) > 0 {
			status.NetworkIP = strings.TrimSpace(localStatus.Self.TailscaleIPs[0])
		}
		status.Details = mergeStatusDetails(status.Details, map[string]any{
			"dns_name":      localStatus.Self.DNSName,
			"hostname":      localStatus.Self.HostName,
			"online":        localStatus.Self.Online,
			"tailscale_ips": localStatus.Self.TailscaleIPs,
		})
	}
	if localStatus.BackendState != "" {
		status.Details = mergeStatusDetails(status.Details, map[string]any{
			"backend_state": localStatus.BackendState,
		})
		if localStatus.BackendState != "Running" && status.State == "ready" {
			status.State = strings.ToLower(localStatus.BackendState)
		}
	}
	if len(localStatus.Health) > 0 {
		status.Details = mergeStatusDetails(status.Details, map[string]any{
			"health": localStatus.Health,
		})
		if status.State == "ready" {
			status.State = "degraded"
			status.Message = localStatus.Health[0]
		}
	}
	if status.NetworkIP == "" && status.State == "ready" {
		status.State = "starting"
		status.Message = "tailscaled is starting and has not received a tailnet IP yet"
	}
}

func (d *nativeClientDriver) listTailscaleNodes(ctx context.Context, botID string) ([]NodeOption, error) {
	req := AttachmentRequest{BotID: botID}
	localStatus, err := readTailscaleLocalStatusWithPeers(ctx, d.tailscaleSocketPath(req), true)
	if err != nil {
		return nil, err
	}
	selectedValue := stringConfig(d.config.Config, "exit_node")
	items := make([]NodeOption, 0, len(localStatus.Peer))
	for peerKey, peer := range localStatus.Peer {
		if peer == nil || !peer.ExitNodeOption {
			continue
		}
		addresses := append([]string(nil), peer.TailscaleIPs...)
		value := firstNonEmptyString(append(addresses, strings.TrimSuffix(strings.TrimSpace(peer.DNSName), "."), strings.TrimSpace(peer.HostName), strings.TrimSpace(peer.ID), strings.TrimSpace(peerKey))...)
		displayName := firstNonEmptyString(strings.TrimSpace(peer.HostName), strings.TrimSuffix(strings.TrimSpace(peer.DNSName), "."), value)
		items = append(items, NodeOption{
			ID:          firstNonEmptyString(strings.TrimSpace(peer.ID), strings.TrimSpace(peerKey), value),
			Value:       value,
			DisplayName: displayName,
			Description: strings.TrimSuffix(strings.TrimSpace(peer.DNSName), "."),
			Online:      peer.Online,
			Addresses:   addresses,
			CanExitNode: peer.ExitNodeOption,
			Selected:    selectedValue != "" && selectedValue == value,
			Details: map[string]any{
				"dns_name": strings.TrimSpace(peer.DNSName),
				"hostname": strings.TrimSpace(peer.HostName),
				"os":       strings.TrimSpace(peer.OS),
				"active":   peer.Active,
			},
		})
	}
	return items, nil
}

func mergeStatusDetails(base map[string]any, extra map[string]any) map[string]any {
	out := cloneMap(base)
	for key, value := range extra {
		out[key] = value
	}
	return out
}
