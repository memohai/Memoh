import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

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
 *
 * Pass `queryKey` to mirror the detail state into the URL as `?<queryKey>=1`
 * (written with router.replace — an in-page state sync, not a navigation, same
 * contract as useSyncedQueryParam). This is required for the settings-sidebar
 * "re-click the tab you're already on" affordance to work: sidebar navigation
 * is a plain `router.push({ name })` with no query. If detail state lives ONLY
 * in a local ref, re-clicking the current tab resolves to the exact same
 * location the router is already at, so Vue Router treats it as a duplicate
 * push and silently drops it — the page never hears about it, so a detail view
 * left open has no way to snap back to its list. Mirroring the flag into the
 * query makes list-vs-detail part of the resolved location, so leaving detail
 * open changes the URL, and a bare re-push to the tab's un-queried route is a
 * genuinely different destination that the router actually navigates to. Omit
 * `queryKey` for swaps that don't need this (e.g. a swap nested inside another
 * already-synced tab) — `view` then stays purely local, as before.
 */
export function useViewSwap(queryKey?: string) {
  const route = queryKey ? useRoute() : null
  const router = queryKey ? useRouter() : null

  const queryDetail = computed(() => !!queryKey && route!.query[queryKey] === '1')
  const view = ref<'list' | 'detail'>(queryDetail.value ? 'detail' : 'list')
  const direction = ref<SwapDirection>('forward')

  if (queryKey && route && router) {
    // Reacts to the query flag changing for ANY reason — our own replace()
    // calls below, but also external navigations we don't control: a sidebar
    // re-click that lands on the un-queried route, or the user's own browser
    // back/forward. Whatever the cause, `view` must follow the URL, and losing
    // the flag always reads as "closing", so it always animates as 'back'.
    // The equality guard skips the case where openDetail/backToList already
    // applied this exact state locally before writing the query, so the two
    // never fight over `direction` for the same transition.
    watch(queryDetail, (isDetail) => {
      if (isDetail === (view.value === 'detail')) return
      direction.value = isDetail ? 'forward' : 'back'
      view.value = isDetail ? 'detail' : 'list'
    })
  }

  function openDetail(onSwitched?: () => void) {
    direction.value = 'forward'
    onSwitched?.()
    view.value = 'detail'
    if (queryKey && route && router) {
      void router.replace({ query: { ...route.query, [queryKey]: '1' } })
    }
  }

  function backToList(onSwitched?: () => void) {
    direction.value = 'back'
    onSwitched?.()
    view.value = 'list'
    if (queryKey && route && router) {
      const { [queryKey]: _drop, ...rest } = route.query
      void router.replace({ query: rest })
    }
  }

  return { view, direction, openDetail, backToList }
}
