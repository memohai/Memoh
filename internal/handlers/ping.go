package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/container"
	"github.com/memohai/memoh/internal/version"
)

type PingResponse struct {
	Status                string `json:"status"`
	ContainerBackend      string `json:"container_backend"`
	LocalWorkspaceEnabled bool   `json:"local_workspace_enabled"`
	SnapshotSupported     bool   `json:"snapshot_supported"`
	Version               string `json:"version"`
	CommitHash            string `json:"commit_hash"`
}

type PingHandler struct {
	logger  *slog.Logger
	runtime *boot.RuntimeConfig
	service ctr.Service
	cfg     config.Config
}

type snapshotCapabilityProvider interface {
	SnapshotSupported(ctx context.Context) bool
}

func NewPingHandler(log *slog.Logger, rc *boot.RuntimeConfig, service ctr.Service, cfg config.Config) *PingHandler {
	return &PingHandler{
		logger:  log.With(slog.String("handler", "ping")),
		runtime: rc,
		service: service,
		cfg:     cfg,
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
// @Router /ping [get].
func (h *PingHandler) Ping(c echo.Context) error {
	return c.JSON(http.StatusOK, PingResponse{
		Status:                "ok",
		ContainerBackend:      ctr.NormalizeBackend(h.runtime.ContainerBackend),
		LocalWorkspaceEnabled: h.cfg.Local.Enabled,
		SnapshotSupported:     h.snapshotSupported(c.Request().Context()),
		Version:               version.Version,
		CommitHash:            version.ShortCommitHash(),
	})
}

func (*PingHandler) PingHead(c echo.Context) error {
	return c.NoContent(http.StatusOK)
}

func (h *PingHandler) snapshotSupported(ctx context.Context) bool {
	switch h.runtime.ContainerBackend {
	case "apple":
		return false
	case ctr.BackendKubernetes, ctr.BackendK8s:
		provider, ok := h.service.(snapshotCapabilityProvider)
		if !ok {
			return false
		}
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return provider.SnapshotSupported(probeCtx)
	default:
		return true
	}
}
