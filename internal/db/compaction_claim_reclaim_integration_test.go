package db

import (
	"context"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestInvalidCompactionClaimCanBeReclaimedPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	ctx := context.Background()
	installListUncompactedFixture(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
ALTER TABLE bot_history_messages
  ADD COLUMN turn_superseded_by_turn_id UUID,
  ADD COLUMN turn_superseded_at TIMESTAMPTZ,
  ADD COLUMN turn_superseded_reason TEXT;
CREATE TABLE bot_history_message_assets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  message_id UUID NOT NULL REFERENCES bot_history_messages(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'attachment',
  ordinal INTEGER NOT NULL DEFAULT 0,
  content_hash TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (message_id, content_hash)
);
CREATE TABLE bot_history_message_compact_parent_edges (
  artifact_id UUID NOT NULL REFERENCES bot_history_message_compacts(id) ON DELETE CASCADE,
  parent_id UUID NOT NULL REFERENCES bot_history_message_compacts(id),
  ordinal INTEGER NOT NULL,
  PRIMARY KEY (artifact_id, parent_id)
)
`); err != nil {
		t.Fatalf("create claim reclaim fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, readEmbeddedMigration(t, "postgres/migrations/0109_compaction_source_revision.up.sql")); err != nil {
		t.Fatalf("apply source revision migration: %v", err)
	}
	if _, err := pool.Exec(ctx, readEmbeddedMigration(t, "postgres/migrations/0112_compaction_claim_invalidation.up.sql")); err != nil {
		t.Fatalf("apply claim invalidation migration: %v", err)
	}

	botID, sessionID := testUUID(), testUUID()
	messageID, oldLogID := testUUID(), testUUID()
	insertFinalizeMessages(t, ctx, pool, botID, sessionID, []pgtype.UUID{messageID})
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{oldLogID})
	queries := sqlc.New(pool)
	original := listSingleCompactionSource(t, ctx, queries, sessionID, messageID)
	result, err := queries.FinalizeCompactionArtifact(ctx, finalizeParams(
		oldLogID,
		botID,
		sessionID,
		[]pgtype.UUID{messageID},
		[]string{original.SourceVersion},
		"old summary",
	))
	if err != nil || !result.Finalized {
		t.Fatalf("finalize original artifact = result:%+v error:%v", result, err)
	}
	assertMessageSourceContextNull(t, ctx, pool, messageID)

	if _, err := pool.Exec(ctx, `UPDATE bot_history_messages SET content = '{"edited":true}' WHERE id = $1`, messageID); err != nil {
		t.Fatalf("edit compacted source: %v", err)
	}
	assertCompactionClaimCurrent(t, ctx, pool, oldLogID, false)

	stale := listSingleCompactionSource(t, ctx, queries, sessionID, messageID)
	if stale.CompactID != oldLogID {
		t.Fatalf("reclaim candidate owner = %s, want %s", stale.CompactID, oldLogID)
	}
	revisionBefore, err := strconv.ParseInt(stale.SourceVersion, 10, 64)
	if err != nil {
		t.Fatalf("parse reclaim source version %q: %v", stale.SourceVersion, err)
	}

	newLogID := testUUID()
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{newLogID})
	replacement, err := queries.FinalizeCompactionArtifact(ctx, finalizeParams(
		newLogID,
		botID,
		sessionID,
		[]pgtype.UUID{messageID},
		[]string{stale.SourceVersion},
		"replacement summary",
		[]string{oldLogID.String()},
	))
	if err != nil || !replacement.Finalized {
		t.Fatalf("finalize replacement artifact = result:%+v error:%v", replacement, err)
	}
	assertCompactionClaimCurrent(t, ctx, pool, newLogID, true)
	assertMessageSourceContextNull(t, ctx, pool, messageID)
	if revisionAfter := readMessageSourceRevision(t, ctx, pool, messageID); revisionAfter != revisionBefore {
		t.Fatalf("reclaim source revision = %d, want unchanged %d", revisionAfter, revisionBefore)
	}
	rows, err := queries.ListUncompactedMessagesBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("list after replacement: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("replacement left uncompacted rows: %#v", rows)
	}

	if _, err := pool.Exec(ctx, `UPDATE bot_history_messages SET turn_visible = false WHERE id = $1`, messageID); err != nil {
		t.Fatalf("hide replacement source: %v", err)
	}
	assertCompactionClaimCurrent(t, ctx, pool, newLogID, false)
	rows, err = queries.ListUncompactedMessagesBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("list after hide: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("hidden invalid source reappeared as raw history: %#v", rows)
	}
}

func assertMessageSourceContextNull(t *testing.T, ctx context.Context, pool *pgxpool.Pool, messageID pgtype.UUID) {
	t.Helper()
	var sourceContext []byte
	if err := pool.QueryRow(ctx, `SELECT source_context FROM bot_history_messages WHERE id = $1`, messageID).Scan(&sourceContext); err != nil {
		t.Fatalf("read message source context: %v", err)
	}
	if sourceContext != nil {
		t.Fatalf("message source context = %s, want NULL", sourceContext)
	}
}
