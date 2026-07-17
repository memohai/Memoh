package message

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

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
