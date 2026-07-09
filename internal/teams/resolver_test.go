package teams

import (
	"context"
	"errors"
	"testing"
)

type fakeMembership struct {
	role             string
	found            bool
	err              error
	gotTeam, gotUser string
}

func (f *fakeMembership) Membership(_ context.Context, teamID, userID string) (string, bool, error) {
	f.gotTeam, f.gotUser = teamID, userID
	return f.role, f.found, f.err
}

func TestSingleTeamResolverMemberGetsScopedRole(t *testing.T) {
	m := &fakeMembership{role: "admin", found: true}
	got, err := NewSingleTeamResolver(m).Resolve(context.Background(), "u1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.TeamID != DefaultTeamID || got.UserID != "u1" || got.Role != "admin" {
		t.Fatalf("scope = %+v", got)
	}
	if m.gotTeam != DefaultTeamID || m.gotUser != "u1" {
		t.Fatalf("membership queried with team=%q user=%q", m.gotTeam, m.gotUser)
	}
}

func TestSingleTeamResolverNonMemberRejected(t *testing.T) {
	m := &fakeMembership{found: false}
	_, err := NewSingleTeamResolver(m).Resolve(context.Background(), "u1")
	if !errors.Is(err, ErrNotTeamMember) {
		t.Fatalf("err = %v, want ErrNotTeamMember", err)
	}
}

func TestSingleTeamResolverPropagatesLookupError(t *testing.T) {
	sentinel := errors.New("db down")
	m := &fakeMembership{err: sentinel}
	_, err := NewSingleTeamResolver(m).Resolve(context.Background(), "u1")
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
}
