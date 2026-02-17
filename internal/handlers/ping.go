package handlers

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
)

// PingHandler serves /ping and HEAD /health for liveness.
type PingHandler struct {
	logger *slog.Logger
}

// NewPingHandler creates a ping handler.
func NewPingHandler(log *slog.Logger) *PingHandler {
	return &PingHandler{logger: log.With(slog.String("handler", "ping"))}
}

// Register mounts GET /ping and HEAD /health on the Echo instance.
func (h *PingHandler) Register(e *echo.Echo) {
	e.GET("/ping", h.Ping)
	e.HEAD("/health", h.PingHead)
}

// Ping returns 200 JSON {"status":"ok"}.
func (h *PingHandler) Ping(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// PingHead returns 200 No Content for health checks.
func (h *PingHandler) PingHead(c echo.Context) error {
	return c.NoContent(http.StatusOK)
}
