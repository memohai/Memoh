package turn

import (
	"strings"
)

// ExtractAssistantOutputs collects assistant-role outputs from a slice of ModelMessages.
func ExtractAssistantOutputs(messages []ModelMessage) []AssistantOutput {
	if len(messages) == 0 {
		return nil
	}
	outputs := make([]AssistantOutput, 0, len(messages))
	for _, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}
		if HasToolCallContent(msg) {
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
		outputs = append(outputs, AssistantOutput{Content: content, Parts: parts})
	}
	return outputs
}

func HasToolCallContent(msg ModelMessage) bool {
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

func filterVisibleContentParts(parts []ContentPart) []ContentPart {
	if len(parts) == 0 {
		return nil
	}
	filtered := make([]ContentPart, 0, len(parts))
	for _, p := range parts {
		if isVisibleContentPart(p) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func isVisibleContentPart(part ContentPart) bool {
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

func visibleContentText(parts []ContentPart) string {
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

func visibleContentPartText(part ContentPart) string {
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
