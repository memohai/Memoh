package db

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCompactionTopologyRevisionMigrationPostgresPath(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin migration test: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	schema := "compaction_topology_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+quotedSchema); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	if _, err := tx.Exec(ctx, `
CREATE TABLE bot_sessions (
  id UUID PRIMARY KEY
);
CREATE TABLE bot_history_message_compacts (
  id UUID PRIMARY KEY
);
CREATE TABLE bot_history_messages (
  id UUID PRIMARY KEY,
  bot_id UUID NOT NULL,
  session_id UUID REFERENCES bot_sessions(id) ON DELETE SET NULL,
  content JSONB NOT NULL DEFAULT '{}',
  metadata JSONB NOT NULL DEFAULT '{}',
  turn_id UUID,
  turn_position BIGINT,
  turn_message_seq BIGINT,
  turn_visible BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`); err != nil {
		t.Fatalf("create pre-0113 schema: %v", err)
	}

	up := readEmbeddedMigration(t, "postgres/migrations/0114_compaction_topology_positions.up.sql")
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatalf("apply 0113 up: %v", err)
	}
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatalf("reapply 0113 up: %v", err)
	}

	sessionID, botID := uuid.New(), uuid.New()
	activeID, hiddenID := uuid.New(), uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO bot_sessions (id) VALUES ($1)`, sessionID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_messages (
  id, bot_id, session_id, turn_id, turn_position, turn_message_seq, turn_visible
) VALUES ($1, $2, $3, $4, 1, 1, true)
`, activeID, botID, sessionID, uuid.New()); err != nil {
		t.Fatalf("insert active message: %v", err)
	}
	assertTopologyRevision(t, ctx, tx, sessionID, 1, map[int64]int64{1: 1})
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_messages (
  id, bot_id, session_id, turn_id, turn_position, turn_message_seq, turn_visible
) VALUES ($1, $2, $3, $4, 2, 1, false)
`, hiddenID, botID, sessionID, uuid.New()); err != nil {
		t.Fatalf("insert hidden message: %v", err)
	}
	assertTopologyRevision(t, ctx, tx, sessionID, 1, map[int64]int64{1: 1})

	if _, err := tx.Exec(ctx, `SAVEPOINT rolled_back_activation`); err != nil {
		t.Fatalf("savepoint hidden activation: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE bot_history_messages SET turn_visible = true WHERE id = $1`, hiddenID); err != nil {
		t.Fatalf("activate hidden message: %v", err)
	}
	assertTopologyRevision(t, ctx, tx, sessionID, 2, map[int64]int64{1: 1, 2: 2})
	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT rolled_back_activation`); err != nil {
		t.Fatalf("rollback hidden activation: %v", err)
	}
	assertTopologyRevision(t, ctx, tx, sessionID, 1, map[int64]int64{1: 1})

	if _, err := tx.Exec(ctx, `UPDATE bot_history_messages SET turn_visible = true WHERE id = $1`, hiddenID); err != nil {
		t.Fatalf("commit hidden activation: %v", err)
	}
	assertTopologyRevision(t, ctx, tx, sessionID, 2, map[int64]int64{1: 1, 2: 2})
	if _, err := tx.Exec(ctx, `UPDATE bot_history_messages SET metadata = '{"label":"unchanged topology"}' WHERE id = $1`, hiddenID); err != nil {
		t.Fatalf("update unrelated metadata: %v", err)
	}
	assertTopologyRevision(t, ctx, tx, sessionID, 2, map[int64]int64{1: 1, 2: 2})
	if _, err := tx.Exec(ctx, `UPDATE bot_history_messages SET turn_position = 3 WHERE id = $1`, hiddenID); err != nil {
		t.Fatalf("move active message: %v", err)
	}
	assertTopologyRevision(t, ctx, tx, sessionID, 3, map[int64]int64{1: 1, 2: 3, 3: 3})
	if _, err := tx.Exec(ctx, `UPDATE bot_history_messages SET metadata = '{"trigger_mode":"passive_sync"}' WHERE id = $1`, hiddenID); err != nil {
		t.Fatalf("make message passive: %v", err)
	}
	assertTopologyRevision(t, ctx, tx, sessionID, 4, map[int64]int64{1: 1, 2: 3, 3: 4})
	if _, err := tx.Exec(ctx, `DELETE FROM bot_history_messages WHERE id = $1`, activeID); err != nil {
		t.Fatalf("delete active message: %v", err)
	}
	assertTopologyRevision(t, ctx, tx, sessionID, 5, map[int64]int64{1: 5, 2: 3, 3: 4})

	compactID := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_message_compacts (id) VALUES ($1)`, compactID); err != nil {
		t.Fatalf("insert compact: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_message_compact_topology (
  compact_id, session_id, topology_revision, range_start_turn_position, range_end_turn_position
) VALUES ($1, $2, 5, 1, 3)
`, compactID, sessionID); err != nil {
		t.Fatalf("insert compact topology: %v", err)
	}

	down := readEmbeddedMigration(t, "postgres/migrations/0114_compaction_topology_positions.down.sql")
	if _, err := tx.Exec(ctx, down); err != nil {
		t.Fatalf("apply 0113 down: %v", err)
	}
	if _, err := tx.Exec(ctx, down); err != nil {
		t.Fatalf("reapply 0113 down: %v", err)
	}
	var topologyTable *string
	if err := tx.QueryRow(ctx, `SELECT to_regclass('bot_history_topology_positions')::text`).Scan(&topologyTable); err != nil {
		t.Fatalf("check topology table removal: %v", err)
	}
	if topologyTable != nil {
		t.Fatalf("topology positions table remains after down: %s", *topologyTable)
	}
}

func TestCompactionTopologyRevisionDeferredCommitAndLifecyclePostgresPath(t *testing.T) {
	pool := openTopologyMigrationTestPool(t)
	ctx := context.Background()
	sessionA, sessionB := uuid.New(), uuid.New()
	messageA, messageB := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO bot_sessions (id) VALUES ($1), ($2)`, sessionA, sessionB); err != nil {
		t.Fatalf("insert concurrent sessions: %v", err)
	}
	for _, message := range []struct {
		id        uuid.UUID
		sessionID uuid.UUID
	}{
		{id: messageA, sessionID: sessionA},
		{id: messageB, sessionID: sessionB},
	} {
		if _, err := pool.Exec(ctx, `
INSERT INTO bot_history_messages (
  id, bot_id, session_id, turn_id, turn_position, turn_message_seq, turn_visible
) VALUES ($1, $2, $3, $4, 1, 1, true)
`, message.id, uuid.New(), message.sessionID, uuid.New()); err != nil {
			t.Fatalf("insert committed active message: %v", err)
		}
	}
	assertCommittedTopologyRevision(t, ctx, pool, sessionA, 1)
	assertCommittedTopologyRevision(t, ctx, pool, sessionB, 1)

	rolledBack, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin rolled-back topology mutation: %v", err)
	}
	if _, err := rolledBack.Exec(ctx, `
INSERT INTO bot_history_messages (
  id, bot_id, session_id, turn_id, turn_position, turn_message_seq, turn_visible
) VALUES ($1, $2, $3, $4, 2, 1, true)
`, uuid.New(), uuid.New(), sessionA, uuid.New()); err != nil {
		t.Fatalf("insert rolled-back topology mutation: %v", err)
	}
	var inTransactionRevision int64
	if err := rolledBack.QueryRow(ctx, `SELECT revision FROM bot_history_topology_counters WHERE session_id = $1`, sessionA).Scan(&inTransactionRevision); err != nil {
		t.Fatalf("read deferred topology revision: %v", err)
	}
	if inTransactionRevision != 1 {
		t.Fatalf("revision before deferred commit = %d, want 1", inTransactionRevision)
	}
	if err := rolledBack.Rollback(ctx); err != nil {
		t.Fatalf("rollback topology mutation: %v", err)
	}
	assertCommittedTopologyRevision(t, ctx, pool, sessionA, 1)

	transactions := make([]pgx.Tx, 2)
	for index, position := range []int64{2, 3} {
		transactions[index], err = pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin concurrent topology mutation %d: %v", index, err)
		}
		if _, err := transactions[index].Exec(ctx, `
INSERT INTO bot_history_messages (
  id, bot_id, session_id, turn_id, turn_position, turn_message_seq, turn_visible
) VALUES ($1, $2, $3, $4, $5, 1, true)
`, uuid.New(), uuid.New(), sessionA, uuid.New(), position); err != nil {
			t.Fatalf("insert concurrent topology mutation %d: %v", index, err)
		}
	}
	commitErrs := make(chan error, len(transactions))
	for _, transaction := range transactions {
		go func() { commitErrs <- transaction.Commit(ctx) }()
	}
	for range transactions {
		if err := <-commitErrs; err != nil {
			t.Fatalf("commit concurrent topology mutation: %v", err)
		}
	}
	assertCommittedTopologyRevision(t, ctx, pool, sessionA, 3)

	moveA, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin A to B move: %v", err)
	}
	moveB, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin B to A move: %v", err)
	}
	if _, err := moveA.Exec(ctx, `UPDATE bot_history_messages SET session_id = $2, turn_position = 4 WHERE id = $1`, messageA, sessionB); err != nil {
		t.Fatalf("move message A to session B: %v", err)
	}
	if _, err := moveB.Exec(ctx, `UPDATE bot_history_messages SET session_id = $2, turn_position = 4 WHERE id = $1`, messageB, sessionA); err != nil {
		t.Fatalf("move message B to session A: %v", err)
	}
	commitErrs = make(chan error, 2)
	go func() { commitErrs <- moveA.Commit(ctx) }()
	go func() { commitErrs <- moveB.Commit(ctx) }()
	for range 2 {
		if err := <-commitErrs; err != nil {
			t.Fatalf("commit opposing session move: %v", err)
		}
	}
	assertCommittedTopologyRevision(t, ctx, pool, sessionA, 5)
	assertCommittedTopologyRevision(t, ctx, pool, sessionB, 3)

	deletedSession, deletedMessage, compactID := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO bot_sessions (id) VALUES ($1)`, deletedSession); err != nil {
		t.Fatalf("insert deleted session: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO bot_history_messages (
  id, bot_id, session_id, turn_id, turn_position, turn_message_seq, turn_visible
) VALUES ($1, $2, $3, $4, 1, 1, true)
`, deletedMessage, uuid.New(), deletedSession, uuid.New()); err != nil {
		t.Fatalf("insert deleted session message: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO bot_history_message_compacts (id) VALUES ($1)`, compactID); err != nil {
		t.Fatalf("insert retained compact: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO bot_history_message_compact_topology (
  compact_id, session_id, topology_revision, range_start_turn_position, range_end_turn_position
) VALUES ($1, $2, 1, 1, 1)
`, compactID, deletedSession); err != nil {
		t.Fatalf("insert deleted session compact topology: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM bot_sessions WHERE id = $1`, deletedSession); err != nil {
		t.Fatalf("delete session with topology state: %v", err)
	}
	var messageSession *uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT session_id FROM bot_history_messages WHERE id = $1`, deletedMessage).Scan(&messageSession); err != nil {
		t.Fatalf("read detached message: %v", err)
	}
	if messageSession != nil {
		t.Fatalf("deleted session message retained session %s", *messageSession)
	}
	for _, table := range []string{
		"bot_history_topology_counters",
		"bot_history_topology_positions",
		"bot_history_message_compact_topology",
	} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM `+pgx.Identifier{table}.Sanitize()+` WHERE session_id = $1`, deletedSession).Scan(&count); err != nil {
			t.Fatalf("count deleted session rows in %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("deleted session retained %d rows in %s", count, table)
		}
	}
	var compactCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bot_history_message_compacts WHERE id = $1`, compactID).Scan(&compactCount); err != nil {
		t.Fatalf("count retained compact: %v", err)
	}
	if compactCount != 1 {
		t.Fatalf("session deletion removed compact log")
	}

	up := readEmbeddedMigration(t, "postgres/migrations/0114_compaction_topology_positions.up.sql")
	if _, err := pool.Exec(ctx, up); err != nil {
		t.Fatalf("reapply 0113 with topology data: %v", err)
	}
	assertCommittedTopologyRevision(t, ctx, pool, sessionA, 5)

	down := readEmbeddedMigration(t, "postgres/migrations/0114_compaction_topology_positions.down.sql")
	if _, err := pool.Exec(ctx, down); err != nil {
		t.Fatalf("apply 0113 down after data: %v", err)
	}
	if _, err := pool.Exec(ctx, down); err != nil {
		t.Fatalf("reapply 0113 down after data: %v", err)
	}
	for _, relation := range []string{
		"bot_history_topology_counters",
		"bot_history_topology_positions",
		"bot_history_topology_pending",
		"bot_history_message_compact_topology",
	} {
		var found *string
		if err := pool.QueryRow(ctx, `SELECT to_regclass($1)::text`, relation).Scan(&found); err != nil {
			t.Fatalf("check %s removal: %v", relation, err)
		}
		if found != nil {
			t.Fatalf("relation %s remains after down", relation)
		}
	}
	for _, function := range []string{
		"enqueue_history_topology_position(uuid,bigint)",
		"record_history_message_topology_change()",
		"flush_history_topology_positions()",
		"cleanup_history_topology_session()",
	} {
		var found *string
		if err := pool.QueryRow(ctx, `SELECT to_regprocedure($1)::text`, function).Scan(&found); err != nil {
			t.Fatalf("check %s removal: %v", function, err)
		}
		if found != nil {
			t.Fatalf("function %s remains after down", function)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE bot_history_messages SET content = '{"after_down":true}' WHERE id = $1`, messageA); err != nil {
		t.Fatalf("base message DML failed after down: %v", err)
	}
}

func openTopologyMigrationTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect topology migration postgres: %v", err)
	}
	schema := "compaction_topology_commit_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		admin.Close()
		t.Fatalf("create topology commit schema: %v", err)
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		admin.Close()
		t.Fatalf("parse topology migration config: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		admin.Close()
		t.Fatalf("open topology migration schema: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+quotedSchema+" CASCADE")
		admin.Close()
	})
	if _, err := pool.Exec(ctx, `
CREATE TABLE bot_sessions (id UUID PRIMARY KEY);
CREATE TABLE bot_history_message_compacts (id UUID PRIMARY KEY);
CREATE TABLE bot_history_messages (
  id UUID PRIMARY KEY,
  bot_id UUID NOT NULL,
  session_id UUID REFERENCES bot_sessions(id) ON DELETE SET NULL,
  content JSONB NOT NULL DEFAULT '{}',
  metadata JSONB NOT NULL DEFAULT '{}',
  turn_id UUID,
  turn_position BIGINT,
  turn_message_seq BIGINT,
  turn_visible BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`); err != nil {
		t.Fatalf("create topology commit fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, readEmbeddedMigration(t, "postgres/migrations/0114_compaction_topology_positions.up.sql")); err != nil {
		t.Fatalf("apply topology commit migration: %v", err)
	}
	return pool
}

