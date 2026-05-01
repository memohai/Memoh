// Single source of truth for the desktop settings routes. Both the settings
// renderer (which mounts the real components) and the chat renderer (which
// installs no-op stubs so name-based `router.push({ name: 'bot-detail' })`
// calls coming from reused @memohai/web components resolve cleanly before
// being intercepted and forwarded to the settings BrowserWindow over IPC)
// import this list. Keep it in sync with @memohai/web's `/settings/*`
// children — adding a new settings page to web means adding an entry here.

import type { Component } from 'vue'

export interface SettingsRouteSpec {
  name: string
  path: string
  loader: () => Promise<Component | { default: Component }>
}

export const SETTINGS_ROUTE_SPECS: SettingsRouteSpec[] = [
  { name: 'bots', path: '/settings/bots', loader: () => import('../pages/bots/index.vue') },
  { name: 'bot-detail', path: '/settings/bots/:botId', loader: () => import('@memohai/web/pages/bots/detail.vue') },
  { name: 'providers', path: '/settings/providers', loader: () => import('@memohai/web/pages/providers/index.vue') },
  { name: 'web-search', path: '/settings/web-search', loader: () => import('@memohai/web/pages/web-search/index.vue') },
  { name: 'memory', path: '/settings/memory', loader: () => import('@memohai/web/pages/memory/index.vue') },
  { name: 'speech', path: '/settings/speech', loader: () => import('@memohai/web/pages/speech/index.vue') },
  { name: 'transcription', path: '/settings/transcription', loader: () => import('@memohai/web/pages/transcription/index.vue') },
  { name: 'email', path: '/settings/email', loader: () => import('@memohai/web/pages/email/index.vue') },
  { name: 'browser', path: '/settings/browser', loader: () => import('@memohai/web/pages/browser/index.vue') },
  { name: 'usage', path: '/settings/usage', loader: () => import('@memohai/web/pages/usage/index.vue') },
  { name: 'profile', path: '/settings/profile', loader: () => import('@memohai/web/pages/profile/index.vue') },
  { name: 'platform', path: '/settings/platform', loader: () => import('@memohai/web/pages/platform/index.vue') },
  { name: 'supermarket', path: '/settings/supermarket', loader: () => import('@memohai/web/pages/supermarket/index.vue') },
  { name: 'about', path: '/settings/about', loader: () => import('@memohai/web/pages/about/index.vue') },
]

// Default landing path used by the settings window's root redirect, and by
// the chat window when it forwards a generic `/settings` open request.
export const SETTINGS_DEFAULT_PATH = '/settings/bots'
