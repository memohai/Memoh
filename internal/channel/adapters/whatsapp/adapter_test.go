package whatsapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/media"
)

func TestDescriptorMarksQRSetupAndPrivateTextAndAttachments(t *testing.T) {
	desc := NewAdapter(nil, t.TempDir()).Descriptor()
	if desc.Type != Type {
		t.Fatalf("type = %s", desc.Type)
	}
	if desc.SetupMode != channel.SetupModeQR {
		t.Fatalf("setup mode = %q, want qr", desc.SetupMode)
	}
	if !desc.Capabilities.Text || !desc.Capabilities.BlockStreaming {
		t.Fatalf("text/block streaming capabilities not enabled: %+v", desc.Capabilities)
	}
	if !desc.Capabilities.Attachments || desc.Capabilities.Media || !desc.Capabilities.Reply {
		t.Fatalf("attachment/media/reply capabilities mismatch: %+v", desc.Capabilities)
	}
	if got := desc.Capabilities.ChatTypes; len(got) != 1 || got[0] != channel.ConversationTypePrivate {
		t.Fatalf("chat types = %#v", got)
	}
	if len(desc.ConfigSchema.Fields) != 0 {
		t.Fatalf("config fields = %#v, want QR-only empty schema", desc.ConfigSchema.Fields)
	}
}

func TestNormalizeConfigRequiresQRGeneratedStore(t *testing.T) {
	adapter := NewAdapter(nil, t.TempDir())
	if _, err := adapter.NormalizeConfig(nil); err == nil {
		t.Fatal("expected empty config to be rejected")
	}
	got, err := adapter.NormalizeConfig(map[string]any{"needs_relink": true})
	if err != nil {
		t.Fatalf("needs relink config rejected: %v", err)
	}
	if got["needsRelink"] != true {
		t.Fatalf("needsRelink = %#v", got["needsRelink"])
	}
	got, err = adapter.NormalizeConfig(map[string]any{"store_id": "cfg-1"})
	if err != nil {
		t.Fatalf("store config rejected: %v", err)
	}
	if got["storeId"] != "cfg-1" {
		t.Fatalf("storeId = %#v", got["storeId"])
	}
	for _, storeID := range []string{"../cfg-1", "cfg/1", `cfg\1`, "cfg 1"} {
		if _, err := adapter.NormalizeConfig(map[string]any{"storeId": storeID}); err == nil {
			t.Fatalf("storeId %q should be rejected", storeID)
		}
	}
}

func TestTargetAndBindingNormalization(t *testing.T) {
	adapter := NewAdapter(nil, t.TempDir())
	if got := adapter.NormalizeTarget("+1 (234) 567-8900"); got != "12345678900@s.whatsapp.net" {
		t.Fatalf("normalized phone = %q", got)
	}
	if got := adapter.NormalizeTarget("jid:12345@s.whatsapp.net"); got != "12345@s.whatsapp.net" {
		t.Fatalf("normalized jid = %q", got)
	}
	binding, err := adapter.NormalizeUserConfig(map[string]any{"phone": "+1 234 567 8900"})
	if err != nil {
		t.Fatalf("normalize binding: %v", err)
	}
	target, err := adapter.ResolveTarget(binding)
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if target != "12345678900@s.whatsapp.net" {
		t.Fatalf("target = %q", target)
	}
	if !adapter.MatchBinding(binding, channel.BindingCriteria{Attributes: map[string]string{"phone": "12345678900"}}) {
		t.Fatal("binding did not match phone criteria")
	}
	lidBinding, err := adapter.NormalizeUserConfig(map[string]any{"jid": "220701894168771@lid"})
	if err != nil {
		t.Fatalf("normalize lid binding: %v", err)
	}
	if lidBinding["jid"] != "220701894168771@lid" {
		t.Fatalf("lid jid = %#v", lidBinding["jid"])
	}
}

func TestRejectsNonPrivateTargets(t *testing.T) {
	adapter := NewAdapter(nil, t.TempDir())
	if _, err := adapter.NormalizeUserConfig(map[string]any{"jid": "12345@g.us"}); err == nil {
		t.Fatal("expected group jid binding to be rejected")
	}
	if _, err := adapter.NormalizeUserConfig(map[string]any{"jid": "s.whatsapp.net"}); err == nil {
		t.Fatal("expected empty private jid user to be rejected")
	}
	if _, err := adapter.NormalizeUserConfig(map[string]any{"jid": "12345:1@s.whatsapp.net"}); err == nil {
		t.Fatal("expected device jid binding to be rejected")
	}
	if _, err := adapter.ResolveTarget(map[string]any{"jid": "status@broadcast"}); err == nil {
		t.Fatal("expected broadcast jid target to be rejected")
	}
	if adapter.MatchBinding(map[string]any{"jid": "12345@g.us"}, channel.BindingCriteria{SubjectID: "12345@g.us"}) {
		t.Fatal("group jid binding should not match")
	}
	err := adapter.Send(context.Background(), channel.ChannelConfig{}, channel.PreparedOutboundMessage{
		Target: "12345@g.us",
		Message: channel.PreparedMessage{Message: channel.Message{
			Text: "hello",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "private chat") {
		t.Fatalf("send group target error = %v", err)
	}
	if _, err := adapter.OpenStream(context.Background(), channel.ChannelConfig{}, "12345@g.us", channel.StreamOptions{}); err == nil || !strings.Contains(err.Error(), "private chat") {
		t.Fatalf("open stream group target error = %v", err)
	}
}

func TestHandleMessageMapsPrivateText(t *testing.T) {
	adapter := NewAdapter(nil, t.TempDir())
	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: Type}
	chat := types.NewJID("12345678900", types.DefaultUserServer)
	var got channel.InboundMessage
	called := false
	adapter.handleMessage(context.Background(), cfg, &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:   chat,
				Sender: chat,
			},
			ID:        "MSGID",
			PushName:  "Alice",
			Timestamp: time.Unix(123, 0).UTC(),
		},
		Message: &waE2E.Message{Conversation: proto.String(" hello ")},
	}, func(_ context.Context, _ channel.ChannelConfig, msg channel.InboundMessage) error {
		called = true
		got = msg
		return nil
	})
	if !called {
		t.Fatal("handler was not called")
	}
	if got.Message.Text != "hello" {
		t.Fatalf("text = %q", got.Message.Text)
	}
	if got.ReplyTarget != "12345678900@s.whatsapp.net" {
		t.Fatalf("reply target = %q", got.ReplyTarget)
	}
	if got.Conversation.Type != channel.ConversationTypePrivate {
		t.Fatalf("conversation type = %q", got.Conversation.Type)
	}
	if got.Sender.Attribute("phone") != "12345678900" {
		t.Fatalf("sender phone = %q", got.Sender.Attribute("phone"))
	}
}

