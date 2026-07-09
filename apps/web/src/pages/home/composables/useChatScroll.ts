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

// The pin entrance runs LONGER than utility jumps: it is the "turn the page"
// moment of sending, and at 450ms the ease-out's deceleration tail is barely
// perceptible over a near-viewport-height distance. TUNE ME with the user.
const PIN_TWEEN_DURATION_MS = 700

// "At the bottom" is a threshold, not a pixel-perfect landing: sub-pixel
// rounding, the last line growing mid-stream, and fractional zoom all leave a
// few px of slack that must still count as "following". 30px stays under one
// line of body text, so a deliberate scroll-up still unlocks. Measured against
// the CONTENT END (last message's bottom edge), never scrollHeight — see the
// content-end geometry section for the business semantic.
const NEAR_BOTTOM_THRESHOLD_PX = 30

// When a turn is pinned, the user prompt lands this far below the viewport
// top. Sized to leave a visible sliver of the previous turn above the prompt —
// context that the page "turned", not teleported: the top 40px sit under the
// fade overlay (h-10), so roughly the remainder is readable tail. TUNE ME with
// the user against the real layout. Measured, so it is width-agnostic — no
// narrow-screen special case needed.
const PIN_TOP_OFFSET_PX = 140

export interface UseChatScrollOptions {
  scrollEl: Ref<HTMLElement | null>
  /** Content-height probe — the scroll viewport's first child. */
  contentEl: Ref<HTMLElement | null>
  /**
   * Container of the LAST turn (chat-pane renders one persistent container
   * per turn and binds this ref to the last one). The pin reserves viewport
   * space by setting an inline min-height on it — imperatively, exactly once
   * per pin (see tryApplyPin), never via a reactive binding, so sizing and
   * the scroll that consumes it stay in one synchronous pass.
   *
   * When a newer turn is pinned, the PREVIOUS last-turn container's residual
   * min-height is cleared first (collapseReserveKeepingView) so the entrance
   * never flies through empty spacing. lastTurnEl always points at the
   * newest turn; appliedPinContainer tracks who currently holds the reserve.
   */
  lastTurnEl: Ref<HTMLElement | null>
  messages: Ref<ChatMessage[]>
  isActive: Ref<boolean>
  sessionId: Ref<string | null | undefined>
}

