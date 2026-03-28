import { defineStore } from 'pinia'
import { useStorage } from '@vueuse/core'

export const useChatSelectionStore = defineStore('chat-selection', () => {
  const currentBotId = useStorage<string | null>('chat-bot-id', null)
  const sessionId = useStorage<string | null>('chat-session-id', null)

  function setBot(botId: string | null) {
    currentBotId.value = (botId ?? '').trim() || null
  }

  function setSession(targetSessionId: string | null) {
    sessionId.value = (targetSessionId ?? '').trim() || null
  }

  function clear() {
    currentBotId.value = null
    sessionId.value = null
  }

  return {
    currentBotId,
    sessionId,
    setBot,
    setSession,
    clear,
  }
})
