package personalwechat

import (
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

func TestBuildInboundMessageGroupQuoteImage(t *testing.T) {
	t.Parallel()

	inbound, ok := buildInboundMessage(bridgeMessage{
		ID:        "msg-1",
		Type:      "Text",
		Text:      "@bot look",
		Timestamp: "2026-06-16T12:00:00Z",
		Sender: bridgeIdentity{
			ID:     "wxid_sender",
			Name:   "Alice",
			Alias:  "alice-alias",
			Remark: "Alice Remark",
		},
		Conversation: bridgeConversation{
			ID:   "room-1@chatroom",
			Type: "group",
			Name: "Test Room",
		},
		Reply: &bridgeReply{
			MessageID: "quoted-1",
			Sender:    "Bob",
			Preview:   "quoted text",
		},
		Attachments: []bridgeAttachment{
			{Type: "image", Path: "/tmp/image.jpg", Mime: "image/jpeg", Name: "image.jpg", Size: 123, Variant: "original"},
		},
	})
	if !ok {
		t.Fatal("expected inbound message")
	}
	if inbound.Channel != Type {
		t.Fatalf("channel = %q", inbound.Channel)
	}
	if inbound.Sender.SubjectID != "wxid_sender" {
		t.Fatalf("sender = %#v", inbound.Sender)
	}
	if inbound.Sender.DisplayName != "Alice Remark" {
		t.Fatalf("display = %q", inbound.Sender.DisplayName)
	}
	if inbound.Conversation.Type != channel.ConversationTypeGroup || inbound.Conversation.ID != "room-1@chatroom" {
		t.Fatalf("conversation = %#v", inbound.Conversation)
	}
	if inbound.ReplyTarget != "room:room-1@chatroom" {
		t.Fatalf("reply target = %q", inbound.ReplyTarget)
	}
	if inbound.Message.Reply == nil || inbound.Message.Reply.MessageID != "quoted-1" || inbound.Message.Reply.Preview != "quoted text" {
		t.Fatalf("reply = %#v", inbound.Message.Reply)
	}
	if len(inbound.Message.Attachments) != 1 {
		t.Fatalf("attachments = %d", len(inbound.Message.Attachments))
	}
	att := inbound.Message.Attachments[0]
	if att.Type != channel.AttachmentImage || att.Path != "/tmp/image.jpg" || att.Mime != "image/jpeg" {
		t.Fatalf("attachment = %#v", att)
	}
	if att.Metadata["variant"] != "original" {
		t.Fatalf("attachment metadata = %#v", att.Metadata)
	}
}

func TestBuildInboundMessageRejectsEmptyContent(t *testing.T) {
	t.Parallel()

	_, ok := buildInboundMessage(bridgeMessage{
		ID:           "msg-empty",
		Sender:       bridgeIdentity{ID: "wxid_sender"},
		Conversation: bridgeConversation{ID: "wxid_sender", Type: "private"},
	})
	if ok {
		t.Fatal("expected empty message to be ignored")
	}
}
