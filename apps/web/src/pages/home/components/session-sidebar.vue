<template>
  <div class="flex flex-col h-full w-[223px] shrink-0 bg-sidebar border-r border-border">
    <!-- <div class="h-[53px] flex items-center px-2 shrink-0">
      <FontAwesomeIcon
        :icon="['fas', 'comment-dots']"
        class="size-6 text-foreground ml-1.5"
      />
      <span class="text-xs font-semibold text-foreground ml-2 flex-1">
        {{ t('sidebar.chat') }}
      </span>
      <DropdownMenu>
        <DropdownMenuTrigger as-child>
          <Button
            variant="ghost"
            size="icon"
            class="size-6 text-muted-foreground hover:text-foreground"
          >
            <FontAwesomeIcon
              :icon="['fas', 'ellipsis']"
              class="size-4"
            />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem @click="handleNewSession">
            <FontAwesomeIcon
              :icon="['fas', 'plus']"
              class="size-3 mr-2"
            />
            {{ t('chat.newSession') }}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div> -->

    <div class="p-2 shrink-0">
      <InputGroup class="h-[30px]">
        <InputGroupAddon class="pl-2.5">
          <FontAwesomeIcon
            :icon="['fas', 'magnifying-glass']"
            class="size-[11px] text-muted-foreground"
          />
        </InputGroupAddon>
        <InputGroupInput
          v-model="searchQuery"
          :placeholder="t('chat.searchSessionPlaceholder')"
          class="text-xs h-[30px]"
        />
      </InputGroup>
    </div>

    <div class="px-1.5 shrink-0">
      <Button
        variant="ghost"
        class="w-full h-12 justify-start gap-4.5 text-xs font-medium"
        :disabled="!currentBotId"
        @click="handleNewSession"
      >
        <FontAwesomeIcon
          :icon="['fas', 'plus']"
          class="size-3"
        />
        {{ t('chat.newSession') }}
      </Button>
    </div>

    <div class="px-3.5 h-[38px] flex items-center shrink-0">
      <DropdownMenu>
        <DropdownMenuTrigger as-child>
          <button class="flex items-center gap-1">
            <FontAwesomeIcon
              :icon="['fas', 'browser']"
              class="size-2.5 text-muted-foreground"
            />
            <span class="text-[10px] font-medium text-muted-foreground uppercase tracking-[0.7px]">
              {{ t('chat.sessionSourcePrefix') }}{{ filterLabel }}
            </span>
            <FontAwesomeIcon
              :icon="['fas', 'chevron-down']"
              class="size-2.5 text-muted-foreground"
            />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuItem
            v-for="opt in filterOptions"
            :key="opt.value ?? 'all'"
            @click="filterType = opt.value"
          >
            <FontAwesomeIcon
              v-if="filterType === opt.value"
              :icon="['fas', 'check']"
              class="size-3 mr-2"
            />
            <span :class="filterType !== opt.value ? 'ml-5' : ''">
              {{ opt.label }}
            </span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>

    <ScrollArea class="flex-1">
      <div class="flex flex-col gap-1 px-1.5 overflow-hidden">
        <SessionItem
          v-for="session in filteredSessions"
          :key="session.id"
          :session="session"
          :is-active="sessionId === session.id"
          @select="handleSelect"
        />
      </div>

      <div
        v-if="currentBotId && !loadingChats && filteredSessions.length === 0"
        class="px-3 py-6 text-center text-xs text-muted-foreground"
      >
        {{ t('chat.noSessions') }}
      </div>

      <div
        v-if="loadingChats"
        class="flex justify-center py-4"
      >
        <FontAwesomeIcon
          :icon="['fas', 'spinner']"
          class="size-4 animate-spin text-muted-foreground"
        />
      </div>
    </ScrollArea>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useChatStore } from '@/store/chat-list'
import type { SessionSummary } from '@/composables/api/useChat'
import {
  Button,
  ScrollArea,
  InputGroup,
  InputGroupInput,
  InputGroupAddon,
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@memohai/ui'
import SessionItem from './session-item.vue'

const { t } = useI18n()
const router = useRouter()
const chatStore = useChatStore()
const { sessions, sessionId, currentBotId, loadingChats } = storeToRefs(chatStore)

const searchQuery = ref('')
const filterType = ref<string>('chat')

const filterOptions = computed(() => [
  { value: 'chat', label: t('chat.sessionTypeChat') },
  { value: 'heartbeat', label: t('chat.sessionTypeHeartbeat') },
  { value: 'schedule', label: t('chat.sessionTypeSchedule') },
  { value: 'subagent', label: t('chat.sessionTypeSubagent') },
])

const filterLabel = computed(() => {
  const opt = filterOptions.value.find(o => o.value === filterType.value)
  return opt?.label ?? t('chat.sessionTypeChat')
})

const filteredSessions = computed(() => {
  let list = sessions.value
  list = list.filter(s => s.type === filterType.value)
  const q = searchQuery.value.trim().toLowerCase()
  if (q) {
    list = list.filter(s =>
      (s.title ?? '').toLowerCase().includes(q)
      || (s.id ?? '').toLowerCase().includes(q),
    )
  }
  return list
})

function handleSelect(session: SessionSummary) {
  chatStore.selectSession(session.id)
  if (currentBotId.value) {
    router.replace({
      name: 'chat',
      params: {
        botId: currentBotId.value,
        sessionId: session.id,
      },
    })
  }
}

function handleNewSession() {
  chatStore.createNewSession()
}
</script>
