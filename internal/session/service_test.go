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
	rewriteForkID := "00000000-0000-0000-0000-000000000104"
	userID := "00000000-0000-0000-0000-000000000201"
	assistantID := "00000000-0000-0000-0000-000000000202"
	rootAssistantAfterForkID := "00000000-0000-0000-0000-000000000204"
	userMessageID := "00000000-0000-0000-0000-000000000301"
	firstTurnID := "00000000-0000-0000-0000-000000000701"
	secondTurnID := "00000000-0000-0000-0000-000000000702"
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
			mustPGUUID(rewriteForkID),
		},
		forkMessages: map[string]sqlc.GetMessageForSessionBranchForkRow{
			assistantID: {
				ID:        assistantUUID,
				SessionID: sessionUUID,
				BranchID:  rootBranchUUID,
				BranchSeq: pgtype.Int8{Int64: 2, Valid: true},
				TurnID:    mustPGUUID(firstTurnID),
				TurnSeq:   1,
				Role:      "assistant",
				CreatedAt: pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
			},
			userMessageID: {
				ID:        mustPGUUID(userMessageID),
				SessionID: sessionUUID,
				BranchID:  rootBranchUUID,
				BranchSeq: pgtype.Int8{Int64: 1, Valid: true},
				TurnID:    mustPGUUID(firstTurnID),
				TurnSeq:   1,
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
				TurnID:               mustPGUUID(firstTurnID),
				TurnSeq:              1,
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
				TurnID:               mustPGUUID(secondTurnID),
				TurnSeq:              2,
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
	if firstFork.ParentBranchID != rootBranchID || firstFork.ForkFromMessageID != assistantID || firstFork.ForkFromTurnSeq != 1 {
		t.Fatalf("first fork = %#v", firstFork)
	}
	if pending := findBranchTurn(graph.Turns, "pending-"+firstForkID); pending != nil {
		t.Fatalf("fork should not create a pending turn card: %#v", pending)
	}
	if forkTurn := findBranchTurn(graph.Turns, firstTurnID); forkTurn == nil || forkTurn.Active {
		t.Fatalf("forked source turn should remain inactive in graph: %#v", forkTurn)
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
	if graph.Branches[1].ForkFromTurnSeq != 1 || graph.Branches[2].ForkFromTurnSeq != 1 {
		t.Fatalf("fork branches should include the forked turn: %#v", graph.Branches)
	}

	graph, err = svc.SetActiveBranch(ctx, sessionID, rootBranchID)
	if err != nil {
		t.Fatalf("SetActiveBranch(root) error = %v", err)
	}
	if graph.ActiveBranchID != rootBranchID {
		t.Fatalf("active branch = %q, want %q", graph.ActiveBranchID, rootBranchID)
	}
	rootAfterFork := findBranchTurn(graph.Turns, secondTurnID)
	if rootAfterFork == nil || !rootAfterFork.Active {
		t.Fatalf("root continuation turn = %#v", rootAfterFork)
	}

	graph, err = svc.ForkBranchFromMessage(ctx, sessionID, userMessageID)
	if err != nil {
		t.Fatalf("ForkBranchFromMessage(user) error = %v", err)
	}
	if graph.ActiveBranchID != rewriteForkID {
		t.Fatalf("active branch = %q, want %q", graph.ActiveBranchID, rewriteForkID)
	}
	if got := graph.Branches[len(graph.Branches)-1].ForkFromTurnSeq; got != 0 {
		t.Fatalf("rewrite fork_from_turn_seq = %d, want 0", got)
	}
	if graph.Branches[len(graph.Branches)-1].ForkFromSeq != 0 {
		t.Fatalf("rewrite fork_from_seq = %d, want 0", graph.Branches[len(graph.Branches)-1].ForkFromSeq)
	}
}

func TestBranchForkServiceForksLaterReplyFromCurrentTurn(t *testing.T) {
	ctx := context.Background()
	sessionID := "00000000-0000-0000-0000-000000000400"
	rootBranchID := "00000000-0000-0000-0000-000000000401"
	forkID := "00000000-0000-0000-0000-000000000402"
	firstAssistantID := "00000000-0000-0000-0000-000000000501"
	secondAssistantID := "00000000-0000-0000-0000-000000000502"
	forkAssistantID := "00000000-0000-0000-0000-000000000503"
	firstTurnID := "00000000-0000-0000-0000-000000000801"
	secondTurnID := "00000000-0000-0000-0000-000000000802"
	forkTurnID := "00000000-0000-0000-0000-000000000803"
	sessionUUID := mustPGUUID(sessionID)
	rootBranchUUID := mustPGUUID(rootBranchID)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	queries := &branchServiceQueries{
		sessionID:     sessionUUID,
		activeID:      rootBranchUUID,
		nextBranchIDs: []pgtype.UUID{mustPGUUID(forkID)},
		forkMessages: map[string]sqlc.GetMessageForSessionBranchForkRow{
			secondAssistantID: {
				ID:                mustPGUUID(secondAssistantID),
				SessionID:         sessionUUID,
				BranchID:          rootBranchUUID,
				BranchSeq:         pgtype.Int8{Int64: 4, Valid: true},
				TurnID:            mustPGUUID(secondTurnID),
				TurnSeq:           2,
				PreviousTurnID:    mustPGUUID(firstTurnID),
				PreviousTurnSeq:   1,
				PreviousBranchSeq: pgtype.Int8{Int64: 2, Valid: true},
				Role:              "assistant",
				CreatedAt:         pgtype.Timestamptz{Time: now.Add(3 * time.Minute), Valid: true},
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
				TurnID:               mustPGUUID(firstTurnID),
				TurnSeq:              1,
				AssistantID:          mustPGUUID(firstAssistantID),
				SessionID:            sessionUUID,
				BranchID:             rootBranchUUID,
				BranchSeq:            pgtype.Int8{Int64: 2, Valid: true},
				AssistantDisplayText: pgtype.Text{String: "first answer", Valid: true},
				AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
				UserID:               mustPGUUID("00000000-0000-0000-0000-000000000601"),
				UserDisplayText:      pgtype.Text{String: "first question", Valid: true},
				UserCreatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
			},
			{
				TurnID:               mustPGUUID(secondTurnID),
				TurnSeq:              2,
				AssistantID:          mustPGUUID(secondAssistantID),
				SessionID:            sessionUUID,
				BranchID:             rootBranchUUID,
				BranchSeq:            pgtype.Int8{Int64: 4, Valid: true},
				AssistantDisplayText: pgtype.Text{String: "second answer", Valid: true},
				AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(3 * time.Minute), Valid: true},
				UserID:               mustPGUUID("00000000-0000-0000-0000-000000000602"),
				UserDisplayText:      pgtype.Text{String: "second question", Valid: true},
				UserCreatedAt:        pgtype.Timestamptz{Time: now.Add(2 * time.Minute), Valid: true},
			},
			{
				TurnID:               mustPGUUID(forkTurnID),
				TurnSeq:              1,
				AssistantID:          mustPGUUID(forkAssistantID),
				SessionID:            sessionUUID,
				BranchID:             mustPGUUID(forkID),
				BranchSeq:            pgtype.Int8{Int64: 2, Valid: true},
				AssistantDisplayText: pgtype.Text{String: "replacement answer", Valid: true},
				AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(5 * time.Minute), Valid: true},
				UserID:               mustPGUUID("00000000-0000-0000-0000-000000000603"),
				UserDisplayText:      pgtype.Text{String: "replacement question", Valid: true},
				UserCreatedAt:        pgtype.Timestamptz{Time: now.Add(4 * time.Minute), Valid: true},
			},
		},
	}
	svc := NewService(nil, queries)

	graph, err := svc.ForkBranchFromMessage(ctx, sessionID, secondAssistantID)
	if err != nil {
		t.Fatalf("ForkBranchFromMessage(second turn) error = %v", err)
	}
	if graph.ActiveBranchID != forkID {
		t.Fatalf("active branch = %q, want %q", graph.ActiveBranchID, forkID)
	}
	if got := graph.Branches[1].ForkFromTurnSeq; got != 2 {
		t.Fatalf("fork_from_turn_seq = %d, want 2", got)
	}
	replacement := findBranchTurn(graph.Turns, forkTurnID)
	if replacement == nil {
		t.Fatalf("replacement turn not found in graph: %#v", graph.Turns)
	}
	if replacement.ParentTurnID != secondTurnID || !replacement.Active {
		t.Fatalf("replacement turn = %#v", replacement)
	}
}

