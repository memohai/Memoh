package handlers

import (
	"context"
	"errors"
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
	service     ctr.Service
	cfg         config.MCPConfig
	namespace   string
	logger      *slog.Logger
	mcpMu       sync.Mutex
	mcpSess     map[string]*mcpSession
	botService  *bots.Service
	userService *users.Service
	queries     *dbsqlc.Queries
}

type CreateContainerRequest struct {
	ContainerID string `json:"container_id"`
	Image       string `json:"image,omitempty"`
	Snapshotter string `json:"snapshotter,omitempty"`
}

type CreateContainerResponse struct {
	ContainerID string `json:"container_id"`
	Image       string `json:"image"`
	Snapshotter string `json:"snapshotter"`
	Started     bool   `json:"started"`
}

type CreateSnapshotRequest struct {
	ContainerID  string `json:"container_id"`
	SnapshotName string `json:"snapshot_name"`
}

type CreateSnapshotResponse struct {
	ContainerID  string `json:"container_id"`
	SnapshotName string `json:"snapshot_name"`
	Snapshotter  string `json:"snapshotter"`
}

type ContainerInfo struct {
	ID          string            `json:"id"`
	Image       string            `json:"image,omitempty"`
	Snapshotter string            `json:"snapshotter,omitempty"`
	SnapshotKey string            `json:"snapshot_key,omitempty"`
	CreatedAt   time.Time         `json:"created_at,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type ListContainersResponse struct {
	Containers []ContainerInfo `json:"containers"`
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

func NewContainerdHandler(log *slog.Logger, service ctr.Service, cfg config.MCPConfig, namespace string, botService *bots.Service, userService *users.Service, queries *dbsqlc.Queries) *ContainerdHandler {
	return &ContainerdHandler{
		service:     service,
		cfg:         cfg,
		namespace:   namespace,
		logger:      log.With(slog.String("handler", "containerd")),
		mcpSess:     make(map[string]*mcpSession),
		botService:  botService,
		userService: userService,
		queries:     queries,
	}
}

func (h *ContainerdHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id/container")
	group.POST("", h.CreateContainer)
	group.GET("/list", h.ListContainers)
	group.DELETE("/:id", h.DeleteContainer)
	group.POST("/snapshots", h.CreateSnapshot)
	group.GET("/snapshots", h.ListSnapshots)
	group.GET("/skills", h.ListSkills)
	group.POST("/skills", h.UpsertSkills)
	group.DELETE("/skills", h.DeleteSkills)
	group.POST("/fs/:id", h.HandleMCPFS)
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
	req.ContainerID = strings.TrimSpace(req.ContainerID)
	if req.ContainerID == "" {
		req.ContainerID = "mcp-" + botID
	}

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
		}),
		oci.WithProcessArgs("/bin/sh", "-lc", "sleep 2147483647"),
	}

	_, err = h.service.CreateContainer(ctx, ctr.CreateContainerRequest{
		ID:          req.ContainerID,
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

	// Persist container record in database
	if h.queries != nil {
		pgBotID, parseErr := parsePgUUID(botID)
		if parseErr == nil {
			ns := strings.TrimSpace(h.namespace)
			if ns == "" {
				ns = "default"
			}
			_ = h.queries.UpsertContainer(c.Request().Context(), dbsqlc.UpsertContainerParams{
				BotID:         pgBotID,
				ContainerID:   req.ContainerID,
				ContainerName: req.ContainerID,
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
	fifoDir, err := h.taskFIFODir()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if _, err := h.service.StartTask(c.Request().Context(), req.ContainerID, &ctr.StartTaskOptions{
		UseStdio: false,
		FIFODir:  fifoDir,
	}); err == nil {
		started = true
	} else {
		h.logger.Error("mcp container start failed",
			slog.String("container_id", req.ContainerID),
			slog.Any("error", err),
		)
	}

	return c.JSON(http.StatusOK, CreateContainerResponse{
		ContainerID: req.ContainerID,
		Image:       image,
		Snapshotter: snapshotter,
		Started:     started,
	})
}

func (h *ContainerdHandler) taskFIFODir() (string, error) {
	if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
		fifoDir := filepath.Join(homeDir, ".memoh", "containerd-fifo")
		if err := os.MkdirAll(fifoDir, 0o755); err != nil {
			return "", err
		}
		return fifoDir, nil
	}
	fifoDir := "/tmp/memoh-containerd-fifo"
	if err := os.MkdirAll(fifoDir, 0o755); err != nil {
		return "", err
	}
	return fifoDir, nil
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

	fifoDir, err := h.taskFIFODir()
	if err != nil {
		return err
	}
	_, err = h.service.StartTask(ctx, containerID, &ctr.StartTaskOptions{
		UseStdio: false,
		FIFODir:  fifoDir,
	})
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

// ListContainers godoc
// @Summary List containers for bot
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} ListContainersResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/list [get]
func (h *ContainerdHandler) ListContainers(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	containers, err := h.service.ListContainersByLabel(ctx, mcp.BotLabelKey, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	infoCtx := ctx
	if strings.TrimSpace(h.namespace) != "" {
		infoCtx = namespaces.WithNamespace(ctx, h.namespace)
	}
	items := make([]ContainerInfo, 0, len(containers))
	for _, container := range containers {
		info, err := container.Info(infoCtx)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		items = append(items, ContainerInfo{
			ID:          info.ID,
			Image:       info.Image,
			Snapshotter: info.Snapshotter,
			SnapshotKey: info.SnapshotKey,
			CreatedAt:   info.CreatedAt,
			UpdatedAt:   info.UpdatedAt,
			Labels:      info.Labels,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	return c.JSON(http.StatusOK, ListContainersResponse{Containers: items})
}

// DeleteContainer godoc
// @Summary Delete MCP container
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Container ID"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/{id} [delete]
func (h *ContainerdHandler) DeleteContainer(c echo.Context) error {
	if _, err := h.requireBotAccess(c); err != nil {
		return err
	}
	containerID := strings.TrimSpace(c.Param("id"))
	if containerID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "container id is required")
	}
	_ = h.service.DeleteTask(c.Request().Context(), containerID, &ctr.DeleteTaskOptions{Force: true})
	if err := h.service.DeleteContainer(c.Request().Context(), containerID, &ctr.DeleteContainerOptions{
		CleanupSnapshot: true,
	}); err != nil {
		if errdefs.IsNotFound(err) {
			return echo.NewHTTPError(http.StatusNotFound, "container not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// CreateSnapshot godoc
// @Summary Create container snapshot
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body CreateSnapshotRequest true "Create snapshot payload"
// @Success 200 {object} CreateSnapshotResponse
// @Router /bots/{bot_id}/container/snapshots [post]
func (h *ContainerdHandler) CreateSnapshot(c echo.Context) error {
	if _, err := h.requireBotAccess(c); err != nil {
		return err
	}
	var req CreateSnapshotRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if strings.TrimSpace(req.ContainerID) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "container_id is required")
	}
	container, err := h.service.GetContainer(c.Request().Context(), req.ContainerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return echo.NewHTTPError(http.StatusNotFound, "container not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	ctx := c.Request().Context()
	if strings.TrimSpace(h.namespace) != "" {
		ctx = namespaces.WithNamespace(ctx, h.namespace)
	}
	info, err := container.Info(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	snapshotName := strings.TrimSpace(req.SnapshotName)
	if snapshotName == "" {
		snapshotName = req.ContainerID + "-" + time.Now().Format("20060102150405")
	}
	if err := h.service.CommitSnapshot(c.Request().Context(), info.Snapshotter, snapshotName, info.SnapshotKey); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, CreateSnapshotResponse{
		ContainerID:  req.ContainerID,
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
