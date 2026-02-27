import { getBots, deleteBotsByBotIdMessages } from '@memoh/sdk'
import type { Bot, ChatSummary } from './useChat.types'

export async function fetchBots(): Promise<Bot[]> {
  const { data } = await getBots({ throwOnError: true })
  return data?.items ?? []
}

export async function fetchChats(botId: string): Promise<ChatSummary[]> {
  const id = botId.trim()
  if (!id) return []
  return [{ id, bot_id: id, kind: 'bot' }]
}

export async function createChat(botId: string): Promise<ChatSummary> {
  const id = botId.trim()
  if (!id) throw new Error('bot id is required')
  return { id, bot_id: id, kind: 'bot' }
}

export async function deleteChat(botId: string, chatId: string): Promise<void> {
  if (botId.trim() !== chatId.trim()) {
    throw new Error('chat id must match bot id')
  }
  await deleteBotsByBotIdMessages({
    path: { bot_id: botId },
    throwOnError: true,
  })
}
