package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	inboxsvc "github.com/memohai/memoh/internal/inbox"
	sdk "github.com/memohai/twilight-ai/sdk"
)

const (
	defaultInboxSearchLimit = 20
	maxInboxSearchLimit     = 100
)

type InboxProvider struct {
	service *inboxsvc.Service
	logger  *slog.Logger
}

func NewInboxProvider(log *slog.Logger, service *inboxsvc.Service) *InboxProvider {
	if log == nil {
		log = slog.Default()
	}
	return &InboxProvider{
		service: service,
		logger:  log.With(slog.String("tool", "inbox")),
	}
}

func (p *InboxProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	if p.service == nil {
		return nil, nil
	}
	sess := session
	return []sdk.Tool{
		{
			Name:        "search_inbox",
			Description: "Search historical inbox messages by keyword. Inbox contains messages from group conversations where the bot was not directly mentioned, as well as notifications from external sources.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":        map[string]any{"type": "string", "description": "Search keyword to match against inbox message content"},
					"start_time":   map[string]any{"type": "string", "description": "ISO 8601 start time filter (e.g. 2025-01-01T00:00:00Z)"},
					"end_time":     map[string]any{"type": "string", "description": "ISO 8601 end time filter"},
					"limit":        map[string]any{"type": "integer", "description": "Maximum number of results (default 20, max 100)"},
					"include_read": map[string]any{"type": "boolean", "description": "Whether to include already-read items (default true)"},
				},
				"required": []string{},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				botID := strings.TrimSpace(sess.BotID)
				if botID == "" {
					return nil, fmt.Errorf("bot_id is required")
				}
				query := StringArg(args, "query")
				limit := defaultInboxSearchLimit
				if value, ok, err := IntArg(args, "limit"); err != nil {
					return nil, err
				} else if ok {
					limit = value
				}
				if limit <= 0 {
					limit = defaultInboxSearchLimit
				}
				if limit > maxInboxSearchLimit {
					limit = maxInboxSearchLimit
				}
				req := inboxsvc.SearchRequest{Query: query, Limit: limit}
				if startStr := StringArg(args, "start_time"); startStr != "" {
					t, err := time.Parse(time.RFC3339, startStr)
					if err != nil {
						return nil, fmt.Errorf("invalid start_time: %v", err)
					}
					req.StartTime = &t
				}
				if endStr := StringArg(args, "end_time"); endStr != "" {
					t, err := time.Parse(time.RFC3339, endStr)
					if err != nil {
						return nil, fmt.Errorf("invalid end_time: %v", err)
					}
					req.EndTime = &t
				}
				if includeRead, ok, err := BoolArg(args, "include_read"); err != nil {
					return nil, err
				} else if ok {
					req.IncludeRead = &includeRead
				}
				items, err := p.service.Search(ctx, botID, req)
				if err != nil {
					return nil, fmt.Errorf("inbox search failed")
				}
				results := make([]map[string]any, 0, len(items))
				for _, item := range items {
					results = append(results, map[string]any{
						"id":         item.ID,
						"source":     item.Source,
						"header":     item.Header,
						"content":    item.Content,
						"is_read":    item.IsRead,
						"created_at": item.CreatedAt.Format(time.RFC3339),
					})
				}
				return map[string]any{"query": query, "total": len(results), "results": results}, nil
			},
		},
	}, nil
}
