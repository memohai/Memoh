package message

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	messageevent "github.com/memohai/memoh/internal/message/event"
)

type runtimeSnapshotQueries struct {
	dbstore.Queries

	created  sqlc.CreateMessageParams
	assetErr error
}

func (q *runtimeSnapshotQueries) CreateMessageAsset(context.Context, sqlc.CreateMessageAssetParams) (sqlc.CreateMessageAssetRow, error) {
	return sqlc.CreateMessageAssetRow{}, q.assetErr
}

type visibleFromQueries struct {
	dbstore.Queries
	params sqlc.ListVisibleMessagesFromBySessionParams
}

type visibleTurnQueries struct {
	dbstore.Queries
	params sqlc.ListVisibleMessagesByTurnIDBySessionParams
}

func (q *visibleFromQueries) ListVisibleMessagesFromBySession(_ context.Context, arg sqlc.ListVisibleMessagesFromBySessionParams) ([]sqlc.ListVisibleMessagesFromBySessionRow, error) {
	q.params = arg
	return []sqlc.ListVisibleMessagesFromBySessionRow{{
		ID:             testMessageUUID("33333333-3333-3333-3333-333333333333"),
		BotID:          testMessageUUID("11111111-1111-1111-1111-111111111111"),
		SessionID:      arg.SessionID,
		Role:           "assistant",
		Content:        []byte(`{"role":"assistant","content":"done"}`),
		Metadata:       []byte(`{}`),
		SessionMode:    "chat",
		RuntimeType:    "model",
		TurnID:         testMessageUUID("44444444-4444-4444-4444-444444444444"),
		TurnPosition:   pgtype.Int8{Int64: 7, Valid: true},
		TurnMessageSeq: pgtype.Int8{Int64: 2, Valid: true},
	}}, nil
}

func (*visibleFromQueries) ListMessageAssetsBatch(context.Context, []pgtype.UUID) ([]sqlc.ListMessageAssetsBatchRow, error) {
	return nil, nil
}

func (q *visibleTurnQueries) ListVisibleMessagesByTurnIDBySession(_ context.Context, arg sqlc.ListVisibleMessagesByTurnIDBySessionParams) ([]sqlc.ListVisibleMessagesByTurnIDBySessionRow, error) {
	q.params = arg
	return []sqlc.ListVisibleMessagesByTurnIDBySessionRow{
		{
			ID: testMessageUUID("33333333-3333-3333-3333-333333333331"), BotID: testMessageUUID("11111111-1111-1111-1111-111111111111"),
			SessionID: arg.SessionID, Role: "user", Content: []byte(`{"role":"user","content":"hello"}`), Metadata: []byte(`{}`),
			SessionMode: "chat", RuntimeType: "model", TurnID: arg.TurnID,
			TurnPosition: pgtype.Int8{Int64: 7, Valid: true}, TurnMessageSeq: pgtype.Int8{Int64: 1, Valid: true},
		},
		{
			ID: testMessageUUID("33333333-3333-3333-3333-333333333332"), BotID: testMessageUUID("11111111-1111-1111-1111-111111111111"),
			SessionID: arg.SessionID, Role: "assistant", Content: []byte(`{"role":"assistant","content":"done"}`), Metadata: []byte(`{}`),
			SessionMode: "chat", RuntimeType: "model", TurnID: arg.TurnID,
			TurnPosition: pgtype.Int8{Int64: 7, Valid: true}, TurnMessageSeq: pgtype.Int8{Int64: 2, Valid: true},
		},
	}, nil
}

func (*visibleTurnQueries) ListMessageAssetsBatch(context.Context, []pgtype.UUID) ([]sqlc.ListMessageAssetsBatchRow, error) {
	return nil, nil
}

func TestListVisibleFromBySessionMapsStableTurnCoordinates(t *testing.T) {
	t.Parallel()

	const (
		sessionID = "22222222-2222-2222-2222-222222222222"
		messageID = "55555555-5555-5555-5555-555555555555"
	)
	queries := &visibleFromQueries{}
	svc := NewService(nil, queries)

	messages, err := svc.ListVisibleFromBySession(context.Background(), sessionID, messageID, 17)
	if err != nil {
		t.Fatalf("ListVisibleFromBySession() error = %v", err)
	}
	if queries.params.SessionID.String() != sessionID || queries.params.MessageID.String() != messageID {
		t.Fatalf("query args = %s/%s, want %s/%s", queries.params.SessionID.String(), queries.params.MessageID.String(), sessionID, messageID)
	}
	if queries.params.MaxCount != 17 {
		t.Fatalf("query maxCount = %d, want 17", queries.params.MaxCount)
	}
	if len(messages) != 1 {
		t.Fatalf("messages length = %d, want 1", len(messages))
	}
	message := messages[0]
	if message.TurnID != "44444444-4444-4444-4444-444444444444" || message.TurnPosition != 7 || message.TurnMessageSeq != 2 {
		t.Fatalf("turn identity = %q/%d/%d, want stable turn coordinates", message.TurnID, message.TurnPosition, message.TurnMessageSeq)
	}
}

