<template>
  <div class="max-w-2xl mx-auto pb-6 space-y-4">
    <!-- Top Action Bar -->
    <div class="flex items-center justify-between pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-10 pt-1">
      <div class="flex items-center gap-3">
        <span class="flex size-10 shrink-0 items-center justify-center rounded-lg border bg-muted/30 text-muted-foreground shadow-sm">
          <ChannelIcon
            :channel="platformType"
            size="1.5em"
          />
        </span>
        <div class="space-y-0.5">
          <h3 class="text-sm font-semibold text-foreground flex items-center gap-2">
            {{ channelTitle }}
          </h3>
          <p class="text-[11px] text-muted-foreground font-mono">
            {{ platformKeyLine }}
          </p>
        </div>
      </div>
      
      <!-- Actions: Status + Save -->
      <div class="flex items-center gap-3 shrink-0">
        <!-- Dynamic context micro-copy -->
        <Transition name="fade">
          <div
            v-if="dirtyPrompt"
            class="flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-muted/40 border border-border/50"
          >
            <div class="size-1 rounded-full bg-muted-foreground/40" />
            <span class="text-[10px] text-muted-foreground font-medium whitespace-nowrap">
              Unsaved
              <template v-if="dirtyPrompt.type === 'other'">
                in <a
                  href="#"
                  class="underline hover:text-foreground font-medium"
                  @click.prevent="$emit('switch-tab', dirtyPrompt.channel)"
                >{{ dirtyPrompt.channelName }}</a>
              </template>
            </span>
          </div>
        </Transition>

        <template v-if="isEditMode">
          <Button
            variant="outline"
            size="sm"
            :disabled="isBusy"
            class="h-8 text-xs font-medium shadow-none"
            @click="handleToggleDisabled"
          >
            <Spinner
              v-if="action === 'toggle'"
              class="mr-1.5 size-3"
            />
            {{ form.disabled ? $t('bots.channels.actionEnable') : $t('bots.channels.actionDisable') }}
          </Button>
          <div
            class="w-px h-4 bg-border mx-1"
            aria-hidden="true"
          />
        </template>

        <Button 
          size="sm" 
          :disabled="(!isFormDirty && isEditMode) || isBusy" 
          class="h-8 text-xs font-medium min-w-24 shadow-none" 
          @click="handleSave"
        >
          <Spinner
            v-if="action === 'save'"
            class="mr-1.5 size-3"
          />
          {{ $t('bots.settings.save') }}
        </Button>
      </div>
    </div>

    <!-- WeChat Configuration Area -->
    <div
      v-if="channelItem.meta.type === 'weixin'"
      class="pt-2"
    >
      <WeixinQrLogin
        :bot-id="botId"
        @login-success="handleWeixinLoginSuccess"
      />
    </div>

    <!-- Standard Form Body -->
    <div
      v-else
      class="space-y-4"
    >
      <!-- Webhook URL (Read-only mode) -->
      <div
        v-if="showWebhookCallback"
        class="rounded-md border border-border bg-background p-4 shadow-none space-y-4"
      >
        <div class="space-y-1">
          <h4 class="text-xs font-medium">
            {{ $t('bots.channels.webhookCallback') }}
          </h4>
          <p class="text-[11px] text-muted-foreground">
            {{ $t('bots.channels.webhookCallbackHint') }}
          </p>
        </div>
        <div
          v-if="webhookCallbackUrl"
          class="flex gap-2"
        >
          <Input
            :model-value="webhookCallbackUrl"
            readonly
            class="font-mono text-[11px] h-8 shadow-none"
          />
          <Button
            variant="outline"
            size="sm"
            class="h-8 text-xs shadow-none"
            @click="copyWebhookCallback"
          >
            {{ $t('common.copy') }}
          </Button>
        </div>
        <p
          v-else
          class="text-[11px] text-muted-foreground italic"
        >
          {{ $t('bots.channels.webhookCallbackPending') }}
        </p>
      </div>

      <!-- Required Core Area (Core Credentials) -->
      <div
        v-if="requiredFieldsKeys.length > 0"
        class="rounded-md border border-border bg-background p-4 shadow-none space-y-4"
      >
        <div class="space-y-1">
          <h4 class="text-xs font-medium">
            {{ $t('bots.channels.credentials') }}
          </h4>
          <p
            v-if="showWebhookCallback"
            class="text-[11px] text-muted-foreground text-warning/80"
          >
            {{ $t('bots.channels.feishuWebhookSecurityHint') }}
          </p>
        </div>
        <div class="grid gap-4 md:grid-cols-2 pt-2">
          <div
            v-for="key in requiredFieldsKeys"
            :key="key"
            class="space-y-1.5"
            :class="isWideChannelField(orderedFields[key], key) ? 'md:col-span-2' : ''"
          >
            <Label
              :for="`channel-field-${key}`"
              class="text-xs font-medium"
            >{{ orderedFields[key].title || key }}</Label>
            <p
              v-if="orderedFields[key].description"
              class="text-[11px] text-muted-foreground leading-tight"
            >
              {{ orderedFields[key].description }}
            </p>
            
            <!-- Dynamic Field Render -->
            <div
              v-if="orderedFields[key].type === 'secret'"
              class="relative"
            >
              <Input
                :id="`channel-field-${key}`"
                :model-value="credentialStringValue(key)"
                :type="visibleSecrets[key] ? 'text' : 'password'"
                :placeholder="orderedFields[key].example ? String(orderedFields[key].example) : ''"
                class="h-8 text-xs shadow-none pr-8"
                @update:model-value="(val) => setCredentialStringValue(key, val)"
              />
              <button
                type="button"
                class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                @click="toggleSecret(key)"
              >
                <component
                  :is="visibleSecrets[key] ? EyeOff : Eye"
                  class="size-3.5"
                />
              </button>
            </div>
            <Switch
              v-else-if="orderedFields[key].type === 'bool'"
              :model-value="!!form.credentials[key]"
              @update:model-value="(val) => form.credentials[key] = !!val"
            />
            <Input
              v-else-if="orderedFields[key].type === 'number'"
              :id="`channel-field-${key}`"
              :model-value="credentialNumberValue(key)"
              type="number"
              :placeholder="orderedFields[key].example ? String(orderedFields[key].example) : ''"
              class="h-8 text-xs shadow-none"
              @update:model-value="(val) => setCredentialNumberValue(key, val)"
            />
            <Select
              v-else-if="orderedFields[key].type === 'enum' && orderedFields[key].enum"
              :model-value="String(form.credentials[key] || '')"
              @update:model-value="(val) => form.credentials[key] = val"
            >
              <SelectTrigger class="h-8 text-xs shadow-none">
                <SelectValue :placeholder="orderedFields[key].title || key" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem
                  v-for="opt in orderedFields[key].enum"
                  :key="opt"
                  :value="opt"
                  class="text-xs"
                >
                  {{ opt }}
                </SelectItem>
              </SelectContent>
            </Select>
            <Input
              v-else
              :id="`channel-field-${key}`"
              :model-value="credentialStringValue(key)"
              type="text"
              :placeholder="orderedFields[key].example ? String(orderedFields[key].example) : ''"
              class="h-8 text-xs shadow-none"
              @update:model-value="(val) => setCredentialStringValue(key, val)"
            />
          </div>
        </div>
      </div>

      <!-- Advanced Settings Collapsible Area -->
      <div class="rounded-md border border-border bg-background p-4 shadow-none space-y-4">
        <div class="flex items-center justify-between">
          <div class="space-y-1">
            <h4 class="text-xs font-medium">
              Advanced Settings
            </h4>
            <p class="text-[11px] text-muted-foreground">
              Optional configuration parameters for this integration.
            </p>
          </div>
          <div class="flex gap-2">
            <button
              :disabled="optionalFieldsKeys.length === 0"
              class="inline-flex items-center justify-center whitespace-nowrap font-medium transition-all disabled:pointer-events-none disabled:opacity-50 outline-none focus-visible:ring-2 focus-visible:ring-ring/30 cursor-pointer hover:bg-accent bg-transparent rounded-lg h-7 px-2 text-xs text-muted-foreground hover:text-foreground"
              @click="isAdvancedExpanded = true"
            >
              Expand All
            </button>
            <button
              :disabled="optionalFieldsKeys.length === 0"
              class="inline-flex items-center justify-center whitespace-nowrap font-medium transition-all disabled:pointer-events-none disabled:opacity-50 outline-none focus-visible:ring-2 focus-visible:ring-ring/30 cursor-pointer hover:bg-accent bg-transparent rounded-lg h-7 px-2 text-xs text-muted-foreground hover:text-foreground"
              @click="isAdvancedExpanded = false"
            >
              Collapse
            </button>
          </div>
        </div>
        
        <div
          v-show="isAdvancedExpanded"
          class="pt-4 border-t border-border/50"
        >
          <div
            v-if="optionalFieldsKeys.length > 0"
            class="grid gap-4 md:grid-cols-2"
          >
            <div
              v-for="key in optionalFieldsKeys"
              :key="key"
              class="space-y-1.5"
              :class="isWideChannelField(orderedFields[key], key) ? 'md:col-span-2' : ''"
            >
              <Label
                :for="`channel-field-${key}`"
                class="text-xs font-medium"
              >{{ orderedFields[key].title || key }}</Label>
              <p
                v-if="orderedFields[key].description"
                class="text-[11px] text-muted-foreground leading-tight"
              >
                {{ orderedFields[key].description }}
              </p>
                
              <!-- Dynamic Field Render -->
              <div
                v-if="orderedFields[key].type === 'secret'"
                class="relative"
              >
                <Input
                  :id="`channel-field-${key}`"
                  :model-value="credentialStringValue(key)"
                  :type="visibleSecrets[key] ? 'text' : 'password'"
                  :placeholder="orderedFields[key].example ? String(orderedFields[key].example) : ''"
                  class="h-8 text-xs shadow-none pr-8"
                  @update:model-value="(val) => setCredentialStringValue(key, val)"
                />
                <button
                  type="button"
                  class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  @click="toggleSecret(key)"
                >
                  <component
                    :is="visibleSecrets[key] ? EyeOff : Eye"
                    class="size-3.5"
                  />
                </button>
              </div>
              <Switch
                v-else-if="orderedFields[key].type === 'bool'"
                :model-value="!!form.credentials[key]"
                @update:model-value="(val) => form.credentials[key] = !!val"
              />
              <Input
                v-else-if="orderedFields[key].type === 'number'"
                :id="`channel-field-${key}`"
                :model-value="credentialNumberValue(key)"
                type="number"
                :placeholder="orderedFields[key].example ? String(orderedFields[key].example) : ''"
                class="h-8 text-xs shadow-none"
                @update:model-value="(val) => setCredentialNumberValue(key, val)"
              />
              <Select
                v-else-if="orderedFields[key].type === 'enum' && orderedFields[key].enum"
                :model-value="String(form.credentials[key] || '')"
                @update:model-value="(val) => form.credentials[key] = val"
              >
                <SelectTrigger class="h-8 text-xs shadow-none">
                  <SelectValue :placeholder="orderedFields[key].title || key" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem
                    v-for="opt in orderedFields[key].enum"
                    :key="opt"
                    :value="opt"
                    class="text-xs"
                  >
                    {{ opt }}
                  </SelectItem>
                </SelectContent>
              </Select>
              <Input
                v-else
                :id="`channel-field-${key}`"
                :model-value="credentialStringValue(key)"
                type="text"
                :placeholder="orderedFields[key].example ? String(orderedFields[key].example) : ''"
                class="h-8 text-xs shadow-none"
                @update:model-value="(val) => setCredentialStringValue(key, val)"
              />
            </div>
          </div>
          <div
            v-else
            class="text-[11px] text-muted-foreground text-center py-2 italic"
          >
            No advanced options available for this platform.
          </div>
        </div>
      </div>
    </div>

    <!-- Stoic Danger Zone -->
    <div
      v-if="isEditMode"
      class="pt-4"
    >
      <div class="space-y-4 rounded-md border border-border bg-background p-4 shadow-none">
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div class="space-y-0.5">
            <h4 class="text-xs font-medium text-destructive">
              Danger Zone
            </h4>
            <p class="text-[11px] text-muted-foreground">
              Deleting this connection cannot be undone. Proceed with caution.
            </p>
          </div>
          <div class="flex justify-end shrink-0">
            <ConfirmPopover
              :message="$t('bots.channels.deleteConfirm')"
              :loading="action === 'delete'"
              @confirm="handleDelete"
            >
              <template #trigger>
                <button 
                  type="button" 
                  :disabled="isBusy"
                  class="inline-flex items-center justify-center whitespace-nowrap transition-all disabled:pointer-events-none disabled:opacity-50 outline-none focus-visible:ring-2 focus-visible:ring-ring/30 cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90 rounded-lg gap-1.5 px-3 min-w-28 h-8 text-xs font-medium shadow-none"
                >
                  <Spinner
                    v-if="action === 'delete'"
                    class="mr-1.5 size-3"
                  />
                  {{ $t('common.delete') }}
                </button>
              </template>
            </ConfirmPopover>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Button, Input, Label, Switch, Spinner, Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@memohai/ui'
