package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	githubcopilot "github.com/memohai/twilight-ai/provider/github/copilot"
	openaicodex "github.com/memohai/twilight-ai/provider/openai/codex"
	sdk "github.com/memohai/twilight-ai/sdk"

	memohcopilot "github.com/memohai/memoh/internal/copilot"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/models"
)

// Service handles provider operations.
type Service struct {
	queries     *sqlc.Queries
	logger      *slog.Logger
	httpClient  *http.Client
	callbackURL string
}

// NewService creates a new provider service.
func NewService(log *slog.Logger, queries *sqlc.Queries, callbackURL string) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries:     queries,
		logger:      log.With(slog.String("service", "providers")),
		httpClient:  &http.Client{Timeout: providerOAuthHTTPTimeout},
		callbackURL: callbackURL,
	}
}

// Create creates a new provider.
func (s *Service) Create(ctx context.Context, req CreateRequest) (GetResponse, error) {
	metadataJSON, err := json.Marshal(req.Metadata)
	if err != nil {
		return GetResponse{}, fmt.Errorf("marshal metadata: %w", err)
	}

	clientType := req.ClientType
	if clientType == "" {
		clientType = string(models.ClientTypeOpenAICompletions)
	}
	configJSON, err := json.Marshal(normalizeProviderConfig(clientType, req.Config))
	if err != nil {
		return GetResponse{}, fmt.Errorf("marshal config: %w", err)
	}

	var icon pgtype.Text
	if req.Icon != "" {
		icon = pgtype.Text{String: req.Icon, Valid: true}
	}

	provider, err := s.queries.CreateProvider(ctx, sqlc.CreateProviderParams{
		Name:       req.Name,
		ClientType: clientType,
		Icon:       icon,
		Enable:     true,
		Config:     configJSON,
		Metadata:   metadataJSON,
	})
	if err != nil {
		return GetResponse{}, fmt.Errorf("create provider: %w", err)
	}

	return s.toGetResponse(provider), nil
}

// Get retrieves a provider by ID.
func (s *Service) Get(ctx context.Context, id string) (GetResponse, error) {
	providerID, err := db.ParseUUID(id)
	if err != nil {
		return GetResponse{}, err
	}

	provider, err := s.queries.GetProviderByID(ctx, providerID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("get provider: %w", err)
	}

	return s.toGetResponse(provider), nil
}

// GetByName retrieves a provider by name.
func (s *Service) GetByName(ctx context.Context, name string) (GetResponse, error) {
	provider, err := s.queries.GetProviderByName(ctx, name)
	if err != nil {
		return GetResponse{}, fmt.Errorf("get provider by name: %w", err)
	}

	return s.toGetResponse(provider), nil
}

// List retrieves all providers.
func (s *Service) List(ctx context.Context) ([]GetResponse, error) {
	providers, err := s.queries.ListProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}

	results := make([]GetResponse, 0, len(providers))
	for _, p := range providers {
		results = append(results, s.toGetResponse(p))
	}
	return results, nil
}

// Update updates an existing provider.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (GetResponse, error) {
	providerID, err := db.ParseUUID(id)
	if err != nil {
		return GetResponse{}, err
	}

	existing, err := s.queries.GetProviderByID(ctx, providerID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("get provider: %w", err)
	}

	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}

	clientType := existing.ClientType
	if req.ClientType != nil {
		clientType = *req.ClientType
	}

	icon := existing.Icon
	if req.Icon != nil {
		icon = pgtype.Text{String: *req.Icon, Valid: *req.Icon != ""}
	}

	enable := existing.Enable
	if req.Enable != nil {
		enable = *req.Enable
	}

	existingConfig := providerConfig(existing.Config)
	if req.Config != nil {
		mergedConfig := mergeProviderConfig(existingConfig, req.Config)
		preserveMaskedConfigSecret(mergedConfig, existingConfig, req.Config, "api_key")
		existingConfig = normalizeProviderConfig(clientType, mergedConfig)
	} else {
		existingConfig = normalizeProviderConfig(clientType, existingConfig)
	}
	configJSON, err := json.Marshal(existingConfig)
	if err != nil {
		return GetResponse{}, fmt.Errorf("marshal config: %w", err)
	}

	metadataMap := providerMetadata(existing.Metadata)
	if req.Metadata != nil {
		metadataMap = req.Metadata
	}
	metadataJSON, err := json.Marshal(metadataMap)
	if err != nil {
		return GetResponse{}, fmt.Errorf("marshal metadata: %w", err)
	}

	updated, err := s.queries.UpdateProvider(ctx, sqlc.UpdateProviderParams{
		ID:         providerID,
		Name:       name,
		ClientType: clientType,
		Icon:       icon,
		Enable:     enable,
		Config:     configJSON,
		Metadata:   metadataJSON,
	})
	if err != nil {
		return GetResponse{}, fmt.Errorf("update provider: %w", err)
	}

	return s.toGetResponse(updated), nil
}

