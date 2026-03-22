<template>
  <div class="flex flex-col gap-1 px-1.5">
    <template v-if="currentBotId">
      <button
        v-for="session in sessions"
        :key="session.id"
        class="group flex items-center h-[58px] w-full rounded-lg px-2.5 text-left transition-colors"
        :class="sessionId === session.id
          ? 'bg-card border border-border'
          : 'hover:bg-card/60'"
        @click="handleSelect(session)"
      >
        <div class="relative shrink-0 mr-2.5">
          <Avatar
            v-if="isIMSession(session)"
            class="size-[26px] border border-border"
          >
            <AvatarImage
              v-if="sessionAvatarUrl(session)"
              :src="sessionAvatarUrl(session)!"
              :alt="sessionDisplayLabel(session)"
            />
            <AvatarFallback class="text-[9px] bg-secondary text-muted-foreground">
              {{ sessionAvatarFallback(session) }}
            </AvatarFallback>
          </Avatar>
          <div
            v-else
            class="flex items-center justify-center size-[26px] rounded-full bg-secondary border border-border"
          >
            <FontAwesomeIcon
              :icon="sessionIcon(session)"
              class="size-2.5"
              :class="sessionIconClass(session)"
            />
          </div>
          <ChannelBadge
            v-if="isIMSession(session)"
            :platform="session.channel_type!"
          />
        </div>

        <div class="flex flex-col gap-[3px] flex-1 min-w-0">
          <div class="text-xs font-medium text-foreground truncate">
            {{ session.title || $t('chat.untitledSession') }}
          </div>
          <div
            v-if="sessionSubLabel(session)"
            class="flex items-center gap-[5px]"
          >
            <span
              v-if="session.type === 'heartbeat' || session.type === 'schedule' || session.type === 'subagent'"
              class="size-[5px] rounded-sm shrink-0"
              :class="statusDotClass(session)"
            />
            <span class="text-[11px] text-muted-foreground truncate leading-[16.5px]">
              {{ sessionSubLabel(session) }}
            </span>
          </div>
          <div
            v-else-if="session.updated_at"
            class="text-[11px] text-muted-foreground truncate leading-[16.5px]"
          >
            {{ formatTime(session.updated_at) }}
          </div>
        </div>

        <div class="flex items-start shrink-0 ml-1 self-start pt-2.5">
          <span
            v-if="session.updated_at"
            class="text-[11px] text-muted-foreground leading-[16.5px] whitespace-nowrap"
          >
            {{ formatTime(session.updated_at) }}
          </span>
        </div>

        <DropdownMenu>
          <DropdownMenuTrigger
            as-child
            @click.stop
          >
            <button
              class="p-1 rounded opacity-0 group-hover:opacity-100 hover:bg-muted transition-opacity ml-0.5 shrink-0"
              :aria-label="$t('common.operation')"
            >
              <FontAwesomeIcon
                :icon="['fas', 'ellipsis-vertical']"
                class="size-3 text-muted-foreground"
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
      </button>
    </template>

    <div
      v-if="currentBotId && !loadingChats && sessions.length === 0"
      class="px-3 py-6 text-center text-xs text-muted-foreground"
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
import { Avatar, AvatarImage, AvatarFallback } from '@memohai/ui'
import ChannelBadge from '@/components/chat-list/channel-badge/index.vue'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@memohai/ui'

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

function statusDotClass(session: SessionSummary): string {
  switch (session.type) {
    case 'heartbeat': return 'bg-rose-400'
    case 'schedule': return 'bg-amber-400'
    case 'subagent': return 'bg-primary'
    default: return 'bg-primary'
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
