package message

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type externalMessagePositionQueries struct {
	dbstore.Queries
	arg  sqlc.ListExternalMessagePositionsBySessionParams
	rows []sqlc.ListExternalMessagePositionsBySessionRow
}

func (q *externalMessagePositionQueries) ListExternalMessagePositionsBySession(
	_ context.Context,
	arg sqlc.ListExternalMessagePositionsBySessionParams,
) ([]sqlc.ListExternalMessagePositionsBySessionRow, error) {
	q.arg = arg
	return q.rows, nil
}

func TestActiveSessionMessagePreservesDurableHistoryPosition(t *testing.T) {
	t.Parallel()

	message := toMessageFromActiveSinceBySessionRow(sqlc.ListActiveMessagesSinceBySessionRow{
		ID:             historyPositionUUID("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		BotID:          historyPositionUUID("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"),
		SessionID:      historyPositionUUID("cccccccc-cccc-cccc-cccc-cccccccccccc"),
		Role:           "assistant",
		Content:        []byte(`{"role":"assistant","content":"answer"}`),
		TurnPosition:   7,
		TurnMessageSeq: 3,
	})

	if message.TurnPosition != 7 || message.TurnMessageSequence != 3 {
		t.Fatalf("history position = %d/%d, want 7/3", message.TurnPosition, message.TurnMessageSequence)
	}
}

func TestMessageDoesNotSerializeInternalHistoryPosition(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(Message{TurnPosition: 7, TurnMessageSequence: 3})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	encoded := string(raw)
	if strings.Contains(encoded, "turn_position") || strings.Contains(encoded, "turn_message_sequence") {
		t.Fatalf("internal history position leaked into JSON: %s", encoded)
	}
}

func TestListExternalMessagePositionsBySessionReturnsEarliestDurablePositions(t *testing.T) {
	t.Parallel()

	queries := &externalMessagePositionQueries{rows: []sqlc.ListExternalMessagePositionsBySessionRow{{
		ExternalMessageID: "old",
		TurnPosition:      1,
		TurnMessageSeq:    1,
	}}}
	service := NewService(nil, queries)
	positions, err := service.ListExternalMessagePositionsBySession(
		context.Background(),
		"cccccccc-cccc-cccc-cccc-cccccccccccc",
		[]string{" old ", "", "old"},
	)
	if err != nil {
		t.Fatalf("ListExternalMessagePositionsBySession() error = %v", err)
	}
	if got := queries.arg.SessionID.String(); got != "cccccccc-cccc-cccc-cccc-cccccccccccc" {
		t.Fatalf("session id = %q", got)
	}
	if len(queries.arg.ExternalMessageIds) != 1 || queries.arg.ExternalMessageIds[0] != "old" {
		t.Fatalf("external ids = %#v, want old", queries.arg.ExternalMessageIds)
	}
	if len(positions) != 1 || positions[0] != (ExternalMessagePosition{
		ExternalMessageID:   "old",
		TurnPosition:        1,
		TurnMessageSequence: 1,
	}) {
		t.Fatalf("positions = %#v", positions)
	}
}

func historyPositionUUID(value string) pgtype.UUID {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		panic(err)
	}
	return id
}
