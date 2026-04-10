package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/sqlc"
)

const (
	workspaceMetadataKey      = "workspace"
	workspaceImageMetadataKey = "image"
	workspaceGPUMetadataKey   = "gpu"
	workspaceGPUDevicesKey    = "devices"
)

type WorkspaceGPUConfig struct {
	Devices []string `json:"devices,omitempty"`
}

func decodeBotMetadata(payload []byte) (map[string]any, error) {
	if len(payload) == 0 {
		return map[string]any{}, nil
	}
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, err
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

func cloneAnyMap(src map[string]any) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(src))
	for key, value := range src {
		cloned[key] = value
	}
	return cloned
}

func workspaceSection(metadata map[string]any) map[string]any {
	raw, ok := metadata[workspaceMetadataKey]
	if !ok {
		return map[string]any{}
	}
	section, ok := raw.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return cloneAnyMap(section)
}

func workspaceImageFromMetadata(metadata map[string]any) string {
	section := workspaceSection(metadata)
	image, _ := section[workspaceImageMetadataKey].(string)
	return strings.TrimSpace(image)
}

func normalizeWorkspaceGPUDevices(devices []string) []string {
	if len(devices) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(devices))
	normalized := make([]string, 0, len(devices))
	for _, raw := range devices {
		device := strings.TrimSpace(raw)
		if device == "" {
			continue
		}
		if _, ok := seen[device]; ok {
			continue
		}
		seen[device] = struct{}{}
		normalized = append(normalized, device)
	}
	return normalized
}

func workspaceGPUFromMetadata(metadata map[string]any) (WorkspaceGPUConfig, bool) {
	section := workspaceSection(metadata)
	raw, ok := section[workspaceGPUMetadataKey]
	if !ok {
		return WorkspaceGPUConfig{}, false
	}

	gpuSection, ok := raw.(map[string]any)
	if !ok {
		return WorkspaceGPUConfig{}, true
	}

	var devices []string
	switch typed := gpuSection[workspaceGPUDevicesKey].(type) {
	case []string:
		devices = append(devices, typed...)
	case []any:
		for _, item := range typed {
			if device, ok := item.(string); ok {
				devices = append(devices, device)
			}
		}
	}

	return WorkspaceGPUConfig{Devices: normalizeWorkspaceGPUDevices(devices)}, true
}

func withWorkspaceImagePreference(metadata map[string]any, image string) map[string]any {
	next := cloneAnyMap(metadata)
	section := workspaceSection(next)
	section[workspaceImageMetadataKey] = strings.TrimSpace(image)
	next[workspaceMetadataKey] = section
	return next
}

func withoutWorkspaceImagePreference(metadata map[string]any) map[string]any {
	next := cloneAnyMap(metadata)
	section := workspaceSection(next)
	delete(section, workspaceImageMetadataKey)
	if len(section) == 0 {
		delete(next, workspaceMetadataKey)
		return next
	}
	next[workspaceMetadataKey] = section
	return next
}

func withWorkspaceGPUPreference(metadata map[string]any, gpu WorkspaceGPUConfig) map[string]any {
	next := cloneAnyMap(metadata)
	section := workspaceSection(next)
	section[workspaceGPUMetadataKey] = map[string]any{
		workspaceGPUDevicesKey: normalizeWorkspaceGPUDevices(gpu.Devices),
	}
	next[workspaceMetadataKey] = section
	return next
}

func withoutWorkspaceGPUPreference(metadata map[string]any) map[string]any {
	next := cloneAnyMap(metadata)
	section := workspaceSection(next)
	delete(section, workspaceGPUMetadataKey)
	if len(section) == 0 {
		delete(next, workspaceMetadataKey)
		return next
	}
	next[workspaceMetadataKey] = section
	return next
}

func (m *Manager) botWorkspaceImagePreference(ctx context.Context, botID string) (string, error) {
	if m.queries == nil {
		return "", nil
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return "", err
	}
	row, err := m.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	metadata, err := decodeBotMetadata(row.Metadata)
	if err != nil {
		return "", err
	}
	return workspaceImageFromMetadata(metadata), nil
}

