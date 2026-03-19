<template>
  <div class="flex flex-col h-full">
    <!-- Session list -->
    <SidebarMenu v-if="currentBotId">
      <SidebarMenuItem
        v-for="session in sessions"
        :key="session.id"
      >
        <SidebarMenuButton
          as-child
          class="justify-start py-3! px-3"
        >
          <Toggle
            :class="`p-2! border border-transparent h-[initial]! w-full ${sessionId === session.id ? 'border-inherit' : ''}`"
            :model-value="sessionId === session.id"
            @click="handleSelect(session)"
          >
            <FontAwesomeIcon
              :icon="['fas', 'message']"
              class="size-3.5 shrink-0 text-muted-foreground"
            />
            <div class="flex-1 text-left min-w-0">
              <div class="text-sm truncate">
                {{ session.title || $t('chat.untitledSession') }}
              </div>
              <div
                v-if="session.updated_at"
                class="text-xs text-muted-foreground truncate"
              >
                {{ formatTime(session.updated_at) }}
              </div>
            </div>
            <DropdownMenu>
              <DropdownMenuTrigger
                as-child
                @click.stop
              >
                <button
                  class="p-1 rounded opacity-0 group-hover:opacity-100 hover:bg-muted transition-opacity"
                  :aria-label="$t('common.operation')"
                >
                  <FontAwesomeIcon
                    :icon="['fas', 'ellipsis-vertical']"
                    class="size-3"
                  />
                </button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem @click.stop="handleDelete(session)">
                  <FontAwesomeIcon
                    :icon="['fas', 'trash']"
                    class="size-3 mr-2"
                  />
                  {{ $t('chat.deleteSession') }}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </Toggle>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>

    <div
      v-if="currentBotId && !loadingChats && sessions.length === 0"
      class="px-3 py-6 text-center text-sm text-muted-foreground"
    >
      {{ $t('chat.noSessions') }}
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
  </div>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useChatStore } from '@/store/chat-list'
import type { SessionSummary } from '@/composables/api/useChat'
import {
  Toggle,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@memoh/ui'

const chatStore = useChatStore()
const { sessions, sessionId, currentBotId, loadingChats } = storeToRefs(chatStore)

function handleSelect(session: SessionSummary) {
  chatStore.selectSession(session.id)
}

function handleDelete(session: SessionSummary) {
  chatStore.removeSession(session.id)
}

function formatTime(dateStr: string): string {
  try {
    const d = new Date(dateStr)
    if (Number.isNaN(d.getTime())) return ''
    const now = new Date()
    const diff = now.getTime() - d.getTime()
    const day = 86400000
    if (diff < day) {
      return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
    }
    if (diff < 7 * day) {
      return d.toLocaleDateString(undefined, { weekday: 'short', hour: '2-digit', minute: '2-digit' })
    }
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
  } catch {
    return ''
  }
}
</script>
