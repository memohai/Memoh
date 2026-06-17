<template>
  <div class="flex flex-col h-full w-full bg-surface-editor">
    <PanelBreadcrumb :path="filePath" />
    <div class="flex-1 min-h-0">
      <KeepAlive>
        <FilePane
          v-if="visible && filePath"
          :key="`file-pane:${currentBotId}:${filePath}`"
          :tab-id="props.params.api.id"
          :file-path="filePath"
        />
      </KeepAlive>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import type { DockviewApi, DockviewPanelApi } from 'dockview-vue'
import { useChatStore } from '@/store/chat-list'
import FilePane from '../file-pane.vue'
import { usePanelVisible } from './use-panel-visible'
import PanelBreadcrumb from './panel-breadcrumb.vue'

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