func TestBranchForkServiceRewritesMiddleRequestFromPreviousTurn(t *testing.T) {
	ctx := context.Background()
	sessionID := "00000000-0000-0000-0000-000000001700"
	rootBranchID := "00000000-0000-0000-0000-000000001701"
	forkID := "00000000-0000-0000-0000-000000001702"
	firstTurnID := "00000000-0000-0000-0000-000000001703"
	secondTurnID := "00000000-0000-0000-0000-000000001704"
	thirdTurnID := "00000000-0000-0000-0000-000000001705"
	requestID := "00000000-0000-0000-0000-000000001706"
	forkTurnID := "00000000-0000-0000-0000-000000001707"
	sessionUUID := mustPGUUID(sessionID)
	rootBranchUUID := mustPGUUID(rootBranchID)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	queries := &branchServiceQueries{
		sessionID:     sessionUUID,
		activeID:      rootBranchUUID,
		nextBranchIDs: []pgtype.UUID{mustPGUUID(forkID)},
		forkMessages: map[string]sqlc.GetMessageForSessionBranchForkRow{
			requestID: {
				ID:                mustPGUUID(requestID),
				SessionID:         sessionUUID,
				BranchID:          rootBranchUUID,
				BranchSeq:         pgtype.Int8{Int64: 3, Valid: true},
				TurnID:            mustPGUUID(secondTurnID),
				TurnSeq:           2,
				PreviousTurnID:    mustPGUUID(firstTurnID),
				PreviousTurnSeq:   1,
				PreviousBranchSeq: pgtype.Int8{Int64: 2, Valid: true},
				Role:              "user",
				CreatedAt:         pgtype.Timestamptz{Time: now.Add(2 * time.Minute), Valid: true},
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
				TurnID:               mustPGUUID(firstTurnID),
				TurnSeq:              1,
				AssistantID:          mustPGUUID("00000000-0000-0000-0000-000000001711"),
				SessionID:            sessionUUID,
				BranchID:             rootBranchUUID,
				BranchSeq:            pgtype.Int8{Int64: 2, Valid: true},
				AssistantDisplayText: pgtype.Text{String: "first answer", Valid: true},
				AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
				UserID:               mustPGUUID("00000000-0000-0000-0000-000000001712"),
				UserDisplayText:      pgtype.Text{String: "first question", Valid: true},
				UserCreatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
			},
			{
				TurnID:               mustPGUUID(secondTurnID),
				TurnSeq:              2,
				AssistantID:          mustPGUUID("00000000-0000-0000-0000-000000001713"),
				SessionID:            sessionUUID,
				BranchID:             rootBranchUUID,
				BranchSeq:            pgtype.Int8{Int64: 4, Valid: true},
				AssistantDisplayText: pgtype.Text{String: "second answer", Valid: true},
				AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(3 * time.Minute), Valid: true},
				UserID:               mustPGUUID(requestID),
				UserDisplayText:      pgtype.Text{String: "second question", Valid: true},
				UserCreatedAt:        pgtype.Timestamptz{Time: now.Add(2 * time.Minute), Valid: true},
			},
			{
				TurnID:               mustPGUUID(thirdTurnID),
				TurnSeq:              3,
				AssistantID:          mustPGUUID("00000000-0000-0000-0000-000000001714"),
				SessionID:            sessionUUID,
				BranchID:             rootBranchUUID,
				BranchSeq:            pgtype.Int8{Int64: 6, Valid: true},
				AssistantDisplayText: pgtype.Text{String: "third answer", Valid: true},
				AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(5 * time.Minute), Valid: true},
				UserID:               mustPGUUID("00000000-0000-0000-0000-000000001715"),
				UserDisplayText:      pgtype.Text{String: "third question", Valid: true},
				UserCreatedAt:        pgtype.Timestamptz{Time: now.Add(4 * time.Minute), Valid: true},
			},
			{
				TurnID:               mustPGUUID(forkTurnID),
				TurnSeq:              1,
				AssistantID:          mustPGUUID("00000000-0000-0000-0000-000000001716"),
				SessionID:            sessionUUID,
				BranchID:             mustPGUUID(forkID),
				BranchSeq:            pgtype.Int8{Int64: 2, Valid: true},
				AssistantDisplayText: pgtype.Text{String: "rewrite answer", Valid: true},
				AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(7 * time.Minute), Valid: true},
				UserID:               mustPGUUID("00000000-0000-0000-0000-000000001717"),
				UserDisplayText:      pgtype.Text{String: "rewritten question", Valid: true},
				UserCreatedAt:        pgtype.Timestamptz{Time: now.Add(6 * time.Minute), Valid: true},
			},
		},
	}
	svc := NewService(nil, queries)

	graph, err := svc.ForkBranchFromMessage(ctx, sessionID, requestID)
	if err != nil {
		t.Fatalf("ForkBranchFromMessage(middle user request) error = %v", err)
	}
	if graph.ActiveBranchID != forkID {
		t.Fatalf("active branch = %q, want %q", graph.ActiveBranchID, forkID)
	}
	rewriteBranch := graph.Branches[len(graph.Branches)-1]
	if rewriteBranch.ParentBranchID != rootBranchID {
		t.Fatalf("rewrite parent branch = %q, want %q", rewriteBranch.ParentBranchID, rootBranchID)
	}
	if rewriteBranch.ForkFromTurnID != firstTurnID || rewriteBranch.ForkFromTurnSeq != 1 || rewriteBranch.ForkFromSeq != 2 {
		t.Fatalf("rewrite fork boundary = %#v", rewriteBranch)
	}
	rewrite := findBranchTurn(graph.Turns, forkTurnID)
	if rewrite == nil {
		t.Fatalf("rewrite turn not found in graph: %#v", graph.Turns)
	}
	if rewrite.ParentTurnID != firstTurnID || !rewrite.Active {
		t.Fatalf("rewrite turn = %#v", rewrite)
	}
	original := findBranchTurn(graph.Turns, secondTurnID)
	if original == nil {
		t.Fatalf("original edited turn not found in graph: %#v", graph.Turns)
	}
	if rewrite.Depth != original.Depth {
		t.Fatalf("rewrite depth = %d, want sibling depth %d", rewrite.Depth, original.Depth)
	}
	third := findBranchTurn(graph.Turns, thirdTurnID)
	if third == nil || third.ParentTurnID != secondTurnID {
		t.Fatalf("third turn should remain on original path after edited turn: %#v", third)
	}
}

