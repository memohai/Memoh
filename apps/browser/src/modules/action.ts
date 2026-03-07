import { Elysia } from 'elysia'
import { storage } from '../storage'
import { ActionRequestModel, type ActionRequest } from '../models'
import type { Page } from 'playwright'
import type { GatewayBrowserContext } from '../types'

function getActivePage(entry: GatewayBrowserContext): Page | null {
  if (entry.activePage && !entry.activePage.isClosed()) {
    return entry.activePage
  }
  const pages = entry.context.pages()
  const page = pages.length > 0 ? pages[0]! : null
  if (page) entry.activePage = page
  return page
}

interface AccessibilityNode {
  role: string
  name?: string
  value?: string
  description?: string
  level?: number
  checked?: boolean | 'mixed'
  disabled?: boolean
  expanded?: boolean
  selected?: boolean
  children?: AccessibilityNode[]
}

function formatAccessibilityTree(node: AccessibilityNode, indent = 0): string {
  const prefix = '  '.repeat(indent) + '- '
  let line = prefix + node.role
  if (node.name) line += ` "${node.name}"`
  const attrs: string[] = []
  if (node.level !== undefined) attrs.push(`level=${node.level}`)
  if (node.value !== undefined) attrs.push(`value="${node.value}"`)
  if (node.checked !== undefined) attrs.push(`checked=${node.checked}`)
  if (node.disabled) attrs.push('disabled')
  if (node.expanded !== undefined) attrs.push(`expanded=${node.expanded}`)
  if (node.selected) attrs.push('selected')
  if (attrs.length) line += ` [${attrs.join(', ')}]`
  const lines = [line]
  if (node.children) {
    for (const child of node.children) {
      lines.push(formatAccessibilityTree(child, indent + 1))
    }
  }
  return lines.join('\n')
}

const INTERACTIVE_SELECTORS = [
  'a[href]',
  'button',
  'input',
  'select',
  'textarea',
  '[role="button"]',
  '[role="link"]',
  '[role="tab"]',
  '[role="menuitem"]',
  '[role="checkbox"]',
  '[role="radio"]',
  '[onclick]',
  '[tabindex]:not([tabindex="-1"])',
].join(', ')

