<template>
  <section>
    <h2 class="mb-2 flex items-center text-base font-semibold">
      <FontAwesomeIcon
        :icon="['fas', 'user']"
        class="mr-2"
      />
      {{ $t('settings.userProfile') }}
    </h2>
    <Separator />
    <div class="mt-4 space-y-4">
      <div class="space-y-2">
        <Label for="settings-user-id">{{ $t('settings.userID') }}</Label>
        <Input
          id="settings-user-id"
          :model-value="displayUserId"
          :aria-label="$t('settings.userID')"
          readonly
        />
      </div>
      <div class="space-y-2">
        <Label for="settings-username">{{ $t('auth.username') }}</Label>
        <Input
          id="settings-username"
          :model-value="displayUsername"
          :aria-label="$t('auth.username')"
          readonly
        />
      </div>
      <div class="space-y-2">
        <Label for="settings-display-name">{{ $t('settings.displayName') }}</Label>
        <Input
          id="settings-display-name"
          :model-value="displayName"
          :aria-label="$t('settings.displayName')"
          @update:model-value="onDisplayNameChange"
        />
      </div>
      <div class="space-y-2">
        <Label for="settings-avatar-url">{{ $t('settings.avatarUrl') }}</Label>
        <Input
          id="settings-avatar-url"
          :model-value="avatarUrl"
          type="url"
          :aria-label="$t('settings.avatarUrl')"
          @update:model-value="onAvatarUrlChange"
        />
      </div>
      <div class="flex justify-end">
        <Button
          :disabled="saving || loading"
          @click="emit('save')"
        >
          <Spinner v-if="saving" />
          {{ $t('settings.saveProfile') }}
        </Button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { Button, Input, Label, Separator, Spinner } from '@memoh/ui'

defineProps<{
  displayUserId: string
  displayUsername: string
  displayName: string
  avatarUrl: string
  saving: boolean
  loading: boolean
}>()

const emit = defineEmits<{
  'update:displayName': [value: string]
  'update:avatarUrl': [value: string]
  save: []
}>()

function onDisplayNameChange(value: string | number) {
  emit('update:displayName', String(value))
}

function onAvatarUrlChange(value: string | number) {
  emit('update:avatarUrl', String(value))
}
</script>
