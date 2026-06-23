import { defineStore } from 'pinia'
import { useStorage } from '@vueuse/core'

export const useChatSelectionStore = defineStore('chat-selection', () => {
  const currentBotId = useStorage<string | null>('chat-bot-id', null)
  const sessionId = useStorage<string | null>('chat-session-id', null)
  // Did the user intentionally sit on the draft "New Session" page (vs. just never
  // having selected anything)? A null sessionId is ambiguous on reload: this flag
  // lets initialize keep the draft instead of force-opening a random session, while
  // a fresh/never-selected load (flag false) still auto-opens the latest session.
  const draftIntent = useStorage<boolean>('chat-draft-intent', false)

  function setBot(botId: string | null) {
    currentBotId.value = (botId ?? '').trim() || null
  }

  function setSession(targetSessionId: string | null) {
    sessionId.value = (targetSessionId ?? '').trim() || null
  }

  function clear() {
    currentBotId.value = null
    sessionId.value = null
    draftIntent.value = false
  }

  return {
    currentBotId,
    sessionId,
    draftIntent,
    setBot,
    setSession,
    clear,
  }
})
