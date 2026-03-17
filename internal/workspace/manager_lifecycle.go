package workspace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	ctr "github.com/memohai/memoh/internal/containerd"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
)

// ---------------------------------------------------------------------------
// Container ID resolution
// ---------------------------------------------------------------------------

// ContainerID resolves the containerd container ID for a bot.
// Resolution order: DB lookup → label search → full container scan.
func (m *Manager) ContainerID(ctx context.Context, botID string) (string, error) {
	if m.queries != nil {
		pgBotID, err := db.ParseUUID(botID)
		if err == nil {
			row, dbErr := m.queries.GetContainerByBotID(ctx, pgBotID)
			if dbErr == nil && strings.TrimSpace(row.ContainerID) != "" {
				return row.ContainerID, nil
			}
			if dbErr != nil && !errors.Is(dbErr, pgx.ErrNoRows) {
				m.logger.Warn("ContainerID: db lookup failed",
					slog.String("bot_id", botID), slog.Any("error", dbErr))
			}
		}
	}

	containers, err := m.service.ListContainersByLabel(ctx, BotLabelKey, botID)
	if err != nil {
		return "", err
	}
	if id, ok := newestContainerID(containers); ok {
		return id, nil
	}

	containers, err = m.service.ListContainers(ctx)
	if err != nil {
		return "", err
	}
	matched := make([]ctr.ContainerInfo, 0, len(containers))
	for _, info := range containers {
		resolvedBotID, ok := BotIDFromContainerInfo(info)
		if !ok || resolvedBotID != botID {
			continue
		}
		matched = append(matched, info)
	}
	if id, ok := newestContainerID(matched); ok {
		return id, nil
	}

	return "", ErrContainerNotFound
}

func newestContainerID(containers []ctr.ContainerInfo) (string, bool) {
	bestID := ""
	var bestUpdated time.Time
	for _, info := range containers {
		if bestID == "" || info.UpdatedAt.After(bestUpdated) {
			bestID = info.ID
			bestUpdated = info.UpdatedAt
		}
	}
	return bestID, bestID != ""
}

// ---------------------------------------------------------------------------
// Task & network helpers
// ---------------------------------------------------------------------------

func (m *Manager) isTaskRunning(ctx context.Context, containerID string) bool {
	tasks, err := m.service.ListTasks(ctx, &ctr.ListTasksOptions{
		Filter: "container.id==" + containerID,
	})
	return err == nil && len(tasks) > 0 && tasks[0].Status == ctr.TaskStatusRunning
}

func (m *Manager) setupNetworkAndGetIP(ctx context.Context, containerID string) (string, error) {
	var lastErr error
	for attempt := range 2 {
		result, err := m.service.SetupNetwork(ctx, ctr.NetworkSetupRequest{
			ContainerID: containerID,
			CNIBinDir:   m.cfg.CNIBinaryDir,
			CNIConfDir:  m.cfg.CNIConfigDir,
		})
		if err != nil {
			lastErr = err
			m.logger.Warn("network setup attempt failed",
				slog.String("container_id", containerID),
				slog.Int("attempt", attempt+1),
				slog.Any("error", err))
			continue
		}
		if strings.TrimSpace(result.IP) == "" {
			lastErr = fmt.Errorf("network setup returned no IP for %s", containerID)
			continue
		}
		return result.IP, nil
	}
	return "", fmt.Errorf("network setup failed for container %s: %w", containerID, lastErr)
}

