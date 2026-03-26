<template>
  <div class="flex h-full">
    <!-- <Teleport to=".main-left-section">
      <SidebarProvider>
        <SidebarContent>
          <BotSidebar />
        </SidebarContent>
      </SidebarProvider>
    </Teleport>
   -->
    <!-- <MasterDetailSidebarLayout>
      <template #sidebar-header>
        <div class="flex items-center gap-2">
          <FontAwesomeIcon
            :icon="['fas', 'comment-dots']"
            class="size-[18px] text-foreground"
          />
          <span class="text-sm font-semibold text-foreground">
            {{ $t('sidebar.chat') }}
          </span>
        </div>
      </template>
<template #sidebar-content>
        <BotSidebar />

        <template v-if="currentBotId">
          <div class="px-3.5 pt-3 pb-1">
            <p class="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">
              {{ $t('chat.sessions') }}
            </p>
          </div>
          <SessionSidebar />
        </template>
</template>
<template #detail>
        <ChatArea />
      </template>
</MasterDetailSidebarLayout> -->
   

    <ChatHeader />
    <BotSidebar />
    <SessionSidebar />
    <ChatArea />
  </div>
</template>

<script setup lang="ts">
import { watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRoute, useRouter } from 'vue-router'
import { useChatStore } from '@/store/chat-list'
// import BotSidebar from './components/bot-sidebar.vue'
// import SessionSidebar from './components/session-sidebar.vue'
import ChatArea from './components/chat-area.vue'
import { defineAsyncComponent } from 'vue'

const BotSidebar = defineAsyncComponent(async () => import('./components/bot-sidebar.vue'))

const SessionSidebar = defineAsyncComponent(() => import('./components/session_sidebar.vue'))

const ChatHeader = defineAsyncComponent(() => import('./components/chat-header.vue'))

const route = useRoute()
const router = useRouter()
const chatStore = useChatStore()
const { currentBotId, sessionId } = storeToRefs(chatStore)

// const searchQuery = ref('')

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
