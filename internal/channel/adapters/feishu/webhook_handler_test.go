package feishu

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/channel"
)

const (
	testWebhookConfigID      = "550e8400-e29b-41d4-a716-446655440000"
	testOtherWebhookConfigID = "550e8400-e29b-41d4-a716-446655440001"
)

type fakeWebhookStore struct {
	config   channel.ChannelConfig
	getCalls []string
	getErr   error
}

func (s *fakeWebhookStore) GetConfigByID(_ context.Context, configID string) (channel.ChannelConfig, error) {
	s.getCalls = append(s.getCalls, configID)
	if s.getErr != nil {
		return channel.ChannelConfig{}, s.getErr
	}
	return s.config, nil
}

type fakeWebhookManager struct {
	calls []struct {
		cfg channel.ChannelConfig
		msg channel.InboundMessage
	}
	err error
}

func (m *fakeWebhookManager) HandleInbound(_ context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage) error {
	m.calls = append(m.calls, struct {
		cfg channel.ChannelConfig
		msg channel.InboundMessage
	}{cfg: cfg, msg: msg})
	return m.err
}

func TestWebhookHandler_URLVerification(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{
		config: channel.ChannelConfig{
			ID:          testWebhookConfigID,
			BotID:       "bot-1",
			ChannelType: Type,
			Credentials: map[string]any{
				"app_id":             "app",
				"app_secret":         "secret",
				"verification_token": "verify-token",
				"inbound_mode":       "webhook",
			},
		},
	}
	manager := &fakeWebhookManager{}
	h := NewWebhookHandler(nil, store, manager)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/channels/feishu/webhook/"+testWebhookConfigID, strings.NewReader(`{"schema":"2.0","header":{"event_type":"im.message.receive_v1","token":"verify-token"},"type":"url_verification","challenge":"hello"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("config_id")
	c.SetParamValues(testWebhookConfigID)

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
	req := httptest.NewRequest(http.MethodGet, "/channels/feishu/webhook/"+testWebhookConfigID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("config_id")
	c.SetParamValues(testWebhookConfigID)

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

func TestWebhookHandler_ConfigLookupRejectsNotFoundCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		configID  string
		store     *fakeWebhookStore
		wantCalls []string
	}{
		{
			name:      "invalid uuid",
			configID:  "not-a-uuid",
			store:     &fakeWebhookStore{},
			wantCalls: nil,
		},
		{
			name:     "other channel type",
			configID: testOtherWebhookConfigID,
			store: &fakeWebhookStore{
				config: channel.ChannelConfig{
					ID:          testOtherWebhookConfigID,
					BotID:       "bot-1",
					ChannelType: channel.ChannelType("telegram"),
				},
			},
			wantCalls: []string{testOtherWebhookConfigID},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := NewWebhookHandler(nil, tc.store, &fakeWebhookManager{})

			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/channels/feishu/webhook/"+tc.configID, strings.NewReader(`{}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("config_id")
			c.SetParamValues(tc.configID)

			err := h.Handle(c)
			if err == nil {
				t.Fatal("expected not found error")
			}
			he := &echo.HTTPError{}
			if !errors.As(err, &he) {
				t.Fatalf("expected HTTPError, got %T", err)
			}
			if he.Code != http.StatusNotFound {
				t.Fatalf("unexpected status code: %d", he.Code)
			}
			if len(tc.store.getCalls) != len(tc.wantCalls) {
				t.Fatalf("unexpected store lookup calls: %#v", tc.store.getCalls)
			}
			for i, want := range tc.wantCalls {
				if tc.store.getCalls[i] != want {
					t.Fatalf("unexpected store lookup calls: %#v", tc.store.getCalls)
				}
			}
		})
	}
}

