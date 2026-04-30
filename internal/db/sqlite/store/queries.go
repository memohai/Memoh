package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

type Queries struct {
	store *Store
}

func NewQueries(store *Store) *Queries {
	return &Queries{store: store}
}

func (q *Queries) WithTx(_ pgx.Tx) dbstore.Queries {
	return q
}

func (q *Queries) call(ctx context.Context, name string, args ...any) (any, error) {
	if q == nil || q.store == nil || q.store.queries == nil {
		return nil, errors.New("sqlite queries not configured")
	}
	method := reflect.ValueOf(q.store.queries).MethodByName(name)
	if !method.IsValid() {
		return nil, fmt.Errorf("sqlite query %s not implemented", name)
	}
	methodType := method.Type()
	if methodType.NumIn() != len(args)+1 {
		return nil, fmt.Errorf("sqlite query %s expects %d args, got %d", name, methodType.NumIn()-1, len(args))
	}
	inputs := make([]reflect.Value, 0, methodType.NumIn())
	inputs = append(inputs, reflect.ValueOf(ctx))
	for i, arg := range args {
		dst := reflect.New(methodType.In(i + 1)).Elem()
		if err := convertValue(arg, dst.Addr().Interface()); err != nil {
			return nil, fmt.Errorf("%s arg %d: %w", name, i+1, err)
		}
		inputs = append(inputs, dst)
	}
	outputs := method.Call(inputs)
	if len(outputs) == 0 {
		return nil, nil
	}
	last := outputs[len(outputs)-1]
	if !last.IsNil() {
		err, _ := last.Interface().(error)
		return nil, err
	}
	if len(outputs) == 1 {
		return nil, nil
	}
	return outputs[0].Interface(), nil
}

func (q *Queries) ApproveToolApprovalRequest(ctx context.Context, arg pgsqlc.ApproveToolApprovalRequestParams) (pgsqlc.ToolApprovalRequest, error) {
	out, err := q.call(ctx, "ApproveToolApprovalRequest", arg)
	if err != nil {
		return pgsqlc.ToolApprovalRequest{}, mapQueryErr(err)
	}
	var result pgsqlc.ToolApprovalRequest
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ToolApprovalRequest{}, err
	}
	return result, nil
}

func (q *Queries) ClearMCPOAuthTokens(ctx context.Context, connectionID pgtype.UUID) error {
	_, err := q.call(ctx, "ClearMCPOAuthTokens", connectionID)
	return mapQueryErr(err)
}

func (q *Queries) CompleteCompactionLog(ctx context.Context, arg pgsqlc.CompleteCompactionLogParams) (pgsqlc.BotHistoryMessageCompact, error) {
	out, err := q.call(ctx, "CompleteCompactionLog", arg)
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
	out, err := q.call(ctx, "CompleteHeartbeatLog", arg)
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
	out, err := q.call(ctx, "CompleteScheduleLog", arg)
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
	out, err := q.call(ctx, "CountAccounts")
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
	out, err := q.call(ctx, "CountCompactionLogsByBot", botID)
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
	out, err := q.call(ctx, "CountEmailOutboxByBot", botID)
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
	out, err := q.call(ctx, "CountHeartbeatLogsByBot", botID)
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
	out, err := q.call(ctx, "CountMemoryProvidersByDefault")
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
	out, err := q.call(ctx, "CountMessagesBySession", sessionID)
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
	out, err := q.call(ctx, "CountModels")
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
	out, err := q.call(ctx, "CountModelsByType", type_)
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
	out, err := q.call(ctx, "CountProviders")
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
	out, err := q.call(ctx, "CountScheduleLogsByBot", botID)
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
	out, err := q.call(ctx, "CountScheduleLogsBySchedule", scheduleID)
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
	out, err := q.call(ctx, "CountSessionEvents", sessionID)
	if err != nil {
		return 0, mapQueryErr(err)
	}
	var result int64
	if err := convertValue(out, &result); err != nil {
		return 0, err
	}
	return result, nil
}

func (q *Queries) CountTokenUsageRecords(ctx context.Context, arg pgsqlc.CountTokenUsageRecordsParams) (int64, error) {
	out, err := q.call(ctx, "CountTokenUsageRecords", arg)
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
	out, err := q.call(ctx, "CreateAccount", arg)
	if err != nil {
		return pgsqlc.User{}, mapQueryErr(err)
	}
	var result pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.User{}, err
	}
	return result, nil
}

func (q *Queries) CreateBindCode(ctx context.Context, arg pgsqlc.CreateBindCodeParams) (pgsqlc.ChannelIdentityBindCode, error) {
	out, err := q.call(ctx, "CreateBindCode", arg)
	if err != nil {
		return pgsqlc.ChannelIdentityBindCode{}, mapQueryErr(err)
	}
	var result pgsqlc.ChannelIdentityBindCode
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ChannelIdentityBindCode{}, err
	}
	return result, nil
}

func (q *Queries) CreateBot(ctx context.Context, arg pgsqlc.CreateBotParams) (pgsqlc.CreateBotRow, error) {
	out, err := q.call(ctx, "CreateBot", arg)
	if err != nil {
		return pgsqlc.CreateBotRow{}, mapQueryErr(err)
	}
	var result pgsqlc.CreateBotRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.CreateBotRow{}, err
	}
	return result, nil
}

