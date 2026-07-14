package postgresstore

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

// Clear-refs delete overrides (schema contract §8.1-§8.2).
//
// These parent tables had ON DELETE SET NULL children that are now ON DELETE
// RESTRICT. Deleting a parent row therefore requires NULLing the referencing
// child columns FIRST, in the SAME transaction. A single-statement CTE
// clear-then-delete does NOT work — PostgreSQL's FK RESTRICT check on the DELETE
// does not observe a same-statement CTE UPDATE — so the clears and the delete
// run as separate statements inside InTx (which degrades to inline execution
// when already inside a transaction, preserving atomicity in both cases).
//
// The clears are scoped to the current tenant by app.current_tenant_id() and
// never touch tenant_id; only the original nullable reference column is cleared.

// clearThenDelete runs the given clear funcs then the delete, atomically.
func (q *Queries) clearThenDelete(ctx context.Context, work func(dbstore.Queries) error) error {
	return q.InTx(ctx, work)
}

func asStore(qi dbstore.Queries) *Queries {
	// Inside InTx the argument is a *Queries; this cast is always valid here.
	return qi.(*Queries)
}

// --- models (13 children) ---

func (q *Queries) DeleteModel(ctx context.Context, id pgtype.UUID) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		if err := clearModelRefs(ctx, asStore(s), id); err != nil {
			return err
		}
		return asStore(s).Queries.DeleteModel(ctx, id)
	})
}

func (q *Queries) DeleteModelByModelID(ctx context.Context, modelID string) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		st := asStore(s)
		row, err := st.Queries.GetModelByModelID(ctx, modelID)
		if err != nil {
			return err
		}
		if err := clearModelRefs(ctx, st, row.ID); err != nil {
			return err
		}
		return st.Queries.DeleteModelByModelID(ctx, modelID)
	})
}

func (q *Queries) DeleteModelByProviderIDAndModelID(ctx context.Context, arg dbsqlc.DeleteModelByProviderIDAndModelIDParams) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		st := asStore(s)
		row, err := st.Queries.GetModelByProviderAndModelID(ctx, dbsqlc.GetModelByProviderAndModelIDParams{
			ProviderID: arg.ProviderID,
			ModelID:    arg.ModelID,
		})
		if err != nil {
			return err
		}
		if err := clearModelRefs(ctx, st, row.ID); err != nil {
			return err
		}
		return st.Queries.DeleteModelByProviderIDAndModelID(ctx, arg)
	})
}

func (q *Queries) DeleteModelByProviderAndType(ctx context.Context, arg dbsqlc.DeleteModelByProviderAndTypeParams) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		st := asStore(s)
		row, err := st.Queries.GetModelByProviderAndModelID(ctx, dbsqlc.GetModelByProviderAndModelIDParams{
			ProviderID: arg.ProviderID,
			ModelID:    arg.ModelID,
		})
		if err != nil {
			return err
		}
		if err := clearModelRefs(ctx, st, row.ID); err != nil {
			return err
		}
		return st.Queries.DeleteModelByProviderAndType(ctx, arg)
	})
}

func clearModelRefs(ctx context.Context, s *Queries, modelID pgtype.UUID) error {
	for _, clear := range []func(context.Context, pgtype.UUID) error{
		s.Queries.ClearRefBotsChatModelId,
		s.Queries.ClearRefBotsHeartbeatModelId,
		s.Queries.ClearRefBotsCompactionModelId,
		s.Queries.ClearRefBotsTitleModelId,
		s.Queries.ClearRefBotsImageModelId,
		s.Queries.ClearRefBotsDiscussProbeModelId,
		s.Queries.ClearRefBotsTtsModelId,
		s.Queries.ClearRefBotsTranscriptionModelId,
		s.Queries.ClearRefBotsVideoModelId,
		s.Queries.ClearRefBotHistoryMessagesModelId,
		s.Queries.ClearRefBotHeartbeatLogsModelId,
		s.Queries.ClearRefBotHistoryMessageCompactsModelId,
		s.Queries.ClearRefScheduleLogsModelId,
	} {
		if err := clear(ctx, modelID); err != nil {
			return err
		}
	}
	return nil
}

