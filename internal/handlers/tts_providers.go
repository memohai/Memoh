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

	g.GET("/:id/models", h.ListModels)
	g.POST("/:id/import-models", h.ImportModels)

	mg := e.Group("/tts-models")
	mg.POST("", h.CreateModel)
	mg.GET("", h.ListAllModels)
	mg.GET("/:id", h.GetModel)
	mg.PUT("/:id", h.UpdateModel)
	mg.DELETE("/:id", h.DeleteModel)
	mg.GET("/:id/capabilities", h.GetModelCapabilities)
	mg.POST("/:id/test", h.TestModel)
}

// ListMeta godoc
// @Summary List TTS provider metadata
// @Description List available TTS provider types with their models and capabilities
// @Tags tts-providers
// @Success 200 {array} tts.ProviderMetaResponse
// @Router /tts-providers/meta [get].
func (h *TtsProvidersHandler) ListMeta(c echo.Context) error {
	return c.JSON(http.StatusOK, h.service.ListMeta(c.Request().Context()))
}

// Create godoc
// @Summary Create a TTS provider
// @Description Create a TTS provider and auto-import its available models
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
func (h *TtsProvidersHandler) Update(c echo.Context) error {
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

// ListModels godoc
// @Summary List models for a TTS provider
// @Tags tts-providers
// @Produce json
// @Param id path string true "Provider ID"
// @Success 200 {array} tts.ModelResponse
// @Failure 500 {object} ErrorResponse
// @Router /tts-providers/{id}/models [get].
func (h *TtsProvidersHandler) ListModels(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	items, err := h.service.ListModelsByProvider(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, items)
}

// ImportModels godoc
// @Summary Import models for a TTS provider
// @Description Discover and import available models from the TTS adapter
// @Tags tts-providers
// @Produce json
// @Param id path string true "Provider ID"
// @Success 200 {array} tts.ModelResponse
// @Failure 500 {object} ErrorResponse
// @Router /tts-providers/{id}/import-models [post].
func (h *TtsProvidersHandler) ImportModels(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	items, err := h.service.ImportModels(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, items)
}

// CreateModel godoc
// @Summary Create a TTS model
// @Description Manually create a TTS model under a specific provider
// @Tags tts-models
// @Accept json
// @Produce json
// @Param request body tts.CreateModelRequest true "TTS model configuration"
// @Success 201 {object} tts.ModelResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tts-models [post].
func (h *TtsProvidersHandler) CreateModel(c echo.Context) error {
	var req tts.CreateModelRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.ModelID) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "model_id is required")
	}
	if strings.TrimSpace(req.TtsProviderID) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tts_provider_id is required")
	}
	resp, err := h.service.CreateModel(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, resp)
}

// ListAllModels godoc
// @Summary List all TTS models
// @Tags tts-models
// @Produce json
// @Success 200 {array} tts.ModelResponse
// @Failure 500 {object} ErrorResponse
// @Router /tts-models [get].
func (h *TtsProvidersHandler) ListAllModels(c echo.Context) error {
	items, err := h.service.ListAllModels(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, items)
}

// GetModel godoc
// @Summary Get a TTS model
// @Tags tts-models
// @Produce json
// @Param id path string true "Model ID"
// @Success 200 {object} tts.ModelResponse
// @Failure 404 {object} ErrorResponse
// @Router /tts-models/{id} [get].
func (h *TtsProvidersHandler) GetModel(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	resp, err := h.service.GetModel(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// UpdateModel godoc
// @Summary Update a TTS model
// @Tags tts-models
// @Accept json
// @Produce json
// @Param id path string true "Model ID"
// @Param request body tts.UpdateModelRequest true "Updated configuration"
// @Success 200 {object} tts.ModelResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tts-models/{id} [put].
func (h *TtsProvidersHandler) UpdateModel(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	var req tts.UpdateModelRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.service.UpdateModel(c.Request().Context(), id, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// DeleteModel godoc
// @Summary Delete a TTS model
// @Tags tts-models
// @Param id path string true "Model ID"
// @Success 204 "No Content"
// @Failure 500 {object} ErrorResponse
// @Router /tts-models/{id} [delete].
func (h *TtsProvidersHandler) DeleteModel(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	if err := h.service.DeleteModel(c.Request().Context(), id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// GetModelCapabilities godoc
// @Summary Get TTS model capabilities
// @Tags tts-models
// @Produce json
// @Param id path string true "Model ID"
// @Success 200 {object} tts.ModelCapabilities
// @Failure 404 {object} ErrorResponse
// @Router /tts-models/{id}/capabilities [get].
func (h *TtsProvidersHandler) GetModelCapabilities(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	caps, err := h.service.GetModelCapabilities(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, caps)
}

// TestModel godoc
// @Summary Test TTS model synthesis
// @Description Synthesize text using a specific model's config and return audio
// @Tags tts-models
// @Accept json
// @Produce application/octet-stream
// @Param id path string true "Model ID"
// @Param request body tts.TestSynthesizeRequest true "Text to synthesize"
// @Success 200 {file} binary "Audio data"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tts-models/{id}/test [post].
func (h *TtsProvidersHandler) TestModel(c echo.Context) error {
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