// Delete deletes a provider by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	providerID, err := db.ParseUUID(id)
	if err != nil {
		return err
	}

	if err := s.queries.DeleteProvider(ctx, providerID); err != nil {
		return fmt.Errorf("delete provider: %w", err)
	}
	return nil
}

// Count returns the total count of providers.
func (s *Service) Count(ctx context.Context) (int64, error) {
	count, err := s.queries.CountProviders(ctx)
	if err != nil {
		return 0, fmt.Errorf("count providers: %w", err)
	}
	return count, nil
}

const probeTimeout = models.DefaultProviderProbeTimeout

// Test probes the provider using the Twilight AI SDK to check
// reachability and authentication.
func (s *Service) Test(ctx context.Context, id string) (TestResponse, error) {
	providerID, err := db.ParseUUID(id)
	if err != nil {
		return TestResponse{}, err
	}

	provider, err := s.queries.GetProviderByID(ctx, providerID)
	if err != nil {
		return TestResponse{}, fmt.Errorf("get provider: %w", err)
	}

	cfg := providerConfig(provider.Config)
	baseURL := strings.TrimRight(configString(cfg, "base_url"), "/")

	clientType := models.ClientType(provider.ClientType)
	creds, err := s.ResolveModelCredentials(ctx, provider)
	if err != nil {
		return TestResponse{}, err
	}

	sdkProvider := models.NewSDKProvider(baseURL, creds.APIKey, creds.CodexAccountID, clientType, probeTimeout, nil)

	start := time.Now()
	result := sdkProvider.Test(ctx)
	latency := time.Since(start).Milliseconds()

	return TestResponse{
		Reachable: result.Status != sdk.ProviderStatusUnreachable,
		LatencyMs: latency,
		Message:   result.Message,
	}, nil
}

// FetchRemoteModels fetches models from the provider's /v1/models endpoint.
func (s *Service) FetchRemoteModels(ctx context.Context, id string) ([]RemoteModel, error) {
	providerID, err := db.ParseUUID(id)
	if err != nil {
		return nil, err
	}

	provider, err := s.queries.GetProviderByID(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}
	if models.ClientType(provider.ClientType) == models.ClientTypeGitHubCopilot {
		creds, err := s.ResolveModelCredentials(ctx, provider)
		if err != nil {
			return nil, err
		}
		sdkProvider := memohcopilot.NewProvider(creds.APIKey, nil)
		if result := sdkProvider.Test(ctx); result.Status != sdk.ProviderStatusOK {
			return nil, fmt.Errorf("github copilot provider test failed: %s", result.Message)
		}

		catalog := githubcopilot.Catalog()
		remoteModels := make([]RemoteModel, 0, len(catalog))
		for _, model := range catalog {
			remoteModels = append(remoteModels, RemoteModel{
				ID:      model.ID,
				Name:    model.DisplayName,
				Object:  "model",
				OwnedBy: "github-copilot",
				Type:    "chat",
				Compatibilities: []string{
					models.CompatVision,
					models.CompatToolCall,
					models.CompatReasoning,
				},
			})
		}
		return remoteModels, nil
	}
	if supportsOAuth(provider) {
		catalog := openaicodex.Catalog()
		remoteModels := make([]RemoteModel, 0, len(catalog))
		for _, model := range catalog {
			compatibilities := make([]string, 0, 2)
			if model.SupportsToolCall {
				compatibilities = append(compatibilities, models.CompatToolCall)
			}
			if model.SupportsReasoning {
				compatibilities = append(compatibilities, models.CompatReasoning)
			}
			remoteModels = append(remoteModels, RemoteModel{
				ID:               model.ID,
				Name:             model.DisplayName,
				Object:           "model",
				OwnedBy:          "openai-codex",
				Type:             "chat",
				Compatibilities:  compatibilities,
				ReasoningEfforts: append([]string(nil), model.ReasoningEfforts...),
			})
		}
		return remoteModels, nil
	}

	cfg := providerConfig(provider.Config)
	baseURL := strings.TrimRight(configString(cfg, "base_url"), "/")
	apiKey := configString(cfg, "api_key")
	modelsURL := fmt.Sprintf("%s/models", baseURL)

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if apiKey != "" && !supportsOAuth(provider) {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	}

	resp, err := models.NewProviderHTTPClient(probeTimeout).Do(req) //nolint:gosec // G704: URL is from operator-configured LLM provider base URL
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var fetchResp FetchModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&fetchResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return fetchResp.Data, nil
}

