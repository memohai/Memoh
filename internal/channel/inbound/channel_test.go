package inbound

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/acl"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/media"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/schedule"
)

type fakeChatGateway struct {
	resp   conversation.ChatResponse
	err    error
	gotReq conversation.ChatRequest
	onChat func(conversation.ChatRequest)
}

func (f *fakeChatGateway) Chat(_ context.Context, req conversation.ChatRequest) (conversation.ChatResponse, error) {
	f.gotReq = req
	if f.onChat != nil {
		f.onChat(req)
	}
	return f.resp, f.err
}

func (f *fakeChatGateway) StreamChat(_ context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
	f.gotReq = req
	if f.onChat != nil {
		f.onChat(req)
	}
	chunks := make(chan conversation.StreamChunk, 1)
	errs := make(chan error, 1)
	if f.err != nil {
		errs <- f.err
		close(chunks)
		close(errs)
		return chunks, errs
	}
	payload := map[string]any{
		"type":     "agent_end",
		"messages": f.resp.Messages,
	}
	data, err := json.Marshal(payload)
	if err == nil {
		chunks <- conversation.StreamChunk(data)
	}
	close(chunks)
	close(errs)
	return chunks, errs
}

func (*fakeChatGateway) TriggerSchedule(_ context.Context, _ string, _ schedule.TriggerPayload, _ string) (schedule.TriggerResult, error) {
	return schedule.TriggerResult{}, nil
}

type fakeReplySender struct {
	sent   []channel.OutboundMessage
	events []channel.StreamEvent
}

func (s *fakeReplySender) Send(_ context.Context, msg channel.OutboundMessage) error {
	s.sent = append(s.sent, msg)
	return nil
}

func (s *fakeReplySender) OpenStream(_ context.Context, target string, _ channel.StreamOptions) (channel.OutboundStream, error) {
	return &fakeOutboundStream{
		sender: s,
		target: strings.TrimSpace(target),
	}, nil
}

type fakeOutboundStream struct {
	sender *fakeReplySender
	target string
}

func (s *fakeOutboundStream) Push(_ context.Context, event channel.StreamEvent) error {
	if s == nil || s.sender == nil {
		return nil
	}
	s.sender.events = append(s.sender.events, event)
	if event.Type == channel.StreamEventFinal && event.Final != nil && !event.Final.Message.IsEmpty() {
		s.sender.sent = append(s.sender.sent, channel.OutboundMessage{
			Target:  s.target,
			Message: event.Final.Message,
		})
	}
	return nil
}

func (*fakeOutboundStream) Close(_ context.Context) error {
	return nil
}

type fakeProcessingStatusNotifier struct {
	startedHandle channel.ProcessingStatusHandle
	startedErr    error
	completedErr  error
	failedErr     error
	events        []string
	info          []channel.ProcessingStatusInfo
	completedSeen channel.ProcessingStatusHandle
	failedSeen    channel.ProcessingStatusHandle
	failedCause   error
}

func (n *fakeProcessingStatusNotifier) ProcessingStarted(_ context.Context, _ channel.ChannelConfig, _ channel.InboundMessage, info channel.ProcessingStatusInfo) (channel.ProcessingStatusHandle, error) {
	n.events = append(n.events, "started")
	n.info = append(n.info, info)
	return n.startedHandle, n.startedErr
}

func (n *fakeProcessingStatusNotifier) ProcessingCompleted(_ context.Context, _ channel.ChannelConfig, _ channel.InboundMessage, info channel.ProcessingStatusInfo, handle channel.ProcessingStatusHandle) error {
	n.events = append(n.events, "completed")
	n.info = append(n.info, info)
	n.completedSeen = handle
	return n.completedErr
}

func (n *fakeProcessingStatusNotifier) ProcessingFailed(_ context.Context, _ channel.ChannelConfig, _ channel.InboundMessage, info channel.ProcessingStatusInfo, handle channel.ProcessingStatusHandle, cause error) error {
	n.events = append(n.events, "failed")
	n.info = append(n.info, info)
	n.failedSeen = handle
	n.failedCause = cause
	return n.failedErr
}

type fakeProcessingStatusAdapter struct {
	notifier *fakeProcessingStatusNotifier
}

func (*fakeProcessingStatusAdapter) Type() channel.ChannelType {
	return channel.ChannelType("feishu")
}

func (*fakeProcessingStatusAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type: channel.ChannelType("feishu"),
		Capabilities: channel.ChannelCapabilities{
			Text:  true,
			Reply: true,
		},
	}
}

func (a *fakeProcessingStatusAdapter) ProcessingStarted(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, info channel.ProcessingStatusInfo) (channel.ProcessingStatusHandle, error) {
	return a.notifier.ProcessingStarted(ctx, cfg, msg, info)
}

func (a *fakeProcessingStatusAdapter) ProcessingCompleted(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, info channel.ProcessingStatusInfo, handle channel.ProcessingStatusHandle) error {
	return a.notifier.ProcessingCompleted(ctx, cfg, msg, info, handle)
}

func (a *fakeProcessingStatusAdapter) ProcessingFailed(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, info channel.ProcessingStatusInfo, handle channel.ProcessingStatusHandle, cause error) error {
	return a.notifier.ProcessingFailed(ctx, cfg, msg, info, handle, cause)
}

type fakeChatService struct {
	resolveResult route.ResolveConversationResult
	resolveErr    error
	persisted     []messagepkg.Message
	persistedIn   []messagepkg.PersistInput
}

type fakeChatACL struct {
	allowed bool
	err     error
	calls   int
	lastReq acl.EvaluateRequest
}

func (f *fakeChatACL) Evaluate(_ context.Context, req acl.EvaluateRequest) (bool, error) {
	f.calls++
	f.lastReq = req
	if f.err != nil {
		return false, f.err
	}
	return f.allowed, nil
}

type fakeMediaIngestor struct {
	nextID          string
	nextMime        string
	ingestErr       error
	calls           int
	inputs          []media.IngestInput
	payloads        [][]byte
	storageKeyAsset media.Asset
	storageKeyErr   error
}

func (f *fakeMediaIngestor) Stat(_ context.Context, _, contentHash string) (media.Asset, error) {
	asset := f.storageKeyAsset
	if asset.ContentHash == "" {
		asset = media.Asset{
			ContentHash: contentHash,
			Mime:        "application/octet-stream",
			StorageKey:  "test/" + contentHash,
		}
	}
	return asset, nil
}

func (f *fakeMediaIngestor) Open(_ context.Context, _, contentHash string) (io.ReadCloser, media.Asset, error) {
	asset := f.storageKeyAsset
	if asset.ContentHash == "" {
		asset = media.Asset{
			ContentHash: contentHash,
			Mime:        "application/octet-stream",
			StorageKey:  "test/" + contentHash,
		}
	}
	return io.NopCloser(bytes.NewReader([]byte("test"))), asset, nil
}

