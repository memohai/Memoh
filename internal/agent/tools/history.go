package tools

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/session"
)

const defaultMaxLookbackDays = 7

// SessionLister is the minimal interface for listing sessions.
type SessionLister interface {
	ListByBot(ctx context.Context, botID string) ([]session.Session, error)
}

// HistoryProvider exposes list_sessions and search_messages tools.
type HistoryProvider struct {
	sessions SessionLister
	queries  dbstore.Queries
	logger   *slog.Logger
}

func NewHistoryProvider(log *slog.Logger, sessions SessionLister, queries dbstore.Queries) *HistoryProvider {
	if log == nil {
		log = slog.Default()
	}
	return &HistoryProvider{
		sessions: sessions,
		queries:  queries,
		logger:   log.With(slog.String("tool", "history")),
	}
}

func (p *HistoryProvider) Tools(_ context.Context, sess SessionContext) ([]sdk.Tool, error) {
	if sess.IsSubagent {
		return nil, nil
	}
	var tools []sdk.Tool

	if p.sessions != nil {
		s := sess
		tools = append(tools, sdk.Tool{
			Name:        "list_sessions",
			Description: "List all chat sessions for the current bot with their bound contact/route information. Use this to discover conversations and find session IDs for search_messages.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type": map[string]any{
						"type":        "string",
						"description": "Filter by session type: chat, heartbeat, or schedule. Returns all types when omitted.",
						"enum":        []string{"chat", "heartbeat", "schedule"},
					},
					"platform": map[string]any{
						"type":        "string",
						"description": "Filter by channel platform (e.g. telegram, feishu). Returns all platforms when omitted.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of sessions to return. Default 50.",
					},
				},
				"required": []string{},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execListSessions(ctx.Context, s, inputAsMap(input))
			},
		})
	}

	if p.queries != nil {
		s := sess
		tools = append(tools, sdk.Tool{
			Name:        "search_messages",
			Description: "Search message history across all sessions. Supports filtering by time range, keyword, session, contact, and role. All parameters are optional. If start_time is not provided, only the last 7 days are searched.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"start_time": map[string]any{
						"type":        "string",
						"description": "ISO 8601 timestamp. Only return messages created at or after this time.",
					},
					"end_time": map[string]any{
						"type":        "string",
						"description": "ISO 8601 timestamp. Only return messages created at or before this time.",
					},
					"keyword": map[string]any{
						"type":        "string",
						"description": "Search keyword — matches against the text content of messages (case-insensitive).",
					},
					"session_id": map[string]any{
						"type":        "string",
						"description": "Filter by session ID. Use list_sessions to find session IDs.",
					},
					"contact_id": map[string]any{
						"type":        "string",
						"description": "Filter by sender channel identity ID. Use get_contacts to find contact IDs.",
					},
					"role": map[string]any{
						"type":        "string",
						"description": "Filter by message role.",
						"enum":        []string{"user", "assistant"},
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of messages to return. Default 50, max 200.",
					},
				},
				"required": []string{},
			},
			Execute: func(ctx *sdk.ToolExecContext, input any) (any, error) {
				return p.execSearchMessages(ctx.Context, s, inputAsMap(input))
			},
		})
	}

	return tools, nil
}

// ---------------------------------------------------------------------------
// list_sessions
// ---------------------------------------------------------------------------

