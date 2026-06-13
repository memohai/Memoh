// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import type { ChatMessage } from '@/store/chat-list'
import ChatMinimap from './chat-minimap.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

const MESSAGE_TOPS = [0, 1000, 2000, 3000, 4000]
const SCROLL_HEIGHT = 5000
const CLIENT_HEIGHT = 400

function userTurn(id: string, text: string): ChatMessage {
  return {
    id,
    role: 'user',
    text,
    attachments: [],
    timestamp: '',
    streaming: false,
    isSelf: true,
  }
}

async function flush() {
  await nextTick()
  await new Promise(resolve => requestAnimationFrame(() => resolve(undefined)))
  await nextTick()
}

describe('chat minimap active pinning', () => {
  let scrollEl: HTMLElement
  let root: HTMLElement
  let app: ReturnType<typeof createApp>

  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', ResizeObserverStub)
    Element.prototype.scrollTo = function (this: Element, options?: ScrollToOptions | number) {
      if (typeof options === 'object' && options?.top !== undefined) this.scrollTop = options.top
    }

    scrollEl = document.createElement('div')
    scrollEl.getBoundingClientRect = () => ({ top: 0 }) as DOMRect
    Object.defineProperty(scrollEl, 'scrollHeight', { value: SCROLL_HEIGHT, configurable: true })
    Object.defineProperty(scrollEl, 'clientHeight', { value: CLIENT_HEIGHT, configurable: true })
    let scrollTop = 0
    Object.defineProperty(scrollEl, 'scrollTop', {
      get: () => scrollTop,
      set: (value: number) => { scrollTop = value },
      configurable: true,
    })
    for (const [index, top] of MESSAGE_TOPS.entries()) {
      const msg = document.createElement('div')
      msg.dataset.messageId = `u${index}`
      msg.getBoundingClientRect = () => ({ top: top - scrollEl.scrollTop }) as DOMRect
      scrollEl.append(msg)
    }
    document.body.append(scrollEl)

    root = document.createElement('div')
    document.body.append(root)
    app = createApp(ChatMinimap, {
      scrollEl,
      contentEl: scrollEl,
      messages: MESSAGE_TOPS.map((_, index) => userTurn(`u${index}`, `question ${index}`)),
    })
    app.mount(root)
  })

  afterEach(() => {
    app.unmount()
    root.remove()
    scrollEl.remove()
    vi.unstubAllGlobals()
  })

  function openPanel() {
    root.querySelector<HTMLButtonElement>('[aria-haspopup="listbox"]')!.click()
  }

  function options() {
    return Array.from(root.querySelectorAll<HTMLButtonElement>('[role="option"]'))
  }

  function selectedIndex() {
    return options().findIndex(option => option.getAttribute('aria-selected') === 'true')
  }

  async function settleScrollAt(top: number) {
    scrollEl.scrollTop = top
    scrollEl.dispatchEvent(new Event('scroll'))
    scrollEl.dispatchEvent(new Event('scrollend'))
    await flush()
  }

  it('keeps the clicked entry active when the landing probe points past it', async () => {
    await flush()
    openPanel()
    await flush()
    options()[2]!.click()
    await flush()
    // Landing puts anchor 3 under the quarter-viewport probe (2900 + 100 >= 3000).
    await settleScrollAt(2900)
    expect(selectedIndex()).toBe(2)
  })

  it('keeps the clicked entry active even when pinned to the bottom', async () => {
    await flush()
    openPanel()
    await flush()
    options()[3]!.click()
    await flush()
    await settleScrollAt(SCROLL_HEIGHT - CLIENT_HEIGHT)
    expect(selectedIndex()).toBe(3)
  })

  it('resumes scroll spy after the user scrolls', async () => {
    await flush()
    openPanel()
    await flush()
    options()[2]!.click()
    await flush()
    await settleScrollAt(2900)
    scrollEl.dispatchEvent(new Event('wheel'))
    await settleScrollAt(0)
    expect(selectedIndex()).toBe(0)
  })

  it('resumes scroll spy when another control scrolls after landing', async () => {
    await flush()
    openPanel()
    await flush()
    options()[2]!.click()
    await flush()
    await settleScrollAt(2900)
    await settleScrollAt(0)
    expect(selectedIndex()).toBe(0)
  })
})
