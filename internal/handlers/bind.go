package handlers

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/bind"
)

// BindHandler manages channel identity bind code issuance via REST API.
type BindHandler struct {
	service *bind.Service
	logger  *slog.Logger
}

// NewBindHandler creates a BindHandler.
func NewBindHandler(log *slog.Logger, service *bind.Service) *BindHandler {
	if log == nil {
		log = slog.Default()
	}
	return &BindHandler{
		service: service,
		logger:  log.With(slog.String("handler", "bind")),
	}
}

// Register registers bind code routes.
func (h *BindHandler) Register(e *echo.Echo) {
	e.POST("/users/me/bind_codes", h.Issue)
}

type bindIssueRequest struct {
	Platform   string `json:"platform,omitempty"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
}

type bindIssueResponse struct {
	Token     string    `json:"token"`
	Platform  string    `json:"platform,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Issue creates a new bind code for the current user.
func (h *BindHandler) Issue(c echo.Context) error {
	if h.service == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "bind service not available")
	}
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}

	var req bindIssueRequest
	if err := c.Bind(&req); err != nil && !errors.Is(err, io.EOF) {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	ttl := 24 * time.Hour
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}

	code, err := h.service.Issue(c.Request().Context(), userID, strings.TrimSpace(req.Platform), ttl)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, bindIssueResponse{
		Token:     code.Token,
		Platform:  code.Platform,
		ExpiresAt: code.ExpiresAt,
	})
}

func (h *BindHandler) requireUserID(c echo.Context) (string, error) {
	return RequireChannelIdentityID(c)
}
