package fetchproviders

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/models"
)

func setFetchProviderTeamID(ctx context.Context, arg any) {
	models.SetTeamIDParam(arg, models.TeamIDFromContext(ctx))
}

func getFetchProviderByIDForScope(ctx context.Context, queries dbstore.Queries, id pgtype.UUID) (sqlc.FetchProvider, error) {
	return models.InvokeTeamQuery[sqlc.FetchProvider](ctx, queries, "GetFetchProviderByIDForTeam", map[string]any{
		"ID": id,
	}, func() (sqlc.FetchProvider, error) {
		return queries.GetFetchProviderByID(ctx, sqlc.GetFetchProviderByIDParams{ID: id, TeamID: models.TeamIDFromContext(ctx)})
	})
}

func listFetchProvidersForScope(ctx context.Context, queries dbstore.Queries) ([]sqlc.FetchProvider, error) {
	return models.InvokeTeamQuery[[]sqlc.FetchProvider](ctx, queries, "ListFetchProvidersForTeam", nil, func() ([]sqlc.FetchProvider, error) {
		return queries.ListFetchProviders(ctx)
	})
}

func listFetchProvidersByProviderForScope(ctx context.Context, queries dbstore.Queries, provider string) ([]sqlc.FetchProvider, error) {
	return models.InvokeTeamQuery[[]sqlc.FetchProvider](ctx, queries, "ListFetchProvidersByProviderForTeam", map[string]any{
		"Provider": provider,
	}, func() ([]sqlc.FetchProvider, error) {
		return queries.ListFetchProvidersByProvider(ctx, provider)
	})
}

func deleteFetchProviderForScope(ctx context.Context, queries dbstore.Queries, id pgtype.UUID) error {
	return models.InvokeTeamExec(ctx, queries, "DeleteFetchProviderForTeam", map[string]any{
		"ID": id,
	}, func() error {
		return queries.DeleteFetchProvider(ctx, sqlc.DeleteFetchProviderParams{ID: id, TeamID: models.TeamIDFromContext(ctx)})
	})
}
