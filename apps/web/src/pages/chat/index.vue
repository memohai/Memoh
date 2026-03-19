<template>
  <div class="flex h-full">
    <MasterDetailSidebarLayout>
      <template #sidebar-header>
        <div class="p-4 border-b">
          <p class="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
            {{ $t('sidebar.bots') }}
          </p>
        </div>
      </template>
      <template #sidebar-content>
        <BotSidebar />

        <template v-if="currentBotId">
          <div class="px-4 pt-4 pb-2 flex items-center justify-between">
            <p class="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
              {{ $t('chat.sessions') }}
            </p>
            <button
              class="p-1 rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
              :aria-label="$t('chat.newSession')"
              @click="chatStore.createNewSession()"
            >
              <FontAwesomeIcon
                :icon="['fas', 'plus']"
                class="size-3.5"
              />
            </button>
          </div>
          <SessionSidebar />
        </template>
      </template>
      <template #detail>
        <ChatArea />
      </template>
    </MasterDetailSidebarLayout>
  </div>
</template>

<script setup lang="ts">
import { watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute, useRouter } from 'vue-router'
import { useChatStore } from '@/store/chat-list'
import BotSidebar from './components/bot-sidebar.vue'
import SessionSidebar from './components/session-sidebar.vue'
import ChatArea from './components/chat-area.vue'
import MasterDetailSidebarLayout from '@/components/master-detail-sidebar-layout/index.vue'

const route = useRoute()
const router = useRouter()
const chatStore = useChatStore()
const { currentBotId, sessionId } = storeToRefs(chatStore)

const urlSessionId = ((route.params.sessionId as string) ?? '').trim()
if (urlSessionId) {
  sessionId.value = urlSessionId
} else {
  currentBotId.value = null
  sessionId.value = null
}

let suppressUrlSync = false

watch(sessionId, (newId) => {
  if (suppressUrlSync) return
  const urlId = ((route.params.sessionId as string) ?? '').trim()
  const storeId = (newId ?? '').trim()
  if (storeId === urlId) return
  router.replace({ name: 'chat', params: { sessionId: storeId || undefined } })
})

watch(() => route.params.sessionId, async (paramId) => {
  const urlId = ((paramId as string) ?? '').trim()
  const storeId = (sessionId.value ?? '').trim()
  if (!urlId || urlId === storeId) return
  suppressUrlSync = true
  try {
    await chatStore.selectSession(urlId)
  } finally {
    suppressUrlSync = false
  }
})
</script>
