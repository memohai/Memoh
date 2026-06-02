import {
  createRouter,
  createWebHistory,
  type RouteLocationNormalized,
} from 'vue-router'
import { h } from 'vue'
import { RouterView } from 'vue-router'
import { i18nRef } from './i18n'
import { useUserStore } from '@/store/user'
import { ensureOnboarding } from '@/router-guards/onboarding'

const routes = [
  {
    path: '/onboarding',
    name: 'onboarding',
    component: () => import('@/pages/onboarding/index.vue'),
  },
  {
    path: '/',
    component: () => import('@/pages/main-section/index.vue'),
    children: [
      {
        name: 'home',
        path: '',
        component: () => import('@/pages/home/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.chat'),
        },
      },
      {
        name: 'bot',
        path: '/bot/:botName?',
        component: () => import('@/pages/home/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.chat'),
        },
      },
      {
        // Backwards-compatible redirect for legacy UUID-based chat links.
        path: '/chat/:botName?',
        redirect: (to) => {
          const botName = (to.params.botName as string) ?? ''
          return botName
            ? { name: 'bot', params: { botName } }
            : { name: 'home' }
        },
      },
    ],
  },
  {
    path: '/settings',
    component: () => import('@/pages/settings-section/index.vue'),
    redirect: '/settings/bots',
    children: [
      {
        path: 'bots',
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
            name: 'bot-new',
            path: 'new',
            component: () => import('@/pages/bots/new.vue'),
            meta: {
              breadcrumb: i18nRef('bots.createBot'),
            },
          },
          {
            name: 'bot-detail',
            path: ':botName',
            component: () => import('@/pages/bots/detail.vue'),
            meta: {
              breadcrumb: (route: RouteLocationNormalized) => route.params.botName,
            },
          },
        ],
      },
      {
        name: 'providers',
        path: 'providers',
        component: () => import('@/pages/providers/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.providers'),
        },
      },
      {
        name: 'web-search',
        path: 'web-search',
        component: () => import('@/pages/web-search/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.webSearch'),
        },
      },
      {
        name: 'memory',
        path: 'memory',
        component: () => import('@/pages/memory/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.memory'),
        },
      },
      {
        name: 'speech',
        path: 'speech',
        component: () => import('@/pages/speech/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.speech'),
        },
      },
      {
        name: 'transcription',
        path: 'transcription',
        component: () => import('@/pages/transcription/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.transcription'),
        },
      },
      {
        name: 'email',
        path: 'email',
        component: () => import('@/pages/email/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.email'),
        },
      },
      {
        name: 'usage',
        path: 'usage',
        component: () => import('@/pages/usage/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.usage'),
        },
      },
      {
        name: 'people',
        path: 'people',
        component: () => import('@/pages/people/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.people'),
          adminOnly: true,
        },
      },
      {
        name: 'appearance',
        path: 'appearance',
        component: () => import('@/pages/appearance/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.appearance'),
        },
      },
      {
        name: 'profile',
        path: 'profile',
        component: () => import('@/pages/profile/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.settings'),
        },
      },
      {
        name: 'platform',
        path: 'platform',
        component: () => import('@/pages/platform/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.platform'),
        },
      },
      {
        name: 'supermarket',
        path: 'supermarket',
        component: () => import('@/pages/supermarket/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.supermarket'),
        },
      },
      {
        name: 'about',
        path: 'about',
        component: () => import('@/pages/about/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.about'),
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

router.beforeEach(async (to) => {
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
