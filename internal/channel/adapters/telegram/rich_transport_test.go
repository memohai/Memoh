package telegram

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type telegramRoundTripFunc func(*http.Request) (*http.Response, error)

func (f telegramRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestTelegramBot(transport http.RoundTripper) *tgbotapi.BotAPI {
	bot := &tgbotapi.BotAPI{
		Token:  "fake",
		Client: &http.Client{Transport: transport},
	}
	bot.SetAPIEndpoint("https://api.telegram.test/bot%s/%s")
	return bot
}

func telegramRichHTMLFromForm(t *testing.T, form url.Values) string {
	t.Helper()

	raw := form.Get("rich_message")
	if raw == "" {
		t.Fatalf("expected rich_message in form: %v", form)
	}
	var rich map[string]any
	if err := json.Unmarshal([]byte(raw), &rich); err != nil {
		t.Fatalf("decode rich_message: %v (raw=%q)", err, raw)
	}
	html, _ := rich["html"].(string)
	if html == "" {
		t.Fatalf("expected rich_message.html, got %#v", rich)
	}
	return html
}
