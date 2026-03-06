import { Elysia } from 'elysia'
import { storage } from '../storage'
import { z } from 'zod'
import { BrowserContextConfigModel } from '../models'
import { browser } from '..'
import { actionModule } from './action'

export const contextModule = new Elysia({ prefix: '/context' })
  .use(actionModule)
  .get('/:id/exists', ({ params }) => {
    return { exists: storage.has(params.id) }
  })
  .get('/', ({ query }) => {
    const { id } = query
    const entry = storage.get(id)
    if (!entry) return null
    return { id: entry.id, name: entry.name, config: entry.config }
  }, {
    query: z.object({
      id: z.string(),
    }),
  })
  .post(
    '/',
    async ({ body }) => {
      const { name, config, id } = body
      const context = await browser.newContext({
        viewport: config.viewport,
        userAgent: config.userAgent,
        deviceScaleFactor: config.deviceScaleFactor,
        isMobile: config.isMobile,
        locale: config.locale,
        timezoneId: config.timezoneId,
        geolocation: config.geolocation,
        permissions: config.permissions,
        extraHTTPHeaders: config.extraHTTPHeaders,
        ignoreHTTPSErrors: config.ignoreHTTPSErrors,
        proxy: config.proxy,
      })
      storage.set(id, { id, name, context, config })
      return { id, name, config }
    },
    {
      body: z.object({
        name: z.string().default(''),
        config: BrowserContextConfigModel.default({}),
        id: z.string().default(crypto.randomUUID()),
      }),
    },
  )
  .delete('/:id', async ({ params }) => {
    const entry = storage.get(params.id)
    if (entry) {
      await entry.context.close()
      storage.delete(params.id)
    }
    return { success: true }
  })
