<template>
  <section>
    <h2 class="mb-2 flex items-center text-base font-semibold">
      <FontAwesomeIcon
        :icon="['fas', 'gear']"
        class="mr-2"
      />
      {{ $t('settings.changePassword') }}
    </h2>
    <Separator />
    <div class="mt-4 space-y-4">
      <div class="space-y-2">
        <Label for="settings-current-password">{{ $t('settings.currentPassword') }}</Label>
        <Input
          id="settings-current-password"
          :model-value="currentPassword"
          type="password"
          :aria-label="$t('settings.currentPassword')"
          @update:model-value="onCurrentPasswordChange"
        />
      </div>
      <div class="space-y-2">
        <Label for="settings-new-password">{{ $t('settings.newPassword') }}</Label>
        <Input
          id="settings-new-password"
          :model-value="newPassword"
          type="password"
          :aria-label="$t('settings.newPassword')"
          @update:model-value="onNewPasswordChange"
        />
      </div>
      <div class="space-y-2">
        <Label for="settings-confirm-password">{{ $t('settings.confirmPassword') }}</Label>
        <Input
          id="settings-confirm-password"
          :model-value="confirmPassword"
          type="password"
          :aria-label="$t('settings.confirmPassword')"
          @update:model-value="onConfirmPasswordChange"
        />
      </div>
      <div class="flex justify-end">
        <Button
          :disabled="saving || loading"
          @click="emit('updatePassword')"
        >
          <Spinner v-if="saving" />
          {{ $t('settings.updatePassword') }}
        </Button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { Button, Input, Label, Separator, Spinner } from '@memoh/ui'

defineProps<{
  currentPassword: string
  newPassword: string
  confirmPassword: string
  saving: boolean
  loading: boolean
}>()

const emit = defineEmits<{
  'update:currentPassword': [value: string]
  'update:newPassword': [value: string]
  'update:confirmPassword': [value: string]
  updatePassword: []
}>()

function onCurrentPasswordChange(value: string | number) {
  emit('update:currentPassword', String(value))
}

function onNewPasswordChange(value: string | number) {
  emit('update:newPassword', String(value))
}

function onConfirmPasswordChange(value: string | number) {
  emit('update:confirmPassword', String(value))
}
</script>
