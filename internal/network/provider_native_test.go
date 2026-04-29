package network

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeProviderStatusRejectsExitNodeWithUserspace(t *testing.T) {
	provider := newTailscaleProvider(nil, t.TempDir())

	status, err := provider.Status(context.Background(), BotOverlayConfig{
		Enabled:  true,
		Provider: "tailscale",
		Config: map[string]any{
			"auth_key":  "tskey-test",
			"userspace": true,
			"exit_node": "100.64.0.10",
		},
	})
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if status.State != StatusStateNeedsConfig {
		t.Fatalf("expected needs config, got %s", status.State)
	}
	if !strings.Contains(status.Description, "userspace=false") {
		t.Fatalf("expected userspace hint, got %q", status.Description)
	}
}

func TestNativeProviderStatusAllowsExitNodeWithKernelTUN(t *testing.T) {
	provider := newTailscaleProvider(nil, t.TempDir())

	status, err := provider.Status(context.Background(), BotOverlayConfig{
		Enabled:  true,
		Provider: "tailscale",
		Config: map[string]any{
			"auth_key":  "tskey-test",
			"userspace": false,
			"exit_node": "100.64.0.10",
		},
	})
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if status.State != StatusStateReady {
		t.Fatalf("expected ready, got %s: %s", status.State, status.Description)
	}
}

func TestNativeProviderStatusAllowsNoExitNode(t *testing.T) {
	provider := newTailscaleProvider(nil, t.TempDir())

	status, err := provider.Status(context.Background(), BotOverlayConfig{
		Enabled:  true,
		Provider: "tailscale",
		Config: map[string]any{
			"auth_key":  "tskey-test",
			"userspace": false,
		},
	})
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if status.State != StatusStateReady {
		t.Fatalf("expected ready, got %s", status.State)
	}
}

func TestNativeClientDriverBuildTailscaleSpecExitNodeArgs(t *testing.T) {
	driver := &nativeClientDriver{
		kind: "tailscale",
		config: BotOverlayConfig{
			Enabled:  true,
			Provider: "tailscale",
			Config: map[string]any{
				"auth_key":  "tskey-test",
				"userspace": false,
			},
		},
		stateRoot: t.TempDir(),
	}

	spec, err := driver.buildTailscaleSpec(AttachmentRequest{
		BotID: "bot-1",
		Overlay: BotOverlayConfig{
			Enabled:  true,
			Provider: "tailscale",
			Config: map[string]any{
				"exit_node": "100.64.0.10",
			},
		},
	})
	if err != nil {
		t.Fatalf("buildTailscaleSpec returned error: %v", err)
	}
	if spec.proxyAddress != "" {
		t.Fatalf("expected no explicit proxy address in transparent mode, got %q", spec.proxyAddress)
	}
	extraArgs := strings.Join(spec.env, " ")
	if !strings.Contains(extraArgs, "--exit-node=100.64.0.10") {
		t.Fatalf("expected exit node arg in env, got %q", extraArgs)
	}
}

