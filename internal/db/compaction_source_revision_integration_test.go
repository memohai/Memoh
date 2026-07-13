package db

import (
	"context"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestFinalizeCompactionArtifactSourceRevisionPostgresPath(t *testing.T) {
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
)
`); err != nil {
		t.Fatalf("create message asset fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, readEmbeddedMigration(t, "postgres/migrations/0110_compaction_source_revision.up.sql")); err != nil {
		t.Fatalf("apply source revision migration: %v", err)
	}

	botID, sessionID, messageID, logID := testUUID(), testUUID(), testUUID(), testUUID()
	insertFinalizeMessages(t, ctx, pool, botID, sessionID, []pgtype.UUID{messageID})
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{logID})
	queries := sqlc.New(pool)

	listed := listSingleCompactionSource(t, ctx, queries, sessionID, messageID)
	assertListedSourceRevision(t, ctx, pool, listed.SourceVersion, messageID)

	asset := sqlc.CreateMessageAssetParams{
		MessageID:   messageID,
		Role:        "attachment",
		Ordinal:     0,
		ContentHash: "asset-1",
		Name:        "one.png",
		Metadata:    []byte(`{"width":100}`),
	}
	var beforeAssetXmin string
	if err := pool.QueryRow(ctx, `SELECT xmin::text FROM bot_history_messages WHERE id = $1`, messageID).Scan(&beforeAssetXmin); err != nil {
		t.Fatalf("read pre-asset xmin: %v", err)
	}
	if _, err := queries.CreateMessageAsset(ctx, asset); err != nil {
		t.Fatalf("create message asset: %v", err)
	}
	afterInsert := listSingleCompactionSource(t, ctx, queries, sessionID, messageID)
	if afterInsert.SourceVersion == listed.SourceVersion {
		t.Fatal("asset insert did not advance source revision")
	}
	var afterAssetXmin string
	if err := pool.QueryRow(ctx, `SELECT xmin::text FROM bot_history_messages WHERE id = $1`, messageID).Scan(&afterAssetXmin); err != nil {
		t.Fatalf("read post-asset xmin: %v", err)
	}
	if afterAssetXmin == beforeAssetXmin {
		t.Fatal("asset insert did not advance parent xmin for mixed-version readers")
	}
	result, err := queries.FinalizeCompactionArtifact(ctx, finalizeParams(
		logID,
		botID,
		sessionID,
		[]pgtype.UUID{messageID},
		[]string{listed.SourceVersion},
		"stale summary",
	))
	if err != nil {
		t.Fatalf("finalize stale asset snapshot: %v", err)
	}
	if result.Finalized || result.MatchedCount != 0 {
		t.Fatalf("stale asset finalization = %+v, want source conflict", result)
	}

	if _, err := queries.CreateMessageAsset(ctx, asset); err != nil {
		t.Fatalf("repeat identical message asset: %v", err)
	}
	afterNoop := listSingleCompactionSource(t, ctx, queries, sessionID, messageID)
	if afterNoop.SourceVersion != afterInsert.SourceVersion {
		t.Fatal("identical asset upsert advanced source revision")
	}
	asset.Metadata = []byte(`{"width":200}`)
	if _, err := queries.CreateMessageAsset(ctx, asset); err != nil {
		t.Fatalf("update message asset: %v", err)
	}
	afterUpdate := listSingleCompactionSource(t, ctx, queries, sessionID, messageID)
	if afterUpdate.SourceVersion == afterNoop.SourceVersion {
		t.Fatal("asset update did not advance source revision")
	}
	if err := queries.DeleteMessageAssets(ctx, messageID); err != nil {
		t.Fatalf("delete message assets: %v", err)
	}
	afterDelete := listSingleCompactionSource(t, ctx, queries, sessionID, messageID)
	if afterDelete.SourceVersion == afterUpdate.SourceVersion {
		t.Fatal("asset delete did not advance source revision")
	}

	beforeRollback := afterDelete.SourceVersion
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin asset rollback: %v", err)
	}
	rollbackAsset := asset
	rollbackAsset.ContentHash = "asset-rollback"
	if _, err := sqlc.New(tx).CreateMessageAsset(ctx, rollbackAsset); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("insert rollback asset: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback asset mutation: %v", err)
	}
	afterRollback := listSingleCompactionSource(t, ctx, queries, sessionID, messageID)
	if afterRollback.SourceVersion != beforeRollback {
		t.Fatal("rolled-back asset mutation advanced source revision")
	}

	if _, err := pool.Exec(ctx, `UPDATE bot_history_messages SET content = '{"changed":true}' WHERE id = $1`, messageID); err != nil {
		t.Fatalf("mutate message source: %v", err)
	}
	afterMessageUpdate := listSingleCompactionSource(t, ctx, queries, sessionID, messageID)
	if afterMessageUpdate.SourceVersion == afterRollback.SourceVersion {
		t.Fatal("message source update did not advance source revision")
	}

	claimLogID := testUUID()
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{claimLogID})
	if err := queries.MarkMessagesCompacted(ctx, sqlc.MarkMessagesCompactedParams{
		CompactID: claimLogID,
		Column2:   []pgtype.UUID{messageID},
	}); err != nil {
		t.Fatalf("mark message compacted: %v", err)
	}
	afterClaim := listSingleCompactionSource(t, ctx, queries, sessionID, messageID)
	if afterClaim.SourceVersion != afterMessageUpdate.SourceVersion {
		t.Fatal("compaction ownership marker changed source revision")
	}

	freshLogID := testUUID()
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{freshLogID})
	freshResult, err := queries.FinalizeCompactionArtifact(ctx, finalizeParams(
		freshLogID,
		botID,
		sessionID,
		[]pgtype.UUID{messageID},
		[]string{afterClaim.SourceVersion},
		"fresh summary",
		[]string{claimLogID.String()},
	))
	if err != nil {
		t.Fatalf("finalize fresh source revision: %v", err)
	}
	if !freshResult.Finalized || freshResult.MatchedCount != 1 || freshResult.ClaimedCount != 1 {
		t.Fatalf("fresh source finalization = %+v, want successful claim", freshResult)
	}

	cascadeMessageID := testUUID()
	insertFinalizeMessages(t, ctx, pool, botID, sessionID, []pgtype.UUID{cascadeMessageID})
	cascadeAsset := asset
	cascadeAsset.MessageID = cascadeMessageID
	cascadeAsset.ContentHash = "asset-cascade"
	if _, err := queries.CreateMessageAsset(ctx, cascadeAsset); err != nil {
		t.Fatalf("create cascading message asset: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM bot_history_messages WHERE id = $1`, cascadeMessageID); err != nil {
		t.Fatalf("delete message with cascading asset: %v", err)
	}
}

