package feishu

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/memohai/memoh/internal/channel"
)

// extractFeishuInbound converts a Feishu P2MessageReceiveV1 event into a channel.InboundMessage.
// botOpenID is the bot's own open_id used to filter mentions; if empty, any mention is treated as bot mention.
func extractFeishuInbound(event *larkim.P2MessageReceiveV1, botOpenID string) channel.InboundMessage {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return channel.InboundMessage{Channel: Type}
	}
	message := event.Event.Message

	var msg channel.Message
	if message.MessageId != nil {
		msg.ID = *message.MessageId
	}

	var contentMap map[string]any
	if message.Content != nil {
		if err := json.Unmarshal([]byte(*message.Content), &contentMap); err != nil {
			slog.Warn("feishu inbound: unmarshal content failed", slog.Any("error", err))
		}
	}
	isMentioned := isFeishuBotMentioned(contentMap, message.Mentions, botOpenID)

	if message.MessageType != nil {
		switch *message.MessageType {
		case larkim.MsgTypeText:
			if txt, ok := contentMap["text"].(string); ok {
				msg.Text = txt
			}
		case larkim.MsgTypePost:
			postText := extractFeishuPostText(contentMap)
			if postText != "" {
				msg.Text = postText
			}
			postAtts := extractFeishuPostAttachments(contentMap, msg.ID)
			for _, att := range postAtts {
				msg.Attachments = append(msg.Attachments, att)
			}
			if len(postAtts) > 0 || postText != "" {
				slog.Debug("feishu post extracted",
					"message_id", msg.ID,
					"text_len", len(postText),
					"attachments", len(postAtts),
				)
			}
		case larkim.MsgTypeImage:
			if key, ok := contentMap["image_key"].(string); ok {
				msg.Attachments = append(msg.Attachments, channel.Attachment{
					Type:           channel.AttachmentImage,
					PlatformKey:    key,
					SourcePlatform: Type.String(),
					Metadata:       map[string]any{"message_id": msg.ID},
				})
			}
		case larkim.MsgTypeFile, larkim.MsgTypeAudio, larkim.MsgTypeMedia:
			if key, ok := contentMap["file_key"].(string); ok {
				name, _ := contentMap["file_name"].(string)
				attType := channel.AttachmentFile
				switch *message.MessageType {
				case larkim.MsgTypeAudio:
					attType = channel.AttachmentAudio
				case larkim.MsgTypeMedia:
					attType = channel.AttachmentVideo
				}
				msg.Attachments = append(msg.Attachments, channel.Attachment{
					Type:           attType,
					PlatformKey:    key,
					SourcePlatform: Type.String(),
					Name:           name,
					Metadata:       map[string]any{"message_id": msg.ID},
				})
			}
		}
	}

	if message.ParentId != nil && *message.ParentId != "" {
		msg.Reply = &channel.ReplyRef{
			MessageID: *message.ParentId,
		}
	}

	senderID, senderOpenID := "", ""
	if event.Event.Sender != nil && event.Event.Sender.SenderId != nil {
		if event.Event.Sender.SenderId.UserId != nil {
			senderID = strings.TrimSpace(*event.Event.Sender.SenderId.UserId)
		}
		if event.Event.Sender.SenderId.OpenId != nil {
			senderOpenID = strings.TrimSpace(*event.Event.Sender.SenderId.OpenId)
		}
	}
	chatID := ""
	chatType := ""
	if message.ChatId != nil {
		chatID = strings.TrimSpace(*message.ChatId)
	}
	if message.ChatType != nil {
		chatType = strings.TrimSpace(*message.ChatType)
	}
	replyTo := senderOpenID
	if replyTo == "" {
		replyTo = senderID
	}
	if chatType != "" && chatType != "p2p" && chatID != "" {
		replyTo = "chat_id:" + chatID
	}
	attrs := map[string]string{}
	if senderID != "" {
		attrs["user_id"] = senderID
	}
	if senderOpenID != "" {
		attrs["open_id"] = senderOpenID
	}
	subjectID := senderOpenID
	if subjectID == "" {
		subjectID = senderID
	}

	return channel.InboundMessage{
		Channel:     Type,
		Message:     msg,
		ReplyTarget: replyTo,
		Sender: channel.Identity{
			SubjectID:  subjectID,
			Attributes: attrs,
		},
		Conversation: channel.Conversation{
			ID:   chatID,
			Type: chatType,
		},
		ReceivedAt: time.Now().UTC(),
		Source:     "feishu",
		Metadata: map[string]any{
			"is_mentioned": isMentioned,
		},
	}
}

// isFeishuBotMentioned checks whether the bot itself is mentioned in the message.
// When botOpenID is provided, only mentions matching the bot's open_id count.
// When botOpenID is empty (fallback), any mention is treated as a bot mention.
func isFeishuBotMentioned(contentMap map[string]any, mentions []*larkim.MentionEvent, botOpenID string) bool {
	botOpenID = strings.TrimSpace(botOpenID)
	if botOpenID == "" {
		return hasAnyFeishuMention(contentMap, mentions)
	}
	for _, m := range mentions {
		if m == nil || m.Id == nil || m.Id.OpenId == nil {
			continue
		}
		if strings.TrimSpace(*m.Id.OpenId) == botOpenID {
			return true
		}
	}
	if matchFeishuContentMention(contentMap, botOpenID) {
		return true
	}
	return false
}