func TestHandleMessageMapsPrivateReplyContext(t *testing.T) {
	adapter := NewAdapter(nil, t.TempDir())
	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: Type}
	chat := types.NewJID("12345678900", types.DefaultUserServer)
	var got channel.InboundMessage
	adapter.handleMessage(context.Background(), cfg, &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:   chat,
				Sender: chat,
			},
			ID:       "MSGID",
			PushName: "Alice",
		},
		Message: &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String("reply text"),
			ContextInfo: &waE2E.ContextInfo{
				StanzaID:    proto.String("quoted-id"),
				RemoteJID:   proto.String("12345678900@s.whatsapp.net"),
				Participant: proto.String("12345678900@s.whatsapp.net"),
				QuotedMessage: &waE2E.Message{
					Conversation: proto.String("quoted preview"),
				},
			},
		}},
	}, func(_ context.Context, _ channel.ChannelConfig, msg channel.InboundMessage) error {
		got = msg
		return nil
	})
	if got.Message.Reply == nil {
		t.Fatal("reply ref was not mapped")
	}
	if got.Message.Reply.MessageID != "quoted-id" {
		t.Fatalf("reply message id = %q", got.Message.Reply.MessageID)
	}
	if got.Message.Reply.Target != "12345678900@s.whatsapp.net" {
		t.Fatalf("reply target = %q", got.Message.Reply.Target)
	}
	if got.Message.Reply.Preview != "quoted preview" {
		t.Fatalf("reply preview = %q", got.Message.Reply.Preview)
	}
}

func TestHandleMessageMapsPrivateImage(t *testing.T) {
	adapter := NewAdapter(nil, t.TempDir())
	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: Type}
	chat := types.NewJID("12345678900", types.DefaultUserServer)
	mediaKey := []byte("media-key")
	fileSHA := []byte("file-sha")
	fileEncSHA := []byte("file-enc-sha")
	var got channel.InboundMessage
	called := false
	adapter.handleMessage(context.Background(), cfg, &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:   chat,
				Sender: chat,
			},
			ID:        "IMGID",
			PushName:  "Alice",
			Timestamp: time.Unix(123, 0).UTC(),
		},
		Message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			Caption:       proto.String(" photo caption "),
			DirectPath:    proto.String("/v/t62.7118-24/image"),
			Mimetype:      proto.String("image/jpeg"),
			MediaKey:      mediaKey,
			FileSHA256:    fileSHA,
			FileEncSHA256: fileEncSHA,
			FileLength:    proto.Uint64(1234),
			Width:         proto.Uint32(640),
			Height:        proto.Uint32(480),
		}},
	}, func(_ context.Context, _ channel.ChannelConfig, msg channel.InboundMessage) error {
		called = true
		got = msg
		return nil
	})
	if !called {
		t.Fatal("handler was not called")
	}
	if got.Message.Text != "photo caption" {
		t.Fatalf("text = %q", got.Message.Text)
	}
	if len(got.Message.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(got.Message.Attachments))
	}
	att := got.Message.Attachments[0]
	if att.Type != channel.AttachmentImage {
		t.Fatalf("attachment type = %q", att.Type)
	}
	if att.PlatformKey != "/v/t62.7118-24/image" {
		t.Fatalf("platform key = %q", att.PlatformKey)
	}
	if att.SourcePlatform != Type.String() {
		t.Fatalf("source platform = %q", att.SourcePlatform)
	}
	if att.Mime != "image/jpeg" || att.Size != 1234 || att.Width != 640 || att.Height != 480 {
		t.Fatalf("attachment metadata = mime:%q size:%d width:%d height:%d", att.Mime, att.Size, att.Width, att.Height)
	}
	if att.Caption != "photo caption" {
		t.Fatalf("caption = %q", att.Caption)
	}
	if got := att.Metadata["media_key"]; got != base64.StdEncoding.EncodeToString(mediaKey) {
		t.Fatalf("media key metadata = %#v", got)
	}
	if got := att.Metadata["file_sha256"]; got != base64.StdEncoding.EncodeToString(fileSHA) {
		t.Fatalf("file sha metadata = %#v", got)
	}
	if got := att.Metadata["file_enc_sha256"]; got != base64.StdEncoding.EncodeToString(fileEncSHA) {
		t.Fatalf("file enc sha metadata = %#v", got)
	}
}

