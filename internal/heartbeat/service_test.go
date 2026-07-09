package heartbeat

import (
	"context"
	"log/slog"
	"testing"

	"github.com/memohai/memoh/internal/teams"
)

func TestNormalizeHeartbeatIntervalDefault(t *testing.T) {
	t.Parallel()

	if got := normalizeHeartbeatInterval(0); got != 1440 {
		t.Fatalf("normalizeHeartbeatInterval(0) = %d, want 1440", got)
	}
	if got := normalizeHeartbeatInterval(-5); got != 1440 {
		t.Fatalf("normalizeHeartbeatInterval(-5) = %d, want 1440", got)
	}
	if got := normalizeHeartbeatInterval(60); got != 60 {
		t.Fatalf("normalizeHeartbeatInterval(60) = %d, want 60", got)
	}
}

type heartbeatTeamSessionCreator struct {
	teamID      string
	botID       string
	sessionType string
}

func (c *heartbeatTeamSessionCreator) CreateSession(_ context.Context, botID, sessionType string) (string, error) {
	c.botID = botID
	c.sessionType = sessionType
	return "11111111-1111-1111-1111-111111111111", nil
}

func (c *heartbeatTeamSessionCreator) CreateSessionForTeam(_ context.Context, teamID, botID, sessionType string) (string, error) {
	c.teamID = teamID
	c.botID = botID
	c.sessionType = sessionType
	return "22222222-2222-2222-2222-222222222222", nil
}

func TestCreateRunSessionPrefersTeamAwareCreator(t *testing.T) {
	creator := &heartbeatTeamSessionCreator{}
	svc := &Service{sessionCreator: creator, logger: slog.Default()}

	sessionID, pgSessionID := svc.createRunSession(context.Background(), teams.DefaultTeamID, "bot-1", "heartbeat")

	if sessionID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("session id = %q, want team-aware session id", sessionID)
	}
	if pgSessionID.String() != sessionID {
		t.Fatalf("pg session id = %q, want %q", pgSessionID.String(), sessionID)
	}
	if creator.teamID != teams.DefaultTeamID {
		t.Fatalf("team id = %q, want %q", creator.teamID, teams.DefaultTeamID)
	}
	if creator.botID != "bot-1" || creator.sessionType != "heartbeat" {
		t.Fatalf("creator got bot=%q type=%q", creator.botID, creator.sessionType)
	}
}

func TestHeartbeatRunContextUsesConfigTeamID(t *testing.T) {
	teamID := "33333333-3333-3333-3333-333333333333"
	ctx := teams.WithScope(context.Background(), teams.Scope{TeamID: "44444444-4444-4444-4444-444444444444"})

	scoped := withTeamScope(ctx, teamID)
	got := teams.ScopeOrDefault(scoped)
	if got.TeamID != teamID {
		t.Fatalf("team id = %q, want %q", got.TeamID, teamID)
	}
}
