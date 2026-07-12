package compaction

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/historyfrag"
	messagepkg "github.com/memohai/memoh/internal/message"
)

type artifactQueries struct {
	*fakeQueries
	assets     []sqlc.ListMessageAssetsBatchRow
	assetsErr  error
	assetCalls int
}

func (q *artifactQueries) ListMessageAssetsBatch(_ context.Context, _ []pgtype.UUID) ([]sqlc.ListMessageAssetsBatchRow, error) {
	q.assetCalls++
	return q.assets, q.assetsErr
}

func TestDoCompactionPersistsDurableCoverageAndAnchor(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", jsonStr(strings.Repeat("old question ", 20)), 100),
		mkRow(t, "assistant", jsonStr(strings.Repeat("old answer ", 20)), 100),
		mkRow(t, "user", `"current question"`, 100),
		mkRow(t, "assistant", `"current answer"`, 100),
	}
	for i := range rows {
		rows[i].CreatedAt = pgtype.Timestamptz{Time: time.UnixMilli(int64(i+1) * 1000), Valid: true}
	}
	priorCompactID := testUUID(t)
	rows[0].SourceVersion = "version-a"
	rows[0].CompactID = priorCompactID
	rows[1].SourceVersion = "version-b"
	q := &fakeQueries{uncompacted: rows}
	stub := &stubModel{summary: "SUMMARY"}

	if _, err := newMachineryService(q).RunCompactionSync(context.Background(), machineryConfig(stub, 200)); err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}

	coverage, err := DecodeArtifactCoverage(q.finalized.Coverage)
	if err != nil {
		t.Fatalf("decode coverage: %v", err)
	}
	if len(coverage) != 2 || coverage[0].Ref.ID != formatUUID(rows[0].ID) || coverage[1].Ref.ID != formatUUID(rows[1].ID) {
		t.Fatalf("coverage = %#v, want durable refs for the two compacted rows", coverage)
	}
	if coverage[0].Ref.ContentHash == "" || coverage[1].Ref.ContentHash == "" {
		t.Fatalf("coverage must preserve source hashes: %#v", coverage)
	}
	if q.finalized.AnchorStartMs != 1000 || q.finalized.AnchorEndMs != 2000 {
		t.Fatalf("anchor = %d..%d, want 1000..2000", q.finalized.AnchorStartMs, q.finalized.AnchorEndMs)
	}
	if len(q.finalized.MessageIds) != 2 || q.finalized.MessageIds[0] != rows[0].ID || q.finalized.MessageIds[1] != rows[1].ID {
		t.Fatalf("message ids = %#v, want source order", q.finalized.MessageIds)
	}
	if len(q.finalized.SourceVersions) != 2 || q.finalized.SourceVersions[0] != "version-a" || q.finalized.SourceVersions[1] != "version-b" {
		t.Fatalf("source versions = %#v, want aligned snapshots", q.finalized.SourceVersions)
	}
	if len(q.finalized.ExpectedCompactIds) != 2 || q.finalized.ExpectedCompactIds[0] != formatUUID(priorCompactID) || q.finalized.ExpectedCompactIds[1] != "" {
		t.Fatalf("expected compact ids = %#v, want aligned owners", q.finalized.ExpectedCompactIds)
	}
}

func TestDoCompactionCoverageHashIncludesMessageAssets(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", jsonStr(strings.Repeat("old question ", 20)), 100),
		mkRow(t, "assistant", jsonStr(strings.Repeat("old answer ", 20)), 100),
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

	coverage, err := DecodeArtifactCoverage(q.finalized.Coverage)
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
		mkRow(t, "user", jsonStr(strings.Repeat("old question ", 20)), 100),
		mkRow(t, "assistant", jsonStr(strings.Repeat("old answer ", 20)), 100),
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
		mkRow(t, "user", jsonStr(strings.Repeat("old question ", 20)), 100),
		mkRow(t, "assistant", jsonStr(strings.Repeat("old answer ", 20)), 100),
		mkRow(t, "user", `"current question"`, 100),
		mkRow(t, "assistant", `"current answer"`, 100),
	}
	finalizeErr := errors.New("artifact finalization failed")
	q := &artifactQueries{
		fakeQueries: &fakeQueries{uncompacted: rows, finalizeErr: finalizeErr},
	}
	ctx, cancel := context.WithCancel(context.Background())
	q.beforeFinalize = cancel
	service := newMachineryService(q)

	config := machineryConfig(&stubModel{summary: "SUMMARY"}, 200)
	_, err := service.RunCompactionSync(ctx, config)
	if !errors.Is(err, finalizeErr) {
		t.Fatalf("RunCompactionSync error = %v, want %v", err, finalizeErr)
	}
	if len(q.markedIDs) != 0 || q.completed.Status != "error" {
		t.Fatalf("failed atomic finalization left split state: marked=%v completion=%q", q.markedIDs, q.completed.Status)
	}
	if q.finalizeCtxErr != nil || q.completeCtxErr != nil || !q.finalizeDeadline || !q.completeDeadline {
		t.Fatalf(
			"persistence context = finalize:(%v,%v) complete:(%v,%v), want active bounded contexts",
			q.finalizeCtxErr,
			q.finalizeDeadline,
			q.completeCtxErr,
			q.completeDeadline,
		)
	}
	if !service.inFailureCooldown(config.SessionID) {
		t.Fatal("finalization failure concurrent with caller cancellation did not arm cooldown")
	}
}

