<template>
  <div
    class="h-dvh w-screen overflow-hidden transition-all duration-[450ms] ease-out"
    :class="[
      entering ? 'scale-[1.15] opacity-0' : 'scale-100 opacity-100',
      workbenchOpen && 'workspace-workbench-open',
      !workbenchOpen && 'workspace-workbench-collapsed',
      !workbenchOpen && macTrafficReserve && 'workspace-workbench-collapsed-mac',
    ]"
  >
    <div class="flex h-full min-h-0">
      <SideBar
        v-if="workbenchOpen"
        :mac-traffic-reserve="macTrafficReserve"
      />
      <div class="relative flex min-w-0 flex-1">
        <div
          class="absolute top-0 z-20 flex h-9 items-center gap-0.5"
          :class="chromeControlsOffsetClass"
        >
          <Button
            variant="ghost"
            size="icon-sm"
            class="text-muted-foreground hover:text-foreground"
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
            class="text-muted-foreground hover:text-foreground"
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
            class="text-muted-foreground hover:text-foreground"
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
const chromeControlsOffsetClass = computed(() =>
  !workbenchOpen.value && macTrafficReserve.value ? 'left-[84px]' : 'left-2',
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
</script>
