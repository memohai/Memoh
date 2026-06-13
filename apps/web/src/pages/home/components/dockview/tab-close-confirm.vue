<template>
  <Dialog
    :open="!!pending"
    @update:open="onOpenChange"
  >
    <DialogContent class="sm:max-w-[420px]">
      <DialogHeader>
        <DialogTitle>{{ t('chat.unsaved.title', { name: pending?.title ?? '' }) }}</DialogTitle>
        <DialogDescription>{{ t('chat.unsaved.description') }}</DialogDescription>
      </DialogHeader>
      <DialogFooter class="mt-4">
        <Button
          variant="ghost"
          @click="resolve('cancel')"
        >
          {{ t('common.cancel') }}
        </Button>
        <Button
          variant="outline"
          @click="resolve('discard')"
        >
          {{ t('chat.unsaved.dontSave') }}
        </Button>
        <Button
          :disabled="saving"
          @click="onSave"
        >
          {{ t('chat.unsaved.save') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@memohai/ui'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'

// Save-on-close confirmation for unsaved file tabs. The store drives this via
// `pendingClose` (the head of its close queue); a batch close walks through one
// dialog per dirty tab. Esc / overlay / corner-X all map to Cancel.
const { t } = useI18n()
const store = useWorkspaceTabsStore()
const { pendingClose: pending } = storeToRefs(store)

const saving = ref(false)

function resolve(action: 'save' | 'discard' | 'cancel') {
  void store.resolvePendingClose(action)
}

async function onSave() {
  saving.value = true
  try {
    await store.resolvePendingClose('save')
  } finally {
    saving.value = false
  }
}

function onOpenChange(open: boolean) {
  if (!open) resolve('cancel')
}
</script>
