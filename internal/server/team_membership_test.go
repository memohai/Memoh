package server

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	dbpkg "github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

type fakeMembershipQueries struct {
	row dbsqlc.GetTeamMembershipRow
	err error
}

func (f fakeMembershipQueries) GetTeamMembership(_ context.Context, _ dbsqlc.GetTeamMembershipParams) (dbsqlc.GetTeamMembershipRow, error) {
	return f.row, f.err
}

func TestMembershipReaderFoundReturnsRole(t *testing.T) {
	r := membershipReader{q: fakeMembershipQueries{row: dbsqlc.GetTeamMembershipRow{Role: "admin"}}}
	role, found, err := r.Membership(context.Background(), "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002")
	if err != nil || !found || role != "admin" {
		t.Fatalf("role=%q found=%v err=%v", role, found, err)
	}
}

func TestMembershipReaderNotFoundIsNotError(t *testing.T) {
	r := membershipReader{q: fakeMembershipQueries{err: pgx.ErrNoRows}}
	_, found, err := r.Membership(context.Background(), "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002")
	if err != nil || found {
		t.Fatalf("found=%v err=%v, want found=false err=nil", found, err)
	}
}

func TestMembershipReaderInvalidUUIDReturnsError(t *testing.T) {
	r := membershipReader{q: fakeMembershipQueries{}}
	_, _, err := r.Membership(context.Background(), "not-a-uuid", "00000000-0000-0000-0000-000000000002")
	if err == nil {
		t.Fatal("expected error for invalid UUID")
	}
}

func TestMembershipReaderDBNotFoundIsAlsoCovered(t *testing.T) {
	r := membershipReader{q: fakeMembershipQueries{err: dbpkg.ErrNotFound}}
	_, found, err := r.Membership(context.Background(), "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002")
	if err != nil || found {
		t.Fatalf("found=%v err=%v, want found=false err=nil", found, err)
	}
}

func TestMembershipReaderPropagatesUnexpectedError(t *testing.T) {
	sentinel := errors.New("db down")
	r := membershipReader{q: fakeMembershipQueries{err: sentinel}}
	_, _, err := r.Membership(context.Background(), "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002")
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
}
