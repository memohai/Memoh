package slack

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

func TestRenderSlackMessagePartsMrkdwn(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		msg      channel.Message
		want     string
		excludes []string
	}{
		{
			name: "plain text",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "hello"},
			}},
			want: "hello",
		},
		{
			name: "bold uses single asterisk",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "bold", Styles: []channel.MessageTextStyle{channel.MessageStyleBold}},
			}},
			want: "*bold*",
		},
		{
			name: "italic uses underscore",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "italic", Styles: []channel.MessageTextStyle{channel.MessageStyleItalic}},
			}},
			want: "_italic_",
		},
		{
			name: "strikethrough uses single tilde",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "old", Styles: []channel.MessageTextStyle{channel.MessageStyleStrikethrough}},
			}},
			want: "~old~",
		},
		{
			name: "inline code wins over other styles",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "x.y", Styles: []channel.MessageTextStyle{channel.MessageStyleCode, channel.MessageStyleBold}},
			}},
			want:     "`x.y`",
			excludes: []string{"*"},
		},
		{
			name: "link uses pipe syntax",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartLink, Text: "docs", URL: "https://example.test/a"},
			}},
			want: "<https://example.test/a|docs>",
		},
		{
			name: "link without text is bare url",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartLink, URL: "https://example.test"},
			}},
			want: "<https://example.test>",
		},
		{
			name: "link with disallowed scheme falls back to text",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartLink, Text: "evil", URL: "javascript:alert(1)"},
			}},
			want:     "evil",
			excludes: []string{"javascript:", "<"},
		},
		{
			name: "code block has no language hint",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartCodeBlock, Text: "print(1)", Language: "python"},
			}},
			want: "```\nprint(1)\n```",
		},
		{
			name: "code block no language",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartCodeBlock, Text: "raw"},
			}},
			want: "```\nraw\n```",
		},
		{
			name: "mention emits text only",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartMention, Text: "@alice"},
			}},
			want:     "@alice",
			excludes: []string{"<@"},
		},
		{
			name: "emoji prefers Emoji field",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartEmoji, Emoji: "🎉"},
			}},
			want: "🎉",
		},
		{
			name: "special chars escaped in inline text",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "x < y & z > 0"},
			}},
			want: "x &lt; y &amp; z &gt; 0",
		},
		{
			name: "link text special chars escaped",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartLink, Text: "<docs>", URL: "https://example.test"},
			}},
			want: "<https://example.test|&lt;docs&gt;>",
		},
		{
			name: "mixed inline + code block + link",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "title", Styles: []channel.MessageTextStyle{channel.MessageStyleBold}},
				{Type: channel.MessagePartCodeBlock, Text: "go test ./..."},
				{Type: channel.MessagePartLink, Text: "see docs", URL: "https://example.test"},
			}},
			want: "*title*\n\n```\ngo test ./...\n```\n\n<https://example.test|see docs>",
		},
		{
			name: "empty parts returns empty",
			msg:  channel.Message{Parts: nil},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := renderSlackMessagePartsMrkdwn(tc.msg)
			if got != tc.want {
				t.Errorf("renderSlackMessagePartsMrkdwn()\n  got:  %q\n  want: %q", got, tc.want)
			}
			for _, no := range tc.excludes {
				if strings.Contains(got, no) {
					t.Errorf("expected %q to NOT contain %q", got, no)
				}
			}
		})
	}
}
