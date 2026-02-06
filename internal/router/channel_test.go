package router

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/chat"
	"github.com/memohai/memoh/internal/contacts"
	"github.com/memohai/memoh/internal/policy"
)

type fakeConfigStore struct {
	session     channel.ChannelSession
	boundUserID string
}

func (f *fakeConfigStore) ResolveEffectiveConfig(ctx context.Context, botID string, channelType channel.ChannelType) (channel.ChannelConfig, error) {
	return channel.ChannelConfig{}, nil
}

func (f *fakeConfigStore) GetUserConfig(ctx context.Context, actorUserID string, channelType channel.ChannelType) (channel.ChannelUserBinding, error) {
	return channel.ChannelUserBinding{}, fmt.Errorf("not implemented")
}

func (f *fakeConfigStore) UpsertUserConfig(ctx context.Context, actorUserID string, channelType channel.ChannelType, req channel.UpsertUserConfigRequest) (channel.ChannelUserBinding, error) {
	return channel.ChannelUserBinding{}, nil
}

func (f *fakeConfigStore) ListConfigsByType(ctx context.Context, channelType channel.ChannelType) ([]channel.ChannelConfig, error) {
	return nil, nil
}

func (f *fakeConfigStore) ResolveUserBinding(ctx context.Context, channelType channel.ChannelType, criteria channel.BindingCriteria) (string, error) {
	if f.boundUserID == "" {
		return "", fmt.Errorf("channel user binding not found")
	}
	return f.boundUserID, nil
}

func (f *fakeConfigStore) ListSessionsByBotPlatform(ctx context.Context, botID, platform string) ([]channel.ChannelSession, error) {
	return nil, nil
}

func (f *fakeConfigStore) GetChannelSession(ctx context.Context, sessionID string) (channel.ChannelSession, error) {
	if f.session.SessionID == sessionID {
		return f.session, nil
	}
	return channel.ChannelSession{}, nil
}

func (f *fakeConfigStore) UpsertChannelSession(ctx context.Context, sessionID string, botID string, channelConfigID string, userID string, contactID string, platform string, replyTarget string, threadID string, metadata map[string]any) error {
	return nil
}

type fakeChatGateway struct {
	resp   chat.ChatResponse
	err    error
	gotReq chat.ChatRequest
}

func (f *fakeChatGateway) Chat(ctx context.Context, req chat.ChatRequest) (chat.ChatResponse, error) {
	f.gotReq = req
	return f.resp, f.err
}

type fakeContactService struct {
	contactID string
}

func (f *fakeContactService) GetByID(ctx context.Context, contactID string) (contacts.Contact, error) {
	return contacts.Contact{}, fmt.Errorf("not found")
}

func (f *fakeContactService) GetByUserID(ctx context.Context, botID, userID string) (contacts.Contact, error) {
	return contacts.Contact{}, fmt.Errorf("not found")
}

func (f *fakeContactService) GetByChannelIdentity(ctx context.Context, botID, platform, externalID string) (contacts.ContactChannel, error) {
	return contacts.ContactChannel{}, fmt.Errorf("not found")
}

func (f *fakeContactService) Create(ctx context.Context, req contacts.CreateRequest) (contacts.Contact, error) {
	return contacts.Contact{ID: "contact-1", BotID: req.BotID, UserID: req.UserID}, nil
}

func (f *fakeContactService) CreateGuest(ctx context.Context, botID, displayName string) (contacts.Contact, error) {
	return contacts.Contact{ID: "contact-guest", BotID: botID}, nil
}

func (f *fakeContactService) UpsertChannel(ctx context.Context, botID, contactID, platform, externalID string, metadata map[string]any) (contacts.ContactChannel, error) {
	return contacts.ContactChannel{ID: "channel-1", ContactID: contactID}, nil
}

type fakePolicyService struct {
	decision policy.Decision
	err      error
}

func (f *fakePolicyService) Resolve(ctx context.Context, botID string) (policy.Decision, error) {
	if f.err != nil {
		return policy.Decision{}, f.err
	}
	decision := f.decision
	if decision.BotID == "" {
		decision.BotID = botID
	}
	return decision, nil
}

type fakeReplySender struct {
	sent []channel.OutboundMessage
}

func (s *fakeReplySender) Send(ctx context.Context, msg channel.OutboundMessage) error {
	s.sent = append(s.sent, msg)
	return nil
}