func TestHandleMessageMapsPrivateDocument(t *testing.T) {
	adapter := NewAdapter(nil, t.TempDir())
	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: Type}
	chat := types.NewJID("12345678900", types.DefaultUserServer)
	mediaKey := []byte("media-key")
	fileSHA := []byte("file-sha")
	fileEncSHA := []byte("file-enc-sha")
	var got channel.InboundMessage
	called := false
	adapter.handleMessage(context.Background(), cfg, &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:   chat,
				Sender: chat,
			},
			ID:       "DOCID",
			PushName: "Alice",
		},
		Message: &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
			Caption:       proto.String(" document caption "),
			DirectPath:    proto.String("/v/t62.7119-24/document"),
			Mimetype:      proto.String("application/pdf"),
			FileName:      proto.String("report.pdf"),
			Title:         proto.String("report.pdf"),
			MediaKey:      mediaKey,
			FileSHA256:    fileSHA,
			FileEncSHA256: fileEncSHA,
			FileLength:    proto.Uint64(4567),
		}},
	}, func(_ context.Context, _ channel.ChannelConfig, msg channel.InboundMessage) error {
		called = true
		got = msg
		return nil
	})
	if !called {
		t.Fatal("handler was not called")
	}
	if got.Message.Text != "document caption" {
		t.Fatalf("text = %q", got.Message.Text)
	}
	if len(got.Message.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(got.Message.Attachments))
	}
	att := got.Message.Attachments[0]
	if att.Type != channel.AttachmentFile {
		t.Fatalf("attachment type = %q", att.Type)
	}
	if att.PlatformKey != "/v/t62.7119-24/document" {
		t.Fatalf("platform key = %q", att.PlatformKey)
	}
	if att.Name != "report.pdf" || att.Mime != "application/pdf" || att.Size != 4567 {
		t.Fatalf("attachment metadata = name:%q mime:%q size:%d", att.Name, att.Mime, att.Size)
	}
	if got := att.Metadata["media_type"]; got != "document" {
		t.Fatalf("media type metadata = %#v", got)
	}
	if got := att.Metadata["file_name"]; got != "report.pdf" {
		t.Fatalf("file name metadata = %#v", got)
	}
	if got := att.Metadata["media_key"]; got != base64.StdEncoding.EncodeToString(mediaKey) {
		t.Fatalf("media key metadata = %#v", got)
	}
}

func TestHandleMessageMapsPrivateLIDText(t *testing.T) {
	adapter := NewAdapter(nil, t.TempDir())
	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: Type}
	chat := types.NewJID("220701894168771", types.HiddenUserServer)
	var got channel.InboundMessage
	called := false
	adapter.handleMessage(context.Background(), cfg, &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:   chat,
				Sender: chat,
			},
			ID:       "LIDMSG",
			PushName: "LID User",
		},
		Message: &waE2E.Message{Conversation: proto.String("hello from lid")},
	}, func(_ context.Context, _ channel.ChannelConfig, msg channel.InboundMessage) error {
		called = true
		got = msg
		return nil
	})
	if !called {
		t.Fatal("handler was not called")
	}
	if got.ReplyTarget != "220701894168771@lid" {
		t.Fatalf("reply target = %q", got.ReplyTarget)
	}
	if got.Sender.Attribute("jid") != "220701894168771@lid" {
		t.Fatalf("sender jid = %q", got.Sender.Attribute("jid"))
	}
	if got.Sender.Attribute("lid") != "220701894168771" {
		t.Fatalf("sender lid = %q", got.Sender.Attribute("lid"))
	}
	if got.Sender.Attribute("phone") != "" {
		t.Fatalf("sender phone = %q, want empty for LID", got.Sender.Attribute("phone"))
	}
	if got.Conversation.Metadata["lid"] != "220701894168771" {
		t.Fatalf("conversation lid = %#v", got.Conversation.Metadata["lid"])
	}
}

func TestHandleMessageFiltersUnsupportedChats(t *testing.T) {
	adapter := NewAdapter(nil, t.TempDir())
	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: Type}
	called := false
	handler := func(context.Context, channel.ChannelConfig, channel.InboundMessage) error {
		called = true
		return nil
	}
	adapter.handleMessage(context.Background(), cfg, &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:    types.NewJID("12345", types.GroupServer),
				Sender:  types.NewJID("12345678900", types.DefaultUserServer),
				IsGroup: true,
			},
			ID: "GROUP",
		},
		Message: &waE2E.Message{Conversation: proto.String("group")},
	}, handler)
	adapter.handleMessage(context.Background(), cfg, &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     types.NewJID("12345678900", types.DefaultUserServer),
				Sender:   types.NewJID("12345678900", types.DefaultUserServer),
				IsFromMe: true,
			},
			ID: "SELF",
		},
		Message: &waE2E.Message{Conversation: proto.String("self")},
	}, handler)
	adapter.handleMessage(context.Background(), cfg, &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: types.StatusBroadcastJID},
			ID:            "STATUS",
		},
		Message: &waE2E.Message{Conversation: proto.String("status")},
	}, handler)
	if called {
		t.Fatal("handler called for unsupported chat")
	}
}

func TestResolveWhatsAppAttachmentDownloadsImage(t *testing.T) {
	mediaKey := []byte("media-key")
	fileSHA := []byte("file-sha")
	fileEncSHA := []byte("file-enc-sha")
	att, ok := imageAttachment(&waE2E.ImageMessage{
		DirectPath:    proto.String("/v/t62.7118-24/image"),
		Mimetype:      proto.String("image/png"),
		MediaKey:      mediaKey,
		FileSHA256:    fileSHA,
		FileEncSHA256: fileEncSHA,
		FileLength:    proto.Uint64(3),
		Width:         proto.Uint32(2),
		Height:        proto.Uint32(1),
	})
	if !ok {
		t.Fatal("image attachment was not built")
	}
	downloader := &fakeWhatsAppDownloader{data: []byte{1, 2, 3}}
	payload, err := resolveWhatsAppAttachment(context.Background(), downloader, att)
	if err != nil {
		t.Fatalf("resolve attachment: %v", err)
	}
	defer func() {
		_ = payload.Reader.Close()
	}()
	data, err := io.ReadAll(payload.Reader)
	if err != nil {
		t.Fatalf("read payload: %v", err)
	}
	if !bytes.Equal(data, []byte{1, 2, 3}) {
		t.Fatalf("payload = %v", data)
	}
	if payload.Mime != "image/png" || payload.Name != "whatsapp-image.png" || payload.Size != 3 {
		t.Fatalf("payload metadata = mime:%q name:%q size:%d", payload.Mime, payload.Name, payload.Size)
	}
	image, ok := downloader.got.(*waE2E.ImageMessage)
	if !ok {
		t.Fatalf("download message = %T", downloader.got)
	}
	if image.GetDirectPath() != "/v/t62.7118-24/image" {
		t.Fatalf("direct path = %q", image.GetDirectPath())
	}
	if !bytes.Equal(image.GetMediaKey(), mediaKey) {
		t.Fatalf("media key = %q", image.GetMediaKey())
	}
	if !bytes.Equal(image.GetFileSHA256(), fileSHA) {
		t.Fatalf("file sha = %q", image.GetFileSHA256())
	}
	if !bytes.Equal(image.GetFileEncSHA256(), fileEncSHA) {
		t.Fatalf("file enc sha = %q", image.GetFileEncSHA256())
	}
}