func assertCommittedTopologyRevision(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sessionID uuid.UUID, want int64) {
	t.Helper()
	var revision int64
	if err := pool.QueryRow(ctx, `SELECT revision FROM bot_history_topology_counters WHERE session_id = $1`, sessionID).Scan(&revision); err != nil {
		t.Fatalf("read committed topology revision: %v", err)
	}
	if revision != want {
		t.Fatalf("committed topology revision = %d, want %d", revision, want)
	}
}

func assertTopologyRevision(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	sessionID uuid.UUID,
	wantRevision int64,
	wantPositions map[int64]int64,
) {
	t.Helper()
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS history_topology_pending_flush IMMEDIATE`); err != nil {
		t.Fatalf("flush topology revision trigger: %v", err)
	}
	if _, err := tx.Exec(ctx, `SET CONSTRAINTS history_topology_pending_flush DEFERRED`); err != nil {
		t.Fatalf("defer topology revision trigger: %v", err)
	}
	var revision int64
	if err := tx.QueryRow(ctx, `
SELECT revision
FROM bot_history_topology_counters
WHERE session_id = $1
`, sessionID).Scan(&revision); err != nil {
		t.Fatalf("read topology counter: %v", err)
	}
	if revision != wantRevision {
		t.Fatalf("topology revision = %d, want %d", revision, wantRevision)
	}
	rows, err := tx.Query(ctx, `
SELECT turn_position, revision
FROM bot_history_topology_positions
WHERE session_id = $1
`, sessionID)
	if err != nil {
		t.Fatalf("read topology positions: %v", err)
	}
	defer rows.Close()
	got := make(map[int64]int64)
	for rows.Next() {
		var position, positionRevision int64
		if err := rows.Scan(&position, &positionRevision); err != nil {
			t.Fatalf("scan topology position: %v", err)
		}
		got[position] = positionRevision
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate topology positions: %v", err)
	}
	if len(got) != len(wantPositions) {
		t.Fatalf("topology positions = %#v, want %#v", got, wantPositions)
	}
	for position, want := range wantPositions {
		if got[position] != want {
			t.Fatalf("topology position %d revision = %d, want %d", position, got[position], want)
		}
	}
}