func (f *fakeMediaIngestor) Ingest(_ context.Context, input media.IngestInput) (media.Asset, error) {
	f.calls++
	f.inputs = append(f.inputs, input)
	if input.Reader != nil {
		payload, _ := io.ReadAll(input.Reader)
		f.payloads = append(f.payloads, payload)
	}
	if f.ingestErr != nil {
		return media.Asset{}, f.ingestErr
	}
	id := strings.TrimSpace(f.nextID)
	if id == "" {
		id = "asset-test-id"
	}
	mime := strings.TrimSpace(f.nextMime)
	if mime == "" {
		mime = strings.TrimSpace(input.Mime)
	}
	return media.Asset{
		ContentHash: id,
		Mime:        mime,
		StorageKey:  "test/" + id,
	}, nil
}

func (f *fakeMediaIngestor) GetByStorageKey(_ context.Context, _, _ string) (media.Asset, error) {
	return f.storageKeyAsset, f.storageKeyErr
}

func (*fakeMediaIngestor) IngestContainerFile(_ context.Context, _, _ string) (media.Asset, error) {
	return media.Asset{}, errors.New("not implemented in test")
}

func (*fakeMediaIngestor) AccessPath(asset media.Asset) string {
	return "/data/media/" + asset.StorageKey
}

type fakeStorageProvider struct {
	objects map[string][]byte
}

func (f *fakeStorageProvider) Put(_ context.Context, key string, reader io.Reader) error {
	if f.objects == nil {
		f.objects = make(map[string][]byte)
	}
	payload, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	f.objects[key] = payload
	return nil
}

func (f *fakeStorageProvider) Open(_ context.Context, key string) (io.ReadCloser, error) {
	payload, ok := f.objects[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(payload)), nil
}

func (f *fakeStorageProvider) Delete(_ context.Context, key string) error {
	delete(f.objects, key)
	return nil
}

func (*fakeStorageProvider) AccessPath(key string) string {
	return "/data/media/" + key
}

type fakeAttachmentResolverAdapter struct {
	typ     channel.ChannelType
	payload channel.AttachmentPayload
}

func (f *fakeAttachmentResolverAdapter) Type() channel.ChannelType {
	if f != nil && strings.TrimSpace(f.typ.String()) != "" {
		return f.typ
	}
	return channel.ChannelType("resolver-test")
}

func (f *fakeAttachmentResolverAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        f.Type(),
		DisplayName: "ResolverTest",
		Capabilities: channel.ChannelCapabilities{
			Text:        true,
			Attachments: true,
		},
	}
}

func (f *fakeAttachmentResolverAdapter) ResolveAttachment(_ context.Context, _ channel.ChannelConfig, _ channel.Attachment) (channel.AttachmentPayload, error) {
	if f != nil && f.payload.Reader != nil {
		return f.payload, nil
	}
	return channel.AttachmentPayload{
		Reader: io.NopCloser(strings.NewReader("resolver-bytes")),
		Mime:   "application/octet-stream",
		Name:   "resolver.bin",
		Size:   int64(len("resolver-bytes")),
	}, nil
}

func (f *fakeChatService) ResolveConversation(_ context.Context, _ route.ResolveInput) (route.ResolveConversationResult, error) {
	if f.resolveErr != nil {
		return route.ResolveConversationResult{}, f.resolveErr
	}
	return f.resolveResult, nil
}

func (f *fakeChatService) Persist(_ context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	f.persistedIn = append(f.persistedIn, input)
	msg := messagepkg.Message{
		BotID:                   input.BotID,
		SessionID:               input.SessionID,
		SenderChannelIdentityID: input.SenderChannelIdentityID,
		SenderUserID:            input.SenderUserID,
		ExternalMessageID:       input.ExternalMessageID,
		SourceReplyToMessageID:  input.SourceReplyToMessageID,
		Role:                    input.Role,
		Content:                 input.Content,
		Metadata:                input.Metadata,
	}
	f.persisted = append(f.persisted, msg)
	return msg, nil
}

func TestChannelInboundProcessorWithIdentity(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-1"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: "route-1"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}
	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "hello"},
		ReplyTarget: "target-id",
		Sender:      channel.Identity{SubjectID: "ext-1", DisplayName: "User1"},
		Conversation: channel.Conversation{
			ID:   "chat-1",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gateway.gotReq.Query != "hello" {
		t.Errorf("expected query 'hello', got: %s", gateway.gotReq.Query)
	}
	if gateway.gotReq.UserID != "channelIdentity-1" {
		t.Errorf("expected user_id 'channelIdentity-1', got: %s", gateway.gotReq.UserID)
	}
	if gateway.gotReq.SourceChannelIdentityID != "channelIdentity-1" {
		t.Errorf("expected source_channel_identity_id 'channelIdentity-1', got: %s", gateway.gotReq.SourceChannelIdentityID)
	}
	if gateway.gotReq.ChatID != "bot-1" {
		t.Errorf("expected bot-scoped chat id 'bot-1', got: %s", gateway.gotReq.ChatID)
	}
	if len(sender.sent) != 1 || sender.sent[0].Message.PlainText() != "AI reply" {
		t.Fatalf("expected AI reply, got: %+v", sender.sent)
	}
}

func TestChannelInboundProcessorDeniedByACL(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-2"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-denied", RouteID: "route-denied"}}
	gateway := &fakeChatGateway{}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	aclSvc := &fakeChatACL{allowed: false}
	processor.SetACLService(aclSvc)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}
	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "hello"},
		ReplyTarget: "target-id",
		Sender:      channel.Identity{SubjectID: "stranger-1"},
		Conversation: channel.Conversation{
			ID:   "chat-1",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gateway.gotReq.Query != "" {
		t.Error("denied user should not trigger chat call")
	}
}