func TestNativeClientDriverBuildTailscaleSpecNoExitNode(t *testing.T) {
	driver := &nativeClientDriver{
		kind: "tailscale",
		config: BotOverlayConfig{
			Enabled:  true,
			Provider: "tailscale",
			Config: map[string]any{
				"auth_key":  "tskey-test",
				"userspace": false,
			},
		},
		stateRoot: t.TempDir(),
	}

	spec, err := driver.buildTailscaleSpec(AttachmentRequest{
		BotID: "bot-1",
		Overlay: BotOverlayConfig{
			Enabled:  true,
			Provider: "tailscale",
			Config:   map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("buildTailscaleSpec returned error: %v", err)
	}
	if spec.proxyAddress != "" {
		t.Fatalf("expected no proxy address without exit node, got %q", spec.proxyAddress)
	}
	if env := strings.Join(spec.env, " "); strings.Contains(env, "--exit-node=") {
		t.Fatalf("did not expect exit node args without exit node configured, got %q", env)
	}
}

func TestNativeClientDriverBuildTailscaleSpecSocks5Enabled(t *testing.T) {
	driver := &nativeClientDriver{
		kind: "tailscale",
		config: BotOverlayConfig{
			Enabled:  true,
			Provider: "tailscale",
			Config: map[string]any{
				"auth_key":       "tskey-test",
				"userspace":      true,
				"socks5_enabled": true,
				"socks5_port":    float64(1055),
			},
		},
		stateRoot: t.TempDir(),
	}

	spec, err := driver.buildTailscaleSpec(AttachmentRequest{
		BotID: "bot-1",
		Overlay: BotOverlayConfig{
			Enabled:  true,
			Provider: "tailscale",
			Config:   map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("buildTailscaleSpec returned error: %v", err)
	}
	if !strings.Contains(spec.proxyAddress, "socks5://") {
		t.Fatalf("expected socks5 proxy address, got %q", spec.proxyAddress)
	}
	env := strings.Join(spec.env, " ")
	if !strings.Contains(env, "TS_SOCKS5_SERVER=:1055") {
		t.Fatalf("expected socks5 server env, got %q", env)
	}
}

func TestNativeClientDriverListTailscaleNodesFiltersExitNodes(t *testing.T) {
	tempDir, err := os.MkdirTemp("/tmp", "tsn-")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	t.Cleanup(func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			t.Logf("remove temp dir: %v", removeErr)
		}
	})
	driver := &nativeClientDriver{
		kind: "tailscale",
		config: BotOverlayConfig{
			Provider: "tailscale",
			Config: map[string]any{
				"exit_node": "100.64.0.10",
			},
		},
		stateRoot: tempDir,
	}
	socketPath := filepath.Join(tempDir, "bot-1", "tailscale", "run", "tailscaled.sock")
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o750); err != nil {
		t.Fatalf("mkdir socket dir: %v", err)
	}
	closeServer := startUnixJSONServer(t, socketPath, map[string]any{
		"BackendState": "Running",
		"Peer": map[string]any{
			"nodekey:exit": map[string]any{
				"ID":             "node-stable-id-1",
				"HostName":       "exit-a",
				"DNSName":        "exit-a.tailnet.ts.net.",
				"Online":         true,
				"TailscaleIPs":   []string{"100.64.0.10"},
				"OS":             "linux",
				"ExitNodeOption": true,
			},
			"nodekey:regular": map[string]any{
				"ID":             "node-stable-id-2",
				"HostName":       "worker-b",
				"DNSName":        "worker-b.tailnet.ts.net.",
				"Online":         true,
				"TailscaleIPs":   []string{"100.64.0.20"},
				"OS":             "linux",
				"ExitNodeOption": false,
			},
		},
	})
	defer closeServer()

	nodes, err := driver.listTailscaleNodes(context.Background(), "bot-1")
	if err != nil {
		t.Fatalf("listTailscaleNodes returned error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 exit node, got %d", len(nodes))
	}
	if nodes[0].Value != "100.64.0.10" {
		t.Fatalf("unexpected node value: %+v", nodes[0])
	}
	if !nodes[0].Selected {
		t.Fatalf("expected selected node, got %+v", nodes[0])
	}
}

func startUnixJSONServer(t *testing.T, socketPath string, payload map[string]any) func() {
	t.Helper()

	if err := os.RemoveAll(socketPath); err != nil {
		t.Fatalf("remove stale socket: %v", err)
	}
	var lc net.ListenConfig
	listener, err := lc.Listen(context.Background(), "unix", socketPath)
	if err != nil {
		t.Fatalf("listen on unix socket: %v", err)
	}
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
		}),
		ReadHeaderTimeout: 2 * time.Second,
	}
	go func() {
		_ = server.Serve(listener)
	}()
	return func() {
		_ = server.Close()
		_ = listener.Close()
	}
}
