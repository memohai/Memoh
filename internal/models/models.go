package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

var (
	ErrModelIDAlreadyExists = errors.New("model_id already exists")
	ErrModelIDAmbiguous     = errors.New("model_id is ambiguous across providers")
)

// Service provides CRUD operations for models.
type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

// NewService creates a new models service.
func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "models")),
	}
}

// Create adds a new model to the database.
func (s *Service) Create(ctx context.Context, req AddRequest) (AddResponse, error) {
	model := Model(req)
	if err := model.Validate(); err != nil {
		return AddResponse{}, fmt.Errorf("validation failed: %w", err)
	}

	providerID, err := db.ParseUUID(model.ProviderID)
	if err != nil {
		return AddResponse{}, fmt.Errorf("invalid provider ID: %w", err)
	}

	configJSON, err := json.Marshal(model.Config)
	if err != nil {
		return AddResponse{}, fmt.Errorf("marshal config: %w", err)
	}

	params := sqlc.CreateModelParams{
		ModelID:    model.ModelID,
		ProviderID: providerID,
		Type:       string(model.Type),
		Config:     configJSON,
	}

	if model.Name != "" {
		params.Name = pgtype.Text{String: model.Name, Valid: true}
	}

	created, err := s.queries.CreateModel(ctx, params)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return AddResponse{}, ErrModelIDAlreadyExists
		}
		return AddResponse{}, fmt.Errorf("failed to create model: %w", err)
	}

	var idStr string
	if created.ID.Valid {
		id, err := uuid.FromBytes(created.ID.Bytes[:])
		if err != nil {
			return AddResponse{}, fmt.Errorf("failed to convert UUID: %w", err)
		}
		idStr = id.String()
	}

	return AddResponse{
		ID:      idStr,
		ModelID: created.ModelID,
	}, nil
}

// GetByID retrieves a model by its internal UUID.
func (s *Service) GetByID(ctx context.Context, id string) (GetResponse, error) {
	uuid, err := db.ParseUUID(id)
	if err != nil {
		return GetResponse{}, fmt.Errorf("invalid ID: %w", err)
	}

	dbModel, err := s.queries.GetModelByID(ctx, uuid)
	if err != nil {
		return GetResponse{}, fmt.Errorf("failed to get model: %w", err)
	}

	return s.convertToGetResponse(dbModel), nil
}

// GetByModelID retrieves a model by its model_id field.
func (s *Service) GetByModelID(ctx context.Context, modelID string) (GetResponse, error) {
	if modelID == "" {
		return GetResponse{}, errors.New("model_id is required")
	}

	dbModel, err := s.findUniqueByModelID(ctx, modelID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("failed to get model: %w", err)
	}

	return s.convertToGetResponse(dbModel), nil
}

// List returns all models.
func (s *Service) List(ctx context.Context) ([]GetResponse, error) {
	dbModels, err := s.queries.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	return s.convertToGetResponseList(dbModels), nil
}

// ListByType returns models filtered by type (chat, embedding, or speech).
func (s *Service) ListByType(ctx context.Context, modelType ModelType) ([]GetResponse, error) {
	if modelType != ModelTypeChat && modelType != ModelTypeEmbedding && modelType != ModelTypeSpeech {
		return nil, fmt.Errorf("invalid model type: %s", modelType)
	}

	dbModels, err := s.queries.ListModelsByType(ctx, string(modelType))
	if err != nil {
		return nil, fmt.Errorf("failed to list models by type: %w", err)
	}

	return s.convertToGetResponseList(dbModels), nil
}

// ListByProviderClientType returns models whose provider has the given client_type.
func (s *Service) ListByProviderClientType(ctx context.Context, clientType ClientType) ([]GetResponse, error) {
	if !IsValidClientType(clientType) {
		return nil, fmt.Errorf("invalid client type: %s", clientType)
	}

	dbModels, err := s.queries.ListModelsByProviderClientType(ctx, string(clientType))
	if err != nil {
		return nil, fmt.Errorf("failed to list models by provider client type: %w", err)
	}

	return s.convertToGetResponseList(dbModels), nil
}

