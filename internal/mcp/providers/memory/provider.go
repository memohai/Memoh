// Package memory provides the MCP memory provider (search and recall tools).
package memory

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"github.com/memohai/memoh/internal/conversation"
	mcpgw "github.com/memohai/memoh/internal/mcp"
	mem "github.com/memohai/memoh/internal/memory"
)

const (
	toolSearchMemory       = "search_memory"
	defaultMemoryToolLimit = 8
	maxMemoryToolLimit     = 50
	sharedMemoryNamespace  = "bot"
)

// Searcher performs memory search (used by memory tool executor).
type Searcher interface {
	Search(ctx context.Context, req mem.SearchRequest) (mem.SearchResponse, error)
}

// AdminChecker checks if a channel identity is admin (for memory tool access).
type AdminChecker interface {
	IsAdmin(ctx context.Context, channelIdentityID string) (bool, error)
}

// Executor is the MCP tool executor for search_memory (delegates to Searcher, checks chat access).
type Executor struct {
	searcher     Searcher
	chatAccessor conversation.Accessor
	adminChecker AdminChecker
	logger       *slog.Logger
}

// NewExecutor creates a memory tool executor.
func NewExecutor(log *slog.Logger, searcher Searcher, chatAccessor conversation.Accessor, adminChecker AdminChecker) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{
		searcher:     searcher,
		chatAccessor: chatAccessor,
		adminChecker: adminChecker,
		logger:       log.With(slog.String("provider", "memory_tool")),
	}
}

// ListTools returns the search_memory tool descriptor.
func (p *Executor) ListTools(_ context.Context, _ mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	if p.searcher == nil || p.chatAccessor == nil {
		return []mcpgw.ToolDescriptor{}, nil
	}
	return []mcpgw.ToolDescriptor{
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

// CallTool runs search_memory (query, limit) and returns MCP result; validates chat access.
func (p *Executor) CallTool(ctx context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if toolName != toolSearchMemory {
		return nil, mcpgw.ErrToolNotFound
	}
	if p.searcher == nil || p.chatAccessor == nil {
		return mcpgw.BuildToolErrorResult("memory service not available"), nil
	}

	query := mcpgw.StringArg(arguments, "query")
	if query == "" {
		return mcpgw.BuildToolErrorResult("query is required"), nil
	}
	botID := strings.TrimSpace(session.BotID)
	chatID := strings.TrimSpace(session.ChatID)
	channelIdentityID := strings.TrimSpace(session.ChannelIdentityID)
	if botID == "" {
		return mcpgw.BuildToolErrorResult("bot_id is required"), nil
	}
	if chatID == "" {
		chatID = botID
	}

	limit := defaultMemoryToolLimit
	if value, ok, err := mcpgw.IntArg(arguments, "limit"); err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	} else if ok {
		limit = value
	}
	if limit <= 0 {
		limit = defaultMemoryToolLimit
	}
	if limit > maxMemoryToolLimit {
		limit = maxMemoryToolLimit
	}

	// When ChatID equals BotID (e.g. tools called without conversation context), search by bot scope only.
	// Otherwise require the conversation to exist and the caller to be a participant.
	if chatID != botID {
		chatObj, err := p.chatAccessor.Get(ctx, chatID)
		if err != nil {
			return mcpgw.BuildToolErrorResult("chat not found"), nil
		}
		if strings.TrimSpace(chatObj.BotID) != botID {
			return mcpgw.BuildToolErrorResult("bot mismatch"), nil
		}
		if channelIdentityID != "" {
			allowed, err := p.canAccessChat(ctx, chatID, channelIdentityID)
			if err != nil {
				return mcpgw.BuildToolErrorResult(err.Error()), nil
			}
			if !allowed {
				return mcpgw.BuildToolErrorResult("not a chat participant"), nil
			}
		}
	}

	resp, err := p.searcher.Search(ctx, mem.SearchRequest{
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
		p.logger.Warn("memory search namespace failed", slog.String("namespace", sharedMemoryNamespace), slog.Any("error", err))
		return mcpgw.BuildToolErrorResult("memory search failed"), nil
	}
	allResults := make([]mem.Item, 0, len(resp.Results))
	allResults = append(allResults, resp.Results...)

	allResults = deduplicateMemoryItems(allResults)
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

	return mcpgw.BuildToolSuccessResult(map[string]any{
		"query":   query,
		"total":   len(results),
		"results": results,
	}), nil
}

func (p *Executor) canAccessChat(ctx context.Context, chatID, channelIdentityID string) (bool, error) {
	if p.adminChecker != nil {
		isAdmin, err := p.adminChecker.IsAdmin(ctx, channelIdentityID)
		if err != nil {
			return false, err
		}
		if isAdmin {
			return true, nil
		}
	}
	return p.chatAccessor.IsParticipant(ctx, chatID, channelIdentityID)
}

func deduplicateMemoryItems(items []mem.Item) []mem.Item {
	if len(items) == 0 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]mem.Item, 0, len(items))
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
