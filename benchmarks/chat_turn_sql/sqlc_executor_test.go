package main

import (
	"context"
	"errors"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	postgresqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestSQLCExecutorConstructsParamsForKnownQueries(t *testing.T) {
	cfg := defaultConfig()
	cfg.Workload.SelectedHeadRatio = 1
	executor := &sqlcExecutor{
		cfg:     cfg,
		queries: postgresqlc.New(failingDBTX{}),
	}
	sample := SessionSeed{
		BotID:              uuid.New(),
		RouteID:            uuid.New(),
		SessionID:          uuid.New(),
		DefaultHeadTurnID:  uuid.New(),
		HeadTurnIDs:        []uuid.UUID{uuid.New()},
		LatestMessageID:    uuid.New(),
		CursorMessageIDs:   []uuid.UUID{uuid.New()},
		CursorCreatedAts:   []time.Time{time.Now().UTC()},
		ExternalMessageID:  "external-message",
		ApprovalRequestID:  uuid.New(),
		ApprovalBaseReqID:  uuid.New(),
		ApprovalShortID:    1,
		ApprovalPromptID:   "approval-prompt",
		UserInputRequestID: uuid.New(),
		UserInputBaseReqID: uuid.New(),
		UserInputShortID:   1,
		UserInputPromptID:  "user-input-prompt",
	}
	// #nosec G404 -- deterministic pseudo-random sampling keeps the test reproducible.
	rng := rand.New(rand.NewPCG(1, 1))
	for _, queryName := range knownQueries {
		_, err := executor.execQuery(context.Background(), queryName, sample, rng)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("%s: expected fake DB error after param construction, got %v", queryName, err)
		}
	}
}

type failingDBTX struct{}

func (failingDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, context.Canceled
}

func (failingDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, context.Canceled
}

func (failingDBTX) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	return failingRow{}
}

type failingRow struct{}

func (failingRow) Scan(...interface{}) error {
	return context.Canceled
}
