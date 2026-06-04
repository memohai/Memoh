package providers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	sqlitestore "github.com/memohai/memoh/internal/db/sqlite/store"
	"github.com/memohai/memoh/internal/models"
)

func TestMaskAPIKey(t *testing.T) {
	t.Parallel()

	t.Run("short key is fully masked", func(t *testing.T) {
		t.Parallel()
		if got := maskAPIKey("sk-12"); got != "*****" {
			t.Fatalf("expected fully masked, got %q", got)
		}
	})

	t.Run("long key preserves prefix", func(t *testing.T) {
		t.Parallel()
		key := "sk-1234567890abcdef"
		masked := maskAPIKey(key)
		if masked == key {
			t.Fatal("masked key should differ from original")
		}
		if len(masked) != len(key) {
			t.Fatalf("masked length %d != original length %d", len(masked), len(key))
		}
		if masked[:8] != key[:8] {
			t.Fatalf("prefix mismatch: %q vs %q", masked[:8], key[:8])
		}
	})

	t.Run("empty key returns empty", func(t *testing.T) {
		t.Parallel()
		if got := maskAPIKey(""); got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})
}

func TestNormalizeProviderConfig(t *testing.T) {
	t.Parallel()

	t.Run("github copilot drops legacy secrets", func(t *testing.T) {
		t.Parallel()

		cfg := normalizeProviderConfig("github-copilot", map[string]any{
			"api_key":                  "gh-secret",
			configOAuthClientSecretKey: "oauth-secret",
			"base_url":                 "ignored",
		})

		if _, exists := cfg[configOAuthClientSecretKey]; exists {
			t.Fatalf("expected oauth client secret to be removed, got %#v", cfg[configOAuthClientSecretKey])
		}
		if _, exists := cfg["api_key"]; exists {
			t.Fatalf("expected legacy api_key to be removed, got %#v", cfg["api_key"])
		}
	})

	t.Run("non copilot providers keep api key key", func(t *testing.T) {
		t.Parallel()

		cfg := normalizeProviderConfig("openai-completions", map[string]any{
			"api_key": "sk-live",
		})

		if got, ok := cfg["api_key"].(string); !ok || got != "sk-live" {
			t.Fatalf("expected api_key to remain untouched, got %#v", cfg["api_key"])
		}
	})
}

func TestIsHiddenRegistryTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider sqlc.Provider
		want     bool
	}{
		{
			name: "disabled registry provider template is hidden",
			provider: sqlc.Provider{
				Enable:   false,
				Metadata: []byte(`{"registry":{"source":"deepseek.yaml"}}`),
			},
			want: true,
		},
		{
			name: "disabled custom provider remains visible",
			provider: sqlc.Provider{
				Enable:   false,
				Metadata: []byte(`{}`),
			},
			want: false,
		},
		{
			name: "disabled configured registry provider remains visible",
			provider: sqlc.Provider{
				Enable:   false,
				Config:   []byte(`{"base_url":"https://api.deepseek.com/v1","api_key":"sk-existing"}`),
				Metadata: []byte(`{"registry":{"source":"deepseek.yaml"}}`),
			},
			want: false,
		},
		{
			name: "enabled registry-derived provider remains visible",
			provider: sqlc.Provider{
				Enable:   true,
				Metadata: []byte(`{"registry":{"source":"deepseek.yaml"}}`),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isHiddenRegistryTemplate(tt.provider); got != tt.want {
				t.Fatalf("isHiddenRegistryTemplate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateActivatesHiddenRegistryTemplate(t *testing.T) {
	ctx := context.Background()
	conn, queries := newProviderServiceTestQueries(t)

	const providerID = "00000000-0000-0000-0000-000000000101"
	_, err := conn.ExecContext(ctx, `
INSERT INTO providers (id, name, client_type, icon, enable, config, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		providerID,
		"DeepSeek",
		string(models.ClientTypeOpenAICompletions),
		"deepseek-color",
		0,
		`{"base_url":"https://api.deepseek.com/v1"}`,
		`{"registry":{"source":"deepseek.yaml"}}`,
	)
	if err != nil {
		t.Fatalf("insert hidden template provider: %v", err)
	}
	_, err = conn.ExecContext(ctx, `
INSERT INTO models (id, model_id, name, provider_id, type, config)
VALUES (?, ?, ?, ?, ?, ?)`,
		"00000000-0000-0000-0000-000000000102",
		"deepseek-chat",
		"DeepSeek Chat",
		providerID,
		string(models.ModelTypeChat),
		`{}`,
	)
	if err != nil {
		t.Fatalf("insert hidden template model: %v", err)
	}

	service := &Service{queries: queries}
	resp, err := service.Create(ctx, CreateRequest{
		Name:       "DeepSeek",
		ClientType: string(models.ClientTypeOpenAICompletions),
		Icon:       "deepseek-color",
		Config: map[string]any{
			"base_url": "https://api.deepseek.com/v1",
			"api_key":  "sk-new",
		},
		Metadata: map[string]any{
			"preset": map[string]any{
				"id":     "deepseek",
				"source": "deepseek.yaml",
			},
		},
	})
	if err != nil {
		t.Fatalf("create provider from hidden template: %v", err)
	}
	if resp.ID != providerID {
		t.Fatalf("provider id = %s, want activated template id %s", resp.ID, providerID)
	}
	if !resp.Enable {
		t.Fatal("provider should be enabled")
	}
	if _, ok := resp.Metadata["registry"]; ok {
		t.Fatalf("registry metadata should be removed from activated provider: %#v", resp.Metadata)
	}

	raw, err := queries.GetProviderByName(ctx, "DeepSeek")
	if err != nil {
		t.Fatalf("get provider by name: %v", err)
	}
	cfg := providerConfig(raw.Config)
	if cfg["api_key"] != "sk-new" {
		t.Fatalf("api_key = %#v, want stored credential", cfg["api_key"])
	}
	providerUUID, err := db.ParseUUID(providerID)
	if err != nil {
		t.Fatalf("parse provider id: %v", err)
	}
	providerModels, err := queries.ListModelsByProviderID(ctx, providerUUID)
	if err != nil {
		t.Fatalf("list provider models: %v", err)
	}
	if len(providerModels) != 1 || providerModels[0].ModelID != "deepseek-chat" {
		t.Fatalf("provider models = %#v, want existing template model retained", providerModels)
	}
	list, err := service.List(ctx)
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	if len(list) != 1 || list[0].Name != "DeepSeek" {
		t.Fatalf("visible providers = %#v, want activated provider", list)
	}
}

func TestMaskConfigSecrets(t *testing.T) {
	t.Parallel()

	cfg := maskConfigSecrets("openai-completions", map[string]any{
		"api_key": "sk-secret-123456",
	})

	masked, _ := cfg["api_key"].(string)
	if masked == "" || masked == "sk-secret-123456" {
		t.Fatalf("expected api key to be masked, got %q", masked)
	}
}

func TestPreserveMaskedConfigSecret(t *testing.T) {
	t.Parallel()

	merged := map[string]any{
		configOAuthClientSecretKey: "*************",
	}
	existing := map[string]any{
		configOAuthClientSecretKey: "gh-secret-1234",
	}
	incoming := map[string]any{
		configOAuthClientSecretKey: maskAPIKey("gh-secret-1234"),
	}

	preserveMaskedConfigSecret(merged, existing, incoming, configOAuthClientSecretKey)

	if got, _ := merged[configOAuthClientSecretKey].(string); got != "gh-secret-1234" {
		t.Fatalf("expected masked value to be restored to original secret, got %q", got)
	}
}

func TestDeviceMetadataRoundTrip(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(2026, time.April, 11, 12, 0, 0, 0, time.UTC)
	device := oauthDeviceMetadata{
		DeviceCode:      "device-code",
		UserCode:        "ABCD-EFGH",
		VerificationURI: "https://github.com/login/device",
		ExpiresAt:       expiresAt,
		IntervalSeconds: 5,
	}

	parsed := deviceMetadataFromMap(device.toMetadata())
	if parsed.DeviceCode != device.DeviceCode {
		t.Fatalf("expected device code %q, got %q", device.DeviceCode, parsed.DeviceCode)
	}
	if parsed.UserCode != device.UserCode {
		t.Fatalf("expected user code %q, got %q", device.UserCode, parsed.UserCode)
	}
	if parsed.VerificationURI != device.VerificationURI {
		t.Fatalf("expected verification uri %q, got %q", device.VerificationURI, parsed.VerificationURI)
	}
	if !parsed.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expiresAt %s, got %s", expiresAt, parsed.ExpiresAt)
	}
	if parsed.IntervalSeconds != device.IntervalSeconds {
		t.Fatalf("expected interval %d, got %d", device.IntervalSeconds, parsed.IntervalSeconds)
	}

	status := parsed.toStatus()
	if status == nil || !status.Pending {
		t.Fatalf("expected pending device status, got %#v", status)
	}
}

func TestAccountMetadataRoundTrip(t *testing.T) {
	t.Parallel()

	account := oauthAccountMetadata{
		Label:      "octocat",
		Login:      "octocat",
		Name:       "The Octocat",
		Email:      "octocat@github.com",
		AvatarURL:  "https://avatars.githubusercontent.com/u/1?v=4",
		ProfileURL: "https://github.com/octocat",
	}

	parsed := accountMetadataFromMap(account.toMetadata())
	if parsed.Label != account.Label {
		t.Fatalf("expected label %q, got %q", account.Label, parsed.Label)
	}
	if parsed.Login != account.Login {
		t.Fatalf("expected login %q, got %q", account.Login, parsed.Login)
	}
	if parsed.Name != account.Name {
		t.Fatalf("expected name %q, got %q", account.Name, parsed.Name)
	}
	if parsed.Email != account.Email {
		t.Fatalf("expected email %q, got %q", account.Email, parsed.Email)
	}
	if parsed.AvatarURL != account.AvatarURL {
		t.Fatalf("expected avatar url %q, got %q", account.AvatarURL, parsed.AvatarURL)
	}
	if parsed.ProfileURL != account.ProfileURL {
		t.Fatalf("expected profile url %q, got %q", account.ProfileURL, parsed.ProfileURL)
	}

	status := parsed.toStatus()
	if status == nil {
		t.Fatal("expected account status")
		return
	}
	if status.Label != account.Label {
		t.Fatalf("expected status label %q, got %q", account.Label, status.Label)
	}
}

func TestOAuthConfigForGitHubCopilotUsesFixedDeviceFlowSettings(t *testing.T) {
	t.Parallel()

	service := &Service{}
	cfg := service.oauthConfigForProvider(sqlc.Provider{
		ClientType: string(models.ClientTypeGitHubCopilot),
		Config:     []byte(`{"api_key":"legacy","oauth_client_secret":"legacy-secret"}`),
		Metadata:   []byte(`{"oauth_client_id":"custom","oauth_scopes":"repo"}`),
	})

	if cfg.ClientID != "Iv1.b507a08c87ecfe98" {
		t.Fatalf("expected fixed client id, got %q", cfg.ClientID)
	}
	if cfg.ClientSecret != "" {
		t.Fatalf("expected empty client secret, got %q", cfg.ClientSecret)
	}
	if cfg.Scopes != "read:user user:email" {
		t.Fatalf("expected fixed scope, got %q", cfg.Scopes)
	}
}

func TestFetchRemoteModelsViaSDK(t *testing.T) {
	t.Parallel()

	t.Run("anthropic", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/models" {
				t.Fatalf("expected /v1/models path (auto-appended), got %q", r.URL.Path)
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"id":           "claude-sonnet-4-20250514",
						"display_name": "Claude Sonnet 4",
						"type":         "model",
					},
				},
				"has_more": false,
			})
		}))
		defer server.Close()

		s := &Service{}
		remoteModels, err := s.fetchRemoteModelsViaSDK(context.Background(), sqlc.Provider{
			ClientType: string(models.ClientTypeAnthropicMessages),
			Config:     []byte(`{"base_url":"` + server.URL + `","api_key":"sk-ant-test"}`),
		})
		if err != nil {
			t.Fatalf("fetch remote models: %v", err)
		}
		if len(remoteModels) != 1 {
			t.Fatalf("expected 1 model, got %d", len(remoteModels))
		}
		if remoteModels[0].Name != "Claude Sonnet 4" {
			t.Fatalf("expected display name, got %q", remoteModels[0].Name)
		}
		if remoteModels[0].Type != string(models.ModelTypeChat) {
			t.Fatalf("expected chat type, got %q", remoteModels[0].Type)
		}
	})

	t.Run("google gemini", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/models":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"models": []map[string]any{
						{
							"name":                       "models/gemini-2.0-flash",
							"displayName":                "Gemini 2.0 Flash",
							"supportedGenerationMethods": []string{"generateContent", "countTokens"},
						},
						{
							"name":                       "models/gemini-embedding-001",
							"displayName":                "Gemini Embedding 001",
							"supportedGenerationMethods": []string{"embedContent", "countTokens"},
						},
					},
				})
			case "/models/gemini-embedding-001:embedContent":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"embedding": map[string]any{
						"values": []float64{0.1, 0.2, 0.3, 0.4, 0.5},
					},
				})
			default:
				t.Fatalf("unexpected path %q", r.URL.Path)
			}
		}))
		defer server.Close()

		s := &Service{}
		remoteModels, err := s.fetchRemoteModelsViaSDK(context.Background(), sqlc.Provider{
			ClientType: string(models.ClientTypeGoogleGenerativeAI),
			Config:     []byte(`{"base_url":"` + server.URL + `","api_key":"gm-test"}`),
		})
		if err != nil {
			t.Fatalf("fetch remote models: %v", err)
		}
		if len(remoteModels) != 2 {
			t.Fatalf("expected 2 models, got %d", len(remoteModels))
		}
		if remoteModels[0].ID != "gemini-2.0-flash" {
			t.Fatalf("expected models/ prefix stripped, got %q", remoteModels[0].ID)
		}
		if remoteModels[0].Name != "Gemini 2.0 Flash" {
			t.Fatalf("expected display name, got %q", remoteModels[0].Name)
		}
		if remoteModels[0].Type != string(models.ModelTypeChat) {
			t.Fatalf("expected chat type, got %q", remoteModels[0].Type)
		}
		if remoteModels[1].ID != "gemini-embedding-001" {
			t.Fatalf("expected embedding model imported, got %q", remoteModels[1].ID)
		}
		if remoteModels[1].Type != string(models.ModelTypeEmbedding) {
			t.Fatalf("expected embedding type, got %q", remoteModels[1].Type)
		}
		if remoteModels[1].Dimensions == nil || *remoteModels[1].Dimensions != 5 {
			t.Fatalf("expected inferred embedding dimensions 5, got %v", remoteModels[1].Dimensions)
		}
	})

	t.Run("google skips embedding when dimensions probe fails", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/models":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"models": []map[string]any{
						{
							"name":                       "models/gemini-2.0-flash",
							"displayName":                "Gemini 2.0 Flash",
							"supportedGenerationMethods": []string{"generateContent", "countTokens"},
						},
						{
							"name":                       "models/gemini-embedding-001",
							"displayName":                "Gemini Embedding 001",
							"supportedGenerationMethods": []string{"embedContent", "countTokens"},
						},
					},
				})
			case "/models/gemini-embedding-001:embedContent":
				http.Error(w, `{"error":{"message":"quota exceeded"}}`, http.StatusTooManyRequests)
			default:
				t.Fatalf("unexpected path %q", r.URL.Path)
			}
		}))
		defer server.Close()

		s := &Service{}
		remoteModels, err := s.fetchRemoteModelsViaSDK(context.Background(), sqlc.Provider{
			ClientType: string(models.ClientTypeGoogleGenerativeAI),
			Config:     []byte(`{"base_url":"` + server.URL + `","api_key":"gm-test"}`),
		})
		if err != nil {
			t.Fatalf("fetch remote models: %v", err)
		}
		if len(remoteModels) != 1 {
			t.Fatalf("expected only chat model after failed embedding probe, got %d", len(remoteModels))
		}
		if remoteModels[0].ID != "gemini-2.0-flash" {
			t.Fatalf("expected chat model to still import, got %q", remoteModels[0].ID)
		}
	})

	t.Run("openai completions", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/models" {
				t.Fatalf("expected /models path, got %q", r.URL.Path)
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "gpt-4o", "object": "model", "created": 1700000000, "owned_by": "openai"},
					{"id": "text-embedding-ada-002", "object": "model", "created": 1700000000, "owned_by": "openai-internal"},
				},
			})
		}))
		defer server.Close()

		s := &Service{}
		remoteModels, err := s.fetchRemoteModelsViaSDK(context.Background(), sqlc.Provider{
			ClientType: string(models.ClientTypeOpenAICompletions),
			Config:     []byte(`{"base_url":"` + server.URL + `","api_key":"sk-test"}`),
		})
		if err != nil {
			t.Fatalf("fetch remote models: %v", err)
		}
		if len(remoteModels) != 2 {
			t.Fatalf("expected 2 models, got %d", len(remoteModels))
		}
		if remoteModels[0].ID != "gpt-4o" {
			t.Fatalf("expected gpt-4o, got %q", remoteModels[0].ID)
		}
		if remoteModels[0].Name != "gpt-4o" {
			t.Fatalf("expected Name to fall back to ID when DisplayName is empty, got %q", remoteModels[0].Name)
		}
	})
}

func newProviderServiceTestQueries(t *testing.T) (*sql.DB, *sqlitestore.Queries) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	execProviderServiceSchema(t, conn)
	store, err := sqlitestore.New(conn)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	return conn, sqlitestore.NewQueries(store)
}

func execProviderServiceSchema(t *testing.T, conn *sql.DB) {
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
		t.Fatalf("exec provider schema: %v", err)
	}
}
