package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
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
