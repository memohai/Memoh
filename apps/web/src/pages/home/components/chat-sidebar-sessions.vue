<template>
  <div class="flex flex-col h-full min-w-0">
    <div class="p-2 shrink-0">
      <InputGroup class="h-[30px]">
        <InputGroupAddon class="pl-2.5">
          <Search
            class="size-2.75 text-muted-foreground"
          />
        </InputGroupAddon>
        <InputGroupInput
          v-model="searchQuery"
          :placeholder="t('chat.searchSessionPlaceholder')"
          class="text-xs h-7.5"
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

    <div class="px-3.5 h-9.5 flex items-center shrink-0">
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
            class="relative"
            @click="filterType = opt.value"
          >
            <Check
              v-if="filterType === opt.value"
              class="size-3 mr-2 absolute"
            />
            <span class="ml-5">
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
              {{ t('common.save') }}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, type Component } from 'vue'
import { Search, Plus, ChevronDown, Check, LoaderCircle, MessageSquare, MessageCircle, HeartPulse, Clock, GitBranch, Bot } from 'lucide-vue-next'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import type { SessionSummary } from '@/composables/api/useChat'
import { resolveApiErrorMessage } from '@/utils/api-error'
import {
  Button,
  Input,
  ScrollArea,
  InputGroup,
  InputGroupInput,
  InputGroupAddon,
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

const searchQuery = ref('')
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
  workspaceTabs.openChat(session.id, session.title ?? '')
}

function handleNewSession() {
  workspaceTabs.openDraft()
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
    const updated = await chatStore.renameSession(target.id, title)
    workspaceTabs.updateChatTitle(target.id, updated.title ?? title)
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
    workspaceTabs.closeChatBySession(target.id)
    deleteSessionDialogOpen.value = false
    sessionPendingDelete.value = null
  } finally {
    deleteSessionLoading.value = false
  }
}
</script>
