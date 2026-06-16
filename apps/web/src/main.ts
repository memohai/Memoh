import { createApp } from 'vue'
// Inter Variable (full 100-900 weight axis). The design system's fractional
// weights (360/450/520 etc. and the chat body) only interpolate with a variable
// font; without this, a locally-installed static Inter snaps them to 100-steps.
import '@fontsource-variable/inter'
import 'markstream-vue/index.css'
import './style.css'
import App from './App.vue'
import router from './router'
import { createKeyboardCommandRegistry } from './lib/keyboard-commands'
import { connectBrowserKeyboardShortcutsLive } from './lib/browser-keyboard-shortcuts'
import { selectWebBindings } from './lib/keyboard-bindings'
import { KEYBOARD_REGISTRY } from './composables/useKeyboardCommand'
import { setupApiClient } from './lib/api-client'
import { registerWorkspaceTabCommands } from './pages/home/commands/workspace-tab-commands'
import { useWorkspaceTabsStore } from './store/workspace-tabs'
import { useKeyboardShortcutsStore } from './store/keyboard-shortcuts'
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
// Browser-owned combos (e.g. Cmd/Ctrl+W on its default) are excluded by
// selectWebBindings, so they keep their native behavior — we don't intercept
// them in the browser. The getter form reads from the shortcuts store on each
// keydown so user overrides take effect without re-binding the listener.
const shortcutsStore = useKeyboardShortcutsStore(pinia)
connectBrowserKeyboardShortcutsLive(
  keyboardCommands,
  () => selectWebBindings(shortcutsStore.effectiveBindings),
)

createApp(App)
  .use(pinia)
  .use(PiniaColada)
  .use(router)
  .use(i18n)
  .provide(KEYBOARD_REGISTRY, keyboardCommands)
  .mount('#app')