func TestWebhookHandler_EventCallbackDispatchesInbound(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{
		config: channel.ChannelConfig{
			ID:          testWebhookConfigID,
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
	}
	manager := &fakeWebhookManager{}
	h := NewWebhookHandler(nil, store, manager)

	e := echo.New()
	body := `{"schema":"2.0","header":{"event_id":"evt_1","event_type":"im.message.receive_v1","token":"verify-token"},"event":{"sender":{"sender_id":{"open_id":"ou_user_1","user_id":"u_user_1"}},"message":{"message_id":"om_1","chat_id":"oc_1","chat_type":"p2p","message_type":"text","content":"{\"text\":\"hello\"}"}},"type":"event_callback"}`
	req := httptest.NewRequest(http.MethodPost, "/channels/feishu/webhook/"+testWebhookConfigID, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("config_id")
	c.SetParamValues(testWebhookConfigID)

	if err := h.Handle(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", rec.Code)
	}
	if len(manager.calls) != 1 {
		t.Fatalf("expected one inbound call, got %d", len(manager.calls))
	}
	if len(store.getCalls) != 1 || store.getCalls[0] != testWebhookConfigID {
		t.Fatalf("expected direct config lookup by id, got %#v", store.getCalls)
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
		config: channel.ChannelConfig{
			ID:               testWebhookConfigID,
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
	}
	manager := &fakeWebhookManager{}
	h := NewWebhookHandler(nil, store, manager)

	e := echo.New()
	body := `{"schema":"2.0","header":{"event_id":"evt_2","event_type":"im.message.receive_v1","token":"verify-token"},"event":{"sender":{"sender_id":{"open_id":"ou_user_2","user_id":"u_user_2"}},"message":{"message_id":"om_2","chat_id":"oc_group_1","chat_type":"group","message_type":"text","content":"{\"text\":\"<at user_id=\\\"ou_other_user\\\"></at> hello\"}"}},"type":"event_callback"}`
	req := httptest.NewRequest(http.MethodPost, "/channels/feishu/webhook/"+testWebhookConfigID, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("config_id")
	c.SetParamValues(testWebhookConfigID)

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

func TestWebhookHandler_EventCallbackRejectsInvalidAuth(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		creds      map[string]any
		body       string
		wantStatus int
	}{
		{
			name: "invalid verification token",
			creds: map[string]any{
				"app_id":             "app",
				"app_secret":         "secret",
				"verification_token": "verify-token",
				"inbound_mode":       "webhook",
			},
			body:       `{"schema":"2.0","header":{"event_id":"evt_1","event_type":"im.message.receive_v1","token":"forged-token"},"event":{"sender":{"sender_id":{"open_id":"ou_user_1"}},"message":{"message_id":"om_1","chat_id":"oc_1","chat_type":"p2p","message_type":"text","content":"{\"text\":\"hello\"}"}},"type":"event_callback"}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "missing verification token configuration",
			creds: map[string]any{
				"app_id":       "app",
				"app_secret":   "secret",
				"inbound_mode": "webhook",
			},
			body:       `{"schema":"2.0","header":{"event_id":"evt_1","event_type":"im.message.receive_v1","token":"verify-token"},"event":{"sender":{"sender_id":{"open_id":"ou_user_1"}},"message":{"message_id":"om_1","chat_id":"oc_1","chat_type":"p2p","message_type":"text","content":"{\"text\":\"hello\"}"}},"type":"event_callback"}`,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &fakeWebhookStore{
				config: channel.ChannelConfig{
					ID:          testWebhookConfigID,
					BotID:       "bot-1",
					ChannelType: Type,
					Credentials: tc.creds,
				},
			}
			manager := &fakeWebhookManager{}
			h := NewWebhookHandler(nil, store, manager)

			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/channels/feishu/webhook/"+testWebhookConfigID, strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("config_id")
			c.SetParamValues(testWebhookConfigID)

			err := h.Handle(c)
			if err == nil {
				t.Fatal("expected auth error")
			}
			he := &echo.HTTPError{}
			if !errors.As(err, &he) {
				t.Fatalf("expected HTTPError, got %T", err)
			}
			if he.Code != tc.wantStatus {
				t.Fatalf("unexpected status code: %d", he.Code)
			}
			if len(manager.calls) != 0 {
				t.Fatalf("expected no inbound calls, got %d", len(manager.calls))
			}
		})
	}
}

func TestWebhookHandler_RejectsOversizedBody(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{
		config: channel.ChannelConfig{
			ID:          testWebhookConfigID,
			BotID:       "bot-1",
			ChannelType: Type,
			Credentials: map[string]any{
				"app_id":             "app",
				"app_secret":         "secret",
				"verification_token": "verify-token",
				"inbound_mode":       "webhook",
			},
		},
	}
	manager := &fakeWebhookManager{}
	h := NewWebhookHandler(nil, store, manager)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/channels/feishu/webhook/"+testWebhookConfigID, strings.NewReader(strings.Repeat("x", int(webhookMaxBodyBytes)+1)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("config_id")
	c.SetParamValues(testWebhookConfigID)

	err := h.Handle(c)
	if err == nil {
		t.Fatal("expected payload-too-large error")
	}
	he := &echo.HTTPError{}
	ok := errors.As(err, &he)
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