func TestChannelInboundProcessorACLGuestDeniedDowngradesToNotify(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-acl-deny"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-acl", RouteID: "route-acl"}}
	gateway := &fakeChatGateway{}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	aclSvc := &fakeChatACL{allowed: false}
	processor.SetACLService(aclSvc)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}
	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "hello"},
		ReplyTarget: "target-id",
		Sender:      channel.Identity{SubjectID: "guest-1"},
		Conversation: channel.Conversation{
			ID:   "chat-1",
			Type: channel.ConversationTypePrivate,
		},
	}

	if err := processor.HandleInbound(context.Background(), cfg, msg, sender); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aclSvc.calls != 1 {
		t.Fatalf("expected acl to be checked once, got %d", aclSvc.calls)
	}
	if aclSvc.lastReq.ChannelType != "feishu" ||
		aclSvc.lastReq.SourceScope.ConversationType != channel.ConversationTypePrivate ||
		aclSvc.lastReq.SourceScope.ConversationID != "chat-1" {
		t.Fatalf("unexpected acl evaluate request: %+v", aclSvc.lastReq)
	}
	if gateway.gotReq.Query != "" {
		t.Fatal("ACL denied guest should not trigger chat call")
	}
	if len(sender.sent) != 0 {
		t.Fatalf("ACL denied guest should not send reply, got %+v", sender.sent)
	}
	if len(chatSvc.persistedIn) != 1 {
		t.Fatalf("ACL denied guest should persist 1 passive message (replacing inbox), got %d", len(chatSvc.persistedIn))
	}
	if chatSvc.persistedIn[0].Role != "user" {
		t.Fatalf("passive message role should be user, got %q", chatSvc.persistedIn[0].Role)
	}
}

func TestChannelInboundProcessorACLReceivesThreadScope(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-thread-scope"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-thread", RouteID: "route-thread"}}
	gateway := &fakeChatGateway{}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	aclSvc := &fakeChatACL{allowed: false}
	processor.SetACLService(aclSvc)
	sender := &fakeReplySender{}

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("discord"),
		Message:     channel.Message{Text: "hello", Thread: &channel.ThreadRef{ID: "thread-1"}},
		ReplyTarget: "discord:thread-1",
		Sender:      channel.Identity{SubjectID: "guest-thread"},
		Conversation: channel.Conversation{
			ID:   "guild-chat-1",
			Type: channel.ConversationTypeThread,
		},
		Metadata: map[string]any{
			"is_mentioned": true,
		},
	}

	if err := processor.HandleInbound(context.Background(), channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1"}, msg, sender); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if aclSvc.calls != 1 {
		t.Fatalf("expected acl to be checked once, got %d", aclSvc.calls)
	}
	if aclSvc.lastReq.ChannelType != "discord" ||
		aclSvc.lastReq.SourceScope.ConversationType != channel.ConversationTypeThread ||
		aclSvc.lastReq.SourceScope.ConversationID != "guild-chat-1" ||
		aclSvc.lastReq.SourceScope.ThreadID != "thread-1" {
		t.Fatalf("unexpected thread acl evaluate request: %+v", aclSvc.lastReq)
	}
}

func TestChannelInboundProcessorIgnoreEmpty(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-3"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{}
	gateway := &fakeChatGateway{}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1"}
	msg := channel.InboundMessage{Message: channel.Message{Text: "  "}}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("empty message should not error: %v", err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("empty message should not produce reply: %+v", sender.sent)
	}
	if gateway.gotReq.Query != "" {
		t.Error("empty message should not trigger chat call")
	}
}

func TestBuildInboundQueryAttachmentOnlyReturnsEmpty(t *testing.T) {
	t.Parallel()

	msg := channel.Message{
		Attachments: []channel.Attachment{
			{Type: channel.AttachmentImage},
			{Type: channel.AttachmentImage},
		},
	}
	if got := strings.TrimSpace(msg.PlainText()); got != "" {
		t.Fatalf("expected empty query for attachment-only message, got %q", got)
	}
}

