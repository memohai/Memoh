package message

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	messageevent "github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/runtimefence"
)

func TestRuntimeFencedTurnResponsePersistenceRejectsStaleOwner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		persist func(context.Context, *DBService) error
	}{
		{
			name: "tail",
			persist: func(ctx context.Context, service *DBService) error {
				_, err := service.PersistTurnResponseTail(ctx, turnResponseTailInputs())
				return err
			},
		},
		{
			name: "cursor",
			persist: func(ctx context.Context, service *DBService) error {
				_, err := service.PersistTurnResponseWithCursor(ctx, turnResponseTailInputs(), testDiscussCursorUpdate())
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			queries := &staleTurnResponseQueries{}
			service := NewService(nil, queries)
			ctx := runtimefence.WithContext(context.Background(), runtimefence.Fence{
				BotID:     "11111111-1111-1111-1111-111111111111",
				SessionID: "22222222-2222-2222-2222-222222222222",
				Token:     4,
			})

			err := tt.persist(ctx, service)
			if !errors.Is(err, runtimefence.ErrStale) {
				t.Fatalf("persist error = %v, want ErrStale", err)
			}
			if queries.writeCalled {
				t.Fatal("stale runtime reached the turn response write callback")
			}
		})
	}
}

func TestPersistTurnResponseTailRollsBackOnLaterMessageFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("persist second tail message")
	queries := &turnResponseTailQueries{failAt: 2, persistErr: wantErr}
	publisher := &recordingPublisher{}
	service := NewService(nil, queries, publisher)

	_, err := service.PersistTurnResponseTail(context.Background(), turnResponseTailInputs())
	if !errors.Is(err, wantErr) {
		t.Fatalf("PersistTurnResponseTail() error = %v, want %v", err, wantErr)
	}
	if queries.txCalls != 1 {
		t.Fatalf("transaction calls = %d, want 1", queries.txCalls)
	}
	if queries.attempts != 2 {
		t.Fatalf("message attempts = %d, want 2", queries.attempts)
	}
	if len(queries.committed) != 0 {
		t.Fatalf("committed messages = %d, want 0 after rollback", len(queries.committed))
	}
	if len(publisher.events) != 0 {
		t.Fatalf("published events = %d, want 0 before commit", len(publisher.events))
	}
}

