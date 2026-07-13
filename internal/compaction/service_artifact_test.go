package compaction

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
)

type artifactQueries struct {
	*fakeQueries
	assets      []sqlc.ListMessageAssetsBatchRow
	assetsErr   error
	completeErr error
	assetCalls  int
}

func (q *artifactQueries) ListMessageAssetsBatch(_ context.Context, _ []pgtype.UUID) ([]sqlc.ListMessageAssetsBatchRow, error) {
	q.assetCalls++
	return q.assets, q.assetsErr
}

func (q *artifactQueries) CompleteCompactionLog(_ context.Context, arg sqlc.CompleteCompactionLogParams) (sqlc.BotHistoryMessageCompact, error) {
	q.completed = arg
	if q.completeErr != nil {
		return sqlc.BotHistoryMessageCompact{}, q.completeErr
	}
	return sqlc.BotHistoryMessageCompact{ID: arg.ID, Status: arg.Status, Summary: arg.Summary}, nil
}

func TestDoCompactionPersistsDurableCoverageAndAnchor(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"old question"`, 100),
		mkRow(t, "assistant", `"old answer"`, 100),
		mkRow(t, "user", `"current question"`, 100),
		mkRow(t, "assistant", `"current answer"`, 100),
	}
	for i := range rows {
		rows[i].CreatedAt = pgtype.Timestamptz{Time: time.UnixMilli(int64(i+1) * 1000), Valid: true}
	}
	q := &fakeQueries{uncompacted: rows}
	stub := &stubModel{summary: "SUMMARY"}

	if _, err := newMachineryService(q).RunCompactionSync(context.Background(), machineryConfig(stub, 200)); err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}

	coverage, err := DecodeArtifactCoverage(q.completed.Coverage)
	if err != nil {
		t.Fatalf("decode coverage: %v", err)
	}
	if len(coverage) != 2 || coverage[0].Ref.ID != formatUUID(rows[0].ID) || coverage[1].Ref.ID != formatUUID(rows[1].ID) {
		t.Fatalf("coverage = %#v, want durable refs for the two compacted rows", coverage)
	}
	if coverage[0].Ref.ContentHash == "" || coverage[1].Ref.ContentHash == "" {
		t.Fatalf("coverage must preserve source hashes: %#v", coverage)
	}
	if q.completed.AnchorStartMs != 1000 || q.completed.AnchorEndMs != 2000 {
		t.Fatalf("anchor = %d..%d, want 1000..2000", q.completed.AnchorStartMs, q.completed.AnchorEndMs)
	}
}

func TestDoCompactionOrdersDurableCoverageBySourceTime(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"old question"`, 100),
		mkRow(t, "assistant", `"old answer"`, 100),
		mkRow(t, "user", `"current question"`, 100),
		mkRow(t, "assistant", `"current answer"`, 100),
	}
	rows[0].CreatedAt = pgtype.Timestamptz{Time: time.UnixMilli(2000), Valid: true}
	rows[1].CreatedAt = pgtype.Timestamptz{Time: time.UnixMilli(1000), Valid: true}
	rows[2].CreatedAt = pgtype.Timestamptz{Time: time.UnixMilli(3000), Valid: true}
	rows[3].CreatedAt = pgtype.Timestamptz{Time: time.UnixMilli(4000), Valid: true}
	q := &fakeQueries{uncompacted: rows}

	if _, err := newMachineryService(q).RunCompactionSync(context.Background(), machineryConfig(&stubModel{summary: "SUMMARY"}, 200)); err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}

	coverage, err := DecodeArtifactCoverage(q.completed.Coverage)
	if err != nil {
		t.Fatalf("decode coverage: %v", err)
	}
	if len(coverage) != 2 || coverage[0].Ref.ID != formatUUID(rows[1].ID) || coverage[1].Ref.ID != formatUUID(rows[0].ID) {
		t.Fatalf("coverage = %#v, want source-time order", coverage)
	}
	if q.completed.AnchorStartMs != 1000 || q.completed.AnchorEndMs != 2000 {
		t.Fatalf("anchor = %d..%d, want 1000..2000", q.completed.AnchorStartMs, q.completed.AnchorEndMs)
	}
}

