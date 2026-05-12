package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/container"
	displaypkg "github.com/memohai/memoh/internal/display"
	"github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/policy"
	"github.com/memohai/memoh/internal/workspace"
)

type ContainerdHandler struct {
	manager          containerWorkspace
	cfg              config.WorkspaceConfig
	containerBackend string
	logger           *slog.Logger
	toolGateway      *mcp.ToolGatewayService
	mcpSess          map[string]*mcpSession
	mcpStdioMu       sync.Mutex
	mcpStdioSess     map[string]*mcpStdioSession
	botService       *bots.Service
	accountService   *accounts.Service
	policyService    *policy.Service
	displayService   *displaypkg.Service
}

type ContainerGPURequest struct {
	Devices []string `json:"devices,omitempty"`
}

type CreateContainerRequest struct {
	Snapshotter        string               `json:"snapshotter,omitempty"`
	RestoreData        bool                 `json:"restore_data,omitempty"`
	Image              string               `json:"image,omitempty"`
	WorkspaceBackend   string               `json:"workspace_backend,omitempty"`
	LocalWorkspacePath string               `json:"local_workspace_path,omitempty"`
	GPU                *ContainerGPURequest `json:"gpu,omitempty"`
}

type CreateContainerResponse struct {
	ContainerID      string   `json:"container_id"`
	WorkspaceBackend string   `json:"workspace_backend"`
	ContainerPath    string   `json:"container_path"`
	Image            string   `json:"image"`
	Snapshotter      string   `json:"snapshotter"`
	CDIDevices       []string `json:"cdi_devices,omitempty"`
	Started          bool     `json:"started"`
	DataRestored     bool     `json:"data_restored"`
	HasPreservedData bool     `json:"has_preserved_data"`
}

// codesync(container-create-stream): keep these SSE payloads in sync with
// packages/sdk/src/container-stream.ts.
type createContainerPullingEvent struct {
	Type  string `json:"type"`
	Image string `json:"image"`
}

type createContainerPullProgressEvent struct {
	Type   string            `json:"type"`
	Layers []ctr.LayerStatus `json:"layers"`
}

type createContainerPullStatusEvent struct {
	Type    string `json:"type"`
	Image   string `json:"image"`
	Message string `json:"message,omitempty"`
}

type createContainerCreatingEvent struct {
	Type string `json:"type"`
}

type createContainerCompleteEvent struct {
	Type      string                  `json:"type"`
	Container CreateContainerResponse `json:"container"`
}

type createContainerRestoringEvent struct {
	Type string `json:"type"`
}

