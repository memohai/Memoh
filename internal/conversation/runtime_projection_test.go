package conversation

import (
	"encoding/json"
	"testing"
	"time"

	messagepkg "github.com/memohai/memoh/internal/message"
)

func TestRuntimePersistedProjectionPreservesRowIdentityAndOrder(t *testing.T) {
	timestamp := time.Date(2026, 7, 16, 5, 0, 0, 0, time.UTC)
	projection := NewRuntimePersistedProjection([]messagepkg.Message{
		{
			ID: "user-row", Role: "user", DisplayContent: "hello", CreatedAt: timestamp,
			TurnID: "turn-7", TurnPosition: 7, TurnMessageSeq: 1,
		},
		{
			ID: "assistant-row", Role: "assistant", CreatedAt: timestamp.Add(time.Second),
			Content: json.RawMessage(`{"role":"assistant","content":"answer"}`),
			TurnID:  "turn-7", TurnPosition: 7, TurnMessageSeq: 2,
		},
	})

	if projection.RequestUserTurn == nil {
		t.Fatal("request user turn is nil")
	}
	if projection.RequestUserTurn.ID != "user-row" || projection.RequestUserTurn.TurnPosition != 7 || projection.RequestUserTurn.TurnMessageSeq != 1 {
		t.Fatalf("request identity = %#v", projection.RequestUserTurn)
	}
	if len(projection.AssistantMessages) != 1 {
		t.Fatalf("assistant messages = %#v", projection.AssistantMessages)
	}
	block := projection.AssistantMessages[0]
	if block.StableID != "assistant-row" || block.TurnPosition != 7 || block.TurnMessageSeq != 2 {
		t.Fatalf("assistant identity = %#v", block)
	}
	if len(projection.RowLedger) != 2 || projection.RowLedger[0].StableID != "user-row" || projection.RowLedger[1].StableID != "assistant-row" {
		t.Fatalf("row ledger = %#v", projection.RowLedger)
	}

	decoded, ok := RuntimePersistedProjectionFromMetadata(map[string]any{
		RuntimePersistedProjectionMetadataKey: projection,
	})
	if !ok || len(decoded.AssistantMessages) != 1 || decoded.AssistantMessages[0].StableID != "assistant-row" || len(decoded.RowLedger) != 2 {
		t.Fatalf("metadata roundtrip = %#v, %v", decoded, ok)
	}
}
