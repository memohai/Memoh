package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/browsercontexts"
	"github.com/memohai/memoh/internal/config"
)

type BrowserContextsHandler struct {
	service           *browsercontexts.Service
	logger            *slog.Logger
	browserGatewayURL string
}

func NewBrowserContextsHandler(log *slog.Logger, service *browsercontexts.Service, cfg config.Config) *BrowserContextsHandler {
	return &BrowserContextsHandler{
		service:           service,
		logger:            log.With(slog.String("handler", "browser_contexts")),
		browserGatewayURL: cfg.BrowserGateway.BaseURL(),
	}
}

func (h *BrowserContextsHandler) Register(e *echo.Echo) {
	group := e.Group("/browser-contexts")
	group.POST("", h.Create)
	group.GET("", h.List)
	group.GET("/cores", h.GetCores)
	group.GET("/:id", h.Get)
	group.PUT("/:id", h.Update)
	group.DELETE("/:id", h.Delete)
}

// Create godoc
// @Summary Create a browser context
// @Description Create a browser context configuration
// @Tags browser-contexts
// @Accept json
// @Produce json
// @Param request body browsercontexts.CreateRequest true "Browser context configuration"
// @Success 201 {object} browsercontexts.BrowserContext
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /browser-contexts [post].
func (h *BrowserContextsHandler) Create(c echo.Context) error {
	var req browsercontexts.CreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.service.Create(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, resp)
}

// List godoc
// @Summary List browser contexts
// @Description List all browser context configurations
// @Tags browser-contexts
// @Produce json
// @Success 200 {array} browsercontexts.BrowserContext
// @Failure 500 {object} ErrorResponse
// @Router /browser-contexts [get].
func (h *BrowserContextsHandler) List(c echo.Context) error {
	items, err := h.service.List(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, items)
}

// Get godoc
// @Summary Get a browser context
// @Description Get browser context by ID
// @Tags browser-contexts
// @Produce json
// @Param id path string true "Browser Context ID"
// @Success 200 {object} browsercontexts.BrowserContext
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /browser-contexts/{id} [get].
func (h *BrowserContextsHandler) Get(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	resp, err := h.service.GetByID(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Update godoc
// @Summary Update a browser context
// @Description Update browser context by ID
// @Tags browser-contexts
// @Accept json
// @Produce json
// @Param id path string true "Browser Context ID"
// @Param request body browsercontexts.UpdateRequest true "Updated configuration"
// @Success 200 {object} browsercontexts.BrowserContext
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /browser-contexts/{id} [put].
func (h *BrowserContextsHandler) Update(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	var req browsercontexts.UpdateRequest
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
// @Summary Delete a browser context
// @Description Delete browser context by ID
// @Tags browser-contexts
// @Param id path string true "Browser Context ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /browser-contexts/{id} [delete].
func (h *BrowserContextsHandler) Delete(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	if err := h.service.Delete(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// GetCores godoc
// @Summary Get available browser cores
// @Description Get the list of browser cores available in the Browser Gateway container
// @Tags browser-contexts
// @Produce json
// @Success 200 {object} BrowserCoresResponse
// @Failure 502 {object} ErrorResponse
// @Router /browser-contexts/cores [get].
func (h *BrowserContextsHandler) GetCores(c echo.Context) error {
	url := fmt.Sprintf("%s/cores/", h.browserGatewayURL)
	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodGet, url, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "failed to create request")
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // URL is from trusted internal config
	if err != nil {
		h.logger.Warn("browser gateway unreachable", slog.String("error", err.Error()))
		return c.JSON(http.StatusOK, BrowserCoresResponse{Cores: []string{"chromium"}})
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "failed to read browser gateway response")
	}

	var result BrowserCoresResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "failed to parse browser gateway response")
	}
	return c.JSON(http.StatusOK, result)
}

// BrowserCoresResponse is the response for the GetCores endpoint.
type BrowserCoresResponse struct {
	Cores []string `json:"cores"`
}
