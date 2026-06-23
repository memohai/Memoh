import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { useSubagentList } from './useSubagentList'

const api = vi.hoisted(() => ({
  connectWebSocket: vi.fn(),
  fetchBots: vi.fn(),
  fetchMessagesUI: vi.fn(),
  fetchSession: vi.fn(),
  fetchSessions: vi.fn(),
  streamBotSessionsActivityEvents: vi.fn(),
  streamSessionMessageEvents: vi.fn(),
}))

const colada = vi.hoisted(() => ({
  data: { value: undefined as unknown },
  options: [] as Array<{
    key: () => unknown[]
    query: () => Promise<unknown>
    enabled: () => boolean
  }>,
  useQuery: vi.fn((options) => {
    colada.options.push(options)
    return { data: colada.data }
  }),
}))

vi.mock('@/composables/api/useChat', () => api)
vi.mock('@pinia/colada', () => ({ useQuery: colada.useQuery }))

describe('useSubagentList', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    colada.data.value = undefined
    colada.options = []
    api.connectWebSocket.mockReturnValue({ close: vi.fn(), send: vi.fn() })
    api.fetchBots.mockResolvedValue([{ id: 'bot-1', name: 'Bot', status: 'ready' }])
    api.fetchMessagesUI.mockResolvedValue([])
    api.fetchSession.mockResolvedValue({
      id: 'parent-1',
      bot_id: 'bot-1',
      title: 'Parent',
      type: 'chat',
    })
    api.streamBotSessionsActivityEvents.mockResolvedValue(undefined)
    api.streamSessionMessageEvents.mockResolvedValue(undefined)
  })

  it('shows active subagent tasks enriched from current session child sessions', async () => {
    const chatStore = useChatStore()
    chatStore.currentBotId = 'bot-1'
    chatStore.sessionId = 'parent-1'
    pushSubagentTool(chatStore, {
      agentSessionId: 'child-1',
      agentId: 'agent-a',
      status: 'running',
    })
    pushSubagentTool(chatStore, {
      agentSessionId: 'child-2',
      agentId: 'agent-b',
      status: 'completed',
      done: true,
    })
    api.fetchSessions.mockResolvedValue({
      items: [
        {
          id: 'child-1',
          bot_id: 'bot-1',
          title: 'Child task',
          type: 'subagent',
          metadata: { agent_id: 'agent-a' },
        },
        {
          id: 'child-2',
          bot_id: 'bot-1',
          title: 'Finished child task',
          type: 'subagent',
          metadata: { agent_id: 'agent-b' },
        },
      ],
      nextCursor: null,
    })

    const { subagents } = useSubagentList()
    const result = await colada.options[0]!.query()
    colada.data.value = result

    expect(colada.options[0]!.key()).toEqual(['session-subagents', 'bot-1', 'parent-1', 'child-1'])
    expect(colada.options[0]!.enabled()).toBe(true)
    expect(api.fetchSessions).toHaveBeenCalledWith('bot-1', {
      types: ['subagent'],
      parentSessionId: 'parent-1',
      limit: 50,
    })
    expect(subagents.value).toEqual([{
      id: 'child-1',
      agentId: 'agent-a',
      title: 'Child task',
    }])
  })

  it('does not show terminated child sessions without an active task', () => {
    const chatStore = useChatStore()
    chatStore.currentBotId = 'bot-1'
    chatStore.sessionId = 'parent-1'
    colada.data.value = [{
      id: 'child-1',
      agentId: 'agent-a',
      title: 'Finished child task',
    }]

    const { subagents } = useSubagentList()

    expect(colada.options[0]!.enabled()).toBe(false)
    expect(subagents.value).toEqual([])
  })

  it('opens the subagent through workspace tabs', () => {
    const chatStore = useChatStore()
    chatStore.currentBotId = 'bot-1'
    chatStore.sessionId = 'parent-1'
    pushSubagentTool(chatStore, {
      agentSessionId: 'child-1',
      agentId: 'agent-a',
      status: 'running',
    })
    colada.data.value = [{
      id: 'child-1',
      agentId: 'agent-a',
      title: 'Child task',
    }]
    const workspaceTabs = useWorkspaceTabsStore()
    const openSessionChat = vi.spyOn(workspaceTabs, 'openSessionChat').mockImplementation(() => {})

    const { navigateToSession } = useSubagentList()
    navigateToSession('child-1')

    expect(openSessionChat).toHaveBeenCalledWith({
      sessionId: 'child-1',
      title: 'Child task',
    })
  })
})

function pushSubagentTool(
  chatStore: ReturnType<typeof useChatStore>,
  task: {
    agentSessionId: string
    agentId: string
    status: string
    done?: boolean
  },
) {
  chatStore.messages.push({
    id: `assistant-${task.agentSessionId}`,
    role: 'assistant',
    timestamp: '2026-01-01T00:00:00.000Z',
    streaming: false,
    messages: [{
      id: 1,
      type: 'tool',
      name: 'spawn_agent',
      input: { id: task.agentId },
      tool_call_id: `tool-${task.agentSessionId}`,
      running: !task.done,
      toolCallId: `tool-${task.agentSessionId}`,
      toolName: 'spawn_agent',
      result: null,
      done: task.done ?? false,
      backgroundTask: {
        taskId: `task-${task.agentSessionId}`,
        status: task.status,
        agentId: task.agentId,
        agentSessionId: task.agentSessionId,
      },
    }],
  })
}
