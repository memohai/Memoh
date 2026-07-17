import { ref } from 'vue'
import { describe, expect, it } from 'vitest'
import { usePendingApprovals } from './usePendingApprovals'
import type { ChatMessage, ToolCallBlock } from '@/store/chat/types'
import type { UIToolApproval } from '@/composables/api/useChat'

function toolBlock(overrides: Partial<ToolCallBlock> & { approval?: UIToolApproval }): ToolCallBlock {
  return {
    id: 1,
    type: 'tool',
    name: 'exec',
    input: { command: 'ls /tmp', description: 'List temporary files' },
    tool_call_id: 'call-1',
    toolCallId: 'call-1',
    toolName: 'exec',
    running: false,
    done: true,
    result: null,
    ...overrides,
  }
}

function assistantTurn(id: string, blocks: ToolCallBlock[]): ChatMessage {
  return {
    id,
    role: 'assistant',
    messages: blocks,
    timestamp: '2026-07-16T10:00:00Z',
    streaming: false,
  } as ChatMessage
}

function pendingApproval(id: string, overrides: Partial<UIToolApproval> = {}): UIToolApproval {
  return { approval_id: id, short_id: 1, status: 'pending', ...overrides }
}

describe('usePendingApprovals', () => {
  it('flattens a pending approval off its tool block with the fields the card needs', () => {
    const messages = ref<ChatMessage[]>([
      assistantTurn('turn-1', [
        toolBlock({
          approval: pendingApproval('approval-1', { short_id: 7 }),
          execution_location: { kind: 'workspace', name: 'Server Workspace' },
        }),
      ]),
    ])

    const { items } = usePendingApprovals(messages)

    expect(items.value).toHaveLength(1)
    expect(items.value[0]).toMatchObject({
      id: 'approval-1',
      toolCallId: 'call-1',
      toolName: 'exec',
      input: { command: 'ls /tmp', description: 'List temporary files' },
      executionLocation: { kind: 'workspace', name: 'Server Workspace' },
    })
  })

  it('queues multiple pendings FIFO — the oldest unresolved one leads', () => {
    const messages = ref<ChatMessage[]>([
      assistantTurn('turn-older', [toolBlock({ approval: pendingApproval('approval-first') })]),
      assistantTurn('turn-newer', [
        toolBlock({ id: 2, toolCallId: 'call-2', approval: pendingApproval('approval-second') }),
        toolBlock({ id: 3, toolCallId: 'call-3', approval: pendingApproval('approval-third') }),
      ]),
    ])

    const { items } = usePendingApprovals(messages)

    expect(items.value.map(item => item.id)).toEqual([
      'approval-first',
      'approval-second',
      'approval-third',
    ])
  })

  it('excludes zombies (can_approve === false) and anything already decided', () => {
    const messages = ref<ChatMessage[]>([
      assistantTurn('turn-1', [
        toolBlock({ approval: pendingApproval('approval-zombie', { can_approve: false }) }),
        toolBlock({ id: 2, toolCallId: 'call-2', approval: pendingApproval('approval-approved', { status: 'approved' }) }),
        toolBlock({ id: 3, toolCallId: 'call-3', approval: pendingApproval('approval-rejected', { status: 'rejected' }) }),
        toolBlock({ id: 4, toolCallId: 'call-4', approval: pendingApproval('approval-live') }),
      ]),
    ])

    const { items } = usePendingApprovals(messages)

    expect(items.value.map(item => item.id)).toEqual(['approval-live'])
  })

  it('ignores user turns, non-tool blocks, and tool calls without an approval', () => {
    const messages = ref<ChatMessage[]>([
      {
        id: 'user-1',
        role: 'user',
        messages: [],
        timestamp: '2026-07-16T10:00:00Z',
        streaming: false,
      } as unknown as ChatMessage,
      assistantTurn('turn-1', [
        toolBlock({ approval: undefined }),
        { id: 2, type: 'text', text: 'hello' } as unknown as ToolCallBlock,
      ]),
    ])

    const { items } = usePendingApprovals(messages)

    expect(items.value).toEqual([])
  })

  it('normalizes a non-object tool input to null instead of leaking it to the card', () => {
    const messages = ref<ChatMessage[]>([
      assistantTurn('turn-1', [
        toolBlock({ input: null, approval: pendingApproval('approval-null-input') }),
        toolBlock({ id: 2, toolCallId: 'call-2', input: ['not', 'an', 'object'], approval: pendingApproval('approval-array-input') }),
      ]),
    ])

    const { items } = usePendingApprovals(messages)

    expect(items.value.map(item => item.input)).toEqual([null, null])
  })

  it('re-derives when the transcript changes (a resolved head reveals the next item)', () => {
    const older = toolBlock({ approval: pendingApproval('approval-first') })
    const newer = toolBlock({ id: 2, toolCallId: 'call-2', approval: pendingApproval('approval-second') })
    const messages = ref<ChatMessage[]>([
      assistantTurn('turn-1', [older]),
      assistantTurn('turn-2', [newer]),
    ])

    const { items } = usePendingApprovals(messages)
    expect(items.value[0]?.id).toBe('approval-first')

    // The store flips the status optimistically once the user answers; the
    // queue must re-project and surface the next one without any local state.
    older.approval = { ...older.approval!, status: 'approved' }
    messages.value = [...messages.value]

    expect(items.value.map(item => item.id)).toEqual(['approval-second'])
  })
})
