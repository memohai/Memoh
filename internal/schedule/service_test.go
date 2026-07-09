package schedule

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/teams"
)

func TestGenerateTriggerToken(t *testing.T) {
	secret := "test-secret-key-for-schedule"
	svc := &Service{
		jwtSecret: secret,
		logger:    slog.Default(),
	}
	userID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	tok, err := svc.generateTriggerToken(userID)
	if err != nil {
		t.Fatalf("generateTriggerToken returned error: %v", err)
	}
	if !strings.HasPrefix(tok, "Bearer ") {
		t.Fatalf("expected Bearer prefix, got: %s", tok)
	}

	raw := strings.TrimPrefix(tok, "Bearer ")
	parsed, err := jwt.Parse(raw, func(_ *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	if err != nil {
		t.Fatalf("failed to parse JWT: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("expected MapClaims")
	}
	if sub, _ := claims["sub"].(string); sub != userID {
		t.Errorf("expected sub=%s, got=%s", userID, sub)
	}
	if uid, _ := claims["user_id"].(string); uid != userID {
		t.Errorf("expected user_id=%s, got=%s", userID, uid)
	}
	exp, _ := claims["exp"].(float64)
	if exp == 0 {
		t.Fatal("expected non-zero exp")
	}
	expTime := time.Unix(int64(exp), 0)
	if expTime.Before(time.Now().Add(9 * time.Minute)) {
		t.Error("token expires too soon")
	}
}

func TestGenerateTriggerToken_EmptySecret(t *testing.T) {
	svc := &Service{
		jwtSecret: "",
		logger:    slog.Default(),
	}
	_, err := svc.generateTriggerToken("user-123")
	if err == nil {
		t.Fatal("expected error for empty secret")
	}
}

func TestGenerateTriggerToken_EmptyUserID(t *testing.T) {
	svc := &Service{
		jwtSecret: "some-secret",
		logger:    slog.Default(),
	}
	_, err := svc.generateTriggerToken("")
	if err == nil {
		t.Fatal("expected error for empty user ID")
	}
}

type scheduleTeamSessionCreator struct {
	teamID      string
	botID       string
	sessionType string
}

func (c *scheduleTeamSessionCreator) CreateSession(_ context.Context, botID, sessionType string) (string, error) {
	c.botID = botID
	c.sessionType = sessionType
	return "11111111-1111-1111-1111-111111111111", nil
}

func (c *scheduleTeamSessionCreator) CreateSessionForTeam(_ context.Context, teamID, botID, sessionType string) (string, error) {
	c.teamID = teamID
	c.botID = botID
	c.sessionType = sessionType
	return "22222222-2222-2222-2222-222222222222", nil
}

func TestCreateRunSessionPrefersTeamAwareCreator(t *testing.T) {
	creator := &scheduleTeamSessionCreator{}
	svc := &Service{sessionCreator: creator, logger: slog.Default()}

	sessionID, pgSessionID := svc.createRunSession(context.Background(), teams.DefaultTeamID, "bot-1", "schedule")

	if sessionID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("session id = %q, want team-aware session id", sessionID)
	}
	if pgSessionID.String() != sessionID {
		t.Fatalf("pg session id = %q, want %q", pgSessionID.String(), sessionID)
	}
	if creator.teamID != teams.DefaultTeamID {
		t.Fatalf("team id = %q, want %q", creator.teamID, teams.DefaultTeamID)
	}
	if creator.botID != "bot-1" || creator.sessionType != "schedule" {
		t.Fatalf("creator got bot=%q type=%q", creator.botID, creator.sessionType)
	}
}

func TestScheduleJobContextUsesScheduleTeamID(t *testing.T) {
	teamID := "33333333-3333-3333-3333-333333333333"
	ctx := teams.WithScope(context.Background(), teams.Scope{TeamID: "44444444-4444-4444-4444-444444444444"})
	row := struct {
		TeamID pgtype.UUID
	}{TeamID: mustPGUUID(t, teamID)}

	scoped := withTeamScopeFromValue(ctx, row)
	got := teams.ScopeOrDefault(scoped)
	if got.TeamID != teamID {
		t.Fatalf("team id = %q, want %q", got.TeamID, teamID)
	}
}

func mustPGUUID(t *testing.T, id string) pgtype.UUID {
	t.Helper()
	var pgID pgtype.UUID
	if err := pgID.Scan(id); err != nil {
		t.Fatalf("scan uuid %q: %v", id, err)
	}
	return pgID
}