func TestPersistTurnResponseTailCommitsBeforePublishing(t *testing.T) {
	t.Parallel()

	queries := &turnResponseTailQueries{}
	publisher := &commitAwareTailPublisher{queries: queries}
	service := NewService(nil, queries, publisher)

	messages, err := service.PersistTurnResponseTail(context.Background(), turnResponseTailInputs())
	if err != nil {
		t.Fatalf("PersistTurnResponseTail() error = %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("persisted messages = %d, want 3", len(messages))
	}
	if len(queries.committed) != 3 {
		t.Fatalf("committed messages = %d, want 3", len(queries.committed))
	}
	if len(publisher.events) != 3 {
		t.Fatalf("published events = %d, want 3", len(publisher.events))
	}
	if publisher.publishedBeforeCommit {
		t.Fatal("message event published before transaction commit")
	}
}

func TestPersistTurnResponseWithCursorRollsBackMessagesWhenCursorFails(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("persist cursor")
	queries := &turnResponseTailQueries{cursorErr: wantErr}
	publisher := &recordingPublisher{}
	service := NewService(nil, queries, publisher)

	_, err := service.PersistTurnResponseWithCursor(
		context.Background(),
		turnResponseTailInputs(),
		testDiscussCursorUpdate(),
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("PersistTurnResponseWithCursor() error = %v, want %v", err, wantErr)
	}
	if len(queries.committed) != 0 || queries.committedCursor != nil {
		t.Fatalf("committed response/cursor = %d/%#v, want rollback", len(queries.committed), queries.committedCursor)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("published events = %d, want 0 before commit", len(publisher.events))
	}
}

func TestPersistTurnResponseWithCursorCommitsBeforePublishing(t *testing.T) {
	t.Parallel()

	queries := &turnResponseTailQueries{}
	publisher := &commitAwareCursorPublisher{queries: queries}
	service := NewService(nil, queries, publisher)
	inputs := turnResponseTailInputs()[:1]

	messages, err := service.PersistTurnResponseWithCursor(
		context.Background(),
		inputs,
		testDiscussCursorUpdate(),
	)
	if err != nil {
		t.Fatalf("PersistTurnResponseWithCursor() error = %v", err)
	}
	if len(messages) != 1 || len(queries.committed) != 1 || queries.committedCursor == nil {
		t.Fatalf("committed response/cursor = %d/%d/%#v, want 1/1/cursor", len(messages), len(queries.committed), queries.committedCursor)
	}
	if got := queries.committedCursor.ConsumedEventCursor; got != 123 {
		t.Fatalf("committed event cursor = %d, want 123", got)
	}
	if publisher.publishedBeforeCommit {
		t.Fatal("message event published before response and cursor committed")
	}
}

func TestPersistTurnResponseWithCursorKeepsInternalUserDecorationsInRequestTurn(t *testing.T) {
	t.Parallel()

	queries := &mixedRoundQueries{}
	service := NewService(nil, queries)
	inputs := mixedCursorInputs()

	if _, err := service.PersistTurnResponseWithCursor(context.Background(), inputs, testDiscussCursorUpdate()); err != nil {
		t.Fatalf("PersistTurnResponseWithCursor() error = %v", err)
	}
	if got := queries.committedRoles; fmt.Sprint(got) != "[assistant tool user assistant tool user assistant]" {
		t.Fatalf("committed roles = %#v", got)
	}
	if len(queries.committedUserIDs) != 0 || len(queries.committedRequestIDs) != 7 {
		t.Fatalf("new-turn user/request bindings = %d/%d, want 0/7", len(queries.committedUserIDs), len(queries.committedRequestIDs))
	}
	wantRequestID := testMessageUUID("77777777-7777-7777-7777-777777777777")
	for i, got := range queries.committedRequestIDs {
		if got != wantRequestID {
			t.Fatalf("message %d request id = %s, want original request %s", i, got.String(), wantRequestID.String())
		}
	}
	if queries.committedCursor == nil || queries.committedCursor.ConsumedEventCursor != 123 {
		t.Fatalf("committed cursor = %#v, want 123", queries.committedCursor)
	}
}

func TestPersistRoundRebindsRealUserBoundaries(t *testing.T) {
	t.Parallel()

	queries := &mixedRoundQueries{}
	service := NewService(nil, queries)

	if _, handled, err := service.PersistRound(context.Background(), mixedRoundInputs(), RoundPersistenceOptions{}); err != nil {
		t.Fatalf("PersistRound() error = %v", err)
	} else if !handled {
		t.Fatal("PersistRound() handled = false, want true")
	}
	if len(queries.committedUserIDs) != 2 || len(queries.committedRequestIDs) != 2 {
		t.Fatalf("committed user/request ids = %d/%d, want 2/2", len(queries.committedUserIDs), len(queries.committedRequestIDs))
	}
	for i, want := range queries.committedUserIDs {
		if queries.committedRequestIDs[i] != want {
			t.Fatalf("assistant %d request id = %s, want user %s", i, queries.committedRequestIDs[i].String(), want.String())
		}
	}
}

func TestPersistRoundRollsBackMixedUserBoundaries(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("persist final response")
	queries := &mixedRoundQueries{failAt: 4, persistErr: wantErr}
	publisher := &recordingPublisher{}
	service := NewService(nil, queries, publisher)

	_, handled, err := service.PersistRound(context.Background(), mixedRoundInputs(), RoundPersistenceOptions{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("PersistRound() error = %v, want %v", err, wantErr)
	}
	if !handled {
		t.Fatal("PersistRound() handled = false, want true")
	}
	if queries.txCalls != 1 {
		t.Fatalf("transaction calls = %d, want 1", queries.txCalls)
	}
	if queries.attempts != 4 {
		t.Fatalf("message attempts = %d, want 4", queries.attempts)
	}
	if len(queries.committedRoles) != 0 {
		t.Fatalf("committed roles = %#v, want none after rollback", queries.committedRoles)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("published events = %d, want 0 before commit", len(publisher.events))
	}
}

func TestPersistRoundDoesNotPublishSkippedMessages(t *testing.T) {
	t.Parallel()

	inputs := mixedRoundInputs()
	for i := range inputs {
		inputs[i].SkipHistoryTurn = true
	}
	queries := &mixedRoundQueries{}
	publisher := &recordingPublisher{}
	service := NewService(nil, queries, publisher)

	messages, handled, err := service.PersistRound(context.Background(), inputs, RoundPersistenceOptions{})
	if err != nil {
		t.Fatalf("PersistRound() error = %v", err)
	}
	if !handled {
		t.Fatal("PersistRound() handled = false, want true")
	}
	if len(messages) != len(inputs) || len(queries.committedRoles) != len(inputs) {
		t.Fatalf("persisted messages = %d/%d, want %d/%d", len(messages), len(queries.committedRoles), len(inputs), len(inputs))
	}
	if len(publisher.events) != 0 {
		t.Fatalf("published events = %d, want 0 before replacement turn is linked", len(publisher.events))
	}
}

type commitAwareTailPublisher struct {
	queries               *turnResponseTailQueries
	events                []messageevent.Event
	publishedBeforeCommit bool
}

type commitAwareCursorPublisher struct {
	queries               *turnResponseTailQueries
	publishedBeforeCommit bool
}

func (p *commitAwareCursorPublisher) Publish(messageevent.Event) {
	if len(p.queries.committed) == 0 || p.queries.committedCursor == nil {
		p.publishedBeforeCommit = true
	}
}

func (p *commitAwareTailPublisher) Publish(event messageevent.Event) {
	if len(p.queries.committed) != len(turnResponseTailInputs()) {
		p.publishedBeforeCommit = true
	}
	p.events = append(p.events, event)
}

type turnResponseTailQueries struct {
	dbstore.Queries
	failAt          int
	persistErr      error
	cursorErr       error
	txCalls         int
	attempts        int
	committed       []sqlc.CreateMessageInHistoryTurnByRequestAndBindParams
	committedCursor *sqlc.UpsertSessionDiscussCursorParams
}

type staleTurnResponseQueries struct {
	dbstore.Queries
	writeCalled bool
}

func (*staleTurnResponseQueries) SupportsTransactions() bool { return true }

func (q *staleTurnResponseQueries) InTx(_ context.Context, fn func(dbstore.Queries) error) error {
	return fn(q)
}

func (*staleTurnResponseQueries) LockBotForSessionWrite(_ context.Context, id pgtype.UUID) (pgtype.UUID, error) {
	return id, nil
}

func (*staleTurnResponseQueries) LockSessionRuntimeFence(context.Context, sqlc.LockSessionRuntimeFenceParams) (int64, error) {
	return 0, pgx.ErrNoRows
}

func (q *staleTurnResponseQueries) CreateMessageInHistoryTurnByRequestAndBind(context.Context, sqlc.CreateMessageInHistoryTurnByRequestAndBindParams) (sqlc.CreateMessageInHistoryTurnByRequestAndBindRow, error) {
	q.writeCalled = true
	return sqlc.CreateMessageInHistoryTurnByRequestAndBindRow{}, nil
}

func (*turnResponseTailQueries) SupportsTransactions() bool {
	return true
}

func (q *turnResponseTailQueries) InTx(_ context.Context, fn func(dbstore.Queries) error) error {
	q.txCalls++
	tx := &turnResponseTailTxQueries{
		failAt:     q.failAt,
		persistErr: q.persistErr,
		cursorErr:  q.cursorErr,
	}
	err := fn(tx)
	q.attempts = tx.attempts
	if err != nil {
		return err
	}
	q.committed = append(q.committed, tx.staged...)
	q.committedCursor = tx.stagedCursor
	return nil
}

type turnResponseTailTxQueries struct {
	dbstore.Queries
	failAt       int
	persistErr   error
	cursorErr    error
	attempts     int
	staged       []sqlc.CreateMessageInHistoryTurnByRequestAndBindParams
	stagedCursor *sqlc.UpsertSessionDiscussCursorParams
}

type mixedRoundQueries struct {
	dbstore.Queries
	failAt              int
	persistErr          error
	txCalls             int
	attempts            int
	committedRoles      []string
	committedUserIDs    []pgtype.UUID
	committedRequestIDs []pgtype.UUID
	committedCursor     *sqlc.UpsertSessionDiscussCursorParams
}

func (*mixedRoundQueries) SupportsTransactions() bool { return true }

func (q *mixedRoundQueries) InTx(_ context.Context, fn func(dbstore.Queries) error) error {
	q.txCalls++
	tx := &mixedRoundTxQueries{failAt: q.failAt, persistErr: q.persistErr}
	err := fn(tx)
	q.attempts = tx.attempts
	if err != nil {
		return err
	}
	q.committedRoles = append(q.committedRoles, tx.stagedRoles...)
	q.committedUserIDs = append(q.committedUserIDs, tx.stagedUserIDs...)
	q.committedRequestIDs = append(q.committedRequestIDs, tx.stagedRequestIDs...)
	q.committedCursor = tx.stagedCursor
	return nil
}

type mixedRoundTxQueries struct {
	dbstore.Queries
	failAt           int
	persistErr       error
	attempts         int
	stagedRoles      []string
	stagedUserIDs    []pgtype.UUID
	stagedRequestIDs []pgtype.UUID
	stagedCursor     *sqlc.UpsertSessionDiscussCursorParams
}

func (q *mixedRoundTxQueries) CreateMessage(
	_ context.Context,
	arg sqlc.CreateMessageParams,
) (sqlc.CreateMessageRow, error) {
	q.attempts++
	if q.attempts == q.failAt {
		return sqlc.CreateMessageRow{}, q.persistErr
	}
	q.stagedRoles = append(q.stagedRoles, arg.Role)
	return sqlc.CreateMessageRow{
		ID:          testMessageUUID(mixedRoundMessageID(q.attempts)),
		BotID:       arg.BotID,
		SessionID:   arg.SessionID,
		Role:        arg.Role,
		Content:     arg.Content,
		Metadata:    arg.Metadata,
		Usage:       arg.Usage,
		SessionMode: arg.SessionMode,
		RuntimeType: arg.RuntimeType,
		DisplayText: arg.DisplayText,
		CreatedAt:   pgtype.Timestamptz{Time: time.Unix(int64(q.attempts), 0), Valid: true},
	}, nil
}

func (q *mixedRoundTxQueries) CreateMessageWithHistoryTurn(
	_ context.Context,
	arg sqlc.CreateMessageWithHistoryTurnParams,
) (sqlc.CreateMessageWithHistoryTurnRow, error) {
	q.attempts++
	if q.attempts == q.failAt {
		return sqlc.CreateMessageWithHistoryTurnRow{}, q.persistErr
	}
	q.stagedRoles = append(q.stagedRoles, arg.Role)
	q.stagedUserIDs = append(q.stagedUserIDs, arg.MessageID)
	return sqlc.CreateMessageWithHistoryTurnRow{
		ID:        arg.MessageID,
		CreatedAt: pgtype.Timestamptz{Time: time.Unix(int64(q.attempts), 0), Valid: true},
	}, nil
}

func (q *mixedRoundTxQueries) CreateMessageInHistoryTurnByRequestAndBind(
	_ context.Context,
	arg sqlc.CreateMessageInHistoryTurnByRequestAndBindParams,
) (sqlc.CreateMessageInHistoryTurnByRequestAndBindRow, error) {
	q.attempts++
	if q.attempts == q.failAt {
		return sqlc.CreateMessageInHistoryTurnByRequestAndBindRow{}, q.persistErr
	}
	q.stagedRoles = append(q.stagedRoles, arg.Role)
	q.stagedRequestIDs = append(q.stagedRequestIDs, arg.RequestMessageID)
	return sqlc.CreateMessageInHistoryTurnByRequestAndBindRow{
		ID:        testMessageUUID(mixedRoundMessageID(q.attempts)),
		CreatedAt: pgtype.Timestamptz{Time: time.Unix(int64(q.attempts), 0), Valid: true},
	}, nil
}

func (q *mixedRoundTxQueries) UpsertSessionDiscussCursor(
	_ context.Context,
	arg sqlc.UpsertSessionDiscussCursorParams,
) (sqlc.BotSessionDiscussCursor, error) {
	q.stagedCursor = &arg
	return sqlc.BotSessionDiscussCursor{}, nil
}

func mixedRoundMessageID(n int) string {
	return fmt.Sprintf("88888888-8888-8888-8888-%012d", n)
}

func mixedRoundInputs() []PersistInput {
	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
	)
	return []PersistInput{
		{BotID: botID, SessionID: sessionID, Role: "user", Content: []byte(`{"role":"user","content":"first"}`), SessionMode: "chat", RuntimeType: "model"},
		{BotID: botID, SessionID: sessionID, Role: "assistant", Content: []byte(`{"role":"assistant","content":"first answer"}`), SessionMode: "chat", RuntimeType: "model"},
		{BotID: botID, SessionID: sessionID, Role: "user", Content: []byte(`{"role":"user","content":"injected"}`), SessionMode: "chat", RuntimeType: "model"},
		{BotID: botID, SessionID: sessionID, Role: "assistant", Content: []byte(`{"role":"assistant","content":"final"}`), SessionMode: "chat", RuntimeType: "model"},
	}
}

func mixedCursorInputs() []PersistInput {
	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
		requestID = "77777777-7777-7777-7777-777777777777"
	)
	return []PersistInput{
		{BotID: botID, SessionID: sessionID, Role: "assistant", Content: []byte(`{"role":"assistant","content":"calling read_media"}`), SessionMode: "discuss", RuntimeType: "model", TurnRequestMessageID: requestID},
		{BotID: botID, SessionID: sessionID, Role: "tool", Content: []byte(`{"role":"tool","content":"read_media closed"}`), SessionMode: "discuss", RuntimeType: "model", TurnRequestMessageID: requestID},
		{BotID: botID, SessionID: sessionID, Role: "user", Content: []byte(`{"role":"user","content":[{"type":"image"}]}`), SessionMode: "discuss", RuntimeType: "model", TurnRequestMessageID: requestID, ContinueHistoryTurn: true},
		{BotID: botID, SessionID: sessionID, Role: "assistant", Content: []byte(`{"role":"assistant","content":"calling read_media again"}`), SessionMode: "discuss", RuntimeType: "model", TurnRequestMessageID: requestID},
		{BotID: botID, SessionID: sessionID, Role: "tool", Content: []byte(`{"role":"tool","content":"second read_media closed"}`), SessionMode: "discuss", RuntimeType: "model", TurnRequestMessageID: requestID},
		{BotID: botID, SessionID: sessionID, Role: "user", Content: []byte(`{"role":"user","content":[{"type":"image"}]}`), SessionMode: "discuss", RuntimeType: "model", TurnRequestMessageID: requestID, ContinueHistoryTurn: true},
		{BotID: botID, SessionID: sessionID, Role: "assistant", Content: []byte(`{"role":"assistant","content":"done"}`), SessionMode: "discuss", RuntimeType: "model", TurnRequestMessageID: requestID},
	}
}

