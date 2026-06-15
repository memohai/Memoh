package telegram

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/memohai/memoh/internal/channel"
)

func TestExtractTelegramMessageParts_PlainTextNoEntities(t *testing.T) {
	t.Parallel()
	msg := &tgbotapi.Message{Text: "hello world"}
	parts := extractTelegramMessageParts(msg)
	if len(parts) != 0 {
		t.Fatalf("expected no parts for plain text, got %+v", parts)
	}
}

func TestExtractTelegramMessageParts_BoldSpanInMiddle(t *testing.T) {
	t.Parallel()
	msg := &tgbotapi.Message{
		Text: "hi shout bye",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bold", Offset: 3, Length: 5},
		},
	}
	parts := extractTelegramMessageParts(msg)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts (text, bold, text), got %d: %+v", len(parts), parts)
	}
	if parts[0].Type != channel.MessagePartText || parts[0].Text != "hi " {
		t.Fatalf("part 0 wrong: %+v", parts[0])
	}
	if parts[1].Type != channel.MessagePartText || parts[1].Text != "shout" || len(parts[1].Styles) != 1 || parts[1].Styles[0] != channel.MessageStyleBold {
		t.Fatalf("part 1 wrong: %+v", parts[1])
	}
	if parts[2].Type != channel.MessagePartText || parts[2].Text != " bye" {
		t.Fatalf("part 2 wrong: %+v", parts[2])
	}
}

func TestExtractTelegramMessageParts_ItalicStrikeCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		entity string
		style  channel.MessageTextStyle
	}{
		{"italic", "italic", channel.MessageStyleItalic},
		{"strikethrough", "strikethrough", channel.MessageStyleStrikethrough},
		{"inline_code", "code", channel.MessageStyleCode},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			msg := &tgbotapi.Message{
				Text: "xx",
				Entities: []tgbotapi.MessageEntity{
					{Type: tc.entity, Offset: 0, Length: 2},
				},
			}
			parts := extractTelegramMessageParts(msg)
			if len(parts) != 1 || parts[0].Type != channel.MessagePartText {
				t.Fatalf("expected single text part, got %+v", parts)
			}
			if len(parts[0].Styles) != 1 || parts[0].Styles[0] != tc.style {
				t.Fatalf("expected style %q, got %+v", tc.style, parts[0].Styles)
			}
		})
	}
}

func TestExtractTelegramMessageParts_CodeBlockWithLanguage(t *testing.T) {
	t.Parallel()
	msg := &tgbotapi.Message{
		Text: "fn main(){}",
		Entities: []tgbotapi.MessageEntity{
			{Type: "pre", Offset: 0, Length: 11, Language: "rust"},
		},
	}
	parts := extractTelegramMessageParts(msg)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %+v", parts)
	}
	if parts[0].Type != channel.MessagePartCodeBlock || parts[0].Language != "rust" || parts[0].Text != "fn main(){}" {
		t.Fatalf("expected pre with language=rust, got %+v", parts[0])
	}
}

func TestExtractTelegramMessageParts_TextLinkWithURL(t *testing.T) {
	t.Parallel()
	msg := &tgbotapi.Message{
		Text: "see Memoh now",
		Entities: []tgbotapi.MessageEntity{
			{Type: "text_link", Offset: 4, Length: 5, URL: "https://example.com"},
		},
	}
	parts := extractTelegramMessageParts(msg)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %+v", parts)
	}
	if parts[1].Type != channel.MessagePartLink || parts[1].URL != "https://example.com" || parts[1].Text != "Memoh" {
		t.Fatalf("link part wrong: %+v", parts[1])
	}
}

