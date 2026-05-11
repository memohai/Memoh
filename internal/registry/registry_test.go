package registry

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	sqlitestore "github.com/memohai/memoh/internal/db/sqlite/store"
)

func TestSyncUpdatesProviderWhenRegistryNameChanges(t *testing.T) {
	ctx := context.Background()
	conn, queries := newRegistryTestQueries(t)
	logger := slog.New(slog.DiscardHandler)

	initial := ProviderDefinition{
		Name:       "OpenAI",
		ClientType: "openai-responses",
		Icon:       "openai",
		BaseURL:    "https://api.openai.com/v1",
		Source:     "openai.yaml",
		Models: []ModelDefinition{{
			ModelID: "gpt-test",
			Name:    "GPT Test",
			Type:    "chat",
			Config:  map[string]any{"context_window": 128000},
		}},
	}
	if err := Sync(ctx, logger, queries, []ProviderDefinition{initial}); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	providers, err := queries.ListProviders(ctx)
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	if len(providers) != 1 {
		t.Fatalf("provider count = %d, want 1", len(providers))
	}
	providerID := providers[0].ID.String()
	_, err = conn.ExecContext(ctx,
		`UPDATE providers SET enable = 1, config = ? WHERE id = ?`,
		`{"base_url":"https://custom.example/v1","api_key":"sk-existing","prompt_cache_ttl":"5m"}`,
		providerID,
	)
	if err != nil {
		t.Fatalf("seed provider config: %v", err)
	}

	renamed := initial
	renamed.Name = "OpenAI Responses"
	renamed.Models[0].Name = "GPT Test Updated"
	renamed.Models[0].Config = map[string]any{"context_window": 256000}
	if err := Sync(ctx, logger, queries, []ProviderDefinition{renamed}); err != nil {
		t.Fatalf("renamed sync: %v", err)
	}

	providers, err = queries.ListProviders(ctx)
	if err != nil {
		t.Fatalf("list providers after rename: %v", err)
	}
	if len(providers) != 1 {
		t.Fatalf("provider count after rename = %d, want 1", len(providers))
	}
	got := providers[0]
	if got.ID.String() != providerID {
		t.Fatalf("provider id = %s, want existing %s", got.ID.String(), providerID)
	}
	if got.Name != "OpenAI Responses" {
		t.Fatalf("provider name = %q, want renamed value", got.Name)
	}
	if !got.Enable {
		t.Fatalf("provider enable = false, want preserved true")
	}
	cfg := jsonMap(t, got.Config)
	if cfg["api_key"] != "sk-existing" {
		t.Fatalf("api_key = %#v, want preserved secret", cfg["api_key"])
	}
	if cfg["base_url"] != "https://custom.example/v1" {
		t.Fatalf("base_url = %#v, want preserved custom value", cfg["base_url"])
	}
	if cfg["prompt_cache_ttl"] != "5m" {
		t.Fatalf("prompt_cache_ttl = %#v, want preserved custom value", cfg["prompt_cache_ttl"])
	}
	assertRegistrySource(t, got.Metadata, "openai.yaml")

	models, err := queries.ListModelsByProviderID(ctx, got.ID)
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(models) != 1 || models[0].Name.String != "GPT Test Updated" {
		t.Fatalf("models = %+v, want updated model", models)
	}
	modelCfg := jsonMap(t, models[0].Config)
	if value := modelCfg["context_window"]; value != float64(256000) {
		t.Fatalf("model context_window = %#v, want 256000", value)
	}
}

func TestSyncMatchesLegacyProviderByBaseURL(t *testing.T) {
	ctx := context.Background()
	conn, queries := newRegistryTestQueries(t)
	logger := slog.New(slog.DiscardHandler)

	const providerID = "00000000-0000-0000-0000-000000000001"
	_, err := conn.ExecContext(ctx, `
INSERT INTO providers (id, name, client_type, icon, enable, config, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		providerID,
		"OpenAI Legacy",
		"openai-responses",
		"openai",
		1,
		`{"base_url":"https://api.openai.com/v1","api_key":"sk-legacy"}`,
		`{}`,
	)
	if err != nil {
		t.Fatalf("insert legacy provider: %v", err)
	}
	_, err = conn.ExecContext(ctx, `
INSERT INTO models (id, model_id, name, provider_id, type, config)
VALUES (?, ?, ?, ?, ?, ?)`,
		"00000000-0000-0000-0000-000000000002",
		"gpt-test",
		"GPT Test Legacy",
		providerID,
		"chat",
		`{"context_window":64000}`,
	)
	if err != nil {
		t.Fatalf("insert legacy model: %v", err)
	}

	def := ProviderDefinition{
		Name:       "OpenAI Responses",
		ClientType: "openai-responses",
		Icon:       "openai",
		BaseURL:    "https://api.openai.com/v1",
		Source:     "openai.yaml",
		Models: []ModelDefinition{{
			ModelID: "gpt-test",
			Name:    "GPT Test",
			Type:    "chat",
			Config:  map[string]any{"context_window": 128000},
		}},
	}
	if err := Sync(ctx, logger, queries, []ProviderDefinition{def}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	providers, err := queries.ListProviders(ctx)
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	if len(providers) != 1 {
		t.Fatalf("provider count = %d, want 1", len(providers))
	}
	got := providers[0]
	if got.ID.String() != providerID {
		t.Fatalf("provider id = %s, want legacy %s", got.ID.String(), providerID)
	}
	if got.Name != "OpenAI Responses" {
		t.Fatalf("provider name = %q, want registry name", got.Name)
	}
	cfg := jsonMap(t, got.Config)
	if cfg["api_key"] != "sk-legacy" {
		t.Fatalf("api_key = %#v, want preserved legacy secret", cfg["api_key"])
	}
	assertRegistrySource(t, got.Metadata, "openai.yaml")
}

func newRegistryTestQueries(t *testing.T) (*sql.DB, *sqlitestore.Queries) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	execRegistrySchema(t, conn)
	store, err := sqlitestore.New(conn)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	return conn, sqlitestore.NewQueries(store)
}

func execRegistrySchema(t *testing.T, conn *sql.DB) {
	t.Helper()
	_, err := conn.ExecContext(context.Background(), `
CREATE TABLE providers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  client_type TEXT NOT NULL DEFAULT 'openai-completions',
  icon TEXT,
  enable INTEGER NOT NULL DEFAULT 1,
  config TEXT NOT NULL DEFAULT '{}',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT providers_name_unique UNIQUE (name)
);

CREATE TABLE models (
  id TEXT PRIMARY KEY,
  model_id TEXT NOT NULL,
  name TEXT,
  provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  type TEXT NOT NULL DEFAULT 'chat',
  config TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT models_provider_id_model_id_unique UNIQUE (provider_id, model_id)
);
`)
	if err != nil {
		t.Fatalf("exec registry schema: %v", err)
	}
}

func jsonMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal json map: %v", err)
	}
	return out
}

func assertRegistrySource(t *testing.T, raw []byte, want string) {
	t.Helper()
	metadata := jsonMap(t, raw)
	registryMeta, ok := metadata[registryMetadataKey].(map[string]any)
	if !ok {
		t.Fatalf("registry metadata missing: %#v", metadata)
	}
	if registryMeta["source"] != want {
		t.Fatalf("registry source = %#v, want %q", registryMeta["source"], want)
	}
}
