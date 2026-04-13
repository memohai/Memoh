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

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/memohai/memoh/internal/channel"
)

func TestResolveTelegramSender(t *testing.T) {
	t.Parallel()

	externalID, displayName, attrs := resolveTelegramSender(nil)
	if externalID != "" || displayName != "" || len(attrs) != 0 {
		t.Fatalf("expected empty sender")
	}
	msg := &tgbotapi.Message{
		From: &tgbotapi.User{ID: 123, UserName: "alice"},
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
		msg := &tgbotapi.Message{
			Text: "hello @MemohBot",
		}
		if !isTelegramBotMentioned(msg, "memohbot") {
			t.Fatalf("expected bot mention from text")
		}
	})

	t.Run("entity text mention matching bot", func(t *testing.T) {
		t.Parallel()
		msg := &tgbotapi.Message{
			Entities: []tgbotapi.MessageEntity{
				{
					Type: "text_mention",
					User: &tgbotapi.User{IsBot: true, UserName: "memohbot"},
				},
			},
		}
		if !isTelegramBotMentioned(msg, "memohbot") {
			t.Fatalf("expected bot mention from text_mention entity")
		}
	})

	t.Run("entity text mention other bot", func(t *testing.T) {
		t.Parallel()
		msg := &tgbotapi.Message{
			Entities: []tgbotapi.MessageEntity{
				{
					Type: "text_mention",
					User: &tgbotapi.User{IsBot: true, UserName: "otherbot"},
				},
			},
		}
		if isTelegramBotMentioned(msg, "memohbot") {
			t.Fatalf("expected no mention for different bot")
		}
	})

	t.Run("not mentioned", func(t *testing.T) {
		t.Parallel()
		msg := &tgbotapi.Message{
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
	msg := &tgbotapi.Message{}
	if buildTelegramReplyRef(msg, "123") != nil {
		t.Fatal("msg without ReplyToMessage should return nil")
	}
	msg.ReplyToMessage = &tgbotapi.Message{MessageID: 42}
	ref := buildTelegramReplyRef(msg, "  -100  ")
	if ref == nil {
		t.Fatal("expected non-nil ref")
		return
	}
	if ref.MessageID != "42" || ref.Target != "-100" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
}

func TestPickTelegramPhoto(t *testing.T) {
	t.Parallel()

	if got := pickTelegramPhoto(nil); got.FileID != "" {
		t.Fatalf("nil should return zero: %+v", got)
	}
	if got := pickTelegramPhoto([]tgbotapi.PhotoSize{}); got.FileID != "" {
		t.Fatalf("empty slice should return zero: %+v", got)
	}
	one := tgbotapi.PhotoSize{FileID: "a", FileSize: 100, Width: 10, Height: 10}
	if got := pickTelegramPhoto([]tgbotapi.PhotoSize{one}); got.FileID != "a" {
		t.Fatalf("single photo should return it: %+v", got)
	}
	photos := []tgbotapi.PhotoSize{
		{FileID: "small", FileSize: 100, Width: 100, Height: 100},
		{FileID: "large", FileSize: 500, Width: 200, Height: 200},
	}
	if got := pickTelegramPhoto(photos); got.FileID != "large" {
		t.Fatalf("should pick largest by size: %+v", got)
	}
}

func TestBuildTelegramMediaGroupInboundMessageAggregatesAttachments(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	bot := &tgbotapi.BotAPI{
		Token: "test",
		Self:  tgbotapi.User{ID: 1001, UserName: "memohbot"},
	}
	cfg := channel.ChannelConfig{}
	first := &tgbotapi.Message{
		MessageID:    101,
		MediaGroupID: "group-1",
		Date:         1710000000,
		Chat:         &tgbotapi.Chat{ID: -10001, Type: "group", Title: "G1"},
		From:         &tgbotapi.User{ID: 10, UserName: "alice"},
		Photo: []tgbotapi.PhotoSize{
			{FileID: "photo-1", Width: 320, Height: 240, FileSize: 10},
		},
	}
	second := &tgbotapi.Message{
		MessageID:    102,
		MediaGroupID: "group-1",
		Date:         1710000001,
		Chat:         &tgbotapi.Chat{ID: -10001, Type: "group", Title: "G1"},
		From:         &tgbotapi.User{ID: 10, UserName: "alice"},
		Caption:      "album caption",
		Photo: []tgbotapi.PhotoSize{
			{FileID: "photo-2", Width: 640, Height: 480, FileSize: 20},
		},
	}

	inbound, ok := adapter.buildTelegramMediaGroupInboundMessage(bot, cfg, []*tgbotapi.Message{first, second})
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
	bot := &tgbotapi.BotAPI{
		Token: "test",
		Self:  tgbotapi.User{ID: 1001, UserName: "memohbot"},
	}
	update := tgbotapi.Update{
		UpdateID: 777,
		Message: &tgbotapi.Message{
			MessageID: 101,
			Date:      1710000000,
			Text:      "hello",
			Chat:      &tgbotapi.Chat{ID: 123, Type: "private"},
			From:      &tgbotapi.User{ID: 10, UserName: "alice"},
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

	msg := &tgbotapi.Message{
		SenderChat: &tgbotapi.Chat{ID: 456, UserName: "group", Title: "My Group"},
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

func TestBuildTelegramAudio(t *testing.T) {
	t.Parallel()

	cfg, err := buildTelegramAudio("@channel", tgbotapi.FileID("f1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ChannelUsername != "@channel" {
		t.Fatalf("unexpected channel: %s", cfg.ChannelUsername)
	}
	_, err = buildTelegramAudio("invalid", tgbotapi.FileID("f1"))
	if err == nil {
		t.Fatal("invalid target should return error")
	}
	if !strings.Contains(err.Error(), "chat_id") {
		t.Fatalf("expected chat_id in error: %v", err)
	}
}

func TestBuildTelegramVoice(t *testing.T) {
	t.Parallel()

	cfg, err := buildTelegramVoice("@ch", tgbotapi.FileID("f1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ChannelUsername != "@ch" {
		t.Fatalf("unexpected channel: %s", cfg.ChannelUsername)
	}
	_, err = buildTelegramVoice("x", tgbotapi.FileID("f1"))
	if err == nil {
		t.Fatal("invalid target should return error")
	}
}

func TestBuildTelegramVideo(t *testing.T) {
	t.Parallel()

	cfg, err := buildTelegramVideo("@ch", tgbotapi.FileID("f1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ChannelUsername != "@ch" {
		t.Fatalf("unexpected channel: %s", cfg.ChannelUsername)
	}
	_, err = buildTelegramVideo("bad", tgbotapi.FileID("f1"))
	if err == nil {
		t.Fatal("invalid target should return error")
	}
}

func TestBuildTelegramAnimation(t *testing.T) {
	t.Parallel()

	cfg, err := buildTelegramAnimation("@ch", tgbotapi.FileID("f1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ChannelUsername != "@ch" {
		t.Fatalf("unexpected channel: %s", cfg.ChannelUsername)
	}
	_, err = buildTelegramAnimation("x", tgbotapi.FileID("f1"))
	if err == nil {
		t.Fatal("invalid target should return error")
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
		{"other api error", tgbotapi.Error{Code: 400, Message: "Bad Request: chat not found"}, false},
		{"message is not modified", tgbotapi.Error{Code: 400, Message: productionMessageNotModified}, true},
		{"production exact", tgbotapi.Error{Code: 400, Message: productionMessageNotModified}, true},
		{"same text but code 500", tgbotapi.Error{Code: 500, Message: "message is not modified"}, false},
		{"wrapped same", fmt.Errorf("wrapped: %w", tgbotapi.Error{Code: 400, Message: "Bad Request: message is not modified"}), true},
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

func TestIsTelegramTooManyRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"429", tgbotapi.Error{Code: 429, Message: "Too Many Requests"}, true},
		{"400", tgbotapi.Error{Code: 400, Message: "Bad Request"}, false},
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
		{"no retry_after", tgbotapi.Error{Code: 429, Message: "Too Many Requests"}, 0},
		{"retry_after 2", tgbotapi.Error{Code: 429, Message: "Too Many Requests", ResponseParameters: tgbotapi.ResponseParameters{RetryAfter: 2}}, 2 * time.Second},
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
	sendEditForTest = func(_ *tgbotapi.BotAPI, _ tgbotapi.EditMessageTextConfig) error {
		sendCalls++
		return tgbotapi.Error{
			Code:               429,
			Message:            "Too Many Requests",
			ResponseParameters: tgbotapi.ResponseParameters{RetryAfter: 1},
		}
	}
	defer func() { sendEditForTest = origSend }()

	bot := &tgbotapi.BotAPI{Token: "test"}
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
	if _, ok := file.(tgbotapi.FileID); !ok {
		t.Fatalf("expected FileID, got %T", file)
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
	if _, ok := file.(tgbotapi.FileURL); !ok {
		t.Fatalf("expected FileURL, got %T", file)
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
	fb, ok := file.(tgbotapi.FileBytes)
	if !ok {
		t.Fatalf("expected FileBytes, got %T", file)
	}
	if fb.Name != "test.png" {
		t.Fatalf("expected name test.png, got %q", fb.Name)
	}
	if string(fb.Bytes) != "png-bytes" {
		t.Fatalf("expected png-bytes, got %q", string(fb.Bytes))
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
