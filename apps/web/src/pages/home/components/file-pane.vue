<template>
  <div class="flex flex-col h-full min-w-0 overflow-hidden">
    <FileViewer
      v-if="botId"
      :bot-id="botId"
      :file="fileInfo"
      @update:dirty="handleDirty"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount } from 'vue'
import { storeToRefs } from 'pinia'
import type { HandlersFsFileInfo } from '@memohai/sdk'
import FileViewer from '@/components/file-manager/file-viewer.vue'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'

const props = defineProps<{
  filePath: string
  tabId: string
}>()

const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)
const workspaceTabs = useWorkspaceTabsStore()

const botId = computed(() => currentBotId.value ?? '')

const fileInfo = computed<HandlersFsFileInfo>(() => {
  const path = props.filePath
  const idx = path.lastIndexOf('/')
  const name = idx >= 0 ? path.slice(idx + 1) : path
  return {
    path,
    name,
    isDir: false,
  } as HandlersFsFileInfo
})

function handleDirty(dirty: boolean) {
  workspaceTabs.setFileDirty(props.tabId, dirty)
}

onBeforeUnmount(() => {
  workspaceTabs.setFileDirty(props.tabId, false)
})
</script>