func TestChannelInboundProcessorAttachmentOnlyUsesFallbackQuery(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-fallback"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-fallback", RouteID: "route-fallback"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelType("telegram")}
	msg := channel.InboundMessage{
		BotID:   "bot-1",
		Channel: channel.ChannelType("telegram"),
		Message: channel.Message{
			Attachments: []channel.Attachment{
				{Type: channel.AttachmentImage, URL: "https://example.com/a.png"},
				{Type: channel.AttachmentImage, URL: "https://example.com/b.png"},
			},
		},
		ReplyTarget: "chat-123",
		Sender:      channel.Identity{SubjectID: "ext-1"},
		Conversation: channel.Conversation{
			ID:   "conv-1",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gateway.gotReq.Query != "" {
		t.Fatalf("expected empty query for attachment-only message, got %q", gateway.gotReq.Query)
	}
	if len(gateway.gotReq.Attachments) != 2 {
		t.Fatalf("expected attachments to pass through, got %d", len(gateway.gotReq.Attachments))
	}
}

func TestChannelInboundProcessorSilentReply(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-4"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-4", RouteID: "route-4"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("NO_REPLY")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1"}
	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("telegram"),
		Message:     channel.Message{Text: "test"},
		ReplyTarget: "chat-123",
		Sender:      channel.Identity{SubjectID: "user-1"},
		Conversation: channel.Conversation{
			ID:   "conv-1",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("NO_REPLY should suppress output: %+v", sender.sent)
	}
}

func TestChannelInboundProcessorGroupPassiveSync(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-5"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-5", RouteID: "route-5"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1"}
	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{ID: "msg-1", Text: "hello everyone"},
		ReplyTarget: "chat_id:oc_123",
		Sender:      channel.Identity{SubjectID: "user-1"},
		Conversation: channel.Conversation{
			ID:   "oc_123",
			Type: "group",
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gateway.gotReq.Query != "" {
		t.Fatalf("group passive sync should not trigger chat call")
	}
	if len(sender.sent) != 0 {
		t.Fatalf("group passive sync should not send reply: %+v", sender.sent)
	}
	if len(chatSvc.persisted) != 1 {
		t.Fatalf("group passive sync should persist 1 passive message (replacing inbox), got: %d", len(chatSvc.persisted))
	}
	if chatSvc.persisted[0].Role != "user" {
		t.Fatalf("passive message role should be user, got %q", chatSvc.persisted[0].Role)
	}
}

func TestChannelInboundProcessorGroupMentionTriggersReply(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-6"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-6", RouteID: "route-6"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1"}
	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{ID: "msg-2", Text: "@bot ping"},
		ReplyTarget: "chat_id:oc_123",
		Sender:      channel.Identity{SubjectID: "user-1"},
		Conversation: channel.Conversation{
			ID:   "oc_123",
			Type: "group",
		},
		Metadata: map[string]any{
			"is_mentioned": true,
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gateway.gotReq.Query == "" {
		t.Fatalf("group mention should trigger chat call")
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected one outbound reply, got %d", len(sender.sent))
	}
	if gateway.gotReq.UserMessagePersisted {
		t.Fatalf("expected UserMessagePersisted=false: user message persistence is deferred to storeRound")
	}
}

type failingOpenStreamSender struct {
	err error
}

func (*failingOpenStreamSender) Send(_ context.Context, _ channel.OutboundMessage) error {
	return nil
}

func (s *failingOpenStreamSender) OpenStream(_ context.Context, _ string, _ channel.StreamOptions) (channel.OutboundStream, error) {
	if s != nil && s.err != nil {
		return nil, s.err
	}
	return nil, errors.New("open stream failed")
}

type failingCloseSender struct {
	err error
}

func (*failingCloseSender) Send(_ context.Context, _ channel.OutboundMessage) error {
	return nil
}

func (s *failingCloseSender) OpenStream(_ context.Context, target string, _ channel.StreamOptions) (channel.OutboundStream, error) {
	return &failingCloseStream{target: strings.TrimSpace(target), err: s.err}, nil
}

type failingCloseStream struct {
	target string
	err    error
}

func (*failingCloseStream) Push(_ context.Context, _ channel.StreamEvent) error {
	return nil
}

func (s *failingCloseStream) Close(_ context.Context) error {
	if s != nil && s.err != nil {
		return s.err
	}
	return errors.New("close stream failed")
}

func TestChannelInboundProcessorDoesNotPersistBeforeOpenStream(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-openstream"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-openstream", RouteID: "route-openstream"}}
	gateway := &fakeChatGateway{}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	sender := &failingOpenStreamSender{err: errors.New("stream unavailable")}

	cfg := channel.ChannelConfig{ID: "cfg-openstream", BotID: "bot-1", ChannelType: channel.ChannelType("qq")}
	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("qq"),
		Message:     channel.Message{ID: "msg-openstream-1", Text: "hello"},
		ReplyTarget: "c2c:user-openid",
		Sender:      channel.Identity{SubjectID: "user-1"},
		Conversation: channel.Conversation{
			ID:   "conv-openstream",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err == nil || err.Error() != "stream unavailable" {
		t.Fatalf("expected open stream error, got: %v", err)
	}
	if len(chatSvc.persistedIn) != 0 {
		t.Fatalf("user message persistence is deferred to storeRound; expected 0 persisted, got %d", len(chatSvc.persistedIn))
	}
	if gateway.gotReq.Query != "" {
		t.Fatalf("runner should not be called when stream open fails")
	}
}

func TestChannelInboundProcessorReturnsCloseStreamError(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-closestream"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-closestream", RouteID: "route-closestream"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("ok")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	sender := &failingCloseSender{err: errors.New("wechat send failed")}

	cfg := channel.ChannelConfig{ID: "cfg-closestream", BotID: "bot-1", ChannelType: channel.ChannelType("wechatoa")}
	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("wechatoa"),
		Message:     channel.Message{ID: "msg-closestream-1", Text: "hello"},
		ReplyTarget: "openid:user-openid",
		Sender:      channel.Identity{SubjectID: "user-1"},
		Conversation: channel.Conversation{
			ID:   "conv-closestream",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err == nil || err.Error() != "wechat send failed" {
		t.Fatalf("expected close stream error, got: %v", err)
	}
}

func TestChannelInboundProcessorPersistsAttachmentAssetRefs(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-asset"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-asset", RouteID: "route-asset"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("ok")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-asset", BotID: "bot-1"}
	msg := channel.InboundMessage{
		BotID:   "bot-1",
		Channel: channel.ChannelType("feishu"),
		Message: channel.Message{
			ID:   "msg-asset-1",
			Text: "attachment test",
			Attachments: []channel.Attachment{
				{
					Type:        channel.AttachmentImage,
					URL:         "https://example.com/img.png",
					ContentHash: "asset-1",
					Name:        "img.png",
					Mime:        "image/png",
				},
			},
		},
		ReplyTarget: "chat_id:oc_asset",
		Sender:      channel.Identity{SubjectID: "ext-asset"},
		Conversation: channel.Conversation{
			ID:   "oc_asset",
			Type: channel.ConversationTypePrivate,
		},
	}

	if err := processor.HandleInbound(context.Background(), cfg, msg, sender); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chatSvc.persistedIn) != 0 {
		t.Fatalf("user message persistence is deferred to storeRound; expected 0 persisted, got %d", len(chatSvc.persistedIn))
	}
	if len(gateway.gotReq.Attachments) != 1 {
		t.Fatalf("expected one gateway attachment, got %d", len(gateway.gotReq.Attachments))
	}
	if got := gateway.gotReq.Attachments[0].ContentHash; got != "asset-1" {
		t.Fatalf("expected gateway attachment content_hash asset-1, got %q", got)
	}
}

func TestChannelInboundProcessorIngestsPlatformKeyWithResolver(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-resolver"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-resolver", RouteID: "route-resolver"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("ok")},
			},
		},
	}
	registry := channel.NewRegistry()
	registry.MustRegister(&fakeAttachmentResolverAdapter{})
	processor := NewChannelInboundProcessor(slog.Default(), registry, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	mediaSvc := &fakeMediaIngestor{nextID: "asset-resolved-1", nextMime: "application/octet-stream"}
	processor.SetMediaService(mediaSvc)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-resolver", BotID: "bot-1", ChannelType: channel.ChannelType("resolver-test")}
	msg := channel.InboundMessage{
		BotID:   "bot-1",
		Channel: channel.ChannelType("resolver-test"),
		Message: channel.Message{
			ID:   "msg-resolver-1",
			Text: "attachment resolver test",
			Attachments: []channel.Attachment{
				{
					Type:        channel.AttachmentFile,
					PlatformKey: "platform-file-1",
				},
			},
		},
		ReplyTarget: "resolver-target",
		Sender:      channel.Identity{SubjectID: "resolver-user"},
		Conversation: channel.Conversation{
			ID:   "resolver-conv",
			Type: channel.ConversationTypePrivate,
		},
	}

	if err := processor.HandleInbound(context.Background(), cfg, msg, sender); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mediaSvc.calls != 1 {
		t.Fatalf("expected media ingest to be called once, got %d", mediaSvc.calls)
	}
	if len(gateway.gotReq.Attachments) != 1 {
		t.Fatalf("expected one gateway attachment, got %d", len(gateway.gotReq.Attachments))
	}
	if got := gateway.gotReq.Attachments[0].ContentHash; got != "asset-resolved-1" {
		t.Fatalf("expected resolved asset id, got %q", got)
	}
	if len(chatSvc.persistedIn) != 0 {
		t.Fatalf("user message persistence is deferred to storeRound; expected 0 persisted, got %d", len(chatSvc.persistedIn))
	}
}

