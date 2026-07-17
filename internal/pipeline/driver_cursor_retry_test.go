package pipeline

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

type retryDiscussCursorStore struct {
	failures int
	calls    int
	stored   DiscussCursorPosition
}

func (*retryDiscussCursorStore) GetDiscussCursor(context.Context, string, string) (DiscussCursorPosition, error) {
	return DiscussCursorPosition{}, nil
}

func (s *retryDiscussCursorStore) UpsertDiscussCursor(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ string,
	position DiscussCursorPosition,
) error {
	s.calls++
	if s.failures > 0 {
		s.failures--
		return errors.New("cursor store unavailable")
	}
	s.stored = position
	return nil
}

func TestDiscussCursorPersistenceFailureRetriesBeforeAdvancingMemoryCursor(t *testing.T) {
	t.Parallel()

	store := &retryDiscussCursorStore{failures: 2}
	resolver := &fakeRunConfigResolver{}
	driver := NewDiscussDriver(DiscussDriverDeps{Resolver: resolver, CursorStore: store})
	cfg := DiscussSessionConfig{BotID: "bot", SessionID: "session", RouteID: "route"}
	sess := &discussSession{config: cfg}
	position := DiscussCursorPosition{SourceCursor: 90, EventCursor: 100}

	driver.advanceDiscussCursor(context.Background(), sess, cfg, position, driver.logger)
	if sess.lastProcessedCursor != 0 || sess.pendingCursor == nil {
		t.Fatalf("failed durable advance = cursor:%d pending:%#v", sess.lastProcessedCursor, sess.pendingCursor)
	}

	if got := driver.retryPendingDiscussCursor(context.Background(), sess, driver.logger); got != pendingDiscussCursorRetryPending {
		t.Fatalf("first retry = %v, want pending", got)
	}
	if got := driver.retryPendingDiscussCursor(context.Background(), sess, driver.logger); got != pendingDiscussCursorRetryCommitted {
		t.Fatalf("second retry = %v, want committed", got)
	}
	if sess.lastProcessedCursor != 100 || sess.pendingCursor != nil || store.stored != position || store.calls != 3 {
		t.Fatalf("retry result = cursor:%d pending:%#v stored:%#v calls:%d", sess.lastProcessedCursor, sess.pendingCursor, store.stored, store.calls)
	}
}

func TestDiscussCursorRetryAbandonsLostLeaseWithoutSwallowingFreshDelivery(t *testing.T) {
	t.Parallel()

	store := &retryDiscussCursorStore{failures: 1}
	driver := NewDiscussDriver(DiscussDriverDeps{CursorStore: store})
	cfg := DiscussSessionConfig{BotID: "bot", SessionID: "session", RouteID: "route"}
	sess := &discussSession{config: cfg}
	position := DiscussCursorPosition{SourceCursor: 90, EventCursor: 100}
	rc := RenderedContext{{
		MessageID:   "message-1",
		EventCursor: 100,
		Content:     []RenderedContentPiece{{Type: "text", Text: "must remain visible"}},
	}}

	driver.advanceDiscussCursor(context.Background(), sess, cfg, position, driver.logger)
	if sess.pendingCursor == nil || sess.lastProcessedCursor != 0 {
		t.Fatalf("failed initial persist = cursor:%d pending:%#v", sess.lastProcessedCursor, sess.pendingCursor)
	}
	queries := &leaseQueries{now: time.Now()}
	leaseStore := newTestLeaseStore(queries, time.Minute, time.Hour)
	oldLease, err := leaseStore.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
	if err != nil || oldLease == nil {
		t.Fatalf("claim old delivery = %#v, %v", oldLease, err)
	}
	sess.pendingCursor.deliveries = []DiscussEventDelivery{{EventID: "event-1", Lease: oldLease}}
	oldLease.markLost()

	if got := driver.retryPendingDiscussCursor(context.Background(), sess, driver.logger); got != pendingDiscussCursorRetryAbandoned {
		t.Fatalf("lost-lease retry = %v, want abandoned", got)
	}
	if sess.pendingCursor != nil || sess.lastProcessedCursor != 0 || store.calls != 1 {
		t.Fatalf("lost lease advanced cursor = cursor:%d pending:%#v calls:%d", sess.lastProcessedCursor, sess.pendingCursor, store.calls)
	}
	if got := LatestExternalEventCursor(rc, sess.lastProcessedCursor); got != 100 {
		t.Fatalf("fresh delivery trigger cursor = %d, want 100", got)
	}

	queries.mu.Lock()
	queries.now = queries.now.Add(time.Minute + time.Millisecond)
	queries.mu.Unlock()
	freshLease, err := leaseStore.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
	if err != nil || freshLease == nil {
		t.Fatalf("claim fresh delivery = %#v, %v", freshLease, err)
	}
	defer func() { _ = freshLease.Release(context.Background()) }()
	freshCtx, cancel := freshLease.Context(context.Background())
	defer cancel()
	if !driver.advanceDiscussCursor(freshCtx, sess, cfg, position, driver.logger) {
		t.Fatal("fresh delivery did not persist the cursor")
	}
	if sess.lastProcessedCursor != 100 || store.calls != 2 {
		t.Fatalf("fresh delivery result = cursor:%d calls:%d", sess.lastProcessedCursor, store.calls)
	}
}