func (p *HistoryProvider) execListSessions(ctx context.Context, sess SessionContext, args map[string]any) (any, error) {
	botID := strings.TrimSpace(sess.BotID)
	if botID == "" {
		return nil, errors.New("bot_id is required")
	}

	sessions, err := p.sessions.ListByBot(ctx, botID)
	if err != nil {
		return nil, err
	}

	typeFilter := strings.ToLower(strings.TrimSpace(StringArg(args, "type")))
	platformFilter := strings.ToLower(strings.TrimSpace(StringArg(args, "platform")))

	limit := 50
	if v, ok, _ := IntArg(args, "limit"); ok && v > 0 {
		limit = v
	}

	results := make([]map[string]any, 0, len(sessions))
	for _, s := range sessions {
		if typeFilter != "" && !strings.EqualFold(s.Type, typeFilter) {
			continue
		}
		if platformFilter != "" && !strings.EqualFold(s.ChannelType, platformFilter) {
			continue
		}

		entry := map[string]any{
			"session_id":        s.ID,
			"type":              s.Type,
			"title":             s.Title,
			"platform":          s.ChannelType,
			"route_id":          s.RouteID,
			"conversation_type": s.RouteConversationType,
			"last_active":       sess.FormatTime(s.UpdatedAt),
			"created_at":        sess.FormatTime(s.CreatedAt),
		}

		if m := s.RouteMetadata; len(m) > 0 {
			if v, _ := m["conversation_name"].(string); v != "" {
				entry["conversation_name"] = v
			}
			if v, _ := m["sender_display_name"].(string); v != "" {
				entry["display_name"] = v
			}
			if v, _ := m["sender_username"].(string); v != "" {
				entry["username"] = v
			}
		}

		results = append(results, entry)
		if len(results) >= limit {
			break
		}
	}

	return map[string]any{
		"ok":       true,
		"bot_id":   botID,
		"count":    len(results),
		"sessions": results,
	}, nil
}

// ---------------------------------------------------------------------------
// search_messages
// ---------------------------------------------------------------------------

func (p *HistoryProvider) execSearchMessages(ctx context.Context, sess SessionContext, args map[string]any) (any, error) {
	botID := strings.TrimSpace(sess.BotID)
	if botID == "" {
		return nil, errors.New("bot_id is required")
	}

	pgBotID, err := dbpkg.ParseUUID(botID)
	if err != nil {
		return nil, errors.New("invalid bot_id")
	}

	limit := int32(50)
	if v, ok, _ := IntArg(args, "limit"); ok && v > 0 && v <= 200 {
		limit = int32(v) //nolint:gosec // bounds-checked above
	}

	params := sqlc.SearchMessagesParams{
		BotID:    pgBotID,
		MaxCount: limit,
	}

	if v := StringArg(args, "session_id"); v != "" {
		params.SessionID = dbpkg.ParseUUIDOrEmpty(v)
	}
	if v := StringArg(args, "contact_id"); v != "" {
		params.ContactID = dbpkg.ParseUUIDOrEmpty(v)
	}
	if v := StringArg(args, "role"); v != "" {
		params.Role = pgtype.Text{String: v, Valid: true}
	}
	if v := StringArg(args, "keyword"); v != "" {
		params.Keyword = pgtype.Text{String: v, Valid: true}
	}
	if v := StringArg(args, "start_time"); v != "" {
		if t, parseErr := parseFlexibleTime(v); parseErr == nil {
			params.StartTime = pgtype.Timestamptz{Time: t, Valid: true}
		}
	} else {
		defaultLookback := time.Now().UTC().AddDate(0, 0, -defaultMaxLookbackDays)
		params.StartTime = pgtype.Timestamptz{Time: defaultLookback, Valid: true}
	}
	if v := StringArg(args, "end_time"); v != "" {
		if t, parseErr := parseFlexibleTime(v); parseErr == nil {
			params.EndTime = pgtype.Timestamptz{Time: t, Valid: true}
		}
	}

	rows, err := p.queries.SearchMessages(ctx, params)
	if err != nil {
		return nil, err
	}

	messages := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		text := extractTextContent(row.Content)

		entry := map[string]any{
			"id":         row.ID.String(),
			"session_id": row.SessionID.String(),
			"role":       row.Role,
			"text":       text,
			"created_at": sess.FormatTime(row.CreatedAt.Time),
		}
		if dbpkg.TextToString(row.Platform) != "" {
			entry["platform"] = dbpkg.TextToString(row.Platform)
		}
		if dbpkg.TextToString(row.SenderDisplayName) != "" {
			entry["sender"] = dbpkg.TextToString(row.SenderDisplayName)
		}
		if row.SenderChannelIdentityID.Valid {
			entry["contact_id"] = row.SenderChannelIdentityID.String()
		}

		messages = append(messages, entry)
	}

	return map[string]any{
		"ok":       true,
		"bot_id":   botID,
		"count":    len(messages),
		"messages": messages,
	}, nil
}

