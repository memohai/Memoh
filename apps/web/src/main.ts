import { createApp } from 'vue'
// Inter Variable (full 100-900 weight axis). The design system's fractional
// weights (360/450/520 etc. and the chat body) only interpolate with a variable
// font; without this, a locally-installed static Inter snaps them to 100-steps.
import '@fontsource-variable/inter'
import 'markstream-vue/index.css'
import './style.css'
import App from './App.vue'
import router from './router'
import { appKeyboardCommands, createKeyboardCommandRegistry } from './lib/keyboard-commands'
import { connectBrowserKeyboardShortcutsLive } from './lib/browser-keyboard-shortcuts'
import { selectWebBindings } from './lib/keyboard-bindings'
import { KEYBOARD_REGISTRY } from './composables/useKeyboardCommand'
import { setupApiClient } from './lib/api-client'
import { DesktopShellKey } from './lib/desktop-shell'
import { registerWorkspaceTabCommands } from './pages/home/commands/workspace-tab-commands'
import { useWorkspaceTabsStore } from './store/workspace-tabs'
import { useKeyboardShortcutsStore } from './store/keyboard-shortcuts'
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
keyboardCommands.register(appKeyboardCommands.openSettings, () => {
  // Already inside settings → no-op. Pushing /settings would redirect to
  // /settings/bots and yank the user off whatever settings page they were on.
  if (router.currentRoute.value.path.startsWith('/settings')) return true
  void router.push('/settings').catch(() => {})
  return true
})

// Capture-only escape hatch: `?desktopShell=1` forces the macOS desktop layout
// (traffic-light reserve + integrated tabs + pinned sidebar) in the browser, so
// the marketing hero can be screen-recorded against the real running build.
// Without the query param the browser keeps the default `false` — this must
// never be on for normal web visitors.
const forceDesktopShell =
  typeof location !== 'undefined'
  && new URLSearchParams(location.search).has('desktopShell')

const app = createApp(App)
  .use(pinia)
  .use(PiniaColada)
  .use(router)
  .use(i18n)
  .provide(KEYBOARD_REGISTRY, keyboardCommands)

if (forceDesktopShell) {
  app.provide(DesktopShellKey, true)
}

app.mount('#app')