func TestDiscussCursorRetryContextIsCanceledWhenLeaseIsLost(t *testing.T) {
	t.Parallel()

	store := &leaseBoundCursorStore{retryStarted: make(chan struct{})}
	driver := NewDiscussDriver(DiscussDriverDeps{CursorStore: store})
	cfg := DiscussSessionConfig{BotID: "bot", SessionID: "session", RouteID: "route"}
	sess := &discussSession{config: cfg}
	position := DiscussCursorPosition{SourceCursor: 90, EventCursor: 100}
	if driver.advanceDiscussCursor(context.Background(), sess, cfg, position, driver.logger) {
		t.Fatal("initial cursor persist unexpectedly succeeded")
	}
	queries := &leaseQueries{now: time.Now()}
	lease, err := newTestLeaseStore(queries, time.Minute, time.Hour).ClaimEventDelivery(
		context.Background(),
		"33333333-3333-3333-3333-333333333333",
	)
	if err != nil || lease == nil {
		t.Fatalf("claim delivery = %#v, %v", lease, err)
	}
	sess.pendingCursor.deliveries = []DiscussEventDelivery{{EventID: "event-1", Lease: lease}}

	result := make(chan pendingDiscussCursorRetryResult, 1)
	go func() {
		result <- driver.retryPendingDiscussCursor(context.Background(), sess, driver.logger)
	}()
	select {
	case <-store.retryStarted:
	case <-time.After(time.Second):
		t.Fatal("cursor retry did not reach the bound store")
	}
	lease.markLost()
	select {
	case got := <-result:
		if got != pendingDiscussCursorRetryAbandoned {
			t.Fatalf("lease-loss retry = %v, want abandoned", got)
		}
	case <-time.After(time.Second):
		t.Fatal("cursor retry context was not canceled by lease loss")
	}
	if sess.pendingCursor != nil || sess.lastProcessedCursor != 0 {
		t.Fatalf("lease-loss retry advanced cursor = cursor:%d pending:%#v", sess.lastProcessedCursor, sess.pendingCursor)
	}
}

func TestDiscussCursorAdvanceReconcilesAppliedWriteThatReturnedError(t *testing.T) {
	t.Parallel()

	store := &ambiguousDiscussCursorStore{}
	driver := NewDiscussDriver(DiscussDriverDeps{CursorStore: store})
	cfg := DiscussSessionConfig{BotID: "bot", SessionID: "session", RouteID: "route"}
	sess := &discussSession{config: cfg}
	position := DiscussCursorPosition{SourceCursor: 90, EventCursor: 100}

	if !driver.advanceDiscussCursor(context.Background(), sess, cfg, position, driver.logger) {
		t.Fatal("applied ambiguous cursor write was not reconciled")
	}
	if sess.pendingCursor != nil || sess.lastProcessedCursor != 100 {
		t.Fatalf("reconciled cursor = cursor:%d pending:%#v", sess.lastProcessedCursor, sess.pendingCursor)
	}
	if store.upsertCalls != 1 || store.getCalls != 1 || store.persisted != position {
		t.Fatalf("ambiguous store = upserts:%d gets:%d persisted:%#v", store.upsertCalls, store.getCalls, store.persisted)
	}
}

