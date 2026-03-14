<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div>
        <h3 class="text-lg font-semibold">
          {{ channelItem.meta.display_name }}
        </h3>
        <p class="text-sm text-muted-foreground">
          {{ channelItem.meta.type }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <template v-if="isEditMode">
          <Button
            variant="outline"
            size="sm"
            :disabled="isBusy"
            @click="handleToggleDisabled"
          >
            <Spinner
              v-if="action === 'toggle'"
              class="mr-1.5"
            />
            {{ form.disabled ? $t('bots.channels.actionEnable') : $t('bots.channels.actionDisable') }}
          </Button>
          <ConfirmPopover
            :message="$t('bots.channels.deleteConfirm')"
            :loading="action === 'delete'"
            @confirm="handleDelete"
          >
            <template #trigger>
              <Button
                variant="destructive"
                size="sm"
                :disabled="isBusy"
              >
                <Spinner
                  v-if="action === 'delete'"
                  class="mr-1.5"
                />
                {{ $t('common.delete') }}
              </Button>
            </template>
          </ConfirmPopover>
        </template>
      </div>
    </div>

    <div
      v-if="showWebhookCallback"
      class="space-y-2"
    >
      <h4 class="text-sm font-medium">
        {{ $t('bots.channels.webhookCallback') }}
      </h4>
      <p class="text-xs text-muted-foreground">
        {{ $t('bots.channels.webhookCallbackHint') }}
      </p>
      <template v-if="webhookCallbackUrl">
        <div class="flex gap-2">
          <Input
            :model-value="webhookCallbackUrl"
            readonly
            class="font-mono text-xs"
          />
          <Button
            variant="outline"
            size="sm"
            @click="copyWebhookCallback"
          >
            {{ $t('common.copy') }}
          </Button>
        </div>
      </template>
      <p
        v-else
        class="text-xs text-muted-foreground"
      >
        {{ $t('bots.channels.webhookCallbackPending') }}
      </p>
    </div>

    <Separator />

    <!-- Credentials form (dynamic from config_schema) -->
    <div class="space-y-4">
      <h4 class="text-sm font-medium">
        {{ $t('bots.channels.credentials') }}
      </h4>

      <div
        v-for="(field, key) in orderedFields"
        :key="key"
        class="space-y-2"
      >
        <Label :for="field.type === 'bool' || field.type === 'enum' ? undefined : `channel-field-${key}`">
          {{ field.title || key }}
          <span
            v-if="!field.required"
            class="text-xs text-muted-foreground ml-1"
          >({{ $t('common.optional') }})</span>
        </Label>
        <p
          v-if="field.description"
          class="text-xs text-muted-foreground"
        >
          {{ field.description }}
        </p>

        <!-- Secret field -->
        <div
          v-if="field.type === 'secret'"
          class="relative"
        >
          <Input
            :id="`channel-field-${key}`"
            :model-value="credentialStringValue(key)"
            :type="visibleSecrets[key] ? 'text' : 'password'"
            :placeholder="field.example ? String(field.example) : ''"
            @update:model-value="(val) => setCredentialStringValue(key, val)"
          />
          <button
            type="button"
            class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            :aria-label="`${visibleSecrets[key] ? 'Hide' : 'Show'} ${field.title || key}`"
            :aria-pressed="!!visibleSecrets[key]"
            @click="visibleSecrets[key] = !visibleSecrets[key]"
          >
            <FontAwesomeIcon
              :icon="['fas', visibleSecrets[key] ? 'eye-slash' : 'eye']"
              class="size-3.5"
            />
          </button>
        </div>

        <!-- Boolean field -->
        <Switch
          v-else-if="field.type === 'bool'"
          :model-value="!!form.credentials[key]"
          @update:model-value="(val) => form.credentials[key] = !!val"
        />

        <!-- Number field -->
        <Input
          v-else-if="field.type === 'number'"
          :id="`channel-field-${key}`"
          :model-value="credentialNumberValue(key)"
          type="number"
          :placeholder="field.example ? String(field.example) : ''"
          @update:model-value="(val) => setCredentialNumberValue(key, val)"
        />

        <!-- Enum field -->
        <Select
          v-else-if="field.type === 'enum' && field.enum"
          :model-value="String(form.credentials[key] || '')"
          @update:model-value="(val) => form.credentials[key] = val"
        >
          <SelectTrigger :aria-label="field.title || key">
            <SelectValue :placeholder="field.title" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem
              v-for="opt in field.enum"
              :key="opt"
              :value="opt"
            >
              {{ opt }}
            </SelectItem>
          </SelectContent>
        </Select>

        <!-- String field (default) -->
        <Input
          v-else
          :id="`channel-field-${key}`"
          :model-value="credentialStringValue(key)"
          type="text"
          :placeholder="field.example ? String(field.example) : ''"
          @update:model-value="(val) => setCredentialStringValue(key, val)"
        />
      </div>
      <template v-if="isMatrixChannel">
        <div class="space-y-2">
          <Label>{{ $t('bots.channels.matrixAutoJoinInvites') }}</Label>
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.channels.matrixAutoJoinInvitesHint') }}
          </p>
          <Switch
            :model-value="matrixAutoJoinInvites"
            @update:model-value="(val) => matrixAutoJoinInvites = !!val"
          />
        </div>

        <div class="space-y-2">
          <Label>{{ $t('bots.channels.matrixAllowedRooms') }}</Label>
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.channels.matrixAllowedRoomsHint') }}
          </p>
          <Textarea
            :model-value="matrixAllowedRoomsText"
            :placeholder="$t('bots.channels.matrixAllowedRoomsPlaceholder')"
            class="min-h-24"
            @update:model-value="(val) => matrixAllowedRoomsText = String(val || '')"
          />
        </div>
      </template>
    </div>

    <Separator />

    <div class="flex justify-end gap-2">
      <template v-if="isEditMode">
        <Button
          :disabled="isBusy"
          @click="handleEditSave"
        >
          <Spinner
            v-if="action === 'save'"
            class="mr-1.5"
          />
          {{ $t('common.save') }}
        </Button>
      </template>
      <template v-else>
        <Button
          variant="outline"
          :disabled="isBusy"
          @click="handleCreateSaveOnly"
        >
          <Spinner
            v-if="action === 'save'"
            class="mr-1.5"
          />
          {{ $t('bots.channels.saveOnly') }}
        </Button>
        <Button
          :disabled="isBusy"
          @click="handleCreateSaveAndEnable"
        >
          <Spinner
            v-if="action === 'save'"
            class="mr-1.5"
          />
          {{ $t('bots.channels.saveAndEnable') }}
        </Button>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Button,
  Input,
  Label,
  Separator,
  Switch,
  Spinner,
  Textarea,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@memoh/ui'
import { reactive, watch, computed, ref } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useMutation, useQueryCache } from '@pinia/colada'
import { putBotsByIdChannelByPlatform, deleteBotsByIdChannelByPlatform, patchBotsByIdChannelByPlatformStatus } from '@memoh/sdk'
import type { HandlersChannelMeta, ChannelChannelConfig, ChannelFieldSchema, ChannelUpsertConfigRequest } from '@memoh/sdk'
import { client } from '@memoh/sdk/client'
import ConfirmPopover from '@/components/confirm-popover/index.vue'

