package pipeline

import (
	"encoding/json"
	"strings"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/messageconv"
)

// DecodeTurnResponseEntry converts a persisted bot message into a TR entry for
// pipeline context composition.
//
// Tool calls and tool results are preserved as native structured content
// parts. They must not be rendered as assistant-visible pseudo-protocol text:
// models may imitate that text instead of emitting real provider tool calls.
func DecodeTurnResponseEntry(msg messagepkg.Message) (TurnResponseEntry, bool) {
	role := strings.TrimSpace(msg.Role)
	if role != "assistant" && role != "tool" {
		return TurnResponseEntry{}, false
	}

	var modelMsg conversation.ModelMessage
	if err := json.Unmarshal(msg.Content, &modelMsg); err != nil {
		return TurnResponseEntry{}, false
	}

	rawContent := messageconv.CanonicalModelMessageContent(modelMsg)

	if len(rawContent) == 0 || strings.TrimSpace(string(rawContent)) == "" {
		return TurnResponseEntry{}, false
	}

	return TurnResponseEntry{
		RequestedAtMs:   msg.CreatedAt.UnixMilli(),
		Role:            role,
		Content:         debugContent(rawContent),
		RawContent:      rawContent,
		SourceMessageID: strings.TrimSpace(msg.ID),
	}, true
}

// turnResponsePart is a permissive view of a persisted content part. It
// purposefully uses json.RawMessage for tool input/output to avoid losing
// structure while keeping the type declaration local to this package.
type turnResponsePart struct {
	Type             string          `json:"type"`
	Text             string          `json:"text,omitempty"`
	ToolCallID       string          `json:"toolCallId,omitempty"`
	ToolName         string          `json:"toolName,omitempty"`
	Input            json.RawMessage `json:"input,omitempty"`
	Output           json.RawMessage `json:"output,omitempty"`
	Result           json.RawMessage `json:"result,omitempty"`
	ProviderMetadata json.RawMessage `json:"providerMetadata,omitempty"`
}

func debugContent(raw json.RawMessage) string {
	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		return plain
	}
	var parts []turnResponsePart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return string(raw)
	}
	var texts []string
	for _, p := range parts {
		switch strings.ToLower(strings.TrimSpace(p.Type)) {
		case "text":
			if text := strings.TrimSpace(p.Text); text != "" {
				texts = append(texts, text)
			}
		case "tool-call":
			texts = append(texts, "[tool call: "+strings.TrimSpace(p.ToolName)+"]")
		case "tool-result":
			texts = append(texts, "[tool result: "+strings.TrimSpace(p.ToolName)+"]")
		}
	}
	return strings.Join(texts, "\n")
}
