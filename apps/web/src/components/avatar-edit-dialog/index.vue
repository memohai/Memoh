<template>
  <Dialog v-model:open="open">
    <DialogContent class="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>{{ title ?? $t('common.editAvatar') }}</DialogTitle>
        <DialogDescription>
          {{ description ?? $t('common.editAvatarDescription') }}
        </DialogDescription>
      </DialogHeader>
      <div class="mt-4 flex flex-col items-center gap-4">
        <Avatar class="size-20 shrink-0 rounded-full">
          <AvatarImage
            v-if="draft.trim()"
            :src="draft.trim()"
            :alt="fallbackText"
          />
          <AvatarFallback class="text-xl">
            {{ fallbackText }}
          </AvatarFallback>
        </Avatar>
        <Input
          v-model="draft"
          type="url"
          class="w-full"
          :placeholder="placeholder ?? $t('common.avatarUrlPlaceholder')"
        />
      </div>
      <DialogFooter class="mt-6">
        <DialogClose as-child>
          <Button variant="outline">
            {{ $t('common.cancel') }}
          </Button>
        </DialogClose>
        <Button
          :disabled="!canConfirm"
          @click="handleConfirm"
        >
          {{ $t('common.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import {
  Avatar,
  AvatarImage,
  AvatarFallback,
  Button,
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
} from '@memohai/ui'
import { ref, computed, watch } from 'vue'

withDefaults(defineProps<{
  fallbackText?: string
  title?: string
  description?: string
  placeholder?: string
}>(), {
  fallbackText: '',
  title: undefined,
  description: undefined,
  placeholder: undefined,
})

const open = defineModel<boolean>('open', { default: false })
const avatarUrl = defineModel<string>('avatarUrl', { default: '' })

const draft = ref('')

const canConfirm = computed(() => {
  const next = draft.value.trim()
  const current = (avatarUrl.value || '').trim()
  return next !== current
})

watch(open, (val) => {
  if (val) {
    draft.value = avatarUrl.value || ''
  }
})

function handleConfirm() {
  if (!canConfirm.value) return
  avatarUrl.value = draft.value.trim()
  open.value = false
}
</script>
