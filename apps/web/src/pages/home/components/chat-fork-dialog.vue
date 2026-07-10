<template>
  <Dialog
    :open="open"
    @update:open="emit('update:open', $event)"
  >
    <DialogContent class="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>{{ $t('chat.forkDialog.title') }}</DialogTitle>
        <DialogDescription>{{ $t('chat.forkDialog.description') }}</DialogDescription>
      </DialogHeader>
      <form
        class="space-y-4"
        @submit.prevent="handleCreateFork"
      >
        <Input
          v-model="forkSessionTitle"
          :aria-label="$t('chat.forkDialog.namePlaceholder')"
          :placeholder="$t('chat.forkDialog.namePlaceholder')"
          :disabled="forkSubmitting"
          maxlength="120"
          autofocus
        />
        <DialogFooter>
          <Button
            type="submit"
            :disabled="!forkSessionTitle.trim() || forkSubmitting"
          >
            <Spinner
              v-if="forkSubmitting"
              class="mr-1 size-3"
            />
            {{ $t('common.create') }}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { Button, Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, Input, Spinner } from '@felinic/ui'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { useChatStore } from '@/store/chat-list'

// Fork-from-message dialog. The parent only opens it with the target message
// id; the fork call itself goes through the chat store here, so a successful
// fork also switches the active session as a side effect of forkMessage.
const props = defineProps<{
  open: boolean
  messageId: string
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const { t } = useI18n()
const chatStore = useChatStore()
const { activeSession } = storeToRefs(chatStore)

const forkSessionTitle = ref('')
const forkSubmitting = ref(false)

function defaultForkSessionTitle() {
  const sourceTitle = activeSession.value?.title?.trim() || t('chat.unknownSession')
  return t('chat.forkDialog.defaultTitle', { session: sourceTitle })
}

// Re-seed the title each time the dialog opens so it always reflects the
// session the user is forking now, not a stale previous open.
watch(() => props.open, (open) => {
  if (open) forkSessionTitle.value = defaultForkSessionTitle()
})

async function handleCreateFork() {
  const messageId = props.messageId.trim()
  const title = forkSessionTitle.value.trim()
  if (!messageId || !title || forkSubmitting.value) return
  forkSubmitting.value = true
  try {
    const ok = await chatStore.forkMessage(messageId, { title })
    if (ok) emit('update:open', false)
  } finally {
    forkSubmitting.value = false
  }
}
</script>
