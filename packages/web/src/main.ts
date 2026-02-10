import { createApp } from 'vue'
import './style.css'
import App from './App.vue'
import router from './router'
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
  faGear,
  faPaperPlane,
  faRobot,
  faMagnifyingGlass,
  faPlus,
  faSpinner,
  faCubes,
  faPenToSquare,
  faCheck,
  faEye,
  faEyeSlash,
} from '@fortawesome/free-solid-svg-icons'
import {
  faRectangleList,
  faTrashCan,
  faComments,
} from '@fortawesome/free-regular-svg-icons'

library.add(
  faGear,
  faPaperPlane,
  faRobot,
  faMagnifyingGlass,
  faPlus,
  faSpinner,
  faCubes,
  faPenToSquare,
  faCheck,
  faEye,
  faEyeSlash,
  faRectangleList,
  faTrashCan,
  faComments,
)

createApp(App)
  .component('FontAwesomeIcon', FontAwesomeIcon)
  .use(createPinia().use(piniaPluginPersistedstate))
  .use(PiniaColada)
  .use(router)
  .use(i18n)
  .mount('#app')
