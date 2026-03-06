package qq

import (
	"context"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

func TestEventToInboundMessageC2C(t *testing.T) {
	t.Parallel()

	msg, ok := eventToInboundMessage(InboundEvent{
		Type: "C2C_MESSAGE_CREATE",
		C2CMessage: &C2CMessageEvent{
			ID:        "msg-1",
			Content:   "hello",
			Timestamp: "2026-03-06T12:00:00Z",
			Author: C2CAuthor{
				UserOpenID: "user-openid",
			},
			Attachments: []MessageAttachment{{
				ContentType: "image/png",
				URL:         "//cdn.qq.com/image.png",
				FileName:    "a.png",
				Width:       120,
				Height:      80,
				Size:        2048,
			}},
		},
	}, "bot-1")
	if !ok {
		t.Fatal("expected inbound message")
	}
	if msg.Channel != Type {
		t.Fatalf("unexpected channel: %s", msg.Channel)
	}
	if msg.BotID != "bot-1" {
		t.Fatalf("unexpected bot id: %s", msg.BotID)
	}
	if msg.ReplyTarget != "c2c:user-openid" {
		t.Fatalf("unexpected reply target: %s", msg.ReplyTarget)
	}
	if msg.Conversation.Type != "direct" {
		t.Fatalf("unexpected conversation type: %s", msg.Conversation.Type)
	}
	if msg.Sender.SubjectID != "user-openid" {
		t.Fatalf("unexpected sender subject: %s", msg.Sender.SubjectID)
	}
	if len(msg.Message.Attachments) != 1 {
		t.Fatalf("unexpected attachments: %d", len(msg.Message.Attachments))
	}
	att := msg.Message.Attachments[0]
	if att.Type != channel.AttachmentImage {
		t.Fatalf("unexpected attachment type: %s", att.Type)
	}
	if att.URL != "https://cdn.qq.com/image.png" {
		t.Fatalf("unexpected attachment url: %s", att.URL)
	}
	if mentioned, _ := msg.Metadata["is_mentioned"].(bool); mentioned {
		t.Fatal("direct message should not be marked mentioned")
	}
}

func TestEventToInboundMessageGroupAt(t *testing.T) {
	t.Parallel()

	msg, ok := eventToInboundMessage(InboundEvent{
		Type: "GROUP_AT_MESSAGE_CREATE",
		GroupMessage: &GroupMessageEvent{
			ID:          "msg-2",
			Content:     "@bot hi",
			Timestamp:   "2026-03-06T12:00:00Z",
			GroupOpenID: "group-openid",
			Author: GroupAuthor{
				MemberOpenID: "member-openid",
			},
		},
	}, "bot-2")
	if !ok {
		t.Fatal("expected inbound message")
	}
	if msg.ReplyTarget != "group:group-openid" {
		t.Fatalf("unexpected reply target: %s", msg.ReplyTarget)
	}
	if msg.Conversation.ID != "group-openid" {
		t.Fatalf("unexpected conversation id: %s", msg.Conversation.ID)
	}
	if msg.Conversation.Type != "group" {
		t.Fatalf("unexpected conversation type: %s", msg.Conversation.Type)
	}
	if msg.Sender.SubjectID != "member-openid" {
		t.Fatalf("unexpected sender subject: %s", msg.Sender.SubjectID)
	}
	if mentioned, _ := msg.Metadata["is_mentioned"].(bool); !mentioned {
		t.Fatal("group at message should be marked mentioned")
	}
}

func TestEventToInboundMessageChannelAt(t *testing.T) {
	t.Parallel()

	msg, ok := eventToInboundMessage(InboundEvent{
		Type: "AT_MESSAGE_CREATE",
		GuildMessage: &GuildMessageEvent{
			ID:        "msg-3",
			Content:   "<@bot> hi",
			Timestamp: "2026-03-06T12:00:00Z",
			ChannelID: "channel-1",
			GuildID:   "guild-1",
			Author: GuildAuthor{
				ID:       "author-1",
				Username: "alice",
			},
		},
	}, "bot-3")
	if !ok {
		t.Fatal("expected inbound message")
	}
	if msg.ReplyTarget != "channel:channel-1" {
		t.Fatalf("unexpected reply target: %s", msg.ReplyTarget)
	}
	if msg.Conversation.Type != "channel" {
		t.Fatalf("unexpected conversation type: %s", msg.Conversation.Type)
	}
	if msg.Sender.DisplayName != "alice" {
		t.Fatalf("unexpected sender display name: %s", msg.Sender.DisplayName)
	}
	if msg.Sender.Attribute("channel_id") != "channel-1" {
		t.Fatalf("unexpected channel_id attribute: %s", msg.Sender.Attribute("channel_id"))
	}
	if msg.Metadata["guild_id"] != "guild-1" {
		t.Fatalf("unexpected guild_id metadata: %#v", msg.Metadata["guild_id"])
	}
	if mentioned, _ := msg.Metadata["is_mentioned"].(bool); !mentioned {
		t.Fatal("channel at message should be marked mentioned")
	}
}

func TestEventToInboundMessageIgnoresUnsupportedType(t *testing.T) {
	t.Parallel()

	if _, ok := eventToInboundMessage(InboundEvent{Type: "READY"}, "bot-1"); ok {
		t.Fatal("unexpected inbound message for READY")
	}
}

func TestEventToInboundMessagePreservesGIFType(t *testing.T) {
	t.Parallel()

	msg, ok := eventToInboundMessage(InboundEvent{
		Type: "C2C_MESSAGE_CREATE",
		C2CMessage: &C2CMessageEvent{
			ID:        "msg-gif",
			Content:   "gif",
			Timestamp: "2026-03-06T12:00:00Z",
			Author: C2CAuthor{
				UserOpenID: "user-openid",
			},
			Attachments: []MessageAttachment{{
				ContentType: "image/gif",
				URL:         "https://cdn.qq.com/animated.gif",
				FileName:    "animated.gif",
			}},
		},
	}, "bot-gif")
	if !ok {
		t.Fatal("expected inbound message")
	}
	if len(msg.Message.Attachments) != 1 {
		t.Fatalf("unexpected attachments: %d", len(msg.Message.Attachments))
	}
	if msg.Message.Attachments[0].Type != channel.AttachmentGIF {
		t.Fatalf("unexpected attachment type: %s", msg.Message.Attachments[0].Type)
	}
}

func TestAdjustSessionAfterInvalidKeepsIntentLevel(t *testing.T) {
	t.Parallel()

	adapter := NewQQAdapter(nil)
	session := sessionState{
		SessionID:   "session-1",
		LastSeq:     42,
		IntentLevel: 0,
	}

	adapter.adjustSessionAfterInvalid("cfg-1", &session)

	if session.SessionID != "" {
		t.Fatalf("unexpected session id: %q", session.SessionID)
	}
	if session.LastSeq != 0 {
		t.Fatalf("unexpected seq: %d", session.LastSeq)
	}
	if session.IntentLevel != 0 {
		t.Fatalf("unexpected intent level: %d", session.IntentLevel)
	}

	saved := adapter.loadSession("cfg-1")
	if saved.IntentLevel != 0 {
		t.Fatalf("unexpected saved intent level: %d", saved.IntentLevel)
	}
}

func TestStartHeartbeatCancelStopsSessionLoop(t *testing.T) {
	t.Parallel()

	adapter := NewQQAdapter(nil)
	heartbeat := adapter.startHeartbeat(context.Background(), &gatewayWriter{}, time.Hour, &sessionState{})
	heartbeat.cancel()

	select {
	case <-heartbeat.done:
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not stop after session cancel")
	}
}
