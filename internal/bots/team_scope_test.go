package bots

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/teams"
)

func TestTeamIDFromContextUsesDefaultScopeWhenMissing(t *testing.T) {
	got, err := teamIDFromContext(context.Background())
	if err != nil {
		t.Fatalf("teamIDFromContext returned error: %v", err)
	}
	if got.String() != teams.DefaultTeamID {
		t.Fatalf("team id = %q, want %q", got.String(), teams.DefaultTeamID)
	}
}

func TestTeamIDFromContextUsesScopedTeam(t *testing.T) {
	const teamID = "00000000-0000-0000-0000-000000000099"
	ctx := teams.WithScope(context.Background(), teams.Scope{TeamID: teamID})

	got, err := teamIDFromContext(ctx)
	if err != nil {
		t.Fatalf("teamIDFromContext returned error: %v", err)
	}
	if got.String() != teamID {
		t.Fatalf("team id = %q, want %q", got.String(), teamID)
	}
}

func TestApplyTeamIDSetsGeneratedParamFieldWhenPresent(t *testing.T) {
	const teamID = "00000000-0000-0000-0000-000000000042"
	parsed, err := teamIDFromContext(teams.WithScope(context.Background(), teams.Scope{TeamID: teamID}))
	if err != nil {
		t.Fatalf("teamIDFromContext returned error: %v", err)
	}
	params := struct {
		TeamID pgtype.UUID
	}{}

	applyTeamID(&params, parsed)

	if params.TeamID.String() != teamID {
		t.Fatalf("team id = %q, want %q", params.TeamID.String(), teamID)
	}
}
