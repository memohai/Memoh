package conversation

import (
	"encoding/json"
	"regexp"
	"strings"

	messagepkg "github.com/memohai/memoh/internal/message"
)

var (
	uiMessageYAMLHeaderRe        = regexp.MustCompile(`(?s)\A---\n.*?\n---\n?`)
	uiMessageAgentTagsRe         = regexp.MustCompile(`(?s)<attachments>.*?</attachments>|<reactions>.*?</reactions>|<speech>.*?</speech>`)
	uiMessageCollapsedNewlinesRe = regexp.MustCompile(`\n{3,}`)
)

type uiContentPart struct {
	Type             string         `json:"type"`
	Text             string         `json:"text,omitempty"`
	URL              string         `json:"url,omitempty"`
	Emoji            string         `json:"emoji,omitempty"`
	ToolCallID       string         `json:"toolCallId,omitempty"`
	ToolName         string         `json:"toolName,omitempty"`
	Input            any            `json:"input,omitempty"`
	Output           any            `json:"output,omitempty"`
	Result           any            `json:"result,omitempty"`
	ProviderMetadata map[string]any `json:"providerMetadata,omitempty"`
}

type uiExtractedToolCall struct {
	ID       string
	Name     string
	Input    any
	Approval *UIToolApproval
}

type uiExtractedToolResult struct {
	ToolCallID string
	Output     any
}

type uiPendingAssistantTurn struct {
	Turn        UITurn
	NextID      int
	ToolIndexes map[string]int
}

// ConvertRawModelMessagesToUIAssistantMessages converts terminal stream payload
// messages into frontend-friendly assistant UI messages.
func ConvertRawModelMessagesToUIAssistantMessages(raw json.RawMessage) []UIMessage {
	if len(raw) == 0 {
		return nil
	}

	var messages []ModelMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		return nil
	}
	return ConvertModelMessagesToUIAssistantMessages(messages)
}

// ConvertModelMessagesToUIAssistantMessages converts assistant/tool output
// messages into frontend-friendly UI message blocks.
func ConvertModelMessagesToUIAssistantMessages(messages []ModelMessage) []UIMessage {
	pending := &uiPendingAssistantTurn{
		ToolIndexes: map[string]int{},
	}

	for _, modelMessage := range messages {
		switch strings.ToLower(strings.TrimSpace(modelMessage.Role)) {
		case "assistant":
			for _, reasoning := range extractPersistedReasoning(modelMessage) {
				appendPendingAssistantMessage(pending, UIMessage{
					Type:    UIMessageReasoning,
					Content: reasoning,
				})
			}

			if text := extractAssistantStreamMessageText(modelMessage); text != "" {
				appendPendingAssistantMessage(pending, UIMessage{
					Type:    UIMessageText,
					Content: text,
				})
			}

			for _, call := range extractPersistedToolCalls(modelMessage) {
				appendPendingAssistantMessage(pending, UIMessage{
					Type:       UIMessageTool,
					Name:       call.Name,
					Input:      call.Input,
					ToolCallID: call.ID,
					Running:    uiBoolPtr(true),
					Approval:   call.Approval,
				})
				if call.ID != "" {
					pending.ToolIndexes[call.ID] = len(pending.Turn.Messages) - 1
				}
			}

		case "tool":
			for _, toolResult := range extractPersistedToolResults(modelMessage) {
				idx, ok := pending.ToolIndexes[toolResult.ToolCallID]
				if !ok || idx < 0 || idx >= len(pending.Turn.Messages) {
					continue
				}

				if isHiddenCurrentConversationToolOutput(toolResult.Output) {
					removePendingAssistantMessage(pending, idx)
					delete(pending.ToolIndexes, toolResult.ToolCallID)
					continue
				}

				pending.Turn.Messages[idx].Output = toolResult.Output
				pending.Turn.Messages[idx].Running = uiBoolPtr(false)
			}
		}
	}

	for _, idx := range pending.ToolIndexes {
		if idx >= 0 && idx < len(pending.Turn.Messages) {
			pending.Turn.Messages[idx].Running = uiBoolPtr(false)
		}
	}

	return pending.Turn.Messages
}

