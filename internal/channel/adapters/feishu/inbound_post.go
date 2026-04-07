package feishu

import (
	"fmt"
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

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
					mime := strings.TrimSpace(stringValue(part["mime_type"]))
					result = append(result, channel.NormalizeInboundChannelAttachment(channel.Attachment{
						Type:           channel.AttachmentImage,
						PlatformKey:    strings.TrimSpace(key),
						SourcePlatform: Type.String(),
						Mime:           mime,
						Metadata:       map[string]any{"message_id": messageID},
					}))
				}
			}
			if tag == "file" {
				if key, ok := part["file_key"].(string); ok && strings.TrimSpace(key) != "" {
					name := strings.TrimSpace(stringValue(part["file_name"]))
					mime := strings.TrimSpace(stringValue(part["mime_type"]))
					result = append(result, channel.NormalizeInboundChannelAttachment(channel.Attachment{
						Type:           channel.AttachmentFile,
						PlatformKey:    strings.TrimSpace(key),
						SourcePlatform: Type.String(),
						Name:           name,
						Mime:           mime,
						Metadata:       map[string]any{"message_id": messageID},
					}))
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
