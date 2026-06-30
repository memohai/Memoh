package inbound

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/command"
	"github.com/memohai/memoh/internal/i18n"
)

// TestTelegramPathSkipsTrailer is the symmetric check: button-capable channels
// must NEVER see the auto-derived trailer in their message body. Telegram
// users see the buttons; appending typing hints alongside would be redundant.
func TestTelegramPathSkipsTrailer(t *testing.T) {
	withButtons := channel.ChannelCapabilities{Text: true, Buttons: true, Markdown: true}

	res := &command.Result{
		Text: "🧠 Reasoning\nCurrent: xhigh",
		Interactive: &command.Interactive{Kind: command.InteractiveChoices, Choices: &command.ChoicesView{
			Title: "Choose a level:",
			Choices: []command.ListItem{
				{Label: "off", Action: &command.ItemAction{Resource: "reasoning", Action: "set", Args: []string{"off"}}},
				{Label: "high", Action: &command.ItemAction{Resource: "reasoning", Action: "set", Args: []string{"high"}}},
			},
		}},
	}
	msg := renderResult(res, RenderContext{Caps: withButtons, T: i18n.New("en")})

	if !strings.Contains(msg.Text, "Choose a level:") {
		t.Errorf("Telegram path should render Choices.Title, got %q", msg.Text)
	}
	if strings.Contains(msg.Text, "Pick with") {
		t.Errorf("Telegram message body leaked the no-button trailer: %q", msg.Text)
	}
	if len(msg.Actions) == 0 {
		t.Errorf("Telegram path should have button actions, got 0")
	}
}
