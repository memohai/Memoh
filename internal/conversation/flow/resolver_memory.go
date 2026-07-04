package flow

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/hooks"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
)

const defaultMemorySearchTimeout = 1200 * time.Millisecond

func (r *Resolver) resolveMemoryProvider(ctx context.Context, botID string) memprovider.Provider {
	_, p := r.resolveMemoryProviderWithID(ctx, botID)
	return p
}

func (r *Resolver) resolveMemoryProviderWithID(ctx context.Context, botID string) (string, memprovider.Provider) {
	if r.memoryRegistry == nil {
		return "", nil
	}
	if r.settingsService == nil {
		return "", nil
	}
	botSettings, err := r.settingsService.GetBot(ctx, botID)
	if err != nil {
		return "", nil
	}
	providerID := strings.TrimSpace(botSettings.MemoryProviderID)
	if providerID == "" {
		return "", nil
	}
	p, err := r.memoryRegistry.Get(providerID)
	if err != nil {
		r.logger.Warn("memory provider lookup failed", slog.String("provider_id", providerID), slog.Any("error", err))
		return "", nil
	}
	return providerID, p
}

func (r *Resolver) loadMemoryContextMessage(ctx context.Context, req conversation.ChatRequest) *conversation.ModelMessage {
	builtQuery := r.buildMemoryQuery(ctx, req)
	if strings.TrimSpace(builtQuery.Query) == "" {
		return nil
	}
	providerID, p := r.resolveMemoryProviderWithID(ctx, req.BotID)
	if p == nil {
		return nil
	}

	before, err := r.runChatHook(ctx, req, hooks.EventBeforeMemorySearch, func(hreq *hooks.Request) {
		hreq.Memory = map[string]any{
			"scope":                 "before_chat",
			"query":                 builtQuery.Query,
			"visible_query":         strings.TrimSpace(req.Query),
			"query_source":          builtQuery.Source,
			"query_recent_messages": builtQuery.RecentMessages,
			"query_truncated":       builtQuery.Truncated,
		}
	})
	if err != nil {
		r.logHookWarn(hooks.EventBeforeMemorySearch, req.BotID, req.SessionID, err)
		if before.Decision == hooks.DecisionDeny {
			return nil
		}
	}

	cacheKey := r.memoryContextCacheKey(ctx, req, providerID, p, builtQuery.Query)
	if cached, ok := r.getMemoryContextCache().Get(cacheKey); ok {
		result := &memprovider.BeforeChatResult{
			ContextText:    cached.ContextText,
			RetrievalMode:  cached.RetrievalMode,
			FallbackReason: cached.FallbackReason,
		}
		return r.memoryContextMessageFromResult(ctx, req, builtQuery, result, "fresh", "")
	}

	searchCtx, cancel := context.WithTimeout(ctx, r.effectiveMemorySearchTimeout())
	result, err := p.OnBeforeChat(searchCtx, memprovider.BeforeChatRequest{
		Query:  builtQuery.Query,
		BotID:  req.BotID,
		ChatID: req.ChatID,
	})
	cancel()

	if err != nil {
		fallbackReason := "provider_error"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(searchCtx.Err(), context.DeadlineExceeded) {
			fallbackReason = "timeout"
		}
		r.logger.Warn("memory provider OnBeforeChat failed",
			slog.String("bot_id", req.BotID),
			slog.String("fallback_reason", fallbackReason),
			slog.Any("error", err),
		)
		if cached, ok := r.getMemoryContextCache().GetStale(cacheKey); ok {
			result := &memprovider.BeforeChatResult{
				ContextText:    cached.ContextText,
				RetrievalMode:  cached.RetrievalMode,
				FallbackReason: firstNonEmpty(fallbackReason, cached.FallbackReason),
			}
			return r.memoryContextMessageFromResult(ctx, req, builtQuery, result, "stale", fallbackReason)
		}
		return r.memoryContextMessageFromResult(ctx, req, builtQuery, nil, "miss", fallbackReason)
	}

	if result == nil || strings.TrimSpace(result.ContextText) == "" {
		return r.memoryContextMessageFromResult(ctx, req, builtQuery, nil, "miss", "empty_result")
	}

	r.getMemoryContextCache().Set(cacheKey, memprovider.MemoryContextCacheValue{
		ContextText:    result.ContextText,
		RetrievalMode:  result.RetrievalMode,
		FallbackReason: result.FallbackReason,
	})
	return r.memoryContextMessageFromResult(ctx, req, builtQuery, result, "miss", strings.TrimSpace(result.FallbackReason))
}

