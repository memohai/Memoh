package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestSQLiteListMessagesBeforeBySessionUsesSQLiteTimestampFormat(t *testing.T) {
	ctx := context.Background()
	conn := openBranchVisibleTestDB(t)
	defer func() { _ = conn.Close() }()

	botID := "00000000-0000-0000-0000-000000002001"
	sessionID := "00000000-0000-0000-0000-000000002002"
	branchID := "00000000-0000-0000-0000-000000002010"
	turnID := "00000000-0000-0000-0000-000000002011"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, channel_type, active_branch_id) VALUES (?, ?, ?)`, sessionID, "local", branchID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_session_branches (id, session_id, created_at, updated_at) VALUES (?, ?, ?, ?)`, branchID, sessionID, "2026-06-13 19:53:50", "2026-06-13 19:53:50"); err != nil {
		t.Fatalf("insert branch: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_history_turns (id, session_id, branch_id, turn_seq, status, created_at, updated_at, completed_at) VALUES (?, ?, ?, ?, 'completed', ?, ?, ?)`, turnID, sessionID, branchID, 1, "2026-06-13 19:53:50", "2026-06-13 19:53:50", "2026-06-13 19:53:50"); err != nil {
		t.Fatalf("insert turn: %v", err)
	}
	for _, item := range []struct {
		id      string
		role    string
		content string
		seq     int
	}{
		{"00000000-0000-0000-0000-000000002003", "user", `{"role":"user","content":"hello"}`, 1},
		{"00000000-0000-0000-0000-000000002004", "assistant", `{"role":"assistant","content":"hi"}`, 2},
	} {
		_, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, branch_id, branch_seq, turn_id, turn_message_seq, role, content, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			item.id,
			botID,
			sessionID,
			branchID,
			item.seq,
			turnID,
			item.seq,
			item.role,
			item.content,
			"2026-06-13 19:53:50",
		)
		if err != nil {
			t.Fatalf("insert message %s: %v", item.id, err)
		}
	}

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	rows, err := q.ListMessagesBeforeBySession(ctx, pgsqlc.ListMessagesBeforeBySessionParams{
		SessionID: mustUUID(t, sessionID),
		CreatedAt: pgtype.Timestamptz{
			Time:  time.Date(2026, 6, 13, 19, 53, 50, 0, time.UTC),
			Valid: true,
		},
		MaxCount: 30,
	})
	if err != nil {
		t.Fatalf("list messages before: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("same-second messages must not be returned by before cursor, got %d", len(rows))
	}
}
