package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/containerd"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
)

// VersionInfo holds version record (id, version number, snapshot id, created_at).
type VersionInfo struct {
	ID         string
	Version    int
	SnapshotID string
	CreatedAt  time.Time
}

// CreateVersion commits the current container snapshot, creates a new container from it, and records the version.
func (m *Manager) CreateVersion(ctx context.Context, userID string) (*VersionInfo, error) {
	if m.db == nil || m.queries == nil {
		return nil, errors.New("db is not configured")
	}
	if err := validateBotID(userID); err != nil {
		return nil, err
	}

	containerID := m.containerID(userID)
	container, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		return nil, err
	}

	info, err := container.Info(ctx)
	if err != nil {
		return nil, err
	}

	if _, err := m.ensureDBRecords(ctx, userID, info.ID, info.Runtime.Name, info.Image); err != nil {
		return nil, err
	}

	if err := m.safeStopTask(ctx, containerID); err != nil {
		return nil, err
	}

	versionSnapshotID := fmt.Sprintf("%s-v%d", containerID, time.Now().UnixNano())
	if err := m.service.CommitSnapshot(ctx, info.Snapshotter, versionSnapshotID, info.SnapshotKey); err != nil {
		return nil, err
	}

	activeSnapshotID := fmt.Sprintf("%s-active-%d", containerID, time.Now().UnixNano())
	if err := m.service.PrepareSnapshot(ctx, info.Snapshotter, activeSnapshotID, versionSnapshotID); err != nil {
		return nil, err
	}

	if err := m.service.DeleteContainer(ctx, containerID, &ctr.DeleteContainerOptions{CleanupSnapshot: false}); err != nil {
		return nil, err
	}

	dataDir, err := m.ensureBotDir(userID)
	if err != nil {
		return nil, err
	}
	dataMount := m.cfg.DataMount
	if dataMount == "" {
		dataMount = config.DefaultDataMount
	}
	resolvPath, err := ctr.ResolveConfSource(dataDir)
	if err != nil {
		return nil, err
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
				Destination: "/etc/resolv.conf",
				Type:        "bind",
				Source:      resolvPath,
				Options:     []string{"rbind", "ro"},
			},
		}),
	}

	_, err = m.service.CreateContainerFromSnapshot(ctx, ctr.CreateContainerRequest{
		ID:          containerID,
		ImageRef:    info.Image,
		SnapshotID:  activeSnapshotID,
		Snapshotter: info.Snapshotter,
		Labels:      info.Labels,
		SpecOpts:    specOpts,
	})
	if err != nil {
		return nil, err
	}

	versionID, versionNumber, createdAt, err := m.insertVersion(ctx, containerID, versionSnapshotID, info.Snapshotter)
	if err != nil {
		return nil, err
	}

	if err := m.insertEvent(ctx, containerID, "version_create", map[string]any{
		"snapshot_id": versionSnapshotID,
		"version":     versionNumber,
	}); err != nil {
		return nil, err
	}

	return &VersionInfo{
		ID:         versionID,
		Version:    versionNumber,
		SnapshotID: versionSnapshotID,
		CreatedAt:  createdAt,
	}, nil
}

// ListVersions returns version records for the bot (userID) from DB, newest first.
func (m *Manager) ListVersions(ctx context.Context, userID string) ([]VersionInfo, error) {
	if m.db == nil || m.queries == nil {
		return nil, errors.New("db is not configured")
	}
	if err := validateBotID(userID); err != nil {
		return nil, err
	}

	containerID := m.containerID(userID)
	versions, err := m.queries.ListVersionsByContainerID(ctx, containerID)
	if err != nil {
		return nil, err
	}

	out := make([]VersionInfo, 0, len(versions))
	for _, row := range versions {
		createdAt := time.Time{}
		if row.CreatedAt.Valid {
			createdAt = row.CreatedAt.Time
		}
		out = append(out, VersionInfo{
			ID:         row.ID,
			Version:    int(row.Version),
			SnapshotID: row.SnapshotID,
			CreatedAt:  createdAt,
		})
	}
	return out, nil
}

