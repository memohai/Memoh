package telegram

import (
	"encoding/json"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/memohai/memoh/internal/channel"
)

type telegramInputRichMessage struct {
	HTML                string `json:"html,omitempty"`
	SkipEntityDetection bool   `json:"skip_entity_detection,omitempty"`
}

func writeTelegramRichParagraph(b *strings.Builder, html string) {
	html = strings.TrimSpace(html)
	if html == "" {
		return
	}
	b.WriteString("<p>")
	b.WriteString(html)
	b.WriteString("</p>")
}

func telegramEscapeAttr(value string) string {
	return strings.ReplaceAll(telegramEscapeHTML(value), `"`, "&quot;")
}

func isAllowedTelegramRichHref(href string) bool {
	href = strings.TrimSpace(href)
	return strings.HasPrefix(href, "https://") ||
		strings.HasPrefix(href, "http://") ||
		strings.HasPrefix(href, "mailto:") ||
		strings.HasPrefix(href, "tel:") ||
		strings.HasPrefix(href, "tg://user?id=") ||
		strings.HasPrefix(href, "#")
}

func sendTelegramRichMessageReturnMessage(
	bot *tgbotapi.BotAPI,
	target string,
	rich telegramInputRichMessage,
	replyTo int,
	actions []channel.Action,
) (chatID int64, messageID int, err error) {
	if strings.TrimSpace(rich.HTML) == "" {
		return 0, 0, nil
	}
	parsedChatID, channelUsername, parseErr := parseTelegramTarget(target)
	if parseErr != nil {
		return 0, 0, parseErr
	}
	params := tgbotapi.Params{}
	if channelUsername != "" {
		params.AddNonEmpty("chat_id", channelUsername)
	} else {
		params.AddNonEmpty("chat_id", strconv.FormatInt(parsedChatID, 10))
	}
	if err := params.AddInterface("rich_message", rich); err != nil {
		return 0, 0, err
	}
	if replyTo > 0 {
		if err := params.AddInterface("reply_parameters", map[string]any{"message_id": replyTo}); err != nil {
			return 0, 0, err
		}
	}
	markup := telegramInlineKeyboard(actions)
	if len(markup.InlineKeyboard) > 0 {
		if err := params.AddInterface("reply_markup", markup); err != nil {
			return 0, 0, err
		}
	}
	resp, err := bot.MakeRequest("sendRichMessage", params)
	if err != nil {
		return 0, 0, err
	}
	var sent tgbotapi.Message
	if err := json.Unmarshal(resp.Result, &sent); err != nil {
		return 0, 0, err
	}
	chatID = parsedChatID
	if sent.Chat != nil {
		chatID = sent.Chat.ID
	}
	return chatID, sent.MessageID, nil
}

func editTelegramRichMessage(
	bot *tgbotapi.BotAPI,
	chatID int64,
	messageID int,
	rich telegramInputRichMessage,
	actions []channel.Action,
) error {
	if strings.TrimSpace(rich.HTML) == "" {
		return nil
	}
	params := tgbotapi.Params{}
	params.AddNonEmpty("chat_id", strconv.FormatInt(chatID, 10))
	params.AddNonZero("message_id", messageID)
	if err := params.AddInterface("rich_message", rich); err != nil {
		return err
	}
	markup := telegramInlineKeyboard(actions)
	if len(markup.InlineKeyboard) > 0 {
		if err := params.AddInterface("reply_markup", markup); err != nil {
			return err
		}
	}
	_, err := bot.MakeRequest("editMessageText", params)
	if err != nil && (isTelegramMessageNotModified(err) || isTelegramEditUnrecoverable(err)) {
		return nil
	}
	return err
}
