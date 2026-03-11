import { Elysia } from 'elysia'
import z from 'zod'
import { createAgent, ModelConfig, type AgentStreamAction } from '@memoh/agent'
import { createAuthFetcher, getBaseUrl } from '../index'
import { bearerMiddleware } from '../middlewares/bearer'
import { AgentSkillModel, AttachmentModel, HeartbeatModel, IdentityContextModel, InboxItemModel, LoopDetectionModel, MCPConnectionModel, ModelConfigModel, ScheduleModel } from '../models'
import { sseChunked } from '../utils/sse'

const AgentModel = z.object({
  model: ModelConfigModel,
  activeContextTime: z.number(),
  channels: z.array(z.string()),
  currentChannel: z.string(),
  messages: z.array(z.any()),
  usableSkills: z.array(AgentSkillModel).optional().default([]),
  skills: z.array(z.string()),
  identity: IdentityContextModel,
  attachments: z.array(AttachmentModel).optional().default([]),
  mcpConnections: z.array(MCPConnectionModel).optional().default([]),
  inbox: z.array(InboxItemModel).optional().default([]),
  loopDetection: LoopDetectionModel,
})

const StreamBodyModel = AgentModel.extend({
  query: z.string().optional().default(''),
})

function buildAgentAndStream(body: z.infer<typeof StreamBodyModel>, bearer: string, signal?: AbortSignal) {
  const auth = {
    bearer,
    baseUrl: getBaseUrl(),
  }
  const authFetcher = createAuthFetcher(auth)
  const { stream } = createAgent({
    model: body.model as ModelConfig,
    activeContextTime: body.activeContextTime,
    channels: body.channels,
    currentChannel: body.currentChannel,
    identity: body.identity,
    auth,
    skills: body.usableSkills,
    mcpConnections: body.mcpConnections,
    inbox: body.inbox,
    loopDetection: body.loopDetection,
  }, authFetcher)
  return stream({
    query: body.query,
    messages: body.messages,
    skills: body.skills,
    attachments: body.attachments,
    signal,
  })
}

export const chatModule = new Elysia({ prefix: '/chat' })
  .use(bearerMiddleware)
  .post('/', async ({ body, bearer }) => {
    console.log('chat', body)
    const auth = {
      bearer: bearer!,
      baseUrl: getBaseUrl(),
    }
    const authFetcher = createAuthFetcher(auth)
    const { ask } = createAgent({
      model: body.model as ModelConfig,
      activeContextTime: body.activeContextTime,
      channels: body.channels,
      currentChannel: body.currentChannel,
      identity: body.identity,
      auth,
      skills: body.usableSkills,
      mcpConnections: body.mcpConnections,
      inbox: body.inbox,
      loopDetection: body.loopDetection,
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
    const abortController = new AbortController()
    try {
      for await (const action of buildAgentAndStream(body, bearer!, abortController.signal)) {
        yield sseChunked(JSON.stringify(action))
      }
    } catch (error) {
      if (abortController.signal.aborted) return
      console.error(error)
      const message = error instanceof Error && error.message.trim()
        ? error.message
        : 'Internal server error'
      yield sseChunked(JSON.stringify({
        type: 'error',
        message,
      }))
    } finally {
      abortController.abort()
    }
  }, {
    body: StreamBodyModel,
  })
  .ws('/ws', (() => {
    const sessions = new Map<unknown, { abortController: AbortController | null; streaming: boolean }>()
    return {
      open(ws: { raw: unknown }) {
        sessions.set(ws.raw, { abortController: null, streaming: false })
      },
      async message(ws: { raw: unknown; send: (data: string) => void }, raw: unknown) {
        const parsed = typeof raw === 'string' ? JSON.parse(raw) : raw
        const session = sessions.get(ws.raw)
        if (!session) return

        if (parsed.type === 'abort') {
          session.abortController?.abort()
          return
        }
        if (parsed.type === 'start') {
          if (session.streaming) {
            ws.send(JSON.stringify({ type: 'error', message: 'Already streaming' }))
            return
          }
          session.streaming = true
          const abortController = new AbortController()
          session.abortController = abortController
          const bearer = parsed.bearer as string | undefined
          if (!bearer) {
            ws.send(JSON.stringify({ type: 'error', message: 'Missing bearer token' }))
            session.streaming = false
            return
          }
          try {
            const body = StreamBodyModel.parse(parsed)
            const streamIter = buildAgentAndStream(body, bearer, abortController.signal)
            for await (const action of streamIter) {
              ws.send(JSON.stringify(action))
            }
          } catch (error) {
            if (!abortController.signal.aborted) {
              console.error(error)
              const message = error instanceof Error && error.message.trim()
                ? error.message
                : 'Internal server error'
              ws.send(JSON.stringify({ type: 'error', message }))
            }
          } finally {
            session.streaming = false
            session.abortController = null
          }
        }
      },
      close(ws: { raw: unknown }) {
        const session = sessions.get(ws.raw)
        if (session) {
          session.abortController?.abort()
          sessions.delete(ws.raw)
        }
      },
    }
  })())
  .post('/trigger-schedule', async ({ body, bearer }) => {
    console.log('trigger-schedule', body)
    const auth = {
      bearer: bearer!,
      baseUrl: getBaseUrl(),
    }
    const authFetcher = createAuthFetcher(auth)
    const { triggerSchedule } = createAgent({
      model: body.model as ModelConfig,
      activeContextTime: body.activeContextTime,
      channels: body.channels,
      currentChannel: body.currentChannel,
      identity: body.identity,
      auth,
      skills: body.usableSkills,
      mcpConnections: body.mcpConnections,
      inbox: body.inbox,
      loopDetection: body.loopDetection,
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
  .post('/trigger-heartbeat', async ({ body, bearer }) => {
    console.log('trigger-heartbeat', body)
    const auth = {
      bearer: bearer!,
      baseUrl: getBaseUrl(),
    }
    const authFetcher = createAuthFetcher(auth)
    const { triggerHeartbeat } = createAgent({
      model: body.model as ModelConfig,
      activeContextTime: body.activeContextTime,
      channels: body.channels,
      currentChannel: body.currentChannel,
      identity: body.identity,
      auth,
      skills: body.usableSkills,
      mcpConnections: body.mcpConnections,
      inbox: body.inbox,
      loopDetection: body.loopDetection,
    }, authFetcher)
    return triggerHeartbeat({
      heartbeat: body.heartbeat,
      messages: body.messages,
      skills: body.skills,
    })
  }, {
    body: AgentModel.extend({
      heartbeat: HeartbeatModel,
    }),
  })
  .post('/subagent', async ({ body, bearer }) => {
    console.log('subagent', body)
    const auth = {
      bearer: bearer!,
      baseUrl: getBaseUrl(),
    }
    const authFetcher = createAuthFetcher(auth)
    const { askAsSubagent } = createAgent({
      model: body.model as ModelConfig,
      identity: body.identity,
      auth,
      isSubagent: true,
      loopDetection: body.loopDetection,
    }, authFetcher)
    return askAsSubagent({
      messages: body.messages,
      input: body.query,
      name: body.name,
      description: body.description,
    })
  }, {
    body: z.object({
      model: ModelConfigModel,
      identity: IdentityContextModel,
      messages: z.array(z.any()).optional().default([]),
      query: z.string(),
      name: z.string(),
      description: z.string(),
      loopDetection: LoopDetectionModel,
    }),
  })
