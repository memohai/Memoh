import { createApp } from 'vue'
import './style.css'
import App from './App.vue'
import router from './router'
import { setupApiClient } from './lib/api-client'
import 'animate.css'

// Configure SDK client before anything else
setupApiClient()
import { createPinia } from 'pinia'
import i18n from './i18n'
import { PiniaColada } from '@pinia/colada'
import piniaPluginPersistedstate from 'pinia-plugin-persistedstate'
import 'markstream-vue/index.css'
import 'katex/dist/katex.min.css'

createApp(App)
  .use(createPinia().use(piniaPluginPersistedstate))
  .use(PiniaColada)
  .use(router)
  .use(i18n)
  .mount('#app')
