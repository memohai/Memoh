<template>
  <div
    class="h-dvh w-screen overflow-hidden transition-all duration-[450ms] ease-out"
    :class="entering ? 'scale-[1.15] opacity-0' : 'scale-100 opacity-100'"
  >
    <!-- PUSH/PULL. The sidebar (a flex sibling) slides out left and the dock,
         flex-1, grows to fill the freed space — the content shifts rather than
         being covered. The dock's RIGHT edge is the viewport edge (fixed), so as
         its width animates the breadcrumb "+" stays pinned; dockview relays out
         per frame (no gating) to keep panels matched the whole way. -->
    <div class="flex h-full min-h-0 overflow-hidden">
      <SideBar :mac-traffic-reserve="macTrafficReserve" />
      <div
        class="relative flex min-w-0 flex-1"
        :style="{ '--ws-tabstrip-pad': `${tabReserve}px` }"
      >
        <!-- Global workspace chrome (collapse/expand + back/forward), overlaid on
             the dock tab strip. It's a child of the dock area, so it rides the
             dock's left edge for free as the sidebar pushes it; only the
             mac-closed traffic-light clearance shifts its `left` (transitioned).
             The dock reserves matching room via --ws-tabstrip-pad. -->
        <div
          class="absolute top-0 z-20 flex h-9 items-center gap-0.5 transition-[left] duration-300 ease-[cubic-bezier(0.32,0.72,0,1)]"
          :style="{ left: `${chromeOffset}px` }"
        >
          <Button
            variant="ghost"
            size="icon-sm"
            class="size-7 text-muted-foreground hover:text-foreground"
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
            class="size-7 text-muted-foreground hover:text-foreground"
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
            class="size-7 text-muted-foreground hover:text-foreground"
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

// Geometry for the chrome overlay (px; see --ws-chrome-* in dockview-theme.css):
//   CHROME_INSET    inset of the chrome buttons from the dock's left edge
//   CHROME_CONTROLS width the buttons block reserves in the tab strip
//   MAC_TRAFFIC     macOS traffic-light clearance (only needed when CLOSED — the
//                   dock then starts at x=0 under the lights; while open the
//                   lights sit over the sidebar, which clears them itself)
const CHROME_INSET = 8
const CHROME_CONTROLS = 96
const MAC_TRAFFIC = 76

// The chrome is a child of the dock area, so it rides the dock's left edge for
// free as the sidebar pushes it. Its own `left` only differs in the mac-closed
// case, where it must clear the traffic lights.
const chromeOffset = computed(() =>
  macTrafficReserve.value && !workbenchOpen.value
    ? MAC_TRAFFIC + CHROME_INSET
    : CHROME_INSET,
)
// Tab strip reserves room for the chrome from the dock's own left edge.
const tabReserve = computed(() => chromeOffset.value + CHROME_CONTROLS)

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
