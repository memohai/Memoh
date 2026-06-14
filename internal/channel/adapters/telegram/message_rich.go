package telegram

import (
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

func renderTelegramMessagePartsRichMessage(msg channel.Message) telegramInputRichMessage {
	if len(msg.Parts) == 0 {
		return telegramInputRichMessage{}
	}
	var b strings.Builder
	for _, part := range msg.Parts {
		switch part.Type {
		case channel.MessagePartText:
			writeTelegramRichInlinePart(&b, part.Text, part.Styles)
		case channel.MessagePartLink:
			writeTelegramRichLinkPart(&b, part)
		case channel.MessagePartCodeBlock:
			writeTelegramRichCodeBlockPart(&b, part)
		case channel.MessagePartMention:
			writeTelegramRichInlinePart(&b, part.Text, part.Styles)
		case channel.MessagePartEmoji:
			text := strings.TrimSpace(part.Text)
			if text == "" {
				text = strings.TrimSpace(part.Emoji)
			}
			writeTelegramRichInlinePart(&b, text, part.Styles)
		}
	}
	html := strings.TrimSpace(b.String())
	if html == "" {
		return telegramInputRichMessage{}
	}
	return telegramInputRichMessage{HTML: html, SkipEntityDetection: true}
}

func writeTelegramRichInlinePart(b *strings.Builder, text string, styles []channel.MessageTextStyle) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	writeTelegramRichParagraph(b, renderTelegramStyledInline(text, styles))
}

func writeTelegramRichLinkPart(b *strings.Builder, part channel.MessagePart) {
	url := strings.TrimSpace(part.URL)
	text := strings.TrimSpace(part.Text)
	if text == "" {
		text = url
	}
	if text == "" {
		return
	}
	if url == "" || !isAllowedTelegramRichHref(url) {
		writeTelegramRichParagraph(b, renderTelegramStyledInline(text, part.Styles))
		return
	}
	link := `<a href="` + telegramEscapeAttr(url) + `">` + telegramEscapeHTML(text) + `</a>`
	writeTelegramRichParagraph(b, link)
}

func writeTelegramRichCodeBlockPart(b *strings.Builder, part channel.MessagePart) {
	text := strings.TrimSpace(part.Text)
	if text == "" {
		return
	}
	lang := telegramRichLanguage(part.Language)
	b.WriteString("<pre>")
	if lang != "" {
		b.WriteString(`<code class="language-`)
		b.WriteString(telegramEscapeAttr(lang))
		b.WriteString(`">`)
		b.WriteString(telegramEscapeHTML(text))
		b.WriteString("</code>")
	} else {
		b.WriteString(telegramEscapeHTML(text))
	}
	b.WriteString("</pre>")
}

func renderTelegramStyledInline(text string, styles []channel.MessageTextStyle) string {
	html := telegramEscapeHTML(text)
	if hasTelegramTextStyle(styles, channel.MessageStyleCode) {
		return "<code>" + html + "</code>"
	}
	if hasTelegramTextStyle(styles, channel.MessageStyleStrikethrough) {
		html = "<s>" + html + "</s>"
	}
	if hasTelegramTextStyle(styles, channel.MessageStyleItalic) {
		html = "<i>" + html + "</i>"
	}
	if hasTelegramTextStyle(styles, channel.MessageStyleBold) {
		html = "<b>" + html + "</b>"
	}
	return html
}

func hasTelegramTextStyle(styles []channel.MessageTextStyle, want channel.MessageTextStyle) bool {
	for _, style := range styles {
		if style == want {
			return true
		}
	}
	return false
}

func telegramRichLanguage(language string) string {
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
