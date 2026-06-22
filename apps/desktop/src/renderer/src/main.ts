// Desktop renderer entry. Owns its bootstrap chain so desktop can layer
// on Electron-specific plugins / stores / providers without touching
// @memohai/web. Pairs with `src/renderer/index.html`.

import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { PiniaColada, useQueryCache } from '@pinia/colada'
import piniaPluginPersistedstate from 'pinia-plugin-persistedstate'

import { watchEffect } from 'vue'
import i18n from '@memohai/web/i18n'
import { setupApiClient } from '@memohai/web/api-client'
import { appKeyboardCommands, createKeyboardCommandRegistry, type AppKeyboardCommand } from '@memohai/web/lib/keyboard-commands'
import { connectBrowserKeyboardShortcutsLive } from '@memohai/web/lib/browser-keyboard-shortcuts'
import { selectDesktopKeydownBindings, toElectronAccelerator } from '@memohai/web/lib/keyboard-bindings'
import { KEYBOARD_REGISTRY } from '@memohai/web/composables/useKeyboardCommand'
import { registerWorkspaceTabCommands } from '@memohai/web/pages/home/commands/workspace-tab-commands'
import { useWorkspaceTabsStore } from '@memohai/web/store/workspace-tabs'
import { useKeyboardShortcutsStore } from '@memohai/web/store/keyboard-shortcuts'

import '@fontsource-variable/inter'
import 'markstream-vue/index.css'
import '@memohai/web/style.css'
import './desktop-shell.css'
import 'animate.css'
import 'katex/dist/katex.min.css'

import App from './chat/App.vue'
import router from './chat/router'
import { setupRendererCacheSync } from './renderer-cache-sync'
import { handleRendererNavigate } from './renderer-navigation'

// Window-management fallback, intentionally kept separate from the close-tab app
// command. Cmd/Ctrl+W closes the active workspace tab; when the registry reports
// no tab claimed the command, the same key closes the window. The
// tab-close binding lives in the shared table; this window decision is an explicit
// desktop product rule, not something derived from that binding.
function closeWindowWhenNoTab(command: AppKeyboardCommand): void {
  if (command !== appKeyboardCommands.closeCurrentWorkspaceTab) return
  void window.api.window.closeSelf()
}

async function bootstrap() {
  const pendingNavigationTargets: string[] = []
  let navigationReady = false
  window.api.window.onNavigate((target) => {
    if (!navigationReady) {
      pendingNavigationTargets.push(target)
      return
    }
    handleRendererNavigate(router, target)
  })

  const status = await window.api.desktop.getServerStatus()
  const token = await window.api.desktop.authToken()
  if (token) {
    localStorage.setItem('token', token)
  }
  setupApiClient({
    baseUrl: status.baseUrl || 'http://127.0.0.1:0',
    onUnauthorized: () => router.replace({ name: 'Login' }),
  })

  const pinia = createPinia().use(piniaPluginPersistedstate)
  const keyboardCommands = createKeyboardCommandRegistry()
  registerWorkspaceTabCommands(keyboardCommands, useWorkspaceTabsStore(pinia))
  // Menu-delivered commands arrive over IPC; closing the window when no tab
  // remains is a distinct window-management concern (see closeWindowWhenNoTab).
  keyboardCommands.connect(window.api.window, closeWindowWhenNoTab)
  // keydown-delivered commands (e.g. save) share the exact web matcher; menu
  // commands are excluded here so they never double-fire with the accelerator.
  // Reading bindings from the store on every keydown lets user overrides from
  // the Keyboard Shortcuts settings page take effect immediately.
  const shortcutsStore = useKeyboardShortcutsStore(pinia)
  connectBrowserKeyboardShortcutsLive(
    keyboardCommands,
    () => selectDesktopKeydownBindings(shortcutsStore.effectiveBindings),
  )
  // Push the latest accelerators for menu-delivered commands to main so the
  // native menu items stay in sync with whatever the user has bound — without
  // this, the menu's accelerator label and matching combo would freeze at the
  // table's default.
  watchEffect(() => {
    const overrides: Record<string, string> = {}
    for (const binding of shortcutsStore.effectiveBindings) {
      if (binding.desktop !== 'menu') continue
      overrides[binding.command] = toElectronAccelerator(binding)
    }
    // Surface IPC failures: a silent reject would leave the native menu frozen
    // at the table's default accelerator until the next mutation, which the
    // user has no way to diagnose.
    window.api.desktop.setMenuAccelerators(overrides).catch((error) => {
      console.warn('failed to push menu accelerators to main', error)
    })
  })

  keyboardCommands.register(appKeyboardCommands.openSettings, () => {
    // Already inside settings → no-op. Pushing /settings would redirect to
    // /settings/bots and yank the user off whatever settings page they were on.
    if (router.currentRoute.value.path.startsWith('/settings')) return true
    void router.push('/settings').catch(() => {})
    return true
  })

  const app = createApp(App)
    .use(pinia)
    .use(PiniaColada)
    .use(router)
    .use(i18n)
    .provide(KEYBOARD_REGISTRY, keyboardCommands)

  // Bridge query-cache invalidations to other renderer hosts.
  // Must run after `PiniaColada` is installed so the store is registered.
  setupRendererCacheSync(useQueryCache())

  app.mount('#app')

  for (const target of pendingNavigationTargets.splice(0)) {
    handleRendererNavigate(router, target)
  }
  navigationReady = true
}

void bootstrap()