func (q *Queries) CreateBotACLRule(ctx context.Context, arg pgsqlc.CreateBotACLRuleParams) (pgsqlc.CreateBotACLRuleRow, error) {
	out, err := q.call(ctx, "CreateBotACLRule", arg)
	if err != nil {
		return pgsqlc.CreateBotACLRuleRow{}, mapQueryErr(err)
	}
	var result pgsqlc.CreateBotACLRuleRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.CreateBotACLRuleRow{}, err
	}
	return result, nil
}

func (q *Queries) CreateBotEmailBinding(ctx context.Context, arg pgsqlc.CreateBotEmailBindingParams) (pgsqlc.BotEmailBinding, error) {
	out, err := q.call(ctx, "CreateBotEmailBinding", arg)
	if err != nil {
		return pgsqlc.BotEmailBinding{}, mapQueryErr(err)
	}
	var result pgsqlc.BotEmailBinding
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotEmailBinding{}, err
	}
	return result, nil
}

func (q *Queries) CreateBrowserContext(ctx context.Context, arg pgsqlc.CreateBrowserContextParams) (pgsqlc.BrowserContext, error) {
	out, err := q.call(ctx, "CreateBrowserContext", arg)
	if err != nil {
		return pgsqlc.BrowserContext{}, mapQueryErr(err)
	}
	var result pgsqlc.BrowserContext
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BrowserContext{}, err
	}
	return result, nil
}

func (q *Queries) CreateChannelIdentity(ctx context.Context, arg pgsqlc.CreateChannelIdentityParams) (pgsqlc.ChannelIdentity, error) {
	out, err := q.call(ctx, "CreateChannelIdentity", arg)
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
	out, err := q.call(ctx, "CreateChat", arg)
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
	out, err := q.call(ctx, "CreateChatRoute", arg)
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
	out, err := q.call(ctx, "CreateCompactionLog", arg)
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
	out, err := q.call(ctx, "CreateEmailOutbox", arg)
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
	out, err := q.call(ctx, "CreateEmailProvider", arg)
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
	out, err := q.call(ctx, "CreateHeartbeatLog", arg)
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
	out, err := q.call(ctx, "CreateMCPConnection", arg)
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
	out, err := q.call(ctx, "CreateMemoryProvider", arg)
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
	out, err := q.call(ctx, "CreateMessage", arg)
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
	out, err := q.call(ctx, "CreateMessageAsset", arg)
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
	out, err := q.call(ctx, "CreateModel", arg)
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
	out, err := q.call(ctx, "CreateModelVariant", arg)
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
	out, err := q.call(ctx, "CreateProvider", arg)
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
	out, err := q.call(ctx, "CreateSchedule", arg)
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
	out, err := q.call(ctx, "CreateScheduleLog", arg)
	if err != nil {
		return pgsqlc.CreateScheduleLogRow{}, mapQueryErr(err)
	}
	var result pgsqlc.CreateScheduleLogRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.CreateScheduleLogRow{}, err
	}
	return result, nil
}

func (q *Queries) CreateSearchProvider(ctx context.Context, arg pgsqlc.CreateSearchProviderParams) (pgsqlc.SearchProvider, error) {
	out, err := q.call(ctx, "CreateSearchProvider", arg)
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
	out, err := q.call(ctx, "CreateSession", arg)
	if err != nil {
		return pgsqlc.BotSession{}, mapQueryErr(err)
	}
	var result pgsqlc.BotSession
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotSession{}, err
	}
	return result, nil
}

func (q *Queries) CreateSessionEvent(ctx context.Context, arg pgsqlc.CreateSessionEventParams) (pgtype.UUID, error) {
	out, err := q.call(ctx, "CreateSessionEvent", arg)
	if err != nil {
		return pgtype.UUID{}, mapQueryErr(err)
	}
	var result pgtype.UUID
	if err := convertValue(out, &result); err != nil {
		return pgtype.UUID{}, err
	}
	return result, nil
}

func (q *Queries) CreateStorageProvider(ctx context.Context, arg pgsqlc.CreateStorageProviderParams) (pgsqlc.StorageProvider, error) {
	out, err := q.call(ctx, "CreateStorageProvider", arg)
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
	out, err := q.call(ctx, "CreateToolApprovalRequest", arg)
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
	out, err := q.call(ctx, "CreateUser", arg)
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
	_, err := q.call(ctx, "DeleteBotACLRuleByID", id)
	return mapQueryErr(err)
}

func (q *Queries) DeleteBotByID(ctx context.Context, id pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteBotByID", id)
	return mapQueryErr(err)
}

func (q *Queries) DeleteBotChannelConfig(ctx context.Context, arg pgsqlc.DeleteBotChannelConfigParams) error {
	_, err := q.call(ctx, "DeleteBotChannelConfig", arg)
	return mapQueryErr(err)
}

func (q *Queries) DeleteBotEmailBinding(ctx context.Context, id pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteBotEmailBinding", id)
	return mapQueryErr(err)
}

func (q *Queries) DeleteBrowserContext(ctx context.Context, id pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteBrowserContext", id)
	return mapQueryErr(err)
}

func (q *Queries) DeleteChat(ctx context.Context, chatID pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteChat", chatID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteChatRoute(ctx context.Context, id pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteChatRoute", id)
	return mapQueryErr(err)
}

func (q *Queries) DeleteCompactionLogsByBot(ctx context.Context, botID pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteCompactionLogsByBot", botID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteContainerByBotID(ctx context.Context, botID pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteContainerByBotID", botID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteEmailOAuthToken(ctx context.Context, emailProviderID pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteEmailOAuthToken", emailProviderID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteEmailProvider(ctx context.Context, id pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteEmailProvider", id)
	return mapQueryErr(err)
}

