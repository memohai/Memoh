<template>
  <div class="relative h-full w-full">
    <DisplayPane
      v-if="currentBotId"
      :bot-id="currentBotId"
      :tab-id="props.params.api.id"
      :title="props.params.api.title ?? ''"
      :active="visible"
      @close="props.params.api.close()"
      @snapshot="handleSnapshot"
    />
  </div>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import type { DockviewApi, DockviewPanelApi } from 'dockview-vue'
import { useChatStore } from '@/store/chat-list'
import { useDisplaySnapshotsStore } from '@/store/display-snapshots'
import DisplayPane from '../display-pane.vue'
import { usePanelVisible } from './use-panel-visible'

// No KeepAlive/v-if: the WebRTC video element must stay attached. The panel
// is added with renderer 'always'.
const props = defineProps<{
  params: {
    params: Record<string, unknown>
    api: DockviewPanelApi
    containerApi: DockviewApi
  }
}>()

const chatStore = useChatStore()
const displaySnapshots = useDisplaySnapshotsStore()
const { currentBotId } = storeToRefs(chatStore)

const visible = usePanelVisible(props.params.api)

function handleSnapshot(payload: { tabId: string; sessionId?: string; dataUrl: string }) {
  const botId = currentBotId.value
  if (!botId) return
  displaySnapshots.upsert(botId, payload)
}
</script>
