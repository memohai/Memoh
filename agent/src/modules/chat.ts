import { Elysia } from 'elysia'
import z from 'zod'
import { createAgent } from '../agent'
import { createAuthFetcher, getBaseUrl } from '../index'
import { ModelConfig } from '../types'
import { bearerMiddleware } from '../middlewares/bearer'
import { AgentSkillModel, AllowedActionModel, AttachmentModel, IdentityContextModel, InboxItemModel, MCPConnectionModel, ModelConfigModel, ScheduleModel } from '../models'
import { allActions } from '../types'
import { sseChunked } from '../utils/sse'

const AgentModel = z.object({
  model: ModelConfigModel,
  activeContextTime: z.number(),
  channels: z.array(z.string()),
  currentChannel: z.string(),
  allowedActions: z.array(AllowedActionModel).optional().default(allActions),
  messages: z.array(z.any()),
  usableSkills: z.array(AgentSkillModel).optional().default([]),
  skills: z.array(z.string()),
  identity: IdentityContextModel,
  attachments: z.array(AttachmentModel).optional().default([]),
  mcpConnections: z.array(MCPConnectionModel).optional().default([]),
  inbox: z.array(InboxItemModel).optional().default([]),
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
      auth: {
        bearer: bearer!,
        baseUrl: getBaseUrl(),
      },
      skills: body.usableSkills,
      mcpConnections: body.mcpConnections,
      inbox: body.inbox,
    }, authFetcher)
    return ask({
      query: body.query,
      messages: body.messages,
      skills: body.skills,
      attachments: body.attachments,
    })
  }, {
    body: AgentModel.extend({
      query: z.string().optional().default(''),
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
        auth: {
          bearer: bearer!,
          baseUrl: getBaseUrl(),
        },
        skills: body.usableSkills,
        mcpConnections: body.mcpConnections,
        inbox: body.inbox,
      }, authFetcher)
      for await (const action of stream({
        query: body.query,
        messages: body.messages,
        skills: body.skills,
        attachments: body.attachments,
      })) {
        yield sseChunked(JSON.stringify(action))
      }
    } catch (error) {
      console.error(error)
      const message = error instanceof Error && error.message.trim()
        ? error.message
        : 'Internal server error'
      yield sseChunked(JSON.stringify({
        type: 'error',
        message,
      }))
    }
  }, {
    body: AgentModel.extend({
      query: z.string().optional().default(''),
    }),
  })
  .post('/trigger-schedule', async ({ body, bearer }) => {
    console.log('trigger-schedule', body)
    const authFetcher = createAuthFetcher(bearer)
    const { triggerSchedule } = createAgent({
      model: body.model as ModelConfig,
      activeContextTime: body.activeContextTime,
      channels: body.channels,
      currentChannel: body.currentChannel,
      identity: body.identity,
      auth: {
        bearer: bearer!,
        baseUrl: getBaseUrl(),
      },
      skills: body.usableSkills,
      mcpConnections: body.mcpConnections,
      inbox: body.inbox,
    }, authFetcher)
    return triggerSchedule({
      schedule: body.schedule,
      messages: body.messages,
      skills: body.skills,
    })
  }, {
    body: AgentModel.extend({
      schedule: ScheduleModel,
    }),
  })