func TestResolveWhatsAppAttachmentDownloadsDocument(t *testing.T) {
	mediaKey := []byte("media-key")
	fileSHA := []byte("file-sha")
	fileEncSHA := []byte("file-enc-sha")
	att, ok := documentAttachment(&waE2E.DocumentMessage{
		DirectPath:    proto.String("/v/t62.7119-24/document"),
		Mimetype:      proto.String("application/pdf"),
		FileName:      proto.String("report.pdf"),
		MediaKey:      mediaKey,
		FileSHA256:    fileSHA,
		FileEncSHA256: fileEncSHA,
		FileLength:    proto.Uint64(4),
	})
	if !ok {
		t.Fatal("document attachment was not built")
	}
	downloader := &fakeWhatsAppDownloader{data: []byte{1, 2, 3, 4}}
	payload, err := resolveWhatsAppAttachment(context.Background(), downloader, att)
	if err != nil {
		t.Fatalf("resolve attachment: %v", err)
	}
	defer func() {
		_ = payload.Reader.Close()
	}()
	data, err := io.ReadAll(payload.Reader)
	if err != nil {
		t.Fatalf("read payload: %v", err)
	}
	if !bytes.Equal(data, []byte{1, 2, 3, 4}) {
		t.Fatalf("payload = %v", data)
	}
	if payload.Mime != "application/pdf" || payload.Name != "report.pdf" || payload.Size != 4 {
		t.Fatalf("payload metadata = mime:%q name:%q size:%d", payload.Mime, payload.Name, payload.Size)
	}
	document, ok := downloader.got.(*waE2E.DocumentMessage)
	if !ok {
		t.Fatalf("download message = %T", downloader.got)
	}
	if document.GetDirectPath() != "/v/t62.7119-24/document" {
		t.Fatalf("direct path = %q", document.GetDirectPath())
	}
	if document.GetFileName() != "report.pdf" {
		t.Fatalf("file name = %q", document.GetFileName())
	}
	if !bytes.Equal(document.GetMediaKey(), mediaKey) {
		t.Fatalf("media key = %q", document.GetMediaKey())
	}
	if !bytes.Equal(document.GetFileSHA256(), fileSHA) {
		t.Fatalf("file sha = %q", document.GetFileSHA256())
	}
	if !bytes.Equal(document.GetFileEncSHA256(), fileEncSHA) {
		t.Fatalf("file enc sha = %q", document.GetFileEncSHA256())
	}
}

func TestResolveWhatsAppAttachmentAllowsUnknownSize(t *testing.T) {
	att, ok := imageAttachment(&waE2E.ImageMessage{
		DirectPath:    proto.String("/v/t62.7118-24/image"),
		Mimetype:      proto.String("image/png"),
		MediaKey:      []byte("media-key"),
		FileSHA256:    []byte("file-sha"),
		FileEncSHA256: []byte("file-enc-sha"),
	})
	if !ok {
		t.Fatal("image attachment was not built")
	}
	downloader := &fakeWhatsAppDownloader{data: []byte{1, 2, 3}}
	payload, err := resolveWhatsAppAttachment(context.Background(), downloader, att)
	if err != nil {
		t.Fatalf("resolve unknown-size attachment: %v", err)
	}
	if downloader.got != nil {
		image, ok := downloader.got.(*waE2E.ImageMessage)
		if !ok || image.GetDirectPath() != "/v/t62.7118-24/image" {
			t.Fatalf("download message = %T", downloader.got)
		}
	} else {
		t.Fatal("download was not attempted for unknown-size attachment")
	}
	defer func() {
		_ = payload.Reader.Close()
	}()
	data, err := io.ReadAll(payload.Reader)
	if err != nil {
		t.Fatalf("read payload: %v", err)
	}
	if !bytes.Equal(data, []byte{1, 2, 3}) {
		t.Fatalf("payload data = %v", data)
	}
	if payload.Size != 3 {
		t.Fatalf("payload size = %d", payload.Size)
	}
}

func TestResolveWhatsAppAttachmentRejectsOversizedUnknownSizeDownload(t *testing.T) {
	att, ok := imageAttachment(&waE2E.ImageMessage{
		DirectPath:    proto.String("/v/t62.7118-24/image"),
		MediaKey:      []byte("media-key"),
		FileSHA256:    []byte("file-sha"),
		FileEncSHA256: []byte("file-enc-sha"),
	})
	if !ok {
		t.Fatal("image attachment was not built")
	}
	downloader := &fakeWhatsAppDownloader{data: []byte{1, 2, 3, 4, 5}}
	_, err := resolveWhatsAppAttachmentWithLimit(context.Background(), downloader, att, 4)
	if !errors.Is(err, media.ErrAssetTooLarge) {
		t.Fatalf("resolve oversized attachment error = %v", err)
	}
}

