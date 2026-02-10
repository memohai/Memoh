package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/containerd"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/identity"
	"github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/users"
)

type ContainerdHandler struct {
	service      ctr.Service
	cfg          config.MCPConfig
	namespace    string
	logger       *slog.Logger
	mcpMu        sync.Mutex
	mcpSess      map[string]*mcpSession
	mcpStdioMu   sync.Mutex
	mcpStdioSess map[string]*mcpStdioSession
	botService   *bots.Service
	userService  *users.Service
	queries      *dbsqlc.Queries
}

type CreateContainerRequest struct {
	Image       string `json:"image,omitempty"`
	Snapshotter string `json:"snapshotter,omitempty"`
}

type CreateContainerResponse struct {
	ContainerID string `json:"container_id"`
	Image       string `json:"image"`
	Snapshotter string `json:"snapshotter"`
	Started     bool   `json:"started"`
}

type GetContainerResponse struct {
	ContainerID   string    `json:"container_id"`
	Image         string    `json:"image"`
	Status        string    `json:"status"`
	Namespace     string    `json:"namespace"`
	HostPath      string    `json:"host_path,omitempty"`
	ContainerPath string    `json:"container_path"`
	TaskRunning   bool      `json:"task_running"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreateSnapshotRequest struct {
	SnapshotName string `json:"snapshot_name"`
}

type CreateSnapshotResponse struct {
	ContainerID  string `json:"container_id"`
	SnapshotName string `json:"snapshot_name"`
	Snapshotter  string `json:"snapshotter"`
}

type SnapshotInfo struct {
	Snapshotter string            `json:"snapshotter"`
	Name        string            `json:"name"`
	Parent      string            `json:"parent,omitempty"`
	Kind        string            `json:"kind"`
	CreatedAt   time.Time         `json:"created_at,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type ListSnapshotsResponse struct {
	Snapshotter string         `json:"snapshotter"`
	Snapshots   []SnapshotInfo `json:"snapshots"`
}

func NewContainerdHandler(log *slog.Logger, service ctr.Service, cfg config.Config, botService *bots.Service, userService *users.Service, queries *dbsqlc.Queries) *ContainerdHandler {
	return &ContainerdHandler{
		service:      service,
		cfg:          cfg.MCP,
		namespace:    cfg.Containerd.Namespace,
		logger:       log.With(slog.String("handler", "containerd")),
		mcpSess:      make(map[string]*mcpSession),
		mcpStdioSess: make(map[string]*mcpStdioSession),
		botService:   botService,
		userService:  userService,
		queries:      queries,
	}
}

func (h *ContainerdHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id/container")
	group.POST("", h.CreateContainer)
	group.GET("", h.GetContainer)
	group.DELETE("", h.DeleteContainer)
	group.POST("/start", h.StartContainer)
	group.POST("/stop", h.StopContainer)
	group.POST("/snapshots", h.CreateSnapshot)
	group.GET("/snapshots", h.ListSnapshots)
	group.GET("/skills", h.ListSkills)
	group.POST("/skills", h.UpsertSkills)
	group.DELETE("/skills", h.DeleteSkills)
	group.POST("/fs-mcp", h.HandleMCPFS)

	root := e.Group("/bots/:bot_id")
	fs := e.Group("/bots/:bot_id/container/fs")
	fs.GET("", h.ListFS)
	fs.GET("/file", h.ReadFSFile)
	fs.GET("/stat", h.StatFS)
	fs.GET("/usage", h.UsageFS)
	fs.POST("/file", h.WriteFSFile)
	fs.POST("/dir", h.MkdirFS)
	fs.POST("/upload", h.UploadFS)
	fs.DELETE("", h.DeleteFS)
	root.POST("/mcp-stdio", h.CreateMCPStdio)
	root.POST("/mcp-stdio/:session_id", h.HandleMCPStdio)
}

