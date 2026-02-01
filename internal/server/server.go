package server

import (
	"log/slog"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/handlers"
	"github.com/memohai/memoh/internal/logger"
)

type Server struct {
	echo *echo.Echo
	addr string
}

func NewServer(addr string, jwtSecret string, pingHandler *handlers.PingHandler, authHandler *handlers.AuthHandler, memoryHandler *handlers.MemoryHandler, embeddingsHandler *handlers.EmbeddingsHandler, chatHandler *handlers.ChatHandler, swaggerHandler *handlers.SwaggerHandler, providersHandler *handlers.ProvidersHandler, modelsHandler *handlers.ModelsHandler, settingsHandler *handlers.SettingsHandler, historyHandler *handlers.HistoryHandler, scheduleHandler *handlers.ScheduleHandler, subagentHandler *handlers.SubagentHandler, containerdHandler *handlers.ContainerdHandler) *Server {
	if addr == "" {
		addr = ":8080"
	}

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus: true,
		LogURI:    true,
		LogMethod: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			logger.L.Info("request",
				slog.String("method", v.Method),
				slog.String("uri", v.URI),
				slog.Int("status", v.Status),
				slog.Duration("latency", v.Latency),
				slog.String("remote_ip", c.RealIP()),
			)
			return nil
		},
	}))
	e.Use(auth.JWTMiddleware(jwtSecret, func(c echo.Context) bool {
		path := c.Request().URL.Path
		if path == "/ping" || path == "/api/swagger.json" || path == "/auth/login" {
			return true
		}
		if strings.HasPrefix(path, "/api/docs") {
			return true
		}
		return false
	}))

	if pingHandler != nil {
		pingHandler.Register(e)
	}
	if authHandler != nil {
		authHandler.Register(e)
	}
	if memoryHandler != nil {
		memoryHandler.Register(e)
	}
	if embeddingsHandler != nil {
		embeddingsHandler.Register(e)
	}
	if chatHandler != nil {
		chatHandler.Register(e)
	}
	if swaggerHandler != nil {
		swaggerHandler.Register(e)
	}
	if settingsHandler != nil {
		settingsHandler.Register(e)
	}
	if historyHandler != nil {
		historyHandler.Register(e)
	}
	if scheduleHandler != nil {
		scheduleHandler.Register(e)
	}
	if subagentHandler != nil {
		subagentHandler.Register(e)
	}
	if providersHandler != nil {
		providersHandler.Register(e)
	}
	if modelsHandler != nil {
		modelsHandler.Register(e)
	}
	if containerdHandler != nil {
		containerdHandler.Register(e)
	}

	return &Server{
		echo: e,
		addr: addr,
	}
}

func (s *Server) Start() error {
	return s.echo.Start(s.addr)
}
