package history

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/sqlc"
)

const defaultListLimit = 50

type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "history")),
	}
}

func (s *Service) Create(ctx context.Context, botID, sessionID string, req CreateRequest) (Record, error) {
	if len(req.Messages) == 0 {
		return Record{}, fmt.Errorf("messages are required")
	}
	botUUID, err := parseUUID(botID)
	if err != nil {
		return Record{}, err
	}
	trimmedSession := strings.TrimSpace(sessionID)
	if trimmedSession == "" {
		return Record{}, fmt.Errorf("session id is required")
	}
	payload, err := json.Marshal(req.Messages)
	if err != nil {
		return Record{}, err
	}
	meta := req.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	metaPayload, err := json.Marshal(meta)
	if err != nil {
		return Record{}, err
	}
	row, err := s.queries.CreateHistory(ctx, sqlc.CreateHistoryParams{
		BotID:     botUUID,
		SessionID: trimmedSession,
		Messages:  payload,
		Metadata:  metaPayload,
		Skills:    normalizeSkills(req.Skills),
		Timestamp: pgtype.Timestamptz{
			Time:  time.Now().UTC(),
			Valid: true,
		},
	})
	if err != nil {
		return Record{}, err
	}
	return toRecord(row)
}

func (s *Service) Get(ctx context.Context, id string) (Record, error) {
	pgID, err := parseUUID(id)
	if err != nil {
		return Record{}, err
	}
	row, err := s.queries.GetHistoryByID(ctx, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, fmt.Errorf("history not found")
		}
		return Record{}, err
	}
	return toRecord(row)
}

func (s *Service) List(ctx context.Context, botID, sessionID string, limit int) ([]Record, error) {
	botUUID, err := parseUUID(botID)
	if err != nil {
		return nil, err
	}
	trimmedSession := strings.TrimSpace(sessionID)
	if trimmedSession == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	rows, err := s.queries.ListHistoryByBotSession(ctx, sqlc.ListHistoryByBotSessionParams{
		BotID:     botUUID,
		SessionID: trimmedSession,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, err
	}
	items := make([]Record, 0, len(rows))
	for _, row := range rows {
		record, err := toRecord(row)
		if err != nil {
			return nil, err
		}
		items = append(items, record)
	}
	return items, nil
}

func (s *Service) ListBySessionSince(ctx context.Context, botID, sessionID string, since time.Time) ([]Record, error) {
	botUUID, err := parseUUID(botID)
	if err != nil {
		return nil, err
	}
	trimmedSession := strings.TrimSpace(sessionID)
	if trimmedSession == "" {
		return nil, fmt.Errorf("session id is required")
	}
	rows, err := s.queries.ListHistoryByBotSessionSince(ctx, sqlc.ListHistoryByBotSessionSinceParams{
		BotID:     botUUID,
		SessionID: trimmedSession,
		Timestamp: pgtype.Timestamptz{
			Time:  since,
			Valid: true,
		},
	})
	if err != nil {
		return nil, err
	}
	items := make([]Record, 0, len(rows))
	for _, row := range rows {
		record, err := toRecord(row)
		if err != nil {
			return nil, err
		}
		items = append(items, record)
	}
	return items, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	pgID, err := parseUUID(id)
	if err != nil {
		return err
	}
	return s.queries.DeleteHistoryByID(ctx, pgID)
}

func (s *Service) DeleteBySession(ctx context.Context, botID, sessionID string) error {
	botUUID, err := parseUUID(botID)
	if err != nil {
		return err
	}
	trimmedSession := strings.TrimSpace(sessionID)
	if trimmedSession == "" {
		return fmt.Errorf("session id is required")
	}
	return s.queries.DeleteHistoryByBotSession(ctx, sqlc.DeleteHistoryByBotSessionParams{
		BotID:     botUUID,
		SessionID: trimmedSession,
	})
}

func toRecord(row sqlc.History) (Record, error) {
	var messages []map[string]any
	if len(row.Messages) > 0 {
		if err := json.Unmarshal(row.Messages, &messages); err != nil {
			return Record{}, err
		}
	}
	var metadata map[string]any
	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
			return Record{}, err
		}
	}
	record := Record{
		Messages: messages,
		Metadata: metadata,
		Skills:   normalizeSkills(row.Skills),
	}
	if row.Timestamp.Valid {
		record.Timestamp = row.Timestamp.Time
	}
	if row.ID.Valid {
		id, err := uuid.FromBytes(row.ID.Bytes[:])
		if err == nil {
			record.ID = id.String()
		}
	}
	if row.BotID.Valid {
		uid, err := uuid.FromBytes(row.BotID.Bytes[:])
		if err == nil {
			record.BotID = uid.String()
		}
	}
	record.SessionID = row.SessionID
	return record, nil
}

func normalizeSkills(skills []string) []string {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(skills))
	for _, skill := range skills {
		trimmed := strings.TrimSpace(skill)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func parseUUID(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID: %w", err)
	}
	var pgID pgtype.UUID
	pgID.Valid = true
	copy(pgID.Bytes[:], parsed[:])
	return pgID, nil
}
