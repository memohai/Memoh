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
		content := strings.TrimSpace(msg.TextContent())
		parts := filterContentParts(msg.ContentParts())
		if content == "" && len(parts) == 0 {
			continue
		}
		outputs = append(outputs, conversation.AssistantOutput{Content: content, Parts: parts})
	}
	return outputs
}

func filterContentParts(parts []conversation.ContentPart) []conversation.ContentPart {
	if len(parts) == 0 {
		return nil
	}
	filtered := make([]conversation.ContentPart, 0, len(parts))
	for _, p := range parts {
		if p.HasValue() {
			filtered = append(filtered, p)
		}
	}
	return filtered
}
