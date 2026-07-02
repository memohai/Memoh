import type { Ref } from 'vue'
import {
  computed,
  nextTick,
  onBeforeUnmount,
  ref,
  watch,
} from 'vue'
import { useScroll } from '@vueuse/core'
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

const TWEEN_DURATION_MS = 450

// "At the bottom" is a threshold, not a pixel-perfect landing: sub-pixel
// rounding, the last line growing mid-stream, and fractional zoom all leave a
// few px of slack that must still count as "following". 30px stays under one
// line of body text, so a deliberate scroll-up still unlocks.
const NEAR_BOTTOM_THRESHOLD_PX = 30

// After the user scrolls down but stops short of the bottom, re-arm the follow
// this soon — optimistic relock so a near-miss doesn't strand them off the
// stream, without relocking on every mid-scroll pause.
const RELOCK_DELAY_MS = 100

export interface UseChatScrollOptions {
  scrollEl: Ref<HTMLElement | null>
  /** Content-height probe — the scroll viewport's first child. */
  contentEl: Ref<HTMLElement | null>
  messages: Ref<ChatMessage[]>
  isActive: Ref<boolean>
  sessionId: Ref<string | null | undefined>
}

/**
 * useChatScroll — every scroll behavior for the chat message list.
 *
 * ─── What it does ─────────────────────────────────────────────────────────
 *   • stick-to-bottom follow while a reply streams in
 *   • let the user scroll away ("escape") and STAY there, then re-arm follow
 *     when they come back to the bottom
 *   • jump-to-message + transient highlight (reply refs, the scroll rail)
 *   • keep the viewport still while older history is prepended at the top
 *   • restore scroll position across KeepAlive tab switches; land a freshly
 *     opened session at the bottom
 *
 * ─── The one idea the whole file rests on ─────────────────────────────────
 * Follow and escape pull the scroll position in opposite directions, so
 * everything hinges on telling *the code's own scrolling* apart from *the
 * user's*. They are indistinguishable at the `scroll`-event layer — both just
 * fire `scroll`. The naive design (which this file was rewritten to kill) reads
 * `scroll` direction to decide "did the user leave?" while also snapping to the
 * bottom on every content growth; the follow's own scroll events then read as
 * user activity, the two fight every frame, and the user physically cannot
 * scroll away from a streaming reply.
 *
 * The fix is two independent guards:
 *   1. `isProgrammaticScroll` brackets every scroll the code performs; the
 *      `scroll` handler ignores events while it is set, so a follow scroll is
 *      never misread as the user leaving.
 *   2. Escape is latched only from signals a programmatic scroll cannot forge:
 *      a physical `wheel` event, and a per-frame `scrollTop` delta.
 *
 * ─── State model ──────────────────────────────────────────────────────────
 * The hot-path latches are plain closure vars, NOT refs on purpose: they move
 * on every frame of a scroll and must never trigger a re-render. Exactly ONE
 * reactive mirror, `isAtBottom`, is exposed to the UI (the jump-to-bottom
 * button); update it wherever scrollTop changes, never read the latches from a
 * template.
 *
 *   isProgrammaticScroll  code is mid-scroll → treat scroll events as "ours"
 *   userEscaped           user left the bottom → follow is suppressed
 *   lastScrollTop         previous frame's scrollTop, for the up/down test
 *   relockTimer           optimistic re-arm after a short down-scroll
 *   lockScroll (ref)      init / cross-tab restore running → freeze BOTH follow
 *                         and escape so their setup scrolls latch nothing
 *
 * ─── Event flow ───────────────────────────────────────────────────────────
 *   MutationObserver(content subtree) ─ streaming mutates the DOM ─▶ follow to
 *       the bottom, gated on !userEscaped. This is the follow heartbeat. Growth
 *       is sensed via DOM mutation, deliberately NOT a height ResizeObserver +
 *       "scrollTop = height" snap — that snap was half of the original bug.
 *   wheel  ─▶ physical intent; deltaY<0 (up) escapes immediately.
 *   scroll ─▶ refresh isAtBottom; if it is not our own scroll, run the latch.
 *
 * ─── Relock (how following comes back) ────────────────────────────────────
 *   • user scrolls down into the 30px bottom band        → relock now
 *   • user scrolls down but stops short                  → relock after a short
 *                                                          optimistic timer
 *   • a programmatic follow confirms it reached bottom   → relock
 *   • user sends a message (chat-pane → followBottom)    → relock
 *   • the active session changes                         → relock + land bottom
 *
 * ─── Prepend (older history) ──────────────────────────────────────────────
 * Loading older messages is treated as an escape (suppressAutoScrollForPrepend
 * → markEscaped): follow stays off, and the browser's native `overflow-anchor`
 * keeps the visible content stationary across the insert. There is NO manual
 * scrollTop compensation — and you must NOT set `overflow-anchor: none` on the
 * viewport, or that native anchoring stops working and the list jumps.
 *
 * ─── chat-pane contract ───────────────────────────────────────────────────
 * chat-pane owns the DOM refs and drives this composable through:
 *   scrollToBottom (jump button) · scrollToMessage (reply refs) · followBottom
 *   (on send) · suppressAutoScrollForPrepend (top sentinel) · markEscaped +
 *   startScrollTween + findMessageElement + getElementAbsoluteTop (scroll rail)
 *   · onMessageActive (per message-item) · onActivatedRestoreScroll /
 *   onDeactivatedResetScroll (its own KeepAlive hooks).
 */
