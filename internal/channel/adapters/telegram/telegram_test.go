package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tele "gopkg.in/telebot.v4"

	"github.com/memohai/memoh/internal/channel"
)

// newStubTelegramBot builds a telebot bot suitable for tests that need a
// concrete *tele.Bot (e.g. for Me access) without making network calls. The
// URL points at a closed local port so any accidental API call fails fast
// instead of hanging.
func newStubTelegramBot(t *testing.T) *tele.Bot {
	t.Helper()
	bot, err := tele.NewBot(tele.Settings{
		Token:   "test",
		URL:     "http://127.0.0.1:1",
		Offline: true,
	})
	if err != nil {
		t.Fatalf("create stub telegram bot: %v", err)
	}
	return bot
}

func TestResolveTelegramSender(t *testing.T) {
	t.Parallel()

	externalID, displayName, attrs := resolveTelegramSender(nil)
	if externalID != "" || displayName != "" || len(attrs) != 0 {
		t.Fatalf("expected empty sender")
	}
	msg := &tele.Message{
		Sender: &tele.User{ID: 123, Username: "alice"},
	}
	externalID, displayName, attrs = resolveTelegramSender(msg)
	if externalID != "123" || displayName != "@alice" {
		t.Fatalf("unexpected sender: %s %s", externalID, displayName)
	}
	if attrs["user_id"] != "123" || attrs["username"] != "alice" {
		t.Fatalf("unexpected attrs: %#v", attrs)
	}
}

func TestIsTelegramBotMentioned(t *testing.T) {
	t.Parallel()

	t.Run("text mention", func(t *testing.T) {
		t.Parallel()
		msg := &tele.Message{
			Text: "hello @MemohBot",
		}
		if !isTelegramBotMentioned(msg, "memohbot") {
			t.Fatalf("expected bot mention from text")
		}
	})

	t.Run("entity text mention matching bot", func(t *testing.T) {
		t.Parallel()
		msg := &tele.Message{
			Entities: []tele.MessageEntity{
				{
					Type: tele.EntityTMention,
					User: &tele.User{IsBot: true, Username: "memohbot"},
				},
			},
		}
		if !isTelegramBotMentioned(msg, "memohbot") {
			t.Fatalf("expected bot mention from text_mention entity")
		}
	})

	t.Run("entity text mention other bot", func(t *testing.T) {
		t.Parallel()
		msg := &tele.Message{
			Entities: []tele.MessageEntity{
				{
					Type: tele.EntityTMention,
					User: &tele.User{IsBot: true, Username: "otherbot"},
				},
			},
		}
		if isTelegramBotMentioned(msg, "memohbot") {
			t.Fatalf("expected no mention for different bot")
		}
	})

	t.Run("not mentioned", func(t *testing.T) {
		t.Parallel()
		msg := &tele.Message{
			Text: "hello everyone",
		}
		if isTelegramBotMentioned(msg, "memohbot") {
			t.Fatalf("expected no mention")
		}
	})
}

func TestTelegramDescriptorIncludesStreaming(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	caps := adapter.Descriptor().Capabilities
	if !caps.Streaming {
		t.Fatal("expected streaming capability")
	}
	if !caps.Media {
		t.Fatal("expected media capability")
	}
}

func TestBuildTelegramAttachmentIncludesPlatformReference(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	att := adapter.buildTelegramAttachment(nil, channel.AttachmentFile, "file_1", "doc.txt", "text/plain", 10)
	if att.PlatformKey != "file_1" {
		t.Fatalf("unexpected platform key: %s", att.PlatformKey)
	}
	if att.SourcePlatform != Type.String() {
		t.Fatalf("unexpected source platform: %s", att.SourcePlatform)
	}
}

func TestBuildTelegramAttachmentInfersTypeFromMime(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	att := adapter.buildTelegramAttachment(nil, channel.AttachmentFile, "file_2", "photo.jpg", "IMAGE/JPEG; charset=utf-8", 10)
	if att.Type != channel.AttachmentImage {
		t.Fatalf("expected image type, got: %s", att.Type)
	}
	if att.Mime != "image/jpeg" {
		t.Fatalf("expected normalized mime image/jpeg, got: %s", att.Mime)
	}
}

