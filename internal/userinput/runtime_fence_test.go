package userinput

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/runtimefence"
)

type staleUserInputQueries struct {
	dbstore.Queries
	cancelCalled              bool
	markPromptDeliveredCalled bool
}

func (*staleUserInputQueries) SupportsTransactions() bool { return true }

func (q *staleUserInputQueries) InTx(_ context.Context, fn func(dbstore.Queries) error) error {
	return fn(q)
}

func (*staleUserInputQueries) LockBotForSessionWrite(_ context.Context, id pgtype.UUID) (pgtype.UUID, error) {
	return id, nil
}

func (*staleUserInputQueries) LockSessionRuntimeFence(context.Context, sqlc.LockSessionRuntimeFenceParams) (int64, error) {
	return 0, pgx.ErrNoRows
}

func (q *staleUserInputQueries) CancelUserInputRequest(context.Context, sqlc.CancelUserInputRequestParams) (sqlc.UserInputRequest, error) {
	q.cancelCalled = true
	return sqlc.UserInputRequest{}, nil
}

func (q *staleUserInputQueries) MarkUserInputPromptDelivered(context.Context, pgtype.UUID) (sqlc.UserInputRequest, error) {
	q.markPromptDeliveredCalled = true
	return sqlc.UserInputRequest{}, nil
}

func TestCancelRejectsStaleRuntimeFence(t *testing.T) {
	t.Parallel()

	queries := &staleUserInputQueries{}
	service := NewService(nil, queries)
	ctx := runtimefence.WithContext(context.Background(), runtimefence.Fence{
		BotID:     "11111111-1111-1111-1111-111111111111",
		SessionID: "22222222-2222-2222-2222-222222222222",
		Token:     4,
	})
	_, err := service.Cancel(ctx, CancelInput{RequestID: "33333333-3333-3333-3333-333333333333"})
	if !errors.Is(err, runtimefence.ErrStale) {
		t.Fatalf("Cancel() error = %v, want ErrStale", err)
	}
	if queries.cancelCalled {
		t.Fatal("stale user input response updated the request")
	}
}

func TestMarkPromptDeliveredRejectsStaleRuntimeFence(t *testing.T) {
	t.Parallel()

	queries := &staleUserInputQueries{}
	service := NewService(nil, queries)
	ctx := runtimefence.WithContext(context.Background(), runtimefence.Fence{
		BotID:     "11111111-1111-1111-1111-111111111111",
		SessionID: "22222222-2222-2222-2222-222222222222",
		Token:     4,
	})
	_, err := service.MarkPromptDelivered(ctx, "33333333-3333-3333-3333-333333333333")
	if !errors.Is(err, runtimefence.ErrStale) {
		t.Fatalf("MarkPromptDelivered() error = %v, want ErrStale", err)
	}
	if queries.markPromptDeliveredCalled {
		t.Fatal("stale runtime marked the user input prompt delivered")
	}
}
