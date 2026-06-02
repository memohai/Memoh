package inbound

import (
	"context"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/command"
	"github.com/memohai/memoh/internal/i18n"
)

// TestSendNewConfirmation_LocalizesActionLabels guards the
// newSession.action.{confirm,cancel} key rename. /new on a button-capable
// channel posts a Confirm/Cancel gate; the labels must render in the user's
// command_ui_language with the correct callback data carrying through.
func TestSendNewConfirmation_LocalizesActionLabels(t *testing.T) {
	p := &ChannelInboundProcessor{}
	cases := []struct {
		locale      string
		wantConfirm string
		wantCancel  string
	}{
		{"en", "✅ Confirm", "✕ Cancel"},
		{"zh", "✅ 确认", "✕ 取消"},
	}
	for _, tc := range cases {
		t.Run(tc.locale, func(t *testing.T) {
			s := &fakeReplySender{}
			err := p.sendNewConfirmation(
				context.Background(),
				channel.InboundMessage{ReplyTarget: "test-target"},
				s,
				i18n.New(tc.locale),
				"chat",
				channel.ChannelCapabilities{Buttons: true, Markdown: true, Text: true},
			)
			if err != nil {
				t.Fatalf("sendNewConfirmation: %v", err)
			}
			if len(s.sent) != 1 {
				t.Fatalf("expected 1 sent message, got %d", len(s.sent))
			}
			out := s.sent[0].Message
			if len(out.Actions) != 2 {
				t.Fatalf("expected 2 actions (confirm + cancel), got %d", len(out.Actions))
			}
			var confirm, cancel channel.Action
			for _, a := range out.Actions {
				if a.Value == command.EncodeConfirmNewCallback("chat") {
					confirm = a
				} else if a.Value == command.DismissCallback() {
					cancel = a
				}
			}
			if confirm.Label != tc.wantConfirm {
				t.Errorf("[%s] confirm label = %q, want %q", tc.locale, confirm.Label, tc.wantConfirm)
			}
			if cancel.Label != tc.wantCancel {
				t.Errorf("[%s] cancel label = %q, want %q", tc.locale, cancel.Label, tc.wantCancel)
			}
			// Body must contain the bold confirm title (markup intact on the
			// Markdown-capable channel used in this test).
			if !strings.Contains(out.Text, "Confirm") && !strings.Contains(out.Text, "确认") {
				t.Errorf("[%s] confirmation body missing confirm token, got %q", tc.locale, out.Text)
			}
		})
	}
}
