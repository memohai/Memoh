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
        name: 'settings',
        path: '/settings',
        component: () => import('@/pages/settings/index.vue'),
        meta: {
          breadcrumb: i18nRef('sidebar.settings'),
        },
      },
      {
        name: 'settings-user',
        path: '/settings/user',
        component: () => import('@/pages/settings/user.vue'),
        meta: {
          breadcrumb: i18nRef('settings.user'),
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
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})
router.beforeEach((to) => {
  const token = localStorage.getItem('token')

  if (to.fullPath !== '/login') {
    return token ? true : { name: 'Login' }
  } else {
    return token ? { path: '/chat' } : true
  }
})

export default router
