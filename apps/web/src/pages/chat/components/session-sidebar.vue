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
            <!-- Avatar area -->
            <div class="relative shrink-0">
              <Avatar
                v-if="isIMSession(session)"
                class="size-8"
              >
                <AvatarImage
                  v-if="sessionAvatarUrl(session)"
                  :src="sessionAvatarUrl(session)!"
                  :alt="sessionDisplayLabel(session)"
                />
                <AvatarFallback class="text-xs bg-primary/10 text-primary">
                  {{ sessionAvatarFallback(session) }}
                </AvatarFallback>
              </Avatar>
              <div
                v-else
                class="flex items-center justify-center size-8"
              >
                <FontAwesomeIcon
                  :icon="sessionIcon(session)"
                  class="size-3.5"
                  :class="sessionIconClass(session)"
                />
              </div>
              <ChannelBadge
                v-if="isIMSession(session)"
                :platform="session.channel_type!"
              />
            </div>

            <div class="flex-1 text-left min-w-0">
              <div class="text-sm truncate">
                {{ session.title || $t('chat.untitledSession') }}
              </div>
              <div
                v-if="sessionSubLabel(session)"
                class="text-xs text-muted-foreground truncate"
              >
                {{ sessionSubLabel(session) }}
              </div>
              <div
                v-else-if="session.updated_at"
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
import { useI18n } from 'vue-i18n'
import { useChatStore } from '@/store/chat-list'
import type { SessionSummary } from '@/composables/api/useChat'
import { Avatar, AvatarImage, AvatarFallback } from '@memoh/ui'
import ChannelBadge from '@/components/chat-list/channel-badge/index.vue'
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

const { t } = useI18n()
const chatStore = useChatStore()
const { sessions, sessionId, currentBotId, loadingChats } = storeToRefs(chatStore)

const WEB_CHANNELS = new Set(['web', ''])

function isIMSession(session: SessionSummary): boolean {
  const ct = (session.channel_type ?? '').trim().toLowerCase()
  return ct !== '' && !WEB_CHANNELS.has(ct)
}

function sessionIcon(session: SessionSummary): string[] {
  switch (session.type) {
    case 'heartbeat': return ['fas', 'heart-pulse']
    case 'schedule': return ['fas', 'clock']
    case 'subagent': return ['fas', 'code-branch']
    default: return ['fas', 'message']
  }
}

function sessionIconClass(session: SessionSummary): string {
  switch (session.type) {
    case 'heartbeat': return 'text-rose-400'
    case 'schedule': return 'text-amber-400'
    case 'subagent': return 'text-violet-400'
    default: return 'text-muted-foreground'
  }
}

function routeMeta(session: SessionSummary): Record<string, unknown> {
  return session.route_metadata ?? {}
}

function isGroupConversation(session: SessionSummary): boolean {
  const ct = (session.route_conversation_type ?? '').trim().toLowerCase()
  return ct === 'group' || ct === 'supergroup' || ct === 'channel'
}

function sessionAvatarUrl(session: SessionSummary): string | null {
  const meta = routeMeta(session)
  if (isGroupConversation(session)) {
    const convAvatar = (meta.conversation_avatar_url as string ?? '').trim()
    if (convAvatar) return convAvatar
  }
  const url = (meta.sender_avatar_url as string ?? '').trim()
  return url || null
}

function sessionAvatarFallback(session: SessionSummary): string {
  const label = sessionDisplayLabel(session)
  return label ? label.charAt(0).toUpperCase() : '?'
}

function sessionDisplayLabel(session: SessionSummary): string {
  const meta = routeMeta(session)
  const convName = (meta.conversation_name as string ?? '').trim()
  if (convName) return convName
  const senderName = (meta.sender_display_name as string ?? '').trim()
  if (senderName) return senderName
  const senderUsername = (meta.sender_username as string ?? '').trim()
  if (senderUsername) return senderUsername
  return ''
}

function sessionSubLabel(session: SessionSummary): string {
  if (session.type === 'heartbeat') return t('chat.sessionTypeHeartbeat')
  if (session.type === 'schedule') return t('chat.sessionTypeSchedule')
  if (session.type === 'subagent') return t('chat.sessionTypeSubagent')

  if (!isIMSession(session)) return ''
  const meta = routeMeta(session)

  if (isGroupConversation(session)) {
    const convHandle = (meta.conversation_handle as string ?? '').trim()
    if (convHandle) return convHandle.startsWith('@') ? convHandle : `@${convHandle}`
    const convName = (meta.conversation_name as string ?? '').trim()
    if (convName) return `@${convName}`
  }

  const senderUsername = (meta.sender_username as string ?? '').trim()
  if (senderUsername) return `@${senderUsername}`
  const senderName = (meta.sender_display_name as string ?? '').trim()
  if (senderName) return senderName
  return ''
}

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
