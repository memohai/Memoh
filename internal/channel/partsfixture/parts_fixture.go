// Package partsfixture exposes the canonical Message.Parts fixture used by
// the cross-platform rendering regression tests. It lives in its own
// package because the existing channeltest package (also a test-helper) is
// imported by internal-package channel tests; channeltest must not import
// channel itself or the test binary would form an import cycle.
package partsfixture

import "github.com/memohai/memoh/internal/channel"

// Canonical returns the shared rich-message fixture used by the
// cross-platform Parts rendering regression tests. The slice covers every
// MessagePartType (text, link, code_block, mention, emoji), the bold and
// italic styles, a code block with language hint, and a masked link URL,
// so adapter renderers exercise their full happy-path shape against a
// single source of truth. Each call returns a fresh copy so callers can
// mutate the slice without affecting other tests.
//
// Adversarial inputs (injection text, fence collision, etc.) intentionally
// stay in each adapter's own test file — this fixture pins the typical
// shape and the per-platform expected outputs (see CanonicalMarkdown,
// CanonicalSlackMrkdwn, CanonicalTelegramRichHTML, …).
func Canonical() []channel.MessagePart {
	return []channel.MessagePart{
		{Type: channel.MessagePartText, Text: "Hello", Styles: []channel.MessageTextStyle{channel.MessageStyleBold}},
		{Type: channel.MessagePartText, Text: "world", Styles: []channel.MessageTextStyle{channel.MessageStyleItalic}},
		{Type: channel.MessagePartLink, Text: "docs", URL: "https://example.test/page"},
		{Type: channel.MessagePartCodeBlock, Text: "go test", Language: "bash"},
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
	CanonicalMarkdown = "**Hello**\n\n*world*\n\n[docs](https://example.test/page)\n\n```bash\ngo test\n```\n\n@alice\n\n🎉"

	// CanonicalPlain is the plain-text degrader output used when neither
	// RichText nor Markdown is supported.
	CanonicalPlain = "Hello\n\nworld\n\ndocs (https://example.test/page)\n\ngo test\n\n@alice\n\n🎉"

	// CanonicalSlackMrkdwn matches Slack's distinct mrkdwn dialect:
	// single-asterisk bold, underscore italic, <url|text> links, fenced
	// code blocks without a language hint.
	CanonicalSlackMrkdwn = "*Hello*\n\n_world_\n\n<https://example.test/page|docs>\n\n```\ngo test\n```\n\n@alice\n\n🎉"

	// CanonicalTelegramRichHTML matches the HTML body Telegram's
	// sendRichMessage path emits — paragraph-wrapped inline elements,
	// <pre><code class="language-…"> for code blocks, no top-level whitespace.
	CanonicalTelegramRichHTML = `<p><b>Hello</b></p><p><i>world</i></p><p><a href="https://example.test/page">docs</a></p><pre><code class="language-bash">go test</code></pre><p>@alice</p><p>🎉</p>`
)
