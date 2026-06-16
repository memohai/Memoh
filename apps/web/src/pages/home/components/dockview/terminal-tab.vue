<template>
  <div
    class="memoh-terminal-tab group/tab flex h-[1.6875rem] w-full min-w-0 cursor-default items-center gap-2 pl-2.5 pr-1.5 rounded-sm text-[0.84375rem] font-[350] tracking-normal select-none [-webkit-font-smoothing:auto] transition-colors"
    :class="isSelected
      ? 'bg-sidebar-accent text-foreground'
      : 'text-foreground/80 hover:bg-[color:var(--sidebar-hover)]'"
    @auxclick.middle.prevent="close"
  >
    <SquareTerminal class="size-3.5 shrink-0 opacity-70" />
    <span class="min-w-0 flex-1 truncate">{{ title }}</span>
    <span
      class="-mr-0.5 flex size-4 shrink-0 items-center justify-center rounded text-muted-foreground transition-[opacity,background-color,color] duration-150 ease-out hover:bg-[color-mix(in_oklab,var(--foreground)_12%,transparent)] hover:text-foreground focus-visible:opacity-100"
      :class="isSelected ? 'opacity-100' : 'opacity-0 group-hover/tab:opacity-100'"
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
  // Recompute on EVERY global active-panel change (the container event the store
  // itself relies on, so it always fires). Comparing against the group's local
  // activePanel keeps exactly one terminal tab lit and preserves that highlight
  // when focus moves up into the chat group. The group-level event was unreliable
  // here, leaving stale tabs stuck "selected" (double-lit chips).
  props.params.containerApi.onDidActivePanelChange(() => {
    syncSelected()
  }),
]

onMounted(() => {
  title.value = props.params.api.title ?? title.value
  syncSelected()
})

function close() {
  workspaceTabs.requestCloseTab(panelId)
}

onBeforeUnmount(() => {
  for (const d of disposables) d.dispose()
})
</script>
