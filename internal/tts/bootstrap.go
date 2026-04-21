package tts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/sqlc"
)

func SyncRegistry(ctx context.Context, logger *slog.Logger, queries *sqlc.Queries, registry *Registry) error {
	for _, def := range registry.List() {
		configJSON, err := json.Marshal(map[string]any{})
		if err != nil {
			return fmt.Errorf("marshal speech provider config: %w", err)
		}
		var icon pgtype.Text
		if def.Icon != "" {
			icon = pgtype.Text{String: def.Icon, Valid: true}
		}

		provider, err := queries.UpsertRegistryProvider(ctx, sqlc.UpsertRegistryProviderParams{
			Name:       def.DisplayName,
			ClientType: string(def.ClientType),
			Icon:       icon,
			Config:     configJSON,
		})
		if err != nil {
			return fmt.Errorf("upsert speech provider %s: %w", def.ClientType, err)
		}

		synced := 0
		for _, model := range def.Models {
			if shouldHideTemplateModel(def, model.ID) {
				if err := queries.DeleteModelByProviderIDAndModelID(ctx, sqlc.DeleteModelByProviderIDAndModelIDParams{
					ProviderID: provider.ID,
					ModelID:    model.ID,
				}); err != nil {
					return fmt.Errorf("delete hidden speech template model %s: %w", model.ID, err)
				}
				continue
			}
			modelConfigJSON, err := json.Marshal(map[string]any{})
			if err != nil {
				return fmt.Errorf("marshal speech model config: %w", err)
			}
			name := pgtype.Text{String: model.Name, Valid: model.Name != ""}
			if _, err := queries.UpsertRegistryModel(ctx, sqlc.UpsertRegistryModelParams{
				ModelID:    model.ID,
				Name:       name,
				ProviderID: provider.ID,
				Type:       "speech",
				Config:     modelConfigJSON,
			}); err != nil {
				return fmt.Errorf("upsert speech model %s: %w", model.ID, err)
			}
			synced++
		}

		if logger != nil {
			logger.Info("speech registry synced", slog.String("provider", string(def.ClientType)), slog.Int("models", synced))
		}
	}
	return nil
}
