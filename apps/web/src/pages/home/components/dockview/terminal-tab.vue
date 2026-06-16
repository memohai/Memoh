<template>
  <div
    class="memoh-terminal-tab group/tab flex min-h-[1.6875rem] min-w-0 cursor-default items-center gap-1 mx-1 mb-px pl-1 pr-1 rounded-sm text-[0.84375rem] font-[350] tracking-normal select-none [-webkit-font-smoothing:auto] transition-colors"
    :class="isSelected
      ? 'bg-sidebar-accent text-foreground'
      : 'text-foreground/80 hover:bg-[color:var(--sidebar-hover)]'"
    @auxclick.middle.prevent="close"
  >
    <SquareTerminal class="size-3.5 shrink-0 opacity-70" />
    <span class="min-w-0 max-w-28 truncate">{{ title }}</span>
    <span
      class="ml-0.5 flex size-4 shrink-0 items-center justify-center rounded opacity-0 transition-opacity group-hover/tab:opacity-100 hover:bg-[color-mix(in_oklab,var(--foreground)_12%,transparent)] hover:text-foreground"
      :aria-label="t('chat.tabMenu.close')"
      @click.stop.prevent="close"
    >
      <X class="size-3" />
    </span>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { SquareTerminal, X } from 'lucide-vue-next'
import type { DockviewApi, DockviewPanelApi } from 'dockview-vue'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'

const props = defineProps<{
  params: {
    api: DockviewPanelApi
    containerApi: DockviewApi
    params: Record<string, unknown>
  }
}>()

const { t } = useI18n()
const workspaceTabs = useWorkspaceTabsStore()

const panelId = props.params.api.id
const title = ref(props.params.api.title ?? '')
// Group-local selection: api.isActive tracks the globally focused panel, so when
// focus moves into the xterm the tab would lose its accent while output still
// shows. Match the group's activePanel instead.
const isSelected = ref(false)

function syncSelected() {
  const panel = props.params.containerApi.getPanel(panelId)
  isSelected.value = panel?.group.activePanel?.id === panelId
}

const disposables = [
  props.params.api.onDidTitleChange((event) => {
    title.value = event.title
  }),
]

onMounted(() => {
  title.value = props.params.api.title ?? title.value
  syncSelected()
  const panel = props.params.containerApi.getPanel(panelId)
  if (panel) {
    disposables.push(panel.group.api.onDidActivePanelChange(() => {
      syncSelected()
    }))
  }
})

function close() {
  workspaceTabs.requestCloseTab(panelId)
}

onBeforeUnmount(() => {
  for (const d of disposables) d.dispose()
})
</script>
