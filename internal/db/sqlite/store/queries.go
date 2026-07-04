package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	sqlitesqlc "github.com/memohai/memoh/internal/db/sqlite/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

var errSQLiteQueriesNotConfigured = errors.New("sqlite queries not configured")

type Queries struct {
	store *Store
}

func NewQueries(store *Store) *Queries {
	return &Queries{store: store}
}

func (q *Queries) WithTx(_ pgx.Tx) dbstore.Queries {
	return q
}

func (q *Queries) ApproveToolApprovalRequest(ctx context.Context, arg pgsqlc.ApproveToolApprovalRequestParams) (pgsqlc.ToolApprovalRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ToolApprovalRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ApproveToolApprovalRequestParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	out, err := q.store.queries.ApproveToolApprovalRequest(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ToolApprovalRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.ToolApprovalRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	return result, nil
}

func (q *Queries) CancelPendingToolApprovalsBySession(ctx context.Context, arg pgsqlc.CancelPendingToolApprovalsBySessionParams) ([]pgsqlc.ToolApprovalRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CancelPendingToolApprovalsBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.CancelPendingToolApprovalsBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ToolApprovalRequest
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) CancelPendingUserInputsBySession(ctx context.Context, arg pgsqlc.CancelPendingUserInputsBySessionParams) ([]pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CancelPendingUserInputsBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.CancelPendingUserInputsBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ClearMCPOAuthTokens(ctx context.Context, connectionID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteConnectionID string
	if err := convertValue(connectionID, &sqliteConnectionID); err != nil {
		return err
	}
	err := q.store.queries.ClearMCPOAuthTokens(ctx, sqliteConnectionID)
	return mapQueryErr(err)
}

func (q *Queries) CompleteCompactionLog(ctx context.Context, arg pgsqlc.CompleteCompactionLogParams) (pgsqlc.BotHistoryMessageCompact, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotHistoryMessageCompact{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CompleteCompactionLogParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotHistoryMessageCompact{}, err
	}
	out, err := q.store.queries.CompleteCompactionLog(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotHistoryMessageCompact{}, mapQueryErr(err)
	}
	var result pgsqlc.BotHistoryMessageCompact
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotHistoryMessageCompact{}, err
	}
	return result, nil
}

func (q *Queries) CompleteHeartbeatLog(ctx context.Context, arg pgsqlc.CompleteHeartbeatLogParams) (pgsqlc.BotHeartbeatLog, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotHeartbeatLog{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CompleteHeartbeatLogParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotHeartbeatLog{}, err
	}
	out, err := q.store.queries.CompleteHeartbeatLog(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotHeartbeatLog{}, mapQueryErr(err)
	}
	var result pgsqlc.BotHeartbeatLog
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotHeartbeatLog{}, err
	}
	return result, nil
}

func (q *Queries) CompleteScheduleLog(ctx context.Context, arg pgsqlc.CompleteScheduleLogParams) (pgsqlc.ScheduleLog, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ScheduleLog{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CompleteScheduleLogParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ScheduleLog{}, err
	}
	out, err := q.store.queries.CompleteScheduleLog(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ScheduleLog{}, mapQueryErr(err)
	}
	var result pgsqlc.ScheduleLog
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ScheduleLog{}, err
	}
	return result, nil
}

func (q *Queries) CountAccounts(ctx context.Context) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.CountAccounts(ctx)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) CountCompactionLogsByBot(ctx context.Context, botID pgtype.UUID) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return 0, err
	}
	out, err := q.store.queries.CountCompactionLogsByBot(ctx, sqliteBotID)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) CountEmailOutboxByBot(ctx context.Context, botID pgtype.UUID) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return 0, err
	}
	out, err := q.store.queries.CountEmailOutboxByBot(ctx, sqliteBotID)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) CountHeartbeatLogsByBot(ctx context.Context, botID pgtype.UUID) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return 0, err
	}
	out, err := q.store.queries.CountHeartbeatLogsByBot(ctx, sqliteBotID)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) CountMemoryProvidersByDefault(ctx context.Context) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.CountMemoryProvidersByDefault(ctx)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) CountMessageAssetsByBot(ctx context.Context, botID pgtype.UUID) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return 0, err
	}
	out, err := q.store.queries.CountMessageAssetsByBot(ctx, sqliteBotID)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) CountMessagesByBot(ctx context.Context, botID pgtype.UUID) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return 0, err
	}
	out, err := q.store.queries.CountMessagesByBot(ctx, sqliteBotID)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) CountMessagesBySession(ctx context.Context, sessionID pgtype.UUID) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID sql.NullString
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return 0, err
	}
	out, err := q.store.queries.CountMessagesBySession(ctx, sqliteSessionID)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) CountModels(ctx context.Context) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.CountModels(ctx)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) CountModelsByType(ctx context.Context, type_ string) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteType_ string
	if err := convertValue(type_, &sqliteType_); err != nil {
		return 0, err
	}
	out, err := q.store.queries.CountModelsByType(ctx, sqliteType_)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) CountProviders(ctx context.Context) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.CountProviders(ctx)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) CountScheduleLogsByBot(ctx context.Context, botID pgtype.UUID) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return 0, err
	}
	out, err := q.store.queries.CountScheduleLogsByBot(ctx, sqliteBotID)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) CountScheduleLogsBySchedule(ctx context.Context, scheduleID pgtype.UUID) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteScheduleID string
	if err := convertValue(scheduleID, &sqliteScheduleID); err != nil {
		return 0, err
	}
	out, err := q.store.queries.CountScheduleLogsBySchedule(ctx, sqliteScheduleID)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) CountSessionEvents(ctx context.Context, sessionID pgtype.UUID) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID string
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return 0, err
	}
	out, err := q.store.queries.CountSessionEvents(ctx, sqliteSessionID)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) DeleteSessionEventsByBot(ctx context.Context, botID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return err
	}
	err := q.store.queries.DeleteSessionEventsByBot(ctx, sqliteBotID)
	return mapQueryErr(err)
}

func (q *Queries) CountTokenUsageRecords(ctx context.Context, arg pgsqlc.CountTokenUsageRecordsParams) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CountTokenUsageRecordsParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return 0, err
	}
	out, err := q.store.queries.CountTokenUsageRecords(ctx, sqliteArg)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) CreateAccount(ctx context.Context, arg pgsqlc.CreateAccountParams) (pgsqlc.User, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.User{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateAccountParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.User{}, err
	}
	out, err := q.store.queries.CreateAccount(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.User{}, mapQueryErr(err)
	}
	var result pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.User{}, err
	}
	return result, nil
}

func (q *Queries) CreateBot(ctx context.Context, arg pgsqlc.CreateBotParams) (pgsqlc.CreateBotRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.CreateBotRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateBotParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.CreateBotRow{}, err
	}
	out, err := q.store.queries.CreateBot(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.CreateBotRow{}, mapQueryErr(err)
	}
	var result pgsqlc.CreateBotRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.CreateBotRow{}, err
	}
	return result, nil
}

func (q *Queries) CreateBotACLRule(ctx context.Context, arg pgsqlc.CreateBotACLRuleParams) (pgsqlc.BotAclRule, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotAclRule{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateBotACLRuleParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotAclRule{}, err
	}
	out, err := q.store.queries.CreateBotACLRule(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotAclRule{}, mapQueryErr(err)
	}
	var result pgsqlc.BotAclRule
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotAclRule{}, err
	}
	return result, nil
}

func (q *Queries) CreateBotUserGrant(ctx context.Context, arg pgsqlc.CreateBotUserGrantParams) (pgsqlc.BotUserGrant, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotUserGrant{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateBotUserGrantParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotUserGrant{}, err
	}
	out, err := q.store.queries.CreateBotUserGrant(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotUserGrant{}, mapQueryErr(err)
	}
	var result pgsqlc.BotUserGrant
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotUserGrant{}, err
	}
	return result, nil
}

func (q *Queries) GetBotUserGrantByID(ctx context.Context, id pgtype.UUID) (pgsqlc.BotUserGrant, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotUserGrant{}, errSQLiteQueriesNotConfigured
	}
	var sqliteID string
	if err := convertValue(id, &sqliteID); err != nil {
		return pgsqlc.BotUserGrant{}, err
	}
	out, err := q.store.queries.GetBotUserGrantByID(ctx, sqliteID)
	if err != nil {
		return pgsqlc.BotUserGrant{}, mapQueryErr(err)
	}
	var result pgsqlc.BotUserGrant
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotUserGrant{}, err
	}
	return result, nil
}

func (q *Queries) UpdateBotUserGrantPermissions(ctx context.Context, arg pgsqlc.UpdateBotUserGrantPermissionsParams) (pgsqlc.BotUserGrant, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotUserGrant{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateBotUserGrantPermissionsParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotUserGrant{}, err
	}
	out, err := q.store.queries.UpdateBotUserGrantPermissions(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotUserGrant{}, mapQueryErr(err)
	}
	var result pgsqlc.BotUserGrant
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotUserGrant{}, err
	}
	return result, nil
}

func (q *Queries) DeleteBotUserGrantByID(ctx context.Context, id pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteID string
	if err := convertValue(id, &sqliteID); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.DeleteBotUserGrantByID(ctx, sqliteID))
}

func (q *Queries) UpsertBotChannelAdmin(ctx context.Context, arg pgsqlc.UpsertBotChannelAdminParams) (pgsqlc.BotChannelAdmin, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotChannelAdmin{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertBotChannelAdminParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotChannelAdmin{}, err
	}
	out, err := q.store.queries.UpsertBotChannelAdmin(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotChannelAdmin{}, mapQueryErr(err)
	}
	var result pgsqlc.BotChannelAdmin
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotChannelAdmin{}, err
	}
	return result, nil
}

func (q *Queries) CreateChannelLinkCode(ctx context.Context, arg pgsqlc.CreateChannelLinkCodeParams) (pgsqlc.ChannelLinkCode, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ChannelLinkCode{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateChannelLinkCodeParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ChannelLinkCode{}, err
	}
	out, err := q.store.queries.CreateChannelLinkCode(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ChannelLinkCode{}, mapQueryErr(err)
	}
	var result pgsqlc.ChannelLinkCode
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ChannelLinkCode{}, err
	}
	return result, nil
}

func (q *Queries) GetChannelLinkCodeByToken(ctx context.Context, token string) (pgsqlc.ChannelLinkCode, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ChannelLinkCode{}, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.GetChannelLinkCodeByToken(ctx, token)
	if err != nil {
		return pgsqlc.ChannelLinkCode{}, mapQueryErr(err)
	}
	var result pgsqlc.ChannelLinkCode
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ChannelLinkCode{}, err
	}
	return result, nil
}

func (q *Queries) MarkChannelLinkCodeConsumed(ctx context.Context, arg pgsqlc.MarkChannelLinkCodeConsumedParams) (pgsqlc.ChannelLinkCode, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ChannelLinkCode{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.MarkChannelLinkCodeConsumedParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ChannelLinkCode{}, err
	}
	out, err := q.store.queries.MarkChannelLinkCodeConsumed(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ChannelLinkCode{}, mapQueryErr(err)
	}
	var result pgsqlc.ChannelLinkCode
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ChannelLinkCode{}, err
	}
	return result, nil
}

func (q *Queries) RedeemChannelLinkCode(ctx context.Context, arg pgsqlc.RedeemChannelLinkCodeParams) (pgsqlc.UserChannelIdentityBinding, error) {
	if q == nil || q.store == nil || q.store.db == nil || q.store.queries == nil {
		return pgsqlc.UserChannelIdentityBinding{}, errSQLiteQueriesNotConfigured
	}
	tx, err := q.store.db.BeginTx(ctx, nil)
	if err != nil {
		return pgsqlc.UserChannelIdentityBinding{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	qtx := q.store.queries.WithTx(tx)

	var markArg sqlitesqlc.MarkChannelLinkCodeConsumedParams
	if err := convertValue(pgsqlc.MarkChannelLinkCodeConsumedParams{
		Token:                     arg.Token,
		ConsumedChannelIdentityID: arg.ChannelIdentityID,
	}, &markArg); err != nil {
		return pgsqlc.UserChannelIdentityBinding{}, err
	}
	code, err := qtx.MarkChannelLinkCodeConsumed(ctx, markArg)
	if err != nil {
		return pgsqlc.UserChannelIdentityBinding{}, mapQueryErr(err)
	}

	binding, err := qtx.UpsertUserChannelIdentityBinding(ctx, sqlitesqlc.UpsertUserChannelIdentityBindingParams{
		UserID:            code.UserID,
		ChannelIdentityID: markArg.ConsumedChannelIdentityID.String,
	})
	if err != nil {
		return pgsqlc.UserChannelIdentityBinding{}, mapQueryErr(err)
	}
	if err := tx.Commit(); err != nil {
		return pgsqlc.UserChannelIdentityBinding{}, err
	}

	var result pgsqlc.UserChannelIdentityBinding
	if err := convertValue(binding, &result); err != nil {
		return pgsqlc.UserChannelIdentityBinding{}, err
	}
	return result, nil
}

func (q *Queries) UpsertUserChannelIdentityBinding(ctx context.Context, arg pgsqlc.UpsertUserChannelIdentityBindingParams) (pgsqlc.UserChannelIdentityBinding, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserChannelIdentityBinding{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertUserChannelIdentityBindingParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserChannelIdentityBinding{}, err
	}
	out, err := q.store.queries.UpsertUserChannelIdentityBinding(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserChannelIdentityBinding{}, mapQueryErr(err)
	}
	var result pgsqlc.UserChannelIdentityBinding
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserChannelIdentityBinding{}, err
	}
	return result, nil
}

func (q *Queries) ListChannelIdentityBindings(ctx context.Context) ([]pgsqlc.ListChannelIdentityBindingsRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListChannelIdentityBindings(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListChannelIdentityBindingsRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListChannelIdentityBindingsForUser(ctx context.Context, userID pgtype.UUID) ([]pgsqlc.ListChannelIdentityBindingsForUserRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteUserID string
	if err := convertValue(userID, &sqliteUserID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListChannelIdentityBindingsForUser(ctx, sqliteUserID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListChannelIdentityBindingsForUserRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListChannelIdentityBindingsForBot(ctx context.Context, botID pgtype.UUID) ([]pgsqlc.ListChannelIdentityBindingsForBotRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListChannelIdentityBindingsForBot(ctx, sqliteBotID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListChannelIdentityBindingsForBotRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) DeleteUserChannelIdentityBinding(ctx context.Context, arg pgsqlc.DeleteUserChannelIdentityBindingParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.DeleteUserChannelIdentityBindingParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.DeleteUserChannelIdentityBinding(ctx, sqliteArg))
}

func (q *Queries) ListUserIDsByChannelIdentity(ctx context.Context, channelIdentityID pgtype.UUID) ([]pgtype.UUID, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteID string
	if err := convertValue(channelIdentityID, &sqliteID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListUserIDsByChannelIdentity(ctx, sqliteID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	result := make([]pgtype.UUID, 0, len(out))
	for _, idStr := range out {
		var id pgtype.UUID
		if err := convertValue(idStr, &id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, nil
}

func (q *Queries) DeleteBotChannelAdmin(ctx context.Context, arg pgsqlc.DeleteBotChannelAdminParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.DeleteBotChannelAdminParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.DeleteBotChannelAdmin(ctx, sqliteArg))
}

func (q *Queries) GetBotChannelAdmin(ctx context.Context, arg pgsqlc.GetBotChannelAdminParams) (pgsqlc.BotChannelAdmin, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotChannelAdmin{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetBotChannelAdminParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotChannelAdmin{}, err
	}
	out, err := q.store.queries.GetBotChannelAdmin(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotChannelAdmin{}, mapQueryErr(err)
	}
	var result pgsqlc.BotChannelAdmin
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotChannelAdmin{}, err
	}
	return result, nil
}

func (q *Queries) ListBotChannelAdmins(ctx context.Context, botID pgtype.UUID) ([]pgsqlc.ListBotChannelAdminsRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListBotChannelAdmins(ctx, sqliteBotID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListBotChannelAdminsRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListBotUserGrants(ctx context.Context, botID pgtype.UUID) ([]pgsqlc.ListBotUserGrantsRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListBotUserGrants(ctx, sqliteBotID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListBotUserGrantsRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListBotUserGrantsForUser(ctx context.Context, arg pgsqlc.ListBotUserGrantsForUserParams) ([]pgsqlc.ListBotUserGrantsForUserRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListBotUserGrantsForUserParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListBotUserGrantsForUser(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListBotUserGrantsForUserRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListAccessibleBots(ctx context.Context, ownerUserID pgtype.UUID) ([]pgsqlc.ListAccessibleBotsRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteUserID string
	if err := convertValue(ownerUserID, &sqliteUserID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListAccessibleBots(ctx, sqliteUserID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListAccessibleBotsRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) CreateBotEmailBinding(ctx context.Context, arg pgsqlc.CreateBotEmailBindingParams) (pgsqlc.BotEmailBinding, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotEmailBinding{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateBotEmailBindingParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotEmailBinding{}, err
	}
	out, err := q.store.queries.CreateBotEmailBinding(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotEmailBinding{}, mapQueryErr(err)
	}
	var result pgsqlc.BotEmailBinding
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotEmailBinding{}, err
	}
	return result, nil
}

func (q *Queries) CreateChannelIdentity(ctx context.Context, arg pgsqlc.CreateChannelIdentityParams) (pgsqlc.ChannelIdentity, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ChannelIdentity{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateChannelIdentityParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ChannelIdentity{}, err
	}
	out, err := q.store.queries.CreateChannelIdentity(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ChannelIdentity{}, mapQueryErr(err)
	}
	var result pgsqlc.ChannelIdentity
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ChannelIdentity{}, err
	}
	return result, nil
}

func (q *Queries) CreateChat(ctx context.Context, arg pgsqlc.CreateChatParams) (pgsqlc.CreateChatRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.CreateChatRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateChatParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.CreateChatRow{}, err
	}
	out, err := q.store.queries.CreateChat(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.CreateChatRow{}, mapQueryErr(err)
	}
	var result pgsqlc.CreateChatRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.CreateChatRow{}, err
	}
	return result, nil
}

func (q *Queries) CreateChatRoute(ctx context.Context, arg pgsqlc.CreateChatRouteParams) (pgsqlc.CreateChatRouteRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.CreateChatRouteRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateChatRouteParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.CreateChatRouteRow{}, err
	}
	out, err := q.store.queries.CreateChatRoute(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.CreateChatRouteRow{}, mapQueryErr(err)
	}
	var result pgsqlc.CreateChatRouteRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.CreateChatRouteRow{}, err
	}
	return result, nil
}

func (q *Queries) CreateCompactionLog(ctx context.Context, arg pgsqlc.CreateCompactionLogParams) (pgsqlc.BotHistoryMessageCompact, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotHistoryMessageCompact{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateCompactionLogParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotHistoryMessageCompact{}, err
	}
	out, err := q.store.queries.CreateCompactionLog(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotHistoryMessageCompact{}, mapQueryErr(err)
	}
	var result pgsqlc.BotHistoryMessageCompact
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotHistoryMessageCompact{}, err
	}
	return result, nil
}

func (q *Queries) CreateEmailOutbox(ctx context.Context, arg pgsqlc.CreateEmailOutboxParams) (pgsqlc.EmailOutbox, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.EmailOutbox{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateEmailOutboxParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.EmailOutbox{}, err
	}
	out, err := q.store.queries.CreateEmailOutbox(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.EmailOutbox{}, mapQueryErr(err)
	}
	var result pgsqlc.EmailOutbox
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.EmailOutbox{}, err
	}
	return result, nil
}

func (q *Queries) CreateEmailProvider(ctx context.Context, arg pgsqlc.CreateEmailProviderParams) (pgsqlc.EmailProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.EmailProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateEmailProviderParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.EmailProvider{}, err
	}
	out, err := q.store.queries.CreateEmailProvider(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.EmailProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.EmailProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.EmailProvider{}, err
	}
	return result, nil
}

func (q *Queries) CreateHeartbeatLog(ctx context.Context, arg pgsqlc.CreateHeartbeatLogParams) (pgsqlc.CreateHeartbeatLogRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.CreateHeartbeatLogRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateHeartbeatLogParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.CreateHeartbeatLogRow{}, err
	}
	out, err := q.store.queries.CreateHeartbeatLog(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.CreateHeartbeatLogRow{}, mapQueryErr(err)
	}
	var result pgsqlc.CreateHeartbeatLogRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.CreateHeartbeatLogRow{}, err
	}
	return result, nil
}

func (q *Queries) CreateMCPConnection(ctx context.Context, arg pgsqlc.CreateMCPConnectionParams) (pgsqlc.McpConnection, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.McpConnection{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateMCPConnectionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.McpConnection{}, err
	}
	out, err := q.store.queries.CreateMCPConnection(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.McpConnection{}, mapQueryErr(err)
	}
	var result pgsqlc.McpConnection
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.McpConnection{}, err
	}
	return result, nil
}

func (q *Queries) CreateMemoryProvider(ctx context.Context, arg pgsqlc.CreateMemoryProviderParams) (pgsqlc.MemoryProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.MemoryProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateMemoryProviderParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.MemoryProvider{}, err
	}
	out, err := q.store.queries.CreateMemoryProvider(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.MemoryProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.MemoryProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.MemoryProvider{}, err
	}
	return result, nil
}

func (q *Queries) CreateMessage(ctx context.Context, arg pgsqlc.CreateMessageParams) (pgsqlc.CreateMessageRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.CreateMessageRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateMessageParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.CreateMessageRow{}, err
	}
	out, err := q.store.queries.CreateMessage(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.CreateMessageRow{}, mapQueryErr(err)
	}
	var result pgsqlc.CreateMessageRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.CreateMessageRow{}, err
	}
	return result, nil
}

func (q *Queries) CreateMessageAsset(ctx context.Context, arg pgsqlc.CreateMessageAssetParams) (pgsqlc.BotHistoryMessageAsset, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotHistoryMessageAsset{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateMessageAssetParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotHistoryMessageAsset{}, err
	}
	out, err := q.store.queries.CreateMessageAsset(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotHistoryMessageAsset{}, mapQueryErr(err)
	}
	var result pgsqlc.BotHistoryMessageAsset
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotHistoryMessageAsset{}, err
	}
	return result, nil
}

