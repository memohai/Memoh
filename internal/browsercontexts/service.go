package browsercontexts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

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
		logger:  log.With(slog.String("service", "browser_contexts")),
	}
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (BrowserContext, error) {
	configBytes, err := marshalConfig(req.Config)
	if err != nil {
		return BrowserContext{}, err
	}

	row, err := s.queries.CreateBrowserContext(ctx, sqlc.CreateBrowserContextParams{
		Name:   req.Name,
		Config: configBytes,
	})
	if err != nil {
		return BrowserContext{}, fmt.Errorf("create browser context: %w", err)
	}
	return rowToContext(row)
}

func (s *Service) GetByID(ctx context.Context, id string) (BrowserContext, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return BrowserContext{}, err
	}
	row, err := s.queries.GetBrowserContextByID(ctx, pgID)
	if err != nil {
		return BrowserContext{}, fmt.Errorf("get browser context: %w", err)
	}
	return rowToContext(row)
}

func (s *Service) List(ctx context.Context) ([]BrowserContext, error) {
	rows, err := s.queries.ListBrowserContexts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list browser contexts: %w", err)
	}
	result := make([]BrowserContext, 0, len(rows))
	for _, row := range rows {
		bc, err := rowToContext(row)
		if err != nil {
			return nil, err
		}
		result = append(result, bc)
	}
	return result, nil
}

func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (BrowserContext, error) {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return BrowserContext{}, err
	}
	configBytes, err := marshalConfig(req.Config)
	if err != nil {
		return BrowserContext{}, err
	}

	row, err := s.queries.UpdateBrowserContext(ctx, sqlc.UpdateBrowserContextParams{
		ID:     pgID,
		Name:   req.Name,
		Config: configBytes,
	})
	if err != nil {
		return BrowserContext{}, fmt.Errorf("update browser context: %w", err)
	}
	return rowToContext(row)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	pgID, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	if err := s.queries.DeleteBrowserContext(ctx, pgID); err != nil {
		return fmt.Errorf("delete browser context: %w", err)
	}
	return nil
}

func rowToContext(row sqlc.BrowserContext) (BrowserContext, error) {
	id, err := uuid.FromBytes(row.ID.Bytes[:])
	if err != nil {
		return BrowserContext{}, fmt.Errorf("convert UUID: %w", err)
	}
	return BrowserContext{
		ID:        id.String(),
		Name:      row.Name,
		Config:    json.RawMessage(row.Config),
		CreatedAt: db.TimeFromPg(row.CreatedAt).Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: db.TimeFromPg(row.UpdatedAt).Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

func marshalConfig(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return []byte("{}"), nil
	}
	if !json.Valid(raw) {
		return nil, errors.New("config is not valid JSON")
	}
	return raw, nil
}
