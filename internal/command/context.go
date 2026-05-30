package command

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// buildContextGroup registers /context — a focused token/context-window view for
// the current session (a Claude-Code-style budget card). It reuses the same
// session queries as /status; it is session-scoped, so the channel processor
// resolves the active session before dispatching (see isStatusCommand).
func (h *Handler) buildContextGroup() *CommandGroup {
	g := newCommandGroup("context", "Show context window usage")
	g.DefaultAction = "show"
	g.Register(SubCommand{
		Name:  "show",
		Usage: "show - Show context window usage for the current session",
		Handler: func(cc CommandContext) (string, error) {
			if strings.TrimSpace(cc.SessionID) == "" {
				return "No active session found for this conversation.", nil
			}
			return h.renderContextUsage(cc, cc.SessionID)
		},
	})
	return g
}

func (h *Handler) renderContextUsage(cc CommandContext, sessionID string) (string, error) {
	if h.queries == nil {
		return "Session info isn't available right now.", nil
	}
	pgSessionID, err := parseCommandUUID(sessionID)
	if err != nil {
		return "", err
	}
	used, err := h.queries.GetLatestAssistantUsage(cc.Ctx, pgSessionID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("get usage: %w", err)
	}
	msgCount, err := h.queries.CountMessagesBySession(cc.Ctx, pgSessionID)
	if err != nil {
		return "", fmt.Errorf("count messages: %w", err)
	}
	cacheRow, _ := h.queries.GetSessionCacheStats(cc.Ctx, pgSessionID)
	var cacheHit float64
	if cacheRow.TotalInputTokens > 0 {
		cacheHit = float64(cacheRow.CacheReadTokens) / float64(cacheRow.TotalInputTokens) * 100
	}
	window := h.resolveContextWindowTokens(cc)

	var b strings.Builder
	b.WriteString(MdBold("Context"))
	b.WriteString("\n\n")
	if window > 0 {
		frac := float64(used) / float64(window)
		fmt.Fprintf(&b, "%s  %.0f%% · %s / %s used", renderProgressBar(frac, 12), frac*100, formatTokens(used), formatTokens(window))
	} else {
		fmt.Fprintf(&b, "%s tokens used", formatTokens(used))
	}
	fmt.Fprintf(&b, "\n\n- Messages: %d", msgCount)
	if cacheRow.TotalInputTokens > 0 {
		fmt.Fprintf(&b, "\n- Cache hit: %.1f%%", cacheHit)
	}
	return b.String(), nil
}

// renderProgressBar draws a fixed-width unicode bar (█ filled, ░ empty). The
// glyphs are plain unicode and survive both the Telegram HTML pass and the
// plain-text strip unchanged.
func renderProgressBar(frac float64, cells int) string {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	filled := int(frac*float64(cells) + 0.5)
	if filled > cells {
		filled = cells
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", cells-filled)
}