func TestDoCompactionReconcilesCommittedFinalizationAfterResponseLoss(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", jsonStr(strings.Repeat("old question ", 20)), 100),
		mkRow(t, "assistant", jsonStr(strings.Repeat("old answer ", 20)), 100),
		mkRow(t, "user", `"current question"`, 100),
	}
	responseLoss := errors.New("finalization response lost")
	initialReconcileErr := errors.New("initial reconciliation response lost")
	q := &artifactQueries{fakeQueries: &fakeQueries{
		uncompacted:      rows,
		finalizeErr:      responseLoss,
		completeErr:      pgx.ErrNoRows,
		reconcileSuccess: true,
		getErrors:        []error{initialReconcileErr},
	}}

	result, err := newMachineryService(q).RunCompactionSync(context.Background(), machineryConfig(&stubModel{summary: "SUMMARY"}, 100))
	if err != nil {
		t.Fatalf("RunCompactionSyncResult error = %v, want reconciled success", err)
	}
	if result.Status != StatusOK || result.MessageCount != 2 || q.completed.Status != "error" || q.getCalls != 2 {
		t.Fatalf("reconciled result = %#v completion=%q, want atomic success", result, q.completed.Status)
	}
}

func TestDoCompactionReconcilesCommittedAttemptAfterCreateResponseLoss(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", jsonStr(strings.Repeat("old question ", 20)), 100),
		mkRow(t, "assistant", jsonStr(strings.Repeat("old answer ", 20)), 100),
		mkRow(t, "user", `"current question"`, 100),
	}
	responseLoss := errors.New("create response lost after commit")
	q := &artifactQueries{fakeQueries: &fakeQueries{
		uncompacted:   rows,
		createErrors:  []error{responseLoss},
		createCommits: []bool{true},
	}}

	result, err := newMachineryService(q).RunCompactionSyncResult(context.Background(), machineryConfig(&stubModel{summary: "SUMMARY"}, 100))
	if err != nil {
		t.Fatalf("RunCompactionSyncResult error = %v, want reconciled success", err)
	}
	if result.Status != StatusOK || q.createCalls != 1 || q.getCalls != 1 {
		t.Fatalf("reconciled create = result:%#v creates:%d gets:%d", result, q.createCalls, q.getCalls)
	}
	assertStableAttemptID(t, q)
}

func TestDoCompactionRetriesUncommittedCreateWithStableAttemptID(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", jsonStr(strings.Repeat("old question ", 20)), 100),
		mkRow(t, "assistant", jsonStr(strings.Repeat("old answer ", 20)), 100),
		mkRow(t, "user", `"current question"`, 100),
	}
	initialFailure := errors.New("create failed before commit")
	q := &artifactQueries{fakeQueries: &fakeQueries{
		uncompacted:  rows,
		createErrors: []error{initialFailure},
	}}

	result, err := newMachineryService(q).RunCompactionSyncResult(context.Background(), machineryConfig(&stubModel{summary: "SUMMARY"}, 100))
	if err != nil {
		t.Fatalf("RunCompactionSyncResult error = %v, want retried success", err)
	}
	if result.Status != StatusOK || q.createCalls != 2 || q.getCalls != 1 {
		t.Fatalf("retried create = result:%#v creates:%d gets:%d", result, q.createCalls, q.getCalls)
	}
	assertStableAttemptID(t, q)
	for index := range q.createCtxErrors {
		if q.createCtxErrors[index] != nil || !q.createDeadlines[index] {
			t.Fatalf("create context %d = (%v,%v), want active bounded context", index, q.createCtxErrors[index], q.createDeadlines[index])
		}
	}
}

