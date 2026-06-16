<template>
  <TerminalTab
    v-if="useTerminalChip"
    :params="params"
  />
  <WorkspaceTab
    v-else
    :params="params"
  />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import type { DockviewApi, DockviewPanelApi } from 'dockview-vue'
import TerminalTab from './terminal-tab.vue'
import WorkspaceTab from './workspace-tab.vue'

const props = defineProps<{
  params: {
    api: DockviewPanelApi
    containerApi: DockviewApi
    params: Record<string, unknown>
  }
}>()

const panelId = props.params.api.id
const isTerminalPanel = panelId.startsWith('terminal:')

// The file-chip tab style belongs to the terminal-ONLY group (the bottom-bar
// panel), NOT to terminal panels as such. A terminal dragged into a mixed/editor
// group must blend in as a normal dock tab, so the choice tracks the panel's
// CURRENT group composition and is re-evaluated on every layout change (moves,
// adds, removes) — not the panel type alone.
const useTerminalChip = ref(false)

function evaluate() {
  if (!isTerminalPanel) {
    useTerminalChip.value = false
    return
  }
  const group = props.params.containerApi.getPanel(panelId)?.group
  useTerminalChip.value = !!group
    && group.panels.length > 0
    && group.panels.every(panel => panel.id.startsWith('terminal:'))
}

const disposable = props.params.containerApi.onDidLayoutChange(() => evaluate())

onMounted(evaluate)
onBeforeUnmount(() => disposable.dispose())
</script>