// ConvertMessagesToUITurns converts persisted message rows into frontend-friendly turns.
func ConvertMessagesToUITurns(messages []messagepkg.Message) []UITurn {
	result := make([]UITurn, 0, len(messages))
	var pending *uiPendingAssistantTurn

	flushPending := func() {
		if pending == nil {
			return
		}

		for _, idx := range pending.ToolIndexes {
			if idx < 0 || idx >= len(pending.Turn.Messages) {
				continue
			}
			pending.Turn.Messages[idx].Running = uiBoolPtr(false)
		}

		if len(pending.Turn.Messages) > 0 {
			result = append(result, pending.Turn)
		}
		pending = nil
	}

	for _, raw := range messages {
		modelMessage := decodePersistedModelMessage(raw)
		switch strings.ToLower(strings.TrimSpace(raw.Role)) {
		case "user":
			flushPending()

			text := extractPersistedMessageText(raw, modelMessage)
			attachments := uiAttachmentsFromMessageAssets(raw)
			if text == "" && len(attachments) == 0 {
				continue
			}

			turn := UITurn{
				Role:        "user",
				Text:        text,
				Attachments: attachments,
				Timestamp:   raw.CreatedAt,
				Platform:    resolveUIPersistencePlatform(raw),
				ID:          strings.TrimSpace(raw.ID),
			}
			if turn.Platform != "" {
				turn.SenderDisplayName = strings.TrimSpace(raw.SenderDisplayName)
				turn.SenderAvatarURL = strings.TrimSpace(raw.SenderAvatarURL)
				turn.SenderUserID = strings.TrimSpace(raw.SenderUserID)
			}
			result = append(result, turn)

		case "assistant":
			toolCalls := extractPersistedToolCalls(modelMessage)
			text := extractPersistedMessageText(raw, modelMessage)
			reasonings := extractPersistedReasoning(modelMessage)
			attachments := uiAttachmentsFromMessageAssets(raw)

			if len(toolCalls) > 0 {
				if pending == nil {
					pending = newPendingAssistantTurn(raw)
				}

				for _, reasoning := range reasonings {
					appendPendingAssistantMessage(pending, UIMessage{
						ID:      pending.NextID,
						Type:    UIMessageReasoning,
						Content: reasoning,
					})
				}

				if text != "" {
					appendPendingAssistantMessage(pending, UIMessage{
						ID:      pending.NextID,
						Type:    UIMessageText,
						Content: text,
					})
				}

				for _, call := range toolCalls {
					block := UIMessage{
						ID:         pending.NextID,
						Type:       UIMessageTool,
						Name:       call.Name,
						Input:      call.Input,
						ToolCallID: call.ID,
						Running:    uiBoolPtr(true),
						Approval:   call.Approval,
					}
					appendPendingAssistantMessage(pending, block)
					if call.ID != "" {
						pending.ToolIndexes[call.ID] = len(pending.Turn.Messages) - 1
					}
				}

				if len(attachments) > 0 {
					appendPendingAssistantMessage(pending, UIMessage{
						ID:          pending.NextID,
						Type:        UIMessageAttachments,
						Attachments: attachments,
					})
				}
				continue
			}

			if pending != nil && (text != "" || len(reasonings) > 0 || len(attachments) > 0) {
				for _, reasoning := range reasonings {
					appendPendingAssistantMessage(pending, UIMessage{
						ID:      pending.NextID,
						Type:    UIMessageReasoning,
						Content: reasoning,
					})
				}
				if text != "" {
					appendPendingAssistantMessage(pending, UIMessage{
						ID:      pending.NextID,
						Type:    UIMessageText,
						Content: text,
					})
				}
				if len(attachments) > 0 {
					appendPendingAssistantMessage(pending, UIMessage{
						ID:          pending.NextID,
						Type:        UIMessageAttachments,
						Attachments: attachments,
					})
				}
				flushPending()
				continue
			}

			flushPending()

			assistantMessages := buildStandaloneAssistantMessages(text, reasonings, attachments)
			if len(assistantMessages) == 0 {
				continue
			}

			result = append(result, UITurn{
				Role:      "assistant",
				Messages:  assistantMessages,
				Timestamp: raw.CreatedAt,
				Platform:  resolveUIPersistencePlatform(raw),
				ID:        strings.TrimSpace(raw.ID),
			})

		case "tool":
			if pending == nil {
				continue
			}

			for _, toolResult := range extractPersistedToolResults(modelMessage) {
				idx, ok := pending.ToolIndexes[toolResult.ToolCallID]
				if !ok || idx < 0 || idx >= len(pending.Turn.Messages) {
					continue
				}

				if isHiddenCurrentConversationToolOutput(toolResult.Output) {
					removePendingAssistantMessage(pending, idx)
					delete(pending.ToolIndexes, toolResult.ToolCallID)
					continue
				}

				pending.Turn.Messages[idx].Output = toolResult.Output
				pending.Turn.Messages[idx].Running = uiBoolPtr(false)
			}
		}
	}

	flushPending()
	return result
}

