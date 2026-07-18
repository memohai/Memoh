package message

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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

func TestPersistRoundLocksDeliveryClaimsAfterWritingResponse(t *testing.T) {
	t.Parallel()

	queries := &turnResponseDeliveryQueries{}
	service := NewService(nil, queries)
	inputs := turnResponseTailInputs()[:1]

	messages, handled, err := service.PersistRound(context.Background(), inputs, RoundPersistenceOptions{
		DeliveryClaims: testDeliveryClaims(),
	})
	if err != nil {
		t.Fatalf("PersistRound() error = %v", err)
	}
	if !handled {
		t.Fatal("PersistRound() handled = false")
	}
	if len(messages) != 1 || len(queries.committed) != 1 {
		t.Fatalf("persisted messages = %d/%d, want 1/1", len(messages), len(queries.committed))
	}
	if len(queries.committedLocks) != 2 {
		t.Fatalf("locked delivery claims = %#v, want 2", queries.committedLocks)
	}
	if got := strings.Join(queries.committedOperations, " "); got != "message claim claim" {
		t.Fatalf("transaction operations = %q, want message before claims", got)
	}
	for i, claim := range testDeliveryClaims() {
		locked := queries.committedLocks[i]
		if locked.EventID.String() != claim.EventID || locked.ClaimToken.String() != claim.ClaimToken {
			t.Fatalf("locked delivery claim %d = %#v, want %#v", i, locked, claim)
		}
	}
}

func TestPersistRoundRollsBackWhenDeliveryClaimIsStale(t *testing.T) {
	t.Parallel()

	queries := &turnResponseDeliveryQueries{lockRows: []bool{true, false}}
	service := NewService(nil, queries)
	_, handled, err := service.PersistRound(context.Background(), turnResponseTailInputs()[:1], RoundPersistenceOptions{
		DeliveryClaims: testDeliveryClaims(),
	})
	if err == nil {
		t.Fatal("PersistRound() error = nil for stale delivery claim")
	}
	if !handled {
		t.Fatal("PersistRound() handled = false")
	}
	if len(queries.committed) != 0 || len(queries.committedLocks) != 0 {
		t.Fatalf("committed responses/claims = %d/%d, want rollback", len(queries.committed), len(queries.committedLocks))
	}
}

func TestPersistRoundRejectsInvalidClaimBackedRounds(t *testing.T) {
	t.Parallel()

	valid := testDeliveryClaims()[0]
	base := turnResponseTailInputs()[0]
	for _, tc := range []struct {
		name   string
		input  PersistInput
		claims []DeliveryClaim
		secret string
	}{
		{name: "no response", input: func() PersistInput {
			input := base
			input.Role = "user"
			return input
		}(), claims: []DeliveryClaim{valid}},
		{name: "skip history", input: func() PersistInput {
			input := base
			input.SkipHistoryTurn = true
			return input
		}(), claims: []DeliveryClaim{valid}},
		{name: "missing request boundary", input: func() PersistInput {
			input := base
			input.TurnRequestMessageID = ""
			return input
		}(), claims: []DeliveryClaim{valid}},
		{name: "invalid token", input: base, claims: []DeliveryClaim{{EventID: valid.EventID, ClaimToken: "sensitive-token"}}, secret: "sensitive-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			queries := &turnResponseDeliveryQueries{}
			service := NewService(nil, queries)
			_, handled, err := service.PersistRound(context.Background(), []PersistInput{tc.input}, RoundPersistenceOptions{
				DeliveryClaims: tc.claims,
			})
			if err == nil {
				t.Fatal("PersistRound() error = nil")
			}
			if !handled {
				t.Fatal("PersistRound() handled = false")
			}
			if tc.secret != "" && strings.Contains(err.Error(), tc.secret) {
				t.Fatalf("validation error leaked claim token: %v", err)
			}
			if queries.txCalls != 0 {
				t.Fatalf("transaction calls = %d, want validation before transaction", queries.txCalls)
			}
		})
	}
}

