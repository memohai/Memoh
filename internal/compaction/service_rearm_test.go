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
	mu                sync.Mutex
	finalizedBatches  [][]pgtype.UUID
	finalizationCount int
	secondFinalize    chan struct{}
}

func (q *rearmQueries) FinalizeCompactionArtifact(_ context.Context, arg sqlc.FinalizeCompactionArtifactParams) (sqlc.FinalizeCompactionArtifactRow, error) {
	q.mu.Lock()
	q.finalizedBatches = append(q.finalizedBatches, append([]pgtype.UUID(nil), arg.MessageIds...))
	q.finalizationCount++
	if q.finalizationCount == 2 {
		close(q.secondFinalize)
	}
	q.mu.Unlock()
	count := int32(len(arg.MessageIds)) //nolint:gosec // test corpus is bounded
	return sqlc.FinalizeCompactionArtifactRow{Finalized: true, Status: "ok", RequestedCount: count, MatchedCount: count, ClaimedCount: count}, nil
}

type blockingCompactionModel struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	summary string
}

func (m *blockingCompactionModel) RoundTrip(*http.Request) (*http.Response, error) {
	m.once.Do(func() { close(m.started) })
	<-m.release
	return compactionModelResponse(m.summary), nil
}

type signalingCompactionModel struct {
	called  chan struct{}
	once    sync.Once
	summary string
}

func (m *signalingCompactionModel) RoundTrip(*http.Request) (*http.Response, error) {
	m.once.Do(func() { close(m.called) })
	return compactionModelResponse(m.summary), nil
}

func compactionModelResponse(summary string) *http.Response {
	if summary == "" {
		summary = "summary"
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`{"id":"stub","object":"chat.completion","created":0,"model":"stub",` +
				`"choices":[{"index":0,"message":{"role":"assistant","content":"` + summary + `"},"finish_reason":"stop"}],` +
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
		secondFinalize: make(chan struct{}),
	}
	service := newMachineryService(queries)
	firstModel := &blockingCompactionModel{started: make(chan struct{}), release: make(chan struct{})}
	supersededModel := &signalingCompactionModel{called: make(chan struct{})}
	secondModel := &signalingCompactionModel{called: make(chan struct{})}
	firstConfig := TriggerConfig{
		BotID:              uuid.NewString(),
		SessionID:          uuid.NewString(),
		ModelID:            "stub-model",
		ClientType:         "openai-completions",
		APIKey:             "test",
		BaseURL:            "http://stub.invalid",
		HTTPClient:         &http.Client{Transport: firstModel},
		TargetTokens:       450,
		ContextTokenBudget: 32_000,
		MaxCompactTokens:   28_800,
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
	case <-queries.secondFinalize:
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
		secondFinalize: make(chan struct{}),
	}
	service := newMachineryService(queries)
	firstModel := &blockingCompactionModel{started: make(chan struct{}), release: make(chan struct{})}
	secondModel := &signalingCompactionModel{called: make(chan struct{})}
	firstConfig := TriggerConfig{
		BotID:              uuid.NewString(),
		SessionID:          uuid.NewString(),
		ModelID:            "stub-model",
		ClientType:         "openai-completions",
		APIKey:             "test",
		BaseURL:            "http://stub.invalid",
		HTTPClient:         &http.Client{Transport: firstModel},
		TargetTokens:       450,
		Manual:             true,
		ContextTokenBudget: 32_000,
		MaxCompactTokens:   28_800,
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
	case <-queries.secondFinalize:
	case <-time.After(2 * time.Second):
		t.Fatal("async demand was not rearmed after synchronous owner")
	}
	assertRearmedCompactionRows(t, queries, secondRows)
}

func assertRearmedCompactionRows(t *testing.T, queries *rearmQueries, secondRows []sqlc.ListUncompactedMessagesBySessionRow) {
	t.Helper()
	secondIDs := make(map[pgtype.UUID]struct{}, len(secondRows))
	for _, row := range secondRows {
		secondIDs[row.ID] = struct{}{}
	}
	queries.mu.Lock()
	finalizedBatches := append([][]pgtype.UUID(nil), queries.finalizedBatches...)
	queries.mu.Unlock()
	if len(finalizedBatches) != 2 || len(finalizedBatches[1]) == 0 {
		t.Fatalf("finalized batches = %#v, want two non-empty passes", finalizedBatches)
	}
	for _, id := range finalizedBatches[1] {
		if _, ok := secondIDs[id]; !ok {
			t.Fatalf("second pass marked stale row %s", id.String())
		}
	}
}