// CreateContainer godoc
// @Summary Create and start MCP container for bot
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body CreateContainerRequest true "Create container payload"
// @Success 200 {object} CreateContainerResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container [post]
func (h *ContainerdHandler) CreateContainer(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	var req CreateContainerRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	containerID := mcp.ContainerPrefix + botID

	image := strings.TrimSpace(req.Image)
	if image == "" {
		image = h.cfg.BusyboxImage
	}
	if image == "" {
		image = config.DefaultBusyboxImg
	}
	snapshotter := strings.TrimSpace(req.Snapshotter)
	if snapshotter == "" {
		snapshotter = h.cfg.Snapshotter
	}

	ctx := c.Request().Context()
	if strings.TrimSpace(h.namespace) != "" {
		ctx = namespaces.WithNamespace(ctx, h.namespace)
	}
	dataRoot := strings.TrimSpace(h.cfg.DataRoot)
	if dataRoot == "" {
		dataRoot = config.DefaultDataRoot
	}
	dataRoot, _ = filepath.Abs(dataRoot)
	dataMount := strings.TrimSpace(h.cfg.DataMount)
	if dataMount == "" {
		dataMount = config.DefaultDataMount
	}
	dataDir := filepath.Join(dataRoot, "bots", botID)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := os.MkdirAll(filepath.Join(dataDir, ".skills"), 0o755); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	resolvPath, err := ctr.ResolveConfSource(dataDir)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	specOpts := []oci.SpecOpts{
		oci.WithMounts([]specs.Mount{
			{
				Destination: dataMount,
				Type:        "bind",
				Source:      dataDir,
				Options:     []string{"rbind", "rw"},
			},
			{
				Destination: "/app",
				Type:        "bind",
				Source:      dataDir,
				Options:     []string{"rbind", "rw"},
			},
			{
				Destination: "/etc/resolv.conf",
				Type:        "bind",
				Source:      resolvPath,
				Options:     []string{"rbind", "ro"},
			},
		}),
		oci.WithProcessArgs("/bin/sh", "-lc", fmt.Sprintf("bootstrap(){ [ -e /app/mcp ] || { mkdir -p /app; [ -f /opt/mcp ] && cp -a /opt/mcp /app/mcp 2>/dev/null || true; }; if [ -d /opt/mcp-template ]; then mkdir -p %q; for f in /opt/mcp-template/*; do name=$(basename \"$f\"); [ -e %q/\"$name\" ] || cp -a \"$f\" %q/\"$name\" 2>/dev/null || true; done; fi; }; bootstrap; exec /app/mcp", dataMount, dataMount, dataMount)),
	}

	_, err = h.service.CreateContainer(ctx, ctr.CreateContainerRequest{
		ID:          containerID,
		ImageRef:    image,
		Snapshotter: snapshotter,
		Labels: map[string]string{
			mcp.BotLabelKey: botID,
		},
		SpecOpts: specOpts,
	})
	if err != nil && !errdefs.IsAlreadyExists(err) {
		return echo.NewHTTPError(http.StatusInternalServerError, "snapshotter="+snapshotter+" image="+image+" err="+err.Error())
	}

	if h.queries != nil {
		pgBotID, parseErr := parsePgUUID(botID)
		if parseErr == nil {
			ns := strings.TrimSpace(h.namespace)
			if ns == "" {
				ns = "default"
			}
			_ = h.queries.UpsertContainer(c.Request().Context(), dbsqlc.UpsertContainerParams{
				BotID:         pgBotID,
				ContainerID:   containerID,
				ContainerName: containerID,
				Image:         image,
				Status:        "created",
				Namespace:     ns,
				AutoStart:     true,
				HostPath:      pgtype.Text{String: dataDir, Valid: true},
				ContainerPath: dataMount,
			})
		}
	}

	started := false
	if task, err := h.service.StartTask(ctx, containerID, &ctr.StartTaskOptions{
		UseStdio: false,
	}); err == nil {
		if netErr := ctr.SetupNetwork(ctx, task, containerID); netErr == nil {
			started = true
			if h.queries != nil {
				if pgBotID, parseErr := parsePgUUID(botID); parseErr == nil {
					_ = h.queries.UpdateContainerStarted(c.Request().Context(), pgBotID)
				}
			}
		} else {
			_ = h.service.StopTask(ctx, containerID, &ctr.StopTaskOptions{Force: true})
			h.logger.Error("mcp container network setup failed",
				slog.String("container_id", containerID),
				slog.Any("error", netErr),
			)
		}
	} else {
		h.logger.Error("mcp container start failed",
			slog.String("container_id", containerID),
			slog.Any("error", err),
		)
	}

	return c.JSON(http.StatusOK, CreateContainerResponse{
		ContainerID: containerID,
		Image:       image,
		Snapshotter: snapshotter,
		Started:     started,
	})
}

