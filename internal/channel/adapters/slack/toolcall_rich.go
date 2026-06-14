package slack

import (
	"strings"
	"unicode/utf8"

	slackapi "github.com/slack-go/slack"

	"github.com/memohai/memoh/internal/channel"
)

const (
	slackToolCallMaxBlocks    = 50
	slackToolCallMaxBlockText = 3000
)

type slackToolCallPayload struct {
	Text   string
	Blocks []slackapi.Block
}

func renderSlackToolCallMessage(p channel.ToolCallPresentation) slackToolCallPayload {
	text := truncateSlackText(strings.TrimSpace(channel.RenderToolCallMessageMarkdown(p)))
	if text == "" {
		return slackToolCallPayload{}
	}

	blocks := make([]slackapi.Block, 0, 2+len(p.Body))
	title := slackEscapeMrkdwn(toolCallTitle(p))
	header := strings.TrimSpace(p.Header)
	if header == "" {
		header = strings.TrimSpace(p.InputSummary)
	}
	first := "*" + title + "*"
	if header != "" {
		first += "\n" + slackEscapeMrkdwn(header)
	}
	blocks = appendSlackSectionBlock(blocks, first)

	for _, block := range p.Body {
		if len(blocks) >= slackToolCallMaxBlocks-1 {
			break
		}
		rendered := renderSlackToolCallBlock(block)
		blocks = appendSlackSectionBlock(blocks, rendered)
	}

	footer := strings.TrimSpace(p.Footer)
	if footer == "" {
		footer = strings.TrimSpace(p.ResultSummary)
	}
	if footer != "" && len(blocks) < slackToolCallMaxBlocks {
		if text := truncateSlackBlockText(slackEscapeMrkdwn(footer)); text != "" {
			blocks = append(blocks, slackapi.NewContextBlock(
				"",
				slackapi.NewTextBlockObject(slackapi.MarkdownType, text, false, false),
			))
		}
	}

	return slackToolCallPayload{Text: text, Blocks: blocks}
}

func renderSlackToolCallBlock(block channel.ToolCallBlock) string {
	switch block.Type {
	case channel.ToolCallBlockLink:
		title := strings.TrimSpace(block.Title)
		url := strings.TrimSpace(block.URL)
		desc := strings.TrimSpace(block.Desc)
		if title == "" {
			title = url
		}
		if url == "" || !isAllowedSlackRichHref(url) {
			return slackEscapeMrkdwn(strings.TrimSpace(strings.Join([]string{title, desc}, "\n")))
		}
		value := "<" + slackEscapeLinkURL(url) + "|" + slackEscapeMrkdwn(title) + ">"
		if desc != "" {
			value += "\n" + slackEscapeMrkdwn(desc)
		}
		return value
	case channel.ToolCallBlockCode:
		text := strings.TrimSpace(block.Text)
		if text == "" {
			return ""
		}
		title := strings.TrimSpace(block.Title)
		value := "```\n" + slackEscapeMrkdwn(text) + "\n```"
		if title != "" {
			value = "*" + slackEscapeMrkdwn(title) + "*\n" + value
		}
		return value
	default:
		return slackEscapeMrkdwn(strings.TrimSpace(block.Text))
	}
}

func appendSlackSectionBlock(blocks []slackapi.Block, text string) []slackapi.Block {
	text = truncateSlackBlockText(strings.TrimSpace(text))
	if text == "" {
		return blocks
	}
	return append(blocks, slackapi.NewSectionBlock(
		slackapi.NewTextBlockObject(slackapi.MarkdownType, text, false, false),
		nil,
		nil,
	))
}

func toolCallTitle(p channel.ToolCallPresentation) string {
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

func slackEscapeMrkdwn(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}

func slackEscapeLinkURL(url string) string {
	url = strings.ReplaceAll(strings.TrimSpace(url), "\n", "")
	url = strings.ReplaceAll(url, "\r", "")
	url = strings.ReplaceAll(url, "|", "%7C")
	return slackEscapeMrkdwn(url)
}

func truncateSlackBlockText(text string) string {
	if utf8.RuneCountInString(text) <= slackToolCallMaxBlockText {
		return text
	}
	runes := []rune(text)
	return string(runes[:slackToolCallMaxBlockText-3]) + "..."
}
