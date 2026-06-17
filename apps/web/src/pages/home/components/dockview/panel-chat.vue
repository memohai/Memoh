<template>
  <div class="flex flex-col h-full w-full">
    <div class="flex-1 min-h-0">
      <KeepAlive>
        <ChatPane
          v-if="visible"
          :key="`chat-pane:${currentBotId}:${chatKey}`"
          :session-id="mySessionId"
          :tab-id="chatTabId"
          :active="visible"
        />
      </KeepAlive>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, watch } from 'vue'
import { storeToRefs } from 'pinia'
import type { DockviewApi, DockviewPanelApi } from 'dockview-vue'
import { useChatStore } from '@/store/chat-list'
import ChatPane from '../chat-pane.vue'
import { usePanelVisible } from './use-panel-visible'

// Two flavours of chat panel share this host, distinguished by dockview panel id:
// - the primary tab (`chat`, plus `chat~` split duplicates) follows the global
//   active session, so single-click in the sidebar swaps it in place.
// - a pinned tab (`chat:<sessionId>`) is bound to one fixed session and renders
//   it independently, which is what lets two chats stream side by side.
const props = defineProps<{
  params: {
    params: Record<string, unknown>
    api: DockviewPanelApi
    containerApi: DockviewApi
  }
}>()

const chatStore = useChatStore()
const { currentBotId, sessionId } = storeToRefs(chatStore)

const panelId = props.params.api.id
const pinnedPrefix = 'chat:'
const pinnedSessionId = panelId.startsWith(pinnedPrefix) ? panelId.slice(pinnedPrefix.length) : null

const visible = usePanelVisible(props.params.api)

// Primary tab follows the active session (incl. the null draft); a pinned tab is
// fixed to its bound session.
const mySessionId = computed(() => pinnedSessionId ?? sessionId.value ?? null)
// A pinned tab is a stable instance (key never changes), so it stays mounted and
// keeps streaming. The primary tab re-keys per active session, as before.
const chatKey = computed(() => pinnedSessionId ?? sessionId.value ?? 'draft')
const chatTabId = computed(() => {
  const sid = mySessionId.value
  return sid ? `chat:${sid}` : 'draft'
})

// While this panel is open, keep its bound session "observed" in the store so it
// receives live streams and inbound refreshes even when another tab is focused.
// The primary panel re-points as the active session changes; a pinned panel
// holds one session for its lifetime. This panel host stays mounted regardless
// of tab visibility, so observation tracks "tab open", not "tab focused".
watch(mySessionId, (sid, prev) => {
  if (prev) chatStore.unobserveSession(prev)
  if (sid) chatStore.observeSession(sid)
}, { immediate: true })
onUnmounted(() => {
  if (mySessionId.value) chatStore.unobserveSession(mySessionId.value)
})
</script>
