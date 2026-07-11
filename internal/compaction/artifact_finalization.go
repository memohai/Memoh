package compaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

const compactionPersistenceTimeout = 30 * time.Second

const compactionSourceChangedErrorMessage = "compaction source changed before finalization"

// ErrCompactionSourceChanged identifies an optimistic finalization miss after
// the selected source snapshot changed while its summary was being generated.
var ErrCompactionSourceChanged = errors.New("compaction: source changed before artifact finalization")

type artifactFinalizationInput struct {
	artifactMetadata
	MessageIDs         []pgtype.UUID
	SourceVersions     []string
	ExpectedCompactIDs []string
}

func artifactFinalizationInputFor(
	items []CompactionCandidate,
	rows []sqlc.ListUncompactedMessagesBySessionRow,
	ids []pgtype.UUID,
) (artifactFinalizationInput, error) {
	metadata, err := artifactMetadataFor(items, ids)
	if err != nil {
		return artifactFinalizationInput{}, err
	}
	covered, err := DecodeArtifactCoverage(metadata.Coverage)
	if err != nil {
		return artifactFinalizationInput{}, fmt.Errorf("validate compaction artifact coverage: %w", err)
	}
	if len(covered) != len(ids) {
		return artifactFinalizationInput{}, fmt.Errorf(
			"compaction artifact coverage count %d does not match source count %d",
			len(covered),
			len(ids),
		)
	}
	if len(covered) == 0 {
		return artifactFinalizationInput{}, errors.New("compaction artifact coverage is empty")
	}
	if metadata.AnchorStartMs != covered[0].CreatedAtMs || metadata.AnchorEndMs != covered[len(covered)-1].CreatedAtMs {
		return artifactFinalizationInput{}, errors.New("compaction artifact anchors do not match coverage")
	}
	selectedIDs := make(map[pgtype.UUID]struct{}, len(ids))
	for index, source := range covered {
		if _, duplicate := selectedIDs[ids[index]]; duplicate {
			return artifactFinalizationInput{}, fmt.Errorf("duplicate selected compaction source %s", formatUUID(ids[index]))
		}
		selectedIDs[ids[index]] = struct{}{}
		if source.Ref.Namespace != "bot_history_message" || source.Ref.ID != formatUUID(ids[index]) {
			return artifactFinalizationInput{}, fmt.Errorf(
				"compaction artifact coverage %d identifies %s/%s instead of message %s",
				index,
				source.Ref.Namespace,
				source.Ref.ID,
				formatUUID(ids[index]),
			)
		}
	}

	rowsByID := make(map[pgtype.UUID]sqlc.ListUncompactedMessagesBySessionRow, len(rows))
	for _, row := range rows {
		if _, exists := rowsByID[row.ID]; exists {
			return artifactFinalizationInput{}, fmt.Errorf("duplicate compaction source snapshot %s", formatUUID(row.ID))
		}
		rowsByID[row.ID] = row
	}
	input := artifactFinalizationInput{
		artifactMetadata:   metadata,
		MessageIDs:         append([]pgtype.UUID(nil), ids...),
		SourceVersions:     make([]string, 0, len(ids)),
		ExpectedCompactIDs: make([]string, 0, len(ids)),
	}
	for _, id := range ids {
		row, ok := rowsByID[id]
		if !ok {
			return artifactFinalizationInput{}, fmt.Errorf("compaction source snapshot %s is missing", formatUUID(id))
		}
		version := strings.TrimSpace(row.SourceVersion)
		if version == "" {
			return artifactFinalizationInput{}, fmt.Errorf("compaction source snapshot %s has no version", formatUUID(id))
		}
		input.SourceVersions = append(input.SourceVersions, version)
		input.ExpectedCompactIDs = append(input.ExpectedCompactIDs, formatUUID(row.CompactID))
	}
	return input, nil
}

type compactionSourceChangedError struct {
	Requested int32
	Matched   int32
	Claimed   int32
}

