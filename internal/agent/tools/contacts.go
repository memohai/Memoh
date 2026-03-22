package tools

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/channel/route"
)

type ContactsProvider struct {
	routeService route.Service
	logger       *slog.Logger
}

func NewContactsProvider(log *slog.Logger, routeService route.Service) *ContactsProvider {
	if log == nil {
		log = slog.Default()
	}
	return &ContactsProvider{
		routeService: routeService,
		logger:       log.With(slog.String("tool", "contacts")),
	}
}

func (p *ContactsProvider) Tools(_ context.Context, session SessionContext) ([]sdk.Tool, error) {
	if session.IsSubagent || p.routeService == nil {
		return nil, nil
	}
	sess := session
	return []sdk.Tool{
		{
			Name:        "get_contacts",
			Description: "List all known contacts and conversations for the current bot. Returns platform, conversation type, reply target, and metadata for each route.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"platform": map[string]any{
						"type":        "string",
						"description": "Filter by channel platform (e.g. telegram, feishu). Returns all platforms when omitted.",
					},
				},
				"required": []string{},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				args := inputAsMap(input)
				botID := strings.TrimSpace(sess.BotID)
				if botID == "" {
					return nil, errors.New("bot_id is required")
				}
				routes, err := p.routeService.List(ctx.Context, botID)
				if err != nil {
					return nil, err
				}
				platformFilter := strings.ToLower(strings.TrimSpace(FirstStringArg(args, "platform")))
				contacts := make([]map[string]any, 0, len(routes))
				for _, r := range routes {
					if platformFilter != "" && !strings.EqualFold(r.Platform, platformFilter) {
						continue
					}
					entry := map[string]any{
						"route_id":          r.ID,
						"platform":          r.Platform,
						"conversation_type": r.ConversationType,
						"target":            r.ReplyTarget,
						"conversation_id":   r.ConversationID,
						"last_active":       r.UpdatedAt.Format("2006-01-02T15:04:05Z"),
					}
					if len(r.Metadata) > 0 {
						if v, ok := r.Metadata["conversation_name"].(string); ok && v != "" {
							entry["display_name"] = v
						} else if v, ok := r.Metadata["sender_display_name"].(string); ok && v != "" {
							entry["display_name"] = v
						}
						if v, ok := r.Metadata["sender_username"].(string); ok && v != "" {
							entry["username"] = v
						}
						entry["metadata"] = r.Metadata
					}
					contacts = append(contacts, entry)
				}
				return map[string]any{
					"ok":       true,
					"bot_id":   botID,
					"count":    len(contacts),
					"contacts": contacts,
				}, nil
			},
		},
	}, nil
}
