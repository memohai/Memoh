<template>
  <div class="flex h-full">
    <MasterDetailSidebarLayout>
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
        <div class="px-1.5 pb-2">
          <div class="flex items-center gap-1.5 h-[30px] rounded-lg border border-border bg-card px-2.5">
            <FontAwesomeIcon
              :icon="['fas', 'magnifying-glass']"
              class="size-[11px] text-muted-foreground shrink-0"
            />
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="$t('chat.searchPlaceholder')"
              class="flex-1 min-w-0 bg-transparent text-xs text-foreground placeholder:text-muted-foreground outline-none"
            >
          </div>
        </div>

        <div class="px-3.5 pt-2 pb-1">
          <p class="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">
            {{ $t('sidebar.bots') }}
          </p>
        </div>
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
      <template #sidebar-footer>
        <div class="px-4 pb-3 pt-1">
          <button
            class="flex items-center gap-[7px] w-full h-[34px] rounded-lg bg-foreground text-background px-3.5 text-xs font-medium hover:bg-foreground/90 transition-colors"
            @click="chatStore.createNewSession()"
          >
            <FontAwesomeIcon
              :icon="['fas', 'plus']"
              class="size-3"
            />
            <span>{{ $t('chat.newSession') }}</span>
          </button>
        </div>
      </template>
      <template #detail>
        <ChatArea />
      </template>
    </MasterDetailSidebarLayout>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
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

const searchQuery = ref('')

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