// toGetResponse converts a database provider to a response.
func (s *Service) toGetResponse(provider sqlc.Provider) GetResponse {
	var metadata map[string]any
	if len(provider.Metadata) > 0 {
		if err := json.Unmarshal(provider.Metadata, &metadata); err != nil {
			if s.logger != nil {
				s.logger.Warn("provider metadata unmarshal failed", slog.String("id", provider.ID.String()), slog.Any("error", err))
			}
		}
	}

	cfg := providerConfig(provider.Config)
	maskedCfg := maskConfigSecrets(provider.ClientType, cfg)

	var icon string
	if provider.Icon.Valid {
		icon = provider.Icon.String
	}

	return GetResponse{
		ID:         provider.ID.String(),
		Name:       provider.Name,
		ClientType: provider.ClientType,
		Icon:       icon,
		Enable:     provider.Enable,
		Config:     maskedCfg,
		Metadata:   metadata,
		CreatedAt:  provider.CreatedAt.Time,
		UpdatedAt:  provider.UpdatedAt.Time,
	}
}

// providerConfig parses the provider config JSONB.
func providerConfig(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return map[string]any{}
	}
	if cfg == nil {
		return map[string]any{}
	}
	return cfg
}

// configString extracts a string from the config map.
func configString(cfg map[string]any, key string) string {
	if cfg == nil {
		return ""
	}
	v, _ := cfg[key].(string)
	return v
}

// ProviderConfigString is a public helper for extracting a string from the config JSONB.
func ProviderConfigString(provider sqlc.Provider, key string) string {
	return configString(providerConfig(provider.Config), key)
}

func cloneConfig(cfg map[string]any) map[string]any {
	result := make(map[string]any, len(cfg))
	for k, v := range cfg {
		result[k] = v
	}
	return result
}

func mergeProviderConfig(existing, incoming map[string]any) map[string]any {
	result := cloneConfig(existing)
	for k, v := range incoming {
		result[k] = v
	}
	return result
}

func preserveMaskedConfigSecret(merged, existing, incoming map[string]any, key string) {
	existingValue := strings.TrimSpace(configString(existing, key))
	newValue := strings.TrimSpace(configString(incoming, key))
	if existingValue == "" || newValue == "" {
		return
	}
	if newValue == maskAPIKey(existingValue) {
		merged[key] = existingValue
	}
}

// normalizeProviderConfig keeps provider-specific secrets under stable keys while
// preserving backward compatibility for legacy stored configs.
func normalizeProviderConfig(clientType string, cfg map[string]any) map[string]any {
	result := cloneConfig(cfg)
	if models.ClientType(clientType) == models.ClientTypeGitHubCopilot {
		delete(result, "api_key")
		delete(result, configOAuthClientSecretKey)
	}
	return result
}

// maskConfigSecrets returns a copy of config with all known secret fields masked.
func maskConfigSecrets(clientType string, cfg map[string]any) map[string]any {
	result := normalizeProviderConfig(clientType, cfg)
	for _, key := range []string{"api_key", configOAuthClientSecretKey} {
		if value, _ := result[key].(string); value != "" {
			result[key] = maskAPIKey(value)
		}
	}
	return result
}

// maskAPIKey masks an API key for security.
func maskAPIKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	if len(apiKey) <= 8 {
		return strings.Repeat("*", len(apiKey))
	}
	return apiKey[:8] + strings.Repeat("*", len(apiKey)-8)
}