func TestTelegramResolveAttachmentRequiresReference(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	_, err := adapter.ResolveAttachment(context.Background(), channel.ChannelConfig{}, channel.Attachment{})
	if err == nil {
		t.Fatal("expected error when attachment has no platform_key/url")
	}
	if !strings.Contains(err.Error(), "platform_key") {
		t.Fatalf("expected platform_key error, got: %v", err)
	}
}

func TestParseReplyToMessageID(t *testing.T) {
	t.Parallel()

	if got := parseReplyToMessageID(nil); got != 0 {
		t.Fatalf("nil reply should return 0: %d", got)
	}
	if got := parseReplyToMessageID(&channel.ReplyRef{}); got != 0 {
		t.Fatalf("empty MessageID should return 0: %d", got)
	}
	if got := parseReplyToMessageID(&channel.ReplyRef{MessageID: "  123  "}); got != 123 {
		t.Fatalf("expected 123: %d", got)
	}
	if got := parseReplyToMessageID(&channel.ReplyRef{MessageID: "abc"}); got != 0 {
		t.Fatalf("invalid number should return 0: %d", got)
	}
}

func TestBuildTelegramReplyRef(t *testing.T) {
	t.Parallel()

	if buildTelegramReplyRef(nil, "123") != nil {
		t.Fatal("nil msg should return nil")
	}
	msg := &tele.Message{}
	if buildTelegramReplyRef(msg, "123") != nil {
		t.Fatal("msg without ReplyToMessage should return nil")
	}
	msg.ReplyTo = &tele.Message{ID: 42}
	ref := buildTelegramReplyRef(msg, "  -100  ")
	if ref == nil {
		t.Fatal("expected non-nil ref")
		return
	}
	if ref.MessageID != "42" || ref.Target != "-100" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
}

func TestBuildTelegramForwardRefFromChannel(t *testing.T) {
	t.Parallel()

	ref := buildTelegramForwardRef(&tele.Message{
		OriginalChat:      &tele.Chat{ID: -10001, Title: "Source Channel", Username: "source_channel"},
		OriginalMessageID: 99,
		OriginalUnixtime:  1710000000,
	})
	if ref == nil {
		t.Fatal("expected forward ref")
		return
	}
	if ref.MessageID != "99" || ref.FromConversationID != "-10001" {
		t.Fatalf("unexpected forward ids: %+v", ref)
	}
	if ref.Sender != "Source Channel (@source_channel)" || ref.Date != 1710000000 {
		t.Fatalf("unexpected forward metadata: %+v", ref)
	}
}

func TestTelegramInboundKeepsForwardOutOfText(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	inbound, ok := adapter.toInboundTelegramMessage(nil, channel.ChannelConfig{}, &tele.Message{
		ID:                10,
		Unixtime:          1710000000,
		Chat:              &tele.Chat{ID: -10001, Type: tele.ChatGroup, Title: "Test Group"},
		Sender:            &tele.User{ID: 42, Username: "sender"},
		Text:              "forwarded body",
		OriginalChat:      &tele.Chat{ID: -10002, Title: "Source Channel"},
		OriginalMessageID: 11,
	}, "forwarded body", nil, nil)
	if !ok {
		t.Fatal("expected inbound message")
	}
	if inbound.Message.Text != "forwarded body" {
		t.Fatalf("expected original text without forward prefix, got %q", inbound.Message.Text)
	}
	if inbound.Message.Forward == nil || inbound.Message.Forward.MessageID != "11" {
		t.Fatalf("expected structured forward ref, got %+v", inbound.Message.Forward)
	}
}

