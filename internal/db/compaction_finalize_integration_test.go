package db

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestFinalizeCompactionArtifactPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	ctx := context.Background()

	t.Run("concurrent attempts claim the source exactly once", func(t *testing.T) {
		botID := testUUID()
		sessionID := testUUID()
		messageIDs := []pgtype.UUID{testUUID(), testUUID()}
		logIDs := []pgtype.UUID{testUUID(), testUUID()}
		priorLogID := testUUID()
		insertFinalizeMessages(t, ctx, pool, botID, sessionID, messageIDs)
		insertFinalizeLogs(t, ctx, pool, botID, sessionID, append(logIDs, priorLogID))
		if _, err := pool.Exec(ctx, `UPDATE bot_history_messages SET compact_id = $1 WHERE id = $2`, priorLogID, messageIDs[0]); err != nil {
			t.Fatalf("attach prior failed attempt: %v", err)
		}
		if _, err := pool.Exec(ctx, `UPDATE bot_history_message_compacts SET status = 'error' WHERE id = $1`, priorLogID); err != nil {
			t.Fatalf("terminalize prior attempt: %v", err)
		}
		versions := readMessageVersions(t, ctx, pool, messageIDs)
		expectedCompactIDs := []string{priorLogID.String(), ""}

		type outcome struct {
			logID     pgtype.UUID
			finalized bool
			err       error
		}
		start := make(chan struct{})
		outcomes := make(chan outcome, len(logIDs))
		for _, logID := range logIDs {
			go func() {
				<-start
				result, err := sqlc.New(pool).FinalizeCompactionArtifact(ctx, finalizeParams(
					logID,
					botID,
					sessionID,
					messageIDs,
					versions,
					"summary",
					expectedCompactIDs,
				))
				outcomes <- outcome{logID: logID, finalized: result.Finalized, err: err}
			}()
		}
		close(start)

		var winner pgtype.UUID
		for range logIDs {
			got := <-outcomes
			if got.err != nil {
				t.Fatalf("finalize %s: %v", got.logID, got.err)
			}
			if got.finalized {
				if winner.Valid {
					t.Fatalf("both compaction attempts finalized: %s and %s", winner, got.logID)
				}
				winner = got.logID
			}
		}
		if !winner.Valid {
			t.Fatal("neither concurrent compaction attempt finalized")
		}
		assertClaimedBy(t, ctx, pool, messageIDs, winner)
		assertSingleSuccessfulLog(t, ctx, pool, logIDs, winner)
	})

	t.Run("changed source version is stale", func(t *testing.T) {
		botID, sessionID, messageID, logID := testUUID(), testUUID(), testUUID(), testUUID()
		insertFinalizeMessages(t, ctx, pool, botID, sessionID, []pgtype.UUID{messageID})
		insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{logID})
		versions := readMessageVersions(t, ctx, pool, []pgtype.UUID{messageID})
		if _, err := pool.Exec(ctx, `UPDATE bot_history_messages SET content = '{"changed":true}' WHERE id = $1`, messageID); err != nil {
			t.Fatalf("mutate source message: %v", err)
		}

		result, err := sqlc.New(pool).FinalizeCompactionArtifact(ctx, finalizeParams(
			logID,
			botID,
			sessionID,
			[]pgtype.UUID{messageID},
			versions,
			"summary",
		))
		if err != nil {
			t.Fatalf("finalize stale source: %v", err)
		}
		if result.Finalized {
			t.Fatal("finalized a summary generated from a stale source version")
		}
		assertStatusAndUnclaimed(t, ctx, pool, logID, []pgtype.UUID{messageID}, "error")
	})

	t.Run("currently hidden source is stale", func(t *testing.T) {
		botID, sessionID, messageID, logID := testUUID(), testUUID(), testUUID(), testUUID()
		insertFinalizeMessages(t, ctx, pool, botID, sessionID, []pgtype.UUID{messageID})
		insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{logID})
		if _, err := pool.Exec(ctx, `UPDATE bot_history_messages SET turn_visible = false WHERE id = $1`, messageID); err != nil {
			t.Fatalf("hide source message: %v", err)
		}
		versions := readMessageVersions(t, ctx, pool, []pgtype.UUID{messageID})

		result, err := sqlc.New(pool).FinalizeCompactionArtifact(ctx, finalizeParams(
			logID,
			botID,
			sessionID,
			[]pgtype.UUID{messageID},
			versions,
			"summary",
		))
		if err != nil {
			t.Fatalf("finalize hidden source: %v", err)
		}
		if result.Finalized {
			t.Fatal("finalized a summary for a source outside visible history")
		}
		assertStatusAndUnclaimed(t, ctx, pool, logID, []pgtype.UUID{messageID}, "error")
	})

	t.Run("invalid request shape cannot partially claim", func(t *testing.T) {
		botID, sessionID, messageID, logID := testUUID(), testUUID(), testUUID(), testUUID()
		insertFinalizeMessages(t, ctx, pool, botID, sessionID, []pgtype.UUID{messageID})
		insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{logID})
		version := readMessageVersions(t, ctx, pool, []pgtype.UUID{messageID})[0]

		result, err := sqlc.New(pool).FinalizeCompactionArtifact(ctx, finalizeParams(
			logID,
			botID,
			sessionID,
			[]pgtype.UUID{messageID, messageID},
			[]string{version, version},
			"summary",
		))
		if err != nil {
			t.Fatalf("finalize duplicate source request: %v", err)
		}
		if result.Finalized {
			t.Fatal("finalized a request containing duplicate source ids")
		}
		assertStatusAndUnclaimed(t, ctx, pool, logID, []pgtype.UUID{messageID}, "error")
	})

	t.Run("statement failure rolls back source claims", func(t *testing.T) {
		botID, sessionID, messageID, logID := testUUID(), testUUID(), testUUID(), testUUID()
		insertFinalizeMessages(t, ctx, pool, botID, sessionID, []pgtype.UUID{messageID})
		insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{logID})
		versions := readMessageVersions(t, ctx, pool, []pgtype.UUID{messageID})

		_, err := sqlc.New(pool).FinalizeCompactionArtifact(ctx, finalizeParams(
			logID,
			botID,
			sessionID,
			[]pgtype.UUID{messageID},
			versions,
			"reject",
		))
		if err == nil {
			t.Fatal("finalize unexpectedly bypassed the test log constraint")
		}
		assertStatusAndUnclaimed(t, ctx, pool, logID, []pgtype.UUID{messageID}, "pending")
	})

	t.Run("malformed coverage cannot become active", func(t *testing.T) {
		botID, sessionID, messageID, logID := testUUID(), testUUID(), testUUID(), testUUID()
		insertFinalizeMessages(t, ctx, pool, botID, sessionID, []pgtype.UUID{messageID})
		insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{logID})
		versions := readMessageVersions(t, ctx, pool, []pgtype.UUID{messageID})
		params := finalizeParams(logID, botID, sessionID, []pgtype.UUID{messageID}, versions, "summary")
		var coverage []map[string]any
		if err := json.Unmarshal(params.Coverage, &coverage); err != nil {
			t.Fatalf("decode test coverage: %v", err)
		}
		coverage[0]["ref"].(map[string]any)["version"] = "1"
		coverage[0]["created_at_ms"] = "1"
		coverage[0]["external_message_id"] = 1
		params.Coverage, _ = json.Marshal(coverage)

		result, err := sqlc.New(pool).FinalizeCompactionArtifact(ctx, params)
		if err != nil {
			t.Fatalf("reject malformed coverage: %v", err)
		}
		if result.Finalized || result.ClaimedCount != 0 {
			t.Fatalf("malformed coverage finalized: %+v", result)
		}
		assertStatusAndUnclaimed(t, ctx, pool, logID, []pgtype.UUID{messageID}, "error")
	})

	t.Run("descending coverage cannot become active", func(t *testing.T) {
		botID, sessionID, logID := testUUID(), testUUID(), testUUID()
		messageIDs := []pgtype.UUID{testUUID(), testUUID()}
		insertFinalizeMessages(t, ctx, pool, botID, sessionID, messageIDs)
		insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{logID})
		if _, err := pool.Exec(ctx, `UPDATE bot_history_messages SET created_at = to_timestamp(0.002) WHERE id = $1`, messageIDs[0]); err != nil {
			t.Fatalf("move first source after second: %v", err)
		}
		versions := readMessageVersions(t, ctx, pool, messageIDs)
		params := finalizeParams(logID, botID, sessionID, messageIDs, versions, "summary")
		var coverage []map[string]any
		if err := json.Unmarshal(params.Coverage, &coverage); err != nil {
			t.Fatalf("decode test coverage: %v", err)
		}
		coverage[0]["created_at_ms"] = int64(2)
		params.Coverage, _ = json.Marshal(coverage)
		params.AnchorStartMs = 2
		params.AnchorEndMs = 1

		result, err := sqlc.New(pool).FinalizeCompactionArtifact(ctx, params)
		if err != nil {
			t.Fatalf("reject descending coverage: %v", err)
		}
		if result.Finalized || result.MatchedCount != 0 || result.ClaimedCount != 0 {
			t.Fatalf("descending coverage finalized: %+v", result)
		}
		assertStatusAndUnclaimed(t, ctx, pool, logID, messageIDs, "error")
	})

	t.Run("terminal log cannot be overwritten", func(t *testing.T) {
		botID, sessionID, logID := testUUID(), testUUID(), testUUID()
		insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{logID})
		queries := sqlc.New(pool)
		params := sqlc.CompleteCompactionLogParams{
			ID:           logID,
			Status:       "error",
			ErrorMessage: "first",
			Coverage:     []byte(`[]`),
		}
		if _, err := queries.CompleteCompactionLog(ctx, params); err != nil {
			t.Fatalf("complete pending log: %v", err)
		}
		params.Status = "ok"
		params.Summary = "late"
		if _, err := queries.CompleteCompactionLog(ctx, params); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("overwrite terminal log error = %v, want pgx.ErrNoRows", err)
		}
		var status, summary string
		if err := pool.QueryRow(ctx, `SELECT status, summary FROM bot_history_message_compacts WHERE id = $1`, logID).Scan(&status, &summary); err != nil {
			t.Fatalf("read terminal log: %v", err)
		}
		if status != "error" || summary != "" {
			t.Fatalf("terminal log = status %q summary %q, want error and empty summary", status, summary)
		}
	})
}

