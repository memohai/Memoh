package inbound

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/bind"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/preauth"
)

type fakeChannelIdentityService struct {
	channelIdentity identities.ChannelIdentity
	bySubject       map[string]identities.ChannelIdentity
	err             error
	canonical       map[string]string
	linked          map[string]string
	calls           int
	lastDisplayName string
	lastMeta        map[string]any
}

func (f *fakeChannelIdentityService) ResolveByChannelIdentity(ctx context.Context, platform, externalID, displayName string, meta map[string]any) (identities.ChannelIdentity, error) {
	f.calls++
	f.lastDisplayName = displayName
	f.lastMeta = meta
	if f.err != nil {
		return identities.ChannelIdentity{}, f.err
	}
	if f.bySubject != nil {
		if identity, ok := f.bySubject[externalID]; ok {
			return identity, nil
		}
		return identities.ChannelIdentity{}, nil
	}
	return f.channelIdentity, nil
}

func (f *fakeChannelIdentityService) Canonicalize(ctx context.Context, channelIdentityID string) (string, error) {
	if f.canonical != nil {
		if value, ok := f.canonical[channelIdentityID]; ok {
			return value, nil
		}
	}
	return channelIdentityID, nil
}

func (f *fakeChannelIdentityService) GetLinkedUserID(ctx context.Context, channelIdentityID string) (string, error) {
	if f.linked != nil {
		if value, ok := f.linked[channelIdentityID]; ok {
			return value, nil
		}
		return "", nil
	}
	// Default to one-to-one mapping for tests that do not set explicit links.
	return channelIdentityID, nil
}

func (f *fakeChannelIdentityService) LinkChannelIdentityToUser(ctx context.Context, channelIdentityID, userID string) error {
	if f.linked == nil {
		f.linked = map[string]string{}
	}
	f.linked[channelIdentityID] = userID
	return nil
}

type fakeMemberService struct {
	isMember     bool
	upsertCalled bool
}

func (f *fakeMemberService) IsMember(ctx context.Context, botID, channelIdentityID string) (bool, error) {
	return f.isMember, nil
}

func (f *fakeMemberService) UpsertMemberSimple(ctx context.Context, botID, channelIdentityID, role string) error {
	f.upsertCalled = true
	return nil
}

type fakePolicyService struct {
	allow       bool
	botType     string
	ownerUserID string
	err         error
}

func (f *fakePolicyService) AllowGuest(ctx context.Context, botID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.allow, nil
}

func (f *fakePolicyService) BotType(ctx context.Context, botID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.botType, nil
}

func (f *fakePolicyService) BotOwnerUserID(ctx context.Context, botID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.ownerUserID, nil
}

type fakePreauthServiceIdentity struct {
	key      preauth.Key
	err      error
	markUsed bool
}

func (f *fakePreauthServiceIdentity) Get(ctx context.Context, token string) (preauth.Key, error) {
	if f.err != nil {
		return preauth.Key{}, f.err
	}
	if f.key.Token == "" || f.key.Token != token {
		return preauth.Key{}, preauth.ErrKeyNotFound
	}
	return f.key, nil
}

func (f *fakePreauthServiceIdentity) MarkUsed(ctx context.Context, id string) (preauth.Key, error) {
	f.markUsed = true
	return f.key, nil
}

type fakeBindService struct {
	code          bind.Code
	getErr        error
	consumeErr    error
	consumeCalled bool
	onConsume     func(channelChannelIdentityID string)
}

func (f *fakeBindService) Get(ctx context.Context, token string) (bind.Code, error) {
	if f.getErr != nil {
		return bind.Code{}, f.getErr
	}
	if f.code.Token == "" || f.code.Token != token {
		return bind.Code{}, bind.ErrCodeNotFound
	}
	return f.code, nil
}

func (f *fakeBindService) Consume(ctx context.Context, code bind.Code, channelChannelIdentityID string) error {
	f.consumeCalled = true
	if f.onConsume != nil {
		f.onConsume(channelChannelIdentityID)
	}
	return f.consumeErr
}

type fakeDirectoryAdapter struct {
	channelType channel.ChannelType
	resolveFn   func(ctx context.Context, cfg channel.ChannelConfig, input string, kind channel.DirectoryEntryKind) (channel.DirectoryEntry, error)
}

func (f *fakeDirectoryAdapter) Type() channel.ChannelType {
	return f.channelType
}

