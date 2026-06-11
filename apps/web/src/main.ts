import { createApp } from 'vue'
import 'markstream-vue/index.css'
import './style.css'
import App from './App.vue'
import router from './router'
import { createKeyboardCommandRegistry } from './lib/keyboard-commands'
import { connectBrowserKeyboardShortcuts } from './lib/browser-keyboard-shortcuts'
import { keyboardBindings, selectWebBindings } from './lib/keyboard-bindings'
import { KEYBOARD_REGISTRY } from './composables/useKeyboardCommand'
import { setupApiClient } from './lib/api-client'
import { registerWorkspaceTabCommands } from './pages/home/commands/workspace-tab-commands'
import { useWorkspaceTabsStore } from './store/workspace-tabs'
import 'animate.css'
import { createPinia } from 'pinia'
import i18n from './i18n'
import { PiniaColada } from '@pinia/colada'
import piniaPluginPersistedstate from 'pinia-plugin-persistedstate'
import 'katex/dist/katex.min.css'

setupApiClient({
  onUnauthorized: () => router.replace({ name: 'Login' }),
})

const pinia = createPinia().use(piniaPluginPersistedstate)
const keyboardCommands = createKeyboardCommandRegistry()
registerWorkspaceTabCommands(keyboardCommands, useWorkspaceTabsStore(pinia))
// Browser-owned combos (e.g. Cmd/Ctrl+W) are excluded by selectWebBindings, so
// they keep their native behavior — we don't intercept them in the browser.
connectBrowserKeyboardShortcuts(keyboardCommands, selectWebBindings(keyboardBindings))

createApp(App)
  .use(pinia)
  .use(PiniaColada)
  .use(router)
  .use(i18n)
  .provide(KEYBOARD_REGISTRY, keyboardCommands)
  .mount('#app')
