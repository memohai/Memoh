package flow

import (
	"encoding/json"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
)

// sdkMessagesToModelMessages converts SDK messages to the persistence/API format
// at the resolver boundary. This is the only place where this conversion should happen.
func sdkMessagesToModelMessages(msgs []sdk.Message) []conversation.ModelMessage {
	result := make([]conversation.ModelMessage, 0, len(msgs))
	for _, msg := range msgs {
		data, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		var envelope struct {
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			continue
		}
		var usage json.RawMessage
		if msg.Usage != nil {
			usage, _ = json.Marshal(msg.Usage)
		}
		result = append(result, conversation.ModelMessage{
			Role:    string(msg.Role),
			Content: envelope.Content,
			Usage:   usage,
		})
	}
	return result
}

// modelMessageToSDKMessage converts a persistence format message to SDK message
// at the resolver boundary using sdk.Message's native JSON deserialization.
func modelMessageToSDKMessage(mm conversation.ModelMessage) sdk.Message {
	var s string
	if err := json.Unmarshal(mm.Content, &s); err == nil {
		return sdk.Message{
			Role:    sdk.MessageRole(mm.Role),
			Content: []sdk.MessagePart{sdk.TextPart{Text: s}},
		}
	}

	// Try the full sdk.Message format (content is an array of typed parts)
	envelope, _ := json.Marshal(struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}{
		Role:    mm.Role,
		Content: mm.Content,
	})
	var msg sdk.Message
	if err := json.Unmarshal(envelope, &msg); err == nil {
		return msg
	}

	return sdk.Message{Role: sdk.MessageRole(mm.Role)}
}

// prependUserMessage prepends the user query as a ModelMessage to the output
// messages from the agent. The SDK only returns output messages (assistant + tool);
// user messages must be added back at the resolver boundary for persistence.
func prependUserMessage(query string, output []conversation.ModelMessage) []conversation.ModelMessage {
	if strings.TrimSpace(query) == "" {
		return output
	}
	round := make([]conversation.ModelMessage, 0, 1+len(output))
	round = append(round, conversation.ModelMessage{
		Role:    "user",
		Content: conversation.NewTextContent(query),
	})
	return append(round, output...)
}

// modelMessagesToSDKMessages converts a slice of persistence messages to SDK messages.
func modelMessagesToSDKMessages(msgs []conversation.ModelMessage) []sdk.Message {
	result := make([]sdk.Message, 0, len(msgs))
	for _, mm := range msgs {
		result = append(result, modelMessageToSDKMessage(mm))
	}
	return result
}