func TestFinalizeCompactionArtifactLegacyAssetSourceRevisionPostgresPath(t *testing.T) {
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
  asset_id UUID NOT NULL,
  role TEXT NOT NULL DEFAULT 'attachment',
  ordinal INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (message_id, asset_id)
)
`); err != nil {
		t.Fatalf("create legacy asset fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, readEmbeddedMigration(t, "postgres/migrations/0110_compaction_source_revision.up.sql")); err != nil {
		t.Fatalf("apply source revision migration: %v", err)
	}

	botID, sessionID, messageID := testUUID(), testUUID(), testUUID()
	insertFinalizeMessages(t, ctx, pool, botID, sessionID, []pgtype.UUID{messageID})
	var before int64
	if err := pool.QueryRow(ctx, `SELECT source_revision FROM bot_history_messages WHERE id = $1`, messageID).Scan(&before); err != nil {
		t.Fatalf("read legacy pre-asset revision: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO bot_history_message_assets (message_id, asset_id, role, ordinal)
VALUES ($1, $2, 'attachment', 0)
`, messageID, testUUID()); err != nil {
		t.Fatalf("insert legacy message asset: %v", err)
	}
	var after int64
	if err := pool.QueryRow(ctx, `SELECT source_revision FROM bot_history_messages WHERE id = $1`, messageID).Scan(&after); err != nil {
		t.Fatalf("read legacy post-asset revision: %v", err)
	}
	if after != before+1 {
		t.Fatalf("legacy asset revision = %d, want %d", after, before+1)
	}
	if _, err := pool.Exec(ctx, readEmbeddedMigration(t, "postgres/migrations/0110_compaction_source_revision.down.sql")); err != nil {
		t.Fatalf("rollback source revision migration: %v", err)
	}
	var revisionColumnExists bool
	if err := pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM information_schema.columns
  WHERE table_schema = current_schema()
    AND table_name = 'bot_history_messages'
    AND column_name = 'source_revision'
)
`).Scan(&revisionColumnExists); err != nil {
		t.Fatalf("inspect rolled-back source revision: %v", err)
	}
	if revisionColumnExists {
		t.Fatal("source revision column survived down migration")
	}
}

func TestCompactionClaimInvalidationPostgresPath(t *testing.T) {
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
);
CREATE OR REPLACE FUNCTION resolve_history_message_source_context(bot_history_messages)
RETURNS JSONB
LANGUAGE sql
STABLE
AS $$
  SELECT '{"version":1,"sender_display_name":"","platform":"","conversation_type":"","conversation_name":""}'::jsonb
$$
`); err != nil {
		t.Fatalf("create claim invalidation fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, readEmbeddedMigration(t, "postgres/migrations/0110_compaction_source_revision.up.sql")); err != nil {
		t.Fatalf("apply source revision migration: %v", err)
	}

	botID, sessionID := testUUID(), testUUID()
	queries := sqlc.New(pool)
	legacyMessageID, legacyLogID := testUUID(), testUUID()
	insertFinalizeMessages(t, ctx, pool, botID, sessionID, []pgtype.UUID{legacyMessageID})
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{legacyLogID})
	legacySource := listSingleCompactionSource(t, ctx, queries, sessionID, legacyMessageID)
	legacyResult, err := queries.FinalizeCompactionArtifact(ctx, finalizeParams(
		legacyLogID,
		botID,
		sessionID,
		[]pgtype.UUID{legacyMessageID},
		[]string{legacySource.SourceVersion},
		"pre-migration summary",
	))
	if err != nil || !legacyResult.Finalized {
		t.Fatalf("finalize pre-migration artifact = result:%+v error:%v", legacyResult, err)
	}
	var legacySourceContext []byte
	if err := pool.QueryRow(ctx, `SELECT source_context FROM bot_history_messages WHERE id = $1`, legacyMessageID).Scan(&legacySourceContext); err != nil {
		t.Fatalf("read pre-migration source context: %v", err)
	}
	if legacySourceContext != nil {
		t.Fatalf("pre-migration compacted source context = %s, want NULL", legacySourceContext)
	}
	if _, err := pool.Exec(ctx, `
CREATE OR REPLACE FUNCTION capture_history_message_source_context()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  IF NEW.source_context IS NULL
     AND (
       TG_OP = 'INSERT'
       OR (OLD.compact_id IS NOT NULL AND NEW.compact_id IS NULL)
     ) THEN
    NEW.source_context := resolve_history_message_source_context(NEW);
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER history_message_source_context_capture
BEFORE INSERT OR UPDATE OF compact_id, source_context ON bot_history_messages
FOR EACH ROW
EXECUTE FUNCTION capture_history_message_source_context()
`); err != nil {
		t.Fatalf("activate source context capture: %v", err)
	}

	if _, err := pool.Exec(ctx, readEmbeddedMigration(t, "postgres/migrations/0113_compaction_claim_invalidation.up.sql")); err != nil {
		t.Fatalf("apply claim invalidation migration: %v", err)
	}
	assertCompactionClaimCurrent(t, ctx, pool, legacyLogID, true)
	if _, err := pool.Exec(ctx, `UPDATE bot_history_messages SET content = '{"legacy_edited":true}' WHERE id = $1`, legacyMessageID); err != nil {
		t.Fatalf("mutate pre-migration compacted source: %v", err)
	}
	assertCompactionClaimCurrent(t, ctx, pool, legacyLogID, false)

	mutations := []struct {
		name   string
		mutate func(pgtype.UUID) error
	}{
		{
			name: "content edit",
			mutate: func(messageID pgtype.UUID) error {
				_, err := pool.Exec(ctx, `UPDATE bot_history_messages SET content = '{"edited":true}' WHERE id = $1`, messageID)
				return err
			},
		},
		{
			name: "hide",
			mutate: func(messageID pgtype.UUID) error {
				_, err := pool.Exec(ctx, `UPDATE bot_history_messages SET turn_visible = false WHERE id = $1`, messageID)
				return err
			},
		},
		{
			name: "asset insert",
			mutate: func(messageID pgtype.UUID) error {
				_, err := pool.Exec(ctx, `
INSERT INTO bot_history_message_assets (message_id, content_hash)
VALUES ($1, 'post-compact-asset')
`, messageID)
				return err
			},
		},
		{
			name: "delete",
			mutate: func(messageID pgtype.UUID) error {
				_, err := pool.Exec(ctx, `DELETE FROM bot_history_messages WHERE id = $1`, messageID)
				return err
			},
		},
	}
	invalidLogIDs := []pgtype.UUID{legacyLogID}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutationSessionID := testUUID()
			messageID, logID := testUUID(), testUUID()
			invalidLogIDs = append(invalidLogIDs, logID)
			insertFinalizeMessages(t, ctx, pool, botID, mutationSessionID, []pgtype.UUID{messageID})
			insertFinalizeLogs(t, ctx, pool, botID, mutationSessionID, []pgtype.UUID{logID})
			source := listSingleCompactionSource(t, ctx, queries, mutationSessionID, messageID)
			result, err := queries.FinalizeCompactionArtifact(ctx, finalizeParams(
				logID,
				botID,
				mutationSessionID,
				[]pgtype.UUID{messageID},
				[]string{source.SourceVersion},
				mutation.name+" summary",
			))
			if err != nil || !result.Finalized {
				t.Fatalf("finalize artifact = result:%+v error:%v", result, err)
			}
			assertCompactionClaimCurrent(t, ctx, pool, logID, true)

			if err := mutation.mutate(messageID); err != nil {
				t.Fatalf("mutate compacted source: %v", err)
			}
			assertCompactionClaimCurrent(t, ctx, pool, logID, false)
		})
	}
	preservedSessionID := testUUID()
	preservedMessageID, preservedLogID := testUUID(), testUUID()
	insertFinalizeMessages(t, ctx, pool, botID, preservedSessionID, []pgtype.UUID{preservedMessageID})
	insertFinalizeLogs(t, ctx, pool, botID, preservedSessionID, []pgtype.UUID{preservedLogID})
	preservedSource := listSingleCompactionSource(t, ctx, queries, preservedSessionID, preservedMessageID)
	preservedResult, err := queries.FinalizeCompactionArtifact(ctx, finalizeParams(
		preservedLogID,
		botID,
		preservedSessionID,
		[]pgtype.UUID{preservedMessageID},
		[]string{preservedSource.SourceVersion},
		"preserved summary",
	))
	if err != nil || !preservedResult.Finalized {
		t.Fatalf("finalize preserved artifact = result:%+v error:%v", preservedResult, err)
	}

	if _, err := pool.Exec(ctx, readEmbeddedMigration(t, "postgres/migrations/0113_compaction_claim_invalidation.down.sql")); err != nil {
		t.Fatalf("roll back claim invalidation migration: %v", err)
	}
	var invalidLogsRemaining int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM bot_history_message_compacts
