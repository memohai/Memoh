<template>
  <section>
    <h2 class="mb-2 flex items-center text-base font-semibold">
      <FontAwesomeIcon
        :icon="['fas', 'plug']"
        class="mr-2"
      />
      {{ $t('settings.bindCode') }}
    </h2>
    <Separator />
    <div class="mt-4 space-y-4">
      <div class="flex flex-wrap gap-3 items-end">
        <div class="space-y-2">
          <Label>{{ $t('settings.platform') }}</Label>
          <Select
            :model-value="platform || anyPlatformValue"
            @update:model-value="onPlatformChange"
          >
            <SelectTrigger
              class="w-56"
              :aria-label="$t('settings.platform')"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem :value="anyPlatformValue">
                  {{ $t('settings.platformAny') }}
                </SelectItem>
                <SelectItem
                  v-for="platformOption in platformOptions"
                  :key="platformOption"
                  :value="platformOption"
                >
                  {{ platformLabel(platformOption) }}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
        <div class="space-y-2">
          <Label for="settings-bind-ttl">{{ $t('settings.bindCodeTTL') }}</Label>
          <Input
            id="settings-bind-ttl"
            :model-value="ttlSeconds"
            type="number"
            min="60"
            class="w-40"
            :aria-label="$t('settings.bindCodeTTL')"
            @update:model-value="onTtlChange"
          />
        </div>
        <Button
          :disabled="generating || loading"
          @click="emit('generate')"
        >
          <Spinner v-if="generating" />
          {{ $t('settings.generateBindCode') }}
        </Button>
      </div>
      <div
        v-if="bindCode"
        class="space-y-2"
      >
        <Label for="settings-bind-code-value">{{ $t('settings.bindCodeValue') }}</Label>
        <div class="flex gap-2">
          <Input
            id="settings-bind-code-value"
            :model-value="bindCode.token"
            :aria-label="$t('settings.bindCodeValue')"
            readonly
          />
          <Button
            variant="outline"
            @click="emit('copy')"
          >
            {{ $t('settings.copyBindCode') }}
          </Button>
        </div>
        <p class="text-xs text-muted-foreground">
          {{ $t('settings.bindCodeExpiresAt') }}: {{ formatDate(bindCode.expires_at) }}
        </p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import {
  Button,
  Input,
  Label,
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Separator,
  Spinner,
} from '@memoh/ui'

interface BindCodeValue {
  token: string
  expires_at: string
}

defineProps<{
  anyPlatformValue: string
  platform: string
  platformOptions: string[]
  ttlSeconds: number
  generating: boolean
  loading: boolean
  bindCode: BindCodeValue | null
  platformLabel: (value: string) => string
  formatDate: (value: string) => string
}>()

const emit = defineEmits<{
  'update:platform': [value: string]
  'update:ttlSeconds': [value: number]
  generate: []
  copy: []
}>()

function onPlatformChange(value: string | number) {
  emit('update:platform', String(value))
}

function onTtlChange(value: string | number) {
  const next = Number(value)
  emit('update:ttlSeconds', Number.isFinite(next) ? next : 0)
}
</script>