func TestDiscussRunSessionProcessesReplacementAfterPendingLeaseLoss(t *testing.T) {
	t.Parallel()

	cursorStore := &replacementCursorStore{
		firstUpsert: make(chan struct{}),
		lostRead:    make(chan struct{}),
	}
	baseQueries := &leaseQueries{now: time.Now(), historyReady: true}
	queries := &cursorCompletionQueries{leaseQueries: baseQueries, cursorStore: cursorStore}
	leaseStore := newTestLeaseStore(queries, time.Minute, time.Hour)
	oldLease, err := leaseStore.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
	if err != nil || oldLease == nil {
		t.Fatalf("claim old delivery = %#v, %v", oldLease, err)
	}
	agent := &countingCursorOnlyAgent{}
	driver := NewDiscussDriver(DiscussDriverDeps{
		Agent:       agent,
		Resolver:    &cursorOnlyResolver{fakeRunConfigResolver: &fakeRunConfigResolver{resolveResult: ResolveRunConfigResult{ModelID: "model-1"}}},
		CursorStore: cursorStore,
	})
	driver.cursorRetryDelay = 20 * time.Millisecond
	t.Cleanup(driver.StopAll)
	rc := RenderedContext{{
		MessageID:       "message-1",
		ReceivedAtMs:    90,
		EventCursor:     100,
		LastEventCursor: 100,
		Content:         []RenderedContentPiece{{Type: "text", Text: "must be decided"}},
	}}
	config := DiscussSessionConfig{
		BotID:                  "bot-1",
		SessionID:              "session-1",
		RouteID:                "route-1",
		PersistedUserMessageID: "user-message-1",
		EventDelivery: &DiscussEventDelivery{
			EventID:     "33333333-3333-3333-3333-333333333333",
			EventCursor: 100,
			Lease:       oldLease,
		},
	}
	driver.NotifyRC(context.Background(), config.SessionID, rc, config)
	select {
	case <-cursorStore.firstUpsert:
	case <-time.After(time.Second):
		t.Fatal("initial cursor persist was not attempted")
	}
	oldLease.markLost()
	baseQueries.mu.Lock()
	baseQueries.now = baseQueries.now.Add(time.Minute + time.Millisecond)
	baseQueries.mu.Unlock()
	freshLease, err := leaseStore.ClaimEventDelivery(context.Background(), "33333333-3333-3333-3333-333333333333")
	if err != nil || freshLease == nil {
		t.Fatalf("claim replacement delivery = %#v, %v", freshLease, err)
	}
	t.Cleanup(func() { _ = freshLease.Release(context.Background()) })
	select {
	case <-cursorStore.lostRead:
	case <-time.After(time.Second):
		t.Fatal("lost pending cursor was not reconciled")
	}
	if leaseQueriesCompleted(baseQueries) {
		t.Fatal("lost delivery was completed before replacement processing")
	}

	config.EventDelivery = &DiscussEventDelivery{
		EventID:     "33333333-3333-3333-3333-333333333333",
		EventCursor: 100,
		Lease:       freshLease,
	}
	driver.NotifyRC(context.Background(), config.SessionID, rc, config)
	deadline := time.Now().Add(time.Second)
	for !leaseQueriesCompleted(baseQueries) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !leaseQueriesCompleted(baseQueries) {
		t.Fatal("replacement delivery was not completed")
	}
	if got := agent.calls.Load(); got != 2 {
		t.Fatalf("agent decisions = %d, want old attempt plus replacement", got)
	}
	if got := cursorStore.position(); got.EventCursor != 100 {
		t.Fatalf("durable cursor = %#v, want event cursor 100", got)
	}
	if queries.completedBeforeCursor.Load() {
		t.Fatal("replacement delivery completed before its cursor was durable")
	}
}

