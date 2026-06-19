// Package agentpayload defines the on-wire shapes for agent-emitted events
// forwarded to per-session SSE subscribers. Producers (cmd/agent and
// internal/conversation/flow) build their payloads via these helpers so the
// contract is exercised by one set of tests instead of being duplicated as
// map literals across packages.
//
// Any field added to the returned maps lands on the wire. The top-level
// `session_id` placement is load-bearing: internal/handlers/message_stream.go
// routes events by reading that key directly, without unwrapping nested
// objects.
package agentpayload

import "github.com/memohai/memoh/internal/agent/background"

// BackgroundTask builds the wire payload for a background task event. The
// publisher in cmd/agent's bgManager.SetEventFunc marshals this map and
// forwards it verbatim to the per-session SSE handler, which stamps `type`
// and `bot_id` on the way out.
func BackgroundTask(evt background.TaskEvent) map[string]any {
	return map[string]any{
		"event":      evt.Event,
		"session_id": evt.SessionID,
		"task":       evt,
	}
}

// AgentStream builds the wire payload for a server-initiated agent stream
// event. The publisher in internal/conversation/flow's resolver emits this
// shape; the per-session SSE handler routes by the top-level `session_id`
// and stamps `type` and `bot_id` on egress.
func AgentStream(sessionID string, stream map[string]any) map[string]any {
	return map[string]any{
		"session_id": sessionID,
		"stream":     stream,
	}
}
