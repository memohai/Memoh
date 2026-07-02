import type { Ref } from 'vue'
import {
  computed,
  nextTick,
  onBeforeUnmount,
  ref,
  watch,
  watchEffect,
} from 'vue'
import { useElementBounding, useScroll } from '@vueuse/core'
import type { ChatMessage } from '@/store/chat-list'

export interface ScrollTweenOptions {
  duration?: number
  now?: () => number
  raf?: (cb: FrameRequestCallback) => number
  caf?: (handle: number) => void
}

// The tween re-reads its target every frame, so positions shifted by
// late layout settles (markdown re-render, code highlighting, image
// loads, KaTeX/Mermaid resolves) still land exactly.
export function animateScrollTo(
  el: { scrollTop: number },
  getTarget: () => number,
  options: ScrollTweenOptions = {},
): () => void {
  const duration = options.duration ?? 450
  const now = options.now ?? (() => performance.now())
  const raf = options.raf ?? (cb => requestAnimationFrame(cb))
  const caf = options.caf ?? (handle => cancelAnimationFrame(handle))
  const start = el.scrollTop
  const startedAt = now()
  let cancelled = false
  let handle = 0
  const frame = () => {
    if (cancelled) return
    const progress = duration > 0 ? Math.min(1, (now() - startedAt) / duration) : 1
    const eased = 1 - (1 - progress) ** 5
    el.scrollTop = start + (getTarget() - start) * eased
    if (progress < 1) handle = raf(frame)
  }
  handle = raf(frame)
  return () => {
    if (cancelled) return
    cancelled = true
    caf(handle)
  }
}

export interface UseChatScrollOptions {
  scrollEl: Ref<HTMLElement | null>
  /** Content-height probe — the scroll viewport's first child. */
  contentEl: Ref<HTMLElement | null>
  messages: Ref<ChatMessage[]>
  isActive: Ref<boolean>
  sessionId: Ref<string | null | undefined>
}

/**
 * Owns every scroll behavior for the chat message list: stick-to-bottom
 * follow, user-scroll escape/relock, jump-to-message, prepend-load
 * suppression, and cross-tab (KeepAlive) position restore.
 *
 * Extracted from chat-pane.vue verbatim — behavior is unchanged from the
 * pre-extraction implementation, only the ownership boundary moved.
 */
