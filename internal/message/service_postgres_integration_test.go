package message

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbpkg "github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
)

func TestPostgresReplaceTurnRetryHidesSupersededAssistant(t *testing.T) {
	ctx := context.Background()
	tx := beginPostgresMessageTestTx(t, ctx)
	setupPostgresMessageTestFixtures(t, ctx, tx)

	svc := NewService(nil, postgresstore.NewQueries(dbsqlc.New(tx)))
	user, err := svc.Persist(ctx, PersistInput{
		BotID:     postgresMessageTestBotID,
		SessionID: postgresMessageTestSessionID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"hello"}`),
	})
	if err != nil {
		t.Fatalf("persist user: %v", err)
	}
	assistant, err := svc.Persist(ctx, PersistInput{
		BotID:     postgresMessageTestBotID,
		SessionID: postgresMessageTestSessionID,
		Role:      "assistant",
		Content:   []byte(`{"role":"assistant","content":"old"}`),
	})
	if err != nil {
		t.Fatalf("persist assistant: %v", err)
	}
	oldTurn, err := svc.GetVisibleTurnByMessage(ctx, postgresMessageTestSessionID, assistant.ID)
	if err != nil {
		t.Fatalf("get old turn: %v", err)
	}
	replacement, err := svc.Persist(ctx, PersistInput{
		BotID:           postgresMessageTestBotID,
		SessionID:       postgresMessageTestSessionID,
		Role:            "assistant",
		Content:         []byte(`{"role":"assistant","content":"new"}`),
		SkipHistoryTurn: true,
	})
	if err != nil {
		t.Fatalf("persist replacement assistant: %v", err)
	}
	if _, err := svc.ReplaceTurn(ctx, postgresMessageTestSessionID, oldTurn.ID, user.ID, replacement.ID, "retry"); err != nil {
		t.Fatalf("replace turn: %v", err)
	}

	assertPostgresVisibleMessageIDs(t, ctx, svc, user.ID, replacement.ID)
	assertPostgresMessageVisibility(t, ctx, tx, assistant.ID, false, true)
}

func TestPostgresReplaceTurnAdvancesSessionCompactionEpoch(t *testing.T) {
	ctx := context.Background()
	tx := beginPostgresMessageTestTx(t, ctx)
	setupPostgresMessageTestFixtures(t, ctx, tx)

	svc := NewService(nil, postgresstore.NewQueries(dbsqlc.New(tx)))
	user, err := svc.Persist(ctx, PersistInput{
		BotID:     postgresMessageTestBotID,
		SessionID: postgresMessageTestSessionID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"hello"}`),
	})
	if err != nil {
		t.Fatalf("persist user: %v", err)
	}
	assistant, err := svc.Persist(ctx, PersistInput{
		BotID:     postgresMessageTestBotID,
		SessionID: postgresMessageTestSessionID,
		Role:      "assistant",
		Content:   []byte(`{"role":"assistant","content":"old"}`),
	})
	if err != nil {
		t.Fatalf("persist assistant: %v", err)
	}
	oldTurn, err := svc.GetVisibleTurnByMessage(ctx, postgresMessageTestSessionID, assistant.ID)
	if err != nil {
		t.Fatalf("get old turn: %v", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_history_message_compacts (id, bot_id, session_id, status, summary)
		VALUES
			('00000000-0000-0000-0000-000000074131', $1, $2, 'ok', 'covers replaced source'),
			('00000000-0000-0000-0000-000000074132', $1, $2, 'ok', 'same session frontier')
	`, postgresMessageTestBotID, postgresMessageTestSessionID); err != nil {
		t.Fatalf("insert compaction artifacts: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE bot_history_messages
		SET compact_id = '00000000-0000-0000-0000-000000074131'
		WHERE id = $1
	`, assistant.ID); err != nil {
		t.Fatalf("link assistant to compaction artifact: %v", err)
	}

	replacement, err := svc.Persist(ctx, PersistInput{
		BotID:           postgresMessageTestBotID,
		SessionID:       postgresMessageTestSessionID,
		Role:            "assistant",
		Content:         []byte(`{"role":"assistant","content":"new"}`),
		SkipHistoryTurn: true,
	})
	if err != nil {
		t.Fatalf("persist replacement assistant: %v", err)
	}
	if _, err := svc.ReplaceTurn(ctx, postgresMessageTestSessionID, oldTurn.ID, user.ID, replacement.ID, "retry"); err != nil {
		t.Fatalf("replace turn: %v", err)
	}

	var sessionEpoch int64
	if err := tx.QueryRow(ctx, `SELECT compaction_epoch FROM bot_sessions WHERE id = $1`, postgresMessageTestSessionID).Scan(&sessionEpoch); err != nil {
		t.Fatalf("read session compaction epoch: %v", err)
	}
	if sessionEpoch != 1 {
		var compactID *string
		var artifactEpoch *int64
		if err := tx.QueryRow(ctx, `
			SELECT message.compact_id::text, compact.compaction_epoch
			FROM bot_history_messages message
			LEFT JOIN bot_history_message_compacts compact ON compact.id = message.compact_id
			WHERE message.id = $1
		`, assistant.ID).Scan(&compactID, &artifactEpoch); err != nil {
			t.Fatalf("diagnose unchanged compaction epoch: %v", err)
		}
		compactValue := "<nil>"
		if compactID != nil {
			compactValue = *compactID
		}
		artifactValue := int64(-1)
		if artifactEpoch != nil {
			artifactValue = *artifactEpoch
		}
		t.Fatalf("session compaction epoch = %d, want 1 (message compact_id=%s artifact_epoch=%d)", sessionEpoch, compactValue, artifactValue)
	}
	var currentArtifacts int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM bot_history_message_compacts compact
		JOIN bot_sessions session ON session.id = compact.session_id
		WHERE compact.session_id = $1
		  AND compact.compaction_epoch = session.compaction_epoch
	`, postgresMessageTestSessionID).Scan(&currentArtifacts); err != nil {
		t.Fatalf("count current compaction artifacts: %v", err)
	}
	if currentArtifacts != 0 {
		t.Fatalf("current compaction artifacts = %d, want 0 after replacement", currentArtifacts)
	}
}

func TestPostgresReplaceTurnEditHidesSupersededTurn(t *testing.T) {
	ctx := context.Background()
	tx := beginPostgresMessageTestTx(t, ctx)
	setupPostgresMessageTestFixtures(t, ctx, tx)

	svc := NewService(nil, postgresstore.NewQueries(dbsqlc.New(tx)))
	oldUser, err := svc.Persist(ctx, PersistInput{
		BotID:     postgresMessageTestBotID,
		SessionID: postgresMessageTestSessionID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"old prompt"}`),
	})
	if err != nil {
		t.Fatalf("persist old user: %v", err)
	}
	oldAssistant, err := svc.Persist(ctx, PersistInput{
		BotID:     postgresMessageTestBotID,
		SessionID: postgresMessageTestSessionID,
		Role:      "assistant",
		Content:   []byte(`{"role":"assistant","content":"old answer"}`),
	})
	if err != nil {
		t.Fatalf("persist old assistant: %v", err)
	}
	oldTurn, err := svc.GetVisibleTurnByMessage(ctx, postgresMessageTestSessionID, oldAssistant.ID)
	if err != nil {
		t.Fatalf("get old turn: %v", err)
	}
	newUser, err := svc.Persist(ctx, PersistInput{
		BotID:           postgresMessageTestBotID,
		SessionID:       postgresMessageTestSessionID,
		Role:            "user",
		Content:         []byte(`{"role":"user","content":"new prompt"}`),
		SkipHistoryTurn: true,
	})
	if err != nil {
		t.Fatalf("persist new user: %v", err)
	}
	newAssistant, err := svc.Persist(ctx, PersistInput{
		BotID:           postgresMessageTestBotID,
		SessionID:       postgresMessageTestSessionID,
		Role:            "assistant",
		Content:         []byte(`{"role":"assistant","content":"new answer"}`),
		SkipHistoryTurn: true,
	})
	if err != nil {
		t.Fatalf("persist new assistant: %v", err)
	}
	if _, err := svc.ReplaceTurn(ctx, postgresMessageTestSessionID, oldTurn.ID, newUser.ID, newAssistant.ID, "edit"); err != nil {
		t.Fatalf("replace turn: %v", err)
	}

	assertPostgresVisibleMessageIDs(t, ctx, svc, newUser.ID, newAssistant.ID)
	assertPostgresMessageVisibility(t, ctx, tx, oldUser.ID, false, true)
	assertPostgresMessageVisibility(t, ctx, tx, oldAssistant.ID, false, true)
}

