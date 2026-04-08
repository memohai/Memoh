package messaging

import (
	"context"
	"io"
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

func TestSendDirectSameConversationWithAttachments(t *testing.T) {
	t.Parallel()

	sender := &testSender{}
	exec := &Executor{
		Sender:   sender,
		Resolver: testResolver{},
	}

	session := SessionContext{
		BotID:           "bot_1",
		CurrentPlatform: "feishu",
		ReplyTarget:     "chat_id:oc_group_1",
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
	if sender.req.Target != "chat_id:oc_group_1" {
		t.Fatalf("unexpected target: %q", sender.req.Target)
	}
	if len(sender.req.Message.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(sender.req.Message.Attachments))
	}
	att := sender.req.Message.Attachments[0]
	if att.URL != "/data/screenshot.png" {
		t.Fatalf("unexpected attachment url: %q", att.URL)
	}
	if att.Type != channel.AttachmentImage {
		t.Fatalf("unexpected attachment type: %q", att.Type)
	}
}

func TestSendSameConversationWithAttachmentsUsesLocalResult(t *testing.T) {
	t.Parallel()

	sender := &testSender{}
	exec := &Executor{
		Sender:   sender,
		Resolver: testResolver{},
	}

	session := SessionContext{
		BotID:           "bot_1",
		CurrentPlatform: "feishu",
		ReplyTarget:     "chat_id:oc_group_1",
	}

	result, err := exec.Send(context.Background(), session, map[string]any{
		"attachments": []any{"screenshot.png"},
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
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
	if att.URL != "/data/screenshot.png" {
		t.Fatalf("unexpected local attachment url: %q", att.URL)
	}
	if att.Type != channel.AttachmentImage {
		t.Fatalf("unexpected local attachment type: %q", att.Type)
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
	if att.URL != "https://example.com/media/hash_1" {
		t.Fatalf("expected public access URL after promotion, got %q", att.URL)
	}
}
