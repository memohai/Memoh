package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestFinalizeCompactionArtifactOverlapPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	botID, sessionID := testUUID(), testUUID()
	messageIDs := []pgtype.UUID{testUUID(), testUUID(), testUUID()}
	logIDs := []pgtype.UUID{testUUID(), testUUID()}
	insertFinalizeMessages(t, ctx, pool, botID, sessionID, messageIDs)
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, logIDs)
	versions := readMessageVersions(t, ctx, pool, messageIDs)
	control, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin overlap control transaction: %v", err)
	}
	defer func() { _ = control.Rollback(context.Background()) }()
	lockRows, err := control.Query(ctx, `
SELECT id FROM bot_history_messages WHERE id = ANY($1::uuid[]) ORDER BY id FOR UPDATE
`, messageIDs)
	if err != nil {
		t.Fatalf("lock overlap sources: %v", err)
	}
	for lockRows.Next() {
	}
	if err := lockRows.Err(); err != nil {
		t.Fatalf("consume overlap source locks: %v", err)
	}
	lockRows.Close()
	applications := []string{
		"finalize-overlap-" + logIDs[0].String(),
		"finalize-overlap-" + logIDs[1].String(),
	}
	connections := make([]*pgxpool.Conn, len(applications))
	for index, application := range applications {
		connections[index], err = pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("acquire overlap connection: %v", err)
		}
		defer connections[index].Release()
		if _, err := connections[index].Exec(ctx, `SELECT set_config('application_name', $1, false)`, application); err != nil {
			t.Fatalf("name overlap connection: %v", err)
		}
	}

	type outcome struct {
		index  int
		result sqlc.FinalizeCompactionArtifactRow
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	attempts := []struct {
		ids      []pgtype.UUID
		versions []string
	}{
		{ids: []pgtype.UUID{messageIDs[0], messageIDs[1]}, versions: []string{versions[0], versions[1]}},
		{ids: []pgtype.UUID{messageIDs[2], messageIDs[1]}, versions: []string{versions[2], versions[1]}},
	}
	for index, attempt := range attempts {
		go func() {
			<-start
			result, err := sqlc.New(connections[index]).FinalizeCompactionArtifact(ctx, finalizeParams(
				logIDs[index],
				botID,
				sessionID,
				attempt.ids,
				attempt.versions,
				"summary",
			))
			outcomes <- outcome{index: index, result: result, err: err}
		}()
	}
	close(start)
	waitForApplicationLocks(t, ctx, pool, applications)
	if err := control.Commit(ctx); err != nil {
		t.Fatalf("release overlap source locks: %v", err)
	}

	winner := -1
	for range attempts {
		got := <-outcomes
		if got.err != nil {
			t.Fatalf("partially overlapping finalize %d: %v", got.index, got.err)
		}
		if got.result.Finalized {
			if winner >= 0 {
				t.Fatalf("both partially overlapping attempts finalized: %d and %d", winner, got.index)
			}
			winner = got.index
		}
	}
	if winner < 0 {
		t.Fatal("neither partially overlapping attempt finalized")
	}
	assertClaimedBy(t, ctx, pool, attempts[winner].ids, logIDs[winner])
	loserExclusive := messageIDs[2]
	if winner == 1 {
		loserExclusive = messageIDs[0]
	}
	assertMessageUnclaimed(t, ctx, pool, loserExclusive)
	assertSingleSuccessfulLog(t, ctx, pool, logIDs, logIDs[winner])
}

func TestFinalizeCompactionArtifactMixedStaleBatchPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	ctx := context.Background()
	botID, sessionID, logID := testUUID(), testUUID(), testUUID()
	messageIDs := []pgtype.UUID{testUUID(), testUUID()}
	insertFinalizeMessages(t, ctx, pool, botID, sessionID, messageIDs)
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{logID})
	versions := readMessageVersions(t, ctx, pool, messageIDs)
	if _, err := pool.Exec(ctx, `UPDATE bot_history_messages SET metadata = '{"changed":true}' WHERE id = $1`, messageIDs[0]); err != nil {
		t.Fatalf("mutate one source in batch: %v", err)
	}

	result, err := sqlc.New(pool).FinalizeCompactionArtifact(ctx, finalizeParams(
		logID,
		botID,
		sessionID,
		messageIDs,
		versions,
		"summary",
	))
	if err != nil {
		t.Fatalf("finalize mixed stale batch: %v", err)
	}
	if result.Finalized || result.MatchedCount != 1 || result.ClaimedCount != 0 {
		t.Fatalf("mixed stale result = %+v, want stale with one match and zero claims", result)
	}
	assertStatusAndUnclaimed(t, ctx, pool, logID, messageIDs, "error")
}

