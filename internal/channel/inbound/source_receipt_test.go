package inbound

import (
	"testing"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/messagesource"
)

func TestBuildInboundUserReceiptCapturesCanonicalOrigin(t *testing.T) {
	t.Parallel()

	receipt, err := buildInboundUserReceipt(
		InboundIdentity{
			ChannelIdentityID: "11111111-1111-1111-1111-111111111111",
			UserID:            "22222222-2222-2222-2222-222222222222",
			DisplayName:       " Alice ",
		},
		channel.InboundMessage{
			Channel: channel.ChannelTypeTelegram,
			Message: channel.Message{
				ID: "external-1",
				Reply: &channel.ReplyRef{
					MessageID: "reply-1",
					Sender:    "Bob",
					Preview:   "earlier",
				},
				Forward: &channel.ForwardRef{MessageID: "forward-1", Sender: "Carol"},
			},
			Conversation: channel.Conversation{Type: "direct", Name: "Alice Chat"},
		},
		"  hello  ",
		[]conversation.ChatAttachment{{ContentHash: "asset-1", Mime: "image/png"}},
		" route-1 ",
		"33333333-3333-3333-3333-333333333333",
	)
	if err != nil {
		t.Fatalf("buildInboundUserReceipt() error = %v", err)
	}
	if receipt.ID == "" || receipt.DisplayText != "hello" || len(receipt.Attachments) != 1 {
		t.Fatalf("receipt payload = %+v", receipt)
	}
	origin := receipt.Origin.Values()
	if origin.SenderChannelIdentityID != "11111111-1111-1111-1111-111111111111" ||
		origin.SenderUserID != "22222222-2222-2222-2222-222222222222" ||
		origin.ExternalMessageID != "external-1" || origin.SourceReplyToMessageID != "reply-1" ||
		origin.EventID != "33333333-3333-3333-3333-333333333333" ||
		origin.Context != messagesource.NewV1("Alice", "telegram", "private", "Alice Chat") {
		t.Fatalf("origin = %+v", origin)
	}
	if receipt.Metadata["route_id"] != "route-1" || receipt.Metadata["platform"] != "telegram" {
		t.Fatalf("route metadata = %#v", receipt.Metadata)
	}
	reply := receipt.Metadata["reply"].(map[string]any)
	forward := receipt.Metadata["forward"].(map[string]any)
	if reply["message_id"] != "reply-1" || reply["sender"] != "Bob" || reply["preview"] != "earlier" ||
		forward["message_id"] != "forward-1" || forward["sender"] != "Carol" {
		t.Fatalf("interaction metadata = %#v", receipt.Metadata)
	}
}

func TestBuildInboundUserReceiptUsesZeroContextForIncompleteOrigin(t *testing.T) {
	t.Parallel()

	receipt, err := buildInboundUserReceipt(
		InboundIdentity{ChannelIdentityID: "11111111-1111-1111-1111-111111111111", DisplayName: "Alice"},
		channel.InboundMessage{Channel: channel.ChannelTypeTelegram, Conversation: channel.Conversation{Type: "group"}},
		"hello", nil, "route-1", "",
	)
	if err != nil {
		t.Fatalf("buildInboundUserReceipt() error = %v", err)
	}
	if receipt.Origin.Values().Context != (messagesource.Context{}) {
		t.Fatalf("incomplete origin context = %+v", receipt.Origin.Values().Context)
	}
}

func TestBuildInboundUserReceiptRejectsInvalidInternalUUID(t *testing.T) {
	t.Parallel()

	if _, err := buildInboundUserReceipt(
		InboundIdentity{ChannelIdentityID: "not-a-uuid", DisplayName: "Alice"},
		channel.InboundMessage{Channel: channel.ChannelTypeTelegram, Conversation: channel.Conversation{Type: "private", Name: "Chat"}},
		"hello", nil, "route-1", "",
	); err == nil {
		t.Fatal("invalid channel identity UUID was accepted")
	}
}