func TestCappedWhatsAppFileStopsWritesBeyondLimit(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "capped-whatsapp-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer func() {
		_ = tmp.Close()
	}()
	file := &cappedWhatsAppFile{file: tmp, writeLimit: 4, assetLimit: 3}
	if _, ok := any(file).(io.ReaderFrom); ok {
		t.Fatal("capped file must not expose os.File ReadFrom")
	}
	n, err := file.Write([]byte{1, 2, 3, 4, 5})
	if !errors.Is(err, media.ErrAssetTooLarge) {
		t.Fatalf("write error = %v", err)
	}
	if n != 4 {
		t.Fatalf("written bytes = %d, want 4", n)
	}
	info, err := tmp.Stat()
	if err != nil {
		t.Fatalf("stat temp file: %v", err)
	}
	if info.Size() != 4 {
		t.Fatalf("temp file size = %d, want 4", info.Size())
	}
}

func TestSendWhatsAppImageAttachmentUploadsImage(t *testing.T) {
	sender := &fakeWhatsAppImageSender{}
	jid := types.NewJID("12345678900", types.DefaultUserServer)
	raw := []byte{0x89, 'P', 'N', 'G'}
	err := sendWhatsAppImageAttachment(context.Background(), sender, jid, channel.PreparedAttachment{
		Logical: channel.Attachment{
			Type:   channel.AttachmentImage,
			Width:  2,
			Height: 3,
		},
		Kind: channel.PreparedAttachmentUpload,
		Mime: "image/png",
		Open: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(raw)), nil
		},
	}, " caption ", nil)
	if err != nil {
		t.Fatalf("send image attachment: %v", err)
	}
	if sender.uploadType != whatsmeow.MediaImage {
		t.Fatalf("upload type = %q", sender.uploadType)
	}
	if !bytes.Equal(sender.uploadData, raw) {
		t.Fatalf("upload data = %v", sender.uploadData)
	}
	if sender.sentTo != jid {
		t.Fatalf("sent jid = %s", sender.sentTo)
	}
	image := sender.sentMessage.GetImageMessage()
	if image == nil {
		t.Fatal("image message was not sent")
	}
	if image.GetCaption() != "caption" {
		t.Fatalf("caption = %q", image.GetCaption())
	}
	if image.GetMimetype() != "image/png" {
		t.Fatalf("mime = %q", image.GetMimetype())
	}
	if image.GetDirectPath() != "/v/t62.7118-24/uploaded-image" || image.GetURL() != "https://example.test/image" {
		t.Fatalf("uploaded refs = url:%q direct:%q", image.GetURL(), image.GetDirectPath())
	}
	if image.GetFileLength() != uint64(len(raw)) || image.GetWidth() != 2 || image.GetHeight() != 3 {
		t.Fatalf("image metadata = len:%d width:%d height:%d", image.GetFileLength(), image.GetWidth(), image.GetHeight())
	}
}

func TestSendWhatsAppDocumentAttachmentUploadsDocument(t *testing.T) {
	sender := &fakeWhatsAppImageSender{}
	jid := types.NewJID("12345678900", types.DefaultUserServer)
	raw := []byte("%PDF")
	err := sendWhatsAppAttachment(context.Background(), sender, jid, channel.PreparedAttachment{
		Logical: channel.Attachment{
			Type: channel.AttachmentFile,
			Name: "report.pdf",
		},
		Kind: channel.PreparedAttachmentUpload,
		Mime: "application/pdf",
		Open: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(raw)), nil
		},
	}, " caption ", nil)
	if err != nil {
		t.Fatalf("send document attachment: %v", err)
	}
	if sender.uploadType != whatsmeow.MediaDocument {
		t.Fatalf("upload type = %q", sender.uploadType)
	}
	if !bytes.Equal(sender.uploadData, raw) {
		t.Fatalf("upload data = %v", sender.uploadData)
	}
	document := sender.sentMessage.GetDocumentMessage()
	if document == nil {
		t.Fatal("document message was not sent")
	}
	if document.GetCaption() != "caption" {
		t.Fatalf("caption = %q", document.GetCaption())
	}
	if document.GetMimetype() != "application/pdf" {
		t.Fatalf("mime = %q", document.GetMimetype())
	}
	if document.GetFileName() != "report.pdf" || document.GetTitle() != "report.pdf" {
		t.Fatalf("document name/title = %q/%q", document.GetFileName(), document.GetTitle())
	}
	if document.GetDirectPath() != "/v/t62.7118-24/uploaded-image" || document.GetURL() != "https://example.test/image" {
		t.Fatalf("uploaded refs = url:%q direct:%q", document.GetURL(), document.GetDirectPath())
	}
	if document.GetFileLength() != uint64(len(raw)) {
		t.Fatalf("document length = %d", document.GetFileLength())
	}
}

func TestSendWhatsAppAttachmentRejectsGIF(t *testing.T) {
	sender := &fakeWhatsAppImageSender{}
	jid := types.NewJID("12345678900", types.DefaultUserServer)
	err := sendWhatsAppAttachment(context.Background(), sender, jid, channel.PreparedAttachment{
		Logical: channel.Attachment{Type: channel.AttachmentGIF},
		Kind:    channel.PreparedAttachmentUpload,
		Mime:    "image/gif",
		Open: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("gif"))), nil
		},
	}, "", nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported attachment type") {
		t.Fatalf("gif send error = %v", err)
	}
	if sender.sentMessage != nil || len(sender.uploadData) > 0 {
		t.Fatal("gif attachment should not be uploaded or sent")
	}
}

