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
func (*WebhookHandler) HandleProbe(c echo.Context) error {
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

	botOpenID := h.adapter.resolveBotOpenID(context.WithoutCancel(c.Request().Context()), cfg)

	reqCtx := c.Request().Context()
	eventDispatcher := dispatcher.NewEventDispatcher(feishuCfg.VerificationToken, feishuCfg.EncryptKey)
	webhookReq, err := inspectWebhookRequest(reqCtx, eventDispatcher, c.Request(), payload)
	if err != nil {
		return err
	}
	if err := validateWebhookCallbackAuth(webhookReq, feishuCfg); err != nil {
		return err
	}
	if challengeResp := buildWebhookChallengeResponse(webhookReq); challengeResp != nil {
		return writeEventResponse(c, challengeResp)
	}
	eventDispatcher.OnP2MessageReceiveV1(func(_ context.Context, event *larkim.P2MessageReceiveV1) error {
		msg := extractFeishuInbound(event, botOpenID, h.adapter.logger)
		if strings.TrimSpace(msg.Message.PlainText()) == "" && len(msg.Message.Attachments) == 0 {
			return nil
		}
		h.adapter.enrichSenderProfile(reqCtx, cfg, event, &msg)
		h.adapter.enrichQuotedMessage(reqCtx, cfg, &msg, botOpenID)
		msg.BotID = cfg.BotID
		return h.manager.HandleInbound(reqCtx, cfg, msg)
	})

	resp := eventDispatcher.Handle(c.Request().Context(), &larkevent.EventReq{
		Header:     c.Request().Header,
		Body:       payload,
		RequestURI: c.Request().RequestURI,
	})
	if resp == nil {
		return c.NoContent(http.StatusOK)
	}
	return writeEventResponse(c, resp)
}

func inspectWebhookRequest(ctx context.Context, eventDispatcher *dispatcher.EventDispatcher, req *http.Request, payload []byte) (larkevent.EventFuzzy, error) {
	plainPayload, err := parseWebhookPayload(ctx, eventDispatcher, req, payload)
	if err != nil {
		return larkevent.EventFuzzy{}, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid feishu webhook payload: %v", err))
	}

	var fuzzy larkevent.EventFuzzy
	if err := json.Unmarshal([]byte(plainPayload), &fuzzy); err != nil {
		return larkevent.EventFuzzy{}, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("invalid feishu webhook payload: %v", err))
	}
	return fuzzy, nil
}

func validateWebhookCallbackAuth(fuzzy larkevent.EventFuzzy, cfg Config) error {
	expectedToken := strings.TrimSpace(cfg.VerificationToken)
	encryptKey := strings.TrimSpace(cfg.EncryptKey)
	if expectedToken == "" && encryptKey == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "feishu webhook requires encrypt_key or verification_token")
	}

	requestToken := webhookRequestToken(fuzzy)
	if expectedToken == "" {
		return nil
	}
	if requestToken == "" || requestToken != expectedToken {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid feishu webhook token")
	}
	return nil
}

func buildWebhookChallengeResponse(fuzzy larkevent.EventFuzzy) *larkevent.EventResp {
	if webhookRequestType(fuzzy) != larkevent.ReqTypeChallenge {
		return nil
	}
	return &larkevent.EventResp{
		Header:     http.Header{larkevent.ContentTypeHeader: []string{larkevent.DefaultContentType}},
		Body:       []byte(fmt.Sprintf(larkevent.ChallengeResponseFormat, fuzzy.Challenge)),
		StatusCode: http.StatusOK,
	}
}

func webhookRequestToken(fuzzy larkevent.EventFuzzy) string {
	requestToken := strings.TrimSpace(fuzzy.Token)
	if fuzzy.Header != nil && strings.TrimSpace(fuzzy.Header.Token) != "" {
		requestToken = strings.TrimSpace(fuzzy.Header.Token)
	}
	return requestToken
}

func webhookRequestType(fuzzy larkevent.EventFuzzy) larkevent.ReqType {
	return larkevent.ReqType(strings.TrimSpace(fuzzy.Type))
}

func parseWebhookPayload(ctx context.Context, eventDispatcher *dispatcher.EventDispatcher, req *http.Request, payload []byte) (string, error) {
	cipherPayload, err := eventDispatcher.ParseReq(ctx, &larkevent.EventReq{
		Header:     req.Header,
		Body:       payload,
		RequestURI: req.RequestURI,
	})
	if err != nil {
		return "", err
	}
	return eventDispatcher.DecryptEvent(ctx, cipherPayload)
}

func writeEventResponse(c echo.Context, resp *larkevent.EventResp) error {
	for key, values := range resp.Header {
		for _, value := range values {
			c.Response().Header().Add(key, value)
		}
	}
	c.Response().WriteHeader(resp.StatusCode)
	if len(resp.Body) == 0 {
		return nil
	}
	_, err := c.Response().Write(resp.Body)
	return err
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
