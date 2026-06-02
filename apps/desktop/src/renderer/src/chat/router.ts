import {
  createRouter,
  createMemoryHistory,
  type RouteLocationNormalized,
  type RouteRecordRaw,
} from 'vue-router'
import { SETTINGS_ROUTE_SPECS } from '../shared/settings-routes'
import { ensureOnboarding } from '@memohai/web/router-guards/onboarding'

// Chat-window router. Owns ONLY chat-related routes — visiting `/settings`
// (e.g. via the chat sidebar's settings button or any reused @memohai/web
// component that pushes `name: 'bot-detail'`, `name: 'bots'`, etc.) is
// intercepted in a navigation guard and forwarded to the main process,
// which focuses the dedicated settings BrowserWindow and instructs it to
// router.push the requested path. Memory history matches Electron's file://
// runtime cleanly and keeps the URL bar irrelevant.

// Stub component used by the settings name placeholders below. The
// `beforeEach` guard returns `false` before vue-router ever renders these,
// so the placeholder never instantiates — it exists purely so that
// `router.push({ name: 'bot-detail', params: { botId } })` resolves to a
// concrete `/settings/...` path that we can hand off to the IPC bridge.
const SettingsRouteStub = { render: () => null }

const settingsStubs: RouteRecordRaw[] = SETTINGS_ROUTE_SPECS.map(({ name, path }) => ({
  name,
  path,
  component: SettingsRouteStub,
}))

const routes: RouteRecordRaw[] = [
  {
    path: '/connect',
    name: 'ConnectServer',
    component: () => import('../connect/ConnectServer.vue'),
  },
  {
    path: '/onboarding',
    name: 'onboarding',
    component: () => import('@memohai/web/pages/onboarding/index.vue'),
  },
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
        name: 'bot',
        path: '/bot/:botName?/:sessionId?',
        component: () => import('@memohai/web/pages/home/index.vue'),
      },
      {
        // Backwards-compatible redirect for legacy UUID-based chat links.
        path: '/chat/:botName?/:sessionId?',
        redirect: (to) => {
          const botName = (to.params.botName as string) ?? ''
          return botName
            ? { name: 'bot', params: { botName, sessionId: to.params.sessionId } }
            : { name: 'home' }
        },
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
  ...settingsStubs,
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

router.beforeEach(async (to: RouteLocationNormalized) => {
  const status = await window.api.desktop.getServerStatus()
  const needsRemoteBaseUrl = status.mode === 'remote' && !status.baseUrl
  if (needsRemoteBaseUrl) {
    return to.path === '/connect' ? true : { name: 'ConnectServer' }
  }
  if (to.path === '/connect') {
    return { name: 'Login' }
  }

  // Settings lives in its own BrowserWindow. Any in-app navigation aimed at
  // the settings tree — whether via path (`router.push('/settings/bots')`)
  // or via name resolved through the placeholder stubs above
  // (`router.push({ name: 'bot-detail', params: { botId }, query: { tab } })`)
  // — is forwarded to the main process. The handler focuses an existing
  // settings window or creates one, then asks the renderer to push the same
  // full path internally. Returning `false` aborts the in-place navigation
  // so the chat window stays where it was.
  //
  // Must run before the auth check below — otherwise an anonymous user
  // bouncing through a settings link would be sent to /login on the chat
  // side instead of opening settings.
  if (to.path === '/settings' || to.path.startsWith('/settings/')) {
    const openSettings = window.api?.window?.openSettings
    if (typeof openSettings === 'function') {
      void openSettings(to.fullPath)
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
  if (!token) {
    return { name: 'Login' }
  }

  // Onboarding: redirect completed users away, let incomplete users through
  if (to.path === '/onboarding') {
    const completed = await ensureOnboarding()
    return completed ? { path: '/' } : true
  }

  const completed = await ensureOnboarding()
  if (!completed) {
    return { path: '/onboarding' }
  }

  return true
})

export default router
