package inbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "inbox")),
	}
}

const (
	ActionTrigger = "trigger"
	ActionNotify  = "notify"
)

type Item struct {
	ID        string         `json:"id"`
	BotID     string         `json:"bot_id"`
	Source    string         `json:"source"`
	Header    map[string]any `json:"header"`
	Content   string         `json:"content"`
	Action    string         `json:"action"`
	IsRead    bool           `json:"is_read"`
	CreatedAt time.Time      `json:"created_at"`
	ReadAt    time.Time      `json:"read_at,omitempty"`
}

type CreateRequest struct {
	BotID   string         `json:"bot_id"`
	Source  string         `json:"source"`
	Header  map[string]any `json:"header"`
	Content string         `json:"content"`
	Action  string         `json:"action"`
}

type ListFilter struct {
	IsRead *bool  `json:"is_read,omitempty"`
	Source string `json:"source,omitempty"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

type SearchRequest struct {
	Query       string     `json:"query"`
	StartTime   *time.Time `json:"start_time,omitempty"`
	EndTime     *time.Time `json:"end_time,omitempty"`
	IncludeRead *bool      `json:"include_read,omitempty"`
	Limit       int        `json:"limit"`
}

type CountResult struct {
	Unread int64 `json:"unread"`
	Total  int64 `json:"total"`
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Item, error) {
	botUUID, err := db.ParseUUID(req.BotID)
	if err != nil {
		return Item{}, err
	}
	header, err := json.Marshal(req.Header)
	if err != nil {
		return Item{}, err
	}
	action := req.Action
	if action != ActionTrigger && action != ActionNotify {
		action = ActionNotify
	}

	row, err := s.queries.CreateInboxItem(ctx, sqlc.CreateInboxItemParams{
		BotID:   botUUID,
		Source:  req.Source,
		Header:  header,
		Content: req.Content,
		Action:  action,
	})
	if err != nil {
		return Item{}, err
	}
	return rowToItem(row), nil
}

func (s *Service) GetByID(ctx context.Context, botID, itemID string) (Item, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Item{}, err
	}
	itemUUID, err := db.ParseUUID(itemID)
	if err != nil {
		return Item{}, err
	}
	row, err := s.queries.GetInboxItemByID(ctx, sqlc.GetInboxItemByIDParams{
		ID:    itemUUID,
		BotID: botUUID,
	})
	if err != nil {
		return Item{}, err
	}
	return rowToItem(row), nil
}

func (s *Service) List(ctx context.Context, botID string, filter ListFilter) ([]Item, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.queries.ListInboxItems(ctx, sqlc.ListInboxItemsParams{
		BotID:      botUUID,
		IsRead:     boolOrNull(filter.IsRead),
		Source:      textOrNull(filter.Source),
		MaxCount:   int32(limit),
		ItemOffset: int32(filter.Offset),
	})
	if err != nil {
		return nil, err
	}
	return rowsToItems(rows), nil
}

func (s *Service) ListUnread(ctx context.Context, botID string, limit int) ([]Item, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.queries.ListUnreadInboxItems(ctx, sqlc.ListUnreadInboxItemsParams{
		BotID:    botUUID,
		MaxCount: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	return rowsToItems(rows), nil
}

func (s *Service) MarkRead(ctx context.Context, botID string, ids []string) error {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	pgIDs := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		pgID, err := db.ParseUUID(id)
		if err != nil {
			continue
		}
		pgIDs = append(pgIDs, pgID)
	}
	if len(pgIDs) == 0 {
		return nil
	}
	return s.queries.MarkInboxItemsRead(ctx, sqlc.MarkInboxItemsReadParams{
		BotID: botUUID,
		Ids:   pgIDs,
	})
}

func (s *Service) Search(ctx context.Context, botID string, req SearchRequest) ([]Item, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	params := sqlc.SearchInboxItemsParams{
		BotID:    botUUID,
		Query:    pgtype.Text{String: req.Query, Valid: true},
		MaxCount: int32(limit),
	}
	if req.StartTime != nil {
		params.StartTime = pgtype.Timestamptz{Time: *req.StartTime, Valid: true}
	}
	if req.EndTime != nil {
		params.EndTime = pgtype.Timestamptz{Time: *req.EndTime, Valid: true}
	}
	if req.IncludeRead != nil {
		params.IncludeRead = pgtype.Bool{Bool: *req.IncludeRead, Valid: true}
	}
	rows, err := s.queries.SearchInboxItems(ctx, params)
	if err != nil {
		return nil, err
	}
	return rowsToItems(rows), nil
}

func (s *Service) Count(ctx context.Context, botID string) (CountResult, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return CountResult{}, err
	}
	unread, err := s.queries.CountUnreadInboxItems(ctx, botUUID)
	if err != nil {
		return CountResult{}, err
	}
	total, err := s.queries.CountInboxItems(ctx, botUUID)
	if err != nil {
		return CountResult{}, err
	}
	return CountResult{Unread: unread, Total: total}, nil
}

func (s *Service) Delete(ctx context.Context, botID, itemID string) error {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	itemUUID, err := db.ParseUUID(itemID)
	if err != nil {
		return err
	}
	return s.queries.DeleteInboxItem(ctx, sqlc.DeleteInboxItemParams{
		ID:    itemUUID,
		BotID: botUUID,
	})
}

// --- conversion helpers ---

func rowToItem(row sqlc.BotInbox) Item {
	var header map[string]any
	if len(row.Header) > 0 {
		_ = json.Unmarshal(row.Header, &header)
	}
	if header == nil {
		header = map[string]any{}
	}
	return Item{
		ID:        pgUUIDToString(row.ID),
		BotID:     pgUUIDToString(row.BotID),
		Source:    row.Source,
		Header:    header,
		Content:   row.Content,
		Action:    row.Action,
		IsRead:    row.IsRead,
		CreatedAt: db.TimeFromPg(row.CreatedAt),
		ReadAt:    db.TimeFromPg(row.ReadAt),
	}
}

func rowsToItems(rows []sqlc.BotInbox) []Item {
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, rowToItem(row))
	}
	return items
}

func pgUUIDToString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

func textOrNull(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func boolOrNull(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}