func (q *Queries) DeleteHeartbeatLogsByBot(ctx context.Context, botID pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteHeartbeatLogsByBot", botID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteMCPConnection(ctx context.Context, arg pgsqlc.DeleteMCPConnectionParams) error {
	_, err := q.call(ctx, "DeleteMCPConnection", arg)
	return mapQueryErr(err)
}

func (q *Queries) DeleteMCPOAuthToken(ctx context.Context, connectionID pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteMCPOAuthToken", connectionID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteMemoryProvider(ctx context.Context, id pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteMemoryProvider", id)
	return mapQueryErr(err)
}

func (q *Queries) DeleteMessageAssets(ctx context.Context, messageID pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteMessageAssets", messageID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteMessagesByBot(ctx context.Context, botID pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteMessagesByBot", botID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteMessagesBySession(ctx context.Context, sessionID pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteMessagesBySession", sessionID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteModel(ctx context.Context, id pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteModel", id)
	return mapQueryErr(err)
}

func (q *Queries) DeleteModelByModelID(ctx context.Context, modelID string) error {
	_, err := q.call(ctx, "DeleteModelByModelID", modelID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteModelByProviderAndType(ctx context.Context, arg pgsqlc.DeleteModelByProviderAndTypeParams) error {
	_, err := q.call(ctx, "DeleteModelByProviderAndType", arg)
	return mapQueryErr(err)
}

func (q *Queries) DeleteModelByProviderIDAndModelID(ctx context.Context, arg pgsqlc.DeleteModelByProviderIDAndModelIDParams) error {
	_, err := q.call(ctx, "DeleteModelByProviderIDAndModelID", arg)
	return mapQueryErr(err)
}

func (q *Queries) DeleteProvider(ctx context.Context, id pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteProvider", id)
	return mapQueryErr(err)
}

func (q *Queries) DeleteProviderOAuthToken(ctx context.Context, providerID pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteProviderOAuthToken", providerID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteSchedule(ctx context.Context, id pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteSchedule", id)
	return mapQueryErr(err)
}

func (q *Queries) DeleteScheduleLogsByBot(ctx context.Context, botID pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteScheduleLogsByBot", botID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteScheduleLogsBySchedule(ctx context.Context, scheduleID pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteScheduleLogsBySchedule", scheduleID)
	return mapQueryErr(err)
}

func (q *Queries) DeleteSearchProvider(ctx context.Context, id pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteSearchProvider", id)
	return mapQueryErr(err)
}

func (q *Queries) DeleteSettingsByBotID(ctx context.Context, id pgtype.UUID) error {
	_, err := q.call(ctx, "DeleteSettingsByBotID", id)
	return mapQueryErr(err)
}

func (q *Queries) DeleteUserProviderOAuthToken(ctx context.Context, arg pgsqlc.DeleteUserProviderOAuthTokenParams) error {
	_, err := q.call(ctx, "DeleteUserProviderOAuthToken", arg)
	return mapQueryErr(err)
}

func (q *Queries) EvaluateBotACLRule(ctx context.Context, arg pgsqlc.EvaluateBotACLRuleParams) (string, error) {
	out, err := q.call(ctx, "EvaluateBotACLRule", arg)
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
	out, err := q.call(ctx, "FindChatRoute", arg)
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
	out, err := q.call(ctx, "GetAccountByIdentity", identity)
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
	out, err := q.call(ctx, "GetAccountByUserID", userID)
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
	out, err := q.call(ctx, "GetActiveSessionForRoute", routeID)
	if err != nil {
		return pgsqlc.BotSession{}, mapQueryErr(err)
	}
	var result pgsqlc.BotSession
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotSession{}, err
	}
	return result, nil
}

func (q *Queries) GetBindCode(ctx context.Context, token string) (pgsqlc.ChannelIdentityBindCode, error) {
	out, err := q.call(ctx, "GetBindCode", token)
	if err != nil {
		return pgsqlc.ChannelIdentityBindCode{}, mapQueryErr(err)
	}
	var result pgsqlc.ChannelIdentityBindCode
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ChannelIdentityBindCode{}, err
	}
	return result, nil
}

func (q *Queries) GetBindCodeForUpdate(ctx context.Context, token string) (pgsqlc.ChannelIdentityBindCode, error) {
	out, err := q.call(ctx, "GetBindCodeForUpdate", token)
	if err != nil {
		return pgsqlc.ChannelIdentityBindCode{}, mapQueryErr(err)
	}
	var result pgsqlc.ChannelIdentityBindCode
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ChannelIdentityBindCode{}, err
	}
	return result, nil
}

func (q *Queries) GetBotACLDefaultEffect(ctx context.Context, id pgtype.UUID) (string, error) {
	out, err := q.call(ctx, "GetBotACLDefaultEffect", id)
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
	out, err := q.call(ctx, "GetBotByID", id)
	if err != nil {
		return pgsqlc.GetBotByIDRow{}, mapQueryErr(err)
	}
	var result pgsqlc.GetBotByIDRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.GetBotByIDRow{}, err
	}
	return result, nil
}

func (q *Queries) GetBotChannelConfig(ctx context.Context, arg pgsqlc.GetBotChannelConfigParams) (pgsqlc.BotChannelConfig, error) {
	out, err := q.call(ctx, "GetBotChannelConfig", arg)
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
	out, err := q.call(ctx, "GetBotChannelConfigByExternalIdentity", arg)
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
	out, err := q.call(ctx, "GetBotEmailBindingByBotAndProvider", arg)
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
	out, err := q.call(ctx, "GetBotEmailBindingByID", id)
	if err != nil {
		return pgsqlc.BotEmailBinding{}, mapQueryErr(err)
	}
	var result pgsqlc.BotEmailBinding
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotEmailBinding{}, err
	}
	return result, nil
}