type replacementCursorStore struct {
	mu           sync.Mutex
	persisted    DiscussCursorPosition
	upsertCalls  int
	getCalls     int
	firstUpsert  chan struct{}
	firstOnce    sync.Once
	lostRead     chan struct{}
	lostReadOnce sync.Once
}

func (s *replacementCursorStore) GetDiscussCursor(context.Context, string, string) (DiscussCursorPosition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getCalls++
	if s.upsertCalls >= 1 && s.getCalls >= 3 {
		s.lostReadOnce.Do(func() { close(s.lostRead) })
	}
	return s.persisted, nil
}

func (s *replacementCursorStore) UpsertDiscussCursor(
	_ context.Context,
	_, _, _, _ string,
	position DiscussCursorPosition,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upsertCalls++
	if s.upsertCalls == 1 {
		s.firstOnce.Do(func() { close(s.firstUpsert) })
		return errors.New("cursor store unavailable")
	}
	s.persisted = position
	return nil
}

func (s *replacementCursorStore) position() DiscussCursorPosition {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persisted
}

type cursorCompletionQueries struct {
	*leaseQueries
	cursorStore           *replacementCursorStore
	completedBeforeCursor atomic.Bool
}

func (q *cursorCompletionQueries) CompleteSessionEventDelivery(
	ctx context.Context,
	arg sqlc.CompleteSessionEventDeliveryParams,
) (int64, error) {
	if q.cursorStore.position().EventCursor < 100 {
		q.completedBeforeCursor.Store(true)
	}
	return q.leaseQueries.CompleteSessionEventDelivery(ctx, arg)
}

type countingCursorOnlyAgent struct {
	calls atomic.Int32
}

func (a *countingCursorOnlyAgent) Stream(context.Context, agentpkg.RunConfig) <-chan agentpkg.StreamEvent {
	a.calls.Add(1)
	events := make(chan agentpkg.StreamEvent, 1)
	events <- agentpkg.StreamEvent{Type: agentpkg.EventAgentEnd, Messages: []byte(`[]`)}
	close(events)
	return events
}

type cursorOnlyResolver struct {
	*fakeRunConfigResolver
}

func (*cursorOnlyResolver) ScheduleCompaction(context.Context, string, string, string, int, int) {}

func leaseQueriesCompleted(queries *leaseQueries) bool {
	queries.mu.Lock()
	defer queries.mu.Unlock()
	return queries.completed
}

type ambiguousDiscussCursorStore struct {
	persisted   DiscussCursorPosition
	upsertCalls int
	getCalls    int
}

func (s *ambiguousDiscussCursorStore) GetDiscussCursor(context.Context, string, string) (DiscussCursorPosition, error) {
	s.getCalls++
	return s.persisted, nil
}

func (s *ambiguousDiscussCursorStore) UpsertDiscussCursor(
	_ context.Context,
	_, _, _, _ string,
	position DiscussCursorPosition,
) error {
	s.upsertCalls++
	s.persisted = position
	return errors.New("connection lost after commit")
}

type leaseBoundCursorStore struct {
	calls        int
	retryStarted chan struct{}
}

func (*leaseBoundCursorStore) GetDiscussCursor(context.Context, string, string) (DiscussCursorPosition, error) {
	return DiscussCursorPosition{}, nil
}

func (s *leaseBoundCursorStore) UpsertDiscussCursor(
	ctx context.Context,
	_, _, _, _ string,
	_ DiscussCursorPosition,
) error {
	s.calls++
	if s.calls == 1 {
		return errors.New("cursor store unavailable")
	}
	close(s.retryStarted)
	<-ctx.Done()
	return ctx.Err()
}
