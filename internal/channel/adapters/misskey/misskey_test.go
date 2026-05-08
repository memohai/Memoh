package misskey

import "testing"

func TestBuildInboundMessageReplyKeepsTextClean(t *testing.T) {
	t.Parallel()

	adapter := &MisskeyAdapter{}
	inbound, ok := adapter.buildInboundMessage(&meResponse{ID: "bot-1", Username: "bot"}, misskeyNote{
		ID:      "note-1",
		Text:    "@bot reply body",
		UserID:  "user-1",
		User:    misskeyUser{Username: "sender", Name: "Sender"},
		ReplyID: "source-note",
		Reply: &misskeyNote{
			ID:   "source-note",
			Text: "quoted text",
			User: misskeyUser{Username: "original", Name: "Original"},
		},
	})
	if !ok {
		t.Fatal("expected inbound message")
	}
	if inbound.Message.Text != "reply body" {
		t.Fatalf("unexpected text: %q", inbound.Message.Text)
	}
	if inbound.Message.Reply == nil {
		t.Fatal("expected reply ref")
	}
	if inbound.Message.Reply.MessageID != "source-note" ||
		inbound.Message.Reply.Sender != "Original" ||
		inbound.Message.Reply.Preview != "quoted text" {
		t.Fatalf("unexpected reply ref: %#v", inbound.Message.Reply)
	}
}

func TestBuildInboundMessageRenoteMapsForward(t *testing.T) {
	t.Parallel()

	adapter := &MisskeyAdapter{}
	inbound, ok := adapter.buildInboundMessage(&meResponse{ID: "bot-1", Username: "bot"}, misskeyNote{
		ID:       "note-1",
		Text:     "@bot check this",
		UserID:   "user-1",
		User:     misskeyUser{Username: "sender", Name: "Sender"},
		RenoteID: "renote-1",
		Renote: &misskeyNote{
			ID:        "renote-1",
			UserID:    "source-user",
			User:      misskeyUser{Username: "source", Name: "Source"},
			CreatedAt: "2026-05-09T00:00:00Z",
		},
	})
	if !ok {
		t.Fatal("expected inbound message")
	}
	if inbound.Message.Text != "check this" {
		t.Fatalf("unexpected text: %q", inbound.Message.Text)
	}
	if inbound.Message.Forward == nil {
		t.Fatal("expected forward ref")
	}
	if inbound.Message.Forward.MessageID != "renote-1" ||
		inbound.Message.Forward.FromUserID != "source-user" ||
		inbound.Message.Forward.Sender != "Source" ||
		inbound.Message.Forward.Date == 0 {
		t.Fatalf("unexpected forward ref: %#v", inbound.Message.Forward)
	}
}
