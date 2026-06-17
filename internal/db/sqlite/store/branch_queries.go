package store

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	sqlitesqlc "github.com/memohai/memoh/internal/db/sqlite/sqlc"
)

const sqliteGetMessageForSessionBranchFork = `
SELECT
  m.id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
  m.turn_id,
  t.turn_seq,
  (
    SELECT pt.id
    FROM bot_history_turns pt
    WHERE pt.branch_id = t.branch_id
      AND pt.turn_seq < t.turn_seq
      AND pt.status = 'completed'
    ORDER BY pt.turn_seq DESC
    LIMIT 1
  ) AS previous_turn_id,
  COALESCE((
    SELECT pt.turn_seq
    FROM bot_history_turns pt
    WHERE pt.branch_id = t.branch_id
      AND pt.turn_seq < t.turn_seq
      AND pt.status = 'completed'
    ORDER BY pt.turn_seq DESC
    LIMIT 1
  ), 0) AS previous_turn_seq,
  (
    SELECT pm.branch_seq
    FROM bot_history_turns pt
    JOIN bot_history_messages pm ON pm.id = pt.final_assistant_message_id
    WHERE pt.branch_id = t.branch_id
      AND pt.turn_seq < t.turn_seq
      AND pt.status = 'completed'
    ORDER BY pt.turn_seq DESC
    LIMIT 1
  ) AS previous_branch_seq,
  m.role,
  m.created_at
FROM bot_history_messages m
JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.id = ?
  AND m.session_id = ?
LIMIT 1
`

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

func (q *Queries) GetSessionBranchForPersist(ctx context.Context, arg pgsqlc.GetSessionBranchForPersistParams) (pgtype.UUID, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgtype.UUID{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetSessionBranchForPersistParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgtype.UUID{}, err
	}
	out, err := q.store.queries.GetSessionBranchForPersist(ctx, sqliteArg)
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

func (q *Queries) IsSessionPersistContextVisible(ctx context.Context, arg pgsqlc.IsSessionPersistContextVisibleParams) (bool, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return false, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.IsSessionPersistContextVisibleParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return false, err
	}
	visible, err := q.store.queries.IsSessionPersistContextVisible(ctx, sqliteArg)
	if err != nil {
		return false, mapQueryErr(err)
	}
	return sqliteBool(visible)
}

func sqliteBool(value any) (bool, error) {
	switch v := value.(type) {
	case nil:
		return false, nil
	case bool:
		return v, nil
	case int64:
		return v != 0, nil
	case int:
		return v != 0, nil
	case []byte:
		parsed, err := strconv.ParseBool(string(v))
		if err == nil {
			return parsed, nil
		}
		asInt, intErr := strconv.ParseInt(string(v), 10, 64)
		if intErr == nil {
			return asInt != 0, nil
		}
		return false, err
	case string:
		parsed, err := strconv.ParseBool(v)
		if err == nil {
			return parsed, nil
		}
		asInt, intErr := strconv.ParseInt(v, 10, 64)
		if intErr == nil {
			return asInt != 0, nil
		}
		return false, err
	default:
		return false, fmt.Errorf("unsupported sqlite bool value %T", value)
	}
}

func (q *Queries) SetActiveSessionBranch(ctx context.Context, arg pgsqlc.SetActiveSessionBranchParams) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.SetActiveSessionBranchParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return 0, err
	}
	rowsAffected, err := q.store.queries.SetActiveSessionBranch(ctx, sqliteArg)
	return rowsAffected, mapQueryErr(err)
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

func (q *Queries) DeleteSessionBranch(ctx context.Context, arg pgsqlc.DeleteSessionBranchParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.DeleteSessionBranchParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.DeleteSessionBranch(ctx, sqliteArg))
}

func (q *Queries) GetMessageForSessionBranchFork(ctx context.Context, arg pgsqlc.GetMessageForSessionBranchForkParams) (pgsqlc.GetMessageForSessionBranchForkRow, error) {
	if q == nil || q.store == nil || q.store.db == nil {
		return pgsqlc.GetMessageForSessionBranchForkRow{}, errSQLiteQueriesNotConfigured
	}
	var messageID pgtype.UUID
	if err := convertValue(arg.MessageID, &messageID); err != nil {
		return pgsqlc.GetMessageForSessionBranchForkRow{}, err
	}
	var sessionID sql.NullString
	if err := convertValue(arg.SessionID, &sessionID); err != nil {
		return pgsqlc.GetMessageForSessionBranchForkRow{}, err
	}

	var row struct {
		ID                string
		SessionID         sql.NullString
		BranchID          sql.NullString
		BranchSeq         sql.NullInt64
		TurnID            sql.NullString
		TurnSeq           int64
		PreviousTurnID    sql.NullString
		PreviousTurnSeq   sql.NullInt64
		PreviousBranchSeq sql.NullInt64
		Role              string
		CreatedAt         string
	}
	err := q.store.db.QueryRowContext(ctx, sqliteGetMessageForSessionBranchFork, messageID.String(), sessionID.String).Scan(
		&row.ID,
		&row.SessionID,
		&row.BranchID,
		&row.BranchSeq,
		&row.TurnID,
		&row.TurnSeq,
		&row.PreviousTurnID,
		&row.PreviousTurnSeq,
		&row.PreviousBranchSeq,
		&row.Role,
		&row.CreatedAt,
	)
	if err != nil {
		return pgsqlc.GetMessageForSessionBranchForkRow{}, mapQueryErr(err)
	}

	var result pgsqlc.GetMessageForSessionBranchForkRow
	if err := convertValue(row.ID, &result.ID); err != nil {
		return pgsqlc.GetMessageForSessionBranchForkRow{}, err
	}
	if err := convertValue(row.SessionID, &result.SessionID); err != nil {
		return pgsqlc.GetMessageForSessionBranchForkRow{}, err
	}
	if err := convertValue(row.BranchID, &result.BranchID); err != nil {
		return pgsqlc.GetMessageForSessionBranchForkRow{}, err
	}
	if row.BranchSeq.Valid {
		result.BranchSeq = pgtype.Int8{Int64: row.BranchSeq.Int64, Valid: true}
	}
	if err := convertValue(row.TurnID, &result.TurnID); err != nil {
		return pgsqlc.GetMessageForSessionBranchForkRow{}, err
	}
	result.TurnSeq = row.TurnSeq
	if row.PreviousTurnID.Valid {
		if err := convertValue(row.PreviousTurnID, &result.PreviousTurnID); err != nil {
			return pgsqlc.GetMessageForSessionBranchForkRow{}, err
		}
	}
	if row.PreviousTurnSeq.Valid {
		result.PreviousTurnSeq = row.PreviousTurnSeq.Int64
	}
	if row.PreviousBranchSeq.Valid {
		result.PreviousBranchSeq = pgtype.Int8{Int64: row.PreviousBranchSeq.Int64, Valid: true}
	}
	result.Role = row.Role
	if err := convertValue(row.CreatedAt, &result.CreatedAt); err != nil {
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
