<template>
  <div
    class="h-dvh w-screen overflow-hidden transition-all duration-[450ms] ease-out"
    :class="entering ? 'scale-[1.15] opacity-0' : 'scale-100 opacity-100'"
  >
    <!-- PUSH/PULL. The sidebar (a flex sibling) slides out left and the dock,
         flex-1, grows to fill the freed space — content shifts rather than being
         covered. dockview relays out per frame (no gating) to keep panels matched
         the whole way. -->
    <div class="flex h-full min-h-0 overflow-hidden">
      <SideBar :mac-traffic-reserve="macTrafficReserve" />
      <div class="flex min-w-0 min-h-0 flex-1 flex-col">
        <MainContainer />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { DesktopShellKey } from '@/lib/desktop-shell'
import SideBar from '@/components/sidebar/index.vue'
import MainContainer from '@/components/main-container/index.vue'
import { ONBOARDING_KEYS } from '@/pages/onboarding/constants'
import { safeSessionGet, safeSessionRemove } from '@/utils/safe-storage'
import { useKeyboardCommand } from '@/composables/useKeyboardCommand'
import { appKeyboardCommands } from '@/lib/keyboard-commands'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'

const desktopShell = inject(DesktopShellKey, false)
const macTrafficReserve = computed(() =>
  desktopShell
  && typeof navigator !== 'undefined'
  && navigator.platform.toLowerCase().includes('mac'),
)

const shouldAnimateEntry = safeSessionGet(ONBOARDING_KEYS.entryAnimation) === '1'
if (shouldAnimateEntry) {
  safeSessionRemove(ONBOARDING_KEYS.entryAnimation)
}

const entering = ref(shouldAnimateEntry)

onMounted(() => {
  if (!shouldAnimateEntry) return
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      entering.value = false
    })
  })
})

// toggleSidebar is registered here (not in main-layout, which is the settings
// overlay) because MainSection owns the chat shell's visible sidebar +
// workbench pane. MainSection stays mounted behind the settings overlay too,
// so the handler is always available; we no-op on settings routes to preserve
// the desktop settings sidebar's pinned-open intent AND prevent the web
// browser from falling through to its native Mod+B (bookmarks bar).
const workspaceTabs = useWorkspaceTabsStore()
const route = useRoute()
useKeyboardCommand(appKeyboardCommands.toggleSidebar, () => {
  if (route.path.startsWith('/settings')) return true
  workspaceTabs.toggleWorkbench()
  return true
})
</script>
