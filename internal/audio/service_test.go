package audio

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"

	_ "modernc.org/sqlite"

	sqlitestore "github.com/memohai/memoh/internal/db/sqlite/store"
	"github.com/memohai/memoh/internal/models"
)

func TestListSpeechModelsByProviderSkipsDisabledModels(t *testing.T) {
	ctx := context.Background()
	conn, queries := newAudioTestDB(t)
	const providerID = "00000000-0000-0000-0000-000000000301"
	insertAudioProvider(t, conn, providerID, models.ClientTypeOpenAISpeech)
	insertAudioModel(t, conn, "00000000-0000-0000-0000-000000000302", "tts-enabled", providerID, models.ModelTypeSpeech, true)
	insertAudioModel(t, conn, "00000000-0000-0000-0000-000000000303", "tts-disabled", providerID, models.ModelTypeSpeech, false)

	service := NewService(slog.New(slog.DiscardHandler), queries, NewRegistry())
	got, err := service.ListSpeechModelsByProvider(ctx, providerID)
	if err != nil {
		t.Fatalf("ListSpeechModelsByProvider() error = %v", err)
	}
	if len(got) != 1 || got[0].ModelID != "tts-enabled" {
		t.Fatalf("ListSpeechModelsByProvider() = %#v, want only enabled model", got)
	}
}

func TestListTranscriptionModelsByProviderSkipsDisabledModels(t *testing.T) {
	ctx := context.Background()
	conn, queries := newAudioTestDB(t)
	const providerID = "00000000-0000-0000-0000-000000000401"
	insertAudioProvider(t, conn, providerID, models.ClientTypeOpenAITranscription)
	insertAudioModel(t, conn, "00000000-0000-0000-0000-000000000402", "asr-enabled", providerID, models.ModelTypeTranscription, true)
	insertAudioModel(t, conn, "00000000-0000-0000-0000-000000000403", "asr-disabled", providerID, models.ModelTypeTranscription, false)

	service := NewService(slog.New(slog.DiscardHandler), queries, NewRegistry())
	got, err := service.ListTranscriptionModelsByProvider(ctx, providerID)
	if err != nil {
		t.Fatalf("ListTranscriptionModelsByProvider() error = %v", err)
	}
	if len(got) != 1 || got[0].ModelID != "asr-enabled" {
		t.Fatalf("ListTranscriptionModelsByProvider() = %#v, want only enabled model", got)
	}
}

func newAudioTestDB(t *testing.T) (*sql.DB, *sqlitestore.Queries) {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	execAudioSchema(t, conn)
	store, err := sqlitestore.New(conn)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	return conn, sqlitestore.NewQueries(store)
}

func execAudioSchema(t *testing.T, conn *sql.DB) {
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
  enable INTEGER NOT NULL DEFAULT 1,
  config TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT models_provider_id_model_id_unique UNIQUE (provider_id, model_id)
);
`)
	if err != nil {
		t.Fatalf("exec audio schema: %v", err)
	}
}

func insertAudioProvider(t *testing.T, conn *sql.DB, id string, clientType models.ClientType) {
	t.Helper()
	_, err := conn.ExecContext(context.Background(), `
INSERT INTO providers (id, name, client_type, icon, enable, config, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id,
		"provider-"+id,
		string(clientType),
		"",
		1,
		`{}`,
		`{}`,
	)
	if err != nil {
		t.Fatalf("insert audio provider: %v", err)
	}
}

func insertAudioModel(t *testing.T, conn *sql.DB, id string, modelID string, providerID string, modelType models.ModelType, enable bool) {
	t.Helper()
	enableValue := 0
	if enable {
		enableValue = 1
	}
	_, err := conn.ExecContext(context.Background(), `
INSERT INTO models (id, model_id, name, provider_id, type, enable, config)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id,
		modelID,
		modelID,
		providerID,
		string(modelType),
		enableValue,
		`{}`,
	)
	if err != nil {
		t.Fatalf("insert audio model: %v", err)
	}
}
