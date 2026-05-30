package bots

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/acl"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
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
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (d *fakeDBTX) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	if d.execFunc != nil {
		return d.execFunc(ctx, sql, args...)
	}
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
// Column order: id, owner_user_id, name, display_name, avatar_url, timezone, is_active, status,
// language, reasoning_enabled, reasoning_effort,
// chat_model_id, search_provider_id, memory_provider_id,
// heartbeat_enabled, heartbeat_interval, heartbeat_prompt,
// compaction_enabled, compaction_threshold, compaction_ratio, compaction_model_id,
// metadata, created_at, updated_at.
func makeBotRow(botID, ownerUserID pgtype.UUID) *fakeRow {
	return &fakeRow{
		scanFunc: func(dest ...any) error {
			if len(dest) < 24 {
				return pgx.ErrNoRows
			}
			*dest[0].(*pgtype.UUID) = botID
			*dest[1].(*pgtype.UUID) = ownerUserID
			*dest[2].(*string) = "test-bot" // Name
			*dest[3].(*pgtype.Text) = pgtype.Text{String: "test-bot", Valid: true}
			*dest[4].(*pgtype.Text) = pgtype.Text{}
			*dest[5].(*pgtype.Text) = pgtype.Text{}
			*dest[6].(*bool) = true
			*dest[7].(*string) = BotStatusReady
			*dest[8].(*string) = "en"                // Language
			*dest[9].(*bool) = false                 // ReasoningEnabled
			*dest[10].(*string) = "medium"           // ReasoningEffort
			*dest[11].(*pgtype.UUID) = pgtype.UUID{} // ChatModelID
			*dest[12].(*pgtype.UUID) = pgtype.UUID{} // SearchProviderID
			*dest[13].(*pgtype.UUID) = pgtype.UUID{} // MemoryProviderID
			*dest[14].(*bool) = false                // HeartbeatEnabled
			*dest[15].(*int32) = 30                  // HeartbeatInterval
			*dest[16].(*string) = ""                 // HeartbeatPrompt
			*dest[17].(*bool) = false                // CompactionEnabled
			*dest[18].(*int32) = 100000              // CompactionThreshold
			*dest[19].(*int32) = 80                  // CompactionRatio
			*dest[20].(*pgtype.UUID) = pgtype.UUID{} // CompactionModelID
			*dest[21].(*[]byte) = []byte(`{}`)
			*dest[22].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[23].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			return nil
		},
	}
}

func mustParseUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(s)
	return u
}

type fakeContainerLifecycle struct {
	onSetup    func()
	setupBotID string
	setupErr   error
}

func (f *fakeContainerLifecycle) SetupBotContainer(_ context.Context, botID string) error {
	if f.onSetup != nil {
		f.onSetup()
	}
	f.setupBotID = botID
	return f.setupErr
}

func (*fakeContainerLifecycle) CleanupBotContainer(context.Context, string, bool) error {
	return nil
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
			svc := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))

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

func TestCreateRejectsUnknownACLPreset(t *testing.T) {
	ownerUUID := mustParseUUID("00000000-0000-0000-0000-000000000001")
	createCalled := false

	db := &fakeDBTX{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM users") && strings.Contains(sql, "WHERE id = $1"):
				return &fakeRow{scanFunc: func(_ ...any) error { return nil }}
			case strings.Contains(sql, "INSERT INTO bots"):
				createCalled = true
				return &fakeRow{scanFunc: func(_ ...any) error { return nil }}
			default:
				return &fakeRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
			}
		},
	}

	svc := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))
	_, err := svc.Create(context.Background(), ownerUUID.String(), CreateBotRequest{
		DisplayName: "test-bot",
		AclPreset:   "not_a_real_preset",
	})
	if !errors.Is(err, acl.ErrUnknownPreset) {
		t.Fatalf("expected ErrUnknownPreset, got %v", err)
	}
	if createCalled {
		t.Fatal("bot row should not be created when acl preset is invalid")
	}
}

func TestRunCreateLifecycleSetsUpContainerBeforeReady(t *testing.T) {
	botUUID := mustParseUUID("00000000-0000-0000-0000-000000000002")
	botID := botUUID.String()
	events := make([]string, 0, 2)

	db := &fakeDBTX{
		execFunc: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "UPDATE bots") && strings.Contains(sql, "SET status = $2") {
				events = append(events, "status")
				if got := args[0].(pgtype.UUID); got != botUUID {
					t.Fatalf("expected status update for %s, got %s", botUUID, got)
				}
				if got := args[1].(string); got != BotStatusReady {
					t.Fatalf("expected status %q, got %q", BotStatusReady, got)
				}
			}
			return pgconn.CommandTag{}, nil
		},
	}
	lifecycle := &fakeContainerLifecycle{
		onSetup: func() {
			events = append(events, "setup")
		},
	}
	svc := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))
	svc.SetContainerLifecycle(lifecycle)

	if err := svc.runCreateLifecycle(context.Background(), botID); err != nil {
		t.Fatalf("run create lifecycle: %v", err)
	}
	if lifecycle.setupBotID != botID {
		t.Fatalf("expected setup for bot %s, got %s", botID, lifecycle.setupBotID)
	}
	if len(events) != 2 || events[0] != "setup" || events[1] != "status" {
		t.Fatalf("expected setup before ready status update, got events %v", events)
	}
}
