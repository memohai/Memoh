import { createRouter, createMemoryHistory, type RouteRecordRaw } from 'vue-router'
import { SETTINGS_ROUTE_SPECS, SETTINGS_DEFAULT_PATH } from '../shared/settings-routes'

// Settings-window router. Mirrors the path layout under `/settings/*` from
// @memohai/web's main router so the reused @memohai/web `SettingsSidebar`
// (whose `isItemActive` checks `route.path.startsWith('/settings/bots')`)
// keeps highlighting the active item correctly. The window boots straight
// into `/settings/bots` (or whatever path the chat window asks for via the
// `settings:navigate` IPC).

const realRoutes: RouteRecordRaw[] = SETTINGS_ROUTE_SPECS.map(({ name, path, loader }) => ({
  name,
  path,
  component: loader,
}))

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