func (h *ContainerdHandler) ensureTaskRunning(ctx context.Context, containerID string) error {
	tasks, err := h.service.ListTasks(ctx, &ctr.ListTasksOptions{
		Filter: "container.id==" + containerID,
	})
	if err != nil {
		return err
	}
	if len(tasks) > 0 {
		if tasks[0].Status == tasktypes.Status_RUNNING {
			return nil
		}
		_ = h.service.DeleteTask(ctx, containerID, &ctr.DeleteTaskOptions{Force: true})
	}

	task, err := h.service.StartTask(ctx, containerID, &ctr.StartTaskOptions{
		UseStdio: false,
	})
	if err != nil {
		return err
	}
	if err := ctr.SetupNetwork(ctx, task, containerID); err != nil {
		_ = h.service.StopTask(ctx, containerID, &ctr.StopTaskOptions{Force: true})
		return err
	}
	return err
}

// botContainerID resolves container_id for a bot from the database.
func (h *ContainerdHandler) botContainerID(ctx context.Context, botID string) (string, error) {
	if h.queries != nil {
		pgBotID, err := parsePgUUID(botID)
		if err == nil {
			row, err := h.queries.GetContainerByBotID(ctx, pgBotID)
			if err == nil && strings.TrimSpace(row.ContainerID) != "" {
				return row.ContainerID, nil
			}
		}
	}
	// Fallback: search by containerd label
	containers, err := h.service.ListContainersByLabel(ctx, mcp.BotLabelKey, botID)
	if err != nil {
		return "", err
	}
	if len(containers) == 0 {
		return "", echo.NewHTTPError(http.StatusNotFound, "container not found")
	}
	infoCtx := ctx
	if strings.TrimSpace(h.namespace) != "" {
		infoCtx = namespaces.WithNamespace(ctx, h.namespace)
	}
	bestID := ""
	var bestUpdated time.Time
	for _, container := range containers {
		info, err := container.Info(infoCtx)
		if err != nil {
			return "", err
		}
		if bestID == "" || info.UpdatedAt.After(bestUpdated) {
			bestID = info.ID
			bestUpdated = info.UpdatedAt
		}
	}
	return bestID, nil
}

// GetContainer godoc
// @Summary Get container info for bot
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} GetContainerResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container [get]
func (h *ContainerdHandler) GetContainer(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()

	if h.queries != nil {
		pgBotID, parseErr := parsePgUUID(botID)
		if parseErr == nil {
			row, dbErr := h.queries.GetContainerByBotID(ctx, pgBotID)
			if dbErr == nil {
				taskRunning := h.isTaskRunning(ctx, row.ContainerID)
				hostPath := ""
				if row.HostPath.Valid {
					hostPath = row.HostPath.String
				}
				createdAt := time.Time{}
				if row.CreatedAt.Valid {
					createdAt = row.CreatedAt.Time
				}
				updatedAt := time.Time{}
				if row.UpdatedAt.Valid {
					updatedAt = row.UpdatedAt.Time
				}
				return c.JSON(http.StatusOK, GetContainerResponse{
					ContainerID:   row.ContainerID,
					Image:         row.Image,
					Status:        row.Status,
					Namespace:     row.Namespace,
					HostPath:      hostPath,
					ContainerPath: row.ContainerPath,
					TaskRunning:   taskRunning,
					CreatedAt:     createdAt,
					UpdatedAt:     updatedAt,
				})
			}
		}
	}

	containerID, err := h.botContainerID(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "container not found for bot")
	}
	infoCtx := ctx
	if strings.TrimSpace(h.namespace) != "" {
		infoCtx = namespaces.WithNamespace(ctx, h.namespace)
	}
	container, err := h.service.GetContainer(infoCtx, containerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return echo.NewHTTPError(http.StatusNotFound, "container not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	info, err := container.Info(infoCtx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, GetContainerResponse{
		ContainerID: info.ID,
		Image:       info.Image,
		Status:      "unknown",
		Namespace:   h.namespace,
		TaskRunning: h.isTaskRunning(ctx, containerID),
		CreatedAt:   info.CreatedAt,
		UpdatedAt:   info.UpdatedAt,
	})
}

