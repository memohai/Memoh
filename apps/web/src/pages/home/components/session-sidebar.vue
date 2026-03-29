<template>
  <div
    class="flex shrink-0 h-full relative"
    :style="{ width: `${sidebarWidth}px` }"
  >
    <div class="flex flex-col h-full flex-1 min-w-0 bg-sidebar border-r border-border">
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
            <Search
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
          <Plus
            class="size-3"
          />
          {{ t('chat.newSession') }}
        </Button>
      </div>

      <div class="px-3.5 h-[38px] flex items-center shrink-0">
        <DropdownMenu>
          <DropdownMenuTrigger as-child>
            <button class="flex items-center gap-1">
              <component
                :is="filterIconComponent"
                class="size-2.5"
                :class="filterIconClass"
              />
              <span class="text-[10px] font-medium text-muted-foreground uppercase tracking-[0.7px]">
                {{ t('chat.sessionSourcePrefix') }}{{ filterLabel }}
              </span>
              <ChevronDown
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
              <Check
                v-if="filterType === opt.value"
                class="size-3 mr-2"
              />
              <span :class="filterType !== opt.value ? 'ml-5' : ''">
                {{ opt.label }}
              </span>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <div class="flex-1 relative min-h-0">
        <div class="absolute inset-0">
          <ScrollArea class="h-full">
            <div class="flex flex-col gap-1 px-1.5">
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
              <LoaderCircle
                class="size-4 animate-spin text-muted-foreground"
              />
            </div>
          </ScrollArea>
        </div>
      </div>
    </div>

    <div
      class="absolute top-0 right-0 w-1 h-full cursor-col-resize z-10 group"
      @mousedown="onResizeStart"
    >
      <div
        class="w-full h-full transition-colors group-hover:bg-primary/20"
        :class="{ 'bg-primary/30': isResizing }"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onBeforeUnmount, type Component } from 'vue'
import { useLocalStorage } from '@vueuse/core'
import { Search, Plus, ChevronDown, Check, LoaderCircle, MessageSquare, HeartPulse, Clock, GitBranch } from 'lucide-vue-next'
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

const MIN_WIDTH = 180
const MAX_WIDTH = 480
const DEFAULT_WIDTH = 223

const sidebarWidth = useLocalStorage('session-sidebar-width', DEFAULT_WIDTH)
const isResizing = ref(false)

function onResizeStart(e: MouseEvent) {
  e.preventDefault()
  isResizing.value = true
  const startX = e.clientX
  const startWidth = sidebarWidth.value

  function onMouseMove(ev: MouseEvent) {
    const delta = ev.clientX - startX
    sidebarWidth.value = Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, startWidth + delta))
  }

  function onMouseUp() {
    isResizing.value = false
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
    document.body.style.cursor = ''
    document.body.style.userSelect = ''
  }

  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}

onBeforeUnmount(() => {
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
})

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

const filterIconComponent = computed<Component>(() => {
  switch (filterType.value) {
    case 'heartbeat': return HeartPulse
    case 'schedule': return Clock
    case 'subagent': return GitBranch
    default: return MessageSquare
  }
})

const filterIconClass = computed(() => {
  switch (filterType.value) {
    case 'heartbeat': return 'text-rose-400'
    case 'schedule': return 'text-amber-400'
    case 'subagent': return 'text-violet-400'
    default: return 'text-muted-foreground'
  }
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
