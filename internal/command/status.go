package command

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (h *Handler) buildStatusGroup() *CommandGroup {
	g := newCommandGroup("status", "View current session status")
	g.DefaultAction = "show"
	g.Register(SubCommand{
		Name:  "show",
		Usage: "show - Show current session status",
		Handler: func(cc CommandContext) (string, error) {
			botUUID, err := parseBotUUID(cc.BotID)
			if err != nil {
				return "", err
			}

			sessionID, err := h.queries.GetLatestSessionIDByBot(cc.Ctx, botUUID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "No active session found.", nil
				}
				return "", err
			}

			msgCount, err := h.queries.CountMessagesBySession(cc.Ctx, sessionID)
			if err != nil {
				return "", fmt.Errorf("count messages: %w", err)
			}

			var usedTokens int64
			latestUsage, err := h.queries.GetLatestAssistantUsage(cc.Ctx, sessionID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return "", fmt.Errorf("get usage: %w", err)
			}
			if err == nil {
				usedTokens = latestUsage
			}

			cacheRow, err := h.queries.GetSessionCacheStats(cc.Ctx, sessionID)
			if err != nil {
				return "", fmt.Errorf("get cache: %w", err)
			}

			var contextWindowStr string
			if h.settingsService != nil {
				s, sErr := h.settingsService.GetBot(cc.Ctx, cc.BotID)
				if sErr == nil && s.ChatModelID != "" && h.modelsService != nil {
					m, mErr := h.modelsService.GetByID(cc.Ctx, s.ChatModelID)
					if mErr == nil && m.Config.ContextWindow != nil {
						contextWindowStr = formatTokens(int64(*m.Config.ContextWindow))
					}
				}
			}

			var cacheHitRate float64
			if cacheRow.TotalInputTokens > 0 {
				cacheHitRate = float64(cacheRow.CacheReadTokens) / float64(cacheRow.TotalInputTokens) * 100
			}

			skills, _ := h.queries.GetSessionUsedSkills(cc.Ctx, sessionID)

			var b strings.Builder
			b.WriteString("Session Status:\n\n")
			fmt.Fprintf(&b, "- Messages: %d\n", msgCount)
			if contextWindowStr != "" {
				fmt.Fprintf(&b, "- Context: %s / %s\n", formatTokens(usedTokens), contextWindowStr)
			} else {
				fmt.Fprintf(&b, "- Context: %s\n", formatTokens(usedTokens))
			}
			fmt.Fprintf(&b, "- Cache Hit Rate: %.1f%%\n", cacheHitRate)
			fmt.Fprintf(&b, "- Cache Read: %s\n", formatTokens(cacheRow.CacheReadTokens))
			fmt.Fprintf(&b, "- Cache Write: %s\n", formatTokens(cacheRow.CacheWriteTokens))

			if len(skills) > 0 {
				fmt.Fprintf(&b, "- Skills: %s\n", strings.Join(skills, ", "))
			}

			return strings.TrimRight(b.String(), "\n"), nil
		},
	})
	return g
}

func formatTokens(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return strconv.FormatInt(n, 10)
}
