package searchproviders

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/models"
)

func setSearchProviderTeamID(ctx context.Context, arg any) {
	models.SetTeamIDParam(arg, models.TeamIDFromContext(ctx))
}

func getSearchProviderByIDForScope(ctx context.Context, queries dbstore.Queries, id pgtype.UUID) (sqlc.SearchProvider, error) {
	return models.InvokeTeamQuery[sqlc.SearchProvider](ctx, queries, "GetSearchProviderByIDForTeam", map[string]any{
		"ID": id,
	}, func() (sqlc.SearchProvider, error) {
		return queries.GetSearchProviderByID(ctx, sqlc.GetSearchProviderByIDParams{ID: id, TeamID: models.TeamIDFromContext(ctx)})
	})
}

func listSearchProvidersForScope(ctx context.Context, queries dbstore.Queries) ([]sqlc.SearchProvider, error) {
	return models.InvokeTeamQuery[[]sqlc.SearchProvider](ctx, queries, "ListSearchProvidersForTeam", nil, func() ([]sqlc.SearchProvider, error) {
		return queries.ListSearchProviders(ctx)
	})
}

func listSearchProvidersByProviderForScope(ctx context.Context, queries dbstore.Queries, provider string) ([]sqlc.SearchProvider, error) {
	return models.InvokeTeamQuery[[]sqlc.SearchProvider](ctx, queries, "ListSearchProvidersByProviderForTeam", map[string]any{
		"Provider": provider,
	}, func() ([]sqlc.SearchProvider, error) {
		return queries.ListSearchProvidersByProvider(ctx, provider)
	})
}

func deleteSearchProviderForScope(ctx context.Context, queries dbstore.Queries, id pgtype.UUID) error {
	return models.InvokeTeamExec(ctx, queries, "DeleteSearchProviderForTeam", map[string]any{
		"ID": id,
	}, func() error {
		return queries.DeleteSearchProvider(ctx, sqlc.DeleteSearchProviderParams{ID: id, TeamID: models.TeamIDFromContext(ctx)})
	})
}
