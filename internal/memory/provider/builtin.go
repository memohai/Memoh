package provider

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/mcp"
)

const (
	BuiltinType = "builtin"

	sharedMemoryNamespace      = "bot"
	memoryContextLimitPerScope = 4
	memoryContextMaxItems      = 8
	memoryContextItemMaxChars  = 220

	defaultMemoryToolLimit = 8
	maxMemoryToolLimit     = 50
	toolSearchMemory       = "search_memory"
)

// BuiltinProvider wraps the existing Service as a Provider.
type BuiltinProvider struct {
	service      memoryRuntime
	chatAccessor conversation.Accessor
	adminChecker AdminChecker
	logger       *slog.Logger
}

// memoryRuntime is the runtime memory backend required by the builtin provider.
// It is intentionally defined as an interface to decouple provider wiring from
// concrete service structs in the memory package.
type memoryRuntime interface {
	Add(ctx context.Context, req AddRequest) (SearchResponse, error)
	Search(ctx context.Context, req SearchRequest) (SearchResponse, error)
	GetAll(ctx context.Context, req GetAllRequest) (SearchResponse, error)
	Update(ctx context.Context, req UpdateRequest) (MemoryItem, error)
	Delete(ctx context.Context, memoryID string) (DeleteResponse, error)
	DeleteBatch(ctx context.Context, memoryIDs []string) (DeleteResponse, error)
	DeleteAll(ctx context.Context, req DeleteAllRequest) (DeleteResponse, error)
	Compact(ctx context.Context, filters map[string]any, ratio float64, decayDays int) (CompactResult, error)
	Usage(ctx context.Context, filters map[string]any) (UsageResponse, error)
}

// AdminChecker checks whether a channel identity has admin privileges.
type AdminChecker interface {
	IsAdmin(ctx context.Context, channelIdentityID string) (bool, error)
}

func NewBuiltinProvider(log *slog.Logger, service any, chatAccessor conversation.Accessor, adminChecker AdminChecker) *BuiltinProvider {
	if log == nil {
		log = slog.Default()
	}
	runtimeService, _ := service.(memoryRuntime)
	return &BuiltinProvider{
		service:      runtimeService,
		chatAccessor: chatAccessor,
		adminChecker: adminChecker,
		logger:       log.With(slog.String("provider", BuiltinType)),
	}
}

func (p *BuiltinProvider) Type() string { return BuiltinType }

// --- Conversation Hooks ---

func (p *BuiltinProvider) OnBeforeChat(ctx context.Context, req BeforeChatRequest) (*BeforeChatResult, error) {
	if p.service == nil {
		return nil, nil
	}
	if strings.TrimSpace(req.Query) == "" || strings.TrimSpace(req.BotID) == "" {
		return nil, nil
	}

	resp, err := p.service.Search(ctx, SearchRequest{
		Query: req.Query,
		BotID: req.BotID,
		Limit: memoryContextLimitPerScope,
		Filters: map[string]any{
			"namespace": sharedMemoryNamespace,
			"scopeId":   req.BotID,
			"bot_id":    req.BotID,
		},
		NoStats: true,
	})
	if err != nil {
		p.logger.Warn("memory search for context failed", slog.Any("error", err))
		return nil, nil
	}

	seen := map[string]struct{}{}
	type contextItem struct {
		namespace string
		item      MemoryItem
	}
	results := make([]contextItem, 0, memoryContextLimitPerScope)
	for _, item := range resp.Results {
		key := strings.TrimSpace(item.ID)
		if key == "" {
			key = sharedMemoryNamespace + ":" + strings.TrimSpace(item.Memory)
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		results = append(results, contextItem{namespace: sharedMemoryNamespace, item: item})
	}
	if len(results) == 0 {
		return nil, nil
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].item.Score > results[j].item.Score
	})
	if len(results) > memoryContextMaxItems {
		results = results[:memoryContextMaxItems]
	}

	var sb strings.Builder
	sb.WriteString("Relevant memory context (use when helpful):\n")
	for _, entry := range results {
		text := strings.TrimSpace(entry.item.Memory)
		if text == "" {
			continue
		}
		sb.WriteString("- [")
		sb.WriteString(entry.namespace)
		sb.WriteString("] ")
		sb.WriteString(truncateSnippet(text, memoryContextItemMaxChars))
		sb.WriteString("\n")
	}
	payload := strings.TrimSpace(sb.String())
	if payload == "" {
		return nil, nil
	}
	return &BeforeChatResult{ContextText: payload}, nil
}

