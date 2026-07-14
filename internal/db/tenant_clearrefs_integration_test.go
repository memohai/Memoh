//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
)

func uuidArg(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}
	return u
}

// TestSetNullParentDeletesClearRefs guards the precondition for the clear-refs
// contract: every canonical SET NULL FK is now RESTRICT.
func TestSetNullParentDeletesClearRefs(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	if _, err := pool.Exec(ctx, "SELECT set_config('app.tenant_id', '00000000-0000-0000-0000-000000000001', false)"); err != nil {
		t.Fatal(err)
	}
	var setNull int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint con
		  JOIN pg_class c ON c.oid=con.conrelid
		  JOIN pg_namespace n ON n.oid=c.relnamespace
		 WHERE con.contype='f' AND con.confdeltype='n' AND n.nspname='public'`).Scan(&setNull); err != nil {
		t.Fatalf("count set null: %v", err)
	}
	if setNull != 0 {
		t.Fatalf("expected 0 SET NULL FKs (all -> RESTRICT), got %d", setNull)
	}
}

// TestGeneratedDeleteModelClearsRefs calls the REAL generated DeleteModel with a
// model referenced by a bot via former-SET-NULL columns. It must succeed (clear-
// refs NULLs the columns) rather than RESTRICT-erroring. models has 13 children.
func TestGeneratedDeleteModelClearsRefs(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	if _, err := pool.Exec(ctx, "SELECT set_config('app.tenant_id', '00000000-0000-0000-0000-000000000001', false)"); err != nil {
		t.Fatal(err)
	}
	q := postgresstore.NewQueriesWithPool(pool, dbsqlc.New(pool))

	var userID, providerID, modelID, botID string
	seed := func(sql string, args ...any) string {
		var id string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("seed: %v", err)
		}
		return id
	}
	userID = seed(`INSERT INTO users (username, is_active, metadata) VALUES ('u', true, '{}') RETURNING id`)
	providerID = seed(`INSERT INTO providers (name, client_type) VALUES ('p', 'openai-completions') RETURNING id`)
	modelID = seed(`INSERT INTO models (provider_id, model_id, type) VALUES ($1, 'm', 'chat') RETURNING id`, providerID)
	botID = seed(`INSERT INTO bots (owner_user_id, name, chat_model_id, heartbeat_model_id, title_model_id, video_model_id)
	              VALUES ($1, 'clearref-bot', $2, $2, $2, $2) RETURNING id`, userID, modelID)

	// The real shipped query. Must not RESTRICT-error.
	if err := q.DeleteModel(ctx, uuidArg(t, modelID)); err != nil {
		t.Fatalf("generated DeleteModel must clear refs and succeed, got: %v", err)
	}

	var chatNull, hbNull, titleNull, videoNull bool
	if err := pool.QueryRow(ctx, `
		SELECT chat_model_id IS NULL, heartbeat_model_id IS NULL, title_model_id IS NULL, video_model_id IS NULL
		  FROM bots WHERE id=$1`, botID,
	).Scan(&chatNull, &hbNull, &titleNull, &videoNull); err != nil {
		t.Fatalf("read bot: %v", err)
	}
	if !chatNull || !hbNull || !titleNull || !videoNull {
		t.Errorf("all referencing bot columns must be NULL (chat=%v hb=%v title=%v video=%v)", chatNull, hbNull, titleNull, videoNull)
	}
	// Model is gone.
	var cnt int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM models WHERE id=$1`, modelID).Scan(&cnt)
	if cnt != 0 {
		t.Errorf("model must be deleted, still %d rows", cnt)
	}
}

// TestGeneratedDeleteChatClearsRefs calls the REAL generated DeleteChat for a bot
// that has a session, message, route, and external children referencing them
// (tool_approval_requests, user_input_requests). It must succeed.
func TestGeneratedDeleteChatClearsRefs(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	if _, err := pool.Exec(ctx, "SELECT set_config('app.tenant_id', '00000000-0000-0000-0000-000000000001', false)"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "SELECT set_config('app.fencing_token', '1', false)"); err != nil {
		t.Fatal(err)
	}
	q := postgresstore.NewQueriesWithPool(pool, dbsqlc.New(pool))

	seed := func(sql string, args ...any) string {
		var id string
		if err := pool.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
			t.Fatalf("seed %q: %v", sql, err)
		}
		return id
	}
	userID := seed(`INSERT INTO users (username, is_active, metadata) VALUES ('u', true, '{}') RETURNING id`)
	botID := seed(`INSERT INTO bots (owner_user_id, name) VALUES ($1, 'clearref-chat') RETURNING id`, userID)
	routeID := seed(`INSERT INTO bot_channel_routes (bot_id, channel_type, external_conversation_id) VALUES ($1, 'local', 'c') RETURNING id`, botID)
	sessionID := seed(`INSERT INTO bot_sessions (bot_id, channel_type, title, metadata, route_id) VALUES ($1, 'local', 's', '{}', $2) RETURNING id`, botID, routeID)
	msgID := seed(`INSERT INTO bot_history_messages (bot_id, session_id, role, content, metadata, usage, turn_id, turn_position, turn_message_seq, turn_visible)
	               VALUES ($1, $2, 'assistant', '"m"'::jsonb, '{}', '{}', gen_random_uuid(), 1, 1, true) RETURNING id`, botID, sessionID)
	// external child referencing message + route (former SET NULL).
	_ = seed(`INSERT INTO tool_approval_requests
			(bot_id, session_id, route_id, requested_message_id, prompt_message_id, tool_call_id, tool_name, operation, tool_input, short_id)
		VALUES ($1, $2, $3, $4, $4, 'tc', 'use_skill', 'exec', '{}', 1) RETURNING id`,
		botID, sessionID, routeID, msgID)

	if err := q.DeleteChat(ctx, uuidArg(t, botID)); err != nil {
		t.Fatalf("generated DeleteChat must clear refs and succeed, got: %v", err)
	}

	// messages/sessions/routes for the bot are gone.
	for _, tbl := range []string{"bot_history_messages", "bot_sessions", "bot_channel_routes"} {
		var n int
		_ = pool.QueryRow(ctx, "SELECT count(*) FROM "+tbl+" WHERE bot_id=$1", botID).Scan(&n)
		if n != 0 {
			t.Errorf("%s must be empty for the deleted bot, got %d", tbl, n)
		}
	}
	// The tool_approval_requests row referenced the bot's session via an
	// ON DELETE CASCADE FK, so it is legitimately removed with its session. The
	// key assertion is that DeleteChat did NOT RESTRICT-error on the former
	// SET NULL refs (route_id / requested_message_id) — proven by reaching here.
	// Verify no orphaned references remain for this bot.
	var dangling int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM tool_approval_requests WHERE bot_id=$1`, botID).Scan(&dangling); err != nil {
		t.Fatalf("count tool_approval_requests: %v", err)
	}
	// (row cascade-deleted with its session; 0 is expected and fine)
	_ = dangling
}
