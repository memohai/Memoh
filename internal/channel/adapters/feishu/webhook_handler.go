package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/memohai/memoh/internal/channel"
)

type webhookConfigStore interface {
	ListConfigsByType(ctx context.Context, channelType channel.ChannelType) ([]channel.ChannelConfig, error)
}

type webhookInboundManager interface {
	HandleInbound(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage) error
}

const webhookMaxBodyBytes int64 = 1 << 20 // 1 MiB

// WebhookHandler receives Feishu/Lark event-subscription callbacks.
type WebhookHandler struct {
	logger  *slog.Logger
	store   webhookConfigStore
	manager webhookInboundManager
	adapter *FeishuAdapter
}

// NewWebhookHandler creates a public webhook handler for Feishu/Lark callbacks.
func NewWebhookHandler(log *slog.Logger, store webhookConfigStore, manager webhookInboundManager) *WebhookHandler {
	if log == nil {
		log = slog.Default()
	}
	return &WebhookHandler{
		logger:  log.With(slog.String("handler", "feishu_webhook")),
		store:   store,
		manager: manager,
		adapter: NewFeishuAdapter(log),
	}
}

// NewWebhookServerHandler is a DI-friendly constructor for fx/dig, using concrete
// channel types as parameters.
func NewWebhookServerHandler(log *slog.Logger, store *channel.Store, manager *channel.Manager) *WebhookHandler {
	return NewWebhookHandler(log, store, manager)
}

// Register registers webhook callback routes.
func (h *WebhookHandler) Register(e *echo.Echo) {
	e.GET("/channels/feishu/webhook/:config_id", h.HandleProbe)
	e.POST("/channels/feishu/webhook/:config_id", h.Handle)
}

// HandleProbe responds to health/probe requests on the webhook URL.
func (h *WebhookHandler) HandleProbe(c echo.Context) error {
	return c.String(http.StatusOK, "ok")
}

// Handle processes Feishu/Lark event-subscription webhook requests.
func (h *WebhookHandler) Handle(c echo.Context) error {
	if h.store == nil || h.manager == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "feishu webhook dependencies not configured")
	}
	configID := strings.TrimSpace(c.Param("config_id"))
	if configID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "config id is required")
	}
	cfg, err := h.findConfigByID(c.Request().Context(), configID)
	if err != nil {
		return err
	}
	if cfg.Disabled {
		return echo.NewHTTPError(http.StatusForbidden, "channel config is disabled")
	}
	feishuCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if feishuCfg.InboundMode != inboundModeWebhook {
		return echo.NewHTTPError(http.StatusBadRequest, "feishu inbound_mode is not webhook")
	}

	payload, err := io.ReadAll(io.LimitReader(c.Request().Body, webhookMaxBodyBytes+1))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("read body: %v", err))
	}
	if int64(len(payload)) > webhookMaxBodyBytes {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, fmt.Sprintf("payload too large: max %d bytes", webhookMaxBodyBytes))
	}
	if err := validateWebhookCallbackAuth(payload, feishuCfg); err != nil {
		return err
	}

	botOpenID := h.adapter.resolveBotOpenID(context.WithoutCancel(c.Request().Context()), cfg)

	eventDispatcher := dispatcher.NewEventDispatcher(feishuCfg.VerificationToken, feishuCfg.EncryptKey)
	eventDispatcher.OnP2MessageReceiveV1(func(_ context.Context, event *larkim.P2MessageReceiveV1) error {
		msg := extractFeishuInbound(event, botOpenID)
		if strings.TrimSpace(msg.Message.PlainText()) == "" && len(msg.Message.Attachments) == 0 {
			return nil
		}
		h.adapter.enrichSenderProfile(context.WithoutCancel(c.Request().Context()), cfg, event, &msg)
		msg.BotID = cfg.BotID
		return h.manager.HandleInbound(context.WithoutCancel(c.Request().Context()), cfg, msg)
	})

	resp := eventDispatcher.Handle(c.Request().Context(), &larkevent.EventReq{
		Header:     c.Request().Header,
		Body:       payload,
		RequestURI: c.Request().RequestURI,
	})
	if resp == nil {
		return c.NoContent(http.StatusOK)
	}
	for key, values := range resp.Header {
		for _, value := range values {
			c.Response().Header().Add(key, value)
		}
	}
	c.Response().WriteHeader(resp.StatusCode)
	if len(resp.Body) == 0 {
		return nil
	}
	_, err = c.Response().Write(resp.Body)
	return err
}

func validateWebhookCallbackAuth(payload []byte, cfg Config) error {
	if strings.TrimSpace(cfg.EncryptKey) != "" {
		// Lark SDK signature verification is enabled only when encryptKey is configured.
		return nil
	}
	var fuzzy larkevent.EventFuzzy
	if err := json.Unmarshal(payload, &fuzzy); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid feishu webhook payload: %v", err))
	}
	if larkevent.ReqType(strings.TrimSpace(fuzzy.Type)) == larkevent.ReqTypeChallenge {
		return nil
	}
	expectedToken := strings.TrimSpace(cfg.VerificationToken)
	if expectedToken == "" {
		return echo.NewHTTPError(http.StatusForbidden, "feishu webhook requires verification_token when encrypt_key is empty")
	}
	requestToken := strings.TrimSpace(fuzzy.Token)
	if fuzzy.Header != nil && strings.TrimSpace(fuzzy.Header.Token) != "" {
		requestToken = strings.TrimSpace(fuzzy.Header.Token)
	}
	if requestToken == "" || requestToken != expectedToken {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid feishu webhook token")
	}
	return nil
}

func (h *WebhookHandler) findConfigByID(ctx context.Context, configID string) (channel.ChannelConfig, error) {
	items, err := h.store.ListConfigsByType(ctx, Type)
	if err != nil {
		return channel.ChannelConfig{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	for _, item := range items {
		if strings.TrimSpace(item.ID) == configID {
			return item, nil
		}
	}
	return channel.ChannelConfig{}, echo.NewHTTPError(http.StatusNotFound, "channel config not found")
}