import { Eye, EyeOff } from 'lucide-vue-next'
import { reactive, watch, computed, ref, toRaw } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useMutation } from '@pinia/colada'
import { putBotsByIdChannelByPlatform, deleteBotsByIdChannelByPlatform, patchBotsByIdChannelByPlatformStatus } from '@memohai/sdk'
import type { HandlersChannelMeta, ChannelChannelConfig, ChannelFieldSchema, ChannelUpsertConfigRequest } from '@memohai/sdk'
import { client } from '@memohai/sdk/client'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ChannelIcon from '@/components/channel-icon/index.vue'
import WeixinQrLogin from './weixin-qr-login.vue'
import { channelTypeDisplayName } from '@/utils/channel-type-label'

export interface BotChannelItem {
  meta: HandlersChannelMeta
  config: ChannelChannelConfig | null
  configured: boolean
}

const props = defineProps<{
  botId: string
  channelItem: BotChannelItem
  allDirtyStates: Record<string, boolean>
}>()

const emit = defineEmits<{
  saved: []
  'update:dirty': [isDirty: boolean]
  'switch-tab': [channel: string]
}>()

const { t } = useI18n()
const botIdRef = computed(() => props.botId)
const platformType = computed(() => String(props.channelItem.meta.type || '').trim())
const channelTitle = computed(() => channelTypeDisplayName(t, props.channelItem.meta.type, props.channelItem.meta.display_name))
const platformKeyLine = computed(() => t('bots.channels.platformKey', { key: platformType.value }))
// queryCache removed

