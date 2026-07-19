package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestEventDeliveryLeaseStartsLocalDeadlineAfterClaimReturns(t *testing.T) {
	t.Parallel()

	claimRelease := make(chan struct{})
	queries := &leaseQueries{
		now:             time.Now(),
		claimStarted:    make(chan struct{}),
		claimRelease:    claimRelease,
		claimFromReturn: 200 * time.Millisecond,
	}
	result := make(chan struct {
		lease *EventDeliveryLease
		err   error
	}, 1)
	go func() {
		lease, err := newTestLeaseStore(queries, 200*time.Millisecond, time.Hour).ClaimEventDelivery(
			context.Background(),
			"33333333-3333-3333-3333-333333333333",
		)
		result <- struct {
			lease *EventDeliveryLease
			err   error
		}{lease: lease, err: err}
	}()
	select {
	case <-queries.claimStarted:
	case <-time.After(time.Second):
		t.Fatal("claim did not start")
	}
	time.Sleep(250 * time.Millisecond)
	close(claimRelease)

	select {
	case got := <-result:
		if got.err != nil || got.lease == nil {
			t.Fatalf("claim after query wait = %#v, %v", got.lease, got.err)
		}
		if !got.lease.Active() {
			t.Fatal("claim returned an inactive lease")
		}
		got.lease.stopKeepalive()
	case <-time.After(time.Second):
		t.Fatal("claim did not return")
	}
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
