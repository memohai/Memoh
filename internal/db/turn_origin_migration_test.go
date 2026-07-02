package db

import (
	"context"
	"database/sql"
	"testing"
)

// TestSQLiteTurnOriginBackfillGroupsRetrySiblings migrates a database with
// pre-0029 turn data and verifies the 0029 backfill: sibling turns whose
// request messages share the same text fingerprint join one request group
// (leader = earliest turn), while siblings with a different request stay in
// their own group (NULL request_group_id).
func TestSQLiteTurnOriginBackfillGroupsRetrySiblings(t *testing.T) {
	dsn := tempSQLiteMigrationDSN(t)
	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, sqliteMigrationsFSUpTo(t, 28), "up", nil); err != nil {
		t.Fatalf("migrate up to 0028: %v", err)
	}

	conn := openMigrationSQLite(t, dsn)
	ctx := context.Background()

	userID := "00000000-0000-0000-0000-000000000001"
	botID := "00000000-0000-0000-0000-000000000101"
	sessionID := "00000000-0000-0000-0000-000000000201"
	parentTurn := "00000000-0000-0000-0000-000000000301"
	siblingA := "00000000-0000-0000-0000-000000000302"
	siblingB := "00000000-0000-0000-0000-000000000303"
	siblingC := "00000000-0000-0000-0000-000000000304"

	for _, stmt := range []string{
		`INSERT INTO users(id,email) VALUES('` + userID + `','origin-backfill@example.com')`,
		`INSERT INTO bots(id,owner_user_id,type,name) VALUES('` + botID + `','` + userID + `','personal','origin-backfill')`,
		`INSERT INTO bot_sessions(id,bot_id,type,title) VALUES('` + sessionID + `','` + botID + `','chat','Backfill')`,
		`INSERT INTO bot_history_turns(id,bot_id,owner_session_id,created_at) VALUES('` + parentTurn + `','` + botID + `','` + sessionID + `','2026-07-01 10:00:00')`,
		`INSERT INTO bot_history_turns(id,bot_id,owner_session_id,parent_turn_id,created_at) VALUES('` + siblingA + `','` + botID + `','` + sessionID + `','` + parentTurn + `','2026-07-01 10:01:00')`,
		`INSERT INTO bot_history_turns(id,bot_id,owner_session_id,parent_turn_id,created_at) VALUES('` + siblingB + `','` + botID + `','` + sessionID + `','` + parentTurn + `','2026-07-01 10:02:00')`,
		`INSERT INTO bot_history_turns(id,bot_id,owner_session_id,parent_turn_id,created_at) VALUES('` + siblingC + `','` + botID + `','` + sessionID + `','` + parentTurn + `','2026-07-01 10:03:00')`,
	} {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	// Request messages: A and B carry the same text (a retry pair), C differs.
	for i, msg := range []struct {
		id, turnID, text string
	}{
		{"00000000-0000-0000-0000-000000000401", siblingA, "same question"},
		{"00000000-0000-0000-0000-000000000402", siblingB, "same question"},
		{"00000000-0000-0000-0000-000000000403", siblingC, "edited question"},
	} {
		if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, turn_id, turn_message_seq, role, content, display_text, created_at)
VALUES (?, ?, ?, ?, 1, 'user', '{"role":"user"}', ?, ?)`,
			msg.id, botID, sessionID, msg.turnID, msg.text,
			"2026-07-01 10:0"+string(rune('1'+i))+":00",
		); err != nil {
			t.Fatalf("insert request message %s: %v", msg.id, err)
		}
		if _, err := conn.ExecContext(ctx, `UPDATE bot_history_turns SET request_message_id = ? WHERE id = ?`, msg.id, msg.turnID); err != nil {
			t.Fatalf("link request message %s: %v", msg.id, err)
		}
	}
	closeMigrationSQLite(t, conn)

	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, sqliteMigrationsFS(t), "up", nil); err != nil {
		t.Fatalf("migrate up through 0029: %v", err)
	}

	conn = openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, conn)

	groupOf := func(turnID string) sql.NullString {
		t.Helper()
		var group sql.NullString
		if err := conn.QueryRowContext(ctx, `SELECT request_group_id FROM bot_history_turns WHERE id = ?`, turnID).Scan(&group); err != nil {
			t.Fatalf("select request_group_id for %s: %v", turnID, err)
		}
		return group
	}

	groupA := groupOf(siblingA)
	groupB := groupOf(siblingB)
	if !groupA.Valid || groupA.String != siblingA {
		t.Fatalf("sibling A group = %#v, want leader %s", groupA, siblingA)
	}
	if !groupB.Valid || groupB.String != siblingA {
		t.Fatalf("sibling B group = %#v, want inherited leader %s", groupB, siblingA)
	}
	if groupC := groupOf(siblingC); groupC.Valid {
		t.Fatalf("sibling C group = %#v, want NULL (own group)", groupC)
	}
	if groupParent := groupOf(parentTurn); groupParent.Valid {
		t.Fatalf("parent turn group = %#v, want NULL (own group)", groupParent)
	}
}
