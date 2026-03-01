package email

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

// Service manages email provider CRUD and bindings.
type Service struct {
	queries  *sqlc.Queries
	logger   *slog.Logger
	registry *Registry
}

func NewService(log *slog.Logger, queries *sqlc.Queries, registry *Registry) *Service {
	return &Service{
		queries:  queries,
		logger:   log.With(slog.String("service", "email")),
		registry: registry,
	}
}

func (s *Service) Registry() *Registry { return s.registry }

// ---- Provider CRUD ----

func (s *Service) ListMeta(_ context.Context) []ProviderMeta {
	return s.registry.ListMeta()
}

func (s *Service) CreateProvider(ctx context.Context, req CreateProviderRequest) (ProviderResponse, error) {
	if _, err := s.registry.Get(req.Provider); err != nil {
		return ProviderResponse{}, fmt.Errorf("unsupported provider: %s", req.Provider)
	}
	if len(req.Config) > 0 {
		if a, err := s.registry.Get(req.Provider); err == nil {
			normalized, normErr := a.NormalizeConfig(req.Config)
			if normErr != nil {
				return ProviderResponse{}, fmt.Errorf("invalid config: %w", normErr)
			}
			req.Config = normalized
		}
	}
	if req.Config == nil {
		req.Config = make(map[string]any)
	}
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		return ProviderResponse{}, fmt.Errorf("marshal config: %w", err)
	}
	row, err := s.queries.CreateEmailProvider(ctx, sqlc.CreateEmailProviderParams{
		Name:     strings.TrimSpace(req.Name),
		Provider: string(req.Provider),
		Config:   configJSON,
	})
	if err != nil {
		return ProviderResponse{}, fmt.Errorf("create email provider: %w", err)
	}
	return s.toProviderResponse(row), nil
}

func (s *Service) GetProvider(ctx context.Context, id string) (ProviderResponse, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return ProviderResponse{}, err
	}
	row, err := s.queries.GetEmailProviderByID(ctx, pgID)
	if err != nil {
		return ProviderResponse{}, fmt.Errorf("get email provider: %w", err)
	}
	return s.toProviderResponse(row), nil
}

func (s *Service) GetRawProvider(ctx context.Context, id string) (sqlc.EmailProvider, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return sqlc.EmailProvider{}, err
	}
	return s.queries.GetEmailProviderByID(ctx, pgID)
}

func (s *Service) ListProviders(ctx context.Context, provider string) ([]ProviderResponse, error) {
	provider = strings.TrimSpace(provider)
	var (
		rows []sqlc.EmailProvider
		err  error
	)
	if provider == "" {
		rows, err = s.queries.ListEmailProviders(ctx)
	} else {
		rows, err = s.queries.ListEmailProvidersByProvider(ctx, provider)
	}
	if err != nil {
		return nil, fmt.Errorf("list email providers: %w", err)
	}
	items := make([]ProviderResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.toProviderResponse(row))
	}
	return items, nil
}

func (s *Service) UpdateProvider(ctx context.Context, id string, req UpdateProviderRequest) (ProviderResponse, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return ProviderResponse{}, err
	}
	current, err := s.queries.GetEmailProviderByID(ctx, pgID)
	if err != nil {
		return ProviderResponse{}, fmt.Errorf("get email provider: %w", err)
	}
	name := current.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	provider := current.Provider
	if req.Provider != nil {
		if _, err := s.registry.Get(*req.Provider); err != nil {
			return ProviderResponse{}, fmt.Errorf("unsupported provider: %s", *req.Provider)
		}
		provider = string(*req.Provider)
	}
	config := current.Config
	if req.Config != nil {
		if a, aErr := s.registry.Get(ProviderName(provider)); aErr == nil {
			normalized, normErr := a.NormalizeConfig(req.Config)
			if normErr != nil {
				return ProviderResponse{}, fmt.Errorf("invalid config: %w", normErr)
			}
			req.Config = normalized
		}
		configJSON, marshalErr := json.Marshal(req.Config)
		if marshalErr != nil {
			return ProviderResponse{}, fmt.Errorf("marshal config: %w", marshalErr)
		}
		config = configJSON
	}
	updated, err := s.queries.UpdateEmailProvider(ctx, sqlc.UpdateEmailProviderParams{
		ID:       pgID,
		Name:     name,
		Provider: provider,
		Config:   config,
	})
	if err != nil {
		return ProviderResponse{}, fmt.Errorf("update email provider: %w", err)
	}
	return s.toProviderResponse(updated), nil
}

func (s *Service) DeleteProvider(ctx context.Context, id string) error {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.queries.DeleteEmailProvider(ctx, pgID)
}

