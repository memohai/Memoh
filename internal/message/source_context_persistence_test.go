package message

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/messagesource"
)

type sourceContextDirectQueries struct {
	*runtimeSnapshotQueries
	userArg     sqlc.CreateMessageWithHistoryTurnParams
	responseArg sqlc.CreateMessageInHistoryTurnByRequestAndBindParams
	toolTailArg sqlc.CreateToolTailRoundParams
	responseErr error
	toolCalls   int
}

func (*sourceContextDirectQueries) SupportsAtomicDirectHistoryTurnWrites() bool {
	return true
}

func (q *sourceContextDirectQueries) CreateMessageWithHistoryTurn(
	_ context.Context,
	arg sqlc.CreateMessageWithHistoryTurnParams,
) (sqlc.CreateMessageWithHistoryTurnRow, error) {
	q.userArg = arg
	return sqlc.CreateMessageWithHistoryTurnRow{
		ID:        arg.MessageID,
		CreatedAt: pgtype.Timestamptz{Valid: true},
	}, nil
}

func (q *sourceContextDirectQueries) CreateMessageInHistoryTurnByRequestAndBind(
	_ context.Context,
	arg sqlc.CreateMessageInHistoryTurnByRequestAndBindParams,
) (sqlc.CreateMessageInHistoryTurnByRequestAndBindRow, error) {
	q.responseArg = arg
	if q.responseErr != nil {
		return sqlc.CreateMessageInHistoryTurnByRequestAndBindRow{}, q.responseErr
	}
	return sqlc.CreateMessageInHistoryTurnByRequestAndBindRow{
		ID:        testMessageUUID("88888888-8888-8888-8888-888888888888"),
		CreatedAt: pgtype.Timestamptz{Valid: true},
	}, nil
}

func (q *sourceContextDirectQueries) CreateToolTailRound(
	_ context.Context,
	arg sqlc.CreateToolTailRoundParams,
) ([]sqlc.CreateToolTailRoundRow, error) {
	q.toolTailArg = arg
	q.toolCalls++
	return []sqlc.CreateToolTailRoundRow{
		{ID: arg.UserMessageID, CreatedAt: pgtype.Timestamptz{Valid: true}},
		{ID: arg.ToolCallAssistantMessageID, CreatedAt: pgtype.Timestamptz{Valid: true}},
		{ID: arg.ToolMessageID, CreatedAt: pgtype.Timestamptz{Valid: true}},
		{ID: arg.FinalAssistantMessageID, CreatedAt: pgtype.Timestamptz{Valid: true}},
	}, nil
}

func TestPersistCarriesSourceContextThroughAtomicDirectWriters(t *testing.T) {
	sourceContext := messagesource.NewV1("Historical Sender", "telegram", "group", "Historical Room")

	t.Run("user turn", func(t *testing.T) {
		queries := &sourceContextDirectQueries{runtimeSnapshotQueries: &runtimeSnapshotQueries{}}
		svc := NewService(nil, queries)
		msg, err := svc.Persist(context.Background(), PersistInput{
			BotID: "11111111-1111-1111-1111-111111111111", SessionID: "22222222-2222-2222-2222-222222222222",
			Role: "user", Content: []byte(`{"type":"text","text":"hello"}`), SourceContext: sourceContext,
		})
		if err != nil {
			t.Fatalf("Persist() error = %v", err)
		}
		assertEncodedSourceContext(t, queries.userArg.SourceContext, sourceContext)
		if msg.SourceContext != sourceContext {
			t.Fatalf("returned source context = %+v, want %+v", msg.SourceContext, sourceContext)
		}
	})

	t.Run("assistant append", func(t *testing.T) {
		queries := &sourceContextDirectQueries{runtimeSnapshotQueries: &runtimeSnapshotQueries{}}
		svc := NewService(nil, queries)
		msg, err := svc.Persist(context.Background(), PersistInput{
			BotID: "11111111-1111-1111-1111-111111111111", SessionID: "22222222-2222-2222-2222-222222222222",
			Role: "assistant", Content: []byte(`{"type":"text","text":"hello"}`), SourceContext: sourceContext,
			TurnRequestMessageID: "77777777-7777-7777-7777-777777777777",
		})
		if err != nil {
			t.Fatalf("Persist() error = %v", err)
		}
		assertEncodedSourceContext(t, queries.responseArg.SourceContext, sourceContext)
		if msg.SourceContext != sourceContext {
			t.Fatalf("returned source context = %+v, want %+v", msg.SourceContext, sourceContext)
		}
	})

	t.Run("assistant direct fallback", func(t *testing.T) {
		queries := &sourceContextDirectQueries{
			runtimeSnapshotQueries: &runtimeSnapshotQueries{},
			responseErr:            pgx.ErrNoRows,
		}
		svc := NewService(nil, queries)
		msg, err := svc.Persist(context.Background(), PersistInput{
			BotID: "11111111-1111-1111-1111-111111111111", SessionID: "22222222-2222-2222-2222-222222222222",
			Role: "assistant", Content: []byte(`{"type":"text","text":"hello"}`), SourceContext: sourceContext,
			TurnRequestMessageID: "77777777-7777-7777-7777-777777777777",
		})
		if err != nil {
			t.Fatalf("Persist() error = %v", err)
		}
		assertEncodedSourceContext(t, queries.created.SourceContext, sourceContext)
		if msg.SourceContext != sourceContext {
			t.Fatalf("returned source context = %+v, want %+v", msg.SourceContext, sourceContext)
		}
	})
}

