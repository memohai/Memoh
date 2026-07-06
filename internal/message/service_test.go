package message

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	messageevent "github.com/memohai/memoh/internal/message/event"
)

type runtimeSnapshotQueries struct {
	dbstore.Queries

	created sqlc.CreateMessageParams
}

func (*runtimeSnapshotQueries) GetSessionByID(_ context.Context, id pgtype.UUID) (sqlc.BotSession, error) {
	return sqlc.BotSession{
		ID:          id,
		Type:        "acp_agent",
		SessionMode: "chat",
		RuntimeType: "acp_agent",
	}, nil
}

func (q *runtimeSnapshotQueries) CreateMessage(_ context.Context, arg sqlc.CreateMessageParams) (sqlc.CreateMessageRow, error) {
	q.created = arg
	return sqlc.CreateMessageRow{
		ID:          testMessageUUID("33333333-3333-3333-3333-333333333333"),
		BotID:       arg.BotID,
		SessionID:   arg.SessionID,
		Role:        arg.Role,
		Content:     arg.Content,
		Metadata:    arg.Metadata,
		Usage:       arg.Usage,
		SessionMode: arg.SessionMode,
		RuntimeType: arg.RuntimeType,
		DisplayText: arg.DisplayText,
		CreatedAt:   pgtype.Timestamptz{Valid: true},
	}, nil
}

func (*runtimeSnapshotQueries) CreateHistoryTurn(context.Context, sqlc.CreateHistoryTurnParams) (dbstore.HistoryTurn, error) {
	return dbstore.HistoryTurn{}, nil
}

func (*runtimeSnapshotQueries) AppendMessageToHistoryTurnByRequest(context.Context, sqlc.AppendMessageToHistoryTurnByRequestParams) (pgtype.UUID, error) {
	return pgtype.UUID{}, nil
}

func (*runtimeSnapshotQueries) BindHistoryTurnAssistantByRequest(context.Context, sqlc.BindHistoryTurnAssistantByRequestParams) (dbstore.HistoryTurn, error) {
	return dbstore.HistoryTurn{}, nil
}

func (*runtimeSnapshotQueries) BindLatestHistoryTurnAssistant(context.Context, sqlc.BindLatestHistoryTurnAssistantParams) (dbstore.HistoryTurn, error) {
	return dbstore.HistoryTurn{}, nil
}

func (*runtimeSnapshotQueries) GetLatestVisibleHistoryTurnBySession(context.Context, pgtype.UUID) (dbstore.HistoryTurn, error) {
	return dbstore.HistoryTurn{}, nil
}

func (*runtimeSnapshotQueries) LinkMessageToHistoryTurn(_ context.Context, arg sqlc.LinkMessageToHistoryTurnParams) (pgtype.UUID, error) {
	return arg.MessageID, nil
}

func (*runtimeSnapshotQueries) AppendMessageToLatestHistoryTurn(_ context.Context, arg sqlc.AppendMessageToLatestHistoryTurnParams) (pgtype.UUID, error) {
	return arg.MessageID, nil
}

func (*runtimeSnapshotQueries) LinkUnassignedMessagesAfterHistoryTurnAssistant(context.Context, pgtype.UUID) error {
	return nil
}

func TestPersistResolvesRuntimeSnapshotFromSession(t *testing.T) {
	queries := &runtimeSnapshotQueries{}
	svc := NewService(nil, queries)

	msg, err := svc.Persist(context.Background(), PersistInput{
		BotID:     "11111111-1111-1111-1111-111111111111",
		SessionID: "22222222-2222-2222-2222-222222222222",
		Role:      "user",
		Content:   []byte(`{"type":"text","text":"hello"}`),
	})
	if err != nil {
		t.Fatalf("Persist() error = %v", err)
	}

	if queries.created.SessionMode != "chat" || queries.created.RuntimeType != "acp_agent" {
		t.Fatalf("CreateMessage runtime snapshot = %q/%q, want chat/acp_agent", queries.created.SessionMode, queries.created.RuntimeType)
	}
	if msg.SessionMode != "chat" || msg.RuntimeType != "acp_agent" {
		t.Fatalf("message runtime snapshot = %q/%q, want chat/acp_agent", msg.SessionMode, msg.RuntimeType)
	}
}

type failingHistoryTurnQueries struct {
	dbstore.Queries

	deleted []pgtype.UUID
}

func (*failingHistoryTurnQueries) GetSessionByID(_ context.Context, id pgtype.UUID) (sqlc.BotSession, error) {
	return sqlc.BotSession{
		ID:          id,
		Type:        "chat",
		SessionMode: "chat",
		RuntimeType: "model",
	}, nil
}

