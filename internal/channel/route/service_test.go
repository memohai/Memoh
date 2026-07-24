package route

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestToRouteFieldsUsesBotAndThreadTerminology(t *testing.T) {
	botID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	activeThreadID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	got := toRouteFields(
		pgtype.UUID{Bytes: [16]byte{3}, Valid: true},
		botID,
		"telegram",
		pgtype.UUID{},
		"external-conversation",
		pgtype.Text{String: "external-thread", Valid: true},
		pgtype.Text{String: "group", Valid: true},
		pgtype.Text{String: "target", Valid: true},
		activeThreadID,
		nil,
		pgtype.Timestamptz{},
		pgtype.Timestamptz{},
	)

	if got.BotID != botID.String() {
		t.Fatalf("BotID = %q, want %q", got.BotID, botID.String())
	}
	if got.ExternalConversationID != "external-conversation" {
		t.Fatalf("ExternalConversationID = %q", got.ExternalConversationID)
	}
	if got.ExternalThreadID != "external-thread" {
		t.Fatalf("ExternalThreadID = %q", got.ExternalThreadID)
	}
	if got.ActiveThreadID != activeThreadID.String() {
		t.Fatalf("ActiveThreadID = %q, want %q", got.ActiveThreadID, activeThreadID.String())
	}
}
