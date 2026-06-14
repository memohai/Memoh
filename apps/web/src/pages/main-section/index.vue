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
        <!-- Global workspace chrome: a slim strip ABOVE all editor groups (not
             overlaid on one group), so a vertical split's second group no longer
             inherits a reserved chrome width — it simply has no chrome of its own.
             On desktop the strip doubles as the window drag region; the buttons
             opt out. When the rail is closed on macOS the dock owns the top-left,
             so the strip clears the traffic lights. -->
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
        </div>
        <MainContainer />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import { ChevronLeft, ChevronRight, PanelLeftClose, PanelLeftOpen } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import SideBar from '@/components/sidebar/index.vue'
import MainContainer from '@/components/main-container/index.vue'
import { ONBOARDING_KEYS } from '@/pages/onboarding/constants'
import { safeSessionGet, safeSessionRemove } from '@/utils/safe-storage'
import { DesktopShellKey } from '@/lib/desktop-shell'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'

const router = useRouter()
const desktopShell = inject(DesktopShellKey, false)
const macTrafficReserve = computed(() =>
  desktopShell
  && typeof navigator !== 'undefined'
  && navigator.platform.toLowerCase().includes('mac'),
)
const workspaceTabs = useWorkspaceTabsStore()
const { workbenchOpen } = storeToRefs(workspaceTabs)

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
</script>