func TestPersistTurnResponseWithCursorCompletesEveryDeliveryClaim(t *testing.T) {
	t.Parallel()

	queries := &turnResponseDeliveryQueries{}
	service := NewService(nil, queries)
	update := testDiscussCursorUpdate()
	update.DeliveryClaims = testDeliveryClaims()

	if _, err := service.PersistTurnResponseWithCursor(context.Background(), turnResponseTailInputs()[:1], update); err != nil {
		t.Fatalf("PersistTurnResponseWithCursor() error = %v", err)
	}
	if len(queries.committedClaims) != 2 {
		t.Fatalf("committed delivery claims = %#v, want 2", queries.committedClaims)
	}
	for i, claim := range update.DeliveryClaims {
		if queries.committedClaims[i].EventID.String() != claim.EventID ||
			queries.committedClaims[i].ClaimToken.String() != claim.ClaimToken {
			t.Fatalf("committed delivery claim %d = %#v, want %#v", i, queries.committedClaims[i], claim)
		}
		if got := queries.committedClaims[i].ResponseMessageID.String(); got != "88888888-8888-8888-8888-888888888888" {
			t.Fatalf("delivery claim %d response evidence = %s", i, got)
		}
	}
}

func TestPersistTurnResponseWithCursorRollsBackWhenLaterDeliveryClaimIsStale(t *testing.T) {
	t.Parallel()

	queries := &turnResponseDeliveryQueries{completeRows: []int64{1, 0}}
	publisher := &recordingPublisher{}
	service := NewService(nil, queries, publisher)
	update := testDiscussCursorUpdate()
	update.DeliveryClaims = testDeliveryClaims()

	if _, err := service.PersistTurnResponseWithCursor(context.Background(), turnResponseTailInputs()[:1], update); err == nil {
		t.Fatal("PersistTurnResponseWithCursor() error = nil for stale delivery claim")
	}
	if len(queries.committed) != 0 || queries.committedCursor != nil || len(queries.committedClaims) != 0 {
		t.Fatalf("committed response/cursor/claims = %d/%#v/%d, want rollback", len(queries.committed), queries.committedCursor, len(queries.committedClaims))
	}
	if len(publisher.events) != 0 {
		t.Fatalf("published events = %d, want 0 before commit", len(publisher.events))
	}
}

func TestPersistTurnResponseWithCursorRejectsInvalidDeliveryClaims(t *testing.T) {
	t.Parallel()

	valid := testDeliveryClaims()
	for _, tc := range []struct {
		name   string
		claims []DeliveryClaim
		secret string
	}{
		{name: "invalid event id", claims: []DeliveryClaim{{EventID: "bad", ClaimToken: valid[0].ClaimToken}}},
		{name: "invalid claim token", claims: []DeliveryClaim{{EventID: valid[0].EventID, ClaimToken: "sensitive-token"}}, secret: "sensitive-token"},
		{name: "duplicate event", claims: []DeliveryClaim{valid[0], valid[0]}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			queries := &turnResponseDeliveryQueries{}
			service := NewService(nil, queries)
			update := testDiscussCursorUpdate()
			update.DeliveryClaims = tc.claims

			_, err := service.PersistTurnResponseWithCursor(context.Background(), turnResponseTailInputs()[:1], update)
			if err == nil {
				t.Fatal("PersistTurnResponseWithCursor() error = nil")
			}
			if tc.secret != "" && strings.Contains(err.Error(), tc.secret) {
				t.Fatalf("validation error leaked claim token: %v", err)
			}
			if queries.txCalls != 0 {
				t.Fatalf("transaction calls = %d, want validation before transaction", queries.txCalls)
			}
		})
	}
}

func TestPersistTurnResponseWithCursorRejectsClaimsWithoutResponseEvidence(t *testing.T) {
	t.Parallel()

	queries := &turnResponseDeliveryQueries{}
	service := NewService(nil, queries)
	input := turnResponseTailInputs()[0]
	input.Role = "user"
	input.Content = []byte(`{"role":"user","content":"internal decoration"}`)
	input.ContinueHistoryTurn = true
	update := testDiscussCursorUpdate()
	update.DeliveryClaims = testDeliveryClaims()

	if _, err := service.PersistTurnResponseWithCursor(context.Background(), []PersistInput{input}, update); err == nil {
		t.Fatal("PersistTurnResponseWithCursor() error = nil without response evidence")
	}
	if queries.txCalls != 0 {
		t.Fatalf("transaction calls = %d, want validation before transaction", queries.txCalls)
	}
}

