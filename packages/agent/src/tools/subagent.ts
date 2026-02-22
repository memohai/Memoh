import { tool } from 'ai'
import { z } from 'zod'
import { createAgent } from '../agent'
import { ModelConfig, AgentAuthContext } from '../types'
import { AuthFetcher } from '../types'
import { AgentAction, IdentityContext } from '../types/agent'

export interface SubagentToolParams {
  fetch: AuthFetcher
  model: ModelConfig
  identity: IdentityContext
  auth: AgentAuthContext
}

export const getSubagentTools = ({ fetch, model, identity, auth }: SubagentToolParams) => {
  const botId = identity.botId.trim()
  const base = `/bots/${botId}/subagents`

  const listSubagents = tool({
    description: 'List subagents for current user',
    inputSchema: z.object({}),
    execute: async () => {
      if (!botId) {
        throw new Error('bot_id is required')
      }
      const response = await fetch(base, { method: 'GET' })
      return response.json()
    },
  })

  const createSubagent = tool({
    description: 'Create a new subagent',
    inputSchema: z.object({
      name: z.string(),
      description: z.string(),
    }),
    execute: async ({ name, description }) => {
      if (!botId) {
        throw new Error('bot_id is required')
      }
      const response = await fetch(base, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, description }),
      })
      return response.json()
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
      const response = await fetch(`${base}/${id}`, { method: 'DELETE' })
      return response.status === 204 ? { success: true } : response.json()
    },
  })

  const querySubagent = tool({
    description: 'Query a subagent',
    inputSchema: z.object({
      name: z.string(),
      query: z.string().describe('The prompt to ask the subagent to do.'),
    }),
    execute: async ({ name, query }) => {
      if (!botId) {
        throw new Error('bot_id is required')
      }
      const listResponse = await fetch(base, { method: 'GET' })
      const listPayload = await listResponse.json()
      const items = Array.isArray(listPayload?.items) ? listPayload.items : []
      const target = items.find((item: { name?: string }) => item?.name === name)
      if (!target?.id) {
        throw new Error(`subagent not found: ${name}`)
      }
      const contextResponse = await fetch(`${base}/${target.id}/context`, { method: 'GET' })
      const contextPayload = await contextResponse.json()
      const contextMessages = Array.isArray(contextPayload?.messages) ? contextPayload.messages : []
      const { askAsSubagent } = createAgent({
        model,
        allowedActions: [
          AgentAction.Web,
        ],
        identity,
        auth,
      }, fetch)
      const result = await askAsSubagent({
        messages: contextMessages,
        input: query,
        name: target.name,
        description: target.description,
      })
      const updatedMessages = [...contextMessages, ...result.messages]
      await fetch(`${base}/${target.id}/context`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ messages: updatedMessages }),
      })
      return {
        success: true,
        result: result.messages[result.messages.length - 1].content,
      }
    },
  })

  return {
    'list_subagents': listSubagents,
    'create_subagent': createSubagent,
    'delete_subagent': deleteSubagent,
    'query_subagent': querySubagent,
  }
}