func TestDoCompactionReconcilesCommittedAttemptAfterCreateRetry(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", jsonStr(strings.Repeat("old question ", 20)), 100),
		mkRow(t, "assistant", jsonStr(strings.Repeat("old answer ", 20)), 100),
		mkRow(t, "user", `"current question"`, 100),
	}
	responseLoss := errors.New("create response lost after commit")
	reconcileFailure := errors.New("initial create reconciliation failed")
	q := &artifactQueries{fakeQueries: &fakeQueries{
		uncompacted:   rows,
		createErrors:  []error{responseLoss},
		createCommits: []bool{true},
		getErrors:     []error{reconcileFailure},
	}}

	result, err := newMachineryService(q).RunCompactionSyncResult(context.Background(), machineryConfig(&stubModel{summary: "SUMMARY"}, 100))
	if err != nil {
		t.Fatalf("RunCompactionSyncResult error = %v, want retry reconciliation success", err)
	}
	if result.Status != StatusOK || q.createCalls != 2 || q.getCalls != 2 {
		t.Fatalf("retried reconciliation = result:%#v creates:%d gets:%d", result, q.createCalls, q.getCalls)
	}
	assertStableAttemptID(t, q)
}

func TestDoCompactionRejectsAttemptIDCollisionBeforeModelCall(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", jsonStr(strings.Repeat("old question ", 20)), 100),
		mkRow(t, "assistant", jsonStr(strings.Repeat("old answer ", 20)), 100),
		mkRow(t, "user", `"current question"`, 100),
	}
	stub := &stubModel{summary: "unused"}
	q := &artifactQueries{fakeQueries: &fakeQueries{
		uncompacted:   rows,
		createErrors:  []error{pgx.ErrNoRows},
		createCommits: []bool{true},
		mutateCreatedLog: func(row *sqlc.BotHistoryMessageCompact) {
			row.BotID = testUUID(t)
		},
	}}

	_, err := newMachineryService(q).RunCompactionSync(context.Background(), machineryConfig(stub, 100))
	if err == nil || !strings.Contains(err.Error(), "unexpected persisted row") {
		t.Fatalf("RunCompactionSync error = %v, want attempt collision invariant", err)
	}
	if stub.calls != 0 || q.createCalls != 1 || q.getCalls != 1 {
		t.Fatalf("collision path = model:%d creates:%d gets:%d", stub.calls, q.createCalls, q.getCalls)
	}
}

func assertStableAttemptID(t *testing.T, q *artifactQueries) {
	t.Helper()
	if len(q.createParams) == 0 || !q.createParams[0].ID.Valid {
		t.Fatalf("create params = %#v, want runtime-generated attempt id", q.createParams)
	}
	for _, params := range q.createParams[1:] {
		if params.ID != q.createParams[0].ID {
			t.Fatalf("create attempt ids changed across retry: %#v", q.createParams)
		}
	}
	if q.finalized.CompactID != q.createParams[0].ID {
		t.Fatalf("finalized attempt %s, want created attempt %s", q.finalized.CompactID, q.createParams[0].ID)
	}
}

func TestDoCompactionReconcilesCommittedSourceConflictAfterResponseLoss(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", jsonStr(strings.Repeat("old question ", 20)), 100),
		mkRow(t, "assistant", jsonStr(strings.Repeat("old answer ", 20)), 100),
		mkRow(t, "user", `"current question"`, 100),
	}
	responseLoss := errors.New("source conflict response lost")
	q := &artifactQueries{fakeQueries: &fakeQueries{
		uncompacted:       rows,
		finalizeErr:       responseLoss,
		reconcileConflict: true,
	}}
	service := newMachineryService(q)
	config := machineryConfig(&stubModel{summary: "SUMMARY"}, 100)

	_, err := service.RunCompactionSync(context.Background(), config)
	if !errors.Is(err, ErrCompactionSourceChanged) {
		t.Fatalf("RunCompactionSync error = %v, want reconciled source conflict", err)
	}
	if q.completed.Status != "" || q.getCalls != 1 {
		t.Fatalf("reconciled conflict completion=%q gets=%d, want no fallback completion and one read", q.completed.Status, q.getCalls)
	}
	if service.inFailureCooldown(config.SessionID) {
		t.Fatal("reconciled source conflict armed failure cooldown")
	}
}

