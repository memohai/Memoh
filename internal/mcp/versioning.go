package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/config"

	ctr "github.com/memohai/memoh/internal/containerd"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
)

const (
	SnapshotSourceManual   = "manual"
	SnapshotSourcePreExec  = "pre_exec"
	SnapshotSourceRollback = "rollback"
)

type VersionInfo struct {
	ID           string
	Version      int
	SnapshotName string
	CreatedAt    time.Time
}

type SnapshotCreateInfo struct {
	ContainerID  string
	SnapshotName string
	Snapshotter  string
	Version      int
	CreatedAt    time.Time
}

func (m *Manager) CreateSnapshot(ctx context.Context, botID, snapshotName, source string) (*SnapshotCreateInfo, error) {
	if m.db == nil || m.queries == nil {
		return nil, fmt.Errorf("db is not configured")
	}
	if err := validateBotID(botID); err != nil {
		return nil, err
	}

	containerID := m.containerID(botID)
	unlock := m.lockContainer(containerID)
	defer unlock()

	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		return nil, err
	}
	if _, err := m.ensureDBRecords(ctx, botID, info.ID, info.Runtime.Name, info.Image); err != nil {
		return nil, err
	}

	normalizedSnapshotName := strings.TrimSpace(snapshotName)
	if normalizedSnapshotName == "" {
		normalizedSnapshotName = fmt.Sprintf("%s-%s", containerID, time.Now().Format("20060102150405"))
	}
	normalizedSource := normalizeSnapshotSource(source)

	if err := m.service.CommitSnapshot(ctx, info.Snapshotter, normalizedSnapshotName, info.SnapshotKey); err != nil {
		return nil, err
	}

	_, versionNumber, createdAt, err := m.recordSnapshotVersion(
		ctx,
		containerID,
		normalizedSnapshotName,
		info.SnapshotKey,
		info.Snapshotter,
		normalizedSource,
	)
	if err != nil {
		return nil, err
	}
	if err := m.insertEvent(ctx, containerID, "snapshot_create", map[string]any{
		"snapshot_name": normalizedSnapshotName,
		"snapshotter":   info.Snapshotter,
		"source":        normalizedSource,
		"version":       versionNumber,
	}); err != nil {
		return nil, err
	}

	return &SnapshotCreateInfo{
		ContainerID:  containerID,
		SnapshotName: normalizedSnapshotName,
		Snapshotter:  info.Snapshotter,
		Version:      versionNumber,
		CreatedAt:    createdAt,
	}, nil
}

func (m *Manager) CreateVersion(ctx context.Context, botID string) (*VersionInfo, error) {
	if m.db == nil || m.queries == nil {
		return nil, fmt.Errorf("db is not configured")
	}
	if err := validateBotID(botID); err != nil {
		return nil, err
	}

	containerID := m.containerID(botID)
	unlock := m.lockContainer(containerID)
	defer unlock()

	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		return nil, err
	}

	if _, err := m.ensureDBRecords(ctx, botID, info.ID, info.Runtime.Name, info.Image); err != nil {
		return nil, err
	}

	if err := m.safeStopTask(ctx, containerID); err != nil {
		return nil, err
	}

	versionSnapshotName := fmt.Sprintf("%s-v%d", containerID, time.Now().UnixNano())
	if err := m.service.CommitSnapshot(ctx, info.Snapshotter, versionSnapshotName, info.SnapshotKey); err != nil {
		return nil, err
	}

	activeSnapshotName := fmt.Sprintf("%s-active-%d", containerID, time.Now().UnixNano())
	if err := m.service.PrepareSnapshot(ctx, info.Snapshotter, activeSnapshotName, versionSnapshotName); err != nil {
		return nil, err
	}

	if err := m.service.DeleteContainer(ctx, containerID, &ctr.DeleteContainerOptions{CleanupSnapshot: false}); err != nil {
		return nil, err
	}

	spec, err := m.buildVersionSpec(botID)
	if err != nil {
		return nil, err
	}

	_, err = m.service.CreateContainerFromSnapshot(ctx, ctr.CreateContainerRequest{
		ID:          containerID,
		ImageRef:    info.Image,
		SnapshotID:  activeSnapshotName,
		Snapshotter: info.Snapshotter,
		Labels:      info.Labels,
		Spec:        spec,
	})
	if err != nil {
		return nil, err
	}

	versionID, versionNumber, createdAt, err := m.recordSnapshotVersion(
		ctx,
		containerID,
		versionSnapshotName,
		info.SnapshotKey,
		info.Snapshotter,
		SnapshotSourcePreExec,
	)
	if err != nil {
		return nil, err
	}

	if err := m.insertEvent(ctx, containerID, "version_create", map[string]any{
		"snapshot_name": versionSnapshotName,
		"version":       versionNumber,
		"version_id":    versionID,
	}); err != nil {
		return nil, err
	}

	return &VersionInfo{
		ID:           versionID,
		Version:      versionNumber,
		SnapshotName: versionSnapshotName,
		CreatedAt:    createdAt,
	}, nil
}

func (m *Manager) ListVersions(ctx context.Context, botID string) ([]VersionInfo, error) {
	if m.db == nil || m.queries == nil {
		return nil, fmt.Errorf("db is not configured")
	}
	if err := validateBotID(botID); err != nil {
		return nil, err
	}

	containerID := m.containerID(botID)
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
			ID:           uuidString(row.ID),
			Version:      int(row.Version),
			SnapshotName: row.RuntimeSnapshotName,
			CreatedAt:    createdAt,
		})
	}
	return out, nil
}

