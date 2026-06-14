package feishu

import (
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

func renderFeishuMessagePartsLarkMD(msg channel.Message) string {
	if len(msg.Parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range msg.Parts {
		switch part.Type {
		case channel.MessagePartText:
			writeFeishuRichInlinePart(&b, part.Text, part.Styles)
		case channel.MessagePartLink:
			writeFeishuRichLinkPart(&b, part)
		case channel.MessagePartCodeBlock:
			writeFeishuRichCodeBlockPart(&b, part)
		case channel.MessagePartMention:
			writeFeishuRichInlinePart(&b, part.Text, part.Styles)
		case channel.MessagePartEmoji:
			text := strings.TrimSpace(part.Text)
			if text == "" {
				text = strings.TrimSpace(part.Emoji)
			}
			writeFeishuRichInlinePart(&b, text, part.Styles)
		}
	}
	return strings.TrimSpace(b.String())
}

func writeFeishuRichInlinePart(b *strings.Builder, text string, styles []channel.MessageTextStyle) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString(renderFeishuRichStyledInline(text, styles))
}

func writeFeishuRichLinkPart(b *strings.Builder, part channel.MessagePart) {
	url := strings.TrimSpace(part.URL)
	text := strings.TrimSpace(part.Text)
	if text == "" {
		text = url
	}
	if text == "" {
		return
	}
	if url == "" || !isAllowedFeishuRichHref(url) {
		writeFeishuRichInlinePart(b, text, part.Styles)
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("[")
	b.WriteString(text)
	b.WriteString("](")
	b.WriteString(url)
	b.WriteString(")")
}

func writeFeishuRichCodeBlockPart(b *strings.Builder, part channel.MessagePart) {
	text := strings.Trim(part.Text, "\n\r")
	if strings.TrimSpace(text) == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	lang := feishuRichLanguage(part.Language)
	b.WriteString("```")
	b.WriteString(lang)
	b.WriteString("\n")
	b.WriteString(text)
	b.WriteString("\n```")
}

func renderFeishuRichStyledInline(text string, styles []channel.MessageTextStyle) string {
	if hasFeishuRichTextStyle(styles, channel.MessageStyleCode) {
		return "`" + text + "`"
	}
	if hasFeishuRichTextStyle(styles, channel.MessageStyleStrikethrough) {
		text = "~~" + text + "~~"
	}
	if hasFeishuRichTextStyle(styles, channel.MessageStyleItalic) {
		text = "*" + text + "*"
	}
	if hasFeishuRichTextStyle(styles, channel.MessageStyleBold) {
		text = "**" + text + "**"
	}
	return text
}

func hasFeishuRichTextStyle(styles []channel.MessageTextStyle, want channel.MessageTextStyle) bool {
	for _, s := range styles {
		if s == want {
			return true
		}
	}
	return false
}

func feishuRichLanguage(language string) string {
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

func isAllowedFeishuRichHref(href string) bool {
	href = strings.TrimSpace(href)
	return strings.HasPrefix(href, "https://") ||
		strings.HasPrefix(href, "http://") ||
		strings.HasPrefix(href, "mailto:") ||
		strings.HasPrefix(href, "tel:")
}