func TestFinalizeCompactionArtifactPriorStatusRacePostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	botID, sessionID := testUUID(), testUUID()
	messageID, priorLogID, nextLogID := testUUID(), testUUID(), testUUID()
	insertFinalizeMessages(t, ctx, pool, botID, sessionID, []pgtype.UUID{messageID})
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{priorLogID, nextLogID})
	if _, err := pool.Exec(ctx, `UPDATE bot_history_messages SET compact_id = $1 WHERE id = $2`, priorLogID, messageID); err != nil {
		t.Fatalf("attach pending prior attempt: %v", err)
	}
	versions := readMessageVersions(t, ctx, pool, []pgtype.UUID{messageID})

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin prior finalization: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `UPDATE bot_history_message_compacts SET status = 'ok' WHERE id = $1`, priorLogID); err != nil {
		t.Fatalf("stage prior finalization: %v", err)
	}

	type outcome struct {
		result sqlc.FinalizeCompactionArtifactRow
		err    error
	}
	application := "finalize-prior-race-" + nextLogID.String()
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire prior-race connection: %v", err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, `SELECT set_config('application_name', $1, false)`, application); err != nil {
		t.Fatalf("name prior-race connection: %v", err)
	}
	finished := make(chan outcome, 1)
	go func() {
		result, finalizeErr := sqlc.New(connection).FinalizeCompactionArtifact(ctx, finalizeParams(
			nextLogID,
			botID,
			sessionID,
			[]pgtype.UUID{messageID},
			versions,
			"summary",
			[]string{priorLogID.String()},
		))
		finished <- outcome{result: result, err: finalizeErr}
	}()
	waitForApplicationLocks(t, ctx, pool, []string{application})
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit prior finalization: %v", err)
	}
	got := <-finished
	if got.err != nil {
		t.Fatalf("finalize after prior status transition: %v", got.err)
	}
	if got.result.Finalized || got.result.ClaimedCount != 0 {
		t.Fatalf("finalized over newly successful prior artifact: %+v", got.result)
	}
	assertClaimedBy(t, ctx, pool, []pgtype.UUID{messageID}, priorLogID)
	assertLogStatus(t, ctx, pool, nextLogID, "error")
}

func TestFinalizeCompactionArtifactRetiresPriorPendingPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	ctx := context.Background()
	botID, sessionID := testUUID(), testUUID()
	messageID, priorLogID, nextLogID := testUUID(), testUUID(), testUUID()
	insertFinalizeMessages(t, ctx, pool, botID, sessionID, []pgtype.UUID{messageID})
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{priorLogID, nextLogID})
	if _, err := pool.Exec(ctx, `UPDATE bot_history_messages SET compact_id = $1 WHERE id = $2`, priorLogID, messageID); err != nil {
		t.Fatalf("attach pending prior attempt: %v", err)
	}
	versions := readMessageVersions(t, ctx, pool, []pgtype.UUID{messageID})

	result, err := sqlc.New(pool).FinalizeCompactionArtifact(ctx, finalizeParams(
		nextLogID,
		botID,
		sessionID,
		[]pgtype.UUID{messageID},
		versions,
		"summary",
		[]string{priorLogID.String()},
	))
	if err != nil {
		t.Fatalf("reclaim prior pending attempt: %v", err)
	}
	if !result.Finalized {
		t.Fatalf("reclaim result = %+v, want finalized", result)
	}
	assertClaimedBy(t, ctx, pool, []pgtype.UUID{messageID}, nextLogID)
	assertLogStatus(t, ctx, pool, priorLogID, "error")
	assertLogStatus(t, ctx, pool, nextLogID, "ok")

	_, err = sqlc.New(pool).CompleteCompactionLog(ctx, sqlc.CompleteCompactionLogParams{
		ID:       priorLogID,
		Status:   "ok",
		Summary:  "late prior summary",
		Coverage: []byte(`[]`),
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("late prior completion error = %v, want pgx.ErrNoRows", err)
	}
}

