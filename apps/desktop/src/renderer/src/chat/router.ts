import { h } from 'vue'
import {
  createRouter,
  createMemoryHistory,
  RouterView,
  type RouteLocationNormalized,
  type RouteRecordRaw,
} from 'vue-router'
import { SETTINGS_DEFAULT_PATH, SETTINGS_ROUTE_SPECS } from '../shared/settings-routes'
import { ensureOnboarding } from '@memohai/web/router-guards/onboarding'
import { useUserStore } from '@memohai/web/store/user'

import type { SettingsRouteSpec } from '../shared/settings-routes'

// Map settings route specs to actual components instead of stubs
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

const realSettingsRoutes: RouteRecordRaw[] = SETTINGS_ROUTE_SPECS.map(mapSpecToRoute)

const routes: RouteRecordRaw[] = [
  {
    path: '/connect',
    redirect: { name: 'Login' },
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
  // Dev-only component wall / design-token reference. Registered only in dev
  // builds. Open with Cmd/Ctrl+Shift+D (see chat/App.vue) or, from devtools,
  // `window.__memohRouter.push('/dev/components')`.
  ...(import.meta.env.DEV
    ? [
        {
          name: 'dev-components',
          path: '/dev/components',
          component: () => import('@memohai/web/pages/dev/components/index.vue'),
        } satisfies RouteRecordRaw,
      ]
    : []),
  {
    path: '/settings',
    component: () => import('@memohai/web/pages/settings-section/index.vue'),
    redirect: SETTINGS_DEFAULT_PATH,
    children: realSettingsRoutes,
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

router.beforeEach(async (to: RouteLocationNormalized) => {
  // Dev component wall: allow in dev builds, never reachable in prod.
  if (to.path.startsWith('/dev/')) {
    return import.meta.env.DEV ? true : { path: '/' }
  }

  if (to.path === '/connect') {
    return { name: 'Login' }
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
  if (to.meta.adminOnly) {
    const userStore = useUserStore()
    if (String(userStore.userInfo.role).toLowerCase() !== 'admin') {
      return { name: 'bots' }
    }
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

window.api?.window?.onSettingsNavigate?.((target: string) => {
  void router.push(target)
})

// Dev convenience: reach the component wall from devtools without a URL bar
// (memory history). e.g. `window.__memohRouter.push('/dev/components')`.
if (import.meta.env.DEV) {
  ;(window as unknown as { __memohRouter?: typeof router }).__memohRouter = router
}

export default router
