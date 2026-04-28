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
        <TerminalPane
          v-else-if="activeTab.type === 'terminal' && currentBotId"
          :key="`terminal-pane:${activeTab.id}`"
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
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { useChatStore } from '@/store/chat-list'
import WorkspaceTabBar from './workspace-tab-bar.vue'
import ChatPane from './chat-pane.vue'
import FilePane from './file-pane.vue'
import TerminalPane from './terminal-pane.vue'

const { t } = useI18n()
const store = useWorkspaceTabsStore()
const { activeTab } = storeToRefs(store)
const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)
</script>
