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
        <VncPane
          v-if="activeTab.type === 'vnc' && currentBotId"
          :key="`vnc-pane:${activeTab.id}`"
          :bot-id="currentBotId"
        />
      </template>
      <div
        v-else
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
import VncPane from './vnc-pane.vue'

const { t } = useI18n()
const store = useWorkspaceTabsStore()
const { activeTab, tabs } = storeToRefs(store)
const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)

type TerminalTab = Extract<WorkspaceTab, { type: 'terminal' }>

const terminalTabs = computed<TerminalTab[]>(() =>
  tabs.value.filter((tab): tab is TerminalTab => tab.type === 'terminal'),
)
</script>