// extractTextContent deserialises the JSONB content column (a ModelMessage)
// and returns a human-readable text summary.
func extractTextContent(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var msg conversation.ModelMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return ""
	}

	if text := extractVisibleHistoryText(msg.Content); text != "" {
		return text
	}

	if names := extractHistoryToolCallNames(msg); len(names) > 0 {
		return "[tool_call: " + strings.Join(names, ", ") + "]"
	}

	if names := extractHistoryToolResultNames(msg.Content); len(names) > 0 {
		return "[tool_result: " + strings.Join(names, ", ") + "]"
	}

	return ""
}

type historyContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	URL      string          `json:"url,omitempty"`
	Emoji    string          `json:"emoji,omitempty"`
	ToolName string          `json:"toolName,omitempty"`
	Content  json.RawMessage `json:"content,omitempty"`
}

func extractVisibleHistoryText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return ""
		}
		if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
			if nested := extractVisibleHistoryText(json.RawMessage(trimmed)); nested != "" {
				return nested
			}
		}
		return trimmed
	}

	parts := extractHistoryContentParts(raw)
	if len(parts) > 0 {
		lines := make([]string, 0, len(parts))
		for _, part := range parts {
			partType := strings.ToLower(strings.TrimSpace(part.Type))
			switch {
			case partType == "reasoning", partType == "tool-call", partType == "tool-result":
				continue
			case partType == "text" && strings.TrimSpace(part.Text) != "":
				lines = append(lines, strings.TrimSpace(part.Text))
			case partType == "link" && strings.TrimSpace(part.URL) != "":
				lines = append(lines, strings.TrimSpace(part.URL))
			case partType == "emoji" && strings.TrimSpace(part.Emoji) != "":
				lines = append(lines, strings.TrimSpace(part.Emoji))
			case strings.TrimSpace(part.Text) != "":
				lines = append(lines, strings.TrimSpace(part.Text))
			}
		}
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}

	var object map[string]any
	if err := json.Unmarshal(raw, &object); err == nil {
		if value, ok := object["text"].(string); ok {
			return strings.TrimSpace(value)
		}
	}

	return ""
}

func extractHistoryToolCallNames(msg conversation.ModelMessage) []string {
	names := make([]string, 0, len(msg.ToolCalls))
	for _, part := range extractHistoryContentParts(msg.Content) {
		if strings.ToLower(strings.TrimSpace(part.Type)) != "tool-call" {
			continue
		}
		if name := strings.TrimSpace(part.ToolName); name != "" {
			names = append(names, name)
		}
	}
	if len(names) > 0 {
		return dedupeHistoryNames(names)
	}

	for _, tc := range msg.ToolCalls {
		if name := strings.TrimSpace(tc.Function.Name); name != "" {
			names = append(names, name)
		}
	}
	return dedupeHistoryNames(names)
}

func extractHistoryToolResultNames(raw json.RawMessage) []string {
	parts := extractHistoryContentParts(raw)
	if len(parts) == 0 {
		return nil
	}

	names := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.ToLower(strings.TrimSpace(part.Type)) != "tool-result" {
			continue
		}
		if name := strings.TrimSpace(part.ToolName); name != "" {
			names = append(names, name)
		}
	}
	return dedupeHistoryNames(names)
}

func extractHistoryContentParts(raw json.RawMessage) []historyContentPart {
	if len(raw) == 0 {
		return nil
	}

	var parts []historyContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		return parts
	}

	var encoded string
	if err := json.Unmarshal(raw, &encoded); err == nil {
		trimmed := strings.TrimSpace(encoded)
		if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
			return extractHistoryContentParts(json.RawMessage(trimmed))
		}
	}

	var object struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &object); err == nil && len(object.Content) > 0 {
		return extractHistoryContentParts(object.Content)
	}

	return nil
}

func dedupeHistoryNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

var timeFormats = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

func parseFlexibleTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range timeFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("unsupported time format")
}
