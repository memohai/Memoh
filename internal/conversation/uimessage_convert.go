package conversation

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/textutil"
)

const uiReplyPreviewMaxRunes = 120

var (
	uiMessageYAMLHeaderRe        = regexp.MustCompile(`(?s)\A---\n.*?\n---\n?`)
	uiMessageAgentTagsRe         = regexp.MustCompile(`(?s)<attachments>.*?</attachments>|<reactions>.*?</reactions>|<speech>.*?</speech>`)
	uiMessageCollapsedNewlinesRe = regexp.MustCompile(`\n{3,}`)
	uiTaskNotificationRe         = regexp.MustCompile(`(?s)<task-notification>\s*(.*?)\s*</task-notification>`)
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

type uiBackgroundToolRef struct {
	TurnIndex    int
	MessageIndex int
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

				applyToolResultToUIMessage(&pending.Turn.Messages[idx], toolResult.Output)
			}
		}
	}

	for _, idx := range pending.ToolIndexes {
		if idx >= 0 && idx < len(pending.Turn.Messages) {
			if !isBackgroundToolStillRunning(pending.Turn.Messages[idx]) {
				pending.Turn.Messages[idx].Running = uiBoolPtr(false)
			}
		}
	}

	return pending.Turn.Messages
}

// ConvertMessagesToUITurns converts persisted message rows into frontend-friendly turns.
func ConvertMessagesToUITurns(messages []messagepkg.Message) []UITurn {
	result := make([]UITurn, 0, len(messages))
	var pending *uiPendingAssistantTurn
	backgroundToolRefs := map[string]uiBackgroundToolRef{}

	registerBackgroundTools := func(turnIndex int) {
		if turnIndex < 0 || turnIndex >= len(result) {
			return
		}
		for msgIndex, message := range result[turnIndex].Messages {
			if message.Background == nil {
				continue
			}
			taskID := strings.TrimSpace(message.Background.TaskID)
			if taskID == "" {
				continue
			}
			backgroundToolRefs[taskID] = uiBackgroundToolRef{TurnIndex: turnIndex, MessageIndex: msgIndex}
		}
	}

	completeBackgroundTool := func(task UIBackgroundTask) {
		taskID := strings.TrimSpace(task.TaskID)
		if taskID == "" {
			return
		}
		ref, ok := backgroundToolRefs[taskID]
		if !ok || ref.TurnIndex < 0 || ref.TurnIndex >= len(result) {
			return
		}
		turn := &result[ref.TurnIndex]
		if ref.MessageIndex < 0 || ref.MessageIndex >= len(turn.Messages) {
			return
		}
		mergeBackgroundTaskIntoTool(&turn.Messages[ref.MessageIndex], task)
	}

	flushPending := func() {
		if pending == nil {
			return
		}

		for _, idx := range pending.ToolIndexes {
			if idx < 0 || idx >= len(pending.Turn.Messages) {
				continue
			}
			if !isBackgroundToolStillRunning(pending.Turn.Messages[idx]) {
				pending.Turn.Messages[idx].Running = uiBoolPtr(false)
			}
		}

		if len(pending.Turn.Messages) > 0 {
			result = append(result, pending.Turn)
			registerBackgroundTools(len(result) - 1)
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
			reply := uiReplyFromMessage(raw)
			forward := uiForwardFromMessage(raw)
			if text == "" && len(attachments) == 0 && reply == nil && forward == nil {
				continue
			}
			if task, ok := parseBackgroundTaskNotification(text); ok {
				completeBackgroundTool(task)
				result = append(result, UITurn{
					Role:           "system",
					Kind:           "background_task",
					BackgroundTask: &task,
					Timestamp:      raw.CreatedAt,
					Platform:       resolveUIPersistencePlatform(raw),
					ID:             strings.TrimSpace(raw.ID),
				})
				continue
			}
			if strings.EqualFold(strings.TrimSpace(text), "[background notification]") {
				continue
			}

			turn := UITurn{
				Role:              "user",
				Text:              text,
				Attachments:       attachments,
				Reply:             reply,
				Forward:           forward,
				Timestamp:         raw.CreatedAt,
				Platform:          resolveUIPersistencePlatform(raw),
				ExternalMessageID: strings.TrimSpace(raw.ExternalMessageID),
				ID:                strings.TrimSpace(raw.ID),
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
				Role:              "assistant",
				Messages:          assistantMessages,
				Timestamp:         raw.CreatedAt,
				Platform:          resolveUIPersistencePlatform(raw),
				ExternalMessageID: strings.TrimSpace(raw.ExternalMessageID),
				ID:                strings.TrimSpace(raw.ID),
			})
			registerBackgroundTools(len(result) - 1)

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

				applyToolResultToUIMessage(&pending.Turn.Messages[idx], toolResult.Output)
			}
		}
	}

	flushPending()
	return result
}

