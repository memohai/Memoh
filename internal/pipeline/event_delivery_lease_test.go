package pipeline

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type leaseQueries struct {
	dbstore.Queries

	mu                sync.Mutex
	now               time.Time
	claimUntil        time.Time
	renewStarted      chan struct{}
	renewRelease      chan struct{}
	renewOnce         sync.Once
	token             pgtype.UUID
	until             time.Time
	renewErr          error
	historyReady      bool
	completed         bool
	forceCompleteRows bool
	completeRows      int64
	completeErr       error
	completionReads   int
	completionReadErr error
}

func (q *leaseQueries) ClaimSessionEventDelivery(_ context.Context, arg sqlc.ClaimSessionEventDeliveryParams) (pgtype.Timestamptz, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.completed {
		return pgtype.Timestamptz{}, pgx.ErrNoRows
	}
	if q.token.Valid && q.token != arg.ClaimToken && q.until.After(q.now) {
		return pgtype.Timestamptz{}, pgx.ErrNoRows
	}
	q.token = arg.ClaimToken
	q.until = q.claimUntil
	if q.until.IsZero() {
		q.until = q.now.Add(time.Duration(arg.LeaseMs) * time.Millisecond)
	}
	return pgtype.Timestamptz{Time: q.until, Valid: true}, nil
}

func (q *leaseQueries) CompleteSessionEventDelivery(_ context.Context, arg sqlc.CompleteSessionEventDeliveryParams) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.completeErr != nil {
		return 0, q.completeErr
	}
	if q.forceCompleteRows {
		return q.completeRows, nil
	}
	if !q.historyReady || !q.token.Valid || q.token != arg.ClaimToken {
		return 0, nil
	}
	q.completed = true
	q.token = pgtype.UUID{}
	q.until = time.Time{}
	return 1, nil
}

func (q *leaseQueries) IsSessionEventDeliveryCompleted(_ context.Context, _ pgtype.UUID) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.completionReads++
	if q.completionReadErr != nil {
		return false, q.completionReadErr
	}
	return q.completed, nil
}

