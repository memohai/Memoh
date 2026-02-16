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
  faChevronLeft,
  faChevronRight,
  faChevronDown,
  faEllipsisVertical,
  faNetworkWired,
  faPlug,
  faUser,
  faArrowLeft,
  faCommentDots,
  faHashtag,
  faGlobe,
  faBuilding,
  faBell,
  faRotate,
  faFileLines,
  faBrain,
  faCopy,
  faCompress,
  faPaperclip,
  faXmark,
  faImage,
  faFile,
  faMusic,
  faVideo,
} from '@fortawesome/free-solid-svg-icons'
import {
  faRectangleList,
  faTrashCan,
  faComments,
  faComment,
} from '@fortawesome/free-regular-svg-icons'
import { faSlack, faBrave } from '@fortawesome/free-brands-svg-icons'

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
  faChevronLeft,
  faChevronRight,
  faChevronDown,
  faEllipsisVertical,
  faNetworkWired,
  faPlug,
  faUser,
  faArrowLeft,
  faCommentDots,
  faHashtag,
  faGlobe,
  faBuilding,
  faBell,
  faRotate,
  faFileLines,
  faBrain,
  faCopy,
  faCompress,
  faPaperclip,
  faXmark,
  faImage,
  faFile,
  faMusic,
  faVideo,
  faRectangleList,
  faTrashCan,
  faComments,
  faComment,
  faSlack,
  faBrave,
)

createApp(App)
  .component('FontAwesomeIcon', FontAwesomeIcon)
  .use(createPinia().use(piniaPluginPersistedstate))
  .use(PiniaColada)
  .use(router)
  .use(i18n)
  .mount('#app')
