package main

import (
	"log/slog"
	"testing"

	"go.uber.org/fx"

	agenttools "github.com/memohai/memoh/internal/agent/tools"
)

func TestFXOptionsValidate(t *testing.T) {
	if err := fx.ValidateApp(options()); err != nil {
		t.Fatal(err)
	}
}

func TestACPToolProvidersExcludeUnsupportedInteractiveTools(t *testing.T) {
	providers := acpToolProviders([]agenttools.ToolProvider{
		agenttools.NewAskUserProvider(slog.Default()),
		agenttools.NewSkillProvider(slog.Default()),
	})

	for _, provider := range providers {
		if _, ok := provider.(*agenttools.AskUserProvider); ok {
			t.Fatal("ask_user should not be exposed to ACP until ACP user-input pause/resume is implemented")
		}
	}
	if len(providers) != 1 {
		t.Fatalf("filtered providers = %d, want 1", len(providers))
	}
}
