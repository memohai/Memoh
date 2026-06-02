package inbound

import (
	"context"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/command"
	"github.com/memohai/memoh/internal/i18n"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

// TestResolveNewSessionType_BareConfirmFlag guards the hand-typed "/new
// --confirm" edge: extractFlags doesn't recognize --confirm, so it lands as the
// first positional (the mode slot). It must NOT be read as a session type —
// resolveNewSessionType should fall through to context defaults exactly like a
// bare "/new", not error with `unknown session type "--confirm"`.
func TestResolveNewSessionType_BareConfirmFlag(t *testing.T) {
	msg := channel.InboundMessage{Channel: channel.ChannelTypeTelegram}

	bare, errBare := resolveNewSessionType("/new", msg)
	if errBare != nil {
		t.Fatalf("/new returned error: %v", errBare)
	}
	withFlag, err := resolveNewSessionType("/new --confirm", msg)
	if err != nil {
		t.Fatalf("/new --confirm should not error, got: %v", err)
	}
	if withFlag != bare {
		t.Errorf("/new --confirm resolved to %q, want same as bare /new (%q)", withFlag, bare)
	}
	// Explicit modes must still resolve normally.
	if got, err := resolveNewSessionType("/new chat", msg); err != nil || got != sessionpkg.TypeChat {
		t.Errorf("/new chat = (%q, %v), want (%q, nil)", got, err, sessionpkg.TypeChat)
	}
	if got, err := resolveNewSessionType("/new discuss", msg); err != nil || got != sessionpkg.TypeDiscuss {
		t.Errorf("/new discuss = (%q, %v), want (%q, nil)", got, err, sessionpkg.TypeDiscuss)
	}
	// A genuinely unknown mode still errors.
	if _, err := resolveNewSessionType("/new bogus", msg); err == nil {
		t.Errorf("/new bogus should error on unknown session type")
	}
}

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
