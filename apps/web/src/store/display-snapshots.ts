import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { onAuthSessionCleared } from '@/lib/auth-session'

export interface DisplaySnapshot {
  botId: string
  tabId: string
  sessionId?: string
  dataUrl: string
  updatedAt: number
}

export const useDisplaySnapshotsStore = defineStore('display-snapshots', () => {
  const snapshots = ref<Record<string, DisplaySnapshot>>({})

  function key(botId: string, id: string) {
    return `${botId}:${id}`
  }

  function upsert(botId: string, payload: { tabId: string; sessionId?: string; dataUrl: string }) {
    const bid = botId.trim()
    if (!bid || !payload.tabId || !payload.dataUrl) return
    const snapshot: DisplaySnapshot = {
      botId: bid,
      tabId: payload.tabId,
      sessionId: payload.sessionId,
      dataUrl: payload.dataUrl,
      updatedAt: Date.now(),
    }
    snapshots.value = {
      ...snapshots.value,
      [key(bid, payload.tabId)]: snapshot,
      ...(payload.sessionId ? { [key(bid, payload.sessionId)]: snapshot } : {}),
    }
  }

  function find(botId: string, id: string | undefined) {
    if (!id) return undefined
    return snapshots.value[key(botId, id)]
  }

  function clear() {
    snapshots.value = {}
  }

  onAuthSessionCleared(() => clear())

  const items = computed(() => Object.values(snapshots.value))

  return {
    items,
    upsert,
    find,
    clear,
  }
})
