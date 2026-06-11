<template>
  <div class="flex flex-col h-full min-w-0">
    <div class="flex items-center px-2 py-1.5 shrink-0">
      <DropdownMenu>
        <DropdownMenuTrigger as-child>
          <TextButton class="text-label">
            <component
              :is="filterIconComponent"
              class="size-3.5"
              :class="filterIconClass"
            />
            {{ filterLabel }}
            <ChevronDown class="size-3.5 opacity-60" />
          </TextButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuItem
            v-for="opt in filterOptions"
            :key="opt.value ?? 'all'"
            @click="filterType = opt.value"
          >
            {{ opt.label }}
            <Check
              v-if="filterType === opt.value"
              class="size-3.5 ml-auto text-muted-foreground"
            />
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>

    <div class="flex-1 relative min-h-0">
      <div class="absolute inset-0">
        <ScrollArea class="h-full">
          <div class="flex flex-col gap-1 px-2">
            <SessionItem
              v-for="session in filteredSessions"
              :key="session.id"
              :session="session"
              :is-active="sessionId === session.id"
              :streaming="chatStore.isSessionStreaming(session.id)"
              @select="handleSelect"
              @rename="openRenameSessionDialog"
              @delete="confirmDeleteSession"
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

    <Dialog v-model:open="deleteSessionDialogOpen">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ t('chat.deleteSession') }}</DialogTitle>
          <DialogDescription>{{ t('chat.deleteSessionConfirm') }}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="outline"
            :disabled="deleteSessionLoading"
            @click="deleteSessionDialogOpen = false"
          >
            {{ t('common.cancel') }}
          </Button>
          <Button
            variant="destructive"
            :disabled="deleteSessionLoading"
            @click="handleDeleteSession"
          >
            <LoaderCircle
              v-if="deleteSessionLoading"
              class="mr-1 size-3 animate-spin"
            />
            {{ t('common.confirm') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <Dialog v-model:open="renameSessionDialogOpen">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ t('chat.renameSession') }}</DialogTitle>
          <DialogDescription>{{ t('chat.renameSessionDescription') }}</DialogDescription>
        </DialogHeader>
        <form
          class="space-y-4"
          @submit.prevent="handleRenameSession"
        >
          <Input
            v-model="renameSessionTitle"
            :placeholder="t('chat.renameSessionPlaceholder')"
            :disabled="renameSessionLoading"
            autofocus
          />
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              :disabled="renameSessionLoading"
              @click="renameSessionDialogOpen = false"
            >
              {{ t('common.cancel') }}
            </Button>
            <Button
              type="submit"
              :disabled="!renameSessionTitle.trim() || renameSessionLoading"
            >
              <LoaderCircle
                v-if="renameSessionLoading"
                class="mr-1 size-3 animate-spin"
              />
              {{ t('common.confirm') }}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, type Component } from 'vue'
import { ChevronDown, Check, LoaderCircle, MessageSquare, MessageCircle, HeartPulse, Clock, GitBranch, Bot } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { toast } from '@memohai/ui'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { sortByRecency } from '@/store/chat-list.utils'
import type { SessionSummary } from '@/composables/api/useChat'
import { resolveApiErrorMessage } from '@/utils/api-error'
import {
  Button,
  TextButton,
  Input,
  ScrollArea,
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@memohai/ui'
import SessionItem from './session-item.vue'

const { t } = useI18n()
const chatStore = useChatStore()
const workspaceTabs = useWorkspaceTabsStore()
const { sessions, sessionId, currentBotId, loadingChats } = storeToRefs(chatStore)

const filterType = ref<string>('chat')

const filterOptions = computed(() => [
  { value: 'chat', label: t('chat.sessionTypeChat') },
  { value: 'discuss', label: t('chat.sessionTypeDiscuss') },
  { value: 'heartbeat', label: t('chat.sessionTypeHeartbeat') },
  { value: 'schedule', label: t('chat.sessionTypeSchedule') },
  { value: 'subagent', label: t('chat.sessionTypeSubagent') },
  { value: 'acp_agent', label: t('chat.sessionTypeACPAgent') },
])

const filterLabel = computed(() => {
  const opt = filterOptions.value.find(o => o.value === filterType.value)
  return opt?.label ?? t('chat.sessionTypeChat')
})

const filterIconComponent = computed<Component>(() => {
  switch (filterType.value) {
    case 'discuss': return MessageCircle
    case 'heartbeat': return HeartPulse
    case 'schedule': return Clock
    case 'subagent': return GitBranch
    case 'acp_agent': return Bot
    default: return MessageSquare
  }
})

const filterIconClass = computed(() => {
  switch (filterType.value) {
    case 'discuss': return 'text-event-discuss'
    case 'heartbeat': return 'text-event-heartbeat'
    case 'schedule': return 'text-event-schedule'
    case 'subagent': return 'text-event-subagent'
    case 'acp_agent': return 'text-muted-foreground'
    default: return 'text-muted-foreground'
  }
})

const filteredSessions = computed(() => {
  let list = sessions.value
  if (filterType.value === 'chat') {
    list = list.filter(s => s.type === 'chat' || s.type === 'discuss' || s.type === 'acp_agent')
  } else {
    list = list.filter(s => s.type === filterType.value)
  }
  return sortByRecency(list)
})

function handleSelect(session: SessionSummary) {
  void chatStore.selectSession(session.id)
  workspaceTabs.openChat((session.title ?? '').trim() || t('chat.untitledSession'))
}

const deleteSessionDialogOpen = ref(false)
const deleteSessionLoading = ref(false)
const sessionPendingDelete = ref<SessionSummary | null>(null)
const renameSessionDialogOpen = ref(false)
const renameSessionLoading = ref(false)
const sessionPendingRename = ref<SessionSummary | null>(null)
const renameSessionTitle = ref('')

function confirmDeleteSession(session: SessionSummary) {
  sessionPendingDelete.value = session
  deleteSessionDialogOpen.value = true
}

function openRenameSessionDialog(session: SessionSummary) {
  sessionPendingRename.value = session
  renameSessionTitle.value = session.title?.trim() || ''
  renameSessionDialogOpen.value = true
}

async function handleRenameSession() {
  const target = sessionPendingRename.value
  const title = renameSessionTitle.value.trim()
  if (!target || !title || renameSessionLoading.value) return
  renameSessionLoading.value = true
  try {
    await chatStore.renameSession(target.id, title)
    renameSessionDialogOpen.value = false
    sessionPendingRename.value = null
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('chat.renameSessionFailed')))
  } finally {
    renameSessionLoading.value = false
  }
}

async function handleDeleteSession() {
  const target = sessionPendingDelete.value
  if (!target || deleteSessionLoading.value) return
  deleteSessionLoading.value = true
  try {
    await chatStore.removeSession(target.id)
    deleteSessionDialogOpen.value = false
    sessionPendingDelete.value = null
  } finally {
    deleteSessionLoading.value = false
  }
}
</script>
