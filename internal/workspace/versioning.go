package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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
	ID                  string
	Version             int
	SnapshotName        string
	RuntimeSnapshotName string
	DisplayName         string
	CreatedAt           time.Time
}

type SnapshotCreateInfo struct {
	ContainerID         string
	SnapshotName        string
	RuntimeSnapshotName string
	DisplayName         string
	Snapshotter         string
	Version             int
	CreatedAt           time.Time
}

type ManagedSnapshotMeta struct {
	Source      string
	Version     *int
	DisplayName string
}

type BotSnapshotData struct {
	ContainerID      string
	Info             ctr.ContainerInfo
	Snapshotter      string
	RuntimeSnapshots []ctr.SnapshotInfo
	ManagedMeta      map[string]ManagedSnapshotMeta
}

func (m *Manager) CreateSnapshot(ctx context.Context, botID, snapshotName, source string) (*SnapshotCreateInfo, error) {
	if m.db == nil || m.queries == nil {
		return nil, errors.New("db is not configured")
	}
	if err := validateBotID(botID); err != nil {
		return nil, err
	}

	containerID := m.resolveContainerID(ctx, botID)
	unlock := m.lockContainer(containerID)
	defer unlock()

	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		return nil, err
	}
	if _, err := m.ensureDBRecords(ctx, botID, info.ID, info.Runtime.Name, info.Image); err != nil {
		return nil, err
	}

	displayName := strings.TrimSpace(snapshotName)
	runtimeSnapshotName := fmt.Sprintf("%s-snapshot-%d", containerID, time.Now().UnixNano())
	normalizedSource := normalizeSnapshotSource(source)

	// The sequence below (stop → commit → replace → start) is atomic from the
	// container's perspective: interrupting it mid-way leaves the container missing.
	// Use a detached context so a cancelled HTTP request cannot break it.
	dctx := context.WithoutCancel(ctx)

	if err := m.safeStopTask(dctx, containerID); err != nil {
		return nil, err
	}

	if err := m.service.CommitSnapshot(dctx, info.Snapshotter, runtimeSnapshotName, info.SnapshotKey); err != nil {
		return nil, err
	}

	activeSnapshotName := fmt.Sprintf("%s-active-%d", containerID, time.Now().UnixNano())
	if err := m.replaceContainerSnapshot(dctx, botID, containerID, info, activeSnapshotName, runtimeSnapshotName); err != nil {
		return nil, err
	}

	_, versionNumber, createdAt, err := m.recordSnapshotVersion(
		dctx,
		containerID,
		runtimeSnapshotName,
		displayName,
		info.SnapshotKey,
		info.Snapshotter,
		normalizedSource,
	)
	if err != nil {
		return nil, err
	}
	if err := m.insertEvent(dctx, containerID, "snapshot_create", map[string]any{
		"snapshot_name":         coalesceSnapshotName(displayName, versionNumber),
		"display_name":          displayName,
		"runtime_snapshot_name": runtimeSnapshotName,
		"snapshotter":           info.Snapshotter,
		"source":                normalizedSource,
		"version":               versionNumber,
	}); err != nil {
		return nil, err
	}

	return &SnapshotCreateInfo{
		ContainerID:         containerID,
		SnapshotName:        coalesceSnapshotName(displayName, versionNumber),
		RuntimeSnapshotName: runtimeSnapshotName,
		DisplayName:         displayName,
		Snapshotter:         info.Snapshotter,
		Version:             versionNumber,
		CreatedAt:           createdAt,
	}, nil
}

func (m *Manager) CreateVersion(ctx context.Context, botID string) (*VersionInfo, error) {
	if m.db == nil || m.queries == nil {
		return nil, errors.New("db is not configured")
	}
	if err := validateBotID(botID); err != nil {
		return nil, err
	}

	containerID := m.resolveContainerID(ctx, botID)
	unlock := m.lockContainer(containerID)
	defer unlock()

	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		return nil, err
	}

	if _, err := m.ensureDBRecords(ctx, botID, info.ID, info.Runtime.Name, info.Image); err != nil {
		return nil, err
	}

	dctx := context.WithoutCancel(ctx)

	if err := m.safeStopTask(dctx, containerID); err != nil {
		return nil, err
	}

	versionSnapshotName := fmt.Sprintf("%s-v%d", containerID, time.Now().UnixNano())
	if err := m.service.CommitSnapshot(dctx, info.Snapshotter, versionSnapshotName, info.SnapshotKey); err != nil {
		return nil, err
	}

	activeSnapshotName := fmt.Sprintf("%s-active-%d", containerID, time.Now().UnixNano())
	if err := m.replaceContainerSnapshot(dctx, botID, containerID, info, activeSnapshotName, versionSnapshotName); err != nil {
		return nil, err
	}

	versionID, versionNumber, createdAt, err := m.recordSnapshotVersion(
		dctx,
		containerID,
		versionSnapshotName,
		"",
		info.SnapshotKey,
		info.Snapshotter,
		SnapshotSourcePreExec,
	)
	if err != nil {
		return nil, err
	}

	if err := m.insertEvent(dctx, containerID, "version_create", map[string]any{
		"snapshot_name": versionSnapshotName,
		"version":       versionNumber,
		"version_id":    versionID,
	}); err != nil {
		return nil, err
	}

	return &VersionInfo{
		ID:                  versionID,
		Version:             versionNumber,
		SnapshotName:        fmt.Sprintf("Version %d", versionNumber),
		RuntimeSnapshotName: versionSnapshotName,
		DisplayName:         "",
		CreatedAt:           createdAt,
	}, nil
}