func TestListVisibleMessagesByTurnIDBySessionMapsCompleteTurnCoordinates(t *testing.T) {
	t.Parallel()

	const (
		sessionID = "22222222-2222-2222-2222-222222222222"
		turnID    = "44444444-4444-4444-4444-444444444444"
	)
	queries := &visibleTurnQueries{}
	svc := NewService(nil, queries)

	messages, err := svc.ListVisibleMessagesByTurnIDBySession(context.Background(), sessionID, turnID)
	if err != nil {
		t.Fatalf("ListVisibleMessagesByTurnIDBySession() error = %v", err)
	}
	if queries.params.SessionID.String() != sessionID || queries.params.TurnID.String() != turnID {
		t.Fatalf("query args = %s/%s, want %s/%s", queries.params.SessionID.String(), queries.params.TurnID.String(), sessionID, turnID)
	}
	if len(messages) != 2 {
		t.Fatalf("messages length = %d, want complete two-row turn", len(messages))
	}
	for index, message := range messages {
		if message.TurnID != turnID || message.TurnPosition != 7 || message.TurnMessageSeq != int64(index+1) {
			t.Fatalf("message %d turn identity = %q/%d/%d", index, message.TurnID, message.TurnPosition, message.TurnMessageSeq)
		}
	}
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

func TestPersistPropagatesMessageAssetCreationFailure(t *testing.T) {
	queries := &runtimeSnapshotQueries{assetErr: errors.New("asset link failed")}
	svc := NewService(nil, queries)

	_, err := svc.Persist(context.Background(), PersistInput{
		BotID:     "11111111-1111-1111-1111-111111111111",
		SessionID: "22222222-2222-2222-2222-222222222222",
		Role:      "user",
		Content:   []byte(`{"type":"text","text":"hello"}`),
		Assets:    []AssetRef{{ContentHash: "sha256:asset", Role: "attachment"}},
	})
	if err == nil || !strings.Contains(err.Error(), "asset link failed") {
		t.Fatalf("Persist() error = %v, want asset creation failure", err)
	}
}

type clearHistoryQueries struct {
	dbstore.Queries
	botID     pgtype.UUID
	sessionID pgtype.UUID
}

func (q *clearHistoryQueries) ClearHistoryByBot(_ context.Context, id pgtype.UUID) error {
	q.botID = id
	return nil
}

func (q *clearHistoryQueries) ClearHistoryBySession(_ context.Context, id pgtype.UUID) error {
	q.sessionID = id
	return nil
}

func TestDeleteByScopeClearsCanonicalHistory(t *testing.T) {
	queries := &clearHistoryQueries{}
	svc := NewService(nil, queries)
	const botID = "11111111-1111-1111-1111-111111111111"
	const sessionID = "22222222-2222-2222-2222-222222222222"

	if err := svc.DeleteByBot(context.Background(), botID); err != nil {
		t.Fatalf("DeleteByBot() error = %v", err)
	}
	if err := svc.DeleteBySession(context.Background(), sessionID); err != nil {
		t.Fatalf("DeleteBySession() error = %v", err)
	}
	if queries.botID.String() != botID || queries.sessionID.String() != sessionID {
		t.Fatalf("cleared scopes = bot:%s session:%s", queries.botID.String(), queries.sessionID.String())
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

type runtimeReservationQueries struct {
	dbstore.Queries

	allocatedSession pgtype.UUID
	created          []sqlc.CreateReservedMessageParams
}

func (q *runtimeReservationQueries) AllocateSessionTurnPosition(_ context.Context, sessionID pgtype.UUID) (int64, error) {
	q.allocatedSession = sessionID
	return 12, nil
}

func (q *runtimeReservationQueries) InTx(_ context.Context, fn func(dbstore.Queries) error) error {
	return fn(q)
}

func (*runtimeReservationQueries) GetSessionByID(_ context.Context, id pgtype.UUID) (sqlc.BotSession, error) {
	return sqlc.BotSession{ID: id, SessionMode: "chat", RuntimeType: "model"}, nil
}

func (q *runtimeReservationQueries) CreateReservedMessage(_ context.Context, arg sqlc.CreateReservedMessageParams) (sqlc.CreateReservedMessageRow, error) {
	q.created = append(q.created, arg)
	return sqlc.CreateReservedMessageRow{
		ID:                      arg.MessageID,
		BotID:                   arg.BotID,
		SessionID:               arg.SessionID,
		SenderChannelIdentityID: arg.SenderChannelIdentityID,
		SenderUserID:            arg.SenderUserID,
		ExternalMessageID:       arg.ExternalMessageID,
		SourceReplyToMessageID:  arg.SourceReplyToMessageID,
		Role:                    arg.Role,
		Content:                 arg.Content,
		Metadata:                arg.Metadata,
		Usage:                   arg.Usage,
		SessionMode:             arg.SessionMode,
		RuntimeType:             arg.RuntimeType,
		EventID:                 arg.EventID,
		DisplayText:             arg.DisplayText,
		TurnID:                  arg.TurnID,
		TurnPosition:            arg.TurnPosition,
		TurnMessageSeq:          arg.TurnMessageSeq,
		CreatedAt:               pgtype.Timestamptz{Valid: true},
	}, nil
}

func TestReserveRuntimeTurnUsesSessionCounterAndStableRequestID(t *testing.T) {
	queries := &runtimeReservationQueries{}
	svc := NewService(nil, queries)
	reservation, err := svc.ReserveRuntimeTurn(
		context.Background(),
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		"33333333-3333-3333-3333-333333333333",
	)
	if err != nil {
		t.Fatalf("ReserveRuntimeTurn() error = %v", err)
	}
	if got := queries.allocatedSession.String(); got != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("allocated session = %s", got)
	}
	if reservation.TurnID == "" || reservation.TurnPosition != 12 {
		t.Fatalf("turn reservation = %#v", reservation)
	}
	request := reservation.Request
	if request.MessageID != "33333333-3333-3333-3333-333333333333" || request.Role != "user" || request.TurnID != reservation.TurnID || request.TurnPosition != 12 || request.TurnMessageSeq != 1 {
		t.Fatalf("request reservation = %#v", request)
	}
}

func TestPersistWritesSingleRuntimeReservationVerbatim(t *testing.T) {
	queries := &runtimeReservationQueries{}
	publisher := &recordingPublisher{}
	svc := NewService(nil, queries, publisher)
	input := PersistInput{
		MessageID:      "33333333-3333-3333-3333-333333333333",
		BotID:          "11111111-1111-1111-1111-111111111111",
		SessionID:      "22222222-2222-2222-2222-222222222222",
		Role:           "user",
		Content:        []byte(`{"type":"text","text":"hello"}`),
		TurnID:         "55555555-5555-5555-5555-555555555555",
		TurnPosition:   12,
		TurnMessageSeq: 1,
	}
	persisted, err := svc.Persist(context.Background(), input)
	if err != nil {
		t.Fatalf("Persist() error = %v", err)
	}
	if len(queries.created) != 1 {
		t.Fatalf("created rows = %d, want 1", len(queries.created))
	}
	created := queries.created[0]
	if created.MessageID.String() != input.MessageID || created.TurnID.String() != input.TurnID || created.TurnPosition.Int64 != 12 || created.TurnMessageSeq.Int64 != 1 || !created.TurnVisible {
		t.Fatalf("created reserved row = %#v", created)
	}
	if persisted.ID != input.MessageID || persisted.TurnID != input.TurnID || persisted.TurnPosition != 12 || persisted.TurnMessageSeq != 1 {
		t.Fatalf("persisted reserved row = %#v", persisted)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("published events = %d, want 1", len(publisher.events))
	}
}

func TestPersistRoundWritesLocalRuntimeReservationsVerbatim(t *testing.T) {
	queries := &runtimeReservationQueries{}
	publisher := &recordingPublisher{}
	svc := NewService(nil, queries, publisher)
	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
		turnID    = "55555555-5555-5555-5555-555555555555"
	)
	inputs := []PersistInput{
		{
			MessageID: "33333333-3333-3333-3333-333333333333", BotID: botID, SessionID: sessionID,
			Role: "user", Content: []byte(`{"type":"text","text":"hello"}`),
			TurnID: turnID, TurnPosition: 12, TurnMessageSeq: 1,
		},
		{
			MessageID: "44444444-4444-4444-4444-444444444444", BotID: botID, SessionID: sessionID,
			Role: "assistant", Content: []byte(`{"role":"assistant","content":"done"}`),
			TurnID: turnID, TurnPosition: 12, TurnMessageSeq: 2,
		},
	}
	persisted, handled, err := svc.PersistRound(context.Background(), inputs, RoundPersistenceOptions{})
	if err != nil {
		t.Fatalf("PersistRound() error = %v", err)
	}
	if !handled || len(persisted) != 2 || len(queries.created) != 2 {
		t.Fatalf("handled/persisted/created = %v/%d/%d", handled, len(persisted), len(queries.created))
	}
	for i, created := range queries.created {
		if created.MessageID.String() != inputs[i].MessageID || created.TurnID.String() != turnID || created.TurnPosition.Int64 != 12 || created.TurnMessageSeq.Int64 != int64(i+1) || !created.TurnVisible {
			t.Fatalf("created reserved row %d = %#v", i, created)
		}
		if persisted[i].ID != inputs[i].MessageID || persisted[i].TurnID != turnID || persisted[i].TurnPosition != 12 || persisted[i].TurnMessageSeq != int64(i+1) {
			t.Fatalf("persisted reserved row %d = %#v", i, persisted[i])
		}
	}
	if len(publisher.events) != 2 {
		t.Fatalf("published events = %d, want 2", len(publisher.events))
	}
}

func testMessageUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		panic(err)
	}
	return id
}
