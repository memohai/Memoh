package sessionmode

import "strings"

const (
	Chat      = "chat"
	Heartbeat = "heartbeat"
	Schedule  = "schedule"
	Subagent  = "subagent"
	Discuss   = "discuss"
	ACPAgent  = "acp_agent"

	// BackgroundDelivery is an internal agent run mode used while draining
	// proactive background notifications. It is not a persisted session type.
	BackgroundDelivery = "background_delivery"
)

// IsInteractive reports whether a run mode can pause and wait for user-facing
// approval or input. Discuss streams events to observers, but it does not have
// a chat-flow continuation path for deferred user input.
func IsInteractive(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", Chat, ACPAgent:
		return true
	default:
		return false
	}
}
