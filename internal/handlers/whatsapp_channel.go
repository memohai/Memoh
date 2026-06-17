package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel/adapters/whatsapp"
)

type WhatsAppChannelHandler struct {
	logger         *slog.Logger
	service        *whatsapp.Service
	botService     *bots.Service
	accountService *accounts.Service
}

func NewWhatsAppChannelHandler(log *slog.Logger, service *whatsapp.Service, botService *bots.Service, accountService *accounts.Service) *WhatsAppChannelHandler {
	if log == nil {
		log = slog.Default()
	}
	return &WhatsAppChannelHandler{
		logger:         log.With(slog.String("handler", "whatsapp_channel")),
		service:        service,
		botService:     botService,
		accountService: accountService,
	}
}

func (h *WhatsAppChannelHandler) Register(e *echo.Echo) {
	e.POST("/bots/:id/channel/whatsapp/qr/start", h.StartQR)
	e.POST("/bots/:id/channel/whatsapp/qr/poll", h.PollQR)
	e.POST("/bots/:id/channel/whatsapp/phone/start", h.StartPhone)
	e.POST("/bots/:id/channel/whatsapp/phone/poll", h.PollPhone)
	e.POST("/bots/:id/channel/whatsapp/login/cancel", h.CancelLogin)
	e.GET("/bots/:id/channel/whatsapp/status", h.Status)
	e.POST("/bots/:id/channel/whatsapp/logout", h.Logout)
}

// StartQR godoc
// @Summary Start WhatsApp QR login
// @Description Starts an experimental WhatsApp Web QR pairing flow for a bot channel.
// @Tags bots
// @Param id path string true "Bot ID"
// @Success 200 {object} whatsapp.QRStartResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id}/channel/whatsapp/qr/start [post].
func (h *WhatsAppChannelHandler) StartQR(c echo.Context) error {
	botID, err := h.authorize(c)
	if err != nil {
		return err
	}
	if h.service == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "whatsapp service not configured")
	}
	resp, err := h.service.StartQR(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

type WhatsAppQRPollRequest struct {
	LoginID string `json:"login_id" binding:"required"`
}

// PollQR godoc
// @Summary Poll WhatsApp QR login
// @Description Polls a WhatsApp QR pairing flow and finalizes the bot channel when pairing succeeds.
// @Tags bots
// @Param id path string true "Bot ID"
// @Param payload body WhatsAppQRPollRequest true "WhatsApp QR login id"
// @Success 200 {object} whatsapp.QRPollResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id}/channel/whatsapp/qr/poll [post].
func (h *WhatsAppChannelHandler) PollQR(c echo.Context) error {
	botID, err := h.authorize(c)
	if err != nil {
		return err
	}
	if h.service == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "whatsapp service not configured")
	}
	var req WhatsAppQRPollRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	loginID := strings.TrimSpace(req.LoginID)
	if loginID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "login_id is required")
	}
	resp, err := h.service.PollQR(c.Request().Context(), botID, loginID)
	if err != nil {
		return whatsappServiceHTTPError(err)
	}
	return c.JSON(http.StatusOK, resp)
}

type WhatsAppPhoneStartRequest struct {
	Phone string `json:"phone" binding:"required"`
}

// StartPhone godoc
// @Summary Start WhatsApp phone pairing
// @Description Starts an experimental WhatsApp Web phone pairing flow and returns a pairing code to enter on the phone.
// @Tags bots
// @Param id path string true "Bot ID"
// @Param payload body WhatsAppPhoneStartRequest true "WhatsApp phone number"
// @Success 200 {object} whatsapp.PhoneStartResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id}/channel/whatsapp/phone/start [post].
func (h *WhatsAppChannelHandler) StartPhone(c echo.Context) error {
	botID, err := h.authorize(c)
	if err != nil {
		return err
	}
	if h.service == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "whatsapp service not configured")
	}
	var req WhatsAppPhoneStartRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	phone := strings.TrimSpace(req.Phone)
	if phone == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "phone is required")
	}
	resp, err := h.service.StartPhone(c.Request().Context(), botID, phone)
	if err != nil {
		return whatsappServiceHTTPError(err)
	}
	return c.JSON(http.StatusOK, resp)
}

