package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/providers"
)

type ProvidersHandler struct {
	service       *providers.Service
	modelsService *models.Service
	logger        *slog.Logger
}

func NewProvidersHandler(log *slog.Logger, service *providers.Service, modelsService *models.Service) *ProvidersHandler {
	return &ProvidersHandler{
		service:       service,
		modelsService: modelsService,
		logger:        log.With(slog.String("handler", "providers")),
	}
}

func (h *ProvidersHandler) Register(e *echo.Echo) {
	group := e.Group("/providers")
	group.POST("", h.Create)
	group.GET("", h.List)
	group.GET("/:id", h.Get)
	group.GET("/:id/models", h.ListModelsByProvider)
	group.GET("/name/:name", h.GetByName)
	group.PUT("/:id", h.Update)
	group.DELETE("/:id", h.Delete)
	group.GET("/count", h.Count)
	group.POST("/:id/test", h.Test)
}

// Create godoc
// @Summary Create a new LLM provider
// @Description Create a new LLM provider configuration
// @Tags providers
// @Accept json
// @Produce json
// @Param request body providers.CreateRequest true "Provider configuration"
// @Success 201 {object} providers.GetResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers [post]
func (h *ProvidersHandler) Create(c echo.Context) error {
	var req providers.CreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Validate required fields
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if req.BaseURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "base_url is required")
	}

	resp, err := h.service.Create(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, resp)
}

// List godoc
// @Summary List all LLM providers
// @Description Get a list of all configured LLM providers
// @Tags providers
// @Accept json
// @Produce json
// @Success 200 {array} providers.GetResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers [get]
func (h *ProvidersHandler) List(c echo.Context) error {
	resp, err := h.service.List(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// Get godoc
// @Summary Get provider by ID
// @Description Get a provider configuration by its ID
// @Tags providers
// @Accept json
// @Produce json
// @Param id path string true "Provider ID (UUID)"
// @Success 200 {object} providers.GetResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/{id} [get]
func (h *ProvidersHandler) Get(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	resp, err := h.service.Get(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// ListModelsByProvider godoc
// @Summary List provider models
// @Description Get models for a provider by id, optionally filtered by type
// @Tags providers
// @Param id path string true "Provider ID (UUID)"
// @Param type query string false "Model type (chat, embedding)"
// @Success 200 {array} models.GetResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/{id}/models [get]
func (h *ProvidersHandler) ListModelsByProvider(c echo.Context) error {
	if h.modelsService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "models service not configured")
	}
	id := c.Param("id")
	if strings.TrimSpace(id) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	modelType := strings.TrimSpace(c.QueryParam("type"))
	var (
		resp []models.GetResponse
		err  error
	)
	if modelType == "" {
		resp, err = h.modelsService.ListByProviderID(c.Request().Context(), id)
	} else {
		resp, err = h.modelsService.ListByProviderIDAndType(c.Request().Context(), id, models.ModelType(modelType))
	}
	if err != nil {
		if strings.Contains(err.Error(), "invalid") {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// GetByName godoc
// @Summary Get provider by name
// @Description Get a provider configuration by its name
// @Tags providers
// @Accept json
// @Produce json
// @Param name path string true "Provider name"
// @Success 200 {object} providers.GetResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/name/{name} [get]
func (h *ProvidersHandler) GetByName(c echo.Context) error {
	name := c.Param("name")
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}

	resp, err := h.service.GetByName(c.Request().Context(), name)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// Update godoc
// @Summary Update provider
// @Description Update an existing provider configuration
// @Tags providers
// @Accept json
// @Produce json
// @Param id path string true "Provider ID (UUID)"
// @Param request body providers.UpdateRequest true "Updated provider configuration"
// @Success 200 {object} providers.GetResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/{id} [put]
func (h *ProvidersHandler) Update(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	var req providers.UpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	resp, err := h.service.Update(c.Request().Context(), id, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}

// Delete godoc
// @Summary Delete provider
// @Description Delete a provider configuration
// @Tags providers
// @Accept json
// @Produce json
// @Param id path string true "Provider ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/{id} [delete]
func (h *ProvidersHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	if err := h.service.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// Count godoc
// @Summary Count providers
// @Description Get the total count of providers
// @Tags providers
// @Accept json
// @Produce json
// @Success 200 {object} providers.CountResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/count [get]
func (h *ProvidersHandler) Count(c echo.Context) error {
	count, err := h.service.Count(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, providers.CountResponse{Count: count})
}

// Test godoc
// @Summary Test provider connectivity
// @Description Probe a provider's base URL to check reachability, supported client types, and embedding support
// @Tags providers
// @Accept json
// @Produce json
// @Param id path string true "Provider ID (UUID)"
// @Success 200 {object} providers.TestResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /providers/{id}/test [post]
func (h *ProvidersHandler) Test(c echo.Context) error {
	id := c.Param("id")
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}

	resp, err := h.service.Test(c.Request().Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "invalid") {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, resp)
}
