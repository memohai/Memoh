// @vitest-environment jsdom
import type { App, Ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import type { ChatMessage } from '@/store/chat-list'
import { animateScrollTo, useChatScroll } from './useChatScroll'

vi.mock('@vueuse/core', async () => {
  const vue = await import('vue')
  return {
    useScroll: () => ({ isScrolling: vue.ref(false) }),
  }
})

type ChatScroll = ReturnType<typeof useChatScroll>

interface ScrollGeometry {
  scrollHeight: number
  clientHeight: number
}

interface Harness {
  app: App
  host: HTMLElement
  viewport: HTMLElement
  content: HTMLElement
  geometry: ScrollGeometry
  scrollTo: ReturnType<typeof vi.fn>
  scrollBy: ReturnType<typeof vi.fn>
  messages: Ref<ChatMessage[]>
  lastTurnEl: Ref<HTMLElement | null>
  scroll: ChatScroll
}

class ResizeObserverMock {
  static instances: ResizeObserverMock[] = []

  readonly observe = vi.fn()
  readonly unobserve = vi.fn()
  readonly disconnect = vi.fn()
  private readonly callback: ResizeObserverCallback

  constructor(callback: ResizeObserverCallback) {
    this.callback = callback
    ResizeObserverMock.instances.push(this)
  }

  trigger(target: Element) {
    this.callback([{ target } as ResizeObserverEntry], this as unknown as ResizeObserver)
  }
}

const harnesses: Harness[] = []
let nextAnimationFrameId = 1
const animationFrameTimers = new Map<number, ReturnType<typeof setTimeout>>()

function userMessage(id: string, text = id): ChatMessage {
  return {
    id,
    role: 'user',
    text,
    attachments: [],
    timestamp: '2026-07-10T00:00:00.000Z',
    streaming: false,
    isSelf: true,
  }
}

function assistantMessage(id: string): ChatMessage {
  return {
    id,
    role: 'assistant',
    messages: [],
    timestamp: '2026-07-10T00:00:01.000Z',
    streaming: false,
  }
}

function rect(top: number, height: number): DOMRect {
  return {
    x: 0,
    y: top,
    top,
    right: 600,
    bottom: top + height,
    left: 0,
    width: 600,
    height,
    toJSON: () => ({}),
  }
}

function touchEvent(type: string, clientY: number): TouchEvent {
  const event = new Event(type, { bubbles: true }) as TouchEvent
  Object.defineProperty(event, 'touches', {
    configurable: true,
    value: [{ clientY }],
  })
  return event
}

function mountHarness(
  initialMessages: ChatMessage[] = [],
  sessionId: Ref<string> = ref('session-1'),
  loadingMessages: Ref<boolean> = ref(false),
): Harness {
  const host = document.createElement('div')
  const viewport = document.createElement('div')
  const content = document.createElement('div')
  viewport.append(content)
  document.body.append(host, viewport)

  const geometry: ScrollGeometry = {
    scrollHeight: 1_000,
    clientHeight: 200,
  }
  viewport.scrollTop = 800
  Object.defineProperties(viewport, {
    scrollHeight: {
      configurable: true,
      get: () => geometry.scrollHeight,
    },
    clientHeight: {
      configurable: true,
      get: () => geometry.clientHeight,
    },
  })
  viewport.getBoundingClientRect = () => rect(0, geometry.clientHeight)

  const scrollTo = vi.fn((options: ScrollToOptions | number) => {
    viewport.scrollTop = typeof options === 'number' ? options : (options.top ?? viewport.scrollTop)
    viewport.dispatchEvent(new Event('scroll'))
  })
  Object.defineProperty(viewport, 'scrollTo', {
    configurable: true,
    value: scrollTo,
  })
  const scrollBy = vi.fn((options: ScrollToOptions) => {
    viewport.scrollTop += options.top ?? 0
    viewport.dispatchEvent(new Event('scroll'))
  })
  Object.defineProperty(viewport, 'scrollBy', {
    configurable: true,
    value: scrollBy,
  })

  const messages = ref<ChatMessage[]>(initialMessages)
  const lastTurnEl = ref<HTMLElement | null>(null)
  let scroll!: ChatScroll
  const app = createApp(defineComponent({
    setup() {
      scroll = useChatScroll({
        scrollEl: ref(viewport),
        contentEl: ref(content),
        lastTurnEl,
        messages,
        loadingMessages,
        isActive: ref(true),
        sessionId,
      })
      return () => h('div')
    },
  }))
  app.mount(host)
  scroll.lockScroll.value = false
  scroll.sessionLandingPending.value = false

  const harness = {
    app,
    host,
    viewport,
    content,
    geometry,
    scrollTo,
    scrollBy,
    messages,
    lastTurnEl,
    scroll,
  }
  harnesses.push(harness)
  return harness
}

function appendMessageAnchor(
  harness: Harness,
  id: string,
  documentTop: { value: number },
): HTMLElement {
  const element = document.createElement('div')
  element.dataset.messageId = id
  element.getBoundingClientRect = () => rect(documentTop.value - harness.viewport.scrollTop, 40)
  harness.content.append(element)
  return element
}

async function flushDom() {
  await Promise.resolve()
  await nextTick()
  await new Promise(resolve => setTimeout(resolve, 0))
  await nextTick()
}

async function flushAnimationFrames(count: number) {
  for (let i = 0; i < count; i++) {
    await new Promise<void>((resolve) => {
      requestAnimationFrame(() => resolve())
    })
  }
}

function installManualRaf() {
  let nextId = 1
  const callbacks = new Map<number, FrameRequestCallback>()
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
    const id = nextId++
    callbacks.set(id, callback)
    return id
  })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => {
    callbacks.delete(id)
  })
  return {
    flushOne() {
      const first = callbacks.entries().next()
      if (first.done) return
      const [id, callback] = first.value
      callbacks.delete(id)
      callback(performance.now())
    },
    pending: () => callbacks.size,
  }
}