func TestExtractTelegramMessageParts_BareURLEntity(t *testing.T) {
	t.Parallel()
	msg := &tgbotapi.Message{
		Text: "go https://example.com",
		Entities: []tgbotapi.MessageEntity{
			{Type: "url", Offset: 3, Length: 19},
		},
	}
	parts := extractTelegramMessageParts(msg)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %+v", parts)
	}
	if parts[1].Type != channel.MessagePartLink || parts[1].URL != "https://example.com" || parts[1].Text != "https://example.com" {
		t.Fatalf("url part wrong: %+v", parts[1])
	}
}

func TestExtractTelegramMessageParts_MentionPreserved(t *testing.T) {
	t.Parallel()
	msg := &tgbotapi.Message{
		Text: "hi @bot ok",
		Entities: []tgbotapi.MessageEntity{
			{Type: "mention", Offset: 3, Length: 4},
		},
	}
	parts := extractTelegramMessageParts(msg)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %+v", parts)
	}
	if parts[1].Type != channel.MessagePartMention || parts[1].Text != "@bot" {
		t.Fatalf("mention part wrong: %+v", parts[1])
	}
}

func TestExtractTelegramMessageParts_TextMentionCarriesUserMetadata(t *testing.T) {
	t.Parallel()
	msg := &tgbotapi.Message{
		Text: "hi Alice",
		Entities: []tgbotapi.MessageEntity{
			{
				Type:   "text_mention",
				Offset: 3,
				Length: 5,
				User:   &tgbotapi.User{ID: 7, FirstName: "Alice", UserName: "ali"},
			},
		},
	}
	parts := extractTelegramMessageParts(msg)
	if len(parts) < 2 {
		t.Fatalf("expected at least 2 parts, got %+v", parts)
	}
	mention := parts[len(parts)-1]
	if mention.Type != channel.MessagePartMention {
		t.Fatalf("expected mention, got %+v", mention)
	}
	// The visible text the sender wrote (the entity slice) is preserved; the
	// user identity is surfaced through Metadata + ChannelIdentityID rather
	// than overwriting the displayed label.
	if mention.Text != "Alice" {
		t.Fatalf("expected slice 'Alice' preserved, got %q", mention.Text)
	}
	if mention.ChannelIdentityID != "7" {
		t.Fatalf("expected channel_identity_id=7, got %q", mention.ChannelIdentityID)
	}
	if got := mention.Metadata["user_id"]; got != "7" {
		t.Fatalf("expected user_id=7, got %v", got)
	}
	if got := mention.Metadata["username"]; got != "ali" {
		t.Fatalf("expected username=ali, got %v", got)
	}
}

func TestExtractTelegramMessageParts_TextMentionPreservesSenderLabel(t *testing.T) {
	t.Parallel()
	// A text_mention can anchor a profile link onto an arbitrary label such as
	// "the reviewer". The displayed slice is what the LLM should see, not the
	// linked profile's first name.
	msg := &tgbotapi.Message{
		Text: "ask the reviewer please",
		Entities: []tgbotapi.MessageEntity{
			{
				Type:   "text_mention",
				Offset: 4,
				Length: 12,
				User:   &tgbotapi.User{ID: 42, FirstName: "Alice", UserName: "ali"},
			},
		},
	}
	parts := extractTelegramMessageParts(msg)
	var mention *channel.MessagePart
	for i := range parts {
		if parts[i].Type == channel.MessagePartMention {
			mention = &parts[i]
			break
		}
	}
	if mention == nil {
		t.Fatalf("expected mention, got %+v", parts)
	}
	if mention.Text != "the reviewer" {
		t.Fatalf("expected sender-typed label preserved, got %q", mention.Text)
	}
	if got := mention.Metadata["user_id"]; got != "42" {
		t.Fatalf("expected user_id=42, got %v", got)
	}
}

func TestExtractTelegramMessageParts_UnsupportedEntityKeepsTextAsPlain(t *testing.T) {
	t.Parallel()
	msg := &tgbotapi.Message{
		Text: "see #news today",
		Entities: []tgbotapi.MessageEntity{
			{Type: "hashtag", Offset: 4, Length: 5},
		},
	}
	parts := extractTelegramMessageParts(msg)
	if len(parts) != 0 {
		t.Fatalf("expected no parts when only hashtag entity (whole text plain), got %+v", parts)
	}
}

