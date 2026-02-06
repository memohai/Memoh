package router

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/contacts"
	"github.com/memohai/memoh/internal/policy"
	"github.com/memohai/memoh/internal/preauth"
)

type fakePolicyServiceIdentity struct {
	decision policy.Decision
	err      error
}

func (f *fakePolicyServiceIdentity) Resolve(ctx context.Context, botID string) (policy.Decision, error) {
	if f.err != nil {
		return policy.Decision{}, f.err
	}
	decision := f.decision
	if decision.BotID == "" {
		decision.BotID = botID
	}
	return decision, nil
}

type fakeIdentityConfigStore struct{}

func (f *fakeIdentityConfigStore) ResolveEffectiveConfig(ctx context.Context, botID string, channelType channel.ChannelType) (channel.ChannelConfig, error) {
	return channel.ChannelConfig{}, nil
}

func (f *fakeIdentityConfigStore) GetUserConfig(ctx context.Context, actorUserID string, channelType channel.ChannelType) (channel.ChannelUserBinding, error) {
	return channel.ChannelUserBinding{}, fmt.Errorf("not implemented")
}

func (f *fakeIdentityConfigStore) UpsertUserConfig(ctx context.Context, actorUserID string, channelType channel.ChannelType, req channel.UpsertUserConfigRequest) (channel.ChannelUserBinding, error) {
	return channel.ChannelUserBinding{}, nil
}

func (f *fakeIdentityConfigStore) ListConfigsByType(ctx context.Context, channelType channel.ChannelType) ([]channel.ChannelConfig, error) {
	return nil, nil
}

func (f *fakeIdentityConfigStore) ResolveUserBinding(ctx context.Context, channelType channel.ChannelType, criteria channel.BindingCriteria) (string, error) {
	return "", fmt.Errorf("channel user binding not found")
}

func (f *fakeIdentityConfigStore) ListSessionsByBotPlatform(ctx context.Context, botID string, platform string) ([]channel.ChannelSession, error) {
	return nil, nil
}

func (f *fakeIdentityConfigStore) GetChannelSession(ctx context.Context, sessionID string) (channel.ChannelSession, error) {
	return channel.ChannelSession{}, nil
}

func (f *fakeIdentityConfigStore) UpsertChannelSession(ctx context.Context, sessionID string, botID string, channelConfigID string, userID string, contactID string, platform string, replyTarget string, threadID string, metadata map[string]any) error {
	return nil
}

type fakeIdentityContactService struct {
	createGuestCalled bool
	upsertCalled      bool
}

func (f *fakeIdentityContactService) GetByID(ctx context.Context, contactID string) (contacts.Contact, error) {
	return contacts.Contact{}, fmt.Errorf("not found")
}

func (f *fakeIdentityContactService) GetByUserID(ctx context.Context, botID, userID string) (contacts.Contact, error) {
	return contacts.Contact{}, fmt.Errorf("not found")
}

func (f *fakeIdentityContactService) GetByChannelIdentity(ctx context.Context, botID, platform, externalID string) (contacts.ContactChannel, error) {
	return contacts.ContactChannel{}, fmt.Errorf("not found")
}

func (f *fakeIdentityContactService) Create(ctx context.Context, req contacts.CreateRequest) (contacts.Contact, error) {
	return contacts.Contact{ID: "contact-1", BotID: req.BotID}, nil
}

func (f *fakeIdentityContactService) CreateGuest(ctx context.Context, botID, displayName string) (contacts.Contact, error) {
	f.createGuestCalled = true
	return contacts.Contact{ID: "contact-guest", BotID: botID}, nil
}

func (f *fakeIdentityContactService) UpsertChannel(ctx context.Context, botID, contactID, platform, externalID string, metadata map[string]any) (contacts.ContactChannel, error) {
	f.upsertCalled = true
	return contacts.ContactChannel{ID: "channel-1", ContactID: contactID}, nil
}

type fakePreauthService struct {
	key      preauth.Key
	err      error
	markUsed bool
}

func (f *fakePreauthService) Get(ctx context.Context, token string) (preauth.Key, error) {
	if f.err != nil {
		return preauth.Key{}, f.err
	}
	if f.key.Token == "" || f.key.Token != token {
		return preauth.Key{}, preauth.ErrKeyNotFound
	}
	return f.key, nil
}

func (f *fakePreauthService) MarkUsed(ctx context.Context, id string) (preauth.Key, error) {
	f.markUsed = true
	return f.key, nil
}

func TestIdentityResolverAllowGuestCreatesContact(t *testing.T) {
	store := &fakeIdentityConfigStore{}
	contactsService := &fakeIdentityContactService{}
	policyService := &fakePolicyServiceIdentity{decision: policy.Decision{AllowGuest: true}}
	resolver := NewIdentityResolver(slog.Default(), store, contactsService, policyService, nil, "禁止访问", "授权成功")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "hello"},
		ReplyTarget: "target-id",
		Sender:      channel.Identity{ExternalID: "user-1", DisplayName: "访客"},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if state.Identity.ContactID != "contact-guest" {
		t.Fatalf("应创建访客联系人，实际: %s", state.Identity.ContactID)
	}
	if !contactsService.createGuestCalled {
		t.Fatalf("应调用 CreateGuest")
	}
}

func TestIdentityResolverPreauthKeyAllowsGuest(t *testing.T) {
	store := &fakeIdentityConfigStore{}
	contactsService := &fakeIdentityContactService{}
	policyService := &fakePolicyServiceIdentity{}
	preauthService := &fakePreauthService{
		key: preauth.Key{
			ID:        "key-1",
			BotID:     "bot-1",
			Token:     "PREAUTH123",
			ExpiresAt: time.Now().UTC().Add(1 * time.Hour),
		},
	}
	resolver := NewIdentityResolver(slog.Default(), store, contactsService, policyService, preauthService, "禁止访问", "授权成功")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "PREAUTH123"},
		ReplyTarget: "target-id",
		Sender:      channel.Identity{ExternalID: "user-1"},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if state.Decision == nil || !state.Decision.Stop {
		t.Fatalf("应返回授权确认")
	}
	if !contactsService.upsertCalled {
		t.Fatalf("应执行联系人绑定")
	}
	if !preauthService.markUsed {
		t.Fatalf("应标记预授权码已使用")
	}
}

func TestIdentityResolverPreauthKeyExpired(t *testing.T) {
	store := &fakeIdentityConfigStore{}
	contactsService := &fakeIdentityContactService{}
	policyService := &fakePolicyServiceIdentity{}
	preauthService := &fakePreauthService{
		key: preauth.Key{
			ID:        "key-1",
			BotID:     "bot-1",
			Token:     "PREAUTH123",
			ExpiresAt: time.Now().UTC().Add(-1 * time.Hour),
		},
	}
	resolver := NewIdentityResolver(slog.Default(), store, contactsService, policyService, preauthService, "禁止访问", "授权成功")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "PREAUTH123"},
		ReplyTarget: "target-id",
		Sender:      channel.Identity{ExternalID: "user-1"},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if state.Decision == nil || !state.Decision.Stop {
		t.Fatalf("过期预授权码应被拒绝")
	}
	if preauthService.markUsed {
		t.Fatalf("过期预授权码不应被使用")
	}
}