beforeEach(() => {
  ResizeObserverMock.instances = []
  vi.stubGlobal('ResizeObserver', ResizeObserverMock)
  vi.stubGlobal('CSS', {
    ...(globalThis.CSS ?? {}),
    escape: (value: string) => value.replaceAll('"', '\\"'),
  })
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
    const id = nextAnimationFrameId++
    const timer = setTimeout(() => {
      animationFrameTimers.delete(id)
      callback(performance.now())
    }, 16)
    animationFrameTimers.set(id, timer)
    return id
  })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => {
    const timer = animationFrameTimers.get(id)
    if (timer) clearTimeout(timer)
    animationFrameTimers.delete(id)
  })
})

afterEach(() => {
  for (const harness of harnesses.splice(0)) {
    harness.app.unmount()
    harness.host.remove()
    harness.viewport.remove()
  }
  for (const timer of animationFrameTimers.values()) clearTimeout(timer)
  animationFrameTimers.clear()
  vi.unstubAllGlobals()
})

describe('animateScrollTo', () => {
  function manualClock() {
    let now = 0
    let nextId = 1
    const frames = new Map<number, FrameRequestCallback>()
    return {
      now: () => now,
      raf: (callback: FrameRequestCallback) => {
        const id = nextId++
        frames.set(id, callback)
        return id
      },
      caf: (id: number) => frames.delete(id),
      step: (elapsed: number) => {
        now += elapsed
        const current = [...frames.values()]
        frames.clear()
        for (const callback of current) callback(now)
      },
      pending: () => frames.size,
    }
  }

  it('lands exactly on the target', () => {
    const clock = manualClock()
    const el = { scrollTop: 20 }

    animateScrollTo(el, () => 220, {
      duration: 400,
      now: clock.now,
      raf: clock.raf,
      caf: clock.caf,
    })
    clock.step(400)

    expect(el.scrollTop).toBe(220)
    expect(clock.pending()).toBe(0)
  })

  it('re-reads a moving target during the tween', () => {
    const clock = manualClock()
    const el = { scrollTop: 0 }
    let target = 100

    animateScrollTo(el, () => target, {
      duration: 400,
      now: clock.now,
      raf: clock.raf,
      caf: clock.caf,
    })
    clock.step(200)
    target = 240
    clock.step(200)

    expect(el.scrollTop).toBe(240)
  })

  it('stops writing after cancellation', () => {
    const clock = manualClock()
    const el = { scrollTop: 0 }
    const cancel = animateScrollTo(el, () => 100, {
      duration: 400,
      now: clock.now,
      raf: clock.raf,
      caf: clock.caf,
    })

    cancel()
    clock.step(400)

    expect(el.scrollTop).toBe(0)
    expect(clock.pending()).toBe(0)
  })
})

