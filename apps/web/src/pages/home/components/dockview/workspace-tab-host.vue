<template>
  <TerminalTab
    v-if="isTerminal"
    :params="params"
  />
  <WorkspaceTab
    v-else
    :params="params"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
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

const isTerminal = computed(() => props.params.api.id.startsWith('terminal:'))
</script>
