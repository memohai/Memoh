package aidevengine

import (
	"context"
	"errors"
	"testing"
)

func TestStatusReportsCodexCLIAvailable(t *testing.T) {
	service := NewService()
	service.detectCodexVersion = func(context.Context) (string, error) {
		return "codex-cli 0.132.0\n", nil
	}

	status := service.Status(context.Background())

	if status.Status != "available" {
		t.Fatalf("expected status available, got %q", status.Status)
	}
	if status.AuthStatus != "not_configured" {
		t.Fatalf("expected auth status not_configured, got %q", status.AuthStatus)
	}
	if status.DisplayName != displayName {
		t.Fatalf("expected display name %q, got %q", displayName, status.DisplayName)
	}
	if status.Version != "codex-cli 0.132.0" {
		t.Fatalf("expected trimmed version, got %q", status.Version)
	}
	if status.ErrorSummary != "" {
		t.Fatalf("expected empty error summary, got %q", status.ErrorSummary)
	}
}

func TestStatusReportsCodexCLIUnavailable(t *testing.T) {
	service := NewService()
	service.detectCodexVersion = func(context.Context) (string, error) {
		return "", errors.New("access denied while running version check")
	}

	status := service.Status(context.Background())

	if status.Status != "unavailable" {
		t.Fatalf("expected status unavailable, got %q", status.Status)
	}
	if status.Version != "" {
		t.Fatalf("expected empty version, got %q", status.Version)
	}
	if status.ErrorSummary == "" {
		t.Fatal("expected safe error summary")
	}
}

func TestCapabilitiesRemainDisabled(t *testing.T) {
	service := NewService()

	capabilities := service.Capabilities(context.Background())

	for _, capability := range capabilities.Capabilities {
		if capability.Enabled {
			t.Fatalf("expected capability %q to remain disabled", capability.Key)
		}
	}
}
