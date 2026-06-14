package feishu

import (
	"strings"
	"unicode/utf8"

	"github.com/memohai/memoh/internal/channel"
)

const feishuToolCallMaxCardText = 8000

func renderFeishuToolCallCardLarkMD(p channel.ToolCallPresentation) string {
	var b strings.Builder

	title := feishuToolCallTitle(p)
	if strings.TrimSpace(title) != "" {
		appendFeishuToolCallSection(&b, "**"+escapeFeishuInlineLarkMD(title)+"**")
	}

	header := strings.TrimSpace(p.Header)
	if header == "" {
		header = strings.TrimSpace(p.InputSummary)
	}
	if header != "" {
		appendFeishuToolCallSection(&b, escapeFeishuInlineLarkMD(header))
	}

	for _, block := range p.Body {
		rendered := renderFeishuToolCallBlock(block)
		if rendered != "" {
			appendFeishuToolCallSection(&b, rendered)
		}
	}

	footer := strings.TrimSpace(p.Footer)
	if footer == "" {
		footer = strings.TrimSpace(p.ResultSummary)
	}
	if footer != "" {
		appendFeishuToolCallSection(&b, escapeFeishuInlineLarkMD(footer))
	}

	return truncateFeishuToolCallCardText(strings.TrimSpace(b.String()))
}

func renderFeishuToolCallBlock(block channel.ToolCallBlock) string {
	switch block.Type {
	case channel.ToolCallBlockLink:
		title := strings.TrimSpace(block.Title)
		url := strings.TrimSpace(block.URL)
		desc := strings.TrimSpace(block.Desc)
		if title == "" {
			title = url
		}

		value := ""
		if url != "" && isAllowedFeishuRichHref(url) {
			value = "[" + escapeFeishuLinkText(title) + "](" + escapeFeishuLinkURL(url) + ")"
		} else if title != "" {
			value = escapeFeishuInlineLarkMD(title)
		}
		if desc != "" {
			if value != "" {
				value += "\n"
			}
			value += escapeFeishuInlineLarkMD(desc)
		}
		return value
	case channel.ToolCallBlockCode:
		text := strings.Trim(block.Text, "\n\r")
		if strings.TrimSpace(text) == "" {
			return ""
		}
		title := strings.TrimSpace(block.Title)
		fence := selectFeishuBacktickFence(text, 3)
		value := fence + "\n" + text + "\n" + fence
		if title != "" {
			value = "**" + escapeFeishuInlineLarkMD(title) + "**\n" + value
		}
		return value
	default:
		text := strings.TrimSpace(block.Text)
		title := strings.TrimSpace(block.Title)
		if text == "" {
			return escapeFeishuInlineLarkMD(title)
		}
		if title != "" {
			return "**" + escapeFeishuInlineLarkMD(title) + "**\n" + escapeFeishuInlineLarkMD(text)
		}
		return escapeFeishuInlineLarkMD(text)
	}
}

func appendFeishuToolCallSection(b *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString(text)
}

func feishuToolCallTitle(p channel.ToolCallPresentation) string {
	emoji := strings.TrimSpace(p.Emoji)
	if emoji == "" {
		emoji = channel.ExternalToolCallEmoji
	}
	name := strings.TrimSpace(p.ToolName)
	if name == "" {
		name = "tool"
	}
	title := emoji + " " + name
	if p.Status != "" {
		title += " · " + string(p.Status)
	}
	return title
}

func truncateFeishuToolCallCardText(text string) string {
	if utf8.RuneCountInString(text) <= feishuToolCallMaxCardText {
		return text
	}
	runes := []rune(text)
	return string(runes[:feishuToolCallMaxCardText-3]) + "..."
}