const action = ref<'save' | 'toggle' | 'delete' | ''>('')
const isBusy = computed(() => action.value !== '')
const isEditMode = computed(() => props.channelItem.configured)
const lastSavedConfigId = ref('')

const form = reactive<{ credentials: Record<string, unknown>; disabled: boolean }>({ credentials: {}, disabled: false })
const initialCredentialsString = ref('')
const visibleSecrets = reactive<Record<string, boolean>>({})
const isAdvancedExpanded = ref(false)

const { mutateAsync: upsertChannel } = useMutation({
  mutation: async ({ platform, data }: { platform: string; data: ChannelUpsertConfigRequest }) => {
    const { data: result } = await putBotsByIdChannelByPlatform({ path: { id: botIdRef.value, platform }, body: data, throwOnError: true })
    return result
  }
})
const { mutateAsync: updateChannelStatus } = useMutation({
  mutation: async ({ platform, disabled }: { platform: string; disabled: boolean }) => {
    const { data } = await patchBotsByIdChannelByPlatformStatus({ path: { id: botIdRef.value, platform }, body: { disabled }, throwOnError: true })
    return data
  }
})

function isWideChannelField(field: ChannelFieldSchema, key: string): boolean {
  if (field.type === 'secret') return true
  const lower = key.toLowerCase()
  if (lower.includes('url') || lower.includes('endpoint') || lower.includes('key') || lower.includes('token') || lower.includes('path') || lower.includes('uri') || lower.includes('webhook')) return true
  if ((field.description ?? '').length > 80) return true
  return false
}

