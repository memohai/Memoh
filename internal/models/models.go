package models

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

var ErrModelIDAlreadyExists = errors.New("model_id already exists")
var ErrModelIDAmbiguous = errors.New("model_id is ambiguous across providers")

// Service provides CRUD operations for models
type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

// NewService creates a new models service
func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "models")),
	}
}

// Create adds a new model to the database
func (s *Service) Create(ctx context.Context, req AddRequest) (AddResponse, error) {
	model := Model(req)
	if err := model.Validate(); err != nil {
		return AddResponse{}, fmt.Errorf("validation failed: %w", err)
	}

	// Convert to sqlc params
	llmProviderID, err := db.ParseUUID(model.LlmProviderID)
	if err != nil {
		return AddResponse{}, fmt.Errorf("invalid llm provider ID: %w", err)
	}

	inputMod := []string{}
	if model.Type == ModelTypeChat {
		inputMod = normalizeModalities(model.InputModalities, []string{ModelInputText})
	}
	params := sqlc.CreateModelParams{
		ModelID:           model.ModelID,
		LlmProviderID:    llmProviderID,
		InputModalities:   inputMod,
		SupportsReasoning: model.SupportsReasoning,
		Type:              string(model.Type),
	}
	if model.ClientType != "" {
		params.ClientType = pgtype.Text{String: string(model.ClientType), Valid: true}
	}

	// Handle optional name field
	if model.Name != "" {
		params.Name = pgtype.Text{String: model.Name, Valid: true}
	}

	// Handle optional dimensions field (only for embedding models)
	if model.Type == ModelTypeEmbedding && model.Dimensions > 0 {
		params.Dimensions = pgtype.Int4{Int32: int32(model.Dimensions), Valid: true}
	}

	created, err := s.queries.CreateModel(ctx, params)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return AddResponse{}, ErrModelIDAlreadyExists
		}
		return AddResponse{}, fmt.Errorf("failed to create model: %w", err)
	}

	// Convert pgtype.UUID to string
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

// GetByID retrieves a model by its internal UUID
func (s *Service) GetByID(ctx context.Context, id string) (GetResponse, error) {
	uuid, err := db.ParseUUID(id)
	if err != nil {
		return GetResponse{}, fmt.Errorf("invalid ID: %w", err)
	}

	dbModel, err := s.queries.GetModelByID(ctx, uuid)
	if err != nil {
		return GetResponse{}, fmt.Errorf("failed to get model: %w", err)
	}

	return convertToGetResponse(dbModel), nil
}

// GetByModelID retrieves a model by its model_id field
func (s *Service) GetByModelID(ctx context.Context, modelID string) (GetResponse, error) {
	if modelID == "" {
		return GetResponse{}, fmt.Errorf("model_id is required")
	}

	dbModel, err := s.findUniqueByModelID(ctx, modelID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("failed to get model: %w", err)
	}

	return convertToGetResponse(dbModel), nil
}

// List returns all models
func (s *Service) List(ctx context.Context) ([]GetResponse, error) {
	dbModels, err := s.queries.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	return convertToGetResponseList(dbModels), nil
}

// ListByType returns models filtered by type (chat or embedding)
func (s *Service) ListByType(ctx context.Context, modelType ModelType) ([]GetResponse, error) {
	if modelType != ModelTypeChat && modelType != ModelTypeEmbedding {
		return nil, fmt.Errorf("invalid model type: %s", modelType)
	}

	dbModels, err := s.queries.ListModelsByType(ctx, string(modelType))
	if err != nil {
		return nil, fmt.Errorf("failed to list models by type: %w", err)
	}

	return convertToGetResponseList(dbModels), nil
}