func (m *Manager) setupNetworkOrFail(ctx context.Context, containerID, botID string) error {
	ip, err := m.setupNetworkAndGetIP(ctx, containerID)
	if err != nil {
		return err
	}
	// Legacy containers use TCP gRPC — cache their IP for the pool.
	if m.IsLegacyContainer(ctx, containerID) {
		m.SetLegacyIP(botID, ip)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Lifecycle: ensure / stop / info
// ---------------------------------------------------------------------------

// EnsureRunning verifies the container exists and its task is running.
// If the container is missing, it rebuilds via SetupBotContainer.
// If the task is stopped, it restarts and sets up networking.
func (m *Manager) EnsureRunning(ctx context.Context, botID string) error {
	containerID, err := m.ContainerID(ctx, botID)
	if err != nil {
		if errors.Is(err, ErrContainerNotFound) {
			m.logger.Warn("container missing, rebuilding", slog.String("bot_id", botID))
			return m.SetupBotContainer(ctx, botID)
		}
		return err
	}

	_, err = m.service.GetContainer(ctx, containerID)
	if err != nil {
		if !errdefs.IsNotFound(err) {
			return err
		}
		m.logger.Warn("container missing in containerd, rebuilding",
			slog.String("bot_id", botID), slog.String("container_id", containerID))
		return m.SetupBotContainer(ctx, botID)
	}

	tasks, err := m.service.ListTasks(ctx, &ctr.ListTasksOptions{
		Filter: "container.id==" + containerID,
	})
	if err != nil {
		return err
	}
	if len(tasks) > 0 {
		if tasks[0].Status == ctr.TaskStatusRunning {
			return m.setupNetworkOrFail(ctx, containerID, botID)
		}
		if err := m.service.DeleteTask(ctx, containerID, &ctr.DeleteTaskOptions{Force: true}); err != nil {
			if !errdefs.IsNotFound(err) {
				m.logger.Warn("cleanup: delete task failed",
					slog.String("container_id", containerID), slog.Any("error", err))
				return err
			}
		}
	}

	if err := m.service.StartContainer(ctx, containerID, nil); err != nil {
		return err
	}
	return m.setupNetworkOrFail(ctx, containerID, botID)
}

// StopBot stops the container task for a bot and marks it stopped in DB.
func (m *Manager) StopBot(ctx context.Context, botID string) error {
	containerID, err := m.ContainerID(ctx, botID)
	if err != nil {
		return err
	}

	if err := m.service.StopContainer(ctx, containerID, &ctr.StopTaskOptions{
		Timeout: 10 * time.Second,
		Force:   true,
	}); err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	if err := m.service.DeleteTask(ctx, containerID, &ctr.DeleteTaskOptions{Force: true}); err != nil {
		m.logger.Warn("cleanup: delete task failed",
			slog.String("container_id", containerID), slog.Any("error", err))
	}

	m.markContainerStopped(ctx, botID)
	return nil
}

// GetContainerInfo returns current container status for a bot,
// combining DB records with live containerd state.
func (m *Manager) GetContainerInfo(ctx context.Context, botID string) (*ContainerStatus, error) {
	if m.queries != nil {
		pgBotID, parseErr := db.ParseUUID(botID)
		if parseErr == nil {
			row, dbErr := m.queries.GetContainerByBotID(ctx, pgBotID)
			if dbErr == nil {
				createdAt := time.Time{}
				if row.CreatedAt.Valid {
					createdAt = row.CreatedAt.Time
				}
				updatedAt := time.Time{}
				if row.UpdatedAt.Valid {
					updatedAt = row.UpdatedAt.Time
				}
				return &ContainerStatus{
					ContainerID:      row.ContainerID,
					Image:            row.Image,
					Status:           row.Status,
					Namespace:        row.Namespace,
					ContainerPath:    row.ContainerPath,
					TaskRunning:      m.isTaskRunning(ctx, row.ContainerID),
					HasPreservedData: m.HasPreservedData(botID),
					Legacy:           m.IsLegacyContainer(ctx, row.ContainerID),
					CreatedAt:        createdAt,
					UpdatedAt:        updatedAt,
				}, nil
			}
		}
	}

	containerID, err := m.ContainerID(ctx, botID)
	if err != nil {
		return nil, err
	}
	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, ErrContainerNotFound
		}
		return nil, err
	}
	return &ContainerStatus{
		ContainerID:      info.ID,
		Image:            info.Image,
		Status:           "unknown",
		Namespace:        m.namespace,
		TaskRunning:      m.isTaskRunning(ctx, containerID),
		HasPreservedData: m.HasPreservedData(botID),
		Legacy:           m.IsLegacyContainer(ctx, containerID),
		CreatedAt:        info.CreatedAt,
		UpdatedAt:        info.UpdatedAt,
	}, nil
}

// PullImage pulls a container image. This is exposed so the HTTP layer can
// pass progress callbacks for SSE streaming without needing direct ctr.Service access.
func (m *Manager) PullImage(ctx context.Context, image string, opts *ctr.PullImageOptions) (ctr.ImageInfo, error) {
	return m.service.PullImage(ctx, image, opts)
}

