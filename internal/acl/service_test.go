package acl

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type fakeDBTX struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (f *fakeDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if f.execFunc != nil {
		return f.execFunc(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (*fakeDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (f *fakeDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if f.queryRowFunc != nil {
		return f.queryRowFunc(ctx, sql, args...)
	}
	return &fakeRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
}

type fakeRow struct {
	scanFunc func(dest ...any) error
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.scanFunc == nil {
		return pgx.ErrNoRows
	}
	return r.scanFunc(dest...)
}

func makeBotRow(botID, ownerUserID pgtype.UUID) *fakeRow {
	return &fakeRow{
		scanFunc: func(dest ...any) error {
			*dest[0].(*pgtype.UUID) = botID
			*dest[1].(*pgtype.UUID) = ownerUserID
			*dest[2].(*string) = bots.BotTypePublic
			*dest[3].(*pgtype.Text) = pgtype.Text{String: "bot", Valid: true}
			*dest[4].(*pgtype.Text) = pgtype.Text{}
			*dest[5].(*bool) = true
			*dest[6].(*string) = bots.BotStatusReady
			*dest[7].(*int32) = 30
			*dest[8].(*int32) = 0
			*dest[9].(*int32) = 50
			*dest[10].(*string) = "auto"
			*dest[11].(*bool) = false
			*dest[12].(*string) = "medium"
			*dest[13].(*pgtype.UUID) = pgtype.UUID{}
			*dest[14].(*pgtype.UUID) = pgtype.UUID{}
			*dest[15].(*pgtype.UUID) = pgtype.UUID{}
			*dest[16].(*bool) = false
			*dest[17].(*int32) = 30
			*dest[18].(*string) = ""
			*dest[19].(*[]byte) = []byte(`{}`)
			*dest[20].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[21].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			return nil
		},
	}
}

func makeBoolRow(value bool) *fakeRow {
	return &fakeRow{
		scanFunc: func(dest ...any) error {
			*dest[0].(*bool) = value
			return nil
		},
	}
}

func TestCanPerformChatTrigger(t *testing.T) {
	botUUID := pgtype.UUID{Bytes: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Valid: true}
	ownerUUID := pgtype.UUID{Bytes: uuid.MustParse("22222222-2222-2222-2222-222222222222"), Valid: true}
	userUUID := pgtype.UUID{Bytes: uuid.MustParse("44444444-4444-4444-4444-444444444444"), Valid: true}
	channelIdentityUUID := pgtype.UUID{Bytes: uuid.MustParse("55555555-5555-5555-5555-555555555555"), Valid: true}

	tests := []struct {
		name              string
		userID            string
		channelIdentityID string
		denyUser          bool
		allowUser         bool
		denyChannel       bool
		allowChannel      bool
		allowGuestAll     bool
		wantAllowed       bool
	}{
		{name: "owner bypass", userID: ownerUUID.String(), wantAllowed: true},
		{name: "deny user wins", userID: userUUID.String(), denyUser: true, allowGuestAll: true, wantAllowed: false},
		{name: "allow user", userID: userUUID.String(), allowUser: true, wantAllowed: true},
		{name: "deny channel wins", channelIdentityID: channelIdentityUUID.String(), denyChannel: true, allowGuestAll: true, wantAllowed: false},
		{name: "allow channel identity", channelIdentityID: channelIdentityUUID.String(), allowChannel: true, wantAllowed: true},
		{name: "guest_all fallback", allowGuestAll: true, wantAllowed: true},
		{name: "default deny", wantAllowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeDBTX{
				queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
					switch {
					case strings.Contains(sql, "FROM bots"):
						return makeBotRow(botUUID, ownerUUID)
					case strings.Contains(sql, "subject_kind = 'user'"):
						effect := args[1].(string)
						if effect == EffectDeny {
							return makeBoolRow(tt.denyUser)
						}
						return makeBoolRow(tt.allowUser)
					case strings.Contains(sql, "subject_kind = 'channel_identity'"):
						effect := args[1].(string)
						if effect == EffectDeny {
							return makeBoolRow(tt.denyChannel)
						}
						return makeBoolRow(tt.allowChannel)
					case strings.Contains(sql, "subject_kind = 'guest_all'"):
						return makeBoolRow(tt.allowGuestAll)
					default:
						return &fakeRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
					}
				},
			}

			queries := sqlc.New(db)
			botService := bots.NewService(nil, queries)
			service := NewService(nil, queries, botService)
			allowed, err := service.CanPerformChatTrigger(context.Background(), botUUID.String(), tt.userID, tt.channelIdentityID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if allowed != tt.wantAllowed {
				t.Fatalf("expected allowed=%v, got %v", tt.wantAllowed, allowed)
			}
		})
	}
}
