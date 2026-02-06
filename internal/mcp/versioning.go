package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/memohai/memoh/internal/config"

	ctr "github.com/memohai/memoh/internal/containerd"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
)

type VersionInfo struct {
	ID         string
	Version    int
	SnapshotID string
	CreatedAt  time.Time
}

func (m *Manager) CreateVersion(ctx context.Context, userID string) (*VersionInfo, error) {
	if m.db == nil || m.queries == nil {
		return nil, fmt.Errorf("db is not configured")
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

func (m *Manager) ListVersions(ctx context.Context, userID string) ([]VersionInfo, error) {
	if m.db == nil || m.queries == nil {
		return nil, fmt.Errorf("db is not configured")
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

func (m *Manager) RollbackVersion(ctx context.Context, userID string, version int) error {
	if m.db == nil || m.queries == nil {
		return fmt.Errorf("db is not configured")
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

func (m *Manager) VersionSnapshotID(ctx context.Context, userID string, version int) (string, error) {
	if m.db == nil || m.queries == nil {
		return "", fmt.Errorf("db is not configured")
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

func (m *Manager) ensureDBRecords(ctx context.Context, botID, containerID, runtime, imageRef string) (pgtype.UUID, error) {
	hostPath, err := m.DataDir(botID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	botUUID, err := parseUUID(botID)
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
	defer tx.Rollback(ctx)

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

func parseUUID(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID: %w", err)
	}
	var pgID pgtype.UUID
	pgID.Valid = true
	copy(pgID.Bytes[:], parsed[:])
	return pgID, nil
}