// DeleteContainer godoc
// @Summary Delete MCP container for bot
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Success 204
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container [delete]
func (h *ContainerdHandler) DeleteContainer(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	if err := h.CleanupBotContainer(c.Request().Context(), botID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// StartContainer godoc
// @Summary Start container task for bot
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} object
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/start [post]
func (h *ContainerdHandler) StartContainer(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	containerID, err := h.botContainerID(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "container not found for bot")
	}
	if err := h.ensureTaskRunning(ctx, containerID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if h.queries != nil {
		if pgBotID, parseErr := parsePgUUID(botID); parseErr == nil {
			_ = h.queries.UpdateContainerStarted(ctx, pgBotID)
		}
	}
	return c.JSON(http.StatusOK, map[string]bool{"started": true})
}

// StopContainer godoc
// @Summary Stop container task for bot
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} object
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/stop [post]
func (h *ContainerdHandler) StopContainer(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	containerID, err := h.botContainerID(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "container not found for bot")
	}
	if err := h.service.StopTask(ctx, containerID, &ctr.StopTaskOptions{
		Timeout: 10 * time.Second,
		Force:   true,
	}); err != nil && !errdefs.IsNotFound(err) {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	_ = h.service.DeleteTask(ctx, containerID, &ctr.DeleteTaskOptions{Force: true})
	if h.queries != nil {
		if pgBotID, parseErr := parsePgUUID(botID); parseErr == nil {
			_ = h.queries.UpdateContainerStopped(ctx, pgBotID)
		}
	}
	return c.JSON(http.StatusOK, map[string]bool{"stopped": true})
}

