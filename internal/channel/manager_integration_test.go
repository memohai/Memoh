package channel

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func init() {
	_ = RegisterChannel(ChannelDescriptor{
		Type:                ChannelType("test"),
		DisplayName:         "Test",
		NormalizeConfig:     normalizeEmpty,
		NormalizeUserConfig: normalizeEmpty,
		ResolveTarget:       resolveTestTarget,
		Capabilities: ChannelCapabilities{
			Text: true,
		},
	})
}

func normalizeEmpty(map[string]any) (map[string]any, error) {
	return map[string]any{}, nil
}

func resolveTestTarget(config map[string]any) (string, error) {
	value := strings.TrimSpace(ReadString(config, "target"))
	if value == "" {
		return "", fmt.Errorf("missing target")
	}
	return "resolved:" + value, nil
}

type fakeConfigStore struct {
	effectiveConfig ChannelConfig
	userConfig      ChannelUserBinding
	configsByType   map[ChannelType][]ChannelConfig
	session         ChannelSession
	boundUserID     string
}

func (f *fakeConfigStore) ResolveEffectiveConfig(ctx context.Context, botID string, channelType ChannelType) (ChannelConfig, error) {
	return f.effectiveConfig, nil
}

func (f *fakeConfigStore) GetUserConfig(ctx context.Context, actorUserID string, channelType ChannelType) (ChannelUserBinding, error) {
	if f.userConfig.ID == "" && len(f.userConfig.Config) == 0 {
		return ChannelUserBinding{}, fmt.Errorf("channel user config not found")
	}
	return f.userConfig, nil
}

func (f *fakeConfigStore) UpsertUserConfig(ctx context.Context, actorUserID string, channelType ChannelType, req UpsertUserConfigRequest) (ChannelUserBinding, error) {
	return f.userConfig, nil
}

func (f *fakeConfigStore) ListConfigsByType(ctx context.Context, channelType ChannelType) ([]ChannelConfig, error) {
	if f.configsByType == nil {
		return nil, nil
	}
	return f.configsByType[channelType], nil
}

func (f *fakeConfigStore) ResolveUserBinding(ctx context.Context, channelType ChannelType, criteria BindingCriteria) (string, error) {
	if f.boundUserID == "" {
		return "", fmt.Errorf("channel user binding not found")
	}
	return f.boundUserID, nil
}

func (f *fakeConfigStore) ListSessionsByBotPlatform(ctx context.Context, botID, platform string) ([]ChannelSession, error) {
	return nil, nil
}

func (f *fakeConfigStore) GetChannelSession(ctx context.Context, sessionID string) (ChannelSession, error) {
	if f.session.SessionID == sessionID {
		return f.session, nil
	}
	return ChannelSession{}, nil
}

func (f *fakeConfigStore) UpsertChannelSession(ctx context.Context, sessionID string, botID string, channelConfigID string, userID string, contactID string, platform string, replyTarget string, threadID string, metadata map[string]any) error {
	return nil
}

type fakeInboundProcessorIntegration struct {
	resp   *OutboundMessage
	err    error
	gotCfg ChannelConfig
	gotMsg InboundMessage
}

func (f *fakeInboundProcessorIntegration) HandleInbound(ctx context.Context, cfg ChannelConfig, msg InboundMessage, sender ReplySender) error {
	f.gotCfg = cfg
	f.gotMsg = msg
	if f.err != nil {
		return f.err
	}
	if f.resp == nil {
		return nil
	}
	if sender == nil {
		return fmt.Errorf("sender missing")
	}
	return sender.Send(ctx, *f.resp)
}

type fakeAdapter struct {
	channelType ChannelType
	mu          sync.Mutex
	started     []ChannelConfig
	sent        []OutboundMessage
	stops       int
}

func (f *fakeAdapter) Type() ChannelType {
	return f.channelType
}

func (f *fakeAdapter) Connect(ctx context.Context, cfg ChannelConfig, handler InboundHandler) (Connection, error) {
	f.mu.Lock()
	f.started = append(f.started, cfg)
	f.mu.Unlock()
	stop := func(context.Context) error {
		f.mu.Lock()
		f.stops++
		f.mu.Unlock()
		return nil
	}
	return NewConnection(cfg, stop), nil
}