func (e *compactionSourceChangedError) Error() string {
	return fmt.Sprintf(
		"%s: requested=%d matched=%d claimed=%d",
		ErrCompactionSourceChanged,
		e.Requested,
		e.Matched,
		e.Claimed,
	)
}

func (*compactionSourceChangedError) Unwrap() error {
	return ErrCompactionSourceChanged
}

func validateArtifactFinalization(result sqlc.FinalizeCompactionArtifactRow, expectedCount int) error {
	if !result.Finalized {
		if result.Status != "error" {
			return fmt.Errorf(
				"compaction artifact finalizer returned finalized=false with status %q",
				result.Status,
			)
		}
		if int(result.RequestedCount) != expectedCount ||
			result.ClaimedCount != 0 ||
			result.MatchedCount < 0 ||
			result.MatchedCount >= result.RequestedCount {
			return fmt.Errorf(
				"compaction artifact finalizer returned inconsistent conflict: requested=%d matched=%d claimed=%d expected=%d",
				result.RequestedCount,
				result.MatchedCount,
				result.ClaimedCount,
				expectedCount,
			)
		}
		return &compactionSourceChangedError{
			Requested: result.RequestedCount,
			Matched:   result.MatchedCount,
			Claimed:   result.ClaimedCount,
		}
	}
	if result.Status != "ok" ||
		int(result.RequestedCount) != expectedCount ||
		int(result.MatchedCount) != expectedCount ||
		int(result.ClaimedCount) != expectedCount {
		return fmt.Errorf(
			"compaction artifact finalizer returned inconsistent success: status=%q requested=%d matched=%d claimed=%d expected=%d",
			result.Status,
			result.RequestedCount,
			result.MatchedCount,
			result.ClaimedCount,
			expectedCount,
		)
	}
	return nil
}

func detachedCompactionPersistenceContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), compactionPersistenceTimeout)
}

func (s *Service) finalizeArtifact(ctx context.Context, params sqlc.FinalizeCompactionArtifactParams) error {
	finalizeCtx, cancel := detachedCompactionPersistenceContext(ctx)
	result, err := s.queries.FinalizeCompactionArtifact(finalizeCtx, params)
	cancel()
	if err != nil {
		return s.reconcileFinalizationError(ctx, params, fmt.Errorf("finalize compaction artifact: %w", err))
	}
	if err := validateArtifactFinalization(result, len(params.MessageIds)); err != nil {
		if !result.Finalized && result.Status == "error" {
			return err
		}
		return s.reconcileFinalizationError(ctx, params, err)
	}
	return nil
}

func (s *Service) terminalizeAttemptFailure(ctx context.Context, logID pgtype.UUID, primary error) error {
	completeErr := s.completeLog(ctx, logID, "error", "", primary.Error(), 0, nil, pgtype.UUID{}, nil)
	if completeErr == nil {
		return primary
	}
	row, getErr := s.getCompactionLog(ctx, logID)
	if getErr == nil && row.Status == "error" {
		return primary
	}
	joined := []error{primary, fmt.Errorf("terminalize failed compaction attempt: %w", completeErr)}
	if getErr != nil {
		joined = append(joined, fmt.Errorf("reconcile failed compaction attempt: %w", getErr))
	}
	return errors.Join(joined...)
}