func TestChannelInboundProcessorIngestsBase64Attachment(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-base64"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-base64", RouteID: "route-base64"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("ok")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	mediaSvc := &fakeMediaIngestor{nextID: "asset-base64-1", nextMime: "image/png"}
	processor.SetMediaService(mediaSvc)
	sender := &fakeReplySender{}

	encoded := base64.StdEncoding.EncodeToString([]byte("fake-image-bytes"))
	cfg := channel.ChannelConfig{ID: "cfg-base64", BotID: "bot-1", ChannelType: channel.ChannelType("local")}
	msg := channel.InboundMessage{
		BotID:   "bot-1",
		Channel: channel.ChannelType("local"),
		Message: channel.Message{
			ID:   "msg-base64-1",
			Text: "attachment base64 test",
			Attachments: []channel.Attachment{
				{
					Type:   channel.AttachmentImage,
					Base64: "data:image/png;base64," + encoded,
					Name:   "cat.png",
				},
			},
		},
		ReplyTarget: "web-target",
		Sender: channel.Identity{
			SubjectID: "web-subject",
			Attributes: map[string]string{
				"user_id": "web-user-id",
			},
		},
		Conversation: channel.Conversation{
			ID:   "web-conv",
			Type: channel.ConversationTypePrivate,
		},
	}

	if err := processor.HandleInbound(context.Background(), cfg, msg, sender); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mediaSvc.calls != 1 {
		t.Fatalf("expected media ingest to be called once, got %d", mediaSvc.calls)
	}
	if len(mediaSvc.payloads) != 1 || string(mediaSvc.payloads[0]) != "fake-image-bytes" {
		t.Fatalf("unexpected ingested payload: %+v", mediaSvc.payloads)
	}
	if len(gateway.gotReq.Attachments) != 1 {
		t.Fatalf("expected one gateway attachment, got %d", len(gateway.gotReq.Attachments))
	}
	gotAttachment := gateway.gotReq.Attachments[0]
	if gotAttachment.ContentHash != "asset-base64-1" {
		t.Fatalf("expected resolved asset id, got %q", gotAttachment.ContentHash)
	}
	if gotAttachment.Base64 != "" {
		t.Fatalf("expected base64 to be cleared after ingest, got %q", gotAttachment.Base64)
	}
	if !strings.HasPrefix(gotAttachment.Path, "/data/media/") {
		t.Fatalf("expected attachment path under /data/media/, got %q", gotAttachment.Path)
	}
	if len(chatSvc.persistedIn) != 0 {
		t.Fatalf("user message persistence is deferred to storeRound; expected 0 persisted, got %d", len(chatSvc.persistedIn))
	}
}

func TestChannelInboundProcessorIngestsQQFileAttachmentKeepsOriginalExtWhenMimeGeneric(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-qq-file"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-qq-file", RouteID: "route-qq-file"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("ok")},
			},
		},
	}
	registry := channel.NewRegistry()
	registry.MustRegister(&fakeAttachmentResolverAdapter{
		typ: channel.ChannelType("qq"),
		payload: channel.AttachmentPayload{
			Reader: io.NopCloser(bytes.NewReader([]byte{0x00, 0x01, 0x02, 0x03, 0x04})),
			Mime:   "application/octet-stream",
			Size:   5,
		},
	})
	processor := NewChannelInboundProcessor(slog.Default(), registry, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	storage := &fakeStorageProvider{}
	mediaSvc := media.NewService(slog.Default(), storage)
	processor.SetMediaService(mediaSvc)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-qq-file", BotID: "bot-1", ChannelType: channel.ChannelType("qq")}
	msg := channel.InboundMessage{
		BotID:   "bot-1",
		Channel: channel.ChannelType("qq"),
		Message: channel.Message{
			ID:   "msg-qq-file-1",
			Text: "[User sent 1 attachment]",
			Attachments: []channel.Attachment{
				{
					Type:        channel.AttachmentFile,
					PlatformKey: "qq-file-1",
					Name:        "test.md",
					Mime:        "file",
				},
			},
		},
		ReplyTarget: "c2c:user-openid",
		Sender:      channel.Identity{SubjectID: "qq-user"},
		Conversation: channel.Conversation{
			ID:   "qq-user",
			Type: channel.ConversationTypePrivate,
		},
	}

	if err := processor.HandleInbound(context.Background(), cfg, msg, sender); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gateway.gotReq.Attachments) != 1 {
		t.Fatalf("expected one attachment in gateway request, got %d", len(gateway.gotReq.Attachments))
	}
	storageKey, _ := gateway.gotReq.Attachments[0].Metadata["storage_key"].(string)
	if !strings.HasSuffix(storageKey, ".md") {
		t.Fatalf("expected storage key to keep .md extension, got %q", storageKey)
	}
	if strings.HasSuffix(storageKey, ".bin") {
		t.Fatalf("expected storage key to avoid .bin fallback, got %q", storageKey)
	}
}

func TestChannelInboundProcessorPersonalGroupNonOwnerIgnored(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-member"}}
	policySvc := &fakePolicyService{ownerUserID: "channelIdentity-owner"}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-personal-1", RouteID: "route-personal-1"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1"}
	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{ID: "msg-personal-1", Text: "hello"},
		ReplyTarget: "chat_id:oc_personal",
		Sender:      channel.Identity{SubjectID: "ext-member-1"},
		Conversation: channel.Conversation{
			ID:   "oc_personal",
			Type: "group",
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gateway.gotReq.Query != "" {
		t.Fatalf("non-owner should not trigger chat call")
	}
	if len(sender.sent) != 0 {
		t.Fatalf("non-owner should be ignored silently: %+v", sender.sent)
	}
	if len(chatSvc.persisted) != 1 {
		t.Fatalf("non-owner group message should persist 1 passive message (replacing inbox), got %d", len(chatSvc.persisted))
	}
	if chatSvc.persisted[0].Role != "user" {
		t.Fatalf("passive message role should be user, got %q", chatSvc.persisted[0].Role)
	}
}

func TestChannelInboundProcessorPersonalGroupOwnerWithoutMentionUsesPassivePersistence(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-owner"}}
	policySvc := &fakePolicyService{ownerUserID: "channelIdentity-owner"}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-personal-2", RouteID: "route-personal-2"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1"}
	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{ID: "msg-personal-2", Text: "owner says hi"},
		ReplyTarget: "chat_id:oc_personal",
		Sender:      channel.Identity{SubjectID: "ext-owner-1"},
		Conversation: channel.Conversation{
			ID:   "oc_personal",
			Type: "group",
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gateway.gotReq.Query != "" {
		t.Fatalf("owner group message without mention should not trigger chat call")
	}
	if len(sender.sent) != 0 {
		t.Fatalf("owner group message without mention should not send reply")
	}
	if len(chatSvc.persisted) != 1 {
		t.Fatalf("owner non-mentioned message should persist 1 passive message (replacing inbox), got: %d", len(chatSvc.persisted))
	}
	if chatSvc.persisted[0].Role != "user" {
		t.Fatalf("passive message role should be user, got %q", chatSvc.persisted[0].Role)
	}
}

