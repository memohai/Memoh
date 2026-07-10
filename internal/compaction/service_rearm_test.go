package compaction

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

type rearmQueries struct {
	*fakeQueries
	mu              sync.Mutex
	markedBatches   [][]pgtype.UUID
	completionCount int
	secondComplete  chan struct{}
}

func (q *rearmQueries) MarkMessagesCompacted(_ context.Context, arg sqlc.MarkMessagesCompactedParams) error {
	q.mu.Lock()
	q.markedBatches = append(q.markedBatches, append([]pgtype.UUID(nil), arg.Column2...))
	q.mu.Unlock()
	return nil
}

func (q *rearmQueries) CompleteCompactionLog(_ context.Context, arg sqlc.CompleteCompactionLogParams) (sqlc.BotHistoryMessageCompact, error) {
	q.mu.Lock()
	q.completionCount++
	if q.completionCount == 2 {
		close(q.secondComplete)
	}
	q.mu.Unlock()
	return sqlc.BotHistoryMessageCompact{ID: arg.ID, Status: arg.Status, Summary: arg.Summary}, nil
}

type blockingCompactionModel struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (m *blockingCompactionModel) RoundTrip(*http.Request) (*http.Response, error) {
	m.once.Do(func() { close(m.started) })
	<-m.release
	return compactionModelResponse(), nil
}

type signalingCompactionModel struct {
	called chan struct{}
	once   sync.Once
}

func (m *signalingCompactionModel) RoundTrip(*http.Request) (*http.Response, error) {
	m.once.Do(func() { close(m.called) })
	return compactionModelResponse(), nil
}

func compactionModelResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`{"id":"stub","object":"chat.completion","created":0,"model":"stub",` +
				`"choices":[{"index":0,"message":{"role":"assistant","content":"summary"},"finish_reason":"stop"}],` +
				`"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120}}`,
		)),
		Header: http.Header{"Content-Type": []string{"application/json"}},
	}
}

func TestTriggerCompactionRearmsDemandQueuedWhileSessionIsInFlight(t *testing.T) {
	firstRows := machineryCorpus(t)
	secondRows := machineryCorpus(t)
	for i := range secondRows {
		secondRows[i].ID = pgtype.UUID{Bytes: uuid.New(), Valid: true}
		secondRows[i].CreatedAt = pgtype.Timestamptz{Time: time.Now().UTC().Add(time.Duration(i) * time.Millisecond), Valid: true}
	}
	queries := &rearmQueries{
		fakeQueries:    &fakeQueries{uncompacted: firstRows},
		secondComplete: make(chan struct{}),
	}
	service := newMachineryService(queries)
	firstModel := &blockingCompactionModel{started: make(chan struct{}), release: make(chan struct{})}
	supersededModel := &signalingCompactionModel{called: make(chan struct{})}
	secondModel := &signalingCompactionModel{called: make(chan struct{})}
	firstConfig := TriggerConfig{
		BotID:        uuid.NewString(),
		SessionID:    uuid.NewString(),
		ModelID:      "stub-model",
		ClientType:   "openai-completions",
		APIKey:       "test",
		BaseURL:      "http://stub.invalid",
		HTTPClient:   &http.Client{Transport: firstModel},
		TargetTokens: 450,
	}
	supersededConfig := firstConfig
	supersededConfig.HTTPClient = &http.Client{Transport: supersededModel}
	secondConfig := firstConfig
	secondConfig.HTTPClient = &http.Client{Transport: secondModel}

	service.TriggerCompaction(context.Background(), firstConfig)
	select {
	case <-firstModel.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first compaction did not start")
	}
	queries.uncompacted = secondRows
	service.TriggerCompaction(context.Background(), supersededConfig)
	service.TriggerCompaction(context.Background(), secondConfig)
	close(firstModel.release)

	select {
	case <-queries.secondComplete:
	case <-time.After(2 * time.Second):
		t.Fatal("queued compaction demand did not complete")
	}
	select {
	case <-supersededModel.called:
		t.Fatal("superseded queued config was executed")
	default:
	}
	assertRearmedCompactionRows(t, queries, secondRows)
}

