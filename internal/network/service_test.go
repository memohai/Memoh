package network

import (
	"testing"
)

func TestPrepareBotConfigForWriteAllowsDisabledInvalidProviderDraft(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(newTailscaleProvider(nil, t.TempDir())); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	svc := &Service{registry: registry}

	cfg, err := svc.PrepareBotConfigForWrite(BotOverlayConfig{
		Enabled:  false,
		Provider: "tailscale",
		// Missing auth_key, which would be invalid when enabled.
		Config: map[string]any{},
	})
	if err != nil {
		t.Fatalf("PrepareBotConfigForWrite returned error: %v", err)
	}
	if cfg.Provider != "tailscale" {
		t.Fatalf("expected provider draft to be preserved, got %+v", cfg)
	}
}

func TestPrepareBotConfigForWriteRejectsExitNodeWithUserspaceEnabled(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(newTailscaleProvider(nil, t.TempDir())); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	svc := &Service{registry: registry}

	_, err := svc.PrepareBotConfigForWrite(BotOverlayConfig{
		Enabled:  true,
		Provider: "tailscale",
		Config: map[string]any{
			"auth_key":  "tskey-test",
			"userspace": true,
			"exit_node": "100.64.0.10",
		},
	})
	if err == nil {
		t.Fatal("expected exit node + userspace config to be rejected")
	}
}

func TestPrepareBotConfigForWriteAllowsExitNodeWithKernelTUN(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(newTailscaleProvider(nil, t.TempDir())); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	svc := &Service{registry: registry}

	cfg, err := svc.PrepareBotConfigForWrite(BotOverlayConfig{
		Enabled:  true,
		Provider: "tailscale",
		Config: map[string]any{
			"auth_key":  "tskey-test",
			"userspace": false,
			"exit_node": "100.64.0.10",
		},
	})
	if err != nil {
		t.Fatalf("PrepareBotConfigForWrite returned error: %v", err)
	}
	if cfg.Provider != "tailscale" {
		t.Fatalf("unexpected provider: %+v", cfg)
	}
}
