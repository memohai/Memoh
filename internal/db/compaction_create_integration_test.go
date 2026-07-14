package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestCreateCompactionLogPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	ctx := context.Background()
	queries := sqlc.New(pool)

	t.Run("repeated attempt id creates one pending row", func(t *testing.T) {
		params := sqlc.CreateCompactionLogParams{
			ID:        testUUID(),
			BotID:     testUUID(),
			SessionID: testUUID(),
		}

		row, err := queries.CreateCompactionLog(ctx, params)
		if err != nil {
			t.Fatalf("create compaction log: %v", err)
		}
		assertPristineCompactionLog(t, row, params)
		if _, err := queries.CreateCompactionLog(ctx, params); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("repeat create error = %v, want no returned duplicate", err)
		}
		assertCompactionLogCount(t, ctx, pool, params.ID, 1)
	})

	t.Run("concurrent attempt id creates one pending row", func(t *testing.T) {
		params := sqlc.CreateCompactionLogParams{
			ID:        testUUID(),
			BotID:     testUUID(),
			SessionID: testUUID(),
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		for range 2 {
			go func() {
				<-start
				_, err := queries.CreateCompactionLog(ctx, params)
				results <- err
			}()
		}
		close(start)

		var created, duplicate int
		for range 2 {
			switch err := <-results; {
			case err == nil:
				created++
			case errors.Is(err, pgx.ErrNoRows):
				duplicate++
			default:
				t.Fatalf("concurrent create: %v", err)
			}
		}
		if created != 1 || duplicate != 1 {
			t.Fatalf("concurrent results = created:%d duplicate:%d, want 1/1", created, duplicate)
		}
		assertCompactionLogCount(t, ctx, pool, params.ID, 1)
	})

	t.Run("attempt id collision cannot overwrite scope", func(t *testing.T) {
		id := testUUID()
		existingBotID, existingSessionID := testUUID(), testUUID()
		if _, err := pool.Exec(ctx, `
INSERT INTO bot_history_message_compacts (id, bot_id, session_id) VALUES ($1, $2, $3)
`, id, existingBotID, existingSessionID); err != nil {
			t.Fatalf("insert existing attempt: %v", err)
		}
		params := sqlc.CreateCompactionLogParams{ID: id, BotID: testUUID(), SessionID: testUUID()}
		if _, err := queries.CreateCompactionLog(ctx, params); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("colliding create error = %v, want no returned row", err)
		}

		var botID, sessionID pgtype.UUID
		if err := pool.QueryRow(ctx, `SELECT bot_id, session_id FROM bot_history_message_compacts WHERE id = $1`, id).Scan(&botID, &sessionID); err != nil {
			t.Fatalf("read colliding attempt: %v", err)
		}
		if botID != existingBotID || sessionID != existingSessionID {
			t.Fatalf("collision changed scope to bot=%s session=%s", botID, sessionID)
		}
	})
}

func assertPristineCompactionLog(t *testing.T, row sqlc.BotHistoryMessageCompact, params sqlc.CreateCompactionLogParams) {
	t.Helper()
	if row.ID != params.ID || row.BotID != params.BotID || row.SessionID != params.SessionID || row.Status != "pending" {
		t.Fatalf("created attempt identity = id:%s bot:%s session:%s status:%q", row.ID, row.BotID, row.SessionID, row.Status)
	}
	if row.Summary != "" || row.MessageCount != 0 || row.ErrorMessage != "" || len(row.Usage) != 0 || row.ModelID.Valid ||
		row.ArtifactVersion != 1 || string(row.Coverage) != "[]" || row.AnchorStartMs != 0 || row.AnchorEndMs != 0 ||
		row.ArtifactLevel != 0 || len(row.ParentIds) != 0 || row.SupersededBy.Valid || row.SupersededAt.Valid ||
		!row.StartedAt.Valid || row.CompletedAt.Valid {
		t.Fatalf("created attempt is not pristine: %#v", row)
	}
}

func assertCompactionLogCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id pgtype.UUID, expected int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bot_history_message_compacts WHERE id = $1`, id).Scan(&count); err != nil {
		t.Fatalf("count compaction attempts: %v", err)
	}
	if count != expected {
		t.Fatalf("attempt row count = %d, want %d", count, expected)
	}
}
