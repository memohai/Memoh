import { computed, onScopeDispose, type Ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useQuery } from '@pinia/colada'
import { fetchAllSessions } from '@/composables/api/useChat'
import { useChatStore } from '@/store/chat-list'
import type { SessionSummary } from '@/composables/api/useChat.types'

const SUBAGENT_LIST_REFRESH_MS = 3_000
const SUBAGENT_LIST_LIMIT = 100

interface SubagentSession {
  id: string
  title: string
  agent_id?: string
  created_at?: string
}

function toSubagentSession(s: SessionSummary): SubagentSession {
  const meta = s.metadata as Record<string, unknown> | undefined
  return {
    id: s.id,
    title: s.title || (meta?.agent_id as string) || s.id.slice(0, 8),
    agent_id: (meta?.agent_id as string) ?? undefined,
    created_at: s.created_at,
  }
}

export function useSubagentList(visible: Ref<boolean>) {
  const chatStore = useChatStore()
  const { currentBotId, sessionId } = storeToRefs(chatStore)

  const { data, error, isLoading, refetch } = useQuery({
    key: () => ['session-subagents', currentBotId.value ?? '', sessionId.value ?? ''],
    query: async () => {
      return fetchAllSessions(currentBotId.value!, {
        types: ['subagent'],
        parentSessionId: sessionId.value!,
        limit: SUBAGENT_LIST_LIMIT,
      })
    },
    enabled: () => !!currentBotId.value && !!sessionId.value && visible.value,
    refetchOnWindowFocus: false,
    staleTime: SUBAGENT_LIST_REFRESH_MS,
  })

  const subagents = computed<SubagentSession[]>(() => (data.value ?? []).map(toSubagentSession))

  let refreshTimer: ReturnType<typeof setInterval> | null = null
  function stopRefreshTimer() {
    if (!refreshTimer) return
    clearInterval(refreshTimer)
    refreshTimer = null
  }

  watch([currentBotId, sessionId, visible], ([botId, sid, isVisible]) => {
    stopRefreshTimer()
    if (!botId || !sid || !isVisible) return
    refreshTimer = setInterval(() => {
      void refetch()
    }, SUBAGENT_LIST_REFRESH_MS)
  }, { immediate: true })

  onScopeDispose(stopRefreshTimer)

  function navigateToSession(subagentSessionId: string) {
    if (!subagentSessionId || !currentBotId.value) return
    const target = data.value?.find(session => session.id === subagentSessionId)
    if (target) {
      chatStore.rememberSession(target)
    }
    void chatStore.selectSession(subagentSessionId)
  }

  return {
    subagents,
    error,
    isLoading,
    refetch,
    navigateToSession,
  }
}

export type { SubagentSession }
