<template>
  <div class="flex flex-col h-full min-w-0">
    <!-- Section label where the type filter used to be. The list below is a
         single recency-ordered stream of the user's real conversations
         (chat / discuss / agent); system-generated runs (heartbeat / schedule /
         subagent) are not surfaced here. -->
    <div class="shrink-0 px-2 pb-0.5 pt-1">
      <span class="pl-[11px] text-xs font-[550] tracking-[-0.02em] text-muted-foreground/80">
        {{ t('chat.recents') }}
      </span>
    </div>

    <div class="flex-1 relative min-h-0">
      <div class="absolute inset-0">
        <ScrollArea class="h-full">
          <div class="flex flex-col gap-1 px-2">
            <SessionItem
              v-for="session in visibleSessions"
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
            v-if="currentBotId && !loadingChats && visibleSessions.length === 0"
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
import { ref, computed } from 'vue'
import { LoaderCircle } from 'lucide-vue-next'
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
  Input,
  ScrollArea,
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

// One recency-ordered stream of the user's real conversations. The old type
// FILTER is gone: chat, discuss and acp_agent are the human-facing types and
// read as a single timeline, while heartbeat/schedule/subagent are
// system-generated runs that live in their own surfaces, not this list.
const USER_SESSION_TYPES = new Set(['chat', 'discuss', 'acp_agent'])
const visibleSessions = computed(() =>
  sortByRecency(sessions.value.filter(s => USER_SESSION_TYPES.has(s.type ?? 'chat'))),
)

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
