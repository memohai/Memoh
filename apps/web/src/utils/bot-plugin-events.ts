export const BOT_PLUGINS_UPDATED_EVENT = 'memoh:bot-plugins-updated'

export type BotPluginsUpdatedDetail = {
  botId: string
}

export function emitBotPluginsUpdated(botId: string): void {
  const normalizedBotId = botId.trim()
  if (!normalizedBotId || typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent<BotPluginsUpdatedDetail>(BOT_PLUGINS_UPDATED_EVENT, {
    detail: { botId: normalizedBotId },
  }))
}

export function isBotPluginsUpdatedEvent(event: Event): event is CustomEvent<BotPluginsUpdatedDetail> {
  return event.type === BOT_PLUGINS_UPDATED_EVENT
}