func TestBuildTelegramMediaGroupInboundMessageAggregatesAttachments(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	bot := newStubTelegramBot(t)
	bot.Me = &tele.User{ID: 1001, Username: "memohbot"}
	cfg := channel.ChannelConfig{}
	first := &tele.Message{
		ID:       101,
		AlbumID:  "group-1",
		Unixtime: 1710000000,
		Chat:     &tele.Chat{ID: -10001, Type: tele.ChatGroup, Title: "G1"},
		Sender:   &tele.User{ID: 10, Username: "alice"},
		Photo:    &tele.Photo{File: tele.File{FileID: "photo-1", FileSize: 10}, Width: 320, Height: 240},
	}
	second := &tele.Message{
		ID:       102,
		AlbumID:  "group-1",
		Unixtime: 1710000001,
		Chat:     &tele.Chat{ID: -10001, Type: tele.ChatGroup, Title: "G1"},
		Sender:   &tele.User{ID: 10, Username: "alice"},
		Caption:  "album caption",
		Photo:    &tele.Photo{File: tele.File{FileID: "photo-2", FileSize: 20}, Width: 640, Height: 480},
	}

	inbound, ok := adapter.buildTelegramMediaGroupInboundMessage(bot, cfg, []*tele.Message{first, second})
	if !ok {
		t.Fatal("expected grouped inbound message")
	}
	if inbound.Message.Text != "album caption" {
		t.Fatalf("unexpected grouped text: %q", inbound.Message.Text)
	}
	if len(inbound.Message.Attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(inbound.Message.Attachments))
	}
	if inbound.Message.Attachments[0].PlatformKey != "photo-1" || inbound.Message.Attachments[1].PlatformKey != "photo-2" {
		t.Fatalf("unexpected attachment order: %#v", inbound.Message.Attachments)
	}
	if inbound.Message.ID != "102" {
		t.Fatalf("expected anchor message id 102, got %s", inbound.Message.ID)
	}
	if inbound.ReplyTarget != "-10001" {
		t.Fatalf("unexpected reply target: %q", inbound.ReplyTarget)
	}
	if got := inbound.Metadata["media_group_id"]; got != "group-1" {
		t.Fatalf("unexpected media_group_id metadata: %#v", got)
	}
	if got := inbound.Metadata["media_group_size"]; got != 2 {
		t.Fatalf("unexpected media_group_size metadata: %#v", got)
	}
}

func TestBuildTelegramInboundMessageIncludesUpdateIDMetadata(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	bot := newStubTelegramBot(t)
	bot.Me = &tele.User{ID: 1001, Username: "memohbot"}
	update := &tele.Update{
		ID: 777,
		Message: &tele.Message{
			ID:       101,
			Unixtime: 1710000000,
			Text:     "hello",
			Chat:     &tele.Chat{ID: 123, Type: tele.ChatPrivate},
			Sender:   &tele.User{ID: 10, Username: "alice"},
		},
	}

	inbound, ok := adapter.buildTelegramInboundMessage(bot, channel.ChannelConfig{}, update)
	if !ok {
		t.Fatal("expected inbound message")
	}
	if got := inbound.Metadata["update_id"]; got != 777 {
		t.Fatalf("unexpected update_id metadata: %#v", got)
	}
}

func TestBuildTelegramInboundMessage_PlainFormatWhenNoEntities(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	bot := newStubTelegramBot(t)
	bot.Me = &tele.User{ID: 1, Username: "memohbot"}
	update := &tele.Update{
		ID: 1,
		Message: &tele.Message{
			ID:     1,
			Text:   "plain text only",
			Chat:   &tele.Chat{ID: 1, Type: tele.ChatPrivate},
			Sender: &tele.User{ID: 1, Username: "alice"},
		},
	}
	inbound, ok := adapter.buildTelegramInboundMessage(bot, channel.ChannelConfig{}, update)
	if !ok {
		t.Fatal("expected inbound message")
	}
	if inbound.Message.Format != channel.MessageFormatPlain {
		t.Fatalf("expected plain format when no entities, got %q", inbound.Message.Format)
	}
	if len(inbound.Message.Parts) != 0 {
		t.Fatalf("expected no Parts, got %+v", inbound.Message.Parts)
	}
}

func TestBuildTelegramInboundMessage_RichFormatWhenEntitiesPopulateParts(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	bot := newStubTelegramBot(t)
	bot.Me = &tele.User{ID: 1, Username: "memohbot"}
	update := &tele.Update{
		ID: 1,
		Message: &tele.Message{
			ID:     1,
			Text:   "hi shout bye",
			Chat:   &tele.Chat{ID: 1, Type: tele.ChatPrivate},
			Sender: &tele.User{ID: 1, Username: "alice"},
			Entities: tele.Entities{
				{Type: tele.EntityBold, Offset: 3, Length: 5},
			},
		},
	}
	inbound, ok := adapter.buildTelegramInboundMessage(bot, channel.ChannelConfig{}, update)
	if !ok {
		t.Fatal("expected inbound message")
	}
	if len(inbound.Message.Parts) == 0 {
		t.Fatalf("expected Parts populated from bold entity, got empty")
	}
	if inbound.Message.Format != channel.MessageFormatRich {
		t.Fatalf("expected rich format when Parts populate, got %q", inbound.Message.Format)
	}
}

