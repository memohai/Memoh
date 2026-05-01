// Settings-window renderer entry. Loaded by the secondary BrowserWindow
// created on demand from the chat window. Shares Pinia-persisted state with
// the chat window via localStorage. Pairs with `src/renderer/settings.html`.

import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { PiniaColada } from '@pinia/colada'
import piniaPluginPersistedstate from 'pinia-plugin-persistedstate'

import i18n from '@memohai/web/i18n'
import { setupApiClient } from '@memohai/web/api-client'

import '@memohai/web/style.css'
import './desktop-shell.css'
import 'animate.css'
import 'markstream-vue/index.css'
import 'katex/dist/katex.min.css'

import App from './settings/App.vue'
import router from './settings/router'

async function bootstrap() {
  setupApiClient({
    baseUrl: await window.api.desktop.apiBaseUrl(),
    // Settings is a satellite window — it doesn't host the login screen.
    // On 401 we close ourselves and let the chat window route to login.
    onUnauthorized: () => {
      void window.api.window.closeSelf()
    },
  })

  // Cross-window navigation: chat-side `router.push('/settings/...')` calls
  // reach us as IPC `settings:navigate` events forwarded by the main
  // process. Wire the listener up before mounting so the very first event
  // (sent on cold-start replay right after `did-finish-load`) is captured.
  window.api.window.onSettingsNavigate((target) => {
    if (router.currentRoute.value.fullPath === target) return
    void router.push(target)
  })

  createApp(App)
    .use(createPinia().use(piniaPluginPersistedstate))
    .use(PiniaColada)
    .use(router)
    .use(i18n)
    .mount('#app')
}

void bootstrap()
