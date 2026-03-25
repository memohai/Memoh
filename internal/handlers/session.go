package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/session"
)

// SessionHandler handles bot session CRUD endpoints.
type SessionHandler struct {
	sessionService *session.Service
	botService     *bots.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

// NewSessionHandler creates a SessionHandler.
func NewSessionHandler(log *slog.Logger, sessionService *session.Service, botService *bots.Service, accountService *accounts.Service) *SessionHandler {
	return &SessionHandler{
		sessionService: sessionService,
		botService:     botService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "session")),
	}
}

// Register registers session routes.
func (h *SessionHandler) Register(e *echo.Echo) {
	g := e.Group("/bots/:bot_id/sessions")
	g.POST("", h.CreateSession)
	g.GET("", h.ListSessions)
	g.GET("/:session_id", h.GetSession)
	g.PATCH("/:session_id", h.UpdateSession)
	g.DELETE("/:session_id", h.DeleteSession)
}

type createSessionRequest struct {
	Title       string         `json:"title"`
	ChannelType string         `json:"channel_type,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type updateSessionRequest struct {
	Title    *string        `json:"title,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// CreateSession godoc
// @Summary Create a new chat session
// @Tags sessions
// @Param bot_id path string true "Bot ID"
// @Param body body createSessionRequest true "Session data"
// @Success 201 {object} session.Session
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions [post].
func (h *SessionHandler) CreateSession(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID); err != nil {
		return err
	}
	var req createSessionRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	sess, err := h.sessionService.Create(c.Request().Context(), session.CreateInput{
		BotID:       botID,
		ChannelType: req.ChannelType,
		Title:       req.Title,
		Metadata:    req.Metadata,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, sess)
}

// ListSessions godoc
// @Summary List bot sessions
// @Tags sessions
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} map[string][]session.Session
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions [get].
func (h *SessionHandler) ListSessions(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID); err != nil {
		return err
	}
	sessions, err := h.sessionService.ListByBot(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"items": sessions})
}

// GetSession godoc
// @Summary Get a session by ID
// @Tags sessions
// @Param bot_id path string true "Bot ID"
// @Param session_id path string true "Session ID"
// @Success 200 {object} session.Session
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions/{session_id} [get].
func (h *SessionHandler) GetSession(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID); err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session id is required")
	}
	sess, err := h.sessionService.Get(c.Request().Context(), sessionID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	if sess.BotID != botID {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	return c.JSON(http.StatusOK, sess)
}

// UpdateSession godoc
// @Summary Update a session
// @Tags sessions
// @Param bot_id path string true "Bot ID"
// @Param session_id path string true "Session ID"
// @Param body body updateSessionRequest true "Fields to update"
// @Success 200 {object} session.Session
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions/{session_id} [patch].
func (h *SessionHandler) UpdateSession(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID); err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session id is required")
	}

	existing, err := h.sessionService.Get(c.Request().Context(), sessionID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	if existing.BotID != botID {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}

	var req updateSessionRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	var result session.Session
	if req.Title != nil {
		result, err = h.sessionService.UpdateTitle(c.Request().Context(), sessionID, *req.Title)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	if req.Metadata != nil {
		result, err = h.sessionService.UpdateMetadata(c.Request().Context(), sessionID, req.Metadata)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	if req.Title == nil && req.Metadata == nil {
		result = existing
	}
	return c.JSON(http.StatusOK, result)
}

// DeleteSession godoc
// @Summary Soft-delete a session
// @Tags sessions
// @Param bot_id path string true "Bot ID"
// @Param session_id path string true "Session ID"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions/{session_id} [delete].
func (h *SessionHandler) DeleteSession(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID); err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session id is required")
	}
	if err := h.sessionService.SoftDelete(c.Request().Context(), sessionID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}
