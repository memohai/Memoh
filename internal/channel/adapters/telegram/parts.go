package telegram

import (
	"sort"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/memohai/memoh/internal/channel"
)

// extractTelegramMessageParts converts Telegram entities into channel.MessagePart
// slices, interleaving styled spans with the surrounding plain text. Returns nil
// when the resulting slice would carry no rich information (so callers can fall
// back to the plain Text field).
func extractTelegramMessageParts(msg *tgbotapi.Message) []channel.MessagePart {
	if msg == nil {
		return nil
	}
	text := msg.Text
	entities := msg.Entities
	if text == "" {
		text = msg.Caption
		entities = msg.CaptionEntities
	}
	if text == "" || len(entities) == 0 {
		return nil
	}

	sorted := append([]tgbotapi.MessageEntity{}, entities...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Offset != sorted[j].Offset {
			return sorted[i].Offset < sorted[j].Offset
		}
		return sorted[i].Length > sorted[j].Length
	})

	runes := []rune(text)
	n := len(runes)
	var parts []channel.MessagePart
	appendPlain := func(s string) {
		if s == "" {
			return
		}
		parts = append(parts, channel.MessagePart{Type: channel.MessagePartText, Text: s})
	}

	cursor := 0
	for _, ent := range sorted {
		if ent.Offset < 0 || ent.Length <= 0 || ent.Offset+ent.Length > n {
			continue
		}
		if ent.Offset < cursor {
			continue
		}
		if ent.Offset > cursor {
			appendPlain(string(runes[cursor:ent.Offset]))
		}
		slice := string(runes[ent.Offset : ent.Offset+ent.Length])
		if part, ok := telegramEntityToPart(ent, slice); ok {
			parts = append(parts, part)
		} else {
			appendPlain(slice)
		}
		cursor = ent.Offset + ent.Length
	}
	if cursor < n {
		appendPlain(string(runes[cursor:]))
	}

	if onlyPlainText(parts) {
		return nil
	}
	return parts
}

func telegramEntityToPart(ent tgbotapi.MessageEntity, slice string) (channel.MessagePart, bool) {
	switch ent.Type {
	case "bold":
		return styledText(slice, channel.MessageStyleBold), true
	case "italic":
		return styledText(slice, channel.MessageStyleItalic), true
	case "strikethrough":
		return styledText(slice, channel.MessageStyleStrikethrough), true
	case "code":
		return styledText(slice, channel.MessageStyleCode), true
	case "pre":
		return channel.MessagePart{
			Type:     channel.MessagePartCodeBlock,
			Text:     slice,
			Language: strings.TrimSpace(ent.Language),
		}, true
	case "text_link":
		return channel.MessagePart{
			Type: channel.MessagePartLink,
			Text: slice,
			URL:  strings.TrimSpace(ent.URL),
		}, true
	case "url":
		return channel.MessagePart{
			Type: channel.MessagePartLink,
			Text: slice,
			URL:  slice,
		}, true
	case "mention":
		return channel.MessagePart{Type: channel.MessagePartMention, Text: slice}, true
	case "text_mention":
		if ent.User == nil {
			return channel.MessagePart{}, false
		}
		name := strings.TrimSpace(ent.User.FirstName + " " + ent.User.LastName)
		if name == "" {
			name = ent.User.UserName
		}
		meta := map[string]any{
			"user_id": strconv.FormatInt(ent.User.ID, 10),
		}
		if ent.User.UserName != "" {
			meta["username"] = ent.User.UserName
		}
		return channel.MessagePart{
			Type:     channel.MessagePartMention,
			Text:     "@" + name,
			Metadata: meta,
		}, true
	default:
		return channel.MessagePart{}, false
	}
}

func styledText(text string, style channel.MessageTextStyle) channel.MessagePart {
	return channel.MessagePart{
		Type:   channel.MessagePartText,
		Text:   text,
		Styles: []channel.MessageTextStyle{style},
	}
}

func onlyPlainText(parts []channel.MessagePart) bool {
	for _, p := range parts {
		if p.Type != channel.MessagePartText || len(p.Styles) > 0 {
			return false
		}
	}
	return true
}