func (s *Service) reconcileFinalizationError(
	ctx context.Context,
	params sqlc.FinalizeCompactionArtifactParams,
	finalizeErr error,
) error {
	row, getErr := s.getCompactionLog(ctx, params.CompactID)
	if getErr == nil {
		if handled, outcome := reconciledFinalizationOutcome(row, params, finalizeErr, nil); handled {
			return outcome
		}
	}

	completeErr := s.completeLog(ctx, params.CompactID, "error", "", finalizeErr.Error(), 0, nil, pgtype.UUID{}, nil)
	if completeErr == nil {
		return finalizeErr
	}
	row, retryErr := s.getCompactionLog(ctx, params.CompactID)
	if retryErr == nil {
		unexpectedStateErr := fmt.Errorf("record compaction finalization failure: %w", completeErr)
		if getErr != nil {
			unexpectedStateErr = errors.Join(
				unexpectedStateErr,
				fmt.Errorf("initial compaction finalization reconciliation: %w", getErr),
			)
		}
		if handled, outcome := reconciledFinalizationOutcome(row, params, finalizeErr, unexpectedStateErr); handled {
			return outcome
		}
	}
	joined := []error{finalizeErr, fmt.Errorf("record compaction finalization failure: %w", completeErr)}
	if getErr != nil {
		joined = append(joined, fmt.Errorf("initial compaction finalization reconciliation: %w", getErr))
	}
	if retryErr != nil {
		joined = append(joined, fmt.Errorf("final compaction finalization reconciliation: %w", retryErr))
	}
	return errors.Join(joined...)
}

func reconciledFinalizationOutcome(
	row sqlc.BotHistoryMessageCompact,
	params sqlc.FinalizeCompactionArtifactParams,
	finalizeErr error,
	unexpectedStateErr error,
) (bool, error) {
	if artifactFinalizationMatches(row, params) {
		return true, nil
	}
	if artifactFinalizationConflictMatches(row, params) {
		return true, fmt.Errorf("%w: persisted conflict recovered after finalization response loss", ErrCompactionSourceChanged)
	}
	if row.Status == "error" {
		return true, finalizeErr
	}
	if row.Status == "ok" {
		return true, errors.Join(
			finalizeErr,
			errors.New("compaction artifact finalization committed with unexpected payload"),
			unexpectedStateErr,
		)
	}
	return false, nil
}

func (s *Service) getCompactionLog(ctx context.Context, id pgtype.UUID) (sqlc.BotHistoryMessageCompact, error) {
	getCtx, cancel := detachedCompactionPersistenceContext(ctx)
	defer cancel()
	return s.queries.GetCompactionLogByID(getCtx, id)
}

func artifactFinalizationMatches(row sqlc.BotHistoryMessageCompact, params sqlc.FinalizeCompactionArtifactParams) bool {
	return row.ID == params.CompactID &&
		row.BotID == params.BotID &&
		row.SessionID == params.SessionID &&
		row.Status == "ok" &&
		row.Summary == params.Summary &&
		int(row.MessageCount) == len(params.MessageIds) &&
		row.ErrorMessage == "" &&
		jsonValuesEqual(row.Usage, params.Usage) &&
		row.ModelID == params.ModelID &&
		row.ArtifactVersion == 1 &&
		jsonValuesEqual(row.Coverage, params.Coverage) &&
		row.AnchorStartMs == params.AnchorStartMs &&
		row.AnchorEndMs == params.AnchorEndMs &&
		row.ArtifactLevel == 0 &&
		len(row.ParentIds) == 0
}

func artifactFinalizationConflictMatches(row sqlc.BotHistoryMessageCompact, params sqlc.FinalizeCompactionArtifactParams) bool {
	return row.ID == params.CompactID &&
		row.BotID == params.BotID &&
		row.SessionID == params.SessionID &&
		row.Status == "error" &&
		row.Summary == "" &&
		row.MessageCount == 0 &&
		row.ErrorMessage == compactionSourceChangedErrorMessage &&
		len(row.Usage) == 0 &&
		!row.ModelID.Valid &&
		row.ArtifactVersion == 1 &&
		jsonValuesEqual(row.Coverage, []byte("[]")) &&
		row.AnchorStartMs == 0 &&
		row.AnchorEndMs == 0 &&
		row.ArtifactLevel == 0 &&
		len(row.ParentIds) == 0
}

func jsonValuesEqual(left, right []byte) bool {
	var leftValue any
	var rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}