// ListBotSnapshotData returns the raw snapshot data for a bot under the
// per-container lock, so callers never observe transient state during
// snapshot/version operations.
func (m *Manager) ListBotSnapshotData(ctx context.Context, botID string) (*BotSnapshotData, error) {
	if err := validateBotID(botID); err != nil {
		return nil, err
	}

	containerID := m.resolveContainerID(ctx, botID)
	unlock := m.lockContainer(containerID)
	defer unlock()

	info, err := m.service.GetContainer(ctx, containerID)
	if err != nil {
		return nil, err
	}

	snapshotter := strings.TrimSpace(info.Snapshotter)
	if snapshotter == "" {
		snapshotter = m.cfg.Snapshotter
	}
	if snapshotter == "" {
		snapshotter = "overlayfs"
	}

	runtimeSnapshots, err := m.service.ListSnapshots(ctx, snapshotter)
	if err != nil {
		return nil, err
	}

	managedMeta := make(map[string]ManagedSnapshotMeta)
	if m.queries != nil {
		rows, err := m.queries.ListSnapshotsWithVersionByContainerID(ctx, containerID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			name := strings.TrimSpace(row.RuntimeSnapshotName)
			if name == "" {
				continue
			}
			meta := ManagedSnapshotMeta{
				Source:      strings.TrimSpace(row.Source),
				DisplayName: strings.TrimSpace(row.DisplayName.String),
			}
			if row.Version.Valid {
				v := int(row.Version.Int32)
				meta.Version = &v
			}
			managedMeta[name] = meta
		}
	}

	return &BotSnapshotData{
		ContainerID:      containerID,
		Info:             info,
		Snapshotter:      snapshotter,
		RuntimeSnapshots: runtimeSnapshots,
		ManagedMeta:      managedMeta,
	}, nil
}

func (m *Manager) ListVersions(ctx context.Context, botID string) ([]VersionInfo, error) {
	if m.db == nil || m.queries == nil {
		return nil, errors.New("db is not configured")
	}
	if err := validateBotID(botID); err != nil {
		return nil, err
	}

	containerID := m.resolveContainerID(ctx, botID)
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
			ID:                  uuidString(row.ID),
			Version:             int(row.Version),
			SnapshotName:        coalesceSnapshotName(row.DisplayName.String, int(row.Version)),
			RuntimeSnapshotName: row.RuntimeSnapshotName,
			DisplayName:         strings.TrimSpace(row.DisplayName.String),
			CreatedAt:           createdAt,
		})
	}
	return out, nil
}

func (m *Manager) RollbackVersion(ctx context.Context, botID string, version int) error {
	if m.db == nil || m.queries == nil {
		return errors.New("db is not configured")
	}
	if err := validateBotID(botID); err != nil {
		return err
	}
	if version < 1 || version > math.MaxInt32 {
		return errors.New("version out of range")
	}

	containerID := m.resolveContainerID(ctx, botID)
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

	dctx := context.WithoutCancel(ctx)

	if err := m.safeStopTask(dctx, containerID); err != nil {
		return err
	}

	activeSnapshotName := fmt.Sprintf("%s-rollback-%d", containerID, time.Now().UnixNano())
	if err := m.replaceContainerSnapshot(dctx, botID, containerID, info, activeSnapshotName, snapshotName); err != nil {
		return err
	}

	return m.insertEvent(dctx, containerID, "version_rollback", map[string]any{
		"snapshot_name": snapshotName,
		"version":       version,
		"source":        SnapshotSourceRollback,
	})
}

func (m *Manager) VersionSnapshotName(ctx context.Context, botID string, version int) (string, error) {
	if m.db == nil || m.queries == nil {
		return "", errors.New("db is not configured")
	}
	if err := validateBotID(botID); err != nil {
		return "", err
	}
	if version < 1 || version > math.MaxInt32 {
		return "", errors.New("version out of range")
	}

	containerID := m.resolveContainerID(ctx, botID)
	return m.queries.GetVersionSnapshotRuntimeName(ctx, dbsqlc.GetVersionSnapshotRuntimeNameParams{
		ContainerID: containerID,
		Version:     int32(version),
	})
}

