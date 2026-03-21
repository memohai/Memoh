package acl

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type fakeDBTX struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	queryFunc    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (f *fakeDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if f.execFunc != nil {
		return f.execFunc(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (f *fakeDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if f.queryFunc != nil {
		return f.queryFunc(ctx, sql, args...)
	}
	return &fakeRows{}, nil
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
			*dest[2].(*pgtype.Text) = pgtype.Text{String: "bot", Valid: true}
			*dest[3].(*pgtype.Text) = pgtype.Text{}
			*dest[4].(*bool) = true
			*dest[5].(*string) = bots.BotStatusReady
			*dest[6].(*int32) = 30
			*dest[7].(*int32) = 0
			*dest[8].(*string) = ""
			*dest[9].(*bool) = false
			*dest[10].(*string) = "medium"
			*dest[11].(*pgtype.UUID) = pgtype.UUID{}
			*dest[12].(*pgtype.UUID) = pgtype.UUID{}
			*dest[13].(*pgtype.UUID) = pgtype.UUID{}
			*dest[14].(*bool) = false
			*dest[15].(*int32) = 30
			*dest[16].(*string) = ""
			*dest[17].(*[]byte) = []byte(`{}`)
			*dest[18].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[19].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
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

type fakeRows struct {
	rows    []func(dest ...any) error
	idx     int
	lastErr error
}

func (*fakeRows) Close()                                       {}
func (r *fakeRows) Err() error                                 { return r.lastErr }
func (*fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (*fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.rows) {
		return errors.New("scan called without next")
	}
	scan := r.rows[r.idx-1]
	if scan == nil {
		return nil
	}
	return scan(dest...)
}
func (*fakeRows) Values() ([]any, error) { return nil, nil }
func (*fakeRows) RawValues() [][]byte    { return nil }
func (*fakeRows) Conn() *pgx.Conn        { return nil }

func textFromArg(value any) string {
	switch v := value.(type) {
	case pgtype.Text:
		return strings.TrimSpace(v.String)
	case *pgtype.Text:
		if v == nil {
			return ""
		}
		return strings.TrimSpace(v.String)
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func scopeMatches(rule *SourceScope, args ...any) bool {
	if rule == nil {
		return false
	}
	scope := rule.Normalize()
	return (scope.Channel == "" || scope.Channel == textFromArg(args[3])) &&
		(scope.ConversationType == "" || scope.ConversationType == textFromArg(args[4])) &&
		(scope.ConversationID == "" || scope.ConversationID == textFromArg(args[5])) &&
		(scope.ThreadID == "" || scope.ThreadID == textFromArg(args[6]))
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
		sourceScope       SourceScope
		denyUserScope     *SourceScope
		allowUserScope    *SourceScope
		denyChannelScope  *SourceScope
		allowChannelScope *SourceScope
		allowGuestAll     bool
		wantAllowed       bool
	}{
		{name: "owner bypass", userID: ownerUUID.String(), wantAllowed: true},
		{name: "deny user wins", userID: userUUID.String(), denyUserScope: &SourceScope{}, allowGuestAll: true, wantAllowed: false},
		{name: "allow user", userID: userUUID.String(), allowUserScope: &SourceScope{}, wantAllowed: true},
		{name: "deny channel wins", channelIdentityID: channelIdentityUUID.String(), denyChannelScope: &SourceScope{}, allowGuestAll: true, wantAllowed: false},
		{name: "allow channel identity", channelIdentityID: channelIdentityUUID.String(), allowChannelScope: &SourceScope{}, wantAllowed: true},
		{
			name:           "scoped allow user private",
			userID:         userUUID.String(),
			sourceScope:    SourceScope{Channel: "feishu", ConversationType: "private", ConversationID: "chat-1"},
			allowUserScope: &SourceScope{Channel: "feishu", ConversationType: "private", ConversationID: "chat-1"},
			wantAllowed:    true,
		},
		{
			name:           "scoped allow user does not match other conversation",
			userID:         userUUID.String(),
			sourceScope:    SourceScope{Channel: "feishu", ConversationType: "private", ConversationID: "chat-2"},
			allowUserScope: &SourceScope{Channel: "feishu", ConversationType: "private", ConversationID: "chat-1"},
			wantAllowed:    false,
		},
		{
			name:              "scoped deny overrides guest fallback",
			channelIdentityID: channelIdentityUUID.String(),
			sourceScope:       SourceScope{Channel: "telegram", ConversationType: "group", ConversationID: "group-1"},
			denyChannelScope:  &SourceScope{Channel: "telegram", ConversationType: "group", ConversationID: "group-1"},
			allowGuestAll:     true,
			wantAllowed:       false,
		},
		{
			name:              "scoped deny does not block different source",
			channelIdentityID: channelIdentityUUID.String(),
			sourceScope:       SourceScope{Channel: "telegram", ConversationType: "group", ConversationID: "group-2"},
			denyChannelScope:  &SourceScope{Channel: "telegram", ConversationType: "group", ConversationID: "group-1"},
			allowGuestAll:     true,
			wantAllowed:       true,
		},
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
							return makeBoolRow(scopeMatches(tt.denyUserScope, args...))
						}
						return makeBoolRow(scopeMatches(tt.allowUserScope, args...))
					case strings.Contains(sql, "subject_kind = 'channel_identity'"):
						effect := args[1].(string)
						if effect == EffectDeny {
							return makeBoolRow(scopeMatches(tt.denyChannelScope, args...))
						}
						return makeBoolRow(scopeMatches(tt.allowChannelScope, args...))
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
			allowed, err := service.CanPerformChatTrigger(context.Background(), ChatTriggerRequest{
				BotID:             botUUID.String(),
				UserID:            tt.userID,
				ChannelIdentityID: tt.channelIdentityID,
				SourceScope:       tt.sourceScope,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if allowed != tt.wantAllowed {
				t.Fatalf("expected allowed=%v, got %v", tt.wantAllowed, allowed)
			}
		})
	}
}

func TestCanPerformChatTriggerRejectsInvalidScope(t *testing.T) {
	service := NewService(nil, nil, nil)
	_, err := service.CanPerformChatTrigger(context.Background(), ChatTriggerRequest{
		BotID: "bot-1",
		SourceScope: SourceScope{
			Channel:  "feishu",
			ThreadID: "thread-1",
		},
	})
	if !errors.Is(err, ErrInvalidSourceScope) {
		t.Fatalf("expected invalid source scope error, got %v", err)
	}
}

func TestListObservedConversationsByChannelIdentity(t *testing.T) {
	botUUID := pgtype.UUID{Bytes: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Valid: true}
	channelIdentityUUID := pgtype.UUID{Bytes: uuid.MustParse("55555555-5555-5555-5555-555555555555"), Valid: true}
	routeUUID := pgtype.UUID{Bytes: uuid.MustParse("66666666-6666-6666-6666-666666666666"), Valid: true}
	now := time.Now().UTC()

	db := &fakeDBTX{
		queryFunc: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if !strings.Contains(sql, "ListObservedConversationsByChannelIdentity") &&
				!strings.Contains(sql, "FROM bot_history_messages m") {
				return &fakeRows{}, nil
			}
			return &fakeRows{
				rows: []func(dest ...any) error{
					func(dest ...any) error {
						*dest[0].(*pgtype.UUID) = routeUUID
						*dest[1].(*string) = "feishu"
						*dest[2].(*string) = "group"
						*dest[3].(*string) = "chat-1"
						*dest[4].(*string) = "thread-1"
						*dest[5].(*string) = "Team Chat"
						*dest[6].(*pgtype.Timestamptz) = pgtype.Timestamptz{Time: now, Valid: true}
						return nil
					},
				},
			}, nil
		},
	}

	service := NewService(nil, sqlc.New(db), nil)
	items, err := service.ListObservedConversationsByChannelIdentity(context.Background(), botUUID.String(), channelIdentityUUID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one observed conversation, got %d", len(items))
	}
	if items[0].RouteID != routeUUID.String() {
		t.Fatalf("unexpected route id: %s", items[0].RouteID)
	}
	if items[0].ConversationID != "chat-1" || items[0].ThreadID != "thread-1" {
		t.Fatalf("unexpected conversation scope: %+v", items[0])
	}
	if items[0].ConversationName != "Team Chat" {
		t.Fatalf("unexpected conversation name: %q", items[0].ConversationName)
	}
}

func TestAddWhitelistEntryChannelIdentityForcesIdentityChannel(t *testing.T) {
	botUUID := pgtype.UUID{Bytes: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Valid: true}
	ruleUUID := pgtype.UUID{Bytes: uuid.MustParse("77777777-7777-7777-7777-777777777777"), Valid: true}
	channelIdentityUUID := pgtype.UUID{Bytes: uuid.MustParse("55555555-5555-5555-5555-555555555555"), Valid: true}
	createdByUUID := pgtype.UUID{Bytes: uuid.MustParse("88888888-8888-8888-8888-888888888888"), Valid: true}
	now := time.Now().UTC()

	db := &fakeDBTX{
		queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM channel_identities"):
				return &fakeRow{
					scanFunc: func(dest ...any) error {
						*dest[0].(*pgtype.UUID) = channelIdentityUUID
						*dest[1].(*pgtype.UUID) = pgtype.UUID{}
						*dest[2].(*string) = "feishu"
						*dest[3].(*string) = "ou_123"
						*dest[4].(*pgtype.Text) = pgtype.Text{String: "Tester", Valid: true}
						*dest[5].(*pgtype.Text) = pgtype.Text{}
						*dest[6].(*[]byte) = []byte(`{}`)
						*dest[7].(*pgtype.Timestamptz) = pgtype.Timestamptz{Time: now, Valid: true}
						*dest[8].(*pgtype.Timestamptz) = pgtype.Timestamptz{Time: now, Valid: true}
						return nil
					},
				}
			case strings.Contains(sql, "INSERT INTO bot_acl_rules"):
				if got := textFromArg(args[4]); got != "feishu" {
					t.Fatalf("expected source_channel to be normalized to feishu, got %q", got)
				}
				return &fakeRow{
					scanFunc: func(dest ...any) error {
						*dest[0].(*pgtype.UUID) = ruleUUID
						*dest[1].(*pgtype.UUID) = botUUID
						*dest[2].(*string) = ActionChatTrigger
						*dest[3].(*string) = EffectAllow
						*dest[4].(*string) = SubjectKindChannelIdentity
						*dest[5].(*pgtype.UUID) = pgtype.UUID{}
						*dest[6].(*pgtype.UUID) = channelIdentityUUID
						*dest[7].(*pgtype.Text) = pgtype.Text{String: "feishu", Valid: true}
						*dest[8].(*pgtype.Text) = pgtype.Text{String: "group", Valid: true}
						*dest[9].(*pgtype.Text) = pgtype.Text{String: "chat-1", Valid: true}
						*dest[10].(*pgtype.Text) = pgtype.Text{}
						*dest[11].(*pgtype.UUID) = createdByUUID
						*dest[12].(*pgtype.Timestamptz) = pgtype.Timestamptz{Time: now, Valid: true}
						*dest[13].(*pgtype.Timestamptz) = pgtype.Timestamptz{Time: now, Valid: true}
						return nil
					},
				}
			default:
				return &fakeRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
			}
		},
	}

	service := NewService(nil, sqlc.New(db), nil)
	rule, err := service.AddWhitelistEntry(context.Background(), botUUID.String(), createdByUUID.String(), UpsertRuleRequest{
		ChannelIdentityID: channelIdentityUUID.String(),
		SourceScope: &SourceScope{
			Channel:          "telegram",
			ConversationType: "group",
			ConversationID:   "chat-1",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.SourceScope == nil || rule.SourceScope.Channel != "feishu" {
		t.Fatalf("expected normalized source scope channel feishu, got %+v", rule.SourceScope)
	}
}
