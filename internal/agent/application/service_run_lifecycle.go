package application

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	contextfrag "github.com/memohai/memoh/internal/agent/context/fragment"
	"github.com/memohai/memoh/internal/agent/runtime/native"
	sessionruntime "github.com/memohai/memoh/internal/agent/runtime/session"
	"github.com/memohai/memoh/internal/apperror"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

const (
	contextLifecycleStatusCompleted      = "completed"
	contextLifecycleStatusFailedProvider = "failed_provider"
	contextLifecycleStatusAborted        = "aborted"
	contextLifecycleWriteTimeout         = 10 * time.Second
)

type contextLifecycleStore interface {
	CreateContextLifecycle(context.Context, sqlc.CreateContextLifecycleParams) (sqlc.ContextLifecycle, error)
	GetContextLifecycleByRunID(context.Context, pgtype.UUID) (sqlc.ContextLifecycle, error)
	GetLatestAssistantContextLifecycleMetadataByRunID(context.Context, pgtype.UUID) ([]byte, error)
	UpdateAbortedContextLifecycleSnapshot(context.Context, sqlc.UpdateAbortedContextLifecycleSnapshotParams) (sqlc.ContextLifecycle, error)
	UpsertAbortedContextLifecycle(context.Context, sqlc.UpsertAbortedContextLifecycleParams) (sqlc.ContextLifecycle, error)
}

func (s *Service) contextLifecycleTerminal(ctx context.Context, cfg native.RunConfig) func(error) {
	var once sync.Once
	return func(cause error) {
		once.Do(func() {
			s.persistRunContextLifecycle(ctx, cfg, cause)
		})
	}
}

func minimalContextLifecycleSnapshot() contextfrag.LifecycleSnapshot {
	return contextfrag.BuildLifecycleSnapshot(contextfrag.BuildManifest(nil))
}

// EnsureTerminalContextLifecycle records a content-light fallback for runs
// that fail before native context assembly creates a snapshot. A terminal
// writer with an authoritative holder always wins this read-before-create race.
func (s *Service) EnsureTerminalContextLifecycle(
	ctx context.Context,
	runID, botID, sessionID string,
	cause error,
) {
	if s == nil || s.contextLifecycles == nil || contextLifecycleOwnershipLost(ctx, cause) {
		return
	}
	ctx = nonNilContext(ctx)
	snapshot := minimalContextLifecycleSnapshot()
	status, _ := classifyContextLifecycleTerminal(ctx, cause)
	runUUID, botUUID, sessionUUID, err := parseContextLifecycleIDs(runID, botID, sessionID)
	if err != nil {
		s.recordContextLifecyclePersistenceError(err, runID, botID, sessionID, status)
		return
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), contextLifecycleWriteTimeout)
	defer cancel()
	existing, err := s.contextLifecycles.GetContextLifecycleByRunID(readCtx, runUUID)
	if err == nil {
		if existing.BotID != botUUID || existing.SessionID != sessionUUID {
			s.recordContextLifecyclePersistenceError(
				errors.New("existing context lifecycle identity does not match terminal fallback"),
				runID,
				botID,
				sessionID,
				status,
			)
		}
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		s.recordContextLifecyclePersistenceError(err, runID, botID, sessionID, status)
		return
	}
	s.persistContextLifecycleSnapshot(ctx, runID, botID, sessionID, &snapshot, cause, false)
}

func (s *Service) persistRunContextLifecycle(ctx context.Context, cfg native.RunConfig, cause error) {
	if cfg.ContextLifecycle == nil {
		return
	}
	snapshot, ok := cfg.ContextLifecycle.Snapshot()
	if !ok {
		return
	}
	s.persistContextLifecycleSnapshot(
		ctx,
		cfg.RunID,
		cfg.Identity.BotID,
		cfg.Identity.SessionID,
		&snapshot,
		cause,
		true,
	)
}

