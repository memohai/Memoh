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

func (d *fakeDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (d *fakeDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (d *fakeDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if d.queryRowFunc != nil {
		return d.queryRowFunc(ctx, sql, args...)
	}
	return &fakeRow{scanFunc: func(dest ...any) error { return pgx.ErrNoRows }}
}

// makeBotRow creates a fakeRow that populates a sqlc.Bot via Scan.
func makeBotRow(botID, ownerUserID pgtype.UUID, botType string, allowGuest bool) *fakeRow {
	return &fakeRow{
		scanFunc: func(dest ...any) error {
			if len(dest) < 16 {
				return pgx.ErrNoRows
			}
			*dest[0].(*pgtype.UUID) = botID
			*dest[1].(*pgtype.UUID) = ownerUserID
			*dest[2].(*string) = botType
			*dest[3].(*pgtype.Text) = pgtype.Text{String: "test-bot", Valid: true}
			*dest[4].(*pgtype.Text) = pgtype.Text{}
			*dest[5].(*bool) = true
			*dest[6].(*string) = BotStatusReady
			*dest[7].(*int32) = 30
			*dest[8].(*string) = "en"
			*dest[9].(*bool) = allowGuest
			*dest[10].(*pgtype.UUID) = pgtype.UUID{}
			*dest[11].(*pgtype.UUID) = pgtype.UUID{}
			*dest[12].(*pgtype.UUID) = pgtype.UUID{}
			*dest[13].(*[]byte) = []byte(`{}`)
			*dest[14].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[15].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			return nil
		},
	}
}

func makeMemberRow(botID, userID pgtype.UUID) *fakeRow {
	return &fakeRow{
		scanFunc: func(dest ...any) error {
			if len(dest) < 4 {
				return pgx.ErrNoRows
			}
			*dest[0].(*pgtype.UUID) = botID
			*dest[1].(*pgtype.UUID) = userID
			*dest[2].(*string) = MemberRoleMember
			*dest[3].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			return nil
		},
	}
}

func makeNoRow() *fakeRow {
	return &fakeRow{scanFunc: func(dest ...any) error { return pgx.ErrNoRows }}
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
	memberUUID := mustParseUUID("00000000-0000-0000-0000-000000000004")

	ownerID := ownerUUID.String()
	botID := botUUID.String()
	strangerID := strangerUUID.String()
	memberID := memberUUID.String()

	tests := []struct {
		name      string
		userID    string
		isAdmin   bool
		policy    AccessPolicy
		botType   string
		allowGst  bool
		isMember  bool
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
			allowGst:  true,
			wantErr:   true,
			wantErrIs: ErrBotAccessDenied,
		},
		{
			name:      "stranger denied when bot guest disabled",
			userID:    strangerID,
			policy:    AccessPolicy{AllowGuest: true},
			botType:   BotTypePublic,
			allowGst:  false,
			wantErr:   true,
			wantErrIs: ErrBotAccessDenied,
		},
		{
			name:    "stranger allowed when policy and bot both allow guest",
			userID:  strangerID,
			policy:  AccessPolicy{AllowGuest: true},
			botType: BotTypePublic,
			allowGst: true,
			wantErr: false,
		},
		{
			name:      "guest not allowed on personal bot",
			userID:    strangerID,
			policy:    AccessPolicy{AllowGuest: true},
			botType:   BotTypePersonal,
			allowGst:  true,
			wantErr:   true,
			wantErrIs: ErrBotAccessDenied,
		},
		{
			name:     "member allowed with AllowPublicMember policy",
			userID:   memberID,
			policy:   AccessPolicy{AllowPublicMember: true},
			botType:  BotTypePublic,
			isMember: true,
			wantErr:  false,
		},
		{
			name:      "non-member denied with AllowPublicMember policy",
			userID:    strangerID,
			policy:    AccessPolicy{AllowPublicMember: true},
			botType:   BotTypePublic,
			isMember:  false,
			wantErr:   true,
			wantErrIs: ErrBotAccessDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeDBTX{
				queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
					// Route to bot or member row based on query.
					if len(args) == 1 {
						return makeBotRow(botUUID, ownerUUID, tt.botType, tt.allowGst)
					}
					if tt.isMember {
						return makeMemberRow(botUUID, memberUUID)
					}
					return makeNoRow()
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
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}