/**
 * useChatScroll — every scroll behavior for the chat message list.
 *
 * ─── What it does ─────────────────────────────────────────────────────────
 *   • pin a just-sent prompt near the viewport top and let the reply stream
 *     into reserved blank below (send = "turn the page")
 *   • stick-to-bottom follow while a reply streams in — but only after the
 *     user opts in by scrolling to the bottom; pin and follow never overlap
 *   • let the user scroll away ("escape") and STAY there
 *   • jump-to-message + transient highlight (reply refs, the scroll rail)
 *   • keep the viewport still while older history is prepended at the top
 *   • restore scroll position across KeepAlive tab switches; land a freshly
 *     opened session pinned at its last prompt (same look as just-after-send),
 *     falling back to the bottom when the session has no user turn
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
 *   followEnabled         content growth may pull the viewport to the bottom;
 *                         false = parked (user escaped, or a turn is pinned)
 *   pinPending/pinAnchorId one-shot pin, applied when the target prompt renders
 *   pinMode               'send' = animated entrance for a just-sent turn;
 *                         'entry' = instant landing when a session opens
 *   pinScrollActive       pin's entrance tween in flight → freeze isAtBottom
 *   lastScrollTop         previous frame's scrollTop, for the up/down test
 *   lockScroll (ref)      init / cross-tab restore running → freeze BOTH follow
 *                         and escape so their setup scrolls latch nothing
 *
 * ─── Event flow ───────────────────────────────────────────────────────────
 *   MutationObserver(content subtree) ─ streaming mutates the DOM ─▶ apply an
 *       armed pin (once, when the fresh prompt is in the DOM), then follow to
 *       the bottom IF followEnabled. Growth is sensed via DOM mutation,
 *       deliberately NOT a height ResizeObserver + "scrollTop = height" snap —
 *       that snap was half of the original bug.
 *   wheel  ─▶ physical intent; deltaY<0 (up) parks the view immediately.
 *   scroll ─▶ refresh isAtBottom; if it is not our own scroll, run the latch.
 *
 * ─── Follow on / off ──────────────────────────────────────────────────────
 * Pin and follow are mutually exclusive phases, switched only by explicit
 * actions:
 *   • user scrolls down INTO the content-end band        → follow ON
 *     (atContentEnd — parked in pin reserve blank does NOT arm)
 *   • jump-to-bottom button / session switch             → follow ON
 *   • any upward wheel / scroll                          → follow OFF
 *   • jump-to-message / rail / prepend (markEscaped)     → follow OFF
 *   • send (pinAfterSend)                                → follow OFF (parked)
 * There is deliberately NO "relock shortly after a downward pause" timer: it
 * would re-arm follow on any small downward nudge while a turn is parked, and
 * the next streamed token would yank the parked view to the bottom.
 *
 * ─── Prepend (older history) ──────────────────────────────────────────────
 * Loading older messages is treated as an escape (suppressAutoScrollForPrepend
 * → markEscaped): follow stays off, and the browser's native `overflow-anchor`
 * keeps the visible content stationary across the insert. There is NO manual
 * scrollTop compensation — and you must NOT set `overflow-anchor: none` on the
 * viewport. That was tried (to protect a browser-native smooth entrance
 * scroll, which anchoring can cancel) and it broke two things at once: each
 * prepend batch twitched, and the entry pin drifted off its offset — a
 * one-shot manual compensation / one-shot landing cannot track the ASYNC
 * layout settles (code highlighting, images, fonts) that keep resizing rows
 * after the DOM lands. Native anchoring corrects continuously; the pin's JS
 * tween is immune to it (it rewrites scrollTop every frame), so they coexist.
 *
 * ─── Browser vs hand-written scroll (read before changing pin/reserve) ───
 * We do NOT replace the browser's scroller. Most of the time the browser
 * owns scrollTop:
 *   • everyday wheel/touch scrolling
 *   • native overflow-anchor during history prepend and async reflow
 *     (Shiki / KaTeX / images / fonts) — see Prepend above
 *   • clamp when content shrinks past the max scroll
 *
 * What the browser does NOT know is chat product geometry:
 *   • residual pin blank is deliberate "reply room", not ordinary content
 *   • send = pin the new prompt at PIN_TOP_OFFSET_PX with previous-turn peek
 *   • that blank SURVIVES stream completion (must not collapse on finish)
 *   • entrance needs a quintic ease-out tween, not native smooth
 *
 * So we hand-write scrollTop only on a NARROW boundary: the frame that
 * hands a pin from one turn to the next, plus the entrance tween itself.
 * That is intentional policy, not a temporary hack — but the surface must
 * stay narrow. If a new height-change path (Thought expand, tool group,
 * split pane, tab restore, …) seems to need the same compensation, do NOT
 * call collapseReserveKeepingView from there and do NOT add a second
 * settle-time / delayed "restore" layer. Either the product geometry
 * changed (rethink reserve structure) or the bug is elsewhere (follow
 * arming, remount, id identity). Three rewrite rounds died on "mechanism
 * got clever"; the surviving rule is: one named handover, one formula,
 * everything else stays dumb and browser-owned.
 *
 * ─── Pin (send + session entry) ───────────────────────────────────────────
 * On send (chat-pane → pinAfterSend) the newest prompt is pinned near the top
 * and the reply streams into reserved blank below. Mechanism: chat-pane
 * renders every turn in its own PERSISTENT container (keyed by the turn's
 * opening message — a send appends a container, it never re-parents previous
 * turns' DOM; re-parenting remounts the subtree and its transient height
 * collapse read as a scroll jump).
 *
 * tryApplyPin order is load-bearing (single coordinate system):
 *   1. Retire the PREVIOUS turn's residual blank via
 *      collapseReserveKeepingView (position-aware — see that function).
 *   2. Measure and set the NEW turn's min-height once.
 *   3. Tween (send) or jump (entry) to prompt top − PIN_TOP_OFFSET_PX.
 *
 * Two rejected alternatives (do not resurrect without a new product reason):
 *   A. Keep old blank for the whole flight, clear at settle.
 *      Document is [prev content | EMPTY SPACING | new prompt | new blank].
 *      The tween pins the new prompt by scrolling through that empty band,
 *      so the previous turn leaves the viewport and looks unmounted; settle
 *      then removes the spacing and the previous turn "reappears". User-
 *      confirmed wrong.
 *   B. Clear old blank before the tween with flat `scrollTop -= fullDelta`.
 *      Keeps content BELOW the blank fixed — but when the user is parked on
 *      the previous pin (reading content ABOVE the blank) or reading history
 *      far above, that yanks the whole view before the entrance starts.
 *      User-confirmed wrong.
 *
 * Position-aware collapse (A/B midpoint) only compensates the removed band
 * that sat above the viewport top. Parked on previous-turn content, the
 * blank is below the viewport top → scrollTop stays put → blank vanishes →
 * the down animation then carries REAL previous-turn content into the peek.
 * From there CSS layout does everything: the reply consumes the new reserve,
 * tool/Thought toggles use more or less of it, content past min-height makes
 * it inert. The new reserve is NEVER recomputed on mutations and SURVIVES
 * stream completion. Only the next send or a session switch touches the
 * reserve. While pinned, follow is OFF until the user scrolls back to the
 * content end (atContentEnd — not merely "no content below").
 *
 * Opening a session uses the SAME paradigm ('entry' mode): the last prompt
 * lands at the pin offset with the previous turn peeking above — identical
 * geometry to just-after-send — but instantly, with no animation. Sessions
 * with no user turn (system/subagent, empty chats) fall back to the bottom:
 * follow stays engaged while the entry pin is pending, so if no prompt ever
 * renders the ordinary follow heartbeat lands the view.
 *
 * ─── chat-pane contract ───────────────────────────────────────────────────
 * chat-pane owns the DOM refs and drives this composable through:
 *   scrollToBottom (jump button) · scrollToMessage (reply refs) · pinAfterSend
 *   (on send) · suppressAutoScrollForPrepend (top sentinel) · markEscaped +
 *   startScrollTween + findMessageElement + getElementAbsoluteTop (scroll rail)
 *   · onMessageActive (per message-item) · onActivatedRestoreScroll /
 *   onDeactivatedResetScroll (its own KeepAlive hooks).
 */