// ---------------------------------------------------------------------------
// Container lifecycle (bots.ContainerLifecycle interface)
// ---------------------------------------------------------------------------

// SetupBotContainer creates/starts the container and upserts the DB record.
func (m *Manager) SetupBotContainer(ctx context.Context, botID string) error {
	image, err := m.resolveWorkspaceImage(ctx, botID)
	if err != nil {
		m.logger.Error("setup bot container: resolve image failed",
			slog.String("bot_id", botID),
			slog.Any("error", err))
		return err
	}

	if err := m.startWithResolvedImage(ctx, botID, image); err != nil {
		m.logger.Error("setup bot container: start failed",
			slog.String("bot_id", botID),
			slog.Any("error", err))
		return err
	}
	if err := m.RememberWorkspaceImage(ctx, botID, image); err != nil {
		m.logger.Warn("setup bot container: remember workspace image failed",
			slog.String("bot_id", botID),
			slog.String("image", image),
			slog.Any("error", err))
	}

	containerID := m.resolveContainerID(ctx, botID)
	m.upsertContainerRecord(ctx, botID, containerID, "running", image)
	return nil
}

// CleanupBotContainer removes the container and DB record for a bot.
// When preserveData is true, /data is exported to a backup archive before deletion.
func (m *Manager) CleanupBotContainer(ctx context.Context, botID string, preserveData bool) error {
	if err := m.Delete(ctx, botID, preserveData); err != nil {
		if preserveData {
			// When preserving data, any error (including NotFound) must
			// block the workflow — we cannot delete the DB record if we
			// failed to preserve data.
			return err
		}
		if !errdefs.IsNotFound(err) {
			return err
		}
		m.logger.Warn("cleanup: container not found in containerd, continuing",
			slog.String("bot_id", botID))
	}

	m.deleteContainerRecord(ctx, botID)
	return nil
}

// ---------------------------------------------------------------------------
// Reconciliation
// ---------------------------------------------------------------------------

// ReconcileContainers compares the DB containers table against actual containerd
// state on startup. For each auto_start container in DB it verifies the container
// and task exist; if missing they are rebuilt.
func (m *Manager) ReconcileContainers(ctx context.Context) {
	if m.queries == nil {
		return
	}
	rows, err := m.queries.ListAutoStartContainers(ctx)
	if err != nil {
		m.logger.Error("reconcile: failed to list containers from DB", slog.Any("error", err))
		return
	}
	if len(rows) == 0 {
		m.logger.Info("reconcile: no auto-start containers in DB")
		return
	}

	m.logger.Info("reconcile: checking containers", slog.Int("count", len(rows)))
	for _, row := range rows {
		containerID := row.ContainerID
		botID := uuid.UUID(row.BotID.Bytes).String()

		_, err := m.service.GetContainer(ctx, containerID)
		if err != nil {
			if !errdefs.IsNotFound(err) {
				m.logger.Error("reconcile: failed to get container",
					slog.String("container_id", containerID), slog.Any("error", err))
				continue
			}
			// Container missing in containerd — rebuild.
			m.logger.Warn("reconcile: container missing, rebuilding",
				slog.String("bot_id", botID), slog.String("container_id", containerID))
			if setupErr := m.SetupBotContainer(ctx, botID); setupErr != nil {
				m.logger.Error("reconcile: rebuild failed",
					slog.String("bot_id", botID), slog.Any("error", setupErr))
				m.markContainerStatus(ctx, botID, "error")
			}
			continue
		}

		// --- legacy container support (mcp- prefix, TCP gRPC) ---
		// Remove when all deployments have migrated to workspace- containers.
		if m.IsLegacyContainer(ctx, containerID) {
			m.logger.Warn("reconcile: legacy container (pre-bridge), using TCP fallback",
				slog.String("bot_id", botID), slog.String("container_id", containerID))

			running := m.isTaskRunning(ctx, containerID)
			if !running {
				if err := m.EnsureRunning(ctx, botID); err != nil {
					m.logger.Error("reconcile: failed to start legacy container",
						slog.String("bot_id", botID), slog.Any("error", err))
					continue
				}
			}
			if ip, netErr := m.setupNetworkAndGetIP(ctx, containerID); netErr != nil {
				m.logger.Error("reconcile: network setup failed for legacy container",
					slog.String("bot_id", botID), slog.Any("error", netErr))
			} else {
				m.SetLegacyIP(botID, ip)
				m.logger.Info("reconcile: legacy container reachable via TCP",
					slog.String("bot_id", botID), slog.String("ip", ip))
			}
			continue
		}

		// Container exists — ensure the task is running.
		running := m.isTaskRunning(ctx, containerID)
		if running {
			if row.Status != "running" {
				m.markContainerStarted(ctx, botID)
			}
			if netErr := m.setupNetworkOrFail(ctx, containerID, botID); netErr != nil {
				m.logger.Error("reconcile: network setup failed for running task, container unreachable",
					slog.String("bot_id", botID),
					slog.String("container_id", containerID),
					slog.Any("error", netErr))
			} else {
				m.logger.Info("reconcile: container healthy",
					slog.String("bot_id", botID), slog.String("container_id", containerID))
			}
			continue
		}

		// Task not running — try to start it.
		m.logger.Warn("reconcile: task not running, starting",
			slog.String("bot_id", botID), slog.String("container_id", containerID))
		if err := m.EnsureRunning(ctx, botID); err != nil {
			m.logger.Error("reconcile: failed to start task",
				slog.String("bot_id", botID), slog.Any("error", err))
			m.markContainerStopped(ctx, botID)
		} else {
			m.markContainerStarted(ctx, botID)
		}
	}
	m.logger.Info("reconcile: completed")
}

