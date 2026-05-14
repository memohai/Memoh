<template>
  <div class="rounded-md border bg-background shadow-sm overflow-hidden">
    <div class="p-4 md:p-6 space-y-5">
      <!-- Readonly: User ID -->
      <div class="flex items-center justify-between">
        <div class="pr-4 shrink-0">
          <Label class="text-[11px] font-medium text-muted-foreground">{{ $t('settings.userID') }}</Label>
        </div>
        <div class="flex h-9 w-full max-w-[240px] md:max-w-xs items-center justify-between rounded-lg border border-border/50 bg-background/50 px-3 text-xs text-muted-foreground transition-colors hover:bg-muted/10">
          <span class="whitespace-nowrap">{{ displayUserId }}</span>
          <button
            class="text-muted-foreground hover:text-foreground transition-colors ml-2 shrink-0"
            @click="copyToClipboard(displayUserId, false)"
          >
            <Copy
              v-if="!copiedId"
              class="size-3.5"
            />
            <Check
              v-else
              class="size-3.5"
            />
          </button>
        </div>
      </div>

      <!-- Readonly: Username -->
      <div class="flex items-center justify-between">
        <div class="pr-4 shrink-0">
          <Label class="text-[11px] font-medium text-muted-foreground">{{ $t('auth.username') }}</Label>
        </div>
        <div class="flex h-9 w-full max-w-[240px] md:max-w-xs items-center justify-between rounded-lg border border-border/50 bg-background/50 px-3 text-xs text-muted-foreground transition-colors hover:bg-muted/10">
          <span class="whitespace-nowrap">{{ displayUsername }}</span>
          <button
            class="text-muted-foreground hover:text-foreground transition-colors ml-2 shrink-0"
            @click="copyToClipboard(displayUsername, true)"
          >
            <Copy
              v-if="!copiedUsername"
              class="size-3.5"
            />
            <Check
              v-else
              class="size-3.5"
            />
          </button>
        </div>
      </div>
      <!-- Flush Input: Display Name -->
      <div class="flex items-center justify-between">
        <div class="pr-4 shrink-0">
          <Label
            for="settings-display-name"
            class="text-[11px] font-medium text-muted-foreground"
          >{{ $t('settings.displayName') }}</Label>
        </div>
        <Input
          id="settings-display-name"
          :model-value="displayName"
          :aria-label="$t('settings.displayName')"
          class="h-9 w-full max-w-[240px] md:max-w-xs bg-background/50 border-border/50 shadow-none transition-colors focus-visible:ring-ring/30"
          @update:model-value="onDisplayNameChange"
        />
      </div>

      <!-- Flush Input: Avatar URL -->
      <div class="flex items-center justify-between">
        <div class="pr-4 shrink-0">
          <Label
            for="settings-avatar-url"
            class="text-[11px] font-medium text-muted-foreground"
          >{{ $t('settings.avatarUrl') }}</Label>
        </div>
        <Input
          id="settings-avatar-url"
          :model-value="avatarUrl"
          type="url"
          :aria-label="$t('settings.avatarUrl')"
          class="h-9 w-full max-w-[240px] md:max-w-xs bg-background/50 border-border/50 shadow-none transition-colors focus-visible:ring-ring/30"
          @update:model-value="onAvatarUrlChange"
        />
      </div>

      <!-- Timezone -->
      <div class="flex items-center justify-between">
        <div class="pr-4 shrink-0">
          <Label
            for="settings-timezone"
            class="text-[11px] font-medium text-muted-foreground"
          >{{ $t('settings.timezone') }}</Label>
        </div>
        <div class="w-full max-w-[240px] md:max-w-xs">
          <TimezoneSelect
            :model-value="timezone"
            :placeholder="$t('settings.timezonePlaceholder')"
            @update:model-value="onTimezoneChange"
          />
        </div>
      </div>
    </div>

    <!-- Action Anchor (Right-aligned) -->
    <div class="flex items-center justify-end gap-3 p-4 md:px-6 bg-muted/10">
      <Button
        v-if="isDirty"
        variant="ghost"
        size="sm"
        :disabled="saving || loading"
        @click="emit('discard')"
      >
        Discard
      </Button>
      <Button
        size="sm"
        :disabled="!isDirty || saving || loading"
        @click="emit('save')"
      >
        <Spinner
          v-if="saving"
          class="mr-2 size-3.5"
        />
        {{ $t('settings.saveProfile') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import {
  Button,
  Input,
  Label,
  Spinner,
} from '@memohai/ui'
import { Copy, Check } from 'lucide-vue-next'
import TimezoneSelect from '@/components/timezone-select/index.vue'

defineProps<{
  displayUserId: string
  displayUsername: string
  displayName: string
  avatarUrl: string
  timezone: string
  saving: boolean
  loading: boolean
  isDirty: boolean
}>()

const emit = defineEmits<{
  'update:displayName': [value: string]
  'update:avatarUrl': [value: string]
  'update:timezone': [value: string]
  save: []
  discard: []
}>()

const copiedId = ref(false)
const copiedUsername = ref(false)

function copyToClipboard(text: string, isUser = false) {
  navigator.clipboard.writeText(text)
  if (isUser) {
    copiedUsername.value = true
    setTimeout(() => copiedUsername.value = false, 2000)
  } else {
    copiedId.value = true
    setTimeout(() => copiedId.value = false, 2000)
  }
}

function onDisplayNameChange(value: string | number) {
  emit('update:displayName', String(value))
}

function onAvatarUrlChange(value: string | number) {
  emit('update:avatarUrl', String(value))
}

function onTimezoneChange(value: string | number | undefined) {
  emit('update:timezone', String(value || 'UTC'))
}
</script>