const orderedFields = computed(() => {
  const fields = props.channelItem.meta.config_schema?.fields ?? {}
  const entries = Object.entries(fields).filter(([key]) => key !== 'status' && key !== 'disabled')
  entries.sort(([keyA, a], [keyB, b]) => {
    if (a.required && !b.required) return -1
    if (!a.required && b.required) return 1
    const orderA = a.order ?? Number.MAX_SAFE_INTEGER
    const orderB = b.order ?? Number.MAX_SAFE_INTEGER
    return orderA !== orderB ? orderA - orderB : keyA.localeCompare(keyB)
  })
  return Object.fromEntries(entries) as Record<string, ChannelFieldSchema>
})

const requiredFieldsKeys = computed(() => Object.keys(orderedFields.value).filter(k => orderedFields.value[k].required))
const optionalFieldsKeys = computed(() => Object.keys(orderedFields.value).filter(k => !orderedFields.value[k].required))

const currentInboundMode = computed(() => String(form.credentials.inboundMode ?? form.credentials.inbound_mode ?? '').trim().toLowerCase())
const showWebhookCallback = computed(() => platformType.value === 'feishu' ? currentInboundMode.value === 'webhook' : platformType.value === 'wechatoa')
const webhookCallbackUrl = computed(() => {
  if (!showWebhookCallback.value) return ''
  const configId = String(props.channelItem.config?.id || lastSavedConfigId.value || '').trim()
  return configId ? buildWebhookCallbackUrl(configId) : ''
})

