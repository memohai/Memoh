package adapters

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/models"
)

func setMemoryProviderTeamID(ctx context.Context, arg any) {
	models.SetTeamIDParam(arg, models.TeamIDFromContext(ctx))
}

func getMemoryProviderByIDForScope(ctx context.Context, queries dbstore.Queries, id pgtype.UUID) (sqlc.MemoryProvider, error) {
	return models.InvokeTeamQuery[sqlc.MemoryProvider](ctx, queries, "GetMemoryProviderByIDForTeam", map[string]any{
		"ID": id,
	}, func() (sqlc.MemoryProvider, error) {
		return queries.GetMemoryProviderByID(ctx, id)
	})
}

func getDefaultMemoryProviderForScope(ctx context.Context, queries dbstore.Queries) (sqlc.MemoryProvider, error) {
	return models.InvokeTeamQuery[sqlc.MemoryProvider](ctx, queries, "GetDefaultMemoryProviderForTeam", nil, func() (sqlc.MemoryProvider, error) {
		return queries.GetDefaultMemoryProvider(ctx)
	})
}

func listMemoryProvidersForScope(ctx context.Context, queries dbstore.Queries) ([]sqlc.MemoryProvider, error) {
	return models.InvokeTeamQuery[[]sqlc.MemoryProvider](ctx, queries, "ListMemoryProvidersForTeam", nil, func() ([]sqlc.MemoryProvider, error) {
		return queries.ListMemoryProviders(ctx)
	})
}

func deleteMemoryProviderForScope(ctx context.Context, queries dbstore.Queries, id pgtype.UUID) error {
	return models.InvokeTeamExec(ctx, queries, "DeleteMemoryProviderForTeam", map[string]any{
		"ID": id,
	}, func() error {
		return queries.DeleteMemoryProvider(ctx, id)
	})
}
