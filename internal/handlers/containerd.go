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

	"github.com/containerd/errdefs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/containerd"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/mcp"
	"github.com/memohai/memoh/internal/policy"
)

type ContainerdHandler struct {
	service          ctr.Service
	manager          *mcp.Manager
	cfg              config.MCPConfig
	namespace        string
	containerBackend string
	logger           *slog.Logger
	toolGateway      *mcp.ToolGatewayService
	mcpMu            sync.Mutex
	mcpSess          map[string]*mcpSession
	mcpStdioMu       sync.Mutex
	mcpStdioSess     map[string]*mcpStdioSession
	botService       *bots.Service
	accountService   *accounts.Service
	policyService    *policy.Service
	queries          *dbsqlc.Queries
}

type CreateContainerRequest struct {
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
	Version      int    `json:"version"`
	Source       string `json:"source"`
}

type SnapshotInfo struct {
	Snapshotter string            `json:"snapshotter"`
	Name        string            `json:"name"`
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

func NewContainerdHandler(log *slog.Logger, service ctr.Service, manager *mcp.Manager, cfg config.MCPConfig, namespace string, containerBackend string, botService *bots.Service, accountService *accounts.Service, policyService *policy.Service, queries *dbsqlc.Queries) *ContainerdHandler {
	return &ContainerdHandler{
		service:          service,
		manager:          manager,
		cfg:              cfg,
		namespace:        namespace,
		containerBackend: containerBackend,
		logger:           log.With(slog.String("handler", "containerd")),
		mcpSess:          make(map[string]*mcpSession),
		mcpStdioSess:     make(map[string]*mcpStdioSession),
		botService:       botService,
		accountService:   accountService,
		policyService:    policyService,
		queries:          queries,
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

	image := h.mcpImageRef()
	snapshotter := strings.TrimSpace(req.Snapshotter)
	if snapshotter == "" {
		snapshotter = h.cfg.Snapshotter
	}

	ctx := c.Request().Context()
	dataRoot := strings.TrimSpace(h.cfg.DataRoot)
	if dataRoot == "" {
		dataRoot = config.DefaultDataRoot
	}
	dataRoot, err = filepath.Abs(dataRoot)
	if err != nil {
		h.logger.Warn("filepath.Abs failed", slog.Any("error", err))
	}
	dataMount := config.DefaultDataMount
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

	spec := h.buildMCPContainerSpec(dataDir, dataMount, resolvPath)

	_, err = h.service.CreateContainer(ctx, ctr.CreateContainerRequest{
		ID:          containerID,
		ImageRef:    image,
		Snapshotter: snapshotter,
		Labels: map[string]string{
			mcp.BotLabelKey: botID,
		},
		Spec: spec,
	})
	if err != nil && !errdefs.IsAlreadyExists(err) {
		return echo.NewHTTPError(http.StatusInternalServerError, "snapshotter="+snapshotter+" image="+image+" err="+err.Error())
	}

	if h.queries != nil {
		pgBotID, parseErr := db.ParseUUID(botID)
		if parseErr == nil {
			ns := strings.TrimSpace(h.namespace)
			if ns == "" {
				ns = "default"
			}
			if dbErr := h.queries.UpsertContainer(c.Request().Context(), dbsqlc.UpsertContainerParams{
				BotID:         pgBotID,
				ContainerID:   containerID,
				ContainerName: containerID,
				Image:         image,
				Status:        "created",
				Namespace:     ns,
				AutoStart:     true,
				HostPath:      pgtype.Text{String: dataDir, Valid: true},
				ContainerPath: dataMount,
			}); dbErr != nil {
				h.logger.Error("failed to upsert container record",
					slog.String("bot_id", botID), slog.Any("error", dbErr))
			}
		}
	}

	started := false
	if err := h.service.StartContainer(ctx, containerID, &ctr.StartTaskOptions{
		UseStdio: false,
	}); err == nil {
		started = true
		if netErr := h.service.SetupNetwork(ctx, ctr.NetworkSetupRequest{
			ContainerID: containerID,
			CNIBinDir:   h.cfg.CNIBinaryDir,
			CNIConfDir:  h.cfg.CNIConfigDir,
		}); netErr != nil {
			h.logger.Warn("mcp container network setup failed, task kept running",
				slog.String("container_id", containerID),
				slog.Any("error", netErr),
			)
		}
		if h.queries != nil {
			if pgBotID, parseErr := db.ParseUUID(botID); parseErr == nil {
				if dbErr := h.queries.UpdateContainerStarted(c.Request().Context(), pgBotID); dbErr != nil {
					h.logger.Error("failed to update container started status",
						slog.String("bot_id", botID), slog.Any("error", dbErr))
				}
			}
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

// ensureContainerAndTask verifies the container exists in containerd and its task is
// running. If the container is missing (e.g. after a VM restart) it is recreated via
// SetupBotContainer. This prevents permanent desync between DB and containerd state.
func (h *ContainerdHandler) ensureContainerAndTask(ctx context.Context, containerID, botID string) error {
	_, err := h.service.GetContainer(ctx, containerID)
	if err != nil {
		if !errdefs.IsNotFound(err) {
			return err
		}
		h.logger.Warn("container missing in containerd, rebuilding",
			slog.String("bot_id", botID),
			slog.String("container_id", containerID),
		)
		return h.SetupBotContainer(ctx, botID)
	}

	tasks, err := h.service.ListTasks(ctx, &ctr.ListTasksOptions{
		Filter: "container.id==" + containerID,
	})
	if err != nil {
		return err
	}
	if len(tasks) > 0 {
		if tasks[0].Status == ctr.TaskStatusRunning {
			if netErr := h.service.SetupNetwork(ctx, ctr.NetworkSetupRequest{
				ContainerID: containerID,
				CNIBinDir:   h.cfg.CNIBinaryDir,
				CNIConfDir:  h.cfg.CNIConfigDir,
			}); netErr != nil {
				h.logger.Warn("network re-setup failed for running task",
					slog.String("container_id", containerID), slog.Any("error", netErr))
			}
			return nil
		}
		if err := h.service.DeleteTask(ctx, containerID, &ctr.DeleteTaskOptions{Force: true}); err != nil {
			h.logger.Warn("cleanup: delete task failed", slog.String("container_id", containerID), slog.Any("error", err))
		}
	}

	if err := h.service.StartContainer(ctx, containerID, &ctr.StartTaskOptions{
		UseStdio: false,
	}); err != nil {
		return err
	}
	if netErr := h.service.SetupNetwork(ctx, ctr.NetworkSetupRequest{
		ContainerID: containerID,
		CNIBinDir:   h.cfg.CNIBinaryDir,
		CNIConfDir:  h.cfg.CNIConfigDir,
	}); netErr != nil {
		h.logger.Warn("network setup failed, task kept running",
			slog.String("container_id", containerID), slog.Any("error", netErr))
	}
	return nil
}

// botContainerID resolves container_id for a bot from the database.
func (h *ContainerdHandler) botContainerID(ctx context.Context, botID string) (string, error) {
	if h.queries != nil {
		pgBotID, err := db.ParseUUID(botID)
		if err == nil {
			row, dbErr := h.queries.GetContainerByBotID(ctx, pgBotID)
			if dbErr == nil && strings.TrimSpace(row.ContainerID) != "" {
				return row.ContainerID, nil
			}
			if dbErr != nil && !errors.Is(dbErr, pgx.ErrNoRows) {
				h.logger.Warn("botContainerID: db lookup failed",
					slog.String("bot_id", botID), slog.Any("error", dbErr))
			}
		}
	}
	containers, err := h.service.ListContainersByLabel(ctx, mcp.BotLabelKey, botID)
	if err != nil {
		return "", err
	}
	if len(containers) == 0 {
		return "", echo.NewHTTPError(http.StatusNotFound, "container not found")
	}
	bestID := ""
	var bestUpdated time.Time
	for _, info := range containers {
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
		pgBotID, parseErr := db.ParseUUID(botID)
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
	info, err := h.service.GetContainer(ctx, containerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return echo.NewHTTPError(http.StatusNotFound, "container not found")
		}
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
	if err := h.ensureContainerAndTask(ctx, containerID, botID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if h.queries != nil {
		if pgBotID, parseErr := db.ParseUUID(botID); parseErr == nil {
			if dbErr := h.queries.UpdateContainerStarted(ctx, pgBotID); dbErr != nil {
				h.logger.Error("failed to update container started status",
					slog.String("bot_id", botID), slog.Any("error", dbErr))
			}
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
	if err := h.service.StopContainer(ctx, containerID, &ctr.StopTaskOptions{
		Timeout: 10 * time.Second,
		Force:   true,
	}); err != nil && !errdefs.IsNotFound(err) {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := h.service.DeleteTask(ctx, containerID, &ctr.DeleteTaskOptions{Force: true}); err != nil {
		h.logger.Warn("cleanup: delete task failed", slog.String("container_id", containerID), slog.Any("error", err))
	}
	if h.queries != nil {
		if pgBotID, parseErr := db.ParseUUID(botID); parseErr == nil {
			if dbErr := h.queries.UpdateContainerStopped(ctx, pgBotID); dbErr != nil {
				h.logger.Error("failed to update container stopped status",
					slog.String("bot_id", botID), slog.Any("error", dbErr))
			}
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
// @Failure 501 {object} ErrorResponse "Snapshots currently not supported on this backend"
// @Router /bots/{bot_id}/container/snapshots [post]
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
	created, err := h.manager.CreateSnapshot(c.Request().Context(), botID, req.SnapshotName, mcp.SnapshotSourceManual)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return echo.NewHTTPError(http.StatusNotFound, "container not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, CreateSnapshotResponse{
		ContainerID:  created.ContainerID,
		SnapshotName: created.SnapshotName,
		Snapshotter:  created.Snapshotter,
		Version:      created.Version,
		Source:       mcp.SnapshotSourceManual,
	})
}

// ListSnapshots godoc
// @Summary List snapshots
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param snapshotter query string false "Snapshotter name"
// @Success 200 {object} ListSnapshotsResponse
// @Failure 501 {object} ErrorResponse "Snapshots currently not supported on this backend"
// @Router /bots/{bot_id}/container/snapshots [get]
func (h *ContainerdHandler) ListSnapshots(c echo.Context) error {
	if h.containerBackend == "apple" {
		return echo.NewHTTPError(http.StatusNotImplemented, "snapshots currently not supported on Apple Container backend")
	}
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	containerID, err := h.botContainerID(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "container not found for bot")
	}
	containerInfo, err := h.service.GetContainer(ctx, containerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return echo.NewHTTPError(http.StatusNotFound, "container not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	requestedSnapshotter := strings.TrimSpace(c.QueryParam("snapshotter"))
	snapshotter := strings.TrimSpace(containerInfo.Snapshotter)
	if requestedSnapshotter != "" {
		if snapshotter != "" && requestedSnapshotter != snapshotter {
			return echo.NewHTTPError(http.StatusBadRequest, "snapshotter does not match container snapshotter")
		}
		snapshotter = requestedSnapshotter
	}
	if snapshotter == "" {
		snapshotter = strings.TrimSpace(h.cfg.Snapshotter)
	}
	if snapshotter == "" {
		snapshotter = "overlayfs"
	}
	snapshotKey := strings.TrimSpace(containerInfo.SnapshotKey)
	if snapshotKey == "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "container snapshot key is empty")
	}

	allSnapshots, err := h.service.ListSnapshots(ctx, snapshotter)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	runtimeByName := make(map[string]ctr.SnapshotInfo, len(allSnapshots))
	for _, info := range allSnapshots {
		name := strings.TrimSpace(info.Name)
		if name == "" {
			continue
		}
		runtimeByName[name] = info
	}
	lineage, ok := snapshotLineage(snapshotKey, allSnapshots)
	if !ok {
		h.logger.Warn("container snapshot chain root not found",
			slog.String("container_id", containerID),
			slog.String("snapshotter", snapshotter),
			slog.String("snapshot_key", snapshotKey),
		)
		return echo.NewHTTPError(http.StatusInternalServerError, "container snapshot chain not found")
	}

	metadataByName := map[string]dbsqlc.ListSnapshotsWithVersionByContainerIDRow{}
	if h.queries != nil {
		managedRows, dbErr := h.queries.ListSnapshotsWithVersionByContainerID(ctx, containerID)
		if dbErr != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, dbErr.Error())
		}
		for _, row := range managedRows {
			name := strings.TrimSpace(row.RuntimeSnapshotName)
			if name == "" {
				continue
			}
			metadataByName[name] = row
		}
	}

	items := make([]SnapshotInfo, 0, len(lineage)+len(metadataByName))
	seen := make(map[string]struct{}, len(lineage)+len(metadataByName))
	appendRuntime := func(runtimeInfo ctr.SnapshotInfo, fallbackSource string, meta *dbsqlc.ListSnapshotsWithVersionByContainerIDRow) {
		source := fallbackSource
		managed := false
		var version *int
		if meta != nil {
			if strings.TrimSpace(meta.Source) != "" {
				source = strings.TrimSpace(meta.Source)
			}
			managed = true
			if meta.Version.Valid {
				v := int(meta.Version.Int32)
				version = &v
			}
		}
		items = append(items, SnapshotInfo{
			Snapshotter: snapshotter,
			Name:        runtimeInfo.Name,
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
		row, hasMeta := metadataByName[name]
		if hasMeta {
			appendRuntime(runtimeInfo, "image_layer", &row)
			continue
		}
		appendRuntime(runtimeInfo, "image_layer", nil)
	}

	for name, row := range metadataByName {
		if _, exists := seen[name]; exists {
			continue
		}
		runtimeInfo, exists := runtimeByName[name]
		if !exists {
			h.logger.Warn("managed snapshot not found in runtime",
				slog.String("container_id", containerID),
				slog.String("snapshot_name", name),
				slog.String("snapshotter", snapshotter),
			)
			continue
		}
		appendRuntime(runtimeInfo, "managed", &row)
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

func (h *ContainerdHandler) mcpImageRef() string {
	return h.cfg.ImageRef()
}

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

func (h *ContainerdHandler) requireChannelIdentityID(c echo.Context) (string, error) {
	return RequireChannelIdentityID(c)
}

func (h *ContainerdHandler) authorizeBotAccess(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccess(ctx, h.botService, h.accountService, channelIdentityID, botID, bots.AccessPolicy{AllowPublicMember: false})
}

// requireBotAccessWithGuest is like requireBotAccess but also allows guest access
// for public bots that have the allow_guest setting enabled.
func (h *ContainerdHandler) requireBotAccessWithGuest(c echo.Context) (string, error) {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return "", err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	policy := bots.AccessPolicy{AllowPublicMember: true, AllowGuest: true}
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID, policy); err != nil {
		return "", err
	}
	return botID, nil
}

// SetupBotContainer creates and starts the MCP container for a bot.
func (h *ContainerdHandler) SetupBotContainer(ctx context.Context, botID string) error {
	containerID := mcp.ContainerPrefix + botID

	image := h.mcpImageRef()
	snapshotter := strings.TrimSpace(h.cfg.Snapshotter)

	dataRoot := strings.TrimSpace(h.cfg.DataRoot)
	if dataRoot == "" {
		dataRoot = config.DefaultDataRoot
	}
	if absRoot, absErr := filepath.Abs(dataRoot); absErr != nil {
		h.logger.Warn("filepath.Abs failed", slog.Any("error", absErr))
	} else {
		dataRoot = absRoot
	}
	dataMount := config.DefaultDataMount
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

	spec := h.buildMCPContainerSpec(dataDir, dataMount, resolvPath)

	_, err = h.service.CreateContainer(ctx, ctr.CreateContainerRequest{
		ID:          containerID,
		ImageRef:    image,
		Snapshotter: snapshotter,
		Labels: map[string]string{
			mcp.BotLabelKey: botID,
		},
		Spec: spec,
	})
	if err != nil && !errdefs.IsAlreadyExists(err) {
		return err
	}

	if h.queries != nil {
		pgBotID, parseErr := db.ParseUUID(botID)
		if parseErr == nil {
			ns := strings.TrimSpace(h.namespace)
			if ns == "" {
				ns = "default"
			}
			if dbErr := h.queries.UpsertContainer(ctx, dbsqlc.UpsertContainerParams{
				BotID:         pgBotID,
				ContainerID:   containerID,
				ContainerName: containerID,
				Image:         image,
				Status:        "created",
				Namespace:     ns,
				AutoStart:     true,
				HostPath:      pgtype.Text{String: dataDir, Valid: true},
				ContainerPath: dataMount,
			}); dbErr != nil {
				h.logger.Error("setup bot container: failed to upsert container record",
					slog.String("bot_id", botID), slog.Any("error", dbErr))
			}
		}
	}

	if err := h.service.StartContainer(ctx, containerID, &ctr.StartTaskOptions{
		UseStdio: false,
	}); err == nil {
		if netErr := h.service.SetupNetwork(ctx, ctr.NetworkSetupRequest{
			ContainerID: containerID,
			CNIBinDir:   h.cfg.CNIBinaryDir,
			CNIConfDir:  h.cfg.CNIConfigDir,
		}); netErr != nil {
			h.logger.Warn("setup bot container: network setup failed, task kept running",
				slog.String("bot_id", botID),
				slog.String("container_id", containerID),
				slog.Any("error", netErr),
			)
		}
		if h.queries != nil {
			if pgBotID, parseErr := db.ParseUUID(botID); parseErr == nil {
				if dbErr := h.queries.UpdateContainerStarted(ctx, pgBotID); dbErr != nil {
					h.logger.Error("setup bot container: failed to update container started status",
						slog.String("bot_id", botID), slog.Any("error", dbErr))
				}
			}
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
	h.logger.Info("CleanupBotContainer starting", slog.String("bot_id", botID))
	containerID, err := h.botContainerID(ctx, botID)
	if err != nil {
		h.logger.Warn("CleanupBotContainer: container not found for bot, cleaning up DB only",
			slog.String("bot_id", botID),
			slog.Any("error", err),
		)
		if h.queries != nil {
			if pgBotID, parseErr := db.ParseUUID(botID); parseErr == nil {
				if dbErr := h.queries.DeleteContainerByBotID(ctx, pgBotID); dbErr != nil {
					h.logger.Error("CleanupBotContainer: failed to delete DB record",
						slog.String("bot_id", botID), slog.Any("error", dbErr))
				}
			}
		}
		return nil
	}

	h.logger.Info("CleanupBotContainer: found container",
		slog.String("bot_id", botID),
		slog.String("container_id", containerID),
	)

	h.logger.Info("CleanupBotContainer: removing network", slog.String("container_id", containerID))
	if err := h.service.RemoveNetwork(ctx, ctr.NetworkSetupRequest{
		ContainerID: containerID,
		CNIBinDir:   h.cfg.CNIBinaryDir,
		CNIConfDir:  h.cfg.CNIConfigDir,
	}); err != nil {
		h.logger.Warn("cleanup: remove network failed", slog.String("container_id", containerID), slog.Any("error", err))
	}
	h.logger.Info("CleanupBotContainer: stopping task", slog.String("container_id", containerID))
	if err := h.service.StopContainer(ctx, containerID, &ctr.StopTaskOptions{
		Timeout: 5 * time.Second,
		Force:   true,
	}); err != nil {
		h.logger.Warn("cleanup: stop task failed", slog.String("container_id", containerID), slog.Any("error", err))
	}
	h.logger.Info("CleanupBotContainer: deleting task", slog.String("container_id", containerID))
	if err := h.service.DeleteTask(ctx, containerID, &ctr.DeleteTaskOptions{Force: true}); err != nil {
		h.logger.Warn("cleanup: delete task failed", slog.String("container_id", containerID), slog.Any("error", err))
	}

	h.logger.Info("CleanupBotContainer: deleting container", slog.String("container_id", containerID))
	if err := h.service.DeleteContainer(ctx, containerID, &ctr.DeleteContainerOptions{
		CleanupSnapshot: true,
	}); err != nil && !errdefs.IsNotFound(err) {
		h.logger.Error("CleanupBotContainer: failed to delete container",
			slog.String("container_id", containerID),
			slog.Any("error", err),
		)
		return err
	}

	if h.queries != nil {
		h.logger.Info("CleanupBotContainer: deleting container record from DB", slog.String("bot_id", botID))
		if pgBotID, parseErr := db.ParseUUID(botID); parseErr == nil {
			if dbErr := h.queries.DeleteContainerByBotID(ctx, pgBotID); dbErr != nil {
				h.logger.Error("CleanupBotContainer: failed to delete DB record",
					slog.String("bot_id", botID), slog.Any("error", dbErr))
			}
		}
	}
	h.logger.Info("CleanupBotContainer finished", slog.String("bot_id", botID))
	return nil
}

func (h *ContainerdHandler) isTaskRunning(ctx context.Context, containerID string) bool {
	tasks, err := h.service.ListTasks(ctx, &ctr.ListTasksOptions{
		Filter: "container.id==" + containerID,
	})
	return err == nil && len(tasks) > 0 && tasks[0].Status == ctr.TaskStatusRunning
}

// ReconcileContainers compares the DB containers table against actual containerd
// state on startup. For each auto_start container in DB it verifies the container
// and task exist; if missing they are rebuilt via SetupBotContainer. Containers that
// the DB claims are running but are not present in containerd get corrected.
func (h *ContainerdHandler) ReconcileContainers(ctx context.Context) {
	if h.queries == nil {
		return
	}
	rows, err := h.queries.ListAutoStartContainers(ctx)
	if err != nil {
		h.logger.Error("reconcile: failed to list containers from DB", slog.Any("error", err))
		return
	}
	if len(rows) == 0 {
		h.logger.Info("reconcile: no auto-start containers in DB")
		return
	}

	h.logger.Info("reconcile: checking containers", slog.Int("count", len(rows)))
	for _, row := range rows {
		containerID := row.ContainerID
		botID := uuid.UUID(row.BotID.Bytes).String()

		_, err := h.service.GetContainer(ctx, containerID)
		if err != nil {
			if !errdefs.IsNotFound(err) {
				h.logger.Error("reconcile: failed to get container",
					slog.String("container_id", containerID), slog.Any("error", err))
				continue
			}
			// Container missing in containerd — rebuild.
			h.logger.Warn("reconcile: container missing, rebuilding",
				slog.String("bot_id", botID), slog.String("container_id", containerID))
			if setupErr := h.SetupBotContainer(ctx, botID); setupErr != nil {
				h.logger.Error("reconcile: rebuild failed",
					slog.String("bot_id", botID), slog.Any("error", setupErr))
				if dbErr := h.queries.UpdateContainerStatus(ctx, dbsqlc.UpdateContainerStatusParams{
					Status: "error",
					BotID:  row.BotID,
				}); dbErr != nil {
					h.logger.Error("reconcile: failed to mark container as error",
						slog.String("bot_id", botID), slog.Any("error", dbErr))
				}
			}
			continue
		}

		// Container exists — ensure the task is running.
		running := h.isTaskRunning(ctx, containerID)
		if running {
			if row.Status != "running" {
				if dbErr := h.queries.UpdateContainerStarted(ctx, row.BotID); dbErr != nil {
					h.logger.Error("reconcile: failed to update DB status to running",
						slog.String("bot_id", botID), slog.Any("error", dbErr))
				}
			}
			if netErr := h.service.SetupNetwork(ctx, ctr.NetworkSetupRequest{
				ContainerID: containerID,
				CNIBinDir:   h.cfg.CNIBinaryDir,
				CNIConfDir:  h.cfg.CNIConfigDir,
			}); netErr != nil {
				h.logger.Warn("reconcile: network re-setup failed for running task",
					slog.String("bot_id", botID),
					slog.String("container_id", containerID),
					slog.Any("error", netErr))
			}
			h.logger.Info("reconcile: container healthy",
				slog.String("bot_id", botID), slog.String("container_id", containerID))
			continue
		}

		// Task not running — try to start it.
		h.logger.Warn("reconcile: task not running, starting",
			slog.String("bot_id", botID), slog.String("container_id", containerID))
		if err := h.ensureContainerAndTask(ctx, containerID, botID); err != nil {
			h.logger.Error("reconcile: failed to start task",
				slog.String("bot_id", botID), slog.Any("error", err))
			if dbErr := h.queries.UpdateContainerStopped(ctx, row.BotID); dbErr != nil {
				h.logger.Error("reconcile: failed to mark container as stopped",
					slog.String("bot_id", botID), slog.Any("error", dbErr))
			}
		} else {
			if dbErr := h.queries.UpdateContainerStarted(ctx, row.BotID); dbErr != nil {
				h.logger.Error("reconcile: failed to update DB status to running",
					slog.String("bot_id", botID), slog.Any("error", dbErr))
			}
		}
	}
	h.logger.Info("reconcile: completed")
}

func (h *ContainerdHandler) buildMCPContainerSpec(dataDir, dataMount, resolvPath string) ctr.ContainerSpec {
	mounts := []ctr.MountSpec{
		{
			Destination: dataMount,
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
	}
	tzMounts, tzEnv := ctr.TimezoneSpec()
	mounts = append(mounts, tzMounts...)

	bootScript := fmt.Sprintf(
		"[ -e /app/mcp ] || { mkdir -p /app; cp -a /opt/mcp /app/mcp 2>/dev/null; true; }; "+
			"cp -an /opt/mcp-template/* %s/ 2>/dev/null; true; "+
			"exec /app/mcp",
		dataMount,
	)

	return ctr.ContainerSpec{
		Cmd:    []string{"/bin/sh", "-c", bootScript},
		Env:    tzEnv,
		Mounts: mounts,
	}
}

func (h *ContainerdHandler) ensureBotDataRoot(botID string) (string, error) {
	dataRoot := strings.TrimSpace(h.cfg.DataRoot)
	if dataRoot == "" {
		dataRoot = config.DefaultDataRoot
	}
	dataRoot, err := filepath.Abs(dataRoot)
	if err != nil {
		return "", err
	}
	root := filepath.Join(dataRoot, "bots", botID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return root, nil
}
