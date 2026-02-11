package router

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channelidentities"
	"github.com/memohai/memoh/internal/chat"
)

type fakeChatGateway struct {
	resp   chat.ChatResponse
	err    error
	gotReq chat.ChatRequest
}

func (f *fakeChatGateway) Chat(ctx context.Context, req chat.ChatRequest) (chat.ChatResponse, error) {
	f.gotReq = req
	return f.resp, f.err
}

type fakeReplySender struct {
	sent []channel.OutboundMessage
}

func (s *fakeReplySender) Send(ctx context.Context, msg channel.OutboundMessage) error {
	s.sent = append(s.sent, msg)
	return nil
}

type fakeChatService struct {
	resolveResult chat.ResolveChatResult
	resolveErr    error
	persisted     []chat.Message
}

func (f *fakeChatService) ResolveChat(ctx context.Context, botID, platform, conversationID, threadID, conversationType, userID, channelConfigID, replyTarget string) (chat.ResolveChatResult, error) {
	if f.resolveErr != nil {
		return chat.ResolveChatResult{}, f.resolveErr
	}
	return f.resolveResult, nil
}

func (f *fakeChatService) PersistMessage(ctx context.Context, chatID, botID, routeID, senderChannelIdentityID, senderUserID, platform, externalMessageID, role string, content json.RawMessage, metadata map[string]any) (chat.Message, error) {
	msg := chat.Message{
		ChatID:                  chatID,
		BotID:                   botID,
		RouteID:                 routeID,
		SenderChannelIdentityID: senderChannelIdentityID,
		SenderUserID:            senderUserID,
		Platform:                platform,
		ExternalMessageID:       externalMessageID,
		Role:                    role,
		Content:                 content,
		Metadata:                metadata,
	}
	f.persisted = append(f.persisted, msg)
	return msg, nil
}

func TestChannelInboundProcessorWithIdentity(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: channelidentities.ChannelIdentity{ID: "channelIdentity-1"}}
	memberSvc := &fakeMemberService{isMember: true}
	policySvc := &fakePolicyService{allow: false}
	chatSvc := &fakeChatService{resolveResult: chat.ResolveChatResult{ChatID: "chat-1", RouteID: "route-1"}}
	gateway := &fakeChatGateway{
		resp: chat.ChatResponse{
			Messages: []chat.ModelMessage{
				{Role: "assistant", Content: chat.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, gateway, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", 0)
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
	if gateway.gotReq.ChatID != "chat-1" {
		t.Errorf("expected chat_id 'chat-1', got: %s", gateway.gotReq.ChatID)
	}
	if len(sender.sent) != 1 || sender.sent[0].Message.PlainText() != "AI reply" {
		t.Fatalf("expected AI reply, got: %+v", sender.sent)
	}
}

func TestChannelInboundProcessorDenied(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: channelidentities.ChannelIdentity{ID: "channelIdentity-2"}}
	memberSvc := &fakeMemberService{isMember: false}
	policySvc := &fakePolicyService{allow: false}
	chatSvc := &fakeChatService{}
	gateway := &fakeChatGateway{}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, gateway, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", 0)
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
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: channelidentities.ChannelIdentity{ID: "channelIdentity-3"}}
	memberSvc := &fakeMemberService{isMember: true}
	policySvc := &fakePolicyService{allow: false}
	chatSvc := &fakeChatService{}
	gateway := &fakeChatGateway{}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, gateway, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", 0)
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
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: channelidentities.ChannelIdentity{ID: "channelIdentity-4"}}
	memberSvc := &fakeMemberService{isMember: true}
	chatSvc := &fakeChatService{resolveResult: chat.ResolveChatResult{ChatID: "chat-4", RouteID: "route-4"}}
	gateway := &fakeChatGateway{
		resp: chat.ChatResponse{
			Messages: []chat.ModelMessage{
				{Role: "assistant", Content: chat.NewTextContent("NO_REPLY")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, gateway, channelIdentitySvc, memberSvc, nil, nil, nil, "", 0)
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
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: channelidentities.ChannelIdentity{ID: "channelIdentity-5"}}
	memberSvc := &fakeMemberService{isMember: true}
	chatSvc := &fakeChatService{resolveResult: chat.ResolveChatResult{ChatID: "chat-5", RouteID: "route-5"}}
	gateway := &fakeChatGateway{
		resp: chat.ChatResponse{
			Messages: []chat.ModelMessage{
				{Role: "assistant", Content: chat.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, gateway, channelIdentitySvc, memberSvc, nil, nil, nil, "", 0)
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
}

func TestChannelInboundProcessorGroupMentionTriggersReply(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: channelidentities.ChannelIdentity{ID: "channelIdentity-6"}}
	memberSvc := &fakeMemberService{isMember: true}
	chatSvc := &fakeChatService{resolveResult: chat.ResolveChatResult{ChatID: "chat-6", RouteID: "route-6"}}
	gateway := &fakeChatGateway{
		resp: chat.ChatResponse{
			Messages: []chat.ModelMessage{
				{Role: "assistant", Content: chat.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, gateway, channelIdentitySvc, memberSvc, nil, nil, nil, "", 0)
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
	if len(chatSvc.persisted) != 0 {
		t.Fatalf("triggered group message should not use passive persistence")
	}
}

func TestChannelInboundProcessorPersonalGroupNonOwnerIgnored(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: channelidentities.ChannelIdentity{ID: "channelIdentity-member"}}
	memberSvc := &fakeMemberService{isMember: true}
	policySvc := &fakePolicyService{allow: false, botType: "personal", ownerUserID: "channelIdentity-owner"}
	chatSvc := &fakeChatService{resolveResult: chat.ResolveChatResult{ChatID: "chat-personal-1", RouteID: "route-personal-1"}}
	gateway := &fakeChatGateway{
		resp: chat.ChatResponse{
			Messages: []chat.ModelMessage{
				{Role: "assistant", Content: chat.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, gateway, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", 0)
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

func TestChannelInboundProcessorPersonalGroupOwnerForceReply(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: channelidentities.ChannelIdentity{ID: "channelIdentity-owner"}}
	memberSvc := &fakeMemberService{isMember: true}
	policySvc := &fakePolicyService{allow: false, botType: "personal", ownerUserID: "channelIdentity-owner"}
	chatSvc := &fakeChatService{resolveResult: chat.ResolveChatResult{ChatID: "chat-personal-2", RouteID: "route-personal-2"}}
	gateway := &fakeChatGateway{
		resp: chat.ChatResponse{
			Messages: []chat.ModelMessage{
				{Role: "assistant", Content: chat.NewTextContent("AI reply")},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), nil, chatSvc, gateway, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", 0)
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
	if gateway.gotReq.Query == "" {
		t.Fatalf("owner should trigger chat call in personal group")
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected one owner reply, got %d", len(sender.sent))
	}
}
