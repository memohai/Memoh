<template>
  <!-- Right actions hug their content at the strip's far right (the void grows to
       fill — and accept drops — between the "+" cluster and here). Only the
       "Open Preview to the Side" action lives here, and only when this group's
       active tab is a markdown/html file (VS Code's editor-title preview action).
       The global right pane toggle is owned by the workspace shell; only the
       spatially top-right top-header group reserves space for it. -->
  <div
    class="flex h-full items-center"
    :class="reservesShellChrome ? 'pr-11' : 'pr-2'"
  >
    <Button
      v-if="previewPath"
      variant="ghost"
      size="icon-sm"
      class="size-7 shrink-0 rounded-full text-muted-foreground hover:text-foreground"
      :title="t('chat.openPreviewToSide')"
      :aria-label="t('chat.openPreviewToSide')"
      @click="openPreviewToSide"
    >
      <Columns2 class="size-3.5" />
    </Button>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Columns2 } from 'lucide-vue-next'
import { Button } from '@memohai/ui'
import type { DockviewApi, DockviewGroupPanelApi, IDockviewGroupPanel } from 'dockview-vue'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import { isHtmlFile, isMarkdownFile } from '@/components/file-manager/utils'
import { hasBotPermission } from '@/utils/bot-permissions'
import { isWorkspaceTopRightGroup } from './chrome-reserve'

const props = defineProps<{
  params: {
    api: DockviewGroupPanelApi
    containerApi: DockviewApi
    group: IDockviewGroupPanel
  }
}>()

const { t } = useI18n()
const store = useWorkspaceTabsStore()
const chatStore = useChatStore()
const { currentBotId, bots } = storeToRefs(chatStore)

const currentBot = computed(() =>
  bots.value.find(bot => bot.id === currentBotId.value) ?? null,
)
const canWorkspaceRead = computed(() =>
  hasBotPermission(currentBot.value?.current_user_permissions ?? [], 'workspace_read'),
)
function isTerminalOnlyGroup(group: { panels: Array<{ id: string }> }): boolean {
  const panels = group.panels
  return panels.length > 0 && panels.every(panel => panel.id.startsWith('terminal:'))
}

const reservesShellChrome = ref(false)
let refreshFrame = 0

function currentGroupShouldReserve(): boolean {
  return !isTerminalOnlyGroup(props.params.group)
    && isWorkspaceTopRightGroup(props.params.containerApi, props.params.group.id)
}

function refreshShellChromeReserve() {
  refreshFrame = 0
  reservesShellChrome.value = currentGroupShouldReserve()
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

// Track THIS group's active tab (not the globally active panel) so the preview
// action only appears in the header of the group currently showing a
// previewable file — correct for split layouts.
const activePanelId = ref<string | null>(props.params.group.activePanel?.id ?? null)
const activePanelSub = props.params.api.onDidActivePanelChange(() => {
  activePanelId.value = props.params.group.activePanel?.id ?? null
})
onMounted(() => {
  refreshShellChromeReserve()
  scheduleRefresh()
})

onBeforeUnmount(() => {
  activePanelSub.dispose()
  if (refreshFrame && typeof window !== 'undefined') {
    window.cancelAnimationFrame(refreshFrame)
  }
  for (const d of disposables) d.dispose()
})

const previewPath = computed(() => {
  if (!canWorkspaceRead.value) return null
  const id = activePanelId.value
  if (!id || !id.startsWith('file:')) return null
  const path = id.slice('file:'.length)
  const name = path.slice(path.lastIndexOf('/') + 1)
  return isMarkdownFile(name) || isHtmlFile(name) ? path : null
})

function openPreviewToSide() {
  const path = previewPath.value
  if (!path) return
  const name = path.slice(path.lastIndexOf('/') + 1)
  store.openPreview(path, t('chat.previewTab', { name }), props.params.group.id)
}
</script>