func TestListVisibleTurnResponsesByRequestUsesExactTurnAnchor(t *testing.T) {
	const (
		sessionID = "22222222-2222-2222-2222-222222222222"
		requestID = "44444444-4444-4444-4444-444444444444"
	)
	queries := &turnResponseReplayQueries{rows: []sqlc.ListVisibleTurnResponsesByRequestRow{
		{ID: testMessageUUID("55555555-5555-5555-5555-555555555555"), Role: "assistant", Content: []byte(`{"role":"assistant","content":"calling"}`), CreatedAt: pgtype.Timestamptz{Time: time.Unix(1, 0), Valid: true}},
		{ID: testMessageUUID("66666666-6666-6666-6666-666666666666"), Role: "tool", Content: []byte(`{"role":"tool","content":"done"}`), CreatedAt: pgtype.Timestamptz{Time: time.Unix(2, 0), Valid: true}},
	}}
	service := NewService(nil, queries)

	messages, err := service.ListVisibleTurnResponsesByRequest(context.Background(), sessionID, requestID)
	if err != nil {
		t.Fatalf("ListVisibleTurnResponsesByRequest() error = %v", err)
	}
	if queries.arg.SessionID.String() != sessionID || queries.arg.RequestMessageID.String() != requestID {
		t.Fatalf("query anchor = %s/%s, want %s/%s", queries.arg.SessionID.String(), queries.arg.RequestMessageID.String(), sessionID, requestID)
	}
	if len(messages) != 2 || messages[0].Role != "assistant" || messages[1].Role != "tool" {
		t.Fatalf("messages = %#v, want ordered assistant/tool tail", messages)
	}
}

func TestTurnResponseReplayQueriesStartAfterArbitraryRequest(t *testing.T) {
	messagesSQL, err := os.ReadFile("../../db/postgres/queries/messages.sql")
	if err != nil {
		t.Fatalf("read message queries: %v", err)
	}
	responseQuery := namedQuery(t, string(messagesSQL), "ListVisibleTurnResponsesByRequest")
	for _, required := range []string{
		"request.turn_id",
		"request.turn_message_seq",
		"MIN(next_request.turn_message_seq)",
		"next_request.event_id IS NOT NULL",
		"response.turn_message_seq > target.turn_message_seq",
		"response.turn_message_seq < target.next_event_user_seq",
	} {
		if !strings.Contains(responseQuery, required) {
			t.Fatalf("ListVisibleTurnResponsesByRequest is missing %q", required)
		}
	}
	if strings.Contains(responseQuery, "request.turn_message_seq = 1") {
		t.Fatal("ListVisibleTurnResponsesByRequest only accepts a turn-leading request")
	}

	eventsSQL, err := os.ReadFile("../../db/postgres/queries/session_events.sql")
	if err != nil {
		t.Fatalf("read session event queries: %v", err)
	}
	deliveryQuery := namedQuery(t, string(eventsSQL), "GetSessionEventDeliveryState")
	if count := strings.Count(deliveryQuery, "response.turn_message_seq > history.turn_message_seq"); count != 2 {
		t.Fatalf("GetSessionEventDeliveryState strict response boundaries = %d, want 2", count)
	}
	for _, required := range []string{
		"AS replay_response_persisted",
		"next_request.event_id IS NOT NULL",
		"next_request.turn_message_seq > visible_history.turn_message_seq",
		"next_request.turn_message_seq < response.turn_message_seq",
	} {
		if !strings.Contains(deliveryQuery, required) {
			t.Fatalf("GetSessionEventDeliveryState is missing %q", required)
		}
	}
	completionQuery := namedQuery(t, string(eventsSQL), "CompleteSessionEventDelivery")
	if count := strings.Count(completionQuery, "response.turn_message_seq > history.turn_message_seq"); count != 1 {
		t.Fatalf("CompleteSessionEventDelivery response lower bounds = %d, want 1", count)
	}
	if strings.Contains(completionQuery, "next_request") {
		t.Fatal("CompleteSessionEventDelivery incorrectly requires strict replay ownership")
	}
}