func (m *Manager) updateBotWorkspaceImagePreference(ctx context.Context, botID, image string, clearPreference bool) error {
	if m.queries == nil {
		return nil
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	row, err := m.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		return err
	}
	metadata, err := decodeBotMetadata(row.Metadata)
	if err != nil {
		return err
	}
	if clearPreference {
		metadata = withoutWorkspaceImagePreference(metadata)
	} else {
		metadata = withWorkspaceImagePreference(metadata, image)
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = m.queries.UpdateBotProfile(ctx, dbsqlc.UpdateBotProfileParams{
		ID:          botUUID,
		DisplayName: row.DisplayName,
		AvatarUrl:   row.AvatarUrl,
		Timezone:    row.Timezone,
		IsActive:    row.IsActive,
		Metadata:    payload,
	})
	return err
}

func (m *Manager) RememberWorkspaceImage(ctx context.Context, botID, image string) error {
	return m.updateBotWorkspaceImagePreference(ctx, botID, config.NormalizeImageRef(image), false)
}

func (m *Manager) ClearWorkspaceImagePreference(ctx context.Context, botID string) error {
	return m.updateBotWorkspaceImagePreference(ctx, botID, "", true)
}

func (m *Manager) botWorkspaceGPUPreference(ctx context.Context, botID string) (WorkspaceGPUConfig, bool, error) {
	if m.queries == nil {
		return WorkspaceGPUConfig{}, false, nil
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return WorkspaceGPUConfig{}, false, err
	}
	row, err := m.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WorkspaceGPUConfig{}, false, nil
		}
		return WorkspaceGPUConfig{}, false, err
	}
	metadata, err := decodeBotMetadata(row.Metadata)
	if err != nil {
		return WorkspaceGPUConfig{}, false, err
	}
	gpu, ok := workspaceGPUFromMetadata(metadata)
	return gpu, ok, nil
}

func (m *Manager) updateBotWorkspaceGPUPreference(ctx context.Context, botID string, gpu WorkspaceGPUConfig, clearPreference bool) error {
	if m.queries == nil {
		return nil
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	row, err := m.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		return err
	}
	metadata, err := decodeBotMetadata(row.Metadata)
	if err != nil {
		return err
	}
	if clearPreference {
		metadata = withoutWorkspaceGPUPreference(metadata)
	} else {
		metadata = withWorkspaceGPUPreference(metadata, gpu)
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = m.queries.UpdateBotProfile(ctx, dbsqlc.UpdateBotProfileParams{
		ID:          botUUID,
		DisplayName: row.DisplayName,
		AvatarUrl:   row.AvatarUrl,
		Timezone:    row.Timezone,
		IsActive:    row.IsActive,
		Metadata:    payload,
	})
	return err
}

func (m *Manager) RememberWorkspaceGPU(ctx context.Context, botID string, gpu WorkspaceGPUConfig) error {
	gpu.Devices = normalizeWorkspaceGPUDevices(gpu.Devices)
	return m.updateBotWorkspaceGPUPreference(ctx, botID, gpu, false)
}

func (m *Manager) ClearWorkspaceGPUPreference(ctx context.Context, botID string) error {
	return m.updateBotWorkspaceGPUPreference(ctx, botID, WorkspaceGPUConfig{}, true)
}

func (m *Manager) ResolveWorkspaceImage(ctx context.Context, botID string) (string, error) {
	return m.resolveWorkspaceImage(ctx, botID)
}

func (m *Manager) ResolveWorkspaceGPU(ctx context.Context, botID string) (WorkspaceGPUConfig, error) {
	return m.resolveWorkspaceGPU(ctx, botID)
}

func (m *Manager) resolveWorkspaceImage(ctx context.Context, botID string) (string, error) {
	if m.queries != nil {
		pgBotID, err := db.ParseUUID(botID)
		if err == nil {
			row, dbErr := m.queries.GetContainerByBotID(ctx, pgBotID)
			if dbErr == nil && strings.TrimSpace(row.Image) != "" {
				return config.NormalizeImageRef(strings.TrimSpace(row.Image)), nil
			}
			if dbErr != nil && !errors.Is(dbErr, pgx.ErrNoRows) {
				return "", dbErr
			}
		}
	}

	preferredImage, err := m.botWorkspaceImagePreference(ctx, botID)
	if err != nil {
		return "", err
	}
	if preferredImage != "" {
		return config.NormalizeImageRef(preferredImage), nil
	}

	return m.imageRef(), nil
}

func (m *Manager) resolveWorkspaceGPU(ctx context.Context, botID string) (WorkspaceGPUConfig, error) {
	preferredGPU, hasPreference, err := m.botWorkspaceGPUPreference(ctx, botID)
	if err != nil {
		return WorkspaceGPUConfig{}, err
	}
	if hasPreference {
		preferredGPU.Devices = normalizeWorkspaceGPUDevices(preferredGPU.Devices)
		return preferredGPU, nil
	}

	return WorkspaceGPUConfig{}, nil
}