func (f *fakeDirectoryAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        f.channelType,
		DisplayName: "FakeDirectory",
		Capabilities: channel.ChannelCapabilities{},
	}
}

func (f *fakeDirectoryAdapter) ListPeers(ctx context.Context, cfg channel.ChannelConfig, query channel.DirectoryQuery) ([]channel.DirectoryEntry, error) {
	return nil, nil
}

func (f *fakeDirectoryAdapter) ListGroups(ctx context.Context, cfg channel.ChannelConfig, query channel.DirectoryQuery) ([]channel.DirectoryEntry, error) {
	return nil, nil
}

func (f *fakeDirectoryAdapter) ListGroupMembers(ctx context.Context, cfg channel.ChannelConfig, groupID string, query channel.DirectoryQuery) ([]channel.DirectoryEntry, error) {
	return nil, nil
}

func (f *fakeDirectoryAdapter) ResolveEntry(ctx context.Context, cfg channel.ChannelConfig, input string, kind channel.DirectoryEntryKind) (channel.DirectoryEntry, error) {
	if f.resolveFn != nil {
		return f.resolveFn(ctx, cfg, input, kind)
	}
	return channel.DirectoryEntry{}, errors.New("resolve not implemented")
}

func TestIdentityResolverAllowGuestWithoutMembershipSideEffect(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-1"}}
	memberSvc := &fakeMemberService{isMember: false}
	policySvc := &fakePolicyService{allow: true, botType: "public"}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", "")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "hello"},
		ReplyTarget: "target-id",
		Sender:      channel.Identity{SubjectID: "ext-1", DisplayName: "Guest"},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Identity.ChannelIdentityID != "channelIdentity-1" {
		t.Fatalf("expected channelIdentity-1, got: %s", state.Identity.ChannelIdentityID)
	}
	if memberSvc.upsertCalled {
		t.Fatal("guest allow should not upsert membership")
	}
	if state.Decision != nil {
		t.Fatal("expected no decision for allowed guest")
	}
}