func (s *Service) toProviderResponse(row sqlc.EmailProvider) ProviderResponse {
	var cfg map[string]any
	if len(row.Config) > 0 {
		if err := json.Unmarshal(row.Config, &cfg); err != nil {
			s.logger.Warn("email provider config unmarshal failed", slog.String("id", row.ID.String()), slog.Any("error", err))
		}
	}
	return ProviderResponse{
		ID:        row.ID.String(),
		Name:      row.Name,
		Provider:  row.Provider,
		Config:    cfg,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
}

// ---- Binding CRUD ----

func (s *Service) CreateBinding(ctx context.Context, botID string, req CreateBindingRequest) (BindingResponse, error) {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return BindingResponse{}, fmt.Errorf("invalid bot_id: %w", err)
	}
	pgProviderID, err := db.ParseUUID(req.EmailProviderID)
	if err != nil {
		return BindingResponse{}, fmt.Errorf("invalid email_provider_id: %w", err)
	}
	canRead, canWrite, canDelete := true, true, false
	if req.CanRead != nil {
		canRead = *req.CanRead
	}
	if req.CanWrite != nil {
		canWrite = *req.CanWrite
	}
	if req.CanDelete != nil {
		canDelete = *req.CanDelete
	}
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		return BindingResponse{}, fmt.Errorf("marshal config: %w", err)
	}
	row, err := s.queries.CreateBotEmailBinding(ctx, sqlc.CreateBotEmailBindingParams{
		BotID:           pgBotID,
		EmailProviderID: pgProviderID,
		EmailAddress:    strings.TrimSpace(req.EmailAddress),
		CanRead:         canRead,
		CanWrite:        canWrite,
		CanDelete:       canDelete,
		Config:          configJSON,
	})
	if err != nil {
		return BindingResponse{}, fmt.Errorf("create email binding: %w", err)
	}
	return s.toBindingResponse(row), nil
}

func (s *Service) GetBinding(ctx context.Context, id string) (BindingResponse, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return BindingResponse{}, err
	}
	row, err := s.queries.GetBotEmailBindingByID(ctx, pgID)
	if err != nil {
		return BindingResponse{}, fmt.Errorf("get email binding: %w", err)
	}
	return s.toBindingResponse(row), nil
}

func (s *Service) ListBindings(ctx context.Context, botID string) ([]BindingResponse, error) {
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListBotEmailBindings(ctx, pgBotID)
	if err != nil {
		return nil, fmt.Errorf("list email bindings: %w", err)
	}
	items := make([]BindingResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.toBindingResponse(row))
	}
	return items, nil
}

func (s *Service) ListReadableBindingsByProvider(ctx context.Context, providerID string) ([]BindingResponse, error) {
	pgID, err := db.ParseUUID(providerID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListReadableBindingsByProvider(ctx, pgID)
	if err != nil {
		return nil, fmt.Errorf("list readable bindings: %w", err)
	}
	items := make([]BindingResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.toBindingResponse(row))
	}
	return items, nil
}

func (s *Service) GetBotBinding(ctx context.Context, botID string) (BindingResponse, error) {
	bindings, err := s.ListBindings(ctx, botID)
	if err != nil {
		return BindingResponse{}, err
	}
	if len(bindings) == 0 {
		return BindingResponse{}, fmt.Errorf("no email binding for bot %s", botID)
	}
	return bindings[0], nil
}

func (s *Service) UpdateBinding(ctx context.Context, id string, req UpdateBindingRequest) (BindingResponse, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return BindingResponse{}, err
	}
	current, err := s.queries.GetBotEmailBindingByID(ctx, pgID)
	if err != nil {
		return BindingResponse{}, fmt.Errorf("get email binding: %w", err)
	}
	emailAddr := current.EmailAddress
	if req.EmailAddress != nil {
		emailAddr = strings.TrimSpace(*req.EmailAddress)
	}
	canRead := current.CanRead
	if req.CanRead != nil {
		canRead = *req.CanRead
	}
	canWrite := current.CanWrite
	if req.CanWrite != nil {
		canWrite = *req.CanWrite
	}
	canDelete := current.CanDelete
	if req.CanDelete != nil {
		canDelete = *req.CanDelete
	}
	config := current.Config
	if req.Config != nil {
		configJSON, marshalErr := json.Marshal(req.Config)
		if marshalErr != nil {
			return BindingResponse{}, fmt.Errorf("marshal config: %w", marshalErr)
		}
		config = configJSON
	}
	updated, err := s.queries.UpdateBotEmailBinding(ctx, sqlc.UpdateBotEmailBindingParams{
		ID:           pgID,
		EmailAddress: emailAddr,
		CanRead:      canRead,
		CanWrite:     canWrite,
		CanDelete:    canDelete,
		Config:       config,
	})
	if err != nil {
		return BindingResponse{}, fmt.Errorf("update email binding: %w", err)
	}
	return s.toBindingResponse(updated), nil
}

func (s *Service) DeleteBinding(ctx context.Context, id string) error {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.queries.DeleteBotEmailBinding(ctx, pgID)
}

func (s *Service) toBindingResponse(row sqlc.BotEmailBinding) BindingResponse {
	var cfg map[string]any
	if len(row.Config) > 0 {
		if err := json.Unmarshal(row.Config, &cfg); err != nil {
			s.logger.Warn("email binding config unmarshal failed", slog.String("id", row.ID.String()), slog.Any("error", err))
		}
	}
	return BindingResponse{
		ID:              row.ID.String(),
		BotID:           row.BotID.String(),
		EmailProviderID: row.EmailProviderID.String(),
		EmailAddress:    row.EmailAddress,
		CanRead:         row.CanRead,
		CanWrite:        row.CanWrite,
		CanDelete:       row.CanDelete,
		Config:          cfg,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
}

// ProviderConfig returns the deserialized config for a given provider ID.
func (s *Service) ProviderConfig(ctx context.Context, providerID string) (ProviderName, map[string]any, error) {
	resp, err := s.GetProvider(ctx, providerID)
	if err != nil {
		return "", nil, err
	}
	return ProviderName(resp.Provider), resp.Config, nil
}
