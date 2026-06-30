package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/registry"
)

// Service handles provider operations.
type Service struct {
	queries      dbstore.Queries
	logger       *slog.Logger
	httpClient   *http.Client
	callbackURL  string
	templatesDir string
}

// NewService creates a new provider service.
func NewService(log *slog.Logger, queries dbstore.Queries, callbackURL string, templatesDir ...string) *Service {
	if log == nil {
		log = slog.Default()
	}
	var dir string
	if len(templatesDir) > 0 {
		dir = templatesDir[0]
	}
	return &Service{
		queries:      queries,
		logger:       log.With(slog.String("service", "providers")),
		httpClient:   &http.Client{Timeout: providerOAuthHTTPTimeout},
		callbackURL:  callbackURL,
		templatesDir: dir,
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
		if isProviderNameConflict(err) {
			if provider, ok, activateErr := s.activateHiddenRegistryTemplate(ctx, req, clientType, icon, configJSON, metadataJSON); ok {
				if activateErr != nil {
					return GetResponse{}, activateErr
				}
				return s.toGetResponse(provider), nil
			}
		}
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
	providers, err := s.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("count providers: %w", err)
	}
	return int64(len(providers)), nil
}

const probeTimeout = models.DefaultProviderProbeTimeout

const (
	registryMetadataKey = "registry"
	metadataSourceKey   = "source"
)

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
	message := providerTestMessage(result)

	switch result.Status {
	case sdk.ProviderStatusUnreachable:
		return TestResponse{
			Status:    TestStatusError,
			Reachable: false,
			LatencyMs: time.Since(start).Milliseconds(),
			Message:   message,
		}, nil
	case sdk.ProviderStatusUnhealthy:
		status := TestStatusError
		if strings.Contains(result.Message, "authentication failed") {
			status = TestStatusAuthError
		}
		return TestResponse{
			Status:    status,
			Reachable: true,
			LatencyMs: time.Since(start).Milliseconds(),
			Message:   message,
		}, nil
	default:
		if _, probeErr := sdkProvider.TestModel(ctx, "__ping__"); probeErr != nil {
			if strings.Contains(probeErr.Error(), "authentication failed") {
				return TestResponse{
					Status:    TestStatusAuthError,
					Reachable: true,
					LatencyMs: time.Since(start).Milliseconds(),
					Message:   probeErr.Error(),
				}, nil
			}
		}
		return TestResponse{
			Status:    TestStatusOK,
			Reachable: true,
			LatencyMs: time.Since(start).Milliseconds(),
			Message:   result.Message,
		}, nil
	}
}

// errorDetailer is implemented by transport errors that can expand into a
// fuller diagnostic, including the raw upstream response body. The probe path
// only fills a short summary (e.g. "service error (404):"), so we reach for
// this richer detail when the upstream replies with an opaque, non-JSON body.
type errorDetailer interface {
	Detail() string
}

// providerTestMessage returns the most informative message for a probe result,
// preferring the upstream response detail over the short summary so that
// opaque statuses still surface the provider's actual response body.
func providerTestMessage(result *sdk.ProviderTestResult) string {
	if result == nil {
		return ""
	}
	var detailer errorDetailer
	if errors.As(result.Error, &detailer) {
		if detail := strings.TrimSpace(detailer.Detail()); detail != "" {
			return detail
		}
	}
	return result.Message
}

// FetchRemoteModels fetches available models from the provider using the Twilight AI SDK.
func (s *Service) FetchRemoteModels(ctx context.Context, id string) ([]RemoteModel, error) {
	providerID, err := db.ParseUUID(id)
	if err != nil {
		return nil, err
	}

	provider, err := s.queries.GetProviderByID(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	if models, ok := s.fetchTemplateModels(provider); ok {
		return models, nil
	}

	var remoteModels []RemoteModel
	switch {
	case models.ClientType(provider.ClientType) == models.ClientTypeGitHubCopilot:
		remoteModels, err = s.fetchGitHubCopilotModels(ctx, provider)
	case supportsOAuth(provider):
		remoteModels = fetchCodexCatalogModels()
	default:
		remoteModels, err = s.fetchRemoteModelsViaSDK(ctx, provider)
	}
	if err != nil {
		return nil, err
	}

	return remoteModels, nil
}

func (s *Service) fetchTemplateModels(provider sqlc.Provider) ([]RemoteModel, bool) {
	source := metadataSectionSource(providerMetadata(provider.Metadata), "preset")
	if source == "" {
		return nil, false
	}
	source = strings.ToLower(strings.TrimSpace(source))
	if strings.TrimSpace(s.templatesDir) == "" {
		return nil, false
	}

	defs, err := registry.Load(s.logger, s.templatesDir)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to load provider template models", slog.String("template_source", source), slog.Any("error", err))
		}
		return nil, false
	}
	for _, def := range defs {
		if strings.EqualFold(def.Source, source) {
			return remoteModelsFromTemplate(def), true
		}
	}
	return nil, false
}