func TestSendWhatsAppPreparedMessageSkipsUnsupportedFirstAttachmentAndPreservesText(t *testing.T) {
	sender := &fakeWhatsAppImageSender{}
	jid := types.NewJID("12345678900", types.DefaultUserServer)
	raw := []byte{0x89, 'P', 'N', 'G'}
	err := sendWhatsAppPreparedMessage(context.Background(), nil, sender, jid, channel.PreparedMessage{
		Message: channel.Message{Format: channel.MessageFormatPlain, Text: " hello "},
		Attachments: []channel.PreparedAttachment{
			{
				Logical: channel.Attachment{Type: channel.AttachmentFile, Name: "clip.mp4"},
				Kind:    channel.PreparedAttachmentUpload,
				Mime:    "video/mp4",
				Open: func(context.Context) (io.ReadCloser, error) {
					t.Fatal("unsupported attachment should not be opened")
					return nil, nil
				},
			},
			{
				Logical: channel.Attachment{Type: channel.AttachmentImage, Name: "image.png"},
				Kind:    channel.PreparedAttachmentUpload,
				Mime:    "image/png",
				Open: func(context.Context) (io.ReadCloser, error) {
					return io.NopCloser(bytes.NewReader(raw)), nil
				},
			},
		},
	}, "cfg-1", "bot-1")
	if err != nil {
		t.Fatalf("send prepared message: %v", err)
	}
	if !bytes.Equal(sender.uploadData, raw) {
		t.Fatalf("upload data = %v", sender.uploadData)
	}
	image := sender.sentMessage.GetImageMessage()
	if image == nil {
		t.Fatal("image message was not sent")
	}
	if image.GetCaption() != "hello" {
		t.Fatalf("image caption = %q", image.GetCaption())
	}
}

func TestSendWhatsAppPreparedMessageSendsTextWhenOnlyUnsupportedAttachments(t *testing.T) {
	sender := &fakeWhatsAppImageSender{}
	jid := types.NewJID("12345678900", types.DefaultUserServer)
	err := sendWhatsAppPreparedMessage(context.Background(), nil, sender, jid, channel.PreparedMessage{
		Message: channel.Message{Format: channel.MessageFormatPlain, Text: " hello "},
		Attachments: []channel.PreparedAttachment{
			{
				Logical: channel.Attachment{Type: channel.AttachmentFile, Name: "clip.mp4"},
				Kind:    channel.PreparedAttachmentUpload,
				Mime:    "video/mp4",
				Open: func(context.Context) (io.ReadCloser, error) {
					t.Fatal("unsupported attachment should not be opened")
					return nil, nil
				},
			},
		},
	}, "cfg-1", "bot-1")
	if err != nil {
		t.Fatalf("send prepared message: %v", err)
	}
	if len(sender.uploadData) > 0 {
		t.Fatalf("unsupported attachment was uploaded: %v", sender.uploadData)
	}
	if got := sender.sentMessage.GetConversation(); got != "hello" {
		t.Fatalf("text message = %q", got)
	}
}

func TestWhatsAppReplyContextUsesBarePrivateTarget(t *testing.T) {
	fallback := types.NewJID("12345678900", types.DefaultUserServer)
	ctx := whatsAppReplyContext(&channel.ReplyRef{
		Target:    "09876543210:2@s.whatsapp.net",
		MessageID: " quoted-id ",
		Preview:   " hello ",
	}, fallback)
	if ctx == nil {
		t.Fatal("reply context is nil")
	}
	if ctx.GetStanzaID() != "quoted-id" {
		t.Fatalf("stanza id = %q", ctx.GetStanzaID())
	}
	if ctx.GetRemoteJID() != "12345678900@s.whatsapp.net" {
		t.Fatalf("remote jid = %q, want fallback when target has a device id", ctx.GetRemoteJID())
	}
	if ctx.GetParticipant() != "12345678900@s.whatsapp.net" {
		t.Fatalf("participant = %q", ctx.GetParticipant())
	}
	if ctx.GetQuotedMessage().GetConversation() != "hello" {
		t.Fatalf("quoted preview = %q", ctx.GetQuotedMessage().GetConversation())
	}

	ctx = whatsAppReplyContext(&channel.ReplyRef{
		Target:    "09876543210@s.whatsapp.net",
		MessageID: "msg-2",
	}, fallback)
	if ctx.GetRemoteJID() != "09876543210@s.whatsapp.net" {
		t.Fatalf("remote jid = %q", ctx.GetRemoteJID())
	}
}

func TestSendWhatsAppImageAttachmentIncludesReplyContext(t *testing.T) {
	sender := &fakeWhatsAppImageSender{}
	jid := types.NewJID("12345678900", types.DefaultUserServer)
	reply := whatsAppReplyContext(&channel.ReplyRef{MessageID: "msg-1"}, jid)
	err := sendWhatsAppAttachment(context.Background(), sender, jid, channel.PreparedAttachment{
		Logical: channel.Attachment{Type: channel.AttachmentImage},
		Kind:    channel.PreparedAttachmentUpload,
		Mime:    "image/png",
		Open: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte{0x89, 'P', 'N', 'G'})), nil
		},
	}, "", reply)
	if err != nil {
		t.Fatalf("send image attachment: %v", err)
	}
	image := sender.sentMessage.GetImageMessage()
	if image == nil || image.GetContextInfo().GetStanzaID() != "msg-1" {
		t.Fatalf("image reply context = %#v", image.GetContextInfo())
	}
}

