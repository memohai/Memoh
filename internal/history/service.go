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

func (s *Service) Create(ctx context.Context, userID string, req CreateRequest) (Record, error) {
	if len(req.Messages) == 0 {
		return Record{}, fmt.Errorf("messages are required")
	}
	pgID, err := parseUUID(userID)
	if err != nil {
		return Record{}, err
	}
	payload, err := json.Marshal(req.Messages)
	if err != nil {
		return Record{}, err
	}
	row, err := s.queries.CreateHistory(ctx, sqlc.CreateHistoryParams{
		Messages: payload,
		Timestamp: pgtype.Timestamptz{
			Time:  time.Now().UTC(),
			Valid: true,
		},
		User: pgID,
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

func (s *Service) List(ctx context.Context, userID string, limit int) ([]Record, error) {
	pgID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	rows, err := s.queries.ListHistoryByUser(ctx, sqlc.ListHistoryByUserParams{
		User:  pgID,
		Limit: int32(limit),
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

func (s *Service) DeleteByUser(ctx context.Context, userID string) error {
	pgID, err := parseUUID(userID)
	if err != nil {
		return err
	}
	return s.queries.DeleteHistoryByUser(ctx, pgID)
}

func toRecord(row sqlc.History) (Record, error) {
	var messages []map[string]interface{}
	if len(row.Messages) > 0 {
		if err := json.Unmarshal(row.Messages, &messages); err != nil {
			return Record{}, err
		}
	}
	record := Record{
		Messages: messages,
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
	if row.User.Valid {
		uid, err := uuid.FromBytes(row.User.Bytes[:])
		if err == nil {
			record.UserID = uid.String()
		}
	}
	return record, nil
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

