<template>
  <div class="h-full w-full bg-surface-editor">
    <KeepAlive>
      <FilePane
        v-if="visible && filePath"
        :key="`file-pane:${currentBotId}:${filePath}`"
        :tab-id="props.params.api.id"
        :file-path="filePath"
      />
    </KeepAlive>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import type { DockviewApi, DockviewPanelApi } from 'dockview-vue'
import { useChatStore } from '@/store/chat-list'
import FilePane from '../file-pane.vue'
import { usePanelVisible } from './use-panel-visible'

const props = defineProps<{
  params: {
    params: { filePath?: string }
    api: DockviewPanelApi
    containerApi: DockviewApi
  }
}>()

const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)

const visible = usePanelVisible(props.params.api)
const filePath = computed(() => props.params.params.filePath ?? '')
</script>