func newPendingAssistantTurn(raw messagepkg.Message) *uiPendingAssistantTurn {
	return &uiPendingAssistantTurn{
		Turn: UITurn{
			Role:              "assistant",
			Timestamp:         raw.CreatedAt,
			Platform:          resolveUIPersistencePlatform(raw),
			ExternalMessageID: strings.TrimSpace(raw.ExternalMessageID),
			ID:                strings.TrimSpace(raw.ID),
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
		return strings.TrimSpace(stripPersistedUserStructuredContext(text))
	}
	return strings.TrimSpace(stripPersistedAgentTags(text))
}

func stripPersistedUserStructuredContext(text string) string {
	text = strings.TrimSpace(stripPersistedYAMLHeader(text))
	if text == "" {
		return ""
	}

	text = stripPersistedMessageEnvelope(text)
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if isPersistedUserContextLine(strings.TrimSpace(line)) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func stripPersistedMessageEnvelope(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "<message") {
		return text
	}

	openEnd := strings.IndexByte(text, '>')
	if openEnd < 0 {
		return text
	}
	openTag := strings.TrimSpace(text[:openEnd+1])
	if strings.HasSuffix(openTag, "/>") {
		return ""
	}

	body := strings.TrimSpace(text[openEnd+1:])
	if !strings.HasSuffix(body, "</message>") {
		return text
	}
	return strings.TrimSpace(strings.TrimSuffix(body, "</message>"))
}

func isPersistedUserContextLine(line string) bool {
	if line == "" {
		return false
	}
	if strings.HasPrefix(line, "<attachment ") && strings.HasSuffix(line, "/>") {
		return true
	}
	if strings.HasPrefix(line, "<image ") && strings.HasSuffix(line, "</image>") {
		return true
	}
	return strings.HasPrefix(line, "<in-reply-to ") && strings.HasSuffix(line, "</in-reply-to>")
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

func uiReplyFromMessage(raw messagepkg.Message) *UIReplyRef {
	reply := UIReplyRef{MessageID: strings.TrimSpace(raw.SourceReplyToMessageID)}
	if meta, ok := raw.Metadata["reply"].(map[string]any); ok {
		if v, ok := meta["message_id"].(string); ok && strings.TrimSpace(v) != "" {
			reply.MessageID = strings.TrimSpace(v)
		}
		if v, ok := meta["sender"].(string); ok {
			reply.Sender = strings.TrimSpace(v)
		}
		if v, ok := meta["preview"].(string); ok {
			reply.Preview = truncateUIReplyPreview(v)
		}
		reply.Attachments = uiAttachmentsFromReplyMetadata(meta["attachments"], raw.BotID)
	}
	if reply.MessageID == "" && reply.Sender == "" && reply.Preview == "" && len(reply.Attachments) == 0 {
		return nil
	}
	return &reply
}

func uiAttachmentsFromReplyMetadata(value any, botID string) []UIAttachment {
	rawItems := replyAttachmentMetadataItems(value)
	if len(rawItems) == 0 {
		return nil
	}
	attachments := make([]UIAttachment, 0, len(rawItems))
	for _, item := range rawItems {
		att := UIAttachment{
			Type:        normalizeUIAttachmentType(stringFromAny(item["type"]), stringFromAny(item["mime"])),
			Path:        stringFromAny(item["path"]),
			URL:         stringFromAny(item["url"]),
			Base64:      stringFromAny(item["base64"]),
			Name:        stringFromAny(item["name"]),
			ContentHash: stringFromAny(item["content_hash"]),
			BotID:       strings.TrimSpace(botID),
			Mime:        stringFromAny(item["mime"]),
			Size:        int64FromAny(item["size"]),
			StorageKey:  stringFromAny(item["storage_key"]),
		}
		if meta, ok := item["metadata"].(map[string]any); ok {
			att.Metadata = meta
			if att.BotID == "" {
				att.BotID = stringFromAny(meta["bot_id"])
			}
			if att.StorageKey == "" {
				att.StorageKey = stringFromAny(meta["storage_key"])
			}
		}
		if att.Type == "" {
			att.Type = "file"
		}
		attachments = append(attachments, att)
	}
	if len(attachments) == 0 {
		return nil
	}
	return attachments
}

func replyAttachmentMetadataItems(value any) []map[string]any {
	switch items := value.(type) {
	case []any:
		result := make([]map[string]any, 0, len(items))
		for _, raw := range items {
			if item, ok := raw.(map[string]any); ok {
				result = append(result, item)
			}
		}
		return result
	case []map[string]any:
		return items
	default:
		return nil
	}
}

func truncateUIReplyPreview(value string) string {
	return textutil.TruncateRunesWithSuffix(strings.TrimSpace(value), uiReplyPreviewMaxRunes, "...")
}

func uiForwardFromMessage(raw messagepkg.Message) *UIForwardRef {
	meta, ok := raw.Metadata["forward"].(map[string]any)
	if !ok {
		return nil
	}
	forward := UIForwardRef{}
	if v, ok := meta["message_id"].(string); ok {
		forward.MessageID = strings.TrimSpace(v)
	}
	if v, ok := meta["from_user_id"].(string); ok {
		forward.FromUserID = strings.TrimSpace(v)
	}
	if v, ok := meta["from_conversation_id"].(string); ok {
		forward.FromConversationID = strings.TrimSpace(v)
	}
	if v, ok := meta["sender"].(string); ok {
		forward.Sender = strings.TrimSpace(v)
	}
	forward.Date = int64FromAny(meta["date"])
	if forward.MessageID == "" && forward.FromUserID == "" && forward.FromConversationID == "" && forward.Sender == "" && forward.Date == 0 {
		return nil
	}
	return &forward
}

func int64FromAny(value any) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	default:
		return 0
	}
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

func applyToolResultToUIMessage(message *UIMessage, output any) {
	if message == nil {
		return
	}
	message.Output = output
	if strings.EqualFold(strings.TrimSpace(message.Name), "exec") {
		if task, ok := backgroundTaskFromExecToolResult(output); ok {
			if task.Command == "" {
				task.Command = stringFromMap(message.Input, "command")
			}
			mergeBackgroundTaskIntoTool(message, task)
			return
		}
	}
	message.Running = uiBoolPtr(false)
}

func backgroundTaskFromExecToolResult(output any) (UIBackgroundTask, bool) {
	payload, ok := toolResultMap(output)
	if !ok {
		return UIBackgroundTask{}, false
	}

	taskID := stringFromMap(payload, "task_id")
	if taskID == "" {
		return UIBackgroundTask{}, false
	}

	statusToken := strings.ToLower(strings.TrimSpace(stringFromMap(payload, "status")))
	status := normalizeBackgroundTaskStatus(statusToken)
	switch statusToken {
	case "background_started", "auto_backgrounded", "started":
		status = "running"
	}
	if status == "" {
		return UIBackgroundTask{}, false
	}

	task := UIBackgroundTask{
		TaskID:     taskID,
		Status:     status,
		Command:    stringFromMap(payload, "command"),
		OutputFile: stringFromMap(payload, "output_file"),
		ExitCode:   int32FromAny(payload["exit_code"]),
		Duration:   stringFromMap(payload, "duration"),
		OutputTail: firstNonEmptyString(stringFromMap(payload, "output_tail"), stringFromMap(payload, "tail")),
		Stalled:    status == "stalled" || boolFromAny(payload["stalled"], false),
	}
	return task, true
}

func mergeBackgroundTaskIntoTool(message *UIMessage, task UIBackgroundTask) {
	if message == nil {
		return
	}
	merged := UIBackgroundTask{}
	if message.Background != nil {
		merged = *message.Background
	}
	if task.TaskID != "" {
		merged.TaskID = task.TaskID
	}
	if task.Status != "" {
		merged.Status = normalizeBackgroundTaskStatus(task.Status)
		if merged.Status == "" {
			merged.Status = task.Status
		}
	}
	if task.Command != "" {
		merged.Command = task.Command
	}
	if task.OutputFile != "" {
		merged.OutputFile = task.OutputFile
	}
	if task.ExitCode != 0 || isBackgroundTerminalStatus(task.Status) {
		merged.ExitCode = task.ExitCode
	}
	if task.Duration != "" {
		merged.Duration = task.Duration
	}
	if task.OutputTail != "" {
		merged.OutputTail = task.OutputTail
	}
	if task.Stream != "" {
		merged.Stream = task.Stream
	}
	if task.Chunk != "" {
		merged.Chunk = task.Chunk
	}
	if task.Stalled {
		merged.Stalled = true
	}
	if merged.Status == "" {
		merged.Status = "running"
	}
	message.Background = &merged
	message.Running = uiBoolPtr(isBackgroundToolStillRunning(*message))
}

func isBackgroundToolStillRunning(message UIMessage) bool {
	if message.Type != UIMessageTool || message.Background == nil {
		return false
	}
	status := normalizeBackgroundTaskStatus(message.Background.Status)
	return status == "running" || status == "stalled"
}

func parseBackgroundTaskNotification(text string) (UIBackgroundTask, bool) {
	match := uiTaskNotificationRe.FindStringSubmatch(text)
	if len(match) < 2 {
		return UIBackgroundTask{}, false
	}
	body := match[1]
	taskID := strings.TrimSpace(extractUITaskNotificationTag(body, "task-id"))
	if taskID == "" {
		return UIBackgroundTask{}, false
	}

	status := normalizeBackgroundTaskStatus(extractUITaskNotificationTag(body, "status"))
	if status == "" {
		status = "completed"
	}
	task := UIBackgroundTask{
		TaskID:     taskID,
		Status:     status,
		Command:    strings.TrimSpace(extractUITaskNotificationTag(body, "command")),
		OutputFile: strings.TrimSpace(extractUITaskNotificationTag(body, "output-file")),
		Duration:   strings.TrimSpace(extractUITaskNotificationTag(body, "duration")),
		OutputTail: strings.TrimSpace(extractUITaskNotificationTag(body, "output-tail")),
		Stalled:    status == "stalled",
	}
	if rawExitCode := strings.TrimSpace(extractUITaskNotificationTag(body, "exit-code")); rawExitCode != "" {
		if exitCode, err := strconv.ParseInt(rawExitCode, 10, 32); err == nil {
			task.ExitCode = int32(exitCode)
		}
	}
	return task, true
}

func extractUITaskNotificationTag(body, tag string) string {
	re := regexp.MustCompile(`(?s)<` + regexp.QuoteMeta(tag) + `>\s*(.*?)\s*</` + regexp.QuoteMeta(tag) + `>`)
	match := re.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func toolResultMap(output any) (map[string]any, bool) {
	typed, ok := output.(map[string]any)
	if !ok {
		if raw, rawOK := output.(json.RawMessage); rawOK {
			var decoded map[string]any
			if err := json.Unmarshal(raw, &decoded); err == nil {
				typed = decoded
				ok = true
			}
		}
		if !ok {
			if text, textOK := output.(string); textOK {
				var decoded map[string]any
				if err := json.Unmarshal([]byte(text), &decoded); err == nil {
					typed = decoded
					ok = true
				}
			}
		}
	}
	if !ok {
		return nil, false
	}

	for _, key := range []string{"structuredContent", "structured_content"} {
		if nested, nestedOK := typed[key].(map[string]any); nestedOK {
			return nested, true
		}
	}
	return typed, true
}

func stringFromMap(value any, key string) string {
	typed, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	return stringFromAny(typed[key])
}

func int32FromAny(value any) int32 {
	return int32(intFromAny(value)) //nolint:gosec // exit codes are small process statuses.
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeBackgroundTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "background_started", "auto_backgrounded", "started", "running":
		return "running"
	case "completed", "complete", "success", "succeeded":
		return "completed"
	case "failed", "failure", "error":
		return "failed"
	case "stalled":
		return "stalled"
	case "killed", "cancelled", "canceled":
		return "killed"
	default:
		return ""
	}
}

func isBackgroundTerminalStatus(status string) bool {
	switch normalizeBackgroundTaskStatus(status) {
	case "completed", "failed", "killed":
		return true
	default:
		return false
	}
}