func TestDoCompactionReturnsSourceConflictWithoutSecondCompletion(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", jsonStr(strings.Repeat("old question ", 20)), 100),
		mkRow(t, "assistant", jsonStr(strings.Repeat("old answer ", 20)), 100),
		mkRow(t, "user", `"current question"`, 100),
	}
	q := &artifactQueries{fakeQueries: &fakeQueries{
		uncompacted: rows,
		finalizeResult: &sqlc.FinalizeCompactionArtifactRow{
			Finalized:      false,
			Status:         "error",
			RequestedCount: 2,
			MatchedCount:   1,
			ClaimedCount:   0,
		},
	}}

	service := newMachineryService(q)
	config := machineryConfig(&stubModel{summary: "SUMMARY"}, 100)
	_, err := service.RunCompactionSync(context.Background(), config)
	if !errors.Is(err, ErrCompactionSourceChanged) {
		t.Fatalf("RunCompactionSync error = %v, want source conflict", err)
	}
	if len(q.markedIDs) != 0 || q.completed.Status != "" {
		t.Fatalf("source conflict used split persistence: marked=%v completion=%q", q.markedIDs, q.completed.Status)
	}
	if service.inFailureCooldown(config.SessionID) {
		t.Fatal("optimistic source conflict armed failure cooldown")
	}
}

func TestDoCompactionJoinsModelAndTerminalizationErrors(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", jsonStr(strings.Repeat("old question ", 20)), 100),
		mkRow(t, "assistant", jsonStr(strings.Repeat("old answer ", 20)), 100),
		mkRow(t, "user", `"current question"`, 100),
	}
	modelErr := &failingModel{}
	cleanupErr := errors.New("terminalization failed")
	q := &artifactQueries{fakeQueries: &fakeQueries{uncompacted: rows, completeErr: cleanupErr}}
	config := machineryConfig(&stubModel{}, 100)
	config.HTTPClient = &http.Client{Transport: modelErr}

	_, err := newMachineryService(q).RunCompactionSync(context.Background(), config)
	if err == nil || !errors.Is(err, cleanupErr) {
		t.Fatalf("RunCompactionSync error = %v, want joined cleanup error", err)
	}
	if modelErr.calls != 1 || q.completed.Status != "error" {
		t.Fatalf("failure terminalization = calls:%d status:%q", modelErr.calls, q.completed.Status)
	}
}

func TestDoCompactionTerminalizesInvalidFinalizerState(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", jsonStr(strings.Repeat("old question ", 20)), 100),
		mkRow(t, "assistant", jsonStr(strings.Repeat("old answer ", 20)), 100),
		mkRow(t, "user", `"current question"`, 100),
	}
	q := &artifactQueries{fakeQueries: &fakeQueries{
		uncompacted: rows,
		finalizeResult: &sqlc.FinalizeCompactionArtifactRow{
			Finalized:      false,
			Status:         "",
			RequestedCount: 2,
			MatchedCount:   2,
			ClaimedCount:   0,
		},
	}}

	_, err := newMachineryService(q).RunCompactionSync(context.Background(), machineryConfig(&stubModel{summary: "SUMMARY"}, 100))
	if err == nil || !strings.Contains(err.Error(), "finalized=false") {
		t.Fatalf("RunCompactionSync error = %v, want invalid finalizer state", err)
	}
	if q.completed.Status != "error" {
		t.Fatalf("invalid finalizer state left pending log: completion=%q", q.completed.Status)
	}
}

func TestDoCompactionRejectsMissingSourceVersionBeforeAttempt(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", jsonStr(strings.Repeat("old question ", 20)), 100),
		mkRow(t, "assistant", jsonStr(strings.Repeat("old answer ", 20)), 100),
		mkRow(t, "user", `"current question"`, 100),
	}
	rows[0].SourceVersion = ""
	q := &artifactQueries{fakeQueries: &fakeQueries{uncompacted: rows}}
	stub := &stubModel{summary: "unused"}

	_, err := newMachineryService(q).RunCompactionSync(context.Background(), machineryConfig(stub, 100))
	if err == nil || !strings.Contains(err.Error(), "has no version") {
		t.Fatalf("RunCompactionSync error = %v, want missing source version", err)
	}
	if stub.calls != 0 || q.created || q.finalizeCalls != 0 {
		t.Fatalf("missing snapshot started attempt: calls=%d created=%v finalized=%d", stub.calls, q.created, q.finalizeCalls)
	}
}