type createContainerErrorEvent struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type GetContainerResponse struct {
	ContainerID      string    `json:"container_id"`
	WorkspaceBackend string    `json:"workspace_backend"`
	Image            string    `json:"image"`
	Status           string    `json:"status"`
	Namespace        string    `json:"namespace"`
	ContainerPath    string    `json:"container_path"`
	CDIDevices       []string  `json:"cdi_devices,omitempty"`
	TaskRunning      bool      `json:"task_running"`
	HasPreservedData bool      `json:"has_preserved_data"`
	Legacy           bool      `json:"legacy"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ContainerMetricsStatusResponse struct {
	Exists      bool `json:"exists"`
	TaskRunning bool `json:"task_running"`
}

type ContainerCPUMetricsResponse struct {
	UsagePercent      float64 `json:"usage_percent"`
	UsageNanoseconds  uint64  `json:"usage_nanoseconds"`
	UsageNanocores    uint64  `json:"usage_nanocores"`
	UserNanoseconds   uint64  `json:"user_nanoseconds"`
	KernelNanoseconds uint64  `json:"kernel_nanoseconds"`
}

type ContainerMemoryMetricsResponse struct {
	UsageBytes   uint64  `json:"usage_bytes"`
	LimitBytes   uint64  `json:"limit_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}

type ContainerStorageMetricsResponse struct {
	Path      string `json:"path"`
	UsedBytes uint64 `json:"used_bytes"`
}

type ContainerMetricsPayloadResponse struct {
	CPU     *ContainerCPUMetricsResponse     `json:"cpu,omitempty"`
	Memory  *ContainerMemoryMetricsResponse  `json:"memory,omitempty"`
	Storage *ContainerStorageMetricsResponse `json:"storage,omitempty"`
}

type GetContainerMetricsResponse struct {
	Supported         bool                            `json:"supported"`
	Backend           string                          `json:"backend"`
	UnsupportedReason string                          `json:"unsupported_reason,omitempty"`
	Status            ContainerMetricsStatusResponse  `json:"status"`
	Metrics           ContainerMetricsPayloadResponse `json:"metrics"`
	SampledAt         *time.Time                      `json:"sampled_at,omitempty"`
}

type RollbackRequest struct {
	Version int `json:"version"`
}

type CreateSnapshotRequest struct {
	SnapshotName string `json:"snapshot_name"`
}

type CreateSnapshotResponse struct {
	ContainerID         string `json:"container_id"`
	SnapshotName        string `json:"snapshot_name"`
	RuntimeSnapshotName string `json:"runtime_snapshot_name"`
	DisplayName         string `json:"display_name"`
	Snapshotter         string `json:"snapshotter"`
	Version             int    `json:"version"`
	Source              string `json:"source"`
}

type SnapshotInfo struct {
	Snapshotter string            `json:"snapshotter"`
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name,omitempty"`
	RuntimeName string            `json:"runtime_snapshot_name"`
	Parent      string            `json:"parent,omitempty"`
	Kind        string            `json:"kind"`
	CreatedAt   time.Time         `json:"created_at,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Source      string            `json:"source"`
	Managed     bool              `json:"managed"`
	Version     *int              `json:"version,omitempty"`
}

type ListSnapshotsResponse struct {
	Snapshotter string         `json:"snapshotter"`
	Snapshots   []SnapshotInfo `json:"snapshots"`
}

func NewContainerdHandler(log *slog.Logger, manager containerWorkspace, cfg config.WorkspaceConfig, containerBackend string, botService *bots.Service, accountService *accounts.Service, policyService *policy.Service) *ContainerdHandler {
	h := &ContainerdHandler{
		manager:          manager,
		cfg:              cfg,
		containerBackend: containerBackend,
		logger:           log.With(slog.String("handler", "containerd")),
		mcpSess:          make(map[string]*mcpSession),
		mcpStdioSess:     make(map[string]*mcpStdioSession),
		botService:       botService,
		accountService:   accountService,
		policyService:    policyService,
	}
	h.displayService = displaypkg.NewService(h.logger, manager)
	return h
}

func (h *ContainerdHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id/container")
	group.POST("", h.CreateContainer)
	group.GET("", h.GetContainer)
	group.GET("/metrics", h.GetContainerMetrics)
	group.DELETE("", h.DeleteContainer)
	group.POST("/start", h.StartContainer)
	group.POST("/stop", h.StopContainer)
	group.POST("/snapshots", h.CreateSnapshot)
	group.GET("/snapshots", h.ListSnapshots)
	group.POST("/snapshots/rollback", h.RollbackSnapshot)
	group.POST("/data/export", h.ExportContainerData)
	group.POST("/data/import", h.ImportContainerData)
	group.POST("/data/restore", h.RestorePreservedData)
	group.GET("/skills", h.ListSkills)
	group.POST("/skills", h.UpsertSkills)
	group.DELETE("/skills", h.DeleteSkills)
	group.POST("/skills/actions", h.ApplySkillAction)
	// Terminal routes
	group.GET("/terminal", h.GetTerminalInfo)
	group.GET("/terminal/ws", h.HandleTerminalWS)
	// Display routes
	group.GET("/display", h.GetDisplayInfo)
	group.POST("/display/prepare", h.PrepareDisplay)
	group.GET("/display/sessions", h.ListDisplaySessions)
	group.DELETE("/display/sessions/:session_id", h.CloseDisplaySession)
	group.POST("/display/webrtc/offer", h.HandleDisplayWebRTCOffer)
	// File manager routes
	group.GET("/fs", h.FSStat)
	group.GET("/fs/list", h.FSList)
	group.GET("/fs/read", h.FSRead)
	group.GET("/fs/download", h.FSDownload)
	group.POST("/fs/write", h.FSWrite)
	group.POST("/fs/upload", h.FSUpload)
	group.POST("/fs/mkdir", h.FSMkdir)
	group.POST("/fs/delete", h.FSDelete)
	group.POST("/fs/rename", h.FSRename)
	root := e.Group("/bots/:bot_id")
	root.POST("/mcp-stdio", h.CreateMCPStdio)
	root.POST("/mcp-stdio/:connection_id", h.HandleMCPStdio)
	root.POST("/tools", h.HandleMCPTools)
}

// CreateContainer godoc
// @Summary Create and start MCP container for bot
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body CreateContainerRequest true "Create container payload"
// @Success 200 {object} CreateContainerResponse "SSE stream of container creation events"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container [post].
func (h *ContainerdHandler) CreateContainer(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	var req CreateContainerRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// Image override lets administrators specify a custom base image.
	// NOTE(saas): if this becomes a multi-tenant SaaS, image override must be
	// validated against an allowlist to prevent SSRF and resource abuse.
	ctx := c.Request().Context()
	imageOverride := strings.TrimSpace(req.Image)
	image, err := h.manager.ResolveWorkspaceImage(ctx, botID)
	if err != nil {
		h.logger.Error("resolve workspace image failed",
			slog.String("bot_id", botID), slog.Any("error", err))
		return nil
	}
	gpu, err := h.manager.ResolveWorkspaceGPU(ctx, botID)
	if err != nil {
		h.logger.Error("resolve workspace gpu failed",
			slog.String("bot_id", botID), slog.Any("error", err))
		return nil
	}
	if imageOverride != "" {
		image = config.NormalizeImageRef(imageOverride)
	}
	if req.GPU != nil {
		gpu = workspace.WorkspaceGPUConfig{Devices: req.GPU.Devices}
	}

	snapshotter := strings.TrimSpace(req.Snapshotter)
	if snapshotter == "" {
		snapshotter = h.cfg.Snapshotter
	}

	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set(echo.HeaderConnection, "keep-alive")
	c.Response().WriteHeader(http.StatusOK)
	writer := bufio.NewWriter(c.Response().Writer)

	var mu sync.Mutex
	send := func(payload any) {
		mu.Lock()
		defer mu.Unlock()
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}
		_ = writeSSEData(writer, flusher, string(data))
	}

	sendError := func(msg string) {
		send(createContainerErrorEvent{Type: "error", Message: msg})
	}

	workspaceBackend := strings.ToLower(strings.TrimSpace(req.WorkspaceBackend))
	if workspaceBackend == "" {
		workspaceBackend = "container"
	}
	if workspaceBackend == "local" {
		image = "local"
		send(createContainerPullStatusEvent{Type: "pull_skipped", Image: image, Message: "local workspace does not use container images"})
	} else {
		// Phase 1: Pull image with progress
		send(createContainerPullingEvent{Type: "pulling", Image: image})

		var pullDone atomic.Bool
		prepareResult, pullErr := h.manager.PrepareImageForCreate(ctx, image, &ctr.PullImageOptions{
			Unpack:        true,
			StorageDriver: snapshotter,
			OnProgress: func(p ctr.PullProgress) {
				if pullDone.Load() {
					return
				}
				send(createContainerPullProgressEvent{Type: "pull_progress", Layers: p.Layers})
			},
		})
		pullDone.Store(true)
		if pullErr != nil {
			h.logger.Error("image preparation failed",
				slog.String("image", image), slog.Any("error", pullErr))
			sendError("image preparation failed: " + pullErr.Error())
			return nil
		}
		switch prepareResult.Mode {
		case workspace.ImagePrepareSkipped:
			send(createContainerPullStatusEvent{Type: "pull_skipped", Image: image, Message: prepareResult.Message})
		case workspace.ImagePrepareDelegated:
			send(createContainerPullStatusEvent{Type: "pull_delegated", Image: image, Message: prepareResult.Message})
		}
	}

	// Phase 2: Create container (image is local, should be fast)
	send(createContainerCreatingEvent{Type: "creating"})

	// Notify the client before starting if data migration will happen,
	// since restoring a large /data volume can take a while.
	if h.manager.HasPreservedData(botID) {
		send(createContainerRestoringEvent{Type: "restoring"})
	}

	if err := h.manager.StartWithWorkspaceConfig(ctx, botID, image, gpu, workspace.WorkspaceStartConfig{
		Backend:            workspaceBackend,
		LocalWorkspacePath: strings.TrimSpace(req.LocalWorkspacePath),
	}); err != nil {
		h.logger.Error("container start failed",
			slog.String("bot_id", botID), slog.Any("error", err))
		sendError("container start failed: " + err.Error())
		return nil
	}
	if err := h.manager.RememberWorkspaceImage(ctx, botID, image); err != nil {
		h.logger.Warn("remember workspace image failed",
			slog.String("bot_id", botID), slog.String("image", image), slog.Any("error", err))
	}
	if req.GPU != nil {
		if err := h.manager.RememberWorkspaceGPU(ctx, botID, gpu); err != nil {
			h.logger.Warn("remember workspace gpu failed",
				slog.String("bot_id", botID), slog.Any("error", err))
		}
	}

	containerID, err := h.manager.ContainerID(ctx, botID)
	if err != nil {
		h.logger.Error("container ID resolution failed after start",
			slog.String("bot_id", botID), slog.Any("error", err))
		sendError("container ID resolution failed: " + err.Error())
		return nil
	}

	dataRestored := false
	if req.RestoreData && h.manager.HasPreservedData(botID) {
		if err := h.manager.RestorePreservedData(ctx, botID); err != nil {
			h.logger.Error("restore preserved data failed",
				slog.String("bot_id", botID), slog.Any("error", err))
			sendError("restore preserved data failed: " + err.Error())
			return nil
		}
		dataRestored = true
	}

	h.manager.RecordContainerRunning(ctx, botID, containerID, image)

	status, statusErr := h.manager.GetContainerInfo(ctx, botID)
	if statusErr != nil {
		h.logger.Warn("load container status after start failed",
			slog.String("bot_id", botID), slog.Any("error", statusErr))
	}
	cdiDevices := gpu.Devices
	containerPath := ""
	responseBackend := workspaceBackend
	if status != nil {
		cdiDevices = status.CDIDevices
		containerPath = status.ContainerPath
		responseBackend = status.WorkspaceBackend
	}

	// Phase 3: Complete
	send(createContainerCompleteEvent{
		Type: "complete",
		Container: CreateContainerResponse{
			ContainerID:      containerID,
			WorkspaceBackend: responseBackend,
			ContainerPath:    containerPath,
			Image:            image,
			Snapshotter:      snapshotter,
			CDIDevices:       cdiDevices,
			Started:          true,
			DataRestored:     dataRestored,
			HasPreservedData: h.manager.HasPreservedData(botID),
		},
	})

	return nil
}

// GetContainer godoc
// @Summary Get container info for bot
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} GetContainerResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container [get].
func (h *ContainerdHandler) GetContainer(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	status, err := h.manager.GetContainerInfo(c.Request().Context(), botID)
	if err != nil {
		if errors.Is(err, workspace.ErrContainerNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "container not found for bot")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, GetContainerResponse{
		ContainerID:      status.ContainerID,
		WorkspaceBackend: status.WorkspaceBackend,
		Image:            status.Image,
		Status:           status.Status,
		Namespace:        status.Namespace,
		ContainerPath:    status.ContainerPath,
		CDIDevices:       status.CDIDevices,
		TaskRunning:      status.TaskRunning,
		HasPreservedData: status.HasPreservedData,
		Legacy:           status.Legacy,
		CreatedAt:        status.CreatedAt,
		UpdatedAt:        status.UpdatedAt,
	})
}

// GetContainerMetrics godoc
// @Summary Get current container metrics for bot
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} GetContainerMetricsResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/metrics [get].
func (h *ContainerdHandler) GetContainerMetrics(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	metrics, err := h.manager.GetContainerMetrics(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	response := GetContainerMetricsResponse{
		Supported:         metrics.Supported,
		Backend:           h.containerBackend,
		UnsupportedReason: metrics.UnsupportedReason,
		Status: ContainerMetricsStatusResponse{
			Exists:      metrics.Status.Exists,
			TaskRunning: metrics.Status.TaskRunning,
		},
		Metrics: ContainerMetricsPayloadResponse{
			CPU:     toContainerCPUMetricsResponse(metrics.CPU),
			Memory:  toContainerMemoryMetricsResponse(metrics.Memory),
			Storage: toContainerStorageMetricsResponse(metrics.Storage),
		},
	}
	if !metrics.SampledAt.IsZero() {
		sampledAt := metrics.SampledAt
		response.SampledAt = &sampledAt
	}

	return c.JSON(http.StatusOK, response)
}

// DeleteContainer godoc
// @Summary Delete MCP container for bot
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param preserve_data query bool false "Export /data before deletion"
// @Success 204
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container [delete].
func (h *ContainerdHandler) DeleteContainer(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	preserveData := c.QueryParam("preserve_data") == "true"
	if err := h.manager.CleanupBotContainer(c.Request().Context(), botID, preserveData); err != nil {
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
// @Router /bots/{bot_id}/container/start [post].
func (h *ContainerdHandler) StartContainer(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	if err := h.manager.EnsureRunning(c.Request().Context(), botID); err != nil {
		if errors.Is(err, workspace.ErrContainerNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "container not found for bot")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
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
// @Router /bots/{bot_id}/container/stop [post].
func (h *ContainerdHandler) StopContainer(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	if err := h.manager.StopBot(c.Request().Context(), botID); err != nil {
		if errors.Is(err, workspace.ErrContainerNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "container not found for bot")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
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
// @Failure 501 {object} ErrorResponse "Snapshots currently not supported on this backend"
// @Router /bots/{bot_id}/container/snapshots [post].
func (h *ContainerdHandler) CreateSnapshot(c echo.Context) error {
	if h.containerBackend == "apple" {
		return echo.NewHTTPError(http.StatusNotImplemented, "snapshots currently not supported on Apple Container backend")
	}
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	if h.manager == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "snapshot manager not configured")
	}
	var req CreateSnapshotRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	created, err := h.manager.CreateSnapshot(c.Request().Context(), botID, req.SnapshotName, workspace.SnapshotSourceManual)
	if err != nil {
		if ctr.IsNotFound(err) {
			return echo.NewHTTPError(http.StatusNotFound, "container not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, CreateSnapshotResponse{
		ContainerID:         created.ContainerID,
		SnapshotName:        created.SnapshotName,
		RuntimeSnapshotName: created.RuntimeSnapshotName,
		DisplayName:         created.DisplayName,
		Snapshotter:         created.Snapshotter,
		Version:             created.Version,
		Source:              workspace.SnapshotSourceManual,
	})
}

// ListSnapshots godoc
// @Summary List snapshots
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param snapshotter query string false "Snapshotter name"
// @Success 200 {object} ListSnapshotsResponse
// @Failure 501 {object} ErrorResponse "Snapshots currently not supported on this backend"
// @Router /bots/{bot_id}/container/snapshots [get].
func (h *ContainerdHandler) ListSnapshots(c echo.Context) error {
	if h.containerBackend == "apple" {
		return echo.NewHTTPError(http.StatusNotImplemented, "snapshots currently not supported on Apple Container backend")
	}
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	if h.manager == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "snapshot manager not configured")
	}

	data, err := h.manager.ListBotSnapshotData(c.Request().Context(), botID)
	if err != nil {
		if ctr.IsNotFound(err) {
			return echo.NewHTTPError(http.StatusNotFound, "container not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if req := strings.TrimSpace(c.QueryParam("snapshotter")); req != "" && req != data.Snapshotter {
		return echo.NewHTTPError(http.StatusBadRequest, "snapshotter does not match container snapshotter")
	}

	snapshotKey := strings.TrimSpace(data.Info.StorageRef.Key)

	resp, ok := buildSnapshotListResponse(data)
	if !ok {
		h.logger.Warn("container snapshot chain root not found",
			slog.String("container_id", data.ContainerID),
			slog.String("snapshotter", data.Snapshotter),
			slog.String("snapshot_key", snapshotKey),
		)
		return echo.NewHTTPError(http.StatusInternalServerError, "container snapshot chain not found")
	}
	return c.JSON(http.StatusOK, resp)
}

// RollbackSnapshot godoc
// @Summary Rollback container to a previous snapshot version
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body RollbackRequest true "Rollback payload"
// @Success 200 {object} object
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/snapshots/rollback [post].
func (h *ContainerdHandler) RollbackSnapshot(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	if h.manager == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "manager not configured")
	}

	var req RollbackRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.Version < 1 {
		return echo.NewHTTPError(http.StatusBadRequest, "version must be >= 1")
	}

	if err := h.manager.RollbackVersion(c.Request().Context(), botID, req.Version); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"rolled_back_to": req.Version})
}

// ExportContainerData godoc
// @Summary Export container /data as a tar.gz archive
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Produce application/gzip
// @Success 200 {file} file
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/data/export [post].
func (h *ContainerdHandler) ExportContainerData(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	if h.manager == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "manager not configured")
	}

	reader, err := h.manager.ExportData(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer func() { _ = reader.Close() }()

	c.Response().Header().Set("Content-Type", "application/gzip")
	c.Response().Header().Set("Content-Disposition", `attachment; filename="`+botID+`-data.tar.gz"`)
	c.Response().WriteHeader(http.StatusOK)
	_, err = io.Copy(c.Response(), reader)
	return err
}

// ImportContainerData godoc
// @Summary Import a tar.gz archive into container /data
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Accept multipart/form-data
// @Param file formData file true "tar.gz archive"
// @Success 200 {object} object
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/data/import [post].
func (h *ContainerdHandler) ImportContainerData(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	if h.manager == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "manager not configured")
	}

	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file is required")
	}
	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to open uploaded file")
	}
	defer func() { _ = src.Close() }()

	if err := h.manager.ImportData(c.Request().Context(), botID, src); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]bool{"imported": true})
}

// RestorePreservedData godoc
// @Summary Restore previously preserved data into container
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} object
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/data/restore [post].
func (h *ContainerdHandler) RestorePreservedData(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	if h.manager == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "manager not configured")
	}

	if !h.manager.HasPreservedData(botID) {
		return echo.NewHTTPError(http.StatusNotFound, "no preserved data found")
	}

	if err := h.manager.RestorePreservedData(c.Request().Context(), botID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]bool{"restored": true})
}

func toContainerCPUMetricsResponse(metrics *ctr.CPUMetrics) *ContainerCPUMetricsResponse {
	if metrics == nil {
		return nil
	}
	return &ContainerCPUMetricsResponse{
		UsagePercent:      metrics.UsagePercent,
		UsageNanoseconds:  metrics.UsageNanoseconds,
		UsageNanocores:    metrics.UsageNanocores,
		UserNanoseconds:   metrics.UserNanoseconds,
		KernelNanoseconds: metrics.KernelNanoseconds,
	}
}

func toContainerMemoryMetricsResponse(metrics *ctr.MemoryMetrics) *ContainerMemoryMetricsResponse {
	if metrics == nil {
		return nil
	}
	return &ContainerMemoryMetricsResponse{
		UsageBytes:   metrics.UsageBytes,
		LimitBytes:   metrics.LimitBytes,
		UsagePercent: metrics.UsagePercent,
	}
}

func toContainerStorageMetricsResponse(metrics *workspace.ContainerStorageMetrics) *ContainerStorageMetricsResponse {
	if metrics == nil {
		return nil
	}
	return &ContainerStorageMetricsResponse{
		Path:      metrics.Path,
		UsedBytes: metrics.UsedBytes,
	}
}

func buildSnapshotListResponse(data *workspace.BotSnapshotData) (ListSnapshotsResponse, bool) {
	if data == nil {
		return ListSnapshotsResponse{}, false
	}

	snapshotter := strings.TrimSpace(data.Snapshotter)
	runtimeByName := make(map[string]ctr.SnapshotInfo, len(data.RuntimeSnapshots))
	for _, info := range data.RuntimeSnapshots {
		name := strings.TrimSpace(info.Name)
		if name == "" {
			continue
		}
		runtimeByName[name] = info
	}

	snapshotKey := strings.TrimSpace(data.Info.StorageRef.Key)
	lineage, ok := snapshotLineage(snapshotKey, data.RuntimeSnapshots)
	if !ok {
		if snapshotLineageRootRequired(snapshotter) {
			return ListSnapshotsResponse{}, false
		}
		lineage = nil
	}

	items := make([]SnapshotInfo, 0, len(lineage)+len(data.ManagedMeta))
	seen := make(map[string]struct{}, len(lineage)+len(data.ManagedMeta))
	appendRuntime := func(runtimeInfo ctr.SnapshotInfo, fallbackSource string, meta *workspace.ManagedSnapshotMeta) {
		source := fallbackSource
		managed := false
		var version *int
		displayName := ""
		itemSnapshotter := snapshotter
		if meta != nil {
			if metaSource := strings.TrimSpace(meta.Source); metaSource != "" {
				source = metaSource
			}
			if metaSnapshotter := strings.TrimSpace(meta.Snapshotter); metaSnapshotter != "" {
				itemSnapshotter = metaSnapshotter
			}
			managed = true
			version = meta.Version
			displayName = strings.TrimSpace(meta.DisplayName)
		}
		if strings.EqualFold(runtimeInfo.Kind, "archive") || hasArchiveSnapshotPrefix(runtimeInfo.Name) {
			itemSnapshotter = "archive"
		}

		name := displayName
		if name == "" {
			if version != nil {
				name = fmt.Sprintf("Version %d", *version)
			} else {
				name = runtimeInfo.Name
			}
		}
		items = append(items, SnapshotInfo{
			Snapshotter: itemSnapshotter,
			Name:        name,
			DisplayName: displayName,
			RuntimeName: runtimeInfo.Name,
			Parent:      runtimeInfo.Parent,
			Kind:        runtimeInfo.Kind,
			CreatedAt:   runtimeInfo.Created,
			UpdatedAt:   runtimeInfo.Updated,
			Labels:      runtimeInfo.Labels,
			Source:      source,
			Managed:     managed,
			Version:     version,
		})
		seen[strings.TrimSpace(runtimeInfo.Name)] = struct{}{}
	}

	for _, runtimeInfo := range lineage {
		name := strings.TrimSpace(runtimeInfo.Name)
		if meta, hasMeta := data.ManagedMeta[name]; hasMeta {
			appendRuntime(runtimeInfo, "image_layer", &meta)
			continue
		}
		appendRuntime(runtimeInfo, "image_layer", nil)
	}

	for name, meta := range data.ManagedMeta {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		runtimeInfo, exists := runtimeByName[name]
		if !exists {
			var syntheticOK bool
			runtimeInfo, syntheticOK = managedArchiveRuntimeInfo(name, meta)
			if !syntheticOK {
				continue
			}
		}
		appendRuntime(runtimeInfo, "managed", &meta)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].Name < items[j].Name
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})

	return ListSnapshotsResponse{
		Snapshotter: snapshotter,
		Snapshots:   items,
	}, true
}

func snapshotLineageRootRequired(snapshotter string) bool {
	switch strings.ToLower(strings.TrimSpace(snapshotter)) {
	case "archive", "docker", "local":
		return false
	default:
		return true
	}
}

func managedArchiveRuntimeInfo(name string, meta workspace.ManagedSnapshotMeta) (ctr.SnapshotInfo, bool) {
	name = strings.TrimSpace(name)
	if name == "" || !managedSnapshotIsArchive(name, meta) {
		return ctr.SnapshotInfo{}, false
	}
	return ctr.SnapshotInfo{
		Name:    name,
		Parent:  strings.TrimSpace(meta.ParentRuntimeSnapshotName),
		Kind:    "archive",
		Created: meta.CreatedAt,
		Updated: meta.CreatedAt,
	}, true
}

func managedSnapshotIsArchive(name string, meta workspace.ManagedSnapshotMeta) bool {
	return strings.EqualFold(strings.TrimSpace(meta.Snapshotter), "archive") || hasArchiveSnapshotPrefix(name)
}

func hasArchiveSnapshotPrefix(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), "archive:")
}

func snapshotLineage(root string, all []ctr.SnapshotInfo) ([]ctr.SnapshotInfo, bool) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, false
	}
	index := make(map[string]ctr.SnapshotInfo, len(all))
	for _, info := range all {
		name := strings.TrimSpace(info.Name)
		if name == "" {
			continue
		}
		index[name] = info
	}
	if _, ok := index[root]; !ok {
		return nil, false
	}
	lineage := make([]ctr.SnapshotInfo, 0, len(index))
	visited := make(map[string]struct{}, len(index))
	current := root
	for current != "" {
		if _, seen := visited[current]; seen {
			break
		}
		info, ok := index[current]
		if !ok {
			break
		}
		lineage = append(lineage, info)
		visited[current] = struct{}{}
		current = strings.TrimSpace(info.Parent)
	}
	return lineage, true
}

// ---------- auth helpers ----------

// requireBotAccess extracts bot_id from path, validates user auth, and authorizes bot access.
func (h *ContainerdHandler) requireBotAccess(c echo.Context) (string, error) {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return "", err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID); err != nil {
		return "", err
	}
	return botID, nil
}

func (*ContainerdHandler) requireChannelIdentityID(c echo.Context) (string, error) {
	return RequireChannelIdentityID(c)
}

func (h *ContainerdHandler) authorizeBotAccess(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccess(ctx, h.botService, h.accountService, channelIdentityID, botID)
}

// requireBotAccessWithGuest is like requireBotAccess but also allows guest access
// via ACL when the caller explicitly opts into guest-compatible access.
func (h *ContainerdHandler) requireBotAccessWithGuest(c echo.Context) (string, error) {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return "", err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID); err != nil {
		return "", err
	}
	return botID, nil
}
