package channel

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeConfigStore struct {
	effectiveConfig        Config
	channelIdentityConfig  IdentityBinding
	configsByType          map[Type][]Config
	boundChannelIdentityID string
}

func (f *fakeConfigStore) ResolveEffectiveConfig(_ context.Context, _ string, _ Type) (Config, error) {
	return f.effectiveConfig, nil
}

func (f *fakeConfigStore) GetChannelIdentityConfig(_ context.Context, _ string, _ Type) (IdentityBinding, error) {
	if f.channelIdentityConfig.ID == "" && len(f.channelIdentityConfig.Config) == 0 {
		return IdentityBinding{}, errors.New("channel user config not found")
	}
	return f.channelIdentityConfig, nil
}

func (f *fakeConfigStore) UpsertChannelIdentityConfig(_ context.Context, _ string, _ Type, _ UpsertChannelIdentityConfigRequest) (IdentityBinding, error) {
	return f.channelIdentityConfig, nil
}

func (f *fakeConfigStore) ListConfigsByType(_ context.Context, channelType Type) ([]Config, error) {
	if f.configsByType == nil {
		return nil, nil
	}
	return f.configsByType[channelType], nil
}

func (f *fakeConfigStore) ResolveIdentityBinding(_ context.Context, _ Type, _ BindingCriteria) (string, error) {
	if f.boundChannelIdentityID == "" {
		return "", errors.New("channel user binding not found")
	}
	return f.boundChannelIdentityID, nil
}

type fakeInboundProcessorIntegration struct {
	resp   *OutboundMessage
	err    error
	gotCfg Config
	gotMsg InboundMessage
}

func (f *fakeInboundProcessorIntegration) HandleInbound(ctx context.Context, cfg Config, msg InboundMessage, sender StreamReplySender) error {
	f.gotCfg = cfg
	f.gotMsg = msg
	if f.err != nil {
		return f.err
	}
	if f.resp == nil {
		return nil
	}
	if sender == nil {
		return errors.New("sender missing")
	}
	return sender.Send(ctx, *f.resp)
}

type fakeAdapter struct {
	channelType Type
	mu          sync.Mutex
	started     []Config
	sent        []OutboundMessage
	stops       int
}

func (f *fakeAdapter) Type() Type {
	return f.channelType
}

func (f *fakeAdapter) Descriptor() Descriptor {
	return Descriptor{Type: f.channelType, DisplayName: "Fake", Capabilities: Capabilities{Text: true}}
}

func (f *fakeAdapter) ResolveTarget(channelIdentityConfig map[string]any) (string, error) {
	value := strings.TrimSpace(ReadString(channelIdentityConfig, "target"))
	if value == "" {
		return "", errors.New("missing target")
	}
	return "resolved:" + value, nil
}

func (f *fakeAdapter) NormalizeTarget(raw string) string { return strings.TrimSpace(raw) }

func (f *fakeAdapter) Connect(_ context.Context, cfg Config, _ InboundHandler) (Connection, error) {
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

func (f *fakeAdapter) Send(_ context.Context, _ Config, msg OutboundMessage) error {
	f.mu.Lock()
	f.sent = append(f.sent, msg)
	f.mu.Unlock()
	return nil
}

func TestManagerHandleInboundIntegratesAdapter(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)
	store := &fakeConfigStore{}
	processor := &fakeInboundProcessorIntegration{
		resp: &OutboundMessage{
			Target: "123",
			Message: Message{
				Text: "ok",
			},
		},
	}
	reg := NewRegistry()
	adapter := &fakeAdapter{channelType: Type("test")}
	manager := NewManager(log, reg, store, processor)
	manager.RegisterAdapter(adapter)

	cfg := Config{
		ID:          "cfg-1",
		BotID:       "bot-1",
		Type:        Type("test"),
		Credentials: map[string]any{"botToken": "token"},
		UpdatedAt:   time.Now(),
	}
	err := manager.handleInbound(context.Background(), cfg, InboundMessage{
		Channel:     Type("test"),
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

	log := slog.New(slog.DiscardHandler)
	store := &fakeConfigStore{
		effectiveConfig: Config{
			ID:          "cfg-1",
			BotID:       "bot-1",
			Type:        Type("test"),
			Credentials: map[string]any{"botToken": "token"},
			UpdatedAt:   time.Now(),
		},
		channelIdentityConfig: IdentityBinding{
			ID:     "binding-1",
			Config: map[string]any{"target": "alice"},
		},
	}
	reg := NewRegistry()
	adapter := &fakeAdapter{channelType: Type("test")}
	manager := NewManager(log, reg, store, &fakeInboundProcessorIntegration{})
	manager.RegisterAdapter(adapter)

	err := manager.Send(context.Background(), "bot-1", Type("test"), SendRequest{
		ChannelIdentityID: "user-1",
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

	log := slog.New(slog.DiscardHandler)
	store := &fakeConfigStore{}
	reg := NewRegistry()
	adapter := &fakeAdapter{channelType: Type("test")}
	manager := NewManager(log, reg, store, &fakeInboundProcessorIntegration{})
	manager.RegisterAdapter(adapter)

	cfg := Config{
		ID:          "cfg-1",
		BotID:       "bot-1",
		Type:        Type("test"),
		Credentials: map[string]any{"botToken": "token"},
		UpdatedAt:   time.Now(),
	}
	manager.reconcile(context.Background(), []Config{cfg})
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
