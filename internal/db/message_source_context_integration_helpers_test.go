package db

import (
	"context"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func createDeletedSourceArtifact(t *testing.T, ctx context.Context, pool *pgxpool.Pool, queries *sqlc.Queries) {
	t.Helper()
	botID, sessionID, messageID, logID := testUUID(), testUUID(), testUUID(), testUUID()
	if _, err := pool.Exec(ctx, `
INSERT INTO bot_sessions (id, bot_id, channel_type) VALUES ($1, $2, 'isolated')
`, sessionID, botID); err != nil {
		t.Fatalf("insert deleted-source session: %v", err)
	}
	insertSourceContextMessage(t, ctx, pool, messageID, botID, sessionID, pgtype.UUID{}, pgtype.UUID{}, 1, `{}`)
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{logID})
	result, err := queries.FinalizeCompactionArtifact(ctx, finalizeParams(
		logID, botID, sessionID, []pgtype.UUID{messageID},
		[]string{strconv.FormatInt(readMessageSourceRevision(t, ctx, pool, messageID), 10)},
		"deleted source summary", []string{""},
	))
	if err != nil || !result.Finalized {
		t.Fatalf("finalize deleted-source artifact: result=%+v err=%v", result, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM bot_history_messages WHERE id = $1`, messageID); err != nil {
		t.Fatalf("delete finalized source: %v", err)
	}
}

func readActivationResidue(
	t *testing.T, ctx context.Context, pool *pgxpool.Pool,
) (compacts, edges, claims int) {
	t.Helper()
	err := pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM bot_history_message_compacts),
  (SELECT count(*) FROM bot_history_message_compact_parent_edges),
  (SELECT count(*) FROM bot_history_messages WHERE compact_id IS NOT NULL OR compact_claim_finalized)
`).Scan(&compacts, &edges, &claims)
	if err != nil {
		t.Fatalf("inspect activation residue: %v", err)
	}
	return compacts, edges, claims
}