func (m *Manager) RollbackVersion(ctx context.Context, botID string, version int) error {
	if m.db == nil || m.queries == nil {
		return fmt.Errorf("db is not configured")
	}
	if err := validateBotID(botID); err != nil {
		return err
	}

	containerID := m.containerID(botID)
	unlock := m.lockContainer(containerID)
	defer unlock()

	snapshotName, err := m.queries.GetVersionSnapshotRuntimeName(ctx, dbsqlc.GetVersionSnapshotRuntimeNameParams{
		ContainerID: containerID,
		Version:     int32(version),
	})
	if err != nil {
		return err
	}

	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		return err
	}

	if err := m.safeStopTask(ctx, containerID); err != nil {
		return err
	}

	activeSnapshotName := fmt.Sprintf("%s-rollback-%d", containerID, time.Now().UnixNano())
	if err := m.service.PrepareSnapshot(ctx, info.Snapshotter, activeSnapshotName, snapshotName); err != nil {
		return err
	}

	if err := m.service.DeleteContainer(ctx, containerID, &ctr.DeleteContainerOptions{CleanupSnapshot: false}); err != nil {
		return err
	}

	spec, err := m.buildVersionSpec(botID)
	if err != nil {
		return err
	}

	_, err = m.service.CreateContainerFromSnapshot(ctx, ctr.CreateContainerRequest{
		ID:          containerID,
		ImageRef:    info.Image,
		SnapshotID:  activeSnapshotName,
		Snapshotter: info.Snapshotter,
		Labels:      info.Labels,
		Spec:        spec,
	})
	if err != nil {
		return err
	}

	return m.insertEvent(ctx, containerID, "version_rollback", map[string]any{
		"snapshot_name": snapshotName,
		"version":       version,
		"source":        SnapshotSourceRollback,
	})
}

func (m *Manager) VersionSnapshotName(ctx context.Context, botID string, version int) (string, error) {
	if m.db == nil || m.queries == nil {
		return "", fmt.Errorf("db is not configured")
	}
	if err := validateBotID(botID); err != nil {
		return "", err
	}

	containerID := m.containerID(botID)
	return m.queries.GetVersionSnapshotRuntimeName(ctx, dbsqlc.GetVersionSnapshotRuntimeNameParams{
		ContainerID: containerID,
		Version:     int32(version),
	})
}

func (m *Manager) buildVersionSpec(botID string) (ctr.ContainerSpec, error) {
	dataDir, err := m.ensureBotDir(botID)
	if err != nil {
		return ctr.ContainerSpec{}, err
	}
	dataMount := m.cfg.DataMount
	if dataMount == "" {
		dataMount = config.DefaultDataMount
	}
	resolvPath, err := ctr.ResolveConfSource(dataDir)
	if err != nil {
		return ctr.ContainerSpec{}, err
	}
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
	return ctr.ContainerSpec{
		Mounts: mounts,
		Env:    tzEnv,
	}, nil
}

func (m *Manager) safeStopTask(ctx context.Context, containerID string) error {
	err := m.service.StopContainer(ctx, containerID, &ctr.StopTaskOptions{
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

func (m *Manager) recordSnapshotVersion(ctx context.Context, containerID, runtimeSnapshotName, parentRuntimeSnapshotName, snapshotter, source string) (string, int, time.Time, error) {
	containerID = strings.TrimSpace(containerID)
	runtimeSnapshotName = strings.TrimSpace(runtimeSnapshotName)
	snapshotter = strings.TrimSpace(snapshotter)
	if containerID == "" || runtimeSnapshotName == "" || snapshotter == "" {
		return "", 0, time.Time{}, ctr.ErrInvalidArgument
	}

	tx, err := m.db.Begin(ctx)
	if err != nil {
		return "", 0, time.Time{}, err
	}
	defer tx.Rollback(ctx)

	qtx := m.queries.WithTx(tx)

	parent := pgtype.Text{}
	normalizedParent := strings.TrimSpace(parentRuntimeSnapshotName)
	if normalizedParent != "" {
		parent = pgtype.Text{String: normalizedParent, Valid: true}
	}
	snapshotRow, err := qtx.UpsertSnapshot(ctx, dbsqlc.UpsertSnapshotParams{
		ContainerID:               containerID,
		RuntimeSnapshotName:       runtimeSnapshotName,
		ParentRuntimeSnapshotName: parent,
		Snapshotter:               snapshotter,
		Source:                    normalizeSnapshotSource(source),
	})
	if err != nil {
		return "", 0, time.Time{}, err
	}

	version, err := qtx.NextVersion(ctx, containerID)
	if err != nil {
		return "", 0, time.Time{}, err
	}

	versionRow, err := qtx.InsertVersion(ctx, dbsqlc.InsertVersionParams{
		ContainerID: containerID,
		SnapshotID:  snapshotRow.ID,
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

	return uuidString(versionRow.ID), int(version), createdAt, nil
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

func normalizeSnapshotSource(source string) string {
	s := strings.TrimSpace(source)
	if s == "" {
		return SnapshotSourceManual
	}
	return s
}

func uuidString(v pgtype.UUID) string {
	if !v.Valid {
		return ""
	}
	return uuid.UUID(v.Bytes).String()
}