func TestPersistToolTailRoundCarriesEachSourceContext(t *testing.T) {
	queries := &sourceContextDirectQueries{runtimeSnapshotQueries: &runtimeSnapshotQueries{}}
	svc := NewService(nil, queries)
	roles := []string{"user", "assistant", "tool", "assistant"}
	contexts := []messagesource.Context{
		messagesource.NewV1("one", "web", "private", "one"),
		messagesource.NewV1("two", "telegram", "group", "two"),
		messagesource.NewV1("three", "slack", "thread", "three"),
		messagesource.NewV1("four", "matrix", "group", "four"),
	}
	inputs := make([]PersistInput, len(roles))
	for i := range roles {
		inputs[i] = PersistInput{
			BotID: "11111111-1111-1111-1111-111111111111", SessionID: "22222222-2222-2222-2222-222222222222",
			Role: roles[i], Content: []byte(`{"type":"text","text":"hello"}`), SourceContext: contexts[i],
		}
	}
	messages, handled, err := svc.PersistToolTailRound(context.Background(), inputs)
	if err != nil || !handled {
		t.Fatalf("PersistToolTailRound() handled=%v error=%v", handled, err)
	}
	got := [][]byte{
		queries.toolTailArg.UserSourceContext,
		queries.toolTailArg.ToolCallAssistantSourceContext,
		queries.toolTailArg.ToolSourceContext,
		queries.toolTailArg.FinalAssistantSourceContext,
	}
	for i := range contexts {
		assertEncodedSourceContext(t, got[i], contexts[i])
		if messages[i].SourceContext != contexts[i] {
			t.Fatalf("returned source context %d = %+v, want %+v", i, messages[i].SourceContext, contexts[i])
		}
	}
}

func TestPersistToolTailRoundRejectsInvalidSourceContextBeforeWrite(t *testing.T) {
	queries := &sourceContextDirectQueries{runtimeSnapshotQueries: &runtimeSnapshotQueries{}}
	svc := NewService(nil, queries)
	roles := []string{"user", "assistant", "tool", "assistant"}
	inputs := make([]PersistInput, len(roles))
	for i, role := range roles {
		inputs[i] = PersistInput{
			BotID: "11111111-1111-1111-1111-111111111111", SessionID: "22222222-2222-2222-2222-222222222222",
			Role: role, Content: []byte(`{}`), SourceContext: messagesource.NewV1("sender", "web", "private", "room"),
		}
	}
	inputs[1].SourceContext = messagesource.Context{Version: 2}
	if messages, handled, err := svc.PersistToolTailRound(context.Background(), inputs); err == nil || !handled || messages != nil {
		t.Fatalf("PersistToolTailRound() messages=%v handled=%v error=%v", messages, handled, err)
	}
	if queries.toolCalls != 0 {
		t.Fatalf("tool tail writes = %d, want 0", queries.toolCalls)
	}
}

func assertEncodedSourceContext(t *testing.T, raw []byte, want messagesource.Context) {
	t.Helper()
	got, err := messagesource.Decode(raw)
	if err != nil {
		t.Fatalf("decode source context: %v", err)
	}
	if got != want {
		t.Fatalf("source context = %+v, want %+v", got, want)
	}
}