// ListEnabled returns all models from enabled providers.
func (s *Service) ListEnabled(ctx context.Context) ([]GetResponse, error) {
	dbModels, err := s.queries.ListEnabledModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled models: %w", err)
	}
	return s.convertToGetResponseList(dbModels), nil
}

// ListEnabledByType returns models from enabled providers filtered by type.
func (s *Service) ListEnabledByType(ctx context.Context, modelType ModelType) ([]GetResponse, error) {
	if modelType != ModelTypeChat && modelType != ModelTypeEmbedding && modelType != ModelTypeSpeech {
		return nil, fmt.Errorf("invalid model type: %s", modelType)
	}
	dbModels, err := s.queries.ListEnabledModelsByType(ctx, string(modelType))
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled models by type: %w", err)
	}
	return s.convertToGetResponseList(dbModels), nil
}

// ListEnabledByProviderClientType returns models from enabled providers with
// the given client_type.
func (s *Service) ListEnabledByProviderClientType(ctx context.Context, clientType ClientType) ([]GetResponse, error) {
	if !IsValidClientType(clientType) {
		return nil, fmt.Errorf("invalid client type: %s", clientType)
	}
	dbModels, err := s.queries.ListEnabledModelsByProviderClientType(ctx, string(clientType))
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled models by provider client type: %w", err)
	}
	return s.convertToGetResponseList(dbModels), nil
}

// ListByProviderID returns models filtered by provider ID.
func (s *Service) ListByProviderID(ctx context.Context, providerID string) ([]GetResponse, error) {
	if strings.TrimSpace(providerID) == "" {
		return nil, errors.New("provider id is required")
	}
	uuid, err := db.ParseUUID(providerID)
	if err != nil {
		return nil, fmt.Errorf("invalid provider id: %w", err)
	}
	dbModels, err := s.queries.ListModelsByProviderID(ctx, uuid)
	if err != nil {
		return nil, fmt.Errorf("failed to list models by provider: %w", err)
	}
	return s.convertToGetResponseList(dbModels), nil
}

// ListByProviderIDAndType returns models filtered by provider ID and type.
func (s *Service) ListByProviderIDAndType(ctx context.Context, providerID string, modelType ModelType) ([]GetResponse, error) {
	if modelType != ModelTypeChat && modelType != ModelTypeEmbedding && modelType != ModelTypeSpeech {
		return nil, fmt.Errorf("invalid model type: %s", modelType)
	}
	if strings.TrimSpace(providerID) == "" {
		return nil, errors.New("provider id is required")
	}
	uuid, err := db.ParseUUID(providerID)
	if err != nil {
		return nil, fmt.Errorf("invalid provider id: %w", err)
	}
	dbModels, err := s.queries.ListModelsByProviderIDAndType(ctx, sqlc.ListModelsByProviderIDAndTypeParams{
		ProviderID: uuid,
		Type:       string(modelType),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list models by provider and type: %w", err)
	}
	return s.convertToGetResponseList(dbModels), nil
}

// UpdateByID updates a model by its internal UUID.
func (s *Service) UpdateByID(ctx context.Context, id string, req UpdateRequest) (GetResponse, error) {
	uuid, err := db.ParseUUID(id)
	if err != nil {
		return GetResponse{}, fmt.Errorf("invalid ID: %w", err)
	}

	model := Model(req)
	if err := model.Validate(); err != nil {
		return GetResponse{}, fmt.Errorf("validation failed: %w", err)
	}

	providerID, err := db.ParseUUID(model.ProviderID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("invalid provider ID: %w", err)
	}

	configJSON, err := json.Marshal(model.Config)
	if err != nil {
		return GetResponse{}, fmt.Errorf("marshal config: %w", err)
	}

	params := sqlc.UpdateModelParams{
		ID:         uuid,
		ModelID:    model.ModelID,
		ProviderID: providerID,
		Type:       string(model.Type),
		Config:     configJSON,
	}

	if model.Name != "" {
		params.Name = pgtype.Text{String: model.Name, Valid: true}
	}

	updated, err := s.queries.UpdateModel(ctx, params)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return GetResponse{}, ErrModelIDAlreadyExists
		}
		return GetResponse{}, fmt.Errorf("failed to update model: %w", err)
	}

	return s.convertToGetResponse(updated), nil
}