// --- single-child providers ---

func (q *Queries) DeleteFetchProvider(ctx context.Context, id pgtype.UUID) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		st := asStore(s)
		if err := st.Queries.ClearRefBotsFetchProviderId(ctx, id); err != nil {
			return err
		}
		return st.Queries.DeleteFetchProvider(ctx, id)
	})
}

func (q *Queries) DeleteSearchProvider(ctx context.Context, id pgtype.UUID) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		st := asStore(s)
		if err := st.Queries.ClearRefBotsSearchProviderId(ctx, id); err != nil {
			return err
		}
		return st.Queries.DeleteSearchProvider(ctx, id)
	})
}

func (q *Queries) DeleteMemoryProvider(ctx context.Context, id pgtype.UUID) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		st := asStore(s)
		if err := st.Queries.ClearRefBotsMemoryProviderId(ctx, id); err != nil {
			return err
		}
		return st.Queries.DeleteMemoryProvider(ctx, id)
	})
}

func (q *Queries) DeleteBotPluginInstallation(ctx context.Context, arg dbsqlc.DeleteBotPluginInstallationParams) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		st := asStore(s)
		row, err := st.Queries.GetBotPluginInstallationByID(ctx, dbsqlc.GetBotPluginInstallationByIDParams{
			BotID: arg.BotID,
			ID:    arg.ID,
		})
		if err != nil {
			return err
		}
		if err := st.Queries.ClearRefMcpConnectionsManagedByPluginInstallationId(ctx, row.ID); err != nil {
			return err
		}
		return st.Queries.DeleteBotPluginInstallation(ctx, arg)
	})
}

// --- bot_channel_configs ---

func (q *Queries) DeleteBotChannelConfig(ctx context.Context, arg dbsqlc.DeleteBotChannelConfigParams) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		st := asStore(s)
		cfg, err := st.Queries.GetBotChannelConfig(ctx, dbsqlc.GetBotChannelConfigParams{
			BotID:       arg.BotID,
			ChannelType: arg.ChannelType,
		})
		if err != nil {
			return err
		}
		if err := st.Queries.ClearRefBotChannelRoutesChannelConfigId(ctx, cfg.ID); err != nil {
			return err
		}
		return st.Queries.DeleteBotChannelConfig(ctx, arg)
	})
}

// --- bot_channel_routes (DeleteChatRoute by route id) ---

func (q *Queries) DeleteChatRoute(ctx context.Context, id pgtype.UUID) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		st := asStore(s)
		for _, clear := range []func(context.Context, pgtype.UUID) error{
			st.Queries.ClearRefBotSessionsRouteId,
			st.Queries.ClearRefBotSessionDiscussCursorsRouteId,
			st.Queries.ClearRefToolApprovalRequestsRouteId,
			st.Queries.ClearRefUserInputRequestsRouteId,
		} {
			if err := clear(ctx, id); err != nil {
				return err
			}
		}
		return st.Queries.DeleteChatRoute(ctx, id)
	})
}

// --- bot_session_events (by bot) ---

func (q *Queries) DeleteSessionEventsByBot(ctx context.Context, botID pgtype.UUID) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		st := asStore(s)
		if err := st.Queries.ClearRefBotHistoryMessagesEventIdByBot(ctx, botID); err != nil {
			return err
		}
		return st.Queries.DeleteSessionEventsByBot(ctx, botID)
	})
}

// --- bot_history_message_compacts (by bot) ---

func (q *Queries) DeleteCompactionLogsByBot(ctx context.Context, botID pgtype.UUID) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		st := asStore(s)
		if err := st.Queries.ClearRefBotHistoryMessagesCompactIdByBot(ctx, botID); err != nil {
			return err
		}
		return st.Queries.DeleteCompactionLogsByBot(ctx, botID)
	})
}