func TestPostgresRepairSupersededMessageVisibilityMigration(t *testing.T) {
	ctx := context.Background()
	tx := beginPostgresMessageTestTx(t, ctx)
	setupPostgresMessageTestFixtures(t, ctx, tx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_history_messages (
			id, bot_id, session_id, role, content,
			turn_id, turn_position, turn_message_seq, turn_visible, turn_superseded_at
		)
		VALUES
			(
				'00000000-0000-0000-0000-000000074111',
				$1, $2, 'assistant', '{"role":"assistant","content":"bad"}'::jsonb,
				'00000000-0000-0000-0000-000000074121', 1, 2, true, now()
			),
			(
				'00000000-0000-0000-0000-000000074112',
				$1, $2, 'assistant', '{"role":"assistant","content":"good"}'::jsonb,
				'00000000-0000-0000-0000-000000074122', 2, 2, true, NULL
			)
	`, postgresMessageTestBotID, postgresMessageTestSessionID); err != nil {
		t.Fatalf("insert repair fixtures: %v", err)
	}

	sql, err := os.ReadFile("../../db/postgres/migrations/0105_repair_superseded_message_visibility.up.sql")
	if err != nil {
		t.Fatalf("read repair migration: %v", err)
	}
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("run repair migration: %v", err)
	}

	assertPostgresMessageVisibility(t, ctx, tx, "00000000-0000-0000-0000-000000074111", false, true)
	assertPostgresMessageVisibility(t, ctx, tx, "00000000-0000-0000-0000-000000074112", true, false)
}

func TestPostgresInjectedRequestReplaysOnlyItsFollowingResponses(t *testing.T) {
	ctx := context.Background()
	tx := beginPostgresMessageTestTx(t, ctx)
	setupPostgresMessageTestFixtures(t, ctx, tx)

	const (
		firstEventID     = "00000000-0000-0000-0000-000000074201"
		turnID           = "00000000-0000-0000-0000-000000074202"
		originalUserID   = "00000000-0000-0000-0000-000000074203"
		priorAssistantID = "00000000-0000-0000-0000-000000074204"
		firstInjectedID  = "00000000-0000-0000-0000-000000074205"
		finalAssistantID = "00000000-0000-0000-0000-000000074206"
		secondEventID    = "00000000-0000-0000-0000-000000074207"
		secondInjectedID = "00000000-0000-0000-0000-000000074208"
	)
	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_session_events (
			id, bot_id, session_id, event_kind, event_data, external_message_id, received_at_ms
		)
		VALUES
			($1, $3, $4, 'message', '{}'::jsonb, 'injected-request-1', 1),
			($2, $3, $4, 'message', '{}'::jsonb, 'injected-request-2', 2)
	`, firstEventID, secondEventID, postgresMessageTestBotID, postgresMessageTestSessionID); err != nil {
		t.Fatalf("insert injected event: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_history_messages (
			id, bot_id, session_id, role, content, metadata, event_id,
			turn_id, turn_position, turn_message_seq, turn_visible
		)
		VALUES
			($1, $7, $8, 'user', '{"role":"user","content":"original"}'::jsonb, '{}'::jsonb, NULL, $9, 1, 1, true),
			($2, $7, $8, 'assistant', '{"role":"assistant","content":"prior"}'::jsonb, '{}'::jsonb, NULL, $9, 1, 2, true),
			($3, $7, $8, 'user', '{"role":"user","content":"first injected"}'::jsonb,
			 '{"pipeline_delivery_state":"pending"}'::jsonb, $4, $9, 1, 3, true),
			($5, $7, $8, 'user', '{"role":"user","content":"second injected"}'::jsonb,
			 '{"pipeline_delivery_state":"pending"}'::jsonb, $6, $9, 1, 4, true)
	`, originalUserID, priorAssistantID, firstInjectedID, firstEventID,
		secondInjectedID, secondEventID, postgresMessageTestBotID, postgresMessageTestSessionID, turnID); err != nil {
		t.Fatalf("insert injected turn: %v", err)
	}

	queries := dbsqlc.New(tx)
	state, err := queries.GetSessionEventDeliveryState(ctx, mustTestUUID(t, firstEventID))
	if err != nil {
		t.Fatalf("read injected delivery state before response: %v", err)
	}
	if state.HistoryMessageID.String() != firstInjectedID || state.ResponsePersisted || state.ReplayResponsePersisted {
		t.Fatalf("delivery state before response = request:%s covered:%t replay:%t, want %s/false/false",
			state.HistoryMessageID.String(), state.ResponsePersisted, state.ReplayResponsePersisted, firstInjectedID)
	}

	service := NewService(nil, postgresstore.NewQueries(queries))
	responses, err := service.ListVisibleTurnResponsesByRequest(ctx, postgresMessageTestSessionID, firstInjectedID)
	if err != nil {
		t.Fatalf("list injected responses before response: %v", err)
	}
	if len(responses) != 0 {
		t.Fatalf("responses before injected reply = %#v, want none", responses)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_history_messages (
			id, bot_id, session_id, role, content, metadata,
			turn_id, turn_position, turn_message_seq, turn_visible
		)
		VALUES ($1, $2, $3, 'assistant', '{"role":"assistant","content":"final"}'::jsonb,
			'{}'::jsonb, $4, 1, 5, true)
	`, finalAssistantID, postgresMessageTestBotID, postgresMessageTestSessionID, turnID); err != nil {
		t.Fatalf("insert response after injected request: %v", err)
	}
	state, err = queries.GetSessionEventDeliveryState(ctx, mustTestUUID(t, firstEventID))
	if err != nil {
		t.Fatalf("read first injected delivery state after response: %v", err)
	}
	if !state.ResponsePersisted || state.ReplayResponsePersisted {
		t.Fatalf("first injected delivery coverage/replay = %t/%t, want true/false", state.ResponsePersisted, state.ReplayResponsePersisted)
	}
	responses, err = service.ListVisibleTurnResponsesByRequest(ctx, postgresMessageTestSessionID, firstInjectedID)
	if err != nil {
		t.Fatalf("list first injected responses after response: %v", err)
	}
	if len(responses) != 0 {
		t.Fatalf("first injected responses = %#v, want none owned by later request", responses)
	}

	state, err = queries.GetSessionEventDeliveryState(ctx, mustTestUUID(t, secondEventID))
	if err != nil {
		t.Fatalf("read second injected delivery state after response: %v", err)
	}
	if !state.ResponsePersisted || !state.ReplayResponsePersisted {
		t.Fatalf("second injected delivery coverage/replay = %t/%t, want true/true", state.ResponsePersisted, state.ReplayResponsePersisted)
	}
	responses, err = service.ListVisibleTurnResponsesByRequest(ctx, postgresMessageTestSessionID, secondInjectedID)
	if err != nil {
		t.Fatalf("list second injected responses after response: %v", err)
	}
	if len(responses) != 1 || responses[0].ID != finalAssistantID {
		t.Fatalf("second injected responses = %#v, want only %s", responses, finalAssistantID)
	}
}

