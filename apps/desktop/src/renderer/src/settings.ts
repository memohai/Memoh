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
import 'animate.css'
import 'markstream-vue/index.css'
import 'katex/dist/katex.min.css'

import App from './settings/App.vue'
import router from './settings/router'

setupApiClient({
  // Settings is a satellite window — it doesn't host the login screen.
  // On 401 we close ourselves and let the chat window route to login.
  onUnauthorized: () => {
    void window.api.window.closeSelf()
  },
})

createApp(App)
  .use(createPinia().use(piniaPluginPersistedstate))
  .use(PiniaColada)
  .use(router)
  .use(i18n)
  .mount('#app')