func TestChannelInboundProcessorProcessingStatusSuccessLifecycle(t *testing.T) {
	notifier := &fakeProcessingStatusNotifier{
		startedHandle: channel.ProcessingStatusHandle{Token: "reaction-1"},
	}
	registry := channel.NewRegistry()
	registry.MustRegister(&fakeProcessingStatusAdapter{notifier: notifier})
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-1"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: "route-1"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("AI reply")},
			},
		},
		onChat: func(_ conversation.ChatRequest) {
			if len(notifier.events) != 1 || notifier.events[0] != "started" {
				t.Fatalf("expected started before chat call, got events: %+v", notifier.events)
			}
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), registry, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	sender := &fakeReplySender{}
	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}
	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{ID: "om_123", Text: "hello"},
		ReplyTarget: "chat_id:oc_123",
		Sender:      channel.Identity{SubjectID: "ext-1"},
		Conversation: channel.Conversation{
			ID:   "oc_123",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.events) != 2 || notifier.events[0] != "started" || notifier.events[1] != "completed" {
		t.Fatalf("unexpected processing status lifecycle: %+v", notifier.events)
	}
	if notifier.completedSeen.Token != "reaction-1" {
		t.Fatalf("expected completed token reaction-1, got: %q", notifier.completedSeen.Token)
	}
	if notifier.failedCause != nil {
		t.Fatalf("expected failed cause nil, got: %v", notifier.failedCause)
	}
	if len(notifier.info) == 0 || notifier.info[0].SourceMessageID != "om_123" {
		t.Fatalf("expected processing info source message id om_123, got: %+v", notifier.info)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected one outbound reply, got %d", len(sender.sent))
	}
}

func TestChannelInboundProcessorProcessingStatusFailureLifecycle(t *testing.T) {
	notifier := &fakeProcessingStatusNotifier{
		startedHandle: channel.ProcessingStatusHandle{Token: "reaction-2"},
	}
	registry := channel.NewRegistry()
	registry.MustRegister(&fakeProcessingStatusAdapter{notifier: notifier})
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-2"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-2", RouteID: "route-2"}}
	chatErr := errors.New("chat gateway unavailable")
	gateway := &fakeChatGateway{err: chatErr}
	processor := NewChannelInboundProcessor(slog.Default(), registry, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	sender := &fakeReplySender{}
	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}
	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{ID: "om_456", Text: "hello"},
		ReplyTarget: "chat_id:oc_456",
		Sender:      channel.Identity{SubjectID: "ext-2"},
		Conversation: channel.Conversation{
			ID:   "oc_456",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if !errors.Is(err, chatErr) {
		t.Fatalf("expected chat error, got: %v", err)
	}
	if len(notifier.events) != 2 || notifier.events[0] != "started" || notifier.events[1] != "failed" {
		t.Fatalf("unexpected processing status lifecycle: %+v", notifier.events)
	}
	if !errors.Is(notifier.failedCause, chatErr) {
		t.Fatalf("expected failed cause chat error, got: %v", notifier.failedCause)
	}
	if notifier.failedSeen.Token != "reaction-2" {
		t.Fatalf("expected failed token reaction-2, got: %q", notifier.failedSeen.Token)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("expected no outbound reply on chat failure, got: %+v", sender.sent)
	}
}

func TestChannelInboundProcessorProcessingStatusErrorsAreBestEffort(t *testing.T) {
	notifier := &fakeProcessingStatusNotifier{
		startedErr:   errors.New("start notify failed"),
		completedErr: errors.New("completed notify failed"),
	}
	registry := channel.NewRegistry()
	registry.MustRegister(&fakeProcessingStatusAdapter{notifier: notifier})
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-3"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-3", RouteID: "route-3"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), registry, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	sender := &fakeReplySender{}
	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}
	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{ID: "om_789", Text: "hello"},
		ReplyTarget: "chat_id:oc_789",
		Sender:      channel.Identity{SubjectID: "ext-3"},
		Conversation: channel.Conversation{
			ID:   "oc_789",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.events) != 2 || notifier.events[0] != "started" || notifier.events[1] != "completed" {
		t.Fatalf("unexpected processing status lifecycle: %+v", notifier.events)
	}
	if notifier.completedSeen.Token != "" {
		t.Fatalf("expected empty completed token after started failure, got: %q", notifier.completedSeen.Token)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected one outbound reply, got %d", len(sender.sent))
	}
}

func TestChannelInboundProcessorProcessingFailedNotifyErrorDoesNotOverrideChatError(t *testing.T) {
	notifier := &fakeProcessingStatusNotifier{
		startedHandle: channel.ProcessingStatusHandle{Token: "reaction-4"},
		failedErr:     errors.New("failed notify error"),
	}
	registry := channel.NewRegistry()
	registry.MustRegister(&fakeProcessingStatusAdapter{notifier: notifier})
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-4"}}
	policySvc := &fakePolicyService{}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-4", RouteID: "route-4"}}
	chatErr := errors.New("chat failed")
	gateway := &fakeChatGateway{err: chatErr}
	processor := NewChannelInboundProcessor(slog.Default(), registry, chatSvc, chatSvc, gateway, channelIdentitySvc, policySvc, nil, "", 0)
	sender := &fakeReplySender{}
	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}
	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{ID: "om_999", Text: "hello"},
		ReplyTarget: "chat_id:oc_999",
		Sender:      channel.Identity{SubjectID: "ext-4"},
		Conversation: channel.Conversation{
			ID:   "oc_999",
			Type: channel.ConversationTypePrivate,
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if !errors.Is(err, chatErr) {
		t.Fatalf("expected original chat error, got: %v", err)
	}
	if len(notifier.events) != 2 || notifier.events[0] != "started" || notifier.events[1] != "failed" {
		t.Fatalf("unexpected processing status lifecycle: %+v", notifier.events)
	}
}

func TestDownloadInboundAttachmentURLTooLarge(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", "999999999")
		_, _ = w.Write([]byte("x"))
	}))
	defer server.Close()

	_, err := openInboundAttachmentURL(context.Background(), server.URL)
	if err == nil {
		t.Fatalf("expected too-large error")
	}
	if !errors.Is(err, media.ErrAssetTooLarge) {
		t.Fatalf("expected ErrAssetTooLarge, got %v", err)
	}
}

func TestMapStreamChunkToChannelEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		chunk         string
		wantType      channel.StreamEventType
		wantDelta     string
		wantPhase     channel.StreamPhase
		wantToolName  string
		wantAttCount  int
		wantError     string
		wantNilEvents bool
	}{
		{
			name:      "text_delta",
			chunk:     `{"type":"text_delta","delta":"hello"}`,
			wantType:  channel.StreamEventDelta,
			wantDelta: "hello",
			wantPhase: channel.StreamPhaseText,
		},
		{
			name:          "text_delta empty",
			chunk:         `{"type":"text_delta","delta":""}`,
			wantNilEvents: true,
		},
		{
			name:      "reasoning_delta",
			chunk:     `{"type":"reasoning_delta","delta":"thinking"}`,
			wantType:  channel.StreamEventDelta,
			wantDelta: "thinking",
			wantPhase: channel.StreamPhaseReasoning,
		},
		{
			name:          "reasoning_delta empty",
			chunk:         `{"type":"reasoning_delta","delta":""}`,
			wantNilEvents: true,
		},
		{
			name:      "reasoning_start",
			chunk:     `{"type":"reasoning_start"}`,
			wantType:  channel.StreamEventPhaseStart,
			wantPhase: channel.StreamPhaseReasoning,
		},
		{
			name:      "reasoning_end",
			chunk:     `{"type":"reasoning_end"}`,
			wantType:  channel.StreamEventPhaseEnd,
			wantPhase: channel.StreamPhaseReasoning,
		},
		{
			name:      "text_start",
			chunk:     `{"type":"text_start"}`,
			wantType:  channel.StreamEventPhaseStart,
			wantPhase: channel.StreamPhaseText,
		},
		{
			name:      "text_end",
			chunk:     `{"type":"text_end"}`,
			wantType:  channel.StreamEventPhaseEnd,
			wantPhase: channel.StreamPhaseText,
		},
		{
			name:         "tool_call_start",
			chunk:        `{"type":"tool_call_start","toolName":"search_web","toolCallId":"tc_1","input":{"query":"test"}}`,
			wantType:     channel.StreamEventToolCallStart,
			wantToolName: "search_web",
		},
		{
			name:         "tool_call_end",
			chunk:        `{"type":"tool_call_end","toolName":"search_web","toolCallId":"tc_1","input":{"query":"test"},"result":{"ok":true}}`,
			wantType:     channel.StreamEventToolCallEnd,
			wantToolName: "search_web",
		},
		{
			name:         "attachment_delta",
			chunk:        `{"type":"attachment_delta","attachments":[{"type":"image","url":"https://example.com/img.png"}]}`,
			wantType:     channel.StreamEventAttachment,
			wantAttCount: 1,
		},
		{
			name:          "attachment_delta empty",
			chunk:         `{"type":"attachment_delta","attachments":[]}`,
			wantNilEvents: true,
		},
		{
			name:      "error",
			chunk:     `{"type":"error","error":"something failed"}`,
			wantType:  channel.StreamEventError,
			wantError: "something failed",
		},
		{
			name:      "error fallback to message",
			chunk:     `{"type":"error","message":"fallback msg"}`,
			wantType:  channel.StreamEventError,
			wantError: "fallback msg",
		},
		{
			name:     "agent_start",
			chunk:    `{"type":"agent_start","input":{"agent":"planner"}}`,
			wantType: channel.StreamEventAgentStart,
		},
		{
			name:     "agent_end",
			chunk:    `{"type":"agent_end","result":{"ok":true}}`,
			wantType: channel.StreamEventAgentEnd,
		},
		{
			name:     "processing_started",
			chunk:    `{"type":"processing_started"}`,
			wantType: channel.StreamEventProcessingStarted,
		},
		{
			name:     "processing_completed",
			chunk:    `{"type":"processing_completed"}`,
			wantType: channel.StreamEventProcessingCompleted,
		},
		{
			name:      "processing_failed",
			chunk:     `{"type":"processing_failed","error":"failed"}`,
			wantType:  channel.StreamEventProcessingFailed,
			wantError: "failed",
		},
		{
			name:          "empty chunk",
			chunk:         ``,
			wantNilEvents: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			events, _, err := mapStreamChunkToChannelEvents(conversation.StreamChunk([]byte(tt.chunk)))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNilEvents {
				if len(events) > 0 {
					t.Fatalf("expected nil/empty events, got %d", len(events))
				}
				return
			}
			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}
			ev := events[0]
			if ev.Type != tt.wantType {
				t.Fatalf("expected type %q, got %q", tt.wantType, ev.Type)
			}
			if tt.wantDelta != "" && ev.Delta != tt.wantDelta {
				t.Fatalf("expected delta %q, got %q", tt.wantDelta, ev.Delta)
			}
			if tt.wantPhase != "" && ev.Phase != tt.wantPhase {
				t.Fatalf("expected phase %q, got %q", tt.wantPhase, ev.Phase)
			}
			if tt.wantToolName != "" {
				if ev.ToolCall == nil {
					t.Fatal("expected non-nil ToolCall")
				}
				if ev.ToolCall.Name != tt.wantToolName {
					t.Fatalf("expected tool name %q, got %q", tt.wantToolName, ev.ToolCall.Name)
				}
			}
			if tt.wantAttCount > 0 && len(ev.Attachments) != tt.wantAttCount {
				t.Fatalf("expected %d attachments, got %d", tt.wantAttCount, len(ev.Attachments))
			}
			if tt.wantError != "" && ev.Error != tt.wantError {
				t.Fatalf("expected error %q, got %q", tt.wantError, ev.Error)
			}
		})
	}
}