export function useChatScroll(options: UseChatScrollOptions) {
  const { scrollEl, messages, isActive, sessionId } = options

  const highlightedMessageId = ref('')
  // Reactive mirror of "is the viewport at the bottom". This is the ONLY
  // follow/escape state that feeds the UI (the jump-to-bottom button); the
  // hot-path latches below are deliberately non-reactive so a scroll storm
  // never triggers re-renders.
  const isAtBottom = ref(true)
  // Held true during session load / cross-tab restore so neither the follow
  // nor the escape latch reacts to the programmatic scrolls those flows make.
  const lockScroll = ref(true)

  const { isScrolling } = useScroll(scrollEl)

  // --- Follow / escape latches (non-reactive on purpose) ---
  // True around a scroll the code itself performs, so the resulting scroll
  // event is not misread as the user leaving the bottom.
  let isProgrammaticScroll = false
  let lastScrollTop = 0
  // The user has scrolled away; while true, streaming never drags the viewport
  // back down.
  let userEscaped = false
  let relockTimer: ReturnType<typeof setTimeout> | null = null

  let highlightTimer: ReturnType<typeof setTimeout> | null = null
  let cancelScrollTween: (() => void) | null = null
  let tweenFlagTimer: ReturnType<typeof setTimeout> | null = null
  let mutationObserver: MutationObserver | null = null

  function isNearBottom(el: HTMLElement): boolean {
    return el.scrollHeight - el.scrollTop - el.clientHeight < NEAR_BOTTOM_THRESHOLD_PX
  }

  function clearRelockTimer() {
    if (relockTimer) {
      clearTimeout(relockTimer)
      relockTimer = null
    }
  }

  // Stop following, immediately. Called for any deliberate move away from the
  // bottom (jump-to-message, rail navigation, prepend of older history).
  function markEscaped() {
    userEscaped = true
    clearRelockTimer()
  }

  // Re-arm following. The MutationObserver picks it back up on the next content
  // growth; used when the user sends (the reply should stream into view).
  function followBottom() {
    userEscaped = false
    clearRelockTimer()
  }

  function startScrollTween(root: HTMLElement, getTarget: () => number) {
    cancelScrollTween?.()
    // A tween is a programmatic scroll; flag it for its whole run so its
    // per-frame scrollTop moves are never latched as a user escape.
    isProgrammaticScroll = true
    if (tweenFlagTimer) {
      clearTimeout(tweenFlagTimer)
      tweenFlagTimer = null
    }
    const stop = animateScrollTo(root, () => {
      const max = Math.max(root.scrollHeight - root.clientHeight, 0)
      return Math.min(Math.max(getTarget(), 0), max)
    }, { duration: TWEEN_DURATION_MS })
    const cancel = () => {
      stop()
      isProgrammaticScroll = false
      if (tweenFlagTimer) {
        clearTimeout(tweenFlagTimer)
        tweenFlagTimer = null
      }
      root.removeEventListener('wheel', cancel)
      root.removeEventListener('touchstart', cancel)
      cancelScrollTween = null
    }
    root.addEventListener('wheel', cancel, { passive: true })
    root.addEventListener('touchstart', cancel, { passive: true })
    cancelScrollTween = cancel
    // animateScrollTo has no completion callback; drop the flag once the tween
    // can no longer be running.
    tweenFlagTimer = setTimeout(() => {
      isProgrammaticScroll = false
      tweenFlagTimer = null
    }, TWEEN_DURATION_MS + 100)
  }

  function getElementAbsoluteTop(target: HTMLElement, root: HTMLElement) {
    return root.scrollTop + target.getBoundingClientRect().top - root.getBoundingClientRect().top
  }

  // Instant follow used by the MutationObserver during streaming. Marks itself
  // programmatic so the scroll it triggers is not read as a user action.
  //
  // Timing: `scrollTo` dispatches its `scroll` event before the next rAF fires,
  // so the scroll handler runs while `isProgrammaticScroll` is still true and
  // correctly ignores it. The rAF then clears the flag and, if we truly landed
  // at the bottom, confirms the relock. Do not "simplify" by clearing the flag
  // synchronously — the scroll event would then latch a spurious escape.
  function stickToBottomNow() {
    const el = scrollEl.value
    if (!el) return
    isProgrammaticScroll = true
    el.scrollTo({ top: el.scrollHeight, behavior: 'auto' })
    requestAnimationFrame(() => {
      isProgrammaticScroll = false
      const cur = scrollEl.value
      if (!cur) return
      isAtBottom.value = isNearBottom(cur)
      if (isAtBottom.value) {
        userEscaped = false
        lastScrollTop = cur.scrollTop
      }
    })
  }

  // Deliberate "go to the latest" — the jump-to-bottom button. Re-arms follow
  // and eases down; the MutationObserver keeps it pinned once there.
  function scrollToBottom() {
    const root = scrollEl.value
    if (!root) return
    followBottom()
    startScrollTween(root, () => root.scrollHeight)
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
    // Landing on a specific message parks the reader there — stop following.
    markEscaped()
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
    && !isAtBottom.value,
  )

  // Tracks the viewport-relative top offset of every "active" message element so
  // onActivated can restore scroll to the same anchor. Keyed by message id for
  // O(1) update/remove on every active/inactive transition; long conversations
  // would otherwise pay a linear scan + splice on each transition.
  const elId = new Map<string, number>()

  function onMessageActive(active: boolean, item: { id: string, top: number }) {
    if (lockScroll.value) return
    if (active) {
      elId.set(item.id, item.top)
    } else {
      elId.delete(item.id)
    }
  }

  // Drop accumulated anchors when the active session changes, and land the new
  // session at the bottom regardless of where the user was parked in the old
  // one. Otherwise a stale anchor (or a lingering escape) would survive the
  // switch and either restore against a foreign id or leave the new session
  // stuck mid-history.
  watch(sessionId, () => {
    elId.clear()
    followBottom()
  })

  watch(isScrolling, (scrolling) => {
    if (scrolling || lockScroll.value || !isActive.value) return
    for (const [id] of elId) {
      const el = findMessageElement(id)
      if (el) elId.set(id, el.getBoundingClientRect().top - 48)
    }
  })

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
            done = true
            unwatch()
            // Restored to a remembered position: follow only if that position
            // is already at the bottom, so a mid-history restore is not yanked
            // down by the first streamed mutation.
            const root = scrollEl.value
            userEscaped = root ? !isNearBottom(root) : false
            if (root) isAtBottom.value = !userEscaped
          })
        } else {
          if (!newValue) {
            setTimeout(() => {
              lockScroll.value = false
              done = true
              unwatch()
              // No remembered anchor (fresh load / previously at bottom): land
              // at the latest message.
              followBottom()
              stickToBottomNow()
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
    followBottom()
    const el = scrollEl.value
    if (el && isNearBottom(el)) {
      elId.clear()
    }
  }

  // --- User-intent detection: the escape latch ---
  // `wheel` is a user-only signal — a programmatic scroll never fires it — so
  // it is the trustworthy source for "the user is moving the view".
  function onWheel(ev: WheelEvent) {
    isProgrammaticScroll = false
    handleUserScroll(ev.deltaY < 0)
  }

  function onScrollEvent() {
    const el = scrollEl.value
    if (!el) return
    const top = el.scrollTop
    const isScrollingUp = top < lastScrollTop
    lastScrollTop = top
    isAtBottom.value = isNearBottom(el)
    // A scroll we triggered ourselves only updates the at-bottom mirror; it
    // must never move the escape latch.
    if (isProgrammaticScroll) return
    handleUserScroll(isScrollingUp)
  }

  function handleUserScroll(isScrollingUp: boolean) {
    const el = scrollEl.value
    if (!el) return
    if (lockScroll.value) return
    const near = isNearBottom(el)
    clearRelockTimer()
    if (isScrollingUp) {
      // Any upward move escapes immediately.
      userEscaped = true
    } else if (near) {
      // Scrolled down and reached the bottom: relock now.
      userEscaped = false
    } else {
      // Scrolled down but still short of the bottom: stay escaped, but
      // optimistically relock shortly after so a near-miss re-arms follow.
      userEscaped = true
      relockTimer = setTimeout(() => {
        userEscaped = false
        relockTimer = null
      }, RELOCK_DELAY_MS)
    }
  }

  // Content growth is the follow heartbeat: streaming tokens mutate the DOM
  // subtree, so re-pin to the bottom while the user has not escaped.
  function onContentMutated() {
    const el = scrollEl.value
    if (!el) return
    if (!isActive.value || lockScroll.value) return
    if (!userEscaped) stickToBottomNow()
    else isAtBottom.value = isNearBottom(el)
  }

  function attach(el: HTMLElement) {
    lastScrollTop = el.scrollTop
    el.addEventListener('wheel', onWheel, { passive: true })
    el.addEventListener('scroll', onScrollEvent, { passive: true })
    mutationObserver = new MutationObserver(onContentMutated)
    // childList catches new bubbles/token spans; characterData catches text
    // that streams into an existing node — either can be the stream's growth.
    mutationObserver.observe(el, { childList: true, subtree: true, characterData: true })
  }

  function detach(el: HTMLElement | null) {
    if (el) {
      el.removeEventListener('wheel', onWheel)
      el.removeEventListener('scroll', onScrollEvent)
    }
    mutationObserver?.disconnect()
    mutationObserver = null
  }

  watch(scrollEl, (el, old) => {
    detach(old ?? null)
    if (el) attach(el)
  }, { immediate: true })

  // Prepend of older history is a deliberate move away from the bottom, so it
  // escapes: the browser's native `overflow-anchor` keeps the visible content
  // stationary across the insert, and the follow stays off until the user
  // scrolls back down. No manual scrollTop compensation needed.
  function suppressAutoScrollForPrepend() {
    markEscaped()
  }

  onBeforeUnmount(() => {
    if (highlightTimer) clearTimeout(highlightTimer)
    clearRelockTimer()
    if (tweenFlagTimer) clearTimeout(tweenFlagTimer)
    cancelScrollTween?.()
    detach(scrollEl.value)
  })

  return {
    // state
    isScrolling,
    lockScroll,
    highlightedMessageId,
    showJumpToBottom,

    // primary actions
    scrollToBottom,
    scrollToMessage,
    suppressAutoScrollForPrepend,
    markEscaped,
    followBottom,

    // lifecycle hooks — call sites live in chat-pane.vue's own onActivated/onDeactivated
    onActivatedRestoreScroll,
    onDeactivatedResetScroll,

    // message-item @active contract
    onMessageActive,

    // low-level primitives kept public for the scroll rail (the rail's own
    // trigger logic still calls these directly)
    startScrollTween,
    findMessageElement,
    getElementAbsoluteTop,
  }
}
