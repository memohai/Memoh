package session

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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