func namedQuery(t *testing.T, source, name string) string {
	t.Helper()
	startMarker := "-- name: " + name + " "
	start := strings.Index(source, startMarker)
	if start < 0 {
		t.Fatalf("query %s not found", name)
	}
	rest := source[start+len(startMarker):]
	if end := strings.Index(rest, "\n-- name: "); end >= 0 {
		rest = rest[:end]
	}
	return rest
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

type turnResponseDeliveryQueries struct {
	dbstore.Queries
	txCalls             int
	completeRows        []int64
	lockRows            []bool
	committed           []sqlc.CreateMessageInHistoryTurnByRequestAndBindParams
	committedCursor     *sqlc.UpsertSessionDiscussCursorParams
	committedClaims     []sqlc.CompleteSessionEventDeliveryWithResponseParams
	committedLocks      []sqlc.LockSessionEventDeliveryClaimParams
	committedOperations []string
}

func (*turnResponseDeliveryQueries) SupportsTransactions() bool { return true }

func (q *turnResponseDeliveryQueries) InTx(_ context.Context, fn func(dbstore.Queries) error) error {
	q.txCalls++
	base := &turnResponseTailTxQueries{}
	tx := &turnResponseDeliveryTxQueries{
		turnResponseTailTxQueries: base,
		completeRows:              q.completeRows,
		lockRows:                  q.lockRows,
	}
	if err := fn(tx); err != nil {
		return err
	}
	q.committed = append(q.committed, base.staged...)
	q.committedCursor = base.stagedCursor
	q.committedClaims = append(q.committedClaims, tx.stagedClaims...)
	q.committedLocks = append(q.committedLocks, tx.stagedLocks...)
	q.committedOperations = append(q.committedOperations, tx.operations...)
	return nil
}

type turnResponseDeliveryTxQueries struct {
	*turnResponseTailTxQueries
	completeRows  []int64
	completeCalls int
	lockRows      []bool
	lockCalls     int
	stagedClaims  []sqlc.CompleteSessionEventDeliveryWithResponseParams
	stagedLocks   []sqlc.LockSessionEventDeliveryClaimParams
	operations    []string
}

func (q *turnResponseDeliveryTxQueries) CreateMessageInHistoryTurnByRequestAndBind(
	ctx context.Context,
	arg sqlc.CreateMessageInHistoryTurnByRequestAndBindParams,
) (sqlc.CreateMessageInHistoryTurnByRequestAndBindRow, error) {
	row, err := q.turnResponseTailTxQueries.CreateMessageInHistoryTurnByRequestAndBind(ctx, arg)
	if err == nil {
		q.operations = append(q.operations, "message")
	}
	return row, err
}

func (q *turnResponseDeliveryTxQueries) LockSessionEventDeliveryClaim(
	_ context.Context,
	arg sqlc.LockSessionEventDeliveryClaimParams,
) (bool, error) {
	locked := true
	if q.lockCalls < len(q.lockRows) {
		locked = q.lockRows[q.lockCalls]
	}
	q.lockCalls++
	if !locked {
		return false, pgx.ErrNoRows
	}
	q.stagedLocks = append(q.stagedLocks, arg)
	q.operations = append(q.operations, "claim")
	return true, nil
}

func (q *turnResponseDeliveryTxQueries) CompleteSessionEventDeliveryWithResponse(
	_ context.Context,
	arg sqlc.CompleteSessionEventDeliveryWithResponseParams,
) (int64, error) {
	rows := int64(1)
	if q.completeCalls < len(q.completeRows) {
		rows = q.completeRows[q.completeCalls]
	}
	q.completeCalls++
	if rows == 1 {
		q.stagedClaims = append(q.stagedClaims, arg)
	}
	return rows, nil
}

func testDeliveryClaims() []DeliveryClaim {
	return []DeliveryClaim{
		{EventID: "44444444-4444-4444-4444-444444444444", ClaimToken: "55555555-5555-5555-5555-555555555555"},
		{EventID: "66666666-6666-6666-6666-666666666666", ClaimToken: "99999999-9999-9999-9999-999999999999"},
	}
}

type turnResponseReplayQueries struct {
	dbstore.Queries
	arg  sqlc.ListVisibleTurnResponsesByRequestParams
	rows []sqlc.ListVisibleTurnResponsesByRequestRow
}

func (q *turnResponseReplayQueries) ListVisibleTurnResponsesByRequest(
	_ context.Context,
	arg sqlc.ListVisibleTurnResponsesByRequestParams,
) ([]sqlc.ListVisibleTurnResponsesByRequestRow, error) {
	q.arg = arg
	return q.rows, nil
}
