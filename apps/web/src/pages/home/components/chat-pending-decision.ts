import type { ChatMessage, ToolCallBlock } from '@/store/chat-list'

export type PendingChatDecision =
  | {
      kind: 'user_input'
      block: ToolCallBlock
      userInput: NonNullable<ToolCallBlock['userInput']>
    }
  | {
      kind: 'tool_approval'
      block: ToolCallBlock
      approval: NonNullable<ToolCallBlock['approval']>
    }

export function findLatestPendingChatDecision(
  messages: readonly ChatMessage[],
): PendingChatDecision | null {
  for (let messageIndex = messages.length - 1; messageIndex >= 0; messageIndex--) {
    const message = messages[messageIndex]
    if (!message || message.role !== 'assistant') continue

    for (let blockIndex = message.messages.length - 1; blockIndex >= 0; blockIndex--) {
      const block = message.messages[blockIndex]
      if (!block || block.type !== 'tool') continue

      const userInput = block.userInput
      if (
        userInput?.user_input_id
        && userInput.status === 'pending'
        && userInput.can_respond !== false
      ) {
        return { kind: 'user_input', block, userInput }
      }

      const approval = block.approval
      if (
        approval?.approval_id
        && approval.status === 'pending'
        && approval.can_approve !== false
      ) {
        return { kind: 'tool_approval', block, approval }
      }
    }
  }

  return null
}