// UpdateByModelID updates a model by its model_id field.
func (s *Service) UpdateByModelID(ctx context.Context, modelID string, req UpdateRequest) (GetResponse, error) {
	if modelID == "" {
		return GetResponse{}, errors.New("model_id is required")
	}
	current, err := s.findUniqueByModelID(ctx, modelID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("failed to update model: %w", err)
	}

	model := Model(req)
	if err := model.Validate(); err != nil {
		return GetResponse{}, fmt.Errorf("validation failed: %w", err)
	}

	providerID, err := db.ParseUUID(model.ProviderID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("invalid provider ID: %w", err)
	}

	configJSON, err := json.Marshal(model.Config)
	if err != nil {
		return GetResponse{}, fmt.Errorf("marshal config: %w", err)
	}

	params := sqlc.UpdateModelParams{
		ID:         current.ID,
		ModelID:    model.ModelID,
		ProviderID: providerID,
		Type:       string(model.Type),
		Config:     configJSON,
	}

	if model.Name != "" {
		params.Name = pgtype.Text{String: model.Name, Valid: true}
	}

	updated, err := s.queries.UpdateModel(ctx, params)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return GetResponse{}, ErrModelIDAlreadyExists
		}
		return GetResponse{}, fmt.Errorf("failed to update model: %w", err)
	}

	return s.convertToGetResponse(updated), nil
}

// DeleteByID deletes a model by its internal UUID.
func (s *Service) DeleteByID(ctx context.Context, id string) error {
	uuid, err := db.ParseUUID(id)
	if err != nil {
		return fmt.Errorf("invalid ID: %w", err)
	}

	if err := s.queries.DeleteModel(ctx, uuid); err != nil {
		return fmt.Errorf("failed to delete model: %w", err)
	}

	return nil
}

// DeleteByModelID deletes a model by its model_id field.
func (s *Service) DeleteByModelID(ctx context.Context, modelID string) error {
	if modelID == "" {
		return errors.New("model_id is required")
	}
	current, err := s.findUniqueByModelID(ctx, modelID)
	if err != nil {
		return fmt.Errorf("failed to delete model: %w", err)
	}

	if err := s.queries.DeleteModel(ctx, current.ID); err != nil {
		return fmt.Errorf("failed to delete model: %w", err)
	}

	return nil
}

// Count returns the total number of models.
func (s *Service) Count(ctx context.Context) (int64, error) {
	count, err := s.queries.CountModels(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count models: %w", err)
	}
	return count, nil
}

// CountByType returns the number of models of a specific type.
func (s *Service) CountByType(ctx context.Context, modelType ModelType) (int64, error) {
	if modelType != ModelTypeChat && modelType != ModelTypeEmbedding && modelType != ModelTypeSpeech {
		return 0, fmt.Errorf("invalid model type: %s", modelType)
	}

	count, err := s.queries.CountModelsByType(ctx, string(modelType))
	if err != nil {
		return 0, fmt.Errorf("failed to count models by type: %w", err)
	}
	return count, nil
}

func (s *Service) convertToGetResponse(dbModel sqlc.Model) GetResponse {
	resp := GetResponse{
		ID:      dbModel.ID.String(),
		ModelID: dbModel.ModelID,
		Model: Model{
			ModelID: dbModel.ModelID,
			Type:    ModelType(dbModel.Type),
		},
	}

	if dbModel.ProviderID.Valid {
		resp.ProviderID = dbModel.ProviderID.String()
	}

	if dbModel.Name.Valid {
		resp.Name = dbModel.Name.String
	}

	if len(dbModel.Config) > 0 {
		if err := json.Unmarshal(dbModel.Config, &resp.Config); err != nil {
			s.logger.Warn("failed to unmarshal model config", slog.String("model_id", dbModel.ModelID), slog.Any("error", err))
		}
	}

	return resp
}