func (q *Queries) GetBotStorageBinding(ctx context.Context, botID pgtype.UUID) (pgsqlc.BotStorageBinding, error) {
	out, err := q.call(ctx, "GetBotStorageBinding", botID)
	if err != nil {
		return pgsqlc.BotStorageBinding{}, mapQueryErr(err)
	}
	var result pgsqlc.BotStorageBinding
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BotStorageBinding{}, err
	}
	return result, nil
}

func (q *Queries) GetBrowserContextByID(ctx context.Context, id pgtype.UUID) (pgsqlc.BrowserContext, error) {
	out, err := q.call(ctx, "GetBrowserContextByID", id)
	if err != nil {
		return pgsqlc.BrowserContext{}, mapQueryErr(err)
	}
	var result pgsqlc.BrowserContext
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BrowserContext{}, err
	}
	return result, nil
}

func (q *Queries) GetChannelIdentityByChannelSubject(ctx context.Context, arg pgsqlc.GetChannelIdentityByChannelSubjectParams) (pgsqlc.ChannelIdentity, error) {
	out, err := q.call(ctx, "GetChannelIdentityByChannelSubject", arg)
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
	out, err := q.call(ctx, "GetChannelIdentityByID", id)
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
	out, err := q.call(ctx, "GetChannelIdentityByIDForUpdate", id)
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
	out, err := q.call(ctx, "GetChatByID", id)
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
	out, err := q.call(ctx, "GetChatParticipant", arg)
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
	out, err := q.call(ctx, "GetChatReadAccessByUser", arg)
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
	out, err := q.call(ctx, "GetChatRouteByID", id)
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
	out, err := q.call(ctx, "GetChatSettings", id)
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
	out, err := q.call(ctx, "GetCompactionLogByID", id)
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
	out, err := q.call(ctx, "GetContainerByBotID", botID)
	if err != nil {
		return pgsqlc.Container{}, mapQueryErr(err)
	}
	var result pgsqlc.Container
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Container{}, err
	}
	return result, nil
}

func (q *Queries) GetDefaultMemoryProvider(ctx context.Context) (pgsqlc.MemoryProvider, error) {
	out, err := q.call(ctx, "GetDefaultMemoryProvider")
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
	out, err := q.call(ctx, "GetEmailOAuthTokenByProvider", emailProviderID)
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
	out, err := q.call(ctx, "GetEmailOAuthTokenByState", state)
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
	out, err := q.call(ctx, "GetEmailOutboxByID", id)
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
	out, err := q.call(ctx, "GetEmailProviderByID", id)
	if err != nil {
		return pgsqlc.EmailProvider{}, mapQueryErr(err)
	}
	var result pgsqlc.EmailProvider
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.EmailProvider{}, err
	}
	return result, nil
}

func (q *Queries) GetEmailProviderByName(ctx context.Context, name string) (pgsqlc.EmailProvider, error) {
	out, err := q.call(ctx, "GetEmailProviderByName", name)
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
	out, err := q.call(ctx, "GetLatestAssistantUsage", sessionID)
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
	out, err := q.call(ctx, "GetLatestPendingToolApprovalBySession", arg)
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
	out, err := q.call(ctx, "GetLatestSessionIDByBot", botID)
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
	out, err := q.call(ctx, "GetMCPConnectionByID", arg)
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
	out, err := q.call(ctx, "GetMCPOAuthToken", connectionID)
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
	out, err := q.call(ctx, "GetMCPOAuthTokenByState", stateParam)
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
	out, err := q.call(ctx, "GetMemoryProviderByID", id)
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
	out, err := q.call(ctx, "GetModelByID", id)
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
	out, err := q.call(ctx, "GetModelByModelID", modelID)
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
	out, err := q.call(ctx, "GetModelByProviderAndModelID", arg)
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
	out, err := q.call(ctx, "GetPendingToolApprovalByReplyMessage", arg)
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
	out, err := q.call(ctx, "GetPendingToolApprovalBySessionShortID", arg)
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
	out, err := q.call(ctx, "GetProviderByClientType", clientType)
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
	out, err := q.call(ctx, "GetProviderByID", id)
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
	out, err := q.call(ctx, "GetProviderByName", name)
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
	out, err := q.call(ctx, "GetProviderOAuthTokenByProvider", providerID)
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
	out, err := q.call(ctx, "GetProviderOAuthTokenByState", state)
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
	out, err := q.call(ctx, "GetScheduleByID", id)
	if err != nil {
		return pgsqlc.Schedule{}, mapQueryErr(err)
	}
	var result pgsqlc.Schedule
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Schedule{}, err
	}
	return result, nil
}

