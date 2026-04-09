package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/tts"
)

type SpeechHandler struct {
	service *tts.Service
	logger  *slog.Logger
}

func NewSpeechHandler(log *slog.Logger, service *tts.Service) *SpeechHandler {
	return &SpeechHandler{
		service: service,
		logger:  log.With(slog.String("handler", "speech")),
	}
}

func (h *SpeechHandler) Register(e *echo.Echo) {
	pg := e.Group("/speech-providers")
	pg.GET("", h.ListProviders)
	pg.GET("/meta", h.ListMeta)

	mg := e.Group("/speech-models")
	mg.GET("", h.ListModels)
	mg.GET("/:id", h.GetModel)
	mg.GET("/:id/capabilities", h.GetModelCapabilities)
	mg.POST("/:id/test", h.TestModel)
}

// ListMeta godoc
// @Summary List speech provider metadata
// @Description List available speech provider types with their models and capabilities
// @Tags speech-providers
// @Success 200 {array} tts.ProviderMetaResponse
// @Router /speech-providers/meta [get].
func (h *SpeechHandler) ListMeta(c echo.Context) error {
	return c.JSON(http.StatusOK, h.service.ListMeta(c.Request().Context()))
}

// ListProviders godoc
// @Summary List speech providers
// @Description List providers that support speech (filtered view of unified providers table)
// @Tags speech-providers
// @Produce json
// @Success 200 {array} tts.SpeechProviderResponse
// @Failure 500 {object} ErrorResponse
// @Router /speech-providers [get].
func (h *SpeechHandler) ListProviders(c echo.Context) error {
	items, err := h.service.ListSpeechProviders(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, items)
}

// ListModels godoc
// @Summary List all speech models
// @Description List all models of type 'speech' (filtered view of unified models table)
// @Tags speech-models
// @Produce json
// @Success 200 {array} tts.SpeechModelResponse
// @Failure 500 {object} ErrorResponse
// @Router /speech-models [get].
func (h *SpeechHandler) ListModels(c echo.Context) error {
	items, err := h.service.ListSpeechModels(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, items)
}

// GetModel godoc
// @Summary Get a speech model
// @Tags speech-models
// @Produce json
// @Param id path string true "Model ID"
// @Success 200 {object} tts.SpeechModelResponse
// @Failure 404 {object} ErrorResponse
// @Router /speech-models/{id} [get].
func (h *SpeechHandler) GetModel(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	resp, err := h.service.GetSpeechModel(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// GetModelCapabilities godoc
// @Summary Get speech model capabilities
// @Tags speech-models
// @Produce json
// @Param id path string true "Model ID"
// @Success 200 {object} tts.ModelCapabilities
// @Failure 404 {object} ErrorResponse
// @Router /speech-models/{id}/capabilities [get].
func (h *SpeechHandler) GetModelCapabilities(c echo.Context) error {
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
// @Summary Test speech model synthesis
// @Description Synthesize text using a specific model's config and return audio
// @Tags speech-models
// @Accept json
// @Produce application/octet-stream
// @Param id path string true "Model ID"
// @Param request body tts.TestSynthesizeRequest true "Text to synthesize"
// @Success 200 {file} binary "Audio data"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /speech-models/{id}/test [post].
func (h *SpeechHandler) TestModel(c echo.Context) error {
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
