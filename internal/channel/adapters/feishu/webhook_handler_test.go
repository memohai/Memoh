package feishu

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/memohai/memoh/internal/channel"
)

const testWebhookConfigID = "cfg-1"

type fakeWebhookStore struct {
	configs []channel.ChannelConfig
	err     error
}

func (s *fakeWebhookStore) ListConfigsByType(_ context.Context, _ channel.ChannelType) ([]channel.ChannelConfig, error) {
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

func (m *fakeWebhookManager) HandleInbound(_ context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage) error {
	m.calls = append(m.calls, struct {
		cfg channel.ChannelConfig
		msg channel.InboundMessage
	}{cfg: cfg, msg: msg})
	return m.err
}

func TestWebhookHandler_URLVerification(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		credentials map[string]any
		body        string
		wantStatus  int
		wantError   bool
	}{
		{
			name: "with verification token",
			credentials: map[string]any{
				"app_id":             "app",
				"app_secret":         "secret",
				"verification_token": "verify-token",
				"inbound_mode":       "webhook",
			},
			body:       `{"schema":"2.0","header":{"event_type":"im.message.receive_v1","token":"verify-token"},"type":"url_verification","challenge":"hello"}`,
			wantStatus: http.StatusOK,
		},
		{
			name: "without webhook secrets",
			credentials: map[string]any{
				"app_id":       "app",
				"app_secret":   "secret",
				"inbound_mode": "webhook",
			},
			body:       `{"type":"url_verification","challenge":"hello"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &fakeWebhookStore{
				configs: []channel.ChannelConfig{
					{
						ID:          testWebhookConfigID,
						BotID:       "bot-1",
						ChannelType: Type,
						Credentials: tc.credentials,
					},
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
			if tc.wantError {
				if err == nil {
					t.Fatal("expected error")
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
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.Code != tc.wantStatus {
				t.Fatalf("unexpected status code: %d", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), `"challenge":"hello"`) {
				t.Fatalf("unexpected challenge response: %s", rec.Body.String())
			}
			if len(manager.calls) != 0 {
				t.Fatalf("expected no inbound calls, got %d", len(manager.calls))
			}
		})
	}
}

func TestWebhookHandler_URLVerificationWithEncryptKeyWithoutVerificationToken(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{
		configs: []channel.ChannelConfig{
			{
				ID:          testWebhookConfigID,
				BotID:       "bot-1",
				ChannelType: Type,
				Credentials: map[string]any{
					"app_id":       "app",
					"app_secret":   "secret",
					"encrypt_key":  "encrypt-key",
					"inbound_mode": "webhook",
				},
			},
		},
	}
	manager := &fakeWebhookManager{}
	h := NewWebhookHandler(nil, store, manager)

	encrypt, err := larkcore.EncryptedEventMsg(context.Background(), `{"challenge":"hello","token":"verify-token","type":"url_verification"}`, "encrypt-key")
	if err != nil {
		t.Fatalf("failed to encrypt challenge payload: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/channels/feishu/webhook/"+testWebhookConfigID, strings.NewReader(`{"encrypt":"`+encrypt+`"}`))
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

func TestWebhookHandler_ConfigLookupRejectsNotFound(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{}
	h := NewWebhookHandler(nil, store, &fakeWebhookManager{})

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/channels/feishu/webhook/not-found", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("config_id")
	c.SetParamValues("not-found")

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
}

func TestWebhookHandler_EventCallbackDispatchesInbound(t *testing.T) {
	t.Parallel()

	store := &fakeWebhookStore{
		configs: []channel.ChannelConfig{
			{
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

func TestWebhookHandler_EventCallbackRejectsInvalidTokenWhenEncryptKeyMissing(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		credentials map[string]any
		body        string
	}{
		{
			name: "plaintext callback",
			credentials: map[string]any{
				"app_id":             "app",
				"app_secret":         "secret",
				"verification_token": "verify-token",
				"inbound_mode":       "webhook",
			},
			body: `{"schema":"2.0","header":{"event_id":"evt_1","event_type":"im.message.receive_v1","token":"forged-token"},"event":{"sender":{"sender_id":{"open_id":"ou_user_1"}},"message":{"message_id":"om_1","chat_id":"oc_1","chat_type":"p2p","message_type":"text","content":"{\"text\":\"hello\"}"}},"type":"event_callback"}`,
		},
		{
			name: "encrypted callback",
			credentials: map[string]any{
				"app_id":             "app",
				"app_secret":         "secret",
				"encrypt_key":        "encrypt-key",
				"verification_token": "verify-token",
				"inbound_mode":       "webhook",
			},
			body: func() string {
				encrypt, err := larkcore.EncryptedEventMsg(context.Background(), `{"schema":"2.0","header":{"event_id":"evt_1","event_type":"im.message.receive_v1","token":"forged-token"},"event":{"sender":{"sender_id":{"open_id":"ou_user_1"}},"message":{"message_id":"om_1","chat_id":"oc_1","chat_type":"p2p","message_type":"text","content":"{\"text\":\"hello\"}"}},"type":"event_callback"}`, "encrypt-key")
				if err != nil {
					t.Fatalf("failed to encrypt event payload: %v", err)
				}
				return `{"encrypt":"` + encrypt + `"}`
			}(),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := &fakeWebhookStore{
				configs: []channel.ChannelConfig{
					{
						ID:          testWebhookConfigID,
						BotID:       "bot-1",
						ChannelType: Type,
						Credentials: tc.credentials,
					},
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
				t.Fatal("expected unauthorized error")
			}
			he := &echo.HTTPError{}
			if !errors.As(err, &he) {
				t.Fatalf("expected HTTPError, got %T", err)
			}
			if he.Code != http.StatusUnauthorized {
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
		configs: []channel.ChannelConfig{
			{
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
