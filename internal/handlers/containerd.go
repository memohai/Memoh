package handlers

import (
	"context"
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
	"github.com/labstack/echo/v4"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/containerd"
	"github.com/memohai/memoh/internal/logger"
	"github.com/memohai/memoh/internal/mcp"
)

type ContainerdHandler struct {
	service   ctr.Service
	cfg       config.MCPConfig
	namespace string
	mcpMu     sync.Mutex
	mcpSess   map[string]*mcpSession
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

func NewContainerdHandler(service ctr.Service, cfg config.MCPConfig, namespace string) *ContainerdHandler {
	return &ContainerdHandler{
		service:   service,
		cfg:       cfg,
		namespace: namespace,
		mcpSess:   make(map[string]*mcpSession),
	}
}

func (h *ContainerdHandler) Register(e *echo.Echo) {
	group := e.Group("/mcp")
	group.POST("/containers", h.CreateContainer)
	group.GET("/containers", h.ListContainers)
	group.DELETE("/containers/:id", h.DeleteContainer)
	group.POST("/snapshots", h.CreateSnapshot)
	group.GET("/snapshots", h.ListSnapshots)
	group.GET("/skills", h.ListSkills)
	group.POST("/skills", h.UpsertSkills)
	group.DELETE("/skills", h.DeleteSkills)
	group.POST("/fs/:id", h.HandleMCPFS)
}

// CreateContainer godoc
// @Summary Create and start MCP container
// @Tags containerd
// @Param payload body CreateContainerRequest true "Create container payload"
// @Success 200 {object} CreateContainerResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /mcp/containers [post]
func (h *ContainerdHandler) CreateContainer(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}

	var req CreateContainerRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req.ContainerID = strings.TrimSpace(req.ContainerID)
	if req.ContainerID == "" {
		req.ContainerID = uuid.NewString()
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
	dataDir := filepath.Join(dataRoot, "users", userID)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := os.MkdirAll(filepath.Join(dataDir, ".skills"), 0o755); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	specOpts := []oci.SpecOpts{
		oci.WithMounts([]specs.Mount{{
			Destination: dataMount,
			Type:        "bind",
			Source:      dataDir,
			Options:     []string{"rbind", "rw"},
		}}),
		oci.WithProcessArgs("/bin/sh", "-lc", "sleep 2147483647"),
	}

	_, err = h.service.CreateContainer(ctx, ctr.CreateContainerRequest{
		ID:          req.ContainerID,
		ImageRef:    image,
		Snapshotter: snapshotter,
		Labels: map[string]string{
			mcp.UserLabelKey: userID,
		},
		SpecOpts: specOpts,
	})
	if err != nil && !errdefs.IsAlreadyExists(err) {
		return echo.NewHTTPError(http.StatusInternalServerError, "snapshotter="+snapshotter+" image="+image+" err="+err.Error())
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
		logger.FromContext(c.Request().Context()).Error("mcp container start failed",
			"container_id", req.ContainerID,
			"error", err,
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

func (h *ContainerdHandler) userContainerID(ctx context.Context, userID string) (string, error) {
	containers, err := h.service.ListContainersByLabel(ctx, mcp.UserLabelKey, userID)
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
// @Summary List containers
// @Tags containerd
// @Success 200 {object} ListContainersResponse
// @Failure 500 {object} ErrorResponse
// @Router /mcp/containers [get]
func (h *ContainerdHandler) ListContainers(c echo.Context) error {
	ctx := c.Request().Context()
	containers, err := h.service.ListContainers(ctx)
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
// @Param id path string true "Container ID"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /mcp/containers/{id} [delete]
func (h *ContainerdHandler) DeleteContainer(c echo.Context) error {
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
// @Param payload body CreateSnapshotRequest true "Create snapshot payload"
// @Success 200 {object} CreateSnapshotResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /mcp/snapshots [post]
func (h *ContainerdHandler) CreateSnapshot(c echo.Context) error {
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
// @Param snapshotter query string false "Snapshotter name"
// @Success 200 {object} ListSnapshotsResponse
// @Failure 500 {object} ErrorResponse
// @Router /mcp/snapshots [get]
func (h *ContainerdHandler) ListSnapshots(c echo.Context) error {
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