func (s *Service) persistContextLifecycleSnapshot(
	ctx context.Context,
	runID, botID, sessionID string,
	snapshot *contextfrag.LifecycleSnapshot,
	cause error,
	authoritative bool,
) {
	if s == nil || s.contextLifecycles == nil || snapshot == nil || contextLifecycleOwnershipLost(ctx, cause) {
		return
	}
	ctx = nonNilContext(ctx)
	status, errorCode := classifyContextLifecycleTerminal(ctx, cause)
	runUUID, botUUID, sessionUUID, err := parseContextLifecycleIDs(runID, botID, sessionID)
	if err != nil {
		s.recordContextLifecyclePersistenceError(err, runID, botID, sessionID, status)
		return
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		s.recordContextLifecyclePersistenceError(err, runID, botID, sessionID, status)
		return
	}
	var code pgtype.Text
	if errorCode != "" {
		code = pgtype.Text{String: errorCode, Valid: true}
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), contextLifecycleWriteTimeout)
	defer cancel()
	_, err = s.contextLifecycles.CreateContextLifecycle(writeCtx, sqlc.CreateContextLifecycleParams{
		RunID:     runUUID,
		BotID:     botUUID,
		SessionID: sessionUUID,
		Status:    status,
		ErrorCode: code,
		Snapshot:  raw,
	})
	if err == nil {
		return
	}
	if !db.IsUniqueViolation(err) {
		s.recordContextLifecyclePersistenceError(err, runID, botID, sessionID, status)
		return
	}
	if !authoritative {
		return
	}
	_, err = s.contextLifecycles.UpdateAbortedContextLifecycleSnapshot(
		writeCtx,
		sqlc.UpdateAbortedContextLifecycleSnapshotParams{
			Snapshot:  raw,
			RunID:     runUUID,
			BotID:     botUUID,
			SessionID: sessionUUID,
		},
	)
	if err == nil {
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		existing, getErr := s.contextLifecycles.GetContextLifecycleByRunID(writeCtx, runUUID)
		if getErr == nil && existing.BotID == botUUID && existing.SessionID == sessionUUID {
			return
		}
		if getErr != nil {
			err = getErr
		} else {
			err = errors.New("existing context lifecycle identity does not match terminal write")
		}
	}
	s.recordContextLifecyclePersistenceError(err, runID, botID, sessionID, status)
}

func classifyContextLifecycleTerminal(ctx context.Context, cause error) (string, string) {
	if cause == nil {
		return contextLifecycleStatusCompleted, ""
	}
	privateCause := apperror.CauseOf(cause)
	explicitlyCanceled := errors.Is(context.Cause(nonNilContext(ctx)), context.Canceled) &&
		(errors.Is(cause, context.Canceled) || errors.Is(privateCause, context.Canceled))
	if explicitlyCanceled {
		return contextLifecycleStatusAborted, ""
	}
	return contextLifecycleStatusFailedProvider, string(apperror.CodeOf(cause))
}

func contextLifecycleOwnershipLost(ctx context.Context, cause error) bool {
	return errors.Is(context.Cause(nonNilContext(ctx)), sessionruntime.ErrRunOwnershipLost) ||
		errors.Is(cause, sessionruntime.ErrRunOwnershipLost) ||
		errors.Is(apperror.CauseOf(cause), sessionruntime.ErrRunOwnershipLost)
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func parseContextLifecycleIDs(runID, botID, sessionID string) (pgtype.UUID, pgtype.UUID, pgtype.UUID, error) {
	runUUID, err := db.ParseUUID(runID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, err
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, err
	}
	sessionUUID, err := db.ParseUUID(sessionID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, err
	}
	return runUUID, botUUID, sessionUUID, nil
}

func (s *Service) recordContextLifecyclePersistenceError(
	err error,
	runID, botID, sessionID, status string,
) {
	count := s.contextLifecyclePersistenceErrors.Add(1)
	if s.logger == nil {
		return
	}
	s.logger.Error("persist context lifecycle failed",
		slog.Any("error", err),
		slog.String("run_id", runID),
		slog.String("bot_id", botID),
		slog.String("session_id", sessionID),
		slog.String("status", status),
		slog.Uint64("failure_count", count),
	)
}
