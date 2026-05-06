<template>
  <div class="flex flex-col flex-1 h-full min-w-0 bg-card">
    <WorkspaceTabBar />

    <div class="flex-1 min-h-0 relative">
      <template v-if="activeTab">
        <ChatPane
          v-if="activeTab.type === 'chat' || activeTab.type === 'draft'"
          key="chat-pane"
        />
        <FilePane
          v-else-if="activeTab.type === 'file'"
          :key="`file-pane:${activeTab.id}`"
          :tab-id="activeTab.id"
          :file-path="activeTab.filePath"
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
        v-if="displayTab && currentBotId"
        v-show="activeTab?.type === 'display'"
        :key="`display-pane:${displayTab.id}:${currentBotId}`"
        :bot-id="currentBotId"
        :class="{ 'pointer-events-none': activeTab?.type !== 'display' }"
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
import WorkspaceTabBar from './workspace-tab-bar.vue'
import ChatPane from './chat-pane.vue'
import FilePane from './file-pane.vue'
import TerminalPane from './terminal-pane.vue'
import DisplayPane from './display-pane.vue'

const { t } = useI18n()
const store = useWorkspaceTabsStore()
const { activeTab, tabs } = storeToRefs(store)
const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)

type TerminalTab = Extract<WorkspaceTab, { type: 'terminal' }>

const terminalTabs = computed<TerminalTab[]>(() =>
  tabs.value.filter((tab): tab is TerminalTab => tab.type === 'terminal'),
)

// The display tab is unique per bot, so we only need to detect its presence.
// Keeping it mounted while present preserves the WebRTC PeerConnection across
// tab focus changes; closing the tab unmounts the pane and tears WebRTC down.
const displayTab = computed(() => tabs.value.find((tab) => tab.type === 'display') ?? null)
</script>
