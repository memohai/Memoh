package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

// Queries is the transitional database interface implemented by sqlc-backed stores.
// Domain-specific stores should replace this broad interface module by module.
type Queries interface {
	ApproveToolApprovalRequest(ctx context.Context, arg dbsqlc.ApproveToolApprovalRequestParams) (dbsqlc.ToolApprovalRequest, error)
	CancelPendingToolApprovalsBySession(ctx context.Context, arg dbsqlc.CancelPendingToolApprovalsBySessionParams) ([]dbsqlc.ToolApprovalRequest, error)
	CancelPendingUserInputsBySession(ctx context.Context, arg dbsqlc.CancelPendingUserInputsBySessionParams) ([]dbsqlc.UserInputRequest, error)
	CancelUserInputRequest(ctx context.Context, arg dbsqlc.CancelUserInputRequestParams) (dbsqlc.UserInputRequest, error)
	ClearRouteActiveSessionsByBot(ctx context.Context, botID pgtype.UUID) error
	ClearMCPOAuthTokens(ctx context.Context, connectionID pgtype.UUID) error
	CompleteCompactionLog(ctx context.Context, arg dbsqlc.CompleteCompactionLogParams) (dbsqlc.BotHistoryMessageCompact, error)
	CompleteHeartbeatLog(ctx context.Context, arg dbsqlc.CompleteHeartbeatLogParams) (dbsqlc.BotHeartbeatLog, error)
	CompleteScheduleLog(ctx context.Context, arg dbsqlc.CompleteScheduleLogParams) (dbsqlc.ScheduleLog, error)
	CountAccounts(ctx context.Context) (int64, error)
	CountCompactionLogsByBot(ctx context.Context, botID pgtype.UUID) (int64, error)
	CountEmailOutboxByBot(ctx context.Context, botID pgtype.UUID) (int64, error)
	CountHeartbeatLogsByBot(ctx context.Context, botID pgtype.UUID) (int64, error)
	CountMemoryProvidersByDefault(ctx context.Context) (int64, error)
	CountMessageAssetsByBot(ctx context.Context, botID pgtype.UUID) (int64, error)
	CountMessagesByBot(ctx context.Context, botID pgtype.UUID) (int64, error)
	CountMessagesBySession(ctx context.Context, sessionID pgtype.UUID) (int64, error)
	CountModels(ctx context.Context) (int64, error)
	CountModelsByType(ctx context.Context, type_ string) (int64, error)
	CountProviders(ctx context.Context) (int64, error)
	CountScheduleLogsByBot(ctx context.Context, botID pgtype.UUID) (int64, error)
	CountScheduleLogsBySchedule(ctx context.Context, scheduleID pgtype.UUID) (int64, error)
	CountSessionEvents(ctx context.Context, sessionID pgtype.UUID) (int64, error)
	CountTokenUsageRecords(ctx context.Context, arg dbsqlc.CountTokenUsageRecordsParams) (int64, error)
	CreateAccount(ctx context.Context, arg dbsqlc.CreateAccountParams) (dbsqlc.User, error)
	CreateBot(ctx context.Context, arg dbsqlc.CreateBotParams) (dbsqlc.CreateBotRow, error)
	CreateBotACLRule(ctx context.Context, arg dbsqlc.CreateBotACLRuleParams) (dbsqlc.BotAclRule, error)
	CreateBotPluginInstallation(ctx context.Context, arg dbsqlc.CreateBotPluginInstallationParams) (dbsqlc.BotPluginInstallation, error)
	CreateBotUserGrant(ctx context.Context, arg dbsqlc.CreateBotUserGrantParams) (dbsqlc.BotUserGrant, error)
	DeleteBotUserGrantByID(ctx context.Context, id pgtype.UUID) error
	UpsertBotChannelAdmin(ctx context.Context, arg dbsqlc.UpsertBotChannelAdminParams) (dbsqlc.BotChannelAdmin, error)
	DeleteBotChannelAdmin(ctx context.Context, arg dbsqlc.DeleteBotChannelAdminParams) error
	GetBotChannelAdmin(ctx context.Context, arg dbsqlc.GetBotChannelAdminParams) (dbsqlc.BotChannelAdmin, error)
	ListBotChannelAdmins(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.ListBotChannelAdminsRow, error)
	CreateChannelLinkCode(ctx context.Context, arg dbsqlc.CreateChannelLinkCodeParams) (dbsqlc.ChannelLinkCode, error)
	GetChannelLinkCodeByToken(ctx context.Context, token string) (dbsqlc.ChannelLinkCode, error)
	MarkChannelLinkCodeConsumed(ctx context.Context, arg dbsqlc.MarkChannelLinkCodeConsumedParams) (dbsqlc.ChannelLinkCode, error)
	UpsertUserChannelIdentityBinding(ctx context.Context, arg dbsqlc.UpsertUserChannelIdentityBindingParams) (dbsqlc.UserChannelIdentityBinding, error)
	ListChannelIdentityBindings(ctx context.Context) ([]dbsqlc.ListChannelIdentityBindingsRow, error)
	ListChannelIdentityBindingsForUser(ctx context.Context, userID pgtype.UUID) ([]dbsqlc.ListChannelIdentityBindingsForUserRow, error)
	ListChannelIdentityBindingsForBot(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.ListChannelIdentityBindingsForBotRow, error)
	DeleteUserChannelIdentityBinding(ctx context.Context, arg dbsqlc.DeleteUserChannelIdentityBindingParams) error
	ListUserIDsByChannelIdentity(ctx context.Context, channelIdentityID pgtype.UUID) ([]pgtype.UUID, error)
	GetBotUserGrantByID(ctx context.Context, id pgtype.UUID) (dbsqlc.BotUserGrant, error)
	ListBotUserGrants(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.ListBotUserGrantsRow, error)
	ListBotUserGrantsForUser(ctx context.Context, arg dbsqlc.ListBotUserGrantsForUserParams) ([]dbsqlc.ListBotUserGrantsForUserRow, error)
	UpdateBotUserGrantPermissions(ctx context.Context, arg dbsqlc.UpdateBotUserGrantPermissionsParams) (dbsqlc.BotUserGrant, error)
	ListAccessibleBots(ctx context.Context, ownerUserID pgtype.UUID) ([]dbsqlc.ListAccessibleBotsRow, error)
	CreateBotEmailBinding(ctx context.Context, arg dbsqlc.CreateBotEmailBindingParams) (dbsqlc.BotEmailBinding, error)
	CreateChannelIdentity(ctx context.Context, arg dbsqlc.CreateChannelIdentityParams) (dbsqlc.ChannelIdentity, error)
	CreateChat(ctx context.Context, arg dbsqlc.CreateChatParams) (dbsqlc.CreateChatRow, error)
	CreateChatRoute(ctx context.Context, arg dbsqlc.CreateChatRouteParams) (dbsqlc.CreateChatRouteRow, error)
	CreateCompactionLog(ctx context.Context, arg dbsqlc.CreateCompactionLogParams) (dbsqlc.BotHistoryMessageCompact, error)
	CreateEmailOutbox(ctx context.Context, arg dbsqlc.CreateEmailOutboxParams) (dbsqlc.EmailOutbox, error)
	CreateEmailProvider(ctx context.Context, arg dbsqlc.CreateEmailProviderParams) (dbsqlc.EmailProvider, error)
	CreateFetchProvider(ctx context.Context, arg dbsqlc.CreateFetchProviderParams) (dbsqlc.FetchProvider, error)
	CreateHistoryTurn(ctx context.Context, arg dbsqlc.CreateHistoryTurnParams) (dbsqlc.BotHistoryTurn, error)
	CreateHeartbeatLog(ctx context.Context, arg dbsqlc.CreateHeartbeatLogParams) (dbsqlc.CreateHeartbeatLogRow, error)
	CreateManagedMCPConnection(ctx context.Context, arg dbsqlc.CreateManagedMCPConnectionParams) (dbsqlc.McpConnection, error)
	CreateMCPConnection(ctx context.Context, arg dbsqlc.CreateMCPConnectionParams) (dbsqlc.McpConnection, error)
	CreateMemoryProvider(ctx context.Context, arg dbsqlc.CreateMemoryProviderParams) (dbsqlc.MemoryProvider, error)
	CreateMessage(ctx context.Context, arg dbsqlc.CreateMessageParams) (dbsqlc.CreateMessageRow, error)
	CreateMessageAsset(ctx context.Context, arg dbsqlc.CreateMessageAssetParams) (dbsqlc.BotHistoryMessageAsset, error)
	CreateModel(ctx context.Context, arg dbsqlc.CreateModelParams) (dbsqlc.Model, error)
	CreateModelVariant(ctx context.Context, arg dbsqlc.CreateModelVariantParams) (dbsqlc.ModelVariant, error)
	CreateProvider(ctx context.Context, arg dbsqlc.CreateProviderParams) (dbsqlc.Provider, error)
	CreateSchedule(ctx context.Context, arg dbsqlc.CreateScheduleParams) (dbsqlc.Schedule, error)
	CreateScheduleLog(ctx context.Context, arg dbsqlc.CreateScheduleLogParams) (dbsqlc.CreateScheduleLogRow, error)
	CreateSearchProvider(ctx context.Context, arg dbsqlc.CreateSearchProviderParams) (dbsqlc.SearchProvider, error)
	CreateSession(ctx context.Context, arg dbsqlc.CreateSessionParams) (dbsqlc.BotSession, error)
	CreateSessionEvent(ctx context.Context, arg dbsqlc.CreateSessionEventParams) (pgtype.UUID, error)
	CreateSessionTurnHead(ctx context.Context, arg dbsqlc.CreateSessionTurnHeadParams) (dbsqlc.BotSessionTurnHead, error)
	CreateStorageProvider(ctx context.Context, arg dbsqlc.CreateStorageProviderParams) (dbsqlc.StorageProvider, error)
	CreateToolApprovalRequest(ctx context.Context, arg dbsqlc.CreateToolApprovalRequestParams) (dbsqlc.ToolApprovalRequest, error)
	CreateToolApprovalRequestForTurn(ctx context.Context, arg dbsqlc.CreateToolApprovalRequestForTurnParams) (dbsqlc.ToolApprovalRequest, error)
	CreateUserInputRequest(ctx context.Context, arg dbsqlc.CreateUserInputRequestParams) (dbsqlc.UserInputRequest, error)
	CreateUserInputRequestForTurn(ctx context.Context, arg dbsqlc.CreateUserInputRequestForTurnParams) (dbsqlc.UserInputRequest, error)
	CreateUser(ctx context.Context, arg dbsqlc.CreateUserParams) (dbsqlc.User, error)
	ClearSessionTurnPointersByBot(ctx context.Context, botID pgtype.UUID) error
	DeleteBotACLRuleByID(ctx context.Context, id pgtype.UUID) error
	DeleteBotByID(ctx context.Context, id pgtype.UUID) error
	DeleteBotChannelConfig(ctx context.Context, arg dbsqlc.DeleteBotChannelConfigParams) error
	DeleteBotEmailBinding(ctx context.Context, id pgtype.UUID) error
	DeleteBotPluginInstallation(ctx context.Context, arg dbsqlc.DeleteBotPluginInstallationParams) error
	DeleteBotPluginResources(ctx context.Context, installationID pgtype.UUID) error
	DeleteChat(ctx context.Context, chatID pgtype.UUID) error
	DeleteChatRoute(ctx context.Context, id pgtype.UUID) error
	DeleteCompactionLogsByBot(ctx context.Context, botID pgtype.UUID) error
	DeleteContainerByBotID(ctx context.Context, botID pgtype.UUID) error
	DeleteEmailOAuthToken(ctx context.Context, emailProviderID pgtype.UUID) error
	DeleteEmailProvider(ctx context.Context, id pgtype.UUID) error
	DeleteEmailProviderByIDAndUser(ctx context.Context, arg dbsqlc.DeleteEmailProviderByIDAndUserParams) error
	DeleteFetchProvider(ctx context.Context, id pgtype.UUID) error
	DeleteHeartbeatLogsByBot(ctx context.Context, botID pgtype.UUID) error
	DeleteMCPConnection(ctx context.Context, arg dbsqlc.DeleteMCPConnectionParams) error
	DeleteMCPConnectionsByPlugin(ctx context.Context, arg dbsqlc.DeleteMCPConnectionsByPluginParams) error
	DeleteMCPOAuthToken(ctx context.Context, connectionID pgtype.UUID) error
	DeleteMemoryProvider(ctx context.Context, id pgtype.UUID) error
	DeleteMessageAssets(ctx context.Context, messageID pgtype.UUID) error
	ClearHistoryTurnMessagePointersByBot(ctx context.Context, botID pgtype.UUID) error
	DeleteHistoryTurnByID(ctx context.Context, id pgtype.UUID) error
	DeleteHistoryTurnsByBot(ctx context.Context, botID pgtype.UUID) error
	DeleteMessagesByBot(ctx context.Context, botID pgtype.UUID) error
	DeleteMessagesBySession(ctx context.Context, sessionID pgtype.UUID) error
	DeleteModel(ctx context.Context, id pgtype.UUID) error
	DeleteModelByModelID(ctx context.Context, modelID string) error
	DeleteSessionDiscussCursorsByBot(ctx context.Context, botID pgtype.UUID) error
	DeleteSessionEventsByBot(ctx context.Context, botID pgtype.UUID) error
	DeleteModelByProviderAndType(ctx context.Context, arg dbsqlc.DeleteModelByProviderAndTypeParams) error
	DeleteModelByProviderIDAndModelID(ctx context.Context, arg dbsqlc.DeleteModelByProviderIDAndModelIDParams) error
	DeleteProvider(ctx context.Context, id pgtype.UUID) error
	DeleteProviderOAuthToken(ctx context.Context, providerID pgtype.UUID) error
	DeleteSchedule(ctx context.Context, id pgtype.UUID) error
	DeleteScheduleLogsByBot(ctx context.Context, botID pgtype.UUID) error
	DeleteScheduleLogsBySchedule(ctx context.Context, scheduleID pgtype.UUID) error
	DeleteSearchProvider(ctx context.Context, id pgtype.UUID) error
	DeleteSettingsByBotID(ctx context.Context, id pgtype.UUID) error
	DeleteSessionTurnHeads(ctx context.Context, sessionID pgtype.UUID) error
	DeleteSessionTurnHeadsByBot(ctx context.Context, botID pgtype.UUID) error
	DeleteUserProviderOAuthToken(ctx context.Context, arg dbsqlc.DeleteUserProviderOAuthTokenParams) error
	EvaluateBotACLRule(ctx context.Context, arg dbsqlc.EvaluateBotACLRuleParams) (string, error)
	FailUserInputRequest(ctx context.Context, arg dbsqlc.FailUserInputRequestParams) (dbsqlc.UserInputRequest, error)
	FindChatRoute(ctx context.Context, arg dbsqlc.FindChatRouteParams) (dbsqlc.FindChatRouteRow, error)
	GetAccountByIdentity(ctx context.Context, identity pgtype.Text) (dbsqlc.User, error)
	GetAccountByUserID(ctx context.Context, userID pgtype.UUID) (dbsqlc.User, error)
	GetActiveSessionForRoute(ctx context.Context, routeID pgtype.UUID) (dbsqlc.BotSession, error)
	GetBotACLDefaultEffect(ctx context.Context, id pgtype.UUID) (string, error)
	GetBotByID(ctx context.Context, id pgtype.UUID) (dbsqlc.GetBotByIDRow, error)
	GetBotByName(ctx context.Context, name string) (dbsqlc.GetBotByNameRow, error)
	GetBotChannelConfig(ctx context.Context, arg dbsqlc.GetBotChannelConfigParams) (dbsqlc.BotChannelConfig, error)
	GetBotChannelConfigByExternalIdentity(ctx context.Context, arg dbsqlc.GetBotChannelConfigByExternalIdentityParams) (dbsqlc.BotChannelConfig, error)
	GetBotEmailBindingByBotAndProvider(ctx context.Context, arg dbsqlc.GetBotEmailBindingByBotAndProviderParams) (dbsqlc.BotEmailBinding, error)
	GetBotEmailBindingByID(ctx context.Context, id pgtype.UUID) (dbsqlc.BotEmailBinding, error)
	GetBotOverlayConfig(ctx context.Context, id pgtype.UUID) (dbsqlc.GetBotOverlayConfigRow, error)
	GetBotPluginInstallationByID(ctx context.Context, arg dbsqlc.GetBotPluginInstallationByIDParams) (dbsqlc.BotPluginInstallation, error)
	GetBotStorageBinding(ctx context.Context, botID pgtype.UUID) (dbsqlc.BotStorageBinding, error)
	GetChannelIdentityByChannelSubject(ctx context.Context, arg dbsqlc.GetChannelIdentityByChannelSubjectParams) (dbsqlc.ChannelIdentity, error)
	GetChannelIdentityByID(ctx context.Context, id pgtype.UUID) (dbsqlc.ChannelIdentity, error)
	GetChannelIdentityByIDForUpdate(ctx context.Context, id pgtype.UUID) (dbsqlc.ChannelIdentity, error)
	GetChatByID(ctx context.Context, id pgtype.UUID) (dbsqlc.GetChatByIDRow, error)
	GetChatParticipant(ctx context.Context, arg dbsqlc.GetChatParticipantParams) (dbsqlc.GetChatParticipantRow, error)
	GetChatReadAccessByUser(ctx context.Context, arg dbsqlc.GetChatReadAccessByUserParams) (dbsqlc.GetChatReadAccessByUserRow, error)
	GetChatRouteByID(ctx context.Context, id pgtype.UUID) (dbsqlc.GetChatRouteByIDRow, error)
	GetChatSettings(ctx context.Context, id pgtype.UUID) (dbsqlc.GetChatSettingsRow, error)
	GetCompactionLogByID(ctx context.Context, id pgtype.UUID) (dbsqlc.BotHistoryMessageCompact, error)
	GetContainerByBotID(ctx context.Context, botID pgtype.UUID) (dbsqlc.Container, error)
	GetBotWorkspaceResourceLimits(ctx context.Context, botID pgtype.UUID) (dbsqlc.BotWorkspaceResourceLimit, error)
	GetDefaultMemoryProvider(ctx context.Context) (dbsqlc.MemoryProvider, error)
	GetEmailOAuthTokenByProvider(ctx context.Context, emailProviderID pgtype.UUID) (dbsqlc.EmailOauthToken, error)
	GetEmailOAuthTokenByState(ctx context.Context, state string) (dbsqlc.EmailOauthToken, error)
	GetEmailOutboxByID(ctx context.Context, id pgtype.UUID) (dbsqlc.EmailOutbox, error)
	GetEmailProviderByID(ctx context.Context, id pgtype.UUID) (dbsqlc.EmailProvider, error)
	GetEmailProviderByIDAndUser(ctx context.Context, arg dbsqlc.GetEmailProviderByIDAndUserParams) (dbsqlc.EmailProvider, error)
	GetEmailProviderByNameAndUser(ctx context.Context, arg dbsqlc.GetEmailProviderByNameAndUserParams) (dbsqlc.EmailProvider, error)
	GetFetchProviderByID(ctx context.Context, id pgtype.UUID) (dbsqlc.FetchProvider, error)
	GetFetchProviderByName(ctx context.Context, name string) (dbsqlc.FetchProvider, error)
	GetHistoryTurnByID(ctx context.Context, id pgtype.UUID) (dbsqlc.BotHistoryTurn, error)
	GetLatestAssistantUsage(ctx context.Context, sessionID pgtype.UUID) (int64, error)
	GetVisibleAssistantMessageTurnForFork(ctx context.Context, arg dbsqlc.GetVisibleAssistantMessageTurnForForkParams) (dbsqlc.GetVisibleAssistantMessageTurnForForkRow, error)
	GetVisibleAssistantTurnForRetry(ctx context.Context, arg dbsqlc.GetVisibleAssistantTurnForRetryParams) (dbsqlc.GetVisibleAssistantTurnForRetryRow, error)
	GetLatestPendingToolApprovalBySession(ctx context.Context, arg dbsqlc.GetLatestPendingToolApprovalBySessionParams) (dbsqlc.ToolApprovalRequest, error)
	GetLatestPendingUserInputBySession(ctx context.Context, arg dbsqlc.GetLatestPendingUserInputBySessionParams) (dbsqlc.UserInputRequest, error)
	GetLatestSessionIDByBot(ctx context.Context, botID pgtype.UUID) (pgtype.UUID, error)
	ListHistoryTurnPathFromHead(ctx context.Context, headTurnID pgtype.UUID) ([]dbsqlc.BotHistoryTurn, error)
	GetMCPConnectionByID(ctx context.Context, arg dbsqlc.GetMCPConnectionByIDParams) (dbsqlc.McpConnection, error)
	GetMCPOAuthToken(ctx context.Context, connectionID pgtype.UUID) (dbsqlc.McpOauthToken, error)
	GetMCPOAuthTokenByState(ctx context.Context, stateParam string) (dbsqlc.McpOauthToken, error)
	GetMemoryProviderByID(ctx context.Context, id pgtype.UUID) (dbsqlc.MemoryProvider, error)
	GetModelByID(ctx context.Context, id pgtype.UUID) (dbsqlc.Model, error)
	GetModelByModelID(ctx context.Context, modelID string) (dbsqlc.Model, error)
	GetModelByProviderAndModelID(ctx context.Context, arg dbsqlc.GetModelByProviderAndModelIDParams) (dbsqlc.Model, error)
	GetPendingToolApprovalByBaseHeadRequestID(ctx context.Context, arg dbsqlc.GetPendingToolApprovalByBaseHeadRequestIDParams) (dbsqlc.ToolApprovalRequest, error)
	GetPendingToolApprovalByReplyMessage(ctx context.Context, arg dbsqlc.GetPendingToolApprovalByReplyMessageParams) (dbsqlc.ToolApprovalRequest, error)
	GetPendingToolApprovalBySessionShortID(ctx context.Context, arg dbsqlc.GetPendingToolApprovalBySessionShortIDParams) (dbsqlc.ToolApprovalRequest, error)
	GetPendingToolApprovalByVisibleRequestID(ctx context.Context, arg dbsqlc.GetPendingToolApprovalByVisibleRequestIDParams) (dbsqlc.ToolApprovalRequest, error)
	GetPendingUserInputByBaseHeadRequestID(ctx context.Context, arg dbsqlc.GetPendingUserInputByBaseHeadRequestIDParams) (dbsqlc.UserInputRequest, error)
	GetPendingUserInputByReplyMessage(ctx context.Context, arg dbsqlc.GetPendingUserInputByReplyMessageParams) (dbsqlc.UserInputRequest, error)
	GetPendingUserInputBySessionShortID(ctx context.Context, arg dbsqlc.GetPendingUserInputBySessionShortIDParams) (dbsqlc.UserInputRequest, error)
	GetPendingUserInputByVisibleRequestID(ctx context.Context, arg dbsqlc.GetPendingUserInputByVisibleRequestIDParams) (dbsqlc.UserInputRequest, error)
	GetProviderByClientType(ctx context.Context, clientType string) (dbsqlc.Provider, error)
	GetProviderByID(ctx context.Context, id pgtype.UUID) (dbsqlc.Provider, error)
	GetProviderByName(ctx context.Context, name string) (dbsqlc.Provider, error)
	GetProviderOAuthTokenByProvider(ctx context.Context, providerID pgtype.UUID) (dbsqlc.ProviderOauthToken, error)
	GetProviderOAuthTokenByState(ctx context.Context, state string) (dbsqlc.ProviderOauthToken, error)
	GetScheduleByID(ctx context.Context, id pgtype.UUID) (dbsqlc.Schedule, error)
	GetSearchProviderByID(ctx context.Context, id pgtype.UUID) (dbsqlc.SearchProvider, error)
	GetSearchProviderByName(ctx context.Context, name string) (dbsqlc.SearchProvider, error)
	GetSessionByID(ctx context.Context, id pgtype.UUID) (dbsqlc.BotSession, error)
	GetSessionDiscussCursor(ctx context.Context, arg dbsqlc.GetSessionDiscussCursorParams) (dbsqlc.BotSessionDiscussCursor, error)
	GetSessionCacheStats(ctx context.Context, sessionID pgtype.UUID) (dbsqlc.GetSessionCacheStatsRow, error)
	GetSessionTurnHead(ctx context.Context, arg dbsqlc.GetSessionTurnHeadParams) (dbsqlc.BotSessionTurnHead, error)
	GetSessionUsedSkills(ctx context.Context, sessionID pgtype.UUID) ([]string, error)
	GetSettingsByBotID(ctx context.Context, id pgtype.UUID) (dbsqlc.GetSettingsByBotIDRow, error)
	GetSnapshotByContainerAndRuntimeName(ctx context.Context, arg dbsqlc.GetSnapshotByContainerAndRuntimeNameParams) (dbsqlc.Snapshot, error)
	GetSpeechModelWithProvider(ctx context.Context, id pgtype.UUID) (dbsqlc.GetSpeechModelWithProviderRow, error)
	GetStorageProviderByID(ctx context.Context, id pgtype.UUID) (dbsqlc.StorageProvider, error)
	GetStorageProviderByName(ctx context.Context, name string) (dbsqlc.StorageProvider, error)
	GetTokenUsageByDayAndType(ctx context.Context, arg dbsqlc.GetTokenUsageByDayAndTypeParams) ([]dbsqlc.GetTokenUsageByDayAndTypeRow, error)
	GetTokenUsageByModel(ctx context.Context, arg dbsqlc.GetTokenUsageByModelParams) ([]dbsqlc.GetTokenUsageByModelRow, error)
	GetToolApprovalRequest(ctx context.Context, id pgtype.UUID) (dbsqlc.ToolApprovalRequest, error)
	GetUserInputRequest(ctx context.Context, id pgtype.UUID) (dbsqlc.UserInputRequest, error)
	GetUserInputRequestBySessionToolCall(ctx context.Context, arg dbsqlc.GetUserInputRequestBySessionToolCallParams) (dbsqlc.UserInputRequest, error)
	GetUserInputRequestBySessionToolCallTurn(ctx context.Context, arg dbsqlc.GetUserInputRequestBySessionToolCallTurnParams) (dbsqlc.UserInputRequest, error)
	GetTranscriptionModelWithProvider(ctx context.Context, id pgtype.UUID) (dbsqlc.GetTranscriptionModelWithProviderRow, error)
	GetUserByID(ctx context.Context, id pgtype.UUID) (dbsqlc.User, error)
	GetVideoModelWithProvider(ctx context.Context, id pgtype.UUID) (dbsqlc.GetVideoModelWithProviderRow, error)
	GetUserChannelBinding(ctx context.Context, arg dbsqlc.GetUserChannelBindingParams) (dbsqlc.UserChannelBinding, error)
	GetUserProviderOAuthToken(ctx context.Context, arg dbsqlc.GetUserProviderOAuthTokenParams) (dbsqlc.UserProviderOauthToken, error)
	GetUserProviderOAuthTokenByState(ctx context.Context, state string) (dbsqlc.UserProviderOauthToken, error)
	GetVersionSnapshotRuntimeName(ctx context.Context, arg dbsqlc.GetVersionSnapshotRuntimeNameParams) (string, error)
	GetNextTurnMessageSeq(ctx context.Context, turnID pgtype.UUID) (int64, error)
	GetVisibleUserMessageTurnForRewrite(ctx context.Context, arg dbsqlc.GetVisibleUserMessageTurnForRewriteParams) (dbsqlc.GetVisibleUserMessageTurnForRewriteRow, error)
	IncrementScheduleCalls(ctx context.Context, id pgtype.UUID) (dbsqlc.Schedule, error)
	InsertLifecycleEvent(ctx context.Context, arg dbsqlc.InsertLifecycleEventParams) error
	InsertVersion(ctx context.Context, arg dbsqlc.InsertVersionParams) (dbsqlc.ContainerVersion, error)
	ListAccounts(ctx context.Context) ([]dbsqlc.User, error)
	ListActiveMessagesSince(ctx context.Context, arg dbsqlc.ListActiveMessagesSinceParams) ([]dbsqlc.ListActiveMessagesSinceRow, error)
	ListActiveMessagesSinceBySession(ctx context.Context, arg dbsqlc.ListActiveMessagesSinceBySessionParams) ([]dbsqlc.ListActiveMessagesSinceBySessionRow, error)
	ListActiveMessagesSinceByTurn(ctx context.Context, arg dbsqlc.ListActiveMessagesSinceByTurnParams) ([]dbsqlc.ListActiveMessagesSinceByTurnRow, error)
	ListAutoStartContainers(ctx context.Context) ([]dbsqlc.Container, error)
	ListBotACLRules(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.ListBotACLRulesRow, error)
	ListBotChannelConfigsByType(ctx context.Context, channelType string) ([]dbsqlc.BotChannelConfig, error)
	ListBotEmailBindings(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.BotEmailBinding, error)
	ListBotEmailBindingsByProvider(ctx context.Context, emailProviderID pgtype.UUID) ([]dbsqlc.BotEmailBinding, error)
	ListBotsByOwner(ctx context.Context, ownerUserID pgtype.UUID) ([]dbsqlc.ListBotsByOwnerRow, error)
	ListChatParticipants(ctx context.Context, chatID pgtype.UUID) ([]dbsqlc.ListChatParticipantsRow, error)
	ListChatRoutes(ctx context.Context, chatID pgtype.UUID) ([]dbsqlc.ListChatRoutesRow, error)
	ListChatsByBotAndUser(ctx context.Context, arg dbsqlc.ListChatsByBotAndUserParams) ([]dbsqlc.ListChatsByBotAndUserRow, error)
	ListCompactionLogsByBot(ctx context.Context, arg dbsqlc.ListCompactionLogsByBotParams) ([]dbsqlc.BotHistoryMessageCompact, error)
	ListCompactionLogsBySession(ctx context.Context, sessionID pgtype.UUID) ([]dbsqlc.BotHistoryMessageCompact, error)
	ListEmailOutboxByBot(ctx context.Context, arg dbsqlc.ListEmailOutboxByBotParams) ([]dbsqlc.EmailOutbox, error)
	ListEmailProviders(ctx context.Context) ([]dbsqlc.EmailProvider, error)
	ListEmailProvidersByProvider(ctx context.Context, provider string) ([]dbsqlc.EmailProvider, error)
	ListEmailProvidersByUser(ctx context.Context, userID pgtype.UUID) ([]dbsqlc.EmailProvider, error)
	ListEmailProvidersByUserAndProvider(ctx context.Context, arg dbsqlc.ListEmailProvidersByUserAndProviderParams) ([]dbsqlc.EmailProvider, error)
	ListFetchProviders(ctx context.Context) ([]dbsqlc.FetchProvider, error)
	ListFetchProvidersByProvider(ctx context.Context, provider string) ([]dbsqlc.FetchProvider, error)
	ListEnabledModels(ctx context.Context) ([]dbsqlc.Model, error)
	ListEnabledModelsByProviderClientType(ctx context.Context, clientType string) ([]dbsqlc.Model, error)
	ListEnabledModelsByType(ctx context.Context, type_ string) ([]dbsqlc.Model, error)
	ListEnabledSchedules(ctx context.Context) ([]dbsqlc.Schedule, error)
	ListHeartbeatEnabledBots(ctx context.Context) ([]dbsqlc.ListHeartbeatEnabledBotsRow, error)
	ListHeartbeatLogsByBot(ctx context.Context, arg dbsqlc.ListHeartbeatLogsByBotParams) ([]dbsqlc.ListHeartbeatLogsByBotRow, error)
	ListBotPluginInstallations(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.BotPluginInstallation, error)
	ListBotPluginResources(ctx context.Context, installationID pgtype.UUID) ([]dbsqlc.BotPluginResource, error)
	ListMCPConnectionsByBotID(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.McpConnection, error)
	ListMemoryProviders(ctx context.Context) ([]dbsqlc.MemoryProvider, error)
	ListMessageAssets(ctx context.Context, messageID pgtype.UUID) ([]dbsqlc.ListMessageAssetsRow, error)
	ListMessageAssetsBatch(ctx context.Context, messageIds []pgtype.UUID) ([]dbsqlc.ListMessageAssetsBatchRow, error)
	ListMessages(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.ListMessagesRow, error)
	GetMessageByExternalIDBySession(ctx context.Context, arg dbsqlc.GetMessageByExternalIDBySessionParams) (dbsqlc.GetMessageByExternalIDBySessionRow, error)
	ListMessagesAfterBySession(ctx context.Context, arg dbsqlc.ListMessagesAfterBySessionParams) ([]dbsqlc.ListMessagesAfterBySessionRow, error)
	ListMessagesBefore(ctx context.Context, arg dbsqlc.ListMessagesBeforeParams) ([]dbsqlc.ListMessagesBeforeRow, error)
	ListMessagesBeforeBySession(ctx context.Context, arg dbsqlc.ListMessagesBeforeBySessionParams) ([]dbsqlc.ListMessagesBeforeBySessionRow, error)
	ListMessagesBySession(ctx context.Context, sessionID pgtype.UUID) ([]dbsqlc.ListMessagesBySessionRow, error)
	ListMessagesLatest(ctx context.Context, arg dbsqlc.ListMessagesLatestParams) ([]dbsqlc.ListMessagesLatestRow, error)
	ListMessagesLatestBySession(ctx context.Context, arg dbsqlc.ListMessagesLatestBySessionParams) ([]dbsqlc.ListMessagesLatestBySessionRow, error)
	ListMessagesSince(ctx context.Context, arg dbsqlc.ListMessagesSinceParams) ([]dbsqlc.ListMessagesSinceRow, error)
	ListMessagesSinceBySession(ctx context.Context, arg dbsqlc.ListMessagesSinceBySessionParams) ([]dbsqlc.ListMessagesSinceBySessionRow, error)
	GetSessionTurnAncestorMatch(ctx context.Context, arg dbsqlc.GetSessionTurnAncestorMatchParams) (pgtype.UUID, error)
	ListSessionTurnPathIDs(ctx context.Context, headTurnID pgtype.UUID) ([]pgtype.UUID, error)
	ListSessionTurnSiblings(ctx context.Context, arg dbsqlc.ListSessionTurnSiblingsParams) ([]dbsqlc.ListSessionTurnSiblingsRow, error)
	ResolveSessionTurnHead(ctx context.Context, arg dbsqlc.ResolveSessionTurnHeadParams) (pgtype.UUID, error)
	ListModels(ctx context.Context) ([]dbsqlc.Model, error)
	ListModelsByModelID(ctx context.Context, modelID string) ([]dbsqlc.Model, error)
	ListModelsByProviderClientType(ctx context.Context, clientType string) ([]dbsqlc.Model, error)
	ListModelsByProviderID(ctx context.Context, providerID pgtype.UUID) ([]dbsqlc.Model, error)
	ListModelsByProviderIDAndType(ctx context.Context, arg dbsqlc.ListModelsByProviderIDAndTypeParams) ([]dbsqlc.Model, error)
	ListModelsByType(ctx context.Context, type_ string) ([]dbsqlc.Model, error)
	ListModelVariantsByModelUUID(ctx context.Context, modelUuid pgtype.UUID) ([]dbsqlc.ModelVariant, error)
	ListObservedConversationsByChannelIdentity(ctx context.Context, arg dbsqlc.ListObservedConversationsByChannelIdentityParams) ([]dbsqlc.ListObservedConversationsByChannelIdentityRow, error)
	ListObservedConversationsByChannelType(ctx context.Context, arg dbsqlc.ListObservedConversationsByChannelTypeParams) ([]dbsqlc.ListObservedConversationsByChannelTypeRow, error)
	ListPendingToolApprovalsBySession(ctx context.Context, arg dbsqlc.ListPendingToolApprovalsBySessionParams) ([]dbsqlc.ToolApprovalRequest, error)
	ListPendingUserInputsBySession(ctx context.Context, arg dbsqlc.ListPendingUserInputsBySessionParams) ([]dbsqlc.UserInputRequest, error)
	ListProviders(ctx context.Context) ([]dbsqlc.Provider, error)
	ListReadableBindingsByProvider(ctx context.Context, emailProviderID pgtype.UUID) ([]dbsqlc.BotEmailBinding, error)
	ListScheduleLogsByBot(ctx context.Context, arg dbsqlc.ListScheduleLogsByBotParams) ([]dbsqlc.ListScheduleLogsByBotRow, error)
	ListScheduleLogsBySchedule(ctx context.Context, arg dbsqlc.ListScheduleLogsByScheduleParams) ([]dbsqlc.ListScheduleLogsByScheduleRow, error)
	ListSchedulesByBot(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.Schedule, error)
	ListSearchProviders(ctx context.Context) ([]dbsqlc.SearchProvider, error)
	ListSearchProvidersByProvider(ctx context.Context, provider string) ([]dbsqlc.SearchProvider, error)
	ListSessionEventsByBot(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.BotSessionEvent, error)
	ListSessionEventsBySession(ctx context.Context, sessionID pgtype.UUID) ([]dbsqlc.BotSessionEvent, error)
	ListSessionEventsBySessionAfter(ctx context.Context, arg dbsqlc.ListSessionEventsBySessionAfterParams) ([]dbsqlc.BotSessionEvent, error)
	ListSessionTurnGraphTurns(ctx context.Context, sessionID pgtype.UUID) ([]dbsqlc.BotHistoryTurn, error)
	ListSessionTurnHeads(ctx context.Context, sessionID pgtype.UUID) ([]dbsqlc.BotSessionTurnHead, error)
	ListSessionDiscussCursorsByBot(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.BotSessionDiscussCursor, error)
	ListSessionsByBot(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.ListSessionsByBotRow, error)
	ListSessionsByBotAndCreatedByUser(ctx context.Context, arg dbsqlc.ListSessionsByBotAndCreatedByUserParams) ([]dbsqlc.ListSessionsByBotAndCreatedByUserRow, error)
	ListSessionsByBotAndCreatedByUserPaged(ctx context.Context, arg dbsqlc.ListSessionsByBotAndCreatedByUserPagedParams) ([]dbsqlc.ListSessionsByBotAndCreatedByUserPagedRow, error)
	ListSessionsByBotPaged(ctx context.Context, arg dbsqlc.ListSessionsByBotPagedParams) ([]dbsqlc.ListSessionsByBotPagedRow, error)
	ListSessionsByRoute(ctx context.Context, routeID pgtype.UUID) ([]dbsqlc.BotSession, error)
	ListSnapshotsByContainerID(ctx context.Context, containerID string) ([]dbsqlc.Snapshot, error)
	ListSnapshotsWithVersionByContainerID(ctx context.Context, containerID string) ([]dbsqlc.ListSnapshotsWithVersionByContainerIDRow, error)
	ListSpeechModels(ctx context.Context) ([]dbsqlc.ListSpeechModelsRow, error)
	ListSpeechModelsByProviderID(ctx context.Context, providerID pgtype.UUID) ([]dbsqlc.Model, error)
	ListSpeechProviders(ctx context.Context) ([]dbsqlc.Provider, error)
	ListStorageProviders(ctx context.Context) ([]dbsqlc.StorageProvider, error)
	ListSubagentSessionsByParent(ctx context.Context, parentSessionID pgtype.UUID) ([]dbsqlc.BotSession, error)
	ListThreadsByParent(ctx context.Context, id pgtype.UUID) ([]dbsqlc.ListThreadsByParentRow, error)
	ListTokenUsageRecords(ctx context.Context, arg dbsqlc.ListTokenUsageRecordsParams) ([]dbsqlc.ListTokenUsageRecordsRow, error)
	ListToolApprovalsBySession(ctx context.Context, arg dbsqlc.ListToolApprovalsBySessionParams) ([]dbsqlc.ToolApprovalRequest, error)
	ListToolApprovalsBySessionToolCalls(ctx context.Context, arg dbsqlc.ListToolApprovalsBySessionToolCallsParams) ([]dbsqlc.ToolApprovalRequest, error)
	ListToolApprovalsBySessionTurnGraph(ctx context.Context, arg dbsqlc.ListToolApprovalsBySessionTurnGraphParams) ([]dbsqlc.ToolApprovalRequest, error)
	ListUserInputsBySession(ctx context.Context, arg dbsqlc.ListUserInputsBySessionParams) ([]dbsqlc.UserInputRequest, error)
	ListUserInputsBySessionToolCalls(ctx context.Context, arg dbsqlc.ListUserInputsBySessionToolCallsParams) ([]dbsqlc.UserInputRequest, error)
	ListUserInputsBySessionTurnGraph(ctx context.Context, arg dbsqlc.ListUserInputsBySessionTurnGraphParams) ([]dbsqlc.UserInputRequest, error)
	ListTranscriptionModels(ctx context.Context) ([]dbsqlc.ListTranscriptionModelsRow, error)
	ListTranscriptionModelsByProviderID(ctx context.Context, providerID pgtype.UUID) ([]dbsqlc.Model, error)
	ListTranscriptionProviders(ctx context.Context) ([]dbsqlc.Provider, error)
	ListUncompactedMessagesBySession(ctx context.Context, sessionID pgtype.UUID) ([]dbsqlc.ListUncompactedMessagesBySessionRow, error)
	ListVideoModels(ctx context.Context) ([]dbsqlc.ListVideoModelsRow, error)
	ListVideoModelsByProviderID(ctx context.Context, providerID pgtype.UUID) ([]dbsqlc.Model, error)
	ListVideoProviders(ctx context.Context) ([]dbsqlc.Provider, error)
	ListUserChannelBindingsByPlatform(ctx context.Context, channelType string) ([]dbsqlc.UserChannelBinding, error)
	ListVersionsByContainerID(ctx context.Context, containerID string) ([]dbsqlc.ListVersionsByContainerIDRow, error)
	ListVisibleChatsByBotAndUser(ctx context.Context, arg dbsqlc.ListVisibleChatsByBotAndUserParams) ([]dbsqlc.ListVisibleChatsByBotAndUserRow, error)
	MarkMessagesCompacted(ctx context.Context, arg dbsqlc.MarkMessagesCompactedParams) error
	NextVersion(ctx context.Context, containerID string) (int32, error)
	RejectToolApprovalRequest(ctx context.Context, arg dbsqlc.RejectToolApprovalRequestParams) (dbsqlc.ToolApprovalRequest, error)
	RedeemChannelLinkCode(ctx context.Context, arg dbsqlc.RedeemChannelLinkCodeParams) (dbsqlc.UserChannelIdentityBinding, error)
	RemoveChatParticipant(ctx context.Context, arg dbsqlc.RemoveChatParticipantParams) error
	ReplaceSessionTurnHead(ctx context.Context, arg dbsqlc.ReplaceSessionTurnHeadParams) (dbsqlc.BotSessionTurnHead, error)
	SaveMatrixSyncSinceToken(ctx context.Context, arg dbsqlc.SaveMatrixSyncSinceTokenParams) (int64, error)
	SearchAccounts(ctx context.Context, arg dbsqlc.SearchAccountsParams) ([]dbsqlc.User, error)
	SearchChannelIdentities(ctx context.Context, arg dbsqlc.SearchChannelIdentitiesParams) ([]dbsqlc.ChannelIdentity, error)
	SearchMessages(ctx context.Context, arg dbsqlc.SearchMessagesParams) ([]dbsqlc.SearchMessagesRow, error)
	SetBotACLDefaultEffect(ctx context.Context, arg dbsqlc.SetBotACLDefaultEffectParams) error
	SetRouteActiveSession(ctx context.Context, arg dbsqlc.SetRouteActiveSessionParams) error
	SoftDeleteSession(ctx context.Context, id pgtype.UUID) error
	SoftDeleteSessionsByBot(ctx context.Context, botID pgtype.UUID) error
	SubmitUserInputRequest(ctx context.Context, arg dbsqlc.SubmitUserInputRequestParams) (dbsqlc.UserInputRequest, error)
	TouchChat(ctx context.Context, chatID pgtype.UUID) error
	TouchSession(ctx context.Context, id pgtype.UUID) error
	UpdateAccountAdmin(ctx context.Context, arg dbsqlc.UpdateAccountAdminParams) (dbsqlc.User, error)
	UpdateAccountLastLogin(ctx context.Context, id pgtype.UUID) (dbsqlc.User, error)
	UpdateAccountPassword(ctx context.Context, arg dbsqlc.UpdateAccountPasswordParams) (dbsqlc.User, error)
	UpdateAccountProfile(ctx context.Context, arg dbsqlc.UpdateAccountProfileParams) (dbsqlc.User, error)
	UpdateBotACLRule(ctx context.Context, arg dbsqlc.UpdateBotACLRuleParams) (dbsqlc.BotAclRule, error)
	UpdateBotChannelConfigDisabled(ctx context.Context, arg dbsqlc.UpdateBotChannelConfigDisabledParams) (dbsqlc.BotChannelConfig, error)
	UpdateBotEmailBinding(ctx context.Context, arg dbsqlc.UpdateBotEmailBindingParams) (dbsqlc.BotEmailBinding, error)
	UpdateBotOwner(ctx context.Context, arg dbsqlc.UpdateBotOwnerParams) (dbsqlc.UpdateBotOwnerRow, error)
	UpdateBotProfile(ctx context.Context, arg dbsqlc.UpdateBotProfileParams) (dbsqlc.UpdateBotProfileRow, error)
	UpdateBotStatus(ctx context.Context, arg dbsqlc.UpdateBotStatusParams) error
	UpdateChatRouteMetadata(ctx context.Context, arg dbsqlc.UpdateChatRouteMetadataParams) error
	UpdateChatRouteReplyTarget(ctx context.Context, arg dbsqlc.UpdateChatRouteReplyTargetParams) error
	UpdateChatTitle(ctx context.Context, arg dbsqlc.UpdateChatTitleParams) (dbsqlc.UpdateChatTitleRow, error)
	UpdateContainerStarted(ctx context.Context, botID pgtype.UUID) error
	UpdateContainerStatus(ctx context.Context, arg dbsqlc.UpdateContainerStatusParams) error
	UpdateContainerStopped(ctx context.Context, botID pgtype.UUID) error
	UpdateEmailOAuthState(ctx context.Context, arg dbsqlc.UpdateEmailOAuthStateParams) error
	UpdateEmailOutboxFailed(ctx context.Context, arg dbsqlc.UpdateEmailOutboxFailedParams) error
	UpdateEmailOutboxSent(ctx context.Context, arg dbsqlc.UpdateEmailOutboxSentParams) error
	UpdateEmailProvider(ctx context.Context, arg dbsqlc.UpdateEmailProviderParams) (dbsqlc.EmailProvider, error)
	UpdateEmailProviderByIDAndUser(ctx context.Context, arg dbsqlc.UpdateEmailProviderByIDAndUserParams) (dbsqlc.EmailProvider, error)
	UpdateFetchProvider(ctx context.Context, arg dbsqlc.UpdateFetchProviderParams) (dbsqlc.FetchProvider, error)
	UpdateBotPluginInstallationStatus(ctx context.Context, arg dbsqlc.UpdateBotPluginInstallationStatusParams) (dbsqlc.BotPluginInstallation, error)
	UpdateMCPConnection(ctx context.Context, arg dbsqlc.UpdateMCPConnectionParams) (dbsqlc.McpConnection, error)
	UpdateMCPConnectionActive(ctx context.Context, arg dbsqlc.UpdateMCPConnectionActiveParams) error
	UpdateMCPConnectionAuthType(ctx context.Context, arg dbsqlc.UpdateMCPConnectionAuthTypeParams) error
	UpdateMCPConnectionProbeResult(ctx context.Context, arg dbsqlc.UpdateMCPConnectionProbeResultParams) error
	UpdateMCPConnectionsActiveByPlugin(ctx context.Context, arg dbsqlc.UpdateMCPConnectionsActiveByPluginParams) error
	UpdateMCPOAuthClientSecret(ctx context.Context, arg dbsqlc.UpdateMCPOAuthClientSecretParams) error
	UpdateMCPOAuthPKCEState(ctx context.Context, arg dbsqlc.UpdateMCPOAuthPKCEStateParams) error
	UpdateMCPOAuthTokens(ctx context.Context, arg dbsqlc.UpdateMCPOAuthTokensParams) error
	UpdateMemoryProvider(ctx context.Context, arg dbsqlc.UpdateMemoryProviderParams) (dbsqlc.MemoryProvider, error)
	UpdateModel(ctx context.Context, arg dbsqlc.UpdateModelParams) (dbsqlc.Model, error)
	UpdateProvider(ctx context.Context, arg dbsqlc.UpdateProviderParams) (dbsqlc.Provider, error)
	UpdateProviderOAuthState(ctx context.Context, arg dbsqlc.UpdateProviderOAuthStateParams) error
	UpdateSchedule(ctx context.Context, arg dbsqlc.UpdateScheduleParams) (dbsqlc.Schedule, error)
	UpdateSearchProvider(ctx context.Context, arg dbsqlc.UpdateSearchProviderParams) (dbsqlc.SearchProvider, error)
	UpdateSessionDefaultHeadTurn(ctx context.Context, arg dbsqlc.UpdateSessionDefaultHeadTurnParams) (dbsqlc.BotSession, error)
	UpdateSessionDefaultHeadTurnIfValid(ctx context.Context, arg dbsqlc.UpdateSessionDefaultHeadTurnIfValidParams) (dbsqlc.BotSession, error)
	UpdateSessionMetadata(ctx context.Context, arg dbsqlc.UpdateSessionMetadataParams) (dbsqlc.BotSession, error)
	UpdateSessionRestoredLinks(ctx context.Context, arg dbsqlc.UpdateSessionRestoredLinksParams) (dbsqlc.BotSession, error)
	UpdateSessionTitle(ctx context.Context, arg dbsqlc.UpdateSessionTitleParams) (dbsqlc.BotSession, error)
	UpdateSessionTypeAndMetadata(ctx context.Context, arg dbsqlc.UpdateSessionTypeAndMetadataParams) (dbsqlc.BotSession, error)
	UpdateHistoryTurnFinalAssistantMessage(ctx context.Context, arg dbsqlc.UpdateHistoryTurnFinalAssistantMessageParams) (dbsqlc.BotHistoryTurn, error)
	UpdateHistoryTurnRequestMessage(ctx context.Context, arg dbsqlc.UpdateHistoryTurnRequestMessageParams) (dbsqlc.BotHistoryTurn, error)
	UpdateToolApprovalPromptMessage(ctx context.Context, arg dbsqlc.UpdateToolApprovalPromptMessageParams) (dbsqlc.ToolApprovalRequest, error)
	UpdateUserInputAssistantMessage(ctx context.Context, arg dbsqlc.UpdateUserInputAssistantMessageParams) (dbsqlc.UserInputRequest, error)
	UpdateUserInputPromptMessage(ctx context.Context, arg dbsqlc.UpdateUserInputPromptMessageParams) (dbsqlc.UserInputRequest, error)
	UpdateUserInputToolResultMessage(ctx context.Context, arg dbsqlc.UpdateUserInputToolResultMessageParams) (dbsqlc.UserInputRequest, error)
	UpdateUserProviderOAuthState(ctx context.Context, arg dbsqlc.UpdateUserProviderOAuthStateParams) error
	UpsertAccountByUsername(ctx context.Context, arg dbsqlc.UpsertAccountByUsernameParams) (dbsqlc.User, error)
	UpsertBotChannelConfig(ctx context.Context, arg dbsqlc.UpsertBotChannelConfigParams) (dbsqlc.BotChannelConfig, error)
	UpsertBotSettings(ctx context.Context, arg dbsqlc.UpsertBotSettingsParams) (dbsqlc.UpsertBotSettingsRow, error)
	UpsertBotStorageBinding(ctx context.Context, arg dbsqlc.UpsertBotStorageBindingParams) (dbsqlc.BotStorageBinding, error)
	UpsertBotWorkspaceResourceLimits(ctx context.Context, arg dbsqlc.UpsertBotWorkspaceResourceLimitsParams) (dbsqlc.BotWorkspaceResourceLimit, error)
	UpsertChannelIdentityByChannelSubject(ctx context.Context, arg dbsqlc.UpsertChannelIdentityByChannelSubjectParams) (dbsqlc.ChannelIdentity, error)
	UpsertChatSettings(ctx context.Context, arg dbsqlc.UpsertChatSettingsParams) (dbsqlc.UpsertChatSettingsRow, error)
	UpsertContainer(ctx context.Context, arg dbsqlc.UpsertContainerParams) error
	UpsertEmailOAuthToken(ctx context.Context, arg dbsqlc.UpsertEmailOAuthTokenParams) (dbsqlc.EmailOauthToken, error)
	UpsertMCPConnectionByName(ctx context.Context, arg dbsqlc.UpsertMCPConnectionByNameParams) (dbsqlc.McpConnection, error)
	UpsertBotPluginResource(ctx context.Context, arg dbsqlc.UpsertBotPluginResourceParams) (dbsqlc.BotPluginResource, error)
	UpsertMCPOAuthDiscovery(ctx context.Context, arg dbsqlc.UpsertMCPOAuthDiscoveryParams) (dbsqlc.McpOauthToken, error)
	UpsertProviderOAuthToken(ctx context.Context, arg dbsqlc.UpsertProviderOAuthTokenParams) (dbsqlc.ProviderOauthToken, error)
	UpsertRegistryModel(ctx context.Context, arg dbsqlc.UpsertRegistryModelParams) (dbsqlc.Model, error)
	UpsertRegistryProvider(ctx context.Context, arg dbsqlc.UpsertRegistryProviderParams) (dbsqlc.Provider, error)
	UpsertSessionDiscussCursor(ctx context.Context, arg dbsqlc.UpsertSessionDiscussCursorParams) (dbsqlc.BotSessionDiscussCursor, error)
	UpsertSnapshot(ctx context.Context, arg dbsqlc.UpsertSnapshotParams) (dbsqlc.Snapshot, error)
	UpsertUserChannelBinding(ctx context.Context, arg dbsqlc.UpsertUserChannelBindingParams) (dbsqlc.UserChannelBinding, error)
	UpsertUserProviderOAuthToken(ctx context.Context, arg dbsqlc.UpsertUserProviderOAuthTokenParams) (dbsqlc.UserProviderOauthToken, error)
	WithTx(tx pgx.Tx) Queries
}