func TestChannelInboundProcessorBoundUser(t *testing.T) {
	store := &fakeConfigStore{
		session: channel.ChannelSession{
			SessionID: "feishu:bot-1:chat-1",
			UserID:    "user-123",
		},
	}
	gateway := &fakeChatGateway{
		resp: chat.ChatResponse{
			Messages: []chat.GatewayMessage{
				{"role": "assistant", "content": "AI回复内容"},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), store, gateway, &fakeContactService{}, &fakePolicyService{}, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}
	msg := channel.InboundMessage{
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "你好"},
		ReplyTarget: "target-id",
		Conversation: channel.Conversation{
			ID:   "chat-1",
			Type: "p2p",
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if gateway.gotReq.Query != "你好" {
		t.Errorf("Chat 请求 Query 错误: %s", gateway.gotReq.Query)
	}
	if gateway.gotReq.SessionID != "feishu:bot-1:chat-1" {
		t.Errorf("SessionID 传递错误: %s", gateway.gotReq.SessionID)
	}
	if len(sender.sent) != 1 || sender.sent[0].Message.PlainText() != "AI回复内容" {
		t.Fatalf("应发送 AI 回复，实际: %+v", sender.sent)
	}
}

func TestChannelInboundProcessorUnboundUser(t *testing.T) {
	store := &fakeConfigStore{}
	gateway := &fakeChatGateway{}
	processor := NewChannelInboundProcessor(slog.Default(), store, gateway, &fakeContactService{}, &fakePolicyService{}, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}
	msg := channel.InboundMessage{
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "你好"},
		ReplyTarget: "target-id",
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if len(sender.sent) != 1 || !strings.Contains(sender.sent[0].Message.PlainText(), "陌生人") {
		t.Fatalf("应发送绑定提示，实际: %+v", sender.sent)
	}
	if gateway.gotReq.Query != "" {
		t.Error("未绑定用户不应触发 Chat 调用")
	}
}

func TestChannelInboundProcessorIgnoreEmpty(t *testing.T) {
	store := &fakeConfigStore{}
	gateway := &fakeChatGateway{}
	processor := NewChannelInboundProcessor(slog.Default(), store, gateway, &fakeContactService{}, &fakePolicyService{}, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1"}
	msg := channel.InboundMessage{Message: channel.Message{Text: "  "}}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("空消息不应报错: %v", err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("空消息不应发送回复: %+v", sender.sent)
	}
	if gateway.gotReq.Query != "" {
		t.Error("空消息不应触发 Chat 调用")
	}
}

func TestChannelInboundProcessorSilentReply(t *testing.T) {
	store := &fakeConfigStore{
		session: channel.ChannelSession{
			SessionID: "feishu:bot-1:chat-1",
			UserID:    "user-123",
		},
	}
	gateway := &fakeChatGateway{
		resp: chat.ChatResponse{
			Messages: []chat.GatewayMessage{
				{"role": "assistant", "content": "NO_REPLY"},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), store, gateway, &fakeContactService{}, &fakePolicyService{}, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}
	msg := channel.InboundMessage{
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "你好"},
		ReplyTarget: "target-id",
		Conversation: channel.Conversation{
			ID:   "chat-1",
			Type: "p2p",
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("NO_REPLY 不应发送回复，实际: %+v", sender.sent)
	}
}

func TestChannelInboundProcessorSuppressOnToolSend(t *testing.T) {
	store := &fakeConfigStore{
		session: channel.ChannelSession{
			SessionID: "feishu:bot-1:chat-1",
			UserID:    "user-123",
		},
	}
	gateway := &fakeChatGateway{
		resp: chat.ChatResponse{
			Messages: []chat.GatewayMessage{
				{
					"role": "assistant",
					"tool_calls": []any{
						map[string]any{
							"type": "function",
							"function": map[string]any{
								"name":      "send_message",
								"arguments": `{"platform":"feishu","target":"target-id","message":{"text":"AI回复内容"}}`,
							},
						},
					},
				},
				{"role": "assistant", "content": "AI回复内容"},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), store, gateway, &fakeContactService{}, &fakePolicyService{}, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}
	msg := channel.InboundMessage{
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "你好"},
		ReplyTarget: "target-id",
		Conversation: channel.Conversation{
			ID:   "chat-1",
			Type: "p2p",
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("工具已发送当前会话消息，应抑制普通回复，实际: %+v", sender.sent)
	}
}

func TestChannelInboundProcessorDedupeWithToolSend(t *testing.T) {
	store := &fakeConfigStore{
		session: channel.ChannelSession{
			SessionID: "feishu:bot-1:chat-1",
			UserID:    "user-123",
		},
	}
	gateway := &fakeChatGateway{
		resp: chat.ChatResponse{
			Messages: []chat.GatewayMessage{
				{
					"role": "assistant",
					"tool_calls": []any{
						map[string]any{
							"type": "function",
							"function": map[string]any{
								"name":      "send_message",
								"arguments": `{"platform":"feishu","target":"other-target","message":{"text":"AI回复内容"}}`,
							},
						},
					},
				},
				{"role": "assistant", "content": "AI回复内容"},
			},
		},
	}
	processor := NewChannelInboundProcessor(slog.Default(), store, gateway, &fakeContactService{}, &fakePolicyService{}, nil, "", 0)
	sender := &fakeReplySender{}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}
	msg := channel.InboundMessage{
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "你好"},
		ReplyTarget: "target-id",
		Conversation: channel.Conversation{
			ID:   "chat-1",
			Type: "p2p",
		},
	}

	err := processor.HandleInbound(context.Background(), cfg, msg, sender)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("工具发送文本与普通回复重复，应去重，实际: %+v", sender.sent)
	}
}
