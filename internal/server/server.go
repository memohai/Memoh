package server

import (
	"context"
	"log/slog"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/memohai/memoh/internal/auth"
)

type Server struct {
	echo   *echo.Echo
	addr   string
	logger *slog.Logger
}

type Handler interface {
	Register(e *echo.Echo)
}

func NewServer(log *slog.Logger, addr string, jwtSecret string,
	handlers ...Handler,
) *Server {
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
			log.Info("request",
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
		return shouldSkipJWT(c.Request().URL.Path)
	}))

	for _, h := range handlers {
		if h != nil {
			h.Register(e)
		}
	}

	return &Server{
		echo:   e,
		addr:   addr,
		logger: log.With(slog.String("component", "server")),
	}
}

func (s *Server) Start() error {
	return s.echo.Start(s.addr)
}

func (s *Server) Stop(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}

func shouldSkipJWT(path string) bool {
	if path == "/" || path == "/ping" || path == "/health" || path == "/api/swagger.json" || path == "/auth/login" {
		return true
	}
	if strings.HasPrefix(path, "/assets/") {
		return true
	}
	if strings.HasPrefix(path, "/api/docs") {
		return true
	}
	if isPublicChannelWebhookPath(path) {
		return true
	}
	if strings.HasPrefix(path, "/email/mailgun/webhook/") {
		return true
	}
	if strings.HasPrefix(path, "/email/oauth/callback") || strings.HasPrefix(path, "/api/email/oauth/callback") {
		return true
	}
	if strings.HasPrefix(path, "/providers/oauth/callback") {
		return true
	}
	if strings.HasPrefix(path, "/auth/callback") {
		return true
	}
	return false
}

func isPublicChannelWebhookPath(path string) bool {
	if !strings.HasPrefix(path, "/channels/") {
		return false
	}
	trimmed := strings.Trim(strings.TrimPrefix(path, "/channels/"), "/")
	parts := strings.Split(trimmed, "/")
	return len(parts) >= 3 && strings.TrimSpace(parts[0]) != "" && parts[1] == "webhook" && strings.TrimSpace(parts[2]) != ""
}
