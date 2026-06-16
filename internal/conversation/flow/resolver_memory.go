package flow

import (
	"context"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/hooks"
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
	before, err := r.runChatHook(ctx, req, hooks.EventBeforeMemorySearch, func(hreq *hooks.Request) {
		hreq.Memory = map[string]any{
			"scope": "before_chat",
			"query": strings.TrimSpace(req.Query),
		}
	})
	if err != nil {
		r.logHookWarn(hooks.EventBeforeMemorySearch, req.BotID, req.SessionID, err)
		if before.Decision == hooks.DecisionDeny {
			return nil
		}
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
		after, err := r.runChatHook(ctx, req, hooks.EventAfterMemorySearch, func(hreq *hooks.Request) {
			hreq.Memory = map[string]any{
				"scope":        "before_chat",
				"query":        strings.TrimSpace(req.Query),
				"result_count": 0,
			}
		})
		if err != nil {
			r.logHookWarn(hooks.EventAfterMemorySearch, req.BotID, req.SessionID, err)
		}
		if strings.TrimSpace(after.AppendContext) != "" {
			return &conversation.ModelMessage{
				Role:    "user",
				Content: conversation.NewTextContent(formatResolverHookContext(hooks.EventAfterMemorySearch, after.AppendContext)),
			}
		}
		return nil
	}
	after, err := r.runChatHook(ctx, req, hooks.EventAfterMemorySearch, func(hreq *hooks.Request) {
		hreq.Memory = map[string]any{
			"scope":         "before_chat",
			"query":         strings.TrimSpace(req.Query),
			"result_count":  1,
			"context_bytes": len(result.ContextText),
		}
	})
	if err != nil {
		r.logHookWarn(hooks.EventAfterMemorySearch, req.BotID, req.SessionID, err)
	}
	contextText := result.ContextText
	if strings.TrimSpace(after.AppendContext) != "" {
		contextText += "\n\n" + formatResolverHookContext(hooks.EventAfterMemorySearch, after.AppendContext)
	}
	return &conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(contextText),
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
	before, err := r.runChatHook(ctx, req, hooks.EventBeforeMemoryWrite, func(hreq *hooks.Request) {
		hreq.Memory = map[string]any{
			"scope":         "after_chat",
			"message_count": len(memMsgs),
		}
	})
	if err != nil {
		r.logHookWarn(hooks.EventBeforeMemoryWrite, botID, req.SessionID, err)
		if before.Decision == hooks.DecisionDeny {
			return
		}
	}
	_, tzLoc := r.resolveTimezone(ctx, req.BotID, req.UserID)
	if err := p.OnAfterChat(ctx, memprovider.AfterChatRequest{
		BotID:             botID,
		Messages:          memMsgs,
		UserID:            strings.TrimSpace(req.UserID),
		ChannelIdentityID: strings.TrimSpace(req.SourceChannelIdentityID),
		DisplayName:       r.resolveDisplayName(ctx, req),
		TimezoneLocation:  tzLoc,
	}); err != nil {
		r.logger.Warn("memory provider OnAfterChat failed", slog.String("bot_id", botID), slog.Any("error", err))
		return
	}
	_, _ = r.runChatHook(ctx, req, hooks.EventMemoryExtracted, func(hreq *hooks.Request) {
		hreq.Memory = map[string]any{
			"scope":         "after_chat",
			"message_count": len(memMsgs),
		}
	})
	if _, err := r.runChatHook(ctx, req, hooks.EventAfterMemoryWrite, func(hreq *hooks.Request) {
		hreq.Memory = map[string]any{
			"scope":         "after_chat",
			"message_count": len(memMsgs),
		}
	}); err != nil {
		r.logHookWarn(hooks.EventAfterMemoryWrite, botID, req.SessionID, err)
	}
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
