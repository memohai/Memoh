package compaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type fakeQueries struct {
	dbstore.Queries
	uncompacted []sqlc.ListUncompactedMessagesBySessionRow
	priorLogs   []sqlc.BotHistoryMessageCompact

	listPanic  bool
	onComplete func()

	createParams      []sqlc.CreateCompactionLogParams
	createErrors      []error
	createCommits     []bool
	createCtxErrors   []error
	createDeadlines   []bool
	mutateCreatedLog  func(*sqlc.BotHistoryMessageCompact)
	createCalls       int
	created           bool
	markedIDs         []pgtype.UUID
	completed         sqlc.CompleteCompactionLogParams
	finalized         sqlc.FinalizeCompactionArtifactParams
	finalizeResult    *sqlc.FinalizeCompactionArtifactRow
	finalizeErr       error
	finalizeCalls     int
	legacyMarkCalled  bool
	beforeFinalize    func()
	finalizeCtxErr    error
	finalizeDeadline  bool
	completeCtxErr    error
	completeDeadline  bool
	completeErr       error
	createdLog        sqlc.BotHistoryMessageCompact
	reconcileSuccess  bool
	reconcileConflict bool
	getErrors         []error
	getCalls          int
}

var errLegacyCompactionMark = errors.New("legacy compaction mark called")

func TestCreateCompactionAttemptPreservesRetryValidationErrors(t *testing.T) {
	t.Parallel()

	initialErr := errors.New("initial create failed")
	params := sqlc.CreateCompactionLogParams{ID: testUUID(t), BotID: testUUID(t), SessionID: testUUID(t)}
	queries := &fakeQueries{
		createErrors: []error{initialErr},
		mutateCreatedLog: func(row *sqlc.BotHistoryMessageCompact) {
			row.BotID = testUUID(t)
		},
	}

	_, err := newMachineryService(queries).createCompactionAttempt(context.Background(), params)
	if !errors.Is(err, initialErr) || !errors.Is(err, pgx.ErrNoRows) || !strings.Contains(err.Error(), "unexpected persisted row") {
		t.Fatalf("createCompactionAttempt error = %v, want complete retry diagnostics", err)
	}
}

func (f *fakeQueries) CreateCompactionLog(ctx context.Context, arg sqlc.CreateCompactionLogParams) (sqlc.BotHistoryMessageCompact, error) {
	f.createCalls++
	f.createParams = append(f.createParams, arg)
	f.createCtxErrors = append(f.createCtxErrors, ctx.Err())
	_, hasDeadline := ctx.Deadline()
	f.createDeadlines = append(f.createDeadlines, hasDeadline)
	id := arg.ID
	if !id.Valid {
		id = pgtype.UUID{Bytes: uuid.New(), Valid: true}
	}
	callIndex := f.createCalls - 1
	var createErr error
	if callIndex < len(f.createErrors) {
		createErr = f.createErrors[callIndex]
	}
	alreadyCommitted := f.createdLog.ID.Valid && f.createdLog.ID == id
	if alreadyCommitted && createErr == nil {
		createErr = pgx.ErrNoRows
	}
	committed := createErr == nil || (callIndex < len(f.createCommits) && f.createCommits[callIndex])
	if committed && !alreadyCommitted {
		f.created = true
		f.createdLog = sqlc.BotHistoryMessageCompact{
			ID:              id,
			BotID:           arg.BotID,
			SessionID:       arg.SessionID,
			Status:          "pending",
			ArtifactVersion: 1,
			Coverage:        json.RawMessage(`[]`),
			StartedAt:       pgtype.Timestamptz{Valid: true},
		}
		if f.mutateCreatedLog != nil {
			f.mutateCreatedLog(&f.createdLog)
		}
	}
	if createErr != nil {
		return sqlc.BotHistoryMessageCompact{}, createErr
	}
	return f.createdLog, nil
}

func (f *fakeQueries) ListUncompactedMessagesBySession(_ context.Context, _ pgtype.UUID) ([]sqlc.ListUncompactedMessagesBySessionRow, error) {
	if f.listPanic {
		panic("boom: injected query panic")
	}
	return f.uncompacted, nil
}

func (*fakeQueries) ListMessageAssetsBatch(_ context.Context, _ []pgtype.UUID) ([]sqlc.ListMessageAssetsBatchRow, error) {
	return nil, nil
}