interface BotChannelItem {
  meta: HandlersChannelMeta
  config: ChannelChannelConfig | null
  configured: boolean
}

const props = defineProps<{
  botId: string
  channelItem: BotChannelItem
}>()

const emit = defineEmits<{
  saved: []
}>()

const { t } = useI18n()
const botIdRef = computed(() => props.botId)
const platformType = computed(() => String(props.channelItem.meta.type || '').trim())
const queryCache = useQueryCache()
const { mutateAsync: upsertChannel, isLoading } = useMutation({
  mutation: async ({ platform, data }: { platform: string; data: ChannelUpsertConfigRequest }) => {
    const { data: result } = await putBotsByIdChannelByPlatform({
      path: { id: botIdRef.value, platform },
      body: data,
      throwOnError: true,
    })
    return result
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['bot-channels', botIdRef.value] }),
})
const { mutateAsync: updateChannelStatus, isLoading: isStatusLoading } = useMutation({
  mutation: async ({ platform, disabled }: { platform: string; disabled: boolean }) => {
    const { data } = await patchBotsByIdChannelByPlatformStatus({
      path: { id: botIdRef.value, platform },
      body: { disabled },
      throwOnError: true,
    })
    return data
  },
  onSettled: () => queryCache.invalidateQueries({ key: ['bot-channels', botIdRef.value] }),
})
const action = ref<'save' | 'toggle' | 'delete' | ''>('')
const isBusy = computed(() => isLoading.value || isStatusLoading.value || action.value !== '')
const isEditMode = computed(() => props.channelItem.configured)
const lastSavedConfigId = ref('')

// ---- Form state ----

const form = reactive<{
  credentials: Record<string, unknown>
  disabled: boolean
}>({
  credentials: {},
  disabled: false,
})

const visibleSecrets = reactive<Record<string, boolean>>({})
const isMatrixChannel = computed(() => platformType.value === 'matrix')