func TestFinalizeCompactionArtifactMarkLockOrderPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	botID, sessionID := testUUID(), testUUID()
	messageIDs := []pgtype.UUID{testUUID(), testUUID()}
	if messageIDs[0].String() < messageIDs[1].String() {
		messageIDs[0], messageIDs[1] = messageIDs[1], messageIDs[0]
	}
	markLogID, completeLogID := testUUID(), testUUID()
	insertFinalizeMessages(t, ctx, pool, botID, sessionID, messageIDs)
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{markLogID, completeLogID})
	queries := sqlc.New(pool)
	if err := queries.MarkMessagesCompacted(ctx, sqlc.MarkMessagesCompactedParams{
		CompactID: completeLogID,
		Column2:   messageIDs,
	}); err != nil {
		t.Fatalf("attach initial claims: %v", err)
	}

	if _, err := pool.Exec(ctx, `
CREATE TABLE compaction_mark_pause (
  application_name TEXT PRIMARY KEY,
  message_id UUID NOT NULL,
  lock_key BIGINT NOT NULL
);
CREATE FUNCTION pause_compaction_mark()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  pause_key BIGINT;
BEGIN
  SELECT pause.lock_key
  INTO pause_key
  FROM compaction_mark_pause pause
  WHERE pause.application_name = current_setting('application_name')
    AND pause.message_id = OLD.id;

  IF FOUND AND NEW.compact_id IS DISTINCT FROM OLD.compact_id THEN
    PERFORM pg_advisory_xact_lock(pause_key);
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER zz_pause_compaction_mark
BEFORE UPDATE OF compact_id ON bot_history_messages
FOR EACH ROW
EXECUTE FUNCTION pause_compaction_mark();
`); err != nil {
		t.Fatalf("install mark pause trigger: %v", err)
	}

	markApplication := "mark-lock-order-" + markLogID.String()
	completeApplication := "complete-lock-order-" + completeLogID.String()
	lockKey := int64(982451653)
	if _, err := pool.Exec(ctx, `INSERT INTO compaction_mark_pause (application_name, message_id, lock_key) VALUES ($1, $2, $3)`, markApplication, messageIDs[0], lockKey); err != nil {
		t.Fatalf("configure mark pause: %v", err)
	}
	control, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin advisory lock control: %v", err)
	}
	defer func() { _ = control.Rollback(context.Background()) }()
	if _, err := control.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
		t.Fatalf("hold mark pause lock: %v", err)
	}

	markConnection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire mark connection: %v", err)
	}
	defer markConnection.Release()
	completeConnection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire complete connection: %v", err)
	}
	defer completeConnection.Release()
	for connection, application := range map[*pgxpool.Conn]string{
		markConnection:     markApplication,
		completeConnection: completeApplication,
	} {
		if _, err := connection.Exec(ctx, `SELECT set_config('application_name', $1, false)`, application); err != nil {
			t.Fatalf("name lock-order connection: %v", err)
		}
	}

	markDone := make(chan error, 1)
	go func() {
		markDone <- sqlc.New(markConnection).MarkMessagesCompacted(ctx, sqlc.MarkMessagesCompactedParams{
			CompactID: markLogID,
			Column2:   messageIDs,
		})
	}()
	waitForApplicationLocks(t, ctx, pool, []string{markApplication})
	completeDone := make(chan error, 1)
	go func() {
		_, completeErr := sqlc.New(completeConnection).CompleteCompactionLog(ctx, sqlc.CompleteCompactionLogParams{
			ID:           completeLogID,
			Status:       "ok",
			Summary:      "losing summary",
			MessageCount: 2,
			Coverage:     []byte(`[]`),
		})
		completeDone <- completeErr
	}()
	waitForApplicationLocks(t, ctx, pool, []string{markApplication, completeApplication})
	if err := control.Commit(ctx); err != nil {
		t.Fatalf("release mark pause lock: %v", err)
	}
	if err := <-markDone; err != nil {
		t.Fatalf("ordered mark failed: %v", err)
	}
	completeErr := <-completeDone
	var pgErr *pgconn.PgError
	if !errors.As(completeErr, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("losing completion error = %v, want claim constraint violation", completeErr)
	}
	for _, messageID := range messageIDs {
		var owner pgtype.UUID
		var finalized bool
		if err := pool.QueryRow(ctx, `SELECT compact_id, compact_claim_finalized FROM bot_history_messages WHERE id = $1`, messageID).Scan(&owner, &finalized); err != nil {
			t.Fatalf("read ordered mark result: %v", err)
		}
		if owner != markLogID || finalized {
			t.Fatalf("message %s claim = (%s, %v), want pending owner %s", messageID, owner, finalized, markLogID)
		}
	}
}