func TestIdentityResolverResolveDisplayNameFromDirectory(t *testing.T) {
	registry := channel.NewRegistry()
	directoryAdapter := &fakeDirectoryAdapter{
		channelType: channel.ChannelType("feishu"),
		resolveFn: func(ctx context.Context, cfg channel.ChannelConfig, input string, kind channel.DirectoryEntryKind) (channel.DirectoryEntry, error) {
			if kind != channel.DirectoryEntryUser {
				t.Fatalf("expected kind user, got %s", kind)
			}
			if input != "ou-directory" {
				t.Fatalf("expected subject id ou-directory, got %s", input)
			}
			return channel.DirectoryEntry{
				Kind: channel.DirectoryEntryUser,
				Name: "Directory Name",
			}, nil
		},
	}
	if err := registry.Register(directoryAdapter); err != nil {
		t.Fatalf("register directory adapter failed: %v", err)
	}

	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-directory"}}
	memberSvc := &fakeMemberService{isMember: true}
	policySvc := &fakePolicyService{allow: false, botType: "public"}
	resolver := NewIdentityResolver(slog.Default(), registry, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", "")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "hello"},
		ReplyTarget: "target-id",
		Sender: channel.Identity{
			SubjectID: "ou-directory",
			Attributes: map[string]string{
				"open_id": "ou-directory",
			},
		},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Identity.DisplayName != "Directory Name" {
		t.Fatalf("expected directory display name, got %q", state.Identity.DisplayName)
	}
	if channelIdentitySvc.lastDisplayName != "Directory Name" {
		t.Fatalf("expected upsert display name Directory Name, got %q", channelIdentitySvc.lastDisplayName)
	}
}

func TestIdentityResolverDirectoryLookupFailureDoesNotFallbackToOpenID(t *testing.T) {
	registry := channel.NewRegistry()
	directoryAdapter := &fakeDirectoryAdapter{
		channelType: channel.ChannelType("feishu"),
		resolveFn: func(ctx context.Context, cfg channel.ChannelConfig, input string, kind channel.DirectoryEntryKind) (channel.DirectoryEntry, error) {
			return channel.DirectoryEntry{}, errors.New("lookup failed")
		},
	}
	if err := registry.Register(directoryAdapter); err != nil {
		t.Fatalf("register directory adapter failed: %v", err)
	}

	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-directory-fail"}}
	memberSvc := &fakeMemberService{isMember: true}
	policySvc := &fakePolicyService{allow: false, botType: "public"}
	resolver := NewIdentityResolver(slog.Default(), registry, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", "")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "hello"},
		ReplyTarget: "target-id",
		Sender: channel.Identity{
			SubjectID: "ou-directory-fail",
			Attributes: map[string]string{
				"open_id": "ou-directory-fail",
				"user_id": "u-directory-fail",
			},
		},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Identity.DisplayName != "" {
		t.Fatalf("expected empty display name when directory lookup fails, got %q", state.Identity.DisplayName)
	}
	if channelIdentitySvc.lastDisplayName != "" {
		t.Fatalf("expected empty upsert display name on lookup failure, got %q", channelIdentitySvc.lastDisplayName)
	}
}

func TestIdentityResolverDirectoryAvatarURLPropagated(t *testing.T) {
	registry := channel.NewRegistry()
	directoryAdapter := &fakeDirectoryAdapter{
		channelType: channel.ChannelType("feishu"),
		resolveFn: func(ctx context.Context, cfg channel.ChannelConfig, input string, kind channel.DirectoryEntryKind) (channel.DirectoryEntry, error) {
			return channel.DirectoryEntry{
				Kind:      channel.DirectoryEntryUser,
				Name:      "Avatar User",
				AvatarURL: "https://example.com/avatar.png",
			}, nil
		},
	}
	if err := registry.Register(directoryAdapter); err != nil {
		t.Fatalf("register directory adapter failed: %v", err)
	}

	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-avatar"}}
	memberSvc := &fakeMemberService{isMember: true}
	policySvc := &fakePolicyService{allow: false, botType: "public"}
	resolver := NewIdentityResolver(slog.Default(), registry, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", "")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "hello"},
		ReplyTarget: "target-id",
		Sender: channel.Identity{
			SubjectID:  "ou-avatar",
			Attributes: map[string]string{"open_id": "ou-avatar"},
		},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1", ChannelType: channel.ChannelType("feishu")}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Identity.DisplayName != "Avatar User" {
		t.Fatalf("expected display name Avatar User, got %q", state.Identity.DisplayName)
	}
	if state.Identity.AvatarURL != "https://example.com/avatar.png" {
		t.Fatalf("expected avatar url, got %q", state.Identity.AvatarURL)
	}
	if channelIdentitySvc.lastMeta == nil {
		t.Fatal("expected metadata with avatar_url to be passed to channel identity service")
	}
	if channelIdentitySvc.lastMeta["avatar_url"] != "https://example.com/avatar.png" {
		t.Fatalf("expected avatar_url in meta, got %v", channelIdentitySvc.lastMeta)
	}
}

func TestIdentityResolverExistingMemberPasses(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-2"}}
	memberSvc := &fakeMemberService{isMember: true}
	policySvc := &fakePolicyService{allow: false, botType: "public"}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", "")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("telegram"),
		Message:     channel.Message{Text: "hello"},
		ReplyTarget: "chat-123",
		Sender:      channel.Identity{SubjectID: "tg-user-1"},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Decision != nil {
		t.Fatal("existing member should pass without decision")
	}
}

func TestIdentityResolverPreauthKey(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-3"}}
	memberSvc := &fakeMemberService{isMember: false}
	policySvc := &fakePolicyService{allow: false, botType: "public"}
	preauthSvc := &fakePreauthServiceIdentity{
		key: preauth.Key{
			ID:        "key-1",
			BotID:     "bot-1",
			Token:     "PREAUTH123",
			ExpiresAt: time.Now().UTC().Add(1 * time.Hour),
		},
	}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, memberSvc, policySvc, preauthSvc, nil, "", "")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "PREAUTH123"},
		ReplyTarget: "target-id",
		Sender:      channel.Identity{SubjectID: "ext-1"},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Decision == nil || !state.Decision.Stop {
		t.Fatal("preauth key should return stop decision")
	}
	if !preauthSvc.markUsed {
		t.Fatal("preauth key should be marked used")
	}
	if !memberSvc.upsertCalled {
		t.Fatal("membership should be upserted via preauth")
	}
}

