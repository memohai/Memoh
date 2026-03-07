import { createApp } from 'vue'
import './style.css'
import App from './App.vue'
import router from './router'
import { setupApiClient } from './lib/api-client'

// Configure SDK client before anything else
setupApiClient()
import { createPinia } from 'pinia'
import i18n from './i18n'
import { PiniaColada } from '@pinia/colada'
import piniaPluginPersistedstate from 'pinia-plugin-persistedstate'
import 'markstream-vue/index.css'
import 'katex/dist/katex.min.css'

// Font Awesome
import { library } from '@fortawesome/fontawesome-svg-core'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import {
  fas
} from '@fortawesome/free-solid-svg-icons'

import {
  far
} from '@fortawesome/free-regular-svg-icons'
import { fab } from '@fortawesome/free-brands-svg-icons'

library.add(
  far,
  fab,
  fas,
)

createApp(App)
  .component('FontAwesomeIcon', FontAwesomeIcon)
  .use(createPinia().use(piniaPluginPersistedstate))
  .use(PiniaColada)
  .use(router)
  .use(i18n)
  .mount('#app')
