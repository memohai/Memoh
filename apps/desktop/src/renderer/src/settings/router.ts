import { createRouter, createMemoryHistory } from 'vue-router'

// Settings-window router. Mirrors the path layout under `/settings/*` from
// @memohai/web's main router so the reused @memohai/web `SettingsSidebar`
// (whose `isItemActive` checks `route.path.startsWith('/settings/bots')`)
// keeps highlighting the active item correctly. The window boots straight
// into `/settings/bots`.

const routes = [
  { path: '/', redirect: '/settings/bots' },
  { path: '/settings', redirect: '/settings/bots' },
  {
    path: '/settings/bots',
    name: 'bots',
    component: () => import('@memohai/web/pages/bots/index.vue'),
  },
  {
    path: '/settings/bots/:botId',
    name: 'bot-detail',
    component: () => import('@memohai/web/pages/bots/detail.vue'),
  },
  {
    path: '/settings/providers',
    name: 'providers',
    component: () => import('@memohai/web/pages/providers/index.vue'),
  },
  {
    path: '/settings/web-search',
    name: 'web-search',
    component: () => import('@memohai/web/pages/web-search/index.vue'),
  },
  {
    path: '/settings/memory',
    name: 'memory',
    component: () => import('@memohai/web/pages/memory/index.vue'),
  },
  {
    path: '/settings/speech',
    name: 'speech',
    component: () => import('@memohai/web/pages/speech/index.vue'),
  },
  {
    path: '/settings/transcription',
    name: 'transcription',
    component: () => import('@memohai/web/pages/transcription/index.vue'),
  },
  {
    path: '/settings/email',
    name: 'email',
    component: () => import('@memohai/web/pages/email/index.vue'),
  },
  {
    path: '/settings/browser',
    name: 'browser',
    component: () => import('@memohai/web/pages/browser/index.vue'),
  },
  {
    path: '/settings/usage',
    name: 'usage',
    component: () => import('@memohai/web/pages/usage/index.vue'),
  },
  {
    path: '/settings/profile',
    name: 'profile',
    component: () => import('@memohai/web/pages/profile/index.vue'),
  },
  {
    path: '/settings/platform',
    name: 'platform',
    component: () => import('@memohai/web/pages/platform/index.vue'),
  },
  {
    path: '/settings/supermarket',
    name: 'supermarket',
    component: () => import('@memohai/web/pages/supermarket/index.vue'),
  },
  {
    path: '/settings/about',
    name: 'about',
    component: () => import('@memohai/web/pages/about/index.vue'),
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

export default router
