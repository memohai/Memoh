<template>
  <div class="flex h-full">
    <template v-if="currentBotId">
      <SessionSidebar />
      <div class="flex-1 flex flex-col min-w-0">
        <ChatArea />
      </div>
      <!-- <SessionMetadata /> -->
    </template>
  </div>
</template>

<script setup lang="ts">
import { watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute, useRouter } from 'vue-router'
import { useChatStore } from '@/store/chat-list'
import SessionSidebar from './components/session-sidebar.vue'
// import SessionMetadata from './components/session-metadata.vue'
import ChatArea from './components/chat-area.vue'

const route = useRoute()
const router = useRouter()
const chatStore = useChatStore()
const { currentBotId, sessionId } = storeToRefs(chatStore)

const urlBotId = ((route.params.botId as string) ?? '').trim()
const urlSessionId = ((route.params.sessionId as string) ?? '').trim()

if (urlBotId) {
  chatStore.selectBot(urlBotId)
  if (urlSessionId) {
    sessionId.value = urlSessionId
  }
} else if (!urlBotId && !urlSessionId) {
  currentBotId.value = null
  sessionId.value = null
}

let suppressUrlSync = false

watch([currentBotId, sessionId], ([newBotId, newSessionId]) => {
  if (suppressUrlSync) return
  const urlBot = ((route.params.botId as string) ?? '').trim()
  const urlSession = ((route.params.sessionId as string) ?? '').trim()
  const storeBot = (newBotId ?? '').trim()
  const storeSession = (newSessionId ?? '').trim()
  if (storeBot === urlBot && storeSession === urlSession) return
  if (storeBot) {
    router.replace({
      name: 'chat',
      params: {
        botId: storeBot,
        sessionId: storeSession || undefined,
      },
    })
  } else if (route.name !== 'home') {
    router.replace({ name: 'home' })
  }
})

watch(
  () => [route.params.botId, route.params.sessionId],
  async ([paramBotId, paramSessionId]) => {
    const urlBot = ((paramBotId as string) ?? '').trim()
    const urlSession = ((paramSessionId as string) ?? '').trim()
    const storeBot = (currentBotId.value ?? '').trim()
    const storeSession = (sessionId.value ?? '').trim()

    suppressUrlSync = true
    try {
      if (urlBot && urlBot !== storeBot) {
        await chatStore.selectBot(urlBot)
      }
      if (urlSession && urlSession !== storeSession) {
        await chatStore.selectSession(urlSession)
      }
      if (!urlBot && storeBot) {
        currentBotId.value = null
        sessionId.value = null
      }
    } finally {
      suppressUrlSync = false
    }
  },
)
</script>
