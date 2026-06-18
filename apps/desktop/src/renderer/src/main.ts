// Chat-window renderer entry. Owns its bootstrap chain so desktop can layer
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
import { useChatStore } from '@memohai/web/store/chat-list'

import '@fontsource-variable/inter'
import 'markstream-vue/index.css'
import '@memohai/web/style.css'
import './desktop-shell.css'
import 'animate.css'
import 'katex/dist/katex.min.css'

import App from './chat/App.vue'
import router from './chat/router'
import { setupCrossWindowCacheSync } from './cross-window-cache-sync'

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
  const status = await window.api.desktop.getServerStatus()
  const token = await window.api.desktop.authToken()
  if (token) {
    localStorage.setItem('token', token)
  }
  setupApiClient({
    baseUrl: status.baseUrl || 'http://127.0.0.1:0',
    onUnauthorized: () => router.replace({ name: 'Login' }),
  })
  window.api.window.onChatNavigate((target) => {
    if (!target.startsWith('/bot') && !target.startsWith('/chat')) return
    if (router.currentRoute.value.fullPath === target) return
    void router.push(target)
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

  // Bridge query-cache invalidations between chat and settings windows.
  // Must run after `PiniaColada` is installed so the store is registered.
  setupCrossWindowCacheSync(useQueryCache())

  // The composer reads its agent list from the chat store's one-shot bot
  // snapshot, not the Colada cache, so the sync above can't refresh it. When the
  // settings window mutates bot config — enabling an ACP agent, renaming,
  // switching model — it invalidates ['bots'] / ['bot', <id>]; mirror that into a
  // store re-pull so the chat window's agent menu updates without a manual
  // reload. (Web does this via its route watcher when leaving the settings
  // overlay; desktop's chat window route never enters settings.)
  const chatStore = useChatStore(pinia)
  window.api.desktop.onInvalidate((payload) => {
    const key = payload?.filters?.key
    const head = Array.isArray(key) ? key[0] : undefined
    if (head === 'bots' || head === 'bot') {
      void chatStore.refreshBots()
    }
  })

  app.mount('#app')
}

void bootstrap()
