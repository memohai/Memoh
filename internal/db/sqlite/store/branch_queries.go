package store

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5/pgtype"

	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	sqlitesqlc "github.com/memohai/memoh/internal/db/sqlite/sqlc"
)

func (q *Queries) GetActiveSessionBranch(ctx context.Context, sessionID pgtype.UUID) (pgtype.UUID, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgtype.UUID{}, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID sql.NullString
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return pgtype.UUID{}, err
	}
	out, err := q.store.queries.GetActiveSessionBranch(ctx, sqliteSessionID.String)
	if err != nil {
		return pgtype.UUID{}, mapQueryErr(err)
	}
	var result pgtype.UUID
	if err := convertValue(out, &result); err != nil {
		return pgtype.UUID{}, err
	}
	return result, nil
}

func (q *Queries) SetActiveSessionBranch(ctx context.Context, arg pgsqlc.SetActiveSessionBranchParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.SetActiveSessionBranchParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.SetActiveSessionBranch(ctx, sqliteArg))
}

func (q *Queries) GetRootSessionBranch(ctx context.Context, sessionID pgtype.UUID) (pgtype.UUID, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgtype.UUID{}, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID sql.NullString
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return pgtype.UUID{}, err
	}
	out, err := q.store.queries.GetRootSessionBranch(ctx, sqliteSessionID.String)
	if err != nil {
		return pgtype.UUID{}, mapQueryErr(err)
	}
	var result pgtype.UUID
	if err := convertValue(out, &result); err != nil {
		return pgtype.UUID{}, err
	}
	return result, nil
}

func (q *Queries) CreateRootSessionBranch(ctx context.Context, sessionID pgtype.UUID) (pgtype.UUID, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgtype.UUID{}, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID sql.NullString
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return pgtype.UUID{}, err
	}
	out, err := q.store.queries.CreateRootSessionBranch(ctx, sqliteSessionID.String)
	if err != nil {
		return pgtype.UUID{}, mapQueryErr(err)
	}
	var result pgtype.UUID
	if err := convertValue(out, &result); err != nil {
		return pgtype.UUID{}, err
	}
	return result, nil
}

func (q *Queries) CreateSessionBranchFromMessage(ctx context.Context, arg pgsqlc.CreateSessionBranchFromMessageParams) (pgtype.UUID, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgtype.UUID{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateSessionBranchFromMessageParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgtype.UUID{}, err
	}
	out, err := q.store.queries.CreateSessionBranchFromMessage(ctx, sqliteArg)
	if err != nil {
		return pgtype.UUID{}, mapQueryErr(err)
	}
	var result pgtype.UUID
	if err := convertValue(out, &result); err != nil {
		return pgtype.UUID{}, err
	}
	return result, nil
}

func (q *Queries) GetMessageForSessionBranchFork(ctx context.Context, arg pgsqlc.GetMessageForSessionBranchForkParams) (pgsqlc.GetMessageForSessionBranchForkRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetMessageForSessionBranchForkRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetMessageForSessionBranchForkParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.GetMessageForSessionBranchForkRow{}, err
	}
	out, err := q.store.queries.GetMessageForSessionBranchFork(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.GetMessageForSessionBranchForkRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetMessageForSessionBranchForkRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetMessageForSessionBranchForkRow{}, err
	}
	return result, nil
}

func (q *Queries) GetSessionBranchForkPoint(ctx context.Context, arg pgsqlc.GetSessionBranchForkPointParams) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID sql.NullString
	if err := convertValue(arg.SessionID, &sqliteSessionID); err != nil {
		return 0, err
	}
	var sqliteBranchID sql.NullString
	if err := convertValue(arg.BranchID, &sqliteBranchID); err != nil {
		return 0, err
	}
	sqliteArg := sqlitesqlc.GetSessionBranchForkPointParams{
		SessionID: sqliteSessionID,
		BranchID:  sqliteBranchID,
		BranchSeq: sql.NullInt64{Int64: arg.BranchSeq, Valid: true},
	}
	out, err := q.store.queries.GetSessionBranchForkPoint(ctx, sqliteArg)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	return out, nil
}

func (q *Queries) ListSessionBranches(ctx context.Context, sessionID pgtype.UUID) ([]pgsqlc.ListSessionBranchesRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID sql.NullString
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSessionBranches(ctx, sqliteSessionID.String)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListSessionBranchesRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSessionBranchPreviewMessages(ctx context.Context, sessionID pgtype.UUID) ([]pgsqlc.ListSessionBranchPreviewMessagesRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID sql.NullString
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSessionBranchPreviewMessages(ctx, sqliteSessionID.String)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListSessionBranchPreviewMessagesRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSessionBranchTurnMessages(ctx context.Context, sessionID pgtype.UUID) ([]pgsqlc.ListSessionBranchTurnMessagesRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID sql.NullString
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSessionBranchTurnMessages(ctx, sqliteSessionID.String)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListSessionBranchTurnMessagesRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}