func (q *leaseQueries) RenewSessionEventDelivery(ctx context.Context, arg sqlc.RenewSessionEventDeliveryParams) (pgtype.Timestamptz, error) {
	if q.renewStarted != nil {
		q.renewOnce.Do(func() { close(q.renewStarted) })
	}
	if q.renewRelease != nil {
		select {
		case <-q.renewRelease:
		case <-ctx.Done():
			return pgtype.Timestamptz{}, ctx.Err()
		}
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.renewErr != nil {
		return pgtype.Timestamptz{}, q.renewErr
	}
	if !q.token.Valid || q.token != arg.ClaimToken {
		return pgtype.Timestamptz{}, pgx.ErrNoRows
	}
	q.until = q.now.Add(time.Duration(arg.LeaseMs) * time.Millisecond)
	return pgtype.Timestamptz{Time: q.until, Valid: true}, nil
}

func TestEventDeliveryLeaseReleaseCancelsInFlightRenewal(t *testing.T) {
	t.Parallel()

	now := time.Now()
	queries := &leaseQueries{
		now:          now,
		renewStarted: make(chan struct{}),
		renewRelease: make(chan struct{}),
	}
	lease, err := newTestLeaseStore(queries, time.Second, 250*time.Millisecond).ClaimEventDelivery(
		context.Background(),
		"33333333-3333-3333-3333-333333333333",
	)
	if err != nil || lease == nil {
		t.Fatalf("claim = %#v, %v", lease, err)
	}
	select {
	case <-queries.renewStarted:
	case <-time.After(time.Second):
		t.Fatal("lease renewal did not start")
	}

	released := make(chan error, 1)
	go func() { released <- lease.Release(context.Background()) }()
	select {
	case err := <-released:
		if err != nil {
			t.Fatalf("Release() error = %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		err := <-released
		t.Fatalf("Release() waited for renewal timeout: %v", err)
	}
}

func (q *leaseQueries) ReleaseSessionEventDelivery(_ context.Context, arg sqlc.ReleaseSessionEventDeliveryParams) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.token.Valid || q.token != arg.ClaimToken {
		return 0, nil
	}
	q.token = pgtype.UUID{}
	q.until = time.Time{}
	return 1, nil
}

func TestEventDeliveryLeaseExcludesCompetingStoreAndRecoversAfterExpiry(t *testing.T) {
	t.Parallel()

	queries := &leaseQueries{now: time.Now()}
	firstStore := newTestLeaseStore(queries, time.Minute, time.Hour)
	secondStore := newTestLeaseStore(queries, time.Minute, time.Hour)
	first, err := firstStore.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
	if err != nil || first == nil {
		t.Fatalf("first claim = %#v, %v", first, err)
	}
	defer first.stopKeepalive()

	competing, err := secondStore.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
	if err != nil || competing != nil {
		t.Fatalf("fresh competing claim = %#v, %v, want nil/nil", competing, err)
	}

	first.stopKeepalive()
	queries.mu.Lock()
	queries.now = queries.now.Add(time.Minute + time.Millisecond)
	queries.mu.Unlock()
	recovered, err := secondStore.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
	if err != nil || recovered == nil {
		t.Fatalf("expired claim recovery = %#v, %v", recovered, err)
	}
	if err := recovered.Release(context.Background()); err != nil {
		t.Fatalf("release recovered claim: %v", err)
	}
}

func TestEventDeliveryLeaseCancelsBoundContextWhenRenewalFails(t *testing.T) {
	t.Parallel()

	queries := &leaseQueries{now: time.Now()}
	store := newTestLeaseStore(queries, time.Minute, time.Millisecond)
	lease, err := store.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
	if err != nil || lease == nil {
		t.Fatalf("claim = %#v, %v", lease, err)
	}
	ctx, cancel := lease.Context(context.Background())
	defer cancel()
	queries.mu.Lock()
	queries.renewErr = pgx.ErrTxClosed
	queries.mu.Unlock()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("bound context was not canceled after lease renewal failure")
	}
}

func TestEventDeliveryLeaseUsesConservativeLocalDeadline(t *testing.T) {
	t.Parallel()

	now := time.Now()
	renewRelease := make(chan struct{})
	queries := &leaseQueries{
		now:          now,
		claimUntil:   now.Add(time.Hour),
		renewStarted: make(chan struct{}),
		renewRelease: renewRelease,
	}
	store := newTestLeaseStore(queries, 200*time.Millisecond, 20*time.Millisecond)
	lease, err := store.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
	if err != nil || lease == nil {
		t.Fatalf("claim = %#v, %v", lease, err)
	}
	defer lease.stopKeepalive()
	defer close(renewRelease)
	ctx, cancel := lease.Context(context.Background())
	defer cancel()
	select {
	case <-queries.renewStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("lease renewal did not start before the local deadline")
	}

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("bound context remained active beyond its conservative local deadline")
	}
}

func TestEventDeliveryLeaseUsesRenewedDatabaseExpiryForLocalDeadline(t *testing.T) {
	t.Parallel()

	now := time.Now()
	queries := &leaseQueries{
		now:        now.Add(-900 * time.Millisecond),
		claimUntil: now.Add(time.Hour),
	}
	lease, err := newTestLeaseStore(queries, time.Second, 10*time.Millisecond).ClaimEventDelivery(
		context.Background(),
		"33333333-3333-3333-3333-333333333333",
	)
	if err != nil || lease == nil {
		t.Fatalf("claim = %#v, %v", lease, err)
	}
	ctx, cancel := lease.Context(context.Background())
	defer cancel()

	select {
	case <-ctx.Done():
	case <-time.After(500 * time.Millisecond):
		lease.stopKeepalive()
		t.Fatal("bound context remained active beyond the database renewal expiry")
	}
}

func TestConservativeLocalLeaseDeadlineCapsDatabaseClockSkew(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	duration := time.Minute
	if got, want := conservativeLocalLeaseDeadline(now, now.Add(time.Hour), duration), now.Add(duration); !got.Equal(want) {
		t.Fatalf("clock-ahead deadline = %s, want %s", got, want)
	}
	if got, want := conservativeLocalLeaseDeadline(now, now.Add(30*time.Second), duration), now.Add(30*time.Second); !got.Equal(want) {
		t.Fatalf("database deadline = %s, want %s", got, want)
	}
}