type WhatsAppPhonePollRequest struct {
	LoginID string `json:"login_id" binding:"required"`
}

// PollPhone godoc
// @Summary Poll WhatsApp phone pairing
// @Description Polls a WhatsApp phone pairing flow and finalizes the bot channel when pairing succeeds.
// @Tags bots
// @Param id path string true "Bot ID"
// @Param payload body WhatsAppPhonePollRequest true "WhatsApp phone pairing login id"
// @Success 200 {object} whatsapp.PhonePollResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id}/channel/whatsapp/phone/poll [post].
func (h *WhatsAppChannelHandler) PollPhone(c echo.Context) error {
	botID, err := h.authorize(c)
	if err != nil {
		return err
	}
	if h.service == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "whatsapp service not configured")
	}
	var req WhatsAppPhonePollRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	loginID := strings.TrimSpace(req.LoginID)
	if loginID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "login_id is required")
	}
	resp, err := h.service.PollPhone(c.Request().Context(), botID, loginID)
	if err != nil {
		return whatsappServiceHTTPError(err)
	}
	return c.JSON(http.StatusOK, resp)
}

type WhatsAppCancelLoginRequest struct {
	LoginID string `json:"login_id" binding:"required"`
}

// CancelLogin godoc
// @Summary Cancel WhatsApp pairing
// @Description Cancels an in-progress experimental WhatsApp Web QR or phone pairing flow.
// @Tags bots
// @Param id path string true "Bot ID"
// @Param payload body WhatsAppCancelLoginRequest true "WhatsApp login id"
// @Success 200 {object} whatsapp.CancelLoginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id}/channel/whatsapp/login/cancel [post].
func (h *WhatsAppChannelHandler) CancelLogin(c echo.Context) error {
	botID, err := h.authorize(c)
	if err != nil {
		return err
	}
	if h.service == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "whatsapp service not configured")
	}
	var req WhatsAppCancelLoginRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	loginID := strings.TrimSpace(req.LoginID)
	if loginID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "login_id is required")
	}
	resp, err := h.service.CancelLogin(c.Request().Context(), botID, loginID)
	if err != nil {
		return whatsappServiceHTTPError(err)
	}
	return c.JSON(http.StatusOK, resp)
}

// Status godoc
// @Summary Get WhatsApp channel status
// @Description Gets the persisted and runtime status for the bot's experimental WhatsApp channel.
// @Tags bots
// @Param id path string true "Bot ID"
// @Success 200 {object} whatsapp.StatusResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id}/channel/whatsapp/status [get].
func (h *WhatsAppChannelHandler) Status(c echo.Context) error {
	botID, err := h.authorize(c)
	if err != nil {
		return err
	}
	if h.service == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "whatsapp service not configured")
	}
	resp, err := h.service.Status(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Logout godoc
// @Summary Logout WhatsApp channel
// @Description Unlinks the WhatsApp Web session when possible, stops runtime connection, deletes local store, and removes channel config.
// @Tags bots
// @Param id path string true "Bot ID"
// @Success 200 {object} whatsapp.LogoutResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{id}/channel/whatsapp/logout [post].
func (h *WhatsAppChannelHandler) Logout(c echo.Context) error {
	botID, err := h.authorize(c)
	if err != nil {
		return err
	}
	if h.service == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "whatsapp service not configured")
	}
	resp, err := h.service.Logout(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *WhatsAppChannelHandler) authorize(c echo.Context) (string, error) {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return "", err
	}
	botID := strings.TrimSpace(c.Param("id"))
	if botID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID); err != nil {
		return "", err
	}
	return botID, nil
}

func whatsappServiceHTTPError(err error) *echo.HTTPError {
	switch {
	case errors.Is(err, whatsapp.ErrLoginNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, whatsapp.ErrPhonePairingFailed):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
}