func TestSeenTelegramUpdate(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	now := time.Unix(1710000000, 0)

	if adapter.seenTelegramUpdate("cfg-1", 42, now) {
		t.Fatal("first update should not be treated as duplicate")
	}
	if !adapter.seenTelegramUpdate("cfg-1", 42, now.Add(time.Second)) {
		t.Fatal("second update should be treated as duplicate")
	}
	if adapter.seenTelegramUpdate("cfg-2", 42, now.Add(time.Second)) {
		t.Fatal("same update_id under different config should not collide")
	}
	if adapter.seenTelegramUpdate("cfg-1", 43, now.Add(time.Second)) {
		t.Fatal("different update_id should not collide")
	}
	if adapter.seenTelegramUpdate("cfg-1", 0, now.Add(time.Second)) {
		t.Fatal("zero update_id should bypass dedupe")
	}

	later := now.Add(telegramUpdateDedupeTTL + time.Second)
	if adapter.seenTelegramUpdate("cfg-1", 42, later) {
		t.Fatal("expired dedupe entry should be accepted again")
	}
}

func TestIsTelegramMediaGroupForChat(t *testing.T) {
	t.Parallel()

	if isTelegramMediaGroupForChat("12:group-a", 12) == false {
		t.Fatal("expected same chat key to match")
	}
	if isTelegramMediaGroupForChat("123:group-a", 12) {
		t.Fatal("expected different chat key to not match")
	}
	if isTelegramMediaGroupForChat("", 12) {
		t.Fatal("expected empty key to not match")
	}
	if isTelegramMediaGroupForChat("12:group-a", 0) {
		t.Fatal("expected zero chat id to not match")
	}
}

func TestTelegramAdapter_Type(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	if adapter.Type() != Type {
		t.Fatalf("Type should return telegram: %s", adapter.Type())
	}
}

func TestTelegramAdapter_OpenStreamEmptyTarget(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	ctx := context.Background()
	cfg := channel.ChannelConfig{}
	_, err := adapter.OpenStream(ctx, cfg, "", channel.StreamOptions{})
	if err == nil {
		t.Fatal("empty target should return error")
	}
	if !strings.Contains(err.Error(), "target") {
		t.Fatalf("expected target in error: %v", err)
	}
}

func TestResolveTelegramSender_SenderChat(t *testing.T) {
	t.Parallel()

	msg := &tele.Message{
		SenderChat: &tele.Chat{ID: 456, Username: "group", Title: "My Group"},
	}
	externalID, displayName, attrs := resolveTelegramSender(msg)
	if externalID != "456" {
		t.Fatalf("unexpected externalID: %s", externalID)
	}
	if displayName != "My Group" {
		t.Fatalf("unexpected displayName: %s", displayName)
	}
	if attrs["sender_chat_id"] != "456" || attrs["sender_chat_username"] != "group" {
		t.Fatalf("unexpected attrs: %#v", attrs)
	}
}

func TestTelegramRecipient_NumericChatID(t *testing.T) {
	t.Parallel()

	r, chatID, err := telegramRecipient("12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Recipient() != "12345" {
		t.Fatalf("unexpected recipient: %q", r.Recipient())
	}
	if chatID != 12345 {
		t.Fatalf("unexpected chatID: %d", chatID)
	}
}

func TestTelegramRecipient_ChannelUsername(t *testing.T) {
	t.Parallel()

	r, chatID, err := telegramRecipient("@channel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Recipient() != "@channel" {
		t.Fatalf("unexpected recipient: %q", r.Recipient())
	}
	if chatID != 0 {
		t.Fatalf("expected zero chatID for channel username: %d", chatID)
	}
}