// Schema fields sorted: required first. Exclude "status"/"disabled" from credential form.
const orderedFields = computed(() => {
  const fields = props.channelItem.meta.config_schema?.fields ?? {}
  const hiddenFields = new Set(['status', 'disabled'])
  if (isMatrixChannel.value) {
    hiddenFields.add('autoJoinInvites')
    hiddenFields.add('allowedRooms')
  }
  const entries = Object.entries(fields).filter(([key]) => !hiddenFields.has(key))
  entries.sort(([, a], [, b]) => {
    if (a.required && !b.required) return -1
    if (!a.required && b.required) return 1
    return 0
  })
  return Object.fromEntries(entries) as Record<string, ChannelFieldSchema>
})

const matrixAutoJoinInvites = computed({
  get: () => {
    const value = form.credentials.autoJoinInvites
    return typeof value === 'boolean' ? value : true
  },
  set: (value: boolean) => {
    form.credentials.autoJoinInvites = value
  },
})

const matrixAllowedRooms = computed({
  get: () => normalizeRoomTags(form.credentials.allowedRooms),
  set: (value: string[]) => {
    form.credentials.allowedRooms = normalizeRoomTags(value)
  },
})

const matrixAllowedRoomsText = computed({
  get: () => matrixAllowedRooms.value.join('\n'),
  set: (value: string) => {
    matrixAllowedRooms.value = normalizeRoomTags(value)
  },
})

const currentInboundMode = computed(() => {
  const value = form.credentials.inboundMode ?? form.credentials.inbound_mode
  if (typeof value !== 'string') return ''
  return value.trim().toLowerCase()
})

const showWebhookCallback = computed(() => {
  return props.channelItem.meta.type === 'feishu' && currentInboundMode.value === 'webhook'
})

const webhookCallbackUrl = computed(() => {
  if (!showWebhookCallback.value) return ''
  const configId = String(props.channelItem.config?.id || lastSavedConfigId.value || '').trim()
  if (!configId) return ''
  return buildWebhookCallbackUrl(configId)
})

function initForm() {
  const schema = props.channelItem.meta.config_schema?.fields ?? {}
  const existingCredentials = props.channelItem.config?.credentials ?? {}

  const creds: Record<string, unknown> = {}
  for (const [key, field] of Object.entries(schema)) {
    const existingValue = existingCredentials[key]
    if (existingValue !== undefined) {
      creds[key] = existingValue
      continue
    }
    if (key === 'autoJoinInvites' && isMatrixChannel.value) {
      creds[key] = true
      continue
    }
    if (key === 'allowedRooms' && isMatrixChannel.value) {
      creds[key] = []
      continue
    }
    if (field.type === 'bool') {
      creds[key] = false
      continue
    }
    creds[key] = ''
  }
  if (isMatrixChannel.value) {
    creds.autoJoinInvites = typeof existingCredentials.autoJoinInvites === 'boolean'
      ? existingCredentials.autoJoinInvites
      : true
    creds.allowedRooms = normalizeRoomTags(existingCredentials.allowedRooms)
  }
  form.credentials = creds
  form.disabled = props.channelItem.config?.disabled ?? false
  lastSavedConfigId.value = String(props.channelItem.config?.id || '').trim()
}

watch(
  () => props.channelItem,
  () => initForm(),
  { immediate: true },
)

function validateRequired(): boolean {
  const schema = props.channelItem.meta.config_schema?.fields ?? {}
  for (const [key, field] of Object.entries(schema)) {
    if (field.required) {
      const val = form.credentials[key]
      if (!val || (typeof val === 'string' && val.trim() === '')) {
        toast.error(t('bots.channels.requiredField', { field: field.title || key }))
        return false
      }
    }
  }
  return true
}

function buildCredentials(): Record<string, unknown> {
  const credentials: Record<string, unknown> = {}
  for (const [key, val] of Object.entries(form.credentials)) {
    if (key === 'status' || key === 'disabled') continue
    if (key === 'allowedRooms') {
      const rooms = normalizeRoomTags(val)
      if (rooms.length > 0) {
        credentials[key] = rooms
      }
      continue
    }
    if (val === '' || val === undefined || val === null) continue
    credentials[key] = val
  }
  return credentials
}

function credentialStringValue(key: string): string | number | undefined {
  const value = form.credentials[key]
  if (typeof value === 'string' || typeof value === 'number') {
    return value
  }
  return undefined
}

function setCredentialStringValue(key: string, value: string | number) {
  form.credentials[key] = value
}

function credentialNumberValue(key: string): string | number | undefined {
  const value = form.credentials[key]
  if (typeof value === 'number' || typeof value === 'string') {
    return value
  }
  return undefined
}

function setCredentialNumberValue(key: string, value: string | number) {
  if (value === '') {
    form.credentials[key] = ''
    return
  }
  const numericValue = typeof value === 'number' ? value : Number(value)
  form.credentials[key] = Number.isNaN(numericValue) ? '' : numericValue
}

