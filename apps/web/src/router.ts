import {
  createRouter,
  createWebHistory,
  type RouteLocationNormalized,
} from 'vue-router'
import { h } from 'vue'
import { RouterView } from 'vue-router'
import { i18nRef } from './i18n'

const routes = [
  {
    path: '/',
    redirect: '/login',
    component: () => import('@/pages/main-section/index.vue'),
    children: [
      {
        name: 'chat',
        path: '/chat',
        component: () => import('@/pages/chat/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.chat'),
        },
      },
      {
        name: 'home',
        path: '/home',
        component: () => import('@/pages/home/index.vue'),
        meta: {
          breadcrumb: i18nRef('home.title'),
        },
      },
      {
        path: '/bots',
        component: { render: () => h(RouterView) },
        meta: {
          breadcrumb: i18nRef('sidebar.bots'),
        },
        children: [
          {
            name: 'bots',
            path: '',
            component: () => import('@/pages/bots/index.vue'),
          },
          {
            name: 'bot-detail',
            path: ':botId',
            component: () => import('@/pages/bots/detail.vue'),
            meta: {
              breadcrumb: (route: RouteLocationNormalized) => route.params.botId,
            },
          },
        ],
      },
      {
        name: 'models',
        path: '/models',
        component: () => import('@/pages/models/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.models'),
        },
      },
      {
        name: 'search-providers',
        path: '/search-providers',
        component: () => import('@/pages/search-providers/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.searchProvider'),
        },
      },
      {
        name: 'memory-providers',
        path: '/memory-providers',
        component: () => import('@/pages/memory-providers/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.memoryProvider'),
        },
      },
      {
        name: 'tts-providers',
        path: '/tts-providers',
        component: () => import('@/pages/tts-providers/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.ttsProvider'),
        },
      },
      {
        name: 'email-providers',
        path: '/email-providers',
        component: () => import('@/pages/email-providers/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.emailProvider'),
        },
      },
      {
        name: 'browser-contexts',
        path: '/browser-contexts',
        component: () => import('@/pages/browser-contexts/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.browserContexts'),
        },
      },
      {
        name: 'usage',
        path: '/usage',
        component: () => import('@/pages/usage/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.usage'),
        },
      },
      {
        name: 'settings',
        path: '/settings',
        component: () => import('@/pages/settings/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.settings'),
        },
      },
      {
        name: 'platform',
        path: '/platform',
        component: () => import('@/pages/platform/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.platform'),
        },
      },
    ],
  },
  {
    name: 'Login',
    path: '/login',
    component: () => import('@/pages/login/index.vue'),
  },
  {
    name: 'oauth-mcp-callback',
    path: '/oauth/mcp/callback',
    component: () => import('@/pages/oauth/mcp-callback.vue'),
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// Handle chunk load errors (e.g. user aborted refresh, network failure, new deployment)
router.onError((error) => {
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

router.beforeEach((to) => {
  const token = localStorage.getItem('token')

  if (to.fullPath === '/login') {
    return token ? { path: '/chat' } : true
  }
  if (to.path.startsWith('/oauth/')) {
    return true
  }
  return token ? true : { name: 'Login' }
})

export default router