func TestTelegramRecipient_Invalid(t *testing.T) {
	t.Parallel()

	_, _, err := telegramRecipient("invalid")
	if err == nil {
		t.Fatal("expected error for invalid target")
	}
	if !strings.Contains(err.Error(), "chat_id") {
		t.Fatalf("expected chat_id in error: %v", err)
	}
}

func TestTelegramAdapter_NormalizeAndResolve(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	norm, err := adapter.NormalizeConfig(map[string]any{"botToken": "t1"})
	if err != nil {
		t.Fatalf("NormalizeConfig: %v", err)
	}
	if norm["botToken"] != "t1" {
		t.Fatalf("unexpected normalized: %#v", norm)
	}
	userNorm, err := adapter.NormalizeUserConfig(map[string]any{"username": "u1"})
	if err != nil {
		t.Fatalf("NormalizeUserConfig: %v", err)
	}
	if userNorm["username"] != "u1" {
		t.Fatalf("unexpected user config: %#v", userNorm)
	}
	if got := adapter.NormalizeTarget("https://t.me/x"); got != "@x" {
		t.Fatalf("NormalizeTarget: %s", got)
	}
	target, err := adapter.ResolveTarget(map[string]any{"chat_id": "123"})
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if target != "123" {
		t.Fatalf("ResolveTarget: %s", target)
	}
}

func TestConfig_Endpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		baseURL  string
		wantAPI  string
		wantFile string
	}{
		{"default", "", "https://api.telegram.org/bot%s/%s", "https://api.telegram.org/file/bot%s/%s"},
		{"custom", "https://tg.example.com", "https://tg.example.com/bot%s/%s", "https://tg.example.com/file/bot%s/%s"},
		{"trailing slash", "https://tg.example.com/", "https://tg.example.com/bot%s/%s", "https://tg.example.com/file/bot%s/%s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := Config{BotToken: "tok", APIBaseURL: tt.baseURL}
			if got := cfg.apiEndpoint(); got != tt.wantAPI {
				t.Fatalf("apiEndpoint() = %q, want %q", got, tt.wantAPI)
			}
			if got := cfg.fileEndpoint(); got != tt.wantFile {
				t.Fatalf("fileEndpoint() = %q, want %q", got, tt.wantFile)
			}
		})
	}
}

func TestParseConfig_APIBaseURL(t *testing.T) {
	t.Parallel()

	t.Run("camelCase key", func(t *testing.T) {
		t.Parallel()
		cfg, err := parseConfig(map[string]any{"botToken": "t1", "apiBaseURL": "https://proxy.example.com"})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.APIBaseURL != "https://proxy.example.com" {
			t.Fatalf("unexpected APIBaseURL: %q", cfg.APIBaseURL)
		}
	})

	t.Run("snake_case key", func(t *testing.T) {
		t.Parallel()
		cfg, err := parseConfig(map[string]any{"bot_token": "t2", "api_base_url": "https://proxy2.example.com"})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.APIBaseURL != "https://proxy2.example.com" {
			t.Fatalf("unexpected APIBaseURL: %q", cfg.APIBaseURL)
		}
	})

	t.Run("empty base URL", func(t *testing.T) {
		t.Parallel()
		cfg, err := parseConfig(map[string]any{"botToken": "t3"})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.APIBaseURL != "" {
			t.Fatalf("expected empty APIBaseURL, got %q", cfg.APIBaseURL)
		}
	})

	t.Run("http proxy config", func(t *testing.T) {
		t.Parallel()
		proxyURL := "http://memoh:" + "secret" + "@sztu.cc:3128"
		cfg, err := parseConfig(map[string]any{
			"botToken":     "t4",
			"httpProxyUrl": proxyURL,
		})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.HTTPProxy.URL != proxyURL {
			t.Fatalf("unexpected httpProxyUrl: %q", cfg.HTTPProxy.URL)
		}
	})
}

