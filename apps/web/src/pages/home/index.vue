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
import { getBotsById } from '@memohai/sdk'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { ACP_NO_PROJECT_MODE, createACPNoProjectPath, normalizeACPAgentID } from '@/utils/acp'
import { openInFileManagerKey } from './composables/useFileManagerProvider'
import ChatSidebar from './components/chat-sidebar.vue'
import ChatWorkspace from './components/chat-workspace.vue'

const route = useRoute()
const router = useRouter()
const chatStore = useChatStore()
const workspaceTabs = useWorkspaceTabsStore()
const { currentBotId, bots } = storeToRefs(chatStore)

// Resolve a bot UUID from a URL name slug. Prefers the already-loaded bot list,
// falling back to the API (which accepts both name and UUID identifiers).
async function resolveBotIdFromName(nameOrId: string): Promise<string | null> {
  const value = nameOrId.trim()
  if (!value) return null
  const cached = bots.value.find((b) => b.name === value || b.id === value)
  if (cached?.id) return cached.id
  try {
    const { data } = await getBotsById({ path: { id: value }, throwOnError: true })
    return data?.id ?? null
  } catch {
    return null
  }
}

// Resolve a URL name slug from a bot UUID, preferring the loaded bot list.
async function resolveBotNameFromId(botId: string): Promise<string | null> {
  const value = botId.trim()
  if (!value) return null
  const cached = bots.value.find((b) => b.id === value)
  if (cached?.name) return cached.name
  try {
    const { data } = await getBotsById({ path: { id: value }, throwOnError: true })
    return data?.name ?? null
  } catch {
    return null
  }
}

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

let suppressUrlSync = false

// One-shot guard so concurrent syncStoreFromUrl() calls can't both start a
// session for the same redirect. Set synchronously before the first await.
let acpStartConsumed = false

function stripAcpQuery() {
  if (route.query.acp === undefined) return
  const query = { ...route.query }
  delete query.acp
  void router.replace({ query })
}

// When onboarding redirects here with ?acp=<agent>, open an ACP session for the
// freshly configured agent so the user lands inside it. Read the query at call
// time (not captured at setup) so it works regardless of mount timing.
async function maybeStartACPSession() {
  if (acpStartConsumed) return
  const raw = route.query.acp
  if (typeof raw !== 'string' || raw === '') {
    stripAcpQuery()
    return
  }
  acpStartConsumed = true
  const agentId = normalizeACPAgentID(raw)
  try {
    if (agentId) {
      const { session } = await chatStore.createACPSession({ agentId, projectMode: ACP_NO_PROJECT_MODE, projectPath: createACPNoProjectPath() })
      workspaceTabs.openChat(session.id, session.title)
    }
  } catch {
    // Bot may not have the agent enabled; user can still pick it from the composer.
  } finally {
    // Always strip the one-shot query param, even for malformed/empty values.
    stripAcpQuery()
  }
}

async function syncStoreFromUrl(rawName: string) {
  const urlName = rawName.trim()
  if (!urlName) return
  const resolvedId = await resolveBotIdFromName(urlName)
  if (!resolvedId) return
  if (resolvedId !== (currentBotId.value ?? '').trim()) {
    suppressUrlSync = true
    try {
      await chatStore.selectBot(resolvedId)
    } finally {
      suppressUrlSync = false
    }
  }
  await maybeStartACPSession()
  // Canonicalize the URL to the bot's name slug. This covers entry points that
  // navigate with a UUID (e.g. returning from settings), where currentBotId is
  // unchanged so the watcher below never fires.
  const canonicalName = await resolveBotNameFromId(resolvedId)
  if (canonicalName && urlName !== canonicalName) {
    void router.replace({ name: 'bot', params: { botName: canonicalName } })
  }
}

void syncStoreFromUrl((route.params.botName as string) ?? '')

watch(currentBotId, async (newBotId) => {
  if (suppressUrlSync) return
  const storeBot = (newBotId ?? '').trim()
  if (!storeBot) {
    if (route.name !== 'home') {
      void router.replace({ name: 'home' })
    }
    return
  }
  const botName = await resolveBotNameFromId(storeBot)
  if (!botName) return
  if (((route.params.botName as string) ?? '').trim() === botName) return
  void router.replace({
    name: 'bot',
    params: { botName },
  })
})

watch(
  () => route.params.botName,
  (paramBotName) => {
    void syncStoreFromUrl((paramBotName as string) ?? '')
  },
)
</script>
