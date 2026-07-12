package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/messagesource"
)

func TestMessageWritersPersistExplicitSourceContextPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	ctx := context.Background()
	installListUncompactedFixture(t, ctx, pool)
	installSourceContextFixture(t, ctx, pool)
	applySourceContextMigration(t, ctx, pool, "0109_compaction_source_revision.up.sql")
	applySourceContextMigration(t, ctx, pool, "0110_message_source_context.up.sql")
	applySourceContextMigration(t, ctx, pool, "0111_activate_message_source_context.up.sql")

	botID, sessionID, routeID := testUUID(), testUUID(), testUUID()
	identityID, eventID := testUUID(), testUUID()
	insertSourceContextParents(t, ctx, pool, botID, sessionID, routeID, identityID, eventID)
	queries := sqlc.New(pool)

	requestID, turnID := testUUID(), testUUID()
	requestContext := messagesource.NewV1("explicit request", "request-platform", "group", "request-room")
	if _, err := queries.CreateMessageWithHistoryTurn(ctx, sqlc.CreateMessageWithHistoryTurnParams{
		SessionID: sessionID, MessageID: requestID, BotID: botID,
		Role: "user", Content: []byte(`{}`), Metadata: []byte(`{}`),
		SessionMode: "chat", RuntimeType: "model", SourceContext: encodeSourceContext(t, requestContext),
		TurnID: turnID, TurnMessageSeq: pgtype.Int8{Int64: 1, Valid: true},
	}); err != nil {
		t.Fatalf("create message with history turn: %v", err)
	}
	assertStoredSourceContext(t, ctx, pool, requestID, requestContext)

	withTurnID := testUUID()
	withTurnContext := messagesource.NewV1("explicit with turn", "turn-platform", "thread", "turn-room")
	if _, err := queries.CreateMessageWithTurn(ctx, sqlc.CreateMessageWithTurnParams{
		MessageID: withTurnID, BotID: botID, SessionID: sessionID,
		Role: "assistant", Content: []byte(`{}`), Metadata: []byte(`{}`),
		SessionMode: "chat", RuntimeType: "model", SourceContext: encodeSourceContext(t, withTurnContext),
		TurnID: turnID, TurnMessageSeq: pgtype.Int8{Int64: 2, Valid: true},
	}); err != nil {
		t.Fatalf("create message with turn: %v", err)
	}
	assertStoredSourceContext(t, ctx, pool, withTurnID, withTurnContext)

	requestAppendContext := messagesource.NewV1("explicit append", "append-platform", "private", "append-room")
	requestAppend, err := queries.CreateMessageInHistoryTurnByRequest(ctx, sqlc.CreateMessageInHistoryTurnByRequestParams{
		Role: "tool", SessionID: sessionID, RequestMessageID: requestID, BotID: botID,
		Content: []byte(`{}`), Metadata: []byte(`{}`), SessionMode: "chat", RuntimeType: "model",
		SourceContext: encodeSourceContext(t, requestAppendContext),
	})
	if err != nil {
		t.Fatalf("create message in request turn: %v", err)
	}
	assertStoredSourceContext(t, ctx, pool, requestAppend.ID, requestAppendContext)

	boundContext := messagesource.NewV1("explicit bind", "bind-platform", "group", "bind-room")
	bound, err := queries.CreateMessageInHistoryTurnByRequestAndBind(ctx, sqlc.CreateMessageInHistoryTurnByRequestAndBindParams{
		Role: "assistant", SessionID: sessionID, RequestMessageID: requestID, BotID: botID,
		Content: []byte(`{}`), Metadata: []byte(`{}`), SessionMode: "chat", RuntimeType: "model",
		SourceContext: encodeSourceContext(t, boundContext),
	})
	if err != nil {
		t.Fatalf("create and bind message in request turn: %v", err)
	}
	assertStoredSourceContext(t, ctx, pool, bound.ID, boundContext)

	toolTailContexts := []messagesource.Context{
		messagesource.NewV1("tail user", "web", "private", "tail-one"),
		messagesource.NewV1("tail assistant", "telegram", "group", "tail-two"),
		messagesource.NewV1("tail tool", "slack", "thread", "tail-three"),
		messagesource.NewV1("tail final", "matrix", "group", "tail-four"),
	}
	toolTailIDs := []pgtype.UUID{testUUID(), testUUID(), testUUID(), testUUID()}
	rows, err := queries.CreateToolTailRound(ctx, sqlc.CreateToolTailRoundParams{
		UserMessageID: toolTailIDs[0], UserContent: []byte(`{}`), UserMetadata: []byte(`{}`),
		UserSessionMode: "chat", UserRuntimeType: "model", UserSourceContext: encodeSourceContext(t, toolTailContexts[0]),
		ToolCallAssistantMessageID: toolTailIDs[1], ToolCallAssistantContent: []byte(`{}`), ToolCallAssistantMetadata: []byte(`{}`),
		ToolCallAssistantSessionMode: "chat", ToolCallAssistantRuntimeType: "model",
		ToolCallAssistantSourceContext: encodeSourceContext(t, toolTailContexts[1]),
		ToolMessageID:                  toolTailIDs[2], ToolContent: []byte(`{}`), ToolMetadata: []byte(`{}`),
		ToolSessionMode: "chat", ToolRuntimeType: "model", ToolSourceContext: encodeSourceContext(t, toolTailContexts[2]),
		FinalAssistantMessageID: toolTailIDs[3], FinalAssistantContent: []byte(`{}`), FinalAssistantMetadata: []byte(`{}`),
		FinalAssistantSessionMode: "chat", FinalAssistantRuntimeType: "model",
		FinalAssistantSourceContext: encodeSourceContext(t, toolTailContexts[3]),
		SessionID:                   sessionID, BotID: botID, TurnID: testUUID(),
	})
	if err != nil {
		t.Fatalf("create tool tail round: %v", err)
	}
	if len(rows) != len(toolTailIDs) {
		t.Fatalf("tool tail rows = %d, want %d", len(rows), len(toolTailIDs))
	}
	for i := range toolTailIDs {
		assertStoredSourceContext(t, ctx, pool, toolTailIDs[i], toolTailContexts[i])
	}

	invalidIDs := []pgtype.UUID{testUUID(), testUUID(), testUUID(), testUUID()}
	invalidContext := []byte(`{"version":2,"sender_display_name":"bad","platform":"bad","conversation_type":"group","conversation_name":"bad"}`)
	if _, err := queries.CreateToolTailRound(ctx, sqlc.CreateToolTailRoundParams{
		UserMessageID: invalidIDs[0], UserContent: []byte(`{}`), UserMetadata: []byte(`{}`),
		UserSessionMode: "chat", UserRuntimeType: "model", UserSourceContext: encodeSourceContext(t, toolTailContexts[0]),
		ToolCallAssistantMessageID: invalidIDs[1], ToolCallAssistantContent: []byte(`{}`), ToolCallAssistantMetadata: []byte(`{}`),
		ToolCallAssistantSessionMode: "chat", ToolCallAssistantRuntimeType: "model", ToolCallAssistantSourceContext: invalidContext,
		ToolMessageID: invalidIDs[2], ToolContent: []byte(`{}`), ToolMetadata: []byte(`{}`),
		ToolSessionMode: "chat", ToolRuntimeType: "model", ToolSourceContext: encodeSourceContext(t, toolTailContexts[2]),
		FinalAssistantMessageID: invalidIDs[3], FinalAssistantContent: []byte(`{}`), FinalAssistantMetadata: []byte(`{}`),
		FinalAssistantSessionMode: "chat", FinalAssistantRuntimeType: "model",
		FinalAssistantSourceContext: encodeSourceContext(t, toolTailContexts[3]),
		SessionID:                   sessionID, BotID: botID, TurnID: testUUID(),
	}); err == nil {
		t.Fatal("tool tail accepted invalid source context")
	}
	var invalidRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM bot_history_messages WHERE id = ANY($1::uuid[])`, invalidIDs).Scan(&invalidRows); err != nil {
		t.Fatalf("count invalid tool tail rows: %v", err)
	}
	if invalidRows != 0 {
		t.Fatalf("invalid tool tail persisted %d rows", invalidRows)
	}

	fallbackID := testUUID()
	if _, err := queries.CreateMessageWithHistoryTurn(ctx, sqlc.CreateMessageWithHistoryTurnParams{
		SessionID: sessionID, MessageID: fallbackID, BotID: botID, SenderChannelIdentityID: identityID,
		Role: "user", Content: []byte(`{}`), Metadata: []byte(`{}`), SessionMode: "chat", RuntimeType: "model",
		EventID: eventID, TurnID: testUUID(), TurnMessageSeq: pgtype.Int8{Int64: 1, Valid: true},
	}); err != nil {
		t.Fatalf("create trigger-fallback message: %v", err)
	}
	assertStoredSourceContext(t, ctx, pool, fallbackID,
		messagesource.NewV1("Event Alice", "slack", "group", "Event Room"))
}

func encodeSourceContext(t *testing.T, sourceContext messagesource.Context) []byte {
	t.Helper()
	raw, err := messagesource.Encode(sourceContext)
	if err != nil {
		t.Fatalf("encode source context: %v", err)
	}
	return raw
}
