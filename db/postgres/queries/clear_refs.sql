-- Clear-refs queries: NULL a child column that referenced a parent via a
-- former ON DELETE SET NULL FK (now RESTRICT). The repository runs the matching
-- clears in the SAME TRANSACTION before deleting the parent (schema contract
-- §8.1-§8.2). A single-statement CTE clear-then-delete does NOT work: the
-- DELETE's FK RESTRICT check does not observe a same-statement CTE UPDATE.

-- name: ClearRefBotChannelRoutesChannelConfigId :exec
UPDATE bot_channel_routes SET channel_config_id = NULL
WHERE tenant_id = app.current_tenant_id() AND channel_config_id = sqlc.arg(parent_id);

-- name: ClearRefBotSessionsRouteId :exec
UPDATE bot_sessions SET route_id = NULL
WHERE tenant_id = app.current_tenant_id() AND route_id = sqlc.arg(parent_id);

-- name: ClearRefBotSessionDiscussCursorsRouteId :exec
UPDATE bot_session_discuss_cursors SET route_id = NULL
WHERE tenant_id = app.current_tenant_id() AND route_id = sqlc.arg(parent_id);

-- name: ClearRefToolApprovalRequestsRouteId :exec
UPDATE tool_approval_requests SET route_id = NULL
WHERE tenant_id = app.current_tenant_id() AND route_id = sqlc.arg(parent_id);

-- name: ClearRefUserInputRequestsRouteId :exec
UPDATE user_input_requests SET route_id = NULL
WHERE tenant_id = app.current_tenant_id() AND route_id = sqlc.arg(parent_id);

-- name: ClearRefBotHistoryMessagesCompactId :exec
UPDATE bot_history_messages SET compact_id = NULL
WHERE tenant_id = app.current_tenant_id() AND compact_id = sqlc.arg(parent_id);

-- name: ClearRefToolApprovalRequestsRequestedMessageId :exec
UPDATE tool_approval_requests SET requested_message_id = NULL
WHERE tenant_id = app.current_tenant_id() AND requested_message_id = sqlc.arg(parent_id);

-- name: ClearRefToolApprovalRequestsPromptMessageId :exec
UPDATE tool_approval_requests SET prompt_message_id = NULL
WHERE tenant_id = app.current_tenant_id() AND prompt_message_id = sqlc.arg(parent_id);

-- name: ClearRefUserInputRequestsAssistantMessageId :exec
UPDATE user_input_requests SET assistant_message_id = NULL
WHERE tenant_id = app.current_tenant_id() AND assistant_message_id = sqlc.arg(parent_id);

-- name: ClearRefUserInputRequestsToolResultMessageId :exec
UPDATE user_input_requests SET tool_result_message_id = NULL
WHERE tenant_id = app.current_tenant_id() AND tool_result_message_id = sqlc.arg(parent_id);

-- name: ClearRefUserInputRequestsPromptMessageId :exec
UPDATE user_input_requests SET prompt_message_id = NULL
WHERE tenant_id = app.current_tenant_id() AND prompt_message_id = sqlc.arg(parent_id);

-- name: ClearRefMcpConnectionsManagedByPluginInstallationId :exec
UPDATE mcp_connections SET managed_by_plugin_installation_id = NULL
WHERE tenant_id = app.current_tenant_id() AND managed_by_plugin_installation_id = sqlc.arg(parent_id);

-- name: ClearRefBotHistoryMessagesEventId :exec
UPDATE bot_history_messages SET event_id = NULL
WHERE tenant_id = app.current_tenant_id() AND event_id = sqlc.arg(parent_id);

-- name: ClearRefBotSessionsParentSessionId :exec
UPDATE bot_sessions SET parent_session_id = NULL
WHERE tenant_id = app.current_tenant_id() AND parent_session_id = sqlc.arg(parent_id);

-- name: ClearRefBotChannelRoutesActiveSessionId :exec
UPDATE bot_channel_routes SET active_session_id = NULL
WHERE tenant_id = app.current_tenant_id() AND active_session_id = sqlc.arg(parent_id);

-- name: ClearRefBotHistoryMessagesSessionId :exec
UPDATE bot_history_messages SET session_id = NULL
WHERE tenant_id = app.current_tenant_id() AND session_id = sqlc.arg(parent_id);

