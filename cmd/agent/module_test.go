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

func TestACPToolProvidersIncludeAskUser(t *testing.T) {
	providers := acpToolProviders([]agenttools.ToolProvider{
		agenttools.NewAskUserProvider(slog.Default()),
		agenttools.NewSkillProvider(slog.Default()),
	})

	foundAskUser := false
	for _, provider := range providers {
		if _, ok := provider.(*agenttools.AskUserProvider); ok {
			foundAskUser = true
		}
	}
	if !foundAskUser {
		t.Fatal("ask_user should be exposed to ACP")
	}
	if len(providers) != 2 {
		t.Fatalf("filtered providers = %d, want 2", len(providers))
	}
}