func (f *fakeAdapter) Send(ctx context.Context, cfg ChannelConfig, msg OutboundMessage) error {
	f.mu.Lock()
	f.sent = append(f.sent, msg)
	f.mu.Unlock()
	return nil
}

func TestManagerHandleInboundIntegratesAdapter(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	store := &fakeConfigStore{
		session: ChannelSession{
			SessionID: "telegram:bot-1:chat-1",
			BotID:     "bot-1",
			UserID:    "user-1",
		},
	}
	processor := &fakeInboundProcessorIntegration{
		resp: &OutboundMessage{
			Target: "123",
			Message: Message{
				Text: "ok",
			},
		},
	}
	adapter := &fakeAdapter{channelType: ChannelType("test")}
	manager := NewManager(log, store, processor)
	manager.RegisterAdapter(adapter)

	cfg := ChannelConfig{
		ID:          "cfg-1",
		BotID:       "bot-1",
		ChannelType: ChannelType("test"),
		Credentials: map[string]any{"botToken": "token"},
		UpdatedAt:   time.Now(),
	}
	err := manager.handleInbound(context.Background(), cfg, InboundMessage{
		Channel:     ChannelType("test"),
		Message:     Message{Text: "hi"},
		BotID:       "bot-1",
		ReplyTarget: "123",
		Conversation: Conversation{
			ID:   "chat-1",
			Type: "p2p",
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if processor.gotMsg.Conversation.ID != "chat-1" || processor.gotMsg.Message.PlainText() != "hi" || processor.gotMsg.BotID != "bot-1" {
		t.Fatalf("unexpected inbound message: %+v", processor.gotMsg)
	}

	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.sent) != 1 {
		t.Fatalf("expected 1 send, got %d", len(adapter.sent))
	}
	if adapter.sent[0].Target != "123" || adapter.sent[0].Message.PlainText() != "ok" {
		t.Fatalf("unexpected outbound message: %+v", adapter.sent[0])
	}
}

func TestManagerSendUsesBinding(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	store := &fakeConfigStore{
		effectiveConfig: ChannelConfig{
			ID:          "cfg-1",
			BotID:       "bot-1",
			ChannelType: ChannelType("test"),
			Credentials: map[string]any{"botToken": "token"},
			UpdatedAt:   time.Now(),
		},
		userConfig: ChannelUserBinding{
			ID:     "binding-1",
			Config: map[string]any{"target": "alice"},
		},
	}
	adapter := &fakeAdapter{channelType: ChannelType("test")}
	manager := NewManager(log, store, &fakeInboundProcessorIntegration{})
	manager.RegisterAdapter(adapter)

	err := manager.Send(context.Background(), "bot-1", ChannelType("test"), SendRequest{
		UserID: "user-1",
		Message: Message{
			Text: "hello",
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.sent) != 1 {
		t.Fatalf("expected 1 send, got %d", len(adapter.sent))
	}
	if adapter.sent[0].Target != "resolved:alice" || adapter.sent[0].Message.PlainText() != "hello" {
		t.Fatalf("unexpected outbound message: %+v", adapter.sent[0])
	}
}

func TestManagerReconcileStartsAndStops(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	store := &fakeConfigStore{}
	adapter := &fakeAdapter{channelType: ChannelType("test")}
	manager := NewManager(log, store, &fakeInboundProcessorIntegration{})
	manager.RegisterAdapter(adapter)

	cfg := ChannelConfig{
		ID:          "cfg-1",
		BotID:       "bot-1",
		ChannelType: ChannelType("test"),
		Credentials: map[string]any{"botToken": "token"},
		UpdatedAt:   time.Now(),
	}
	manager.reconcile(context.Background(), []ChannelConfig{cfg})
	manager.reconcile(context.Background(), nil)

	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.started) != 1 {
		t.Fatalf("expected 1 start, got %d", len(adapter.started))
	}
	if adapter.stops != 1 {
		t.Fatalf("expected 1 stop, got %d", adapter.stops)
	}
}
