<template>
  <!-- Stable pane/navigation buttons are owned by the workspace shell so they survive
       empty dock layouts. This dockview slot only reserves the same strip space
       before the spatially top-left top-header tab strip. Dockview's groups array is
       creation-ordered, not a reliable layout position. -->
  <div
    v-if="reservesShellChrome"
    class="pointer-events-none h-full shrink-0 transition-[width] duration-300 ease-[cubic-bezier(0.32,0.72,0,1)]"
    :class="shouldReserveTrafficLight ? 'w-[11.5rem]' : 'w-[6.875rem]'"
    aria-hidden="true"
  />
  <!-- Non-first group: empty (zero-width) -->
  <div v-else />
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
import type { DockviewApi, DockviewGroupPanelApi, IDockviewGroupPanel } from 'dockview-vue'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { DesktopShellKey } from '@/lib/desktop-shell'
import { isWorkspaceTopLeftGroup } from './chrome-reserve'

const props = defineProps<{
  params: {
    api: DockviewGroupPanelApi
    containerApi: DockviewApi
    group: IDockviewGroupPanel
  }
}>()

const workspaceTabs = useWorkspaceTabsStore()
const { workbenchOpen } = storeToRefs(workspaceTabs)

function isTerminalOnlyGroup(group: { panels: Array<{ id: string }> }): boolean {
  const panels = group.panels
  return panels.length > 0 && panels.every(panel => panel.id.startsWith('terminal:'))
}

function currentGroupShouldReserveByDefault(): boolean {
  if (isTerminalOnlyGroup(props.params.group)) return false
  return isWorkspaceTopLeftGroup(props.params.containerApi, props.params.group.id)
}

const reservesShellChrome = ref(currentGroupShouldReserveByDefault())
let refreshFrame = 0

function refreshShellChromeReserve() {
  refreshFrame = 0
  if (isTerminalOnlyGroup(props.params.group)) {
    reservesShellChrome.value = false
    return
  }
  reservesShellChrome.value = isWorkspaceTopLeftGroup(
    props.params.containerApi,
    props.params.group.id,
  )
}

function scheduleRefresh() {
  if (refreshFrame || typeof window === 'undefined') return
  refreshFrame = window.requestAnimationFrame(refreshShellChromeReserve)
}

const disposables = [
  props.params.containerApi.onDidAddGroup(() => scheduleRefresh()),
  props.params.containerApi.onDidRemoveGroup(() => scheduleRefresh()),
  props.params.containerApi.onDidAddPanel(() => scheduleRefresh()),
  props.params.containerApi.onDidRemovePanel(() => scheduleRefresh()),
  props.params.containerApi.onDidMovePanel(() => scheduleRefresh()),
  props.params.containerApi.onDidLayoutChange(() => scheduleRefresh()),
  props.params.containerApi.onDidLayoutFromJSON(() => scheduleRefresh()),
]

onMounted(() => scheduleRefresh())

onBeforeUnmount(() => {
  if (refreshFrame && typeof window !== 'undefined') {
    window.cancelAnimationFrame(refreshFrame)
  }
  for (const d of disposables) d.dispose()
})

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
