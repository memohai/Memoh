package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/email"
)

type EmailBindingsHandler struct {
	service *email.Service
	manager *email.Manager
	logger  *slog.Logger
}

func NewEmailBindingsHandler(log *slog.Logger, service *email.Service, manager *email.Manager) *EmailBindingsHandler {
	return &EmailBindingsHandler{
		service: service,
		manager: manager,
		logger:  log.With(slog.String("handler", "email_bindings")),
	}
}

func (h *EmailBindingsHandler) Register(e *echo.Echo) {
	g := e.Group("/bots/:bot_id/email-bindings")
	g.POST("", h.Create)
	g.GET("", h.List)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
}

// Create godoc
// @Summary Bind an email provider to a bot
// @Tags email-bindings
// @Accept json
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param request body email.CreateBindingRequest true "Binding configuration"
// @Success 201 {object} email.BindingResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/email-bindings [post]
func (h *EmailBindingsHandler) Create(c echo.Context) error {
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot_id is required")
	}
	var req email.CreateBindingRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.EmailProviderID) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email_provider_id is required")
	}
	if strings.TrimSpace(req.EmailAddress) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email_address is required")
	}
	resp, err := h.service.CreateBinding(c.Request().Context(), botID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	// Refresh provider connections after binding change
	_ = h.manager.RefreshProvider(c.Request().Context(), req.EmailProviderID)
	return c.JSON(http.StatusCreated, resp)
}

// List godoc
// @Summary List email bindings for a bot
// @Tags email-bindings
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Success 200 {array} email.BindingResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/email-bindings [get]
func (h *EmailBindingsHandler) List(c echo.Context) error {
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot_id is required")
	}
	items, err := h.service.ListBindings(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, items)
}

// Update godoc
// @Summary Update an email binding
// @Tags email-bindings
// @Accept json
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Binding ID"
// @Param request body email.UpdateBindingRequest true "Updated binding"
// @Success 200 {object} email.BindingResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/email-bindings/{id} [put]
func (h *EmailBindingsHandler) Update(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	var req email.UpdateBindingRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.service.UpdateBinding(c.Request().Context(), id, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	_ = h.manager.RefreshProvider(c.Request().Context(), resp.EmailProviderID)
	return c.JSON(http.StatusOK, resp)
}

// Delete godoc
// @Summary Remove an email binding
// @Tags email-bindings
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Binding ID"
// @Success 204 "No Content"
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/email-bindings/{id} [delete]
func (h *EmailBindingsHandler) Delete(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	// Get binding info before delete for refresh
	binding, _ := h.service.GetBinding(c.Request().Context(), id)
	if err := h.service.DeleteBinding(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if binding.EmailProviderID != "" {
		_ = h.manager.RefreshProvider(c.Request().Context(), binding.EmailProviderID)
	}
	return c.NoContent(http.StatusNoContent)
}