function normalizeRoomTags(value: unknown): string[] {
  const items = Array.isArray(value)
    ? value
    : typeof value === 'string'
      ? value.split(/\r?\n/)
      : []
  const normalized: string[] = []
  for (const item of items) {
    const room = String(item || '').trim()
    if (!room || normalized.includes(room)) continue
    normalized.push(room)
  }
  return normalized
}

async function saveChannel(disabled: boolean, nextAction: 'save' | 'toggle') {
  if (!validateRequired()) return
  action.value = nextAction
  try {
    const result = await upsertChannel({
      platform: platformType.value,
      data: {
        credentials: buildCredentials(),
        disabled,
      },
    })
    lastSavedConfigId.value = String(result?.id || lastSavedConfigId.value || '').trim()
    form.disabled = disabled
    toast.success(t('bots.channels.saveSuccess'))
    emit('saved')
  } catch (err) {
    let detail = ''
    if (err instanceof Error) {
      detail = err.message
    }
    toast.error(detail ? `${t('bots.channels.saveFailed')}: ${detail}` : t('bots.channels.saveFailed'))
  } finally {
    action.value = ''
  }
}

async function handleCreateSaveOnly() {
  await saveChannel(true, 'save')
}

async function handleCreateSaveAndEnable() {
  await saveChannel(false, 'save')
}

async function handleEditSave() {
  await saveChannel(form.disabled, 'save')
}

async function handleToggleDisabled() {
  action.value = 'toggle'
  try {
    const nextDisabled = !form.disabled
    const result = await updateChannelStatus({
      platform: platformType.value,
      disabled: nextDisabled,
    })
    form.disabled = !!result?.disabled
    toast.success(t('bots.channels.saveSuccess'))
    emit('saved')
  } catch (err) {
    const detail = err instanceof Error ? err.message : ''
    toast.error(detail ? `${t('bots.channels.saveFailed')}: ${detail}` : t('bots.channels.saveFailed'))
  } finally {
    action.value = ''
  }
}

async function handleDelete() {
  action.value = 'delete'
  try {
    await deleteBotsByIdChannelByPlatform({
      path: { id: botIdRef.value, platform: platformType.value },
      throwOnError: true,
    })
    lastSavedConfigId.value = ''
    toast.success(t('bots.channels.deleteSuccess'))
    emit('saved')
  } catch (err) {
    const detail = err instanceof Error ? err.message : ''
    toast.error(detail ? `${t('bots.channels.deleteFailed')}: ${detail}` : t('bots.channels.deleteFailed'))
  } finally {
    action.value = ''
  }
}

function buildWebhookCallbackUrl(configId: string): string {
  const normalizedBase = resolveWebhookCallbackBaseUrl()
  if (!normalizedBase) return ''
  if (typeof window !== 'undefined') {
    const baseUrl = new URL(normalizedBase, window.location.origin)
    baseUrl.pathname = `${baseUrl.pathname.replace(/\/+$/, '')}/channels/feishu/webhook/${encodeURIComponent(configId)}`
    baseUrl.search = ''
    baseUrl.hash = ''
    return baseUrl.toString()
  }
  const base = normalizedBase.replace(/\/+$/, '')
  return `${base}/channels/feishu/webhook/${encodeURIComponent(configId)}`
}

function resolveWebhookCallbackBaseUrl(): string {
  const explicitRaw = String(
    import.meta.env.VITE_WEBHOOK_PUBLIC_BASE_URL?.trim() ||
    import.meta.env.VITE_API_PUBLIC_URL?.trim() ||
    '',
  ).trim()
  if (isAbsoluteHttpUrl(explicitRaw)) {
    return explicitRaw
  }

  const apiBase = String(client.getConfig().baseUrl || import.meta.env.VITE_API_URL?.trim() || '').trim()
  if (isAbsoluteHttpUrl(apiBase)) {
    return apiBase
  }

  if (typeof window === 'undefined') {
    return ''
  }
  const fallbackApiPort = String(import.meta.env.VITE_API_PORT || '8080').trim()
  const fallback = new URL(window.location.origin)
  if (fallbackApiPort) {
    fallback.port = fallbackApiPort
  }
  fallback.search = ''
  fallback.hash = ''
  return fallback.toString().replace(/\/+$/, '')
}

function isAbsoluteHttpUrl(value: string): boolean {
  return /^https?:\/\//i.test(value)
}

async function copyWebhookCallback() {
  if (!webhookCallbackUrl.value) return
  try {
    if (typeof navigator !== 'undefined' && navigator.clipboard) {
      await navigator.clipboard.writeText(webhookCallbackUrl.value)
      toast.success(t('common.copied'))
      return
    }
    toast.error(t('bots.channels.copyFailed'))
  } catch (err) {
    const detail = err instanceof Error ? err.message : ''
    toast.error(detail ? `${t('bots.channels.copyFailed')}: ${detail}` : t('bots.channels.copyFailed'))
  }
}
</script>
