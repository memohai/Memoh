// Package partsfixture exposes the canonical Message.Parts fixture used by
// the cross-platform rendering regression tests. It lives in its own
// package because the existing channeltest package (also a test-helper) is
// imported by internal-package channel tests; channeltest must not import
// channel itself or the test binary would form an import cycle.
package partsfixture

import "github.com/memohai/memoh/internal/channel"

// Canonical returns the shared rich-message fixture used by the
// cross-platform Parts rendering regression tests. The slice covers the
// baseline inline/block mix (text, link, code_block, mention, emoji), bold and
// italic styles, safe and unsafe links, language-bearing and language-sanitized
// code blocks, multiline code, and a mention without platform identity.
// Each call returns a fresh copy so callers can mutate the slice without
// affecting other tests.
func Canonical() []channel.MessagePart {
	return []channel.MessagePart{
		{Type: channel.MessagePartText, Text: "Hello", Styles: []channel.MessageTextStyle{channel.MessageStyleBold}},
		{Type: channel.MessagePartText, Text: "world", Styles: []channel.MessageTextStyle{channel.MessageStyleItalic}},
		{Type: channel.MessagePartLink, Text: "docs", URL: "https://example.test/page"},
		{Type: channel.MessagePartLink, Text: "empty url"},
		{Type: channel.MessagePartLink, Text: "evil", URL: "javascript:alert(1)"},
		{Type: channel.MessagePartCodeBlock, Text: "go test", Language: "bash"},
		{Type: channel.MessagePartCodeBlock, Text: "line1\nline2", Language: "python"},
		{Type: channel.MessagePartCodeBlock, Text: "raw", Language: "<script>"},
		{Type: channel.MessagePartMention, Text: "@alice"},
		{Type: channel.MessagePartEmoji, Emoji: "🎉"},
	}
}

// Expected outputs for Canonical() across each rendering target. Update
// these constants when changing the fixture, and the per-adapter regression
// tests stay aligned automatically.
const (
	// CanonicalMarkdown is the GFM-flavored degrader output, also matching
	// the Discord and Feishu adapters (both speak GFM-aligned dialects).
	CanonicalMarkdown = "**Hello**\n\n*world*\n\n[docs](https://example.test/page)\n\nempty url\n\nevil\n\n```bash\ngo test\n```\n\n```python\nline1\nline2\n```\n\n```\nraw\n```\n\n@alice\n\n🎉"

	// CanonicalPlain is the plain-text degrader output used when neither
	// RichText nor Markdown is supported.
	CanonicalPlain = "Hello\n\nworld\n\ndocs (https://example.test/page)\n\nempty url\n\nevil (javascript:alert(1))\n\ngo test\n\nline1\nline2\n\nraw\n\n@alice\n\n🎉"

	// CanonicalSlackMrkdwn matches Slack's distinct mrkdwn dialect:
	// single-asterisk bold, underscore italic, <url|text> links, fenced
	// code blocks without a language hint.
	CanonicalSlackMrkdwn = "*Hello*\n\n_world_\n\n<https://example.test/page|docs>\n\nempty url\n\nevil\n\n```\ngo test\n```\n\n```\nline1\nline2\n```\n\n```\nraw\n```\n\n@alice\n\n🎉"

	// CanonicalTelegramRichHTML matches the HTML body Telegram's
	// sendRichMessage path emits — paragraph-wrapped inline elements,
	// <pre><code class="language-…"> for code blocks, no top-level whitespace.
	CanonicalTelegramRichHTML = "<p><b>Hello</b></p><p><i>world</i></p><p><a href=\"https://example.test/page\">docs</a></p><p>empty url</p><p>evil</p><pre><code class=\"language-bash\">go test</code></pre><pre><code class=\"language-python\">line1\nline2</code></pre><pre>raw</pre><p>@alice</p><p>🎉</p>"

	// CanonicalMatrixHTML matches Matrix's org.matrix.custom.html output.
	CanonicalMatrixHTML = "<p><strong>Hello</strong></p><p><em>world</em></p><p><a href=\"https://example.test/page\">docs</a></p><p>empty url</p><p>evil</p><pre><code class=\"language-bash\">go test</code></pre><pre><code class=\"language-python\">line1\nline2</code></pre><pre><code>raw</code></pre><p>@alice</p><p>🎉</p>"
)
