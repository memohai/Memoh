package inbound

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/schedule"
)

type fakeChatGateway struct {
	resp   conversation.ChatResponse
	err    error
	gotReq conversation.ChatRequest
	onChat func(conversation.ChatRequest)
}

func (f *fakeChatGateway) Chat(ctx context.Context, req conversation.ChatRequest) (conversation.ChatResponse, error) {
	f.gotReq = req
	if f.onChat != nil {
		f.onChat(req)
	}
	return f.resp, f.err
}

func (f *fakeChatGateway) StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
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

func (f *fakeChatGateway) TriggerSchedule(ctx context.Context, botID string, payload schedule.TriggerPayload, token string) error {
	return nil
}

type fakeReplySender struct {
	sent   []channel.OutboundMessage
	events []channel.StreamEvent
}

func (s *fakeReplySender) Send(ctx context.Context, msg channel.OutboundMessage) error {
	s.sent = append(s.sent, msg)
	return nil
}

func (s *fakeReplySender) OpenStream(ctx context.Context, target string, opts channel.StreamOptions) (channel.OutboundStream, error) {
	return &fakeOutboundStream{
		sender: s,
		target: strings.TrimSpace(target),
	}, nil
}

type fakeOutboundStream struct {
	sender *fakeReplySender
	target string
}

func (s *fakeOutboundStream) Push(ctx context.Context, event channel.StreamEvent) error {
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

func (s *fakeOutboundStream) Close(ctx context.Context) error {
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

func (n *fakeProcessingStatusNotifier) ProcessingStarted(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, info channel.ProcessingStatusInfo) (channel.ProcessingStatusHandle, error) {
	n.events = append(n.events, "started")
	n.info = append(n.info, info)
	return n.startedHandle, n.startedErr
}

func (n *fakeProcessingStatusNotifier) ProcessingCompleted(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, info channel.ProcessingStatusInfo, handle channel.ProcessingStatusHandle) error {
	n.events = append(n.events, "completed")
	n.info = append(n.info, info)
	n.completedSeen = handle
	return n.completedErr
}

func (n *fakeProcessingStatusNotifier) ProcessingFailed(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, info channel.ProcessingStatusInfo, handle channel.ProcessingStatusHandle, cause error) error {
	n.events = append(n.events, "failed")
	n.info = append(n.info, info)
	n.failedSeen = handle
	n.failedCause = cause
	return n.failedErr
}

type fakeProcessingStatusAdapter struct {
	notifier *fakeProcessingStatusNotifier
}

func (a *fakeProcessingStatusAdapter) Type() channel.ChannelType {
	return channel.ChannelType("feishu")
}

func (a *fakeProcessingStatusAdapter) Descriptor() channel.Descriptor {
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
}

func (f *fakeChatService) ResolveConversation(ctx context.Context, input route.ResolveInput) (route.ResolveConversationResult, error) {
	if f.resolveErr != nil {
		return route.ResolveConversationResult{}, f.resolveErr
	}
	return f.resolveResult, nil
}

func (f *fakeChatService) Persist(ctx context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	msg := messagepkg.Message{
		BotID:                   input.BotID,
		RouteID:                 input.RouteID,
		SenderChannelIdentityID: input.SenderChannelIdentityID,
		SenderUserID:            input.SenderUserID,
		Platform:                input.Platform,
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
	memberSvc := &fakeMemberService{isMember: true}
	policySvc := &fakePolicyService{allow: false}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: "route-1"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", 0)
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
			Type: "p2p",
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

func TestChannelInboundProcessorDenied(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-2"}}
	memberSvc := &fakeMemberService{isMember: false}
	policySvc := &fakePolicyService{allow: false}
	chatSvc := &fakeChatService{}
	gateway := &fakeChatGateway{}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}
	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "hello"},
		ReplyTarget: "target-id",
		Sender:      channel.Identity{SubjectID: "stranger-1"},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sender.sent) != 1 || !strings.Contains(sender.sent[0].Message.PlainText(), "denied") {
		t.Fatalf("expected access denied reply, got: %+v", sender.sent)
	}
	if gateway.gotReq.Query != "" {
		t.Error("denied user should not trigger chat call")
	}
}

func TestChannelInboundProcessorIgnoreEmpty(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-3"}}
	memberSvc := &fakeMemberService{isMember: true}
	policySvc := &fakePolicyService{allow: false}
	chatSvc := &fakeChatService{}
	gateway := &fakeChatGateway{}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", 0)
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

func TestChannelInboundProcessorSilentReply(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-4"}}
	memberSvc := &fakeMemberService{isMember: true}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-4", RouteID: "route-4"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("NO_REPLY")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, memberSvc, nil, nil, nil, "", 0)
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
			Type: "p2p",
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
	memberSvc := &fakeMemberService{isMember: true}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-5", RouteID: "route-5"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, memberSvc, nil, nil, nil, "", 0)
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
		t.Fatalf("expected 1 passive persisted message, got: %d", len(chatSvc.persisted))
	}
	if chatSvc.persisted[0].Role != "user" {
		t.Fatalf("expected persisted role user, got: %s", chatSvc.persisted[0].Role)
	}
	if chatSvc.persisted[0].BotID != "bot-1" {
		t.Fatalf("expected passive persisted bot_id bot-1, got: %s", chatSvc.persisted[0].BotID)
	}
}

func TestChannelInboundProcessorGroupMentionTriggersReply(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-6"}}
	memberSvc := &fakeMemberService{isMember: true}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-6", RouteID: "route-6"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, memberSvc, nil, nil, nil, "", 0)
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
	if len(chatSvc.persisted) != 1 {
		t.Fatalf("triggered group message should persist inbound user once, got: %d", len(chatSvc.persisted))
	}
	if got := chatSvc.persisted[0].Metadata["trigger_mode"]; got != "active_chat" {
		t.Fatalf("expected trigger_mode active_chat, got: %v", got)
	}
	if !gateway.gotReq.UserMessagePersisted {
		t.Fatalf("expected UserMessagePersisted=true for pre-persisted inbound message")
	}
}

func TestChannelInboundProcessorPersonalGroupNonOwnerIgnored(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-member"}}
	memberSvc := &fakeMemberService{isMember: true}
	policySvc := &fakePolicyService{allow: false, botType: "personal", ownerUserID: "channelIdentity-owner"}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-personal-1", RouteID: "route-personal-1"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", 0)
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
	if len(chatSvc.persisted) != 0 {
		t.Fatalf("ignored message should not persist in passive mode")
	}
}

