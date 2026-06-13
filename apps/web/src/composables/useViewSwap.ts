import { ref } from 'vue'

// Motion for the in-place list <-> detail swap (a frontend page switch with no
// route change, so no browser-history entry). This is a directional push-pop,
// like iOS navigation:
//   • openDetail (forward): the list slides out to the LEFT while the detail
//     slides in from the RIGHT.
//   • backToList (back):    the detail slides out to the RIGHT while the list
//     slides back in from the LEFT.
// Both panes are on screen at once and cross-slide, so one visibly gives way as
// the other takes its place — no "out, then in" double-jump.
//
// The actual keyframes live in style.css (.swap-pane / [data-swap-dir]); the
// transition name we hand <Transition> is derived from `direction` below. There
// is deliberately no `appear`, so landing on the page from the settings sidebar
// is a plain cut (like Appearance / Profile) — only the swap itself moves.

export type SwapDirection = 'forward' | 'back'

/**
 * Drives a two-pane (list <-> detail) swap inside a single page. Render the list
 * when `view === 'list'` and the detail when `view === 'detail'`, wrapped in the
 * SwapTransition component. `direction` tells that component which way to slide.
 */
export function useViewSwap() {
  const view = ref<'list' | 'detail'>('list')
  const direction = ref<SwapDirection>('forward')

  function openDetail(onSwitched?: () => void) {
    direction.value = 'forward'
    onSwitched?.()
    view.value = 'detail'
  }

  function backToList(onSwitched?: () => void) {
    direction.value = 'back'
    onSwitched?.()
    view.value = 'list'
  }

  return { view, direction, openDetail, backToList }
}