func (*turnResponseTailTxQueries) CreateMessageWithHistoryTurn(
	context.Context,
	sqlc.CreateMessageWithHistoryTurnParams,
) (sqlc.CreateMessageWithHistoryTurnRow, error) {
	return sqlc.CreateMessageWithHistoryTurnRow{}, errors.New("unexpected user message")
}

func (q *turnResponseTailTxQueries) CreateMessageInHistoryTurnByRequestAndBind(
	_ context.Context,
	arg sqlc.CreateMessageInHistoryTurnByRequestAndBindParams,
) (sqlc.CreateMessageInHistoryTurnByRequestAndBindRow, error) {
	q.attempts++
	if q.attempts == q.failAt {
		return sqlc.CreateMessageInHistoryTurnByRequestAndBindRow{}, q.persistErr
	}
	q.staged = append(q.staged, arg)
	return sqlc.CreateMessageInHistoryTurnByRequestAndBindRow{
		ID:        testMessageUUID("88888888-8888-8888-8888-888888888888"),
		CreatedAt: pgtype.Timestamptz{Time: time.Unix(int64(q.attempts), 0), Valid: true},
	}, nil
}

func (q *turnResponseTailTxQueries) UpsertSessionDiscussCursor(
	_ context.Context,
	arg sqlc.UpsertSessionDiscussCursorParams,
) (sqlc.BotSessionDiscussCursor, error) {
	if q.cursorErr != nil {
		return sqlc.BotSessionDiscussCursor{}, q.cursorErr
	}
	q.stagedCursor = &arg
	return sqlc.BotSessionDiscussCursor{}, nil
}