func TestIdentityResolverPreauthKeyExpired(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-4"}}
	memberSvc := &fakeMemberService{isMember: false}
	policySvc := &fakePolicyService{allow: false, botType: "public"}
	preauthSvc := &fakePreauthServiceIdentity{
		key: preauth.Key{
			ID:        "key-1",
			BotID:     "bot-1",
			Token:     "PREAUTH123",
			ExpiresAt: time.Now().UTC().Add(-1 * time.Hour),
		},
	}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, memberSvc, policySvc, preauthSvc, nil, "", "")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "PREAUTH123"},
		ReplyTarget: "target-id",
		Sender:      channel.Identity{SubjectID: "ext-1"},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Decision == nil || !state.Decision.Stop {
		t.Fatal("expired preauth key should be rejected")
	}
	if preauthSvc.markUsed {
		t.Fatal("expired preauth key should not be marked used")
	}
}

func TestIdentityResolverDenied(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-5"}}
	memberSvc := &fakeMemberService{isMember: false}
	policySvc := &fakePolicyService{allow: false, botType: "public"}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, memberSvc, policySvc, nil, nil, "Access denied.", "")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("telegram"),
		Message:     channel.Message{Text: "hello"},
		ReplyTarget: "chat-123",
		Sender:      channel.Identity{SubjectID: "stranger-1"},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Decision == nil || !state.Decision.Stop {
		t.Fatal("stranger without guest access should be denied")
	}
}

func TestIdentityResolverPersonalBotRejectsGroupMessages(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-group"}}
	memberSvc := &fakeMemberService{isMember: true}
	policySvc := &fakePolicyService{allow: false, botType: "personal", ownerUserID: "channelIdentity-owner"}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", "")

	msg := channel.InboundMessage{
		BotID:   "bot-1",
		Channel: channel.ChannelType("feishu"),
		Message: channel.Message{Text: "hello"},
		Sender:  channel.Identity{SubjectID: "ext-group-1"},
		Conversation: channel.Conversation{
			ID:   "group-1",
			Type: "group",
		},
	}

	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Decision == nil || !state.Decision.Stop {
		t.Fatal("personal bot should reject group messages")
	}
	if channelIdentitySvc.calls != 1 {
		t.Fatalf("expected channelIdentity resolution once before owner check, got %d", channelIdentitySvc.calls)
	}
	if !state.Decision.Reply.IsEmpty() {
		t.Fatal("non-owner group message should be silently ignored")
	}
}

func TestIdentityResolverPersonalBotAllowsOwnerInGroup(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-owner"}}
	memberSvc := &fakeMemberService{isMember: false}
	policySvc := &fakePolicyService{allow: false, botType: "personal", ownerUserID: "channelIdentity-owner"}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", "")

	msg := channel.InboundMessage{
		BotID:   "bot-1",
		Channel: channel.ChannelType("feishu"),
		Message: channel.Message{Text: "hello from owner"},
		Sender:  channel.Identity{SubjectID: "ext-owner-1"},
		Conversation: channel.Conversation{
			ID:   "group-1",
			Type: "group",
		},
	}

	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Decision != nil {
		t.Fatal("owner group message should pass")
	}
	if state.Identity.ForceReply {
		t.Fatal("owner group message should not force reply")
	}
}

func TestIdentityResolverPersonalBotAllowsOwnerDirectWithoutMembership(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-owner-direct"}}
	memberSvc := &fakeMemberService{isMember: false}
	policySvc := &fakePolicyService{allow: false, botType: "personal", ownerUserID: "channelIdentity-owner-direct"}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", "")

	msg := channel.InboundMessage{
		BotID:   "bot-1",
		Channel: channel.ChannelType("feishu"),
		Message: channel.Message{Text: "hello from owner"},
		Sender:  channel.Identity{SubjectID: "ext-owner-direct"},
		Conversation: channel.Conversation{
			ID:   "p2p-1",
			Type: "p2p",
		},
	}

	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Decision != nil {
		t.Fatal("owner direct message should pass")
	}
	if state.Identity.ForceReply {
		t.Fatal("owner direct message should not force reply")
	}
}

