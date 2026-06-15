package discord

import (
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

func renderDiscordMessagePartsMarkdown(msg channel.Message) string {
	if len(msg.Parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range msg.Parts {
		switch part.Type {
		case channel.MessagePartText:
			writeDiscordRichInlinePart(&b, part.Text, part.Styles)
		case channel.MessagePartLink:
			writeDiscordRichLinkPart(&b, part)
		case channel.MessagePartCodeBlock:
			writeDiscordRichCodeBlockPart(&b, part)
		case channel.MessagePartMention:
			writeDiscordRichInlinePart(&b, part.Text, part.Styles)
		case channel.MessagePartEmoji:
			text := strings.TrimSpace(part.Text)
			if text == "" {
				text = strings.TrimSpace(part.Emoji)
			}
			writeDiscordRichInlinePart(&b, text, part.Styles)
		}
	}
	return strings.TrimSpace(b.String())
}

func writeDiscordRichInlinePart(b *strings.Builder, text string, styles []channel.MessageTextStyle) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString(renderDiscordRichStyledInline(text, styles))
}

func writeDiscordRichLinkPart(b *strings.Builder, part channel.MessagePart) {
	url := strings.TrimSpace(part.URL)
	text := strings.TrimSpace(part.Text)
	if url == "" || !isAllowedDiscordRichHref(url) {
		if text == "" {
			return
		}
		writeDiscordRichInlinePart(b, text, part.Styles)
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	if text == "" {
		b.WriteString(url)
		return
	}
	b.WriteString("[")
	b.WriteString(escapeDiscordLinkText(text))
	b.WriteString("](")
	b.WriteString(discordEscapeLinkURL(url))
	b.WriteString(")")
}

func writeDiscordRichCodeBlockPart(b *strings.Builder, part channel.MessagePart) {
	text := strings.Trim(part.Text, "\n\r")
	if strings.TrimSpace(text) == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	fence := selectBacktickFence(text, 3)
	lang := discordRichLanguage(part.Language)
	b.WriteString(fence)
	b.WriteString(lang)
	b.WriteString("\n")
	b.WriteString(text)
	b.WriteString("\n")
	b.WriteString(fence)
}

func renderDiscordRichStyledInline(text string, styles []channel.MessageTextStyle) string {
	if hasDiscordRichTextStyle(styles, channel.MessageStyleCode) {
		return wrapDiscordInlineCode(text)
	}
	escaped := escapeDiscordInlineMarkdown(text)
	if hasDiscordRichTextStyle(styles, channel.MessageStyleStrikethrough) {
		escaped = "~~" + escaped + "~~"
	}
	if hasDiscordRichTextStyle(styles, channel.MessageStyleItalic) {
		escaped = "*" + escaped + "*"
	}
	if hasDiscordRichTextStyle(styles, channel.MessageStyleBold) {
		escaped = "**" + escaped + "**"
	}
	return escaped
}

func hasDiscordRichTextStyle(styles []channel.MessageTextStyle, want channel.MessageTextStyle) bool {
	for _, s := range styles {
		if s == want {
			return true
		}
	}
	return false
}

func discordRichLanguage(language string) string {
	language = strings.TrimSpace(language)
	if language == "" || len(language) > 32 {
		return ""
	}
	for _, r := range language {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '+' {
			continue
		}
		return ""
	}
	return language
}

func isAllowedDiscordRichHref(href string) bool {
	href = strings.TrimSpace(href)
	return strings.HasPrefix(href, "https://") ||
		strings.HasPrefix(href, "http://") ||
		strings.HasPrefix(href, "mailto:") ||
		strings.HasPrefix(href, "tel:")
}

// escapeDiscordInlineMarkdown neutralises the markdown control characters that
// could let attacker-supplied text forge links, mentions, code spans, or break
// out of an enclosing style wrapper. `\` must come first so subsequent escapes
// are themselves preserved.
var escapeDiscordInlineMarkdown = strings.NewReplacer(
	`\`, `\\`,
	"`", "\\`",
	`*`, `\*`,
	`_`, `\_`,
	`~`, `\~`,
	`[`, `\[`,
	`]`, `\]`,
	`<`, `\<`,
	`>`, `\>`,
	`|`, `\|`,
).Replace

// escapeDiscordLinkText escapes the characters that can prematurely close or
// split a `[text](url)` label, and collapses control whitespace that would
// otherwise stop Discord from parsing the link.
func escapeDiscordLinkText(text string) string {
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, `\`, `\\`)
	text = strings.ReplaceAll(text, "[", `\[`)
	text = strings.ReplaceAll(text, "]", `\]`)
	return text
}

// selectBacktickFence returns a run of backticks long enough to safely fence
// `text`, with at least `minRun` backticks. Used by code blocks (minRun=3) and
// the inline-code wrapper.
func selectBacktickFence(text string, minRun int) string {
	maxRun, cur := 0, 0
	for _, r := range text {
		if r == '`' {
			cur++
			if cur > maxRun {
				maxRun = cur
			}
			continue
		}
		cur = 0
	}
	n := minRun
	if maxRun >= n {
		n = maxRun + 1
	}
	return strings.Repeat("`", n)
}

// wrapDiscordInlineCode wraps text as inline code, growing the backtick fence
// when text already contains backticks and padding with a space when the text
// starts or ends with a backtick.
func wrapDiscordInlineCode(text string) string {
	fence := selectBacktickFence(text, 1)
	pad := ""
	if strings.ContainsRune(text, '`') {
		pad = " "
	}
	return fence + pad + text + pad + fence
}