// RollbackVersion restores the container from the given version snapshot (delete current, create from snapshot).
func (m *Manager) RollbackVersion(ctx context.Context, userID string, version int) error {
	if m.db == nil || m.queries == nil {
		return errors.New("db is not configured")
	}
	if err := validateBotID(userID); err != nil {
		return err
	}

	containerID := m.containerID(userID)
	snapshotID, err := m.queries.GetVersionSnapshotID(ctx, dbsqlc.GetVersionSnapshotIDParams{
		ContainerID: containerID,
		Version:     int32(version),
	})
	if err != nil {
		return err
	}

	container, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		return err
	}
	info, err := container.Info(ctx)
	if err != nil {
		return err
	}

	if err := m.safeStopTask(ctx, containerID); err != nil {
		return err
	}

	activeSnapshotID := fmt.Sprintf("%s-rollback-%d", containerID, time.Now().UnixNano())
	if err := m.service.PrepareSnapshot(ctx, info.Snapshotter, activeSnapshotID, snapshotID); err != nil {
		return err
	}

	if err := m.service.DeleteContainer(ctx, containerID, &ctr.DeleteContainerOptions{CleanupSnapshot: false}); err != nil {
		return err
	}

	dataDir, err := m.ensureBotDir(userID)
	if err != nil {
		return err
	}
	dataMount := m.cfg.DataMount
	if dataMount == "" {
		dataMount = config.DefaultDataMount
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
				Destination: "/etc/resolv.conf",
				Type:        "bind",
				Source:      resolvPath,
				Options:     []string{"rbind", "ro"},
			},
		}),
	}

	_, err = m.service.CreateContainerFromSnapshot(ctx, ctr.CreateContainerRequest{
		ID:          containerID,
		ImageRef:    info.Image,
		SnapshotID:  activeSnapshotID,
		Snapshotter: info.Snapshotter,
		Labels:      info.Labels,
		SpecOpts:    specOpts,
	})
	if err != nil {
		return err
	}

	return m.insertEvent(ctx, containerID, "version_rollback", map[string]any{
		"snapshot_id": snapshotID,
		"version":     version,
	})
}

// VersionSnapshotID returns the snapshot ID for the given version number from DB.
func (m *Manager) VersionSnapshotID(ctx context.Context, userID string, version int) (string, error) {
	if m.db == nil || m.queries == nil {
		return "", errors.New("db is not configured")
	}
	if err := validateBotID(userID); err != nil {
		return "", err
	}

	containerID := m.containerID(userID)
	return m.queries.GetVersionSnapshotID(ctx, dbsqlc.GetVersionSnapshotIDParams{
		ContainerID: containerID,
		Version:     int32(version),
	})
}

func (m *Manager) safeStopTask(ctx context.Context, containerID string) error {
	err := m.service.StopTask(ctx, containerID, &ctr.StopTaskOptions{
		Timeout: 10 * time.Second,
		Force:   true,
	})
	if err == nil {
		return nil
	}
	if errdefs.IsNotFound(err) {
		return nil
	}
	return err
}

func (m *Manager) ensureDBRecords(ctx context.Context, botID, containerID, _ string, imageRef string) (pgtype.UUID, error) {
	hostPath, err := m.DataDir(botID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if _, err := m.queries.GetBotByID(ctx, botUUID); err != nil {
		return pgtype.UUID{}, err
	}

	containerPath := m.cfg.DataMount
	if containerPath == "" {
		containerPath = config.DefaultDataMount
	}

	if err := m.queries.UpsertContainer(ctx, dbsqlc.UpsertContainerParams{
		BotID:         botUUID,
		ContainerID:   containerID,
		ContainerName: containerID,
		Image:         imageRef,
		Status:        "created",
		Namespace:     "default",
		AutoStart:     true,
		HostPath:      pgtype.Text{String: hostPath, Valid: hostPath != ""},
		ContainerPath: containerPath,
		LastStartedAt: pgtype.Timestamptz{},
		LastStoppedAt: pgtype.Timestamptz{},
	}); err != nil {
		return pgtype.UUID{}, err
	}

	return botUUID, nil
}

func (m *Manager) insertVersion(ctx context.Context, containerID, snapshotID, snapshotter string) (string, int, time.Time, error) {
	tx, err := m.db.Begin(ctx)
	if err != nil {
		return "", 0, time.Time{}, err
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			m.logger.Warn("insert version: tx rollback failed", slog.Any("error", err))
		}
	}()

	qtx := m.queries.WithTx(tx)

	version, err := qtx.NextVersion(ctx, containerID)
	if err != nil {
		return "", 0, time.Time{}, err
	}

	if err := qtx.InsertSnapshot(ctx, dbsqlc.InsertSnapshotParams{
		ID:               snapshotID,
		ContainerID:      containerID,
		ParentSnapshotID: pgtype.Text{},
		Snapshotter:      snapshotter,
		Digest:           pgtype.Text{},
	}); err != nil {
		return "", 0, time.Time{}, err
	}

	id := fmt.Sprintf("%s-%d", containerID, version)
	versionRow, err := qtx.InsertVersion(ctx, dbsqlc.InsertVersionParams{
		ID:          id,
		ContainerID: containerID,
		SnapshotID:  snapshotID,
		Version:     version,
	})
	if err != nil {
		return "", 0, time.Time{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", 0, time.Time{}, err
	}

	createdAt := time.Time{}
	if versionRow.CreatedAt.Valid {
		createdAt = versionRow.CreatedAt.Time
	}

	return id, int(version), createdAt, nil
}

func (m *Manager) insertEvent(ctx context.Context, containerID, eventType string, payload map[string]any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return m.queries.InsertLifecycleEvent(ctx, dbsqlc.InsertLifecycleEventParams{
		ID:          fmt.Sprintf("%s-%d", containerID, time.Now().UnixNano()),
		ContainerID: containerID,
		EventType:   eventType,
		Payload:     b,
	})
}
