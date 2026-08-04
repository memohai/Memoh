package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	contextfrag "github.com/memohai/memoh/internal/agent/context/fragment"
	"github.com/memohai/memoh/internal/agent/runtime/native"
	"github.com/memohai/memoh/internal/apperror"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

const (
	lifecycleTestRunID     = "11111111-1111-4111-8111-111111111111"
	lifecycleTestBotID     = "22222222-2222-4222-8222-222222222222"
	lifecycleTestSessionID = "33333333-3333-4333-8333-333333333333"
)

type recordingContextLifecycleStore struct {
	creates     []sqlc.CreateContextLifecycleParams
	createErr   error
	existing    *sqlc.ContextLifecycle
	getErr      error
	getCalls    int
	updates     []sqlc.UpdateAbortedContextLifecycleSnapshotParams
	updateErr   error
	metadata    []byte
	metadataErr error
	upserts     []sqlc.UpsertAbortedContextLifecycleParams
	upsertErr   error
}

func (s *recordingContextLifecycleStore) CreateContextLifecycle(
	_ context.Context,
	arg sqlc.CreateContextLifecycleParams,
) (sqlc.ContextLifecycle, error) {
	s.creates = append(s.creates, arg)
	if s.createErr != nil {
		return sqlc.ContextLifecycle{}, s.createErr
	}
	return sqlc.ContextLifecycle{
		RunID: arg.RunID, BotID: arg.BotID, SessionID: arg.SessionID,
		Status: arg.Status, ErrorCode: arg.ErrorCode, Snapshot: arg.Snapshot,
	}, nil
}

func (s *recordingContextLifecycleStore) GetContextLifecycleByRunID(
	_ context.Context,
	_ pgtype.UUID,
) (sqlc.ContextLifecycle, error) {
	s.getCalls++
	if s.getErr != nil {
		return sqlc.ContextLifecycle{}, s.getErr
	}
	if s.existing == nil {
		return sqlc.ContextLifecycle{}, pgx.ErrNoRows
	}
	return *s.existing, nil
}

func (s *recordingContextLifecycleStore) GetLatestAssistantContextLifecycleMetadataByRunID(
	_ context.Context,
	_ pgtype.UUID,
) ([]byte, error) {
	return s.metadata, s.metadataErr
}

func (s *recordingContextLifecycleStore) UpdateAbortedContextLifecycleSnapshot(
	_ context.Context,
	arg sqlc.UpdateAbortedContextLifecycleSnapshotParams,
) (sqlc.ContextLifecycle, error) {
	s.updates = append(s.updates, arg)
	return sqlc.ContextLifecycle{}, s.updateErr
}

func (s *recordingContextLifecycleStore) UpsertAbortedContextLifecycle(
	_ context.Context,
	arg sqlc.UpsertAbortedContextLifecycleParams,
) (sqlc.ContextLifecycle, error) {
	s.upserts = append(s.upserts, arg)
	return sqlc.ContextLifecycle{}, s.upsertErr
}

func lifecycleTestRunConfig() native.RunConfig {
	holder := contextfrag.NewLifecycleHolder()
	holder.SetManifest(contextfrag.Manifest{
		View: contextfrag.ViewRunConfigPreProvider,
		Counts: contextfrag.ManifestCounts{
			Fragments: 2,
			Messages:  1,
			TextBytes: 64,
		},
		Items: []contextfrag.ManifestItem{{ID: "private-content-marker"}},
	})
	return native.RunConfig{
		RunID: lifecycleTestRunID,
		Identity: native.SessionContext{
			BotID:     lifecycleTestBotID,
			SessionID: lifecycleTestSessionID,
		},
		ContextLifecycle: holder,
	}
}

func TestContextLifecycleTerminalWritesAdmittedRunExactlyOnce(t *testing.T) {
	store := &recordingContextLifecycleStore{}
	service := &Service{contextLifecycles: store}
	terminal := service.contextLifecycleTerminal(context.Background(), lifecycleTestRunConfig())

	terminal(nil)
	terminal(apperror.New(apperror.CodeWorkspaceUnreachable, nil))

	if len(store.creates) != 1 {
		t.Fatalf("CreateContextLifecycle calls = %d, want 1", len(store.creates))
	}
	got := store.creates[0]
	if pgUUIDString(got.RunID) != lifecycleTestRunID {
		t.Fatalf("persisted run ID = %q, want %q", pgUUIDString(got.RunID), lifecycleTestRunID)
	}
	if got.Status != contextLifecycleStatusCompleted || got.ErrorCode.Valid {
		t.Fatalf("terminal = (%q, %#v), want completed without error code", got.Status, got.ErrorCode)
	}
	if bytes.Contains(got.Snapshot, []byte("private-content-marker")) || bytes.Contains(got.Snapshot, []byte(`"items"`)) {
		t.Fatalf("content-light snapshot leaked manifest items: %s", got.Snapshot)
	}
}

