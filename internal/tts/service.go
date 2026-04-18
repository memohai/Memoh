package tts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/models"
)

type Service struct {
	queries  *sqlc.Queries
	logger   *slog.Logger
	registry *Registry
}

func NewService(log *slog.Logger, queries *sqlc.Queries, registry *Registry) *Service {
	return &Service{
		queries:  queries,
		logger:   log.With(slog.String("service", "tts")),
		registry: registry,
	}
}

func (s *Service) Registry() *Registry { return s.registry }

func (s *Service) ListMeta(_ context.Context) []ProviderMetaResponse {
	return s.registry.ListMeta()
}

func (s *Service) ListSpeechProviders(ctx context.Context) ([]SpeechProviderResponse, error) {
	rows, err := s.queries.ListSpeechProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list speech providers: %w", err)
	}
	items := make([]SpeechProviderResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, toSpeechProviderResponse(row))
	}
	return items, nil
}

func (s *Service) ListSpeechModels(ctx context.Context) ([]SpeechModelResponse, error) {
	rows, err := s.queries.ListSpeechModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list speech models: %w", err)
	}
	items := make([]SpeechModelResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, toSpeechModelFromListRow(row))
	}
	return items, nil
}

func (s *Service) ListSpeechModelsByProvider(ctx context.Context, providerID string) ([]SpeechModelResponse, error) {
	pgID, err := db.ParseUUID(providerID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListSpeechModelsByProviderID(ctx, pgID)
	if err != nil {
		return nil, fmt.Errorf("list speech models by provider: %w", err)
	}
	items := make([]SpeechModelResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, toSpeechModelFromModel(row, ""))
	}
	return items, nil
}

func (s *Service) GetSpeechModel(ctx context.Context, id string) (SpeechModelResponse, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return SpeechModelResponse{}, err
	}
	row, err := s.queries.GetSpeechModelWithProvider(ctx, pgID)
	if err != nil {
		return SpeechModelResponse{}, fmt.Errorf("get speech model: %w", err)
	}
	return toSpeechModelWithProviderResponse(row), nil
}

func (s *Service) Synthesize(ctx context.Context, modelID string, text string, overrideCfg map[string]any) ([]byte, string, error) {
	params, err := s.resolveSpeechParams(ctx, modelID, text, overrideCfg)
	if err != nil {
		return nil, "", err
	}
	result, err := sdk.GenerateSpeech(ctx,
		sdk.WithSpeechModel(params.model),
		sdk.WithText(text),
		sdk.WithSpeechConfig(params.config),
	)
	if err != nil {
		return nil, "", fmt.Errorf("synthesize: %w", err)
	}
	return result.Audio, result.ContentType, nil
}

func (s *Service) StreamToFile(ctx context.Context, modelID string, text string, w io.Writer) (string, error) {
	params, err := s.resolveSpeechParams(ctx, modelID, text, nil)
	if err != nil {
		return "", err
	}
	streamResult, err := sdk.StreamSpeech(ctx,
		sdk.WithSpeechModel(params.model),
		sdk.WithText(text),
		sdk.WithSpeechConfig(params.config),
	)
	if err != nil {
		return "", fmt.Errorf("stream: %w", err)
	}
	audio, err := streamResult.Bytes()
	if err != nil {
		return "", fmt.Errorf("stream: %w", err)
	}
	if _, writeErr := w.Write(audio); writeErr != nil {
		return "", fmt.Errorf("write chunk: %w", writeErr)
	}
	return streamResult.ContentType, nil
}

func (s *Service) GetModelCapabilities(ctx context.Context, modelID string) (*ModelCapabilities, error) {
	pgID, err := db.ParseUUID(modelID)
	if err != nil {
		return nil, err
	}
	modelRow, err := s.queries.GetSpeechModelWithProvider(ctx, pgID)
	if err != nil {
		return nil, fmt.Errorf("get speech model: %w", err)
	}
	def, err := s.registry.Get(models.ClientType(modelRow.ProviderType))
	if err != nil {
		return nil, err
	}
	for _, model := range def.Models {
		if model.ID == modelRow.ModelID {
			caps := model.Capabilities
			if len(caps.ConfigSchema.Fields) == 0 {
				caps.ConfigSchema = model.ConfigSchema
			}
			return &caps, nil
		}
	}
	return nil, fmt.Errorf("speech model capabilities not found: %s", modelRow.ModelID)
}

