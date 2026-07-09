package providers

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/models"
)

func setProviderTeamID(ctx context.Context, arg any) {
	models.SetTeamIDParam(arg, models.TeamIDFromContext(ctx))
}

func getProviderByIDForScope(ctx context.Context, queries dbstore.Queries, id pgtype.UUID) (sqlc.Provider, error) {
	return models.InvokeTeamQuery[sqlc.Provider](ctx, queries, "GetProviderByIDForTeam", map[string]any{
		"ID": id,
	}, func() (sqlc.Provider, error) {
		return queries.GetProviderByID(ctx, id)
	})
}

func getProviderByNameForScope(ctx context.Context, queries dbstore.Queries, name string) (sqlc.Provider, error) {
	return models.InvokeTeamQuery[sqlc.Provider](ctx, queries, "GetProviderByNameForTeam", map[string]any{
		"Name": name,
	}, func() (sqlc.Provider, error) {
		return queries.GetProviderByName(ctx, name)
	})
}

func listProvidersForScope(ctx context.Context, queries dbstore.Queries) ([]sqlc.Provider, error) {
	return models.InvokeTeamQuery[[]sqlc.Provider](ctx, queries, "ListProvidersForTeam", nil, func() ([]sqlc.Provider, error) {
		return queries.ListProviders(ctx)
	})
}

func deleteProviderForScope(ctx context.Context, queries dbstore.Queries, id pgtype.UUID) error {
	return models.InvokeTeamExec(ctx, queries, "DeleteProviderForTeam", map[string]any{
		"ID": id,
	}, func() error {
		return queries.DeleteProvider(ctx, id)
	})
}

func getProviderOAuthTokenByProviderForScope(ctx context.Context, queries dbstore.Queries, providerID pgtype.UUID) (sqlc.ProviderOauthToken, error) {
	return models.InvokeTeamQuery[sqlc.ProviderOauthToken](ctx, queries, "GetProviderOAuthTokenByProviderForTeam", map[string]any{
		"ProviderID": providerID,
	}, func() (sqlc.ProviderOauthToken, error) {
		return queries.GetProviderOAuthTokenByProvider(ctx, providerID)
	})
}

func getProviderOAuthTokenByStateForScope(ctx context.Context, queries dbstore.Queries, state string) (sqlc.ProviderOauthToken, error) {
	return models.InvokeTeamQuery[sqlc.ProviderOauthToken](ctx, queries, "GetProviderOAuthTokenByStateForTeam", map[string]any{
		"State": state,
	}, func() (sqlc.ProviderOauthToken, error) {
		return queries.GetProviderOAuthTokenByState(ctx, state)
	})
}

func deleteProviderOAuthTokenForScope(ctx context.Context, queries dbstore.Queries, providerID pgtype.UUID) error {
	return models.InvokeTeamExec(ctx, queries, "DeleteProviderOAuthTokenForTeam", map[string]any{
		"ProviderID": providerID,
	}, func() error {
		return queries.DeleteProviderOAuthToken(ctx, providerID)
	})
}

func getUserProviderOAuthTokenForScope(ctx context.Context, queries dbstore.Queries, arg sqlc.GetUserProviderOAuthTokenParams) (sqlc.UserProviderOauthToken, error) {
	return models.InvokeTeamQuery[sqlc.UserProviderOauthToken](ctx, queries, "GetUserProviderOAuthTokenForTeam", map[string]any{
		"ProviderID": arg.ProviderID,
		"UserID":     arg.UserID,
	}, func() (sqlc.UserProviderOauthToken, error) {
		return queries.GetUserProviderOAuthToken(ctx, arg)
	})
}

func getUserProviderOAuthTokenByStateForScope(ctx context.Context, queries dbstore.Queries, state string) (sqlc.UserProviderOauthToken, error) {
	return models.InvokeTeamQuery[sqlc.UserProviderOauthToken](ctx, queries, "GetUserProviderOAuthTokenByStateForTeam", map[string]any{
		"State": state,
	}, func() (sqlc.UserProviderOauthToken, error) {
		return queries.GetUserProviderOAuthTokenByState(ctx, state)
	})
}

func deleteUserProviderOAuthTokenForScope(ctx context.Context, queries dbstore.Queries, arg sqlc.DeleteUserProviderOAuthTokenParams) error {
	return models.InvokeTeamExec(ctx, queries, "DeleteUserProviderOAuthTokenForTeam", map[string]any{
		"ProviderID": arg.ProviderID,
		"UserID":     arg.UserID,
	}, func() error {
		return queries.DeleteUserProviderOAuthToken(ctx, arg)
	})
}
