package handlers

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
)

type PingHandler struct {
	logger *slog.Logger
}

func NewPingHandler(log *slog.Logger) *PingHandler {
	return &PingHandler{logger: log}
}

func (h *PingHandler) Register(e *echo.Echo) {
	e.GET("/ping", h.Ping)
}

func (h *PingHandler) Ping(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status": "ok",
	})
}
