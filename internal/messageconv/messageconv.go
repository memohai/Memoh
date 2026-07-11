package messageconv

import (
	"encoding/json"
	"log/slog"
	"strings"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/contextbudget"
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
	msg, _ := decodeModelMessage(mm)
	return supplementLegacyFields(mm, msg)
}

func decodeModelMessage(mm conversation.ModelMessage) (sdk.Message, bool) {
	msg := sdk.Message{Role: sdk.MessageRole(mm.Role)}
	content := normalizeLegacyContent(mm.Content)
	envelope, err := json.Marshal(struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}{
		Role:    mm.Role,
		Content: content,
	})
	if err != nil {
		return msg, false
	}
	var decoded sdk.Message
	if json.Unmarshal(envelope, &decoded) != nil {
		return msg, false
	}
	return decoded, true
}

func supplementLegacyFields(mm conversation.ModelMessage, msg sdk.Message) sdk.Message {
	for _, call := range mm.ToolCalls {
		callID := strings.TrimSpace(call.ID)
		if messageHasToolCall(msg, callID) {
			continue
		}
		input := any(map[string]any{})
		arguments := strings.TrimSpace(call.Function.Arguments)
		if arguments != "" && json.Unmarshal([]byte(arguments), &input) != nil {
			input = arguments
		}
		msg.Content = append(msg.Content, sdk.ToolCallPart{
			ToolCallID: callID,
			ToolName:   strings.TrimSpace(call.Function.Name),
			Input:      input,
		})
	}
	if strings.TrimSpace(mm.ToolCallID) != "" && !messageHasToolResult(msg) {
		var result any
		if len(mm.Content) > 0 && json.Unmarshal(mm.Content, &result) != nil {
			result = string(mm.Content)
		}
		msg.Content = []sdk.MessagePart{sdk.ToolResultPart{
			ToolCallID: strings.TrimSpace(mm.ToolCallID),
			ToolName:   strings.TrimSpace(mm.Name),
			Result:     result,
		}}
	}
	return msg
}

// CanonicalModelMessageContent projects persistence variants onto the native
// content parts sent through pipeline and provider paths.
func CanonicalModelMessageContent(msg conversation.ModelMessage) json.RawMessage {
	raw, _ := canonicalModelMessageContent(msg)
	return raw
}

func canonicalModelMessageContent(msg conversation.ModelMessage) (json.RawMessage, bool) {
	sdkMessage, valid := decodeModelMessage(msg)
	sdkMessage = supplementLegacyFields(msg, sdkMessage)
	sdkMessage = CanonicalSDKMessage(sdkMessage)
	return sdkMessageContent(sdkMessage), valid
}

// CanonicalSDKMessage returns a content-slice-independent provider projection.
func CanonicalSDKMessage(message sdk.Message) sdk.Message {
	canonical := message
	canonical.Content = nil
	for _, part := range message.Content {
		if _, reasoning := part.(sdk.ReasoningPart); reasoning {
			continue
		}
		canonical.Content = append(canonical.Content, part)
	}
	return canonical
}

// EstimateModelMessageTokens meters the canonical model-visible content.
func EstimateModelMessageTokens(msg conversation.ModelMessage) int {
	raw, valid := canonicalModelMessageContent(msg)
	if len(raw) == 0 && len(msg.Content) > 0 && !valid {
		return contextbudget.EstimateTokensForBytes(len(msg.Content))
	}
	return EstimateCanonicalContentTokens(raw)
}

// EstimateCanonicalContentTokens meters canonical native content parts by
// their semantic payload rather than JSON envelope syntax.
func EstimateCanonicalContentTokens(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	envelope, _ := json.Marshal(struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}{
		Role:    "assistant",
		Content: raw,
	})
	var message sdk.Message
	if json.Unmarshal(envelope, &message) != nil {
		return contextbudget.EstimateTokensForBytes(len(raw))
	}
	return EstimateSDKMessageTokens(message)
}

// EstimateSDKMessageTokens meters the provider-visible semantic payload.
func EstimateSDKMessageTokens(message sdk.Message) int {
	message = CanonicalSDKMessage(message)
	bytes := 0
	for _, part := range message.Content {
		switch typed := part.(type) {
		case sdk.TextPart:
			bytes += len(typed.Text)
			bytes += encodedSize(typed.CacheControl) + encodedSize(typed.ProviderMetadata)
		case sdk.ToolCallPart:
			bytes += len(typed.ToolCallID) + len(typed.ToolName)
			bytes += encodedSize(typed.Input) + encodedSize(typed.ProviderMetadata)
		case sdk.ToolResultPart:
			bytes += len(typed.ToolCallID) + len(typed.ToolName)
			bytes += encodedSize(typed.Result)
			if typed.IsError {
				bytes += len("true")
			}
		case sdk.ImagePart:
			bytes += len(typed.Image) + len(typed.MediaType) + encodedSize(typed.CacheControl)
		case sdk.FilePart:
			bytes += len(typed.Data) + len(typed.MediaType) + len(typed.Filename) + encodedSize(typed.CacheControl)
		default:
			bytes += encodedSize(part)
		}
	}
	return contextbudget.EstimateTokensForBytes(bytes)
}

func messageHasToolResult(message sdk.Message) bool {
	for _, part := range message.Content {
		if _, ok := part.(sdk.ToolResultPart); ok {
			return true
		}
	}
	return false
}

func messageHasToolCall(message sdk.Message, callID string) bool {
	if callID == "" {
		return false
	}
	for _, part := range message.Content {
		call, ok := part.(sdk.ToolCallPart)
		if ok && strings.TrimSpace(call.ToolCallID) == callID {
			return true
		}
	}
	return false
}

func sdkMessageContent(message sdk.Message) json.RawMessage {
	if len(message.Content) == 0 {
		return nil
	}
	parts := make([]json.RawMessage, 0, len(message.Content))
	for _, part := range message.Content {
		fields := make(map[string]json.RawMessage)
		raw, err := json.Marshal(part)
		if err != nil || json.Unmarshal(raw, &fields) != nil {
			return nil
		}
		partType, err := json.Marshal(part.PartType())
		if err != nil {
			return nil
		}
		fields["type"] = partType
		raw, err = json.Marshal(fields)
		if err != nil {
			return nil
		}
		parts = append(parts, raw)
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return nil
	}
	return raw
}

func normalizeLegacyContent(content json.RawMessage) json.RawMessage {
	var parts []map[string]json.RawMessage
	if json.Unmarshal(content, &parts) != nil {
		return content
	}
	changed := false
	for _, part := range parts {
		var partType string
		if json.Unmarshal(part["type"], &partType) != nil || partType != string(sdk.MessagePartTypeToolResult) {
			continue
		}
		output, exists := part["output"]
		if !exists {
			continue
		}
		result, hasResult := part["result"]
		if strings.TrimSpace(string(output)) != "null" || !hasResult {
			part["result"] = output
		} else {
			part["result"] = result
		}
		delete(part, "output")
		changed = true
	}
	if !changed {
		return content
	}
	normalized, err := json.Marshal(parts)
	if err != nil {
		return content
	}
	return normalized
}

func encodedSize(value any) int {
	if value == nil {
		return 0
	}
	data, err := json.Marshal(value)
	if err != nil || string(data) == "null" || string(data) == "{}" {
		return 0
	}
	return len(data)
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
