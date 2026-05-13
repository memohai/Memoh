import { h } from 'vue'
import { createRouter, createMemoryHistory, RouterView, type RouteRecordRaw } from 'vue-router'
import { SETTINGS_ROUTE_SPECS, SETTINGS_DEFAULT_PATH, type SettingsRouteSpec } from '../shared/settings-routes'

// Settings-window router. Mirrors the path layout under `/settings/*` from
// @memohai/web's main router so the reused @memohai/web `SettingsSidebar`
// (whose `isItemActive` checks `route.path.startsWith('/settings/bots')`)
// keeps highlighting the active item correctly. The window boots straight
// into `/settings/bots` (or whatever path the chat window asks for via the
// `settings:navigate` IPC).

const mapSpecToRoute = (spec: SettingsRouteSpec): RouteRecordRaw => {
  const route = {
    path: spec.path,
    component: spec.loader ?? { render: () => h(RouterView) },
    ...(spec.name ? { name: spec.name } : {}),
    ...(spec.meta ? { meta: spec.meta } : {}),
    ...(spec.children ? { children: spec.children.map(mapSpecToRoute) } : {}),
  } satisfies RouteRecordRaw

  return route
}

const realRoutes: RouteRecordRaw[] = SETTINGS_ROUTE_SPECS.map(mapSpecToRoute)

const routes: RouteRecordRaw[] = [
  { path: '/', redirect: SETTINGS_DEFAULT_PATH },
  { path: '/settings', redirect: SETTINGS_DEFAULT_PATH },
  ...realRoutes,
]

const router = createRouter({
  history: createMemoryHistory(),
  routes,
})

router.onError((error: Error) => {
  const isChunkLoadError =
    error.message.includes('Failed to fetch dynamically imported module') ||
    error.message.includes('Importing a module script failed') ||
    error.message.includes('error loading dynamically imported module')
  if (isChunkLoadError) {
    console.warn('[Router] Chunk load failed, reloading...', error.message)
    window.location.reload()
    return
  }
  throw error
})

export default router
