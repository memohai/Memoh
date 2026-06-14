<template>
  <!-- border-r hands the tab strip's vertical divider language to the nav cluster:
       the first tab no longer floats edgeless against the buttons but is fenced off
       by the same SOFT inter-tab seam (--tab-divider, inherited from the dockview
       theme) that separates two unselected tabs — full --border stays reserved for
       framing the active tab. pr-2 keeps the line off the last button; the tab's
       own pl-4 keeps the title off the line. -->
  <div
    v-if="isFirstGroup"
    class="flex h-full items-center gap-0.5 border-r [border-color:var(--tab-divider)] pr-2 [-webkit-app-region:drag] transition-[padding] duration-300 ease-[cubic-bezier(0.32,0.72,0,1)]"
    :class="shouldReserveTrafficLight ? 'pl-[76px]' : 'pl-2'"
  >
    <Button
      variant="ghost"
      size="icon-sm"
      class="size-7 text-muted-foreground hover:text-foreground [-webkit-app-region:no-drag]"
      :title="workbenchOpen ? t('chat.topBar.hideWorkbench') : t('chat.topBar.showWorkbench')"
      :aria-label="workbenchOpen ? t('chat.topBar.hideWorkbench') : t('chat.topBar.showWorkbench')"
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
      :title="t('chat.topBar.goBack')"
      :aria-label="t('chat.topBar.goBack')"
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
      :title="t('chat.topBar.goForward')"
      :aria-label="t('chat.topBar.goForward')"
      @click="router.go(1)"
    >
      <ChevronRight
        :stroke-width="1.75"
        class="size-4"
      />
    </Button>
  </div>
  <!-- Non-first group: empty (zero-width) -->
  <div v-else />
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ChevronLeft, ChevronRight, PanelLeftClose, PanelLeftOpen } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import type { DockviewApi, DockviewGroupPanelApi, IDockviewGroupPanel } from 'dockview-vue'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { DesktopShellKey } from '@/lib/desktop-shell'

const props = defineProps<{
  params: {
    api: DockviewGroupPanelApi
    containerApi: DockviewApi
    group: IDockviewGroupPanel
  }
}>()

const { t } = useI18n()
const router = useRouter()
const workspaceTabs = useWorkspaceTabsStore()
const { workbenchOpen } = storeToRefs(workspaceTabs)

// Determine if this is the first (leftmost/topmost) group
const firstGroupId = ref(props.params.containerApi.groups[0]?.id ?? '')

function refreshFirstGroup() {
  firstGroupId.value = props.params.containerApi.groups[0]?.id ?? ''
}

const disposables = [
  props.params.containerApi.onDidAddGroup(() => refreshFirstGroup()),
  props.params.containerApi.onDidRemoveGroup(() => refreshFirstGroup()),
]

onBeforeUnmount(() => {
  for (const d of disposables) d.dispose()
})

const isFirstGroup = computed(() => props.params.group.id === firstGroupId.value)

// macOS traffic light reserve (only when sidebar is closed on desktop mac)
const desktopShell = inject(DesktopShellKey, false)
const macTrafficReserve = computed(() =>
  desktopShell
  && typeof navigator !== 'undefined'
  && navigator.platform.toLowerCase().includes('mac'),
)
const shouldReserveTrafficLight = computed(() =>
  macTrafficReserve.value && !workbenchOpen.value,
)
</script>