func (q *Queries) CreateModel(ctx context.Context, arg pgsqlc.CreateModelParams) (pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Model{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateModelParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.Model{}, err
	}
	out, err := q.store.queries.CreateModel(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.Model{}, mapQueryErr(err)
	}
	var result pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Model{}, err
	}
	return result, nil
}

func (q *Queries) CreateModelVariant(ctx context.Context, arg pgsqlc.CreateModelVariantParams) (pgsqlc.ModelVariant, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ModelVariant{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateModelVariantParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ModelVariant{}, err
	}
	out, err := q.store.queries.CreateModelVariant(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ModelVariant{}, mapQueryErr(err)
	}
	var result pgsqlc.ModelVariant
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ModelVariant{}, err
	}
	return result, nil
}

func (q *Queries) CreateProvider(ctx context.Context, arg pgsqlc.CreateProviderParams) (pgsqlc.Provider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Provider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateProviderParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.Provider{}, err
	}
	out, err := q.store.queries.CreateProvider(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.Provider{}, mapQueryErr(err)
	}
	var result pgsqlc.Provider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Provider{}, err
	}
	return result, nil
}

func (q *Queries) CreateSchedule(ctx context.Context, arg pgsqlc.CreateScheduleParams) (pgsqlc.Schedule, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Schedule{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateScheduleParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.Schedule{}, err
	}
	out, err := q.store.queries.CreateSchedule(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.Schedule{}, mapQueryErr(err)
	}
	var result pgsqlc.Schedule
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Schedule{}, err
	}
	return result, nil
}

func (q *Queries) CreateScheduleLog(ctx context.Context, arg pgsqlc.CreateScheduleLogParams) (pgsqlc.CreateScheduleLogRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.CreateScheduleLogRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateScheduleLogParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.CreateScheduleLogRow{}, err
	}
	out, err := q.store.queries.CreateScheduleLog(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.CreateScheduleLogRow{}, mapQueryErr(err)
	}
	var result pgsqlc.CreateScheduleLogRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.CreateScheduleLogRow{}, err
	}
	return result, nil
}

func (q *Queries) CreateFetchProvider(ctx context.Context, arg pgsqlc.CreateFetchProviderParams) (pgsqlc.FetchProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.FetchProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateFetchProviderParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.FetchProvider{}, err
	}
	out, err := q.store.queries.CreateFetchProvider(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.FetchProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.FetchProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.FetchProvider{}, err
	}
	return result, nil
}

func (q *Queries) CreateSearchProvider(ctx context.Context, arg pgsqlc.CreateSearchProviderParams) (pgsqlc.SearchProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.SearchProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateSearchProviderParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.SearchProvider{}, err
	}
	out, err := q.store.queries.CreateSearchProvider(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.SearchProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.SearchProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.SearchProvider{}, err
	}
	return result, nil
}

func (q *Queries) CreateSession(ctx context.Context, arg pgsqlc.CreateSessionParams) (pgsqlc.BotSession, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotSession{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateSessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotSession{}, err
	}
	out, err := q.store.queries.CreateSession(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotSession{}, mapQueryErr(err)
	}
	var result pgsqlc.BotSession
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotSession{}, err
	}
	return result, nil
}

