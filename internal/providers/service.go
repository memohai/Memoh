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
	sdk "github.com/memohai/twilight-ai/sdk"

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

// Create creates a new LLM provider.
func (s *Service) Create(ctx context.Context, req CreateRequest) (GetResponse, error) {
	authType := req.AuthType
	if authType == "" {
		authType = authTypeFromMetadata(req.Metadata)
	}
	if authType == "" {
		authType = AuthTypeAPIKey
	}
	if err := s.validateAuthType(req.BaseURL, req.ClientType, authType); err != nil {
		return GetResponse{}, err
	}

	metadata := s.normalizeProviderMetadata(req.Metadata, authType)
	// Marshal metadata
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return GetResponse{}, fmt.Errorf("marshal metadata: %w", err)
	}

	clientType := req.ClientType
	if clientType == "" {
		clientType = string(models.ClientTypeOpenAICompletions)
	}

	var icon pgtype.Text
	if req.Icon != "" {
		icon = pgtype.Text{String: req.Icon, Valid: true}
	}

	provider, err := s.queries.CreateLlmProvider(ctx, sqlc.CreateLlmProviderParams{
		Name:       req.Name,
		BaseUrl:    req.BaseURL,
		ApiKey:     req.APIKey,
		ClientType: clientType,
		Icon:       icon,
		Enable:     true,
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

	provider, err := s.queries.GetLlmProviderByID(ctx, providerID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("get provider: %w", err)
	}

	return s.toGetResponse(provider), nil
}

// GetByName retrieves a provider by name.
func (s *Service) GetByName(ctx context.Context, name string) (GetResponse, error) {
	provider, err := s.queries.GetLlmProviderByName(ctx, name)
	if err != nil {
		return GetResponse{}, fmt.Errorf("get provider by name: %w", err)
	}

	return s.toGetResponse(provider), nil
}

// List retrieves all providers.
func (s *Service) List(ctx context.Context) ([]GetResponse, error) {
	providers, err := s.queries.ListLlmProviders(ctx)
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

	// Get existing provider
	existing, err := s.queries.GetLlmProviderByID(ctx, providerID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("get provider: %w", err)
	}

	// Apply updates
	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}

	baseURL := existing.BaseUrl
	if req.BaseURL != nil {
		baseURL = *req.BaseURL
	}

	apiKey := resolveUpdatedAPIKey(existing.ApiKey, req.APIKey)

	clientType := existing.ClientType
	if req.ClientType != nil {
		clientType = *req.ClientType
	}

	authType := s.providerAuthType(existing)
	if req.AuthType != nil {
		authType = *req.AuthType
	} else if req.Metadata != nil {
		authType = authTypeFromMetadata(req.Metadata)
	}
	if authType == "" {
		authType = AuthTypeAPIKey
	}
	if err := s.validateAuthType(baseURL, clientType, authType); err != nil {
		return GetResponse{}, err
	}

	icon := existing.Icon
	if req.Icon != nil {
		icon = pgtype.Text{String: *req.Icon, Valid: *req.Icon != ""}
	}

	enable := existing.Enable
	if req.Enable != nil {
		enable = *req.Enable
	}

	metadata := existing.Metadata
	metadataMap := providerMetadata(existing.Metadata)
	if req.Metadata != nil {
		metadataMap = cloneMetadata(req.Metadata)
	}
	metadataMap = s.normalizeProviderMetadata(metadataMap, authType)
	metadataJSON, err := json.Marshal(metadataMap)
	if err != nil {
		return GetResponse{}, fmt.Errorf("marshal metadata: %w", err)
	}
	metadata = metadataJSON

	// Update provider
	updated, err := s.queries.UpdateLlmProvider(ctx, sqlc.UpdateLlmProviderParams{
		ID:         providerID,
		Name:       name,
		BaseUrl:    baseURL,
		ApiKey:     apiKey,
		ClientType: clientType,
		Icon:       icon,
		Enable:     enable,
		Metadata:   metadata,
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

	if err := s.queries.DeleteLlmProvider(ctx, providerID); err != nil {
		return fmt.Errorf("delete provider: %w", err)
	}
	return nil
}

// Count returns the total count of providers.
func (s *Service) Count(ctx context.Context) (int64, error) {
	count, err := s.queries.CountLlmProviders(ctx)
	if err != nil {
		return 0, fmt.Errorf("count providers: %w", err)
	}
	return count, nil
}

const probeTimeout = 5 * time.Second

// Test probes the provider using the Twilight AI SDK to check
// reachability and authentication.
func (s *Service) Test(ctx context.Context, id string) (TestResponse, error) {
	providerID, err := db.ParseUUID(id)
	if err != nil {
		return TestResponse{}, err
	}

	provider, err := s.queries.GetLlmProviderByID(ctx, providerID)
	if err != nil {
		return TestResponse{}, fmt.Errorf("get provider: %w", err)
	}

	baseURL := strings.TrimRight(provider.BaseUrl, "/")

	clientType := models.ClientType(provider.ClientType)
	httpClient := s.AuthHTTPClient(provider, probeTimeout)
	apiKey := provider.ApiKey
	if s.supportsOAuth(provider) {
		apiKey = ""
	}

	sdkProvider := models.NewSDKProvider(baseURL, apiKey, clientType, probeTimeout, httpClient)

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

	provider, err := s.queries.GetLlmProviderByID(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	baseURL := strings.TrimRight(provider.BaseUrl, "/")
	modelsURL := fmt.Sprintf("%s/models", baseURL)

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if provider.ApiKey != "" && !s.supportsOAuth(provider) {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", provider.ApiKey))
	}

	resp, err := s.AuthHTTPClient(provider, probeTimeout).Do(req) //nolint:gosec // G704: URL is from operator-configured LLM provider base URL
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
func (s *Service) toGetResponse(provider sqlc.LlmProvider) GetResponse {
	var metadata map[string]any
	if len(provider.Metadata) > 0 {
		if err := json.Unmarshal(provider.Metadata, &metadata); err != nil {
			if s.logger != nil {
				s.logger.Warn("provider metadata unmarshal failed", slog.String("id", provider.ID.String()), slog.Any("error", err))
			}
		}
	}

	// Mask API key (show only first 8 characters)
	maskedAPIKey := maskAPIKey(provider.ApiKey)

	var icon string
	if provider.Icon.Valid {
		icon = provider.Icon.String
	}

	return GetResponse{
		ID:         provider.ID.String(),
		Name:       provider.Name,
		BaseURL:    provider.BaseUrl,
		APIKey:     maskedAPIKey,
		ClientType: provider.ClientType,
		AuthType:   s.providerAuthType(provider),
		Icon:       icon,
		Enable:     provider.Enable,
		Metadata:   metadata,
		CreatedAt:  provider.CreatedAt.Time,
		UpdatedAt:  provider.UpdatedAt.Time,
	}
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

// resolveUpdatedAPIKey keeps the original key when the request value matches the masked version.
// This prevents masked placeholder values from overwriting the real stored credential.
func resolveUpdatedAPIKey(existing string, updated *string) string {
	if updated == nil {
		return existing
	}
	if *updated == maskAPIKey(existing) {
		return existing
	}
	return *updated
}