func TestEventDeliveryLeaseCompletionRequiresHistoryAndBlocksReclaim(t *testing.T) {
	t.Parallel()

	queries := &leaseQueries{now: time.Now()}
	store := newTestLeaseStore(queries, time.Minute, time.Hour)
	lease, err := store.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
	if err != nil || lease == nil {
		t.Fatalf("claim = %#v, %v", lease, err)
	}
	if err := lease.Complete(context.Background()); err == nil {
		t.Fatal("Complete() error = nil without durable history")
	}
	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("release incomplete claim: %v", err)
	}

	lease, err = store.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
	if err != nil || lease == nil {
		t.Fatalf("reclaim = %#v, %v", lease, err)
	}
	queries.mu.Lock()
	queries.historyReady = true
	queries.mu.Unlock()
	if err := lease.Complete(context.Background()); err != nil {
		t.Fatalf("complete durable claim: %v", err)
	}
	redelivery, err := store.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
	if err != nil || redelivery != nil {
		t.Fatalf("completed redelivery claim = %#v, %v, want nil/nil", redelivery, err)
	}
}

func TestEventDeliveryLeaseCompleteAcceptsPriorAtomicCompletion(t *testing.T) {
	t.Parallel()

	queries := &leaseQueries{now: time.Now()}
	lease, err := newTestLeaseStore(queries, time.Minute, time.Hour).ClaimEventDelivery(
		context.Background(),
		"33333333-3333-3333-3333-333333333333",
	)
	if err != nil || lease == nil {
		t.Fatalf("claim = %#v, %v", lease, err)
	}
	queries.mu.Lock()
	queries.completed = true
	queries.token = pgtype.UUID{}
	queries.until = time.Time{}
	queries.mu.Unlock()

	if err := lease.Complete(context.Background()); err != nil {
		t.Fatalf("Complete() after atomic completion error = %v", err)
	}
	queries.mu.Lock()
	reads := queries.completionReads
	queries.mu.Unlock()
	if reads != 1 {
		t.Fatalf("completion state reads = %d, want 1", reads)
	}
}

func TestEventDeliveryLeaseCompleteFailsClosed(t *testing.T) {
	t.Parallel()

	updateErr := errors.New("complete update failed")
	readErr := errors.New("completion read failed")
	for _, tc := range []struct {
		name               string
		configure          func(*leaseQueries)
		wantErr            error
		wantCompletionRead int
	}{
		{
			name: "incomplete after zero rows",
			configure: func(q *leaseQueries) {
				q.forceCompleteRows = true
				q.completeRows = 0
			},
			wantCompletionRead: 1,
		},
		{
			name: "completion read error",
			configure: func(q *leaseQueries) {
				q.forceCompleteRows = true
				q.completionReadErr = readErr
			},
			wantErr:            readErr,
			wantCompletionRead: 1,
		},
		{
			name: "completion update error",
			configure: func(q *leaseQueries) {
				q.completeErr = updateErr
			},
			wantErr: updateErr,
		},
		{
			name: "unexpected update cardinality",
			configure: func(q *leaseQueries) {
				q.forceCompleteRows = true
				q.completeRows = 2
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			queries := &leaseQueries{now: time.Now()}
			lease, err := newTestLeaseStore(queries, time.Minute, time.Hour).ClaimEventDelivery(
				context.Background(),
				"33333333-3333-3333-3333-333333333333",
			)
			if err != nil || lease == nil {
				t.Fatalf("claim = %#v, %v", lease, err)
			}
			queries.mu.Lock()
			tc.configure(queries)
			queries.mu.Unlock()

			err = lease.Complete(context.Background())
			if err == nil {
				t.Fatal("Complete() error = nil")
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("Complete() error = %v, want %v", err, tc.wantErr)
			}
			queries.mu.Lock()
			reads := queries.completionReads
			queries.mu.Unlock()
			if reads != tc.wantCompletionRead {
				t.Fatalf("completion state reads = %d, want %d", reads, tc.wantCompletionRead)
			}
		})
	}
}

func TestEventDeliveryLeaseDoneClosesWhenReleased(t *testing.T) {
	t.Parallel()

	queries := &leaseQueries{now: time.Now()}
	lease, err := newTestLeaseStore(queries, time.Minute, time.Hour).ClaimEventDelivery(
		context.Background(),
		"33333333-3333-3333-3333-333333333333",
	)
	if err != nil || lease == nil {
		t.Fatalf("claim = %#v, %v", lease, err)
	}
	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("release: %v", err)
	}
	select {
	case <-lease.Done():
	case <-time.After(time.Second):
		t.Fatal("lease Done did not close after release")
	}
}

func newTestLeaseStore(queries dbstore.Queries, duration, renewInterval time.Duration) *EventStore {
	store := NewEventStore(nil, queries)
	store.deliveryLeaseDuration = duration
	store.deliveryRenewInterval = renewInterval
	return store
}
