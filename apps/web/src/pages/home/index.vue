<template>
  <div class="flex h-full overflow-hidden">
    <ChatWorkspace v-if="currentBotId" />
    <div
      v-else
      class="flex-1 flex items-center justify-center bg-card"
    >
      <div class="text-center px-6">
        <p class="text-xs font-medium text-foreground">
          {{ t('chat.selectBot') }}
        </p>
        <p class="mt-1 text-xs text-muted-foreground">
          {{ t('chat.selectBotHint') }}
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { getBotsById } from '@memohai/sdk'
import { useChatStore } from '@/store/chat-list'
import { ACP_NO_PROJECT_MODE, createACPNoProjectPath, normalizeACPAgentID } from '@/utils/acp'
import ChatWorkspace from './components/chat-workspace.vue'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const chatStore = useChatStore()
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

let suppressUrlSync = false

// home is now mounted persistently (via main-container, which App.vue keeps
// alive across chat↔settings) rather than torn down when leaving chat. So these
// route watchers keep running while the user is in settings. The settings route
// shares the :botName param, so without this guard the route.params.botName
// watcher would "canonicalize" that UUID and router.replace back to /bot/<name>,
// yanking the user out of settings. URL sync must only run while a chat route
// (home/bot) is current. route.name is the reliable signal (no mount/unmount
// timing to race).
const CHAT_ROUTE_NAMES = new Set(['home', 'bot'])
const isChatRoute = () => CHAT_ROUTE_NAMES.has(route.name as string)

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
      await chatStore.selectSession(session.id)
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
  if (!urlName) {
    if (!currentBotId.value) {
      await chatStore.initialize()
    }
    await maybeStartACPSession()
    return
  }
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

watch(
  () => [route.name, route.params.botName] as const,
  ([routeName, paramBotName]) => {
    if (!CHAT_ROUTE_NAMES.has(routeName as string)) return
    void syncStoreFromUrl((paramBotName as string) ?? '')
  },
  { immediate: true },
)

watch(currentBotId, async (newBotId) => {
  if (suppressUrlSync) return
  // Don't touch the URL while a non-chat route (e.g. settings) is current; home
  // stays mounted during /settings route changes and must not redirect away from it.
  if (!isChatRoute()) return
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

</script>
