package message

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	messageevent "github.com/memohai/memoh/internal/message/event"
)

type persistAtomicQueries struct {
	dbstore.Queries
	updateErr error
}

func (q *persistAtomicQueries) RunInTx(_ context.Context, fn func(dbstore.Queries) error) error {
	return fn(q)
}

func (*persistAtomicQueries) GetNextTurnMessageSeq(context.Context, pgtype.UUID) (int64, error) {
	return 1, nil
}

func (*persistAtomicQueries) CreateMessage(_ context.Context, arg sqlc.CreateMessageParams) (sqlc.CreateMessageRow, error) {
	return sqlc.CreateMessageRow{
		ID:             testMessageUUID("33333333-3333-3333-3333-333333333333"),
		BotID:          arg.BotID,
		SessionID:      arg.SessionID,
		TurnID:         arg.TurnID,
		TurnMessageSeq: arg.TurnMessageSeq,
		Role:           arg.Role,
		Content:        arg.Content,
		Metadata:       arg.Metadata,
		Usage:          arg.Usage,
		CreatedAt:      pgtype.Timestamptz{Valid: true},
	}, nil
}

func (q *persistAtomicQueries) UpdateHistoryTurnRequestMessage(context.Context, sqlc.UpdateHistoryTurnRequestMessageParams) (sqlc.BotHistoryTurn, error) {
	if q.updateErr != nil {
		return sqlc.BotHistoryTurn{}, q.updateErr
	}
	return sqlc.BotHistoryTurn{}, nil
}

type recordingPublisher struct {
	events []messageevent.Event
}

func (p *recordingPublisher) Publish(event messageevent.Event) {
	p.events = append(p.events, event)
}

func TestPersistReturnsErrorWhenTurnPointerUpdateFails(t *testing.T) {
	pointerErr := errors.New("pointer update failed")
	publisher := &recordingPublisher{}
	svc := NewService(nil, &persistAtomicQueries{updateErr: pointerErr}, publisher)

	_, err := svc.Persist(context.Background(), PersistInput{
		BotID:     "11111111-1111-1111-1111-111111111111",
		SessionID: "22222222-2222-2222-2222-222222222222",
		TurnID:    "44444444-4444-4444-4444-444444444444",
		Role:      "user",
		Content:   []byte(`{"type":"text","text":"hello"}`),
	})
	if err == nil {
		t.Fatal("Persist() error = nil, want turn pointer update error")
	}
	if !strings.Contains(err.Error(), "update history turn request message") {
		t.Fatalf("Persist() error = %v, want turn pointer context", err)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("published %d events after failed persist, want 0", len(publisher.events))
	}
}

func testMessageUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		panic(err)
	}
	return id
}
