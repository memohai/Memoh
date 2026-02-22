import { tool, type ModelMessage } from 'ai'
import { z } from 'zod'
import { createAgent } from '../agent'
import type { ModelConfig, AgentAuthContext, AuthFetcher } from '../types'
import { AgentAction, type IdentityContext } from '../types/agent'
import {
  createSubagentClient,
  toSubagentUsage,
  addUsage,
} from '../utils/subagent'

export interface SubagentToolParams {
  fetch: AuthFetcher
  model: ModelConfig
  identity: IdentityContext
  auth: AgentAuthContext
}

export const getSubagentTools = ({ fetch, model, identity, auth }: SubagentToolParams) => {
  const botId = identity.botId.trim()
  const client = createSubagentClient(fetch, botId)

  const listSubagents = tool({
    description: 'List subagents for current user',
    inputSchema: z.object({}),
    execute: async () => {
      if (!botId) {
        throw new Error('bot_id is required')
      }
      return client.list()
    },
  })

  const deleteSubagent = tool({
    description: 'Delete a subagent by id',
    inputSchema: z.object({
      id: z.string().describe('Subagent ID'),
    }),
    execute: async ({ id }) => {
      if (!botId) {
        throw new Error('bot_id is required')
      }
      return client.remove(id)
    },
  })

  const querySubagent = tool({
    description: 'Query a subagent. If the subagent does not exist it will be created automatically.',
    inputSchema: z.object({
      name: z.string().describe('The name of the subagent'),
      description: z.string().describe('A short description of the subagent purpose (used when creating)'),
      query: z.string().describe('The prompt to ask the subagent to do.'),
    }),
    execute: async ({ name, description, query }) => {
      if (!botId) {
        throw new Error('bot_id is required')
      }

      // Get or create the subagent
      const target = await client.getOrCreate({ name, description })

      // Load persisted context (messages + usage)
      const ctx = await client.getContext(target.id)
      const contextMessages = (Array.isArray(ctx.messages) ? ctx.messages : []) as ModelMessage[]
      const existingUsage = toSubagentUsage(ctx.usage)

      // Create a scoped agent instance for the subagent
      const { askAsSubagent } = createAgent({
        model,
        allowedActions: [AgentAction.Web],
        identity,
        auth,
      }, fetch)

      const result = await askAsSubagent({
        messages: contextMessages,
        input: query,
        name: target.name,
        description: target.description,
      })

      // Accumulate usage
      const newUsage = addUsage(existingUsage, result.usage)

      // Persist updated messages + usage
      const updatedMessages = [...contextMessages, ...result.messages]
      await client.updateContext(
        target.id,
        updatedMessages as Record<string, unknown>[],
        newUsage,
      )

      return {
        success: true,
        result: result.messages[result.messages.length - 1].content,
      }
    },
  })

  return {
    'list_subagents': listSubagents,
    'delete_subagent': deleteSubagent,
    'query_subagent': querySubagent,
  }
}