func TestBranchForkServiceRewritesFirstForkRequestFromParentTurn(t *testing.T) {
	ctx := context.Background()
	sessionID := "00000000-0000-0000-0000-000000001300"
	rootBranchID := "00000000-0000-0000-0000-000000001301"
	firstForkID := "00000000-0000-0000-0000-000000001302"
	rewriteForkID := "00000000-0000-0000-0000-000000001303"
	rootTurnID := "00000000-0000-0000-0000-000000001401"
	rootSecondTurnID := "00000000-0000-0000-0000-000000001404"
	firstForkTurnID := "00000000-0000-0000-0000-000000001402"
	rewriteTurnID := "00000000-0000-0000-0000-000000001403"
	rootAssistantID := "00000000-0000-0000-0000-000000001501"
	rootSecondAssistantID := "00000000-0000-0000-0000-000000001506"
	forkRequestID := "00000000-0000-0000-0000-000000001502"
	sessionUUID := mustPGUUID(sessionID)
	rootBranchUUID := mustPGUUID(rootBranchID)
	firstForkUUID := mustPGUUID(firstForkID)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	queries := &branchServiceQueries{
		sessionID:     sessionUUID,
		activeID:      firstForkUUID,
		nextBranchIDs: []pgtype.UUID{mustPGUUID(rewriteForkID)},
		forkMessages: map[string]sqlc.GetMessageForSessionBranchForkRow{
			forkRequestID: {
				ID:        mustPGUUID(forkRequestID),
				SessionID: sessionUUID,
				BranchID:  firstForkUUID,
				BranchSeq: pgtype.Int8{Int64: 1, Valid: true},
				TurnID:    mustPGUUID(firstForkTurnID),
				TurnSeq:   1,
				Role:      "user",
				CreatedAt: pgtype.Timestamptz{Time: now.Add(2 * time.Minute), Valid: true},
			},
		},
		branches: []sqlc.ListSessionBranchesRow{
			{
				ID:             rootBranchUUID,
				SessionID:      sessionUUID,
				CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
				UpdatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
				ActiveBranchID: firstForkUUID,
			},
			{
				ID:                firstForkUUID,
				SessionID:         sessionUUID,
				ParentBranchID:    rootBranchUUID,
				ForkFromMessageID: mustPGUUID(rootAssistantID),
				ForkFromSeq:       pgtype.Int8{Int64: 2, Valid: true},
				ForkFromTurnID:    mustPGUUID(rootTurnID),
				ForkFromTurnSeq:   pgtype.Int8{Int64: 1, Valid: true},
				CreatedAt:         pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
				UpdatedAt:         pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
				ActiveBranchID:    firstForkUUID,
			},
		},
		turns: []sqlc.ListSessionBranchTurnMessagesRow{
			{
				TurnID:               mustPGUUID(rootTurnID),
				TurnSeq:              1,
				AssistantID:          mustPGUUID(rootAssistantID),
				SessionID:            sessionUUID,
				BranchID:             rootBranchUUID,
				BranchSeq:            pgtype.Int8{Int64: 2, Valid: true},
				AssistantDisplayText: pgtype.Text{String: "root answer", Valid: true},
				AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
				UserID:               mustPGUUID("00000000-0000-0000-0000-000000001601"),
				UserDisplayText:      pgtype.Text{String: "root question", Valid: true},
				UserCreatedAt:        pgtype.Timestamptz{Time: now, Valid: true},
			},
			{
				TurnID:               mustPGUUID(rootSecondTurnID),
				TurnSeq:              2,
				AssistantID:          mustPGUUID(rootSecondAssistantID),
				SessionID:            sessionUUID,
				BranchID:             rootBranchUUID,
				BranchSeq:            pgtype.Int8{Int64: 4, Valid: true},
				AssistantDisplayText: pgtype.Text{String: "root second answer", Valid: true},
				AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(2 * time.Minute), Valid: true},
				UserID:               mustPGUUID("00000000-0000-0000-0000-000000001602"),
				UserDisplayText:      pgtype.Text{String: "root second question", Valid: true},
				UserCreatedAt:        pgtype.Timestamptz{Time: now.Add(90 * time.Second), Valid: true},
			},
			{
				TurnID:               mustPGUUID(firstForkTurnID),
				TurnSeq:              1,
				AssistantID:          mustPGUUID("00000000-0000-0000-0000-000000001503"),
				SessionID:            sessionUUID,
				BranchID:             firstForkUUID,
				BranchSeq:            pgtype.Int8{Int64: 2, Valid: true},
				AssistantDisplayText: pgtype.Text{String: "fork answer", Valid: true},
				AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(3 * time.Minute), Valid: true},
				UserID:               mustPGUUID(forkRequestID),
				UserDisplayText:      pgtype.Text{String: "fork question", Valid: true},
				UserCreatedAt:        pgtype.Timestamptz{Time: now.Add(2 * time.Minute), Valid: true},
			},
			{
				TurnID:               mustPGUUID(rewriteTurnID),
				TurnSeq:              1,
				AssistantID:          mustPGUUID("00000000-0000-0000-0000-000000001504"),
				SessionID:            sessionUUID,
				BranchID:             mustPGUUID(rewriteForkID),
				BranchSeq:            pgtype.Int8{Int64: 2, Valid: true},
				AssistantDisplayText: pgtype.Text{String: "rewrite answer", Valid: true},
				AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(5 * time.Minute), Valid: true},
				UserID:               mustPGUUID("00000000-0000-0000-0000-000000001505"),
				UserDisplayText:      pgtype.Text{String: "rewritten question", Valid: true},
				UserCreatedAt:        pgtype.Timestamptz{Time: now.Add(4 * time.Minute), Valid: true},
			},
		},
	}
	svc := NewService(nil, queries)

	graph, err := svc.ForkBranchFromMessage(ctx, sessionID, forkRequestID)
	if err != nil {
		t.Fatalf("ForkBranchFromMessage(fork request) error = %v", err)
	}
	if graph.ActiveBranchID != rewriteForkID {
		t.Fatalf("active branch = %q, want %q", graph.ActiveBranchID, rewriteForkID)
	}
	rewriteBranch := graph.Branches[len(graph.Branches)-1]
	if rewriteBranch.ParentBranchID != rootBranchID {
		t.Fatalf("rewrite parent branch = %q, want %q", rewriteBranch.ParentBranchID, rootBranchID)
	}
	if rewriteBranch.ForkFromTurnID != rootTurnID || rewriteBranch.ForkFromTurnSeq != 1 || rewriteBranch.ForkFromSeq != 2 {
		t.Fatalf("rewrite fork boundary = %#v", rewriteBranch)
	}
	rewrite := findBranchTurn(graph.Turns, rewriteTurnID)
	if rewrite == nil {
		t.Fatalf("rewrite turn not found in graph: %#v", graph.Turns)
	}
	if rewrite.ParentTurnID != rootTurnID || !rewrite.Active {
		t.Fatalf("rewrite turn = %#v", rewrite)
	}
}

