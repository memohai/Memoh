package inbox

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	mcpgw "github.com/memohai/memoh/internal/mcp"

	inboxsvc "github.com/memohai/memoh/internal/inbox"
)

const (
	toolSearchInbox       = "search_inbox"
	defaultSearchLimit    = 20
	maxSearchLimit        = 100
)

type Executor struct {
	service *inboxsvc.Service
	logger  *slog.Logger
}

func NewExecutor(log *slog.Logger, service *inboxsvc.Service) *Executor {
	if log == nil {
		log = slog.Default()
	}
	return &Executor{
		service: service,
		logger:  log.With(slog.String("provider", "inbox_tool")),
	}
}

func (e *Executor) ListTools(ctx context.Context, session mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	if e.service == nil {
		return []mcpgw.ToolDescriptor{}, nil
	}
	return []mcpgw.ToolDescriptor{
		{
			Name:        toolSearchInbox,
			Description: "Search historical inbox messages by keyword. Inbox contains messages from group conversations where the bot was not directly mentioned, as well as notifications from external sources.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search keyword to match against inbox message content",
					},
					"start_time": map[string]any{
						"type":        "string",
						"description": "ISO 8601 start time filter (e.g. 2025-01-01T00:00:00Z)",
					},
					"end_time": map[string]any{
						"type":        "string",
						"description": "ISO 8601 end time filter",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of results (default 20, max 100)",
					},
					"include_read": map[string]any{
						"type":        "boolean",
						"description": "Whether to include already-read items (default true)",
					},
				},
				"required": []string{"query"},
			},
		},
	}, nil
}

func (e *Executor) CallTool(ctx context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	if toolName != toolSearchInbox {
		return nil, mcpgw.ErrToolNotFound
	}
	if e.service == nil {
		return mcpgw.BuildToolErrorResult("inbox service not available"), nil
	}

	query := mcpgw.StringArg(arguments, "query")
	if query == "" {
		return mcpgw.BuildToolErrorResult("query is required"), nil
	}
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return mcpgw.BuildToolErrorResult("bot_id is required"), nil
	}

	limit := defaultSearchLimit
	if value, ok, err := mcpgw.IntArg(arguments, "limit"); err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	} else if ok {
		limit = value
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if limit > maxSearchLimit {
		limit = maxSearchLimit
	}

	req := inboxsvc.SearchRequest{
		Query: query,
		Limit: limit,
	}

	if startStr := mcpgw.StringArg(arguments, "start_time"); startStr != "" {
		t, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			return mcpgw.BuildToolErrorResult(fmt.Sprintf("invalid start_time: %v", err)), nil
		}
		req.StartTime = &t
	}
	if endStr := mcpgw.StringArg(arguments, "end_time"); endStr != "" {
		t, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			return mcpgw.BuildToolErrorResult(fmt.Sprintf("invalid end_time: %v", err)), nil
		}
		req.EndTime = &t
	}
	if includeRead, ok, err := mcpgw.BoolArg(arguments, "include_read"); err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	} else if ok {
		req.IncludeRead = &includeRead
	}

	items, err := e.service.Search(ctx, botID, req)
	if err != nil {
		e.logger.Warn("inbox search failed", slog.String("bot_id", botID), slog.Any("error", err))
		return mcpgw.BuildToolErrorResult("inbox search failed"), nil
	}

	results := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := map[string]any{
			"id":         item.ID,
			"source":     item.Source,
			"header":     item.Header,
			"content":    item.Content,
			"is_read":    item.IsRead,
			"created_at": item.CreatedAt.Format(time.RFC3339),
		}
		results = append(results, entry)
	}

	return mcpgw.BuildToolSuccessResult(map[string]any{
		"query":   query,
		"total":   len(results),
		"results": results,
	}), nil
}