func TestContextLifecycleTerminalClassifiesOnlyCurrentStackOutcomes(t *testing.T) {
	canceledCtx, cancel := context.WithCancelCause(context.Background())
	cancel(context.Canceled)

	tests := []struct {
		name      string
		ctx       context.Context
		cause     error
		status    string
		errorCode string
	}{
		{name: "completed", ctx: context.Background(), status: contextLifecycleStatusCompleted},
		{
			name:      "provider failure with stable code",
			ctx:       context.Background(),
			cause:     apperror.New(apperror.CodeWorkspaceUnreachable, nil),
			status:    contextLifecycleStatusFailedProvider,
			errorCode: string(apperror.CodeWorkspaceUnreachable),
		},
		{
			name:   "provider cancellation while owner remains active",
			ctx:    context.Background(),
			cause:  context.Canceled,
			status: contextLifecycleStatusFailedProvider,
		},
		{
			name:   "explicit abort",
			ctx:    canceledCtx,
			cause:  context.Canceled,
			status: contextLifecycleStatusAborted,
		},
		{
			name:   "explicit abort behind private application cause",
			ctx:    canceledCtx,
			cause:  apperror.Wrap(apperror.CodeWorkspaceUnreachable, context.Canceled, nil),
			status: contextLifecycleStatusAborted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code := classifyContextLifecycleTerminal(tt.ctx, tt.cause)
			if status != tt.status || code != tt.errorCode {
				t.Fatalf("classifyContextLifecycleTerminal() = (%q, %q), want (%q, %q)", status, code, tt.status, tt.errorCode)
			}
		})
	}
}

func TestEnsureTerminalContextLifecycleCreatesMinimalFallbackOnlyWhenMissing(t *testing.T) {
	store := &recordingContextLifecycleStore{}
	service := &Service{contextLifecycles: store}
	cause := apperror.New(apperror.CodeWorkspaceUnreachable, nil)

	service.EnsureTerminalContextLifecycle(
		context.Background(),
		lifecycleTestRunID,
		lifecycleTestBotID,
		lifecycleTestSessionID,
		cause,
	)

	if store.getCalls != 1 || len(store.creates) != 1 {
		t.Fatalf("fallback calls = get %d, create %d; want 1, 1", store.getCalls, len(store.creates))
	}
	row := store.creates[0]
	if row.Status != contextLifecycleStatusFailedProvider || row.ErrorCode.String != string(apperror.CodeWorkspaceUnreachable) {
		t.Fatalf("fallback terminal = (%q, %#v)", row.Status, row.ErrorCode)
	}
	var snapshot contextfrag.LifecycleSnapshot
	if err := json.Unmarshal(row.Snapshot, &snapshot); err != nil {
		t.Fatalf("decode fallback snapshot: %v", err)
	}
	if snapshot.Version != 1 || snapshot.Counts != (contextfrag.ManifestCounts{}) {
		t.Fatalf("minimal fallback snapshot = %#v", snapshot)
	}

	store.creates = nil
	runUUID, botUUID, sessionUUID, err := parseContextLifecycleIDs(
		lifecycleTestRunID,
		lifecycleTestBotID,
		lifecycleTestSessionID,
	)
	if err != nil {
		t.Fatal(err)
	}
	store.existing = &sqlc.ContextLifecycle{RunID: runUUID, BotID: botUUID, SessionID: sessionUUID}
	service.EnsureTerminalContextLifecycle(
		context.Background(),
		lifecycleTestRunID,
		lifecycleTestBotID,
		lifecycleTestSessionID,
		cause,
	)
	if len(store.creates) != 0 {
		t.Fatalf("existing authoritative lifecycle was replaced: %#v", store.creates)
	}
}

func TestContextLifecycleStoreFailureIsCountedAndDoesNotEscape(t *testing.T) {
	store := &recordingContextLifecycleStore{createErr: errors.New("database unavailable")}
	service := &Service{contextLifecycles: store}

	service.contextLifecycleTerminal(context.Background(), lifecycleTestRunConfig())(nil)

	if got := service.contextLifecyclePersistenceErrors.Load(); got != 1 {
		t.Fatalf("persistence failure count = %d, want 1", got)
	}
}

func TestAuthoritativeSnapshotReplacesOnlyRecoveredAbortedFallback(t *testing.T) {
	runUUID, botUUID, sessionUUID, err := parseContextLifecycleIDs(
		lifecycleTestRunID,
		lifecycleTestBotID,
		lifecycleTestSessionID,
	)
	if err != nil {
		t.Fatal(err)
	}
	store := &recordingContextLifecycleStore{
		createErr: &pgconn.PgError{Code: "23505"},
		existing: &sqlc.ContextLifecycle{
			RunID: runUUID, BotID: botUUID, SessionID: sessionUUID,
			Status: contextLifecycleStatusAborted,
		},
	}
	service := &Service{contextLifecycles: store}

	service.contextLifecycleTerminal(context.Background(), lifecycleTestRunConfig())(nil)

	if len(store.updates) != 1 {
		t.Fatalf("aborted snapshot updates = %d, want 1", len(store.updates))
	}
}
