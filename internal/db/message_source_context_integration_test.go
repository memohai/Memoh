package db

import (
	"context"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/messagesource"
)

func TestFinalizeCompactionArtifactSourceContextActivationPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	ctx := context.Background()
	installListUncompactedFixture(t, ctx, pool)
	installSourceContextFixture(t, ctx, pool)
	applySourceContextMigration(t, ctx, pool, "0109_compaction_source_revision.up.sql")
	applySourceContextMigration(t, ctx, pool, "0110_message_source_context.up.sql")
	botID, sessionID, routeID := testUUID(), testUUID(), testUUID()
	identityID, eventID := testUUID(), testUUID()
	insertSourceContextParents(t, ctx, pool, botID, sessionID, routeID, identityID, eventID)
	sessionFallbackID, identityFallbackSessionID, malformedEventID, mismatchedEventID := testUUID(), testUUID(), testUUID(), testUUID()
	if _, err := pool.Exec(ctx, `
INSERT INTO bot_sessions (id, bot_id, channel_type, route_id)
VALUES ($1, $3, E'\tsession-platform\n', NULL), ($2, $3, '', NULL)
`, sessionFallbackID, identityFallbackSessionID, botID); err != nil {
		t.Fatalf("insert fallback source sessions: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO bot_session_events (id, bot_id, session_id, event_kind, event_data, sender_channel_identity_id)
VALUES
  ($1, $3, $4, 'message', '{"sender":{"display_name":42},"conversation":{"channel":{},"conversation_type":[],"conversation_name":true}}', $5),
  ($2, $3, $6, 'message', '{"sender":{"display_name":"Wrong Event"},"conversation":{"channel":"wrong-platform","conversation_type":"private","conversation_name":"Wrong Room"}}', $5)
`, malformedEventID, mismatchedEventID, botID, sessionID, identityID, sessionFallbackID); err != nil {
		t.Fatalf("insert fallback source events: %v", err)
	}
	eventMessageID, fallbackMessageID, coveredMessageID := testUUID(), testUUID(), testUUID()
	insertSourceContextMessage(t, ctx, pool, eventMessageID, botID, sessionID, identityID, eventID, 1)
	insertSourceContextMessage(t, ctx, pool, fallbackMessageID, botID, sessionID, identityID, pgtype.UUID{}, 2)
	insertSourceContextMessage(t, ctx, pool, coveredMessageID, botID, sessionID, identityID, eventID, 3)
	routeFallbackID, sessionPlatformID, identityPlatformID := testUUID(), testUUID(), testUUID()
	malformedEventMessageID, mismatchedEventMessageID := testUUID(), testUUID()
	insertSourceContextMessage(t, ctx, pool, routeFallbackID, botID, sessionID, identityID, pgtype.UUID{}, 5, `{"platform":{"bad":true}}`)
	insertSourceContextMessage(t, ctx, pool, sessionPlatformID, botID, sessionFallbackID, identityID, pgtype.UUID{}, 6, `{}`)
	insertSourceContextMessage(t, ctx, pool, identityPlatformID, botID, identityFallbackSessionID, identityID, pgtype.UUID{}, 7, `{}`)
	insertSourceContextMessage(t, ctx, pool, malformedEventMessageID, botID, sessionID, identityID, malformedEventID, 8, `{"platform":"safe-platform"}`)
	insertSourceContextMessage(t, ctx, pool, mismatchedEventMessageID, botID, sessionID, identityID, mismatchedEventID, 9, `{"platform":"safe-platform"}`)
	queries := sqlc.New(pool)
	coveredLogID, pendingLogID := testUUID(), testUUID()
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{coveredLogID, pendingLogID})
	if err := queries.MarkMessagesCompacted(ctx, sqlc.MarkMessagesCompactedParams{
		CompactID: pendingLogID,
		Column2:   []pgtype.UUID{routeFallbackID},
	}); err != nil {
		t.Fatalf("mark pending legacy source: %v", err)
	}
	coveredVersion := strconv.FormatInt(readMessageSourceRevision(t, ctx, pool, coveredMessageID), 10)
	result, err := queries.FinalizeCompactionArtifact(ctx, finalizeParams(
		coveredLogID,
		botID,
		sessionID,
		[]pgtype.UUID{coveredMessageID},
		[]string{coveredVersion},
		"legacy summary",
		[]string{""},
	))
	if err != nil || !result.Finalized {
		t.Fatalf("finalize covered legacy source: result=%+v err=%v", result, err)
	}
	eventRevision := readMessageSourceRevision(t, ctx, pool, eventMessageID)
	fallbackRevision := readMessageSourceRevision(t, ctx, pool, fallbackMessageID)
	coveredRevision := readMessageSourceRevision(t, ctx, pool, coveredMessageID)
	applySourceContextMigration(t, ctx, pool, "0111_activate_message_source_context.up.sql")
	var pendingStatus, pendingError string
	if err := pool.QueryRow(ctx, `SELECT status, error_message FROM bot_history_message_compacts WHERE id = $1`, pendingLogID).Scan(&pendingStatus, &pendingError); err != nil {
		t.Fatalf("read retired pending attempt: %v", err)
	}
	if pendingStatus != "error" || pendingError != "compaction attempt retired by source context activation" {
		t.Fatalf("retired pending attempt = %q/%q", pendingStatus, pendingError)
	}
	if err := queries.MarkMessagesCompacted(ctx, sqlc.MarkMessagesCompactedParams{
		CompactID: pendingLogID,
		Column2:   []pgtype.UUID{sessionPlatformID},
	}); err != nil {
		t.Fatalf("retry retired legacy mark: %v", err)
	}
	var retriedOwner pgtype.UUID
	if err := pool.QueryRow(ctx, `SELECT compact_id FROM bot_history_messages WHERE id = $1`, sessionPlatformID).Scan(&retriedOwner); err != nil {
		t.Fatalf("read retired legacy mark: %v", err)
	}
	if retriedOwner.Valid {
		t.Fatalf("retired legacy attempt reclaimed message as %s", retriedOwner)
	}
	wantContexts := map[pgtype.UUID]messagesource.Context{
		eventMessageID:           messagesource.NewV1("Event Alice", "slack", "group", "Event Room"),
		fallbackMessageID:        messagesource.NewV1("Live Alice", "metadata-platform", "private", "Live Room"),
		routeFallbackID:          messagesource.NewV1("Live Alice", "route-platform", "private", "Live Room"),
		sessionPlatformID:        messagesource.NewV1("Live Alice", "session-platform", "", ""),
		identityPlatformID:       messagesource.NewV1("Live Alice", "telegram", "", ""),
		malformedEventMessageID:  messagesource.NewV1("Live Alice", "safe-platform", "private", "Live Room"),
		mismatchedEventMessageID: messagesource.NewV1("Live Alice", "safe-platform", "private", "Live Room"),
	}
	for messageID, want := range wantContexts {
		assertStoredSourceContext(t, ctx, pool, messageID, want)
	}
	if _, err := pool.Exec(ctx, `UPDATE bot_history_messages SET source_context = jsonb_set(source_context, '{platform}', '"changed"') WHERE id = $1`, fallbackMessageID); err == nil {
		t.Fatal("captured source context remained mutable")
	}
	if got := readMessageSourceRevision(t, ctx, pool, eventMessageID); got != eventRevision+1 {
		t.Fatalf("event backfill revision = %d, want %d", got, eventRevision+1)
	}
	if got := readMessageSourceRevision(t, ctx, pool, fallbackMessageID); got != fallbackRevision+1 {
		t.Fatalf("fallback backfill revision = %d, want %d", got, fallbackRevision+1)
	}
	assertStoredSourceContextAbsent(t, ctx, pool, coveredMessageID)
	if got := readMessageSourceRevision(t, ctx, pool, coveredMessageID); got != coveredRevision {
		t.Fatalf("covered V0 revision = %d, want %d", got, coveredRevision)
	}
	mutations := []struct {
		query string
		id    pgtype.UUID
	}{
		{`UPDATE channel_identities SET display_name = 'Renamed Alice', channel_type = 'discord' WHERE id = $1`, identityID},
		{`UPDATE bot_sessions SET channel_type = 'discord' WHERE id = $1`, sessionID},
		{`UPDATE bot_channel_routes SET conversation_type = 'private', metadata = '{"conversation_name":"Renamed Room"}' WHERE id = $1`, routeID},
	}
	for _, mutation := range mutations {
		if _, err := pool.Exec(ctx, mutation.query, mutation.id); err != nil {
			t.Fatalf("mutate live context: %v", err)
		}
	}
	assertStoredSourceContext(t, ctx, pool, fallbackMessageID, messagesource.NewV1(
		"Live Alice",
		"metadata-platform",
		"private",
		"Live Room",
	))
	newMessageID := testUUID()
	insertSourceContextMessage(t, ctx, pool, newMessageID, botID, sessionID, identityID, eventID, 4)
	assertStoredSourceContext(t, ctx, pool, newMessageID, messagesource.NewV1(
		"Event Alice",
		"slack",
		"group",
		"Event Room",
	))
	backupContext := messagesource.NewV1("Backup Alice", "matrix", "group", "Backup Room")
	backupContextJSON, err := messagesource.Encode(backupContext)
	if err != nil {
		t.Fatalf("encode backup source context: %v", err)
	}
	backupMessage, err := queries.CreateMessage(ctx, sqlc.CreateMessageParams{
		BotID:         botID,
		SessionID:     sessionID,
		Role:          "user",
		Content:       []byte(`{}`),
		Metadata:      []byte(`{}`),
		SessionMode:   "chat",
		RuntimeType:   "model",
		SourceContext: backupContextJSON,
	})
	if err != nil {
		t.Fatalf("restore message source context: %v", err)
	}
	assertStoredSourceContext(t, ctx, pool, backupMessage.ID, backupContext)
	assertBackupSourceContext(t, ctx, queries, botID, backupMessage.ID, backupContext)
	if _, err := pool.Exec(ctx, `UPDATE bot_history_messages SET role = 'assistant' WHERE id = $1`, newMessageID); err != nil {
		t.Fatalf("prepare fork anchor: %v", err)
	}
	forkedSession, err := queries.ForkSessionFromAssistantMessage(ctx, sqlc.ForkSessionFromAssistantMessageParams{
		SessionID: sessionID,
		BotID:     botID,
		MessageID: newMessageID,
		Title:     "fork",
		Metadata:  []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("fork session with source context: %v", err)
	}
	forkedContexts := readSessionSourceContexts(t, ctx, pool, forkedSession.ID)
	if len(forkedContexts) != 4 {
		t.Fatalf("forked source contexts = %d, want 4", len(forkedContexts))
	}
	if want := messagesource.NewV1("Event Alice", "slack", "group", "Event Room"); forkedContexts[0] != want {
		t.Fatalf("forked event source context = %+v, want %+v", forkedContexts[0], want)
	}
	if want := messagesource.NewV1("Live Alice", "metadata-platform", "private", "Live Room"); forkedContexts[1] != want {
		t.Fatalf("forked fallback source context = %+v, want %+v", forkedContexts[1], want)
	}
	var copiedAnchorID string
	if err := pool.QueryRow(ctx, `
SELECT message.id::text
FROM bot_history_messages message
WHERE message.session_id = $1 AND message.role = 'assistant' AND message.turn_position = 4
`, forkedSession.ID).Scan(&copiedAnchorID); err != nil {
		t.Fatalf("inspect fork anchor: %v", err)
	}
	var storedAnchorID string
	if err := pool.QueryRow(ctx, `SELECT metadata #>> '{forked_from,fork_message_id}' FROM bot_sessions WHERE id = $1`, forkedSession.ID).Scan(&storedAnchorID); err != nil {
		t.Fatalf("read fork metadata: %v", err)
	}
	if storedAnchorID != copiedAnchorID || forkedSession.NextTurnPosition != 5 {
		t.Fatalf("fork state = anchor %q/%q next %d", storedAnchorID, copiedAnchorID, forkedSession.NextTurnPosition)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM bot_history_message_compacts WHERE id = $1`, coveredLogID); err != nil {
		t.Fatalf("delete covered legacy artifact: %v", err)
	}
	assertStoredSourceContext(t, ctx, pool, coveredMessageID, messagesource.NewV1(
		"Event Alice",
		"slack",
		"group",
		"Event Room",
	))
	if got := readMessageSourceRevision(t, ctx, pool, coveredMessageID); got != coveredRevision+1 {
		t.Fatalf("lazy upgrade revision = %d, want %d", got, coveredRevision+1)
	}
	retainedLogID := testUUID()
	insertFinalizeLogs(t, ctx, pool, botID, sessionFallbackID, []pgtype.UUID{retainedLogID})
	retainedResult, err := queries.FinalizeCompactionArtifact(ctx, finalizeParams(
		retainedLogID, botID, sessionFallbackID, []pgtype.UUID{sessionPlatformID},
		[]string{strconv.FormatInt(readMessageSourceRevision(t, ctx, pool, sessionPlatformID), 10)},
		"v1 summary", []string{""},
	))
	if err != nil || !retainedResult.Finalized {
		t.Fatalf("finalize retained V1 artifact: result=%+v err=%v", retainedResult, err)
	}
	derivedLogID := testUUID()
	if _, err := pool.Exec(ctx, `
WITH inserted AS (
  INSERT INTO bot_history_message_compacts (
  id, bot_id, session_id, status, error_message, artifact_level, parent_ids, completed_at
) VALUES ($1, $2, $3, 'error', 'rollback lineage', 1, ARRAY[$4]::uuid[], now()) RETURNING id
)
INSERT INTO bot_history_message_compact_parent_edges (artifact_id, parent_id, ordinal)
SELECT id, $4, 0 FROM inserted
`, derivedLogID, botID, sessionFallbackID, retainedLogID); err != nil {
		t.Fatalf("insert rollback lineage: %v", err)
	}
	if compacts, edges, claims := readActivationResidue(t, ctx, pool); compacts != 3 || edges != 1 || claims != 2 {
		t.Fatalf("rollback fixture = compacts %d edges %d claims %d", compacts, edges, claims)
	}
	createDeletedSourceArtifact(t, ctx, pool, queries)

	applySourceContextMigration(t, ctx, pool, "0111_activate_message_source_context.down.sql")
	for _, messageID := range []pgtype.UUID{eventMessageID, fallbackMessageID, coveredMessageID, newMessageID, backupMessage.ID} {
		assertStoredSourceContextAbsent(t, ctx, pool, messageID)
	}
	var remainingSourceContexts int
	if err := pool.QueryRow(ctx, `
SELECT count(*) FROM bot_history_messages WHERE bot_id = $1 AND source_context IS NOT NULL
`, botID).Scan(&remainingSourceContexts); err != nil {
		t.Fatalf("count source contexts after rollback: %v", err)
	}
	if remainingSourceContexts != 0 {
		t.Fatalf("source contexts after rollback = %d, want 0", remainingSourceContexts)
	}
	remainingCompacts, remainingEdges, remainingClaims := readActivationResidue(t, ctx, pool)
	if remainingCompacts != 0 || remainingEdges != 0 || remainingClaims != 0 {
		t.Fatalf("rollback residue = compacts %d edges %d claims %d", remainingCompacts, remainingEdges, remainingClaims)
	}
	beforeUpdate := readMessageSourceRevision(t, ctx, pool, eventMessageID)
	if _, err := pool.Exec(ctx, `UPDATE bot_history_messages SET content = '{"changed":true}' WHERE id = $1`, eventMessageID); err != nil {
		t.Fatalf("update source after activation rollback: %v", err)
	}
	if got := readMessageSourceRevision(t, ctx, pool, eventMessageID); got != beforeUpdate+1 {
		t.Fatalf("rolled-back revision = %d, want %d", got, beforeUpdate+1)
	}
}

func installSourceContextFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
ALTER TABLE bot_history_messages
  ADD COLUMN turn_superseded_by_turn_id UUID,
  ADD COLUMN turn_superseded_at TIMESTAMPTZ,
  ADD COLUMN turn_superseded_reason TEXT;
ALTER TABLE bot_history_messages ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE bot_history_messages
  DROP CONSTRAINT bot_history_messages_compact_id_fkey,
  ADD CONSTRAINT bot_history_messages_compact_id_fkey
    FOREIGN KEY (compact_id) REFERENCES bot_history_message_compacts(id) ON DELETE SET NULL;
ALTER TABLE channel_identities ADD COLUMN channel_type TEXT NOT NULL DEFAULT '';
ALTER TABLE bot_channel_routes
  ADD COLUMN bot_id UUID,
  ADD COLUMN channel_type TEXT;
ALTER TABLE bot_sessions
  ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE bot_sessions
  ADD COLUMN bot_id UUID,
  ADD COLUMN type TEXT NOT NULL DEFAULT 'chat',
  ADD COLUMN session_mode TEXT NOT NULL DEFAULT 'chat',
  ADD COLUMN runtime_type TEXT NOT NULL DEFAULT 'model',
  ADD COLUMN runtime_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN title TEXT NOT NULL DEFAULT '',
  ADD COLUMN metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN next_turn_position BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN parent_session_id UUID,
  ADD COLUMN created_by_user_id UUID,
  ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE TABLE bot_session_events (
  id UUID PRIMARY KEY,
  bot_id UUID NOT NULL,
  session_id UUID NOT NULL,
  event_kind TEXT NOT NULL,
  event_data JSONB NOT NULL,
  sender_channel_identity_id UUID
);
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
		t.Fatalf("install source context fixture: %v", err)
	}
}

func insertSourceContextParents(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	botID, sessionID, routeID, identityID, eventID pgtype.UUID,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
INSERT INTO channel_identities (id, display_name, avatar_url, channel_type)
VALUES ($1, E'\tLive Alice\n', 'alice.png', U&'\00A0telegram\00A0')
`, identityID); err != nil {
		t.Fatalf("insert source identity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO bot_channel_routes (id, bot_id, channel_type, conversation_type, metadata, default_reply_target)
VALUES (
  $1, $2, U&'\00A0route-platform\00A0', E'\tprivate\n',
  jsonb_build_object('conversation_name', U&'\00A0Live Room\00A0'), 'live-target'
)
`, routeID, botID); err != nil {
		t.Fatalf("insert source route: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO bot_sessions (id, bot_id, channel_type, route_id)
VALUES ($1, $2, 'telegram', $3)
`, sessionID, botID, routeID); err != nil {
		t.Fatalf("insert source session: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO bot_session_events (
  id, bot_id, session_id, event_kind, event_data, sender_channel_identity_id
) VALUES (
  $1, $2, $3, 'message',
  '{"sender":{"display_name":"\u00a0Event Alice\u00a0"},"conversation":{"channel":" slack ","conversation_type":" group ","conversation_name":"\u00a0Event Room\u00a0"}}',
  $4
)
`, eventID, botID, sessionID, identityID); err != nil {
		t.Fatalf("insert source event: %v", err)
	}
}

func insertSourceContextMessage(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	messageID, botID, sessionID, identityID, eventID pgtype.UUID,
	position int,
	metadata ...string,
) {
	t.Helper()
	messageMetadata := `{"platform":"\tmetadata-platform\n"}`
	if len(metadata) > 0 {
		messageMetadata = metadata[0]
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO bot_history_messages (
  id, bot_id, session_id, sender_channel_identity_id, event_id,
  content, metadata, turn_id, turn_position, turn_message_seq
) VALUES ($1, $2, $3, $4, $5, '{}', $6::jsonb, $7, $8, 1)
`, messageID, botID, sessionID, identityID, eventID, messageMetadata, testUUID(), position); err != nil {
		t.Fatalf("insert source message: %v", err)
	}
}

func applySourceContextMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) {
	t.Helper()
	if _, err := pool.Exec(ctx, readEmbeddedMigration(t, "postgres/migrations/"+name)); err != nil {
		t.Fatalf("apply %s: %v", name, err)
	}
}

