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
			})
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
	svc := NewService(nil, queries)

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
	svc := NewService(nil, queries)

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
	svc := NewService(nil, queries)

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

func TestBranchForkServiceCreatesSiblingBranchesAndSwitches(t *testing.T) {
	ctx := context.Background()
	sessionID := "00000000-0000-0000-0000-000000000100"
	rootBranchID := "00000000-0000-0000-0000-000000000101"
	firstForkID := "00000000-0000-0000-0000-000000000102"
	secondForkID := "00000000-0000-0000-0000-000000000103"
	userID := "00000000-0000-0000-0000-000000000201"
	assistantID := "00000000-0000-0000-0000-000000000202"
	rootAssistantAfterForkID := "00000000-0000-0000-0000-000000000204"
	userMessageID := "00000000-0000-0000-0000-000000000301"
	sessionUUID := mustPGUUID(sessionID)
	rootBranchUUID := mustPGUUID(rootBranchID)
	assistantUUID := mustPGUUID(assistantID)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	queries := &branchServiceQueries{
		sessionID: sessionUUID,
		activeID:  rootBranchUUID,
		nextBranchIDs: []pgtype.UUID{
			mustPGUUID(firstForkID),
			mustPGUUID(secondForkID),
		},
		forkMessages: map[string]sqlc.GetMessageForSessionBranchForkRow{
			assistantID: {
				ID:        assistantUUID,
				SessionID: sessionUUID,
				BranchID:  rootBranchUUID,
				BranchSeq: pgtype.Int8{Int64: 2, Valid: true},
				Role:      "assistant",
				CreatedAt: pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
			},
			userMessageID: {
				ID:        mustPGUUID(userMessageID),
				SessionID: sessionUUID,
				BranchID:  rootBranchUUID,
				BranchSeq: pgtype.Int8{Int64: 1, Valid: true},
				Role:      "user",
				CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			},
		},
		branches: []sqlc.ListSessionBranchesRow{
			{
				ID:             rootBranchUUID,
				SessionID:      sessionUUID,
				CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
				UpdatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
				ActiveBranchID: rootBranchUUID,
			},
		},
		turns: []sqlc.ListSessionBranchTurnMessagesRow{
			{
				AssistantID:          assistantUUID,
				SessionID:            sessionUUID,
				BranchID:             rootBranchUUID,
				BranchSeq:            pgtype.Int8{Int64: 2, Valid: true},
				AssistantDisplayText: pgtype.Text{String: "root answer", Valid: true},
				AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
				UserID:               mustPGUUID(userID),
				UserDisplayText:      pgtype.Text{String: "root question", Valid: true},
				UserCreatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
			},
			{
				AssistantID:          mustPGUUID(rootAssistantAfterForkID),
				SessionID:            sessionUUID,
				BranchID:             rootBranchUUID,
				BranchSeq:            pgtype.Int8{Int64: 4, Valid: true},
				AssistantDisplayText: pgtype.Text{String: "root continuation", Valid: true},
				AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(3 * time.Minute), Valid: true},
			},
		},
	}
	svc := NewService(nil, queries)

	graph, err := svc.ForkBranchFromMessage(ctx, sessionID, assistantID)
	if err != nil {
		t.Fatalf("ForkBranchFromMessage(first) error = %v", err)
	}
	if graph.ActiveBranchID != firstForkID {
		t.Fatalf("active branch = %q, want %q", graph.ActiveBranchID, firstForkID)
	}
	if len(graph.Branches) != 2 {
		t.Fatalf("branches = %d, want 2", len(graph.Branches))
	}
	firstFork := graph.Branches[1]
	if firstFork.ParentBranchID != rootBranchID || firstFork.ForkFromMessageID != assistantID || firstFork.ForkFromSeq != 2 {
		t.Fatalf("first fork = %#v", firstFork)
	}
	pending := findBranchTurn(graph.Turns, "pending-"+firstForkID)
	if pending == nil || !pending.Pending || pending.ParentTurnID != assistantID || !pending.Active {
		t.Fatalf("pending first fork turn = %#v", pending)
	}

	graph, err = svc.ForkBranchFromMessage(ctx, sessionID, assistantID)
	if err != nil {
		t.Fatalf("ForkBranchFromMessage(second) error = %v", err)
	}
	if graph.ActiveBranchID != secondForkID {
		t.Fatalf("active branch = %q, want %q", graph.ActiveBranchID, secondForkID)
	}
	if len(graph.Branches) != 3 {
		t.Fatalf("branches = %d, want 3", len(graph.Branches))
	}
	if graph.Branches[1].ParentBranchID != rootBranchID || graph.Branches[2].ParentBranchID != rootBranchID {
		t.Fatalf("fork branches are not siblings: %#v", graph.Branches)
	}

	graph, err = svc.SetActiveBranch(ctx, sessionID, rootBranchID)
	if err != nil {
		t.Fatalf("SetActiveBranch(root) error = %v", err)
	}
	if graph.ActiveBranchID != rootBranchID {
		t.Fatalf("active branch = %q, want %q", graph.ActiveBranchID, rootBranchID)
	}
	rootAfterFork := findBranchTurn(graph.Turns, rootAssistantAfterForkID)
	if rootAfterFork == nil || !rootAfterFork.Active {
		t.Fatalf("root continuation turn = %#v", rootAfterFork)
	}

	if _, err := svc.ForkBranchFromMessage(ctx, sessionID, userMessageID); !errors.Is(err, ErrForkSourceNotAssistant) {
		t.Fatalf("ForkBranchFromMessage(user) error = %v, want %v", err, ErrForkSourceNotAssistant)
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

type branchServiceQueries struct {
	dbstore.Queries
	sessionID     pgtype.UUID
	activeID      pgtype.UUID
	nextBranchIDs []pgtype.UUID
	forkMessages  map[string]sqlc.GetMessageForSessionBranchForkRow
	branches      []sqlc.ListSessionBranchesRow
	previews      []sqlc.ListSessionBranchPreviewMessagesRow
	turns         []sqlc.ListSessionBranchTurnMessagesRow
}

func (q *branchServiceQueries) GetActiveSessionBranch(context.Context, pgtype.UUID) (pgtype.UUID, error) {
	return q.activeID, nil
}

func (q *branchServiceQueries) GetRootSessionBranch(context.Context, pgtype.UUID) (pgtype.UUID, error) {
	return q.branches[0].ID, nil
}

func (q *branchServiceQueries) SetActiveSessionBranch(_ context.Context, params sqlc.SetActiveSessionBranchParams) error {
	q.activeID = params.BranchID
	for i := range q.branches {
		q.branches[i].ActiveBranchID = params.BranchID
	}
	return nil
}

func (q *branchServiceQueries) GetMessageForSessionBranchFork(_ context.Context, params sqlc.GetMessageForSessionBranchForkParams) (sqlc.GetMessageForSessionBranchForkRow, error) {
	row, ok := q.forkMessages[params.MessageID.String()]
	if !ok {
		return sqlc.GetMessageForSessionBranchForkRow{}, ErrForkMessageNotFound
	}
	return row, nil
}

func (q *branchServiceQueries) CreateSessionBranchFromMessage(_ context.Context, params sqlc.CreateSessionBranchFromMessageParams) (pgtype.UUID, error) {
	if len(q.nextBranchIDs) == 0 {
		return pgtype.UUID{}, errors.New("no branch ids left")
	}
	id := q.nextBranchIDs[0]
	q.nextBranchIDs = q.nextBranchIDs[1:]
	q.branches = append(q.branches, sqlc.ListSessionBranchesRow{
		ID:                id,
		SessionID:         params.SessionID,
		ParentBranchID:    params.ParentBranchID,
		ForkFromMessageID: params.ForkFromMessageID,
		ForkFromSeq:       params.ForkFromSeq,
		CreatedAt:         pgtype.Timestamptz{Time: time.Date(2026, 1, 1, 12, len(q.branches), 0, 0, time.UTC), Valid: true},
		UpdatedAt:         pgtype.Timestamptz{Time: time.Date(2026, 1, 1, 12, len(q.branches), 0, 0, time.UTC), Valid: true},
		ActiveBranchID:    q.activeID,
	})
	return id, nil
}

func (q *branchServiceQueries) ListSessionBranches(context.Context, pgtype.UUID) ([]sqlc.ListSessionBranchesRow, error) {
	rows := make([]sqlc.ListSessionBranchesRow, len(q.branches))
	copy(rows, q.branches)
	for i := range rows {
		rows[i].ActiveBranchID = q.activeID
	}
	return rows, nil
}

func (q *branchServiceQueries) ListSessionBranchPreviewMessages(context.Context, pgtype.UUID) ([]sqlc.ListSessionBranchPreviewMessagesRow, error) {
	return q.previews, nil
}

func (q *branchServiceQueries) ListSessionBranchTurnMessages(context.Context, pgtype.UUID) ([]sqlc.ListSessionBranchTurnMessagesRow, error) {
	return q.turns, nil
}

func findBranchTurn(turns []BranchTurn, id string) *BranchTurn {
	for i := range turns {
		if turns[i].ID == id {
			return &turns[i]
		}
	}
	return nil
}
