package handlers

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/identity"
	"github.com/memohai/memoh/internal/settings"
)

type SettingsHandler struct {
	service *settings.Service
	logger  *slog.Logger
}

func NewSettingsHandler(log *slog.Logger, service *settings.Service) *SettingsHandler {
	return &SettingsHandler{
		service: service,
		logger:  log.With(slog.String("handler", "settings")),
	}
}

func (h *SettingsHandler) Register(e *echo.Echo) {
	group := e.Group("/settings")
	group.GET("", h.Get)
	group.POST("", h.Upsert)
	group.PUT("", h.Upsert)
	group.DELETE("", h.Delete)
}

// Get godoc
// @Summary Get user settings
// @Description Get agent settings for current user
// @Tags settings
// @Success 200 {object} settings.Settings
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /settings [get]
func (h *SettingsHandler) Get(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	resp, err := h.service.Get(c.Request().Context(), userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Upsert godoc
// @Summary Update user settings
// @Description Update or create agent settings for current user
// @Tags settings
// @Param payload body settings.UpsertRequest true "Settings payload"
// @Success 200 {object} settings.Settings
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /settings [put]
// @Router /settings [post]
func (h *SettingsHandler) Upsert(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	var req settings.UpsertRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.service.Upsert(c.Request().Context(), userID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Delete godoc
// @Summary Delete user settings
// @Description Remove agent settings for current user
// @Tags settings
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /settings [delete]
func (h *SettingsHandler) Delete(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	if err := h.service.Delete(c.Request().Context(), userID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *SettingsHandler) requireUserID(c echo.Context) (string, error) {
	userID, err := auth.UserIDFromContext(c)
	if err != nil {
		return "", err
	}
	if err := identity.ValidateUserID(userID); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return userID, nil
}

