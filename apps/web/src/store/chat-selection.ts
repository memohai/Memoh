import { defineStore } from 'pinia'
import { useStorage } from '@vueuse/core'

export const useChatSelectionStore = defineStore('chat-selection', () => {
  const currentBotId = useStorage<string | null>('chat-bot-id', null)
  const sessionId = useStorage<string | null>('chat-session-id', null)
  // Persist the user's intent separately from the raw session id. `sessionId`
  // can be written by initialize() when it auto-picks the latest conversation;
  // default ACP startup must be allowed to override that. A manually selected
  // or newly-created session is different and should survive reloads.
  const explicitSelection = useStorage<boolean>('chat-explicit-selection', false)
  // Did the user intentionally sit on the draft "New Session" page (vs. just never
  // having selected anything)? A null sessionId is ambiguous on reload: this flag
  // lets initialize keep the draft instead of force-opening a random session, while
  // a fresh/never-selected load (flag false) still auto-opens the latest session.
  const draftIntent = useStorage<boolean>('chat-draft-intent', false)

  function setBot(botId: string | null) {
    currentBotId.value = (botId ?? '').trim() || null
  }

  function setSession(targetSessionId: string | null, options: { explicitSelection?: boolean } = {}) {
    sessionId.value = (targetSessionId ?? '').trim() || null
    if (options.explicitSelection !== undefined) {
      explicitSelection.value = options.explicitSelection
    } else if (!sessionId.value) {
      explicitSelection.value = false
    }
  }

  function setExplicitSelection(value: boolean) {
    explicitSelection.value = value
  }

  function clear() {
    currentBotId.value = null
    sessionId.value = null
    explicitSelection.value = false
    draftIntent.value = false
  }

  return {
    currentBotId,
    sessionId,
    explicitSelection,
    draftIntent,
    setBot,
    setSession,
    setExplicitSelection,
    clear,
  }
})
