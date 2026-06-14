package channel

import "strings"

// RenderPartsAsMarkdown produces a GFM-flavored Markdown string from
// canonical MessageParts. It is the degradation path used by
// coerceFormatForCaps when a target channel supports Markdown but lacks
// RichText capability (e.g. DingTalk, Matrix), and is the reference output
// the per-platform renderers (Telegram HTML / Discord MD / Feishu lark_md /
// Slack mrkdwn) are tested against in the canonical-parts matrix.
//
// Attacker-controllable text and URL fields are neutralised so they cannot
// forge masked links, escape style wrappers, or open autolinks. The escape
// set mirrors what the Discord renderer applies; channels with stricter
// syntax (Slack mrkdwn) bypass this function via the RichText gate and run
// their own renderer instead.
func RenderPartsAsMarkdown(parts []MessagePart) string {
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		switch part.Type {
		case MessagePartText:
			writeDegradeMarkdownInline(&b, part.Text, part.Styles)
		case MessagePartLink:
			writeDegradeMarkdownLink(&b, part)
		case MessagePartCodeBlock:
			writeDegradeMarkdownCodeBlock(&b, part)
		case MessagePartMention:
			writeDegradeMarkdownInline(&b, part.Text, part.Styles)
		case MessagePartEmoji:
			text := strings.TrimSpace(part.Text)
			if text == "" {
				text = strings.TrimSpace(part.Emoji)
			}
			writeDegradeMarkdownInline(&b, text, part.Styles)
		}
	}
	return strings.TrimSpace(b.String())
}

// RenderPartsAsPlain joins canonical MessageParts into a plain-text block
// with style markers, masked-link syntax, and code fences stripped. Used by
// coerceFormatForCaps as the final degradation when the target channel
// supports neither RichText nor Markdown.
func RenderPartsAsPlain(parts []MessagePart) string {
	if len(parts) == 0 {
		return ""
	}
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case MessagePartText, MessagePartMention:
			t := strings.TrimSpace(part.Text)
			if t != "" {
				lines = append(lines, t)
			}
		case MessagePartLink:
			text := strings.TrimSpace(part.Text)
			url := strings.TrimSpace(part.URL)
			switch {
			case text != "" && url != "":
				lines = append(lines, text+" ("+url+")")
			case url != "":
				lines = append(lines, url)
			case text != "":
				lines = append(lines, text)
			}
		case MessagePartCodeBlock:
			t := strings.Trim(part.Text, "\n\r ")
			if t != "" {
				lines = append(lines, t)
			}
		case MessagePartEmoji:
			t := strings.TrimSpace(part.Text)
			if t == "" {
				t = strings.TrimSpace(part.Emoji)
			}
			if t != "" {
				lines = append(lines, t)
			}
		}
	}
	return strings.Join(lines, "\n\n")
}

func writeDegradeMarkdownInline(b *strings.Builder, text string, styles []MessageTextStyle) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString(renderDegradeMarkdownStyled(text, styles))
}

func writeDegradeMarkdownLink(b *strings.Builder, part MessagePart) {
	url := strings.TrimSpace(part.URL)
	text := strings.TrimSpace(part.Text)
	if url == "" || !isAllowedDegradeMarkdownHref(url) {
		if text == "" {
			return
		}
		writeDegradeMarkdownInline(b, text, part.Styles)
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
	b.WriteString(escapeDegradeMarkdownLinkText(text))
	b.WriteString("](")
	b.WriteString(escapeDegradeMarkdownLinkURL(url))
	b.WriteString(")")
}

func writeDegradeMarkdownCodeBlock(b *strings.Builder, part MessagePart) {
	text := strings.Trim(part.Text, "\n\r")
	if strings.TrimSpace(text) == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	fence := selectDegradeMarkdownBacktickFence(text, 3)
	lang := degradeMarkdownLanguage(part.Language)
	b.WriteString(fence)
	b.WriteString(lang)
	b.WriteString("\n")
	b.WriteString(text)
	b.WriteString("\n")
	b.WriteString(fence)
}

func renderDegradeMarkdownStyled(text string, styles []MessageTextStyle) string {
	if hasDegradeMarkdownStyle(styles, MessageStyleCode) {
		return wrapDegradeMarkdownInlineCode(text)
	}
	escaped := escapeDegradeMarkdownInline(text)
	if hasDegradeMarkdownStyle(styles, MessageStyleStrikethrough) {
		escaped = "~~" + escaped + "~~"
	}
	if hasDegradeMarkdownStyle(styles, MessageStyleItalic) {
		escaped = "*" + escaped + "*"
	}
	if hasDegradeMarkdownStyle(styles, MessageStyleBold) {
		escaped = "**" + escaped + "**"
	}
	return escaped
}

func hasDegradeMarkdownStyle(styles []MessageTextStyle, want MessageTextStyle) bool {
	for _, s := range styles {
		if s == want {
			return true
		}
	}
	return false
}

func degradeMarkdownLanguage(language string) string {
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

func isAllowedDegradeMarkdownHref(href string) bool {
	href = strings.TrimSpace(href)
	return strings.HasPrefix(href, "https://") ||
		strings.HasPrefix(href, "http://") ||
		strings.HasPrefix(href, "mailto:") ||
		strings.HasPrefix(href, "tel:")
}

var escapeDegradeMarkdownInline = strings.NewReplacer(
	`\`, `\\`,
	"`", "\\`",
	`*`, `\*`,
	`_`, `\_`,
	`~`, `\~`,
	`[`, `\[`,
	`]`, `\]`,
	`<`, `\<`,
	`>`, `\>`,
).Replace

func escapeDegradeMarkdownLinkText(text string) string {
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.ReplaceAll(text, "\n", " ")
	return escapeDegradeMarkdownInline(text)
}

func escapeDegradeMarkdownLinkURL(url string) string {
	url = strings.ReplaceAll(strings.TrimSpace(url), "\n", "")
	url = strings.ReplaceAll(url, "\r", "")
	url = strings.ReplaceAll(url, " ", "%20")
	url = strings.ReplaceAll(url, "<", "%3C")
	url = strings.ReplaceAll(url, ">", "%3E")
	url = strings.ReplaceAll(url, "(", "%28")
	url = strings.ReplaceAll(url, ")", "%29")
	return url
}

func selectDegradeMarkdownBacktickFence(text string, minRun int) string {
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

func wrapDegradeMarkdownInlineCode(text string) string {
	fence := selectDegradeMarkdownBacktickFence(text, 1)
	pad := ""
	if strings.ContainsRune(text, '`') {
		pad = " "
	}
	return fence + pad + text + pad + fence
}
