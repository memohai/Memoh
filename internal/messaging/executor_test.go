package messaging

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/media"
)

type testSender struct {
	called int
	req    channel.SendRequest
}

func (s *testSender) Send(_ context.Context, _ string, _ channel.ChannelType, req channel.SendRequest) error {
	s.called++
	s.req = req
	return nil
}

type testResolver struct{}

func (testResolver) ParseChannelType(raw string) (channel.ChannelType, error) {
	return channel.ChannelType(raw), nil
}

type testAssetResolver struct {
	ingestCalled int
	lastPath     string
}

func (*testAssetResolver) Stat(_ context.Context, _, _ string) (AssetMeta, error) {
	return AssetMeta{}, context.Canceled
}

func (*testAssetResolver) GetByStorageKey(_ context.Context, _, _ string) (AssetMeta, error) {
	return AssetMeta{}, context.Canceled
}

func (*testAssetResolver) Open(_ context.Context, _, _ string) (io.ReadCloser, media.Asset, error) {
	return nil, media.Asset{}, context.Canceled
}

func (*testAssetResolver) Ingest(_ context.Context, _ media.IngestInput) (media.Asset, error) {
	return media.Asset{}, context.Canceled
}

func (*testAssetResolver) AccessPath(asset media.Asset) string {
	return "https://example.com/media/" + asset.ContentHash
}

func (r *testAssetResolver) IngestContainerFile(_ context.Context, _, containerPath string) (AssetMeta, error) {
	r.ingestCalled++
	r.lastPath = containerPath
	return AssetMeta{
		ContentHash: "hash_1",
		Mime:        "image/png",
		SizeBytes:   42,
		StorageKey:  "media/generated/hash_1",
	}, nil
}