async function executeAction(contextId: string, req: ActionRequest): Promise<Record<string, unknown>> {
  const entry = storage.get(contextId)
  if (!entry) throw new Error(`Context ${contextId} not found`)

  switch (req.action) {
    case 'tab_new': {
      const page = await entry.context.newPage()
      entry.activePage = page
      if (req.url) {
        await page.goto(req.url, { timeout: req.timeout ?? 30000 })
      }
      return { tab_index: entry.context.pages().indexOf(page), url: page.url() }
    }
    case 'tab_select': {
      if (req.tab_index === undefined) throw new Error('tab_index is required for tab_select')
      const pages = entry.context.pages()
      const page = pages[req.tab_index]
      if (!page) throw new Error(`Tab index ${req.tab_index} out of range (${pages.length} tabs)`)
      entry.activePage = page
      await page.bringToFront()
      return { tab_index: req.tab_index, url: page.url(), title: await page.title() }
    }
    case 'tab_close': {
      const pages = entry.context.pages()
      const active = getActivePage(entry)
      const idx = req.tab_index ?? (active ? pages.indexOf(active) : 0)
      const page = pages[idx]
      if (!page) throw new Error(`Tab index ${idx} out of range (${pages.length} tabs)`)
      await page.close()
      const remaining = entry.context.pages()
      if (remaining.length > 0) {
        entry.activePage = remaining[Math.min(idx, remaining.length - 1)]
      } else {
        entry.activePage = undefined
      }
      const activeIdx = entry.activePage ? remaining.indexOf(entry.activePage) : null
      return { closed: idx, active_tab: activeIdx }
    }
    case 'tab_list': {
      const pages = entry.context.pages()
      const active = getActivePage(entry)
      const tabs = await Promise.all(pages.map(async (p, i) => ({
        index: i,
        url: p.url(),
        title: await p.title(),
        active: p === active,
      })))
      return { tabs }
    }
  }

  const page = getActivePage(entry) ?? await entry.context.newPage()
  if (!entry.activePage) entry.activePage = page

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
    case 'dblclick': {
      if (!req.selector) throw new Error('selector is required for dblclick')
      await page.dblclick(req.selector, { timeout: req.timeout ?? 5000 })
      return { dblclicked: req.selector }
    }
    case 'focus': {
      if (!req.selector) throw new Error('selector is required for focus')
      await page.focus(req.selector, { timeout: req.timeout ?? 5000 })
      return { focused: req.selector }
    }
    case 'type': {
      if (!req.selector) throw new Error('selector is required for type')
      if (!req.text) throw new Error('text is required for type')
      await page.locator(req.selector).pressSequentially(req.text, { timeout: req.timeout ?? 5000 })
      return { typed: req.text, selector: req.selector }
    }
    case 'fill': {
      if (!req.selector) throw new Error('selector is required for fill')
      if (!req.text) throw new Error('text is required for fill')
      await page.fill(req.selector, req.text, { timeout: req.timeout ?? 5000 })
      return { filled: req.text, selector: req.selector }
    }
    case 'press': {
      if (!req.key) throw new Error('key is required for press')
      await page.keyboard.press(req.key)
      return { pressed: req.key }
    }
    case 'keyboard_type': {
      if (!req.text) throw new Error('text is required for keyboard_type')
      await page.keyboard.type(req.text)
      return { keyboard_typed: req.text }
    }
    case 'keyboard_inserttext': {
      if (!req.text) throw new Error('text is required for keyboard_inserttext')
      await page.keyboard.insertText(req.text)
      return { inserted_text: req.text }
    }
    case 'keydown': {
      if (!req.key) throw new Error('key is required for keydown')
      await page.keyboard.down(req.key)
      return { keydown: req.key }
    }
    case 'keyup': {
      if (!req.key) throw new Error('key is required for keyup')
      await page.keyboard.up(req.key)
      return { keyup: req.key }
    }
    case 'hover': {
      if (!req.selector) throw new Error('selector is required for hover')
      await page.hover(req.selector, { timeout: req.timeout ?? 5000 })
      return { hovered: req.selector }
    }
    case 'select': {
      if (!req.selector) throw new Error('selector is required for select')
      if (!req.value) throw new Error('value is required for select')
      const selected = await page.selectOption(req.selector, req.value, { timeout: req.timeout ?? 5000 })
      return { selected: selected, selector: req.selector }
    }
    case 'check': {
      if (!req.selector) throw new Error('selector is required for check')
      await page.check(req.selector, { timeout: req.timeout ?? 5000 })
      return { checked: req.selector }
    }
    case 'uncheck': {
      if (!req.selector) throw new Error('selector is required for uncheck')
      await page.uncheck(req.selector, { timeout: req.timeout ?? 5000 })
      return { unchecked: req.selector }
    }
    case 'screenshot': {
      const buffer = await page.screenshot({ fullPage: req.full_page ?? false })
      return { screenshot: buffer.toString('base64'), mimeType: 'image/png' }
    }
    case 'screenshot_annotate': {
      type AnnotationEntry = { ref: number, tag: string, role: string, name: string }

      /* eslint-disable @typescript-eslint/no-explicit-any -- evaluate callbacks run in browser context */
      const annotations: AnnotationEntry[] = await page.evaluate((selectors) => {
        const doc = (globalThis as any).document
        const elements = doc.querySelectorAll(selectors)
        const result: AnnotationEntry[] = []
        let ref = 1
        elements.forEach((el: any) => {
          const rect = el.getBoundingClientRect()
          if (rect.width === 0 || rect.height === 0) return
          if ((globalThis as any).getComputedStyle(el).visibility === 'hidden') return
          result.push({
            ref,
            tag: el.tagName.toLowerCase(),
            role: el.getAttribute('role') || '',
            name: (el.getAttribute('aria-label') || el.textContent || '').trim().slice(0, 80),
          })
          const label = doc.createElement('div')
          label.className = '__memoh_annotation__'
          label.textContent = String(ref)
          label.style.cssText = `position:fixed;left:${rect.left}px;top:${rect.top - 18}px;z-index:2147483647;background:#e63946;color:#fff;font:bold 11px/16px monospace;padding:0 4px;border-radius:3px;pointer-events:none;`
          doc.body.appendChild(label)
          ref++
        })
        return result
      }, INTERACTIVE_SELECTORS)

      const buffer = await page.screenshot({ fullPage: false })

      await page.evaluate(() => {
        ;(globalThis as any).document.querySelectorAll('.__memoh_annotation__').forEach((el: any) => el.remove())
      })
      /* eslint-enable @typescript-eslint/no-explicit-any */

      return {
        screenshot: buffer.toString('base64'),
        mimeType: 'image/png',
        annotations,
      }
    }
    case 'snapshot': {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- accessibility API is deprecated but functional
      const tree = await (page as any).accessibility.snapshot() as AccessibilityNode | null
      if (!tree) return { snapshot: '(empty page)' }
      return { snapshot: formatAccessibilityTree(tree) }
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
      if (req.selector) {
        await page.locator(req.selector).evaluate((el, { dx, dy }) => { el.scrollBy(dx, dy) }, { dx: deltaX, dy: deltaY })
      } else {
        await page.mouse.wheel(deltaX, deltaY)
      }
      return { scrolled: dir, amount: amt, selector: req.selector }
    }
    case 'scrollintoview': {
      if (!req.selector) throw new Error('selector is required for scrollintoview')
      await page.locator(req.selector).scrollIntoViewIfNeeded({ timeout: req.timeout ?? 5000 })
      return { scrolled_into_view: req.selector }
    }
    case 'drag': {
      if (!req.selector) throw new Error('selector (source) is required for drag')
      if (!req.target_selector) throw new Error('target_selector is required for drag')
      await page.dragAndDrop(req.selector, req.target_selector, { timeout: req.timeout ?? 5000 })
      return { dragged: req.selector, target: req.target_selector }
    }
    case 'upload': {
      if (!req.selector) throw new Error('selector is required for upload')
      if (!req.files || req.files.length === 0) throw new Error('files is required for upload')
      await page.setInputFiles(req.selector, req.files, { timeout: req.timeout ?? 5000 })
      return { uploaded: req.files, selector: req.selector }
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
    case 'pdf': {
      const buffer = await page.pdf()
      return { pdf: buffer.toString('base64'), mimeType: 'application/pdf' }
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
