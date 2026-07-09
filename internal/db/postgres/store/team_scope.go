package postgresstore

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/teams"
)

func teamUUIDFromContext(ctx context.Context) pgtype.UUID {
	id, err := db.ParseUUID(teams.ScopeOrDefault(ctx).TeamID)
	if err != nil {
		return pgtype.UUID{}
	}
	return id
}
