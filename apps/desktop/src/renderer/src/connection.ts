import { createApp } from 'vue'

import '@memohai/web/style.css'
import './desktop-shell.css'

import ServerConnection from './connection/ServerConnection.vue'

createApp(ServerConnection).mount('#app')