func TestPostgresInjectedRequestsOwnOnlyResponsesBeforeNextEventUser(t *testing.T) {
	ctx := context.Background()
	tx := beginPostgresMessageTestTx(t, ctx)
	setupPostgresMessageTestFixtures(t, ctx, tx)

	const (
		firstEventID      = "00000000-0000-0000-0000-000000074221"
		secondEventID     = "00000000-0000-0000-0000-000000074222"
		turnID            = "00000000-0000-0000-0000-000000074223"
		firstUserID       = "00000000-0000-0000-0000-000000074224"
		firstAssistantID  = "00000000-0000-0000-0000-000000074225"
		secondUserID      = "00000000-0000-0000-0000-000000074226"
		secondAssistantID = "00000000-0000-0000-0000-000000074227"
	)
	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_session_events (
			id, bot_id, session_id, event_kind, event_data, external_message_id, received_at_ms
		)
		VALUES
			($1, $3, $4, 'message', '{}'::jsonb, 'owned-request-1', 1),
			($2, $3, $4, 'message', '{}'::jsonb, 'owned-request-2', 2)
	`, firstEventID, secondEventID, postgresMessageTestBotID, postgresMessageTestSessionID); err != nil {
		t.Fatalf("insert owned response events: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_history_messages (
			id, bot_id, session_id, role, content, metadata, event_id,
			turn_id, turn_position, turn_message_seq, turn_visible
		)
		VALUES
			($1, $7, $8, 'user', '{"role":"user","content":"first"}'::jsonb, '{}'::jsonb, $2, $9, 1, 1, true),
			($3, $7, $8, 'assistant', '{"role":"assistant","content":"first answer"}'::jsonb, '{}'::jsonb, NULL, $9, 1, 2, true),
			($4, $7, $8, 'user', '{"role":"user","content":"second"}'::jsonb, '{}'::jsonb, $5, $9, 1, 3, true),
			($6, $7, $8, 'assistant', '{"role":"assistant","content":"second answer"}'::jsonb, '{}'::jsonb, NULL, $9, 1, 4, true)
	`, firstUserID, firstEventID, firstAssistantID, secondUserID, secondEventID, secondAssistantID,
		postgresMessageTestBotID, postgresMessageTestSessionID, turnID); err != nil {
		t.Fatalf("insert owned response turn: %v", err)
	}

	queries := dbsqlc.New(tx)
	service := NewService(nil, postgresstore.NewQueries(queries))
	for _, tc := range []struct {
		eventID    string
		requestID  string
		responseID string
	}{
		{eventID: firstEventID, requestID: firstUserID, responseID: firstAssistantID},
		{eventID: secondEventID, requestID: secondUserID, responseID: secondAssistantID},
	} {
		state, err := queries.GetSessionEventDeliveryState(ctx, mustTestUUID(t, tc.eventID))
		if err != nil {
			t.Fatalf("read delivery state for %s: %v", tc.eventID, err)
		}
		if !state.ResponsePersisted || !state.ReplayResponsePersisted {
			t.Fatalf("delivery %s coverage/replay = %t/%t, want true/true", tc.eventID, state.ResponsePersisted, state.ReplayResponsePersisted)
		}
		responses, err := service.ListVisibleTurnResponsesByRequest(ctx, postgresMessageTestSessionID, tc.requestID)
		if err != nil {
			t.Fatalf("list responses for %s: %v", tc.requestID, err)
		}
		if len(responses) != 1 || responses[0].ID != tc.responseID {
			t.Fatalf("responses for %s = %#v, want %s", tc.requestID, responses, tc.responseID)
		}
	}
}

