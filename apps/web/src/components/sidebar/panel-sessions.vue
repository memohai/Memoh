<template>
  <div class="flex flex-col h-full min-w-0">
    <div class="px-2 pt-1.5 shrink-0">
      <Button
        variant="ghost"
        block
        class="h-9 justify-start gap-2.5 px-2.5 text-control"
        :disabled="!currentBotId"
        @click="handleNewSession"
      >
        <SquarePen class="size-4" />
        {{ t('chat.newSession') }}
      </Button>
    </div>
    <Recents class="flex-1 min-h-0" />
  </div>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { Button } from '@memohai/ui'
import { SquarePen } from 'lucide-vue-next'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import Recents from './recents.vue'

const { t } = useI18n()
const chatStore = useChatStore()
const workspaceTabs = useWorkspaceTabsStore()
const { currentBotId } = storeToRefs(chatStore)

function handleNewSession() {
  if (!currentBotId.value) return
  void chatStore.createNewSession()
  workspaceTabs.openChat(t('chat.newSession'))
}
</script>