func TestIdentityResolverPersonalBotOwnerFallbackByAlternateSubject(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{
		bySubject: map[string]identities.ChannelIdentity{
			"ou-open-owner": {ID: "channelIdentity-open-owner"},
			"u-owner":       {ID: "channelIdentity-user-owner"},
		},
		linked: map[string]string{
			"channelIdentity-user-owner": "owner-user-1",
		},
	}
	memberSvc := &fakeMemberService{isMember: false}
	policySvc := &fakePolicyService{allow: false, botType: "personal", ownerUserID: "owner-user-1"}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, memberSvc, policySvc, nil, nil, "", "")

	msg := channel.InboundMessage{
		BotID:   "bot-1",
		Channel: channel.ChannelType("feishu"),
		Message: channel.Message{Text: "hello from owner"},
		Sender: channel.Identity{
			SubjectID: "ou-open-owner",
			Attributes: map[string]string{
				"open_id": "ou-open-owner",
				"user_id": "u-owner",
			},
		},
		Conversation: channel.Conversation{
			ID:   "p2p-1",
			Type: "p2p",
		},
	}

	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Decision != nil {
		t.Fatal("owner direct message should pass after alternate subject fallback")
	}
	if state.Identity.UserID != "owner-user-1" {
		t.Fatalf("expected owner-user-1, got: %s", state.Identity.UserID)
	}
	if state.Identity.ChannelIdentityID != "channelIdentity-user-owner" {
		t.Fatalf("expected fallback channel identity, got: %s", state.Identity.ChannelIdentityID)
	}
	if channelIdentitySvc.calls < 2 {
		t.Fatalf("expected fallback resolution attempts, got calls=%d", channelIdentitySvc.calls)
	}
}

func TestIdentityResolverPersonalBotRejectsNonOwnerDirectEvenIfMember(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-non-owner"}}
	memberSvc := &fakeMemberService{isMember: true}
	policySvc := &fakePolicyService{allow: true, botType: "personal", ownerUserID: "channelIdentity-owner"}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, memberSvc, policySvc, nil, nil, "Access denied.", "")

	msg := channel.InboundMessage{
		BotID:   "bot-1",
		Channel: channel.ChannelType("feishu"),
		Message: channel.Message{Text: "hello from non-owner"},
		Sender:  channel.Identity{SubjectID: "ext-non-owner"},
		Conversation: channel.Conversation{
			ID:   "p2p-2",
			Type: "p2p",
		},
	}

	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Decision == nil || !state.Decision.Stop {
		t.Fatal("non-owner direct message should be rejected for personal bot")
	}
	if !state.Decision.Reply.IsEmpty() {
		t.Fatal("non-owner direct message should be silently ignored")
	}
}

func TestIdentityResolverBindRunsBeforeMembershipCheck(t *testing.T) {
	shadowID := "channelIdentity-shadow"
	humanID := "channelIdentity-human"
	channelIdentitySvc := &fakeChannelIdentityService{
		channelIdentity: identities.ChannelIdentity{ID: shadowID},
		linked: map[string]string{
			shadowID: shadowID,
		},
	}
	memberSvc := &fakeMemberService{isMember: true}
	bindSvc := &fakeBindService{
		code: bind.Code{
			ID:             "code-1",
			Platform:       "feishu",
			Token:          "BIND123",
			IssuedByUserID: humanID,
			ExpiresAt:      time.Now().UTC().Add(1 * time.Hour),
		},
		onConsume: func(channelChannelIdentityID string) {
			channelIdentitySvc.linked[channelChannelIdentityID] = humanID
		},
	}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, memberSvc, nil, nil, bindSvc, "", "")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "BIND123"},
		ReplyTarget: "target-id",
		Sender:      channel.Identity{SubjectID: "ext-bind-1"},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bindSvc.consumeCalled {
		t.Fatal("expected bind consume to run before membership shortcut")
	}
	if state.Decision == nil || !state.Decision.Stop {
		t.Fatal("bind flow should return stop decision")
	}
	if state.Identity.UserID != humanID {
		t.Fatalf("expected linked user to switch to %s, got %s", humanID, state.Identity.UserID)
	}
	if memberSvc.upsertCalled {
		t.Fatal("bind should not upsert bot membership")
	}
}

func TestIdentityResolverBindConsumeErrorHandledAsDecision(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-shadow"}}
	bindSvc := &fakeBindService{
		code: bind.Code{
			ID:             "code-2",
			Platform:       "telegram",
			Token:          "BINDUSED",
			IssuedByUserID: "channelIdentity-human",
			ExpiresAt:      time.Now().UTC().Add(1 * time.Hour),
		},
		consumeErr: bind.ErrCodeUsed,
	}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, &fakeMemberService{}, nil, nil, bindSvc, "", "")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("telegram"),
		Message:     channel.Message{Text: "BINDUSED"},
		ReplyTarget: "chat-123",
		Sender:      channel.Identity{SubjectID: "ext-bind-2"},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Decision == nil || !state.Decision.Stop {
		t.Fatal("bind consume errors should be converted into stop decision")
	}
}