// ListByClientType returns models filtered by client type
func (s *Service) ListByClientType(ctx context.Context, clientType ClientType) ([]GetResponse, error) {
	if !isValidClientType(clientType) {
		return nil, fmt.Errorf("invalid client type: %s", clientType)
	}

	dbModels, err := s.queries.ListModelsByClientType(ctx, pgtype.Text{String: string(clientType), Valid: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list models by client type: %w", err)
	}

	return convertToGetResponseList(dbModels), nil
}

// ListByProviderID returns models filtered by provider ID.
func (s *Service) ListByProviderID(ctx context.Context, providerID string) ([]GetResponse, error) {
	if strings.TrimSpace(providerID) == "" {
		return nil, fmt.Errorf("provider id is required")
	}
	uuid, err := db.ParseUUID(providerID)
	if err != nil {
		return nil, fmt.Errorf("invalid provider id: %w", err)
	}
	dbModels, err := s.queries.ListModelsByProviderID(ctx, uuid)
	if err != nil {
		return nil, fmt.Errorf("failed to list models by provider: %w", err)
	}
	return convertToGetResponseList(dbModels), nil
}

// ListByProviderIDAndType returns models filtered by provider ID and type.
func (s *Service) ListByProviderIDAndType(ctx context.Context, providerID string, modelType ModelType) ([]GetResponse, error) {
	if modelType != ModelTypeChat && modelType != ModelTypeEmbedding {
		return nil, fmt.Errorf("invalid model type: %s", modelType)
	}
	if strings.TrimSpace(providerID) == "" {
		return nil, fmt.Errorf("provider id is required")
	}
	uuid, err := db.ParseUUID(providerID)
	if err != nil {
		return nil, fmt.Errorf("invalid provider id: %w", err)
	}
	dbModels, err := s.queries.ListModelsByProviderIDAndType(ctx, sqlc.ListModelsByProviderIDAndTypeParams{
		LlmProviderID: uuid,
		Type:          string(modelType),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list models by provider and type: %w", err)
	}
	return convertToGetResponseList(dbModels), nil
}

// UpdateByID updates a model by its internal UUID
func (s *Service) UpdateByID(ctx context.Context, id string, req UpdateRequest) (GetResponse, error) {
	uuid, err := db.ParseUUID(id)
	if err != nil {
		return GetResponse{}, fmt.Errorf("invalid ID: %w", err)
	}

	model := Model(req)
	if err := model.Validate(); err != nil {
		return GetResponse{}, fmt.Errorf("validation failed: %w", err)
	}

	inputMod := []string{}
	if model.Type == ModelTypeChat {
		inputMod = normalizeModalities(model.InputModalities, []string{ModelInputText})
	}
	params := sqlc.UpdateModelParams{
		ID:                uuid,
		ModelID:           model.ModelID,
		InputModalities:   inputMod,
		SupportsReasoning: model.SupportsReasoning,
		Type:              string(model.Type),
	}
	if model.ClientType != "" {
		params.ClientType = pgtype.Text{String: string(model.ClientType), Valid: true}
	}

	llmProviderID, err := db.ParseUUID(model.LlmProviderID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("invalid llm provider ID: %w", err)
	}
	params.LlmProviderID = llmProviderID

	if model.Name != "" {
		params.Name = pgtype.Text{String: model.Name, Valid: true}
	}

	if model.Type == ModelTypeEmbedding && model.Dimensions > 0 {
		params.Dimensions = pgtype.Int4{Int32: int32(model.Dimensions), Valid: true}
	}

	updated, err := s.queries.UpdateModel(ctx, params)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return GetResponse{}, ErrModelIDAlreadyExists
		}
		return GetResponse{}, fmt.Errorf("failed to update model: %w", err)
	}

	return convertToGetResponse(updated), nil
}

// UpdateByModelID updates a model by its model_id field
func (s *Service) UpdateByModelID(ctx context.Context, modelID string, req UpdateRequest) (GetResponse, error) {
	if modelID == "" {
		return GetResponse{}, fmt.Errorf("model_id is required")
	}
	current, err := s.findUniqueByModelID(ctx, modelID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("failed to update model: %w", err)
	}

	model := Model(req)
	if err := model.Validate(); err != nil {
		return GetResponse{}, fmt.Errorf("validation failed: %w", err)
	}

	inputMod := []string{}
	if model.Type == ModelTypeChat {
		inputMod = normalizeModalities(model.InputModalities, []string{ModelInputText})
	}
	params := sqlc.UpdateModelParams{
		ID:                current.ID,
		InputModalities:   inputMod,
		SupportsReasoning: model.SupportsReasoning,
		Type:              string(model.Type),
	}
	if model.ClientType != "" {
		params.ClientType = pgtype.Text{String: string(model.ClientType), Valid: true}
	}

	llmProviderID, err := db.ParseUUID(model.LlmProviderID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("invalid llm provider ID: %w", err)
	}
	params.LlmProviderID = llmProviderID

	if model.Name != "" {
		params.Name = pgtype.Text{String: model.Name, Valid: true}
	}

	if model.Type == ModelTypeEmbedding && model.Dimensions > 0 {
		params.Dimensions = pgtype.Int4{Int32: int32(model.Dimensions), Valid: true}
	}

	params.ModelID = model.ModelID

	updated, err := s.queries.UpdateModel(ctx, params)
	if err != nil {
		if db.IsUniqueViolation(err) {
			return GetResponse{}, ErrModelIDAlreadyExists
		}
		return GetResponse{}, fmt.Errorf("failed to update model: %w", err)
	}

	return convertToGetResponse(updated), nil
}

// DeleteByID deletes a model by its internal UUID
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

