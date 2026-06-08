<template>
  <Dialog
    :open="open"
    @update:open="$emit('update:open', $event)"
  >
    <DialogTrigger as-child>
      <Button
        variant="outline"
        size="sm"
      >
        {{ $t('settings.changePassword') }}
      </Button>
    </DialogTrigger>
    <DialogContent :show-close-button="false">
      <DialogHeader>
        <DialogTitle>{{ $t('settings.changePassword') }}</DialogTitle>
      </DialogHeader>

      <div class="space-y-4">
        <div class="space-y-1.5">
          <Label for="pw-current">
            {{ $t('settings.currentPassword') }}
          </Label>
          <Input
            id="pw-current"
            v-model="currentPassword"
            type="password"
            autocomplete="current-password"
          />
        </div>
        <div class="space-y-1.5">
          <Label for="pw-new">
            {{ $t('settings.newPassword') }}
          </Label>
          <Input
            id="pw-new"
            v-model="newPassword"
            type="password"
            autocomplete="new-password"
            :aria-invalid="isMismatch || undefined"
          />
        </div>
        <div class="space-y-1.5">
          <Label for="pw-confirm">
            {{ $t('settings.confirmPassword') }}
          </Label>
          <Input
            id="pw-confirm"
            v-model="confirmPassword"
            type="password"
            autocomplete="new-password"
            :aria-invalid="isMismatch || undefined"
          />
          <p
            v-if="isMismatch"
            class="text-xs text-destructive"
          >
            {{ $t('settings.passwordNotMatch') }}
          </p>
        </div>
      </div>

      <DialogFooter>
        <DialogClose as-child>
          <Button variant="outline">
            {{ $t('common.cancel') }}
          </Button>
        </DialogClose>
        <Button
          :disabled="!hasInput || isMismatch || saving"
          @click="onSubmit"
        >
          <Spinner
            v-if="saving"
            class="mr-2 size-3.5"
          />
          {{ $t('settings.updatePassword') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  Button,
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  Input,
  Label,
  Spinner,
} from '@memohai/ui'

const props = defineProps<{
  open: boolean
  saving: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  submit: [payload: { currentPassword: string, newPassword: string }]
}>()

const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')

const hasInput = computed(() =>
  currentPassword.value.length > 0
  && newPassword.value.length > 0
  && confirmPassword.value.length > 0,
)

const isMismatch = computed(() =>
  newPassword.value.length > 0
  && confirmPassword.value.length > 0
  && newPassword.value !== confirmPassword.value,
)

watch(() => props.open, (open) => {
  if (!open) {
    currentPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
  }
})

function onSubmit() {
  if (!hasInput.value || isMismatch.value) return
  emit('submit', {
    currentPassword: currentPassword.value,
    newPassword: newPassword.value,
  })
}
</script>