func TestDoCompactionCoverageHashIncludesMessageAssets(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"old question"`, 100),
		mkRow(t, "assistant", `"old answer"`, 100),
		mkRow(t, "user", `"current question"`, 100),
		mkRow(t, "assistant", `"current answer"`, 100),
	}
	asset := sqlc.ListMessageAssetsBatchRow{
		MessageID:   rows[0].ID,
		Role:        "attachment",
		Ordinal:     0,
		ContentHash: "sha256:asset-1",
		Name:        "diagram.png",
		Metadata:    []byte(`{"alt":"architecture"}`),
	}
	q := &artifactQueries{
		fakeQueries: &fakeQueries{uncompacted: rows},
		assets:      []sqlc.ListMessageAssetsBatchRow{asset},
	}
	stub := &stubModel{summary: "SUMMARY"}

	if _, err := newMachineryService(q).RunCompactionSync(context.Background(), machineryConfig(stub, 200)); err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}

	coverage, err := DecodeArtifactCoverage(q.completed.Coverage)
	if err != nil {
		t.Fatalf("decode coverage: %v", err)
	}
	if len(coverage) != 2 {
		t.Fatalf("coverage = %#v, want two covered rows", coverage)
	}
	expectedMessage := rowToMessage(rows[0])
	expectedMessage.Assets = []messagepkg.MessageAsset{{
		ContentHash: asset.ContentHash,
		Role:        asset.Role,
		Ordinal:     int(asset.Ordinal),
		Name:        asset.Name,
		Metadata:    map[string]any{"alt": "architecture"},
	}}
	wantHash := historyfrag.DBMessageSourceHash(expectedMessage).Value
	if got := coverage[0].Ref.ContentHash; got != wantHash {
		t.Fatalf("coverage hash = %q, want attachment-aware source hash %q", got, wantHash)
	}
	plainHash := historyfrag.DBMessageSourceHash(rowToMessage(rows[0])).Value
	if coverage[0].Ref.ContentHash == plainHash {
		t.Fatal("coverage hash ignored the attached asset")
	}
	if q.assetCalls != 1 {
		t.Fatalf("asset batch calls = %d, want 1", q.assetCalls)
	}
}

func TestDoCompactionStopsWhenAssetsCannotBeLoaded(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"old question"`, 100),
		mkRow(t, "assistant", `"old answer"`, 100),
		mkRow(t, "user", `"current question"`, 100),
		mkRow(t, "assistant", `"current answer"`, 100),
	}
	assetErr := errors.New("asset read failed")
	q := &artifactQueries{
		fakeQueries: &fakeQueries{uncompacted: rows},
		assetsErr:   assetErr,
	}

	_, err := newMachineryService(q).RunCompactionSync(context.Background(), machineryConfig(&stubModel{summary: "SUMMARY"}, 200))
	if !errors.Is(err, assetErr) {
		t.Fatalf("RunCompactionSync error = %v, want %v", err, assetErr)
	}
	if q.created || len(q.markedIDs) > 0 {
		t.Fatalf("asset failure must stop before persistence: created=%v marked=%v", q.created, q.markedIDs)
	}
}

func TestDoCompactionReturnsSuccessfulArtifactFinalizationError(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"old question"`, 100),
		mkRow(t, "assistant", `"old answer"`, 100),
		mkRow(t, "user", `"current question"`, 100),
		mkRow(t, "assistant", `"current answer"`, 100),
	}
	completeErr := errors.New("artifact finalization failed")
	q := &artifactQueries{
		fakeQueries: &fakeQueries{uncompacted: rows},
		completeErr: completeErr,
	}

	_, err := newMachineryService(q).RunCompactionSync(context.Background(), machineryConfig(&stubModel{summary: "SUMMARY"}, 200))
	if !errors.Is(err, completeErr) {
		t.Fatalf("RunCompactionSync error = %v, want %v", err, completeErr)
	}
	if len(q.markedIDs) != 2 {
		t.Fatalf("marked ids = %v, want generated artifact rows marked before finalization", q.markedIDs)
	}
}
