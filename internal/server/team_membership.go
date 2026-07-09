package server

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	dbpkg "github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

// membershipQuery is the narrow slice of dbstore.Queries the reader needs.
type membershipQuery interface {
	GetTeamMembership(ctx context.Context, arg dbsqlc.GetTeamMembershipParams) (dbsqlc.GetTeamMembershipRow, error)
}

// membershipReader adapts the generated query to teams.MembershipReader,
// mapping a no-rows result to found=false rather than an error.
type membershipReader struct {
	q membershipQuery
}

func (r membershipReader) Membership(ctx context.Context, teamID, userID string) (string, bool, error) {
	tid, err := dbpkg.ParseUUID(teamID)
	if err != nil {
		return "", false, err
	}
	uid, err := dbpkg.ParseUUID(userID)
	if err != nil {
		return "", false, err
	}
	row, err := r.q.GetTeamMembership(ctx, dbsqlc.GetTeamMembershipParams{TeamID: tid, UserID: uid})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, dbpkg.ErrNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return row.Role, true, nil
}