func readMessageSourceRevision(t *testing.T, ctx context.Context, pool *pgxpool.Pool, messageID pgtype.UUID) int64 {
	t.Helper()
	var revision int64
	if err := pool.QueryRow(ctx, `SELECT source_revision FROM bot_history_messages WHERE id = $1`, messageID).Scan(&revision); err != nil {
		t.Fatalf("read source revision: %v", err)
	}
	return revision
}

func assertStoredSourceContext(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	messageID pgtype.UUID, want messagesource.Context,
) {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT source_context FROM bot_history_messages WHERE id = $1`, messageID).Scan(&raw); err != nil {
		t.Fatalf("read source context: %v", err)
	}
	got, err := messagesource.Decode(raw)
	if err != nil {
		t.Fatalf("decode source context: %v", err)
	}
	if got != want {
		t.Fatalf("source context = %+v, want %+v", got, want)
	}
}

func assertStoredSourceContextAbsent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, messageID pgtype.UUID) {
	t.Helper()
	var present bool
	if err := pool.QueryRow(ctx, `SELECT source_context IS NOT NULL FROM bot_history_messages WHERE id = $1`, messageID).Scan(&present); err != nil {
		t.Fatalf("inspect source context: %v", err)
	}
	if present {
		t.Fatalf("message %s unexpectedly has source context", messageID)
	}
}

func assertBackupSourceContext(
	t *testing.T, ctx context.Context, queries *sqlc.Queries,
	botID, messageID pgtype.UUID, want messagesource.Context,
) {
	t.Helper()
	rows, err := queries.ListAllMessagesForBackup(ctx, botID)
	if err != nil {
		t.Fatalf("list backup messages: %v", err)
	}
	for _, row := range rows {
		if row.ID != messageID {
			continue
		}
		got, err := messagesource.Decode(row.SourceContext)
		if err != nil {
			t.Fatalf("decode backup source context: %v", err)
		}
		if got != want {
			t.Fatalf("backup source context = %+v, want %+v", got, want)
		}
		return
	}
	t.Fatalf("backup message %s not found", messageID)
}

func readSessionSourceContexts(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool, sessionID pgtype.UUID,
) []messagesource.Context {
	t.Helper()
	rows, err := pool.Query(ctx, `
SELECT source_context
FROM bot_history_messages
WHERE session_id = $1
ORDER BY turn_position, turn_message_seq, created_at, id
`, sessionID)
	if err != nil {
		t.Fatalf("list session source contexts: %v", err)
	}
	defer rows.Close()

	var contexts []messagesource.Context
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("scan session source context: %v", err)
		}
		context, err := messagesource.Decode(raw)
		if err != nil {
			t.Fatalf("decode session source context: %v", err)
		}
		contexts = append(contexts, context)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate session source contexts: %v", err)
	}
	return contexts
}
