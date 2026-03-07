import { Elysia } from 'elysia'
import { storage } from '../storage'
import { ActionRequestModel, type ActionRequest } from '../models'
import type { Page } from 'playwright'

async function getOrCreatePage(contextId: string): Promise<Page> {
  const entry = storage.get(contextId)
  if (!entry) throw new Error(`Context ${contextId} not found`)
  const pages = entry.context.pages()
  if (pages.length > 0) return pages[0]!
  return await entry.context.newPage()
}

async function executeAction(contextId: string, req: ActionRequest): Promise<Record<string, unknown>> {
  const page = await getOrCreatePage(contextId)

  switch (req.action) {
    case 'navigate': {
      if (!req.url) throw new Error('url is required for navigate')
      const response = await page.goto(req.url, { timeout: req.timeout ?? 30000 })
      return { url: page.url(), status: response?.status() }
    }
    case 'click': {
      if (!req.selector) throw new Error('selector is required for click')
      await page.click(req.selector, { timeout: req.timeout ?? 5000 })
      return { clicked: req.selector }
    }
    case 'type': {
      if (!req.selector) throw new Error('selector is required for type')
      if (!req.text) throw new Error('text is required for type')
      await page.fill(req.selector, req.text, { timeout: req.timeout ?? 5000 })
      return { typed: req.text, selector: req.selector }
    }
    case 'screenshot': {
      const buffer = await page.screenshot({ fullPage: false })
      return { screenshot: buffer.toString('base64'), mimeType: 'image/png' }
    }
    case 'get_content': {
      const text = req.selector
        ? await page.locator(req.selector).innerText({ timeout: req.timeout ?? 5000 })
        : await page.innerText('body')
      return { content: text }
    }
    case 'get_html': {
      const html = req.selector
        ? await page.locator(req.selector).innerHTML({ timeout: req.timeout ?? 5000 })
        : await page.content()
      return { html }
    }
    case 'evaluate': {
      if (!req.script) throw new Error('script is required for evaluate')
      const result = await page.evaluate(req.script)
      return { result }
    }
    case 'scroll': {
      const dir = req.direction ?? 'down'
      const amt = req.amount ?? 500
      const deltaX = dir === 'left' ? -amt : dir === 'right' ? amt : 0
      const deltaY = dir === 'up' ? -amt : dir === 'down' ? amt : 0
      await page.mouse.wheel(deltaX, deltaY)
      return { scrolled: dir, amount: amt }
    }
    case 'wait': {
      if (req.selector) {
        await page.waitForSelector(req.selector, { timeout: req.timeout ?? 10000 })
        return { waited_for: req.selector }
      }
      await page.waitForTimeout(req.timeout ?? 1000)
      return { waited_ms: req.timeout ?? 1000 }
    }
    case 'go_back': {
      await page.goBack({ timeout: req.timeout ?? 30000 })
      return { url: page.url() }
    }
    case 'go_forward': {
      await page.goForward({ timeout: req.timeout ?? 30000 })
      return { url: page.url() }
    }
    case 'reload': {
      await page.reload({ timeout: req.timeout ?? 30000 })
      return { url: page.url() }
    }
    case 'get_url': {
      return { url: page.url() }
    }
    case 'get_title': {
      return { title: await page.title() }
    }
    default:
      throw new Error(`Unknown action: ${req.action}`)
  }
}

export const actionModule = new Elysia()
  .post('/:id/action', async ({ params, body }) => {
    const result = await executeAction(params.id, body)
    return { success: true, data: result }
  }, {
    body: ActionRequestModel,
  })
  .ws('/:id/action/ws', {
    message: async (ws, rawMessage) => {
      try {
        const parsed = ActionRequestModel.safeParse(rawMessage)
        if (!parsed.success) {
          ws.send(JSON.stringify({ success: false, error: parsed.error.message }))
          return
        }
        const result = await executeAction(ws.data.params.id, parsed.data)
        ws.send(JSON.stringify({ success: true, data: result }))
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err)
        ws.send(JSON.stringify({ success: false, error: message }))
      }
    },
  })
