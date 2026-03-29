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

// makeBotRow creates a fakeRow that populates a sqlc.GetBotByIDRow via Scan.
// Column order: id, owner_user_id, display_name, avatar_url, timezone, is_active, status,
// language, reasoning_enabled, reasoning_effort,
// chat_model_id, search_provider_id, memory_provider_id,
// heartbeat_enabled, heartbeat_interval, heartbeat_prompt,
// compaction_enabled, compaction_threshold, compaction_model_id,
// metadata, created_at, updated_at.
func makeBotRow(botID, ownerUserID pgtype.UUID) *fakeRow {
	return &fakeRow{
		scanFunc: func(dest ...any) error {
			if len(dest) < 22 {
				return pgx.ErrNoRows
			}
			*dest[0].(*pgtype.UUID) = botID
			*dest[1].(*pgtype.UUID) = ownerUserID
			*dest[2].(*pgtype.Text) = pgtype.Text{String: "test-bot", Valid: true}
			*dest[3].(*pgtype.Text) = pgtype.Text{}
			*dest[4].(*pgtype.Text) = pgtype.Text{}
			*dest[5].(*bool) = true
			*dest[6].(*string) = BotStatusReady
			*dest[7].(*string) = "en"                // Language
			*dest[8].(*bool) = false                 // ReasoningEnabled
			*dest[9].(*string) = "medium"            // ReasoningEffort
			*dest[10].(*pgtype.UUID) = pgtype.UUID{} // ChatModelID
			*dest[11].(*pgtype.UUID) = pgtype.UUID{} // SearchProviderID
			*dest[12].(*pgtype.UUID) = pgtype.UUID{} // MemoryProviderID
			*dest[13].(*bool) = false                // HeartbeatEnabled
			*dest[14].(*int32) = 30                  // HeartbeatInterval
			*dest[15].(*string) = ""                 // HeartbeatPrompt
			*dest[16].(*bool) = false                // CompactionEnabled
			*dest[17].(*int32) = 100000              // CompactionThreshold
			*dest[18].(*int32) = 80                  // CompactionRatio
			*dest[19].(*pgtype.UUID) = pgtype.UUID{} // CompactionModelID
			*dest[20].(*[]byte) = []byte(`{}`)
			*dest[21].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[22].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
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
		wantErr   bool
		wantErrIs error
	}{
		{
			name:    "owner always allowed",
			userID:  ownerID,
			wantErr: false,
		},
		{
			name:    "admin always allowed",
			userID:  strangerID,
			isAdmin: true,
			wantErr: false,
		},
		{
			name:      "stranger denied",
			userID:    strangerID,
			wantErr:   true,
			wantErrIs: ErrBotAccessDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeDBTX{
				queryRowFunc: func(_ context.Context, _ string, args ...any) pgx.Row {
					_ = args
					return makeBotRow(botUUID, ownerUUID)
				},
			}
			svc := NewService(nil, sqlc.New(db))

			_, err := svc.AuthorizeAccess(context.Background(), tt.userID, botID, tt.isAdmin)
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