func TestNormalizeConfig_APIBaseURL(t *testing.T) {
	t.Parallel()

	t.Run("present", func(t *testing.T) {
		t.Parallel()
		norm, err := normalizeConfig(map[string]any{"botToken": "t1", "apiBaseURL": "https://proxy.example.com"})
		if err != nil {
			t.Fatal(err)
		}
		if norm["apiBaseURL"] != "https://proxy.example.com" {
			t.Fatalf("expected apiBaseURL in output: %#v", norm)
		}
	})

	t.Run("omitted when empty", func(t *testing.T) {
		t.Parallel()
		norm, err := normalizeConfig(map[string]any{"botToken": "t2"})
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := norm["apiBaseURL"]; exists {
			t.Fatalf("empty apiBaseURL should be omitted: %#v", norm)
		}
	})

	t.Run("includes http proxy config", func(t *testing.T) {
		t.Parallel()
		proxyURL := "http://memoh:" + "secret" + "@sztu.cc:3128"
		norm, err := normalizeConfig(map[string]any{
			"botToken":     "t3",
			"httpProxyUrl": proxyURL,
		})
		if err != nil {
			t.Fatal(err)
		}
		if norm["httpProxyUrl"] != proxyURL {
			t.Fatalf("unexpected httpProxyUrl in output: %#v", norm)
		}
	})
}

