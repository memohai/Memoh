import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useQuery } from '@pinia/colada'
import { fetchSessions } from '@/composables/api/useChat'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import type { SessionSummary } from '@/composables/api/useChat'

export interface SubagentSession {
  id: string
  agentId: string
  title: string
}

const SUBAGENT_PAGE_SIZE = 50

export function useSubagentList() {
  const chatStore = useChatStore()
  const workspaceTabs = useWorkspaceTabsStore()
  const { currentBotId, sessionId } = storeToRefs(chatStore)

  const { data } = useQuery({
    key: () => ['session-subagents', currentBotId.value ?? '', sessionId.value ?? ''],
    query: async () => {
      const botId = currentBotId.value?.trim()
      const parentSessionId = sessionId.value?.trim()
      if (!botId || !parentSessionId) return []
      const { items } = await fetchSessions(botId, {
        types: ['subagent'],
        parentSessionId,
        limit: SUBAGENT_PAGE_SIZE,
      })
      return items.map(toSubagentSession)
    },
    enabled: () => Boolean(currentBotId.value && sessionId.value),
    refetchOnWindowFocus: false,
  })

  const subagents = computed<SubagentSession[]>(() => {
    return data.value ?? []
  })

  function navigateToSession(subagentSessionId: string) {
    if (!subagentSessionId || !currentBotId.value) return
    const agent = subagents.value.find(item => item.id === subagentSessionId)
    workspaceTabs.openSessionChat({
      sessionId: subagentSessionId,
      title: agent?.title || agent?.agentId,
    })
  }

  return { subagents, navigateToSession }
}

function toSubagentSession(session: SessionSummary): SubagentSession {
  const agentId = typeof session.metadata?.agent_id === 'string' && session.metadata.agent_id.trim()
    ? session.metadata.agent_id.trim()
    : session.id
  const title = session.title?.trim() || agentId
  return {
    id: session.id,
    agentId,
    title,
  }
}
