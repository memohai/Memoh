package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/memohai/memoh/internal/channel"
)

const feishuQuotedTextMaxLength = 200

// enrichQuotedMessage fetches the parent message via API and prepends a
// quoted-text summary to the inbound message so the AI can see what is
// being replied to. It also sets the "is_reply_to_bot" metadata flag.
func (a *FeishuAdapter) enrichQuotedMessage(ctx context.Context, cfg channel.ChannelConfig, msg *channel.InboundMessage, botOpenID string) {
	if msg == nil || msg.Message.Reply == nil {
		return
	}
	parentID := strings.TrimSpace(msg.Message.Reply.MessageID)
	if parentID == "" {
		return
	}

	feishuCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	client := lark.NewClient(feishuCfg.AppID, feishuCfg.AppSecret, lark.WithOpenBaseUrl(feishuCfg.openBaseURL()))
	resp, err := client.Im.Message.Get(lookupCtx, larkim.NewGetMessageReqBuilder().MessageId(parentID).Build())
	if err != nil {
		if a.logger != nil {
			a.logger.Debug("feishu quoted message fetch failed",
				slog.String("parent_id", parentID),
				slog.Any("error", err),
			)
		}
		return
	}
	if resp == nil || !resp.Success() || resp.Data == nil || len(resp.Data.Items) == 0 {
		if a.logger != nil {
			code, respMsg := 0, ""
			if resp != nil {
				code = resp.Code
				respMsg = resp.Msg
			}
			a.logger.Debug("feishu quoted message fetch empty",
				slog.String("parent_id", parentID),
				slog.Int("code", code),
				slog.String("response_msg", respMsg),
			)
		}
		return
	}

	parent := resp.Data.Items[0]

	// Determine if the parent message is from the bot itself.
	isReplyToBot := false
	senderName := ""
	if parent.Sender != nil {
		senderType := ptrStr(parent.Sender.SenderType)
		senderID := ptrStr(parent.Sender.Id)
		if senderType == "app" {
			// When botOpenID is known, match precisely; otherwise any app sender counts.
			isReplyToBot = strings.TrimSpace(botOpenID) == "" || senderID == strings.TrimSpace(botOpenID)
		}
	}
	if msg.Metadata == nil {
		msg.Metadata = map[string]any{}
	}
	msg.Metadata["is_reply_to_bot"] = isReplyToBot

	// Extract text content from the parent message.
	text := extractFeishuMessageText(parent)
	if text == "" {
		msgType := ptrStr(parent.MsgType)
		if msgType != "" && msgType != larkim.MsgTypeText {
			text = "[" + msgType + "]"
		}
	}
	if text == "" {
		return
	}
	if len([]rune(text)) > feishuQuotedTextMaxLength {
		text = string([]rune(text)[:feishuQuotedTextMaxLength]) + "..."
	}

	var quotedText string
	if senderName != "" {
		quotedText = fmt.Sprintf("[Reply to %s: %s]", senderName, text)
	} else {
		quotedText = fmt.Sprintf("[Reply to: %s]", text)
	}

	current := strings.TrimSpace(msg.Message.Text)
	if current != "" {
		msg.Message.Text = quotedText + "\n" + current
	} else {
		msg.Message.Text = quotedText
	}
}

// extractFeishuMessageText extracts plain text from a Feishu message object
// returned by the Get Message API.
func extractFeishuMessageText(msg *larkim.Message) string {
	if msg == nil || msg.Body == nil || msg.Body.Content == nil {
		return ""
	}
	content := strings.TrimSpace(*msg.Body.Content)
	if content == "" {
		return ""
	}

	var contentMap map[string]any
	if err := json.Unmarshal([]byte(content), &contentMap); err != nil {
		return ""
	}

	msgType := ptrStr(msg.MsgType)
	switch msgType {
	case larkim.MsgTypeText:
		if txt, ok := contentMap["text"].(string); ok {
			return strings.TrimSpace(txt)
		}
	case larkim.MsgTypePost:
		return extractFeishuPostText(contentMap)
	}

	// Fallback: try "text" key for unknown types.
	if txt, ok := contentMap["text"].(string); ok {
		return strings.TrimSpace(txt)
	}
	return ""
}