func (r *Resolver) memoryContextMessageFromResult(ctx context.Context, req conversation.ChatRequest, builtQuery memoryQuery, result *memprovider.BeforeChatResult, cacheState, fallbackReason string) *conversation.ModelMessage {
	contextText := ""
	resultCount := 0
	contextBytes := 0
	retrievalMode := ""
	if result != nil {
		contextText = strings.TrimSpace(result.ContextText)
		if contextText != "" {
			resultCount = 1
			contextBytes = len(contextText)
		}
		retrievalMode = strings.TrimSpace(result.RetrievalMode)
		if fallbackReason == "" {
			fallbackReason = strings.TrimSpace(result.FallbackReason)
		}
	}

	after, err := r.runChatHook(ctx, req, hooks.EventAfterMemorySearch, func(hreq *hooks.Request) {
		hreq.Memory = map[string]any{
			"scope":                 "before_chat",
			"query":                 builtQuery.Query,
			"visible_query":         strings.TrimSpace(req.Query),
			"query_source":          builtQuery.Source,
			"query_recent_messages": builtQuery.RecentMessages,
			"query_truncated":       builtQuery.Truncated,
			"result_count":          resultCount,
			"context_bytes":         contextBytes,
			"cache_hit":             cacheState == "fresh" || cacheState == "stale",
			"cache_state":           cacheState,
			"retrieval_mode":        retrievalMode,
			"fallback_reason":       strings.TrimSpace(fallbackReason),
		}
	})
	if err != nil {
		r.logHookWarn(hooks.EventAfterMemorySearch, req.BotID, req.SessionID, err)
	}

	if strings.TrimSpace(after.AppendContext) != "" {
		hookContext := formatResolverHookContext(hooks.EventAfterMemorySearch, after.AppendContext)
		if contextText == "" {
			contextText = hookContext
		} else {
			contextText += "\n\n" + hookContext
		}
	}
	if strings.TrimSpace(contextText) == "" {
		return nil
	}
	return &conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(contextText),
	}
}

func (r *Resolver) getMemoryContextCache() *memprovider.MemoryContextCache {
	if r == nil {
		return nil
	}
	if r.memoryContextCache != nil {
		return r.memoryContextCache
	}
	r.memoryContextMu.Lock()
	defer r.memoryContextMu.Unlock()
	if r.memoryContextCache == nil {
		r.memoryContextCache = memprovider.NewMemoryContextCache(memprovider.MemoryContextCacheConfig{
			TTL:        time.Minute,
			StaleTTL:   5 * time.Minute,
			MaxEntries: 256,
		})
	}
	return r.memoryContextCache
}

func (*Resolver) memoryContextCacheKey(ctx context.Context, req conversation.ChatRequest, providerID string, p memprovider.Provider, query string) memprovider.MemoryContextCacheKey {
	memoryVersion := ""
	if versioned, ok := p.(memprovider.MemoryVersionProvider); ok {
		memoryVersion = versioned.MemoryVersion(ctx, req.BotID)
	}
	return memprovider.MemoryContextCacheKey{
		BotID:         strings.TrimSpace(req.BotID),
		ChatID:        strings.TrimSpace(req.ChatID),
		ProviderID:    strings.TrimSpace(providerID),
		QueryHash:     memprovider.MemoryContextQueryHash(query),
		MemoryVersion: strings.TrimSpace(memoryVersion),
	}
}

func (r *Resolver) effectiveMemorySearchTimeout() time.Duration {
	if r == nil || r.memorySearchTimeout <= 0 {
		return defaultMemorySearchTimeout
	}
	return r.memorySearchTimeout
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