func TestIdentityResolverBindCodeNotScopedToCurrentBot(t *testing.T) {
	shadowID := "channelIdentity-shadow-any-bot"
	humanID := "channelIdentity-human-any-bot"
	channelIdentitySvc := &fakeChannelIdentityService{
		channelIdentity: identities.ChannelIdentity{ID: shadowID},
		linked: map[string]string{
			shadowID: shadowID,
		},
	}
	bindSvc := &fakeBindService{
		code: bind.Code{
			ID:             "code-any-bot",
			Platform:       "feishu",
			Token:          "BINDANYBOT",
			IssuedByUserID: humanID,
			ExpiresAt:      time.Now().UTC().Add(1 * time.Hour),
		},
		onConsume: func(channelChannelIdentityID string) {
			channelIdentitySvc.linked[channelChannelIdentityID] = humanID
		},
	}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, &fakeMemberService{}, nil, nil, bindSvc, "", "")

	msg := channel.InboundMessage{
		BotID:       "bot-2",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "BINDANYBOT"},
		ReplyTarget: "target-id",
		Sender:      channel.Identity{SubjectID: "ext-bind-any-bot"},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bindSvc.consumeCalled {
		t.Fatal("bind consume should run even when message bot differs")
	}
	if state.Decision == nil || !state.Decision.Stop {
		t.Fatal("bind flow should return stop decision")
	}
	if state.Identity.UserID != humanID {
		t.Fatalf("expected linked user to switch to %s, got %s", humanID, state.Identity.UserID)
	}
}

func TestIdentityResolverPublicBotGroupDeniedSilently(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-group-denied"}}
	memberSvc := &fakeMemberService{isMember: false}
	policySvc := &fakePolicyService{allow: false, botType: "public"}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, memberSvc, policySvc, nil, nil, "Access denied.", "")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "hello"},
		ReplyTarget: "group-target",
		Sender:      channel.Identity{SubjectID: "stranger-group"},
		Conversation: channel.Conversation{
			ID:   "group-1",
			Type: "group",
		},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Decision == nil || !state.Decision.Stop {
		t.Fatal("unauthorized group message should be stopped")
	}
	if !state.Decision.Reply.IsEmpty() {
		t.Fatal("unauthorized group message should be silently dropped, not replied")
	}
}

func TestIdentityResolverPublicBotDirectDeniedWithReply(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-direct-denied"}}
	memberSvc := &fakeMemberService{isMember: false}
	policySvc := &fakePolicyService{allow: false, botType: "public"}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, memberSvc, policySvc, nil, nil, "Access denied.", "")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "hello"},
		ReplyTarget: "direct-target",
		Sender:      channel.Identity{SubjectID: "stranger-direct"},
		Conversation: channel.Conversation{
			ID:   "p2p-1",
			Type: "p2p",
		},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Decision == nil || !state.Decision.Stop {
		t.Fatal("unauthorized direct message should be stopped")
	}
	if state.Decision.Reply.IsEmpty() {
		t.Fatal("unauthorized direct message should reply with access denied")
	}
}

func TestIdentityResolverBindCodePlatformMismatch(t *testing.T) {
	channelIdentitySvc := &fakeChannelIdentityService{channelIdentity: identities.ChannelIdentity{ID: "channelIdentity-platform-mismatch"}}
	bindSvc := &fakeBindService{
		code: bind.Code{
			ID:             "code-platform",
			Platform:       "telegram",
			Token:          "BINDPLATFORM",
			IssuedByUserID: "channelIdentity-human-platform",
			ExpiresAt:      time.Now().UTC().Add(1 * time.Hour),
		},
	}
	resolver := NewIdentityResolver(slog.Default(), nil, channelIdentitySvc, &fakeMemberService{}, nil, nil, bindSvc, "", "")

	msg := channel.InboundMessage{
		BotID:       "bot-1",
		Channel:     channel.ChannelType("feishu"),
		Message:     channel.Message{Text: "BINDPLATFORM"},
		ReplyTarget: "target-id",
		Sender:      channel.Identity{SubjectID: "ext-bind-platform"},
	}
	state, err := resolver.Resolve(context.Background(), channel.ChannelConfig{BotID: "bot-1"}, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bindSvc.consumeCalled {
		t.Fatal("bind consume should not run when platform mismatches")
	}
	if state.Decision == nil || !state.Decision.Stop {
		t.Fatal("platform mismatch should return stop decision")
	}
}
