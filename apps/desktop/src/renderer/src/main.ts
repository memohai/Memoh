// Chat-window renderer entry. Owns its bootstrap chain so desktop can layer
// on Electron-specific plugins / stores / providers without touching
// @memohai/web. Pairs with `src/renderer/index.html`.

import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { PiniaColada, useQueryCache } from '@pinia/colada'
import piniaPluginPersistedstate from 'pinia-plugin-persistedstate'

import i18n from '@memohai/web/i18n'
import { setupApiClient } from '@memohai/web/api-client'

import '@memohai/web/style.css'
import './desktop-shell.css'
import 'animate.css'
import 'markstream-vue/index.css'
import 'katex/dist/katex.min.css'

import App from './chat/App.vue'
import router from './chat/router'
import { setupCrossWindowCacheSync } from './cross-window-cache-sync'

async function bootstrap() {
  const token = await window.api.desktop.authToken()
  if (token) {
    localStorage.setItem('token', token)
  }
  setupApiClient({
    baseUrl: await window.api.desktop.apiBaseUrl(),
    onUnauthorized: () => router.replace({ name: 'Login' }),
  })

  const app = createApp(App)
    .use(createPinia().use(piniaPluginPersistedstate))
    .use(PiniaColada)
    .use(router)
    .use(i18n)

  // Bridge query-cache invalidations between chat and settings windows.
  // Must run after `PiniaColada` is installed so the store is registered.
  setupCrossWindowCacheSync(useQueryCache())

  app.mount('#app')
}

void bootstrap()
