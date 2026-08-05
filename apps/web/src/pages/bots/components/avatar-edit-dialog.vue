<template>
  <Dialog v-model:open="open">
    <DialogContent class="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>{{ $t('bots.editAvatar') }}</DialogTitle>
        <DialogDescription>
          {{ $t('bots.editAvatarDescription') }}
        </DialogDescription>
      </DialogHeader>
      <div class="mt-4 flex flex-col items-center gap-4">
        <Avatar class="size-20 shrink-0 rounded-full">
          <AvatarImage
            v-if="draft.trim()"
            :src="resolveAvatarUrl(draft)"
            :alt="fallbackText"
          />
          <AvatarFallback class="text-xl">
            {{ fallbackText }}
          </AvatarFallback>
        </Avatar>
        <input
          ref="fileInput"
          type="file"
          class="hidden"
          :accept="AVATAR_ACCEPT"
          @change="onFileSelected"
        >
        <Button
          type="button"
          variant="outline"
          class="w-full"
          @click="fileInput?.click()"
        >
          <ImageUp class="size-4" />
          {{ $t('common.uploadImage') }}
        </Button>
        <Input
          v-model="draft"
          type="url"
          class="w-full"
          :placeholder="$t('bots.avatarUrlPlaceholder')"
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
  toast,
} from '@felinic/ui'
import { ImageUp } from 'lucide-vue-next'
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AVATAR_ACCEPT, avatarUploadErrorKey, readAvatarFile } from '@/lib/avatar-upload'
import { resolveAvatarUrl } from '@/lib/avatar-url'

withDefaults(defineProps<{
  fallbackText?: string
}>(), {
  fallbackText: '',
})

const open = defineModel<boolean>('open', { default: false })
const avatarUrl = defineModel<string>('avatarUrl', { default: '' })

const { t } = useI18n()

const draft = ref('')
const fileInput = ref<HTMLInputElement | null>(null)

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

async function onFileSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  try {
    draft.value = await readAvatarFile(file)
  } catch (error) {
    toast.error(t(avatarUploadErrorKey(error)))
  }
}
</script>
