<template>
  <div class="rounded-md border bg-background shadow-sm mt-6">
    <div class="p-4 md:p-6 pb-4">
      <h2 class="text-sm font-medium">
        {{ $t('settings.changePassword') }}
      </h2>
      <p class="text-xs text-muted-foreground mt-1">
        Update your credentials to keep your account secure.
      </p>
    </div>

    <div class="p-4 md:p-6 space-y-5">
      <!-- Current Password -->
      <div class="flex items-center justify-between">
        <div class="pr-4 shrink-0">
          <Label
            for="settings-current-password"
            class="text-[11px] font-medium text-muted-foreground"
          >{{ $t('settings.currentPassword') }}</Label>
        </div>
        <Input
          id="settings-current-password"
          :model-value="currentPassword"
          type="password"
          :aria-label="$t('settings.currentPassword')"
          class="h-9 w-full max-w-[240px] md:max-w-xs bg-background/50 border-border/50 shadow-none transition-colors focus-visible:ring-ring/30"
          @update:model-value="onCurrentPasswordChange"
        />
      </div>

      <!-- New Password -->
      <div class="flex items-center justify-between">
        <div class="pr-4 shrink-0">
          <Label
            for="settings-new-password"
            class="text-[11px] font-medium text-muted-foreground"
          >{{ $t('settings.newPassword') }}</Label>
        </div>
        <Input
          id="settings-new-password"
          :model-value="newPassword"
          type="password"
          :aria-label="$t('settings.newPassword')"
          :class="[
            'h-9 w-full max-w-[240px] md:max-w-xs bg-background/50 border-border/50 shadow-none transition-colors',
            isMismatch ? 'border-destructive focus-visible:ring-destructive/30' : 'focus-visible:ring-ring/30'
          ]"
          @update:model-value="onNewPasswordChange"
        />
      </div>

      <!-- Confirm Password -->
      <div class="flex items-center justify-between">
        <div class="pr-4 shrink-0">
          <Label
            for="settings-confirm-password"
            class="text-[11px] font-medium text-muted-foreground"
          >{{ $t('settings.confirmPassword') }}</Label>
        </div>
        <Input
          id="settings-confirm-password"
          :model-value="confirmPassword"
          type="password"
          :aria-label="$t('settings.confirmPassword')"
          :class="[
            'h-9 w-full max-w-[240px] md:max-w-xs bg-background/50 border-border/50 shadow-none transition-colors',
            isMismatch ? 'border-destructive focus-visible:ring-destructive/30' : 'focus-visible:ring-ring/30'
          ]"
          @update:model-value="onConfirmPasswordChange"
        />
      </div>
    </div>
    
    <!-- Action Anchor -->
    <div class="flex items-center justify-end p-4 md:px-6 bg-muted/10">
      <Button
        size="sm"
        :disabled="saving || loading || !hasInput || isMismatch"
        @click="emit('updatePassword')"
      >
        <Spinner
          v-if="saving"
          class="mr-2 size-3.5"
        />
        {{ $t('settings.updatePassword') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Button, Input, Label, Spinner } from '@memohai/ui'

const props = defineProps<{
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

const hasInput = computed(() => {
  return props.currentPassword.length > 0 && props.newPassword.length > 0 && props.confirmPassword.length > 0
})

const isMismatch = computed(() => {
  return props.newPassword.length > 0 && 
         props.confirmPassword.length > 0 && 
         props.newPassword !== props.confirmPassword
})

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