// replaceContainerSnapshot prepares a new active snapshot from parentSnapshot,
// deletes the old container, recreates it on the new snapshot, and restarts the task.
// Caller must pass a detached context (context.WithoutCancel) to guarantee atomicity.
func (m *Manager) replaceContainerSnapshot(ctx context.Context, botID, containerID string, info ctr.ContainerInfo, activeSnapshotName, parentSnapshot string) error {
	if err := m.service.PrepareSnapshot(ctx, info.Snapshotter, activeSnapshotName, parentSnapshot); err != nil {
		return err
	}
	if err := m.service.DeleteContainer(ctx, containerID, &ctr.DeleteContainerOptions{CleanupSnapshot: false}); err != nil {
		return err
	}
	spec, err := m.buildVersionSpec(ctx, botID, workspaceCDIDevicesFromLabels(info.Labels))
	if err != nil {
		return err
	}
	if _, err := m.service.CreateContainerFromSnapshot(ctx, ctr.CreateContainerRequest{
		ID:          containerID,
		ImageRef:    info.Image,
		SnapshotID:  activeSnapshotName,
		Snapshotter: info.Snapshotter,
		Labels:      info.Labels,
		Spec:        spec,
	}); err != nil {
		return err
	}
	if err := m.service.StartContainer(ctx, containerID, nil); err != nil {
		return err
	}
	// Container process was recreated — evict the stale gRPC connection
	// unconditionally so the next call dials fresh to the new process.
	m.grpcPool.Remove(botID)

	// CNI network setup (for outbound connectivity).
	if _, err := m.service.SetupNetwork(ctx, ctr.NetworkSetupRequest{
		ContainerID: containerID,
		CNIBinDir:   m.cfg.CNIBinaryDir,
		CNIConfDir:  m.cfg.CNIConfigDir,
	}); err != nil {
		return fmt.Errorf("network setup after snapshot replace: %w", err)
	}
	return nil
}

func (m *Manager) buildVersionSpec(ctx context.Context, botID string, cdiDevices []string) (ctr.ContainerSpec, error) {
	if len(cdiDevices) == 0 {
		gpu, err := m.resolveWorkspaceGPU(ctx, botID)
		if err != nil {
			return ctr.ContainerSpec{}, err
		}
		cdiDevices = gpu.Devices
	}
	return m.buildWorkspaceContainerSpec(botID, WorkspaceGPUConfig{Devices: cdiDevices})
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

func (m *Manager) ensureDBRecords(ctx context.Context, botID, containerID, _ string, imageRef string) (pgtype.UUID, error) {
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if _, err := m.queries.GetBotByID(ctx, botUUID); err != nil {
		return pgtype.UUID{}, err
	}

	containerPath := config.DefaultDataMount
	if err := m.queries.UpsertContainer(ctx, dbsqlc.UpsertContainerParams{
		BotID:         botUUID,
		ContainerID:   containerID,
		ContainerName: containerID,
		Image:         imageRef,
		Status:        "created",
		Namespace:     "default",
		AutoStart:     true,
		ContainerPath: containerPath,
		LastStartedAt: pgtype.Timestamptz{},
		LastStoppedAt: pgtype.Timestamptz{},
	}); err != nil {
		return pgtype.UUID{}, err
	}

	return botUUID, nil
}

func (m *Manager) recordSnapshotVersion(ctx context.Context, containerID, runtimeSnapshotName, displayName, parentRuntimeSnapshotName, snapshotter, source string) (string, int, time.Time, error) {
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
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := m.queries.WithTx(tx)

	parent := pgtype.Text{}
	normalizedParent := strings.TrimSpace(parentRuntimeSnapshotName)
	if normalizedParent != "" {
		parent = pgtype.Text{String: normalizedParent, Valid: true}
	}
	snapshotRow, err := qtx.UpsertSnapshot(ctx, dbsqlc.UpsertSnapshotParams{
		ContainerID:               containerID,
		RuntimeSnapshotName:       runtimeSnapshotName,
		DisplayName:               pgtype.Text{String: strings.TrimSpace(displayName), Valid: strings.TrimSpace(displayName) != ""},
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

func coalesceSnapshotName(displayName string, version int) string {
	displayName = strings.TrimSpace(displayName)
	if displayName != "" {
		return displayName
	}
	if version > 0 {
		return fmt.Sprintf("Version %d", version)
	}
	return ""
}

func uuidString(v pgtype.UUID) string {
	if !v.Valid {
		return ""
	}
	return uuid.UUID(v.Bytes).String()
}