func newPendingAssistantTurn(raw messagepkg.Message) *uiPendingAssistantTurn {
	return &uiPendingAssistantTurn{
		Turn: UITurn{
			Role:      "assistant",
			Timestamp: raw.CreatedAt,
			Platform:  resolveUIPersistencePlatform(raw),
			ID:        strings.TrimSpace(raw.ID),
		},
		ToolIndexes: map[string]int{},
	}
}

func appendPendingAssistantMessage(pending *uiPendingAssistantTurn, message UIMessage) {
	if pending == nil {
		return
	}
	message.ID = pending.NextID
	pending.NextID++
	pending.Turn.Messages = append(pending.Turn.Messages, message)
}

func removePendingAssistantMessage(pending *uiPendingAssistantTurn, idx int) {
	if pending == nil || idx < 0 || idx >= len(pending.Turn.Messages) {
		return
	}

	pending.Turn.Messages = append(pending.Turn.Messages[:idx], pending.Turn.Messages[idx+1:]...)
	for callID, currentIdx := range pending.ToolIndexes {
		switch {
		case currentIdx == idx:
			delete(pending.ToolIndexes, callID)
		case currentIdx > idx:
			pending.ToolIndexes[callID] = currentIdx - 1
		}
	}
}

func buildStandaloneAssistantMessages(text string, reasonings []string, attachments []UIAttachment) []UIMessage {
	messages := make([]UIMessage, 0, len(reasonings)+2)
	nextID := 0
	for _, reasoning := range reasonings {
		messages = append(messages, UIMessage{
			ID:      nextID,
			Type:    UIMessageReasoning,
			Content: reasoning,
		})
		nextID++
	}
	if text != "" {
		messages = append(messages, UIMessage{
			ID:      nextID,
			Type:    UIMessageText,
			Content: text,
		})
		nextID++
	}
	if len(attachments) > 0 {
		messages = append(messages, UIMessage{
			ID:          nextID,
			Type:        UIMessageAttachments,
			Attachments: attachments,
		})
	}
	return messages
}

func decodePersistedModelMessage(raw messagepkg.Message) ModelMessage {
	var message ModelMessage
	if err := json.Unmarshal(raw.Content, &message); err != nil {
		return ModelMessage{
			Role:    raw.Role,
			Content: raw.Content,
		}
	}
	message.Role = raw.Role
	return message
}

