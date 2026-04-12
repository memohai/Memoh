package command

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (h *Handler) buildStatusGroup() *CommandGroup {
	g := newCommandGroup("status", "View current session status")
	g.DefaultAction = "show"
	g.Register(SubCommand{
		Name:  "show",
		Usage: "show - Show current session status",
		Handler: func(cc CommandContext) (string, error) {
			if strings.TrimSpace(cc.SessionID) == "" {
				return "No active session found for this conversation.", nil
			}
			return h.renderSessionStatus(cc, cc.SessionID, "current conversation")
		},
	})
	g.Register(SubCommand{
		Name:  "latest",
		Usage: "latest - Show the latest session status for this bot",
		Handler: func(cc CommandContext) (string, error) {
			if h.queries == nil {
				return "Session info is not available.", nil
			}
			botUUID, err := parseBotUUID(cc.BotID)
			if err != nil {
				return "", err
			}
			sessionID, err := h.queries.GetLatestSessionIDByBot(cc.Ctx, botUUID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return "No session found for this bot.", nil
				}
				return "", err
			}
			return h.renderSessionStatus(cc, sessionID.String(), "latest bot session")
		},
	})
	return g
}

func (h *Handler) renderSessionStatus(cc CommandContext, sessionID string, scope string) (string, error) {
	if h.queries == nil {
		return "Session info is not available.", nil
	}
	pgSessionID, err := parseCommandUUID(sessionID)
	if err != nil {
		return "", err
	}
	msgCount, err := h.queries.CountMessagesBySession(cc.Ctx, pgSessionID)
	if err != nil {
		return "", fmt.Errorf("count messages: %w", err)
	}

	var usedTokens int64
	latestUsage, err := h.queries.GetLatestAssistantUsage(cc.Ctx, pgSessionID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("get usage: %w", err)
	}
	if err == nil {
		usedTokens = latestUsage
	}

	cacheRow, err := h.queries.GetSessionCacheStats(cc.Ctx, pgSessionID)
	if err != nil {
		return "", fmt.Errorf("get cache: %w", err)
	}

	var cacheHitRate float64
	if cacheRow.TotalInputTokens > 0 {
		cacheHitRate = float64(cacheRow.CacheReadTokens) / float64(cacheRow.TotalInputTokens) * 100
	}

	skills, _ := h.queries.GetSessionUsedSkills(cc.Ctx, pgSessionID)

	contextUsage := formatTokens(usedTokens)
	if contextWindow := h.resolveContextWindow(cc); contextWindow != "" {
		contextUsage = contextUsage + " / " + contextWindow
	}

	pairs := []kv{
		{"Scope", scope},
		{"Session ID", sessionID},
		{"Messages", strconv.FormatInt(msgCount, 10)},
		{"Context", contextUsage},
		{"Cache Hit Rate", fmt.Sprintf("%.1f%%", cacheHitRate)},
		{"Cache Read", formatTokens(cacheRow.CacheReadTokens)},
		{"Cache Write", formatTokens(cacheRow.CacheWriteTokens)},
	}
	if len(skills) > 0 {
		pairs = append(pairs, kv{"Skills", strings.Join(skills, ", ")})
	}
	return formatKV(pairs), nil
}

func (h *Handler) resolveContextWindow(cc CommandContext) string {
	if h.settingsService == nil || h.modelsService == nil {
		return ""
	}
	s, err := h.settingsService.GetBot(cc.Ctx, cc.BotID)
	if err != nil || s.ChatModelID == "" {
		return ""
	}
	m, err := h.modelsService.GetByID(cc.Ctx, s.ChatModelID)
	if err != nil || m.Config.ContextWindow == nil {
		return ""
	}
	return formatTokens(int64(*m.Config.ContextWindow))
}

func parseCommandUUID(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid: %w", err)
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
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
