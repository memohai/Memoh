package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "memory_providers")),
	}
}

func (s *Service) ListMeta(_ context.Context) []ProviderMeta {
	return []ProviderMeta{
		{
			Provider:    string(ProviderBuiltin),
			DisplayName: "Built-in",
			ConfigSchema: ProviderConfigSchema{
				Fields: map[string]ProviderFieldSchema{
					"memory_model_id": {
						Type:        "model_select",
						Title:       "Memory Model",
						Description: "LLM model used for memory extraction and decision",
						Required:    false,
					},
					"embedding_model_id": {
						Type:        "model_select",
						Title:       "Embedding Model",
						Description: "Embedding model for dense vector search",
						Required:    false,
					},
				},
			},
		},
	}
}

func (s *Service) Create(ctx context.Context, req ProviderCreateRequest) (ProviderGetResponse, error) {
	if !isValidProviderType(req.Provider) {
		return ProviderGetResponse{}, fmt.Errorf("invalid provider type: %s", req.Provider)
	}
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		return ProviderGetResponse{}, fmt.Errorf("marshal config: %w", err)
	}
	row, err := s.queries.CreateMemoryProvider(ctx, sqlc.CreateMemoryProviderParams{
		Name:      strings.TrimSpace(req.Name),
		Provider:  string(req.Provider),
		Config:    configJSON,
		IsDefault: false,
	})
	if err != nil {
		return ProviderGetResponse{}, fmt.Errorf("create memory provider: %w", err)
	}
	return s.toGetResponse(row), nil
}

func (s *Service) Get(ctx context.Context, id string) (ProviderGetResponse, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return ProviderGetResponse{}, err
	}
	row, err := s.queries.GetMemoryProviderByID(ctx, pgID)
	if err != nil {
		return ProviderGetResponse{}, fmt.Errorf("get memory provider: %w", err)
	}
	return s.toGetResponse(row), nil
}

func (s *Service) List(ctx context.Context) ([]ProviderGetResponse, error) {
	rows, err := s.queries.ListMemoryProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("list memory providers: %w", err)
	}
	items := make([]ProviderGetResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.toGetResponse(row))
	}
	return items, nil
}

func (s *Service) Update(ctx context.Context, id string, req ProviderUpdateRequest) (ProviderGetResponse, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return ProviderGetResponse{}, err
	}
	current, err := s.queries.GetMemoryProviderByID(ctx, pgID)
	if err != nil {
		return ProviderGetResponse{}, fmt.Errorf("get memory provider: %w", err)
	}
	name := current.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	config := current.Config
	if req.Config != nil {
		configJSON, marshalErr := json.Marshal(req.Config)
		if marshalErr != nil {
			return ProviderGetResponse{}, fmt.Errorf("marshal config: %w", marshalErr)
		}
		config = configJSON
	}
	updated, err := s.queries.UpdateMemoryProvider(ctx, sqlc.UpdateMemoryProviderParams{
		ID:     pgID,
		Name:   name,
		Config: config,
	})
	if err != nil {
		return ProviderGetResponse{}, fmt.Errorf("update memory provider: %w", err)
	}
	return s.toGetResponse(updated), nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.queries.DeleteMemoryProvider(ctx, pgID)
}

// EnsureDefault creates a default builtin provider if none exists.
func (s *Service) EnsureDefault(ctx context.Context) (ProviderGetResponse, error) {
	row, err := s.queries.GetDefaultMemoryProvider(ctx)
	if err == nil {
		return s.toGetResponse(row), nil
	}
	configJSON, _ := json.Marshal(map[string]any{})
	created, err := s.queries.CreateMemoryProvider(ctx, sqlc.CreateMemoryProviderParams{
		Name:      "Built-in Memory",
		Provider:  string(ProviderBuiltin),
		Config:    configJSON,
		IsDefault: true,
	})
	if err != nil {
		return ProviderGetResponse{}, fmt.Errorf("create default memory provider: %w", err)
	}
	return s.toGetResponse(created), nil
}

func (s *Service) toGetResponse(row sqlc.MemoryProvider) ProviderGetResponse {
	var cfg map[string]any
	if len(row.Config) > 0 {
		if err := json.Unmarshal(row.Config, &cfg); err != nil {
			s.logger.Warn("memory provider config unmarshal failed", slog.String("id", row.ID.String()), slog.Any("error", err))
		}
	}
	return ProviderGetResponse{
		ID:        row.ID.String(),
		Name:      row.Name,
		Provider:  row.Provider,
		Config:    cfg,
		IsDefault: row.IsDefault,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
}

func isValidProviderType(t ProviderType) bool {
	switch t {
	case ProviderBuiltin:
		return true
	default:
		return false
	}
}
