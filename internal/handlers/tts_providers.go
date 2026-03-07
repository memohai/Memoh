package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/tts"
)

type TtsProvidersHandler struct {
	service *tts.Service
	logger  *slog.Logger
}

func NewTtsProvidersHandler(log *slog.Logger, service *tts.Service) *TtsProvidersHandler {
	return &TtsProvidersHandler{
		service: service,
		logger:  log.With(slog.String("handler", "tts_providers")),
	}
}

func (h *TtsProvidersHandler) Register(e *echo.Echo) {
	g := e.Group("/tts-providers")
	g.GET("/meta", h.ListMeta)
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET("/:id", h.Get)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
	g.POST("/:id/test", h.Test)
}

// ListMeta godoc
// @Summary List TTS provider metadata
// @Description List available TTS provider types with their capabilities
// @Tags tts-providers
// @Success 200 {array} tts.ProviderMetaResponse
// @Router /tts-providers/meta [get].
func (h *TtsProvidersHandler) ListMeta(c echo.Context) error {
	return c.JSON(http.StatusOK, h.service.ListMeta(c.Request().Context()))
}

// Create godoc
// @Summary Create a TTS provider
// @Tags tts-providers
// @Accept json
// @Produce json
// @Param request body tts.CreateProviderRequest true "TTS provider configuration"
// @Success 201 {object} tts.ProviderResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tts-providers [post].
func (h *TtsProvidersHandler) Create(c echo.Context) error {
	var req tts.CreateProviderRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.Name) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if strings.TrimSpace(string(req.Provider)) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "provider is required")
	}
	resp, err := h.service.CreateProvider(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, resp)
}

// List godoc
// @Summary List TTS providers
// @Tags tts-providers
// @Produce json
// @Param provider query string false "Provider type filter"
// @Success 200 {array} tts.ProviderResponse
// @Failure 500 {object} ErrorResponse
// @Router /tts-providers [get].
func (h *TtsProvidersHandler) List(c echo.Context) error {
	items, err := h.service.ListProviders(c.Request().Context(), c.QueryParam("provider"))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, items)
}

// Get godoc
// @Summary Get a TTS provider
// @Tags tts-providers
// @Produce json
// @Param id path string true "Provider ID"
// @Success 200 {object} tts.ProviderResponse
// @Failure 404 {object} ErrorResponse
// @Router /tts-providers/{id} [get].
func (h *TtsProvidersHandler) Get(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	resp, err := h.service.GetProvider(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Update godoc
// @Summary Update a TTS provider
// @Tags tts-providers
// @Accept json
// @Produce json
// @Param id path string true "Provider ID"
// @Param request body tts.UpdateProviderRequest true "Updated configuration"
// @Success 200 {object} tts.ProviderResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tts-providers/{id} [put].
func (h *TtsProvidersHandler)  Update(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	var req tts.UpdateProviderRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.service.UpdateProvider(c.Request().Context(), id, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Test godoc
// @Summary Test TTS synthesis
// @Description Synthesize text using the provider's saved config and return audio
// @Tags tts-providers
// @Accept json
// @Produce application/octet-stream
// @Param id path string true "Provider ID"
// @Param request body tts.TestSynthesizeRequest true "Text to synthesize"
// @Success 200 {file} binary "Audio data"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tts-providers/{id}/test [post].
func (h *TtsProvidersHandler) Test(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	var req tts.TestSynthesizeRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "text is required")
	}
	const maxTestTextLen = 500
	if len([]rune(text)) > maxTestTextLen {
		return echo.NewHTTPError(http.StatusBadRequest, "text too long, max 500 characters")
	}
	audio, contentType, err := h.service.Synthesize(c.Request().Context(), id, text, req.Config)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.Blob(http.StatusOK, contentType, audio)
}

// Delete godoc
// @Summary Delete a TTS provider
// @Tags tts-providers
// @Param id path string true "Provider ID"
// @Success 204 "No Content"
// @Failure 500 {object} ErrorResponse
// @Router /tts-providers/{id} [delete].
func (h *TtsProvidersHandler) Delete(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	if err := h.service.DeleteProvider(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}