function initForm() {
  const schema = props.channelItem.meta.config_schema?.fields ?? {}
  const existingCredentials = props.channelItem.config?.credentials ?? {}
  const creds: Record<string, unknown> = {}
  
  let hasPopulatedOptional = false
  for (const [key, field] of Object.entries(schema)) {
    if (existingCredentials[key] !== undefined) {
      creds[key] = existingCredentials[key]
      if (!field.required && creds[key] && String(creds[key]).trim() !== '') hasPopulatedOptional = true
    } else {
      creds[key] = field.type === 'bool' ? undefined : ''
    }
  }
  form.credentials = creds
  form.disabled = props.channelItem.config?.disabled ?? false
  lastSavedConfigId.value = String(props.channelItem.config?.id || '').trim()
  initialCredentialsString.value = JSON.stringify(creds)
  
  // Smart expansion of Advanced Settings
  isAdvancedExpanded.value = hasPopulatedOptional
  emit('update:dirty', false)
}

watch(() => props.channelItem, initForm, { immediate: true })

const isFormDirty = computed(() => JSON.stringify(toRaw(form.credentials)) !== initialCredentialsString.value)
watch(isFormDirty, (val) => emit('update:dirty', val))

const dirtyPrompt = computed(() => {
  if (isFormDirty.value) return { type: 'current', channel: platformType.value, channelName: channelTitle.value }
  const otherDirty = Object.keys(props.allDirtyStates).find(k => props.allDirtyStates[k] && k !== platformType.value)
  if (otherDirty) return { type: 'other', channel: otherDirty, channelName: channelTypeDisplayName(t, otherDirty, otherDirty) }
  return null
})

