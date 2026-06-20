<template>
  <div class="flex flex-col h-full w-full">
    <div class="flex-1 min-h-0">
      <KeepAlive>
        <ChatPane
          v-if="visible"
          :key="`chat-pane:${currentBotId}:${panelId}`"
          :tab-id="panelId"
          :session-id="paramsSessionId"
          :active="visible"
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
import ChatPane from '../chat-pane.vue'
import { usePanelVisible } from './use-panel-visible'

// A per-session chat tab. The panel id (chat:<n>) is stable for the tab's whole
// life; the session it renders lives in params.sessionId (null = unsaved draft)
// and the workspace store mutates it (draft→real promotion) via updateParameters
// WITHOUT remounting ChatPane — the keep-alive key is the panel id, not the
// session. The single global messages array means only the ACTIVE chat tab is
// live; activating a tab selects its session (see workspace-tabs onDidActivePanelChange).
// No breadcrumb: the tab already carries the session title.
const props = defineProps<{
  params: {
    params: { sessionId?: string | null }
    api: DockviewPanelApi
    containerApi: DockviewApi
  }
}>()

const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)

const visible = usePanelVisible(props.params.api)
const panelId = props.params.api.id
const paramsSessionId = computed(() => props.params.params.sessionId ?? null)
</script>
