package session

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

func TestResolveDescriptorRejectsConflictingACPRuntime(t *testing.T) {
	// type=acp_agent unambiguously means an ACP runtime, so an explicit non-ACP
	// runtime_type is contradictory and must error rather than silently
	// downgrade to a plain model chat session.
	if _, _, _, err := ResolveDescriptor(TypeACPAgent, "", RuntimeModel); err == nil {
		t.Fatal("ResolveDescriptor(acp_agent, model) = nil error, want a conflict error")
	} else if !strings.Contains(err.Error(), "conflicts with runtime_type") {
		t.Fatalf("error = %v, want a 'conflicts with runtime_type' message", err)
	}
	if _, _, _, err := ResolveDescriptor(TypeSchedule, "", RuntimeACPAgent); err == nil {
		t.Fatal("ResolveDescriptor(schedule, acp_agent) = nil error, want an unsupported combination error")
	} else if !strings.Contains(err.Error(), "only supported") {
		t.Fatalf("error = %v, want an 'only supported' message", err)
	}

	gotType, gotMode, gotRuntime, err := ResolveDescriptor(TypeDiscuss, "", RuntimeACPAgent)
	if err != nil {
		t.Fatalf("ResolveDescriptor(discuss, acp_agent) error = %v", err)
	}
	if gotType != TypeDiscuss || gotMode != TypeDiscuss || gotRuntime != RuntimeACPAgent {
		t.Fatalf("ResolveDescriptor(discuss, acp_agent) = (%q,%q,%q), want discuss/discuss/acp_agent",
			gotType, gotMode, gotRuntime)
	}
}

