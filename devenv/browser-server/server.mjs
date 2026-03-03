import express from 'express'
import { chromium } from 'playwright'
import { randomUUID } from 'node:crypto'

const app = express()
app.use(express.json({ limit: '1mb' }))

const apiKey = process.env.BROWSER_SERVER_API_KEY || ''
const port = Number(process.env.PORT || 8090)
const maxTextLen = Number(process.env.BROWSER_MAX_TEXT_LEN || 10000)

let browser
const sessions = new Map()

function auth(req, res, next) {
  if (!apiKey) return next()
  const header = req.headers.authorization || ''
  if (header !== `Bearer ${apiKey}`) {
    return res.status(401).json({ error: 'unauthorized' })
  }
  return next()
}

app.use(auth)

async function getBrowser() {
  if (!browser) {
    browser = await chromium.launch({
      headless: true,
      args: ['--no-sandbox', '--disable-dev-shm-usage'],
    })
  }
  return browser
}

app.get('/health', (_req, res) => {
  res.json({ ok: true, sessions: sessions.size })
})

app.post('/sessions', async (req, res) => {
  try {
    const b = await getBrowser()
    const context = await b.newContext()
    const page = await context.newPage()
    const remoteSessionId = `rs_${randomUUID()}`
    const workerId = `worker-${process.pid}`
    sessions.set(remoteSessionId, { context, page })
    return res.json({
      remote_session_id: remoteSessionId,
      worker_id: workerId,
    })
  } catch (error) {
    return res.status(500).json({ error: error instanceof Error ? error.message : 'create session failed' })
  }
})

app.post('/sessions/:id/actions', async (req, res) => {
  const sess = sessions.get(req.params.id)
  if (!sess) {
    return res.status(404).json({ error: 'session not found' })
  }
  const name = String(req.body?.name || '').trim()
  const url = String(req.body?.url || '').trim()
  const target = String(req.body?.target || '').trim()
  const value = String(req.body?.value || '')
  const params = req.body?.params && typeof req.body.params === 'object' ? req.body.params : {}

  try {
    switch (name) {
      case 'goto': {
        if (!url) return res.status(400).json({ error: 'url is required for goto' })
        const response = await sess.page.goto(url, {
          waitUntil: 'domcontentloaded',
          timeout: Number(params.timeout_ms || 15000),
        })
        return res.json({
          action: name,
          current_url: sess.page.url(),
          url: sess.page.url(),
          status_code: response?.status() ?? 0,
          title: await sess.page.title(),
        })
      }
      case 'click': {
        if (!target) return res.status(400).json({ error: 'target is required for click' })
        await sess.page.click(target, { timeout: Number(params.timeout_ms || 10000) })
        return res.json({ action: name, ok: true, current_url: sess.page.url() })
      }
      case 'type': {
        if (!target) return res.status(400).json({ error: 'target is required for type' })
        if (params.clear_first !== false) {
          await sess.page.fill(target, '', { timeout: Number(params.timeout_ms || 10000) })
        }
        await sess.page.type(target, value, { timeout: Number(params.timeout_ms || 10000) })
        return res.json({ action: name, ok: true, current_url: sess.page.url() })
      }
      case 'screenshot': {
        const b64 = await sess.page.screenshot({
          fullPage: params.full_page !== false,
          type: 'png',
        })
        return res.json({
          action: name,
          ok: true,
          current_url: sess.page.url(),
          content_type: 'image/png',
          image_base64: b64.toString('base64'),
        })
      }
      case 'extract_text': {
        if (url) {
          await sess.page.goto(url, {
            waitUntil: 'domcontentloaded',
            timeout: Number(params.timeout_ms || 15000),
          })
        }
        const text = await sess.page.evaluate(() => document.body?.innerText || '')
        const clipped = text.length > maxTextLen ? text.slice(0, maxTextLen) : text
        return res.json({
          action: name,
          current_url: sess.page.url(),
          url: sess.page.url(),
          text: clipped,
          length: clipped.length,
        })
      }
      default:
        return res.status(400).json({ error: `unsupported action: ${name}` })
    }
  } catch (error) {
    return res.status(500).json({
      error: error instanceof Error ? error.message : 'action failed',
      current_url: sess.page.url(),
    })
  }
})

app.delete('/sessions/:id', async (req, res) => {
  const sess = sessions.get(req.params.id)
  if (!sess) return res.status(204).end()
  sessions.delete(req.params.id)
  try {
    await sess.page.close({ runBeforeUnload: false })
  } catch {}
  try {
    await sess.context.close()
  } catch {}
  return res.status(204).end()
})

app.listen(port, '0.0.0.0', () => {
  console.log(`memoh-browser-server listening on :${port}`)
})