func TestSendDirectSameConversationWithAttachmentsResolvesAssets(t *testing.T) {
	t.Parallel()

	sender := &testSender{}
	exec := &Executor{
		Sender:        sender,
		Resolver:      testResolver{},
		AssetResolver: &testAssetResolver{},
	}

	session := SessionContext{
		BotID:              "bot_1",
		CanOmitTarget:      true,
		AllowLocalShortcut: true,
		CurrentPlatform:    "feishu",
		ReplyTarget:        "chat_id:oc_group_1",
	}

	result, err := exec.SendDirect(context.Background(), session, "", map[string]any{
		"attachments": []any{"screenshot.png"},
	})
	if err != nil {
		t.Fatalf("SendDirect returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if sender.called != 1 {
		t.Fatalf("expected sender called once, got %d", sender.called)
	}
	if len(sender.req.Message.Attachments) != 1 || sender.req.Message.Attachments[0].ContentHash != "hash_1" {
		t.Fatalf("unexpected attachments: %+v", sender.req.Message.Attachments)
	}
}

func TestSendSameConversationWithAttachmentsUsesLocalResult(t *testing.T) {
	t.Parallel()

	sender := &testSender{}
	exec := &Executor{
		Sender:        sender,
		Resolver:      testResolver{},
		AssetResolver: &testAssetResolver{},
	}

	session := SessionContext{
		BotID:              "bot_1",
		CanOmitTarget:      true,
		AllowLocalShortcut: true,
		CurrentPlatform:    "feishu",
		ReplyTarget:        "chat_id:oc_group_1",
	}

	result, err := exec.Send(context.Background(), session, map[string]any{
		"attachments": []any{"screenshot.png"},
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
		return
	}
	if !result.Local {
		t.Fatal("expected local result for same-conversation send")
	}
	if sender.called != 0 {
		t.Fatalf("expected sender not called for local result, got %d", sender.called)
	}
	if len(result.LocalAttachments) != 1 {
		t.Fatalf("expected 1 local attachment, got %d", len(result.LocalAttachments))
	}
	att := result.LocalAttachments[0]
	if att.Path != "/data/screenshot.png" {
		t.Fatalf("unexpected local attachment path: %q", att.Path)
	}
	if att.Type != channel.AttachmentImage {
		t.Fatalf("unexpected local attachment type: %q", att.Type)
	}
}

func TestSendSameConversationWithLocalShortcutDisabledUsesSender(t *testing.T) {
	t.Parallel()

	sender := &testSender{}
	exec := &Executor{
		Sender:        sender,
		Resolver:      testResolver{},
		AssetResolver: &testAssetResolver{},
	}

	result, err := exec.Send(context.Background(), SessionContext{
		BotID:           "bot_1",
		CanOmitTarget:   false,
		CurrentPlatform: "feishu",
		ReplyTarget:     "chat_id:oc_group_1",
	}, map[string]any{
		"platform":    "feishu",
		"target":      "chat_id:oc_group_1",
		"attachments": []any{"screenshot.png"},
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Local {
		t.Fatal("expected non-local result when local shortcut is disabled")
	}
	if sender.called != 1 {
		t.Fatalf("expected sender called once, got %d", sender.called)
	}
	if sender.req.Target != "chat_id:oc_group_1" {
		t.Fatalf("unexpected target: %q", sender.req.Target)
	}
	if len(sender.req.Message.Attachments) != 1 || sender.req.Message.Attachments[0].ContentHash != "hash_1" {
		t.Fatalf("unexpected attachments: %+v", sender.req.Message.Attachments)
	}
}

func TestSendSameConversationStructuredMessageAttachmentsAreNormalized(t *testing.T) {
	t.Parallel()

	sender := &testSender{}
	exec := &Executor{
		Sender:   sender,
		Resolver: testResolver{},
	}

	result, err := exec.Send(context.Background(), SessionContext{
		BotID:              "bot_1",
		CanOmitTarget:      true,
		AllowLocalShortcut: true,
		CurrentPlatform:    "feishu",
		ReplyTarget:        "chat_id:oc_group_1",
	}, map[string]any{
		"message": map[string]any{
			"attachments": []any{
				map[string]any{"path": "screenshot.png"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if result == nil || !result.Local {
		t.Fatalf("expected local result, got %#v", result)
	}
	if len(result.LocalAttachments) != 1 {
		t.Fatalf("expected 1 local attachment, got %d", len(result.LocalAttachments))
	}
	att := result.LocalAttachments[0]
	if att.Path != "/data/screenshot.png" {
		t.Fatalf("unexpected local attachment path: %q", att.Path)
	}
	if att.Type != channel.AttachmentImage {
		t.Fatalf("unexpected local attachment type: %q", att.Type)
	}
}

func TestSendSameConversationStructuredMessageAttachmentShorthandIsNormalized(t *testing.T) {
	t.Parallel()

	sender := &testSender{}
	exec := &Executor{
		Sender:   sender,
		Resolver: testResolver{},
	}

	result, err := exec.Send(context.Background(), SessionContext{
		BotID:              "bot_1",
		CanOmitTarget:      true,
		AllowLocalShortcut: true,
		CurrentPlatform:    "feishu",
		ReplyTarget:        "chat_id:oc_group_1",
	}, map[string]any{
		"message": map[string]any{
			"attachments": []any{"screenshot.png"},
		},
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if result == nil || !result.Local {
		t.Fatalf("expected local result, got %#v", result)
	}
	if len(result.LocalAttachments) != 1 {
		t.Fatalf("expected 1 local attachment, got %d", len(result.LocalAttachments))
	}
	if result.LocalAttachments[0].Path != "/data/screenshot.png" {
		t.Fatalf("unexpected local attachment path: %q", result.LocalAttachments[0].Path)
	}
}

func TestParseOutboundMessageRichPartsValidation(t *testing.T) {
	t.Parallel()

	msg, err := ParseOutboundMessage(map[string]any{
		"message": map[string]any{
			"format": "rich",
			"parts": []any{
				map[string]any{
					"type":   "text",
					"text":   "bold",
					"styles": []any{"bold"},
				},
				map[string]any{
					"type":     "code_block",
					"text":     "go test ./...",
					"language": "go",
				},
			},
		},
	}, "")
	if err != nil {
		t.Fatalf("ParseOutboundMessage returned error: %v", err)
	}
	if msg.Format != channel.MessageFormatRich || len(msg.Parts) != 2 {
		t.Fatalf("unexpected parsed message: %#v", msg)
	}
	if got := msg.Parts[0]; got.Type != channel.MessagePartText || len(got.Styles) != 1 || got.Styles[0] != channel.MessageStyleBold {
		t.Fatalf("unexpected first part: %#v", got)
	}

	tests := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{
			name: "unknown message field",
			raw:  map[string]any{"message": map[string]any{"text": "hi", "typo": "dropped"}},
			want: "unknown message field",
		},
		{
			name: "unknown part field",
			raw: map[string]any{"message": map[string]any{"parts": []any{
				map[string]any{"type": "text", "text": "hi", "typo": "dropped"},
			}}},
			want: "unknown message part field",
		},
		{
			name: "unknown part type",
			raw: map[string]any{"message": map[string]any{"parts": []any{
				map[string]any{"type": "heading", "text": "Title"},
			}}},
			want: "unsupported message part type",
		},
		{
			name: "unknown style",
			raw: map[string]any{"message": map[string]any{"parts": []any{
				map[string]any{"type": "text", "text": "hi", "styles": []any{"underline"}},
			}}},
			want: "unsupported message part style",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseOutboundMessage(tt.raw, "")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParseOutboundMessage error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestSendSameConversationTextOnlyFails(t *testing.T) {
	t.Parallel()

	sender := &testSender{}
	exec := &Executor{
		Sender:   sender,
		Resolver: testResolver{},
	}

	_, err := exec.Send(context.Background(), SessionContext{
		BotID:              "bot_1",
		CanOmitTarget:      true,
		AllowLocalShortcut: true,
		CurrentPlatform:    "feishu",
		ReplyTarget:        "chat_id:oc_group_1",
	}, map[string]any{
		"text": "ordinary reply",
	})
	if err == nil || !strings.Contains(err.Error(), "use assistant text") {
		t.Fatalf("Send error = %v, want assistant text guidance", err)
	}
	if sender.called != 0 {
		t.Fatalf("expected sender not called, got %d", sender.called)
	}
}

func TestSendSameConversationAttachmentWithTextFails(t *testing.T) {
	t.Parallel()

	sender := &testSender{}
	exec := &Executor{
		Sender:   sender,
		Resolver: testResolver{},
	}

	_, err := exec.Send(context.Background(), SessionContext{
		BotID:              "bot_1",
		CanOmitTarget:      true,
		AllowLocalShortcut: true,
		CurrentPlatform:    "feishu",
		ReplyTarget:        "chat_id:oc_group_1",
	}, map[string]any{
		"text":        "caption",
		"attachments": []any{"screenshot.png"},
	})
	if err == nil || !strings.Contains(err.Error(), "standalone files or attachments") {
		t.Fatalf("Send error = %v, want standalone attachment guidance", err)
	}
	if sender.called != 0 {
		t.Fatalf("expected sender not called, got %d", sender.called)
	}
}

func TestSendWithoutTargetAndNoCurrentConversationFails(t *testing.T) {
	t.Parallel()

	sender := &testSender{}
	exec := &Executor{
		Sender:   sender,
		Resolver: testResolver{},
	}

	_, err := exec.Send(context.Background(), SessionContext{
		BotID:           "bot_1",
		CurrentPlatform: "feishu",
	}, map[string]any{
		"text": "notify",
	})
	if err == nil || !strings.Contains(err.Error(), "target is required") {
		t.Fatalf("Send error = %v, want target is required", err)
	}
	if sender.called != 0 {
		t.Fatalf("expected sender not called, got %d", sender.called)
	}
}

func TestSendWithDifferentPlatformDoesNotReuseCurrentTarget(t *testing.T) {
	t.Parallel()

	sender := &testSender{}
	exec := &Executor{
		Sender:   sender,
		Resolver: testResolver{},
	}

	_, err := exec.Send(context.Background(), SessionContext{
		BotID:           "bot_1",
		CanOmitTarget:   true,
		CurrentPlatform: "telegram",
		ReplyTarget:     "telegram-chat-1",
	}, map[string]any{
		"platform": "discord",
		"text":     "notify",
	})
	if err == nil || !strings.Contains(err.Error(), "target is required") {
		t.Fatalf("Send error = %v, want target is required", err)
	}
	if sender.called != 0 {
		t.Fatalf("expected sender not called, got %d", sender.called)
	}
}

func TestSendCannotDefaultTargetWhenSessionDisallowsOmission(t *testing.T) {
	t.Parallel()

	sender := &testSender{}
	exec := &Executor{
		Sender:   sender,
		Resolver: testResolver{},
	}

	_, err := exec.Send(context.Background(), SessionContext{
		BotID:           "bot_1",
		CurrentPlatform: "telegram",
		ReplyTarget:     "telegram-chat-1",
	}, map[string]any{
		"text": "notify",
	})
	if err == nil || !strings.Contains(err.Error(), "target is required") {
		t.Fatalf("Send error = %v, want target is required", err)
	}
	if sender.called != 0 {
		t.Fatalf("expected sender not called, got %d", sender.called)
	}
}

type testReactor struct {
	called int
	req    channel.ReactRequest
}

func (r *testReactor) React(_ context.Context, _ string, _ channel.ChannelType, req channel.ReactRequest) error {
	r.called++
	r.req = req
	return nil
}

func TestReactWithDifferentPlatformDoesNotReuseCurrentTarget(t *testing.T) {
	t.Parallel()

	reactor := &testReactor{}
	exec := &Executor{
		Reactor:  reactor,
		Resolver: testResolver{},
	}

	_, err := exec.React(context.Background(), SessionContext{
		BotID:           "bot_1",
		CanOmitTarget:   true,
		CurrentPlatform: "telegram",
		ReplyTarget:     "telegram-chat-1",
	}, map[string]any{
		"platform":   "discord",
		"message_id": "msg-1",
		"emoji":      "👍",
	})
	if err == nil || !strings.Contains(err.Error(), "target is required") {
		t.Fatalf("React error = %v, want target is required", err)
	}
	if reactor.called != 0 {
		t.Fatalf("expected reactor not called, got %d", reactor.called)
	}
}

func TestReactSameConversationLocalShortcut(t *testing.T) {
	t.Parallel()

	reactor := &testReactor{}
	exec := &Executor{
		Reactor:  reactor,
		Resolver: testResolver{},
	}

	result, err := exec.React(context.Background(), SessionContext{
		BotID:              "bot_1",
		CanOmitTarget:      true,
		AllowLocalShortcut: true,
		CurrentPlatform:    "web",
		ReplyTarget:        "bot_1",
	}, map[string]any{
		"message_id": "msg-1",
		"emoji":      "👍",
	})
	if err != nil {
		t.Fatalf("React returned error: %v", err)
	}
	if reactor.called != 0 {
		t.Fatalf("expected reactor not called for local shortcut, got %d", reactor.called)
	}
	if result == nil || !result.Local || result.Target != "bot_1" || result.MessageID != "msg-1" || result.Emoji != "👍" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestSendDirectPromotesDataPathAttachmentToContentHash(t *testing.T) {
	t.Parallel()

	sender := &testSender{}
	assets := &testAssetResolver{}
	exec := &Executor{
		Sender:        sender,
		Resolver:      testResolver{},
		AssetResolver: assets,
	}

	session := SessionContext{
		BotID:           "bot_1",
		CanOmitTarget:   true,
		CurrentPlatform: "feishu",
		ReplyTarget:     "chat_id:oc_group_1",
	}

	_, err := exec.SendDirect(context.Background(), session, "", map[string]any{
		"attachments": []any{"screenshot.png"},
	})
	if err != nil {
		t.Fatalf("SendDirect returned error: %v", err)
	}
	if sender.called != 1 {
		t.Fatalf("expected sender called once, got %d", sender.called)
	}
	if assets.ingestCalled != 1 || assets.lastPath != "/data/screenshot.png" {
		t.Fatalf("expected ingest called with /data path, got called=%d path=%q", assets.ingestCalled, assets.lastPath)
	}
	if len(sender.req.Message.Attachments) != 1 {
		t.Fatalf("expected one attachment, got %d", len(sender.req.Message.Attachments))
	}
	att := sender.req.Message.Attachments[0]
	if att.ContentHash != "hash_1" {
		t.Fatalf("expected promoted content hash, got %q", att.ContentHash)
	}
}
