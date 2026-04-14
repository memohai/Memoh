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

	"github.com/memohai/memoh/internal/db/sqlc"
)

// ---- fake DB infrastructure ----

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

// ---- helpers ----

func makeStringRow(value string) *fakeRow {
	return &fakeRow{
		scanFunc: func(dest ...any) error {
			*dest[0].(*string) = value
			return nil
		},
	}
}

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

// matchedRule returns a fakeRow that scans the given effect string.
func matchedRule(effect string) *fakeRow {
	return makeStringRow(effect)
}

// noRule returns a fakeRow that returns pgx.ErrNoRows (no matching rule).
func noRule() *fakeRow {
	return &fakeRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
}

// ---- Evaluate tests ----

func TestEvaluate(t *testing.T) {
	botUUID := pgtype.UUID{Bytes: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Valid: true}

	tests := []struct {
		name          string
		matchedEffect string // "" means no matching rule
		defaultEffect string
		wantAllowed   bool
	}{
		{
			name:          "first rule allow",
			matchedEffect: EffectAllow,
			defaultEffect: EffectDeny,
			wantAllowed:   true,
		},
		{
			name:          "first rule deny",
			matchedEffect: EffectDeny,
			defaultEffect: EffectAllow,
			wantAllowed:   false,
		},
		{
			name:          "no matching rule - default allow",
			matchedEffect: "",
			defaultEffect: EffectAllow,
			wantAllowed:   true,
		},
		{
			name:          "no matching rule - default deny",
			matchedEffect: "",
			defaultEffect: EffectDeny,
			wantAllowed:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeDBTX{
				queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
					switch {
					case strings.Contains(sql, "FROM bot_acl_rules") && strings.Contains(sql, "LIMIT 1"):
						// Evaluate query
						if tt.matchedEffect == "" {
							return noRule()
						}
						return matchedRule(tt.matchedEffect)
					case strings.Contains(sql, "acl_default_effect"):
						return makeStringRow(tt.defaultEffect)
					default:
						return noRule()
					}
				},
			}
			queries := sqlc.New(db)
			service := NewService(nil, queries)

			allowed, err := service.Evaluate(context.Background(), EvaluateRequest{
				BotID:             botUUID.String(),
				ChannelIdentityID: "55555555-5555-5555-5555-555555555555",
				ChannelType:       "telegram",
				SourceScope: SourceScope{
					ConversationType: "group",
					ConversationID:   "group-1",
				},
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

func TestEvaluateRejectsInvalidScope(t *testing.T) {
	service := NewService(nil, nil)
	_, err := service.Evaluate(context.Background(), EvaluateRequest{
		BotID: "11111111-1111-1111-1111-111111111111",
		SourceScope: SourceScope{
			ThreadID: "thread-1",
			// missing ConversationID - invalid
		},
	})
	if !errors.Is(err, ErrInvalidSourceScope) {
		t.Fatalf("expected ErrInvalidSourceScope, got %v", err)
	}
}

func TestValidateSubject(t *testing.T) {
	tests := []struct {
		name               string
		kind               string
		channelIdentityID  string
		subjectChannelType string
		wantErr            bool
	}{
		{"all - no fields", SubjectKindAll, "", "", false},
		{"all - with identity", SubjectKindAll, "some-id", "", true},
		{"all - with channel type", SubjectKindAll, "", "telegram", true},
		{"channel_identity - valid", SubjectKindChannelIdentity, "some-id", "", false},
		{"channel_identity - missing id", SubjectKindChannelIdentity, "", "", true},
		{"channel_identity - extra channel type", SubjectKindChannelIdentity, "some-id", "telegram", true},
		{"channel_type - valid", SubjectKindChannelType, "", "telegram", false},
		{"channel_type - missing channel type", SubjectKindChannelType, "", "", true},
		{"channel_type - extra identity", SubjectKindChannelType, "some-id", "telegram", true},
		{"unknown kind", "unknown", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSubject(tt.kind, tt.channelIdentityID, tt.subjectChannelType)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSubject() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateEffect(t *testing.T) {
	if err := validateEffect(EffectAllow); err != nil {
		t.Fatalf("allow should be valid: %v", err)
	}
	if err := validateEffect(EffectDeny); err != nil {
		t.Fatalf("deny should be valid: %v", err)
	}
	if err := validateEffect("unknown"); err == nil {
		t.Fatal("expected error for unknown effect")
	}
}

func TestSetDefaultEffect(t *testing.T) {
	botUUID := pgtype.UUID{Bytes: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Valid: true}
	var capturedEffect string
	db := &fakeDBTX{
		execFunc: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "acl_default_effect") {
				capturedEffect = args[1].(string)
			}
			return pgconn.CommandTag{}, nil
		},
	}
	service := NewService(nil, sqlc.New(db))
	if err := service.SetDefaultEffect(context.Background(), botUUID.String(), EffectAllow); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedEffect != EffectAllow {
		t.Fatalf("expected effect %q, got %q", EffectAllow, capturedEffect)
	}
	if err := service.SetDefaultEffect(context.Background(), botUUID.String(), "invalid"); !errors.Is(err, ErrInvalidEffect) {
		t.Fatalf("expected ErrInvalidEffect, got %v", err)
	}
}

func TestListObservedConversationsByChannelIdentity(t *testing.T) {
	botUUID := pgtype.UUID{Bytes: uuid.MustParse("11111111-1111-1111-1111-111111111111"), Valid: true}
	channelIdentityUUID := pgtype.UUID{Bytes: uuid.MustParse("55555555-5555-5555-5555-555555555555"), Valid: true}
	routeUUID := pgtype.UUID{Bytes: uuid.MustParse("66666666-6666-6666-6666-666666666666"), Valid: true}
	now := time.Now().UTC()

	db := &fakeDBTX{
		queryFunc: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if !strings.Contains(sql, "observed_routes") && !strings.Contains(sql, "bot_sessions") {
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

	service := NewService(nil, sqlc.New(db))
	items, err := service.ListObservedConversationsByChannelIdentity(context.Background(), botUUID.String(), channelIdentityUUID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].RouteID != routeUUID.String() {
		t.Fatalf("unexpected route id: %s", items[0].RouteID)
	}
	if items[0].ConversationID != "chat-1" || items[0].ThreadID != "thread-1" {
		t.Fatalf("unexpected conversation scope: %+v", items[0])
	}
}

func TestReorderRules(t *testing.T) {
	ruleUUID := pgtype.UUID{Bytes: uuid.MustParse("77777777-7777-7777-7777-777777777777"), Valid: true}
	var capturedPriority int32
	db := &fakeDBTX{
		execFunc: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "priority") {
				capturedPriority = args[1].(int32)
			}
			return pgconn.CommandTag{}, nil
		},
	}
	service := NewService(nil, sqlc.New(db))
	err := service.ReorderRules(context.Background(), []ReorderItem{
		{ID: ruleUUID.String(), Priority: 42},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPriority != 42 {
		t.Fatalf("expected priority 42, got %d", capturedPriority)
	}
}

func TestTextFromArg(t *testing.T) {
	if got := textFromArg(pgtype.Text{String: "  hello  ", Valid: true}); got != "hello" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := textFromArg("world"); got != "world" {
		t.Fatalf("unexpected: %q", got)
	}
}