func (*failingHistoryTurnQueries) CreateMessage(_ context.Context, arg sqlc.CreateMessageParams) (sqlc.CreateMessageRow, error) {
	return sqlc.CreateMessageRow{
		ID:          testMessageUUID("44444444-4444-4444-4444-444444444444"),
		BotID:       arg.BotID,
		SessionID:   arg.SessionID,
		Role:        arg.Role,
		Content:     arg.Content,
		Metadata:    arg.Metadata,
		Usage:       arg.Usage,
		SessionMode: arg.SessionMode,
		RuntimeType: arg.RuntimeType,
		DisplayText: arg.DisplayText,
		CreatedAt:   pgtype.Timestamptz{Valid: true},
	}, nil
}

func (*failingHistoryTurnQueries) CreateHistoryTurn(context.Context, sqlc.CreateHistoryTurnParams) (dbstore.HistoryTurn, error) {
	return dbstore.HistoryTurn{}, errors.New("boom")
}

func (*failingHistoryTurnQueries) AppendMessageToHistoryTurnByRequest(context.Context, sqlc.AppendMessageToHistoryTurnByRequestParams) (pgtype.UUID, error) {
	return pgtype.UUID{}, nil
}

func (*failingHistoryTurnQueries) BindHistoryTurnAssistantByRequest(context.Context, sqlc.BindHistoryTurnAssistantByRequestParams) (dbstore.HistoryTurn, error) {
	return dbstore.HistoryTurn{}, nil
}

func (*failingHistoryTurnQueries) BindLatestHistoryTurnAssistant(context.Context, sqlc.BindLatestHistoryTurnAssistantParams) (dbstore.HistoryTurn, error) {
	return dbstore.HistoryTurn{}, nil
}

func (*failingHistoryTurnQueries) GetLatestVisibleHistoryTurnBySession(context.Context, pgtype.UUID) (dbstore.HistoryTurn, error) {
	return dbstore.HistoryTurn{}, nil
}

func (*failingHistoryTurnQueries) LinkMessageToHistoryTurn(_ context.Context, arg sqlc.LinkMessageToHistoryTurnParams) (pgtype.UUID, error) {
	return arg.MessageID, nil
}

func (*failingHistoryTurnQueries) AppendMessageToLatestHistoryTurn(_ context.Context, arg sqlc.AppendMessageToLatestHistoryTurnParams) (pgtype.UUID, error) {
	return arg.MessageID, nil
}

func (*failingHistoryTurnQueries) LinkUnassignedMessagesAfterHistoryTurnAssistant(context.Context, pgtype.UUID) error {
	return nil
}

func (q *failingHistoryTurnQueries) DeleteMessagesByIDs(_ context.Context, ids []pgtype.UUID) error {
	q.deleted = append(q.deleted, ids...)
	return nil
}

type recordingPublisher struct {
	events []messageevent.Event
}

func (p *recordingPublisher) Publish(event messageevent.Event) {
	p.events = append(p.events, event)
}

func TestPersistCleansUpMessageWhenHistoryTurnFails(t *testing.T) {
	queries := &failingHistoryTurnQueries{}
	publisher := &recordingPublisher{}
	svc := NewService(nil, queries, publisher)

	_, err := svc.Persist(context.Background(), PersistInput{
		BotID:     "11111111-1111-1111-1111-111111111111",
		SessionID: "22222222-2222-2222-2222-222222222222",
		Role:      "user",
		Content:   []byte(`{"type":"text","text":"hello"}`),
	})
	if err == nil {
		t.Fatal("Persist() error = nil, want history turn failure")
	}
	if len(queries.deleted) != 1 {
		t.Fatalf("deleted messages = %d, want 1", len(queries.deleted))
	}
	if got := queries.deleted[0].String(); got != "44444444-4444-4444-4444-444444444444" {
		t.Fatalf("deleted message id = %s", got)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("published events = %d, want 0", len(publisher.events))
	}
}

type retryTurnSequenceQueries struct {
	dbstore.Queries

	linkAttempts int
	deleted      []pgtype.UUID
}

func (*retryTurnSequenceQueries) GetSessionByID(_ context.Context, id pgtype.UUID) (sqlc.BotSession, error) {
	return sqlc.BotSession{
		ID:          id,
		Type:        "chat",
		SessionMode: "chat",
		RuntimeType: "model",
	}, nil
}

