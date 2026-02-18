package memory

import (
	"context"
	"strings"
)

type contextKey string

const memoryBotIDContextKey contextKey = "memory_bot_id"

// WithBotID attaches bot ID to context so model selection can honor bot settings.
func WithBotID(ctx context.Context, botID string) context.Context {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return ctx
	}
	return context.WithValue(ctx, memoryBotIDContextKey, botID)
}

// BotIDFromContext returns bot ID carried by WithBotID.
func BotIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	botID, _ := ctx.Value(memoryBotIDContextKey).(string)
	return strings.TrimSpace(botID)
}
