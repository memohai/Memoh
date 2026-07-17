//go:build integration

package db_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
)

func TestPostTeamMigrationsCoverEveryTeamWithoutBypassRLS(t *testing.T) {
	ctx := context.Background()
	adminDSN := teamMigrationDSN(t)
	admin := resetToEmpty(t)
	adminCfg := pgConfigFromDSN(t, adminDSN)

	role := "memoh_post_team_" + strconv.Itoa(os.Getpid()) + "_" + strconv.FormatUint(teamTestDBSeq.Add(1), 10)
	const password = "post_team_migration_password"
	if _, err := admin.Exec(ctx, "CREATE ROLE "+role+" LOGIN NOSUPERUSER NOCREATEROLE NOBYPASSRLS PASSWORD '"+password+"'"); err != nil {
		t.Fatalf("create migration role: %v", err)
	}
	if _, err := admin.Exec(ctx, "ALTER DATABASE "+adminCfg.Database+" OWNER TO "+role); err != nil {
		t.Fatalf("assign migration database owner: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, "ALTER DATABASE "+adminCfg.Database+" OWNER TO "+adminCfg.User)
		_, _ = admin.Exec(ctx, "DROP OWNED BY "+role)
		_, _ = admin.Exec(ctx, "DROP ROLE IF EXISTS "+role)
	})

	limitedDSN := db.DSN(config.PostgresConfig{
		Host:     adminCfg.Host,
		Port:     adminCfg.Port,
		User:     role,
		Password: password,
		Database: adminCfg.Database,
		SSLMode:  adminCfg.SSLMode,
	})
	migrateToVersion(t, limitedDSN, teamMigrationVersion)

	type fixture struct {
		teamID      string
		userID      string
		botID       string
		sessionID   string
		eventID     string
		eventCursor int64
	}
	fixtures := []fixture{
		{
			teamID:      "00000000-0000-0000-0000-000000000001",
			userID:      uuid.NewString(),
			botID:       uuid.NewString(),
			sessionID:   uuid.NewString(),
			eventID:     uuid.NewString(),
			eventCursor: 42,
		},
		{
			teamID:      "00000000-0000-0000-0000-0000000000f2",
			userID:      uuid.NewString(),
			botID:       uuid.NewString(),
			sessionID:   uuid.NewString(),
			eventID:     uuid.NewString(),
			eventCursor: 8_000_000_000_000_000,
		},
	}

	tx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatalf("begin fixture setup: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO teams (id, slug) VALUES ($1, 'other')`, fixtures[1].teamID); err != nil {
		t.Fatalf("insert second team: %v", err)
	}
	if _, err := tx.Exec(ctx, `
DROP INDEX IF EXISTS idx_bot_history_messages_event_id_unique;
DROP SEQUENCE IF EXISTS bot_session_event_cursor_seq;
`); err != nil {
		t.Fatalf("restore pre-migration objects: %v", err)
	}
	for i, f := range fixtures {
		if _, err := tx.Exec(ctx, `SELECT set_config('memoh.team_id', $1, true)`, f.teamID); err != nil {
			t.Fatalf("bind team %d: %v", i, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, team_id, username) VALUES ($1, $2, $3)`, f.userID, f.teamID, "migration-user-"+strconv.Itoa(i)); err != nil {
			t.Fatalf("insert team %d user: %v", i, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO bots (id, team_id, owner_user_id, name) VALUES ($1, $2, $3, $4)`, f.botID, f.teamID, f.userID, "migration-bot-"+strconv.Itoa(i)); err != nil {
			t.Fatalf("insert team %d bot: %v", i, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO bot_sessions (id, team_id, bot_id, channel_type) VALUES ($1, $2, $3, 'local')`, f.sessionID, f.teamID, f.botID); err != nil {
			t.Fatalf("insert team %d session: %v", i, err)
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO bot_session_events
  (id, team_id, bot_id, session_id, event_kind, event_data, external_message_id, received_at_ms)
VALUES ($1, $2, $3, $4, 'message', jsonb_build_object('event_cursor', $5::bigint), $6, 20)
`, f.eventID, f.teamID, f.botID, f.sessionID, f.eventCursor, "migration-event-"+strconv.Itoa(i)); err != nil {
			t.Fatalf("insert team %d event: %v", i, err)
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO bot_session_discuss_cursors
  (team_id, session_id, scope_key, consumed_cursor, consumed_event_cursor)
VALUES ($1, $2, 'default', 100, 0)
`, f.teamID, f.sessionID); err != nil {
			t.Fatalf("insert team %d discuss cursor: %v", i, err)
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_messages
  (id, team_id, bot_id, session_id, role, content, metadata, event_id, created_at)
VALUES
  ($1, $3, $4, $5, 'user', '[]'::jsonb, '{}'::jsonb, $6, '2026-01-01T00:00:00Z'),
  ($2, $3, $4, $5, 'user', '[]'::jsonb, '{}'::jsonb, $6, '2026-01-02T00:00:00Z')
`, uuid.NewString(), uuid.NewString(), f.teamID, f.botID, f.sessionID, f.eventID); err != nil {
			t.Fatalf("insert team %d duplicate event history: %v", i, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit fixtures: %v", err)
	}

	migrateAll(t, limitedDSN)

	var superuser, bypassRLS bool
	if err := admin.QueryRow(ctx, `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = $1`, role).Scan(&superuser, &bypassRLS); err != nil {
		t.Fatalf("read migration role attributes: %v", err)
	}
	if superuser || bypassRLS {
		t.Fatalf("migration role privileges = superuser:%t bypassrls:%t", superuser, bypassRLS)
	}
	for i, f := range fixtures {
		var consumedCursor int64
		if err := admin.QueryRow(ctx, `SELECT consumed_event_cursor FROM bot_session_discuss_cursors WHERE team_id = $1 AND session_id = $2`, f.teamID, f.sessionID).Scan(&consumedCursor); err != nil {
			t.Fatalf("read team %d discuss cursor: %v", i, err)
		}
		if consumedCursor != f.eventCursor {
			t.Errorf("team %d consumed event cursor = %d, want %d", i, consumedCursor, f.eventCursor)
		}
		var linked, marked int
		if err := admin.QueryRow(ctx, `
SELECT
  COUNT(*) FILTER (WHERE event_id = $2),
  COUNT(*) FILTER (WHERE metadata ? '_migration_0115_history_event_dedup')
FROM bot_history_messages
WHERE team_id = $1
`, f.teamID, f.eventID).Scan(&linked, &marked); err != nil {
			t.Fatalf("read team %d dedup state: %v", i, err)
		}
		if linked != 1 || marked != 1 {
			t.Errorf("team %d dedup state = linked:%d marked:%d, want 1/1", i, linked, marked)
		}
	}
	var sequenceFloor int64
	if err := admin.QueryRow(ctx, `SELECT last_value FROM bot_session_event_cursor_seq`).Scan(&sequenceFloor); err != nil {
		t.Fatalf("read event cursor sequence: %v", err)
	}
	if sequenceFloor != fixtures[1].eventCursor {
		t.Errorf("event cursor sequence floor = %d, want %d", sequenceFloor, fixtures[1].eventCursor)
	}
	var policyExpression string
	if err := admin.QueryRow(ctx, `
SELECT pg_get_expr(polqual, polrelid)
FROM pg_policy
WHERE polrelid = 'public.teams'::regclass AND polname = 'teams_self_select'
`).Scan(&policyExpression); err != nil {
		t.Fatalf("read restored team policy: %v", err)
	}
	if !strings.Contains(policyExpression, "memoh_current_team_id") {
		t.Errorf("teams_self_select policy was not restored: %s", policyExpression)
	}

	migrateToVersion(t, limitedDSN, 115)
	stepDown(t, limitedDSN, 1)
	for i, f := range fixtures {
		var linked, marked int
		if err := admin.QueryRow(ctx, `
SELECT
  COUNT(*) FILTER (WHERE event_id = $2),
  COUNT(*) FILTER (WHERE metadata ? '_migration_0115_history_event_dedup')
FROM bot_history_messages
WHERE team_id = $1
`, f.teamID, f.eventID).Scan(&linked, &marked); err != nil {
			t.Fatalf("read team %d rollback state: %v", i, err)
		}
		if linked != 2 || marked != 0 {
			t.Errorf("team %d rollback state = linked:%d marked:%d, want 2/0", i, linked, marked)
		}
	}
	stepDown(t, limitedDSN, 1)
	var cursorColumnExists bool
	if err := admin.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.columns
  WHERE table_schema = 'public'
    AND table_name = 'bot_session_discuss_cursors'
    AND column_name = 'consumed_event_cursor'
)
`).Scan(&cursorColumnExists); err != nil {
		t.Fatalf("check discuss cursor rollback: %v", err)
	}
	if cursorColumnExists {
		t.Error("0114 down left consumed_event_cursor behind")
	}
}
