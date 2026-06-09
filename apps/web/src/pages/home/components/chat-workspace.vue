<template>
  <div class="flex flex-col flex-1 h-full min-w-0 bg-card">
    <WorkspaceTabBar />

    <div class="flex-1 min-h-0 relative">
      <template v-if="activeTab">
        <KeepAlive>
          <component
            :is="currentChat?.component"
            v-if="activeTab.type === 'chat' || activeTab.type === 'draft'"
            :key="`chat-pane:${currentBotId}:${currentChat?.id}`"
            :tab-id="currentChat?.id"
            :active="activeTab.id === currentChat?.id"
          />
          <component
            :is="currentFile?.component"
            v-else-if="activeTab.type==='file'&&currentFile?.type==='file'"
            :key="`file-pane:${currentBotId}:${currentFile.id}`"
            :tab-id="currentFile.id"
            :file-path="currentFile.filePath"
          />

          <component
            :is="currentTerminal?.component"
            v-else-if="activeTab.type==='terminal'"
            :key="`terminal-pane:${currentBotId}:${currentTerminal?.id}`"
            :bot-id="currentBotId"
            :tab-id="currentTerminal?.id"
            :active="activeTab.id === currentTerminal?.id"  
          />
        </KeepAlive>
      </template>
      <DisplayPane
        v-for="display in displayTabs"
        v-show="activeTab?.id === display.id"
        :key="`display-pane:${display.id}:${currentBotId}`"
        :bot-id="currentBotId || ''"
        :tab-id="display.id"
        :title="display.title"
        :active="activeTab?.id === display.id"
        @close="store.closeTab(display.id)"
        @snapshot="handleDisplaySnapshot"
      />
      <BrowserPane
        v-for="browser in browserTabs"
        v-show="activeTab?.id === browser.id"
        :key="`browser-pane:${browser.id}:${currentBotId}`"
        :bot-id="currentBotId || ''"
        :tab-id="browser.id"
        :address="browser.address"
        :active="activeTab?.id === browser.id"
      />
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
import BrowserPane from './browser-pane.vue'
import { type ComputedRef } from 'vue'

const { t } = useI18n()
const store = useWorkspaceTabsStore()
const displaySnapshots = useDisplaySnapshotsStore()
const { activeTab, tabs } = storeToRefs(store)
const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)


type TerminalTab = Extract<WorkspaceTab, { type: 'terminal' }>
type ChatTab = Extract<WorkspaceTab, { type: 'chat' | 'draft' }>
type FileTab = Extract<WorkspaceTab, { type: 'file' }>
type BrowserTab = Extract<WorkspaceTab, { type: 'browser' }>
type DisplayTab = Extract<WorkspaceTab, { type: 'display' }>

const chatTabs = computed<ChatTab[]>(() =>
  tabs.value.filter((tab): tab is ChatTab => tab.type === 'chat' || tab.type === 'draft'),
)

function TypeTab<T extends (TerminalTab | ChatTab | FileTab)[]>(tabComp: ComputedRef<T>) {
  const componentMap = {
    chat: ChatPane,
    draft: ChatPane,
    file: FilePane,
    terminal: TerminalPane,
  }
  return computed(() => {
    if (!activeTab.value?.id) return
    const currentTab = tabComp.value.find(v => v.id === activeTab.value?.id)
    if (!currentTab) {
      return
    }
    return { ...currentTab, component: componentMap[activeTab.value['type'] as keyof typeof componentMap] }
  })
}

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
const browserTabs = computed<BrowserTab[]>(() =>
  currentBotId.value
    ? tabs.value.filter((tab): tab is BrowserTab => tab.type === 'browser')
    : [],
)
const currentFile = TypeTab(fileTabs)
const currentChat = TypeTab(chatTabs)
const currentTerminal=TypeTab(terminalTabs)



function handleDisplaySnapshot(payload: { tabId: string; sessionId?: string; dataUrl: string }) {
  const botId = currentBotId.value
  if (!botId) return
  displaySnapshots.upsert(botId, payload)
}
</script>
