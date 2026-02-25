package searchproviders

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
		logger:  log.With(slog.String("service", "search_providers")),
	}
}

func (s *Service) ListMeta(_ context.Context) []ProviderMeta {
	return []ProviderMeta{
		{
			Provider:    string(ProviderBrave),
			DisplayName: "Brave",
			ConfigSchema: ProviderConfigSchema{
				Fields: map[string]ProviderFieldSchema{
					"api_key": {
						Type:        "secret",
						Title:       "API Key",
						Description: "Brave Search API key",
						Required:    true,
					},
					"base_url": {
						Type:        "string",
						Title:       "Base URL",
						Description: "Brave API base URL",
						Required:    false,
						Example:     "https://api.search.brave.com/res/v1/web/search",
					},
					"timeout_seconds": {
						Type:        "number",
						Title:       "Timeout (seconds)",
						Description: "HTTP timeout in seconds",
						Required:    false,
						Example:     15,
					},
				},
			},
		},
		{
			Provider:    string(ProviderBing),
			DisplayName: "Bing",
			ConfigSchema: ProviderConfigSchema{
				Fields: map[string]ProviderFieldSchema{
					"api_key": {
						Type:        "secret",
						Title:       "API Key",
						Description: "Bing Web Search API subscription key",
						Required:    true,
					},
					"base_url": {
						Type:        "string",
						Title:       "Base URL",
						Description: "Bing API base URL",
						Required:    false,
						Example:     "https://api.bing.microsoft.com/v7.0/search",
					},
					"timeout_seconds": {
						Type:        "number",
						Title:       "Timeout (seconds)",
						Description: "HTTP timeout in seconds",
						Required:    false,
						Example:     15,
					},
				},
			},
		},
		{
			Provider:    string(ProviderGoogle),
			DisplayName: "Google",
			ConfigSchema: ProviderConfigSchema{
				Fields: map[string]ProviderFieldSchema{
					"api_key": {
						Type:        "secret",
						Title:       "API Key",
						Description: "Google Custom Search API key",
						Required:    true,
					},
					"cx": {
						Type:        "string",
						Title:       "Search Engine ID",
						Description: "Google Programmable Search Engine ID (cx)",
						Required:    true,
					},
					"base_url": {
						Type:        "string",
						Title:       "Base URL",
						Description: "Google Custom Search API base URL",
						Required:    false,
						Example:     "https://customsearch.googleapis.com/customsearch/v1",
					},
					"timeout_seconds": {
						Type:        "number",
						Title:       "Timeout (seconds)",
						Description: "HTTP timeout in seconds",
						Required:    false,
						Example:     15,
					},
				},
			},
		},
		{
			Provider:    string(ProviderTavily),
			DisplayName: "Tavily",
			ConfigSchema: ProviderConfigSchema{
				Fields: map[string]ProviderFieldSchema{
					"api_key": {
						Type:        "secret",
						Title:       "API Key",
						Description: "Tavily Search API key",
						Required:    true,
					},
					"base_url": {
						Type:        "string",
						Title:       "Base URL",
						Description: "Tavily API base URL",
						Required:    false,
						Example:     "https://api.tavily.com/search",
					},
					"timeout_seconds": {
						Type:        "number",
						Title:       "Timeout (seconds)",
						Description: "HTTP timeout in seconds",
						Required:    false,
						Example:     15,
					},
				},
			},
		},
	}
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (GetResponse, error) {
	if !isValidProviderName(req.Provider) {
		return GetResponse{}, fmt.Errorf("invalid provider: %s", req.Provider)
	}
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		return GetResponse{}, fmt.Errorf("marshal config: %w", err)
	}
	row, err := s.queries.CreateSearchProvider(ctx, sqlc.CreateSearchProviderParams{
		Name:     strings.TrimSpace(req.Name),
		Provider: string(req.Provider),
		Config:   configJSON,
	})
	if err != nil {
		return GetResponse{}, fmt.Errorf("create search provider: %w", err)
	}
	return s.toGetResponse(row), nil
}

func (s *Service) Get(ctx context.Context, id string) (GetResponse, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return GetResponse{}, err
	}
	row, err := s.queries.GetSearchProviderByID(ctx, pgID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("get search provider: %w", err)
	}
	return s.toGetResponse(row), nil
}

func (s *Service) GetRawByID(ctx context.Context, id string) (sqlc.SearchProvider, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return sqlc.SearchProvider{}, err
	}
	return s.queries.GetSearchProviderByID(ctx, pgID)
}

func (s *Service) List(ctx context.Context, provider string) ([]GetResponse, error) {
	provider = strings.TrimSpace(provider)
	var (
		rows []sqlc.SearchProvider
		err  error
	)
	if provider == "" {
		rows, err = s.queries.ListSearchProviders(ctx)
	} else {
		rows, err = s.queries.ListSearchProvidersByProvider(ctx, provider)
	}
	if err != nil {
		return nil, fmt.Errorf("list search providers: %w", err)
	}
	items := make([]GetResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.toGetResponse(row))
	}
	return items, nil
}

func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (GetResponse, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return GetResponse{}, err
	}
	current, err := s.queries.GetSearchProviderByID(ctx, pgID)
	if err != nil {
		return GetResponse{}, fmt.Errorf("get search provider: %w", err)
	}
	name := current.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	provider := current.Provider
	if req.Provider != nil {
		if !isValidProviderName(*req.Provider) {
			return GetResponse{}, fmt.Errorf("invalid provider: %s", *req.Provider)
		}
		provider = string(*req.Provider)
	}
	config := current.Config
	if req.Config != nil {
		configJSON, marshalErr := json.Marshal(req.Config)
		if marshalErr != nil {
			return GetResponse{}, fmt.Errorf("marshal config: %w", marshalErr)
		}
		config = configJSON
	}
	updated, err := s.queries.UpdateSearchProvider(ctx, sqlc.UpdateSearchProviderParams{
		ID:       pgID,
		Name:     name,
		Provider: provider,
		Config:   config,
	})
	if err != nil {
		return GetResponse{}, fmt.Errorf("update search provider: %w", err)
	}
	return s.toGetResponse(updated), nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.queries.DeleteSearchProvider(ctx, pgID)
}

func (s *Service) toGetResponse(row sqlc.SearchProvider) GetResponse {
	var cfg map[string]any
	if len(row.Config) > 0 {
		if err := json.Unmarshal(row.Config, &cfg); err != nil {
			s.logger.Warn("search provider config unmarshal failed", slog.String("id", row.ID.String()), slog.Any("error", err))
		}
	}
	return GetResponse{
		ID:        row.ID.String(),
		Name:      row.Name,
		Provider:  row.Provider,
		Config:    cfg,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
}

func isValidProviderName(name ProviderName) bool {
	switch name {
	case ProviderBrave, ProviderBing, ProviderGoogle,
		ProviderTavily:
		return true
	default:
		return false
	}
}