func (p *BuiltinProvider) OnAfterChat(ctx context.Context, req AfterChatRequest) error {
	if p.service == nil {
		return nil
	}
	botID := strings.TrimSpace(req.BotID)
	if botID == "" {
		return nil
	}
	if len(req.Messages) == 0 {
		return nil
	}
	filters := map[string]any{
		"namespace": sharedMemoryNamespace,
		"scopeId":   botID,
		"bot_id":    botID,
	}
	if _, err := p.service.Add(ctx, AddRequest{
		Messages: req.Messages,
		BotID:    botID,
		Filters:  filters,
	}); err != nil {
		p.logger.Warn("store memory failed", slog.String("bot_id", botID), slog.Any("error", err))
	}
	return nil
}

// --- MCP Tools ---

func (p *BuiltinProvider) ListTools(_ context.Context, _ mcp.ToolSessionContext) ([]mcp.ToolDescriptor, error) {
	if p.service == nil {
		return []mcp.ToolDescriptor{}, nil
	}
	return []mcp.ToolDescriptor{
		{
			Name:        toolSearchMemory,
			Description: "Search for memories relevant to the current chat",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The query to search memories",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of memory results",
					},
				},
				"required": []string{"query"},
			},
		},
	}, nil
}

func (p *BuiltinProvider) CallTool(ctx context.Context, session mcp.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if toolName != toolSearchMemory {
		return nil, mcp.ErrToolNotFound
	}
	if p.service == nil {
		return mcp.BuildToolErrorResult("memory service not available"), nil
	}

	query := mcp.StringArg(arguments, "query")
	if query == "" {
		return mcp.BuildToolErrorResult("query is required"), nil
	}
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return mcp.BuildToolErrorResult("bot_id is required"), nil
	}
	chatID := strings.TrimSpace(session.ChatID)
	if chatID == "" {
		chatID = botID
	}

	limit := defaultMemoryToolLimit
	if value, ok, err := mcp.IntArg(arguments, "limit"); err != nil {
		return mcp.BuildToolErrorResult(err.Error()), nil
	} else if ok {
		limit = value
	}
	if limit <= 0 {
		limit = defaultMemoryToolLimit
	}
	if limit > maxMemoryToolLimit {
		limit = maxMemoryToolLimit
	}

	if chatID != botID {
		if p.chatAccessor == nil {
			return mcp.BuildToolErrorResult("chat service not available"), nil
		}
		chatObj, err := p.chatAccessor.Get(ctx, chatID)
		if err != nil {
			return mcp.BuildToolErrorResult("chat not found"), nil
		}
		if strings.TrimSpace(chatObj.BotID) != botID {
			return mcp.BuildToolErrorResult("bot mismatch"), nil
		}
		channelIdentityID := strings.TrimSpace(session.ChannelIdentityID)
		if channelIdentityID != "" {
			allowed, err := p.canAccessChat(ctx, chatID, channelIdentityID)
			if err != nil {
				return mcp.BuildToolErrorResult(err.Error()), nil
			}
			if !allowed {
				return mcp.BuildToolErrorResult("not a chat participant"), nil
			}
		}
	}

	resp, err := p.service.Search(ctx, SearchRequest{
		Query: query,
		BotID: botID,
		Limit: limit,
		Filters: map[string]any{
			"namespace": sharedMemoryNamespace,
			"scopeId":   botID,
			"bot_id":    botID,
		},
		NoStats: true,
	})
	if err != nil {
		return mcp.BuildToolErrorResult("memory search failed"), nil
	}

	allResults := deduplicateItems(resp.Results)
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})
	if len(allResults) > limit {
		allResults = allResults[:limit]
	}

	results := make([]map[string]any, 0, len(allResults))
	for _, item := range allResults {
		results = append(results, map[string]any{
			"id":     item.ID,
			"memory": item.Memory,
			"score":  item.Score,
		})
	}

	return mcp.BuildToolSuccessResult(map[string]any{
		"query":   query,
		"total":   len(results),
		"results": results,
	}), nil
}

