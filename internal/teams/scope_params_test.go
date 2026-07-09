package teams

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestTeamUUIDUsesDefaultScopeWhenMissing(t *testing.T) {
	got, err := TeamUUID(context.Background())
	if err != nil {
		t.Fatalf("TeamUUID returned error: %v", err)
	}
	if got.String() != DefaultTeamID {
		t.Fatalf("team id = %q, want %q", got.String(), DefaultTeamID)
	}
}

func TestTeamUUIDUsesScopedTeam(t *testing.T) {
	const teamID = "00000000-0000-0000-0000-000000000099"
	ctx := WithScope(context.Background(), Scope{TeamID: teamID})

	got, err := TeamUUID(ctx)
	if err != nil {
		t.Fatalf("TeamUUID returned error: %v", err)
	}
	if got.String() != teamID {
		t.Fatalf("team id = %q, want %q", got.String(), teamID)
	}
}

func TestTeamUUIDOrZeroReturnsZeroOnInvalidScope(t *testing.T) {
	ctx := WithScope(context.Background(), Scope{TeamID: "not-a-uuid"})
	if got := TeamUUIDOrZero(ctx); got.Valid {
		t.Fatalf("expected invalid team id, got %q", got.String())
	}
}

func TestApplyTeamIDSetsGeneratedParamFieldWhenPresent(t *testing.T) {
	const teamID = "00000000-0000-0000-0000-000000000042"
	parsed, err := TeamUUID(WithScope(context.Background(), Scope{TeamID: teamID}))
	if err != nil {
		t.Fatalf("TeamUUID returned error: %v", err)
	}
	params := struct {
		TeamID pgtype.UUID
	}{}

	ApplyTeamID(&params, parsed)

	if params.TeamID.String() != teamID {
		t.Fatalf("team id = %q, want %q", params.TeamID.String(), teamID)
	}
}

func TestWithTeamIDReturnsCopyWithTeamID(t *testing.T) {
	const teamID = "00000000-0000-0000-0000-000000000042"
	parsed, err := TeamUUID(WithScope(context.Background(), Scope{TeamID: teamID}))
	if err != nil {
		t.Fatalf("TeamUUID returned error: %v", err)
	}
	type params struct {
		TeamID pgtype.UUID
		Name   string
	}
	got := WithTeamID(params{Name: "keep"}, parsed)
	if got.TeamID.String() != teamID {
		t.Fatalf("team id = %q, want %q", got.TeamID.String(), teamID)
	}
	if got.Name != "keep" {
		t.Fatalf("name = %q, want %q", got.Name, "keep")
	}
}