const (
	postgresMessageTestUserID    = "00000000-0000-0000-0000-000000074101"
	postgresMessageTestBotID     = "00000000-0000-0000-0000-000000074102"
	postgresMessageTestSessionID = "00000000-0000-0000-0000-000000074103"
)

func beginPostgresMessageTestTx(t *testing.T, ctx context.Context) pgx.Tx {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("skip postgres integration test: TEST_POSTGRES_DSN is not set")
	}
	pool, err := dbpkg.OpenPostgresDSN(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to configured postgres integration database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping configured postgres integration database: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	return tx
}

func setupPostgresMessageTestFixtures(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	name := fmt.Sprintf("postgres-message-test-%d", time.Now().UnixNano())
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, role, is_active)
		VALUES ($1, $2, 'admin', true)
	`, postgresMessageTestUserID, name); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bots (id, owner_user_id, name)
		VALUES ($1, $2, $3)
	`, postgresMessageTestBotID, postgresMessageTestUserID, name); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_sessions (id, bot_id, channel_type)
		VALUES ($1, $2, 'local')
	`, postgresMessageTestSessionID, postgresMessageTestBotID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
}

func assertPostgresVisibleMessageIDs(t *testing.T, ctx context.Context, svc *DBService, want ...string) {
	t.Helper()
	messages, err := svc.ListBySession(ctx, postgresMessageTestSessionID)
	if err != nil {
		t.Fatalf("list session messages: %v", err)
	}
	got := make([]string, 0, len(messages))
	for _, msg := range messages {
		got = append(got, msg.ID)
	}
	if len(got) != len(want) {
		t.Fatalf("visible message ids = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("visible message ids = %#v, want %#v", got, want)
		}
	}
}

func assertPostgresMessageVisibility(t *testing.T, ctx context.Context, tx pgx.Tx, messageID string, wantVisible bool, wantSuperseded bool) {
	t.Helper()
	pgMessageID, err := dbpkg.ParseUUID(messageID)
	if err != nil {
		t.Fatalf("parse message id: %v", err)
	}
	var visible bool
	var superseded bool
	if err := tx.QueryRow(ctx, `
		SELECT turn_visible, turn_superseded_at IS NOT NULL
		FROM bot_history_messages
		WHERE id = $1
	`, pgMessageID).Scan(&visible, &superseded); err != nil {
		t.Fatalf("read message visibility: %v", err)
	}
	if visible != wantVisible || superseded != wantSuperseded {
		t.Fatalf("message %s visible/superseded = %v/%v, want %v/%v", messageID, visible, superseded, wantVisible, wantSuperseded)
	}
}