func TestTriggerCompactionRearmsAfterSynchronousOwnerReleasesSession(t *testing.T) {
	firstRows := machineryCorpus(t)
	secondRows := machineryCorpus(t)
	for i := range secondRows {
		secondRows[i].ID = pgtype.UUID{Bytes: uuid.New(), Valid: true}
		secondRows[i].CreatedAt = pgtype.Timestamptz{Time: time.Now().UTC().Add(time.Duration(i) * time.Millisecond), Valid: true}
	}
	queries := &rearmQueries{
		fakeQueries:    &fakeQueries{uncompacted: firstRows},
		secondComplete: make(chan struct{}),
	}
	service := newMachineryService(queries)
	firstModel := &blockingCompactionModel{started: make(chan struct{}), release: make(chan struct{})}
	secondModel := &signalingCompactionModel{called: make(chan struct{})}
	firstConfig := TriggerConfig{
		BotID:        uuid.NewString(),
		SessionID:    uuid.NewString(),
		ModelID:      "stub-model",
		ClientType:   "openai-completions",
		APIKey:       "test",
		BaseURL:      "http://stub.invalid",
		HTTPClient:   &http.Client{Transport: firstModel},
		TargetTokens: 450,
		Manual:       true,
	}
	secondConfig := firstConfig
	secondConfig.HTTPClient = &http.Client{Transport: secondModel}
	secondConfig.Manual = false
	service.recordCompactionFailure(firstConfig.SessionID)
	syncDone := make(chan error, 1)
	go func() {
		res, err := service.RunCompactionSync(context.Background(), firstConfig)
		_ = res
		syncDone <- err
	}()

	select {
	case <-firstModel.started:
	case <-time.After(2 * time.Second):
		t.Fatal("synchronous compaction did not start")
	}
	queries.uncompacted = secondRows
	service.TriggerCompaction(context.Background(), secondConfig)
	close(firstModel.release)

	select {
	case err := <-syncDone:
		if err != nil {
			t.Fatalf("synchronous compaction failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("synchronous compaction did not finish")
	}
	select {
	case <-queries.secondComplete:
	case <-time.After(2 * time.Second):
		t.Fatal("async demand was not rearmed after synchronous owner")
	}
	assertRearmedCompactionRows(t, queries, secondRows)
}

func TestAutomaticCompactionObservesBusyOwnerBeforeFailureCooldown(t *testing.T) {
	service := newMachineryService(&fakeQueries{})
	sessionID := uuid.NewString()
	service.recordCompactionFailure(sessionID)
	if !service.beginSessionCompaction(sessionID) {
		t.Fatal("manual owner failed to acquire session")
	}

	result, err := service.runCompaction(context.Background(), TriggerConfig{
		BotID:     uuid.NewString(),
		SessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("busy automatic compaction error = %v", err)
	}
	if result.Status != StatusNoop || result.inflightDone == nil {
		t.Fatalf("busy automatic result = %#v, want noop with owner completion signal", result)
	}
	service.endSessionCompaction(sessionID)
	select {
	case <-result.inflightDone:
	default:
		t.Fatal("owner completion signal remained open after release")
	}
}

func assertRearmedCompactionRows(t *testing.T, queries *rearmQueries, secondRows []sqlc.ListUncompactedMessagesBySessionRow) {
	t.Helper()
	secondIDs := make(map[pgtype.UUID]struct{}, len(secondRows))
	for _, row := range secondRows {
		secondIDs[row.ID] = struct{}{}
	}
	queries.mu.Lock()
	markedBatches := append([][]pgtype.UUID(nil), queries.markedBatches...)
	queries.mu.Unlock()
	if len(markedBatches) != 2 || len(markedBatches[1]) == 0 {
		t.Fatalf("marked batches = %#v, want two non-empty passes", markedBatches)
	}
	for _, id := range markedBatches[1] {
		if _, ok := secondIDs[id]; !ok {
			t.Fatalf("second pass marked stale row %s", id.String())
		}
	}
}
