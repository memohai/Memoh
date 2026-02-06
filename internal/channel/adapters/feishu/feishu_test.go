package feishu

import (
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestResolveFeishuReceiveID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw       string
		wantID    string
		wantType  string
		shouldErr bool
	}{
		{raw: "open_id:ou_123", wantID: "ou_123", wantType: "open_id"},
		{raw: "user_id:uu_123", wantID: "uu_123", wantType: "user_id"},
		{raw: "chat_id:oc_123", wantID: "oc_123", wantType: "chat_id"},
		{raw: "ou_999", wantID: "ou_999", wantType: "open_id"},
		{raw: "", shouldErr: true},
	}
	for _, tc := range cases {
		id, idType, err := resolveFeishuReceiveID(tc.raw)
		if tc.shouldErr {
			if err == nil {
				t.Fatalf("expected error for %q", tc.raw)
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", tc.raw, err)
		}
		if id != tc.wantID || idType != tc.wantType {
			t.Fatalf("unexpected result for %q: %s %s", tc.raw, id, idType)
		}
	}
}

func TestExtractFeishuInboundP2P(t *testing.T) {
	t.Parallel()

	text := `{"text":"hi"}`
	msgType := larkim.MsgTypeText
	chatType := "p2p"
	chatID := "oc_1"
	userID := "u_1"
	openID := "ou_1"
	event := &larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Message: &larkim.EventMessage{
				MessageType: &msgType,
				Content:     &text,
				ChatType:    &chatType,
				ChatId:      &chatID,
			},
			Sender: &larkim.EventSender{
				SenderId: &larkim.UserId{
					UserId: &userID,
					OpenId: &openID,
				},
			},
		},
	}
	got := extractFeishuInbound(event)
	if got.Message.PlainText() != "hi" {
		t.Fatalf("unexpected text: %s", got.Message.PlainText())
	}
	if got.ReplyTarget != "ou_1" {
		t.Fatalf("unexpected reply target: %s", got.ReplyTarget)
	}
}

func TestExtractFeishuInboundGroup(t *testing.T) {
	t.Parallel()

	text := `{"text":"hi"}`
	msgType := larkim.MsgTypeText
	chatType := "group"
	chatID := "oc_2"
	userID := "u_2"
	openID := "ou_2"
	event := &larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Message: &larkim.EventMessage{
				MessageType: &msgType,
				Content:     &text,
				ChatType:    &chatType,
				ChatId:      &chatID,
			},
			Sender: &larkim.EventSender{
				SenderId: &larkim.UserId{
					UserId: &userID,
					OpenId: &openID,
				},
			},
		},
	}
	got := extractFeishuInbound(event)
	if got.ReplyTarget != "chat_id:oc_2" {
		t.Fatalf("unexpected reply target: %s", got.ReplyTarget)
	}
}

func TestExtractFeishuInboundNonText(t *testing.T) {
	t.Parallel()

	msgType := "image"
	event := &larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Message: &larkim.EventMessage{
				MessageType: &msgType,
			},
		},
	}
	got := extractFeishuInbound(event)
	if got.Message.PlainText() != "" {
		t.Fatalf("expected empty text, got %s", got.Message.PlainText())
	}
}
