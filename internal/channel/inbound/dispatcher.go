package inbound

import (
	"log/slog"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// InboundMode determines how a new inbound message is handled when an agent
// stream is already active for the same route.
type InboundMode int

const (
	// ModeInject (default, command /btw) injects the message into the active
	// agent stream via the PrepareStep hook so the LLM sees it between tool
	// rounds. When no stream is active, starts one normally.
	ModeInject InboundMode = iota
	// ModeParallel (command /now) starts a new agent stream immediately,
	// running concurrently with any existing stream.
	ModeParallel
	// ModeQueue (command /next) queues the message and processes it after the
	// current agent stream completes.
	ModeQueue
)

// RouteDispatcher owns the linearized lifecycle for work on each route.
type RouteDispatcher struct {
	lifecycleMu    sync.Mutex
	routeLifecycle map[string]*routeLifecycleState
	nextLeaseID    uint64
	nextEpoch      uint64
}

// NewRouteDispatcher creates a route lifecycle dispatcher.
func NewRouteDispatcher(_ *slog.Logger) *RouteDispatcher {
	return &RouteDispatcher{
		routeLifecycle: make(map[string]*routeLifecycleState),
	}
}

// DetectMode parses a message prefix to determine the inbound mode.
// Returns the mode and the text with the prefix stripped.
func DetectMode(text string) (InboundMode, string) {
	if mode, remainder, ok := parseModeCommand(text); ok {
		return mode, remainder
	}
	return ModeInject, strings.TrimSpace(text)
}

// IsModeCommand reports whether the text is a mode-prefix command
// (/btw, /now, /next), so the generic command handler should skip it.
func IsModeCommand(text string) bool {
	_, _, ok := parseModeCommand(text)
	return ok
}

func parseModeCommand(text string) (InboundMode, string, bool) {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)
	for _, candidate := range []struct {
		command string
		mode    InboundMode
	}{
		{command: "/now", mode: ModeParallel},
		{command: "/next", mode: ModeQueue},
		{command: "/btw", mode: ModeInject},
	} {
		if lower == candidate.command {
			return candidate.mode, "", true
		}
		if !strings.HasPrefix(lower, candidate.command) {
			continue
		}
		remainder := trimmed[len(candidate.command):]
		delimiter, _ := utf8.DecodeRuneInString(remainder)
		if unicode.IsSpace(delimiter) {
			return candidate.mode, strings.TrimSpace(remainder), true
		}
	}
	return ModeInject, trimmed, false
}