func TestExtractTelegramMessageParts_OverlappingEntityDropped(t *testing.T) {
	t.Parallel()
	msg := &tgbotapi.Message{
		Text: "bold italic",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bold", Offset: 0, Length: 11},
			{Type: "italic", Offset: 5, Length: 6},
		},
	}
	parts := extractTelegramMessageParts(msg)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part (outer bold), got %+v", parts)
	}
	if parts[0].Type != channel.MessagePartText || parts[0].Text != "bold italic" || len(parts[0].Styles) != 1 || parts[0].Styles[0] != channel.MessageStyleBold {
		t.Fatalf("expected single bold-styled text, got %+v", parts[0])
	}
}

func TestExtractTelegramMessageParts_UsesCaptionAndCaptionEntities(t *testing.T) {
	t.Parallel()
	msg := &tgbotapi.Message{
		Caption: "x y",
		CaptionEntities: []tgbotapi.MessageEntity{
			{Type: "bold", Offset: 0, Length: 1},
		},
	}
	parts := extractTelegramMessageParts(msg)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts from caption, got %+v", parts)
	}
	if parts[0].Type != channel.MessagePartText || parts[0].Text != "x" || len(parts[0].Styles) != 1 {
		t.Fatalf("part 0 wrong: %+v", parts[0])
	}
}

func TestExtractTelegramMessageParts_HonorsUTF16OffsetsForBMP(t *testing.T) {
	t.Parallel()
	// CJK characters live in the BMP, so one UTF-16 code unit equals one rune;
	// the offsets line up under either indexing strategy. The supplementary-plane
	// case is covered separately below.
	msg := &tgbotapi.Message{
		Text: "你好 world",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bold", Offset: 3, Length: 5},
		},
	}
	parts := extractTelegramMessageParts(msg)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %+v", parts)
	}
	if parts[0].Text != "你好 " {
		t.Fatalf("expected leading CJK text, got %q", parts[0].Text)
	}
	if parts[1].Text != "world" || len(parts[1].Styles) != 1 {
		t.Fatalf("expected bold 'world', got %+v", parts[1])
	}
}

func TestExtractTelegramMessageParts_HandlesSupplementaryPlaneEmoji(t *testing.T) {
	t.Parallel()
	// 🎉 (U+1F389) is encoded as a UTF-16 surrogate pair (2 code units) but is a
	// single rune in Go. Telegram entity offsets are documented as UTF-16 code
	// units; rune-based slicing would drift by 1 after each supplementary-plane
	// character.
	msg := &tgbotapi.Message{
		Text: "see 🎉 bold here",
		Entities: []tgbotapi.MessageEntity{
			// "bold" begins at UTF-16 index 7: "see "(0-3) + 🎉(4-5) + " "(6) + "bold"(7-10).
			{Type: "bold", Offset: 7, Length: 4},
		},
	}
	parts := extractTelegramMessageParts(msg)
	var bold *channel.MessagePart
	for i := range parts {
		if len(parts[i].Styles) == 1 && parts[i].Styles[0] == channel.MessageStyleBold {
			bold = &parts[i]
			break
		}
	}
	if bold == nil {
		t.Fatalf("expected a bold part, got %+v", parts)
	}
	if bold.Text != "bold" {
		t.Fatalf("expected bold text 'bold' under UTF-16 offsets, got %q (offsets interpreted as runes drift past the surrogate pair)", bold.Text)
	}
}

func TestExtractTelegramMessageParts_NilMessage(t *testing.T) {
	t.Parallel()
	if got := extractTelegramMessageParts(nil); got != nil {
		t.Fatalf("expected nil for nil msg, got %+v", got)
	}
}