export function useChatScroll(options: UseChatScrollOptions) {
  const { scrollEl, lastTurnEl, messages, isActive, sessionId } = options

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

  // --- Follow / pin latches (non-reactive on purpose) ---
  // True around a scroll the code itself performs, so the resulting scroll
  // event is not misread as user intent.
  let isProgrammaticScroll = false
  let lastScrollTop = 0
  // THE mode switch: while true, content growth pulls the viewport to the
  // bottom; while false the view is parked (user scrolled up, or a just-sent
  // turn is pinned). Flipped only by explicit actions — see "Follow on / off"
  // in the header.
  let followEnabled = true
  // One-shot pin: armed by pinAfterSend / a session switch, applied by the
  // MutationObserver on the first mutation where the target prompt is actually
  // in the DOM.
  let pinPending = false
  // 'send' animates the entrance (shared JS tween); 'entry' lands a
  // freshly opened session at the same geometry instantly.
  let pinMode: 'send' | 'entry' = 'send'
  // Send mode only: last user message id at arm time — the pin waits for a
  // NEWER one, so a stray mutation (a late token of the previous reply) can't
  // size the pin against the previous turn's prompt. Entry mode pins whatever
  // last prompt renders, so it keeps this null.
  let pinAnchorId: string | null = null
  // True while the pin's entrance tween is in flight; freezes the isAtBottom
  // mirror so the jump button doesn't flash mid-animation. Cleared by the
  // settle timer (the tween has no completion callback).
  let pinScrollActive = false
  let pinSettleTimer: ReturnType<typeof setTimeout> | null = null
  // The container currently holding a pin reserve. Containers are per-turn and
  // persistent. On the next pin, tryApplyPin clears THIS container's residual
  // blank BEFORE the entrance tween (collapseReserveKeepingView) so the
  // flight never scrolls through empty spacing — see file-header Pin section.
  let appliedPinContainer: HTMLElement | null = null
  // The pinned reserve in px, held as STATE — not only as the container's
  // inline style. BUSINESS INVARIANT: the reserve must NOT drop when the
  // stream finishes. Completion swaps message ids (temp → server), which
  // re-keys and REMOUNTS the turn container, and an inline style dies with
  // its element — restorePinReserve re-asserts this value onto the fresh
  // container so the invariant holds regardless of DOM churn.
  let pinnedReservePx: number | null = null

  // ── collapseReserveKeepingView ─────────────────────────────────────────
  // THE only place that hand-adjusts scrollTop in response to a pin-reserve
  // height drop. Call site is intentionally singular: tryApplyPin, when the
  // previous pin container is not the container about to receive the new
  // reserve. Do not reuse for Thought/tool expands, stream completion,
  // remounts, or prepend — those stay browser-owned (overflow-anchor) or
  // follow/escape owned.
  //
  // Why we write scrollTop at all (vs "let the browser handle shrink"):
  //   Residual pin blank is product geometry, not content. Clearing
  //   min-height is a deliberate layout edit on send/entry handover. The
  //   browser's clamp / scroll anchoring do not know we want "parked on
  //   previous-turn content → keep that content still; blank below may
  //   vanish". Left alone, clamp/anchoring and a later tween fight; the
  //   user either sees a pre-tween yank or flies through empty spacing.
  //   This function is the narrow policy that makes the handover match
  //   that product intent. It is NOT a general scroll-anchoring reinstall.
  //
  // Geometry (residual blank always at the BOTTOM of its turn container,
  // above the next turn's prompt):
  //
  //   before clear:  container height = max(content, minHeight)
  //   after clear:   container height = content
  //   delta         = before − after  (≥ 0; the blank we remove)
  //   collapseTop   = absolute Y where the blank started
  //                 = containerTop + contentHeight
  //                 = containerTop + (before − delta)
  //   removed band  = [collapseTop, collapseTop + delta)
  //
  // Position-aware scrollTop (only when delta > 0):
  //
  //   • scrollTop ≤ collapseTop
  //       Viewport top is above the blank (reading previous-turn content,
  //       or history further up). Leave scrollTop alone. The blank below
  //       the user vanishes; what they were looking at stays put. This is
  //       the common "send while still parked on the previous pin" path
  //       and "scroll up into history then send".
  //
  //   • scrollTop > collapseTop
  //       Viewport top sits inside or past the blank. Subtract only the
  //       removed length that was above the viewport top:
  //         scrollTop -= min(delta, scrollTop − collapseTop)
  //       so content that lived BELOW the blank does not jump.
  //
  // Rejected formulas (do not bring back):
  //   • scrollTop -= delta always
  //       Anchors content below the blank. Yanks history readers toward
  //       the top; shifts a parked pin's visible content before the
  //       entrance tween — "spacing snaps off, whole view jumps, then
  //       the down animation starts".
  //   • defer clear until tween settle (+ prompt-as-anchor restore)
  //       Flight runs with [prev | EMPTY | new prompt]. Tween scrolls
  //       through the empty band; previous turn looks unmounted until
  //       settle removes spacing and it reappears.
  //   • prompt-as-anchor compensation on the PRE-tween clear
  //       Keeps the NEW prompt fixed; when the user is still looking at
  //       previous-turn content, THAT is what jumps. Wrong anchor for
  //       the send-from-parked-pin case.
  //
  // After this returns, tryApplyPin measures the new reserve and starts
  // the tween in a single-reserve coordinate system — previous-turn
  // content is what peeks above the new prompt, not empty spacing.
  function collapseReserveKeepingView(el: HTMLElement, container: HTMLElement) {
    if (!container.isConnected) {
      container.style.minHeight = ''
      return
    }
    const rootRect = el.getBoundingClientRect()
    const before = container.offsetHeight
    // Absolute top of the container in scroll content coordinates (same
    // basis as scrollTop), measured BEFORE clearing min-height so
    // collapseTop refers to the pre-clear layout.
    const containerTop = el.scrollTop + container.getBoundingClientRect().top - rootRect.top

    container.style.minHeight = ''
    const delta = before - container.offsetHeight
    if (delta <= 0) return

    // Removed band was [containerTop + after, containerTop + before] with
    // after = before - delta (= content height once min-height is gone).
    const collapseTop = containerTop + (before - delta)
    if (el.scrollTop > collapseTop) {
      el.scrollTop = Math.max(0, el.scrollTop - Math.min(delta, el.scrollTop - collapseTop))
    }
  }

  let highlightTimer: ReturnType<typeof setTimeout> | null = null
  let cancelScrollTween: (() => void) | null = null
  let tweenFlagTimer: ReturnType<typeof setTimeout> | null = null
  let mutationObserver: MutationObserver | null = null

  // "Is there anything further to see below?" — used for the jump-to-bottom
  // button. Parked inside the pin's unconsumed reserve blank counts as yes-
  // at-bottom (gap ≤ 0): the blank is not content, so the button must not
  // offer to scroll into empty space.
  function isNearBottom(el: HTMLElement): boolean {
    return contentEndGap(el) < NEAR_BOTTOM_THRESHOLD_PX
  }

  // Whether the viewport sits at the CONTENT END band — the ONLY state
  // allowed to (re-)arm follow. Distinct from isNearBottom on purpose:
  // parking deep in the reserve blank is "nothing more to see" for the jump
  // button, but arming follow from there makes every streamed chunk (or a
  // Thought/tool expand that mutates the tree) yank the parked view up to
  // hug the content end. Arming additionally bounds overshoot past the
  // content end: the span between the last turn container's bottom and
  // scrollHeight is ordinary column padding (still the physical bottom);
  // the container's own overhang past its last message is the reserve's
  // unconsumed blank and is outside the budget. Measured geometrically —
  // do not use getComputedStyle on the reka viewport's first child (its
  // padding is 0; the real pad is layout after the last turn).
  function atContentEnd(el: HTMLElement): boolean {
    const gap = contentEndGap(el)
    if (gap >= NEAR_BOTTOM_THRESHOLD_PX) return false
    const container = lastTurnEl.value
    const pad = container
      ? Math.max(0, el.scrollHeight - getElementAbsoluteTop(container, el) - container.offsetHeight)
      : 0
    return -gap <= pad + NEAR_BOTTOM_THRESHOLD_PX
  }

  // --- Content-end geometry --------------------------------------------
  // BUSINESS SEMANTIC: "the bottom" for content-aware decisions means the
  // END OF CONTENT — the last message's bottom edge — NOT scrollHeight. The
  // pin reserve leaves blank under the last message and that blank is not
  // content. Jump-button visibility uses isNearBottom (blank counts as
  // bottom); follow-arming uses atContentEnd (blank does NOT arm).
  // Cached per last-message id: this runs on every scroll frame and a
  // querySelector per frame would be wasteful.
  let lastMessageElCache: { id: string, el: HTMLElement } | null = null
  function lastMessageElement(): HTMLElement | null {
    const last = messages.value[messages.value.length - 1]
    if (!last) return null
    if (lastMessageElCache?.id === last.id && lastMessageElCache.el.isConnected) {
      return lastMessageElCache.el
    }
    const el = findMessageElement(last.id)
    lastMessageElCache = el ? { id: last.id, el } : null
    return el
  }

  // Px from the viewport's bottom edge down to the content end; <= 0 means
  // the content end is visible (being parked inside the reserve blank counts
  // as "at the bottom" — there is nothing further to see). Falls back to the
  // scrollHeight distance when no message is rendered (empty sessions).
  function contentEndGap(el: HTMLElement): number {
    const lastEl = lastMessageElement()
    if (!lastEl) return el.scrollHeight - el.scrollTop - el.clientHeight
    return getElementAbsoluteTop(lastEl, el) + lastEl.offsetHeight - el.scrollTop - el.clientHeight
  }

  // Scroll target for "go to the latest": content end at the viewport's
  // bottom edge — never scrollHeight, which would dive into the reserve blank.
  function contentEndTarget(el: HTMLElement): number {
    const lastEl = lastMessageElement()
    if (!lastEl) return el.scrollHeight
    return getElementAbsoluteTop(lastEl, el) + lastEl.offsetHeight - el.clientHeight
  }

  // Stop following, immediately. Called for any deliberate move away from the
  // bottom (jump-to-message, rail navigation, prepend of older history).
  function markEscaped() {
    followEnabled = false
  }

  // Re-arm following. The MutationObserver picks it back up on the next
  // content growth; used by the jump-to-bottom button and session switches.
  function followBottom() {
    followEnabled = true
  }

  // Called when the user sends. Pin and follow are MUTUALLY EXCLUSIVE phases:
  // sending parks the view and arms a one-shot pin; the MutationObserver
  // applies it when the new prompt renders (tryApplyPin). Streaming then grows
  // below the fold without moving the view — follow re-engages only when the
  // user scrolls back down to the bottom.
  function pinAfterSend() {
    followEnabled = false
    pinPending = true
    pinMode = 'send'
    pinAnchorId = lastUserMessage()?.id ?? null
    // Arm only — do NOT clear / set reserves here.
    //
    // sendMessage pushes the optimistic user turn only after several awaits
    // (command parse, session setup, …). Anything we mutate now paints one
    // Vue flush BEFORE that turn exists: a positional or "clear previous
    // blank now" edit either hits the wrong container or shrinks scrollHeight
    // under a bottom-parked viewport (zero-frame jerk). The full handover
    // (collapseReserveKeepingView → new min-height → tween) runs in
    // tryApplyPin on the first mutation where the NEW prompt is in the DOM.
  }

  function startScrollTween(
    root: HTMLElement,
    getTarget: () => number,
    duration: number = TWEEN_DURATION_MS,
  ) {
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
    }, { duration })
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
    }, duration + 100)
  }

  function getElementAbsoluteTop(target: HTMLElement, root: HTMLElement) {
    return root.scrollTop + target.getBoundingClientRect().top - root.getBoundingClientRect().top
  }

  // The most recent user turn — the message a pin anchors to the top. Scans
  // back through the flat message list (user/assistant/system interleaved) for
  // the last `user` entry.
  function lastUserMessage(): ChatMessage | null {
    const list = messages.value
    for (let i = list.length - 1; i >= 0; i--) {
      if (list[i]?.role === 'user') return list[i]!
    }
    return null
  }

  // Apply an armed pin: size the last-turn container's reserve ONCE, then move
  // the viewport to the shared pin target (prompt top − offset) — send mode
  // tweens there, entry mode jumps there. The reserve is min-height, so all
  // later geometry is absorbed by CSS layout with zero JS involvement; see
  // the header's Pin section for why it must be set exactly once and left
  // alone (including after the stream completes).
  //
  // Pipeline (order is the product; do not reorder "for simplicity"):
  //   arm (pinAfterSend / session watch) → MO sees new prompt → tryApplyPin
  //     → collapse previous residual blank (if any)
  //     → measure + set new min-height
  //     → pinTarget tween/jump
  //   settle timer only clears pinScrollActive / refreshes isAtBottom —
  //   it must NOT touch reserves (handover already finished before flight).
  //
  // Geometry for the NEW reserve R (measured only after old blank is gone):
  //   with `below` = everything under the new container (column bottom pad),
  //     scrollTop@bottom = containerTop + R + below − clientHeight
  //   want that equal to promptTop − PIN_TOP_OFFSET_PX:
  //     R = clientHeight − below − PIN_TOP_OFFSET_PX + (promptTop − containerTop)
  //   Floor: prompt taller than the ideal reserve can't land its top at the
  //   pin point — keep breathing room below its tail instead.
  function tryApplyPin(el: HTMLElement): boolean {
    const container = lastTurnEl.value
    if (!container) return false
    const prompt = lastUserMessage()
    if (!prompt) return false
    // Send mode: wait until a user message NEWER than the one present at arm
    // time is actually rendered — an unrelated mutation must not size the pin
    // against the previous turn's prompt. Entry mode has no such race (it pins
    // whatever last prompt the freshly opened session renders).
    if (pinMode === 'send' && prompt.id === pinAnchorId) return false
    const promptEl = findMessageElement(prompt.id)
    if (!promptEl) return false
    // The prompt must live INSIDE the last-turn container — mid-patch the ref
    // and the rendered rows can briefly disagree; sizing against a mismatched
    // pair would reserve garbage.
    if (!container.contains(promptEl)) return false
    pinPending = false
    // A pinned turn is a parked view — follow re-engages only when the user
    // scrolls back into the content-end band. (Send already parked in
    // pinAfterSend; this is where ENTRY switches from its land-at-bottom
    // fallback to parked.)
    followEnabled = false

    // ── Reserve HANDOVER (single coordinate system) ────────────────────
    // Step 1: retire previous residual blank with position-aware
    // compensation (collapseReserveKeepingView). MUST run before measuring
    // the new reserve and before the entrance tween:
    //   • If the old blank stays during flight, pinTarget is measured with
    //     empty spacing above the new prompt — the tween scrolls through
    //     that spacing; previous turn looks unmounted until something later
    //     clears the blank.
    //   • If we clear with flat scrollTop -= delta, parked/history views
    //     jump before the tween starts.
    // Bracket as programmatic so any scrollTop tweak cannot latch as a
    // user escape before startScrollTween takes over the flag.
    isProgrammaticScroll = true
    if (appliedPinContainer && appliedPinContainer !== container) {
      collapseReserveKeepingView(el, appliedPinContainer)
    }
    appliedPinContainer = container

    // Step 2: measure + set NEW reserve once. `below` / prompt offsets are
    // only honest after step 1 — dual-reserve layout would inflate
    // containerTop and lie about how much blank the new turn still needs.
    const containerTop = getElementAbsoluteTop(container, el)
    const promptOffsetInTurn = getElementAbsoluteTop(promptEl, el) - containerTop
    const below = el.scrollHeight - containerTop - container.offsetHeight
    const ideal = el.clientHeight - below - PIN_TOP_OFFSET_PX + promptOffsetInTurn
    const floor = promptOffsetInTurn + promptEl.offsetHeight + Math.round(el.clientHeight / 3)
    pinnedReservePx = Math.max(0, Math.round(Math.max(ideal, floor)))
    container.style.minHeight = `${pinnedReservePx}px`
    lastScrollTop = el.scrollTop

    // Step 3: landing target — THE one formula for send and entry so the
    // two arrival paths cannot drift. Viewport top → prompt top −
    // PIN_TOP_OFFSET_PX. Anchored on the PROMPT, never on scrollHeight (a
    // bottom-anchored target moves mid-flight when the reply outgrows the
    // reserve). Re-resolved every tween frame because optimistic → server
    // id swap can remount the prompt row mid-flight. Measured only after
    // step 1 so distance and peek are real previous-turn content.
    const pinTarget = () => {
      const cur = lastUserMessage()
      const curEl = cur ? findMessageElement(cur.id) : null
      return curEl ? getElementAbsoluteTop(curEl, el) - PIN_TOP_OFFSET_PX : el.scrollTop
    }

    if (pinMode === 'send') {
      // Entrance: shared JS tween (same as jump-to-bottom / reply jumps).
      // Native smooth was rejected: Chromium's profile reads as constant-
      // speed, and content mutations cancel the flight mid-stream. The
      // tween re-reads pinTarget each frame; wheel/touch cancels it inside
      // startScrollTween. Settle timer only ends pinScrollActive — no
      // reserve work (that finished in steps 1–2).
      pinScrollActive = true
      if (pinSettleTimer) clearTimeout(pinSettleTimer)
      pinSettleTimer = setTimeout(() => {
        pinSettleTimer = null
        pinScrollActive = false
        const cur = scrollEl.value
        if (!cur) return
        isAtBottom.value = isNearBottom(cur)
        lastScrollTop = cur.scrollTop
      }, PIN_TWEEN_DURATION_MS + 100)
      startScrollTween(el, pinTarget, PIN_TWEEN_DURATION_MS)
    } else {
      // Entry: same pinTarget, no animation — session opens already in place.
      // Old blank was already cleared in step 1 (same helper as send).
      el.scrollTo({ top: pinTarget(), behavior: 'auto' })
      requestAnimationFrame(() => {
        isProgrammaticScroll = false
        const cur = scrollEl.value
        if (!cur) return
        isAtBottom.value = isNearBottom(cur)
        lastScrollTop = cur.scrollTop
      })
    }
    return true
  }

  // Instant follow used by the MutationObserver while follow is engaged. Marks
  // itself programmatic so the scroll it triggers is not read as user intent.
  //
  // Timing: `scrollTo` dispatches its `scroll` event before the next rAF fires,
  // so the scroll handler runs while `isProgrammaticScroll` is still true and
  // correctly ignores it. The rAF then clears the flag and refreshes the
  // at-bottom mirror. Do not "simplify" by clearing the flag synchronously —
  // the scroll event would then be read as a user gesture.
  function stickToBottomNow() {
    const el = scrollEl.value
    if (!el) return
    isProgrammaticScroll = true
    el.scrollTo({ top: contentEndTarget(el), behavior: 'auto' })
    requestAnimationFrame(() => {
      isProgrammaticScroll = false
      const cur = scrollEl.value
      if (!cur) return
      isAtBottom.value = isNearBottom(cur)
      lastScrollTop = cur.scrollTop
    })
  }

  // Deliberate "go to the latest" — the jump-to-bottom button. Re-arms follow
  // and eases down; the MutationObserver keeps it pinned once there.
  function scrollToBottom() {
    const root = scrollEl.value
    if (!root) return
    followBottom()
    startScrollTween(root, () => contentEndTarget(root))
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
  // session with the ENTRY pin: last prompt at the pin offset, previous turn
  // peeking above — the same look as just-after-send, applied instantly by the
  // MutationObserver once the new session's messages render. Follow stays on
  // while the pin is pending so a session with no user turn (system/subagent,
  // empty chat) still lands at the bottom via the ordinary follow heartbeat.
  watch(sessionId, () => {
    elId.clear()
    followBottom()
    pinPending = true
    pinMode = 'entry'
    pinAnchorId = null
    // The old session's turn containers unmount with its messages (inline
    // reserve dies with the element); drop the pin pointers and the held
    // reserve so the entry pin measures the new session fresh.
    appliedPinContainer = null
    pinnedReservePx = null
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
            // is already at the content end (not merely "no content below" —
            // a restore parked in pin blank must stay parked).
            const root = scrollEl.value
            followEnabled = root ? atContentEnd(root) : true
            if (root) isAtBottom.value = isNearBottom(root)
          })
        } else {
          if (!newValue) {
            setTimeout(() => {
              lockScroll.value = false
              done = true
              unwatch()
              // No remembered anchor (fresh open / previously at bottom): land
              // with the entry pin — last prompt at the offset, instantly. If
              // there is nothing to pin yet (no user turn, or rows not
              // rendered), fall back to the bottom and leave the pin armed so
              // a late render can still apply it.
              followBottom()
              pinMode = 'entry'
              pinAnchorId = null
              const root = scrollEl.value
              if (!root || !tryApplyPin(root)) {
                pinPending = true
                stickToBottomNow()
              }
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
    // The pin reserve (last-turn min-height) intentionally SURVIVES tab
    // switches: KeepAlive preserves the DOM, and clearing it here would make
    // the conversation land in a different spot than the user left it.
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
    // Frozen while the pin's entrance tween animates, or the jump button would
    // flash for its whole flight; the settle timer refreshes it on landing.
    if (!pinScrollActive) isAtBottom.value = isNearBottom(el)
    // A scroll we triggered ourselves only updates the at-bottom mirror; it
    // must never move the follow latch.
    if (isProgrammaticScroll) return
    handleUserScroll(isScrollingUp)
  }

  function handleUserScroll(isScrollingUp: boolean) {
    const el = scrollEl.value
    if (!el) return
    if (lockScroll.value) return
    if (isScrollingUp) {
      // Any upward move parks the view immediately (and a pinned turn stays
      // parked).
      followEnabled = false
    } else if (atContentEnd(el)) {
      // Reaching the content end is the only scroll gesture that (re-)arms
      // follow — not merely "no content below" (see atContentEnd: a viewport
      // parked in the pin reserve must not arm). Deliberately NO optimistic
      // "relock shortly after a downward pause" here — see the header's
      // "Follow on / off" section.
      followEnabled = true
    }
  }

  // Re-assert the pinned reserve after the turn container remounts (stream
  // completion re-keys it — see pinnedReservePx). Skipped while a pin is
  // pending: between arm and apply, lastTurnEl already points at the NEXT
  // turn's container, which must measure fresh, not inherit the old reserve.
  // Touches only inline style (no layout reads), so it is safe to run BEFORE
  // any layout-forcing work — that ordering is what prevents a transient
  // reserve-less layout (and the scrollTop clamp it would trigger) from ever
  // materializing on screen.
  // Re-assert the CURRENT pin's min-height after a turn-container remount
  // (stream completion re-keys → v-for remount drops inline style). This is
  // NOT a second handover and must not run collapseReserveKeepingView:
  // we only stamp the already-chosen pinnedReservePx onto the live last-turn
  // node so residual blank survives completion. Skipped while pinPending —
  // between arm and apply, lastTurnEl already points at the NEXT turn, which
  // must measure fresh in tryApplyPin, not inherit the old px.
  function restorePinReserve() {
    if (pinnedReservePx === null || pinPending) return
    const container = lastTurnEl.value
    if (!container || container === appliedPinContainer) return
    if (appliedPinContainer?.isConnected) appliedPinContainer.style.minHeight = ''
    container.style.minHeight = `${pinnedReservePx}px`
    appliedPinContainer = container
  }

  // Follow / pin heartbeat. Streaming (and any other subtree mutation) lands
  // here. Order matters:
  //   1. restorePinReserve — re-stamp current reserve if the container remounted
  //      (style-only; no scrollTop policy).
  //   2. tryApplyPin if armed — owns the one real handover + entrance; return
  //      so this mutation never ALSO follow-snaps (would cancel the pin).
  //   3. else if followEnabled → stick to content end.
  //   4. else refresh jump-button mirror only.
  // Height changes that are NOT a pin handover (Thought expand, tool body,
  // markdown reflow) intentionally do nothing special here when follow is
  // off — the browser keeps the parked view; do not invent collapse/scroll
  // compensation on those paths.
  function onContentMutated() {
    restorePinReserve()
    const el = scrollEl.value
    if (!el) return
    if (!isActive.value || lockScroll.value) return
    if (pinPending && tryApplyPin(el)) return
    if (followEnabled) stickToBottomNow()
    else if (!pinScrollActive) isAtBottom.value = isNearBottom(el)
  }

  // Second belt for the same invariant: Vue's watcher flush and the
  // MutationObserver microtask race — whichever runs first after the remount
  // must re-assert before anything forces layout.
  watch(lastTurnEl, () => {
    restorePinReserve()
  }, { flush: 'post' })

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
  // stationary across the insert — continuously, including through the async
  // layout settles that follow it. No manual scrollTop compensation (see the
  // header's Prepend section for the failed attempt).
  function suppressAutoScrollForPrepend() {
    markEscaped()
  }

  onBeforeUnmount(() => {
    if (highlightTimer) clearTimeout(highlightTimer)
    if (tweenFlagTimer) clearTimeout(tweenFlagTimer)
    if (pinSettleTimer) clearTimeout(pinSettleTimer)
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
    pinAfterSend,

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
