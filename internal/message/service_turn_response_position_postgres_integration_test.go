package message

import (
	"context"
	"testing"
	"time"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
)

func TestPostgresListUncoveredTurnResponsesPreservesDurableOrder(t *testing.T) {
	ctx := context.Background()
	tx := beginPostgresMessageTestTx(t, ctx)
	setupPostgresMessageTestFixtures(t, ctx, tx)

	const (
		firstTurnID       = "00000000-0000-0000-0000-000000074301"
		firstUserID       = "00000000-0000-0000-0000-000000074302"
		firstAssistantID  = "00000000-0000-0000-0000-000000074303"
		secondTurnID      = "00000000-0000-0000-0000-000000074304"
		secondUserID      = "00000000-0000-0000-0000-000000074305"
		secondAssistantID = "00000000-0000-0000-0000-000000074306"
	)
	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_history_messages (
			id, bot_id, session_id, role, content,
			turn_id, turn_position, turn_message_seq, turn_visible, created_at
		)
		VALUES
			($1, $7, $8, 'user', '{"role":"user","content":"first"}'::jsonb, $5, 1, 1, true, now() - interval '4 minutes'),
			($2, $7, $8, 'assistant', '{"role":"assistant","content":"first answer"}'::jsonb, $5, 1, 2, true, now() - interval '1 minute'),
			($3, $7, $8, 'user', '{"role":"user","content":"second"}'::jsonb, $6, 2, 1, true, now() - interval '3 minutes'),
			($4, $7, $8, 'assistant', '{"role":"assistant","content":"second answer"}'::jsonb, $6, 2, 2, true, now() - interval '2 minutes')
	`, firstUserID, firstAssistantID, secondUserID, secondAssistantID,
		firstTurnID, secondTurnID, postgresMessageTestBotID, postgresMessageTestSessionID); err != nil {
		t.Fatalf("insert positioned turn responses: %v", err)
	}

	service := NewService(nil, postgresstore.NewQueries(dbsqlc.New(tx)))
	responses, err := service.ListUncoveredTurnResponsesBySession(
		ctx,
		postgresMessageTestSessionID,
		time.Now().UTC().Add(-time.Hour),
		nil,
	)
	if err != nil {
		t.Fatalf("list uncovered turn responses: %v", err)
	}
	want := []struct {
		id       string
		position int64
		sequence int64
	}{
		{id: firstAssistantID, position: 1, sequence: 2},
		{id: secondAssistantID, position: 2, sequence: 2},
	}
	if len(responses) != len(want) {
		t.Fatalf("uncovered turn responses = %#v, want %d", responses, len(want))
	}
	for i, expected := range want {
		response := responses[i]
		if response.ID != expected.id || response.TurnPosition != expected.position || response.TurnMessageSequence != expected.sequence {
			t.Fatalf("response %d identity/position = %s %d/%d, want %s %d/%d", i,
				response.ID, response.TurnPosition, response.TurnMessageSequence,
				expected.id, expected.position, expected.sequence)
		}
	}
}
