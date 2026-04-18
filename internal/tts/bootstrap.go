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

		provider, err := queries.UpsertRegistryProvider(ctx, sqlc.UpsertRegistryProviderParams{
			Name:       def.DisplayName,
			ClientType: string(def.ClientType),
			Icon:       pgtype.Text{},
			Config:     configJSON,
		})
		if err != nil {
			return fmt.Errorf("upsert speech provider %s: %w", def.ClientType, err)
		}

		for _, model := range def.Models {
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
		}

		if logger != nil {
			logger.Info("speech registry synced", slog.String("provider", string(def.ClientType)), slog.Int("models", len(def.Models)))
		}
	}
	return nil
}