// --- bot_history_messages (by bot / session / ids) ---

func (q *Queries) DeleteMessagesByBot(ctx context.Context, botID pgtype.UUID) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		st := asStore(s)
		for _, clear := range []func(context.Context, pgtype.UUID) error{
			st.Queries.ClearRefToolApprovalRequestedMessageIdByBot,
			st.Queries.ClearRefToolApprovalPromptMessageIdByBot,
			st.Queries.ClearRefUserInputAssistantMessageIdByBot,
			st.Queries.ClearRefUserInputToolResultMessageIdByBot,
			st.Queries.ClearRefUserInputPromptMessageIdByBot,
		} {
			if err := clear(ctx, botID); err != nil {
				return err
			}
		}
		return st.Queries.DeleteMessagesByBot(ctx, botID)
	})
}

func (q *Queries) DeleteMessagesBySession(ctx context.Context, sessionID pgtype.UUID) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		st := asStore(s)
		for _, clear := range []func(context.Context, pgtype.UUID) error{
			st.Queries.ClearRefToolApprovalRequestedMessageIdBySession,
			st.Queries.ClearRefToolApprovalPromptMessageIdBySession,
			st.Queries.ClearRefUserInputAssistantMessageIdBySession,
			st.Queries.ClearRefUserInputToolResultMessageIdBySession,
			st.Queries.ClearRefUserInputPromptMessageIdBySession,
		} {
			if err := clear(ctx, sessionID); err != nil {
				return err
			}
		}
		return st.Queries.DeleteMessagesBySession(ctx, sessionID)
	})
}

func (q *Queries) DeleteMessagesByIDs(ctx context.Context, ids []pgtype.UUID) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		st := asStore(s)
		for _, clear := range []func(context.Context, []pgtype.UUID) error{
			st.Queries.ClearRefToolApprovalRequestedMessageIdByIDs,
			st.Queries.ClearRefToolApprovalPromptMessageIdByIDs,
			st.Queries.ClearRefUserInputAssistantMessageIdByIDs,
			st.Queries.ClearRefUserInputToolResultMessageIdByIDs,
			st.Queries.ClearRefUserInputPromptMessageIdByIDs,
		} {
			if err := clear(ctx, ids); err != nil {
				return err
			}
		}
		return st.Queries.DeleteMessagesByIDs(ctx, ids)
	})
}

// --- DeleteChat (bot's messages + sessions + routes) ---

func (q *Queries) DeleteChat(ctx context.Context, chatID pgtype.UUID) error {
	return q.clearThenDelete(ctx, func(s dbstore.Queries) error {
		st := asStore(s)
		for _, clear := range []func(context.Context, pgtype.UUID) error{
			// children of routes
			st.Queries.ClearRefBotSessionDiscussCursorsRouteIdByBot,
			st.Queries.ClearRefToolApprovalRouteIdByBot,
			st.Queries.ClearRefUserInputRouteIdByBot,
			// external children of messages
			st.Queries.ClearRefToolApprovalRequestedMessageIdByBot,
			st.Queries.ClearRefToolApprovalPromptMessageIdByBot,
			st.Queries.ClearRefUserInputAssistantMessageIdByBot,
			st.Queries.ClearRefUserInputToolResultMessageIdByBot,
			st.Queries.ClearRefUserInputPromptMessageIdByBot,
			// external children of sessions
			st.Queries.ClearRefBotHeartbeatLogsSessionIdByBot,
			st.Queries.ClearRefBotHistoryMessageCompactsSessionIdByBot,
			st.Queries.ClearRefScheduleLogsSessionIdByBot,
			// in-set cross/self refs
			st.Queries.ClearRefBotChannelRoutesActiveSessionIdByBot,
			st.Queries.ClearRefBotSessionsRouteIdByBot,
			st.Queries.ClearRefBotSessionsParentSessionIdByBot,
		} {
			if err := clear(ctx, chatID); err != nil {
				return err
			}
		}
		return st.Queries.DeleteChat(ctx, chatID)
	})
}