func remoteModelsFromTemplate(def registry.ProviderDefinition) []RemoteModel {
	out := make([]RemoteModel, 0, len(def.Models))
	for _, model := range def.Models {
		modelType := strings.TrimSpace(model.Type)
		if modelType == "" {
			modelType = string(models.ModelTypeChat)
		}
		cfg := model.Config
		out = append(out, RemoteModel{
			ID:               model.ModelID,
			Name:             model.Name,
			Type:             modelType,
			Compatibilities:  configStringSlice(cfg, "compatibilities"),
			ReasoningEfforts: configStringSlice(cfg, "reasoning_efforts"),
			ThinkingMode:     configString(cfg, "thinking_mode"),
			ContextWindow:    configIntPtr(cfg, "context_window"),
			Dimensions:       configIntPtr(cfg, "dimensions"),
		})
	}
	return out
}

func (s *Service) fetchGitHubCopilotModels(ctx context.Context, provider sqlc.Provider) ([]RemoteModel, error) {
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

func fetchCodexCatalogModels() []RemoteModel {
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
	return remoteModels
}

func (s *Service) fetchRemoteModelsViaSDK(ctx context.Context, provider sqlc.Provider) ([]RemoteModel, error) {
	cfg := providerConfig(provider.Config)
	baseURL := strings.TrimRight(configString(cfg, "base_url"), "/")
	clientType := models.ClientType(provider.ClientType)

	if clientType == models.ClientTypeAnthropicMessages && baseURL != "" && !strings.HasSuffix(baseURL, "/v1") {
		baseURL += "/v1"
	}

	creds, err := s.ResolveModelCredentials(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("resolve credentials: %w", err)
	}

	sdkProvider := models.NewSDKProvider(baseURL, creds.APIKey, creds.CodexAccountID, clientType, probeTimeout, nil)

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	sdkModels, err := sdkProvider.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}

	remoteModels := make([]RemoteModel, 0, len(sdkModels))
	for _, m := range sdkModels {
		modelType := m.Type
		if modelType == "" {
			modelType = sdk.ModelTypeChat
		}
		if modelType != sdk.ModelTypeChat && modelType != sdk.ModelTypeEmbedding {
			continue
		}
		name := m.DisplayName
		if name == "" {
			name = m.ID
		}
		var dimensions *int
		if modelType == sdk.ModelTypeEmbedding {
			dim, err := models.InferEmbeddingDimensions(ctx, string(clientType), baseURL, creds.APIKey, m.ID, probeTimeout, nil)
			if err != nil {
				logger := s.logger
				if logger == nil {
					logger = slog.Default()
				}
				logger.Warn("skip embedding model import because dimensions probe failed", slog.String("model_id", m.ID), slog.Any("error", err))
				continue
			}
			dimensions = &dim
		}
		remoteModels = append(remoteModels, RemoteModel{
			ID:         m.ID,
			Name:       name,
			Type:       string(modelType),
			Dimensions: dimensions,
		})
	}
	return remoteModels, nil
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

func configStringSlice(cfg map[string]any, key string) []string {
	if cfg == nil {
		return nil
	}
	switch value := cfg[key].(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func configIntPtr(cfg map[string]any, key string) *int {
	if cfg == nil {
		return nil
	}
	switch value := cfg[key].(type) {
	case int:
		if value > 0 {
			return &value
		}
	case int64:
		if value > 0 {
			out := int(value)
			return &out
		}
	case float64:
		if value > 0 {
			out := int(value)
			return &out
		}
	}
	return nil
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

func (s *Service) activateHiddenRegistryTemplate(
	ctx context.Context,
	req CreateRequest,
	clientType string,
	icon pgtype.Text,
	configJSON []byte,
	metadataJSON []byte,
) (sqlc.Provider, bool, error) {
	existing, err := s.queries.GetProviderByName(ctx, req.Name)
	if err != nil {
		return sqlc.Provider{}, false, nil
	}
	if !isHiddenRegistryTemplate(existing) {
		return sqlc.Provider{}, false, nil
	}
	if !icon.Valid {
		icon = existing.Icon
	}

	updated, err := s.queries.UpdateProvider(ctx, sqlc.UpdateProviderParams{
		ID:         existing.ID,
		Name:       req.Name,
		ClientType: clientType,
		Icon:       icon,
		Enable:     true,
		Config:     configJSON,
		Metadata:   metadataJSON,
	})
	if err != nil {
		return sqlc.Provider{}, true, fmt.Errorf("activate registry provider template: %w", err)
	}
	return updated, true, nil
}

func isProviderNameConflict(err error) bool {
	if db.IsUniqueViolation(err) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") &&
		strings.Contains(message, "providers") &&
		strings.Contains(message, "name")
}

func isHiddenRegistryTemplate(provider sqlc.Provider) bool {
	if provider.Enable || registryMetadataSource(provider.Metadata) == "" {
		return false
	}
	cfg := providerConfig(provider.Config)
	return strings.TrimSpace(configString(cfg, "api_key")) == "" &&
		strings.TrimSpace(configString(cfg, configOAuthClientSecretKey)) == ""
}

func registryMetadataSource(raw []byte) string {
	return metadataSectionSource(providerMetadata(raw), registryMetadataKey)
}

func metadataSectionSource(metadata map[string]any, section string) string {
	nested, _ := metadata[section].(map[string]any)
	if nested == nil {
		return ""
	}
	return strings.TrimSpace(stringValue(nested, metadataSourceKey))
}