-- name: ClearRefBotHeartbeatLogsSessionId :exec
UPDATE bot_heartbeat_logs SET session_id = NULL
WHERE tenant_id = app.current_tenant_id() AND session_id = sqlc.arg(parent_id);

-- name: ClearRefBotHistoryMessageCompactsSessionId :exec
UPDATE bot_history_message_compacts SET session_id = NULL
WHERE tenant_id = app.current_tenant_id() AND session_id = sqlc.arg(parent_id);

-- name: ClearRefScheduleLogsSessionId :exec
UPDATE schedule_logs SET session_id = NULL
WHERE tenant_id = app.current_tenant_id() AND session_id = sqlc.arg(parent_id);

-- name: ClearRefBotsFetchProviderId :exec
UPDATE bots SET fetch_provider_id = NULL
WHERE tenant_id = app.current_tenant_id() AND fetch_provider_id = sqlc.arg(parent_id);

-- name: ClearRefBotsMemoryProviderId :exec
UPDATE bots SET memory_provider_id = NULL
WHERE tenant_id = app.current_tenant_id() AND memory_provider_id = sqlc.arg(parent_id);

-- name: ClearRefBotsChatModelId :exec
UPDATE bots SET chat_model_id = NULL
WHERE tenant_id = app.current_tenant_id() AND chat_model_id = sqlc.arg(parent_id);

-- name: ClearRefBotsHeartbeatModelId :exec
UPDATE bots SET heartbeat_model_id = NULL
WHERE tenant_id = app.current_tenant_id() AND heartbeat_model_id = sqlc.arg(parent_id);

-- name: ClearRefBotsCompactionModelId :exec
UPDATE bots SET compaction_model_id = NULL
WHERE tenant_id = app.current_tenant_id() AND compaction_model_id = sqlc.arg(parent_id);

-- name: ClearRefBotsTitleModelId :exec
UPDATE bots SET title_model_id = NULL
WHERE tenant_id = app.current_tenant_id() AND title_model_id = sqlc.arg(parent_id);

-- name: ClearRefBotsImageModelId :exec
UPDATE bots SET image_model_id = NULL
WHERE tenant_id = app.current_tenant_id() AND image_model_id = sqlc.arg(parent_id);

-- name: ClearRefBotsDiscussProbeModelId :exec
UPDATE bots SET discuss_probe_model_id = NULL
WHERE tenant_id = app.current_tenant_id() AND discuss_probe_model_id = sqlc.arg(parent_id);

-- name: ClearRefBotsTtsModelId :exec
UPDATE bots SET tts_model_id = NULL
WHERE tenant_id = app.current_tenant_id() AND tts_model_id = sqlc.arg(parent_id);

-- name: ClearRefBotsTranscriptionModelId :exec
UPDATE bots SET transcription_model_id = NULL
WHERE tenant_id = app.current_tenant_id() AND transcription_model_id = sqlc.arg(parent_id);

-- name: ClearRefBotsVideoModelId :exec
UPDATE bots SET video_model_id = NULL
WHERE tenant_id = app.current_tenant_id() AND video_model_id = sqlc.arg(parent_id);

-- name: ClearRefBotHistoryMessagesModelId :exec
UPDATE bot_history_messages SET model_id = NULL
WHERE tenant_id = app.current_tenant_id() AND model_id = sqlc.arg(parent_id);

-- name: ClearRefBotHeartbeatLogsModelId :exec
UPDATE bot_heartbeat_logs SET model_id = NULL
WHERE tenant_id = app.current_tenant_id() AND model_id = sqlc.arg(parent_id);

-- name: ClearRefBotHistoryMessageCompactsModelId :exec
UPDATE bot_history_message_compacts SET model_id = NULL
WHERE tenant_id = app.current_tenant_id() AND model_id = sqlc.arg(parent_id);

-- name: ClearRefScheduleLogsModelId :exec
UPDATE schedule_logs SET model_id = NULL
WHERE tenant_id = app.current_tenant_id() AND model_id = sqlc.arg(parent_id);

-- name: ClearRefBotsSearchProviderId :exec
UPDATE bots SET search_provider_id = NULL
WHERE tenant_id = app.current_tenant_id() AND search_provider_id = sqlc.arg(parent_id);
-- ===== Scoped clear-refs for set-based parent deletes =====
-- These match the SAME row-set as the corresponding set-based delete, so every
-- referencing child of the deleted parents is NULLed first (same transaction).

