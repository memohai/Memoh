package agent

import (
	"log/slog"

	"github.com/memohai/memoh/internal/hooks"
	"github.com/memohai/memoh/internal/workspace/bridge"
)

// Deps holds all service dependencies for the Agent.
type Deps struct {
	BridgeProvider bridge.Provider
	HookService    *hooks.Service
	Logger         *slog.Logger
}
