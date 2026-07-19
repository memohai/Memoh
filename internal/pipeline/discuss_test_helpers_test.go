package pipeline

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

// Shared fakes for the delivery/cursor and compaction test families. Keeping
// them in one place avoids per-file copies of the same fixtures.

type leaseQueries struct {
	dbstore.Queries

	mu                sync.Mutex
	now               time.Time
	claimUntil        time.Time
	claimStarted      chan struct{}
	claimRelease      chan struct{}
	claimOnce         sync.Once
	claimFromReturn   time.Duration
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
	if q.claimStarted != nil {
		q.claimOnce.Do(func() { close(q.claimStarted) })
	}
	if q.claimRelease != nil {
		<-q.claimRelease
	}
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
	if q.claimFromReturn > 0 {
		q.until = time.Now().Add(q.claimFromReturn)
	}
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

func newTestLeaseStore(queries dbstore.Queries, duration, renewInterval time.Duration) *EventStore {
	store := NewEventStore(nil, queries)
	store.deliveryLeaseDuration = duration
	store.deliveryRenewInterval = renewInterval
	return store
}

func leaseQueriesCompleted(queries *leaseQueries) bool {
	queries.mu.Lock()
	defer queries.mu.Unlock()
	return queries.completed
}

func sdkMessageText(messages []sdk.Message) string {
	var text strings.Builder
	for _, message := range messages {
		for _, part := range message.Content {
			if value, ok := part.(sdk.TextPart); ok {
				text.WriteString(value.Text)
				text.WriteByte('\n')
			}
		}
	}
	return text.String()
}

func renderedText(id string, atMs int64, text string) RenderedSegment {
	return RenderedSegment{
		MessageID:    id,
		ReceivedAtMs: atMs,
		Content:      []RenderedContentPiece{{Type: "text", Text: messageXML(id, text)}},
	}
}

func messageXML(id, text string) string {
	return `<message id="` + id + `">` + text + `</message>`
}

func assertContextContents(t *testing.T, messages []ContextMessage, want []string) {
	t.Helper()
	got := make([]string, 0, len(messages))
	for _, message := range messages {
		got = append(got, message.Content)
	}
	if len(got) != len(want) {
		t.Fatalf("message count = %d, want %d:\ngot  %#v\nwant %#v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}