func TestFinalizeCompactionArtifactUsesListedSourceVersionPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	ctx := context.Background()
	installListUncompactedFixture(t, ctx, pool)
	botID, sessionID, messageID, logID := testUUID(), testUUID(), testUUID(), testUUID()
	insertFinalizeMessages(t, ctx, pool, botID, sessionID, []pgtype.UUID{messageID})
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{logID})

	rows, err := sqlc.New(pool).ListUncompactedMessagesBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("list uncompacted source snapshot: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != messageID || rows[0].SourceVersion == "" {
		t.Fatalf("listed source snapshot = %#v, want message %s with a row version", rows, messageID)
	}
	result, err := sqlc.New(pool).FinalizeCompactionArtifact(ctx, finalizeParams(
		logID,
		botID,
		sessionID,
		[]pgtype.UUID{messageID},
		[]string{rows[0].SourceVersion},
		"summary",
	))
	if err != nil {
		t.Fatalf("finalize listed source snapshot: %v", err)
	}
	if !result.Finalized {
		t.Fatalf("listed source finalization = %+v, want finalized", result)
	}
}

func assertMessageUnclaimed(t *testing.T, ctx context.Context, pool *pgxpool.Pool, messageID pgtype.UUID) {
	t.Helper()
	var compactID pgtype.UUID
	if err := pool.QueryRow(ctx, `SELECT compact_id FROM bot_history_messages WHERE id = $1`, messageID).Scan(&compactID); err != nil {
		t.Fatalf("read source claim: %v", err)
	}
	if compactID.Valid {
		t.Fatalf("message %s was partially claimed by %s", messageID, compactID)
	}
}

func assertLogStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, logID pgtype.UUID, want string) {
	t.Helper()
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM bot_history_message_compacts WHERE id = $1`, logID).Scan(&status); err != nil {
		t.Fatalf("read compaction log: %v", err)
	}
	if status != want {
		t.Fatalf("log %s status = %q, want %q", logID, status, want)
	}
}

func waitForApplicationLocks(t *testing.T, ctx context.Context, pool *pgxpool.Pool, applications []string) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked int
		if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM pg_stat_activity
WHERE application_name = ANY($1::text[])
  AND wait_event_type = 'Lock'
`, applications).Scan(&blocked); err != nil {
			t.Fatalf("inspect blocked finalizers: %v", err)
		}
		if blocked == len(applications) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for blocked finalizers: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func installListUncompactedFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
ALTER TABLE bot_history_messages
  ADD COLUMN sender_channel_identity_id UUID,
  ADD COLUMN sender_account_user_id UUID,
  ADD COLUMN source_message_id TEXT,
  ADD COLUMN source_reply_to_message_id TEXT,
  ADD COLUMN usage JSONB,
  ADD COLUMN session_mode TEXT NOT NULL DEFAULT 'chat',
  ADD COLUMN runtime_type TEXT NOT NULL DEFAULT 'model',
  ADD COLUMN model_id UUID,
  ADD COLUMN event_id UUID,
  ADD COLUMN display_text TEXT;
CREATE TABLE channel_identities (
  id UUID PRIMARY KEY,
  display_name TEXT,
  avatar_url TEXT
);
CREATE TABLE bot_channel_routes (
  id UUID PRIMARY KEY,
  conversation_type TEXT,
  metadata JSONB NOT NULL DEFAULT '{}',
  default_reply_target TEXT
);
CREATE TABLE bot_sessions (
  id UUID PRIMARY KEY,
  channel_type TEXT,
  route_id UUID
);
CREATE VIEW bot_visible_history_messages AS
SELECT
  turn_id,
  turn_position,
  turn_message_seq,
  id,
  bot_id,
  session_id,
  sender_channel_identity_id,
  sender_account_user_id,
  source_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  compact_id,
  session_mode,
  runtime_type,
  model_id,
  event_id,
  display_text,
  created_at
FROM bot_history_messages
WHERE turn_visible = true
  AND turn_id IS NOT NULL
  AND turn_position IS NOT NULL
  AND turn_message_seq IS NOT NULL;
`); err != nil {
		t.Fatalf("install uncompacted-list fixture: %v", err)
	}
}
