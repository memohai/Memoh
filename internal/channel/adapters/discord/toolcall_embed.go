package discord

import (
	"strings"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"

	"github.com/memohai/memoh/internal/channel"
)

const (
	discordToolCallMaxEmbedTitle       = 256
	discordToolCallMaxEmbedDescription = 4096
	discordToolCallMaxEmbedFieldName   = 256
	discordToolCallMaxEmbedFieldValue  = 1024
	discordToolCallMaxEmbedFooter      = 2048
	discordToolCallMaxEmbedFields      = 25
)

type discordToolCallPayload struct {
	Content string
	Embed   *discordgo.MessageEmbed
}

func renderDiscordToolCallMessage(p channel.ToolCallPresentation) discordToolCallPayload {
	content := truncateDiscordText(strings.TrimSpace(channel.RenderToolCallMessageMarkdown(p)))
	if content == "" {
		return discordToolCallPayload{}
	}

	embed := &discordgo.MessageEmbed{
		Title:       truncateDiscordEmbedText(escapeDiscordInlineMarkdown(toolCallTitle(p)), discordToolCallMaxEmbedTitle),
		Description: truncateDiscordEmbedText(escapeDiscordInlineMarkdown(toolCallDescription(p)), discordToolCallMaxEmbedDescription),
		Color:       discordToolCallStatusColor(p.Status),
	}

	for _, block := range p.Body {
		if len(embed.Fields) >= discordToolCallMaxEmbedFields {
			break
		}
		field := renderDiscordToolCallField(block)
		if field == nil {
			continue
		}
		embed.Fields = append(embed.Fields, field)
	}

	footer := strings.TrimSpace(p.Footer)
	if footer == "" {
		footer = strings.TrimSpace(p.ResultSummary)
	}
	if footer != "" {
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text: truncateDiscordEmbedText(escapeDiscordInlineMarkdown(footer), discordToolCallMaxEmbedFooter),
		}
	}

	return discordToolCallPayload{Content: content, Embed: embed}
}

func renderDiscordToolCallField(block channel.ToolCallBlock) *discordgo.MessageEmbedField {
	switch block.Type {
	case channel.ToolCallBlockLink:
		title := strings.TrimSpace(block.Title)
		url := strings.TrimSpace(block.URL)
		desc := strings.TrimSpace(block.Desc)
		if title == "" {
			title = "Link"
		}
		value := ""
		if url != "" && isAllowedDiscordRichHref(url) {
			value = "[" + escapeDiscordLinkText(title) + "](" + discordEscapeLinkURL(url) + ")"
		} else if title != "" {
			value = escapeDiscordInlineMarkdown(title)
		}
		if desc != "" {
			if value != "" {
				value += "\n"
			}
			value += escapeDiscordInlineMarkdown(desc)
		}
		if value == "" {
			return nil
		}
		return &discordgo.MessageEmbedField{
			Name:  truncateDiscordEmbedText(escapeDiscordInlineMarkdown(title), discordToolCallMaxEmbedFieldName),
			Value: truncateDiscordEmbedText(value, discordToolCallMaxEmbedFieldValue),
		}
	case channel.ToolCallBlockCode:
		text := strings.TrimSpace(block.Text)
		if text == "" {
			return nil
		}
		name := strings.TrimSpace(block.Title)
		if name == "" {
			name = "Output"
		}
		fence := selectBacktickFence(text, 3)
		value := fence + "\n" + text + "\n" + fence
		return &discordgo.MessageEmbedField{
			Name:  truncateDiscordEmbedText(escapeDiscordInlineMarkdown(name), discordToolCallMaxEmbedFieldName),
			Value: truncateDiscordEmbedText(value, discordToolCallMaxEmbedFieldValue),
		}
	default:
		text := strings.TrimSpace(block.Text)
		if text == "" {
			return nil
		}
		name := strings.TrimSpace(block.Title)
		if name == "" {
			name = "Details"
		}
		return &discordgo.MessageEmbedField{
			Name:  truncateDiscordEmbedText(escapeDiscordInlineMarkdown(name), discordToolCallMaxEmbedFieldName),
			Value: truncateDiscordEmbedText(escapeDiscordInlineMarkdown(text), discordToolCallMaxEmbedFieldValue),
		}
	}
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

func toolCallDescription(p channel.ToolCallPresentation) string {
	header := strings.TrimSpace(p.Header)
	if header == "" {
		header = strings.TrimSpace(p.InputSummary)
	}
	return header
}

func discordToolCallStatusColor(status channel.ToolCallStatus) int {
	switch status {
	case channel.ToolCallStatusApprovalRequired:
		return 0xf0b232
	case channel.ToolCallStatusCompleted:
		return 0x3ba55d
	case channel.ToolCallStatusFailed:
		return 0xed4245
	default:
		return 0x5865f2
	}
}

func discordEscapeLinkURL(url string) string {
	url = strings.ReplaceAll(strings.TrimSpace(url), "\n", "")
	url = strings.ReplaceAll(url, "\r", "")
	url = strings.ReplaceAll(url, ")", "%29")
	return url
}

func truncateDiscordEmbedText(text string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit-3]) + "..."
}