func TestBuildBranchTurnsUsesForkTurnIDForFirstForkTurnParent(t *testing.T) {
	rootBranchID := "00000000-0000-0000-0000-000000001001"
	forkBranchID := "00000000-0000-0000-0000-000000001002"
	firstTurnID := "00000000-0000-0000-0000-000000001101"
	secondTurnID := "00000000-0000-0000-0000-000000001102"
	forkTurnID := "00000000-0000-0000-0000-000000001103"
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	turns := buildBranchTurns([]BranchNode{
		{ID: rootBranchID, SessionID: "00000000-0000-0000-0000-000000001000"},
		{
			ID:              forkBranchID,
			SessionID:       "00000000-0000-0000-0000-000000001000",
			ParentBranchID:  rootBranchID,
			ForkFromTurnID:  firstTurnID,
			ForkFromTurnSeq: 1,
			Active:          true,
		},
	}, []sqlc.ListSessionBranchTurnMessagesRow{
		{
			TurnID:               mustPGUUID(firstTurnID),
			TurnSeq:              1,
			AssistantID:          mustPGUUID("00000000-0000-0000-0000-000000001201"),
			BranchID:             mustPGUUID(rootBranchID),
			BranchSeq:            pgtype.Int8{Int64: 2, Valid: true},
			AssistantDisplayText: pgtype.Text{String: "first answer", Valid: true},
			AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
		},
		{
			TurnID:               mustPGUUID(secondTurnID),
			TurnSeq:              2,
			AssistantID:          mustPGUUID("00000000-0000-0000-0000-000000001202"),
			BranchID:             mustPGUUID(rootBranchID),
			BranchSeq:            pgtype.Int8{Int64: 4, Valid: true},
			AssistantDisplayText: pgtype.Text{String: "second answer", Valid: true},
			AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(3 * time.Minute), Valid: true},
		},
		{
			TurnID:               mustPGUUID(forkTurnID),
			TurnSeq:              1,
			AssistantID:          mustPGUUID("00000000-0000-0000-0000-000000001203"),
			BranchID:             mustPGUUID(forkBranchID),
			BranchSeq:            pgtype.Int8{Int64: 2, Valid: true},
			AssistantDisplayText: pgtype.Text{String: "fork answer", Valid: true},
			AssistantCreatedAt:   pgtype.Timestamptz{Time: now.Add(5 * time.Minute), Valid: true},
		},
	})

	forkTurn := findBranchTurn(turns, forkTurnID)
	if forkTurn == nil {
		t.Fatalf("fork turn not found: %#v", turns)
	}
	if forkTurn.ParentTurnID != firstTurnID {
		t.Fatalf("fork turn parent = %q, want %q", forkTurn.ParentTurnID, firstTurnID)
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

func (q *branchServiceQueries) SetActiveSessionBranch(_ context.Context, params sqlc.SetActiveSessionBranchParams) (int64, error) {
	q.activeID = params.BranchID
	for i := range q.branches {
		q.branches[i].ActiveBranchID = params.BranchID
	}
	return 1, nil
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
		ForkFromTurnID:    params.ForkFromTurnID,
		ForkFromTurnSeq:   params.ForkFromTurnSeq,
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
