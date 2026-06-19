package session

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/acpprofile"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

func TestIsKnownTypeIncludesACPAgent(t *testing.T) {
	for _, typ := range []string{TypeChat, TypeHeartbeat, TypeSchedule, TypeSubagent, TypeDiscuss, TypeACPAgent} {
		if !IsKnownType(typ) {
			t.Fatalf("IsKnownType(%q) = false", typ)
		}
	}
	if IsKnownType("conversation") {
		t.Fatalf("old conversation-like type should not be accepted")
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
			meta: map[string]any{"acp_agent_id": "codex", "project_path": "/data"},
		},
		{
			name:    "missing agent",
			meta:    map[string]any{"project_path": "/data"},
			wantErr: "acp_agent_id is required",
		},
		{
			name:    "missing project path",
			meta:    map[string]any{"acp_agent_id": "codex"},
			wantErr: "project_path is required",
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
					acpprofile.AgentCodexID: map[string]any{"enabled": true},
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
						acpprofile.AgentCodexID: map[string]any{"enabled": true},
					},
				},
			}),
		},
	}
	svc := NewService(nil, queries, nil)

	created, err := svc.Create(context.Background(), CreateInput{
		BotID: botID,
		Type:  TypeACPAgent,
		Title: "Codex",
		Metadata: map[string]any{
			"acp_agent_id": acpprofile.AgentCodexID,
			"project_path": "/data/app",
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
	if got := created.Metadata["acp_agent_id"]; got != acpprofile.AgentCodexID {
		t.Fatalf("created metadata acp_agent_id = %#v", got)
	}
}

func TestCreateACPAgentSessionDefaultsProjectPath(t *testing.T) {
	botID := "00000000-0000-0000-0000-000000000001"
	botUUID := mustPGUUID(botID)
	queries := &createACPQueries{
		bot: sqlc.GetBotByIDRow{
			ID: botUUID,
			Metadata: mustSessionJSON(map[string]any{
				acpprofile.MetadataKeyACP: map[string]any{
					"agents": map[string]any{
						acpprofile.AgentCodexID: map[string]any{"enabled": true},
					},
				},
			}),
		},
	}
	svc := NewService(nil, queries, nil)

	created, err := svc.Create(context.Background(), CreateInput{
		BotID: botID,
		Type:  TypeACPAgent,
		Title: "Codex",
		Metadata: map[string]any{
			"acp_agent_id": acpprofile.AgentCodexID,
		},
	})
	if err != nil {
		t.Fatalf("Create(acp_agent) error = %v", err)
	}
	if created.Metadata["project_path"] != DefaultACPProjectPath {
		t.Fatalf("created metadata project_path = %#v, want %q", created.Metadata["project_path"], DefaultACPProjectPath)
	}
	if created.Metadata["acp_project_mode"] != DefaultACPProjectMode {
		t.Fatalf("created metadata acp_project_mode = %#v, want %q", created.Metadata["acp_project_mode"], DefaultACPProjectMode)
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
						acpprofile.AgentCodexID: map[string]any{"enabled": true},
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

	updated, err := svc.UpdateTypeAndMetadata(context.Background(), sessionID, TypeACPAgent, map[string]any{
		"acp_agent_id": acpprofile.AgentCodexID,
		"project_path": "/data/app",
	})
	if err != nil {
		t.Fatalf("UpdateTypeAndMetadata(acp_agent) error = %v", err)
	}
	if queries.updateParams.Type != TypeACPAgent {
		t.Fatalf("UpdateSessionTypeAndMetadata type = %q, want %q", queries.updateParams.Type, TypeACPAgent)
	}
	if updated.Type != TypeACPAgent {
		t.Fatalf("updated type = %q, want %q", updated.Type, TypeACPAgent)
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
		Title:           params.Title,
		Metadata:        params.Metadata,
		ParentSessionID: params.ParentSessionID,
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

func TestListByBotPagedZeroCursorSkipsCursorFilter(t *testing.T) {
	stub := &pagedQueriesStub{}
	svc := NewService(nil, stub, nil)

	if _, err := svc.ListByBotPaged(context.Background(), "11111111-1111-1111-1111-111111111111", []string{TypeChat}, SessionCursor{}, 10); err != nil {
		t.Fatalf("ListByBotPaged: %v", err)
	}
	if stub.pagedArg.UseCursor {
		t.Fatalf("UseCursor should be false for the zero-value cursor")
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

// TestSessionCursorIsZeroDistinguishesPartialFromEmpty pins down the contract
// that pagedCursorParams relies on: a partial cursor (only one half set) is
// not zero, so a misconstructed cursor surfaces as an error rather than
// silently restarting the listing from the head.
func TestSessionCursorIsZeroDistinguishesPartialFromEmpty(t *testing.T) {
	if !(SessionCursor{}).IsZero() {
		t.Fatalf("zero-value cursor should be zero")
	}
	partialID := SessionCursor{ID: "33333333-3333-3333-3333-333333333333"}
	if partialID.IsZero() {
		t.Fatalf("cursor with only id set should not be zero")
	}
	partialTS := SessionCursor{UpdatedAt: time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)}
	if partialTS.IsZero() {
		t.Fatalf("cursor with only updated_at set should not be zero")
	}
}

func TestListByBotPagedRejectsPartialCursor(t *testing.T) {
	stub := &pagedQueriesStub{}
	svc := NewService(nil, stub)
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
