// Single source of truth for the desktop settings routes. Both the settings
// renderer (which mounts the real components) and the chat renderer (which
// installs no-op stubs so name-based `router.push({ name: 'bot-detail' })`
// calls coming from reused @memohai/web components resolve cleanly before
// being intercepted and forwarded to the settings BrowserWindow over IPC)
// import this list. Keep it in sync with @memohai/web's `/settings/*`
// children — adding a new settings page to web means adding an entry here.

import type { Component } from 'vue'
import { i18nRef } from '@memohai/web/i18n'

export interface SettingsRouteSpec {
  name?: string
  path: string
  loader?: () => Promise<Component | { default: Component }>
  meta?: {
    breadcrumb?: string | { value: string } | ((route: { params: Record<string, string | string[] | undefined> }) => string | undefined)
  }
  children?: SettingsRouteSpec[]
}

export const SETTINGS_ROUTE_SPECS: SettingsRouteSpec[] = [
  {
    path: '/settings/bots',
    meta: { breadcrumb: i18nRef('sidebar.bots') },
    children: [
      {
        name: 'bots',
        path: '',
        loader: () => import('@memohai/web/pages/bots/index.vue'),
      },
      {
        name: 'bot-new',
        path: 'new',
        loader: () => import('@memohai/web/pages/bots/new.vue'),
        meta: { breadcrumb: i18nRef('bots.createBot') }
      },
      {
        name: 'bot-detail',
        path: ':botId',
        loader: () => import('@memohai/web/pages/bots/detail.vue'),
        meta: { breadcrumb: (route: { params: { botId?: string } }) => route.params.botId }
      },
    ]
  },
  {
    name: 'providers',
    path: '/settings/providers',
    loader: () => import('@memohai/web/pages/providers/index.vue'),
    meta: { breadcrumb: i18nRef('sidebar.providers') }
  },
  {
    name: 'web-search',
    path: '/settings/web-search',
    loader: () => import('@memohai/web/pages/web-search/index.vue'),
    meta: { breadcrumb: i18nRef('sidebar.webSearch') }
  },
  {
    name: 'memory',
    path: '/settings/memory',
    loader: () => import('@memohai/web/pages/memory/index.vue'),
    meta: { breadcrumb: i18nRef('sidebar.memory') }
  },
  {
    name: 'speech',
    path: '/settings/speech',
    loader: () => import('@memohai/web/pages/speech/index.vue'),
    meta: { breadcrumb: i18nRef('sidebar.speech') }
  },
  {
    name: 'transcription',
    path: '/settings/transcription',
    loader: () => import('@memohai/web/pages/transcription/index.vue'),
    meta: { breadcrumb: i18nRef('sidebar.transcription') }
  },
  {
    name: 'email',
    path: '/settings/email',
    loader: () => import('@memohai/web/pages/email/index.vue'),
    meta: { breadcrumb: i18nRef('sidebar.email') }
  },
  {
    name: 'usage',
    path: '/settings/usage',
    loader: () => import('@memohai/web/pages/usage/index.vue'),
    meta: { breadcrumb: i18nRef('sidebar.usage') }
  },
  {
    name: 'appearance',
    path: '/settings/appearance',
    loader: () => import('@memohai/web/pages/appearance/index.vue'),
    meta: { breadcrumb: i18nRef('sidebar.appearance') }
  },
  {
    name: 'profile',
    path: '/settings/profile',
    loader: () => import('@memohai/web/pages/profile/index.vue'),
    meta: { breadcrumb: i18nRef('sidebar.profile') }
  },
  {
    name: 'platform',
    path: '/settings/platform',
    loader: () => import('@memohai/web/pages/platform/index.vue'),
    meta: { breadcrumb: i18nRef('sidebar.platform') }
  },
  {
    name: 'supermarket',
    path: '/settings/supermarket',
    loader: () => import('@memohai/web/pages/supermarket/index.vue'),
    meta: { breadcrumb: i18nRef('sidebar.supermarket') }
  },
  {
    name: 'about',
    path: '/settings/about',
    loader: () => import('@memohai/web/pages/about/index.vue'),
    meta: { breadcrumb: i18nRef('sidebar.about') }
  },
]

// Default landing path used by the settings window's root redirect, and by
// the chat window when it forwards a generic `/settings` open request.
export const SETTINGS_DEFAULT_PATH = '/settings/bots'
