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
 * Pass `queryKey` to mirror the detail state into the URL as `?<queryKey>=<value>`
 * (written with router.replace — an in-page state sync, not a navigation, same
 * contract as useSyncedQueryParam). The value is whatever the page hands
 * `openDetail` — pages pass the open resource's stable ID so a refresh can
 * restore the exact item (the page resolves `queryValue` back to an object
 * itself; this composable stays ignorant of what the value means). Mirroring
 * into the query is also required for the settings-sidebar
 * "re-click the tab you're already on" affordance to work: sidebar navigation
 * is a plain `router.push({ name })` with no query. If detail state lives ONLY
 * in a local ref, re-clicking the current tab resolves to the exact same
 * location the router is already at, so Vue Router treats it as a duplicate
 * push and silently drops it — the page never hears about it, so a detail view
 * left open has no way to snap back to its list. Mirroring the value into the
 * query makes list-vs-detail part of the resolved location, so leaving detail
 * open changes the URL, and a bare re-push to the tab's un-queried route is a
 * genuinely different destination that the router actually navigates to. Omit
 * `queryKey` for swaps that don't need this (e.g. a swap nested inside another
 * already-synced tab) — `view` then stays purely local, as before.
 *
 * The `queryKey` MUST be unique per settings page. `useRoute()` reads the ONE
 * global route, and every settings page stays mounted under <KeepAlive>
 * (settings-section/index.vue) after you navigate away — so a cached page's
 * watchers keep reading the live URL. If two pages shared a key, the cached
 * page would read the active page's value off the shared key, fail to resolve
 * that foreign ID against its own list, and strip the query — snapping the
 * active page's detail shut. Distinct keys keep each page's URL state its own;
 * a page only ever sees the key it wrote, so there is nothing to fight over.
 */
export function useViewSwap(queryKey?: string) {
  const route = queryKey ? useRoute() : null
  const router = queryKey ? useRouter() : null

  // The raw query value ('' when absent). Detail is open iff it's non-empty —
  // the page owns what the value means (a resource ID); we only care presence.
  const queryValue = computed(() => {
    if (!queryKey || !route) return ''
    const value = route.query[queryKey]
    return typeof value === 'string' ? value : ''
  })
  const queryDetail = computed(() => queryValue.value !== '')
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

  // `detailValue` is what lands in the URL. Falls back to '1' so an unkeyed or
  // legacy call still flips the view and keeps the sidebar-re-click affordance
  // — but keyed pages should always pass the resource ID, or a refresh has
  // nothing to restore.
  function openDetail(detailValue?: string) {
    direction.value = 'forward'
    view.value = 'detail'
    if (queryKey && route && router) {
      void router.replace({ query: { ...route.query, [queryKey]: detailValue || '1' } })
    }
  }

  function backToList() {
    direction.value = 'back'
    view.value = 'list'
    if (queryKey && route && router) {
      const { [queryKey]: _drop, ...rest } = route.query
      void router.replace({ query: rest })
    }
  }

  return { view, direction, queryValue, openDetail, backToList }
}