func extractPersistedMessageText(raw messagepkg.Message, message ModelMessage) string {
	if strings.EqualFold(raw.Role, "user") {
		if text := strings.TrimSpace(raw.DisplayContent); text != "" {
			return text
		}
	}

	text := strings.TrimSpace(extractTextFromPersistedContent(message.Content))
	if text == "" {
		return ""
	}

	if strings.EqualFold(raw.Role, "user") {
		return strings.TrimSpace(stripPersistedYAMLHeader(text))
	}
	return strings.TrimSpace(stripPersistedAgentTags(text))
}

func extractAssistantStreamMessageText(message ModelMessage) string {
	return strings.TrimSpace(stripPersistedAgentTags(extractTextFromPersistedContent(message.Content)))
}

func extractTextFromPersistedContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}

	parts := extractPersistedContentParts(raw)
	if len(parts) > 0 {
		lines := make([]string, 0, len(parts))
		for _, part := range parts {
			partType := strings.ToLower(strings.TrimSpace(part.Type))
			if partType == "reasoning" {
				continue
			}
			switch {
			case partType == "text" && strings.TrimSpace(part.Text) != "":
				lines = append(lines, strings.TrimSpace(part.Text))
			case partType == "link" && strings.TrimSpace(part.URL) != "":
				lines = append(lines, strings.TrimSpace(part.URL))
			case partType == "emoji" && strings.TrimSpace(part.Emoji) != "":
				lines = append(lines, strings.TrimSpace(part.Emoji))
			case strings.TrimSpace(part.Text) != "":
				lines = append(lines, strings.TrimSpace(part.Text))
			}
		}
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}

	var object map[string]any
	if err := json.Unmarshal(raw, &object); err == nil {
		if value, ok := object["text"].(string); ok {
			return strings.TrimSpace(value)
		}
	}

	return ""
}

func extractPersistedReasoning(message ModelMessage) []string {
	parts := extractPersistedContentParts(message.Content)
	if len(parts) == 0 {
		return nil
	}

	reasonings := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.ToLower(strings.TrimSpace(part.Type)) != "reasoning" {
			continue
		}
		if text := strings.TrimSpace(part.Text); text != "" {
			reasonings = append(reasonings, text)
		}
	}
	return reasonings
}

func extractPersistedToolCalls(message ModelMessage) []uiExtractedToolCall {
	parts := extractPersistedContentParts(message.Content)
	calls := make([]uiExtractedToolCall, 0, len(parts)+len(message.ToolCalls))
	for _, part := range parts {
		if strings.ToLower(strings.TrimSpace(part.Type)) != "tool-call" {
			continue
		}
		calls = append(calls, uiExtractedToolCall{
			ID:       strings.TrimSpace(part.ToolCallID),
			Name:     strings.TrimSpace(part.ToolName),
			Input:    part.Input,
			Approval: extractApprovalMetadata(part.ProviderMetadata),
		})
	}
	if len(calls) > 0 {
		return calls
	}

	for _, toolCall := range message.ToolCalls {
		input := any(nil)
		if rawArgs := strings.TrimSpace(toolCall.Function.Arguments); rawArgs != "" {
			if err := json.Unmarshal([]byte(rawArgs), &input); err != nil {
				input = rawArgs
			}
		}
		calls = append(calls, uiExtractedToolCall{
			ID:    strings.TrimSpace(toolCall.ID),
			Name:  strings.TrimSpace(toolCall.Function.Name),
			Input: input,
		})
	}
	return calls
}

func extractApprovalMetadata(metadata map[string]any) *UIToolApproval {
	if metadata == nil {
		return nil
	}
	raw, ok := metadata["approval"]
	if !ok {
		return nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	approvalID, _ := obj["approval_id"].(string)
	approvalID = strings.TrimSpace(approvalID)
	if approvalID == "" {
		return nil
	}
	status, _ := obj["status"].(string)
	status = strings.TrimSpace(status)
	if status == "" {
		status = "pending"
	}
	return &UIToolApproval{
		ApprovalID:     approvalID,
		ShortID:        intFromAny(obj["short_id"]),
		Status:         status,
		DecisionReason: stringFromAny(obj["decision_reason"]),
		CanApprove:     boolFromAny(obj["can_approve"], true),
	}
}

func intFromAny(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	default:
		return 0
	}
}