func TestChannelInboundProcessorPersonalGroupOwnerWithoutMentionUsesPassivePersistence(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-owner"}}
	memberSvc := &fakeMemberService{isMember: true}
	policySvc := &fakePolicyService{allow: false, botType: "personal", ownerUserID: "channelIdentity-owner"}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-personal-2", RouteID: "route-personal-2"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, chatSvc, gateway, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", 0)
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
		t.Fatalf("expected one passive persisted message, got: %d", len(chatSvc.persisted))
	}
	if got := chatSvc.persisted[0].Metadata["trigger_mode"]; got != "passive_sync" {
		t.Fatalf("expected trigger_mode passive_sync, got: %v", got)
	}
}

func TestChannelInboundProcessorProcessingStatusSuccessLifecycle(t *testing.T) {
	notifier := &fakeProcessingStatusNotifier{
		startedHandle: channel.ProcessingStatusHandle{Token: "reaction-1"},
	}
	registry := channel.NewRegistry()
	registry.MustRegister(&fakeProcessingStatusAdapter{notifier: notifier})
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-1"}}
	memberSvc := &fakeMemberService{isMember: true}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-1", RouteID: "route-1"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("AI reply")},
			},
		},
		onChat: func(req conversation.ChatRequest) {
			if len(notifier.events) != 1 || notifier.events[0] != "started" {
				t.Fatalf("expected started before chat call, got events: %+v", notifier.events)
			}
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), registry, chatSvc, chatSvc, gateway, channelIdentitySvc, memberSvc, nil, nil, nil, "", 0)
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
			Type: "p2p",
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
	memberSvc := &fakeMemberService{isMember: true}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-2", RouteID: "route-2"}}
	chatErr := errors.New("chat gateway unavailable")
	gateway := &fakeChatGateway{err: chatErr}
	processor := NewChannelInboundProcessor(slog.Default(), registry, chatSvc, chatSvc, gateway, channelIdentitySvc, memberSvc, nil, nil, nil, "", 0)
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
			Type: "p2p",
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
	memberSvc := &fakeMemberService{isMember: true}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-3", RouteID: "route-3"}}
	gateway := &fakeChatGateway{
		resp: conversation.ChatResponse{
			Messages: []conversation.ModelMessage{
				{Role: "assistant", Content: conversation.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), registry, chatSvc, chatSvc, gateway, channelIdentitySvc, memberSvc, nil, nil, nil, "", 0)
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
			Type: "p2p",
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
	memberSvc := &fakeMemberService{isMember: true}
	chatSvc := &fakeChatService{resolveResult: route.ResolveConversationResult{ChatID: "chat-4", RouteID: "route-4"}}
	chatErr := errors.New("chat failed")
	gateway := &fakeChatGateway{err: chatErr}
	processor := NewChannelInboundProcessor(slog.Default(), registry, chatSvc, chatSvc, gateway, channelIdentitySvc, memberSvc, nil, nil, nil, "", 0)
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
			Type: "p2p",
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
