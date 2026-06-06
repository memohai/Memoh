import type { InjectionKey } from 'vue'
import { inject } from 'vue'

// Arkloop-style hover bridge (no convex "grace area"). The root owns the open
// state + a single close timer; trigger and content share these handlers so the
// ONLY hit targets are the real trigger element and the real card — there is no
// invisible polygon spanning the gap. Moving the pointer off both starts a short
// timer; re-entering either cancels it. This makes the card close at the same
// speed in every direction (reka's HoverCard hard-wires a grace-area hull that,
// for a card wider than its trigger, lingers far on whichever side it overhangs).
export interface HoverCardContext {
  /** Pointer entered the trigger: cancel any pending close, arm the open timer. */
  scheduleOpen: () => void
  /** Pointer left the trigger or content: arm the close timer. */
  scheduleClose: () => void
  /** Pointer entered the content: cancel the pending close (bridges the gap). */
  cancelClose: () => void
}

export const HOVER_CARD_INJECTION_KEY: InjectionKey<HoverCardContext>
  = Symbol('HoverCard')

export function injectHoverCardContext(): HoverCardContext {
  const ctx = inject(HOVER_CARD_INJECTION_KEY)
  if (!ctx)
    throw new Error('HoverCard parts must be used inside <HoverCard>')
  return ctx
}
