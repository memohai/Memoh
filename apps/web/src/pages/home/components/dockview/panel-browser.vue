<template>
  <div class="relative h-full w-full">
    <BrowserPane
      v-if="currentBotId"
      :bot-id="currentBotId"
      :tab-id="props.params.api.id"
      :address="address"
      :active="visible"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import type { DockviewApi, DockviewPanelApi } from 'dockview-vue'
import { useChatStore } from '@/store/chat-list'
import BrowserPane from '../browser-pane.vue'
import { usePanelVisible } from './use-panel-visible'

// No KeepAlive/v-if: detaching the iframe would reload the page. The panel is
// added with renderer 'always' so dockview keeps the DOM mounted while hidden.
const props = defineProps<{
  params: {
    params: { address?: string }
    api: DockviewPanelApi
    containerApi: DockviewApi
  }
}>()

const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)

const visible = usePanelVisible(props.params.api)
const address = computed(() => props.params.params.address ?? '')
</script>
