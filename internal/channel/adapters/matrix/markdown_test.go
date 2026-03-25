package matrix

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

func TestNormalizeMatrixMarkdownTaskList(t *testing.T) {
	input := "- [ ] todo\n- [x] done"
	got := normalizeMatrixMarkdown(input)
	if got != "- ☐ todo\n- ☑ done" {
		t.Fatalf("unexpected normalized markdown: %q", got)
	}
}

func TestNormalizeMatrixMarkdownTablesBecomeCodeBlocks(t *testing.T) {
	input := "| A | B |\n| --- | --- |\n| 1 | 2 |"
	got := normalizeMatrixMarkdown(input)
	if !strings.HasPrefix(got, "```text\n") || !strings.Contains(got, "| 1 | 2 |") || !strings.HasSuffix(got, "\n```") {
		t.Fatalf("unexpected normalized markdown: %q", got)
	}
}

func TestFormatMatrixMessageMarkdownUsesNormalizedBody(t *testing.T) {
	formatted := formatMatrixMessage(channel.Message{
		Text:   "- [x] done\n\n| A |\n| --- |\n| 1 |",
		Format: channel.MessageFormatMarkdown,
	})
	if !strings.Contains(formatted.Body, "☑ done") {
		t.Fatalf("expected task list checkbox in body, got %q", formatted.Body)
	}
	if !strings.Contains(formatted.FormattedBody, "<pre><code") || !strings.Contains(formatted.FormattedBody, "| A |") {
		t.Fatalf("expected table fallback code block in formatted body, got %q", formatted.FormattedBody)
	}
	if strings.Contains(formatted.FormattedBody, "<table") {
		t.Fatalf("expected no html table in formatted body, got %q", formatted.FormattedBody)
	}
}
