import { describe, expect, it } from 'vitest'
import type { ChatAssistantTurn, ChatMessage, ToolCallBlock } from '@/store/chat-list'
import { findLatestPendingChatDecision } from './chat-pending-decision'

function toolBlock(id: number, values: Partial<ToolCallBlock> = {}): ToolCallBlock {
  return {
    id,
    type: 'tool',
    name: 'exec',
    input: { command: `command-${id}` },
    tool_call_id: `tool-call-${id}`,
    running: false,
    toolCallId: `tool-call-${id}`,
    toolName: 'exec',
    result: null,
    done: false,
    ...values,
  }
}

function assistantTurn(id: string, blocks: ToolCallBlock[]): ChatAssistantTurn {
  return {
    id,
    role: 'assistant',
    messages: blocks,
    timestamp: '',
    streaming: false,
  }
}

describe('findLatestPendingChatDecision', () => {
  it('keeps ask_user pending after the user selects an option locally', () => {
    const block = toolBlock(1, {
      toolName: 'ask_user',
      userInput: {
        user_input_id: 'input-1',
        status: 'pending',
        can_respond: true,
        questions: [{
          id: 'q1',
          text: 'Choose one',
          kind: 'single_select',
          options: [{ id: 'q1.o1', label: 'One' }, { id: 'q1.o2', label: 'Two' }],
        }],
      },
    })
    const messages: ChatMessage[] = [assistantTurn('assistant-1', [block])]

    expect(findLatestPendingChatDecision(messages)).toMatchObject({
      kind: 'user_input',
      userInput: { user_input_id: 'input-1', status: 'pending' },
    })
  })

  it('returns pending approvals newest first and exposes the next one after resolution', () => {
    const older = toolBlock(1, {
      approval: { approval_id: 'approval-1', status: 'pending', can_approve: true },
    })
    const newer = toolBlock(2, {
      approval: { approval_id: 'approval-2', status: 'pending', can_approve: true },
    })
    const messages: ChatMessage[] = [assistantTurn('assistant-1', [older, newer])]

    expect(findLatestPendingChatDecision(messages)).toMatchObject({
      kind: 'tool_approval',
      approval: { approval_id: 'approval-2' },
    })

    newer.approval = { ...newer.approval!, status: 'approved', can_approve: false }

    expect(findLatestPendingChatDecision(messages)).toMatchObject({
      kind: 'tool_approval',
      approval: { approval_id: 'approval-1' },
    })
  })

  it('ignores decisions that cannot be answered from this chat', () => {
    const messages: ChatMessage[] = [assistantTurn('assistant-1', [
      toolBlock(1, {
        approval: { approval_id: 'approval-1', status: 'pending', can_approve: false },
      }),
      toolBlock(2, {
        userInput: { user_input_id: 'input-1', status: 'submitted', can_respond: false },
      }),
    ])]

    expect(findLatestPendingChatDecision(messages)).toBeNull()
  })
})
