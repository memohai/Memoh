import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useChatStore, type ChatAssistantTurn, type ToolCallBlock } from '@/store/chat-list'

export interface SubagentSession {
  id: string
  agentId: string
  title: string
}

const SPAWN_TOOLS = new Set(['spawn_agent', 'send_message'])

export function useSubagentList() {
  const chatStore = useChatStore()
  const { messages, currentBotId } = storeToRefs(chatStore)

  const subagents = computed<SubagentSession[]>(() => {
    const seen = new Map<string, SubagentSession>()
    for (const msg of messages.value) {
      if (msg.role !== 'assistant') continue
      for (const block of (msg as ChatAssistantTurn).messages) {
        if (block.type !== 'tool') continue
        const tool = block as ToolCallBlock
        if (!SPAWN_TOOLS.has(tool.toolName)) continue
        if (tool.done) continue
        const bg = tool.backgroundTask
        const sessionId = bg?.agentSessionId
        if (!sessionId) continue
        if (seen.has(sessionId)) continue
        const agentId = bg?.agentId || bg?.taskId || tool.toolCallId
        seen.set(sessionId, {
          id: sessionId,
          agentId,
          title: agentId,
        })
      }
    }
    return [...seen.values()]
  })

  function navigateToSession(subagentSessionId: string) {
    if (!subagentSessionId || !currentBotId.value) return
    void chatStore.selectSession(subagentSessionId)
  }

  return { subagents, navigateToSession }
}