-- DeleteSessionEventsByBot: NULL messages that reference this bot's events.
-- name: ClearRefBotHistoryMessagesEventIdByBot :exec
UPDATE bot_history_messages SET event_id = NULL
WHERE tenant_id = app.current_tenant_id()
  AND event_id IN (SELECT bot_session_events.id FROM bot_session_events
                    WHERE bot_session_events.tenant_id = app.current_tenant_id()
                      AND bot_session_events.bot_id = sqlc.arg(bot_id));

-- DeleteCompactionLogsByBot: NULL messages that reference this bot's compacts.
-- name: ClearRefBotHistoryMessagesCompactIdByBot :exec
UPDATE bot_history_messages SET compact_id = NULL
WHERE tenant_id = app.current_tenant_id()
  AND compact_id IN (SELECT bot_history_message_compacts.id FROM bot_history_message_compacts
                      WHERE bot_history_message_compacts.tenant_id = app.current_tenant_id()
                        AND bot_history_message_compacts.bot_id = sqlc.arg(bot_id));

-- DeleteMessagesByBot: NULL tool_approval / user_input refs to this bot's messages.
-- name: ClearRefToolApprovalRequestedMessageIdByBot :exec
UPDATE tool_approval_requests SET requested_message_id = NULL
WHERE tenant_id = app.current_tenant_id() AND bot_id = sqlc.arg(bot_id);

-- name: ClearRefToolApprovalPromptMessageIdByBot :exec
UPDATE tool_approval_requests SET prompt_message_id = NULL
WHERE tenant_id = app.current_tenant_id() AND bot_id = sqlc.arg(bot_id);

-- name: ClearRefUserInputAssistantMessageIdByBot :exec
UPDATE user_input_requests SET assistant_message_id = NULL
WHERE tenant_id = app.current_tenant_id() AND bot_id = sqlc.arg(bot_id);

-- name: ClearRefUserInputToolResultMessageIdByBot :exec
UPDATE user_input_requests SET tool_result_message_id = NULL
WHERE tenant_id = app.current_tenant_id() AND bot_id = sqlc.arg(bot_id);

-- name: ClearRefUserInputPromptMessageIdByBot :exec
UPDATE user_input_requests SET prompt_message_id = NULL
WHERE tenant_id = app.current_tenant_id() AND bot_id = sqlc.arg(bot_id);

-- DeleteMessagesBySession: NULL tool_approval / user_input refs to a session's messages.
-- name: ClearRefToolApprovalRequestedMessageIdBySession :exec
UPDATE tool_approval_requests SET requested_message_id = NULL
WHERE tenant_id = app.current_tenant_id()
  AND requested_message_id IN (SELECT bot_history_messages.id FROM bot_history_messages
                                WHERE bot_history_messages.tenant_id = app.current_tenant_id()
                                  AND bot_history_messages.session_id = sqlc.arg(session_id));

-- name: ClearRefToolApprovalPromptMessageIdBySession :exec
UPDATE tool_approval_requests SET prompt_message_id = NULL
WHERE tenant_id = app.current_tenant_id()
  AND prompt_message_id IN (SELECT bot_history_messages.id FROM bot_history_messages
                             WHERE bot_history_messages.tenant_id = app.current_tenant_id()
                               AND bot_history_messages.session_id = sqlc.arg(session_id));

-- name: ClearRefUserInputAssistantMessageIdBySession :exec
UPDATE user_input_requests SET assistant_message_id = NULL
WHERE tenant_id = app.current_tenant_id()
  AND assistant_message_id IN (SELECT bot_history_messages.id FROM bot_history_messages
                                WHERE bot_history_messages.tenant_id = app.current_tenant_id()
                                  AND bot_history_messages.session_id = sqlc.arg(session_id));

-- name: ClearRefUserInputToolResultMessageIdBySession :exec
UPDATE user_input_requests SET tool_result_message_id = NULL
WHERE tenant_id = app.current_tenant_id()
  AND tool_result_message_id IN (SELECT bot_history_messages.id FROM bot_history_messages
                                  WHERE bot_history_messages.tenant_id = app.current_tenant_id()
                                    AND bot_history_messages.session_id = sqlc.arg(session_id));

