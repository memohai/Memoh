package models

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

func setModelTeamID(ctx context.Context, arg any) {
	SetTeamIDParam(arg, TeamIDFromContext(ctx))
}

func getModelByIDForScope(ctx context.Context, queries dbstore.Queries, id pgtype.UUID) (sqlc.Model, error) {
	return InvokeTeamQuery[sqlc.Model](ctx, queries, "GetModelByIDForTeam", map[string]any{
		"ID": id,
	}, func() (sqlc.Model, error) {
		return queries.GetModelByID(ctx, sqlc.GetModelByIDParams{ID: id, TeamID: TeamIDFromContext(ctx)})
	})
}

func listModelsByModelIDForScope(ctx context.Context, queries dbstore.Queries, modelID string) ([]sqlc.Model, error) {
	return InvokeTeamQuery[[]sqlc.Model](ctx, queries, "ListModelsByModelIDForTeam", map[string]any{
		"ModelID": modelID,
	}, func() ([]sqlc.Model, error) {
		return queries.ListModelsByModelID(ctx, modelID)
	})
}

func listModelsForScope(ctx context.Context, queries dbstore.Queries) ([]sqlc.Model, error) {
	return InvokeTeamQuery[[]sqlc.Model](ctx, queries, "ListModelsForTeam", nil, func() ([]sqlc.Model, error) {
		return queries.ListModels(ctx)
	})
}

func listModelsByTypeForScope(ctx context.Context, queries dbstore.Queries, modelType string) ([]sqlc.Model, error) {
	return InvokeTeamQuery[[]sqlc.Model](ctx, queries, "ListModelsByTypeForTeam", map[string]any{
		"Type": modelType,
	}, func() ([]sqlc.Model, error) {
		return queries.ListModelsByType(ctx, modelType)
	})
}

func listModelsByProviderClientTypeForScope(ctx context.Context, queries dbstore.Queries, clientType string) ([]sqlc.Model, error) {
	return InvokeTeamQuery[[]sqlc.Model](ctx, queries, "ListModelsByProviderClientTypeForTeam", map[string]any{
		"ClientType": clientType,
	}, func() ([]sqlc.Model, error) {
		return queries.ListModelsByProviderClientType(ctx, clientType)
	})
}

func listEnabledModelsForScope(ctx context.Context, queries dbstore.Queries) ([]sqlc.Model, error) {
	return InvokeTeamQuery[[]sqlc.Model](ctx, queries, "ListEnabledModelsForTeam", nil, func() ([]sqlc.Model, error) {
		return queries.ListEnabledModels(ctx)
	})
}

func listEnabledModelsByTypeForScope(ctx context.Context, queries dbstore.Queries, modelType string) ([]sqlc.Model, error) {
	return InvokeTeamQuery[[]sqlc.Model](ctx, queries, "ListEnabledModelsByTypeForTeam", map[string]any{
		"Type": modelType,
	}, func() ([]sqlc.Model, error) {
		return queries.ListEnabledModelsByType(ctx, modelType)
	})
}

func listEnabledModelsByProviderClientTypeForScope(ctx context.Context, queries dbstore.Queries, clientType string) ([]sqlc.Model, error) {
	return InvokeTeamQuery[[]sqlc.Model](ctx, queries, "ListEnabledModelsByProviderClientTypeForTeam", map[string]any{
		"ClientType": clientType,
	}, func() ([]sqlc.Model, error) {
		return queries.ListEnabledModelsByProviderClientType(ctx, clientType)
	})
}

func listModelsByProviderIDForScope(ctx context.Context, queries dbstore.Queries, providerID pgtype.UUID) ([]sqlc.Model, error) {
	return InvokeTeamQuery[[]sqlc.Model](ctx, queries, "ListModelsByProviderIDForTeam", map[string]any{
		"ProviderID": providerID,
	}, func() ([]sqlc.Model, error) {
		return queries.ListModelsByProviderID(ctx, providerID)
	})
}

func listModelsByProviderIDAndTypeForScope(ctx context.Context, queries dbstore.Queries, arg sqlc.ListModelsByProviderIDAndTypeParams) ([]sqlc.Model, error) {
	return InvokeTeamQuery[[]sqlc.Model](ctx, queries, "ListModelsByProviderIDAndTypeForTeam", map[string]any{
		"ProviderID": arg.ProviderID,
		"Type":       arg.Type,
	}, func() ([]sqlc.Model, error) {
		return queries.ListModelsByProviderIDAndType(ctx, arg)
	})
}

func getModelByProviderAndModelIDForScope(ctx context.Context, queries dbstore.Queries, arg sqlc.GetModelByProviderAndModelIDParams) (sqlc.Model, error) {
	return InvokeTeamQuery[sqlc.Model](ctx, queries, "GetModelByProviderAndModelIDForTeam", map[string]any{
		"ProviderID": arg.ProviderID,
		"ModelID":    arg.ModelID,
	}, func() (sqlc.Model, error) {
		return queries.GetModelByProviderAndModelID(ctx, arg)
	})
}

func deleteModelForScope(ctx context.Context, queries dbstore.Queries, id pgtype.UUID) error {
	return InvokeTeamExec(ctx, queries, "DeleteModelForTeam", map[string]any{
		"ID": id,
	}, func() error {
		return queries.DeleteModel(ctx, sqlc.DeleteModelParams{ID: id, TeamID: TeamIDFromContext(ctx)})
	})
}

func countModelsForScope(ctx context.Context, queries dbstore.Queries) (int64, error) {
	return InvokeTeamQuery[int64](ctx, queries, "CountModelsForTeam", nil, func() (int64, error) {
		return queries.CountModels(ctx)
	})
}

func countModelsByTypeForScope(ctx context.Context, queries dbstore.Queries, modelType string) (int64, error) {
	return InvokeTeamQuery[int64](ctx, queries, "CountModelsByTypeForTeam", map[string]any{
		"Type": modelType,
	}, func() (int64, error) {
		return queries.CountModelsByType(ctx, modelType)
	})
}

func getProviderByIDForScope(ctx context.Context, queries dbstore.Queries, id pgtype.UUID) (sqlc.Provider, error) {
	return InvokeTeamQuery[sqlc.Provider](ctx, queries, "GetProviderByIDForTeam", map[string]any{
		"ID": id,
	}, func() (sqlc.Provider, error) {
		return queries.GetProviderByID(ctx, sqlc.GetProviderByIDParams{ID: id, TeamID: TeamIDFromContext(ctx)})
	})
}

func getProviderOAuthTokenByProviderForScope(ctx context.Context, queries dbstore.Queries, providerID pgtype.UUID) (sqlc.ProviderOauthToken, error) {
	return InvokeTeamQuery[sqlc.ProviderOauthToken](ctx, queries, "GetProviderOAuthTokenByProviderForTeam", map[string]any{
		"ProviderID": providerID,
	}, func() (sqlc.ProviderOauthToken, error) {
		return queries.GetProviderOAuthTokenByProvider(ctx, providerID)
	})
}

func getUserProviderOAuthTokenForScope(ctx context.Context, queries dbstore.Queries, arg sqlc.GetUserProviderOAuthTokenParams) (sqlc.UserProviderOauthToken, error) {
	return InvokeTeamQuery[sqlc.UserProviderOauthToken](ctx, queries, "GetUserProviderOAuthTokenForTeam", map[string]any{
		"ProviderID": arg.ProviderID,
		"UserID":     arg.UserID,
	}, func() (sqlc.UserProviderOauthToken, error) {
		return queries.GetUserProviderOAuthToken(ctx, arg)
	})
}
