package workspace

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/teams"
)

func teamIDFromContext(ctx context.Context) (pgtype.UUID, error) {
	teamID, err := teams.TeamUUID(ctx)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid team_id: %w", err)
	}
	return teamID, nil
}