type resolvedSpeechParams struct {
	model  *sdk.SpeechModel
	config map[string]any
}

func (s *Service) resolveSpeechParams(ctx context.Context, modelID string, text string, overrideCfg map[string]any) (*resolvedSpeechParams, error) {
	_ = text
	pgID, err := db.ParseUUID(modelID)
	if err != nil {
		return nil, err
	}

	modelRow, err := s.queries.GetSpeechModelWithProvider(ctx, pgID)
	if err != nil {
		return nil, fmt.Errorf("get speech model: %w", err)
	}
	providerRow, err := s.queries.GetProviderByID(ctx, modelRow.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("get speech provider: %w", err)
	}

	def, err := s.registry.Get(models.ClientType(providerRow.ClientType))
	if err != nil {
		return nil, err
	}
	provider, err := def.Factory(parseConfig(providerRow.Config))
	if err != nil {
		return nil, fmt.Errorf("build speech provider: %w", err)
	}

	cfg := mergeConfig(parseConfig(providerRow.Config), parseConfig(modelRow.Config), overrideCfg)
	return &resolvedSpeechParams{
		model:  &sdk.SpeechModel{ID: modelRow.ModelID, Provider: provider},
		config: cfg,
	}, nil
}

func parseConfig(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg == nil {
		return map[string]any{}
	}
	return cfg
}

func mergeConfig(parts ...map[string]any) map[string]any {
	out := make(map[string]any)
	for _, part := range parts {
		for key, value := range part {
			out[key] = value
		}
	}
	return out
}

func toSpeechProviderResponse(row sqlc.Provider) SpeechProviderResponse {
	return SpeechProviderResponse{
		ID:         row.ID.String(),
		Name:       row.Name,
		ClientType: row.ClientType,
		Enable:     row.Enable,
		CreatedAt:  row.CreatedAt.Time,
		UpdatedAt:  row.UpdatedAt.Time,
	}
}

func toSpeechModelFromListRow(row sqlc.ListSpeechModelsRow) SpeechModelResponse {
	var cfg map[string]any
	if len(row.Config) > 0 {
		_ = json.Unmarshal(row.Config, &cfg)
	}
	name := ""
	if row.Name.Valid {
		name = row.Name.String
	}
	return SpeechModelResponse{
		ID:           row.ID.String(),
		ModelID:      row.ModelID,
		Name:         name,
		ProviderID:   row.ProviderID.String(),
		ProviderType: row.ProviderType,
		Config:       cfg,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}
}

func toSpeechModelFromModel(row sqlc.Model, providerType string) SpeechModelResponse {
	var cfg map[string]any
	if len(row.Config) > 0 {
		_ = json.Unmarshal(row.Config, &cfg)
	}
	name := ""
	if row.Name.Valid {
		name = row.Name.String
	}
	return SpeechModelResponse{
		ID:           row.ID.String(),
		ModelID:      row.ModelID,
		Name:         name,
		ProviderID:   row.ProviderID.String(),
		ProviderType: providerType,
		Config:       cfg,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}
}

func toSpeechModelWithProviderResponse(row sqlc.GetSpeechModelWithProviderRow) SpeechModelResponse {
	var cfg map[string]any
	if len(row.Config) > 0 {
		_ = json.Unmarshal(row.Config, &cfg)
	}
	name := ""
	if row.Name.Valid {
		name = row.Name.String
	}
	return SpeechModelResponse{
		ID:           row.ID.String(),
		ModelID:      row.ModelID,
		Name:         name,
		ProviderID:   row.ProviderID.String(),
		ProviderType: row.ProviderType,
		Config:       cfg,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}
}