func TestFinalOnlyStreamSendsBufferedAttachmentsWithFinal(t *testing.T) {
	var got channel.PreparedOutboundMessage
	stream := &finalOnlyStream{
		cfg: channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1"},
		to:  "12345678900@s.whatsapp.net",
		send: func(_ context.Context, _ channel.ChannelConfig, msg channel.PreparedOutboundMessage) error {
			got = msg
			return nil
		},
	}
	buffered := channel.PreparedAttachment{
		Logical: channel.Attachment{Type: channel.AttachmentImage, Name: "image.png"},
		Kind:    channel.PreparedAttachmentUpload,
		Name:    "image.png",
	}
	final := channel.PreparedAttachment{
		Logical: channel.Attachment{Type: channel.AttachmentFile, Name: "final.pdf"},
		Kind:    channel.PreparedAttachmentUpload,
		Name:    "final.pdf",
	}
	if err := stream.Push(context.Background(), channel.PreparedStreamEvent{
		Type:        channel.StreamEventAttachment,
		Attachments: []channel.PreparedAttachment{buffered},
	}); err != nil {
		t.Fatalf("push attachment: %v", err)
	}
	if err := stream.Push(context.Background(), channel.PreparedStreamEvent{
		Type: channel.StreamEventFinal,
		Final: &channel.PreparedStreamFinalizePayload{Message: channel.PreparedMessage{
			Message:     channel.Message{Format: channel.MessageFormatPlain, Text: "done"},
			Attachments: []channel.PreparedAttachment{final},
		}},
	}); err != nil {
		t.Fatalf("push final: %v", err)
	}
	if got.Target != "12345678900@s.whatsapp.net" {
		t.Fatalf("target = %q", got.Target)
	}
	if len(got.Message.Attachments) != 2 || got.Message.Attachments[0].Name != "image.png" || got.Message.Attachments[1].Name != "final.pdf" {
		t.Fatalf("prepared attachments = %#v", got.Message.Attachments)
	}
	if len(got.Message.Message.Attachments) != 2 || got.Message.Message.Attachments[0].Name != "image.png" || got.Message.Message.Attachments[1].Name != "final.pdf" {
		t.Fatalf("logical attachments = %#v", got.Message.Message.Attachments)
	}
	if err := stream.Close(context.Background()); err != nil {
		t.Fatalf("close stream: %v", err)
	}
}

func TestFinalOnlyStreamFlushesDeltaOnEmptyFinalClose(t *testing.T) {
	var got channel.PreparedOutboundMessage
	stream := &finalOnlyStream{
		cfg: channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1"},
		to:  "12345678900@s.whatsapp.net",
		send: func(_ context.Context, _ channel.ChannelConfig, msg channel.PreparedOutboundMessage) error {
			got = msg
			return nil
		},
	}
	if err := stream.Push(context.Background(), channel.PreparedStreamEvent{Type: channel.StreamEventDelta, Delta: "hello "}); err != nil {
		t.Fatalf("push delta 1: %v", err)
	}
	if err := stream.Push(context.Background(), channel.PreparedStreamEvent{Type: channel.StreamEventDelta, Delta: "world"}); err != nil {
		t.Fatalf("push delta 2: %v", err)
	}
	if err := stream.Push(context.Background(), channel.PreparedStreamEvent{
		Type:  channel.StreamEventFinal,
		Final: &channel.PreparedStreamFinalizePayload{},
	}); err != nil {
		t.Fatalf("push empty final: %v", err)
	}
	if got.Message.Message.Text != "hello world" {
		t.Fatalf("sent text = %q", got.Message.Message.Text)
	}
	if err := stream.Close(context.Background()); err != nil {
		t.Fatalf("close stream: %v", err)
	}
}