func testDiscussCursorUpdate() DiscussCursorUpdate {
	return DiscussCursorUpdate{
		SessionID:           "22222222-2222-2222-2222-222222222222",
		ScopeKey:            "route:33333333-3333-3333-3333-333333333333",
		RouteID:             "33333333-3333-3333-3333-333333333333",
		Source:              "telegram",
		ConsumedCursor:      100,
		ConsumedEventCursor: 123,
	}
}

func turnResponseTailInputs() []PersistInput {
	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
		requestID = "77777777-7777-7777-7777-777777777777"
	)
	return []PersistInput{
		{
			BotID:                botID,
			SessionID:            sessionID,
			Role:                 "assistant",
			Content:              []byte(`{"role":"assistant","content":"calling tool"}`),
			SessionMode:          "chat",
			RuntimeType:          "model",
			TurnRequestMessageID: requestID,
		},
		{
			BotID:                botID,
			SessionID:            sessionID,
			Role:                 "tool",
			Content:              []byte(`{"role":"tool","content":"ok"}`),
			SessionMode:          "chat",
			RuntimeType:          "model",
			TurnRequestMessageID: requestID,
		},
		{
			BotID:                botID,
			SessionID:            sessionID,
			Role:                 "assistant",
			Content:              []byte(`{"role":"assistant","content":"done"}`),
			SessionMode:          "chat",
			RuntimeType:          "model",
			TurnRequestMessageID: requestID,
		},
	}
}
