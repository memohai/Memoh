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
	b.WriteString(discordEscapeLinkText(text))
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
	lang := discordRichLanguage(part.Language)
	b.WriteString("```")
	b.WriteString(lang)
	b.WriteString("\n")
	b.WriteString(text)
	b.WriteString("\n```")
}

func renderDiscordRichStyledInline(text string, styles []channel.MessageTextStyle) string {
	if hasDiscordRichTextStyle(styles, channel.MessageStyleCode) {
		return "`" + text + "`"
	}
	if hasDiscordRichTextStyle(styles, channel.MessageStyleStrikethrough) {
		text = "~~" + text + "~~"
	}
	if hasDiscordRichTextStyle(styles, channel.MessageStyleItalic) {
		text = "*" + text + "*"
	}
	if hasDiscordRichTextStyle(styles, channel.MessageStyleBold) {
		text = "**" + text + "**"
	}
	return text
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
