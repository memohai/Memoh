package telegram

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestResolveTelegramSender(t *testing.T) {
	t.Parallel()

	externalID, displayName, attrs := resolveTelegramSender(nil)
	if externalID != "" || displayName != "" || len(attrs) != 0 {
		t.Fatalf("expected empty sender")
	}
	msg := &tgbotapi.Message{
		From: &tgbotapi.User{ID: 123, UserName: "alice"},
	}
	externalID, displayName, attrs = resolveTelegramSender(msg)
	if externalID != "123" || displayName != "alice" {
		t.Fatalf("unexpected sender: %s %s", externalID, displayName)
	}
	if attrs["user_id"] != "123" || attrs["username"] != "alice" {
		t.Fatalf("unexpected attrs: %#v", attrs)
	}
}
