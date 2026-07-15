import { computed, ref, watch, watchEffect } from 'vue'
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
 * Pass `queryKey` to mirror the open resource into the URL as `?<queryKey>=<value>`
 * (written with router.replace — an in-page state sync, not a navigation, same
 * contract as useSyncedQueryParam). The value is the open resource's stable ID
 * (or a page-owned encoding such as `speech:<id>`), so a refresh can restore the
 * exact item. Query mirroring is also required for the settings-sidebar
 * "re-click the tab you're already on" affordance: sidebar navigation is a plain
 * `router.push({ name })` with no query. If detail state lives ONLY in a local
 * ref, re-clicking the current tab resolves to the exact same location the
 * router is already at, so Vue Router treats it as a duplicate push and silently
 * drops it — the page never hears about it. Mirroring the value into the query
 * makes list-vs-detail part of the resolved location. Omit `queryKey` for swaps
 * that don't need this (e.g. a swap nested inside another already-synced tab) —
 * `view` then stays purely local.
 *
 * The `queryKey` MUST be unique per settings page. `useRoute()` reads the ONE
 * global route, and every settings page stays mounted under <KeepAlive>
 * (settings-section/index.vue) after you navigate away — so a cached page's
 * watchers keep reading the live URL. If two pages shared a key, the cached
 * page would read the active page's value off the shared key, fail to resolve
 * that foreign ID against its own list, and strip the query — snapping the
 * active page's detail shut. Distinct keys keep each page's URL state its own.
 *
 * For resource resolution (URL → selected object, invalid id → list, loading
 * gate), use `useRoutedViewSwap` on top of this. This composable only owns
 * motion + query presence.
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
    // Reacts to the query value changing for ANY reason — our own replace()
    // calls below, but also external navigations we don't control: a sidebar
    // re-click that lands on the un-queried route, or the user's own browser
    // back/forward. Whatever the cause, `view` must follow the URL, and losing
    // the value always reads as "closing", so it always animates as 'back'.
    // The equality guard skips the case where openDetail/backToList already
    // applied this exact state locally before writing the query, so the two
    // never fight over `direction` for the same transition.
    watch(queryDetail, (isDetail) => {
      if (isDetail === (view.value === 'detail')) return
      direction.value = isDetail ? 'forward' : 'back'
      view.value = isDetail ? 'detail' : 'list'
    })
  }

  // Keyed pages must pass a non-empty resource value — no silent '1' fallback.
  // Unkeyed swaps ignore the argument (pure local list ↔ detail).
  function openDetail(detailValue?: string) {
    if (queryKey && route && router) {
      if (!detailValue) return
      direction.value = 'forward'
      view.value = 'detail'
      void router.replace({ query: { ...route.query, [queryKey]: detailValue } })
      return
    }
    direction.value = 'forward'
    view.value = 'detail'
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

export interface RoutedViewSwapOptions<T> {
  /** Page-owned query key — must be unique under settings KeepAlive. */
  key: string
  items: () => readonly T[]
  selected: () => T | undefined
  select: (item: T | undefined) => void
  getRouteValue: (item: T) => string
  /**
   * True while the source that would resolve `routeValue` is still fetching.
   * Used with `isReady` so a missing match during a stale-cache refresh does
   * not snap the detail shut before fresh data arrives.
   */
  isLoading: (routeValue: string) => boolean
  /**
   * True once the relevant data source has answered at least once
   * (`data !== undefined`). Only then is "not in list" treated as deleted.
   */
  isReady: (routeValue: string) => boolean
}

/**
 * Adds resource resolution to useViewSwap. The URL remains the source of truth;
 * the selected object is a projection of the latest query result. A route is
 * rejected only after its relevant data source is ready and no longer loading.
 *
 * `isDetailLoading` is true when the URL asks for detail but the object is not
 * resolved yet — DetailPane can show a skeleton instead of an empty shell.
 */
export function useRoutedViewSwap<T>(options: RoutedViewSwapOptions<T>) {
  const swap = useViewSwap(options.key)

  function openDetail(item: T) {
    const routeValue = options.getRouteValue(item)
    if (!routeValue) return
    options.select(item)
    swap.openDetail(routeValue)
  }

  function backToList() {
    options.select(undefined)
    swap.backToList()
  }

  watchEffect(() => {
    const routeValue = swap.queryValue.value
    if (!routeValue) {
      // URL closed detail (sidebar re-click, browser back, etc.) — drop
      // selection only once view has followed the URL to list. openDetail
      // selects first, then replace() updates the query; clearing while view is
      // still 'detail' would wipe that optimistic selection before the route
      // catches up.
      if (swap.view.value === 'list' && options.selected() !== undefined) {
        options.select(undefined)
      }
      return
    }

    const selected = options.items().find(item => options.getRouteValue(item) === routeValue)
    if (selected) {
      options.select(selected)
      return
    }

    if (!options.isLoading(routeValue) && options.isReady(routeValue)) {
      backToList()
    }
  })

  const isDetailLoading = computed(() => {
    const routeValue = swap.queryValue.value
    if (!routeValue) return false
    if (options.selected() !== undefined) return false
    return options.isLoading(routeValue) || !options.isReady(routeValue)
  })

  return {
    view: swap.view,
    direction: swap.direction,
    queryValue: swap.queryValue,
    isDetailLoading,
    openDetail,
    backToList,
  }
}
