package handlers

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/aidevengine"
)

type AIDevelopmentEngineHandler struct {
	service *aidevengine.Service
	logger  *slog.Logger
}

func NewAIDevelopmentEngineHandler(log *slog.Logger, service *aidevengine.Service) *AIDevelopmentEngineHandler {
	return &AIDevelopmentEngineHandler{
		service: service,
		logger:  log.With(slog.String("handler", "ai-development-engine")),
	}
}

func (h *AIDevelopmentEngineHandler) Register(e *echo.Echo) {
	group := e.Group("/ai-development-engine")
	group.GET("/status", h.Status)
	group.GET("/capabilities", h.Capabilities)
}

// Status godoc
// @Summary Get AI development engine status
// @Description Returns the static MVP connection status for the AI development engine.
// @Tags ai-development-engine
// @Produce json
// @Success 200 {object} aidevengine.StatusResponse
// @Router /ai-development-engine/status [get].
func (h *AIDevelopmentEngineHandler) Status(c echo.Context) error {
	return c.JSON(http.StatusOK, h.service.Status(c.Request().Context()))
}

// Capabilities godoc
// @Summary Get AI development engine capabilities
// @Description Returns the static MVP capability list for the AI development engine.
// @Tags ai-development-engine
// @Produce json
// @Success 200 {object} aidevengine.CapabilitiesResponse
// @Router /ai-development-engine/capabilities [get].
func (h *AIDevelopmentEngineHandler) Capabilities(c echo.Context) error {
	return c.JSON(http.StatusOK, h.service.Capabilities(c.Request().Context()))
}
