package flow

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type recordingRuntimeFenceQueries struct {
	dbstore.Queries
	activateParams sqlc.ActivateSessionRuntimeFenceParams
	token          int64
}

func (*recordingRuntimeFenceQueries) SupportsTransactions() bool { return true }

func (q *recordingRuntimeFenceQueries) InTx(_ context.Context, fn func(dbstore.Queries) error) error {
	return fn(q)
}

func (*recordingRuntimeFenceQueries) LockBotForSessionWrite(_ context.Context, id pgtype.UUID) (pgtype.UUID, error) {
	return id, nil
}

func (q *recordingRuntimeFenceQueries) NextSessionRuntimeFenceToken(context.Context) (int64, error) {
	return q.token, nil
}

func (q *recordingRuntimeFenceQueries) ActivateSessionRuntimeFence(_ context.Context, params sqlc.ActivateSessionRuntimeFenceParams) (int64, error) {
	q.activateParams = params
	return params.RuntimeFencingToken, nil
}

func (*recordingRuntimeFenceQueries) LockSessionRuntimeFenceForActivation(context.Context, sqlc.LockSessionRuntimeFenceForActivationParams) (int64, error) {
	return 0, nil
}

func (*recordingRuntimeFenceQueries) SupersedePendingToolApprovalsBySession(context.Context, sqlc.SupersedePendingToolApprovalsBySessionParams) ([]sqlc.ToolApprovalRequest, error) {
	return nil, nil
}

func (*recordingRuntimeFenceQueries) SupersedePendingUserInputsBySession(context.Context, sqlc.SupersedePendingUserInputsBySessionParams) ([]sqlc.UserInputRequest, error) {
	return nil, nil
}

func TestAllocateAndActivateRuntimePersistenceFence(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
	)
	queries := &recordingRuntimeFenceQueries{token: 12}
	resolver := &Resolver{queries: queries}
	fence, err := resolver.AllocateRuntimePersistenceFence(context.Background(), botID, sessionID)
	if err != nil {
		t.Fatalf("AllocateRuntimePersistenceFence() error = %v", err)
	}
	if fence.BotID != botID || fence.SessionID != sessionID || fence.Token != 12 {
		t.Fatalf("runtime fence = %#v", fence)
	}
	if err := resolver.ActivateRuntimePersistenceFence(context.Background(), fence); err != nil {
		t.Fatalf("ActivateRuntimePersistenceFence() error = %v", err)
	}
	if queries.activateParams.BotID != mustRuntimeFenceUUID(t, botID) || queries.activateParams.SessionID != mustRuntimeFenceUUID(t, sessionID) || queries.activateParams.RuntimeFencingToken != 12 {
		t.Fatalf("activate params = %#v", queries.activateParams)
	}
}

func mustRuntimeFenceUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatalf("parse UUID %q: %v", value, err)
	}
	return id
}
