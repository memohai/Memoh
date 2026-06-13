<template>
  <div class="flex flex-col relative h-full w-full">
    <div class="flex-1 min-h-0">
      <KeepAlive>
        <TerminalPane
          v-if="visible && currentBotId"
          :key="`terminal-pane:${currentBotId}:${props.params.api.id}`"
          :bot-id="currentBotId"
          :tab-id="props.params.api.id"
          :active="visible"
        />
      </KeepAlive>
    </div>
  </div>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import type { DockviewApi, DockviewPanelApi } from 'dockview-vue'
import { useChatStore } from '@/store/chat-list'
import TerminalPane from '../terminal-pane.vue'
import { usePanelVisible } from './use-panel-visible'

const props = defineProps<{
  params: {
    params: Record<string, unknown>
    api: DockviewPanelApi
    containerApi: DockviewApi
  }
}>()

const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)

const visible = usePanelVisible(props.params.api)
</script>
