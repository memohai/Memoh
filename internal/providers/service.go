package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/sqlc"
)

// Service handles provider operations
type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

// NewService creates a new provider service
func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "providers")),
	}
}

// Create creates a new LLM provider
func (s *Service) Create(ctx context.Context, req CreateRequest) (GetResponse, error) {
	// Validate client type
	if !isValidClientType(req.ClientType) {
		return GetResponse{}, fmt.Errorf("invalid client_type: %s", req.ClientType)
	}

	// Marshal metadata
	metadataJSON, err := json.Marshal(req.Metadata)
	if err != nil {
		return GetResponse{}, fmt.Errorf("marshal metadata: %w", err)
	}

	// Create provider
	provider, err := s.queries.CreateLlmProvider(ctx, sqlc.CreateLlmProviderParams{
		Name:       req.Name,
		ClientType: string(req.ClientType),
		BaseUrl:    req.BaseURL,
		ApiKey:     req.APIKey,
		Metadata:   metadataJSON,
	})
	if err != nil {
		return GetResponse{}, fmt.Errorf("create provider: %w", err)
	}

	return s.toGetResponse(provider), nil
}

// Get retrieves a provider by ID
func (s *Service) Get(ctx context.Context, id string) (GetResponse, error) {
	providerID, err := parseUUID(id)
	if err != nil {
		return GetResponse{}, err
	}

	provider, err := s.queries.GetLlmProviderByID(ctx, providerID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("get provider: %w", err)
	}

	return s.toGetResponse(provider), nil
}

// GetByName retrieves a provider by name
func (s *Service) GetByName(ctx context.Context, name string) (GetResponse, error) {
	provider, err := s.queries.GetLlmProviderByName(ctx, name)
	if err != nil {
		return GetResponse{}, fmt.Errorf("get provider by name: %w", err)
	}

	return s.toGetResponse(provider), nil
}

// List retrieves all providers
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

// ListByClientType retrieves providers by client type
func (s *Service) ListByClientType(ctx context.Context, clientType ClientType) ([]GetResponse, error) {
	if !isValidClientType(clientType) {
		return nil, fmt.Errorf("invalid client_type: %s", clientType)
	}

	providers, err := s.queries.ListLlmProvidersByClientType(ctx, string(clientType))
	if err != nil {
		return nil, fmt.Errorf("list providers by client type: %w", err)
	}

	results := make([]GetResponse, 0, len(providers))
	for _, p := range providers {
		results = append(results, s.toGetResponse(p))
	}
	return results, nil
}

// Update updates an existing provider
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (GetResponse, error) {
	providerID, err := parseUUID(id)
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

	clientType := existing.ClientType
	if req.ClientType != nil {
		if !isValidClientType(*req.ClientType) {
			return GetResponse{}, fmt.Errorf("invalid client_type: %s", *req.ClientType)
		}
		clientType = string(*req.ClientType)
	}

	baseURL := existing.BaseUrl
	if req.BaseURL != nil {
		baseURL = *req.BaseURL
	}

	apiKey := existing.ApiKey
	if req.APIKey != nil {
		apiKey = *req.APIKey
	}

	metadata := existing.Metadata
	if req.Metadata != nil {
		metadataJSON, err := json.Marshal(req.Metadata)
		if err != nil {
			return GetResponse{}, fmt.Errorf("marshal metadata: %w", err)
		}
		metadata = metadataJSON
	}

	// Update provider
	updated, err := s.queries.UpdateLlmProvider(ctx, sqlc.UpdateLlmProviderParams{
		ID:         providerID,
		Name:       name,
		ClientType: clientType,
		BaseUrl:    baseURL,
		ApiKey:     apiKey,
		Metadata:   metadata,
	})
	if err != nil {
		return GetResponse{}, fmt.Errorf("update provider: %w", err)
	}

	return s.toGetResponse(updated), nil
}

// Delete deletes a provider by ID
func (s *Service) Delete(ctx context.Context, id string) error {
	providerID, err := parseUUID(id)
	if err != nil {
		return err
	}

	if err := s.queries.DeleteLlmProvider(ctx, providerID); err != nil {
		return fmt.Errorf("delete provider: %w", err)
	}
	return nil
}

// Count returns the total count of providers
func (s *Service) Count(ctx context.Context) (int64, error) {
	count, err := s.queries.CountLlmProviders(ctx)
	if err != nil {
		return 0, fmt.Errorf("count providers: %w", err)
	}
	return count, nil
}

// CountByClientType returns the count of providers by client type
func (s *Service) CountByClientType(ctx context.Context, clientType ClientType) (int64, error) {
	if !isValidClientType(clientType) {
		return 0, fmt.Errorf("invalid client_type: %s", clientType)
	}

	count, err := s.queries.CountLlmProvidersByClientType(ctx, string(clientType))
	if err != nil {
		return 0, fmt.Errorf("count providers by client type: %w", err)
	}
	return count, nil
}

// toGetResponse converts a database provider to a response
func (s *Service) toGetResponse(provider sqlc.LlmProvider) GetResponse {
	var metadata map[string]any
	if len(provider.Metadata) > 0 {
		_ = json.Unmarshal(provider.Metadata, &metadata)
	}

	// Mask API key (show only first 8 characters)
	maskedAPIKey := maskAPIKey(provider.ApiKey)

	// Convert pgtype.UUID to string
	var id [16]byte
	copy(id[:], provider.ID.Bytes[:])
	idUUID := uuid.UUID(id)

	return GetResponse{
		ID:         idUUID.String(),
		Name:       provider.Name,
		ClientType: provider.ClientType,
		BaseURL:    provider.BaseUrl,
		APIKey:     maskedAPIKey,
		Metadata:   metadata,
		CreatedAt:  provider.CreatedAt.Time,
		UpdatedAt:  provider.UpdatedAt.Time,
	}
}

// parseUUID parses a UUID string
func parseUUID(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID: %w", err)
	}
	var pgID pgtype.UUID
	pgID.Valid = true
	copy(pgID.Bytes[:], parsed[:])
	return pgID, nil
}

// isValidClientType checks if a client type is valid
func isValidClientType(clientType ClientType) bool {
	switch clientType {
	case ClientTypeOpenAI, ClientTypeOpenAICompat, ClientTypeAnthropic, ClientTypeGoogle, ClientTypeOllama:
		return true
	default:
		return false
	}
}

// maskAPIKey masks an API key for security
func maskAPIKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	if len(apiKey) <= 8 {
		return strings.Repeat("*", len(apiKey))
	}
	return apiKey[:8] + strings.Repeat("*", len(apiKey)-8)
}