// hasAnyFeishuMention is the fallback when the bot's open_id is unknown.
func hasAnyFeishuMention(contentMap map[string]any, mentions []*larkim.MentionEvent) bool {
	if len(mentions) > 0 {
		return true
	}
	if len(contentMap) == 0 {
		return false
	}
	if raw, ok := contentMap["mentions"]; ok {
		switch values := raw.(type) {
		case []any:
			if len(values) > 0 {
				return true
			}
		case []map[string]any:
			if len(values) > 0 {
				return true
			}
		}
	}
	if text, ok := contentMap["text"].(string); ok {
		normalized := strings.ToLower(strings.TrimSpace(text))
		if strings.Contains(normalized, "@_user_") || strings.Contains(normalized, "<at ") || strings.Contains(normalized, "</at>") {
			return true
		}
	}
	return hasFeishuAtTag(contentMap)
}

// matchFeishuContentMention checks rich-text at tags for the bot's open_id.
func matchFeishuContentMention(raw any, botOpenID string) bool {
	switch value := raw.(type) {
	case map[string]any:
		if tag, ok := value["tag"].(string); ok && strings.EqualFold(strings.TrimSpace(tag), "at") {
			if uid, ok := value["user_id"].(string); ok && strings.TrimSpace(uid) == botOpenID {
				return true
			}
			if uid, ok := value["open_id"].(string); ok && strings.TrimSpace(uid) == botOpenID {
				return true
			}
		}
		for _, child := range value {
			if matchFeishuContentMention(child, botOpenID) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if matchFeishuContentMention(child, botOpenID) {
				return true
			}
		}
	}
	return false
}

func hasFeishuAtTag(raw any) bool {
	switch value := raw.(type) {
	case map[string]any:
		if tag, ok := value["tag"].(string); ok && strings.EqualFold(strings.TrimSpace(tag), "at") {
			return true
		}
		for _, child := range value {
			if hasFeishuAtTag(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if hasFeishuAtTag(child) {
				return true
			}
		}
	}
	return false
}

// getFeishuPostContentLines returns content lines from post message.
// Feishu event payload uses root-level content: {"title":"","content":[[...],[...]]}.
func getFeishuPostContentLines(contentMap map[string]any) []any {
	if lines, ok := contentMap["content"].([]any); ok {
		return lines
	}
	return nil
}

// extractFeishuPostAttachments extracts image/file attachments from post content (e.g. img elements).
func extractFeishuPostAttachments(contentMap map[string]any, messageID string) []channel.Attachment {
	var result []channel.Attachment
	linesRaw := getFeishuPostContentLines(contentMap)
	if linesRaw == nil {
		return result
	}
	for _, rawLine := range linesRaw {
		line, ok := rawLine.([]any)
		if !ok {
			continue
		}
		for _, rawPart := range line {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			tag := strings.ToLower(strings.TrimSpace(stringValue(part["tag"])))
			if tag == "img" {
				if key, ok := part["image_key"].(string); ok && strings.TrimSpace(key) != "" {
					result = append(result, channel.Attachment{
						Type:           channel.AttachmentImage,
						PlatformKey:    strings.TrimSpace(key),
						SourcePlatform: Type.String(),
						Metadata:       map[string]any{"message_id": messageID},
					})
				}
			}
			if tag == "file" {
				if key, ok := part["file_key"].(string); ok && strings.TrimSpace(key) != "" {
					name := strings.TrimSpace(stringValue(part["file_name"]))
					result = append(result, channel.Attachment{
						Type:           channel.AttachmentFile,
						PlatformKey:    strings.TrimSpace(key),
						SourcePlatform: Type.String(),
						Name:           name,
						Metadata:       map[string]any{"message_id": messageID},
					})
				}
			}
		}
	}
	return result
}

func extractFeishuPostText(contentMap map[string]any) string {
	linesRaw := getFeishuPostContentLines(contentMap)
	if linesRaw == nil {
		return ""
	}
	parts := make([]string, 0, 8)
	for _, rawLine := range linesRaw {
		line, ok := rawLine.([]any)
		if !ok {
			continue
		}
		for _, rawPart := range line {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			tag := strings.ToLower(strings.TrimSpace(stringValue(part["tag"])))
			switch tag {
			case "text", "a":
				text := strings.TrimSpace(stringValue(part["text"]))
				if text != "" {
					parts = append(parts, text)
				}
			case "at":
				name := strings.TrimSpace(stringValue(part["text"]))
				if name == "" {
					name = strings.TrimSpace(stringValue(part["name"]))
				}
				if name == "" {
					name = strings.TrimSpace(stringValue(part["user_name"]))
				}
				if name == "" {
					parts = append(parts, "@")
					continue
				}
				if !strings.HasPrefix(name, "@") {
					name = "@" + name
				}
				parts = append(parts, name)
			default:
				text := strings.TrimSpace(stringValue(part["text"]))
				if text != "" {
					parts = append(parts, text)
				}
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	value, ok := raw.(string)
	if ok {
		return value
	}
	return fmt.Sprint(raw)
}

// resolveFeishuReceiveID parses target (open_id:/user_id:/chat_id: prefix) and returns receiveID and receiveType.
func resolveFeishuReceiveID(raw string) (string, string, error) {
	if raw == "" {
		return "", "", fmt.Errorf("feishu target is required")
	}
	if strings.HasPrefix(raw, "open_id:") {
		return strings.TrimPrefix(raw, "open_id:"), larkim.ReceiveIdTypeOpenId, nil
	}
	if strings.HasPrefix(raw, "user_id:") {
		return strings.TrimPrefix(raw, "user_id:"), larkim.ReceiveIdTypeUserId, nil
	}
	if strings.HasPrefix(raw, "chat_id:") {
		return strings.TrimPrefix(raw, "chat_id:"), larkim.ReceiveIdTypeChatId, nil
	}
	return raw, larkim.ReceiveIdTypeOpenId, nil
}
