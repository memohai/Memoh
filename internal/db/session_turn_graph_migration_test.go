package db

import (
	"context"
	"strings"
	"testing"
)

func TestSQLiteSessionTurnPointersRequireSameBot(t *testing.T) {
	dsn := tempSQLiteMigrationDSN(t)
	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, sqliteMigrationsFS(t), "up", nil); err != nil {
		t.Fatalf("fresh full migrate up failed: %v", err)
	}

	conn := openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, conn)
	ctx := context.Background()

	userID := "00000000-0000-0000-0000-000000000001"
	botID := "00000000-0000-0000-0000-000000000101"
	otherBotID := "00000000-0000-0000-0000-000000000102"
	originSessionID := "00000000-0000-0000-0000-000000000201"
	forkSessionID := "00000000-0000-0000-0000-000000000202"
	turnID := "00000000-0000-0000-0000-000000000301"
	otherTurnID := "00000000-0000-0000-0000-000000000302"

	for _, stmt := range []string{
		`INSERT INTO users(id,email) VALUES('` + userID + `','same-bot@example.com')`,
		`INSERT INTO bots(id,owner_user_id,type,name) VALUES('` + botID + `','` + userID + `','personal','same-bot-a')`,
		`INSERT INTO bots(id,owner_user_id,type,name) VALUES('` + otherBotID + `','` + userID + `','personal','same-bot-b')`,
		`INSERT INTO bot_sessions(id,bot_id,type,title) VALUES('` + originSessionID + `','` + botID + `','chat','Origin')`,
		`INSERT INTO bot_sessions(id,bot_id,type,title) VALUES('` + forkSessionID + `','` + botID + `','chat','Fork')`,
		`INSERT INTO bot_history_turns(id,bot_id,owner_session_id) VALUES('` + turnID + `','` + botID + `','` + originSessionID + `')`,
		`INSERT INTO bot_history_turns(id,bot_id,owner_session_id) VALUES('` + otherTurnID + `','` + otherBotID + `',NULL)`,
	} {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}

	if _, err := conn.ExecContext(ctx,
		`UPDATE bot_sessions SET default_head_turn_id = ?, forked_from_turn_id = ? WHERE id = ?`,
		turnID, turnID, forkSessionID,
	); err != nil {
		t.Fatalf("same-bot borrowed turn should be allowed: %v", err)
	}

	_, err := conn.ExecContext(ctx,
		`UPDATE bot_sessions SET default_head_turn_id = ? WHERE id = ?`,
		otherTurnID, forkSessionID,
	)
	if err == nil || !strings.Contains(err.Error(), "default_head_turn_id must reference a turn from the same bot") {
		t.Fatalf("cross-bot default_head_turn_id error = %v, want same-bot trigger error", err)
	}

	_, err = conn.ExecContext(ctx,
		`UPDATE bot_sessions SET forked_from_turn_id = ? WHERE id = ?`,
		otherTurnID, forkSessionID,
	)
	if err == nil || !strings.Contains(err.Error(), "forked_from_turn_id must reference a turn from the same bot") {
		t.Fatalf("cross-bot forked_from_turn_id error = %v, want same-bot trigger error", err)
	}
}

func TestPostgresSessionTurnPointerConstraintsUseBotBoundary(t *testing.T) {
	for _, path := range []string{
		"postgres/migrations/0001_init.up.sql",
		"postgres/migrations/0103_session_turn_graph.up.sql",
	} {
		t.Run(path, func(t *testing.T) {
			sql := readEmbeddedMigration(t, path)
			for _, want := range []string{
				"FOREIGN KEY (default_head_turn_id, bot_id) REFERENCES bot_history_turns(id, bot_id)",
				"ON DELETE SET NULL (default_head_turn_id)",
				"FOREIGN KEY (forked_from_turn_id, bot_id) REFERENCES bot_history_turns(id, bot_id)",
				"ON DELETE SET NULL (forked_from_turn_id)",
			} {
				if !strings.Contains(sql, want) {
					t.Fatalf("%s missing %q", path, want)
				}
			}
		})
	}
}
