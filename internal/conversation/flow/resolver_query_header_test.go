package flow

import (
	"testing"

	"github.com/memohai/memoh/internal/conversation"
)

// TestUserQueryNeedsHeader pins the attachment-only regression fix: a message
// with no caption but with attachments must still get the user header (it is
// what carries sender/channel/attachment paths to the model and what makes
// the user turn persist with its asset links). A no-prompt skill activation
// stays headerless so its stored user content remains empty.
func TestUserQueryNeedsHeader(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		req         conversation.ChatRequest
		attachments int
		want        bool
	}{
		{name: "text message", req: conversation.ChatRequest{Query: "hello"}, want: true},
		{name: "text with attachments", req: conversation.ChatRequest{Query: "look"}, attachments: 1, want: true},
		{name: "attachment only", req: conversation.ChatRequest{}, attachments: 2, want: true},
		{name: "empty message", req: conversation.ChatRequest{}, want: false},
		{name: "whitespace query no attachments", req: conversation.ChatRequest{Query: "  "}, want: false},
		{
			name:        "no-prompt skill activation with attachments",
			req:         conversation.ChatRequest{UserMessageKind: conversation.UserMessageKindSkillActivation},
			attachments: 1,
			want:        false,
		},
		{
			name: "skill activation with prompt",
			req:  conversation.ChatRequest{Query: "do it", UserMessageKind: conversation.UserMessageKindSkillActivation},
			want: true,
		},
	}
	for _, tc := range cases {
		if got := userQueryNeedsHeader(tc.req, tc.attachments); got != tc.want {
			t.Errorf("%s: userQueryNeedsHeader = %v, want %v", tc.name, got, tc.want)
		}
	}
}
