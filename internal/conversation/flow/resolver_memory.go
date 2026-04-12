package flow

import (
	"context"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
)

func (r *Resolver) resolveMemoryProvider(ctx context.Context, botID string) memprovider.Provider {
	if r.memoryRegistry == nil {
		return nil
	}
	if r.settingsService == nil {
		return nil
	}
	botSettings, err := r.settingsService.GetBot(ctx, botID)
	if err != nil {
		return nil
	}
	providerID := strings.TrimSpace(botSettings.MemoryProviderID)
	if providerID == "" {
		return nil
	}
	p, err := r.memoryRegistry.Get(providerID)
	if err != nil {
		r.logger.Warn("memory provider lookup failed", slog.String("provider_id", providerID), slog.Any("error", err))
		return nil
	}
	return p
}

func (r *Resolver) loadMemoryContextMessage(ctx context.Context, req conversation.ChatRequest) *conversation.ModelMessage {
	p := r.resolveMemoryProvider(ctx, req.BotID)
	if p == nil {
		return nil
	}
	result, err := p.OnBeforeChat(ctx, memprovider.BeforeChatRequest{
		Query:  req.Query,
		BotID:  req.BotID,
		ChatID: req.ChatID,
	})
	if err != nil {
		r.logger.Warn("memory provider OnBeforeChat failed", slog.Any("error", err))
		return nil
	}
	if result == nil || strings.TrimSpace(result.ContextText) == "" {
		return nil
	}
	return &conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(result.ContextText),
	}
}

func (r *Resolver) storeMemory(ctx context.Context, req conversation.ChatRequest, messages []conversation.ModelMessage) {
	botID := strings.TrimSpace(req.BotID)
	if botID == "" {
		return
	}
	memMsgs := toProviderMessages(messages)
	if len(memMsgs) == 0 {
		return
	}

	p := r.resolveMemoryProvider(ctx, botID)
	if p == nil {
		return
	}
	_, tzLoc := r.resolveTimezone(ctx, req.BotID, req.UserID)
	if err := p.OnAfterChat(ctx, memprovider.AfterChatRequest{
		BotID:             botID,
		Messages:          memMsgs,
		UserID:            strings.TrimSpace(req.UserID),
		ChannelIdentityID: strings.TrimSpace(req.SourceChannelIdentityID),
		DisplayName:       r.resolveDisplayName(ctx, req),
		TimezoneLocation:  tzLoc,
		ConversationType:  strings.TrimSpace(req.ConversationType),
		ConversationName:  strings.TrimSpace(req.ConversationName),
		Platform:          strings.TrimSpace(req.CurrentChannel),
		BotName:           r.resolveBotDisplayName(ctx, botID),
	}); err != nil {
		r.logger.Warn("memory provider OnAfterChat failed", slog.String("bot_id", botID), slog.Any("error", err))
	}
}

func (r *Resolver) resolveBotDisplayName(ctx context.Context, botID string) string {
	if r.queries == nil {
		return ""
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return ""
	}
	row, err := r.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		return ""
	}
	if row.DisplayName.Valid {
		return strings.TrimSpace(row.DisplayName.String)
	}
	return ""
}

func toProviderMessages(messages []conversation.ModelMessage) []memprovider.Message {
	out := make([]memprovider.Message, 0, len(messages))
	for _, msg := range messages {
		text := strings.TrimSpace(msg.TextContent())
		if text == "" {
			continue
		}
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "assistant"
		}
		out = append(out, memprovider.Message{Role: role, Content: text})
	}
	return out
}
