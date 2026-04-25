import { createApp } from 'vue'
import './style.css'
import App from './App.vue'
import router from './router'
import { setupApiClient } from './lib/api-client'
import 'animate.css'
import { createPinia } from 'pinia'
import i18n from './i18n'
import { PiniaColada } from '@pinia/colada'
import piniaPluginPersistedstate from 'pinia-plugin-persistedstate'
import 'markstream-vue/index.css'
import 'katex/dist/katex.min.css'

setupApiClient({
  onUnauthorized: () => router.replace({ name: 'Login' }),
})

createApp(App)
  .use(createPinia().use(piniaPluginPersistedstate))
  .use(PiniaColada)
  .use(router)
  .use(i18n)
  .mount('#app')
