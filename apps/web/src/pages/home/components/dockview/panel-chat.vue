<template>
  <div class="h-full w-full bg-surface-editor">
    <KeepAlive>
      <ChatPane
        v-if="visible"
        :key="`chat-pane:${currentBotId}:${chatKey}`"
        :tab-id="chatTabId"
        :active="visible"
      />
    </KeepAlive>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import type { DockviewApi, DockviewPanelApi } from 'dockview-vue'
import { useChatStore } from '@/store/chat-list'
import ChatPane from '../chat-pane.vue'
import { usePanelVisible } from './use-panel-visible'

// The chat panel is a singleton whose content follows the active session
// (chat-selection store). Multi-session side-by-side rendering needs
// per-session message state in the chat store first.
const props = defineProps<{
  params: {
    params: Record<string, unknown>
    api: DockviewPanelApi
    containerApi: DockviewApi
  }
}>()

const chatStore = useChatStore()
const { currentBotId, sessionId } = storeToRefs(chatStore)

const visible = usePanelVisible(props.params.api)
const chatKey = computed(() => sessionId.value ?? 'draft')
const chatTabId = computed(() =>
  sessionId.value ? `chat:${sessionId.value}` : 'draft',
)
</script>
