package postgresstore

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/teams"
)

func teamUUIDFromContext(ctx context.Context) pgtype.UUID {
	return teams.TeamUUIDOrZero(ctx)
}
