// Chat-window renderer entry. Owns its bootstrap chain so desktop can layer
// on Electron-specific plugins / stores / providers without touching
// @memohai/web. Pairs with `src/renderer/index.html`.

import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { PiniaColada, useQueryCache } from '@pinia/colada'
import piniaPluginPersistedstate from 'pinia-plugin-persistedstate'

import i18n from '@memohai/web/i18n'
import { setupApiClient } from '@memohai/web/api-client'
import { useWorkspaceTabsStore } from '@memohai/web/store/workspace-tabs'

import 'markstream-vue/index.css'
import '@memohai/web/style.css'
import './desktop-shell.css'
import 'animate.css'
import 'katex/dist/katex.min.css'

import App from './chat/App.vue'
import router from './chat/router'
import { setupCrossWindowCacheSync } from './cross-window-cache-sync'
import { createKeyboardCommandRegistry } from './keyboard-command-registry'
import { registerWorkspaceTabCommands } from './chat/workspace-tab-commands'

async function bootstrap() {
  const status = await window.api.desktop.getServerStatus()
  const token = await window.api.desktop.authToken()
  if (token) {
    localStorage.setItem('token', token)
  }
  setupApiClient({
    baseUrl: status.baseUrl || 'http://127.0.0.1:0',
    onUnauthorized: () => router.replace({ name: 'Login' }),
  })
  window.api.window.onChatNavigate((target) => {
    if (!target.startsWith('/bot') && !target.startsWith('/chat')) return
    if (router.currentRoute.value.fullPath === target) return
    void router.push(target)
  })

  const pinia = createPinia().use(piniaPluginPersistedstate)
  const keyboardCommands = createKeyboardCommandRegistry()
  keyboardCommands.connect(window.api.window)
  registerWorkspaceTabCommands(keyboardCommands, useWorkspaceTabsStore(pinia))

  const app = createApp(App)
    .use(pinia)
    .use(PiniaColada)
    .use(router)
    .use(i18n)

  // Bridge query-cache invalidations between chat and settings windows.
  // Must run after `PiniaColada` is installed so the store is registered.
  setupCrossWindowCacheSync(useQueryCache())

  app.mount('#app')
}

void bootstrap()