function validateRequired(): boolean {
  for (const key of requiredFieldsKeys.value) {
    const val = form.credentials[key]
    if (!val || (typeof val === 'string' && val.trim() === '')) {
      toast.error(t('bots.channels.requiredField', { field: orderedFields.value[key].title || key }))
      return false
    }
  }
  if (platformType.value === 'feishu' && currentInboundMode.value === 'webhook') {
    if (!String(form.credentials.encryptKey || form.credentials.encrypt_key || '').trim() && !String(form.credentials.verificationToken || form.credentials.verification_token || '').trim()) {
      toast.error(t('bots.channels.feishuWebhookSecretRequired'))
      return false
    }
  }
  return true
}

async function handleSave() {
  if (!validateRequired()) return
  action.value = 'save'
  try {
    const cleanCreds = Object.fromEntries(Object.entries(form.credentials).filter(([k, v]) => k !== 'status' && k !== 'disabled' && v !== '' && v !== undefined && v !== null))
    const result = await upsertChannel({ platform: platformType.value, data: { credentials: cleanCreds, disabled: form.disabled } })
    lastSavedConfigId.value = String(result?.id || lastSavedConfigId.value || '').trim()
    initialCredentialsString.value = JSON.stringify(toRaw(form.credentials))
    toast.success(t('bots.channels.saveSuccess'))
    emit('update:dirty', false)
    emit('saved')
  } catch (err) {
    toast.error(err.message ? `${t('bots.channels.saveFailed')}: ${err.message}` : t('bots.channels.saveFailed'))
  } finally {
    action.value = ''
  }
}

async function handleToggleDisabled() {
  action.value = 'toggle'
  try {
    const result = await updateChannelStatus({ platform: platformType.value, disabled: !form.disabled })
    form.disabled = !!result?.disabled
    toast.success(t('bots.channels.saveSuccess'))
    emit('saved')
  } catch (err) {
    toast.error(err.message ? `${t('bots.channels.saveFailed')}: ${err.message}` : t('bots.channels.saveFailed'))
  } finally {
    action.value = ''
  }
}

async function handleDelete() {
  action.value = 'delete'
  try {
    await deleteBotsByIdChannelByPlatform({ path: { id: botIdRef.value, platform: platformType.value }, throwOnError: true })
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

function toggleSecret(key: string) { visibleSecrets[key] = !visibleSecrets[key] }

function credentialStringValue(key: string): string | undefined {
  const value = form.credentials[key]
  if (typeof value === 'string' || typeof value === 'number') {
    return String(value)
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

function buildWebhookCallbackUrl(configId: string): string {
  const base = (import.meta.env.VITE_WEBHOOK_PUBLIC_BASE_URL?.trim() || import.meta.env.VITE_API_PUBLIC_URL?.trim() || client.getConfig().baseUrl || import.meta.env.VITE_API_URL?.trim() || (typeof window !== 'undefined' ? new URL(window.location.origin).toString() : '')).replace(/\/+$/, '')
  return `${base}/channels/${encodeURIComponent(platformType.value)}/webhook/${encodeURIComponent(configId)}`
}

async function copyWebhookCallback() {
  if (webhookCallbackUrl.value && typeof navigator !== 'undefined' && navigator.clipboard) {
    await navigator.clipboard.writeText(webhookCallbackUrl.value)
    toast.success(t('common.copied'))
  } else {
    toast.error(t('bots.channels.copyFailed'))
  }
}

function handleWeixinLoginSuccess() { emit('saved') }
</script>

<style scoped>
.fade-enter-active, .fade-leave-active { transition: opacity 0.2s ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
</style>