func (*retryTurnSequenceQueries) CreateMessage(_ context.Context, arg sqlc.CreateMessageParams) (sqlc.CreateMessageRow, error) {
	return sqlc.CreateMessageRow{
		ID:          testMessageUUID("55555555-5555-5555-5555-555555555555"),
		BotID:       arg.BotID,
		SessionID:   arg.SessionID,
		Role:        arg.Role,
		Content:     arg.Content,
		Metadata:    arg.Metadata,
		Usage:       arg.Usage,
		SessionMode: arg.SessionMode,
		RuntimeType: arg.RuntimeType,
		DisplayText: arg.DisplayText,
		CreatedAt:   pgtype.Timestamptz{Valid: true},
	}, nil
}

func (*retryTurnSequenceQueries) CreateHistoryTurn(_ context.Context, arg sqlc.CreateHistoryTurnParams) (dbstore.HistoryTurn, error) {
	return dbstore.HistoryTurn{
		ID:        testMessageUUID("66666666-6666-6666-6666-666666666666"),
		BotID:     arg.BotID,
		SessionID: arg.SessionID,
		Position:  1,
	}, nil
}

func (*retryTurnSequenceQueries) AppendMessageToHistoryTurnByRequest(context.Context, sqlc.AppendMessageToHistoryTurnByRequestParams) (pgtype.UUID, error) {
	return pgtype.UUID{}, nil
}

func (*retryTurnSequenceQueries) BindHistoryTurnAssistantByRequest(_ context.Context, arg sqlc.BindHistoryTurnAssistantByRequestParams) (dbstore.HistoryTurn, error) {
	return dbstore.HistoryTurn{
		ID:                 testMessageUUID("66666666-6666-6666-6666-666666666666"),
		SessionID:          arg.SessionID,
		RequestMessageID:   arg.RequestMessageID,
		AssistantMessageID: arg.AssistantMessageID,
		Position:           1,
	}, nil
}

func (*retryTurnSequenceQueries) BindLatestHistoryTurnAssistant(context.Context, sqlc.BindLatestHistoryTurnAssistantParams) (dbstore.HistoryTurn, error) {
	return dbstore.HistoryTurn{}, nil
}

func (*retryTurnSequenceQueries) GetLatestVisibleHistoryTurnBySession(context.Context, pgtype.UUID) (dbstore.HistoryTurn, error) {
	return dbstore.HistoryTurn{}, nil
}

func (q *retryTurnSequenceQueries) LinkMessageToHistoryTurn(_ context.Context, arg sqlc.LinkMessageToHistoryTurnParams) (pgtype.UUID, error) {
	q.linkAttempts++
	if q.linkAttempts == 1 {
		return pgtype.UUID{}, &pgconn.PgError{Code: "23505", ConstraintName: "idx_bot_history_messages_turn_seq_unique"}
	}
	return arg.MessageID, nil
}

func (*retryTurnSequenceQueries) AppendMessageToLatestHistoryTurn(_ context.Context, arg sqlc.AppendMessageToLatestHistoryTurnParams) (pgtype.UUID, error) {
	return arg.MessageID, nil
}

func (*retryTurnSequenceQueries) LinkUnassignedMessagesAfterHistoryTurnAssistant(context.Context, pgtype.UUID) error {
	return nil
}

func (q *retryTurnSequenceQueries) DeleteMessagesByIDs(_ context.Context, ids []pgtype.UUID) error {
	q.deleted = append(q.deleted, ids...)
	return nil
}

func TestPersistRetriesTurnSequenceUniqueViolation(t *testing.T) {
	queries := &retryTurnSequenceQueries{}
	publisher := &recordingPublisher{}
	svc := NewService(nil, queries, publisher)

	msg, err := svc.Persist(context.Background(), PersistInput{
		BotID:                "11111111-1111-1111-1111-111111111111",
		SessionID:            "22222222-2222-2222-2222-222222222222",
		Role:                 "assistant",
		Content:              []byte(`{"type":"text","text":"hello"}`),
		TurnRequestMessageID: "77777777-7777-7777-7777-777777777777",
	})
	if err != nil {
		t.Fatalf("Persist() error = %v", err)
	}
	if msg.ID != "55555555-5555-5555-5555-555555555555" {
		t.Fatalf("message id = %s", msg.ID)
	}
	if queries.linkAttempts != 2 {
		t.Fatalf("link attempts = %d, want 2", queries.linkAttempts)
	}
	if len(queries.deleted) != 1 {
		t.Fatalf("deleted messages = %d, want 1", len(queries.deleted))
	}
	if len(publisher.events) != 1 {
		t.Fatalf("published events = %d, want 1", len(publisher.events))
	}
}

func testMessageUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		panic(err)
	}
	return id
}
