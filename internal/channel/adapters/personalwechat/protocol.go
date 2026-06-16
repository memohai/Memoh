package personalwechat

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

type bridgeEnvelope struct {
	Type    string          `json:"type"`
	Message *bridgeMessage  `json:"message,omitempty"`
	Status  string          `json:"status,omitempty"`
	Error   string          `json:"error,omitempty"`
	Raw     json.RawMessage `json:"raw,omitempty"`
}

type bridgeMessage struct {
	ID           string             `json:"id"`
	Type         string             `json:"type,omitempty"`
	Text         string             `json:"text,omitempty"`
	Timestamp    string             `json:"timestamp,omitempty"`
	ReplyTarget  string             `json:"replyTarget,omitempty"`
	Sender       bridgeIdentity     `json:"sender"`
	Conversation bridgeConversation `json:"conversation"`
	Reply        *bridgeReply       `json:"reply,omitempty"`
	Attachments  []bridgeAttachment `json:"attachments,omitempty"`
	Raw          map[string]any     `json:"raw,omitempty"`
}

type bridgeIdentity struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Alias       string `json:"alias,omitempty"`
	Remark      string `json:"remark,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Self        bool   `json:"self,omitempty"`
}

type bridgeConversation struct {
	ID   string         `json:"id,omitempty"`
	Type string         `json:"type,omitempty"`
	Name string         `json:"name,omitempty"`
	Raw  map[string]any `json:"raw,omitempty"`
}

type bridgeReply struct {
	Target      string             `json:"target,omitempty"`
	MessageID   string             `json:"messageId,omitempty"`
	Sender      string             `json:"sender,omitempty"`
	Preview     string             `json:"preview,omitempty"`
	Attachments []bridgeAttachment `json:"attachments,omitempty"`
	Raw         map[string]any     `json:"raw,omitempty"`
}

type bridgeAttachment struct {
	Type     string         `json:"type,omitempty"`
	Path     string         `json:"path,omitempty"`
	URL      string         `json:"url,omitempty"`
	Base64   string         `json:"base64,omitempty"`
	Mime     string         `json:"mime,omitempty"`
	Name     string         `json:"name,omitempty"`
	Size     int64          `json:"size,omitempty"`
	Width    int            `json:"width,omitempty"`
	Height   int            `json:"height,omitempty"`
	Variant  string         `json:"variant,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type bridgeSendCommand struct {
	Type    string            `json:"type"`
	Target  string            `json:"target"`
	Message bridgeSendMessage `json:"message"`
}

type bridgeSendMessage struct {
	Text        string             `json:"text,omitempty"`
	Reply       *channel.ReplyRef  `json:"reply,omitempty"`
	Format      string             `json:"format,omitempty"`
	Attachments []bridgeAttachment `json:"attachments,omitempty"`
}

type bridgeStopCommand struct {
	Type string `json:"type"`
}

