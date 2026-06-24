package flow

import (
	"testing"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
)

func TestDedupePersistedCurrentUserMessageRemovesCurrentInboundFromHistory(t *testing.T) {
	t.Parallel()

	history := []historyfrag.HistoryRecord{
		historyRecord("row-1", conversation.ModelMessage{
			Role:    "user",
			Content: conversation.NewTextContent("---\nmessage-id: qq-msg-1\nchannel: qq\n---\nhello"),
		}, func(record *historyfrag.HistoryRecord) {
			record.ExternalMessageID = "qq-msg-1"
			record.Platform = "qq"
			record.SenderChannelIdentityID = "channel-identity-1"
		}),
		historyRecord("row-2", conversation.ModelMessage{
			Role:    "assistant",
			Content: conversation.NewTextContent("ok"),
		}, nil),
	}

	got := dedupePersistedCurrentUserMessage(history, conversation.ChatRequest{
		UserMessagePersisted:    true,
		RouteID:                 "route-1",
		ExternalMessageID:       "qq-msg-1",
		CurrentChannel:          "qq",
		SourceChannelIdentityID: "channel-identity-1",
	})

	if len(got) != 1 {
		t.Fatalf("expected 1 message after dedupe, got %d", len(got))
	}
	if got[0].ModelMessage.Role != "assistant" {
		t.Fatalf("unexpected remaining role: %s", got[0].ModelMessage.Role)
	}
}
