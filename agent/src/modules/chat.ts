import { Elysia, sse } from 'elysia'
import z from 'zod'
import { createAgent } from '../agent'
import { createAuthFetcher } from '../index'
import { ModelConfig } from '../types'
import { bearerMiddleware } from '../middlewares/bearer'
import { AllowedActionModel, AttachmentModel, IdentityContextModel, ModelConfigModel } from '../models'
import { allActions } from '../types'

const AgentModel = z.object({
  model: ModelConfigModel,
  activeContextTime: z.number(),
  platforms: z.array(z.string()),
  currentPlatform: z.string(),
  allowedActions: z.array(AllowedActionModel).optional().default(allActions),
  messages: z.array(z.any()),
  skills: z.array(z.string()),
  query: z.string(),
  identity: IdentityContextModel,
  attachments: z.array(AttachmentModel),
})

export const chatModule = new Elysia({ prefix: '/chat' })
  .use(bearerMiddleware)
  .post('/', async ({ body, bearer }) => {
    const authFetcher = createAuthFetcher(bearer)
    const { ask } = createAgent({
      model: body.model as ModelConfig,
      activeContextTime: body.activeContextTime,
      platforms: body.platforms,
      currentPlatform: body.currentPlatform,
      allowedActions: body.allowedActions,
      identity: body.identity,
    }, authFetcher)
    return ask({
      query: body.query,
      messages: body.messages,
      skills: body.skills,
      attachments: body.attachments,
    })
  }, {
    body: AgentModel,
  })
  .post('/stream', async function* ({ body, bearer }) {
    const authFetcher = createAuthFetcher(bearer)
    const { stream } = createAgent({
      model: body.model as ModelConfig,
      activeContextTime: body.activeContextTime,
      platforms: body.platforms,
      currentPlatform: body.currentPlatform,
      allowedActions: body.allowedActions,
      identity: body.identity,
    }, authFetcher)
    for await (const action of stream({
      query: body.query,
      messages: body.messages,
      skills: body.skills,
      attachments: body.attachments,
    })) {
      yield sse(JSON.stringify(action))
    }
  }, {
    body: AgentModel,
  })
  