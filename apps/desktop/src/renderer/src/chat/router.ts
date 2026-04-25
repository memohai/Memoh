import { createRouter, createMemoryHistory, type RouteLocationNormalized } from 'vue-router'

// Chat-window router. Owns ONLY chat-related routes — visiting `/settings`
// (e.g. via the chat sidebar's settings button reused from @memohai/web)
// is intercepted in a navigation guard and forwarded to the main process,
// which opens the dedicated settings BrowserWindow instead of routing
// in-place. Memory history matches Electron's file:// runtime cleanly and
// keeps the URL bar irrelevant.

const routes = [
  {
    path: '/',
    component: () => import('@memohai/web/pages/main-section/index.vue'),
    children: [
      {
        name: 'home',
        path: '',
        component: () => import('@memohai/web/pages/home/index.vue'),
      },
      {
        name: 'chat',
        path: '/chat/:botId?/:sessionId?',
        component: () => import('@memohai/web/pages/home/index.vue'),
      },
    ],
  },
  {
    name: 'Login',
    path: '/login',
    component: () => import('@memohai/web/pages/login/index.vue'),
  },
  {
    name: 'oauth-mcp-callback',
    path: '/oauth/mcp/callback',
    component: () => import('@memohai/web/pages/oauth/mcp-callback.vue'),
  },
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

router.beforeEach((to: RouteLocationNormalized) => {
  // Settings lives in its own BrowserWindow. Any in-app `router.push('/settings')`
  // (e.g. from @memohai/web's chat sidebar) is hijacked here and forwarded to
  // the main process. Returning `false` aborts the navigation so the chat
  // window stays where it was — must happen unconditionally, otherwise the
  // router falls through to the auth check below and bounces to `/login`.
  if (to.path === '/settings' || to.path.startsWith('/settings/')) {
    const openSettings = window.api?.window?.openSettings
    if (typeof openSettings === 'function') {
      void openSettings()
    } else {
      // Most common cause: a long-running `electron-vite dev` session is
      // serving a renderer page paired with a preload bundle that pre-dates
      // the IPC surface. Restart the dev process or reload the window.
      console.warn(
        '[chat-router] window.api.window.openSettings unavailable; ' +
        'preload may be stale (restart electron-vite dev) or running outside Electron',
      )
    }
    return false
  }

  const token = localStorage.getItem('token')
  if (to.fullPath === '/login') {
    return token ? { path: '/' } : true
  }
  if (to.path.startsWith('/oauth/')) {
    return true
  }
  return token ? true : { name: 'Login' }
})

export default router
