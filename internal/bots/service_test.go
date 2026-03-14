package bots

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/sqlc"
)

// fakeRow implements pgx.Row with a custom scan function.
type fakeRow struct {
	scanFunc func(dest ...any) error
}

func (r *fakeRow) Scan(dest ...any) error {
	return r.scanFunc(dest...)
}

// fakeDBTX implements sqlc.DBTX for unit testing.
type fakeDBTX struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (*fakeDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (*fakeDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (d *fakeDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if d.queryRowFunc != nil {
		return d.queryRowFunc(ctx, sql, args...)
	}
	return &fakeRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
}

// makeBotRow creates a fakeRow that populates a sqlc.Bot via Scan.
// Column order: id, owner_user_id, type, display_name, avatar_url, is_active, status,
// max_context_load_time, max_context_tokens, max_inbox_items, language,
// reasoning_enabled, reasoning_effort, chat_model_id, search_provider_id, memory_provider_id,
// heartbeat_enabled, heartbeat_interval, heartbeat_prompt, metadata, created_at, updated_at.
func makeBotRow(botID, ownerUserID pgtype.UUID, botType string, allowGuest bool) *fakeRow {
	return &fakeRow{
		scanFunc: func(dest ...any) error {
			if len(dest) < 22 {
				return pgx.ErrNoRows
			}
			*dest[0].(*pgtype.UUID) = botID
			*dest[1].(*pgtype.UUID) = ownerUserID
			*dest[2].(*string) = botType
			*dest[3].(*pgtype.Text) = pgtype.Text{String: "test-bot", Valid: true}
			*dest[4].(*pgtype.Text) = pgtype.Text{}
			*dest[5].(*bool) = true
			*dest[6].(*string) = BotStatusReady
			*dest[7].(*int32) = 30   // MaxContextLoadTime
			*dest[8].(*int32) = 4096 // MaxContextTokens
			*dest[9].(*int32) = 10   // MaxInboxItems
			*dest[10].(*string) = "en"
			_ = allowGuest
			*dest[11].(*bool) = false                // ReasoningEnabled
			*dest[12].(*string) = "medium"           // ReasoningEffort
			*dest[13].(*pgtype.UUID) = pgtype.UUID{} // ChatModelID
			*dest[14].(*pgtype.UUID) = pgtype.UUID{} // SearchProviderID
			*dest[15].(*pgtype.UUID) = pgtype.UUID{} // MemoryProviderID
			*dest[16].(*bool) = false                // HeartbeatEnabled
			*dest[17].(*int32) = 30                  // HeartbeatInterval
			*dest[18].(*string) = ""                 // HeartbeatPrompt
			*dest[19].(*[]byte) = []byte(`{}`)
			*dest[20].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[21].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			return nil
		},
	}
}

func mustParseUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(s)
	return u
}

func TestAuthorizeAccess(t *testing.T) {
	ownerUUID := mustParseUUID("00000000-0000-0000-0000-000000000001")
	botUUID := mustParseUUID("00000000-0000-0000-0000-000000000002")
	strangerUUID := mustParseUUID("00000000-0000-0000-0000-000000000003")
	ownerID := ownerUUID.String()
	botID := botUUID.String()
	strangerID := strangerUUID.String()

	tests := []struct {
		name      string
		userID    string
		isAdmin   bool
		policy    AccessPolicy
		botType   string
		allowGst  bool
		wantErr   bool
		wantErrIs error
	}{
		{
			name:    "owner always allowed",
			userID:  ownerID,
			policy:  AccessPolicy{},
			botType: BotTypePublic,
			wantErr: false,
		},
		{
			name:    "admin always allowed",
			userID:  strangerID,
			isAdmin: true,
			policy:  AccessPolicy{},
			botType: BotTypePublic,
			wantErr: false,
		},
		{
			name:      "stranger denied without guest on public bot",
			userID:    strangerID,
			policy:    AccessPolicy{AllowGuest: false},
			botType:   BotTypePublic,
			wantErr:   true,
			wantErrIs: ErrBotAccessDenied,
		},
		{
			name:    "stranger allowed when policy allows guest",
			userID:  strangerID,
			policy:  AccessPolicy{AllowGuest: true},
			botType: BotTypePublic,
			wantErr: false,
		},
		{
			name:      "guest not allowed on personal bot",
			userID:    strangerID,
			policy:    AccessPolicy{AllowGuest: true},
			botType:   BotTypePersonal,
			wantErr:   true,
			wantErrIs: ErrBotAccessDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeDBTX{
				queryRowFunc: func(_ context.Context, _ string, args ...any) pgx.Row {
					_ = args
					return makeBotRow(botUUID, ownerUUID, tt.botType, tt.allowGst)
				},
			}
			svc := NewService(nil, sqlc.New(db))

			_, err := svc.AuthorizeAccess(context.Background(), tt.userID, botID, tt.isAdmin, tt.policy)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrIs != nil && err.Error() != tt.wantErrIs.Error() {
					t.Fatalf("expected error %q, got %q", tt.wantErrIs, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
