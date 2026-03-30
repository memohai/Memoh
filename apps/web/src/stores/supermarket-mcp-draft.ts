import { ref } from 'vue'
import type { HandlersSupermarketMcpEntry } from '@memohai/sdk'

const pendingDraft = ref<HandlersSupermarketMcpEntry | null>(null)

export function useSupermarketMcpDraft() {
  function setPendingDraft(entry: HandlersSupermarketMcpEntry) {
    pendingDraft.value = entry
  }

  function consumePendingDraft(): HandlersSupermarketMcpEntry | null {
    const draft = pendingDraft.value
    pendingDraft.value = null
    return draft
  }

  return { pendingDraft, setPendingDraft, consumePendingDraft }
}