export function useChatScroll(options: UseChatScrollOptions) {
  const { scrollEl, contentEl, messages, isActive, sessionId } = options

  const isAutoScroll = ref(true)
  const isInstant = ref(false)
  const highlightedMessageId = ref('')

  const { y, directions, arrivedState, isScrolling } = useScroll(scrollEl, {
    behavior: computed(() => (isAutoScroll.value && isInstant.value ? 'smooth' : 'instant')),
  })
  const { height } = useElementBounding(contentEl)

  let highlightTimer: ReturnType<typeof setTimeout> | null = null
  let cancelScrollTween: (() => void) | null = null

  function startScrollTween(root: HTMLElement, getTarget: () => number) {
    cancelScrollTween?.()
    const stop = animateScrollTo(root, () => {
      const max = Math.max(root.scrollHeight - root.clientHeight, 0)
      return Math.min(Math.max(getTarget(), 0), max)
    })
    const cancel = () => {
      stop()
      root.removeEventListener('wheel', cancel)
      root.removeEventListener('touchstart', cancel)
      cancelScrollTween = null
    }
    root.addEventListener('wheel', cancel, { passive: true })
    root.addEventListener('touchstart', cancel, { passive: true })
    cancelScrollTween = cancel
  }

  function getElementAbsoluteTop(target: HTMLElement, root: HTMLElement) {
    return root.scrollTop + target.getBoundingClientRect().top - root.getBoundingClientRect().top
  }

  function scrollViewportTo(getTop: () => number) {
    const root = scrollEl.value
    if (!root) return
    startScrollTween(root, getTop)
  }

  function scrollToBottom() {
    const root = scrollEl.value
    if (!root) return
    isAutoScroll.value = true
    isInstant.value = true
    scrollViewportTo(() => root.scrollHeight)
  }

  function findMessageElement(messageId: string): HTMLElement | null {
    const root = scrollEl.value
    if (!root) return null
    return root.querySelector<HTMLElement>(`[data-message-id="${CSS.escape(messageId)}"]`)
  }

  async function scrollToMessage(messageId: string): Promise<boolean> {
    await nextTick()
    const root = scrollEl.value
    const target = findMessageElement(messageId)
    if (!root || !target) return false
    isAutoScroll.value = false
    isInstant.value = false
    const scrollMargin = Number.parseFloat(getComputedStyle(target).scrollMarginTop) || 0
    startScrollTween(root, () => {
      const el = findMessageElement(messageId)
      return el ? getElementAbsoluteTop(el, root) - scrollMargin : root.scrollTop
    })
    highlightedMessageId.value = messageId
    if (highlightTimer) clearTimeout(highlightTimer)
    highlightTimer = setTimeout(() => {
      if (highlightedMessageId.value === messageId) {
        highlightedMessageId.value = ''
      }
    }, 1800)
    return true
  }

  const showJumpToBottom = computed(() =>
    isActive.value
    && messages.value.length > 0
    && !arrivedState.bottom,
  )

  // Tracks the viewport-relative top offset of every "active" message element so
  // onActivated can restore scroll to the same anchor. Keyed by message id for
  // O(1) update/remove on every active/inactive transition; long conversations
  // would otherwise pay a linear scan + splice on each transition.
  const elId = new Map<string, number>()
  const lockScroll = ref(true)

  function onMessageActive(active: boolean, item: { id: string, top: number }) {
    if (lockScroll.value) return
    if (active) {
      elId.set(item.id, item.top)
    } else {
      elId.delete(item.id)
    }
  }

  // Drop accumulated anchors when the active session changes. Otherwise an
  // anchor for a message that only exists in session B would survive into A
  // when the user switches back, and the onActivated restore would query
  // the DOM with a foreign id (or worse, find a coincidentally-matching
  // element from the new session's load). Scroll position restoration is
  // preserved across route activation but reset across cross-session
  // switches.
  watch(sessionId, () => {
    elId.clear()
  })

  watch(isScrolling, (scrolling) => {
    if (scrolling || lockScroll.value || !isActive.value) return
    for (const [id] of elId) {
      const el = findMessageElement(id)
      if (el) elId.set(id, el.getBoundingClientRect().top - 48)
    }
  })

  let isInit = false

  function onActivatedRestoreScroll(loadingMessages: Ref<boolean>) {
    if (!isActive.value) return
    let done = false
    const unwatch = watch(loadingMessages, async (newValue) => {
      if (done) return
      try {
        // Pick the anchor closest to the top edge of the viewport so the
        // restore lands on the message the user was reading rather than an
        // arbitrary entry from earlier hover state.
        let anchorId: string | undefined
        let anchorTop = Number.POSITIVE_INFINITY
        for (const [id, top] of elId) {
          if (Math.abs(top) < Math.abs(anchorTop)) {
            anchorId = id
            anchorTop = top
          }
        }

        if (anchorId && !newValue) {
          const el: HTMLElement | null = document.querySelector(`[data-message-id="${anchorId}"]`)
          if (el) {
            const cachePos = anchorTop
            el.scrollIntoView()
            requestAnimationFrame(() => {
              requestAnimationFrame(() => {
                scrollEl.value?.scrollBy({
                  top: -cachePos,
                })
              })
            })
          }
          setTimeout(() => {
            lockScroll.value = false
            isInit = true
            done = true
            unwatch()
          })
        } else {
          isInit = true
          if (!newValue) {
            setTimeout(async () => {
              lockScroll.value = false
              done = true
              unwatch()
            })
          }
        }
      } catch (error) {
        done = true
        unwatch()
        throw error
      }
    }, {
      immediate: true,
      flush: 'post',
    })
  }

  function onDeactivatedResetScroll() {
    lockScroll.value = true
    isInstant.value = false
    isAutoScroll.value = true
    isInit = false
    if (arrivedState.bottom) {
      elId.clear()
    }
  }

  watchEffect(() => {
    if (!isActive.value) return
    if (directions.top && !lockScroll.value) {
      isAutoScroll.value = false
      isInstant.value = false
      return
    }

    if (arrivedState.bottom && !lockScroll.value) {
      isAutoScroll.value = true
      isInstant.value = true
      return
    }
  })

  watch([isAutoScroll, height, isActive], async () => {
    if (!isActive.value) return
    if (isAutoScroll.value && height.value && isInit) {
      y.value = height.value
    }
  }, {
    flush: 'post',
    deep: true,
  })

  // Sentinel-based infinite scroll for older history. Fires once per
  // IntersectionObserver transition: load one batch. We do NOT manually
  // reposition scrollTop after the prepend.
  //
  // Why no manual compensation: the browser's `overflow-anchor: auto`
  // already keeps the visible content stationary across a prepend when
  // `scrollTop > 0`, which is the case whenever the user is reading mid-
  // history. When the user has scrolled all the way to `scrollTop === 0`,
  // the spec deliberately suppresses overflow-anchor to avoid jitter at
  // the top of a document — and that's exactly what we want: leaving
  // scrollTop at 0 means the freshly-prepended older messages render at
  // the top of the viewport, which is what a user who just scrolled to
  // the top to see older history actually wants to see.
  //
  // Prior versions of this function ran an offset-from-bottom or anchor-
  // based scrollTop correction after each prepend. Both produced a
  // visible discontinuity: the user saw new content for one frame, then
  // got yanked to a different scroll position — the "scroll jumps back"
  // symptom users reported. The browser already does the right thing on
  // both sides of the scrollTop=0 boundary; our job is just to suppress
  // the `isAutoScroll`-driven jump-to-bottom and let the prepend land.
  function suppressAutoScrollForPrepend() {
    // The `watch([isAutoScroll, height, isActive], ...)` effect slams
    // scrollTop to the bottom whenever content height grows and
    // isAutoScroll is true. Prepend grows height, would fire that, would
    // hurl the user back to the bottom. arrivedState.bottom will re-
    // enable it when the user scrolls back down to the latest messages.
    isAutoScroll.value = false
  }

  onBeforeUnmount(() => {
    if (highlightTimer) clearTimeout(highlightTimer)
    cancelScrollTween?.()
  })

  return {
    // state
    y,
    directions,
    arrivedState,
    isScrolling,
    isAutoScroll,
    isInstant,
    lockScroll,
    highlightedMessageId,
    showJumpToBottom,

    // primary actions
    scrollToBottom,
    scrollToMessage,
    suppressAutoScrollForPrepend,

    // lifecycle hooks — call sites live in chat-pane.vue's own onActivated/onDeactivated
    onActivatedRestoreScroll,
    onDeactivatedResetScroll,

    // message-item @active contract
    onMessageActive,

    // low-level primitives kept public for the scroll rail (out of scope for
    // this extraction — the rail's own trigger logic still calls these directly)
    scrollViewportTo,
    startScrollTween,
    findMessageElement,
    getElementAbsoluteTop,
  }
}