func (p *BuiltinProvider) canAccessChat(ctx context.Context, chatID, channelIdentityID string) (bool, error) {
	if p.adminChecker != nil {
		isAdmin, err := p.adminChecker.IsAdmin(ctx, channelIdentityID)
		if err != nil {
			return false, err
		}
		if isAdmin {
			return true, nil
		}
	}
	if p.chatAccessor == nil {
		return false, fmt.Errorf("chat service not available")
	}
	return p.chatAccessor.IsParticipant(ctx, chatID, channelIdentityID)
}

// --- CRUD ---

func (p *BuiltinProvider) Add(ctx context.Context, req AddRequest) (SearchResponse, error) {
	if p.service == nil {
		return SearchResponse{}, fmt.Errorf("memory runtime not configured")
	}
	return p.service.Add(ctx, req)
}

func (p *BuiltinProvider) Search(ctx context.Context, req SearchRequest) (SearchResponse, error) {
	if p.service == nil {
		return SearchResponse{}, fmt.Errorf("memory runtime not configured")
	}
	return p.service.Search(ctx, req)
}

func (p *BuiltinProvider) GetAll(ctx context.Context, req GetAllRequest) (SearchResponse, error) {
	if p.service == nil {
		return SearchResponse{}, fmt.Errorf("memory runtime not configured")
	}
	return p.service.GetAll(ctx, req)
}

func (p *BuiltinProvider) Update(ctx context.Context, req UpdateRequest) (MemoryItem, error) {
	if p.service == nil {
		return MemoryItem{}, fmt.Errorf("memory runtime not configured")
	}
	return p.service.Update(ctx, req)
}

func (p *BuiltinProvider) Delete(ctx context.Context, memoryID string) (DeleteResponse, error) {
	if p.service == nil {
		return DeleteResponse{}, fmt.Errorf("memory runtime not configured")
	}
	return p.service.Delete(ctx, memoryID)
}

func (p *BuiltinProvider) DeleteBatch(ctx context.Context, memoryIDs []string) (DeleteResponse, error) {
	if p.service == nil {
		return DeleteResponse{}, fmt.Errorf("memory runtime not configured")
	}
	return p.service.DeleteBatch(ctx, memoryIDs)
}

func (p *BuiltinProvider) DeleteAll(ctx context.Context, req DeleteAllRequest) (DeleteResponse, error) {
	if p.service == nil {
		return DeleteResponse{}, fmt.Errorf("memory runtime not configured")
	}
	return p.service.DeleteAll(ctx, req)
}

func (p *BuiltinProvider) Compact(ctx context.Context, filters map[string]any, ratio float64, decayDays int) (CompactResult, error) {
	if p.service == nil {
		return CompactResult{}, fmt.Errorf("memory runtime not configured")
	}
	return p.service.Compact(ctx, filters, ratio, decayDays)
}

func (p *BuiltinProvider) Usage(ctx context.Context, filters map[string]any) (UsageResponse, error) {
	if p.service == nil {
		return UsageResponse{}, fmt.Errorf("memory runtime not configured")
	}
	return p.service.Usage(ctx, filters)
}

// --- helpers ---

func truncateSnippet(s string, n int) string {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) <= n {
		return trimmed
	}
	return strings.TrimSpace(trimmed[:n]) + "..."
}

func deduplicateItems(items []MemoryItem) []MemoryItem {
	if len(items) == 0 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]MemoryItem, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = strings.TrimSpace(item.Memory)
		}
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, item)
	}
	return result
}
