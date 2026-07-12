package postgresstore

import (
	"context"

	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

func (s *Store) UpsertBotRemoteRuntimeBinding(ctx context.Context, input dbstore.UpsertBotRemoteRuntimeBindingInput) (dbstore.BotRemoteRuntimeBindingRecord, error) {
	botID, err := db.ParseUUID(input.BotID)
	if err != nil {
		return dbstore.BotRemoteRuntimeBindingRecord{}, err
	}
	runtimeID, err := db.ParseUUID(input.RuntimeID)
	if err != nil {
		return dbstore.BotRemoteRuntimeBindingRecord{}, err
	}
	if _, err := s.queries.UpsertBotRemoteRuntimeBinding(ctx, dbsqlc.UpsertBotRemoteRuntimeBindingParams{
		BotID: botID, RuntimeID: runtimeID, WorkspacePath: input.WorkspacePath,
	}); err != nil {
		return dbstore.BotRemoteRuntimeBindingRecord{}, mapQueryErr(err)
	}
	return s.GetBotRemoteRuntimeBinding(ctx, input.BotID)
}

func (s *Store) GetBotRemoteRuntimeBinding(ctx context.Context, botID string) (dbstore.BotRemoteRuntimeBindingRecord, error) {
	id, err := db.ParseUUID(botID)
	if err != nil {
		return dbstore.BotRemoteRuntimeBindingRecord{}, err
	}
	row, err := s.queries.GetBotRemoteRuntimeBinding(ctx, id)
	if err != nil {
		return dbstore.BotRemoteRuntimeBindingRecord{}, mapQueryErr(err)
	}
	return dbstore.BotRemoteRuntimeBindingRecord{
		BotID:          row.BotID.String(),
		RuntimeID:      row.RuntimeID.String(),
		WorkspacePath:  row.WorkspacePath,
		RuntimeName:    row.RuntimeName,
		RuntimeUserID:  row.RuntimeUserID.String(),
		BotOwnerUserID: row.BotOwnerUserID.String(),
		RuntimeRevoked: row.RuntimeUnavailable.Bool,
		CreatedAt:      db.TimeFromPg(row.CreatedAt),
		UpdatedAt:      db.TimeFromPg(row.UpdatedAt),
	}, nil
}

func (s *Store) DeleteBotRemoteRuntimeBinding(ctx context.Context, botID string) error {
	id, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	_, err = s.queries.DeleteBotRemoteRuntimeBinding(ctx, id)
	return mapQueryErr(err)
}
