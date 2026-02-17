// Package server provides the HTTP server and Echo setup for the agent API.
package server

import (
	"context"
	"log/slog"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/memohai/memoh/internal/auth"
)

// Server is the HTTP server (Echo) with JWT middleware and registered handlers.
type Server struct {
	echo   *echo.Echo
	addr   string
	logger *slog.Logger
}

// Handler registers routes on the Echo instance.
type Handler interface {
	Register(e *echo.Echo)
}

// NewServer builds the Echo server with recovery, request logging, JWT auth, and the given handlers.
func NewServer(log *slog.Logger, addr, jwtSecret string,
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
		path := c.Request().URL.Path
		if path == "/ping" || path == "/health" || path == "/api/swagger.json" || path == "/auth/login" {
			return true
		}
		if strings.HasPrefix(path, "/api/docs") {
			return true
		}
		return false
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

// Start starts the HTTP server (blocks until shutdown).
func (s *Server) Start() error {
	return s.echo.Start(s.addr)
}

// Stop gracefully shuts down the server using the given context.
func (s *Server) Stop(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}