func TestValidateACPMetadata(t *testing.T) {
	tests := []struct {
		name    string
		meta    map[string]any
		wantErr string
	}{
		{
			name: "valid acp_agent_id",
			meta: map[string]any{"acp_agent_id": "codex", "project_path": "/data", "runtime_owner_account_id": "owner-1"},
		},
		{
			name:    "missing agent",
			meta:    map[string]any{"project_path": "/data"},
			wantErr: "acp_agent_id is required",
		},
		{
			name:    "missing project path",
			meta:    map[string]any{"acp_agent_id": "codex", "runtime_owner_account_id": "owner-1"},
			wantErr: "project_path is required",
		},
		{
			name:    "missing runtime owner",
			meta:    map[string]any{"acp_agent_id": "codex", "project_path": "/data"},
			wantErr: "runtime_owner_account_id is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateACPMetadata(tc.meta)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateACPMetadata() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateACPMetadata() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateACPCreatePolicy(t *testing.T) {
	botID := mustPGUUID("00000000-0000-0000-0000-000000000001")
	enabledBot := sqlc.GetBotByIDRow{
		ID: botID,
		Metadata: mustSessionJSON(map[string]any{
			acpprofile.MetadataKeyACP: map[string]any{
				"agents": map[string]any{
					acpprofile.AgentCodexID: map[string]any{"enabled": true, "setup_mode": "self"},
				},
			},
		}),
	}
	disabledBot := enabledBot
	disabledBot.Metadata = mustSessionJSON(map[string]any{
		acpprofile.MetadataKeyACP: map[string]any{
			"agents": map[string]any{
				acpprofile.AgentCodexID: map[string]any{"enabled": false},
			},
		},
	})

	tests := []struct {
		name     string
		bot      sqlc.GetBotByIDRow
		sessions []sqlc.ListSessionsByBotRow
		meta     map[string]any
		wantErr  string
	}{
		{
			name: "enabled",
			bot:  enabledBot,
			meta: map[string]any{"acp_agent_id": "codex", "project_path": "/data"},
		},
		{
			name: "enabled but missing managed configuration",
			bot: sqlc.GetBotByIDRow{
				ID: botID,
				Metadata: mustSessionJSON(map[string]any{
					acpprofile.MetadataKeyACP: map[string]any{
						"agents": map[string]any{
							acpprofile.AgentCodexID: map[string]any{"enabled": true, "setup_mode": "api_key"},
						},
					},
				}),
			},
			meta:    map[string]any{"acp_agent_id": "codex", "project_path": "/data"},
			wantErr: "is not configured",
		},
		{
			name:    "unknown agent",
			bot:     enabledBot,
			meta:    map[string]any{"acp_agent_id": "unknown", "project_path": "/data"},
			wantErr: "unknown ACP agent",
		},
		{
			name:    "bot disabled",
			bot:     disabledBot,
			meta:    map[string]any{"acp_agent_id": "codex", "project_path": "/data"},
			wantErr: "is not enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(nil, acpPolicyQueries{
				bot:      tt.bot,
				sessions: tt.sessions,
			}, nil)
			err := svc.validateACPCreatePolicy(context.Background(), botID, tt.meta)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateACPCreatePolicy() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateACPCreatePolicy() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestCreateACPAgentSessionRunsValidationAndPersistsType(t *testing.T) {
	botID := "00000000-0000-0000-0000-000000000001"
	botUUID := mustPGUUID(botID)
	queries := &createACPQueries{
		bot: sqlc.GetBotByIDRow{
			ID: botUUID,
			Metadata: mustSessionJSON(map[string]any{
				acpprofile.MetadataKeyACP: map[string]any{
					"agents": map[string]any{
						acpprofile.AgentCodexID: map[string]any{"enabled": true, "setup_mode": "self"},
					},
				},
			}),
		},
	}
	svc := NewService(nil, queries, nil)

	created, err := svc.Create(context.Background(), CreateInput{
		BotID:           botID,
		Type:            TypeACPAgent,
		Title:           "Codex",
		CreatedByUserID: "00000000-0000-0000-0000-000000000003",
		Metadata: map[string]any{
			"acp_agent_id":             acpprofile.AgentCodexID,
			"project_path":             "/data/app",
			"runtime_owner_account_id": "00000000-0000-0000-0000-000000000099",
		},
	})
	if err != nil {
		t.Fatalf("Create(acp_agent) error = %v", err)
	}
	if queries.createParams.Type != TypeACPAgent {
		t.Fatalf("CreateSession type = %q, want %q", queries.createParams.Type, TypeACPAgent)
	}
	if created.Type != TypeACPAgent {
		t.Fatalf("created type = %q, want %q", created.Type, TypeACPAgent)
	}
	if queries.createParams.SessionMode != TypeChat || queries.createParams.RuntimeType != RuntimeACPAgent {
		t.Fatalf("CreateSession descriptor = %q/%q, want chat/acp_agent", queries.createParams.SessionMode, queries.createParams.RuntimeType)
	}
	if created.SessionMode != TypeChat || created.RuntimeType != RuntimeACPAgent {
		t.Fatalf("created descriptor = %q/%q, want chat/acp_agent", created.SessionMode, created.RuntimeType)
	}
	if got := created.Metadata["acp_agent_id"]; got != acpprofile.AgentCodexID {
		t.Fatalf("created metadata acp_agent_id = %#v", got)
	}
	if got := created.RuntimeMetadata["acp_agent_id"]; got != acpprofile.AgentCodexID {
		t.Fatalf("created runtime metadata acp_agent_id = %#v", got)
	}
	if got := created.RuntimeMetadata["runtime_owner_account_id"]; got != "00000000-0000-0000-0000-000000000003" {
		t.Fatalf("created runtime owner = %#v, want server owner", got)
	}
}

func TestCreateWithDefaultHeadCreatesTurnHeadInTransaction(t *testing.T) {
	botID := "00000000-0000-0000-0000-000000000001"
	headID := "00000000-0000-0000-0000-000000000003"
	queries := &createTurnHeadQueries{}
	svc := NewService(nil, queries, nil)

	created, err := svc.Create(context.Background(), CreateInput{
		BotID:             botID,
		Type:              TypeChat,
		Title:             "Fork",
		DefaultHeadTurnID: headID,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !queries.inTx {
		t.Fatal("Create() did not use transaction")
	}
	if created.DefaultHeadTurnID != headID {
		t.Fatalf("DefaultHeadTurnID = %q, want %q", created.DefaultHeadTurnID, headID)
	}
	if queries.createdHead.SessionID != queries.createdSessionID {
		t.Fatalf("turn head session = %v, want created session %v", queries.createdHead.SessionID, queries.createdSessionID)
	}
	if queries.createdHead.HeadTurnID.String() != headID {
		t.Fatalf("turn head = %s, want %s", queries.createdHead.HeadTurnID.String(), headID)
	}
}

func TestUpdateTypeAndMetadataACPAgentRunsPolicy(t *testing.T) {
	botID := "00000000-0000-0000-0000-000000000001"
	sessionID := "00000000-0000-0000-0000-000000000002"
	botUUID := mustPGUUID(botID)
	sessionUUID := mustPGUUID(sessionID)
	queries := &updateACPQueries{
		bot: sqlc.GetBotByIDRow{
			ID: botUUID,
			Metadata: mustSessionJSON(map[string]any{
				acpprofile.MetadataKeyACP: map[string]any{
					"agents": map[string]any{
						acpprofile.AgentCodexID: map[string]any{"enabled": true, "setup_mode": "self"},
					},
				},
			}),
		},
		session: sqlc.BotSession{
			ID:    sessionUUID,
			BotID: botUUID,
			Type:  TypeACPAgent,
		},
	}
	svc := NewService(nil, queries, nil)

	updated, err := svc.UpdateTypeAndMetadataWithOwner(context.Background(), sessionID, TypeACPAgent, map[string]any{
		"acp_agent_id":             acpprofile.AgentCodexID,
		"project_path":             "/data/app",
		"runtime_owner_account_id": "00000000-0000-0000-0000-000000000099",
	}, "00000000-0000-0000-0000-000000000003")
	if err != nil {
		t.Fatalf("UpdateTypeAndMetadata(acp_agent) error = %v", err)
	}
	if queries.updateParams.Type != TypeACPAgent {
		t.Fatalf("UpdateSessionTypeAndMetadata type = %q, want %q", queries.updateParams.Type, TypeACPAgent)
	}
	if updated.Type != TypeACPAgent {
		t.Fatalf("updated type = %q, want %q", updated.Type, TypeACPAgent)
	}
	if queries.updateParams.SessionMode != TypeChat || queries.updateParams.RuntimeType != RuntimeACPAgent {
		t.Fatalf("UpdateSessionTypeAndMetadata descriptor = %q/%q, want chat/acp_agent", queries.updateParams.SessionMode, queries.updateParams.RuntimeType)
	}
	if updated.SessionMode != TypeChat || updated.RuntimeType != RuntimeACPAgent {
		t.Fatalf("updated descriptor = %q/%q, want chat/acp_agent", updated.SessionMode, updated.RuntimeType)
	}
	if updated.RuntimeMetadata["runtime_owner_account_id"] != "00000000-0000-0000-0000-000000000003" {
		t.Fatalf("updated runtime owner = %#v, want server owner", updated.RuntimeMetadata["runtime_owner_account_id"])
	}
}

type acpPolicyQueries struct {
	dbstore.Queries
	bot      sqlc.GetBotByIDRow
	sessions []sqlc.ListSessionsByBotRow
}

func (q acpPolicyQueries) GetBotByID(context.Context, pgtype.UUID) (sqlc.GetBotByIDRow, error) {
	return q.bot, nil
}

func (q acpPolicyQueries) ListSessionsByBot(context.Context, pgtype.UUID) ([]sqlc.ListSessionsByBotRow, error) {
	return q.sessions, nil
}

type createACPQueries struct {
	dbstore.Queries
	bot          sqlc.GetBotByIDRow
	sessions     []sqlc.ListSessionsByBotRow
	createParams sqlc.CreateSessionParams
}

type createTurnHeadQueries struct {
	dbstore.Queries
	inTx             bool
	createdSessionID pgtype.UUID
	createdHead      sqlc.CreateSessionTurnHeadParams
}

func (q *createTurnHeadQueries) RunInTx(_ context.Context, fn func(dbstore.Queries) error) error {
	q.inTx = true
	return fn(q)
}

func (q *createTurnHeadQueries) CreateSession(_ context.Context, params sqlc.CreateSessionParams) (sqlc.BotSession, error) {
	q.createdSessionID = mustPGUUID("00000000-0000-0000-0000-000000000002")
	return sqlc.BotSession{
		ID:                q.createdSessionID,
		BotID:             params.BotID,
		Type:              params.Type,
		Title:             params.Title,
		Metadata:          params.Metadata,
		DefaultHeadTurnID: params.DefaultHeadTurnID,
		CreatedAt:         pgtype.Timestamptz{Valid: true},
		UpdatedAt:         pgtype.Timestamptz{Valid: true},
	}, nil
}

func (q *createTurnHeadQueries) CreateSessionTurnHead(_ context.Context, params sqlc.CreateSessionTurnHeadParams) (sqlc.BotSessionTurnHead, error) {
	q.createdHead = params
	return sqlc.BotSessionTurnHead{
		SessionID:  params.SessionID,
		HeadTurnID: params.HeadTurnID,
	}, nil
}

func (q *createACPQueries) GetBotByID(context.Context, pgtype.UUID) (sqlc.GetBotByIDRow, error) {
	return q.bot, nil
}

func (q *createACPQueries) ListSessionsByBot(context.Context, pgtype.UUID) ([]sqlc.ListSessionsByBotRow, error) {
	return q.sessions, nil
}

func (q *createACPQueries) CreateSession(_ context.Context, params sqlc.CreateSessionParams) (sqlc.BotSession, error) {
	q.createParams = params
	return sqlc.BotSession{
		ID:              mustPGUUID("00000000-0000-0000-0000-000000000002"),
		BotID:           params.BotID,
		RouteID:         params.RouteID,
		ChannelType:     params.ChannelType,
		Type:            params.Type,
		SessionMode:     params.SessionMode,
		RuntimeType:     params.RuntimeType,
		RuntimeMetadata: params.RuntimeMetadata,
		Title:           params.Title,
		Metadata:        params.Metadata,
		ParentSessionID: params.ParentSessionID,
		CreatedByUserID: params.CreatedByUserID,
	}, nil
}

type updateACPQueries struct {
	dbstore.Queries
	bot          sqlc.GetBotByIDRow
	session      sqlc.BotSession
	sessions     []sqlc.ListSessionsByBotRow
	updateParams sqlc.UpdateSessionTypeAndMetadataParams
}

func (q *updateACPQueries) GetBotByID(context.Context, pgtype.UUID) (sqlc.GetBotByIDRow, error) {
	return q.bot, nil
}

func (q *updateACPQueries) GetSessionByID(context.Context, pgtype.UUID) (sqlc.BotSession, error) {
	return q.session, nil
}

func (q *updateACPQueries) ListSessionsByBot(context.Context, pgtype.UUID) ([]sqlc.ListSessionsByBotRow, error) {
	return q.sessions, nil
}

func (q *updateACPQueries) UpdateSessionTypeAndMetadata(_ context.Context, params sqlc.UpdateSessionTypeAndMetadataParams) (sqlc.BotSession, error) {
	q.updateParams = params
	updated := q.session
	updated.Type = params.Type
	updated.SessionMode = params.SessionMode
	updated.RuntimeType = params.RuntimeType
	updated.RuntimeMetadata = params.RuntimeMetadata
	updated.Metadata = params.Metadata
	return updated, nil
}

func mustPGUUID(value string) pgtype.UUID {
	var out pgtype.UUID
	if err := out.Scan(value); err != nil {
		panic(err)
	}
	return out
}

func mustSessionJSON(value map[string]any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func TestSoftDeleteCleansOnlyUnsharedVisibleTurns(t *testing.T) {
	sessionID := "00000000-0000-0000-0000-000000000010"
	sharedTurnID := mustPGUUID("00000000-0000-0000-0000-000000000011")
	privateTurnID := mustPGUUID("00000000-0000-0000-0000-000000000012")
	ancestorTurnID := mustPGUUID("00000000-0000-0000-0000-000000000013")
	queries := &softDeleteCleanupQueries{
		ownedTurns: []sqlc.BotHistoryTurn{
			{ID: privateTurnID},
			{ID: sharedTurnID},
			{ID: ancestorTurnID},
		},
		sharedTurnIDs: []pgtype.UUID{sharedTurnID},
	}
	svc := NewService(nil, queries, nil)

	if err := svc.SoftDelete(context.Background(), sessionID); err != nil {
		t.Fatalf("SoftDelete() error = %v", err)
	}
	if !queries.softDeleted {
		t.Fatal("session was not soft deleted")
	}
	if !queries.deletedHeads {
		t.Fatal("session turn heads were not deleted")
	}
	wantDeleted := []pgtype.UUID{privateTurnID, ancestorTurnID}
	if !sameUUIDSlice(queries.deletedMessageTurns, wantDeleted) {
		t.Fatalf("deleted message turns = %v, want %v", queries.deletedMessageTurns, wantDeleted)
	}
	if !sameUUIDSlice(queries.deletedTurns, wantDeleted) {
		t.Fatalf("deleted turns = %v, want %v", queries.deletedTurns, wantDeleted)
	}
}

func TestSoftDeletePropagatesTurnCleanupFailure(t *testing.T) {
	sessionID := "00000000-0000-0000-0000-000000000010"
	cleanupErr := errors.New("cleanup failed")
	queries := &softDeleteCleanupQueries{
		deleteHeadsErr: cleanupErr,
	}
	svc := NewService(nil, queries, nil)

	err := svc.SoftDelete(context.Background(), sessionID)
	if err == nil || !strings.Contains(err.Error(), "delete session turn heads") {
		t.Fatalf("SoftDelete() error = %v, want cleanup failure", err)
	}
	if !queries.softDeleted {
		t.Fatal("session was not soft deleted before cleanup")
	}
}

type softDeleteCleanupQueries struct {
	dbstore.Queries
	softDeleted         bool
	deletedHeads        bool
	ownedTurns          []sqlc.BotHistoryTurn
	sharedTurnIDs       []pgtype.UUID
	deletedMessageTurns []pgtype.UUID
	deletedTurns        []pgtype.UUID
	deleteHeadsErr      error
}

func (q *softDeleteCleanupQueries) SoftDeleteSession(context.Context, pgtype.UUID) error {
	q.softDeleted = true
	return nil
}

func (q *softDeleteCleanupQueries) DeleteSessionTurnHeads(context.Context, pgtype.UUID) error {
	q.deletedHeads = true
	return q.deleteHeadsErr
}

func (q *softDeleteCleanupQueries) ListSessionOwnedTurnsForCleanup(context.Context, pgtype.UUID) ([]sqlc.BotHistoryTurn, error) {
	return q.ownedTurns, nil
}

func (q *softDeleteCleanupQueries) ListOtherActiveSessionVisibleTurnIDs(context.Context, pgtype.UUID) ([]pgtype.UUID, error) {
	return q.sharedTurnIDs, nil
}

func (q *softDeleteCleanupQueries) DeleteMessagesByTurnID(_ context.Context, id pgtype.UUID) error {
	q.deletedMessageTurns = append(q.deletedMessageTurns, id)
	return nil
}

func (q *softDeleteCleanupQueries) DeleteHistoryTurnByID(_ context.Context, id pgtype.UUID) error {
	q.deletedTurns = append(q.deletedTurns, id)
	return nil
}

func sameUUIDSlice(got, want []pgtype.UUID) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// pagedQueriesStub captures the params handed to the paged session queries so
// the test can assert that the service forwards botID, types, cursor, and
// limit without modification.
type pagedQueriesStub struct {
	dbstore.Queries
	pagedArg     sqlc.ListSessionsByBotPagedParams
	pagedRows    []sqlc.ListSessionsByBotPagedRow
	userPagedArg sqlc.ListSessionsByBotAndCreatedByUserPagedParams
	userRows     []sqlc.ListSessionsByBotAndCreatedByUserPagedRow
}

func (s *pagedQueriesStub) ListSessionsByBotPaged(_ context.Context, arg sqlc.ListSessionsByBotPagedParams) ([]sqlc.ListSessionsByBotPagedRow, error) {
	s.pagedArg = arg
	return s.pagedRows, nil
}

func (s *pagedQueriesStub) ListSessionsByBotAndCreatedByUserPaged(_ context.Context, arg sqlc.ListSessionsByBotAndCreatedByUserPagedParams) ([]sqlc.ListSessionsByBotAndCreatedByUserPagedRow, error) {
	s.userPagedArg = arg
	return s.userRows, nil
}

func TestListByBotPagedForwardsParams(t *testing.T) {
	stub := &pagedQueriesStub{}
	svc := NewService(nil, stub, nil)
	botID := "11111111-1111-1111-1111-111111111111"
	cursorID := "22222222-2222-2222-2222-222222222222"
	cursorAt := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	types := []string{TypeChat, TypeDiscuss}

	if _, err := svc.ListByBotPaged(context.Background(), botID, types, SessionCursor{UpdatedAt: cursorAt, ID: cursorID}, 25); err != nil {
		t.Fatalf("ListByBotPaged: %v", err)
	}
	if stub.pagedArg.BotID.String() != botID {
		t.Fatalf("BotID = %s, want %s", stub.pagedArg.BotID.String(), botID)
	}
	if got, want := stub.pagedArg.Types, types; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Types = %v, want %v", got, want)
	}
	if !stub.pagedArg.UseCursor {
		t.Fatalf("UseCursor should be true when a non-zero cursor is supplied")
	}
	if !stub.pagedArg.CursorUpdatedAt.Time.Equal(cursorAt) {
		t.Fatalf("CursorUpdatedAt = %v, want %v", stub.pagedArg.CursorUpdatedAt.Time, cursorAt)
	}
	if stub.pagedArg.CursorID.String() != cursorID {
		t.Fatalf("CursorID = %s, want %s", stub.pagedArg.CursorID.String(), cursorID)
	}
	if stub.pagedArg.LimitCount != 25 {
		t.Fatalf("LimitCount = %d, want 25", stub.pagedArg.LimitCount)
	}
}

func TestListByBotPagedWithFilterForwardsParentSessionID(t *testing.T) {
	stub := &pagedQueriesStub{}
	svc := NewService(nil, stub, nil)
	botID := "11111111-1111-1111-1111-111111111111"
	parentID := "22222222-2222-2222-2222-222222222222"

	if _, err := svc.ListByBotPagedWithFilter(context.Background(), botID, []string{TypeSubagent}, SessionCursor{}, 25, ListFilter{
		ParentSessionID: parentID,
	}); err != nil {
		t.Fatalf("ListByBotPagedWithFilter: %v", err)
	}
	if !stub.pagedArg.UseParentSession {
		t.Fatalf("UseParentSession should be true when a parent session filter is supplied")
	}
	if stub.pagedArg.ParentSessionID.String() != parentID {
		t.Fatalf("ParentSessionID = %s, want %s", stub.pagedArg.ParentSessionID.String(), parentID)
	}
}

func TestListByBotPagedMapsRowsToSessions(t *testing.T) {
	rowID := "33333333-3333-3333-3333-333333333333"
	updated := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	stub := &pagedQueriesStub{
		pagedRows: []sqlc.ListSessionsByBotPagedRow{{
			ID:        mustPGUUID(rowID),
			BotID:     mustPGUUID("11111111-1111-1111-1111-111111111111"),
			Type:      TypeChat,
			Title:     "hello",
			Metadata:  []byte(`{"k":"v"}`),
			CreatedAt: pgtype.Timestamptz{Time: updated, Valid: true},
			UpdatedAt: pgtype.Timestamptz{Time: updated, Valid: true},
		}},
	}
	svc := NewService(nil, stub, nil)

	got, err := svc.ListByBotPaged(context.Background(), "11111111-1111-1111-1111-111111111111", []string{TypeChat}, SessionCursor{}, 10)
	if err != nil {
		t.Fatalf("ListByBotPaged: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if got[0].ID != rowID {
		t.Fatalf("session ID = %s, want %s", got[0].ID, rowID)
	}
	if !got[0].UpdatedAt.Equal(updated) {
		t.Fatalf("session UpdatedAt = %v, want %v", got[0].UpdatedAt, updated)
	}
	if got[0].Metadata["k"] != "v" {
		t.Fatalf("session metadata = %v, want k=v", got[0].Metadata)
	}
}

func TestListByBotAndCreatedByUserPagedForwardsUserScope(t *testing.T) {
	stub := &pagedQueriesStub{}
	svc := NewService(nil, stub, nil)
	botID := "11111111-1111-1111-1111-111111111111"
	userID := "44444444-4444-4444-4444-444444444444"

	if _, err := svc.ListByBotAndCreatedByUserPaged(context.Background(), botID, userID, []string{TypeChat}, SessionCursor{}, 5); err != nil {
		t.Fatalf("ListByBotAndCreatedByUserPaged: %v", err)
	}
	if stub.userPagedArg.BotID.String() != botID {
		t.Fatalf("BotID = %s, want %s", stub.userPagedArg.BotID.String(), botID)
	}
	if stub.userPagedArg.CreatedByUserID.String() != userID {
		t.Fatalf("CreatedByUserID = %s, want %s", stub.userPagedArg.CreatedByUserID.String(), userID)
	}
	if stub.userPagedArg.LimitCount != 5 {
		t.Fatalf("LimitCount = %d, want 5", stub.userPagedArg.LimitCount)
	}
}

func TestListByBotPagedRejectsPartialCursor(t *testing.T) {
	stub := &pagedQueriesStub{}
	svc := NewService(nil, stub, nil)
	botID := "11111111-1111-1111-1111-111111111111"

	_, err := svc.ListByBotPaged(context.Background(), botID, []string{TypeChat},
		SessionCursor{ID: "33333333-3333-3333-3333-333333333333"}, 10)
	if err == nil {
		t.Fatalf("ListByBotPaged with id-only cursor should error rather than restart from the head")
	}
	if !strings.Contains(err.Error(), "cursor must carry both") {
		t.Fatalf("error = %v, want partial-cursor message", err)
	}
}
