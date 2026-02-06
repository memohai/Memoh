import { Elysia, sse } from 'elysia'
import z from 'zod'
import { createAgent } from '../agent'
import { createAuthFetcher } from '../index'
import { ModelConfig } from '../types'
import { bearerMiddleware } from '../middlewares/bearer'
import { AllowedActionModel, AttachmentModel, IdentityContextModel, MCPConnectionModel, ModelConfigModel, ScheduleModel } from '../models'
import { allActions } from '../types'

const AgentModel = z.object({
  model: ModelConfigModel,
  activeContextTime: z.number(),
  channels: z.array(z.string()),
  currentChannel: z.string(),
  allowedActions: z.array(AllowedActionModel).optional().default(allActions),
  messages: z.array(z.any()),
  skills: z.array(z.string()),
  identity: IdentityContextModel,
  attachments: z.array(AttachmentModel).optional().default([]),
  mcpConnections: z.array(MCPConnectionModel).optional().default([]),
})

export const chatModule = new Elysia({ prefix: '/chat' })
  .use(bearerMiddleware)
  .post('/', async ({ body, bearer }) => {
    console.log('chat', body)
    const authFetcher = createAuthFetcher(bearer)
    const { ask } = createAgent({
      model: body.model as ModelConfig,
      activeContextTime: body.activeContextTime,
      channels: body.channels,
      currentChannel: body.currentChannel,
      allowedActions: body.allowedActions,
      identity: body.identity,
      mcpConnections: body.mcpConnections,
    }, authFetcher)
    return ask({
      query: body.query,
      messages: body.messages,
      skills: body.skills,
      attachments: body.attachments,
    })
  }, {
    body: AgentModel.extend({
      query: z.string(),
    }),
  })
  .post('/stream', async function* ({ body, bearer }) {
    console.log('stream', body)
    try {
      const authFetcher = createAuthFetcher(bearer)
      const { stream } = createAgent({
        model: body.model as ModelConfig,
        activeContextTime: body.activeContextTime,
        channels: body.channels,
        currentChannel: body.currentChannel,
        allowedActions: body.allowedActions,
        identity: body.identity,
        mcpConnections: body.mcpConnections,
      }, authFetcher)
      for await (const action of stream({
        query: body.query,
        messages: body.messages,
        skills: body.skills,
        attachments: body.attachments,
      })) {
        yield sse(JSON.stringify(action))
      }
    } catch (error) {
      console.error(error)
      yield sse(JSON.stringify({
        type: 'error',
        message: 'Internal server error',
      }))
    }
  }, {
    body: AgentModel.extend({
      query: z.string(),
    }),
  })
  .post('/trigger-schedule', async ({ body, bearer }) => {
    const authFetcher = createAuthFetcher(bearer)
    const { triggerSchedule } = createAgent({
      model: body.model as ModelConfig,
      activeContextTime: body.activeContextTime,
      channels: body.channels,
      currentChannel: body.currentChannel,
      identity: body.identity,
      mcpConnections: body.mcpConnections,
    }, authFetcher)
    return triggerSchedule({
      schedule: body.schedule,
      messages: body.messages,
    })
  }, {
    body: AgentModel.extend({
      schedule: ScheduleModel,
    }),
  })