// CreateSnapshot godoc
// @Summary Create container snapshot for bot
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body CreateSnapshotRequest true "Create snapshot payload"
// @Success 200 {object} CreateSnapshotResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/snapshots [post]
func (h *ContainerdHandler) CreateSnapshot(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	var req CreateSnapshotRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	containerID, err := h.botContainerID(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "container not found for bot")
	}
	container, err := h.service.GetContainer(ctx, containerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return echo.NewHTTPError(http.StatusNotFound, "container not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	infoCtx := ctx
	if strings.TrimSpace(h.namespace) != "" {
		infoCtx = namespaces.WithNamespace(ctx, h.namespace)
	}
	info, err := container.Info(infoCtx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	snapshotName := strings.TrimSpace(req.SnapshotName)
	if snapshotName == "" {
		snapshotName = containerID + "-" + time.Now().Format("20060102150405")
	}
	if err := h.service.CommitSnapshot(ctx, info.Snapshotter, snapshotName, info.SnapshotKey); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, CreateSnapshotResponse{
		ContainerID:  containerID,
		SnapshotName: snapshotName,
		Snapshotter:  info.Snapshotter,
	})
}

// ListSnapshots godoc
// @Summary List snapshots
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param snapshotter query string false "Snapshotter name"
// @Success 200 {object} ListSnapshotsResponse
// @Router /bots/{bot_id}/container/snapshots [get]
func (h *ContainerdHandler) ListSnapshots(c echo.Context) error {
	if _, err := h.requireBotAccess(c); err != nil {
		return err
	}
	snapshotter := strings.TrimSpace(c.QueryParam("snapshotter"))
	if snapshotter == "" {
		snapshotter = strings.TrimSpace(h.cfg.Snapshotter)
	}
	if snapshotter == "" {
		snapshotter = "overlayfs"
	}
	snapshots, err := h.service.ListSnapshots(c.Request().Context(), snapshotter)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	items := make([]SnapshotInfo, 0, len(snapshots))
	for _, info := range snapshots {
		items = append(items, SnapshotInfo{
			Snapshotter: snapshotter,
			Name:        info.Name,
			Parent:      info.Parent,
			Kind:        info.Kind.String(),
			CreatedAt:   info.Created,
			UpdatedAt:   info.Updated,
			Labels:      info.Labels,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].Name < items[j].Name
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return c.JSON(http.StatusOK, ListSnapshotsResponse{
		Snapshotter: snapshotter,
		Snapshots:   items,
	})
}

// ---------- auth helpers ----------

// requireBotAccess extracts bot_id from path, validates user auth, and authorizes bot access.
func (h *ContainerdHandler) requireBotAccess(c echo.Context) (string, error) {
	userID, err := h.requireUserID(c)
	if err != nil {
		return "", err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return "", err
	}
	return botID, nil
}

func (h *ContainerdHandler) requireUserID(c echo.Context) (string, error) {
	userID, err := auth.UserIDFromContext(c)
	if err != nil {
		return "", err
	}
	if err := identity.ValidateUserID(userID); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return userID, nil
}

func (h *ContainerdHandler) authorizeBotAccess(ctx context.Context, actorID, botID string) (bots.Bot, error) {
	if h.botService == nil || h.userService == nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, "bot services not configured")
	}
	isAdmin, err := h.userService.IsAdmin(ctx, actorID)
	if err != nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	bot, err := h.botService.AuthorizeAccess(ctx, actorID, botID, isAdmin, bots.AccessPolicy{AllowPublicMember: false})
	if err != nil {
		if errors.Is(err, bots.ErrBotNotFound) {
			return bots.Bot{}, echo.NewHTTPError(http.StatusNotFound, "bot not found")
		}
		if errors.Is(err, bots.ErrBotAccessDenied) {
			return bots.Bot{}, echo.NewHTTPError(http.StatusForbidden, "bot access denied")
		}
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return bot, nil
}

// SetupBotContainer creates and starts the MCP container for a bot.
func (h *ContainerdHandler) SetupBotContainer(ctx context.Context, botID string) error {
	containerID := mcp.ContainerPrefix + botID

	image := strings.TrimSpace(h.cfg.BusyboxImage)
	if image == "" {
		image = config.DefaultBusyboxImg
	}
	snapshotter := strings.TrimSpace(h.cfg.Snapshotter)

	if strings.TrimSpace(h.namespace) != "" {
		ctx = namespaces.WithNamespace(ctx, h.namespace)
	}

	dataRoot := strings.TrimSpace(h.cfg.DataRoot)
	if dataRoot == "" {
		dataRoot = config.DefaultDataRoot
	}
	dataRoot, _ = filepath.Abs(dataRoot)
	dataMount := strings.TrimSpace(h.cfg.DataMount)
	if dataMount == "" {
		dataMount = config.DefaultDataMount
	}
	dataDir := filepath.Join(dataRoot, "bots", botID)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, ".skills"), 0o755); err != nil {
		return err
	}
	resolvPath, err := ctr.ResolveConfSource(dataDir)
	if err != nil {
		return err
	}

	specOpts := []oci.SpecOpts{
		oci.WithMounts([]specs.Mount{
			{
				Destination: dataMount,
				Type:        "bind",
				Source:      dataDir,
				Options:     []string{"rbind", "rw"},
			},
			{
				Destination: "/app",
				Type:        "bind",
				Source:      dataDir,
				Options:     []string{"rbind", "rw"},
			},
			{
				Destination: "/etc/resolv.conf",
				Type:        "bind",
				Source:      resolvPath,
				Options:     []string{"rbind", "ro"},
			},
		}),
		oci.WithProcessArgs("/bin/sh", "-lc", fmt.Sprintf("bootstrap(){ [ -e /app/mcp ] || { mkdir -p /app; [ -f /opt/mcp ] && cp -a /opt/mcp /app/mcp 2>/dev/null || true; }; if [ -d /opt/mcp-template ]; then mkdir -p %q; for f in /opt/mcp-template/*; do name=$(basename \"$f\"); [ -e %q/\"$name\" ] || cp -a \"$f\" %q/\"$name\" 2>/dev/null || true; done; fi; }; bootstrap; exec /app/mcp", dataMount, dataMount, dataMount)),
	}

	_, err = h.service.CreateContainer(ctx, ctr.CreateContainerRequest{
		ID:          containerID,
		ImageRef:    image,
		Snapshotter: snapshotter,
		Labels: map[string]string{
			mcp.BotLabelKey: botID,
		},
		SpecOpts: specOpts,
	})
	if err != nil && !errdefs.IsAlreadyExists(err) {
		return err
	}

	if h.queries != nil {
		pgBotID, parseErr := parsePgUUID(botID)
		if parseErr == nil {
			ns := strings.TrimSpace(h.namespace)
			if ns == "" {
				ns = "default"
			}
			_ = h.queries.UpsertContainer(ctx, dbsqlc.UpsertContainerParams{
				BotID:         pgBotID,
				ContainerID:   containerID,
				ContainerName: containerID,
				Image:         image,
				Status:        "created",
				Namespace:     ns,
				AutoStart:     true,
				HostPath:      pgtype.Text{String: dataDir, Valid: true},
				ContainerPath: dataMount,
			})
		}
	}

	if task, err := h.service.StartTask(ctx, containerID, &ctr.StartTaskOptions{
		UseStdio: false,
	}); err == nil {
		if netErr := ctr.SetupNetwork(ctx, task, containerID); netErr == nil {
			if h.queries != nil {
				if pgBotID, parseErr := parsePgUUID(botID); parseErr == nil {
					_ = h.queries.UpdateContainerStarted(ctx, pgBotID)
				}
			}
		} else {
			_ = h.service.StopTask(ctx, containerID, &ctr.StopTaskOptions{Force: true})
			h.logger.Error("setup bot container: network setup failed",
				slog.String("bot_id", botID),
				slog.String("container_id", containerID),
				slog.Any("error", netErr),
			)
		}
	} else {
		h.logger.Error("setup bot container: task start failed",
			slog.String("bot_id", botID),
			slog.String("container_id", containerID),
			slog.Any("error", err),
		)
	}
	return nil
}

