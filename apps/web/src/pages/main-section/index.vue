<template>
  <div
    class="h-dvh w-screen overflow-hidden transition-all duration-[450ms] ease-out"
    :class="entering ? 'scale-[1.15] opacity-0' : 'scale-100 opacity-100'"
  >
    <!-- PUSH/PULL. Both sidebars are flex siblings of the middle column. The left
         one slides out left and the right branch history slides out right; the
         middle column grows to fill the freed space. -->
    <div class="flex h-full min-h-0 overflow-hidden">
      <SideBar :mac-traffic-reserve="macTrafficReserve" />
      <div class="flex min-w-0 min-h-0 flex-1 flex-col">
        <div
          class="flex h-9 shrink-0 items-center gap-0.5 bg-surface-chrome pr-2 transition-[padding] duration-300 ease-[cubic-bezier(0.32,0.72,0,1)] [-webkit-app-region:drag]"
          :class="macTrafficReserve && !workbenchOpen ? 'pl-[76px]' : 'pl-2'"
        >
          <Button
            variant="ghost"
            size="icon-sm"
            class="size-7 text-muted-foreground hover:text-foreground [-webkit-app-region:no-drag]"
            :title="workbenchOpen ? $t('chat.topBar.hideWorkbench') : $t('chat.topBar.showWorkbench')"
            :aria-label="workbenchOpen ? $t('chat.topBar.hideWorkbench') : $t('chat.topBar.showWorkbench')"
            :aria-pressed="workbenchOpen"
            @click="workspaceTabs.toggleWorkbench()"
          >
            <PanelLeftClose
              v-if="workbenchOpen"
              :stroke-width="1.75"
              class="size-4"
            />
            <PanelLeftOpen
              v-else
              :stroke-width="1.75"
              class="size-4"
            />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            class="size-7 text-muted-foreground hover:text-foreground [-webkit-app-region:no-drag]"
            :title="$t('chat.topBar.goBack')"
            :aria-label="$t('chat.topBar.goBack')"
            @click="router.go(-1)"
          >
            <ChevronLeft
              :stroke-width="1.75"
              class="size-4"
            />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            class="size-7 text-muted-foreground hover:text-foreground [-webkit-app-region:no-drag]"
            :title="$t('chat.topBar.goForward')"
            :aria-label="$t('chat.topBar.goForward')"
            @click="router.go(1)"
          >
            <ChevronRight
              :stroke-width="1.75"
              class="size-4"
            />
          </Button>
          <div class="flex-1" />
          <Button
            variant="ghost"
            size="icon-sm"
            class="size-7 text-muted-foreground hover:text-foreground [-webkit-app-region:no-drag]"
            :title="branchSidebarOpen ? $t('chat.topBar.hideBranchSidebar') : $t('chat.topBar.showBranchSidebar')"
            :aria-label="branchSidebarOpen ? $t('chat.topBar.hideBranchSidebar') : $t('chat.topBar.showBranchSidebar')"
            :aria-pressed="branchSidebarOpen"
            @click="workspaceTabs.toggleBranchSidebar()"
          >
            <PanelRightClose
              v-if="branchSidebarOpen"
              :stroke-width="1.75"
              class="size-4"
            />
            <PanelRightOpen
              v-else
              :stroke-width="1.75"
              class="size-4"
            />
          </Button>
        </div>
        <MainContainer />
      </div>
      <BranchHistorySidebar />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute, useRouter } from 'vue-router'
import { ChevronLeft, ChevronRight, PanelLeftClose, PanelLeftOpen, PanelRightClose, PanelRightOpen } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import { DesktopShellKey } from '@/lib/desktop-shell'
import SideBar from '@/components/sidebar/index.vue'
import MainContainer from '@/components/main-container/index.vue'
import BranchHistorySidebar from '@/pages/home/components/branch-history-sidebar.vue'
import { ONBOARDING_KEYS } from '@/pages/onboarding/constants'
import { safeSessionGet, safeSessionRemove } from '@/utils/safe-storage'
import { useKeyboardCommand } from '@/composables/useKeyboardCommand'
import { appKeyboardCommands } from '@/lib/keyboard-commands'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'

const router = useRouter()
const desktopShell = inject(DesktopShellKey, false)
const macTrafficReserve = computed(() =>
  desktopShell
  && typeof navigator !== 'undefined'
  && navigator.platform.toLowerCase().includes('mac'),
)
const workspaceTabs = useWorkspaceTabsStore()
const { workbenchOpen, branchSidebarOpen } = storeToRefs(workspaceTabs)

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
const route = useRoute()
useKeyboardCommand(appKeyboardCommands.toggleSidebar, () => {
  if (route.path.startsWith('/settings')) return true
  workspaceTabs.toggleWorkbench()
  return true
})
</script>