func (q *Queries) GetSearchProviderByID(ctx context.Context, id pgtype.UUID) (pgsqlc.SearchProvider, error) {
	out, err := q.call(ctx, "GetSearchProviderByID", id)
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
	out, err := q.call(ctx, "GetSearchProviderByName", name)
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
	out, err := q.call(ctx, "GetSessionByID", id)
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
	out, err := q.call(ctx, "GetSessionCacheStats", sessionID)
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
	out, err := q.call(ctx, "GetSessionUsedSkills", sessionID)
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
	out, err := q.call(ctx, "GetSettingsByBotID", id)
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
	out, err := q.call(ctx, "GetSnapshotByContainerAndRuntimeName", arg)
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
	out, err := q.call(ctx, "GetSpeechModelWithProvider", id)
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
	out, err := q.call(ctx, "GetStorageProviderByID", id)
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
	out, err := q.call(ctx, "GetStorageProviderByName", name)
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
	out, err := q.call(ctx, "GetTokenUsageByDayAndType", arg)
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
	out, err := q.call(ctx, "GetTokenUsageByModel", arg)
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
	out, err := q.call(ctx, "GetToolApprovalRequest", id)
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
	out, err := q.call(ctx, "GetTranscriptionModelWithProvider", id)
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
	out, err := q.call(ctx, "GetUserByID", id)
	if err != nil {
		return pgsqlc.User{}, mapQueryErr(err)
	}
	var result pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.User{}, err
	}
	return result, nil
}

func (q *Queries) GetUserChannelBinding(ctx context.Context, arg pgsqlc.GetUserChannelBindingParams) (pgsqlc.UserChannelBinding, error) {
	out, err := q.call(ctx, "GetUserChannelBinding", arg)
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
	out, err := q.call(ctx, "GetUserProviderOAuthToken", arg)
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
	out, err := q.call(ctx, "GetUserProviderOAuthTokenByState", state)
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
	out, err := q.call(ctx, "GetVersionSnapshotRuntimeName", arg)
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
	out, err := q.call(ctx, "IncrementScheduleCalls", id)
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
	_, err := q.call(ctx, "InsertLifecycleEvent", arg)
	return mapQueryErr(err)
}

