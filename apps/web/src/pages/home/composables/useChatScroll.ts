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
   * per turn and binds this ref to the last one). Used for MEASUREMENT only
   * — the reserve itself is declarative render state keyed by turn id
   * (turnReserveStyle, bound by chat-pane on every turn container).
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
 *   • user scrolls down INTO the 30px bottom band        → follow ON
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
 * ─── Pin (send + session entry) ───────────────────────────────────────────
 * On send (chat-pane → pinAfterSend) the newest prompt is pinned near the top
 * and the reply streams into reserved blank below. Mechanism: chat-pane
 * renders every turn in its own PERSISTENT container (keyed by the turn's
 * opening message — a send appends a container, it never re-parents previous
 * turns' DOM; re-parenting remounts the subtree and its transient height
 * collapse read as a scroll jump). On the mutation where the new prompt
 * renders, tryApplyPin measures ONCE and sets the DECLARATIVE reserve
 * (turnReserves → per-turn :style binding, see the reserve comment in the
 * body)
 * — sized so the prompt can sit at PIN_TOP_OFFSET_PX with reserved blank
 * below — then moves the viewport to the shared pin target
 * (prompt top - offset) with the shared entrance tween (the same animation as
 * jump-to-bottom; browser-native smooth was tried and rejected — see the
 * comment inside tryApplyPin). The first turn never pins (nothing above it).
 * From there CSS layout does everything: the reply consumes the reserve, a
 * tool group toggling open/closed just uses more or less of it, and content
 * outgrowing the reserve makes the min-height inert (natural layout resumes).
 * The reserve is NEVER recomputed on mutations and SURVIVES stream completion
 * and container remounts (it is render state, not a DOM annotation). Only the
 * next send (handover: old blank retires, released at the entrance settle) or
 * a session switch touches it. While pinned, follow is OFF; the view stays
 * parked until the user scrolls back to the bottom.
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
  // --- The pin reserve: DECLARATIVE render state, keyed by TURN IDENTITY --
  // This map is the source of truth; chat-pane projects it per turn via
  // turnReserveStyle(turn.id) as a :style min-height binding. Two design
  // points, both learned the hard way:
  //   • Declarative, not an imperative inline style: because the style is
  //     part of the render, a container remount re-renders WITH the reserve
  //     — a reserve-less layout cannot be produced. The imperative design
  //     needed three layers of compensation to approximate this.
  //   • Keyed by the turn's opening message id, NOT by position (last /
  //     second-to-last). A positional binding re-maps whatever values it
  //     holds onto DIFFERENT containers the instant the turn count changes
  //     — and the optimistic append lands several microtasks after
  //     pinAfterSend (sendMessage awaits command parsing / session setup
  //     before pushing), so a positional handover rendered one flush with
  //     the reserve on the WRONG turns: a viewport of blank injected above
  //     the view (visible content shoved away) while the real blank
  //     collapsed (scrollHeight shrink → clamp → hard upward yank). Id
  //     keying is inert to turn-count changes. It requires render ids to be
  //     stable across the optimistic → server consolidation, which the
  //     store now guarantees (adoptRenderIdentity in chat-list).
  //
  // Lifecycle (contract A3 + 3b): tryApplyPin sets the new turn's entry;
  // the previous turn's entry is deliberately left untouched at arm and
  // apply time — its blank must survive until the new entrance settles
  // (collapsing it earlier teleports a viewport parked in that blank) —
  // and is pruned at the settle. Session switches clear the map. Writes
  // replace the map so the :style projections re-evaluate.
  const turnReserves = ref<Map<string, number>>(new Map())
  function turnReserveStyle(turnId: string): { minHeight: string } | undefined {
    const px = turnReserves.value.get(turnId)
    return px === undefined ? undefined : { minHeight: `${px}px` }
  }
  // Turn id holding the CURRENT pin's reserve — the one entry a settle keeps.
  let pinnedTurnId: string | null = null

  // Drop every reserve except the given turn's (null prunes all). Runs at
  // the entrance settle / entry landing — never at arm time (contract 3b);
  // native scroll anchoring holds the view still while blank above the
  // pinned prompt collapses on the next flush.
  function pruneReservesTo(keepTurnId: string | null) {
    const cur = turnReserves.value
    const keepPx = keepTurnId === null ? undefined : cur.get(keepTurnId)
    if (cur.size === (keepPx === undefined ? 0 : 1)) return
    turnReserves.value = keepPx === undefined
      ? new Map()
      : new Map([[keepTurnId!, keepPx]])
  }

  let highlightTimer: ReturnType<typeof setTimeout> | null = null
  let cancelScrollTween: (() => void) | null = null
  let tweenFlagTimer: ReturnType<typeof setTimeout> | null = null
  let mutationObserver: MutationObserver | null = null

  function isNearBottom(el: HTMLElement): boolean {
    return contentEndGap(el) < NEAR_BOTTOM_THRESHOLD_PX
  }

  // Whether the viewport sits AT the content end — the ONLY state allowed to
  // (re-)arm follow. Deliberately distinct from isNearBottom: isNearBottom
  // answers "is there content below to see" for the jump button, and a
  // viewport parked inside the pin's reserve blank correctly counts as
  // at-bottom there (gap is negative — nothing further to see). But arming
  // follow from inside that blank would make every streamed chunk snap the
  // view UP to hug the content end — the pinned park would twitch chunk by
  // chunk toward the top. So arming additionally bounds the overshoot past
  // the content end: the span between the last turn container's bottom and
  // scrollHeight is ordinary column padding (being anywhere in it is still
  // the physical bottom), while the container's own overhang past its last
  // message is exactly the reserve's unconsumed blank — outside the budget.
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
  // BUSINESS SEMANTIC: "the bottom" everywhere in this file means the END OF
  // CONTENT — the last message's bottom edge — NOT scrollHeight. The pin
  // reserve leaves blank under the last message and that blank is not
  // content: the jump button must not appear when only blank is below (it
  // would scroll into nothing), follow must not drag the view into the
  // blank, and reaching the content end is what re-arms follow.
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
    // Deliberately NO reserve mutation here. The previous turn's blank must
    // survive until the new entrance settles (contract 3b), and its map
    // entry is keyed by that turn's id, so the upcoming append cannot re-map
    // it — and CANNOT run here: the optimistic push happens several awaits
    // into sendMessage, so anything mutated now renders one flush BEFORE the
    // new turn exists. tryApplyPin sets the new entry; the settle prunes.
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
  // the viewport to the shared pin target (prompt top - offset) — send mode
  // tweens there, entry mode jumps there. The reserve is min-height,
  // so all later geometry is absorbed by CSS layout with zero JS involvement;
  // see the header's Pin section for why it must be set exactly once and left
  // alone (including after the stream completes).
  //
  // Geometry: with reserve R on the container and `below` = everything under
  // the container (the column's bottom padding),
  //   scrollTop@bottom = containerTop + R + below - clientHeight
  // and we want that to equal promptTop - PIN_TOP_OFFSET_PX, giving
  //   R = clientHeight - below - PIN_TOP_OFFSET_PX + (promptTop - containerTop)
  // A prompt taller than the reserve can't land its top at the pin point; the
  // floor guarantees breathing room below its tail instead (its end plus the
  // incoming reply stay visible).
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
    // The first turn never pins (reference behavior: pin only when older
    // turns exist above) — with nothing before it the content already sits
    // at the top, and a viewport of reserved blank would only add dead
    // scroll range.
    if (messages.value[0]?.id === prompt.id) {
      pinPending = false
      return false
    }
    pinPending = false
    // A pinned turn is a parked view — follow re-engages only when the user
    // scrolls back into the bottom band. (Send already parked in pinAfterSend;
    // this is where ENTRY switches from its land-at-bottom fallback to parked.)
    followEnabled = false

    const containerTop = getElementAbsoluteTop(container, el)
    const promptOffsetInTurn = getElementAbsoluteTop(promptEl, el) - containerTop
    const below = el.scrollHeight - containerTop - container.offsetHeight
    const ideal = el.clientHeight - below - PIN_TOP_OFFSET_PX + promptOffsetInTurn
    const floor = promptOffsetInTurn + promptEl.offsetHeight + Math.round(el.clientHeight / 3)
    const reservePx = Math.max(0, Math.round(Math.max(ideal, floor)))
    pinnedTurnId = prompt.id
    turnReserves.value = new Map(turnReserves.value).set(prompt.id, reservePx)
    // Immediate projection of the same value: the reactive binding lands on
    // Vue's next flush, but the scroll below needs this frame's geometry
    // (the entry jump would otherwise clamp against a reserve-less
    // scrollHeight). The binding renders the identical value and owns it
    // from the next patch on.
    container.style.minHeight = `${reservePx}px`

    // THE one landing computation, shared verbatim by send and entry so the
    // two ways of arriving at a pinned turn cannot drift apart: the viewport
    // top goes to (prompt top - PIN_TOP_OFFSET_PX). Anchored on the PROMPT,
    // never on scrollHeight — a bottom-anchored target moves mid-flight when
    // the reply outgrows the reserve while streaming, landing deeper than the
    // offset (the sliver visibly eroded). Re-resolved on every read because
    // the optimistic → server id swap remounts the prompt row mid-flight.
    const pinTarget = () => {
      const cur = lastUserMessage()
      const curEl = cur ? findMessageElement(cur.id) : null
      return curEl ? getElementAbsoluteTop(curEl, el) - PIN_TOP_OFFSET_PX : el.scrollTop
    }

    if (pinMode === 'send') {
      // Entrance animation: the shared JS tween — the same one the
      // jump-to-bottom button and reply jumps use. Browser-native smooth
      // scroll was tried first (per spec) and rejected in testing: Chromium
      // animates scrollTo at a flat constant-speed profile (reads as
      // mechanical, no ease-out), and cancels the flight outright when the
      // scroller's content changes — which streaming guarantees, so the view
      // kept stopping partway. The tween is per-frame and re-reads its target,
      // so it can't be cancelled by mutations and lands exactly even while
      // the reply grows; a real wheel/touch still cancels it (user intent
      // wins, handled inside startScrollTween).
      pinScrollActive = true
      if (pinSettleTimer) clearTimeout(pinSettleTimer)
      pinSettleTimer = setTimeout(() => {
        pinSettleTimer = null
        pinScrollActive = false
        // The entrance has settled — retire the previous turn's blank now
        // (contract: never at arm/animation start). Declarative: its :style
        // binding drops the min-height on the next flush.
        pruneReservesTo(pinnedTurnId)
        const cur = scrollEl.value
        if (!cur) return
        isAtBottom.value = isNearBottom(cur)
        lastScrollTop = cur.scrollTop
      }, PIN_TWEEN_DURATION_MS + 100)
      startScrollTween(el, pinTarget, PIN_TWEEN_DURATION_MS)
    } else {
      // Entry: same target, no animation — a session opens already in place,
      // so there is no flight to protect: retire any other turn's blank
      // right away.
      pruneReservesTo(prompt.id)
      isProgrammaticScroll = true
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
    // The old session's reserves are meaningless in the new one; the entry
    // pin measures the new session fresh.
    pinnedTurnId = null
    turnReserves.value = new Map()
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
            // is at the content end, so a mid-history restore (or a park in
            // the reserve blank) is not yanked by the first streamed mutation.
            const root = scrollEl.value
            followEnabled = root ? atContentEnd(root) : true
            if (root) isAtBottom.value = followEnabled
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
      // Reaching the CONTENT END is the only scroll gesture that (re-)arms
      // follow — not merely "no content below" (see atContentEnd for why a
      // park inside the reserve blank must never arm). Deliberately NO
      // optimistic "relock shortly after a downward pause" here — see the
      // header's "Follow on / off" section.
      followEnabled = true
    }
  }

  // The follow heartbeat: streaming tokens mutate the DOM subtree. First give
  // an armed pin its chance to apply (it needs the target prompt in the DOM);
  // the mutation that applies a pin belongs to the pin alone — never also
  // follow on it, or the entry landing would be yanked straight to the bottom.
  function onContentMutated() {
    const el = scrollEl.value
    if (!el) return
    if (!isActive.value || lockScroll.value) return
    if (pinPending && tryApplyPin(el)) return
    if (followEnabled) stickToBottomNow()
    else if (!pinScrollActive) isAtBottom.value = isNearBottom(el)
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
    // reserve projection — chat-pane binds this per turn container
    // (see the declarative-reserve comment above)
    turnReserveStyle,

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
