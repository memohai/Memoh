import {
  getBots,
  deleteBotsByBotIdMessages,
  getBotsByBotIdSessions,
  postBotsByBotIdSessions,
  deleteBotsByBotIdSessionsBySessionId,
  patchBotsByBotIdSessionsBySessionId,
} from '@memoh/sdk'
import type { Bot, SessionSummary } from './useChat.types'

export async function fetchBots(): Promise<Bot[]> {
  const { data } = await getBots({ throwOnError: true })
  return data?.items ?? []
}

export async function fetchSessions(botId: string): Promise<SessionSummary[]> {
  const id = botId.trim()
  if (!id) return []
  const { data } = await getBotsByBotIdSessions({
    path: { bot_id: id },
    throwOnError: true,
  })
  return (data as Record<string, unknown>)?.items as SessionSummary[] ?? []
}

export async function createSession(botId: string, title?: string): Promise<SessionSummary> {
  const id = botId.trim()
  if (!id) throw new Error('bot id is required')
  const { data } = await postBotsByBotIdSessions({
    path: { bot_id: id },
    body: { title: title ?? '', channel_type: 'web' },
    throwOnError: true,
  })
  return data as SessionSummary
}

export async function updateSessionTitle(botId: string, sessionId: string, title: string): Promise<SessionSummary> {
  const { data } = await patchBotsByBotIdSessionsBySessionId({
    path: { bot_id: botId.trim(), session_id: sessionId.trim() },
    body: { title },
    throwOnError: true,
  })
  return data as SessionSummary
}

export async function deleteSession(botId: string, sessionId: string): Promise<void> {
  await deleteBotsByBotIdSessionsBySessionId({
    path: { bot_id: botId.trim(), session_id: sessionId.trim() },
    throwOnError: true,
  })
}

export async function deleteAllMessages(botId: string): Promise<void> {
  await deleteBotsByBotIdMessages({
    path: { bot_id: botId },
    throwOnError: true,
  })
}