func (q *Queries) ForkSessionFromAssistantMessage(ctx context.Context, arg pgsqlc.ForkSessionFromAssistantMessageParams) (pgsqlc.ForkSessionFromAssistantMessageRow, error) {
	if q == nil || q.store == nil || q.store.db == nil || q.store.queries == nil {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, errSQLiteQueriesNotConfigured
	}

	var sessionID string
	if err := convertValue(arg.SessionID, &sessionID); err != nil {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, err
	}
	var botID string
	if err := convertValue(arg.BotID, &botID); err != nil {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, err
	}
	var messageID string
	if err := convertValue(arg.MessageID, &messageID); err != nil {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, err
	}

	tx, err := q.store.db.BeginTx(ctx, nil)
	if err != nil {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	qtx := q.store.queries.WithTx(tx)

	source, err := qtx.GetSessionByID(ctx, sessionID)
	if err != nil {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, mapQueryErr(err)
	}
	if source.BotID != botID || source.Type != "chat" {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, pgx.ErrNoRows
	}

	forkTarget, err := loadSQLiteForkTarget(ctx, tx, sessionID, messageID)
	if err != nil {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, mapQueryErr(err)
	}
	turns, err := sqliteForkVisibleTurns(ctx, tx, sessionID, forkTarget)
	if err != nil {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, mapQueryErr(err)
	}
	if len(turns) == 0 {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, pgx.ErrNoRows
	}

	createdByUserID := sql.NullString{}
	if arg.CreatedByUserID.Valid {
		createdByUserID = sql.NullString{String: arg.CreatedByUserID.String(), Valid: true}
	}
	created, err := qtx.CreateSession(ctx, sqlitesqlc.CreateSessionParams{
		BotID:           source.BotID,
		ChannelType:     source.ChannelType,
		Type:            source.Type,
		SessionMode:     source.SessionMode,
		RuntimeType:     source.RuntimeType,
		RuntimeMetadata: source.RuntimeMetadata,
		Title:           arg.Title,
		Metadata:        string(arg.Metadata),
		CreatedByUserID: createdByUserID,
	})
	if err != nil {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, mapQueryErr(err)
	}

	messageIDMap := make(map[string]string)
	visibleMessages, err := sqliteForkVisibleMessages(ctx, tx, sessionID, forkTarget)
	if err != nil {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, mapQueryErr(err)
	}
	for _, sourceMessage := range visibleMessages {
		if _, err := sqliteForkCopyMessage(ctx, tx, qtx, sourceMessage.ID, created.ID, messageIDMap); err != nil {
			return pgsqlc.ForkSessionFromAssistantMessageRow{}, err
		}
	}
	forkMessageID, err := sqliteForkAnchorMessageID(ctx, tx, sessionID, forkTarget, messageIDMap)
	if err != nil {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, mapQueryErr(err)
	}
	if forkMessageID == "" {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, pgx.ErrNoRows
	}
	patchedMetadata, err := sqliteForkMetadataWithAnchor(created.Metadata, forkMessageID)
	if err != nil {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, err
	}
	created, err = qtx.UpdateSessionMetadata(ctx, sqlitesqlc.UpdateSessionMetadataParams{
		ID:       created.ID,
		Metadata: patchedMetadata,
	})
	if err != nil {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, mapQueryErr(err)
	}
	turnIDByPosition := make(map[int64]string, len(turns))
	for _, turn := range turns {
		var requestID sql.NullString
		if turn.RequestMessageID.Valid {
			requestID = sql.NullString{String: messageIDMap[turn.RequestMessageID.String], Valid: true}
		}
		var assistantID sql.NullString
		if turn.AssistantMessageID.Valid {
			assistantID = sql.NullString{String: messageIDMap[turn.AssistantMessageID.String], Valid: true}
		}
		createdTurn, err := qtx.CreateHistoryTurn(ctx, sqlitesqlc.CreateHistoryTurnParams{
			BotID:              created.BotID,
			SessionID:          created.ID,
			RequestMessageID:   requestID,
			AssistantMessageID: assistantID,
		})
		if err != nil {
			return pgsqlc.ForkSessionFromAssistantMessageRow{}, mapQueryErr(err)
		}
		turnIDByPosition[turn.Position] = createdTurn.ID
	}
	for _, sourceMessage := range visibleMessages {
		copiedID := messageIDMap[sourceMessage.ID]
		turnID := turnIDByPosition[sourceMessage.TurnPosition]
		if copiedID == "" || turnID == "" {
			return pgsqlc.ForkSessionFromAssistantMessageRow{}, pgx.ErrNoRows
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE bot_history_messages
SET turn_id = ?, turn_message_seq = ?
WHERE id = ?
`, turnID, sourceMessage.TurnMessageSeq, copiedID); err != nil {
			return pgsqlc.ForkSessionFromAssistantMessageRow{}, mapQueryErr(err)
		}
	}

	if err := tx.Commit(); err != nil {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, err
	}
	var result pgsqlc.ForkSessionFromAssistantMessageRow
	if err := convertValue(created, &result); err != nil {
		return pgsqlc.ForkSessionFromAssistantMessageRow{}, err
	}
	return result, nil
}

func sqliteForkMetadataWithAnchor(metadata, forkMessageID string) (string, error) {
	var meta map[string]any
	if strings.TrimSpace(metadata) != "" {
		if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
			return "", err
		}
	}
	if meta == nil {
		meta = map[string]any{}
	}
	forkedFrom, _ := meta["forked_from"].(map[string]any)
	if forkedFrom == nil {
		forkedFrom = map[string]any{}
	}
	forkedFrom["fork_message_id"] = strings.TrimSpace(forkMessageID)
	meta["forked_from"] = forkedFrom
	out, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (q *Queries) CreateSessionEvent(ctx context.Context, arg pgsqlc.CreateSessionEventParams) (pgtype.UUID, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgtype.UUID{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateSessionEventParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgtype.UUID{}, err
	}
	out, err := q.store.queries.CreateSessionEvent(ctx, sqliteArg)
	if err != nil {
		return pgtype.UUID{}, mapQueryErr(err)
	}
	var result pgtype.UUID
	if err := convertValue(out, &result); err != nil {
		return pgtype.UUID{}, err
	}
	return result, nil
}

type sqliteForkTurn struct {
	ID                 string
	Position           int64
	RequestMessageID   sql.NullString
	AssistantMessageID sql.NullString
}

type sqliteForkTarget struct {
	Position  int64
	MessageID string
}

type sqliteForkVisibleMessage struct {
	ID             string
	TurnPosition   int64
	TurnMessageSeq int64
}

func sqliteForkVisibleTurns(ctx context.Context, tx *sql.Tx, sessionID string, target sqliteForkTarget) ([]sqliteForkTurn, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, position, request_message_id, assistant_message_id
FROM bot_history_turns
WHERE session_id = ?
  AND superseded_at IS NULL
  AND position <= ?
ORDER BY position ASC
`, sessionID, target.Position)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var turns []sqliteForkTurn
	for rows.Next() {
		var turn sqliteForkTurn
		if err := rows.Scan(&turn.ID, &turn.Position, &turn.RequestMessageID, &turn.AssistantMessageID); err != nil {
			return nil, err
		}
		turns = append(turns, turn)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return turns, nil
}

func loadSQLiteForkTarget(ctx context.Context, tx *sql.Tx, sessionID, messageID string) (sqliteForkTarget, error) {
	row := tx.QueryRowContext(ctx, `
SELECT vm.turn_position, vm.id
FROM bot_visible_history_messages vm
JOIN bot_history_turns t ON t.id = vm.turn_id
WHERE vm.session_id = ?
  AND vm.id = ?
  AND vm.role = 'assistant'
  AND t.superseded_at IS NULL
LIMIT 1
`, sessionID, messageID)
	var target sqliteForkTarget
	if err := row.Scan(&target.Position, &target.MessageID); err != nil {
		return sqliteForkTarget{}, err
	}
	return target, nil
}

func sqliteForkVisibleMessages(ctx context.Context, tx *sql.Tx, sessionID string, target sqliteForkTarget) ([]sqliteForkVisibleMessage, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT vm.id, vm.turn_position, vm.turn_message_seq
FROM bot_visible_history_messages vm
WHERE vm.session_id = ?
  AND (
    vm.turn_position < ?
    OR vm.turn_position = ?
  )
ORDER BY vm.turn_position ASC, vm.turn_message_seq ASC, vm.created_at ASC, vm.id ASC
`, sessionID, target.Position, target.Position)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var messages []sqliteForkVisibleMessage
	for rows.Next() {
		var message sqliteForkVisibleMessage
		if err := rows.Scan(&message.ID, &message.TurnPosition, &message.TurnMessageSeq); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func sqliteForkAnchorMessageID(ctx context.Context, tx *sql.Tx, sessionID string, target sqliteForkTarget, messageIDMap map[string]string) (string, error) {
	row := tx.QueryRowContext(ctx, `
SELECT t.assistant_message_id
FROM bot_history_turns t
WHERE t.session_id = ?
  AND t.position = ?
  AND t.superseded_at IS NULL
LIMIT 1
`, sessionID, target.Position)
	var oldMessageID string
	if err := row.Scan(&oldMessageID); err != nil {
		return "", err
	}
	return messageIDMap[oldMessageID], nil
}

func sqliteForkCopyMessage(ctx context.Context, tx *sql.Tx, qtx *sqlitesqlc.Queries, oldMessageID, newSessionID string, messageIDMap map[string]string) (string, error) {
	if copiedID := messageIDMap[oldMessageID]; copiedID != "" {
		return copiedID, nil
	}
	source, err := sqliteForkGetMessage(ctx, tx, oldMessageID)
	if err != nil {
		return "", mapQueryErr(err)
	}
	createdID, err := sqliteForkInsertCopiedMessage(ctx, tx, source, newSessionID)
	if err != nil {
		return "", mapQueryErr(err)
	}
	assets, err := qtx.ListMessageAssets(ctx, oldMessageID)
	if err != nil {
		return "", mapQueryErr(err)
	}
	for _, asset := range assets {
		if _, err := qtx.CreateMessageAsset(ctx, sqlitesqlc.CreateMessageAssetParams{
			MessageID:   createdID,
			Role:        asset.Role,
			Ordinal:     asset.Ordinal,
			ContentHash: asset.ContentHash,
			Name:        asset.Name,
			Metadata:    asset.Metadata,
		}); err != nil {
			return "", mapQueryErr(err)
		}
	}
	messageIDMap[oldMessageID] = createdID
	return createdID, nil
}

type sqliteForkMessage struct {
	ID                      string
	BotID                   string
	SessionID               sql.NullString
	SenderChannelIdentityID sql.NullString
	SenderUserID            sql.NullString
	ExternalMessageID       sql.NullString
	SourceReplyToMessageID  sql.NullString
	Role                    string
	Content                 string
	Metadata                string
	Usage                   sql.NullString
	SessionMode             string
	RuntimeType             string
	ModelID                 sql.NullString
	EventID                 sql.NullString
	DisplayText             sql.NullString
	CreatedAt               string
}

func sqliteForkGetMessage(ctx context.Context, tx *sql.Tx, messageID string) (sqliteForkMessage, error) {
	row := tx.QueryRowContext(ctx, `
SELECT id, bot_id, session_id, sender_channel_identity_id, sender_account_user_id,
  source_message_id, source_reply_to_message_id, role, content, metadata,
  usage, session_mode, runtime_type, model_id, event_id, display_text, created_at
FROM bot_history_messages
WHERE id = ?
LIMIT 1
`, messageID)
	var msg sqliteForkMessage
	err := row.Scan(
		&msg.ID,
		&msg.BotID,
		&msg.SessionID,
		&msg.SenderChannelIdentityID,
		&msg.SenderUserID,
		&msg.ExternalMessageID,
		&msg.SourceReplyToMessageID,
		&msg.Role,
		&msg.Content,
		&msg.Metadata,
		&msg.Usage,
		&msg.SessionMode,
		&msg.RuntimeType,
		&msg.ModelID,
		&msg.EventID,
		&msg.DisplayText,
		&msg.CreatedAt,
	)
	return msg, err
}

func sqliteForkInsertCopiedMessage(ctx context.Context, tx *sql.Tx, source sqliteForkMessage, newSessionID string) (string, error) {
	newID, err := sqliteGenerateUUID(ctx, tx)
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO bot_history_messages (
  id,
  bot_id,
  session_id,
  sender_channel_identity_id,
  sender_account_user_id,
  source_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  session_mode,
  runtime_type,
  model_id,
  event_id,
  display_text,
  created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
		newID,
		source.BotID,
		sql.NullString{String: newSessionID, Valid: true},
		source.SenderChannelIdentityID,
		source.SenderUserID,
		source.ExternalMessageID,
		source.SourceReplyToMessageID,
		source.Role,
		source.Content,
		source.Metadata,
		source.Usage,
		source.SessionMode,
		source.RuntimeType,
		source.ModelID,
		sql.NullString{},
		source.DisplayText,
		source.CreatedAt,
	)
	if err != nil {
		return "", err
	}
	return newID, nil
}

func sqliteGenerateUUID(ctx context.Context, tx *sql.Tx) (string, error) {
	row := tx.QueryRowContext(ctx, `
SELECT lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6)))
`)
	var id string
	if err := row.Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

func (q *Queries) CreateStorageProvider(ctx context.Context, arg pgsqlc.CreateStorageProviderParams) (pgsqlc.StorageProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.StorageProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateStorageProviderParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.StorageProvider{}, err
	}
	out, err := q.store.queries.CreateStorageProvider(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.StorageProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.StorageProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.StorageProvider{}, err
	}
	return result, nil
}

func (q *Queries) CreateToolApprovalRequest(ctx context.Context, arg pgsqlc.CreateToolApprovalRequestParams) (pgsqlc.ToolApprovalRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ToolApprovalRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateToolApprovalRequestParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	out, err := q.store.queries.CreateToolApprovalRequest(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ToolApprovalRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.ToolApprovalRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	return result, nil
}

func (q *Queries) CreateUser(ctx context.Context, arg pgsqlc.CreateUserParams) (pgsqlc.User, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.User{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateUserParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.User{}, err
	}
	out, err := q.store.queries.CreateUser(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.User{}, mapQueryErr(err)
	}
	var result pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.User{}, err
	}
	return result, nil
}

func (q *Queries) DeleteBotACLRuleByID(ctx context.Context, id pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return err
	}
	err := q.store.queries.DeleteBotACLRuleByID(ctx, sqliteId)
	return mapQueryErr(err)
}

func (q *Queries) DeleteBotByID(ctx context.Context, id pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return err
	}
	err := q.store.queries.DeleteBotByID(ctx, sqliteId)
	return mapQueryErr(err)
}

func (q *Queries) DeleteBotChannelConfig(ctx context.Context, arg pgsqlc.DeleteBotChannelConfigParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.DeleteBotChannelConfigParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.DeleteBotChannelConfig(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) DeleteBotEmailBinding(ctx context.Context, id pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return err
	}
	err := q.store.queries.DeleteBotEmailBinding(ctx, sqliteId)
	return mapQueryErr(err)
}

func (q *Queries) DeleteChat(ctx context.Context, chatID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteChatID string
	if err := convertValue(chatID, &sqliteChatID); err != nil {
		return err
	}
	err := q.store.queries.DeleteChat(ctx, sqliteChatID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteChatRoute(ctx context.Context, id pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return err
	}
	err := q.store.queries.DeleteChatRoute(ctx, sqliteId)
	return mapQueryErr(err)
}

func (q *Queries) DeleteCompactionLogsByBot(ctx context.Context, botID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return err
	}
	err := q.store.queries.DeleteCompactionLogsByBot(ctx, sqliteBotID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteContainerByBotID(ctx context.Context, botID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return err
	}
	err := q.store.queries.DeleteContainerByBotID(ctx, sqliteBotID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteEmailOAuthToken(ctx context.Context, emailProviderID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteEmailProviderID string
	if err := convertValue(emailProviderID, &sqliteEmailProviderID); err != nil {
		return err
	}
	err := q.store.queries.DeleteEmailOAuthToken(ctx, sqliteEmailProviderID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteEmailProvider(ctx context.Context, id pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return err
	}
	err := q.store.queries.DeleteEmailProvider(ctx, sqliteId)
	return mapQueryErr(err)
}

func (q *Queries) DeleteEmailProviderByIDAndUser(ctx context.Context, arg pgsqlc.DeleteEmailProviderByIDAndUserParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.DeleteEmailProviderByIDAndUserParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.DeleteEmailProviderByIDAndUser(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) DeleteHeartbeatLogsByBot(ctx context.Context, botID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return err
	}
	err := q.store.queries.DeleteHeartbeatLogsByBot(ctx, sqliteBotID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteMCPConnection(ctx context.Context, arg pgsqlc.DeleteMCPConnectionParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.DeleteMCPConnectionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.DeleteMCPConnection(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) DeleteMCPOAuthToken(ctx context.Context, connectionID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteConnectionID string
	if err := convertValue(connectionID, &sqliteConnectionID); err != nil {
		return err
	}
	err := q.store.queries.DeleteMCPOAuthToken(ctx, sqliteConnectionID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteMemoryProvider(ctx context.Context, id pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return err
	}
	err := q.store.queries.DeleteMemoryProvider(ctx, sqliteId)
	return mapQueryErr(err)
}

func (q *Queries) DeleteMessageAssets(ctx context.Context, messageID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteMessageID string
	if err := convertValue(messageID, &sqliteMessageID); err != nil {
		return err
	}
	err := q.store.queries.DeleteMessageAssets(ctx, sqliteMessageID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteMessagesByBot(ctx context.Context, botID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return err
	}
	err := q.store.queries.DeleteMessagesByBot(ctx, sqliteBotID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteMessagesByIDs(ctx context.Context, ids []pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteIDs []string
	if err := convertValue(ids, &sqliteIDs); err != nil {
		return err
	}
	err := q.store.queries.DeleteMessagesByIDs(ctx, sqliteIDs)
	return mapQueryErr(err)
}

func (q *Queries) DeleteMessagesBySession(ctx context.Context, sessionID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID sql.NullString
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return err
	}
	err := q.store.queries.DeleteMessagesBySession(ctx, sqliteSessionID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteModel(ctx context.Context, id pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return err
	}
	err := q.store.queries.DeleteModel(ctx, sqliteId)
	return mapQueryErr(err)
}

func (q *Queries) DeleteModelByModelID(ctx context.Context, modelID string) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteModelID string
	if err := convertValue(modelID, &sqliteModelID); err != nil {
		return err
	}
	err := q.store.queries.DeleteModelByModelID(ctx, sqliteModelID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteModelByProviderAndType(ctx context.Context, arg pgsqlc.DeleteModelByProviderAndTypeParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.DeleteModelByProviderAndTypeParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.DeleteModelByProviderAndType(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) DeleteModelByProviderIDAndModelID(ctx context.Context, arg pgsqlc.DeleteModelByProviderIDAndModelIDParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.DeleteModelByProviderIDAndModelIDParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.DeleteModelByProviderIDAndModelID(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) DeleteProvider(ctx context.Context, id pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return err
	}
	err := q.store.queries.DeleteProvider(ctx, sqliteId)
	return mapQueryErr(err)
}

func (q *Queries) DeleteProviderOAuthToken(ctx context.Context, providerID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteProviderID string
	if err := convertValue(providerID, &sqliteProviderID); err != nil {
		return err
	}
	err := q.store.queries.DeleteProviderOAuthToken(ctx, sqliteProviderID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteSchedule(ctx context.Context, id pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return err
	}
	err := q.store.queries.DeleteSchedule(ctx, sqliteId)
	return mapQueryErr(err)
}

func (q *Queries) DeleteScheduleLogsByBot(ctx context.Context, botID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return err
	}
	err := q.store.queries.DeleteScheduleLogsByBot(ctx, sqliteBotID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteScheduleLogsBySchedule(ctx context.Context, scheduleID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteScheduleID string
	if err := convertValue(scheduleID, &sqliteScheduleID); err != nil {
		return err
	}
	err := q.store.queries.DeleteScheduleLogsBySchedule(ctx, sqliteScheduleID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteFetchProvider(ctx context.Context, id pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return err
	}
	err := q.store.queries.DeleteFetchProvider(ctx, sqliteId)
	return mapQueryErr(err)
}

func (q *Queries) DeleteSearchProvider(ctx context.Context, id pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return err
	}
	err := q.store.queries.DeleteSearchProvider(ctx, sqliteId)
	return mapQueryErr(err)
}

func (q *Queries) DeleteSettingsByBotID(ctx context.Context, id pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return err
	}
	err := q.store.queries.DeleteSettingsByBotID(ctx, sqliteId)
	return mapQueryErr(err)
}

func (q *Queries) DeleteUserProviderOAuthToken(ctx context.Context, arg pgsqlc.DeleteUserProviderOAuthTokenParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.DeleteUserProviderOAuthTokenParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.DeleteUserProviderOAuthToken(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) EvaluateBotACLRule(ctx context.Context, arg pgsqlc.EvaluateBotACLRuleParams) (string, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return "", errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.EvaluateBotACLRuleParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return "", err
	}
	out, err := q.store.queries.EvaluateBotACLRule(ctx, sqliteArg)
	if err != nil {
		return "", mapQueryErr(err)
	}
	var result string
	if err := convertValue(out, &result); err != nil {
		return "", err
	}
	return result, nil
}

func (q *Queries) FindChatRoute(ctx context.Context, arg pgsqlc.FindChatRouteParams) (pgsqlc.FindChatRouteRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.FindChatRouteRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.FindChatRouteParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.FindChatRouteRow{}, err
	}
	out, err := q.store.queries.FindChatRoute(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.FindChatRouteRow{}, mapQueryErr(err)
	}
	var result pgsqlc.FindChatRouteRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.FindChatRouteRow{}, err
	}
	return result, nil
}

func (q *Queries) GetAccountByIdentity(ctx context.Context, identity pgtype.Text) (pgsqlc.User, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.User{}, errSQLiteQueriesNotConfigured
	}
	var sqliteIdentity sql.NullString
	if err := convertValue(identity, &sqliteIdentity); err != nil {
		return pgsqlc.User{}, err
	}
	out, err := q.store.queries.GetAccountByIdentity(ctx, sqliteIdentity)
	if err != nil {
		return pgsqlc.User{}, mapQueryErr(err)
	}
	var result pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.User{}, err
	}
	return result, nil
}

func (q *Queries) GetAccountByUserID(ctx context.Context, userID pgtype.UUID) (pgsqlc.User, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.User{}, errSQLiteQueriesNotConfigured
	}
	var sqliteUserID string
	if err := convertValue(userID, &sqliteUserID); err != nil {
		return pgsqlc.User{}, err
	}
	out, err := q.store.queries.GetAccountByUserID(ctx, sqliteUserID)
	if err != nil {
		return pgsqlc.User{}, mapQueryErr(err)
	}
	var result pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.User{}, err
	}
	return result, nil
}

func (q *Queries) GetActiveSessionForRoute(ctx context.Context, routeID pgtype.UUID) (pgsqlc.BotSession, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotSession{}, errSQLiteQueriesNotConfigured
	}
	var sqliteRouteID string
	if err := convertValue(routeID, &sqliteRouteID); err != nil {
		return pgsqlc.BotSession{}, err
	}
	out, err := q.store.queries.GetActiveSessionForRoute(ctx, sqliteRouteID)
	if err != nil {
		return pgsqlc.BotSession{}, mapQueryErr(err)
	}
	var result pgsqlc.BotSession
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotSession{}, err
	}
	return result, nil
}

func (q *Queries) GetBotACLDefaultEffect(ctx context.Context, id pgtype.UUID) (string, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return "", errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return "", err
	}
	out, err := q.store.queries.GetBotACLDefaultEffect(ctx, sqliteId)
	if err != nil {
		return "", mapQueryErr(err)
	}
	var result string
	if err := convertValue(out, &result); err != nil {
		return "", err
	}
	return result, nil
}

func (q *Queries) GetBotByID(ctx context.Context, id pgtype.UUID) (pgsqlc.GetBotByIDRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetBotByIDRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.GetBotByIDRow{}, err
	}
	out, err := q.store.queries.GetBotByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.GetBotByIDRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetBotByIDRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetBotByIDRow{}, err
	}
	return result, nil
}

func (q *Queries) GetBotByName(ctx context.Context, name string) (pgsqlc.GetBotByNameRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetBotByNameRow{}, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.GetBotByName(ctx, name)
	if err != nil {
		return pgsqlc.GetBotByNameRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetBotByNameRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetBotByNameRow{}, err
	}
	return result, nil
}

func (q *Queries) GetBotChannelConfig(ctx context.Context, arg pgsqlc.GetBotChannelConfigParams) (pgsqlc.BotChannelConfig, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotChannelConfig{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetBotChannelConfigParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotChannelConfig{}, err
	}
	out, err := q.store.queries.GetBotChannelConfig(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotChannelConfig{}, mapQueryErr(err)
	}
	var result pgsqlc.BotChannelConfig
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotChannelConfig{}, err
	}
	return result, nil
}

func (q *Queries) GetBotChannelConfigByExternalIdentity(ctx context.Context, arg pgsqlc.GetBotChannelConfigByExternalIdentityParams) (pgsqlc.BotChannelConfig, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotChannelConfig{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetBotChannelConfigByExternalIdentityParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotChannelConfig{}, err
	}
	out, err := q.store.queries.GetBotChannelConfigByExternalIdentity(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotChannelConfig{}, mapQueryErr(err)
	}
	var result pgsqlc.BotChannelConfig
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotChannelConfig{}, err
	}
	return result, nil
}

func (q *Queries) GetBotEmailBindingByBotAndProvider(ctx context.Context, arg pgsqlc.GetBotEmailBindingByBotAndProviderParams) (pgsqlc.BotEmailBinding, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotEmailBinding{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetBotEmailBindingByBotAndProviderParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotEmailBinding{}, err
	}
	out, err := q.store.queries.GetBotEmailBindingByBotAndProvider(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotEmailBinding{}, mapQueryErr(err)
	}
	var result pgsqlc.BotEmailBinding
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotEmailBinding{}, err
	}
	return result, nil
}

func (q *Queries) GetBotEmailBindingByID(ctx context.Context, id pgtype.UUID) (pgsqlc.BotEmailBinding, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotEmailBinding{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.BotEmailBinding{}, err
	}
	out, err := q.store.queries.GetBotEmailBindingByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.BotEmailBinding{}, mapQueryErr(err)
	}
	var result pgsqlc.BotEmailBinding
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotEmailBinding{}, err
	}
	return result, nil
}

func (q *Queries) GetBotOverlayConfig(ctx context.Context, id pgtype.UUID) (pgsqlc.GetBotOverlayConfigRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetBotOverlayConfigRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteID string
	if err := convertValue(id, &sqliteID); err != nil {
		return pgsqlc.GetBotOverlayConfigRow{}, err
	}
	out, err := q.store.queries.GetBotOverlayConfig(ctx, sqliteID)
	if err != nil {
		return pgsqlc.GetBotOverlayConfigRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetBotOverlayConfigRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetBotOverlayConfigRow{}, err
	}
	return result, nil
}

func (q *Queries) GetBotStorageBinding(ctx context.Context, botID pgtype.UUID) (pgsqlc.BotStorageBinding, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotStorageBinding{}, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return pgsqlc.BotStorageBinding{}, err
	}
	out, err := q.store.queries.GetBotStorageBinding(ctx, sqliteBotID)
	if err != nil {
		return pgsqlc.BotStorageBinding{}, mapQueryErr(err)
	}
	var result pgsqlc.BotStorageBinding
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotStorageBinding{}, err
	}
	return result, nil
}

func (q *Queries) GetChannelIdentityByChannelSubject(ctx context.Context, arg pgsqlc.GetChannelIdentityByChannelSubjectParams) (pgsqlc.ChannelIdentity, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ChannelIdentity{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetChannelIdentityByChannelSubjectParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ChannelIdentity{}, err
	}
	out, err := q.store.queries.GetChannelIdentityByChannelSubject(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ChannelIdentity{}, mapQueryErr(err)
	}
	var result pgsqlc.ChannelIdentity
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ChannelIdentity{}, err
	}
	return result, nil
}

func (q *Queries) GetChannelIdentityByID(ctx context.Context, id pgtype.UUID) (pgsqlc.ChannelIdentity, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ChannelIdentity{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.ChannelIdentity{}, err
	}
	out, err := q.store.queries.GetChannelIdentityByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.ChannelIdentity{}, mapQueryErr(err)
	}
	var result pgsqlc.ChannelIdentity
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ChannelIdentity{}, err
	}
	return result, nil
}

func (q *Queries) GetChannelIdentityByIDForUpdate(ctx context.Context, id pgtype.UUID) (pgsqlc.ChannelIdentity, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ChannelIdentity{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.ChannelIdentity{}, err
	}
	out, err := q.store.queries.GetChannelIdentityByIDForUpdate(ctx, sqliteId)
	if err != nil {
		return pgsqlc.ChannelIdentity{}, mapQueryErr(err)
	}
	var result pgsqlc.ChannelIdentity
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ChannelIdentity{}, err
	}
	return result, nil
}

func (q *Queries) GetChatByID(ctx context.Context, id pgtype.UUID) (pgsqlc.GetChatByIDRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetChatByIDRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.GetChatByIDRow{}, err
	}
	out, err := q.store.queries.GetChatByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.GetChatByIDRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetChatByIDRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetChatByIDRow{}, err
	}
	return result, nil
}

func (q *Queries) GetChatParticipant(ctx context.Context, arg pgsqlc.GetChatParticipantParams) (pgsqlc.GetChatParticipantRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetChatParticipantRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetChatParticipantParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.GetChatParticipantRow{}, err
	}
	out, err := q.store.queries.GetChatParticipant(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.GetChatParticipantRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetChatParticipantRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetChatParticipantRow{}, err
	}
	return result, nil
}

func (q *Queries) GetChatReadAccessByUser(ctx context.Context, arg pgsqlc.GetChatReadAccessByUserParams) (pgsqlc.GetChatReadAccessByUserRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetChatReadAccessByUserRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetChatReadAccessByUserParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.GetChatReadAccessByUserRow{}, err
	}
	out, err := q.store.queries.GetChatReadAccessByUser(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.GetChatReadAccessByUserRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetChatReadAccessByUserRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetChatReadAccessByUserRow{}, err
	}
	return result, nil
}

func (q *Queries) GetChatRouteByID(ctx context.Context, id pgtype.UUID) (pgsqlc.GetChatRouteByIDRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetChatRouteByIDRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.GetChatRouteByIDRow{}, err
	}
	out, err := q.store.queries.GetChatRouteByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.GetChatRouteByIDRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetChatRouteByIDRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetChatRouteByIDRow{}, err
	}
	return result, nil
}

func (q *Queries) GetChatSettings(ctx context.Context, id pgtype.UUID) (pgsqlc.GetChatSettingsRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetChatSettingsRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.GetChatSettingsRow{}, err
	}
	out, err := q.store.queries.GetChatSettings(ctx, sqliteId)
	if err != nil {
		return pgsqlc.GetChatSettingsRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetChatSettingsRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetChatSettingsRow{}, err
	}
	return result, nil
}

func (q *Queries) GetCompactionLogByID(ctx context.Context, id pgtype.UUID) (pgsqlc.BotHistoryMessageCompact, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotHistoryMessageCompact{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.BotHistoryMessageCompact{}, err
	}
	out, err := q.store.queries.GetCompactionLogByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.BotHistoryMessageCompact{}, mapQueryErr(err)
	}
	var result pgsqlc.BotHistoryMessageCompact
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotHistoryMessageCompact{}, err
	}
	return result, nil
}

func (q *Queries) GetContainerByBotID(ctx context.Context, botID pgtype.UUID) (pgsqlc.Container, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Container{}, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return pgsqlc.Container{}, err
	}
	out, err := q.store.queries.GetContainerByBotID(ctx, sqliteBotID)
	if err != nil {
		return pgsqlc.Container{}, mapQueryErr(err)
	}
	var result pgsqlc.Container
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Container{}, err
	}
	return result, nil
}

func (q *Queries) GetBotWorkspaceResourceLimits(ctx context.Context, botID pgtype.UUID) (pgsqlc.BotWorkspaceResourceLimit, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotWorkspaceResourceLimit{}, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return pgsqlc.BotWorkspaceResourceLimit{}, err
	}
	out, err := q.store.queries.GetBotWorkspaceResourceLimits(ctx, sqliteBotID)
	if err != nil {
		return pgsqlc.BotWorkspaceResourceLimit{}, mapQueryErr(err)
	}
	var result pgsqlc.BotWorkspaceResourceLimit
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotWorkspaceResourceLimit{}, err
	}
	return result, nil
}

func (q *Queries) GetDefaultMemoryProvider(ctx context.Context) (pgsqlc.MemoryProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.MemoryProvider{}, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.GetDefaultMemoryProvider(ctx)
	if err != nil {
		return pgsqlc.MemoryProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.MemoryProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.MemoryProvider{}, err
	}
	return result, nil
}

func (q *Queries) GetEmailOAuthTokenByProvider(ctx context.Context, emailProviderID pgtype.UUID) (pgsqlc.EmailOauthToken, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.EmailOauthToken{}, errSQLiteQueriesNotConfigured
	}
	var sqliteEmailProviderID string
	if err := convertValue(emailProviderID, &sqliteEmailProviderID); err != nil {
		return pgsqlc.EmailOauthToken{}, err
	}
	out, err := q.store.queries.GetEmailOAuthTokenByProvider(ctx, sqliteEmailProviderID)
	if err != nil {
		return pgsqlc.EmailOauthToken{}, mapQueryErr(err)
	}
	var result pgsqlc.EmailOauthToken
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.EmailOauthToken{}, err
	}
	return result, nil
}

func (q *Queries) GetEmailOAuthTokenByState(ctx context.Context, state string) (pgsqlc.EmailOauthToken, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.EmailOauthToken{}, errSQLiteQueriesNotConfigured
	}
	var sqliteState string
	if err := convertValue(state, &sqliteState); err != nil {
		return pgsqlc.EmailOauthToken{}, err
	}
	out, err := q.store.queries.GetEmailOAuthTokenByState(ctx, sqliteState)
	if err != nil {
		return pgsqlc.EmailOauthToken{}, mapQueryErr(err)
	}
	var result pgsqlc.EmailOauthToken
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.EmailOauthToken{}, err
	}
	return result, nil
}

func (q *Queries) GetEmailOutboxByID(ctx context.Context, id pgtype.UUID) (pgsqlc.EmailOutbox, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.EmailOutbox{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.EmailOutbox{}, err
	}
	out, err := q.store.queries.GetEmailOutboxByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.EmailOutbox{}, mapQueryErr(err)
	}
	var result pgsqlc.EmailOutbox
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.EmailOutbox{}, err
	}
	return result, nil
}

func (q *Queries) GetEmailProviderByID(ctx context.Context, id pgtype.UUID) (pgsqlc.EmailProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.EmailProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.EmailProvider{}, err
	}
	out, err := q.store.queries.GetEmailProviderByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.EmailProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.EmailProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.EmailProvider{}, err
	}
	return result, nil
}

func (q *Queries) GetEmailProviderByIDAndUser(ctx context.Context, arg pgsqlc.GetEmailProviderByIDAndUserParams) (pgsqlc.EmailProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.EmailProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetEmailProviderByIDAndUserParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.EmailProvider{}, err
	}
	out, err := q.store.queries.GetEmailProviderByIDAndUser(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.EmailProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.EmailProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.EmailProvider{}, err
	}
	return result, nil
}

func (q *Queries) GetEmailProviderByNameAndUser(ctx context.Context, arg pgsqlc.GetEmailProviderByNameAndUserParams) (pgsqlc.EmailProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.EmailProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetEmailProviderByNameAndUserParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.EmailProvider{}, err
	}
	out, err := q.store.queries.GetEmailProviderByNameAndUser(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.EmailProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.EmailProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.EmailProvider{}, err
	}
	return result, nil
}

func (q *Queries) GetLatestAssistantUsage(ctx context.Context, sessionID pgtype.UUID) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID sql.NullString
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return 0, err
	}
	out, err := q.store.queries.GetLatestAssistantUsage(ctx, sqliteSessionID)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) GetLatestPendingToolApprovalBySession(ctx context.Context, arg pgsqlc.GetLatestPendingToolApprovalBySessionParams) (pgsqlc.ToolApprovalRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ToolApprovalRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetLatestPendingToolApprovalBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	out, err := q.store.queries.GetLatestPendingToolApprovalBySession(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ToolApprovalRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.ToolApprovalRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	return result, nil
}

func (q *Queries) GetLatestSessionIDByBot(ctx context.Context, botID pgtype.UUID) (pgtype.UUID, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgtype.UUID{}, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return pgtype.UUID{}, err
	}
	out, err := q.store.queries.GetLatestSessionIDByBot(ctx, sqliteBotID)
	if err != nil {
		return pgtype.UUID{}, mapQueryErr(err)
	}
	var result pgtype.UUID
	if err := convertValue(out, &result); err != nil {
		return pgtype.UUID{}, err
	}
	return result, nil
}

func (q *Queries) GetMCPConnectionByID(ctx context.Context, arg pgsqlc.GetMCPConnectionByIDParams) (pgsqlc.McpConnection, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.McpConnection{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetMCPConnectionByIDParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.McpConnection{}, err
	}
	out, err := q.store.queries.GetMCPConnectionByID(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.McpConnection{}, mapQueryErr(err)
	}
	var result pgsqlc.McpConnection
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.McpConnection{}, err
	}
	return result, nil
}

func (q *Queries) GetMCPOAuthToken(ctx context.Context, connectionID pgtype.UUID) (pgsqlc.McpOauthToken, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.McpOauthToken{}, errSQLiteQueriesNotConfigured
	}
	var sqliteConnectionID string
	if err := convertValue(connectionID, &sqliteConnectionID); err != nil {
		return pgsqlc.McpOauthToken{}, err
	}
	out, err := q.store.queries.GetMCPOAuthToken(ctx, sqliteConnectionID)
	if err != nil {
		return pgsqlc.McpOauthToken{}, mapQueryErr(err)
	}
	var result pgsqlc.McpOauthToken
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.McpOauthToken{}, err
	}
	return result, nil
}

func (q *Queries) GetMCPOAuthTokenByState(ctx context.Context, stateParam string) (pgsqlc.McpOauthToken, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.McpOauthToken{}, errSQLiteQueriesNotConfigured
	}
	var sqliteStateParam string
	if err := convertValue(stateParam, &sqliteStateParam); err != nil {
		return pgsqlc.McpOauthToken{}, err
	}
	out, err := q.store.queries.GetMCPOAuthTokenByState(ctx, sqliteStateParam)
	if err != nil {
		return pgsqlc.McpOauthToken{}, mapQueryErr(err)
	}
	var result pgsqlc.McpOauthToken
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.McpOauthToken{}, err
	}
	return result, nil
}

func (q *Queries) GetMemoryProviderByID(ctx context.Context, id pgtype.UUID) (pgsqlc.MemoryProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.MemoryProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.MemoryProvider{}, err
	}
	out, err := q.store.queries.GetMemoryProviderByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.MemoryProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.MemoryProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.MemoryProvider{}, err
	}
	return result, nil
}

func (q *Queries) GetModelByID(ctx context.Context, id pgtype.UUID) (pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Model{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.Model{}, err
	}
	out, err := q.store.queries.GetModelByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.Model{}, mapQueryErr(err)
	}
	var result pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Model{}, err
	}
	return result, nil
}

func (q *Queries) GetModelByModelID(ctx context.Context, modelID string) (pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Model{}, errSQLiteQueriesNotConfigured
	}
	var sqliteModelID string
	if err := convertValue(modelID, &sqliteModelID); err != nil {
		return pgsqlc.Model{}, err
	}
	out, err := q.store.queries.GetModelByModelID(ctx, sqliteModelID)
	if err != nil {
		return pgsqlc.Model{}, mapQueryErr(err)
	}
	var result pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Model{}, err
	}
	return result, nil
}

func (q *Queries) GetModelByProviderAndModelID(ctx context.Context, arg pgsqlc.GetModelByProviderAndModelIDParams) (pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Model{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetModelByProviderAndModelIDParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.Model{}, err
	}
	out, err := q.store.queries.GetModelByProviderAndModelID(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.Model{}, mapQueryErr(err)
	}
	var result pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Model{}, err
	}
	return result, nil
}

func (q *Queries) GetPendingToolApprovalByReplyMessage(ctx context.Context, arg pgsqlc.GetPendingToolApprovalByReplyMessageParams) (pgsqlc.ToolApprovalRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ToolApprovalRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetPendingToolApprovalByReplyMessageParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	out, err := q.store.queries.GetPendingToolApprovalByReplyMessage(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ToolApprovalRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.ToolApprovalRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	return result, nil
}

func (q *Queries) GetPendingToolApprovalBySessionShortID(ctx context.Context, arg pgsqlc.GetPendingToolApprovalBySessionShortIDParams) (pgsqlc.ToolApprovalRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ToolApprovalRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetPendingToolApprovalBySessionShortIDParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	out, err := q.store.queries.GetPendingToolApprovalBySessionShortID(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ToolApprovalRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.ToolApprovalRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	return result, nil
}

func (q *Queries) GetProviderByClientType(ctx context.Context, clientType string) (pgsqlc.Provider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Provider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteClientType string
	if err := convertValue(clientType, &sqliteClientType); err != nil {
		return pgsqlc.Provider{}, err
	}
	out, err := q.store.queries.GetProviderByClientType(ctx, sqliteClientType)
	if err != nil {
		return pgsqlc.Provider{}, mapQueryErr(err)
	}
	var result pgsqlc.Provider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Provider{}, err
	}
	return result, nil
}

func (q *Queries) GetProviderByID(ctx context.Context, id pgtype.UUID) (pgsqlc.Provider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Provider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.Provider{}, err
	}
	out, err := q.store.queries.GetProviderByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.Provider{}, mapQueryErr(err)
	}
	var result pgsqlc.Provider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Provider{}, err
	}
	return result, nil
}

func (q *Queries) GetProviderByName(ctx context.Context, name string) (pgsqlc.Provider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Provider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteName string
	if err := convertValue(name, &sqliteName); err != nil {
		return pgsqlc.Provider{}, err
	}
	out, err := q.store.queries.GetProviderByName(ctx, sqliteName)
	if err != nil {
		return pgsqlc.Provider{}, mapQueryErr(err)
	}
	var result pgsqlc.Provider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Provider{}, err
	}
	return result, nil
}

func (q *Queries) GetProviderOAuthTokenByProvider(ctx context.Context, providerID pgtype.UUID) (pgsqlc.ProviderOauthToken, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ProviderOauthToken{}, errSQLiteQueriesNotConfigured
	}
	var sqliteProviderID string
	if err := convertValue(providerID, &sqliteProviderID); err != nil {
		return pgsqlc.ProviderOauthToken{}, err
	}
	out, err := q.store.queries.GetProviderOAuthTokenByProvider(ctx, sqliteProviderID)
	if err != nil {
		return pgsqlc.ProviderOauthToken{}, mapQueryErr(err)
	}
	var result pgsqlc.ProviderOauthToken
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ProviderOauthToken{}, err
	}
	return result, nil
}

func (q *Queries) GetProviderOAuthTokenByState(ctx context.Context, state string) (pgsqlc.ProviderOauthToken, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ProviderOauthToken{}, errSQLiteQueriesNotConfigured
	}
	var sqliteState string
	if err := convertValue(state, &sqliteState); err != nil {
		return pgsqlc.ProviderOauthToken{}, err
	}
	out, err := q.store.queries.GetProviderOAuthTokenByState(ctx, sqliteState)
	if err != nil {
		return pgsqlc.ProviderOauthToken{}, mapQueryErr(err)
	}
	var result pgsqlc.ProviderOauthToken
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ProviderOauthToken{}, err
	}
	return result, nil
}

func (q *Queries) GetScheduleByID(ctx context.Context, id pgtype.UUID) (pgsqlc.Schedule, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Schedule{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.Schedule{}, err
	}
	out, err := q.store.queries.GetScheduleByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.Schedule{}, mapQueryErr(err)
	}
	var result pgsqlc.Schedule
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Schedule{}, err
	}
	return result, nil
}

func (q *Queries) GetFetchProviderByID(ctx context.Context, id pgtype.UUID) (pgsqlc.FetchProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.FetchProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.FetchProvider{}, err
	}
	out, err := q.store.queries.GetFetchProviderByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.FetchProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.FetchProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.FetchProvider{}, err
	}
	return result, nil
}

func (q *Queries) GetFetchProviderByName(ctx context.Context, name string) (pgsqlc.FetchProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.FetchProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteName string
	if err := convertValue(name, &sqliteName); err != nil {
		return pgsqlc.FetchProvider{}, err
	}
	out, err := q.store.queries.GetFetchProviderByName(ctx, sqliteName)
	if err != nil {
		return pgsqlc.FetchProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.FetchProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.FetchProvider{}, err
	}
	return result, nil
}

func (q *Queries) GetSearchProviderByID(ctx context.Context, id pgtype.UUID) (pgsqlc.SearchProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.SearchProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.SearchProvider{}, err
	}
	out, err := q.store.queries.GetSearchProviderByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.SearchProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.SearchProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.SearchProvider{}, err
	}
	return result, nil
}

func (q *Queries) GetSearchProviderByName(ctx context.Context, name string) (pgsqlc.SearchProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.SearchProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteName string
	if err := convertValue(name, &sqliteName); err != nil {
		return pgsqlc.SearchProvider{}, err
	}
	out, err := q.store.queries.GetSearchProviderByName(ctx, sqliteName)
	if err != nil {
		return pgsqlc.SearchProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.SearchProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.SearchProvider{}, err
	}
	return result, nil
}

func (q *Queries) GetSessionByID(ctx context.Context, id pgtype.UUID) (pgsqlc.BotSession, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotSession{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.BotSession{}, err
	}
	out, err := q.store.queries.GetSessionByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.BotSession{}, mapQueryErr(err)
	}
	var result pgsqlc.BotSession
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotSession{}, err
	}
	return result, nil
}

func (q *Queries) GetSessionCacheStats(ctx context.Context, sessionID pgtype.UUID) (pgsqlc.GetSessionCacheStatsRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetSessionCacheStatsRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID sql.NullString
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return pgsqlc.GetSessionCacheStatsRow{}, err
	}
	out, err := q.store.queries.GetSessionCacheStats(ctx, sqliteSessionID)
	if err != nil {
		return pgsqlc.GetSessionCacheStatsRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetSessionCacheStatsRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetSessionCacheStatsRow{}, err
	}
	return result, nil
}

func (q *Queries) GetSessionUsedSkills(ctx context.Context, sessionID pgtype.UUID) ([]string, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID sql.NullString
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.GetSessionUsedSkills(ctx, sqliteSessionID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []string
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) GetSettingsByBotID(ctx context.Context, id pgtype.UUID) (pgsqlc.GetSettingsByBotIDRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetSettingsByBotIDRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.GetSettingsByBotIDRow{}, err
	}
	out, err := q.store.queries.GetSettingsByBotID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.GetSettingsByBotIDRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetSettingsByBotIDRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetSettingsByBotIDRow{}, err
	}
	return result, nil
}

func (q *Queries) GetSnapshotByContainerAndRuntimeName(ctx context.Context, arg pgsqlc.GetSnapshotByContainerAndRuntimeNameParams) (pgsqlc.Snapshot, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Snapshot{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetSnapshotByContainerAndRuntimeNameParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.Snapshot{}, err
	}
	out, err := q.store.queries.GetSnapshotByContainerAndRuntimeName(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.Snapshot{}, mapQueryErr(err)
	}
	var result pgsqlc.Snapshot
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Snapshot{}, err
	}
	return result, nil
}

func (q *Queries) GetSpeechModelWithProvider(ctx context.Context, id pgtype.UUID) (pgsqlc.GetSpeechModelWithProviderRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetSpeechModelWithProviderRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.GetSpeechModelWithProviderRow{}, err
	}
	out, err := q.store.queries.GetSpeechModelWithProvider(ctx, sqliteId)
	if err != nil {
		return pgsqlc.GetSpeechModelWithProviderRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetSpeechModelWithProviderRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetSpeechModelWithProviderRow{}, err
	}
	return result, nil
}

func (q *Queries) GetStorageProviderByID(ctx context.Context, id pgtype.UUID) (pgsqlc.StorageProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.StorageProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.StorageProvider{}, err
	}
	out, err := q.store.queries.GetStorageProviderByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.StorageProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.StorageProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.StorageProvider{}, err
	}
	return result, nil
}

func (q *Queries) GetStorageProviderByName(ctx context.Context, name string) (pgsqlc.StorageProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.StorageProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteName string
	if err := convertValue(name, &sqliteName); err != nil {
		return pgsqlc.StorageProvider{}, err
	}
	out, err := q.store.queries.GetStorageProviderByName(ctx, sqliteName)
	if err != nil {
		return pgsqlc.StorageProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.StorageProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.StorageProvider{}, err
	}
	return result, nil
}

func (q *Queries) GetTokenUsageByDayAndType(ctx context.Context, arg pgsqlc.GetTokenUsageByDayAndTypeParams) ([]pgsqlc.GetTokenUsageByDayAndTypeRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetTokenUsageByDayAndTypeParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.GetTokenUsageByDayAndType(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.GetTokenUsageByDayAndTypeRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) GetTokenUsageByModel(ctx context.Context, arg pgsqlc.GetTokenUsageByModelParams) ([]pgsqlc.GetTokenUsageByModelRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetTokenUsageByModelParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.GetTokenUsageByModel(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.GetTokenUsageByModelRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) GetToolApprovalRequest(ctx context.Context, id pgtype.UUID) (pgsqlc.ToolApprovalRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ToolApprovalRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	out, err := q.store.queries.GetToolApprovalRequest(ctx, sqliteId)
	if err != nil {
		return pgsqlc.ToolApprovalRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.ToolApprovalRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	return result, nil
}

func (q *Queries) GetTranscriptionModelWithProvider(ctx context.Context, id pgtype.UUID) (pgsqlc.GetTranscriptionModelWithProviderRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetTranscriptionModelWithProviderRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.GetTranscriptionModelWithProviderRow{}, err
	}
	out, err := q.store.queries.GetTranscriptionModelWithProvider(ctx, sqliteId)
	if err != nil {
		return pgsqlc.GetTranscriptionModelWithProviderRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetTranscriptionModelWithProviderRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetTranscriptionModelWithProviderRow{}, err
	}
	return result, nil
}

func (q *Queries) GetUserByID(ctx context.Context, id pgtype.UUID) (pgsqlc.User, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.User{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.User{}, err
	}
	out, err := q.store.queries.GetUserByID(ctx, sqliteId)
	if err != nil {
		return pgsqlc.User{}, mapQueryErr(err)
	}
	var result pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.User{}, err
	}
	return result, nil
}

func (q *Queries) GetVideoModelWithProvider(ctx context.Context, id pgtype.UUID) (pgsqlc.GetVideoModelWithProviderRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetVideoModelWithProviderRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.GetVideoModelWithProviderRow{}, err
	}
	out, err := q.store.queries.GetVideoModelWithProvider(ctx, sqliteId)
	if err != nil {
		return pgsqlc.GetVideoModelWithProviderRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetVideoModelWithProviderRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetVideoModelWithProviderRow{}, err
	}
	return result, nil
}

func (q *Queries) GetUserChannelBinding(ctx context.Context, arg pgsqlc.GetUserChannelBindingParams) (pgsqlc.UserChannelBinding, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserChannelBinding{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetUserChannelBindingParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserChannelBinding{}, err
	}
	out, err := q.store.queries.GetUserChannelBinding(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserChannelBinding{}, mapQueryErr(err)
	}
	var result pgsqlc.UserChannelBinding
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserChannelBinding{}, err
	}
	return result, nil
}

func (q *Queries) GetUserProviderOAuthToken(ctx context.Context, arg pgsqlc.GetUserProviderOAuthTokenParams) (pgsqlc.UserProviderOauthToken, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserProviderOauthToken{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetUserProviderOAuthTokenParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserProviderOauthToken{}, err
	}
	out, err := q.store.queries.GetUserProviderOAuthToken(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserProviderOauthToken{}, mapQueryErr(err)
	}
	var result pgsqlc.UserProviderOauthToken
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserProviderOauthToken{}, err
	}
	return result, nil
}

func (q *Queries) GetUserProviderOAuthTokenByState(ctx context.Context, state string) (pgsqlc.UserProviderOauthToken, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserProviderOauthToken{}, errSQLiteQueriesNotConfigured
	}
	var sqliteState string
	if err := convertValue(state, &sqliteState); err != nil {
		return pgsqlc.UserProviderOauthToken{}, err
	}
	out, err := q.store.queries.GetUserProviderOAuthTokenByState(ctx, sqliteState)
	if err != nil {
		return pgsqlc.UserProviderOauthToken{}, mapQueryErr(err)
	}
	var result pgsqlc.UserProviderOauthToken
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserProviderOauthToken{}, err
	}
	return result, nil
}

func (q *Queries) GetVersionSnapshotRuntimeName(ctx context.Context, arg pgsqlc.GetVersionSnapshotRuntimeNameParams) (string, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return "", errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetVersionSnapshotRuntimeNameParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return "", err
	}
	out, err := q.store.queries.GetVersionSnapshotRuntimeName(ctx, sqliteArg)
	if err != nil {
		return "", mapQueryErr(err)
	}
	var result string
	if err := convertValue(out, &result); err != nil {
		return "", err
	}
	return result, nil
}

func (q *Queries) IncrementScheduleCalls(ctx context.Context, id pgtype.UUID) (pgsqlc.Schedule, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Schedule{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.Schedule{}, err
	}
	out, err := q.store.queries.IncrementScheduleCalls(ctx, sqliteId)
	if err != nil {
		return pgsqlc.Schedule{}, mapQueryErr(err)
	}
	var result pgsqlc.Schedule
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Schedule{}, err
	}
	return result, nil
}

func (q *Queries) InsertLifecycleEvent(ctx context.Context, arg pgsqlc.InsertLifecycleEventParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.InsertLifecycleEventParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.InsertLifecycleEvent(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) InsertVersion(ctx context.Context, arg pgsqlc.InsertVersionParams) (pgsqlc.ContainerVersion, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ContainerVersion{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.InsertVersionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ContainerVersion{}, err
	}
	out, err := q.store.queries.InsertVersion(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ContainerVersion{}, mapQueryErr(err)
	}
	var result pgsqlc.ContainerVersion
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ContainerVersion{}, err
	}
	return result, nil
}

func (q *Queries) ListAccounts(ctx context.Context) ([]pgsqlc.User, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListAccounts(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListActiveMessagesSince(ctx context.Context, arg pgsqlc.ListActiveMessagesSinceParams) ([]pgsqlc.ListActiveMessagesSinceRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListActiveMessagesSinceParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListActiveMessagesSince(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListActiveMessagesSinceRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListActiveMessagesSinceBySession(ctx context.Context, arg pgsqlc.ListActiveMessagesSinceBySessionParams) ([]pgsqlc.ListActiveMessagesSinceBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListActiveMessagesSinceBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListActiveMessagesSinceBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListActiveMessagesSinceBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListAutoStartContainers(ctx context.Context) ([]pgsqlc.Container, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListAutoStartContainers(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Container
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListBotACLRules(ctx context.Context, botID pgtype.UUID) ([]pgsqlc.ListBotACLRulesRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListBotACLRules(ctx, sqliteBotID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListBotACLRulesRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListBotChannelConfigsByType(ctx context.Context, channelType string) ([]pgsqlc.BotChannelConfig, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteChannelType string
	if err := convertValue(channelType, &sqliteChannelType); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListBotChannelConfigsByType(ctx, sqliteChannelType)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.BotChannelConfig
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListBotEmailBindings(ctx context.Context, botID pgtype.UUID) ([]pgsqlc.BotEmailBinding, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListBotEmailBindings(ctx, sqliteBotID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.BotEmailBinding
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListBotEmailBindingsByProvider(ctx context.Context, emailProviderID pgtype.UUID) ([]pgsqlc.BotEmailBinding, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteEmailProviderID string
	if err := convertValue(emailProviderID, &sqliteEmailProviderID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListBotEmailBindingsByProvider(ctx, sqliteEmailProviderID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.BotEmailBinding
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListBotsByOwner(ctx context.Context, ownerUserID pgtype.UUID) ([]pgsqlc.ListBotsByOwnerRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteOwnerUserID string
	if err := convertValue(ownerUserID, &sqliteOwnerUserID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListBotsByOwner(ctx, sqliteOwnerUserID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListBotsByOwnerRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListChatParticipants(ctx context.Context, chatID pgtype.UUID) ([]pgsqlc.ListChatParticipantsRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteChatID string
	if err := convertValue(chatID, &sqliteChatID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListChatParticipants(ctx, sqliteChatID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListChatParticipantsRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListChatRoutes(ctx context.Context, chatID pgtype.UUID) ([]pgsqlc.ListChatRoutesRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteChatID string
	if err := convertValue(chatID, &sqliteChatID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListChatRoutes(ctx, sqliteChatID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListChatRoutesRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListChatsByBotAndUser(ctx context.Context, arg pgsqlc.ListChatsByBotAndUserParams) ([]pgsqlc.ListChatsByBotAndUserRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListChatsByBotAndUserParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListChatsByBotAndUser(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListChatsByBotAndUserRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListCompactionLogsByBot(ctx context.Context, arg pgsqlc.ListCompactionLogsByBotParams) ([]pgsqlc.BotHistoryMessageCompact, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListCompactionLogsByBotParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListCompactionLogsByBot(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.BotHistoryMessageCompact
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListCompactionLogsBySession(ctx context.Context, sessionID pgtype.UUID) ([]pgsqlc.BotHistoryMessageCompact, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID sql.NullString
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListCompactionLogsBySession(ctx, sqliteSessionID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.BotHistoryMessageCompact
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListEmailOutboxByBot(ctx context.Context, arg pgsqlc.ListEmailOutboxByBotParams) ([]pgsqlc.EmailOutbox, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListEmailOutboxByBotParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListEmailOutboxByBot(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.EmailOutbox
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListEmailProviders(ctx context.Context) ([]pgsqlc.EmailProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListEmailProviders(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.EmailProvider
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListEmailProvidersByProvider(ctx context.Context, provider string) ([]pgsqlc.EmailProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteProvider string
	if err := convertValue(provider, &sqliteProvider); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListEmailProvidersByProvider(ctx, sqliteProvider)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.EmailProvider
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListEmailProvidersByUser(ctx context.Context, userID pgtype.UUID) ([]pgsqlc.EmailProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteUserID string
	if err := convertValue(userID, &sqliteUserID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListEmailProvidersByUser(ctx, sqliteUserID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.EmailProvider
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListEmailProvidersByUserAndProvider(ctx context.Context, arg pgsqlc.ListEmailProvidersByUserAndProviderParams) ([]pgsqlc.EmailProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListEmailProvidersByUserAndProviderParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListEmailProvidersByUserAndProvider(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.EmailProvider
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListEnabledModels(ctx context.Context) ([]pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListEnabledModels(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListEnabledModelsByProviderClientType(ctx context.Context, clientType string) ([]pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteClientType string
	if err := convertValue(clientType, &sqliteClientType); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListEnabledModelsByProviderClientType(ctx, sqliteClientType)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListEnabledModelsByType(ctx context.Context, type_ string) ([]pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteType_ string
	if err := convertValue(type_, &sqliteType_); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListEnabledModelsByType(ctx, sqliteType_)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListEnabledSchedules(ctx context.Context) ([]pgsqlc.Schedule, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListEnabledSchedules(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Schedule
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListHeartbeatEnabledBots(ctx context.Context) ([]pgsqlc.ListHeartbeatEnabledBotsRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListHeartbeatEnabledBots(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListHeartbeatEnabledBotsRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListHeartbeatLogsByBot(ctx context.Context, arg pgsqlc.ListHeartbeatLogsByBotParams) ([]pgsqlc.ListHeartbeatLogsByBotRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListHeartbeatLogsByBotParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListHeartbeatLogsByBot(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListHeartbeatLogsByBotRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMCPConnectionsByBotID(ctx context.Context, botID pgtype.UUID) ([]pgsqlc.McpConnection, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMCPConnectionsByBotID(ctx, sqliteBotID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.McpConnection
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMemoryProviders(ctx context.Context) ([]pgsqlc.MemoryProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListMemoryProviders(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.MemoryProvider
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessageAssets(ctx context.Context, messageID pgtype.UUID) ([]pgsqlc.ListMessageAssetsRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteMessageID string
	if err := convertValue(messageID, &sqliteMessageID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessageAssets(ctx, sqliteMessageID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessageAssetsRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessageAssetsBatch(ctx context.Context, messageIds []pgtype.UUID) ([]pgsqlc.ListMessageAssetsBatchRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteMessageIds []string
	if err := convertValue(messageIds, &sqliteMessageIds); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessageAssetsBatch(ctx, sqliteMessageIds)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessageAssetsBatchRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessages(ctx context.Context, botID pgtype.UUID) ([]pgsqlc.ListMessagesRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessages(ctx, sqliteBotID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) CreateHistoryTurn(ctx context.Context, arg pgsqlc.CreateHistoryTurnParams) (pgsqlc.BotHistoryTurn, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotHistoryTurn{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateHistoryTurnParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotHistoryTurn{}, err
	}
	out, err := q.store.queries.CreateHistoryTurn(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotHistoryTurn{}, mapQueryErr(err)
	}
	var result pgsqlc.BotHistoryTurn
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotHistoryTurn{}, err
	}
	return result, nil
}

func (q *Queries) BindHistoryTurnAssistantByRequest(ctx context.Context, arg pgsqlc.BindHistoryTurnAssistantByRequestParams) (pgsqlc.BotHistoryTurn, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotHistoryTurn{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.BindHistoryTurnAssistantByRequestParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotHistoryTurn{}, err
	}
	out, err := q.store.queries.BindHistoryTurnAssistantByRequest(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotHistoryTurn{}, mapQueryErr(err)
	}
	var result pgsqlc.BotHistoryTurn
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotHistoryTurn{}, err
	}
	return result, nil
}

func (q *Queries) BindLatestHistoryTurnAssistant(ctx context.Context, arg pgsqlc.BindLatestHistoryTurnAssistantParams) (pgsqlc.BotHistoryTurn, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotHistoryTurn{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.BindLatestHistoryTurnAssistantParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotHistoryTurn{}, err
	}
	out, err := q.store.queries.BindLatestHistoryTurnAssistant(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotHistoryTurn{}, mapQueryErr(err)
	}
	var result pgsqlc.BotHistoryTurn
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotHistoryTurn{}, err
	}
	return result, nil
}

func (q *Queries) LinkMessageToHistoryTurn(ctx context.Context, arg pgsqlc.LinkMessageToHistoryTurnParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.LinkMessageToHistoryTurnParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.LinkMessageToHistoryTurn(ctx, sqliteArg))
}

func (q *Queries) AppendMessageToHistoryTurnByRequest(ctx context.Context, arg pgsqlc.AppendMessageToHistoryTurnByRequestParams) (pgtype.UUID, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgtype.UUID{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.AppendMessageToHistoryTurnByRequestParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgtype.UUID{}, err
	}
	out, err := q.store.queries.AppendMessageToHistoryTurnByRequest(ctx, sqliteArg)
	if err != nil {
		return pgtype.UUID{}, mapQueryErr(err)
	}
	var result pgtype.UUID
	if err := convertValue(out, &result); err != nil {
		return pgtype.UUID{}, err
	}
	return result, nil
}

func (q *Queries) AppendMessageToLatestHistoryTurn(ctx context.Context, arg pgsqlc.AppendMessageToLatestHistoryTurnParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.AppendMessageToLatestHistoryTurnParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.AppendMessageToLatestHistoryTurn(ctx, sqliteArg))
}

func (q *Queries) LinkUnassignedMessagesAfterHistoryTurnAssistant(ctx context.Context, turnID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteTurnID string
	if err := convertValue(turnID, &sqliteTurnID); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.LinkUnassignedMessagesAfterHistoryTurnAssistant(ctx, sqliteTurnID))
}

func (q *Queries) GetVisibleHistoryTurnByMessage(ctx context.Context, arg pgsqlc.GetVisibleHistoryTurnByMessageParams) (pgsqlc.BotHistoryTurn, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotHistoryTurn{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetVisibleHistoryTurnByMessageParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotHistoryTurn{}, err
	}
	out, err := q.store.queries.GetVisibleHistoryTurnByMessage(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotHistoryTurn{}, mapQueryErr(err)
	}
	var result pgsqlc.BotHistoryTurn
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotHistoryTurn{}, err
	}
	return result, nil
}

func (q *Queries) GetHistoryTurnByID(ctx context.Context, arg pgsqlc.GetHistoryTurnByIDParams) (pgsqlc.BotHistoryTurn, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotHistoryTurn{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetHistoryTurnByIDParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotHistoryTurn{}, err
	}
	out, err := q.store.queries.GetHistoryTurnByID(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotHistoryTurn{}, mapQueryErr(err)
	}
	var result pgsqlc.BotHistoryTurn
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotHistoryTurn{}, err
	}
	return result, nil
}

func (q *Queries) GetLatestVisibleHistoryTurnBySession(ctx context.Context, sessionID pgtype.UUID) (pgsqlc.BotHistoryTurn, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotHistoryTurn{}, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID string
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return pgsqlc.BotHistoryTurn{}, err
	}
	out, err := q.store.queries.GetLatestVisibleHistoryTurnBySession(ctx, sqliteSessionID)
	if err != nil {
		return pgsqlc.BotHistoryTurn{}, mapQueryErr(err)
	}
	var result pgsqlc.BotHistoryTurn
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotHistoryTurn{}, err
	}
	return result, nil
}

func (q *Queries) SupersedeHistoryTurn(ctx context.Context, arg pgsqlc.SupersedeHistoryTurnParams) (pgsqlc.BotHistoryTurn, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotHistoryTurn{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.SupersedeHistoryTurnParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotHistoryTurn{}, err
	}
	out, err := q.store.queries.SupersedeHistoryTurn(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotHistoryTurn{}, mapQueryErr(err)
	}
	var result pgsqlc.BotHistoryTurn
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotHistoryTurn{}, err
	}
	return result, nil
}

func (q *Queries) ReplaceHistoryTurn(ctx context.Context, arg pgsqlc.ReplaceHistoryTurnParams) (pgsqlc.ReplaceHistoryTurnRow, error) {
	if q == nil || q.store == nil || q.store.db == nil || q.store.queries == nil {
		return pgsqlc.ReplaceHistoryTurnRow{}, errSQLiteQueriesNotConfigured
	}
	tx, err := q.store.db.BeginTx(ctx, nil)
	if err != nil {
		return pgsqlc.ReplaceHistoryTurnRow{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	qtx := q.store.queries.WithTx(tx)

	var oldTurnID string
	if err := convertValue(arg.OldTurnID, &oldTurnID); err != nil {
		return pgsqlc.ReplaceHistoryTurnRow{}, err
	}
	var sessionID string
	if err := convertValue(arg.SessionID, &sessionID); err != nil {
		return pgsqlc.ReplaceHistoryTurnRow{}, err
	}
	oldTurn, err := qtx.GetHistoryTurnByID(ctx, sqlitesqlc.GetHistoryTurnByIDParams{
		OldTurnID: oldTurnID,
		SessionID: sessionID,
	})
	if err != nil {
		return pgsqlc.ReplaceHistoryTurnRow{}, mapQueryErr(err)
	}

	var createArg sqlitesqlc.CreateHistoryTurnParams
	if err := convertValue(pgsqlc.CreateHistoryTurnParams{
		SessionID:          arg.SessionID,
		RequestMessageID:   arg.RequestMessageID,
		AssistantMessageID: arg.AssistantMessageID,
	}, &createArg); err != nil {
		return pgsqlc.ReplaceHistoryTurnRow{}, err
	}
	createArg.BotID = oldTurn.BotID
	created, err := qtx.CreateHistoryTurn(ctx, createArg)
	if err != nil {
		return pgsqlc.ReplaceHistoryTurnRow{}, mapQueryErr(err)
	}
	var supersedeArg sqlitesqlc.SupersedeHistoryTurnParams
	if err := convertValue(pgsqlc.SupersedeHistoryTurnParams{
		OldTurnID:        arg.OldTurnID,
		SessionID:        arg.SessionID,
		SupersededAt:     arg.SupersededAt,
		SupersededReason: arg.SupersededReason,
	}, &supersedeArg); err != nil {
		return pgsqlc.ReplaceHistoryTurnRow{}, err
	}
	supersedeArg.SupersededByTurnID = sql.NullString{String: created.ID, Valid: strings.TrimSpace(created.ID) != ""}
	if _, err := qtx.SupersedeHistoryTurn(ctx, supersedeArg); err != nil {
		return pgsqlc.ReplaceHistoryTurnRow{}, mapQueryErr(err)
	}
	if err := tx.Commit(); err != nil {
		return pgsqlc.ReplaceHistoryTurnRow{}, err
	}
	var result pgsqlc.ReplaceHistoryTurnRow
	if err := convertValue(created, &result); err != nil {
		return pgsqlc.ReplaceHistoryTurnRow{}, err
	}
	return result, nil
}

func (q *Queries) ListMessagesBefore(ctx context.Context, arg pgsqlc.ListMessagesBeforeParams) ([]pgsqlc.ListMessagesBeforeRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListMessagesBeforeParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessagesBefore(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesBeforeRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) GetMessageByExternalIDBySession(ctx context.Context, arg pgsqlc.GetMessageByExternalIDBySessionParams) (pgsqlc.GetMessageByExternalIDBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetMessageByExternalIDBySessionRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetMessageByExternalIDBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.GetMessageByExternalIDBySessionRow{}, err
	}
	out, err := q.store.queries.GetMessageByExternalIDBySession(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.GetMessageByExternalIDBySessionRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetMessageByExternalIDBySessionRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetMessageByExternalIDBySessionRow{}, err
	}
	return result, nil
}

func (q *Queries) GetVisibleMessageCursorByExternalIDBySession(ctx context.Context, arg pgsqlc.GetVisibleMessageCursorByExternalIDBySessionParams) (pgsqlc.GetVisibleMessageCursorByExternalIDBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetVisibleMessageCursorByExternalIDBySessionRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetVisibleMessageCursorByExternalIDBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.GetVisibleMessageCursorByExternalIDBySessionRow{}, err
	}
	out, err := q.store.queries.GetVisibleMessageCursorByExternalIDBySession(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.GetVisibleMessageCursorByExternalIDBySessionRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetVisibleMessageCursorByExternalIDBySessionRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetVisibleMessageCursorByExternalIDBySessionRow{}, err
	}
	return result, nil
}

func (q *Queries) GetMessageByIDBySession(ctx context.Context, arg pgsqlc.GetMessageByIDBySessionParams) (pgsqlc.GetMessageByIDBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.GetMessageByIDBySessionRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetMessageByIDBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.GetMessageByIDBySessionRow{}, err
	}
	out, err := q.store.queries.GetMessageByIDBySession(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.GetMessageByIDBySessionRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetMessageByIDBySessionRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetMessageByIDBySessionRow{}, err
	}
	return result, nil
}

func (q *Queries) ListMessagesAfterBySession(ctx context.Context, arg pgsqlc.ListMessagesAfterBySessionParams) ([]pgsqlc.ListMessagesAfterBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListMessagesAfterBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessagesAfterBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesAfterBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessagesAfterCursorBySession(ctx context.Context, arg pgsqlc.ListMessagesAfterCursorBySessionParams) ([]pgsqlc.ListMessagesAfterCursorBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListMessagesAfterCursorBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessagesAfterCursorBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesAfterCursorBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessagesAfterMessageBySession(ctx context.Context, arg pgsqlc.ListMessagesAfterMessageBySessionParams) ([]pgsqlc.ListMessagesAfterMessageBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListMessagesAfterMessageBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessagesAfterMessageBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesAfterMessageBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessagesBeforeBySession(ctx context.Context, arg pgsqlc.ListMessagesBeforeBySessionParams) ([]pgsqlc.ListMessagesBeforeBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListMessagesBeforeBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessagesBeforeBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesBeforeBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessagesBeforeMessageBySession(ctx context.Context, arg pgsqlc.ListMessagesBeforeMessageBySessionParams) ([]pgsqlc.ListMessagesBeforeMessageBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListMessagesBeforeMessageBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessagesBeforeMessageBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesBeforeMessageBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessagesBeforeCursorBySession(ctx context.Context, arg pgsqlc.ListMessagesBeforeCursorBySessionParams) ([]pgsqlc.ListMessagesBeforeCursorBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListMessagesBeforeCursorBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessagesBeforeCursorBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesBeforeCursorBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessagesBySession(ctx context.Context, sessionID pgtype.UUID) ([]pgsqlc.ListMessagesBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID sql.NullString
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessagesBySession(ctx, sqliteSessionID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessagesLatest(ctx context.Context, arg pgsqlc.ListMessagesLatestParams) ([]pgsqlc.ListMessagesLatestRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListMessagesLatestParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessagesLatest(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesLatestRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessagesLatestBySession(ctx context.Context, arg pgsqlc.ListMessagesLatestBySessionParams) ([]pgsqlc.ListMessagesLatestBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListMessagesLatestBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessagesLatestBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesLatestBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessagesLatestUIBySession(ctx context.Context, arg pgsqlc.ListMessagesLatestUIBySessionParams) ([]pgsqlc.ListMessagesLatestUIBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListMessagesLatestUIBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessagesLatestUIBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesLatestUIBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListVisibleMessagesFromBySession(ctx context.Context, arg pgsqlc.ListVisibleMessagesFromBySessionParams) ([]pgsqlc.ListVisibleMessagesFromBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListVisibleMessagesFromBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListVisibleMessagesFromBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListVisibleMessagesFromBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessagesSince(ctx context.Context, arg pgsqlc.ListMessagesSinceParams) ([]pgsqlc.ListMessagesSinceRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListMessagesSinceParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessagesSince(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesSinceRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessagesSinceBySession(ctx context.Context, arg pgsqlc.ListMessagesSinceBySessionParams) ([]pgsqlc.ListMessagesSinceBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListMessagesSinceBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListMessagesSinceBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesSinceBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListModels(ctx context.Context) ([]pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListModels(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListModelsByModelID(ctx context.Context, modelID string) ([]pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteModelID string
	if err := convertValue(modelID, &sqliteModelID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListModelsByModelID(ctx, sqliteModelID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListModelsByProviderClientType(ctx context.Context, clientType string) ([]pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteClientType string
	if err := convertValue(clientType, &sqliteClientType); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListModelsByProviderClientType(ctx, sqliteClientType)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListModelsByProviderID(ctx context.Context, providerID pgtype.UUID) ([]pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteProviderID string
	if err := convertValue(providerID, &sqliteProviderID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListModelsByProviderID(ctx, sqliteProviderID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListModelsByProviderIDAndType(ctx context.Context, arg pgsqlc.ListModelsByProviderIDAndTypeParams) ([]pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListModelsByProviderIDAndTypeParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListModelsByProviderIDAndType(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListModelsByType(ctx context.Context, type_ string) ([]pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteType_ string
	if err := convertValue(type_, &sqliteType_); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListModelsByType(ctx, sqliteType_)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListModelVariantsByModelUUID(ctx context.Context, modelUuid pgtype.UUID) ([]pgsqlc.ModelVariant, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteModelUuid string
	if err := convertValue(modelUuid, &sqliteModelUuid); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListModelVariantsByModelUUID(ctx, sqliteModelUuid)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ModelVariant
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListObservedConversationsByChannelIdentity(ctx context.Context, arg pgsqlc.ListObservedConversationsByChannelIdentityParams) ([]pgsqlc.ListObservedConversationsByChannelIdentityRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListObservedConversationsByChannelIdentityParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListObservedConversationsByChannelIdentity(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListObservedConversationsByChannelIdentityRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListObservedConversationsByChannelType(ctx context.Context, arg pgsqlc.ListObservedConversationsByChannelTypeParams) ([]pgsqlc.ListObservedConversationsByChannelTypeRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListObservedConversationsByChannelTypeParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListObservedConversationsByChannelType(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListObservedConversationsByChannelTypeRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListPendingToolApprovalsBySession(ctx context.Context, arg pgsqlc.ListPendingToolApprovalsBySessionParams) ([]pgsqlc.ToolApprovalRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListPendingToolApprovalsBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListPendingToolApprovalsBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ToolApprovalRequest
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListProviders(ctx context.Context) ([]pgsqlc.Provider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListProviders(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Provider
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListReadableBindingsByProvider(ctx context.Context, emailProviderID pgtype.UUID) ([]pgsqlc.BotEmailBinding, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteEmailProviderID string
	if err := convertValue(emailProviderID, &sqliteEmailProviderID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListReadableBindingsByProvider(ctx, sqliteEmailProviderID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.BotEmailBinding
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListScheduleLogsByBot(ctx context.Context, arg pgsqlc.ListScheduleLogsByBotParams) ([]pgsqlc.ListScheduleLogsByBotRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListScheduleLogsByBotParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListScheduleLogsByBot(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListScheduleLogsByBotRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListScheduleLogsBySchedule(ctx context.Context, arg pgsqlc.ListScheduleLogsByScheduleParams) ([]pgsqlc.ListScheduleLogsByScheduleRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListScheduleLogsByScheduleParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListScheduleLogsBySchedule(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListScheduleLogsByScheduleRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSchedulesByBot(ctx context.Context, botID pgtype.UUID) ([]pgsqlc.Schedule, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSchedulesByBot(ctx, sqliteBotID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Schedule
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListFetchProviders(ctx context.Context) ([]pgsqlc.FetchProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListFetchProviders(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.FetchProvider
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListFetchProvidersByProvider(ctx context.Context, provider string) ([]pgsqlc.FetchProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteProvider string
	if err := convertValue(provider, &sqliteProvider); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListFetchProvidersByProvider(ctx, sqliteProvider)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.FetchProvider
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSearchProviders(ctx context.Context) ([]pgsqlc.SearchProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListSearchProviders(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.SearchProvider
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSearchProvidersByProvider(ctx context.Context, provider string) ([]pgsqlc.SearchProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteProvider string
	if err := convertValue(provider, &sqliteProvider); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSearchProvidersByProvider(ctx, sqliteProvider)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.SearchProvider
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSessionEventsByBot(ctx context.Context, botID pgtype.UUID) ([]pgsqlc.BotSessionEvent, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSessionEventsByBot(ctx, sqliteBotID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.BotSessionEvent
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSessionEventsBySession(ctx context.Context, sessionID pgtype.UUID) ([]pgsqlc.BotSessionEvent, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID string
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSessionEventsBySession(ctx, sqliteSessionID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.BotSessionEvent
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSessionEventsBySessionAfter(ctx context.Context, arg pgsqlc.ListSessionEventsBySessionAfterParams) ([]pgsqlc.BotSessionEvent, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListSessionEventsBySessionAfterParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSessionEventsBySessionAfter(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.BotSessionEvent
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSessionsByBot(ctx context.Context, botID pgtype.UUID) ([]pgsqlc.ListSessionsByBotRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSessionsByBot(ctx, sqliteBotID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListSessionsByBotRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSessionsByBotAndCreatedByUser(ctx context.Context, arg pgsqlc.ListSessionsByBotAndCreatedByUserParams) ([]pgsqlc.ListSessionsByBotAndCreatedByUserRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListSessionsByBotAndCreatedByUserParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSessionsByBotAndCreatedByUser(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListSessionsByBotAndCreatedByUserRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSessionsByBotPaged(ctx context.Context, arg pgsqlc.ListSessionsByBotPagedParams) ([]pgsqlc.ListSessionsByBotPagedRow, error) {
	return q.listSessionsByBotPaged(ctx, arg)
}

func (q *Queries) ListSessionsByBotAndCreatedByUserPaged(ctx context.Context, arg pgsqlc.ListSessionsByBotAndCreatedByUserPagedParams) ([]pgsqlc.ListSessionsByBotAndCreatedByUserPagedRow, error) {
	return q.listSessionsByBotAndCreatedByUserPaged(ctx, arg)
}

func (q *Queries) ListSessionsByRoute(ctx context.Context, routeID pgtype.UUID) ([]pgsqlc.BotSession, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteRouteID sql.NullString
	if err := convertValue(routeID, &sqliteRouteID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSessionsByRoute(ctx, sqliteRouteID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.BotSession
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSnapshotsByContainerID(ctx context.Context, containerID string) ([]pgsqlc.Snapshot, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteContainerID string
	if err := convertValue(containerID, &sqliteContainerID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSnapshotsByContainerID(ctx, sqliteContainerID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Snapshot
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSnapshotsWithVersionByContainerID(ctx context.Context, containerID string) ([]pgsqlc.ListSnapshotsWithVersionByContainerIDRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteContainerID string
	if err := convertValue(containerID, &sqliteContainerID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSnapshotsWithVersionByContainerID(ctx, sqliteContainerID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListSnapshotsWithVersionByContainerIDRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSpeechModels(ctx context.Context) ([]pgsqlc.ListSpeechModelsRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListSpeechModels(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListSpeechModelsRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSpeechModelsByProviderID(ctx context.Context, providerID pgtype.UUID) ([]pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteProviderID string
	if err := convertValue(providerID, &sqliteProviderID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSpeechModelsByProviderID(ctx, sqliteProviderID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSpeechProviders(ctx context.Context) ([]pgsqlc.Provider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListSpeechProviders(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Provider
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListStorageProviders(ctx context.Context) ([]pgsqlc.StorageProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListStorageProviders(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.StorageProvider
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSubagentSessionsByParent(ctx context.Context, parentSessionID pgtype.UUID) ([]pgsqlc.BotSession, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteParentSessionID sql.NullString
	if err := convertValue(parentSessionID, &sqliteParentSessionID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSubagentSessionsByParent(ctx, sqliteParentSessionID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.BotSession
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListThreadsByParent(ctx context.Context, id pgtype.UUID) ([]pgsqlc.ListThreadsByParentRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListThreadsByParent(ctx, sqliteId)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListThreadsByParentRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListTokenUsageRecords(ctx context.Context, arg pgsqlc.ListTokenUsageRecordsParams) ([]pgsqlc.ListTokenUsageRecordsRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListTokenUsageRecordsParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListTokenUsageRecords(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListTokenUsageRecordsRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListToolApprovalsBySession(ctx context.Context, arg pgsqlc.ListToolApprovalsBySessionParams) ([]pgsqlc.ToolApprovalRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListToolApprovalsBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListToolApprovalsBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ToolApprovalRequest
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListToolApprovalsBySessionToolCalls(ctx context.Context, arg pgsqlc.ListToolApprovalsBySessionToolCallsParams) ([]pgsqlc.ToolApprovalRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListToolApprovalsBySessionToolCallsParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListToolApprovalsBySessionToolCalls(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ToolApprovalRequest
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListTranscriptionModels(ctx context.Context) ([]pgsqlc.ListTranscriptionModelsRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListTranscriptionModels(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListTranscriptionModelsRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListTranscriptionModelsByProviderID(ctx context.Context, providerID pgtype.UUID) ([]pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteProviderID string
	if err := convertValue(providerID, &sqliteProviderID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListTranscriptionModelsByProviderID(ctx, sqliteProviderID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListTranscriptionProviders(ctx context.Context) ([]pgsqlc.Provider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListTranscriptionProviders(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Provider
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListUncompactedMessagesBySession(ctx context.Context, sessionID pgtype.UUID) ([]pgsqlc.ListUncompactedMessagesBySessionRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteSessionID sql.NullString
	if err := convertValue(sessionID, &sqliteSessionID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListUncompactedMessagesBySession(ctx, sqliteSessionID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListUncompactedMessagesBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListVideoModels(ctx context.Context) ([]pgsqlc.ListVideoModelsRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListVideoModels(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListVideoModelsRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListVideoModelsByProviderID(ctx context.Context, providerID pgtype.UUID) ([]pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteProviderID string
	if err := convertValue(providerID, &sqliteProviderID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListVideoModelsByProviderID(ctx, sqliteProviderID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListVideoProviders(ctx context.Context) ([]pgsqlc.Provider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	out, err := q.store.queries.ListVideoProviders(ctx)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Provider
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListUserChannelBindingsByPlatform(ctx context.Context, channelType string) ([]pgsqlc.UserChannelBinding, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteChannelType string
	if err := convertValue(channelType, &sqliteChannelType); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListUserChannelBindingsByPlatform(ctx, sqliteChannelType)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.UserChannelBinding
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListVersionsByContainerID(ctx context.Context, containerID string) ([]pgsqlc.ListVersionsByContainerIDRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteContainerID string
	if err := convertValue(containerID, &sqliteContainerID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListVersionsByContainerID(ctx, sqliteContainerID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListVersionsByContainerIDRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListVisibleChatsByBotAndUser(ctx context.Context, arg pgsqlc.ListVisibleChatsByBotAndUserParams) ([]pgsqlc.ListVisibleChatsByBotAndUserRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListVisibleChatsByBotAndUserParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListVisibleChatsByBotAndUser(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListVisibleChatsByBotAndUserRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) MarkMessagesCompacted(ctx context.Context, arg pgsqlc.MarkMessagesCompactedParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.MarkMessagesCompactedParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.MarkMessagesCompacted(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) NextVersion(ctx context.Context, containerID string) (int32, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteContainerID string
	if err := convertValue(containerID, &sqliteContainerID); err != nil {
		return 0, err
	}
	out, err := q.store.queries.NextVersion(ctx, sqliteContainerID)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int32
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) RejectToolApprovalRequest(ctx context.Context, arg pgsqlc.RejectToolApprovalRequestParams) (pgsqlc.ToolApprovalRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ToolApprovalRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.RejectToolApprovalRequestParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	out, err := q.store.queries.RejectToolApprovalRequest(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ToolApprovalRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.ToolApprovalRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	return result, nil
}

func (q *Queries) RemoveChatParticipant(ctx context.Context, arg pgsqlc.RemoveChatParticipantParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.RemoveChatParticipantParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.RemoveChatParticipant(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) SaveMatrixSyncSinceToken(ctx context.Context, arg pgsqlc.SaveMatrixSyncSinceTokenParams) (int64, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return 0, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.SaveMatrixSyncSinceTokenParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return 0, err
	}
	out, err := q.store.queries.SaveMatrixSyncSinceToken(ctx, sqliteArg)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) SearchAccounts(ctx context.Context, arg pgsqlc.SearchAccountsParams) ([]pgsqlc.User, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.SearchAccountsParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.SearchAccounts(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) SearchChannelIdentities(ctx context.Context, arg pgsqlc.SearchChannelIdentitiesParams) ([]pgsqlc.ChannelIdentity, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.SearchChannelIdentitiesParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.SearchChannelIdentities(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ChannelIdentity
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) SearchMessages(ctx context.Context, arg pgsqlc.SearchMessagesParams) ([]pgsqlc.SearchMessagesRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.SearchMessagesParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.SearchMessages(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.SearchMessagesRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) SetBotACLDefaultEffect(ctx context.Context, arg pgsqlc.SetBotACLDefaultEffectParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.SetBotACLDefaultEffectParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.SetBotACLDefaultEffect(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) SetRouteActiveSession(ctx context.Context, arg pgsqlc.SetRouteActiveSessionParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.SetRouteActiveSessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.SetRouteActiveSession(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) SoftDeleteSession(ctx context.Context, id pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return err
	}
	err := q.store.queries.SoftDeleteSession(ctx, sqliteId)
	return mapQueryErr(err)
}

func (q *Queries) SoftDeleteSessionsByBot(ctx context.Context, botID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return err
	}
	err := q.store.queries.SoftDeleteSessionsByBot(ctx, sqliteBotID)
	return mapQueryErr(err)
}

func (q *Queries) TouchChat(ctx context.Context, chatID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteChatID string
	if err := convertValue(chatID, &sqliteChatID); err != nil {
		return err
	}
	err := q.store.queries.TouchChat(ctx, sqliteChatID)
	return mapQueryErr(err)
}

func (q *Queries) TouchSession(ctx context.Context, id pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return err
	}
	err := q.store.queries.TouchSession(ctx, sqliteId)
	return mapQueryErr(err)
}

func (q *Queries) UpdateAccountAdmin(ctx context.Context, arg pgsqlc.UpdateAccountAdminParams) (pgsqlc.User, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.User{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateAccountAdminParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.User{}, err
	}
	out, err := q.store.queries.UpdateAccountAdmin(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.User{}, mapQueryErr(err)
	}
	var result pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.User{}, err
	}
	return result, nil
}

func (q *Queries) UpdateAccountLastLogin(ctx context.Context, id pgtype.UUID) (pgsqlc.User, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.User{}, errSQLiteQueriesNotConfigured
	}
	var sqliteId string
	if err := convertValue(id, &sqliteId); err != nil {
		return pgsqlc.User{}, err
	}
	out, err := q.store.queries.UpdateAccountLastLogin(ctx, sqliteId)
	if err != nil {
		return pgsqlc.User{}, mapQueryErr(err)
	}
	var result pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.User{}, err
	}
	return result, nil
}

func (q *Queries) UpdateAccountPassword(ctx context.Context, arg pgsqlc.UpdateAccountPasswordParams) (pgsqlc.User, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.User{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateAccountPasswordParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.User{}, err
	}
	out, err := q.store.queries.UpdateAccountPassword(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.User{}, mapQueryErr(err)
	}
	var result pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.User{}, err
	}
	return result, nil
}

func (q *Queries) UpdateAccountProfile(ctx context.Context, arg pgsqlc.UpdateAccountProfileParams) (pgsqlc.User, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.User{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateAccountProfileParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.User{}, err
	}
	out, err := q.store.queries.UpdateAccountProfile(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.User{}, mapQueryErr(err)
	}
	var result pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.User{}, err
	}
	return result, nil
}

func (q *Queries) UpdateBotACLRule(ctx context.Context, arg pgsqlc.UpdateBotACLRuleParams) (pgsqlc.BotAclRule, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotAclRule{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateBotACLRuleParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotAclRule{}, err
	}
	out, err := q.store.queries.UpdateBotACLRule(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotAclRule{}, mapQueryErr(err)
	}
	var result pgsqlc.BotAclRule
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotAclRule{}, err
	}
	return result, nil
}

func (q *Queries) UpdateBotChannelConfigDisabled(ctx context.Context, arg pgsqlc.UpdateBotChannelConfigDisabledParams) (pgsqlc.BotChannelConfig, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotChannelConfig{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateBotChannelConfigDisabledParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotChannelConfig{}, err
	}
	out, err := q.store.queries.UpdateBotChannelConfigDisabled(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotChannelConfig{}, mapQueryErr(err)
	}
	var result pgsqlc.BotChannelConfig
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotChannelConfig{}, err
	}
	return result, nil
}

func (q *Queries) UpdateBotEmailBinding(ctx context.Context, arg pgsqlc.UpdateBotEmailBindingParams) (pgsqlc.BotEmailBinding, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotEmailBinding{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateBotEmailBindingParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotEmailBinding{}, err
	}
	out, err := q.store.queries.UpdateBotEmailBinding(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotEmailBinding{}, mapQueryErr(err)
	}
	var result pgsqlc.BotEmailBinding
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotEmailBinding{}, err
	}
	return result, nil
}

func (q *Queries) UpdateBotOwner(ctx context.Context, arg pgsqlc.UpdateBotOwnerParams) (pgsqlc.UpdateBotOwnerRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UpdateBotOwnerRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateBotOwnerParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UpdateBotOwnerRow{}, err
	}
	out, err := q.store.queries.UpdateBotOwner(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UpdateBotOwnerRow{}, mapQueryErr(err)
	}
	var result pgsqlc.UpdateBotOwnerRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UpdateBotOwnerRow{}, err
	}
	return result, nil
}

func (q *Queries) UpdateBotProfile(ctx context.Context, arg pgsqlc.UpdateBotProfileParams) (pgsqlc.UpdateBotProfileRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UpdateBotProfileRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateBotProfileParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UpdateBotProfileRow{}, err
	}
	out, err := q.store.queries.UpdateBotProfile(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UpdateBotProfileRow{}, mapQueryErr(err)
	}
	var result pgsqlc.UpdateBotProfileRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UpdateBotProfileRow{}, err
	}
	return result, nil
}

func (q *Queries) UpdateBotStatus(ctx context.Context, arg pgsqlc.UpdateBotStatusParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateBotStatusParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.UpdateBotStatus(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateChatRouteMetadata(ctx context.Context, arg pgsqlc.UpdateChatRouteMetadataParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateChatRouteMetadataParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.UpdateChatRouteMetadata(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateChatRouteReplyTarget(ctx context.Context, arg pgsqlc.UpdateChatRouteReplyTargetParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateChatRouteReplyTargetParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.UpdateChatRouteReplyTarget(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateChatTitle(ctx context.Context, arg pgsqlc.UpdateChatTitleParams) (pgsqlc.UpdateChatTitleRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UpdateChatTitleRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateChatTitleParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UpdateChatTitleRow{}, err
	}
	out, err := q.store.queries.UpdateChatTitle(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UpdateChatTitleRow{}, mapQueryErr(err)
	}
	var result pgsqlc.UpdateChatTitleRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UpdateChatTitleRow{}, err
	}
	return result, nil
}

func (q *Queries) UpdateContainerStarted(ctx context.Context, botID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return err
	}
	err := q.store.queries.UpdateContainerStarted(ctx, sqliteBotID)
	return mapQueryErr(err)
}

func (q *Queries) UpdateContainerStatus(ctx context.Context, arg pgsqlc.UpdateContainerStatusParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateContainerStatusParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.UpdateContainerStatus(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateContainerStopped(ctx context.Context, botID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return err
	}
	err := q.store.queries.UpdateContainerStopped(ctx, sqliteBotID)
	return mapQueryErr(err)
}

func (q *Queries) UpdateEmailOAuthState(ctx context.Context, arg pgsqlc.UpdateEmailOAuthStateParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateEmailOAuthStateParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.UpdateEmailOAuthState(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateEmailOutboxFailed(ctx context.Context, arg pgsqlc.UpdateEmailOutboxFailedParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateEmailOutboxFailedParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.UpdateEmailOutboxFailed(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateEmailOutboxSent(ctx context.Context, arg pgsqlc.UpdateEmailOutboxSentParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateEmailOutboxSentParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.UpdateEmailOutboxSent(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateEmailProvider(ctx context.Context, arg pgsqlc.UpdateEmailProviderParams) (pgsqlc.EmailProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.EmailProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateEmailProviderParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.EmailProvider{}, err
	}
	out, err := q.store.queries.UpdateEmailProvider(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.EmailProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.EmailProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.EmailProvider{}, err
	}
	return result, nil
}

func (q *Queries) UpdateEmailProviderByIDAndUser(ctx context.Context, arg pgsqlc.UpdateEmailProviderByIDAndUserParams) (pgsqlc.EmailProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.EmailProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateEmailProviderByIDAndUserParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.EmailProvider{}, err
	}
	out, err := q.store.queries.UpdateEmailProviderByIDAndUser(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.EmailProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.EmailProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.EmailProvider{}, err
	}
	return result, nil
}

func (q *Queries) UpdateMCPConnection(ctx context.Context, arg pgsqlc.UpdateMCPConnectionParams) (pgsqlc.McpConnection, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.McpConnection{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateMCPConnectionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.McpConnection{}, err
	}
	out, err := q.store.queries.UpdateMCPConnection(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.McpConnection{}, mapQueryErr(err)
	}
	var result pgsqlc.McpConnection
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.McpConnection{}, err
	}
	return result, nil
}

func (q *Queries) UpdateMCPConnectionAuthType(ctx context.Context, arg pgsqlc.UpdateMCPConnectionAuthTypeParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateMCPConnectionAuthTypeParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.UpdateMCPConnectionAuthType(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateMCPConnectionProbeResult(ctx context.Context, arg pgsqlc.UpdateMCPConnectionProbeResultParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateMCPConnectionProbeResultParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.UpdateMCPConnectionProbeResult(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateMCPOAuthClientSecret(ctx context.Context, arg pgsqlc.UpdateMCPOAuthClientSecretParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateMCPOAuthClientSecretParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.UpdateMCPOAuthClientSecret(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateMCPOAuthPKCEState(ctx context.Context, arg pgsqlc.UpdateMCPOAuthPKCEStateParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateMCPOAuthPKCEStateParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.UpdateMCPOAuthPKCEState(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateMCPOAuthTokens(ctx context.Context, arg pgsqlc.UpdateMCPOAuthTokensParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateMCPOAuthTokensParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.UpdateMCPOAuthTokens(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateMemoryProvider(ctx context.Context, arg pgsqlc.UpdateMemoryProviderParams) (pgsqlc.MemoryProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.MemoryProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateMemoryProviderParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.MemoryProvider{}, err
	}
	out, err := q.store.queries.UpdateMemoryProvider(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.MemoryProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.MemoryProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.MemoryProvider{}, err
	}
	return result, nil
}

func (q *Queries) UpdateModel(ctx context.Context, arg pgsqlc.UpdateModelParams) (pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Model{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateModelParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.Model{}, err
	}
	out, err := q.store.queries.UpdateModel(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.Model{}, mapQueryErr(err)
	}
	var result pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Model{}, err
	}
	return result, nil
}

func (q *Queries) UpdateProvider(ctx context.Context, arg pgsqlc.UpdateProviderParams) (pgsqlc.Provider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Provider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateProviderParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.Provider{}, err
	}
	out, err := q.store.queries.UpdateProvider(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.Provider{}, mapQueryErr(err)
	}
	var result pgsqlc.Provider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Provider{}, err
	}
	return result, nil
}

func (q *Queries) UpdateProviderOAuthState(ctx context.Context, arg pgsqlc.UpdateProviderOAuthStateParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateProviderOAuthStateParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.UpdateProviderOAuthState(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateSchedule(ctx context.Context, arg pgsqlc.UpdateScheduleParams) (pgsqlc.Schedule, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Schedule{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateScheduleParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.Schedule{}, err
	}
	out, err := q.store.queries.UpdateSchedule(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.Schedule{}, mapQueryErr(err)
	}
	var result pgsqlc.Schedule
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Schedule{}, err
	}
	return result, nil
}

func (q *Queries) UpdateFetchProvider(ctx context.Context, arg pgsqlc.UpdateFetchProviderParams) (pgsqlc.FetchProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.FetchProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateFetchProviderParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.FetchProvider{}, err
	}
	out, err := q.store.queries.UpdateFetchProvider(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.FetchProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.FetchProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.FetchProvider{}, err
	}
	return result, nil
}

func (q *Queries) UpdateSearchProvider(ctx context.Context, arg pgsqlc.UpdateSearchProviderParams) (pgsqlc.SearchProvider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.SearchProvider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateSearchProviderParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.SearchProvider{}, err
	}
	out, err := q.store.queries.UpdateSearchProvider(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.SearchProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.SearchProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.SearchProvider{}, err
	}
	return result, nil
}

func (q *Queries) UpdateSessionMetadata(ctx context.Context, arg pgsqlc.UpdateSessionMetadataParams) (pgsqlc.BotSession, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotSession{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateSessionMetadataParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotSession{}, err
	}
	out, err := q.store.queries.UpdateSessionMetadata(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotSession{}, mapQueryErr(err)
	}
	var result pgsqlc.BotSession
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotSession{}, err
	}
	return result, nil
}

func (q *Queries) UpdateSessionTypeAndMetadata(ctx context.Context, arg pgsqlc.UpdateSessionTypeAndMetadataParams) (pgsqlc.BotSession, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotSession{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateSessionTypeAndMetadataParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotSession{}, err
	}
	out, err := q.store.queries.UpdateSessionTypeAndMetadata(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotSession{}, mapQueryErr(err)
	}
	var result pgsqlc.BotSession
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotSession{}, err
	}
	return result, nil
}

func (q *Queries) UpdateSessionTitle(ctx context.Context, arg pgsqlc.UpdateSessionTitleParams) (pgsqlc.BotSession, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotSession{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateSessionTitleParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotSession{}, err
	}
	out, err := q.store.queries.UpdateSessionTitle(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotSession{}, mapQueryErr(err)
	}
	var result pgsqlc.BotSession
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotSession{}, err
	}
	return result, nil
}

func (q *Queries) UpdateToolApprovalPromptMessage(ctx context.Context, arg pgsqlc.UpdateToolApprovalPromptMessageParams) (pgsqlc.ToolApprovalRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ToolApprovalRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateToolApprovalPromptMessageParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	out, err := q.store.queries.UpdateToolApprovalPromptMessage(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ToolApprovalRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.ToolApprovalRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	return result, nil
}

func (q *Queries) UpdateUserProviderOAuthState(ctx context.Context, arg pgsqlc.UpdateUserProviderOAuthStateParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateUserProviderOAuthStateParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.UpdateUserProviderOAuthState(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) UpsertAccountByUsername(ctx context.Context, arg pgsqlc.UpsertAccountByUsernameParams) (pgsqlc.User, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.User{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertAccountByUsernameParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.User{}, err
	}
	out, err := q.store.queries.UpsertAccountByUsername(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.User{}, mapQueryErr(err)
	}
	var result pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.User{}, err
	}
	return result, nil
}

func (q *Queries) UpsertBotChannelConfig(ctx context.Context, arg pgsqlc.UpsertBotChannelConfigParams) (pgsqlc.BotChannelConfig, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotChannelConfig{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertBotChannelConfigParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotChannelConfig{}, err
	}
	out, err := q.store.queries.UpsertBotChannelConfig(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotChannelConfig{}, mapQueryErr(err)
	}
	var result pgsqlc.BotChannelConfig
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotChannelConfig{}, err
	}
	return result, nil
}

func (q *Queries) UpsertBotSettings(ctx context.Context, arg pgsqlc.UpsertBotSettingsParams) (pgsqlc.UpsertBotSettingsRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UpsertBotSettingsRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertBotSettingsParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UpsertBotSettingsRow{}, err
	}
	out, err := q.store.queries.UpsertBotSettings(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UpsertBotSettingsRow{}, mapQueryErr(err)
	}
	var result pgsqlc.UpsertBotSettingsRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UpsertBotSettingsRow{}, err
	}
	return result, nil
}

func (q *Queries) UpsertBotStorageBinding(ctx context.Context, arg pgsqlc.UpsertBotStorageBindingParams) (pgsqlc.BotStorageBinding, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotStorageBinding{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertBotStorageBindingParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotStorageBinding{}, err
	}
	out, err := q.store.queries.UpsertBotStorageBinding(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotStorageBinding{}, mapQueryErr(err)
	}
	var result pgsqlc.BotStorageBinding
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotStorageBinding{}, err
	}
	return result, nil
}

func (q *Queries) UpsertChannelIdentityByChannelSubject(ctx context.Context, arg pgsqlc.UpsertChannelIdentityByChannelSubjectParams) (pgsqlc.ChannelIdentity, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ChannelIdentity{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertChannelIdentityByChannelSubjectParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ChannelIdentity{}, err
	}
	out, err := q.store.queries.UpsertChannelIdentityByChannelSubject(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ChannelIdentity{}, mapQueryErr(err)
	}
	var result pgsqlc.ChannelIdentity
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ChannelIdentity{}, err
	}
	return result, nil
}

func (q *Queries) UpsertChatSettings(ctx context.Context, arg pgsqlc.UpsertChatSettingsParams) (pgsqlc.UpsertChatSettingsRow, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UpsertChatSettingsRow{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertChatSettingsParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UpsertChatSettingsRow{}, err
	}
	out, err := q.store.queries.UpsertChatSettings(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UpsertChatSettingsRow{}, mapQueryErr(err)
	}
	var result pgsqlc.UpsertChatSettingsRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UpsertChatSettingsRow{}, err
	}
	return result, nil
}

func (q *Queries) UpsertContainer(ctx context.Context, arg pgsqlc.UpsertContainerParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertContainerParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	err := q.store.queries.UpsertContainer(ctx, sqliteArg)
	return mapQueryErr(err)
}

func (q *Queries) UpsertBotWorkspaceResourceLimits(ctx context.Context, arg pgsqlc.UpsertBotWorkspaceResourceLimitsParams) (pgsqlc.BotWorkspaceResourceLimit, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotWorkspaceResourceLimit{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertBotWorkspaceResourceLimitsParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotWorkspaceResourceLimit{}, err
	}
	out, err := q.store.queries.UpsertBotWorkspaceResourceLimits(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotWorkspaceResourceLimit{}, mapQueryErr(err)
	}
	var result pgsqlc.BotWorkspaceResourceLimit
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotWorkspaceResourceLimit{}, err
	}
	return result, nil
}

func (q *Queries) UpsertEmailOAuthToken(ctx context.Context, arg pgsqlc.UpsertEmailOAuthTokenParams) (pgsqlc.EmailOauthToken, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.EmailOauthToken{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertEmailOAuthTokenParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.EmailOauthToken{}, err
	}
	out, err := q.store.queries.UpsertEmailOAuthToken(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.EmailOauthToken{}, mapQueryErr(err)
	}
	var result pgsqlc.EmailOauthToken
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.EmailOauthToken{}, err
	}
	return result, nil
}

func (q *Queries) UpsertMCPConnectionByName(ctx context.Context, arg pgsqlc.UpsertMCPConnectionByNameParams) (pgsqlc.McpConnection, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.McpConnection{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertMCPConnectionByNameParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.McpConnection{}, err
	}
	out, err := q.store.queries.UpsertMCPConnectionByName(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.McpConnection{}, mapQueryErr(err)
	}
	var result pgsqlc.McpConnection
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.McpConnection{}, err
	}
	return result, nil
}

func (q *Queries) UpsertMCPOAuthDiscovery(ctx context.Context, arg pgsqlc.UpsertMCPOAuthDiscoveryParams) (pgsqlc.McpOauthToken, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.McpOauthToken{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertMCPOAuthDiscoveryParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.McpOauthToken{}, err
	}
	out, err := q.store.queries.UpsertMCPOAuthDiscovery(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.McpOauthToken{}, mapQueryErr(err)
	}
	var result pgsqlc.McpOauthToken
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.McpOauthToken{}, err
	}
	return result, nil
}

func (q *Queries) UpsertProviderOAuthToken(ctx context.Context, arg pgsqlc.UpsertProviderOAuthTokenParams) (pgsqlc.ProviderOauthToken, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.ProviderOauthToken{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertProviderOAuthTokenParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.ProviderOauthToken{}, err
	}
	out, err := q.store.queries.UpsertProviderOAuthToken(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.ProviderOauthToken{}, mapQueryErr(err)
	}
	var result pgsqlc.ProviderOauthToken
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ProviderOauthToken{}, err
	}
	return result, nil
}

func (q *Queries) UpsertRegistryModel(ctx context.Context, arg pgsqlc.UpsertRegistryModelParams) (pgsqlc.Model, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Model{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertRegistryModelParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.Model{}, err
	}
	out, err := q.store.queries.UpsertRegistryModel(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.Model{}, mapQueryErr(err)
	}
	var result pgsqlc.Model
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Model{}, err
	}
	return result, nil
}

func (q *Queries) UpsertRegistryProvider(ctx context.Context, arg pgsqlc.UpsertRegistryProviderParams) (pgsqlc.Provider, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Provider{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertRegistryProviderParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.Provider{}, err
	}
	out, err := q.store.queries.UpsertRegistryProvider(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.Provider{}, mapQueryErr(err)
	}
	var result pgsqlc.Provider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Provider{}, err
	}
	return result, nil
}

func (q *Queries) UpsertSnapshot(ctx context.Context, arg pgsqlc.UpsertSnapshotParams) (pgsqlc.Snapshot, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.Snapshot{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertSnapshotParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.Snapshot{}, err
	}
	out, err := q.store.queries.UpsertSnapshot(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.Snapshot{}, mapQueryErr(err)
	}
	var result pgsqlc.Snapshot
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Snapshot{}, err
	}
	return result, nil
}

func (q *Queries) UpsertUserChannelBinding(ctx context.Context, arg pgsqlc.UpsertUserChannelBindingParams) (pgsqlc.UserChannelBinding, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserChannelBinding{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertUserChannelBindingParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserChannelBinding{}, err
	}
	out, err := q.store.queries.UpsertUserChannelBinding(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserChannelBinding{}, mapQueryErr(err)
	}
	var result pgsqlc.UserChannelBinding
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserChannelBinding{}, err
	}
	return result, nil
}

func (q *Queries) UpsertUserProviderOAuthToken(ctx context.Context, arg pgsqlc.UpsertUserProviderOAuthTokenParams) (pgsqlc.UserProviderOauthToken, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserProviderOauthToken{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertUserProviderOAuthTokenParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserProviderOauthToken{}, err
	}
	out, err := q.store.queries.UpsertUserProviderOAuthToken(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserProviderOauthToken{}, mapQueryErr(err)
	}
	var result pgsqlc.UserProviderOauthToken
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserProviderOauthToken{}, err
	}
	return result, nil
}

func (q *Queries) CancelUserInputRequest(ctx context.Context, arg pgsqlc.CancelUserInputRequestParams) (pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserInputRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CancelUserInputRequestParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	out, err := q.store.queries.CancelUserInputRequest(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserInputRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	return result, nil
}

func (q *Queries) CreateUserInputRequest(ctx context.Context, arg pgsqlc.CreateUserInputRequestParams) (pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserInputRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateUserInputRequestParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	out, err := q.store.queries.CreateUserInputRequest(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserInputRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	return result, nil
}

func (q *Queries) FailUserInputRequest(ctx context.Context, arg pgsqlc.FailUserInputRequestParams) (pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserInputRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.FailUserInputRequestParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	out, err := q.store.queries.FailUserInputRequest(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserInputRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	return result, nil
}

func (q *Queries) GetLatestPendingUserInputBySession(ctx context.Context, arg pgsqlc.GetLatestPendingUserInputBySessionParams) (pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserInputRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetLatestPendingUserInputBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	out, err := q.store.queries.GetLatestPendingUserInputBySession(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserInputRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	return result, nil
}

func (q *Queries) GetPendingUserInputByReplyMessage(ctx context.Context, arg pgsqlc.GetPendingUserInputByReplyMessageParams) (pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserInputRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetPendingUserInputByReplyMessageParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	out, err := q.store.queries.GetPendingUserInputByReplyMessage(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserInputRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	return result, nil
}

func (q *Queries) GetPendingUserInputBySessionShortID(ctx context.Context, arg pgsqlc.GetPendingUserInputBySessionShortIDParams) (pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserInputRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetPendingUserInputBySessionShortIDParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	out, err := q.store.queries.GetPendingUserInputBySessionShortID(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserInputRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	return result, nil
}

func (q *Queries) GetUserInputRequest(ctx context.Context, id pgtype.UUID) (pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserInputRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteID string
	if err := convertValue(id, &sqliteID); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	out, err := q.store.queries.GetUserInputRequest(ctx, sqliteID)
	if err != nil {
		return pgsqlc.UserInputRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	return result, nil
}

func (q *Queries) GetUserInputRequestBySessionToolCall(ctx context.Context, arg pgsqlc.GetUserInputRequestBySessionToolCallParams) (pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserInputRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetUserInputRequestBySessionToolCallParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	out, err := q.store.queries.GetUserInputRequestBySessionToolCall(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserInputRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	return result, nil
}

func (q *Queries) ListPendingUserInputsBySession(ctx context.Context, arg pgsqlc.ListPendingUserInputsBySessionParams) ([]pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListPendingUserInputsBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListPendingUserInputsBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListUserInputsBySession(ctx context.Context, arg pgsqlc.ListUserInputsBySessionParams) ([]pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListUserInputsBySessionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListUserInputsBySession(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListUserInputsBySessionToolCalls(ctx context.Context, arg pgsqlc.ListUserInputsBySessionToolCallsParams) ([]pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.ListUserInputsBySessionToolCallsParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListUserInputsBySessionToolCalls(ctx, sqliteArg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) SubmitUserInputRequest(ctx context.Context, arg pgsqlc.SubmitUserInputRequestParams) (pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserInputRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.SubmitUserInputRequestParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	out, err := q.store.queries.SubmitUserInputRequest(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserInputRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	return result, nil
}

func (q *Queries) UpdateUserInputAssistantMessage(ctx context.Context, arg pgsqlc.UpdateUserInputAssistantMessageParams) (pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserInputRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateUserInputAssistantMessageParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	out, err := q.store.queries.UpdateUserInputAssistantMessage(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserInputRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	return result, nil
}

func (q *Queries) UpdateUserInputPromptMessage(ctx context.Context, arg pgsqlc.UpdateUserInputPromptMessageParams) (pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserInputRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateUserInputPromptMessageParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	out, err := q.store.queries.UpdateUserInputPromptMessage(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserInputRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	return result, nil
}

func (q *Queries) UpdateUserInputToolResultMessage(ctx context.Context, arg pgsqlc.UpdateUserInputToolResultMessageParams) (pgsqlc.UserInputRequest, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.UserInputRequest{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateUserInputToolResultMessageParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	out, err := q.store.queries.UpdateUserInputToolResultMessage(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.UserInputRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.UserInputRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserInputRequest{}, err
	}
	return result, nil
}

func (q *Queries) CreateBotPluginInstallation(ctx context.Context, arg pgsqlc.CreateBotPluginInstallationParams) (pgsqlc.BotPluginInstallation, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotPluginInstallation{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateBotPluginInstallationParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotPluginInstallation{}, err
	}
	out, err := q.store.queries.CreateBotPluginInstallation(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotPluginInstallation{}, mapQueryErr(err)
	}
	var result pgsqlc.BotPluginInstallation
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotPluginInstallation{}, err
	}
	return result, nil
}

func (q *Queries) GetBotPluginInstallationByID(ctx context.Context, arg pgsqlc.GetBotPluginInstallationByIDParams) (pgsqlc.BotPluginInstallation, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotPluginInstallation{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetBotPluginInstallationByIDParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotPluginInstallation{}, err
	}
	out, err := q.store.queries.GetBotPluginInstallationByID(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotPluginInstallation{}, mapQueryErr(err)
	}
	var result pgsqlc.BotPluginInstallation
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotPluginInstallation{}, err
	}
	return result, nil
}

func (q *Queries) ListBotPluginInstallations(ctx context.Context, botID pgtype.UUID) ([]pgsqlc.BotPluginInstallation, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListBotPluginInstallations(ctx, sqliteBotID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.BotPluginInstallation
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) UpdateBotPluginInstallationStatus(ctx context.Context, arg pgsqlc.UpdateBotPluginInstallationStatusParams) (pgsqlc.BotPluginInstallation, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotPluginInstallation{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateBotPluginInstallationStatusParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotPluginInstallation{}, err
	}
	out, err := q.store.queries.UpdateBotPluginInstallationStatus(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotPluginInstallation{}, mapQueryErr(err)
	}
	var result pgsqlc.BotPluginInstallation
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotPluginInstallation{}, err
	}
	return result, nil
}

func (q *Queries) DeleteBotPluginInstallation(ctx context.Context, arg pgsqlc.DeleteBotPluginInstallationParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.DeleteBotPluginInstallationParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.DeleteBotPluginInstallation(ctx, sqliteArg))
}

func (q *Queries) UpsertBotPluginResource(ctx context.Context, arg pgsqlc.UpsertBotPluginResourceParams) (pgsqlc.BotPluginResource, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotPluginResource{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertBotPluginResourceParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotPluginResource{}, err
	}
	out, err := q.store.queries.UpsertBotPluginResource(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotPluginResource{}, mapQueryErr(err)
	}
	var result pgsqlc.BotPluginResource
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotPluginResource{}, err
	}
	return result, nil
}

func (q *Queries) ListBotPluginResources(ctx context.Context, installationID pgtype.UUID) ([]pgsqlc.BotPluginResource, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteInstallationID string
	if err := convertValue(installationID, &sqliteInstallationID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListBotPluginResources(ctx, sqliteInstallationID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.BotPluginResource
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) DeleteBotPluginResources(ctx context.Context, installationID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteInstallationID string
	if err := convertValue(installationID, &sqliteInstallationID); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.DeleteBotPluginResources(ctx, sqliteInstallationID))
}

func (q *Queries) CreateManagedMCPConnection(ctx context.Context, arg pgsqlc.CreateManagedMCPConnectionParams) (pgsqlc.McpConnection, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.McpConnection{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.CreateManagedMCPConnectionParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.McpConnection{}, err
	}
	out, err := q.store.queries.CreateManagedMCPConnection(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.McpConnection{}, mapQueryErr(err)
	}
	var result pgsqlc.McpConnection
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.McpConnection{}, err
	}
	return result, nil
}

func (q *Queries) UpdateMCPConnectionActive(ctx context.Context, arg pgsqlc.UpdateMCPConnectionActiveParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateMCPConnectionActiveParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.UpdateMCPConnectionActive(ctx, sqliteArg))
}

func (q *Queries) UpdateMCPConnectionsActiveByPlugin(ctx context.Context, arg pgsqlc.UpdateMCPConnectionsActiveByPluginParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpdateMCPConnectionsActiveByPluginParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.UpdateMCPConnectionsActiveByPlugin(ctx, sqliteArg))
}

func (q *Queries) DeleteMCPConnectionsByPlugin(ctx context.Context, arg pgsqlc.DeleteMCPConnectionsByPluginParams) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.DeleteMCPConnectionsByPluginParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return err
	}
	return mapQueryErr(q.store.queries.DeleteMCPConnectionsByPlugin(ctx, sqliteArg))
}

func (q *Queries) GetSessionDiscussCursor(ctx context.Context, arg pgsqlc.GetSessionDiscussCursorParams) (pgsqlc.BotSessionDiscussCursor, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotSessionDiscussCursor{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.GetSessionDiscussCursorParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotSessionDiscussCursor{}, err
	}
	out, err := q.store.queries.GetSessionDiscussCursor(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotSessionDiscussCursor{}, mapQueryErr(err)
	}
	var result pgsqlc.BotSessionDiscussCursor
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotSessionDiscussCursor{}, err
	}
	return result, nil
}

func (q *Queries) ListSessionDiscussCursorsByBot(ctx context.Context, botID pgtype.UUID) ([]pgsqlc.BotSessionDiscussCursor, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return nil, err
	}
	out, err := q.store.queries.ListSessionDiscussCursorsByBot(ctx, sqliteBotID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.BotSessionDiscussCursor
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) DeleteSessionDiscussCursorsByBot(ctx context.Context, botID pgtype.UUID) error {
	if q == nil || q.store == nil || q.store.queries == nil {
		return errSQLiteQueriesNotConfigured
	}
	var sqliteBotID string
	if err := convertValue(botID, &sqliteBotID); err != nil {
		return err
	}
	err := q.store.queries.DeleteSessionDiscussCursorsByBot(ctx, sqliteBotID)
	return mapQueryErr(err)
}

func (q *Queries) UpsertSessionDiscussCursor(ctx context.Context, arg pgsqlc.UpsertSessionDiscussCursorParams) (pgsqlc.BotSessionDiscussCursor, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return pgsqlc.BotSessionDiscussCursor{}, errSQLiteQueriesNotConfigured
	}
	var sqliteArg sqlitesqlc.UpsertSessionDiscussCursorParams
	if err := convertValue(arg, &sqliteArg); err != nil {
		return pgsqlc.BotSessionDiscussCursor{}, err
	}
	out, err := q.store.queries.UpsertSessionDiscussCursor(ctx, sqliteArg)
	if err != nil {
		return pgsqlc.BotSessionDiscussCursor{}, mapQueryErr(err)
	}
	var result pgsqlc.BotSessionDiscussCursor
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotSessionDiscussCursor{}, err
	}
	return result, nil
}
