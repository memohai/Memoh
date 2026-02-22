package handlers

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/memohai/memoh/internal/boot"
)

type PingResponse struct {
	Status            string `json:"status"`
	ContainerBackend  string `json:"container_backend"`
	SnapshotSupported bool   `json:"snapshot_supported"`
}

type PingHandler struct {
	logger  *slog.Logger
	runtime *boot.RuntimeConfig
}

func NewPingHandler(log *slog.Logger, rc *boot.RuntimeConfig) *PingHandler {
	return &PingHandler{
		logger:  log.With(slog.String("handler", "ping")),
		runtime: rc,
	}
}

func (h *PingHandler) Register(e *echo.Echo) {
	e.GET("/ping", h.Ping)
	e.HEAD("/health", h.PingHead)
}

// Ping godoc
// @Summary Health check with server capabilities
// @Tags system
// @Success 200 {object} PingResponse
// @Router /ping [get]
func (h *PingHandler) Ping(c echo.Context) error {
	return c.JSON(http.StatusOK, PingResponse{
		Status:            "ok",
		ContainerBackend:  h.runtime.ContainerBackend,
		SnapshotSupported: h.runtime.ContainerBackend != "apple",
	})
}

func (h *PingHandler) PingHead(c echo.Context) error {
	return c.NoContent(http.StatusOK)
}