func buildInboundMessage(msg bridgeMessage) (channel.InboundMessage, bool) {
	if strings.TrimSpace(msg.ID) == "" {
		return channel.InboundMessage{}, false
	}
	senderID := strings.TrimSpace(msg.Sender.ID)
	if senderID == "" {
		senderID = strings.TrimSpace(msg.Sender.Name)
	}
	convID := strings.TrimSpace(msg.Conversation.ID)
	if convID == "" {
		convID = senderID
	}
	if senderID == "" || convID == "" {
		return channel.InboundMessage{}, false
	}
	attachments := mapAttachments(msg.Attachments)
	reply := mapReply(msg.Reply)
	if strings.TrimSpace(msg.Text) == "" && len(attachments) == 0 && reply == nil {
		return channel.InboundMessage{}, false
	}
	convType := channel.NormalizeConversationType(msg.Conversation.Type)
	target := strings.TrimSpace(msg.ReplyTarget)
	if target == "" {
		if convType == channel.ConversationTypeGroup {
			target = "room:" + convID
		} else {
			target = "contact:" + senderID
		}
	}
	receivedAt := time.Now().UTC()
	if ts := strings.TrimSpace(msg.Timestamp); ts != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			receivedAt = parsed.UTC()
		}
	}
	meta := map[string]any{
		"wechat_type": strings.TrimSpace(msg.Type),
		"target":      target,
	}
	if len(msg.Raw) > 0 {
		meta["raw"] = msg.Raw
	}
	if len(msg.Conversation.Raw) > 0 {
		meta["conversation_raw"] = msg.Conversation.Raw
	}
	return channel.InboundMessage{
		Channel: Type,
		Message: channel.Message{
			ID:          strings.TrimSpace(msg.ID),
			Format:      channel.MessageFormatPlain,
			Text:        msg.Text,
			Reply:       reply,
			Attachments: attachments,
			Metadata:    meta,
		},
		ReplyTarget: target,
		Sender: channel.Identity{
			SubjectID:   senderID,
			DisplayName: displayName(msg.Sender),
			Attributes: map[string]string{
				"user_id":      senderID,
				"target":       "contact:" + senderID,
				"name":         strings.TrimSpace(msg.Sender.Name),
				"alias":        strings.TrimSpace(msg.Sender.Alias),
				"remark":       strings.TrimSpace(msg.Sender.Remark),
				"display_name": displayName(msg.Sender),
			},
		},
		Conversation: channel.Conversation{
			ID:   convID,
			Type: convType,
			Name: strings.TrimSpace(msg.Conversation.Name),
			Metadata: map[string]any{
				"target": target,
			},
		},
		ReceivedAt: receivedAt,
		Source:     Type.String(),
		Metadata:   meta,
	}, true
}

func displayName(id bridgeIdentity) string {
	for _, value := range []string{id.DisplayName, id.Remark, id.Alias, id.Name, id.ID} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mapReply(reply *bridgeReply) *channel.ReplyRef {
	if reply == nil {
		return nil
	}
	ref := &channel.ReplyRef{
		Target:      strings.TrimSpace(reply.Target),
		MessageID:   strings.TrimSpace(reply.MessageID),
		Sender:      strings.TrimSpace(reply.Sender),
		Preview:     strings.TrimSpace(reply.Preview),
		Attachments: mapAttachments(reply.Attachments),
	}
	if ref.Target == "" && ref.MessageID == "" && ref.Sender == "" && ref.Preview == "" && len(ref.Attachments) == 0 {
		return nil
	}
	return ref
}

func mapAttachments(items []bridgeAttachment) []channel.Attachment {
	if len(items) == 0 {
		return nil
	}
	out := make([]channel.Attachment, 0, len(items))
	for _, item := range items {
		attType := mapAttachmentType(item.Type)
		att := channel.Attachment{
			Type:           attType,
			Path:           strings.TrimSpace(item.Path),
			URL:            strings.TrimSpace(item.URL),
			Base64:         strings.TrimSpace(item.Base64),
			Mime:           strings.TrimSpace(item.Mime),
			Name:           strings.TrimSpace(item.Name),
			Size:           item.Size,
			Width:          item.Width,
			Height:         item.Height,
			SourcePlatform: Type.String(),
			Metadata:       map[string]any{},
		}
		for key, value := range item.Metadata {
			att.Metadata[key] = value
		}
		if variant := strings.TrimSpace(item.Variant); variant != "" {
			att.Metadata["variant"] = variant
		}
		if !att.HasReference() {
			continue
		}
		out = append(out, att)
	}
	return out
}

func mapAttachmentType(value string) channel.AttachmentType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image", "photo":
		return channel.AttachmentImage
	case "gif", "emoticon":
		return channel.AttachmentGIF
	case "audio":
		return channel.AttachmentAudio
	case "voice":
		return channel.AttachmentVoice
	case "video":
		return channel.AttachmentVideo
	default:
		return channel.AttachmentFile
	}
}