func stringFromAny(value any) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func boolFromAny(value any, fallback bool) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	return fallback
}

func extractPersistedToolResults(message ModelMessage) []uiExtractedToolResult {
	parts := extractPersistedContentParts(message.Content)
	results := make([]uiExtractedToolResult, 0, len(parts))
	for _, part := range parts {
		if strings.ToLower(strings.TrimSpace(part.Type)) != "tool-result" {
			continue
		}
		output := part.Output
		if output == nil {
			output = part.Result
		}
		results = append(results, uiExtractedToolResult{
			ToolCallID: strings.TrimSpace(part.ToolCallID),
			Output:     output,
		})
	}
	if len(results) > 0 {
		return results
	}

	if strings.TrimSpace(message.ToolCallID) == "" {
		return nil
	}

	var output any
	if err := json.Unmarshal(message.Content, &output); err != nil {
		output = strings.TrimSpace(string(message.Content))
	}
	return []uiExtractedToolResult{{
		ToolCallID: strings.TrimSpace(message.ToolCallID),
		Output:     output,
	}}
}

func extractPersistedContentParts(raw json.RawMessage) []uiContentPart {
	if len(raw) == 0 {
		return nil
	}

	var parts []uiContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		return parts
	}

	var encoded string
	if err := json.Unmarshal(raw, &encoded); err == nil {
		trimmed := strings.TrimSpace(encoded)
		if strings.HasPrefix(trimmed, "[") && json.Unmarshal([]byte(trimmed), &parts) == nil {
			return parts
		}
	}

	var object struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &object); err == nil && len(object.Content) > 0 {
		return extractPersistedContentParts(object.Content)
	}

	return nil
}

func uiAttachmentsFromMessageAssets(raw messagepkg.Message) []UIAttachment {
	if len(raw.Assets) == 0 {
		return nil
	}

	attachments := make([]UIAttachment, 0, len(raw.Assets))
	for _, asset := range raw.Assets {
		attachments = append(attachments, UIAttachment{
			ID:          strings.TrimSpace(asset.ContentHash),
			Type:        normalizeUIAttachmentType("", asset.Mime),
			Name:        strings.TrimSpace(asset.Name),
			ContentHash: strings.TrimSpace(asset.ContentHash),
			BotID:       strings.TrimSpace(raw.BotID),
			Mime:        strings.TrimSpace(asset.Mime),
			Size:        asset.SizeBytes,
			StorageKey:  strings.TrimSpace(asset.StorageKey),
			Metadata:    asset.Metadata,
		})
	}
	return attachments
}

func resolveUIPersistencePlatform(raw messagepkg.Message) string {
	direct := strings.ToLower(strings.TrimSpace(raw.Platform))
	if direct == "local" {
		return ""
	}
	if direct != "" {
		return direct
	}

	if raw.Metadata != nil {
		if platform, ok := raw.Metadata["platform"].(string); ok {
			trimmed := strings.ToLower(strings.TrimSpace(platform))
			if trimmed == "local" {
				return ""
			}
			return trimmed
		}
	}
	return ""
}

func stripPersistedYAMLHeader(text string) string {
	return strings.TrimSpace(uiMessageYAMLHeaderRe.ReplaceAllString(text, ""))
}

func stripPersistedAgentTags(text string) string {
	stripped := uiMessageAgentTagsRe.ReplaceAllString(text, "")
	return strings.TrimSpace(uiMessageCollapsedNewlinesRe.ReplaceAllString(stripped, "\n\n"))
}

func isHiddenCurrentConversationToolOutput(output any) bool {
	typed, ok := output.(map[string]any)
	if !ok {
		return false
	}
	delivered, _ := typed["delivered"].(string)
	return strings.EqualFold(strings.TrimSpace(delivered), "current_conversation")
}
