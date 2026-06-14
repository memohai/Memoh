package slack

import (
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

// Security model: Slack mrkdwn has no `\` escape, so we rely on entity
// encoding of <, >, and & — the only characters that can form a Slack tag
// (<url|text>, <@user>, <!channel>, etc.). This blocks all injection of
// clickable links, user/channel mentions, and broadcast pings. The remaining
// mrkdwn markers (*, _, ~, `) cannot be entity-escaped and may cause visual
// glitches when attacker-supplied text overlaps with style wrappers, but
// they cannot forge links or mentions, so they are a display concern only.

func renderSlackMessagePartsMrkdwn(msg channel.Message) string {
	if len(msg.Parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range msg.Parts {
		switch part.Type {
		case channel.MessagePartText:
			writeSlackRichInlinePart(&b, part.Text, part.Styles)
		case channel.MessagePartLink:
			writeSlackRichLinkPart(&b, part)
		case channel.MessagePartCodeBlock:
			writeSlackRichCodeBlockPart(&b, part)
		case channel.MessagePartMention:
			writeSlackRichInlinePart(&b, part.Text, part.Styles)
		case channel.MessagePartEmoji:
			text := strings.TrimSpace(part.Text)
			if text == "" {
				text = strings.TrimSpace(part.Emoji)
			}
			writeSlackRichInlinePart(&b, text, part.Styles)
		}
	}
	return strings.TrimSpace(b.String())
}

func writeSlackRichInlinePart(b *strings.Builder, text string, styles []channel.MessageTextStyle) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString(renderSlackRichStyledInline(slackEscapeMrkdwn(text), styles))
}

func writeSlackRichLinkPart(b *strings.Builder, part channel.MessagePart) {
	url := strings.TrimSpace(part.URL)
	text := strings.TrimSpace(part.Text)
	if url == "" || !isAllowedSlackRichHref(url) {
		if text == "" {
			return
		}
		writeSlackRichInlinePart(b, text, part.Styles)
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("<")
	b.WriteString(slackEscapeMrkdwnURL(url))
	if text != "" {
		b.WriteString("|")
		b.WriteString(slackEscapeMrkdwn(text))
	}
	b.WriteString(">")
}

func writeSlackRichCodeBlockPart(b *strings.Builder, part channel.MessagePart) {
	text := strings.Trim(part.Text, "\n\r")
	if strings.TrimSpace(text) == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("```\n")
	b.WriteString(text)
	b.WriteString("\n```")
}

func renderSlackRichStyledInline(escaped string, styles []channel.MessageTextStyle) string {
	if hasSlackRichTextStyle(styles, channel.MessageStyleCode) {
		return "`" + escaped + "`"
	}
	if hasSlackRichTextStyle(styles, channel.MessageStyleStrikethrough) {
		escaped = "~" + escaped + "~"
	}
	if hasSlackRichTextStyle(styles, channel.MessageStyleItalic) {
		escaped = "_" + escaped + "_"
	}
	if hasSlackRichTextStyle(styles, channel.MessageStyleBold) {
		escaped = "*" + escaped + "*"
	}
	return escaped
}

func hasSlackRichTextStyle(styles []channel.MessageTextStyle, want channel.MessageTextStyle) bool {
	for _, s := range styles {
		if s == want {
			return true
		}
	}
	return false
}

// slackEscapeMrkdwnURL strips characters that would prematurely terminate a
// <url|text> link sequence. Slack accepts raw URLs without entity escaping
// inside <...>, but a literal <, |, or > would corrupt the marker — a stray
// < would let attacker-controlled URL content open a nested Slack mrkdwn tag.
func slackEscapeMrkdwnURL(url string) string {
	url = strings.ReplaceAll(strings.TrimSpace(url), "\n", "")
	url = strings.ReplaceAll(url, "\r", "")
	url = strings.ReplaceAll(url, "<", "%3C")
	url = strings.ReplaceAll(url, "|", "%7C")
	url = strings.ReplaceAll(url, ">", "%3E")
	return url
}

func isAllowedSlackRichHref(href string) bool {
	href = strings.TrimSpace(href)
	return strings.HasPrefix(href, "https://") ||
		strings.HasPrefix(href, "http://") ||
		strings.HasPrefix(href, "mailto:") ||
		strings.HasPrefix(href, "tel:")
}
