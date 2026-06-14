package feishu

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

func TestRenderFeishuMessagePartsLarkMD(t *testing.T) {
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
			name: "bold then italic on separate parts",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "bold", Styles: []channel.MessageTextStyle{channel.MessageStyleBold}},
				{Type: channel.MessagePartText, Text: "italic", Styles: []channel.MessageTextStyle{channel.MessageStyleItalic}},
			}},
			want: "**bold**\n\n*italic*",
		},
		{
			name: "strikethrough",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "old", Styles: []channel.MessageTextStyle{channel.MessageStyleStrikethrough}},
			}},
			want: "~~old~~",
		},
		{
			name: "inline code wins over other styles",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "x.y", Styles: []channel.MessageTextStyle{channel.MessageStyleCode, channel.MessageStyleBold}},
			}},
			want:     "`x.y`",
			excludes: []string{"**"},
		},
		{
			name: "link with text",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartLink, Text: "docs", URL: "https://example.test/a"},
			}},
			want: "[docs](https://example.test/a)",
		},
		{
			name: "link without text uses url",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartLink, URL: "https://example.test"},
			}},
			want: "[https://example.test](https://example.test)",
		},
		{
			name: "link with disallowed scheme falls back to plain text",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartLink, Text: "evil", URL: "javascript:alert(1)"},
			}},
			want:     "evil",
			excludes: []string{"javascript:", "["},
		},
		{
			name: "code block with language",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartCodeBlock, Text: "print(1)", Language: "python"},
			}},
			want: "```python\nprint(1)\n```",
		},
		{
			name: "code block no language",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartCodeBlock, Text: "raw"},
			}},
			want: "```\nraw\n```",
		},
		{
			name: "code block strips invalid language",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartCodeBlock, Text: "raw", Language: "<script>"},
			}},
			want: "```\nraw\n```",
		},
		{
			name: "mention emits text only",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartMention, Text: "@alice"},
			}},
			want:     "@alice",
			excludes: []string{"<at"},
		},
		{
			name: "emoji prefers Emoji field when Text empty",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartEmoji, Emoji: "🎉"},
			}},
			want: "🎉",
		},
		{
			name: "mixed inline + code block + link",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "title", Styles: []channel.MessageTextStyle{channel.MessageStyleBold}},
				{Type: channel.MessagePartCodeBlock, Text: "go test ./...", Language: "bash"},
				{Type: channel.MessagePartLink, Text: "see docs", URL: "https://example.test"},
			}},
			want: "**title**\n\n```bash\ngo test ./...\n```\n\n[see docs](https://example.test)",
		},
		{
			name: "skips empty parts",
			msg: channel.Message{Parts: []channel.MessagePart{
				{Type: channel.MessagePartText, Text: "   "},
				{Type: channel.MessagePartText, Text: "kept"},
			}},
			want: "kept",
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
			got := renderFeishuMessagePartsLarkMD(tc.msg)
			if got != tc.want {
				t.Errorf("renderFeishuMessagePartsLarkMD()\n  got:  %q\n  want: %q", got, tc.want)
			}
			for _, no := range tc.excludes {
				if strings.Contains(got, no) {
					t.Errorf("expected %q to NOT contain %q", got, no)
				}
			}
		})
	}
}

func TestBuildFeishuCardContentWrapsLarkMDFromParts(t *testing.T) {
	t.Parallel()

	msg := channel.Message{Parts: []channel.MessagePart{
		{Type: channel.MessagePartText, Text: "title", Styles: []channel.MessageTextStyle{channel.MessageStyleBold}},
		{Type: channel.MessagePartLink, Text: "docs", URL: "https://example.test"},
	}}
	rendered := renderFeishuMessagePartsLarkMD(msg)
	if rendered == "" {
		t.Fatalf("expected non-empty render")
	}
	content, err := buildFeishuCardContent(rendered)
	if err != nil {
		t.Fatalf("buildFeishuCardContent: %v", err)
	}
	for _, want := range []string{
		`"tag":"lark_md"`,
		"**title**",
		"[docs](https://example.test)",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected card content to contain %q\nfull: %s", want, content)
		}
	}
}