func (s *Service) convertToGetResponseList(dbModels []sqlc.Model) []GetResponse {
	responses := make([]GetResponse, 0, len(dbModels))
	for _, dbModel := range dbModels {
		responses = append(responses, s.convertToGetResponse(dbModel))
	}
	return responses
}

func (s *Service) findUniqueByModelID(ctx context.Context, modelID string) (sqlc.Model, error) {
	rows, err := s.queries.ListModelsByModelID(ctx, modelID)
	if err != nil {
		return sqlc.Model{}, err
	}
	if len(rows) == 0 {
		return sqlc.Model{}, pgx.ErrNoRows
	}
	if len(rows) > 1 {
		return sqlc.Model{}, ErrModelIDAmbiguous
	}
	return rows[0], nil
}

// IsValidClientType returns true if the given client type is supported.
func IsValidClientType(clientType ClientType) bool {
	switch clientType {
	case ClientTypeOpenAIResponses,
		ClientTypeOpenAICompletions,
		ClientTypeAnthropicMessages,
		ClientTypeGoogleGenerativeAI,
		ClientTypeOpenAICodex,
		ClientTypeGitHubCopilot,
		ClientTypeEdgeSpeech:
		return true
	default:
		return false
	}
}

// SelectMemoryModel selects a chat model for memory operations.
// It only considers models from enabled providers.
func SelectMemoryModel(ctx context.Context, modelsService *Service, queries *sqlc.Queries) (GetResponse, sqlc.Provider, error) {
	if modelsService == nil {
		return GetResponse{}, sqlc.Provider{}, errors.New("models service not configured")
	}
	if queries == nil {
		return GetResponse{}, sqlc.Provider{}, errors.New("queries not configured")
	}
	candidates, err := modelsService.ListEnabledByType(ctx, ModelTypeChat)
	if err != nil || len(candidates) == 0 {
		return GetResponse{}, sqlc.Provider{}, errors.New("no enabled chat models available for memory operations")
	}
	selected := candidates[0]
	provider, err := FetchProviderByID(ctx, queries, selected.ProviderID)
	if err != nil {
		return GetResponse{}, sqlc.Provider{}, err
	}
	return selected, provider, nil
}

// SelectMemoryModelForBot selects a chat model for memory operations.
// If botID is provided, it attempts to use the bot's configured chat model first,
// falling back to the first enabled chat model globally.
func SelectMemoryModelForBot(ctx context.Context, modelsService *Service, queries *sqlc.Queries, chatModelID string) (GetResponse, sqlc.Provider, error) {
	// If a specific model is configured (e.g. bot's chat_model_id), try to use it.
	if chatModelID = strings.TrimSpace(chatModelID); chatModelID != "" {
		model, err := modelsService.GetByModelID(ctx, chatModelID)
		if err == nil && model.Type == ModelTypeChat {
			provider, pErr := FetchProviderByID(ctx, queries, model.ProviderID)
			if pErr == nil && provider.Enable {
				return model, provider, nil
			}
		}
		// UUID-based lookup fallback
		model, err = modelsService.GetByID(ctx, chatModelID)
		if err == nil && model.Type == ModelTypeChat {
			provider, pErr := FetchProviderByID(ctx, queries, model.ProviderID)
			if pErr == nil && provider.Enable {
				return model, provider, nil
			}
		}
	}
	// Fallback: pick first enabled chat model globally.
	return SelectMemoryModel(ctx, modelsService, queries)
}

// FetchProviderByID fetches a provider by ID.
func FetchProviderByID(ctx context.Context, queries *sqlc.Queries, providerID string) (sqlc.Provider, error) {
	if strings.TrimSpace(providerID) == "" {
		return sqlc.Provider{}, errors.New("provider id missing")
	}
	parsed, err := db.ParseUUID(providerID)
	if err != nil {
		return sqlc.Provider{}, err
	}
	provider, err := queries.GetProviderByID(ctx, parsed)
	if err != nil {
		return sqlc.Provider{}, err
	}
	apiKey := providerConfigString(provider.Config, "api_key")
	if strings.TrimSpace(apiKey) != "" {
		channel.SetIMErrorSecrets("provider:"+providerID, apiKey)
	}
	return provider, nil
}
