package chat

import "strings"

type AssistantOutput struct {
	Content string
	Parts   []ContentPart
}

func ExtractAssistantOutputs(messages []GatewayMessage) []AssistantOutput {
	if len(messages) == 0 {
		return nil
	}
	outputs := make([]AssistantOutput, 0, len(messages))
	for _, msg := range messages {
		normalized := normalizeGatewayMessage(msg)
		for _, item := range normalized {
			if item.Role != "assistant" {
				continue
			}
			content := strings.TrimSpace(item.Content)
			parts := make([]ContentPart, 0, len(item.Parts))
			for _, part := range item.Parts {
				if !hasContentPartValue(part) {
					continue
				}
				parts = append(parts, part)
			}
			if content == "" && len(parts) == 0 {
				continue
			}
			outputs = append(outputs, AssistantOutput{
				Content: content,
				Parts:   parts,
			})
		}
	}
	return outputs
}

func hasContentPartValue(part ContentPart) bool {
	if strings.TrimSpace(part.Text) != "" {
		return true
	}
	if strings.TrimSpace(part.URL) != "" {
		return true
	}
	if strings.TrimSpace(part.Emoji) != "" {
		return true
	}
	return false
}
