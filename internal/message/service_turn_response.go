package message

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

type visibleTurnResponseQueries interface {
	ListVisibleTurnResponsesByRequest(context.Context, sqlc.ListVisibleTurnResponsesByRequestParams) ([]sqlc.ListVisibleTurnResponsesByRequestRow, error)
}

// LatestTurnResponseAtBySession returns the newest visible assistant/tool timestamp.
func (s *DBService) LatestTurnResponseAtBySession(ctx context.Context, sessionID string) (time.Time, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return time.Time{}, err
	}
	latest, err := s.queries.GetLatestActiveTurnResponseAtBySession(ctx, pgSessionID)
	if err != nil {
		return time.Time{}, err
	}
	if !latest.Valid {
		return time.Time{}, nil
	}
	return latest.Time.UTC(), nil
}

// ListUncoveredTurnResponsesBySession returns non-leading turn rows not
// replaced by the active compaction projection.
func (s *DBService) ListUncoveredTurnResponsesBySession(ctx context.Context, sessionID string, since time.Time, coveredMessageIDs []string) ([]Message, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	pgCoveredMessageIDs := make([]pgtype.UUID, 0, len(coveredMessageIDs))
	for _, messageID := range coveredMessageIDs {
		pgMessageID, err := dbpkg.ParseUUID(messageID)
		if err != nil {
			return nil, err
		}
		pgCoveredMessageIDs = append(pgCoveredMessageIDs, pgMessageID)
	}
	rows, err := s.queries.ListUncoveredTurnResponsesBySession(ctx, sqlc.ListUncoveredTurnResponsesBySessionParams{
		SessionID:         pgSessionID,
		CreatedAt:         pgtype.Timestamptz{Time: since, Valid: true},
		CoveredMessageIds: pgCoveredMessageIDs,
	})
	if err != nil {
		return nil, err
	}
	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, Message{
			ID:                  row.ID.String(),
			Role:                row.Role,
			Content:             row.Content,
			TurnPosition:        row.TurnPosition,
			TurnMessageSequence: row.TurnMessageSeq,
			CreatedAt:           row.CreatedAt.Time,
		})
	}
	return messages, nil
}

func (s *DBService) ListVisibleTurnResponsesByRequest(ctx context.Context, sessionID, requestMessageID string) ([]Message, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	pgRequestMessageID, err := dbpkg.ParseUUID(requestMessageID)
	if err != nil {
		return nil, err
	}
	queries, ok := s.queries.(visibleTurnResponseQueries)
	if !ok {
		return nil, errors.New("visible turn response reader is not configured")
	}
	rows, err := queries.ListVisibleTurnResponsesByRequest(ctx, sqlc.ListVisibleTurnResponsesByRequestParams{
		SessionID:        pgSessionID,
		RequestMessageID: pgRequestMessageID,
	})
	if err != nil {
		return nil, err
	}
	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, Message{
			ID:        row.ID.String(),
			SessionID: sessionID,
			Role:      row.Role,
			Content:   row.Content,
			CreatedAt: row.CreatedAt.Time,
		})
	}
	return messages, nil
}
