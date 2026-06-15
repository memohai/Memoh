package store

import (
	"context"
	"database/sql"
	"reflect"

	"github.com/jackc/pgx/v5/pgtype"

	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	sqlitesqlc "github.com/memohai/memoh/internal/db/sqlite/sqlc"
)

func (q *Queries) CreateHistoryTurn(ctx context.Context, arg pgsqlc.CreateHistoryTurnParams) (pgtype.UUID, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgtype.UUID{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateHistoryTurnParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgtype.UUID{}, err
	}
	out, err := q.store.queries.CreateHistoryTurn(ctx, sqliteArg)
	if err != nil {
		return pgtype.UUID{}, mapQueryErr(err)
	}
	var result pgtype.UUID
	if err := convertValue(out, &result); err != nil {
		return pgtype.UUID{}, err
	}
	return result, nil
}

func (q *Queries) GetOpenHistoryTurnForBranch(ctx context.Context, arg pgsqlc.GetOpenHistoryTurnForBranchParams) (pgtype.UUID, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgtype.UUID{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetOpenHistoryTurnForBranchParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgtype.UUID{}, err
	}
	out, err := q.store.queries.GetOpenHistoryTurnForBranch(ctx, sqliteArg)
	if err != nil {
		return pgtype.UUID{}, mapQueryErr(err)
	}
	var result pgtype.UUID
	if err := convertValue(out, &result); err != nil {
		return pgtype.UUID{}, err
	}
	return result, nil
}

func (q *Queries) SetHistoryTurnRequestMessage(ctx context.Context, arg pgsqlc.SetHistoryTurnRequestMessageParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.SetHistoryTurnRequestMessageParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.SetHistoryTurnRequestMessage(ctx, sqliteArg))
}

func (q *Queries) CompleteHistoryTurnWithAssistant(ctx context.Context, arg pgsqlc.CompleteHistoryTurnWithAssistantParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CompleteHistoryTurnWithAssistantParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.CompleteHistoryTurnWithAssistant(ctx, sqliteArg))
}

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
	result := make([]pgsqlc.ListSessionBranchesRow, 0, len(out))
	for _, row := range out {
		mapped, err := mapListSessionBranchesRow(row)
		if err != nil {
			return nil, err
		}
		result = append(result, mapped)
	}
	return result, nil
}

func mapListSessionBranchesRow(row sqlitesqlc.ListSessionBranchesRow) (pgsqlc.ListSessionBranchesRow, error) {
	var result pgsqlc.ListSessionBranchesRow
	if err := convertValue(row.ID, &result.ID); err != nil {
		return result, err
	}
	if err := convertValue(row.SessionID, &result.SessionID); err != nil {
		return result, err
	}
	if err := convertValue(row.ParentBranchID, &result.ParentBranchID); err != nil {
		return result, err
	}
	if err := convertValue(row.ForkFromMessageID, &result.ForkFromMessageID); err != nil {
		return result, err
	}
	if err := convertValue(row.ForkFromSeq, &result.ForkFromSeq); err != nil {
		return result, err
	}
	if err := convertValue(row.ForkFromTurnID, &result.ForkFromTurnID); err != nil {
		return result, err
	}
	if row.ForkFromTurnSeq != nil {
		result.ForkFromTurnSeq = pgtype.Int8{Int64: intValue(reflect.ValueOf(row.ForkFromTurnSeq)), Valid: true}
	}
	if err := convertValue(row.Title, &result.Title); err != nil {
		return result, err
	}
	if err := convertValue(row.CreatedAt, &result.CreatedAt); err != nil {
		return result, err
	}
	if err := convertValue(row.UpdatedAt, &result.UpdatedAt); err != nil {
		return result, err
	}
	if err := convertValue(row.ActiveBranchID, &result.ActiveBranchID); err != nil {
		return result, err
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
