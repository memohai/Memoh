package plugins

import (
	"context"

	dbstore "github.com/memohai/memoh/internal/db/store"
)

type transactionalQueries interface {
	InTx(context.Context, func(dbstore.Queries) error) error
}

func (s *Service) inTransaction(ctx context.Context, fn func(*Service) error) error {
	tx, ok := s.queries.(transactionalQueries)
	if !ok {
		return fn(s)
	}
	return tx.InTx(ctx, func(queries dbstore.Queries) error {
		return fn(s.withQueries(queries))
	})
}

func (s *Service) withQueries(queries dbstore.Queries) *Service {
	clone := *s
	clone.queries = queries
	clone.mcpService = s.mcpService.WithQueries(queries)
	clone.oauthService = s.oauthService.WithQueries(queries)
	return &clone
}