func (f *fakeQueries) ListCompactionArtifactLineageBySession(_ context.Context, _ pgtype.UUID) ([]sqlc.BotHistoryMessageCompact, error) {
	return f.priorLogs, nil
}

func (f *fakeQueries) MarkMessagesCompacted(_ context.Context, arg sqlc.MarkMessagesCompactedParams) error {
	f.legacyMarkCalled = true
	return fmt.Errorf("%w: %d messages", errLegacyCompactionMark, len(arg.Column2))
}

func (f *fakeQueries) FinalizeCompactionArtifact(ctx context.Context, arg sqlc.FinalizeCompactionArtifactParams) (sqlc.FinalizeCompactionArtifactRow, error) {
	f.finalizeCalls++
	f.finalized = arg
	if f.beforeFinalize != nil {
		f.beforeFinalize()
	}
	f.finalizeCtxErr = ctx.Err()
	_, f.finalizeDeadline = ctx.Deadline()
	if f.finalizeErr != nil {
		return sqlc.FinalizeCompactionArtifactRow{}, f.finalizeErr
	}
	count := int32(len(arg.MessageIds)) //nolint:gosec // test corpus is bounded
	result := sqlc.FinalizeCompactionArtifactRow{
		Finalized:      true,
		Status:         "ok",
		RequestedCount: count,
		MatchedCount:   count,
		ClaimedCount:   count,
	}
	if f.finalizeResult != nil {
		result = *f.finalizeResult
	}
	if result.Finalized {
		f.markedIDs = append([]pgtype.UUID(nil), arg.MessageIds...)
	}
	return result, nil
}

func (f *fakeQueries) CompleteCompactionLog(ctx context.Context, arg sqlc.CompleteCompactionLogParams) (sqlc.BotHistoryMessageCompact, error) {
	if f.onComplete != nil {
		f.onComplete()
	}
	f.completed = arg
	f.completeCtxErr = ctx.Err()
	_, f.completeDeadline = ctx.Deadline()
	if f.completeErr != nil {
		return sqlc.BotHistoryMessageCompact{}, f.completeErr
	}
	f.createdLog.Status = arg.Status
	f.createdLog.Summary = arg.Summary
	f.createdLog.MessageCount = arg.MessageCount
	f.createdLog.ErrorMessage = arg.ErrorMessage
	f.createdLog.Usage = arg.Usage
	f.createdLog.ModelID = arg.ModelID
	f.createdLog.Coverage = arg.Coverage
	f.createdLog.AnchorStartMs = arg.AnchorStartMs
	f.createdLog.AnchorEndMs = arg.AnchorEndMs
	return f.createdLog, nil
}

func (f *fakeQueries) GetCompactionLogByID(_ context.Context, id pgtype.UUID) (sqlc.BotHistoryMessageCompact, error) {
	f.getCalls++
	if f.getCalls <= len(f.getErrors) && f.getErrors[f.getCalls-1] != nil {
		return sqlc.BotHistoryMessageCompact{}, f.getErrors[f.getCalls-1]
	}
	if !f.createdLog.ID.Valid || f.createdLog.ID != id {
		return sqlc.BotHistoryMessageCompact{}, pgx.ErrNoRows
	}
	if f.reconcileSuccess {
		count := int32(len(f.finalized.MessageIds)) //nolint:gosec // test corpus is bounded
		return sqlc.BotHistoryMessageCompact{
			ID:              f.finalized.CompactID,
			BotID:           f.finalized.BotID,
			SessionID:       f.finalized.SessionID,
			Status:          "ok",
			Summary:         f.finalized.Summary,
			MessageCount:    count,
			Usage:           f.finalized.Usage,
			ModelID:         f.finalized.ModelID,
			ArtifactVersion: 1,
			Coverage:        f.finalized.Coverage,
			AnchorStartMs:   f.finalized.AnchorStartMs,
			AnchorEndMs:     f.finalized.AnchorEndMs,
			ArtifactLevel:   0,
		}, nil
	}
	if f.reconcileConflict {
		return sqlc.BotHistoryMessageCompact{
			ID:              f.finalized.CompactID,
			BotID:           f.finalized.BotID,
			SessionID:       f.finalized.SessionID,
			Status:          "error",
			ErrorMessage:    "compaction source changed before finalization",
			ArtifactVersion: 1,
			Coverage:        json.RawMessage(`[]`),
			ArtifactLevel:   0,
		}, nil
	}
	return f.createdLog, nil
}
