package postgresstore

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func (q *Queries) CountCompactionLogsByBot(ctx context.Context, botID pgtype.UUID) (int64, error) {
	return q.Queries.CountCompactionLogsByBot(ctx, dbsqlc.CountCompactionLogsByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) CountEmailOutboxByBot(ctx context.Context, botID pgtype.UUID) (int64, error) {
	return q.Queries.CountEmailOutboxByBot(ctx, dbsqlc.CountEmailOutboxByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) CountHeartbeatLogsByBot(ctx context.Context, botID pgtype.UUID) (int64, error) {
	return q.Queries.CountHeartbeatLogsByBot(ctx, dbsqlc.CountHeartbeatLogsByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) CountMessageAssetsByBot(ctx context.Context, botID pgtype.UUID) (int64, error) {
	return q.Queries.CountMessageAssetsByBot(ctx, dbsqlc.CountMessageAssetsByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) CountMessagesByBot(ctx context.Context, botID pgtype.UUID) (int64, error) {
	return q.Queries.CountMessagesByBot(ctx, dbsqlc.CountMessagesByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) CountMessagesBySession(ctx context.Context, sessionID pgtype.UUID) (int64, error) {
	return q.Queries.CountMessagesBySession(ctx, dbsqlc.CountMessagesBySessionParams{
		SessionID: sessionID,
		TeamID:    teamUUIDFromContext(ctx),
	})
}

func (q *Queries) CountScheduleLogsByBot(ctx context.Context, botID pgtype.UUID) (int64, error) {
	return q.Queries.CountScheduleLogsByBot(ctx, dbsqlc.CountScheduleLogsByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) CountScheduleLogsBySchedule(ctx context.Context, scheduleID pgtype.UUID) (int64, error) {
	return q.Queries.CountScheduleLogsBySchedule(ctx, dbsqlc.CountScheduleLogsByScheduleParams{
		ScheduleID: scheduleID,
		TeamID:     teamUUIDFromContext(ctx),
	})
}

func (q *Queries) CountSessionEvents(ctx context.Context, sessionID pgtype.UUID) (int64, error) {
	return q.Queries.CountSessionEvents(ctx, dbsqlc.CountSessionEventsParams{
		SessionID: sessionID,
		TeamID:    teamUUIDFromContext(ctx),
	})
}

func (q *Queries) CreateSessionEvent(ctx context.Context, arg dbsqlc.CreateSessionEventParams) (pgtype.UUID, error) {
	if !arg.TeamID.Valid {
		arg.TeamID = teamUUIDFromContext(ctx)
	}
	return q.Queries.CreateSessionEvent(ctx, arg)
}

func (q *Queries) GetSessionDiscussCursor(ctx context.Context, arg dbsqlc.GetSessionDiscussCursorParams) (dbsqlc.BotSessionDiscussCursor, error) {
	if !arg.TeamID.Valid {
		arg.TeamID = teamUUIDFromContext(ctx)
	}
	return q.Queries.GetSessionDiscussCursor(ctx, arg)
}

func (q *Queries) UpsertSessionDiscussCursor(ctx context.Context, arg dbsqlc.UpsertSessionDiscussCursorParams) (dbsqlc.BotSessionDiscussCursor, error) {
	if !arg.TeamID.Valid {
		arg.TeamID = teamUUIDFromContext(ctx)
	}
	return q.Queries.UpsertSessionDiscussCursor(ctx, arg)
}

func (q *Queries) DeleteBotACLRuleByID(ctx context.Context, id pgtype.UUID) error {
	return q.Queries.DeleteBotACLRuleByID(ctx, dbsqlc.DeleteBotACLRuleByIDParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteBotByID(ctx context.Context, id pgtype.UUID) error {
	return q.Queries.DeleteBotByID(ctx, dbsqlc.DeleteBotByIDParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteBotUserGrantByID(ctx context.Context, id pgtype.UUID) error {
	return q.Queries.DeleteBotUserGrantByID(ctx, dbsqlc.DeleteBotUserGrantByIDParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteBotEmailBinding(ctx context.Context, id pgtype.UUID) error {
	return q.Queries.DeleteBotEmailBinding(ctx, dbsqlc.DeleteBotEmailBindingParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteChat(ctx context.Context, chatID pgtype.UUID) error {
	return q.Queries.DeleteChat(ctx, dbsqlc.DeleteChatParams{
		ChatID: chatID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteChatRoute(ctx context.Context, id pgtype.UUID) error {
	return q.Queries.DeleteChatRoute(ctx, dbsqlc.DeleteChatRouteParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteCompactionLogsByBot(ctx context.Context, botID pgtype.UUID) error {
	return q.Queries.DeleteCompactionLogsByBot(ctx, dbsqlc.DeleteCompactionLogsByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteContainerByBotID(ctx context.Context, botID pgtype.UUID) error {
	return q.Queries.DeleteContainerByBotID(ctx, dbsqlc.DeleteContainerByBotIDParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteEmailOAuthToken(ctx context.Context, emailProviderID pgtype.UUID) error {
	return q.Queries.DeleteEmailOAuthToken(ctx, dbsqlc.DeleteEmailOAuthTokenParams{
		EmailProviderID: emailProviderID,
		TeamID:          teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteEmailProvider(ctx context.Context, id pgtype.UUID) error {
	return q.Queries.DeleteEmailProvider(ctx, dbsqlc.DeleteEmailProviderParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteHeartbeatLogsByBot(ctx context.Context, botID pgtype.UUID) error {
	return q.Queries.DeleteHeartbeatLogsByBot(ctx, dbsqlc.DeleteHeartbeatLogsByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteMessageAssets(ctx context.Context, messageID pgtype.UUID) error {
	return q.Queries.DeleteMessageAssets(ctx, dbsqlc.DeleteMessageAssetsParams{
		MessageID: messageID,
		TeamID:    teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteMessagesByBot(ctx context.Context, botID pgtype.UUID) error {
	return q.Queries.DeleteMessagesByBot(ctx, dbsqlc.DeleteMessagesByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteMessagesByIDs(ctx context.Context, ids []pgtype.UUID) error {
	return q.Queries.DeleteMessagesByIDs(ctx, dbsqlc.DeleteMessagesByIDsParams{
		Ids:    ids,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteMessagesBySession(ctx context.Context, sessionID pgtype.UUID) error {
	return q.Queries.DeleteMessagesBySession(ctx, dbsqlc.DeleteMessagesBySessionParams{
		SessionID: sessionID,
		TeamID:    teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteSchedule(ctx context.Context, id pgtype.UUID) error {
	return q.Queries.DeleteSchedule(ctx, dbsqlc.DeleteScheduleParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteScheduleLogsByBot(ctx context.Context, botID pgtype.UUID) error {
	return q.Queries.DeleteScheduleLogsByBot(ctx, dbsqlc.DeleteScheduleLogsByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteScheduleLogsBySchedule(ctx context.Context, scheduleID pgtype.UUID) error {
	return q.Queries.DeleteScheduleLogsBySchedule(ctx, dbsqlc.DeleteScheduleLogsByScheduleParams{
		ScheduleID: scheduleID,
		TeamID:     teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteSessionDiscussCursorsByBot(ctx context.Context, botID pgtype.UUID) error {
	return q.Queries.DeleteSessionDiscussCursorsByBot(ctx, dbsqlc.DeleteSessionDiscussCursorsByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) DeleteSessionEventsByBot(ctx context.Context, botID pgtype.UUID) error {
	return q.Queries.DeleteSessionEventsByBot(ctx, dbsqlc.DeleteSessionEventsByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetBotACLDefaultEffect(ctx context.Context, id pgtype.UUID) (string, error) {
	return q.Queries.GetBotACLDefaultEffect(ctx, dbsqlc.GetBotACLDefaultEffectParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetBotEmailBindingByID(ctx context.Context, id pgtype.UUID) (dbsqlc.BotEmailBinding, error) {
	return q.Queries.GetBotEmailBindingByID(ctx, dbsqlc.GetBotEmailBindingByIDParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetActiveSessionForRoute(ctx context.Context, routeID pgtype.UUID) (dbsqlc.BotSession, error) {
	return q.Queries.GetActiveSessionForRoute(ctx, dbsqlc.GetActiveSessionForRouteParams{
		RouteID: routeID,
		TeamID:  teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetBotByID(ctx context.Context, id pgtype.UUID) (dbsqlc.GetBotByIDRow, error) {
	return q.Queries.GetBotByID(ctx, dbsqlc.GetBotByIDParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetBotByName(ctx context.Context, name string) (dbsqlc.GetBotByNameRow, error) {
	return q.Queries.GetBotByName(ctx, dbsqlc.GetBotByNameParams{
		Name:   name,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetBotStorageBinding(ctx context.Context, botID pgtype.UUID) (dbsqlc.BotStorageBinding, error) {
	return q.Queries.GetBotStorageBinding(ctx, dbsqlc.GetBotStorageBindingParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetBotUserGrantByID(ctx context.Context, id pgtype.UUID) (dbsqlc.BotUserGrant, error) {
	return q.Queries.GetBotUserGrantByID(ctx, dbsqlc.GetBotUserGrantByIDParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetBotWorkspaceResourceLimits(ctx context.Context, botID pgtype.UUID) (dbsqlc.BotWorkspaceResourceLimit, error) {
	return q.Queries.GetBotWorkspaceResourceLimits(ctx, dbsqlc.GetBotWorkspaceResourceLimitsParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetChannelIdentityByID(ctx context.Context, id pgtype.UUID) (dbsqlc.ChannelIdentity, error) {
	return q.Queries.GetChannelIdentityByID(ctx, dbsqlc.GetChannelIdentityByIDParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetChannelIdentityByIDForUpdate(ctx context.Context, id pgtype.UUID) (dbsqlc.ChannelIdentity, error) {
	return q.Queries.GetChannelIdentityByIDForUpdate(ctx, dbsqlc.GetChannelIdentityByIDForUpdateParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetChannelLinkCodeByToken(ctx context.Context, token string) (dbsqlc.ChannelLinkCode, error) {
	return q.Queries.GetChannelLinkCodeByToken(ctx, dbsqlc.GetChannelLinkCodeByTokenParams{
		Token:  token,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetChatByID(ctx context.Context, id pgtype.UUID) (dbsqlc.GetChatByIDRow, error) {
	return q.Queries.GetChatByID(ctx, dbsqlc.GetChatByIDParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetChatRouteByID(ctx context.Context, id pgtype.UUID) (dbsqlc.GetChatRouteByIDRow, error) {
	return q.Queries.GetChatRouteByID(ctx, dbsqlc.GetChatRouteByIDParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetChatSettings(ctx context.Context, id pgtype.UUID) (dbsqlc.GetChatSettingsRow, error) {
	return q.Queries.GetChatSettings(ctx, dbsqlc.GetChatSettingsParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetCompactionLogByID(ctx context.Context, id pgtype.UUID) (dbsqlc.BotHistoryMessageCompact, error) {
	return q.Queries.GetCompactionLogByID(ctx, dbsqlc.GetCompactionLogByIDParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetContainerByBotID(ctx context.Context, botID pgtype.UUID) (dbsqlc.Container, error) {
	return q.Queries.GetContainerByBotID(ctx, dbsqlc.GetContainerByBotIDParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetEmailOAuthTokenByProvider(ctx context.Context, emailProviderID pgtype.UUID) (dbsqlc.EmailOauthToken, error) {
	return q.Queries.GetEmailOAuthTokenByProvider(ctx, dbsqlc.GetEmailOAuthTokenByProviderParams{
		EmailProviderID: emailProviderID,
		TeamID:          teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetEmailOAuthTokenByState(ctx context.Context, state string) (dbsqlc.EmailOauthToken, error) {
	return q.Queries.GetEmailOAuthTokenByState(ctx, dbsqlc.GetEmailOAuthTokenByStateParams{
		State:  state,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetEmailOutboxByID(ctx context.Context, id pgtype.UUID) (dbsqlc.EmailOutbox, error) {
	return q.Queries.GetEmailOutboxByID(ctx, dbsqlc.GetEmailOutboxByIDParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetEmailProviderByID(ctx context.Context, id pgtype.UUID) (dbsqlc.EmailProvider, error) {
	return q.Queries.GetEmailProviderByID(ctx, dbsqlc.GetEmailProviderByIDParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetLatestAssistantUsage(ctx context.Context, sessionID pgtype.UUID) (int64, error) {
	return q.Queries.GetLatestAssistantUsage(ctx, dbsqlc.GetLatestAssistantUsageParams{
		SessionID: sessionID,
		TeamID:    teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetLatestSessionIDByBot(ctx context.Context, botID pgtype.UUID) (pgtype.UUID, error) {
	return q.Queries.GetLatestSessionIDByBot(ctx, dbsqlc.GetLatestSessionIDByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetScheduleByID(ctx context.Context, id pgtype.UUID) (dbsqlc.Schedule, error) {
	return q.Queries.GetScheduleByID(ctx, dbsqlc.GetScheduleByIDParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetSessionByID(ctx context.Context, id pgtype.UUID) (dbsqlc.BotSession, error) {
	return q.Queries.GetSessionByID(ctx, dbsqlc.GetSessionByIDParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetSessionCacheStats(ctx context.Context, sessionID pgtype.UUID) (dbsqlc.GetSessionCacheStatsRow, error) {
	return q.Queries.GetSessionCacheStats(ctx, dbsqlc.GetSessionCacheStatsParams{
		SessionID: sessionID,
		TeamID:    teamUUIDFromContext(ctx),
	})
}

func (q *Queries) GetSessionUsedSkills(ctx context.Context, sessionID pgtype.UUID) ([]string, error) {
	return q.Queries.GetSessionUsedSkills(ctx, dbsqlc.GetSessionUsedSkillsParams{
		SessionID: sessionID,
		TeamID:    teamUUIDFromContext(ctx),
	})
}

func (q *Queries) HideMessagesByHistoryTurn(ctx context.Context, turnID pgtype.UUID) error {
	return q.Queries.HideMessagesByHistoryTurn(ctx, dbsqlc.HideMessagesByHistoryTurnParams{
		TurnID: turnID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListBotACLRules(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.ListBotACLRulesRow, error) {
	return q.Queries.ListBotACLRules(ctx, dbsqlc.ListBotACLRulesParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListBotEmailBindings(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.BotEmailBinding, error) {
	return q.Queries.ListBotEmailBindings(ctx, dbsqlc.ListBotEmailBindingsParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListBotEmailBindingsByProvider(ctx context.Context, emailProviderID pgtype.UUID) ([]dbsqlc.BotEmailBinding, error) {
	return q.Queries.ListBotEmailBindingsByProvider(ctx, dbsqlc.ListBotEmailBindingsByProviderParams{
		EmailProviderID: emailProviderID,
		TeamID:          teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListBotUserGrants(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.ListBotUserGrantsRow, error) {
	return q.Queries.ListBotUserGrants(ctx, dbsqlc.ListBotUserGrantsParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListBotChannelConfigsByType(ctx context.Context, channelType string) ([]dbsqlc.BotChannelConfig, error) {
	return q.Queries.ListBotChannelConfigsByType(ctx, dbsqlc.ListBotChannelConfigsByTypeParams{
		ChannelType: channelType,
		TeamID:      teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListAccessibleBots(ctx context.Context, ownerUserID pgtype.UUID) ([]dbsqlc.ListAccessibleBotsRow, error) {
	return q.Queries.ListAccessibleBots(ctx, dbsqlc.ListAccessibleBotsParams{
		OwnerUserID: ownerUserID,
		TeamID:      teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListAutoStartContainers(ctx context.Context) ([]dbsqlc.Container, error) {
	return q.Queries.ListAutoStartContainers(ctx, teamUUIDFromContext(ctx))
}

func (q *Queries) ListBotsByOwner(ctx context.Context, ownerUserID pgtype.UUID) ([]dbsqlc.ListBotsByOwnerRow, error) {
	return q.Queries.ListBotsByOwner(ctx, dbsqlc.ListBotsByOwnerParams{
		OwnerUserID: ownerUserID,
		TeamID:      teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListChatParticipants(ctx context.Context, chatID pgtype.UUID) ([]dbsqlc.ListChatParticipantsRow, error) {
	return q.Queries.ListChatParticipants(ctx, dbsqlc.ListChatParticipantsParams{
		ChatID: chatID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListChatRoutes(ctx context.Context, chatID pgtype.UUID) ([]dbsqlc.ListChatRoutesRow, error) {
	return q.Queries.ListChatRoutes(ctx, dbsqlc.ListChatRoutesParams{
		ChatID: chatID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListChannelIdentityBindings(ctx context.Context) ([]dbsqlc.ListChannelIdentityBindingsRow, error) {
	return q.Queries.ListChannelIdentityBindings(ctx, teamUUIDFromContext(ctx))
}

func (q *Queries) ListChannelIdentityBindingsForBot(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.ListChannelIdentityBindingsForBotRow, error) {
	return q.Queries.ListChannelIdentityBindingsForBot(ctx, botID)
}

func (q *Queries) ListChannelIdentityBindingsForUser(ctx context.Context, userID pgtype.UUID) ([]dbsqlc.ListChannelIdentityBindingsForUserRow, error) {
	return q.Queries.ListChannelIdentityBindingsForUser(ctx, dbsqlc.ListChannelIdentityBindingsForUserParams{
		UserID: userID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListCompactionLogsBySession(ctx context.Context, sessionID pgtype.UUID) ([]dbsqlc.BotHistoryMessageCompact, error) {
	return q.Queries.ListCompactionLogsBySession(ctx, dbsqlc.ListCompactionLogsBySessionParams{
		SessionID: sessionID,
		TeamID:    teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListEmailProviders(ctx context.Context) ([]dbsqlc.EmailProvider, error) {
	return q.Queries.ListEmailProviders(ctx, teamUUIDFromContext(ctx))
}

func (q *Queries) ListEmailProvidersByProvider(ctx context.Context, provider string) ([]dbsqlc.EmailProvider, error) {
	return q.Queries.ListEmailProvidersByProvider(ctx, dbsqlc.ListEmailProvidersByProviderParams{
		Provider: provider,
		TeamID:   teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListEmailProvidersByUser(ctx context.Context, userID pgtype.UUID) ([]dbsqlc.EmailProvider, error) {
	return q.Queries.ListEmailProvidersByUser(ctx, dbsqlc.ListEmailProvidersByUserParams{
		UserID: userID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) LinkUnassignedMessagesAfterHistoryTurnAssistant(ctx context.Context, turnID pgtype.UUID) error {
	return q.Queries.LinkUnassignedMessagesAfterHistoryTurnAssistant(ctx, dbsqlc.LinkUnassignedMessagesAfterHistoryTurnAssistantParams{
		TurnID: turnID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListAllMessagesForBackup(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.ListAllMessagesForBackupRow, error) {
	return q.Queries.ListAllMessagesForBackup(ctx, dbsqlc.ListAllMessagesForBackupParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListEnabledSchedules(ctx context.Context) ([]dbsqlc.Schedule, error) {
	return q.Queries.ListEnabledSchedules(ctx, teamUUIDFromContext(ctx))
}

func (q *Queries) ListHeartbeatEnabledBots(ctx context.Context) ([]dbsqlc.ListHeartbeatEnabledBotsRow, error) {
	return q.Queries.ListHeartbeatEnabledBots(ctx, teamUUIDFromContext(ctx))
}

func (q *Queries) ListMessageAssets(ctx context.Context, messageID pgtype.UUID) ([]dbsqlc.ListMessageAssetsRow, error) {
	return q.Queries.ListMessageAssets(ctx, dbsqlc.ListMessageAssetsParams{
		MessageID: messageID,
		TeamID:    teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListMessageAssetsBatch(ctx context.Context, messageIds []pgtype.UUID) ([]dbsqlc.ListMessageAssetsBatchRow, error) {
	return q.Queries.ListMessageAssetsBatch(ctx, dbsqlc.ListMessageAssetsBatchParams{
		MessageIds: messageIds,
		TeamID:     teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListMessages(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.ListMessagesRow, error) {
	return q.Queries.ListMessages(ctx, dbsqlc.ListMessagesParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListMessagesBySession(ctx context.Context, sessionID pgtype.UUID) ([]dbsqlc.ListMessagesBySessionRow, error) {
	return q.Queries.ListMessagesBySession(ctx, dbsqlc.ListMessagesBySessionParams{
		SessionID: sessionID,
		TeamID:    teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListReadableBindingsByProvider(ctx context.Context, emailProviderID pgtype.UUID) ([]dbsqlc.BotEmailBinding, error) {
	return q.Queries.ListReadableBindingsByProvider(ctx, dbsqlc.ListReadableBindingsByProviderParams{
		EmailProviderID: emailProviderID,
		TeamID:          teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListSchedulesByBot(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.Schedule, error) {
	return q.Queries.ListSchedulesByBot(ctx, dbsqlc.ListSchedulesByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListSessionsByBot(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.ListSessionsByBotRow, error) {
	return q.Queries.ListSessionsByBot(ctx, dbsqlc.ListSessionsByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListSessionsByRoute(ctx context.Context, routeID pgtype.UUID) ([]dbsqlc.BotSession, error) {
	return q.Queries.ListSessionsByRoute(ctx, dbsqlc.ListSessionsByRouteParams{
		RouteID: routeID,
		TeamID:  teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListSnapshotsByContainerID(ctx context.Context, containerID string) ([]dbsqlc.Snapshot, error) {
	return q.Queries.ListSnapshotsByContainerID(ctx, dbsqlc.ListSnapshotsByContainerIDParams{
		ContainerID: containerID,
		TeamID:      teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListSnapshotsWithVersionByContainerID(ctx context.Context, containerID string) ([]dbsqlc.ListSnapshotsWithVersionByContainerIDRow, error) {
	return q.Queries.ListSnapshotsWithVersionByContainerID(ctx, dbsqlc.ListSnapshotsWithVersionByContainerIDParams{
		ContainerID: containerID,
		TeamID:      teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListSessionEventsByBot(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.BotSessionEvent, error) {
	return q.Queries.ListSessionEventsByBot(ctx, dbsqlc.ListSessionEventsByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListSessionEventsBySession(ctx context.Context, sessionID pgtype.UUID) ([]dbsqlc.BotSessionEvent, error) {
	return q.Queries.ListSessionEventsBySession(ctx, dbsqlc.ListSessionEventsBySessionParams{
		SessionID: sessionID,
		TeamID:    teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListSessionDiscussCursorsByBot(ctx context.Context, botID pgtype.UUID) ([]dbsqlc.BotSessionDiscussCursor, error) {
	return q.Queries.ListSessionDiscussCursorsByBot(ctx, dbsqlc.ListSessionDiscussCursorsByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListSubagentSessionsByParent(ctx context.Context, parentSessionID pgtype.UUID) ([]dbsqlc.BotSession, error) {
	return q.Queries.ListSubagentSessionsByParent(ctx, dbsqlc.ListSubagentSessionsByParentParams{
		ParentSessionID: parentSessionID,
		TeamID:          teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListThreadsByParent(ctx context.Context, id pgtype.UUID) ([]dbsqlc.ListThreadsByParentRow, error) {
	return q.Queries.ListThreadsByParent(ctx, dbsqlc.ListThreadsByParentParams{
		ParentChatID: id,
		TeamID:       teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListUserIDsByChannelIdentity(ctx context.Context, channelIdentityID pgtype.UUID) ([]pgtype.UUID, error) {
	return q.Queries.ListUserIDsByChannelIdentity(ctx, dbsqlc.ListUserIDsByChannelIdentityParams{
		ChannelIdentityID: channelIdentityID,
		TeamID:            teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListVersionsByContainerID(ctx context.Context, containerID string) ([]dbsqlc.ListVersionsByContainerIDRow, error) {
	return q.Queries.ListVersionsByContainerID(ctx, dbsqlc.ListVersionsByContainerIDParams{
		ContainerID: containerID,
		TeamID:      teamUUIDFromContext(ctx),
	})
}

func (q *Queries) NextVersion(ctx context.Context, containerID string) (int32, error) {
	return q.Queries.NextVersion(ctx, dbsqlc.NextVersionParams{
		ContainerID: containerID,
		TeamID:      teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListUserChannelBindingsByPlatform(ctx context.Context, channelType string) ([]dbsqlc.UserChannelBinding, error) {
	return q.Queries.ListUserChannelBindingsByPlatform(ctx, dbsqlc.ListUserChannelBindingsByPlatformParams{
		ChannelType: channelType,
		TeamID:      teamUUIDFromContext(ctx),
	})
}

func (q *Queries) ListUncompactedMessagesBySession(ctx context.Context, sessionID pgtype.UUID) ([]dbsqlc.ListUncompactedMessagesBySessionRow, error) {
	return q.Queries.ListUncompactedMessagesBySession(ctx, dbsqlc.ListUncompactedMessagesBySessionParams{
		SessionID: sessionID,
		TeamID:    teamUUIDFromContext(ctx),
	})
}

func (q *Queries) IncrementScheduleCalls(ctx context.Context, id pgtype.UUID) (dbsqlc.Schedule, error) {
	return q.Queries.IncrementScheduleCalls(ctx, dbsqlc.IncrementScheduleCallsParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) SoftDeleteSession(ctx context.Context, id pgtype.UUID) error {
	return q.Queries.SoftDeleteSession(ctx, dbsqlc.SoftDeleteSessionParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) SoftDeleteSessionsByBot(ctx context.Context, botID pgtype.UUID) error {
	return q.Queries.SoftDeleteSessionsByBot(ctx, dbsqlc.SoftDeleteSessionsByBotParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) TouchChat(ctx context.Context, chatID pgtype.UUID) error {
	return q.Queries.TouchChat(ctx, dbsqlc.TouchChatParams{
		ChatID: chatID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) TouchSession(ctx context.Context, id pgtype.UUID) error {
	return q.Queries.TouchSession(ctx, dbsqlc.TouchSessionParams{
		ID:     id,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) UpdateContainerStarted(ctx context.Context, botID pgtype.UUID) error {
	return q.Queries.UpdateContainerStarted(ctx, dbsqlc.UpdateContainerStartedParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}

func (q *Queries) UpdateContainerStopped(ctx context.Context, botID pgtype.UUID) error {
	return q.Queries.UpdateContainerStopped(ctx, dbsqlc.UpdateContainerStoppedParams{
		BotID:  botID,
		TeamID: teamUUIDFromContext(ctx),
	})
}