func TestFinalOnlyStreamFinalPushSendsBeforeClose(t *testing.T) {
	var sent []string
	stream := &finalOnlyStream{
		cfg: channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1"},
		to:  "12345678900@s.whatsapp.net",
		send: func(_ context.Context, _ channel.ChannelConfig, msg channel.PreparedOutboundMessage) error {
			sent = append(sent, msg.Message.Message.Text)
			return nil
		},
	}
	if err := stream.Push(context.Background(), channel.PreparedStreamEvent{
		Type: channel.StreamEventFinal,
		Final: &channel.PreparedStreamFinalizePayload{Message: channel.PreparedMessage{
			Message: channel.Message{Format: channel.MessageFormatPlain, Text: "first chunk"},
		}},
	}); err != nil {
		t.Fatalf("push final: %v", err)
	}
	if len(sent) != 1 || sent[0] != "first chunk" {
		t.Fatalf("sent before close = %#v", sent)
	}
	if err := stream.Close(context.Background()); err != nil {
		t.Fatalf("close stream: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("close resent messages: %#v", sent)
	}
}

func TestFinalOnlyStreamFinalSendErrorDoesNotFlushOnClose(t *testing.T) {
	sendErr := errors.New("send failed")
	var sendCount int
	stream := &finalOnlyStream{
		cfg: channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1"},
		to:  "12345678900@s.whatsapp.net",
		send: func(_ context.Context, _ channel.ChannelConfig, _ channel.PreparedOutboundMessage) error {
			sendCount++
			return sendErr
		},
	}
	if err := stream.Push(context.Background(), channel.PreparedStreamEvent{Type: channel.StreamEventDelta, Delta: "hello"}); err != nil {
		t.Fatalf("push delta: %v", err)
	}
	err := stream.Push(context.Background(), channel.PreparedStreamEvent{
		Type: channel.StreamEventFinal,
		Final: &channel.PreparedStreamFinalizePayload{Message: channel.PreparedMessage{
			Message: channel.Message{Format: channel.MessageFormatPlain, Text: "final"},
		}},
	})
	if !errors.Is(err, sendErr) {
		t.Fatalf("final push error = %v", err)
	}
	if err := stream.Close(context.Background()); err != nil {
		t.Fatalf("close stream: %v", err)
	}
	if sendCount != 1 {
		t.Fatalf("send count = %d, want 1", sendCount)
	}
}

func TestFinalOnlyStreamUsesReplyOptionFallback(t *testing.T) {
	var got channel.PreparedOutboundMessage
	stream := &finalOnlyStream{
		cfg:   channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1"},
		to:    "12345678900@s.whatsapp.net",
		reply: &channel.ReplyRef{Target: "12345678900@s.whatsapp.net", MessageID: "source-msg"},
		send: func(_ context.Context, _ channel.ChannelConfig, msg channel.PreparedOutboundMessage) error {
			got = msg
			return nil
		},
	}
	if err := stream.Push(context.Background(), channel.PreparedStreamEvent{Type: channel.StreamEventDelta, Delta: "hello"}); err != nil {
		t.Fatalf("push delta: %v", err)
	}
	if err := stream.Push(context.Background(), channel.PreparedStreamEvent{Type: channel.StreamEventFinal, Final: &channel.PreparedStreamFinalizePayload{}}); err != nil {
		t.Fatalf("push final: %v", err)
	}
	if got.Message.Message.Reply == nil || got.Message.Message.Reply.MessageID != "source-msg" {
		t.Fatalf("reply fallback = %#v", got.Message.Message.Reply)
	}
}

func TestPhonePairingCodeStatusSurvivesQRRefresh(t *testing.T) {
	p := &pendingLogin{
		Mode:     pendingModePhone,
		Status:   "pair_code",
		PairCode: "ABCD-EFGH",
	}
	(&Service{}).applyQRItem(p, whatsmeow.QRChannelItem{
		Event: whatsmeow.QRChannelEventCode,
		Code:  "ignored-qr-code",
	})
	if p.Status != "pair_code" {
		t.Fatalf("status = %q, want pair_code", p.Status)
	}
	if p.PairCode != "ABCD-EFGH" {
		t.Fatalf("pair code changed to %q", p.PairCode)
	}
}

func TestPollQRReturnsExpiredBeforeRemovingPending(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	svc := &Service{
		dataRoot: t.TempDir(),
		pending:  map[string]*pendingLogin{},
		now:      func() time.Time { return now },
	}
	p := &pendingLogin{
		ID:        "login-1",
		BotID:     "bot-1",
		Mode:      pendingModeQR,
		Status:    StatusQRPending,
		ExpiresAt: now.Add(-time.Second),
	}
	svc.pending[p.ID] = p
	resp, err := svc.PollQR(context.Background(), "bot-1", "login-1")
	if err != nil {
		t.Fatalf("poll expired qr: %v", err)
	}
	if resp.Status != "expired" {
		t.Fatalf("status = %q", resp.Status)
	}
	if svc.getPending("login-1") != nil {
		t.Fatal("expired pending login was not removed after response")
	}
}

func TestLogoutWithRelinkOnlyConfigDoesNotCreateStrayStore(t *testing.T) {
	root := t.TempDir()
	adapter := NewAdapter(nil, root)
	err := adapter.Logout(context.Background(), channel.ChannelConfig{
		ID:          "cfg-1",
		BotID:       "bot-1",
		ChannelType: Type,
		Credentials: map[string]any{"needsRelink": true},
	})
	if err != nil {
		t.Fatalf("logout relink-only config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "channels", "whatsapp", "store.db")); !os.IsNotExist(err) {
		t.Fatalf("stray store.db was created or unexpected stat err: %v", err)
	}
}

func TestMoveStoreSecuresDirectoryAndFiles(t *testing.T) {
	root := t.TempDir()
	src := pendingStorePaths(root, "login-1")
	dst := finalStorePaths(root, "cfg-1")
	if err := ensureStoreDir(src.Dir); err != nil {
		t.Fatalf("ensure src: %v", err)
	}
	for _, path := range []string{src.DB, src.DB + "-wal", src.DB + "-shm"} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if err := moveStore(src, dst); err != nil {
		t.Fatalf("move store: %v", err)
	}
	if _, err := os.Stat(src.Dir); !os.IsNotExist(err) {
		t.Fatalf("source dir still exists or unexpected stat err: %v", err)
	}
	assertMode(t, dst.Dir, 0o700)
	assertMode(t, dst.DB, 0o600)
	assertMode(t, dst.DB+"-wal", 0o600)
	assertMode(t, dst.DB+"-shm", 0o600)
	if filepath.Dir(dst.Dir) != filepath.Join(root, "channels", "whatsapp") {
		t.Fatalf("unexpected dst root: %s", dst.Dir)
	}
}

func TestReplaceStoreRollbackRestoresPreviousStore(t *testing.T) {
	root := t.TempDir()
	src := pendingStorePaths(root, "login-1")
	dst := finalStorePaths(root, "cfg-1")
	if err := ensureStoreDir(src.Dir); err != nil {
		t.Fatalf("ensure src: %v", err)
	}
	if err := os.WriteFile(src.DB, []byte("new"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := ensureStoreDir(dst.Dir); err != nil {
		t.Fatalf("ensure dst: %v", err)
	}
	if err := os.WriteFile(dst.DB, []byte("old"), 0o600); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	replacement, err := replaceStore(src, dst, "login-1")
	if err != nil {
		t.Fatalf("replace store: %v", err)
	}
	if got, err := os.ReadFile(dst.DB); err != nil || string(got) != "new" {
		t.Fatalf("dst after replace = %q, %v", got, err)
	}
	replacement.Rollback()
	if got, err := os.ReadFile(dst.DB); err != nil || string(got) != "old" {
		t.Fatalf("dst after rollback = %q, %v", got, err)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}

type fakeWhatsAppDownloader struct {
	got  whatsmeow.DownloadableMessage
	data []byte
	err  error
}

func (f *fakeWhatsAppDownloader) DownloadToFile(_ context.Context, msg whatsmeow.DownloadableMessage, file whatsmeow.File) error {
	f.got = msg
	if f.err != nil {
		return f.err
	}
	_, err := io.Copy(file, bytes.NewReader(f.data))
	return err
}

type fakeWhatsAppImageSender struct {
	uploadData  []byte
	uploadType  whatsmeow.MediaType
	sentTo      types.JID
	sentMessage *waE2E.Message
}

func (f *fakeWhatsAppImageSender) Upload(_ context.Context, plaintext []byte, appInfo whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
	f.uploadData = append([]byte(nil), plaintext...)
	f.uploadType = appInfo
	return whatsmeow.UploadResponse{
		URL:           "https://example.test/image",
		DirectPath:    "/v/t62.7118-24/uploaded-image",
		MediaKey:      []byte("media-key"),
		FileEncSHA256: []byte("file-enc-sha"),
		FileSHA256:    []byte("file-sha"),
		FileLength:    uint64(len(plaintext)),
	}, nil
}

func (f *fakeWhatsAppImageSender) SendMessage(_ context.Context, to types.JID, message *waE2E.Message, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	f.sentTo = to
	f.sentMessage = message
	return whatsmeow.SendResponse{}, nil
}
