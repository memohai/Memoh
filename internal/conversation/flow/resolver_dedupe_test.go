package flow

import (
	"testing"

	"github.com/memohai/memoh/internal/conversation"
)

func TestDedupePersistedCurrentUserMessageRemovesCurrentInboundFromHistory(t *testing.T) {
	t.Parallel()

	history := []messageWithUsage{
		{
			Message: conversation.ModelMessage{
				Role:    "user",
				Content: conversation.NewTextContent("---\nmessage-id: qq-msg-1\nchannel: qq\n---\nhello"),
			},
			ExternalMessageID: "qq-msg-1",
			Platform:          "qq",
			SenderChannelID:   "channel-identity-1",
		},
		{
			Message: conversation.ModelMessage{
				Role:    "assistant",
				Content: conversation.NewTextContent("ok"),
			},
		},
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
	if got[0].Message.Role != "assistant" {
		t.Fatalf("unexpected remaining role: %s", got[0].Message.Role)
	}
}
