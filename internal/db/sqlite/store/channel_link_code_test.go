package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestRedeemChannelLinkCodeRollsBackWhenBindingFails(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, `
CREATE TABLE channel_link_codes (
  token TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  channel_type TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL,
  consumed_at TEXT,
  consumed_channel_identity_id TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE user_channel_identity_bindings (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  channel_identity_id TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT user_channel_identity_bindings_unique UNIQUE (user_id, channel_identity_id)
);
CREATE TRIGGER fail_channel_identity_binding
BEFORE INSERT ON user_channel_identity_bindings
BEGIN
  SELECT RAISE(ABORT, 'binding failed');
END;
`)

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	queries := NewQueries(store)
	userID := "00000000-0000-0000-0000-000000000001"
	identityID := mustPgUUID(t, "00000000-0000-0000-0000-000000000002")
	_, err = conn.ExecContext(ctx,
		`INSERT INTO channel_link_codes (token, user_id, expires_at) VALUES (?, ?, ?)`,
		"ABC12345", userID, time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano),
	)
	if err != nil {
		t.Fatalf("insert link code: %v", err)
	}

	_, err = queries.RedeemChannelLinkCode(ctx, pgsqlc.RedeemChannelLinkCodeParams{
		Token:             "ABC12345",
		ChannelIdentityID: identityID,
	})
	if err == nil {
		t.Fatal("expected binding failure")
	}

	var consumedAt sql.NullString
	if err := conn.QueryRowContext(ctx, `SELECT consumed_at FROM channel_link_codes WHERE token = ?`, "ABC12345").Scan(&consumedAt); err != nil {
		t.Fatalf("read consumed_at: %v", err)
	}
	if consumedAt.Valid {
		t.Fatalf("link code was consumed despite failed binding: %q", consumedAt.String)
	}
}

func mustPgUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatalf("parse uuid %q: %v", value, err)
	}
	return id
}
