<template>
  <div class="flex flex-col flex-1 h-full min-w-0 bg-card">
    <WorkspaceTabBar />

    <div class="flex-1 min-h-0 relative">
      <template v-if="currentChat&&activeTab">
        <KeepAlive>
          <component
            :is="ChatPane"
            :key="`chat-pane:${currentBotId}:${currentChat.id}`"
            :tab-id="currentChat.id"
            :active="activeTab.id === currentChat.id"
          />
        </KeepAlive>
      
        <FilePane
          v-for="tab in fileTabs"
          v-show="activeTab.id === tab.id"
          :key="`file-pane:${currentBotId}:${tab.id}`"
          :tab-id="tab.id"
          :file-path="tab.filePath"
        />
        <template v-if="currentBotId">
          <TerminalPane
            v-for="tab in terminalTabs"
            v-show="activeTab.id === tab.id"
            :key="`terminal-pane:${currentBotId}:${tab.id}`"
            :bot-id="currentBotId"
            :tab-id="tab.id"
            :active="activeTab.id === tab.id"
          />
        </template>
      </template>
      <div
        v-if="!activeTab"
        class="absolute inset-0 flex items-center justify-center"
      >
        <div class="text-center px-6">
          <p class="text-xs font-medium text-foreground">
            {{ t('chat.emptyWorkspace') }}
          </p>
          <p class="mt-1 text-xs text-muted-foreground">
            {{ t('chat.emptyWorkspaceHint') }}
          </p>
        </div>
      </div>

      <!--
        Display pane is intentionally kept mounted while the display tab exists,
        even when another tab is focused. This preserves the WebRTC connection
        and avoids a black-frame reconnect when the user comes back to it.
        Visibility is toggled via v-show; pointer-events are disabled while
        hidden so the offscreen video does not steal focus or events.
      -->
      <DisplayPane
        v-for="tab in displayTabs"
        v-show="activeTab?.id === tab.id"
        :key="`display-pane:${tab.id}:${currentBotId}`"
        :bot-id="currentBotId || ''"
        :tab-id="tab.id"
        :title="tab.title"
        :active="activeTab?.id === tab.id"
        :class="{ 'pointer-events-none': activeTab?.id !== tab.id }"
        @close="store.closeTab(tab.id)"
        @snapshot="handleDisplaySnapshot"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useWorkspaceTabsStore, type WorkspaceTab } from '@/store/workspace-tabs'
import { useChatStore } from '@/store/chat-list'
import { useDisplaySnapshotsStore } from '@/store/display-snapshots'
import WorkspaceTabBar from './workspace-tab-bar.vue'
import ChatPane from './chat-pane.vue'
import FilePane from './file-pane.vue'
import TerminalPane from './terminal-pane.vue'
import DisplayPane from './display-pane.vue'



const { t } = useI18n()
const store = useWorkspaceTabsStore()
const displaySnapshots = useDisplaySnapshotsStore()
const { activeTab, tabs } = storeToRefs(store)
const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)

/*
     <!-- <ChatPane
            v-for="tab in chatTabs"
            v-show="activeTab.id === tab.id"
            :key="`chat-pane:${currentBotId}:${tab.id}`"
            :tab-id="tab.id"
            :active="activeTab.id === tab.id"
          /> -->
*/ 


type TerminalTab = Extract<WorkspaceTab, { type: 'terminal' }>
type DisplayTab = Extract<WorkspaceTab, { type: 'display' }>
type ChatTab = Extract<WorkspaceTab, { type: 'chat' | 'draft' }>
type FileTab = Extract<WorkspaceTab, { type: 'file' }>

const chatTabs = computed<ChatTab[]>(() =>
  tabs.value.filter((tab): tab is ChatTab => tab.type === 'chat' || tab.type === 'draft'),
)

const currentChat = computed(() => {

  if (!activeTab.value?.id) return
  return chatTabs.value.find(v=>v.id === activeTab.value?.id)
})


const fileTabs = computed<FileTab[]>(() =>
  tabs.value.filter((tab): tab is FileTab => tab.type === 'file'),
)

const terminalTabs = computed<TerminalTab[]>(() =>
  tabs.value.filter((tab): tab is TerminalTab => tab.type === 'terminal'),
)

const displayTabs = computed<DisplayTab[]>(() =>
  currentBotId.value
    ? tabs.value.filter((tab): tab is DisplayTab => tab.type === 'display')
    : [],
)

function handleDisplaySnapshot(payload: { tabId: string; sessionId?: string; dataUrl: string }) {
  const botId = currentBotId.value
  if (!botId) return
  displaySnapshots.upsert(botId, payload)
}
</script>