func (q *Queries) InsertVersion(ctx context.Context, arg pgsqlc.InsertVersionParams) (pgsqlc.ContainerVersion, error) {
	out, err := q.call(ctx, "InsertVersion", arg)
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
	out, err := q.call(ctx, "ListAccounts")
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
	out, err := q.call(ctx, "ListActiveMessagesSince", arg)
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
	out, err := q.call(ctx, "ListActiveMessagesSinceBySession", arg)
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
	out, err := q.call(ctx, "ListAutoStartContainers")
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
	out, err := q.call(ctx, "ListBotACLRules", botID)
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
	out, err := q.call(ctx, "ListBotChannelConfigsByType", channelType)
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
	out, err := q.call(ctx, "ListBotEmailBindings", botID)
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
	out, err := q.call(ctx, "ListBotEmailBindingsByProvider", emailProviderID)
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
	out, err := q.call(ctx, "ListBotsByOwner", ownerUserID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListBotsByOwnerRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListBrowserContexts(ctx context.Context) ([]pgsqlc.BrowserContext, error) {
	out, err := q.call(ctx, "ListBrowserContexts")
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.BrowserContext
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListChannelIdentitiesByUserID(ctx context.Context, userID pgtype.UUID) ([]pgsqlc.ChannelIdentity, error) {
	out, err := q.call(ctx, "ListChannelIdentitiesByUserID", userID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ChannelIdentity
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListChatParticipants(ctx context.Context, chatID pgtype.UUID) ([]pgsqlc.ListChatParticipantsRow, error) {
	out, err := q.call(ctx, "ListChatParticipants", chatID)
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
	out, err := q.call(ctx, "ListChatRoutes", chatID)
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
	out, err := q.call(ctx, "ListChatsByBotAndUser", arg)
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
	out, err := q.call(ctx, "ListCompactionLogsByBot", arg)
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
	out, err := q.call(ctx, "ListCompactionLogsBySession", sessionID)
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
	out, err := q.call(ctx, "ListEmailOutboxByBot", arg)
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
	out, err := q.call(ctx, "ListEmailProviders")
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
	out, err := q.call(ctx, "ListEmailProvidersByProvider", provider)
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
	out, err := q.call(ctx, "ListEnabledModels")
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
	out, err := q.call(ctx, "ListEnabledModelsByProviderClientType", clientType)
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
	out, err := q.call(ctx, "ListEnabledModelsByType", type_)
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
	out, err := q.call(ctx, "ListEnabledSchedules")
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
	out, err := q.call(ctx, "ListHeartbeatEnabledBots")
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
	out, err := q.call(ctx, "ListHeartbeatLogsByBot", arg)
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
	out, err := q.call(ctx, "ListMCPConnectionsByBotID", botID)
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
	out, err := q.call(ctx, "ListMemoryProviders")
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
	out, err := q.call(ctx, "ListMessageAssets", messageID)
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
	out, err := q.call(ctx, "ListMessageAssetsBatch", messageIds)
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
	out, err := q.call(ctx, "ListMessages", botID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessagesBefore(ctx context.Context, arg pgsqlc.ListMessagesBeforeParams) ([]pgsqlc.ListMessagesBeforeRow, error) {
	out, err := q.call(ctx, "ListMessagesBefore", arg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesBeforeRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessagesBeforeBySession(ctx context.Context, arg pgsqlc.ListMessagesBeforeBySessionParams) ([]pgsqlc.ListMessagesBeforeBySessionRow, error) {
	out, err := q.call(ctx, "ListMessagesBeforeBySession", arg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesBeforeBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessagesBySession(ctx context.Context, sessionID pgtype.UUID) ([]pgsqlc.ListMessagesBySessionRow, error) {
	out, err := q.call(ctx, "ListMessagesBySession", sessionID)
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
	out, err := q.call(ctx, "ListMessagesLatest", arg)
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
	out, err := q.call(ctx, "ListMessagesLatestBySession", arg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListMessagesLatestBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListMessagesSince(ctx context.Context, arg pgsqlc.ListMessagesSinceParams) ([]pgsqlc.ListMessagesSinceRow, error) {
	out, err := q.call(ctx, "ListMessagesSince", arg)
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
	out, err := q.call(ctx, "ListMessagesSinceBySession", arg)
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
	out, err := q.call(ctx, "ListModels")
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
	out, err := q.call(ctx, "ListModelsByModelID", modelID)
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
	out, err := q.call(ctx, "ListModelsByProviderClientType", clientType)
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
	out, err := q.call(ctx, "ListModelsByProviderID", providerID)
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
	out, err := q.call(ctx, "ListModelsByProviderIDAndType", arg)
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
	out, err := q.call(ctx, "ListModelsByType", type_)
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
	out, err := q.call(ctx, "ListModelVariantsByModelUUID", modelUuid)
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
	out, err := q.call(ctx, "ListObservedConversationsByChannelIdentity", arg)
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
	out, err := q.call(ctx, "ListObservedConversationsByChannelType", arg)
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
	out, err := q.call(ctx, "ListPendingToolApprovalsBySession", arg)
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
	out, err := q.call(ctx, "ListProviders")
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
	out, err := q.call(ctx, "ListReadableBindingsByProvider", emailProviderID)
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
	out, err := q.call(ctx, "ListScheduleLogsByBot", arg)
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
	out, err := q.call(ctx, "ListScheduleLogsBySchedule", arg)
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
	out, err := q.call(ctx, "ListSchedulesByBot", botID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.Schedule
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSearchProviders(ctx context.Context) ([]pgsqlc.SearchProvider, error) {
	out, err := q.call(ctx, "ListSearchProviders")
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
	out, err := q.call(ctx, "ListSearchProvidersByProvider", provider)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.SearchProvider
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSessionEventsBySession(ctx context.Context, sessionID pgtype.UUID) ([]pgsqlc.BotSessionEvent, error) {
	out, err := q.call(ctx, "ListSessionEventsBySession", sessionID)
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
	out, err := q.call(ctx, "ListSessionEventsBySessionAfter", arg)
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
	out, err := q.call(ctx, "ListSessionsByBot", botID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListSessionsByBotRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListSessionsByRoute(ctx context.Context, routeID pgtype.UUID) ([]pgsqlc.BotSession, error) {
	out, err := q.call(ctx, "ListSessionsByRoute", routeID)
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
	out, err := q.call(ctx, "ListSnapshotsByContainerID", containerID)
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
	out, err := q.call(ctx, "ListSnapshotsWithVersionByContainerID", containerID)
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
	out, err := q.call(ctx, "ListSpeechModels")
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
	out, err := q.call(ctx, "ListSpeechModelsByProviderID", providerID)
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
	out, err := q.call(ctx, "ListSpeechProviders")
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
	out, err := q.call(ctx, "ListStorageProviders")
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
	out, err := q.call(ctx, "ListSubagentSessionsByParent", parentSessionID)
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
	out, err := q.call(ctx, "ListThreadsByParent", id)
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
	out, err := q.call(ctx, "ListTokenUsageRecords", arg)
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
	out, err := q.call(ctx, "ListToolApprovalsBySession", arg)
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
	out, err := q.call(ctx, "ListTranscriptionModels")
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
	out, err := q.call(ctx, "ListTranscriptionModelsByProviderID", providerID)
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
	out, err := q.call(ctx, "ListTranscriptionProviders")
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
	out, err := q.call(ctx, "ListUncompactedMessagesBySession", sessionID)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListUncompactedMessagesBySessionRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) ListUserChannelBindingsByPlatform(ctx context.Context, channelType string) ([]pgsqlc.UserChannelBinding, error) {
	out, err := q.call(ctx, "ListUserChannelBindingsByPlatform", channelType)
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
	out, err := q.call(ctx, "ListVersionsByContainerID", containerID)
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
	out, err := q.call(ctx, "ListVisibleChatsByBotAndUser", arg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.ListVisibleChatsByBotAndUserRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) MarkBindCodeUsed(ctx context.Context, arg pgsqlc.MarkBindCodeUsedParams) (pgsqlc.ChannelIdentityBindCode, error) {
	out, err := q.call(ctx, "MarkBindCodeUsed", arg)
	if err != nil {
		return pgsqlc.ChannelIdentityBindCode{}, mapQueryErr(err)
	}
	var result pgsqlc.ChannelIdentityBindCode
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ChannelIdentityBindCode{}, err
	}
	return result, nil
}

func (q *Queries) MarkMessagesCompacted(ctx context.Context, arg pgsqlc.MarkMessagesCompactedParams) error {
	_, err := q.call(ctx, "MarkMessagesCompacted", arg)
	return mapQueryErr(err)
}

func (q *Queries) NextVersion(ctx context.Context, containerID string) (int32, error) {
	out, err := q.call(ctx, "NextVersion", containerID)
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
	out, err := q.call(ctx, "RejectToolApprovalRequest", arg)
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
	_, err := q.call(ctx, "RemoveChatParticipant", arg)
	return mapQueryErr(err)
}

func (q *Queries) SaveMatrixSyncSinceToken(ctx context.Context, arg pgsqlc.SaveMatrixSyncSinceTokenParams) (int64, error) {
	out, err := q.call(ctx, "SaveMatrixSyncSinceToken", arg)
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
	out, err := q.call(ctx, "SearchAccounts", arg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) SearchChannelIdentities(ctx context.Context, arg pgsqlc.SearchChannelIdentitiesParams) ([]pgsqlc.SearchChannelIdentitiesRow, error) {
	out, err := q.call(ctx, "SearchChannelIdentities", arg)
	if err != nil {
		return nil, mapQueryErr(err)
	}
	var result []pgsqlc.SearchChannelIdentitiesRow
	if err := convertValue(out, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (q *Queries) SearchMessages(ctx context.Context, arg pgsqlc.SearchMessagesParams) ([]pgsqlc.SearchMessagesRow, error) {
	out, err := q.call(ctx, "SearchMessages", arg)
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
	_, err := q.call(ctx, "SetBotACLDefaultEffect", arg)
	return mapQueryErr(err)
}

func (q *Queries) SetChannelIdentityLinkedUser(ctx context.Context, arg pgsqlc.SetChannelIdentityLinkedUserParams) (pgsqlc.ChannelIdentity, error) {
	out, err := q.call(ctx, "SetChannelIdentityLinkedUser", arg)
	if err != nil {
		return pgsqlc.ChannelIdentity{}, mapQueryErr(err)
	}
	var result pgsqlc.ChannelIdentity
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.ChannelIdentity{}, err
	}
	return result, nil
}

func (q *Queries) SetRouteActiveSession(ctx context.Context, arg pgsqlc.SetRouteActiveSessionParams) error {
	_, err := q.call(ctx, "SetRouteActiveSession", arg)
	return mapQueryErr(err)
}

func (q *Queries) SoftDeleteSession(ctx context.Context, id pgtype.UUID) error {
	_, err := q.call(ctx, "SoftDeleteSession", id)
	return mapQueryErr(err)
}

func (q *Queries) SoftDeleteSessionsByBot(ctx context.Context, botID pgtype.UUID) error {
	_, err := q.call(ctx, "SoftDeleteSessionsByBot", botID)
	return mapQueryErr(err)
}

func (q *Queries) TouchChat(ctx context.Context, chatID pgtype.UUID) error {
	_, err := q.call(ctx, "TouchChat", chatID)
	return mapQueryErr(err)
}

func (q *Queries) TouchSession(ctx context.Context, id pgtype.UUID) error {
	_, err := q.call(ctx, "TouchSession", id)
	return mapQueryErr(err)
}

func (q *Queries) UpdateAccountAdmin(ctx context.Context, arg pgsqlc.UpdateAccountAdminParams) (pgsqlc.User, error) {
	out, err := q.call(ctx, "UpdateAccountAdmin", arg)
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
	out, err := q.call(ctx, "UpdateAccountLastLogin", id)
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
	out, err := q.call(ctx, "UpdateAccountPassword", arg)
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
	out, err := q.call(ctx, "UpdateAccountProfile", arg)
	if err != nil {
		return pgsqlc.User{}, mapQueryErr(err)
	}
	var result pgsqlc.User
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.User{}, err
	}
	return result, nil
}

func (q *Queries) UpdateBotACLRule(ctx context.Context, arg pgsqlc.UpdateBotACLRuleParams) (pgsqlc.UpdateBotACLRuleRow, error) {
	out, err := q.call(ctx, "UpdateBotACLRule", arg)
	if err != nil {
		return pgsqlc.UpdateBotACLRuleRow{}, mapQueryErr(err)
	}
	var result pgsqlc.UpdateBotACLRuleRow
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UpdateBotACLRuleRow{}, err
	}
	return result, nil
}

func (q *Queries) UpdateBotACLRulePriority(ctx context.Context, arg pgsqlc.UpdateBotACLRulePriorityParams) error {
	_, err := q.call(ctx, "UpdateBotACLRulePriority", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateBotChannelConfigDisabled(ctx context.Context, arg pgsqlc.UpdateBotChannelConfigDisabledParams) (pgsqlc.BotChannelConfig, error) {
	out, err := q.call(ctx, "UpdateBotChannelConfigDisabled", arg)
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
	out, err := q.call(ctx, "UpdateBotEmailBinding", arg)
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
	out, err := q.call(ctx, "UpdateBotOwner", arg)
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
	out, err := q.call(ctx, "UpdateBotProfile", arg)
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
	_, err := q.call(ctx, "UpdateBotStatus", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateBrowserContext(ctx context.Context, arg pgsqlc.UpdateBrowserContextParams) (pgsqlc.BrowserContext, error) {
	out, err := q.call(ctx, "UpdateBrowserContext", arg)
	if err != nil {
		return pgsqlc.BrowserContext{}, mapQueryErr(err)
	}
	var result pgsqlc.BrowserContext
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.BrowserContext{}, err
	}
	return result, nil
}

func (q *Queries) UpdateChatRouteMetadata(ctx context.Context, arg pgsqlc.UpdateChatRouteMetadataParams) error {
	_, err := q.call(ctx, "UpdateChatRouteMetadata", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateChatRouteReplyTarget(ctx context.Context, arg pgsqlc.UpdateChatRouteReplyTargetParams) error {
	_, err := q.call(ctx, "UpdateChatRouteReplyTarget", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateChatTitle(ctx context.Context, arg pgsqlc.UpdateChatTitleParams) (pgsqlc.UpdateChatTitleRow, error) {
	out, err := q.call(ctx, "UpdateChatTitle", arg)
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
	_, err := q.call(ctx, "UpdateContainerStarted", botID)
	return mapQueryErr(err)
}

func (q *Queries) UpdateContainerStatus(ctx context.Context, arg pgsqlc.UpdateContainerStatusParams) error {
	_, err := q.call(ctx, "UpdateContainerStatus", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateContainerStopped(ctx context.Context, botID pgtype.UUID) error {
	_, err := q.call(ctx, "UpdateContainerStopped", botID)
	return mapQueryErr(err)
}

func (q *Queries) UpdateEmailOAuthState(ctx context.Context, arg pgsqlc.UpdateEmailOAuthStateParams) error {
	_, err := q.call(ctx, "UpdateEmailOAuthState", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateEmailOutboxFailed(ctx context.Context, arg pgsqlc.UpdateEmailOutboxFailedParams) error {
	_, err := q.call(ctx, "UpdateEmailOutboxFailed", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateEmailOutboxSent(ctx context.Context, arg pgsqlc.UpdateEmailOutboxSentParams) error {
	_, err := q.call(ctx, "UpdateEmailOutboxSent", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateEmailProvider(ctx context.Context, arg pgsqlc.UpdateEmailProviderParams) (pgsqlc.EmailProvider, error) {
	out, err := q.call(ctx, "UpdateEmailProvider", arg)
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
	out, err := q.call(ctx, "UpdateMCPConnection", arg)
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
	_, err := q.call(ctx, "UpdateMCPConnectionAuthType", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateMCPConnectionProbeResult(ctx context.Context, arg pgsqlc.UpdateMCPConnectionProbeResultParams) error {
	_, err := q.call(ctx, "UpdateMCPConnectionProbeResult", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateMCPOAuthClientSecret(ctx context.Context, arg pgsqlc.UpdateMCPOAuthClientSecretParams) error {
	_, err := q.call(ctx, "UpdateMCPOAuthClientSecret", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateMCPOAuthPKCEState(ctx context.Context, arg pgsqlc.UpdateMCPOAuthPKCEStateParams) error {
	_, err := q.call(ctx, "UpdateMCPOAuthPKCEState", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateMCPOAuthTokens(ctx context.Context, arg pgsqlc.UpdateMCPOAuthTokensParams) error {
	_, err := q.call(ctx, "UpdateMCPOAuthTokens", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateMemoryProvider(ctx context.Context, arg pgsqlc.UpdateMemoryProviderParams) (pgsqlc.MemoryProvider, error) {
	out, err := q.call(ctx, "UpdateMemoryProvider", arg)
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
	out, err := q.call(ctx, "UpdateModel", arg)
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
	out, err := q.call(ctx, "UpdateProvider", arg)
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
	_, err := q.call(ctx, "UpdateProviderOAuthState", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpdateSchedule(ctx context.Context, arg pgsqlc.UpdateScheduleParams) (pgsqlc.Schedule, error) {
	out, err := q.call(ctx, "UpdateSchedule", arg)
	if err != nil {
		return pgsqlc.Schedule{}, mapQueryErr(err)
	}
	var result pgsqlc.Schedule
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.Schedule{}, err
	}
	return result, nil
}

func (q *Queries) UpdateSearchProvider(ctx context.Context, arg pgsqlc.UpdateSearchProviderParams) (pgsqlc.SearchProvider, error) {
	out, err := q.call(ctx, "UpdateSearchProvider", arg)
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
	out, err := q.call(ctx, "UpdateSessionMetadata", arg)
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
	out, err := q.call(ctx, "UpdateSessionTitle", arg)
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
	out, err := q.call(ctx, "UpdateToolApprovalPromptMessage", arg)
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
	_, err := q.call(ctx, "UpdateUserProviderOAuthState", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpsertAccountByUsername(ctx context.Context, arg pgsqlc.UpsertAccountByUsernameParams) (pgsqlc.User, error) {
	out, err := q.call(ctx, "UpsertAccountByUsername", arg)
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
	out, err := q.call(ctx, "UpsertBotChannelConfig", arg)
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
	out, err := q.call(ctx, "UpsertBotSettings", arg)
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
	out, err := q.call(ctx, "UpsertBotStorageBinding", arg)
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
	out, err := q.call(ctx, "UpsertChannelIdentityByChannelSubject", arg)
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
	out, err := q.call(ctx, "UpsertChatSettings", arg)
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
	_, err := q.call(ctx, "UpsertContainer", arg)
	return mapQueryErr(err)
}

func (q *Queries) UpsertEmailOAuthToken(ctx context.Context, arg pgsqlc.UpsertEmailOAuthTokenParams) (pgsqlc.EmailOauthToken, error) {
	out, err := q.call(ctx, "UpsertEmailOAuthToken", arg)
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
	out, err := q.call(ctx, "UpsertMCPConnectionByName", arg)
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
	out, err := q.call(ctx, "UpsertMCPOAuthDiscovery", arg)
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
	out, err := q.call(ctx, "UpsertProviderOAuthToken", arg)
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
	out, err := q.call(ctx, "UpsertRegistryModel", arg)
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
	out, err := q.call(ctx, "UpsertRegistryProvider", arg)
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
	out, err := q.call(ctx, "UpsertSnapshot", arg)
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
	out, err := q.call(ctx, "UpsertUserChannelBinding", arg)
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
	out, err := q.call(ctx, "UpsertUserProviderOAuthToken", arg)
	if err != nil {
		return pgsqlc.UserProviderOauthToken{}, mapQueryErr(err)
	}
	var result pgsqlc.UserProviderOauthToken
	if err := convertValue(out, &result); err != nil {
		return pgsqlc.UserProviderOauthToken{}, err
	}
	return result, nil
}
