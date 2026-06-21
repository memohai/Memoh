import { computed, type Ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useQuery } from '@pinia/colada'
import { client } from '@memohai/sdk/client'
import { useChatStore } from '@/store/chat-list'
import type { SessionSummary } from '@/composables/api/useChat.types'

interface SubagentSession {
  id: string
  title: string
  agent_id?: string
  status?: string
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

  const { data, isLoading } = useQuery({
    key: () => ['session-subagents', currentBotId.value ?? '', sessionId.value ?? ''],
    query: async () => {
      const resp = await client.get({
        url: '/bots/{bot_id}/sessions/{session_id}/subagents',
        path: {
          bot_id: currentBotId.value!,
          session_id: sessionId.value!,
        },
        throwOnError: true,
      })
      const body = resp.data as { items?: SessionSummary[] } | undefined
      return (body?.items ?? []).map(toSubagentSession)
    },
    enabled: () => !!currentBotId.value && !!sessionId.value && visible.value,
    refetchOnWindowFocus: false,
    staleTime: 10_000,
  })

  const subagents = computed<SubagentSession[]>(() => data.value ?? [])

  function navigateToSession(subagentSessionId: string) {
    if (!subagentSessionId || !currentBotId.value) return
    void chatStore.selectSession(subagentSessionId)
  }

  return {
    subagents,
    isLoading,
    navigateToSession,
  }
}

export type { SubagentSession }
