package flow

import (
	"strings"

	"github.com/memohai/memoh/internal/conversation"
)

// ExtractAssistantOutputs collects assistant-role outputs from a slice of ModelMessages.
func ExtractAssistantOutputs(messages []conversation.ModelMessage) []conversation.AssistantOutput {
	if len(messages) == 0 {
		return nil
	}
	outputs := make([]conversation.AssistantOutput, 0, len(messages))
	for _, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}
		if hasToolCallContent(msg) {
			continue
		}
		rawParts := msg.ContentParts()
		parts := filterVisibleContentParts(rawParts)
		content := visibleContentText(parts)
		if len(rawParts) == 0 {
			content = strings.TrimSpace(msg.TextContent())
		}
		if content == "" && len(parts) == 0 {
			continue
		}
		outputs = append(outputs, conversation.AssistantOutput{Content: content, Parts: parts})
	}
	return outputs
}

func hasToolCallContent(msg conversation.ModelMessage) bool {
	if len(msg.ToolCalls) > 0 {
		return true
	}
	for _, p := range msg.ContentParts() {
		if p.Type == "tool-call" {
			return true
		}
	}
	return false
}

func filterVisibleContentParts(parts []conversation.ContentPart) []conversation.ContentPart {
	if len(parts) == 0 {
		return nil
	}
	filtered := make([]conversation.ContentPart, 0, len(parts))
	for _, p := range parts {
		if isVisibleContentPart(p) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func isVisibleContentPart(part conversation.ContentPart) bool {
	if !part.HasValue() {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(part.Type)) {
	case "reasoning", "tool-call", "tool-result":
		return false
	default:
		return true
	}
}

func visibleContentText(parts []conversation.ContentPart) string {
	if len(parts) == 0 {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		text := strings.TrimSpace(visibleContentPartText(part))
		if text == "" {
			continue
		}
		texts = append(texts, text)
	}
	return strings.TrimSpace(strings.Join(texts, "\n"))
}

func visibleContentPartText(part conversation.ContentPart) string {
	if strings.TrimSpace(part.Text) != "" {
		return part.Text
	}
	if strings.TrimSpace(part.URL) != "" {
		return part.URL
	}
	if strings.TrimSpace(part.Emoji) != "" {
		return part.Emoji
	}
	return ""
}