WHERE id = ANY($1::uuid[])
`, invalidLogIDs).Scan(&invalidLogsRemaining); err != nil {
		t.Fatalf("count retired invalid artifacts: %v", err)
	}
	if invalidLogsRemaining != 0 {
		t.Fatalf("invalid artifacts remaining after rollback = %d, want 0", invalidLogsRemaining)
	}
	var validLogsRemaining int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bot_history_message_compacts WHERE id = $1`, preservedLogID).Scan(&validLogsRemaining); err != nil {
		t.Fatalf("count preserved valid artifact: %v", err)
	}
	if validLogsRemaining != 1 {
		t.Fatalf("valid artifacts remaining after rollback = %d, want 1", validLogsRemaining)
	}
}

func assertCompactionClaimCurrent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, compactID pgtype.UUID, want bool) {
	t.Helper()
	var current bool
	if err := pool.QueryRow(ctx, `
SELECT sources_current
FROM bot_history_message_compact_claim_validity
WHERE compact_id = $1
`, compactID).Scan(&current); err != nil {
		t.Fatalf("read compaction claim validity: %v", err)
	}
	if current != want {
		t.Fatalf("compaction claim current = %v, want %v", current, want)
	}
}

func listSingleCompactionSource(
	t *testing.T,
	ctx context.Context,
	queries *sqlc.Queries,
	sessionID pgtype.UUID,
	messageID pgtype.UUID,
) sqlc.ListUncompactedMessagesBySessionRow {
	t.Helper()
	rows, err := queries.ListUncompactedMessagesBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("list uncompacted sources: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != messageID {
		t.Fatalf("listed sources = %#v, want %s", rows, messageID)
	}
	return rows[0]
}

func assertListedSourceRevision(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	listed string,
	messageID pgtype.UUID,
) {
	t.Helper()
	var revision int64
	if err := pool.QueryRow(ctx, `SELECT source_revision FROM bot_history_messages WHERE id = $1`, messageID).Scan(&revision); err != nil {
		t.Fatalf("read source revision: %v", err)
	}
	if listed != strconv.FormatInt(revision, 10) {
		t.Fatalf("listed source version = %q, want revision %d", listed, revision)
	}
}