describe('useChatScroll gesture and layout handling', () => {
  it('parks follow on upward touch intent before the first scroll event', () => {
    const harness = mountHarness([userMessage('user-1')])
    harness.scrollTo.mockClear()

    harness.viewport.dispatchEvent(touchEvent('touchstart', 100))
    harness.viewport.dispatchEvent(touchEvent('touchmove', 140))
    harness.geometry.scrollHeight = 1_100
    ResizeObserverMock.instances[0]?.trigger(harness.content)

    expect(harness.viewport.scrollTop).toBe(800)
    expect(harness.scrollTo).not.toHaveBeenCalled()
  })

  it('keeps touch escape latched after pointercancel', () => {
    const harness = mountHarness([userMessage('user-1')])
    harness.scrollTo.mockClear()

    harness.viewport.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    harness.viewport.dispatchEvent(touchEvent('touchstart', 100))
    window.dispatchEvent(new Event('pointercancel'))
    window.dispatchEvent(new Event('touchcancel'))
    harness.viewport.scrollTop = 680
    harness.viewport.dispatchEvent(new Event('scroll'))

    expect(harness.viewport.scrollTop).toBe(680)
    expect(harness.scrollTo).not.toHaveBeenCalled()

    harness.geometry.scrollHeight = 1_100
    ResizeObserverMock.instances[0]?.trigger(harness.content)
    expect(harness.scrollTo).not.toHaveBeenCalled()
  })

  it('re-arms follow after a downward wheel reaches the bottom', async () => {
    const harness = mountHarness([userMessage('user-1')])
    harness.scroll.markEscaped()
    harness.viewport.scrollTop = 700
    harness.scrollTo.mockClear()

    harness.viewport.dispatchEvent(new WheelEvent('wheel', { deltaY: 100, bubbles: true }))
    harness.viewport.scrollTop = 800
    harness.viewport.dispatchEvent(new Event('scroll'))
    harness.scrollTo.mockClear()

    harness.geometry.scrollHeight = 1_100
    harness.content.append(document.createTextNode('stream growth'))
    await flushDom()

    expect(harness.scrollTo).toHaveBeenCalledWith({ top: 900, behavior: 'auto' })
  })

  it('follows layout-only content growth reported by ResizeObserver', () => {
    const harness = mountHarness([userMessage('user-1')])
    harness.scrollTo.mockClear()

    harness.geometry.scrollHeight = 1_400
    ResizeObserverMock.instances[0]?.trigger(harness.content)

    expect(harness.scrollTo).toHaveBeenCalledWith({ top: 1_200, behavior: 'auto' })
  })

  it('restores the previous follow mode when a send pin is rolled back', async () => {
    const harness = mountHarness([userMessage('user-1')])
    const rollback = harness.scroll.pinAfterSend()

    rollback()
    await nextTick()
    harness.scrollTo.mockClear()
    harness.geometry.scrollHeight = 1_200
    ResizeObserverMock.instances[0]?.trigger(harness.content)

    expect(harness.scrollTo).toHaveBeenCalledWith({ top: 1_000, behavior: 'auto' })
  })

  it('migrates a pinned reserve when messages are replaced in place', async () => {
    const harness = mountHarness([
      userMessage('user-1'),
      assistantMessage('assistant-1'),
    ])
    harness.geometry.scrollHeight = 800
    harness.geometry.clientHeight = 300
    harness.viewport.scrollTop = 500

    const firstTurn = document.createElement('div')
    const firstPrompt = document.createElement('div')
    firstPrompt.dataset.messageId = 'user-1'
    firstTurn.append(firstPrompt)
    harness.content.append(firstTurn)
    harness.lastTurnEl.value = firstTurn
    await flushDom()

    harness.scroll.pinAfterSend()
    const secondTurn = document.createElement('div')
    const secondPrompt = document.createElement('div')
    secondPrompt.dataset.messageId = 'optimistic-user-2'
    secondTurn.append(secondPrompt)
    secondTurn.getBoundingClientRect = () => rect(120, 60)
    secondPrompt.getBoundingClientRect = () => rect(120, 40)
    Object.defineProperty(secondTurn, 'offsetHeight', {
      configurable: true,
      get: () => Math.max(60, Number.parseFloat(secondTurn.style.minHeight) || 0),
    })
    Object.defineProperty(secondPrompt, 'offsetHeight', {
      configurable: true,
      get: () => 40,
    })

    harness.messages.value.push(
      userMessage('optimistic-user-2'),
      assistantMessage('optimistic-assistant-2'),
    )
    harness.lastTurnEl.value = secondTurn
    harness.content.append(secondTurn)
    await flushDom()

    const reserve = harness.scroll.turnReserveStyle('optimistic-user-2')
    expect(reserve?.minHeight).toMatch(/^\d+px$/)

    harness.messages.value.splice(2, 1, userMessage('server-user-2'))
    await nextTick()

    expect(harness.scroll.turnReserveStyle('optimistic-user-2')).toBeUndefined()
    expect(harness.scroll.turnReserveStyle('server-user-2')).toEqual(reserve)
  })

  it('lands a fresh session at the bottom before arming history pagination', async () => {
    const loadingMessages = ref(true)
    const harness = mountHarness([], ref('session-1'), loadingMessages)
    harness.scroll.onActivatedRestoreScroll()
    harness.geometry.scrollHeight = 900
    harness.viewport.scrollTop = 0

    harness.messages.value.push(userMessage('user-1'))
    loadingMessages.value = false
    await flushDom()
    await new Promise<void>(resolve => requestAnimationFrame(() => resolve()))

    expect(harness.viewport.scrollTop).toBe(700)
    expect(harness.scroll.sessionLandingPending.value).toBe(false)
    expect(harness.scroll.lockScroll.value).toBe(false)
  })

  it('applies exact-top prepend compensation when scrollTop was zero', async () => {
    const harness = mountHarness([userMessage('user-1')])
    const anchorTop = { value: 0 }
    appendMessageAnchor(harness, 'user-1', anchorTop)
    harness.viewport.scrollTop = 0
    harness.geometry.scrollHeight = 1_000
    harness.scroll.beginHistoryPrepend()
    anchorTop.value = 300
    harness.geometry.scrollHeight = 1_300
    harness.scroll.finishHistoryPrepend()

    expect(harness.viewport.scrollTop).toBe(300)
  })

  it('keeps a non-zero reading viewport fixed across history prepend', () => {
    const harness = mountHarness([userMessage('user-1')])
    const anchorTop = { value: 120 }
    appendMessageAnchor(harness, 'user-1', anchorTop)
    harness.viewport.scrollTop = 120
    harness.geometry.scrollHeight = 1_000
    harness.scroll.beginHistoryPrepend()
    anchorTop.value = 420
    harness.geometry.scrollHeight = 1_300
    harness.scroll.finishHistoryPrepend()

    expect(harness.viewport.scrollTop).toBe(420)
  })

  it('does not count streaming growth below the anchor as prepended history', () => {
    const harness = mountHarness([userMessage('user-1')])
    const anchorTop = { value: 0 }
    appendMessageAnchor(harness, 'user-1', anchorTop)
    harness.viewport.scrollTop = 0
    harness.geometry.scrollHeight = 1_000
    harness.scroll.beginHistoryPrepend()

    // 300px was inserted above the old row while another 500px streamed
    // below it. Only the old row's 300px displacement belongs to prepend.
    anchorTop.value = 300
    harness.geometry.scrollHeight = 1_800
    harness.scroll.finishHistoryPrepend()

    expect(harness.viewport.scrollTop).toBe(300)
  })

  it('still compensates after upward wheel at exact top during fetch', async () => {
    const harness = mountHarness([userMessage('user-1')])
    const anchorTop = { value: 0 }
    appendMessageAnchor(harness, 'user-1', anchorTop)
    harness.viewport.scrollTop = 0
    harness.geometry.scrollHeight = 1_000
    harness.scroll.beginHistoryPrepend()
    harness.viewport.dispatchEvent(new WheelEvent('wheel', { deltaY: -10, bubbles: true }))
    anchorTop.value = 300
    harness.geometry.scrollHeight = 1_300
    harness.scroll.finishHistoryPrepend()

    expect(harness.viewport.scrollTop).toBe(300)
  })

  it('skips exact-top compensation after downward wheel during fetch', async () => {
    const harness = mountHarness([userMessage('user-1')])
    const anchorTop = { value: 0 }
    appendMessageAnchor(harness, 'user-1', anchorTop)
    harness.viewport.scrollTop = 0
    harness.geometry.scrollHeight = 1_000
    harness.scroll.beginHistoryPrepend()
    harness.viewport.dispatchEvent(new WheelEvent('wheel', { deltaY: 10, bubbles: true }))
    anchorTop.value = 300
    harness.geometry.scrollHeight = 1_300
    harness.scroll.finishHistoryPrepend()

    expect(harness.viewport.scrollTop).toBe(0)
  })

  it('ignores stale prepend capture after session switch', async () => {
    const sessionId = ref('session-a')
    const loadingMessages = ref(true)
    const harness = mountHarness([userMessage('user-1')], sessionId, loadingMessages)
    const anchorTop = { value: 0 }
    appendMessageAnchor(harness, 'user-1', anchorTop)
    harness.viewport.scrollTop = 0
    harness.geometry.scrollHeight = 1_000
    harness.scroll.beginHistoryPrepend()
    sessionId.value = 'session-b'
    await nextTick()
    anchorTop.value = 300
    harness.geometry.scrollHeight = 1_300
    harness.scroll.finishHistoryPrepend()

    expect(harness.viewport.scrollTop).toBe(0)
  })

  it('does not land from the pre-bind false state when loading starts in the same tick', async () => {
    const loadingMessages = ref(false)
    const sessionId = ref('session-a')
    const harness = mountHarness([], sessionId, loadingMessages)

    sessionId.value = 'session-b'
    loadingMessages.value = true
    await nextTick()

    expect(harness.scroll.sessionLandingPending.value).toBe(true)
    expect(harness.scroll.lockScroll.value).toBe(true)
  })

  it('locks landing again when the session id changes', async () => {
    const sessionId = ref('session-1')
    const harness = mountHarness([userMessage('user-1')], sessionId)
    expect(harness.scroll.sessionLandingPending.value).toBe(false)

    sessionId.value = 'session-2'
    await nextTick()

    expect(harness.scroll.lockScroll.value).toBe(true)
    expect(harness.scroll.sessionLandingPending.value).toBe(true)
  })

  it('keeps pagination locked until anchor offset restore completes', async () => {
    const loadingMessages = ref(true)
    const harness = mountHarness(
      [userMessage('msg-1'), userMessage('msg-2')],
      ref('session-1'),
      loadingMessages,
    )
    harness.viewport.scrollTop = 100
    harness.scroll.onMessageActive(true, { id: 'msg-1', top: 48 })
    harness.scroll.onDeactivatedResetScroll()
    harness.scroll.onActivatedRestoreScroll()

    const msgEl = document.createElement('div')
    msgEl.dataset.messageId = 'msg-1'
    msgEl.scrollIntoView = vi.fn()
    harness.content.append(msgEl)

    loadingMessages.value = false
    await nextTick()

    expect(harness.scroll.lockScroll.value).toBe(true)
    expect(harness.scroll.sessionLandingPending.value).toBe(true)

    await flushAnimationFrames(3)

    expect(harness.scroll.lockScroll.value).toBe(false)
    expect(harness.scroll.sessionLandingPending.value).toBe(false)
    expect(harness.scrollBy).toHaveBeenCalledWith({ top: -48 })
  })

  it('does not fresh-bottom land while anchor restore is in progress', async () => {
    const loadingMessages = ref(true)
    const harness = mountHarness(
      [userMessage('msg-1'), userMessage('msg-2')],
      ref('session-1'),
      loadingMessages,
    )
    harness.viewport.scrollTop = 100
    harness.scroll.onMessageActive(true, { id: 'msg-1', top: 48 })
    harness.scroll.onDeactivatedResetScroll()
    harness.scroll.onActivatedRestoreScroll()

    const msgEl = document.createElement('div')
    msgEl.dataset.messageId = 'msg-1'
    msgEl.scrollIntoView = vi.fn()
    harness.content.append(msgEl)

    loadingMessages.value = false
    await nextTick()

    ResizeObserverMock.instances[0]?.trigger(harness.content)
    expect(harness.viewport.scrollTop).toBe(100)

    await flushAnimationFrames(3)
    expect(harness.viewport.scrollTop).toBe(52)
  })

  it('restores anchor from the current scroll root only', async () => {
    const loadingMessages = ref(true)
    const harness = mountHarness(
      [userMessage('msg-1')],
      ref('session-1'),
      loadingMessages,
    )
    harness.viewport.scrollTop = 100
    harness.scroll.onMessageActive(true, { id: 'msg-1', top: 32 })
    harness.scroll.onDeactivatedResetScroll()
    harness.scroll.onActivatedRestoreScroll()

    const foreignRoot = document.createElement('div')
    const foreignMsg = document.createElement('div')
    foreignMsg.dataset.messageId = 'msg-1'
    foreignRoot.append(foreignMsg)
    document.body.append(foreignRoot)

    const paneMsg = document.createElement('div')
    paneMsg.dataset.messageId = 'msg-1'
    const paneSpy = vi.fn()
    paneMsg.scrollIntoView = paneSpy
    harness.content.append(paneMsg)
    const foreignSpy = vi.fn()
    foreignMsg.scrollIntoView = foreignSpy

    loadingMessages.value = false
    await nextTick()
    await flushAnimationFrames(3)

    expect(paneSpy).toHaveBeenCalled()
    expect(foreignSpy).not.toHaveBeenCalled()
    foreignRoot.remove()
  })

  it('lands after one activation binding tick when no load is needed', async () => {
    const loadingMessages = ref(false)
    const harness = mountHarness([userMessage('msg-1')], ref('session-1'), loadingMessages)

    expect(() => {
      harness.scroll.onActivatedRestoreScroll()
    }).not.toThrow()

    await nextTick()
    await flushAnimationFrames(1)
    expect(harness.scroll.sessionLandingPending.value).toBe(false)
    expect(harness.scroll.lockScroll.value).toBe(false)
  })

  it('waits for a KeepAlive rebind load that starts after the child activated hook', async () => {
    const manualRaf = installManualRaf()
    const loadingMessages = ref(false)
    const harness = mountHarness([userMessage('cached')], ref('session-1'), loadingMessages)
    harness.geometry.scrollHeight = 900
    harness.viewport.scrollTop = 0

    harness.scroll.onDeactivatedResetScroll()
    harness.scroll.onActivatedRestoreScroll()
    // panel-chat's activated hook binds the view after the child hook; the
    // transcript starts loading in that same Vue activation turn.
    loadingMessages.value = true
    await nextTick()

    expect(harness.viewport.scrollTop).toBe(0)
    expect(harness.scroll.sessionLandingPending.value).toBe(true)
    expect(manualRaf.pending()).toBe(0)

    harness.messages.value = [userMessage('latest')]
    loadingMessages.value = false
    await nextTick()

    expect(harness.viewport.scrollTop).toBe(700)
    expect(harness.scroll.sessionLandingPending.value).toBe(true)
    expect(manualRaf.pending()).toBe(1)

    manualRaf.flushOne()
    expect(harness.scroll.sessionLandingPending.value).toBe(false)
    expect(harness.scroll.lockScroll.value).toBe(false)
  })

  it('ignores stale fresh-landing rAF after session switch', async () => {
    const manualRaf = installManualRaf()
    const sessionId = ref('session-a')
    const loadingMessages = ref(false)
    const harness = mountHarness([userMessage('msg-1')], sessionId, loadingMessages)

    harness.scroll.onActivatedRestoreScroll()
    await nextTick()
    expect(manualRaf.pending()).toBe(1)

    loadingMessages.value = true
    sessionId.value = 'session-b'
    await nextTick()
    loadingMessages.value = false
    await nextTick()

    expect(harness.scroll.lockScroll.value).toBe(true)
    expect(harness.scroll.sessionLandingPending.value).toBe(true)
    expect(harness.viewport.scrollTop).toBe(800)
    expect(manualRaf.pending()).toBeGreaterThan(0)

    manualRaf.flushOne()
    expect(manualRaf.pending()).toBeGreaterThan(0)

    harness.viewport.scrollTop = 750
    harness.viewport.dispatchEvent(new Event('scroll'))
    expect(harness.viewport.scrollTop).toBe(750)
  })

  it('ignores stale anchor-restore rAF after session switch', async () => {
    const manualRaf = installManualRaf()
    const sessionId = ref('session-a')
    const loadingMessages = ref(true)
    const harness = mountHarness([userMessage('msg-1')], sessionId, loadingMessages)
    harness.viewport.scrollTop = 100
    harness.scroll.onMessageActive(true, { id: 'msg-1', top: 48 })
    harness.scroll.onDeactivatedResetScroll()
    harness.scroll.onActivatedRestoreScroll()

    const msgEl = document.createElement('div')
    msgEl.dataset.messageId = 'msg-1'
    msgEl.scrollIntoView = vi.fn()
    harness.content.append(msgEl)

    loadingMessages.value = false
    await nextTick()

    loadingMessages.value = true
    sessionId.value = 'session-b'
    await nextTick()
    loadingMessages.value = false
    await nextTick()

    expect(harness.scroll.lockScroll.value).toBe(true)
    expect(harness.scroll.sessionLandingPending.value).toBe(true)
    expect(harness.viewport.scrollTop).toBe(800)
    expect(manualRaf.pending()).toBeGreaterThan(0)

    manualRaf.flushOne()
    expect(manualRaf.pending()).toBeGreaterThan(0)

    harness.viewport.scrollTop = 750
    harness.viewport.dispatchEvent(new Event('scroll'))
    expect(harness.viewport.scrollTop).toBe(750)
  })
})