func TestIsTelegramMessageNotModified(t *testing.T) {
	t.Parallel()

	// Exact production error from Telegram API (editMessageText when content unchanged).
	const productionMessageNotModified = "Bad Request: message is not modified: specified new message content and reply markup are exactly the same as a current content and reply markup of the message"

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("network error"), false},
		{"other api error", &tele.Error{Code: 400, Description: "Bad Request: chat not found"}, false},
		{"message is not modified", &tele.Error{Code: 400, Description: productionMessageNotModified}, true},
		{"production exact", &tele.Error{Code: 400, Description: productionMessageNotModified}, true},
		{"same text but code 500", &tele.Error{Code: 500, Description: "message is not modified"}, false},
		{"wrapped same", fmt.Errorf("wrapped: %w", &tele.Error{Code: 400, Description: "Bad Request: message is not modified"}), true},
		// telebot only types errors whose description matches a sentinel; longer or
		// novel variants of "message is not modified" arrive as fmt.Errorf strings
		// shaped "telegram: <description> (<code>)" with no *tele.Error in the chain.
		{"telebot fmt-wrapped variant", errors.New("telegram: Bad Request: message is not modified: caption is unchanged (400)"), true},
		{"telebot fmt-wrapped wrong code", errors.New("telegram: Bad Request: message is not modified (500)"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTelegramMessageNotModified(tt.err)
			if got != tt.want {
				t.Fatalf("isTelegramMessageNotModified() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTelegramEditUnrecoverable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("network error"), false},
		{"message to edit not found", &tele.Error{Code: 400, Description: "Bad Request: message to edit not found"}, true},
		{"message can't be edited", &tele.Error{Code: 400, Description: "Bad Request: message can't be edited"}, true},
		{"message_id_invalid", &tele.Error{Code: 400, Description: "Bad Request: MESSAGE_ID_INVALID"}, true},
		{"not modified is not this", &tele.Error{Code: 400, Description: "Bad Request: message is not modified"}, false},
		{"transient chat not found", &tele.Error{Code: 400, Description: "Bad Request: chat not found"}, false},
		{"rate limit not terminal", &tele.Error{Code: 429, Description: "Too Many Requests"}, false},
		{"same text but code 500", &tele.Error{Code: 500, Description: "message to edit not found"}, false},
		{"wrapped terminal", fmt.Errorf("wrapped: %w", &tele.Error{Code: 400, Description: "Bad Request: message to edit not found"}), true},
		// telebot has no sentinel for these; extractOk emits the raw fmt.Errorf
		// shape, which used to slip past the *tele.Error check and broke the
		// streaming final-edit recovery path.
		{"telebot fmt-wrapped not found", errors.New("telegram: Bad Request: message to edit not found (400)"), true},
		{"telebot fmt-wrapped message_id_invalid", errors.New("telegram: Bad Request: MESSAGE_ID_INVALID (400)"), true},
		{"telebot fmt-wrapped wrong code", errors.New("telegram: Bad Request: message to edit not found (500)"), false},
		{"telebot fmt-wrapped unrelated", errors.New("telegram: Bad Request: chat not found (400)"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTelegramEditUnrecoverable(tt.err)
			if got != tt.want {
				t.Fatalf("isTelegramEditUnrecoverable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTelegramTooManyRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"429", &tele.Error{Code: 429, Description: "Too Many Requests"}, true},
		{"400", &tele.Error{Code: 400, Description: "Bad Request"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTelegramTooManyRequests(tt.err)
			if got != tt.want {
				t.Fatalf("isTelegramTooManyRequests() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetTelegramRetryAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want time.Duration
	}{
		{"nil", nil, 0},
		{"no retry_after", &tele.Error{Code: 429, Description: "Too Many Requests"}, 0},
		{"retry_after 2", tele.FloodError{RetryAfter: 2}, 2 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getTelegramRetryAfter(tt.err)
			if got != tt.want {
				t.Fatalf("getTelegramRetryAfter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTruncateTelegramText(t *testing.T) {
	t.Parallel()

	short := "hello"
	if got := truncateTelegramText(short); got != short {
		t.Fatalf("short text should not be truncated: %q", got)
	}

	// Exactly at limit.
	exact := strings.Repeat("a", telegramMaxMessageLength)
	if got := truncateTelegramText(exact); got != exact {
		t.Fatalf("exact-limit text should not be truncated, len=%d", len(got))
	}

	// Over limit with ASCII.
	over := strings.Repeat("a", telegramMaxMessageLength+100)
	got := truncateTelegramText(over)
	if utf8.RuneCountInString(got) > telegramMaxMessageLength {
		t.Fatalf("truncated text should be <= %d chars: got %d", telegramMaxMessageLength, utf8.RuneCountInString(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("truncated text should end with '...': %q", got[len(got)-10:])
	}

	// Over limit with multi-byte characters (Chinese: 3 bytes each).
	multi := strings.Repeat("\u4f60", telegramMaxMessageLength+1)
	got = truncateTelegramText(multi)
	if utf8.RuneCountInString(got) > telegramMaxMessageLength {
		t.Fatalf("truncated multi-byte text should be <= %d chars: got %d", telegramMaxMessageLength, utf8.RuneCountInString(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatal("truncated multi-byte text should end with '...'")
	}
	if utf8.RuneCountInString(got) != telegramMaxMessageLength {
		t.Fatalf("truncated multi-byte text should keep exact char budget: got %d", utf8.RuneCountInString(got))
	}
	// Verify no broken runes.
	trimmed := strings.TrimSuffix(got, "...")
	for i := 0; i < len(trimmed); {
		r, size := utf8.DecodeRuneInString(trimmed[i:])
		if r == utf8.RuneError && size == 1 {
			t.Fatalf("truncated text contains invalid UTF-8 at byte %d", i)
		}
		i += size
	}
}

func TestSanitizeTelegramText(t *testing.T) {
	t.Parallel()

	valid := "hello world"
	if got := sanitizeTelegramText(valid); got != valid {
		t.Fatalf("valid text should not change: %q", got)
	}

	// Invalid UTF-8 byte sequence.
	invalid := "hello\xff\xfeworld"
	got := sanitizeTelegramText(invalid)
	if !utf8.ValidString(got) {
		t.Fatalf("sanitized text should be valid UTF-8: %q", got)
	}
	if got != "helloworld" {
		t.Fatalf("expected invalid bytes stripped: %q", got)
	}
}

func TestEditTelegramMessageText_429ReturnsError(t *testing.T) {
	t.Parallel()

	var sendCalls int
	origSend := sendEditForTest
	sendEditForTest = func(_ *tele.Bot, _ int64, _ int, _ string, _ string) error {
		sendCalls++
		return tele.FloodError{RetryAfter: 1}
	}
	defer func() { sendEditForTest = origSend }()

	bot := &tele.Bot{Token: "test"}
	err := editTelegramMessageText(bot, 1, 1, "hi", "")
	if err == nil {
		t.Fatal("editTelegramMessageText on 429 should return error for caller to handle")
	}
	if !isTelegramTooManyRequests(err) {
		t.Fatalf("expected 429 error: %v", err)
	}
	if sendCalls != 1 {
		t.Fatalf("send should be called once (no internal retry): got %d", sendCalls)
	}
}

func TestTelegramAdapter_ImplementsProcessingStatusNotifier(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	var _ channel.ProcessingStatusNotifier = adapter
}

func TestProcessingStarted_EmptyParams(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	ctx := context.Background()
	cfg := channel.ChannelConfig{}
	msg := channel.InboundMessage{}

	handle, err := adapter.ProcessingStarted(ctx, cfg, msg, channel.ProcessingStatusInfo{})
	if err != nil {
		t.Fatalf("empty params should not error: %v", err)
	}
	if handle.Token != "" {
		t.Fatalf("empty params should return empty handle: %q", handle.Token)
	}
}

func TestProcessingCompleted_EmptyHandle(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	ctx := context.Background()

	err := adapter.ProcessingCompleted(ctx, channel.ChannelConfig{}, channel.InboundMessage{}, channel.ProcessingStatusInfo{}, channel.ProcessingStatusHandle{})
	if err != nil {
		t.Fatalf("empty handle should be no-op: %v", err)
	}
}

func TestProcessingFailed_DelegatesToCompleted(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	ctx := context.Background()

	err := adapter.ProcessingFailed(ctx, channel.ChannelConfig{}, channel.InboundMessage{}, channel.ProcessingStatusInfo{}, channel.ProcessingStatusHandle{}, errors.New("test"))
	if err != nil {
		t.Fatalf("empty handle should be no-op: %v", err)
	}
}

func TestResolveTelegramFile_PlatformKey(t *testing.T) {
	t.Parallel()

	file, err := resolveTelegramFile(context.Background(), channel.PreparedAttachment{
		Logical:   channel.Attachment{Type: channel.AttachmentImage},
		Kind:      channel.PreparedAttachmentNativeRef,
		NativeRef: "file_id_123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.FileID != "file_id_123" {
		t.Fatalf("expected FileID populated, got %+v", file)
	}
}

func TestResolveTelegramFile_PublicURL(t *testing.T) {
	t.Parallel()

	file, err := resolveTelegramFile(context.Background(), channel.PreparedAttachment{
		Logical:   channel.Attachment{Type: channel.AttachmentImage},
		Kind:      channel.PreparedAttachmentPublicURL,
		PublicURL: "https://example.com/img.png",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.FileURL != "https://example.com/img.png" {
		t.Fatalf("expected FileURL populated, got %+v", file)
	}
}

func TestResolveTelegramFile_Upload(t *testing.T) {
	t.Parallel()

	file, err := resolveTelegramFile(context.Background(), channel.PreparedAttachment{
		Logical: channel.Attachment{Type: channel.AttachmentImage},
		Kind:    channel.PreparedAttachmentUpload,
		Mime:    "image/png",
		Name:    "test.png",
		Open: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("png-bytes")), nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.FileReader == nil {
		t.Fatalf("expected FileReader populated, got %+v", file)
	}
	body, readErr := io.ReadAll(file.FileReader)
	if readErr != nil {
		t.Fatalf("read upload bytes: %v", readErr)
	}
	if string(body) != "png-bytes" {
		t.Fatalf("expected png-bytes, got %q", string(body))
	}
}

func TestResolveTelegramFile_NoReference(t *testing.T) {
	t.Parallel()

	_, err := resolveTelegramFile(context.Background(), channel.PreparedAttachment{
		Logical: channel.Attachment{Type: channel.AttachmentImage},
	})
	if err == nil {
		t.Fatal("expected error when no reference available")
	}
}

func TestResolveTelegramFile_UploadRequiresOpen(t *testing.T) {
	t.Parallel()

	_, err := resolveTelegramFile(context.Background(), channel.PreparedAttachment{
		Logical: channel.Attachment{Type: channel.AttachmentImage},
		Kind:    channel.PreparedAttachmentUpload,
	})
	if err == nil {
		t.Fatal("expected missing upload opener to fail")
	}
}

func TestFileNameFromMime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mime         string
		fallbackType string
		want         string
	}{
		{"image/png", "image", "image.png"},
		{"image/jpeg", "image", "image.jpg"},
		{"image/gif", "image", "image.gif"},
		{"video/mp4", "video", "video.mp4"},
		{"", "image", "image.png"},
		{"application/octet-stream", "", "file.bin"},
	}
	for _, tt := range tests {
		if got := fileNameFromMime(tt.mime, tt.fallbackType); got != tt.want {
			t.Errorf("fileNameFromMime(%q, %q) = %q, want %q", tt.mime, tt.fallbackType, got, tt.want)
		}
	}
}
