package acpclient

import (
	"strings"

	"github.com/memohai/memoh/internal/agent/event"
	"github.com/memohai/memoh/internal/contextlimit"
)

type ToolOutputLimit = contextlimit.ToolOutputLimit

func LimitStreamEvent(ev event.StreamEvent, limit ToolOutputLimit) event.StreamEvent {
	if !hasToolOutputLimit(limit) || ev.Type != event.ToolCallEnd {
		return ev
	}
	label := "tool result (" + ev.ToolName + ")"
	ev.Result = contextlimit.LimitToolOutput(ev.Result, label, limit)
	if strings.TrimSpace(ev.Error) != "" {
		ev.Error = contextlimit.LimitString(ev.Error, label, limit)
	}
	return ev
}

func hasToolOutputLimit(limit ToolOutputLimit) bool {
	return limit.MaxBytes > 0 || limit.MaxLines > 0
}

func normalizedToolOutputLimit(limit ToolOutputLimit) ToolOutputLimit {
	return contextlimit.NormalizedLimit(limit)
}

func limitToolOutputString(text, label string, limit ToolOutputLimit) string {
	return contextlimit.LimitString(text, label, limit)
}
