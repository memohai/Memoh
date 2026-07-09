package models

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

type teamScopeFakeQueries struct{}

// Mirrors sqlc output for a single-parameter ForTeam query: the parameter is a
// bare pgtype.UUID, not a Params struct.
func (teamScopeFakeQueries) ListWidgetsForTeam(_ context.Context, teamID pgtype.UUID) ([]pgtype.UUID, error) {
	if !teamID.Valid {
		return nil, errors.New("team id not set")
	}
	return []pgtype.UUID{teamID}, nil
}

type listWidgetsByKindForTeamParams struct {
	Kind   string
	TeamID pgtype.UUID
}

// Mirrors sqlc output for a multi-parameter ForTeam query.
func (teamScopeFakeQueries) ListWidgetsByKindForTeam(_ context.Context, arg listWidgetsByKindForTeamParams) ([]pgtype.UUID, error) {
	if !arg.TeamID.Valid {
		return nil, errors.New("team id not set")
	}
	if arg.Kind != "gadget" {
		return nil, errors.New("kind not set")
	}
	return []pgtype.UUID{arg.TeamID}, nil
}

func (teamScopeFakeQueries) DeleteWidgetsForTeam(_ context.Context, teamID pgtype.UUID) error {
	if !teamID.Valid {
		return errors.New("team id not set")
	}
	return nil
}

func defaultTeamUUID(t *testing.T) pgtype.UUID {
	t.Helper()
	teamID, ok := parseTeamID(OpenSourceDefaultTeamID)
	if !ok {
		t.Fatal("parse default team id")
	}
	return teamID
}

func TestInvokeTeamQueryBareUUIDParam(t *testing.T) {
	got, err := InvokeTeamQuery[[]pgtype.UUID](context.Background(), teamScopeFakeQueries{}, "ListWidgetsForTeam", nil, func() ([]pgtype.UUID, error) {
		t.Fatal("fallback should not run when the ForTeam method exists")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("InvokeTeamQuery: %v", err)
	}
	if len(got) != 1 || got[0] != defaultTeamUUID(t) {
		t.Fatalf("expected default team id passed as bare param, got %v", got)
	}
}

func TestInvokeTeamQueryBareUUIDParamRejectsFields(t *testing.T) {
	fallbackRan := false
	_, err := InvokeTeamQuery[[]pgtype.UUID](context.Background(), teamScopeFakeQueries{}, "ListWidgetsForTeam", map[string]any{"Kind": "gadget"}, func() ([]pgtype.UUID, error) {
		fallbackRan = true
		return nil, nil
	})
	if err != nil {
		t.Fatalf("InvokeTeamQuery: %v", err)
	}
	if !fallbackRan {
		t.Fatal("expected fallback when fields cannot fit a bare uuid param")
	}
}

func TestInvokeTeamQueryStructParam(t *testing.T) {
	got, err := InvokeTeamQuery[[]pgtype.UUID](context.Background(), teamScopeFakeQueries{}, "ListWidgetsByKindForTeam", map[string]any{"Kind": "gadget"}, func() ([]pgtype.UUID, error) {
		t.Fatal("fallback should not run when the ForTeam method exists")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("InvokeTeamQuery: %v", err)
	}
	if len(got) != 1 || got[0] != defaultTeamUUID(t) {
		t.Fatalf("expected default team id in params struct, got %v", got)
	}
}

func TestInvokeTeamExecBareUUIDParam(t *testing.T) {
	err := InvokeTeamExec(context.Background(), teamScopeFakeQueries{}, "DeleteWidgetsForTeam", nil, func() error {
		t.Fatal("fallback should not run when the ForTeam method exists")
		return nil
	})
	if err != nil {
		t.Fatalf("InvokeTeamExec: %v", err)
	}
}
