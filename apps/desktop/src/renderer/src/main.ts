// Chat-window renderer entry. Owns its bootstrap chain so desktop can layer
// on Electron-specific plugins / stores / providers without touching
// @memohai/web. Pairs with `src/renderer/index.html`.

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

import App from './chat/App.vue'
import router from './chat/router'

async function bootstrap() {
  setupApiClient({
    baseUrl: await window.api.desktop.apiBaseUrl(),
    onUnauthorized: () => router.replace({ name: 'Login' }),
  })

  createApp(App)
    .use(createPinia().use(piniaPluginPersistedstate))
    .use(PiniaColada)
    .use(router)
    .use(i18n)
    .mount('#app')
}

void bootstrap()
