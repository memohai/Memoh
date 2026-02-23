package feishu

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/channel"
)

type fakeWebhookStore struct {
	configs []channel.ChannelConfig
	err     error
}

func (s *fakeWebhookStore) ListConfigsByType(ctx context.Context, channelType channel.ChannelType) ([]channel.ChannelConfig, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.configs, nil
}

type fakeWebhookManager struct {
	calls []struct {
		cfg channel.ChannelConfig
		msg channel.InboundMessage
	}
	err error
}

func (m *fakeWebhookManager) HandleInbound(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage) error {
	m.calls = append(m.calls, struct {
		cfg channel.ChannelConfig
		msg channel.InboundMessage
	}{cfg: cfg, msg: msg})
	return m.err
}

func TestWebhookHandler_URLVerification(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{
		configs: []channel.ChannelConfig{
			{
				ID:          "cfg-1",
				BotID:       "bot-1",
				ChannelType: Type,
				Credentials: map[string]any{
					"app_id":             "app",
					"app_secret":         "secret",
					"verification_token": "verify-token",
					"inbound_mode":       "webhook",
				},
			},
		},
	}
	manager := &fakeWebhookManager{}
	h := NewWebhookHandler(nil, store, manager)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/channels/feishu/webhook/cfg-1", strings.NewReader(`{"schema":"2.0","header":{"event_type":"im.message.receive_v1","token":"verify-token"},"type":"url_verification","challenge":"hello"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("config_id")
	c.SetParamValues("cfg-1")

	if err := h.Handle(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"challenge":"hello"`) {
		t.Fatalf("unexpected challenge response: %s", rec.Body.String())
	}
	if len(manager.calls) != 0 {
		t.Fatalf("expected no inbound calls, got %d", len(manager.calls))
	}
}

func TestWebhookHandler_Probe(t *testing.T) {
	t.Parallel()

	h := NewWebhookHandler(nil, &fakeWebhookStore{}, &fakeWebhookManager{})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/channels/feishu/webhook/cfg-1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("config_id")
	c.SetParamValues("cfg-1")

	if err := h.HandleProbe(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Fatalf("unexpected probe response: %q", rec.Body.String())
	}
}

func TestWebhookHandler_EventCallbackDispatchesInbound(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{
		configs: []channel.ChannelConfig{
			{
				ID:          "cfg-1",
				BotID:       "bot-1",
				ChannelType: Type,
				SelfIdentity: map[string]any{
					"open_id": "ou_bot_1",
				},
				Credentials: map[string]any{
					"app_id":             "app",
					"app_secret":         "secret",
					"verification_token": "verify-token",
					"inbound_mode":       "webhook",
				},
			},
		},
	}
	manager := &fakeWebhookManager{}
	h := NewWebhookHandler(nil, store, manager)

	e := echo.New()
	body := `{"schema":"2.0","header":{"event_id":"evt_1","event_type":"im.message.receive_v1","token":"verify-token"},"event":{"sender":{"sender_id":{"open_id":"ou_user_1","user_id":"u_user_1"}},"message":{"message_id":"om_1","chat_id":"oc_1","chat_type":"p2p","message_type":"text","content":"{\"text\":\"hello\"}"}},"type":"event_callback"}`
	req := httptest.NewRequest(http.MethodPost, "/channels/feishu/webhook/cfg-1", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("config_id")
	c.SetParamValues("cfg-1")

	if err := h.Handle(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", rec.Code)
	}
	if len(manager.calls) != 1 {
		t.Fatalf("expected one inbound call, got %d", len(manager.calls))
	}
	got := manager.calls[0].msg
	if got.BotID != "bot-1" {
		t.Fatalf("unexpected bot id: %s", got.BotID)
	}
	if got.Message.PlainText() != "hello" {
		t.Fatalf("unexpected message text: %q", got.Message.PlainText())
	}
}

func TestWebhookHandler_EventCallbackUsesExternalIdentityForMentionFilter(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{
		configs: []channel.ChannelConfig{
			{
				ID:               "cfg-1",
				BotID:            "bot-1",
				ChannelType:      Type,
				ExternalIdentity: "open_id:ou_bot_1",
				Credentials: map[string]any{
					"app_id":             "app",
					"app_secret":         "secret",
					"verification_token": "verify-token",
					"inbound_mode":       "webhook",
				},
			},
		},
	}
	manager := &fakeWebhookManager{}
	h := NewWebhookHandler(nil, store, manager)

	e := echo.New()
	body := `{"schema":"2.0","header":{"event_id":"evt_2","event_type":"im.message.receive_v1","token":"verify-token"},"event":{"sender":{"sender_id":{"open_id":"ou_user_2","user_id":"u_user_2"}},"message":{"message_id":"om_2","chat_id":"oc_group_1","chat_type":"group","message_type":"text","content":"{\"text\":\"<at user_id=\\\"ou_other_user\\\"></at> hello\"}"}},"type":"event_callback"}`
	req := httptest.NewRequest(http.MethodPost, "/channels/feishu/webhook/cfg-1", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("config_id")
	c.SetParamValues("cfg-1")

	if err := h.Handle(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", rec.Code)
	}
	if len(manager.calls) != 1 {
		t.Fatalf("expected one inbound call, got %d", len(manager.calls))
	}
	mentioned, _ := manager.calls[0].msg.Metadata["is_mentioned"].(bool)
	if mentioned {
		t.Fatalf("expected mention flag=false when mentioning another user")
	}
}

func TestWebhookHandler_EventCallbackRejectsInvalidTokenWhenEncryptKeyMissing(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{
		configs: []channel.ChannelConfig{
			{
				ID:          "cfg-1",
				BotID:       "bot-1",
				ChannelType: Type,
				Credentials: map[string]any{
					"app_id":             "app",
					"app_secret":         "secret",
					"verification_token": "verify-token",
					"inbound_mode":       "webhook",
				},
			},
		},
	}
	manager := &fakeWebhookManager{}
	h := NewWebhookHandler(nil, store, manager)

	e := echo.New()
	body := `{"schema":"2.0","header":{"event_id":"evt_1","event_type":"im.message.receive_v1","token":"forged-token"},"event":{"sender":{"sender_id":{"open_id":"ou_user_1"}},"message":{"message_id":"om_1","chat_id":"oc_1","chat_type":"p2p","message_type":"text","content":"{\"text\":\"hello\"}"}},"type":"event_callback"}`
	req := httptest.NewRequest(http.MethodPost, "/channels/feishu/webhook/cfg-1", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("config_id")
	c.SetParamValues("cfg-1")

	err := h.Handle(c)
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if he.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status code: %d", he.Code)
	}
	if len(manager.calls) != 0 {
		t.Fatalf("expected no inbound calls, got %d", len(manager.calls))
	}
}

func TestWebhookHandler_EventCallbackRequiresVerificationTokenWhenEncryptKeyMissing(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{
		configs: []channel.ChannelConfig{
			{
				ID:          "cfg-1",
				BotID:       "bot-1",
				ChannelType: Type,
				Credentials: map[string]any{
					"app_id":       "app",
					"app_secret":   "secret",
					"inbound_mode": "webhook",
				},
			},
		},
	}
	manager := &fakeWebhookManager{}
	h := NewWebhookHandler(nil, store, manager)

	e := echo.New()
	body := `{"schema":"2.0","header":{"event_id":"evt_1","event_type":"im.message.receive_v1","token":"verify-token"},"event":{"sender":{"sender_id":{"open_id":"ou_user_1"}},"message":{"message_id":"om_1","chat_id":"oc_1","chat_type":"p2p","message_type":"text","content":"{\"text\":\"hello\"}"}},"type":"event_callback"}`
	req := httptest.NewRequest(http.MethodPost, "/channels/feishu/webhook/cfg-1", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("config_id")
	c.SetParamValues("cfg-1")

	err := h.Handle(c)
	if err == nil {
		t.Fatal("expected forbidden error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if he.Code != http.StatusForbidden {
		t.Fatalf("unexpected status code: %d", he.Code)
	}
	if len(manager.calls) != 0 {
		t.Fatalf("expected no inbound calls, got %d", len(manager.calls))
	}
}

func TestWebhookHandler_RejectsOversizedBody(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{
		configs: []channel.ChannelConfig{
			{
				ID:          "cfg-1",
				BotID:       "bot-1",
				ChannelType: Type,
				Credentials: map[string]any{
					"app_id":             "app",
					"app_secret":         "secret",
					"verification_token": "verify-token",
					"inbound_mode":       "webhook",
				},
			},
		},
	}
	manager := &fakeWebhookManager{}
	h := NewWebhookHandler(nil, store, manager)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/channels/feishu/webhook/cfg-1", strings.NewReader(strings.Repeat("x", int(webhookMaxBodyBytes)+1)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("config_id")
	c.SetParamValues("cfg-1")

	err := h.Handle(c)
	if err == nil {
		t.Fatal("expected payload-too-large error")
	}
	he, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if he.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("unexpected status code: %d", he.Code)
	}
	if len(manager.calls) != 0 {
		t.Fatalf("expected no inbound calls, got %d", len(manager.calls))
	}
}
