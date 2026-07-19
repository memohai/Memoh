package message

import (
	"context"
	"strings"

	dbpkg "github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func (s *DBService) ListExternalMessagePositionsBySession(
	ctx context.Context,
	sessionID string,
	externalMessageIDs []string,
) ([]ExternalMessagePosition, error) {
	pgSessionID, err := dbpkg.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	ids := uniqueExternalMessageIDs(externalMessageIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.queries.ListExternalMessagePositionsBySession(ctx, sqlc.ListExternalMessagePositionsBySessionParams{
		SessionID:          pgSessionID,
		ExternalMessageIds: ids,
	})
	if err != nil {
		return nil, err
	}
	positions := make([]ExternalMessagePosition, 0, len(rows))
	for _, row := range rows {
		positions = append(positions, ExternalMessagePosition{
			ExternalMessageID:   row.ExternalMessageID,
			TurnPosition:        row.TurnPosition,
			TurnMessageSequence: row.TurnMessageSeq,
		})
	}
	return positions, nil
}

func uniqueExternalMessageIDs(ids []string) []string {
	unique := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}