func TestMapStreamChunkToChannelEvents_ToolCallFields(t *testing.T) {
	t.Parallel()

	chunk := `{"type":"tool_call_end","toolName":"calc","toolCallId":"c1","input":{"x":1},"result":{"sum":2}}`
	events, _, err := mapStreamChunkToChannelEvents(conversation.StreamChunk([]byte(chunk)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	tc := events[0].ToolCall
	if tc == nil {
		t.Fatal("expected non-nil ToolCall")
		return
	}
	if tc.Name != "calc" || tc.CallID != "c1" {
		t.Fatalf("unexpected name/callID: %q / %q", tc.Name, tc.CallID)
	}
	if tc.Input == nil || tc.Result == nil {
		t.Fatal("expected non-nil Input and Result")
	}
}

func TestMapStreamChunkToChannelEvents_FinalMessages(t *testing.T) {
	t.Parallel()

	chunk := `{"type":"agent_end","messages":[{"role":"assistant","content":"done"}]}`
	events, messages, err := mapStreamChunkToChannelEvents(conversation.StreamChunk([]byte(chunk)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != channel.StreamEventAgentEnd {
		t.Fatalf("expected event type %q, got %q", channel.StreamEventAgentEnd, events[0].Type)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 final message, got %d", len(messages))
	}
	if messages[0].Role != "assistant" {
		t.Fatalf("expected role assistant, got %q", messages[0].Role)
	}
}

func TestIngestOutboundAttachments_DataURL(t *testing.T) {
	t.Parallel()

	p := &ChannelInboundProcessor{}
	attachments := []channel.Attachment{
		{Type: channel.AttachmentImage, URL: "data:image/png;base64,iVBORw0KGgo=", Mime: "image/png"},
	}
	// Without media service, attachments pass through unchanged.
	result := p.ingestOutboundAttachments(context.Background(), "bot-1", channel.ChannelType("telegram"), attachments)
	if len(result) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(result))
	}
	if result[0].ContentHash != "" {
		t.Fatalf("expected empty content_hash without media service, got %q", result[0].ContentHash)
	}
}

func TestIngestOutboundAttachments_NonDataURL(t *testing.T) {
	t.Parallel()

	p := &ChannelInboundProcessor{}
	attachments := []channel.Attachment{
		{Type: channel.AttachmentImage, URL: "https://example.com/img.png"},
		{Type: channel.AttachmentImage, ContentHash: "existing-asset", URL: "/data/media/img.png"},
	}
	result := p.ingestOutboundAttachments(context.Background(), "bot-1", channel.ChannelType("telegram"), attachments)
	if len(result) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(result))
	}
	if result[0].URL != "https://example.com/img.png" {
		t.Fatalf("expected public URL preserved, got %q", result[0].URL)
	}
	if result[1].ContentHash != "existing-asset" {
		t.Fatalf("expected existing content_hash preserved, got %q", result[1].ContentHash)
	}
}

func TestChannelAttachmentsToAssetRefs(t *testing.T) {
	t.Parallel()

	attachments := []channel.Attachment{
		{ContentHash: "a1", Type: channel.AttachmentImage},
		{Type: channel.AttachmentFile},
		{ContentHash: "a2", Type: channel.AttachmentAudio},
	}
	refs := channelAttachmentsToAssetRefs(attachments, "output")
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0].ContentHash != "a1" || refs[0].Role != "output" {
		t.Fatalf("unexpected ref[0]: %+v", refs[0])
	}
	if refs[1].ContentHash != "a2" {
		t.Fatalf("unexpected ref[1]: %+v", refs[1])
	}
}

func TestIsDataURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  bool
	}{
		{"data:image/png;base64,abc", true},
		{"DATA:text/plain;base64,abc", true},
		{"https://example.com", false},
		{"/data/media/img.png", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isDataURL(tt.input); got != tt.want {
			t.Errorf("isDataURL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestExtractStorageKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		accessPath string
		botID      string
		want       string
	}{
		{"/data/media/26da/26da0cc7.jpg", "bot-1", "26da/26da0cc7.jpg"},
		{"/data/media/abcd/abcd1234.pdf", "bot-2", "abcd/abcd1234.pdf"},
		{"https://example.com/img.png", "bot-1", ""},
		{"", "bot-1", ""},
	}
	for _, tt := range tests {
		got := extractStorageKey(tt.accessPath, tt.botID)
		if got != tt.want {
			t.Errorf("extractStorageKey(%q, %q) = %q, want %q", tt.accessPath, tt.botID, got, tt.want)
		}
	}
}

func TestIsHTTPURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  bool
	}{
		{"https://example.com/img.png", true},
		{"http://localhost:8080/test", true},
		{"HTTP://EXAMPLE.COM", true},
		{"/data/media/img.png", false},
		{"data:image/png;base64,abc", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isHTTPURL(tt.input); got != tt.want {
			t.Errorf("isHTTPURL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIngestOutboundAttachments_ContainerPath(t *testing.T) {
	t.Parallel()

	ms := &fakeMediaIngestor{
		storageKeyAsset: media.Asset{ContentHash: "resolved-asset-1", Mime: "image/jpeg", SizeBytes: 1024},
	}
	p := &ChannelInboundProcessor{mediaService: ms}
	attachments := []channel.Attachment{
		{Type: channel.AttachmentImage, URL: "/data/media/26da/26da0cc7.jpg"},
	}
	result := p.ingestOutboundAttachments(context.Background(), "bot-1", channel.ChannelType("telegram"), attachments)
	if len(result) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(result))
	}
	if result[0].ContentHash != "resolved-asset-1" {
		t.Fatalf("expected content_hash resolved-asset-1, got %q", result[0].ContentHash)
	}
	if result[0].Metadata["bot_id"] != "bot-1" {
		t.Fatalf("expected bot_id in metadata, got %v", result[0].Metadata)
	}
}

func TestIngestOutboundAttachments_ContainerPathNotFound(t *testing.T) {
	t.Parallel()

	ms := &fakeMediaIngestor{
		storageKeyErr: errors.New("not found"),
	}
	p := &ChannelInboundProcessor{mediaService: ms}
	attachments := []channel.Attachment{
		{Type: channel.AttachmentImage, URL: "/data/media/26da/missing.jpg"},
	}
	result := p.ingestOutboundAttachments(context.Background(), "bot-1", channel.ChannelType("telegram"), attachments)
	if len(result) != 1 {
		t.Fatalf("expected unresolved container attachment to remain unchanged, got %d", len(result))
	}
	if result[0].URL != "/data/media/26da/missing.jpg" {
		t.Fatalf("expected original path preserved, got %q", result[0].URL)
	}
	if result[0].ContentHash != "" {
		t.Fatalf("expected empty content_hash for unresolved path, got %q", result[0].ContentHash)
	}
}

func TestMapChannelToChatAttachments(t *testing.T) {
	t.Parallel()

	attachments := []channel.Attachment{
		{
			Type:        channel.AttachmentImage,
			ContentHash: "asset-1",
			URL:         "/data/media/ab/c.png",
			Base64:      "AAAA",
			Mime:        "image/png",
		},
		{
			Type: channel.AttachmentFile,
			URL:  "https://example.com/doc.pdf",
			Name: "doc.pdf",
		},
	}

	mapped := mapChannelToChatAttachments(attachments)
	if len(mapped) != 2 {
		t.Fatalf("expected 2 mapped attachments, got %d", len(mapped))
	}
	if mapped[0].Path != "/data/media/ab/c.png" {
		t.Fatalf("expected asset attachment path, got %q", mapped[0].Path)
	}
	if !strings.HasPrefix(mapped[0].Base64, "data:image/png;base64,") {
		t.Fatalf("expected normalized base64 data url, got %q", mapped[0].Base64)
	}
	if mapped[1].URL != "https://example.com/doc.pdf" {
		t.Fatalf("expected non-asset attachment URL, got %q", mapped[1].URL)
	}
}
