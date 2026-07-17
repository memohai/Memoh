package message

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type turnResponseReplayQueries struct {
	dbstore.Queries
	arg  sqlc.ListVisibleTurnResponsesByRequestParams
	rows []sqlc.ListVisibleTurnResponsesByRequestRow
}

func (q *turnResponseReplayQueries) ListVisibleTurnResponsesByRequest(
	_ context.Context,
	arg sqlc.ListVisibleTurnResponsesByRequestParams,
) ([]sqlc.ListVisibleTurnResponsesByRequestRow, error) {
	q.arg = arg
	return q.rows, nil
}

func TestListVisibleTurnResponsesByRequestUsesExactTurnAnchor(t *testing.T) {
	const (
		sessionID = "22222222-2222-2222-2222-222222222222"
		requestID = "44444444-4444-4444-4444-444444444444"
	)
	queries := &turnResponseReplayQueries{rows: []sqlc.ListVisibleTurnResponsesByRequestRow{
		{ID: replayTestUUID("55555555-5555-5555-5555-555555555555"), Role: "assistant", Content: []byte(`{"role":"assistant","content":"calling"}`), CreatedAt: pgtype.Timestamptz{Time: time.Unix(1, 0), Valid: true}},
		{ID: replayTestUUID("66666666-6666-6666-6666-666666666666"), Role: "tool", Content: []byte(`{"role":"tool","content":"done"}`), CreatedAt: pgtype.Timestamptz{Time: time.Unix(2, 0), Valid: true}},
	}}
	service := NewService(nil, queries)

	messages, err := service.ListVisibleTurnResponsesByRequest(context.Background(), sessionID, requestID)
	if err != nil {
		t.Fatalf("ListVisibleTurnResponsesByRequest() error = %v", err)
	}
	if queries.arg.SessionID.String() != sessionID || queries.arg.RequestMessageID.String() != requestID {
		t.Fatalf("query anchor = %s/%s, want %s/%s", queries.arg.SessionID.String(), queries.arg.RequestMessageID.String(), sessionID, requestID)
	}
	if len(messages) != 2 || messages[0].Role != "assistant" || messages[1].Role != "tool" {
		t.Fatalf("messages = %#v, want ordered assistant/tool tail", messages)
	}
}

func TestTurnResponseReplayQueriesStartAfterArbitraryRequest(t *testing.T) {
	messagesSQL, err := os.ReadFile("../../db/postgres/queries/messages.sql")
	if err != nil {
		t.Fatalf("read message queries: %v", err)
	}
	responseQuery := namedQuery(t, string(messagesSQL), "ListVisibleTurnResponsesByRequest")
	for _, required := range []string{
		"request.turn_id",
		"request.turn_message_seq",
		"MIN(next_request.turn_message_seq)",
		"next_request.event_id IS NOT NULL",
		"response.turn_message_seq > target.turn_message_seq",
		"response.turn_message_seq < target.next_event_user_seq",
	} {
		if !strings.Contains(responseQuery, required) {
			t.Fatalf("ListVisibleTurnResponsesByRequest is missing %q", required)
		}
	}
	if strings.Contains(responseQuery, "request.turn_message_seq = 1") {
		t.Fatal("ListVisibleTurnResponsesByRequest only accepts a turn-leading request")
	}

	eventsSQL, err := os.ReadFile("../../db/postgres/queries/session_events.sql")
	if err != nil {
		t.Fatalf("read session event queries: %v", err)
	}
	deliveryQuery := namedQuery(t, string(eventsSQL), "GetSessionEventDeliveryState")
	if count := strings.Count(deliveryQuery, "response.turn_message_seq > history.turn_message_seq"); count != 2 {
		t.Fatalf("GetSessionEventDeliveryState strict response boundaries = %d, want 2", count)
	}
	for _, required := range []string{
		"AS replay_response_persisted",
		"next_request.event_id IS NOT NULL",
		"next_request.turn_message_seq > visible_history.turn_message_seq",
		"next_request.turn_message_seq < response.turn_message_seq",
	} {
		if !strings.Contains(deliveryQuery, required) {
			t.Fatalf("GetSessionEventDeliveryState is missing %q", required)
		}
	}
	completionQuery := namedQuery(t, string(eventsSQL), "CompleteSessionEventDelivery")
	if count := strings.Count(completionQuery, "response.turn_message_seq > history.turn_message_seq"); count != 1 {
		t.Fatalf("CompleteSessionEventDelivery response lower bounds = %d, want 1", count)
	}
	if strings.Contains(completionQuery, "next_request") {
		t.Fatal("CompleteSessionEventDelivery incorrectly requires strict replay ownership")
	}
}

func namedQuery(t *testing.T, source, name string) string {
	t.Helper()
	startMarker := "-- name: " + name + " "
	start := strings.Index(source, startMarker)
	if start < 0 {
		t.Fatalf("query %s not found", name)
	}
	rest := source[start+len(startMarker):]
	if end := strings.Index(rest, "\n-- name: "); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

func replayTestUUID(value string) pgtype.UUID {
	var result pgtype.UUID
	if err := result.Scan(value); err != nil {
		panic(err)
	}
	return result
}