// DeleteByModelID deletes a model by its model_id field
func (s *Service) DeleteByModelID(ctx context.Context, modelID string) error {
	if modelID == "" {
		return fmt.Errorf("model_id is required")
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

// Count returns the total number of models
func (s *Service) Count(ctx context.Context) (int64, error) {
	count, err := s.queries.CountModels(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count models: %w", err)
	}
	return count, nil
}

// CountByType returns the number of models of a specific type
func (s *Service) CountByType(ctx context.Context, modelType ModelType) (int64, error) {
	if modelType != ModelTypeChat && modelType != ModelTypeEmbedding {
		return 0, fmt.Errorf("invalid model type: %s", modelType)
	}

	count, err := s.queries.CountModelsByType(ctx, string(modelType))
	if err != nil {
		return 0, fmt.Errorf("failed to count models by type: %w", err)
	}
	return count, nil
}

// Helper functions

func convertToGetResponse(dbModel sqlc.Model) GetResponse {
	resp := GetResponse{
		ID:      dbModel.ID.String(),
		ModelID: dbModel.ModelID,
		Model: Model{
			ModelID:           dbModel.ModelID,
			SupportsReasoning: dbModel.SupportsReasoning,
			Type:              ModelType(dbModel.Type),
		},
	}
	if dbModel.ClientType.Valid {
		resp.Model.ClientType = ClientType(dbModel.ClientType.String)
	}
	if resp.Model.Type == ModelTypeChat {
		resp.Model.InputModalities = normalizeModalities(dbModel.InputModalities, []string{ModelInputText})
	}

	if dbModel.LlmProviderID.Valid {
		resp.Model.LlmProviderID = dbModel.LlmProviderID.String()
	}

	if dbModel.Name.Valid {
		resp.Model.Name = dbModel.Name.String
	}

	if dbModel.Dimensions.Valid {
		resp.Model.Dimensions = int(dbModel.Dimensions.Int32)
	}

	return resp
}

func convertToGetResponseList(dbModels []sqlc.Model) []GetResponse {
	responses := make([]GetResponse, 0, len(dbModels))
	for _, dbModel := range dbModels {
		responses = append(responses, convertToGetResponse(dbModel))
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

// normalizeModalities returns modalities if non-empty, otherwise the provided fallback.
func normalizeModalities(modalities []string, fallback []string) []string {
	if len(modalities) == 0 {
		return fallback
	}
	return modalities
}

func isValidClientType(clientType ClientType) bool {
	switch clientType {
	case ClientTypeOpenAIResponses,
		ClientTypeOpenAICompletions,
		ClientTypeAnthropicMessages,
		ClientTypeGoogleGenerativeAI:
		return true
	default:
		return false
	}
}

// SelectMemoryModel selects a chat model for memory operations.
func SelectMemoryModel(ctx context.Context, modelsService *Service, queries *sqlc.Queries) (GetResponse, sqlc.LlmProvider, error) {
	if modelsService == nil {
		return GetResponse{}, sqlc.LlmProvider{}, fmt.Errorf("models service not configured")
	}
	if queries == nil {
		return GetResponse{}, sqlc.LlmProvider{}, fmt.Errorf("queries not configured")
	}
	candidates, err := modelsService.ListByType(ctx, ModelTypeChat)
	if err != nil || len(candidates) == 0 {
		return GetResponse{}, sqlc.LlmProvider{}, fmt.Errorf("no chat models available for memory operations")
	}
	selected := candidates[0]
	provider, err := FetchProviderByID(ctx, queries, selected.LlmProviderID)
	if err != nil {
		return GetResponse{}, sqlc.LlmProvider{}, err
	}
	return selected, provider, nil
}

// SelectMemoryModelForBot selects memory model by bot settings first, then falls back to SelectMemoryModel.
func SelectMemoryModelForBot(ctx context.Context, modelsService *Service, queries *sqlc.Queries, botID string) (GetResponse, sqlc.LlmProvider, error) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return SelectMemoryModel(ctx, modelsService, queries)
	}
	if queries == nil {
		return SelectMemoryModel(ctx, modelsService, queries)
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return SelectMemoryModel(ctx, modelsService, queries)
	}
	bot, err := queries.GetBotByID(ctx, pgBotID)
	if err != nil {
		return SelectMemoryModel(ctx, modelsService, queries)
	}
	if !bot.MemoryModelID.Valid {
		return SelectMemoryModel(ctx, modelsService, queries)
	}
	dbModel, err := queries.GetModelByID(ctx, bot.MemoryModelID)
	if err != nil {
		return SelectMemoryModel(ctx, modelsService, queries)
	}
	selected := convertToGetResponse(dbModel)
	if selected.Type != ModelTypeChat {
		return SelectMemoryModel(ctx, modelsService, queries)
	}
	provider, err := FetchProviderByID(ctx, queries, selected.LlmProviderID)
	if err != nil {
		return GetResponse{}, sqlc.LlmProvider{}, err
	}
	return selected, provider, nil
}

// FetchProviderByID fetches a provider by ID.
func FetchProviderByID(ctx context.Context, queries *sqlc.Queries, providerID string) (sqlc.LlmProvider, error) {
	if strings.TrimSpace(providerID) == "" {
		return sqlc.LlmProvider{}, fmt.Errorf("provider id missing")
	}
	parsed, err := db.ParseUUID(providerID)
	if err != nil {
		return sqlc.LlmProvider{}, err
	}
	return queries.GetLlmProviderByID(ctx, parsed)
}
