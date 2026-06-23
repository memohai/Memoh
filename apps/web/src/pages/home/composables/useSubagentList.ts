import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useChatStore, type BackgroundTask, type ChatAssistantTurn, type ToolCallBlock } from '@/store/chat-list'

export interface SubagentSession {
  id: string
  agentId: string
  title: string
}

const SPAWN_TOOLS = new Set(['spawn_agent', 'send_message'])

function isActiveAgent(task: BackgroundTask): boolean {
  const s = task.status
  return s === 'running' || s === 'queued' || s === 'stalled'
}

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
        const bg = tool.backgroundTask
        if (!bg?.agentSessionId || !isActiveAgent(bg)) continue
        if (seen.has(bg.agentSessionId)) continue
        seen.set(bg.agentSessionId, {
          id: bg.agentSessionId,
          agentId: bg.agentId || bg.taskId,
          title: bg.command || bg.agentId || bg.taskId,
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
