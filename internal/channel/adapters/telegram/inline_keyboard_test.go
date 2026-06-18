package telegram

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/memohai/memoh/internal/channel"
)

func TestTelegramInlineKeyboard_CallbackButton(t *testing.T) {
	t.Parallel()
	mk := telegramInlineKeyboard([]channel.Action{{Label: "Approve", Value: "approve:42"}})
	btn := firstButton(t, mk)
	if btn.Text != "Approve" || btn.CallbackData == nil || *btn.CallbackData != "approve:42" {
		t.Fatalf("expected callback button, got %+v", btn)
	}
	if btn.URL != nil {
		t.Fatalf("expected no URL on callback button, got %q", *btn.URL)
	}
}

func TestTelegramInlineKeyboard_URLButton(t *testing.T) {
	t.Parallel()
	mk := telegramInlineKeyboard([]channel.Action{{Label: "Read more", URL: "https://example.com/article"}})
	btn := firstButton(t, mk)
	if btn.Text != "Read more" || btn.URL == nil || *btn.URL != "https://example.com/article" {
		t.Fatalf("expected URL button pointing at example.com, got %+v", btn)
	}
	if btn.CallbackData != nil {
		t.Fatalf("expected no callback data on URL button, got %q", *btn.CallbackData)
	}
}

func TestTelegramInlineKeyboard_ValuePrecedesURL(t *testing.T) {
	t.Parallel()

	mk := telegramInlineKeyboard([]channel.Action{{Label: "Open", URL: "https://example.com/x", Value: "fallback"}})
	btn := firstButton(t, mk)
	if btn.CallbackData == nil || *btn.CallbackData != "fallback" {
		t.Fatalf("expected callback value to win, got %+v", btn)
	}
	if btn.URL != nil {
		t.Fatalf("expected URL to be dropped when callback value is present, got %q", *btn.URL)
	}
}

func TestTelegramInlineKeyboard_RejectsUnsafeURL(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"javascript:alert(1)", "data:text/html,xx", "mailto:a@example.com", "tel:+123", "#anchor", "tg://resolve?domain=x"} {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			mk := telegramInlineKeyboard([]channel.Action{{Label: "x", URL: raw}})
			if len(mk.InlineKeyboard) != 0 {
				t.Fatalf("expected empty keyboard for %q, got %+v", raw, mk.InlineKeyboard)
			}
		})
	}
}

func TestTelegramInlineKeyboard_DropsActionWithoutTarget(t *testing.T) {
	t.Parallel()
	mk := telegramInlineKeyboard([]channel.Action{{Label: "Empty"}})
	if len(mk.InlineKeyboard) != 0 {
		t.Fatalf("expected empty keyboard when no URL or Value, got %+v", mk.InlineKeyboard)
	}
}

func firstButton(t *testing.T, mk tgbotapi.InlineKeyboardMarkup) tgbotapi.InlineKeyboardButton {
	t.Helper()
	if len(mk.InlineKeyboard) == 0 || len(mk.InlineKeyboard[0]) == 0 {
		t.Fatalf("expected at least one row+button, got %+v", mk.InlineKeyboard)
	}
	return mk.InlineKeyboard[0][0]
}
