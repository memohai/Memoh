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
)

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

func (m *Manager) ResolveWorkspaceImage(ctx context.Context, botID string) (string, error) {
	return m.resolveWorkspaceImage(ctx, botID)
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
