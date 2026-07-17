// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import type { UIMessage, UITurn } from '@/composables/api/useChat.types'
import { createBackgroundTaskTracker } from '../background-tasks'
import { createTranscriptController } from '../transcript'
import type { ChatMessage } from '../types'

vi.mock('@/store/user', () => ({
  useUserStore: () => ({ userInfo: { id: 'user-1' } }),
}))

function makeTranscript() {
  const backgroundTasks = createBackgroundTaskTracker()
  return createTranscriptController({
    currentBotId: ref<string | null>('bot-1'),
    sessionId: ref<string | null>('session-1'),
    rememberBackgroundTask: backgroundTasks.rememberBackgroundTask,
    applyPendingBackgroundEventsToTool: backgroundTasks.applyPendingBackgroundEventsToTool,
    mergeBackgroundTaskIntoMatchingTools: backgroundTasks.mergeBackgroundTaskIntoMatchingTools,
    bumpFsChangedAtIfFsMutation: vi.fn(),
    fetchMessages: vi.fn().mockResolvedValue([]),
    locateMessage: vi.fn().mockResolvedValue({ items: [] }),
  })
}

function groupTurns(messages: ChatMessage[]) {
  const groups: Array<{ id: string, messages: ChatMessage[] }> = []
  for (const message of messages) {
    const last = groups[groups.length - 1]
    if (message.role === 'user' || !last) groups.push({ id: message.id, messages: [message] })
    else last.messages.push(message)
  }
  return groups
}

describe('runtime handoff DOM identity', () => {
  let root: HTMLDivElement | undefined
  let app: ReturnType<typeof createApp> | undefined

  afterEach(() => {
    app?.unmount()
    root?.remove()
    app = undefined
    root = undefined
  })

  it('does not remount keyed turn or message leaves from live to settled', async () => {
    const transcript = makeTranscript()
    const user = transcript.createOptimisticUserTurn('question', [], 'stream-1')
    const assistant = transcript.createOptimisticAssistantTurn('assistant-local')
    transcript.setMessageSyncState(user, {
      run: 'running', presence: 'live', persistence: 'unknown', streamId: 'stream-1', generation: 'generation-1',
    })
    transcript.setMessageSyncState(assistant, {
      run: 'running', presence: 'live', persistence: 'unknown', streamId: 'stream-1', generation: 'generation-1',
    })
    transcript.setAssistantRowIdentity(assistant, {
      stableId: 'assistant-row', turnPosition: 7, turnMessageSeq: 2,
    })
    transcript.upsertAssistantUIMessage(assistant, {
      id: 0,
      stable_id: 'assistant-row',
      turn_position: 7,
      turn_message_seq: 2,
      type: 'text',
      content: 'live answer',
    })
    transcript.appendToView(user, assistant)

    const Harness = defineComponent({
      setup() {
        return () => h('main', groupTurns(transcript.messages).map(turn =>
          h('section', { key: turn.id, 'data-turn-key': turn.id }, turn.messages.map(message =>
            h('article', { key: message.id, 'data-message-key': message.id },
              message.role === 'assistant'
                ? (message.messages.find(block => block.type === 'text')?.content ?? '')
                : message.text),
          )),
        ))
      },
    })
    root = document.createElement('div')
    document.body.append(root)
    app = createApp(Harness)
    app.mount(root)
    await nextTick()

    const turnNode = root.querySelector('[data-turn-key]')
    const userNode = root.querySelector(`[data-message-key="${user.id}"]`)
    const assistantNode = root.querySelector(`[data-message-key="${assistant.id}"]`)

    const persistedBlock: UIMessage = {
      id: 0,
      stable_id: 'assistant-row',
      turn_position: 7,
      turn_message_seq: 2,
      type: 'text',
      content: 'settled answer',
    }
    const history: UITurn[] = [
      {
        id: 'user-row', role: 'user', text: 'question', timestamp: '2026-07-16T00:00:00Z',
        platform: 'local', external_message_id: 'stream-1', turn_position: 7, turn_message_seq: 1,
      },
      {
        id: 'assistant-row', role: 'assistant', messages: [persistedBlock],
        timestamp: '2026-07-16T00:00:01Z', turn_position: 7, turn_message_seq: 2,
      },
    ]
    transcript.replacePersistedWindow(history, 'session-1')
    await nextTick()

    expect(root.querySelector('[data-turn-key]')).toBe(turnNode)
    expect(root.querySelector(`[data-message-key="${user.id}"]`)).toBe(userNode)
    expect(root.querySelector(`[data-message-key="${assistant.id}"]`)).toBe(assistantNode)
    expect(assistantNode?.textContent).toBe('settled answer')
  })
})