// RecordContainerRunning upserts a DB record marking the resolved container as running.
// This is exported for the HTTP handler's SSE-based creation flow, where the
// pull + start happen in the handler but the DB write belongs to Manager.
func (m *Manager) RecordContainerRunning(ctx context.Context, botID, containerID, image string) {
	m.upsertContainerRecord(ctx, botID, containerID, "running", image)
}

// ---------------------------------------------------------------------------
// DB record helpers (unexported)
// ---------------------------------------------------------------------------

func (m *Manager) upsertContainerRecord(ctx context.Context, botID, containerID, status, image string) {
	if m.queries == nil {
		return
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return
	}
	ns := strings.TrimSpace(m.namespace)
	if ns == "" {
		ns = "default"
	}
	if dbErr := m.queries.UpsertContainer(ctx, dbsqlc.UpsertContainerParams{
		BotID:         pgBotID,
		ContainerID:   containerID,
		ContainerName: containerID,
		Image:         image,
		Status:        status,
		Namespace:     ns,
		AutoStart:     true,
	}); dbErr != nil {
		m.logger.Error("failed to upsert container record",
			slog.String("bot_id", botID), slog.Any("error", dbErr))
	}
	if status == "running" {
		m.markContainerStarted(ctx, botID)
	}
}

func (m *Manager) deleteContainerRecord(ctx context.Context, botID string) {
	if m.queries == nil {
		return
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return
	}
	if dbErr := m.queries.DeleteContainerByBotID(ctx, pgBotID); dbErr != nil {
		m.logger.Error("failed to delete container record",
			slog.String("bot_id", botID), slog.Any("error", dbErr))
	}
}

func (m *Manager) markContainerStarted(ctx context.Context, botID string) {
	if m.queries == nil {
		return
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return
	}
	if dbErr := m.queries.UpdateContainerStarted(ctx, pgBotID); dbErr != nil {
		m.logger.Error("failed to update container started status",
			slog.String("bot_id", botID), slog.Any("error", dbErr))
	}
}

func (m *Manager) markContainerStopped(ctx context.Context, botID string) {
	if m.queries == nil {
		return
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return
	}
	if dbErr := m.queries.UpdateContainerStopped(ctx, pgBotID); dbErr != nil {
		m.logger.Error("failed to update container stopped status",
			slog.String("bot_id", botID), slog.Any("error", dbErr))
	}
}

func (m *Manager) markContainerStatus(ctx context.Context, botID, status string) {
	if m.queries == nil {
		return
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return
	}
	if dbErr := m.queries.UpdateContainerStatus(ctx, dbsqlc.UpdateContainerStatusParams{
		Status: status,
		BotID:  pgBotID,
	}); dbErr != nil {
		m.logger.Error("failed to update container status",
			slog.String("bot_id", botID), slog.Any("error", dbErr))
	}
}