-- name: ClearRefUserInputPromptMessageIdBySession :exec
UPDATE user_input_requests SET prompt_message_id = NULL
WHERE tenant_id = app.current_tenant_id()
  AND prompt_message_id IN (SELECT bot_history_messages.id FROM bot_history_messages
                             WHERE bot_history_messages.tenant_id = app.current_tenant_id()
                               AND bot_history_messages.session_id = sqlc.arg(session_id));

-- DeleteMessagesByIDs: NULL tool_approval / user_input refs to the given message IDs.
-- name: ClearRefToolApprovalRequestedMessageIdByIDs :exec
UPDATE tool_approval_requests SET requested_message_id = NULL
WHERE tenant_id = app.current_tenant_id() AND requested_message_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ClearRefToolApprovalPromptMessageIdByIDs :exec
UPDATE tool_approval_requests SET prompt_message_id = NULL
WHERE tenant_id = app.current_tenant_id() AND prompt_message_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ClearRefUserInputAssistantMessageIdByIDs :exec
UPDATE user_input_requests SET assistant_message_id = NULL
WHERE tenant_id = app.current_tenant_id() AND assistant_message_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ClearRefUserInputToolResultMessageIdByIDs :exec
UPDATE user_input_requests SET tool_result_message_id = NULL
WHERE tenant_id = app.current_tenant_id() AND tool_result_message_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ClearRefUserInputPromptMessageIdByIDs :exec
UPDATE user_input_requests SET prompt_message_id = NULL
WHERE tenant_id = app.current_tenant_id() AND prompt_message_id = ANY(sqlc.arg(ids)::uuid[]);
-- ===== DeleteChat (deletes a bot's messages, sessions, and routes) =====
-- Clears all children referencing any of the bot's routes/sessions/messages,
-- plus the in-set cross/self refs, all scoped by bot_id.

-- children of the bot's channel routes
-- name: ClearRefBotSessionDiscussCursorsRouteIdByBot :exec
UPDATE bot_session_discuss_cursors SET route_id = NULL
WHERE tenant_id = app.current_tenant_id()
  AND route_id IN (SELECT bot_channel_routes.id FROM bot_channel_routes
                    WHERE bot_channel_routes.tenant_id = app.current_tenant_id()
                      AND bot_channel_routes.bot_id = sqlc.arg(bot_id));

-- name: ClearRefToolApprovalRouteIdByBot :exec
UPDATE tool_approval_requests SET route_id = NULL
WHERE tenant_id = app.current_tenant_id() AND bot_id = sqlc.arg(bot_id);

-- name: ClearRefUserInputRouteIdByBot :exec
UPDATE user_input_requests SET route_id = NULL
WHERE tenant_id = app.current_tenant_id() AND bot_id = sqlc.arg(bot_id);

-- children of the bot's sessions (external tables)
-- name: ClearRefBotHeartbeatLogsSessionIdByBot :exec
UPDATE bot_heartbeat_logs SET session_id = NULL
WHERE tenant_id = app.current_tenant_id() AND bot_id = sqlc.arg(bot_id);

-- name: ClearRefBotHistoryMessageCompactsSessionIdByBot :exec
UPDATE bot_history_message_compacts SET session_id = NULL
WHERE tenant_id = app.current_tenant_id() AND bot_id = sqlc.arg(bot_id);

-- name: ClearRefScheduleLogsSessionIdByBot :exec
UPDATE schedule_logs SET session_id = NULL
WHERE tenant_id = app.current_tenant_id() AND bot_id = sqlc.arg(bot_id);

-- in-set cross/self refs so the three deletes don't RESTRICT on each other
-- name: ClearRefBotChannelRoutesActiveSessionIdByBot :exec
UPDATE bot_channel_routes SET active_session_id = NULL
WHERE tenant_id = app.current_tenant_id() AND bot_id = sqlc.arg(bot_id);

-- name: ClearRefBotSessionsRouteIdByBot :exec
UPDATE bot_sessions SET route_id = NULL
WHERE tenant_id = app.current_tenant_id() AND bot_id = sqlc.arg(bot_id);

-- name: ClearRefBotSessionsParentSessionIdByBot :exec
UPDATE bot_sessions SET parent_session_id = NULL
WHERE tenant_id = app.current_tenant_id() AND bot_id = sqlc.arg(bot_id);
