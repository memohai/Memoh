package messageconv

import (
	"encoding/json"
	"log/slog"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
)

func SDKMessagesToModelMessages(msgs []sdk.Message) []conversation.ModelMessage {
	return SDKMessagesToModelMessagesWithLogger(nil, msgs)
}

func SDKMessagesToModelMessagesWithLogger(log *slog.Logger, msgs []sdk.Message) []conversation.ModelMessage {
	result := make([]conversation.ModelMessage, 0, len(msgs))
	for _, msg := range msgs {
		data, err := json.Marshal(msg)
		if err != nil {
			if log != nil {
				log.Warn("messageconv: sdk message marshal failed", slog.String("role", string(msg.Role)), slog.Any("error", err))
			}
			continue
		}
		var envelope struct {
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			if log != nil {
				log.Warn("messageconv: sdk message content extract failed", slog.String("role", string(msg.Role)), slog.Any("error", err))
			}
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

func ModelMessageToSDKMessage(mm conversation.ModelMessage) sdk.Message {
	var s string
	if err := json.Unmarshal(mm.Content, &s); err == nil {
		return sdk.Message{
			Role:    sdk.MessageRole(mm.Role),
			Content: []sdk.MessagePart{sdk.TextPart{Text: s}},
		}
	}

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

func ModelMessagesToSDKMessages(msgs []conversation.ModelMessage) []sdk.Message {
	result := make([]sdk.Message, 0, len(msgs))
	for _, mm := range msgs {
		result = append(result, ModelMessageToSDKMessage(mm))
	}
	return result
}

func PrependUserMessage(query string, output []conversation.ModelMessage) []conversation.ModelMessage {
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
