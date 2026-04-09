package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"gopkg.in/yaml.v3"

	"github.com/memohai/memoh/internal/db/sqlc"
)

// Load reads all .yaml / .yml files from dir and returns parsed provider
// definitions. It returns nil (no error) when the directory does not exist.
// Malformed files are skipped with a warning logged via log.
func Load(log *slog.Logger, dir string) ([]ProviderDefinition, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read providers dir %s: %w", dir, err)
	}

	var defs []ProviderDefinition
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path) //nolint:gosec // operator-managed config directory
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		var def ProviderDefinition
		if err := yaml.Unmarshal(data, &def); err != nil {
			log.Warn("registry: skipping malformed provider file",
				slog.String("path", path), slog.Any("error", err))
			continue
		}
		if def.Name == "" {
			continue
		}
		defs = append(defs, def)
	}
	return defs, nil
}

// Sync upserts the given provider definitions into the database. New providers
// are created with enable=false and an empty API key. Existing providers get
// their icon and client_type refreshed. Models are upserted by (provider_id,
// model_id), overwriting name/type/config.
func Sync(ctx context.Context, logger *slog.Logger, queries *sqlc.Queries, defs []ProviderDefinition) error {
	for _, def := range defs {
		var icon pgtype.Text
		if def.Icon != "" {
			icon = pgtype.Text{String: def.Icon, Valid: true}
		}

		providerCfg := make(map[string]any)
		for k, v := range def.Config {
			providerCfg[k] = v
		}
		if def.BaseURL != "" {
			providerCfg["base_url"] = def.BaseURL
		}
		providerConfigJSON, err := json.Marshal(providerCfg)
		if err != nil {
			logger.Warn("registry: failed to marshal provider config",
				slog.String("name", def.Name), slog.Any("error", err))
			continue
		}

		provider, err := queries.UpsertRegistryProvider(ctx, sqlc.UpsertRegistryProviderParams{
			Name:       def.Name,
			ClientType: def.ClientType,
			Icon:       icon,
			Config:     providerConfigJSON,
		})
		if err != nil {
			logger.Warn("registry: failed to upsert provider", slog.String("name", def.Name), slog.Any("error", err))
			continue
		}

		for _, m := range def.Models {
			configJSON, err := json.Marshal(m.Config)
			if err != nil {
				logger.Warn("registry: failed to marshal model config",
					slog.String("provider", def.Name), slog.String("model", m.ModelID), slog.Any("error", err))
				continue
			}

			var name pgtype.Text
			if m.Name != "" {
				name = pgtype.Text{String: m.Name, Valid: true}
			}

			typ := m.Type
			if typ == "" {
				typ = "chat"
			}

			_, err = queries.UpsertRegistryModel(ctx, sqlc.UpsertRegistryModelParams{
				ModelID:    m.ModelID,
				Name:       name,
				ProviderID: provider.ID,
				Type:       typ,
				Config:     configJSON,
			})
			if err != nil {
				logger.Warn("registry: failed to upsert model",
					slog.String("provider", def.Name), slog.String("model", m.ModelID), slog.Any("error", err))
				continue
			}
		}

		logger.Info("registry: synced provider", slog.String("name", def.Name), slog.Int("models", len(def.Models)))
	}
	return nil
}
