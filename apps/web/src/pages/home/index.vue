<template>
  <div class="flex h-full overflow-hidden">
    <template v-if="currentBotId">
      <ChatSidebar ref="sidebarRef" />
      <ChatWorkspace />
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, provide, nextTick } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute, useRouter } from 'vue-router'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { openInFileManagerKey } from './composables/useFileManagerProvider'
import ChatSidebar from './components/chat-sidebar.vue'
import ChatWorkspace from './components/chat-workspace.vue'

const route = useRoute()
const router = useRouter()
const chatStore = useChatStore()
const workspaceTabs = useWorkspaceTabsStore()
const { currentBotId } = storeToRefs(chatStore)

const sidebarRef = ref<InstanceType<typeof ChatSidebar> | null>(null)

const FILE_MANAGER_ROOT = '/data'

function normalizeFileManagerPath(path: string): string {
  const trimmedPath = path.trim()
  if (!trimmedPath) return FILE_MANAGER_ROOT
  if (trimmedPath === FILE_MANAGER_ROOT || trimmedPath.startsWith(`${FILE_MANAGER_ROOT}/`)) {
    return trimmedPath
  }
  if (trimmedPath === '/') return FILE_MANAGER_ROOT
  if (trimmedPath.startsWith('/')) {
    return `${FILE_MANAGER_ROOT}${trimmedPath}`
  }
  return `${FILE_MANAGER_ROOT}/${trimmedPath}`
}

provide(openInFileManagerKey, (path: string, isDir = false) => {
  const normalizedPath = normalizeFileManagerPath(path)
  if (isDir) {
    void nextTick(() => sidebarRef.value?.openFilesAt(normalizedPath))
  } else {
    workspaceTabs.openFile(normalizedPath)
  }
})

const urlBotId = ((route.params.botId as string) ?? '').trim()

if (urlBotId) {
  void chatStore.selectBot(urlBotId)
}

let suppressUrlSync = false

watch(currentBotId, (newBotId) => {
  if (suppressUrlSync) return
  const urlBot = ((route.params.botId as string) ?? '').trim()
  const storeBot = (newBotId ?? '').trim()
  if (storeBot === urlBot) return
  if (storeBot) {
    void router.replace({
      name: 'chat',
      params: { botId: storeBot },
    })
  } else if (route.name !== 'home') {
    void router.replace({ name: 'home' })
  }
})

watch(
  () => route.params.botId,
  async (paramBotId) => {
    const urlBot = ((paramBotId as string) ?? '').trim()
    const storeBot = (currentBotId.value ?? '').trim()
    if (!urlBot || urlBot === storeBot) return

    suppressUrlSync = true
    try {
      await chatStore.selectBot(urlBot)
    } finally {
      suppressUrlSync = false
    }
  },
)
</script>