// CleanupBotContainer removes the containerd container and DB record for a bot.
func (h *ContainerdHandler) CleanupBotContainer(ctx context.Context, botID string) error {
	containerID, err := h.botContainerID(ctx, botID)
	if err != nil {
		if h.queries != nil {
			if pgBotID, parseErr := parsePgUUID(botID); parseErr == nil {
				_ = h.queries.DeleteContainerByBotID(ctx, pgBotID)
			}
		}
		return nil
	}

	if task, taskErr := h.service.GetTask(ctx, containerID); taskErr == nil {
		_ = ctr.RemoveNetwork(ctx, task, containerID)
	}
	_ = h.service.StopTask(ctx, containerID, &ctr.StopTaskOptions{
		Timeout: 5 * time.Second,
		Force:   true,
	})
	_ = h.service.DeleteTask(ctx, containerID, &ctr.DeleteTaskOptions{Force: true})

	if err := h.service.DeleteContainer(ctx, containerID, &ctr.DeleteContainerOptions{
		CleanupSnapshot: true,
	}); err != nil && !errdefs.IsNotFound(err) {
		return err
	}

	if h.queries != nil {
		if pgBotID, parseErr := parsePgUUID(botID); parseErr == nil {
			_ = h.queries.DeleteContainerByBotID(ctx, pgBotID)
		}
	}
	return nil
}

func (h *ContainerdHandler) isTaskRunning(ctx context.Context, containerID string) bool {
	tasks, err := h.service.ListTasks(ctx, &ctr.ListTasksOptions{
		Filter: "container.id==" + containerID,
	})
	return err == nil && len(tasks) > 0 && tasks[0].Status == tasktypes.Status_RUNNING
}

func parsePgUUID(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return pgtype.UUID{}, err
	}
	var pgID pgtype.UUID
	pgID.Valid = true
	copy(pgID.Bytes[:], parsed[:])
	return pgID, nil
}
