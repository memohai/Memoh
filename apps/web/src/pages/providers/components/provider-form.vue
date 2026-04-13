<template>
  <form @submit="editProvider">
    <div class="space-y-4">
      <section class="space-y-2">
        <FormField
          v-slot="{ componentField }"
          name="name"
        >
          <FormItem>
            <FormLabel>{{ $t('common.name') }}</FormLabel>
            <FormControl>
              <Input
                type="text"
                :placeholder="$t('common.namePlaceholder')"
                :aria-label="$t('common.name')"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>

      <section
        v-if="!['openai-codex', 'github-copilot'].includes(form.values.client_type)"
        class="space-y-2"
      >
        <FormField
          v-slot="{ componentField }"
          name="api_key"
        >
          <FormItem>
            <FormLabel>{{ $t('provider.apiKey') }}</FormLabel>
            <FormControl>
              <Input
                type="password"
                :placeholder="getStoredSecret(props.provider?.config as Record<string, unknown> | undefined) || $t('provider.apiKeyPlaceholder')"
                :aria-label="$t('provider.apiKey')"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>

      <section
        v-if="form.values.client_type !== 'github-copilot'"
        class="space-y-2"
      >
        <FormField
          v-slot="{ componentField }"
          name="base_url"
        >
          <FormItem>
            <FormLabel>{{ $t('provider.url') }}</FormLabel>
            <FormControl>
              <Input
                type="text"
                :placeholder="$t('provider.urlPlaceholder')"
                :aria-label="$t('provider.url')"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>

      <section class="space-y-2">
        <FormField
          v-slot="{ value, handleChange }"
          name="client_type"
        >
          <FormItem>
            <FormLabel>{{ $t('provider.clientType') }}</FormLabel>
            <FormControl>
              <SearchableSelectPopover
                :model-value="value"
                :options="clientTypeOptions"
                :placeholder="$t('models.clientTypePlaceholder')"
                @update:model-value="handleChange"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>

      <section
        v-if="['openai-codex', 'github-copilot'].includes(form.values.client_type)"
        class="rounded-lg border p-4 space-y-3 text-xs"
      >
        <div class="space-y-1">
          <div class="font-medium">
            {{ $t(form.values.client_type === 'github-copilot' ? 'provider.oauth.githubDeviceTitle' : 'provider.oauth.openaiTitle') }}
          </div>
          <div class="text-muted-foreground">
            {{ $t(form.values.client_type === 'github-copilot' ? 'provider.oauth.githubDeviceDescription' : 'provider.oauth.openaiDescription') }}
          </div>
          <div
            class="text-xs"
            :class="oauthExpired ? 'text-destructive' : 'text-muted-foreground'"
          >
            <template v-if="oauthStatusLoading">
              {{ $t('provider.oauth.status.checking') }}
            </template>
            <template v-else-if="oauthStatus && !oauthStatus.configured">
              {{ $t('provider.oauth.status.notConfigured') }}
            </template>
            <template v-else-if="oauthExpired">
              {{ $t('provider.oauth.status.expired') }}
            </template>
            <template v-else-if="oauthStatus?.has_token">
              {{ $t(form.values.client_type === 'github-copilot' ? 'provider.oauth.status.authorizedCurrent' : 'provider.oauth.status.authorized') }}
            </template>
            <template v-else-if="oauthStatus?.device?.pending">
              {{ $t('provider.oauth.status.pendingDevice') }}
            </template>
            <template v-else>
              {{ $t('provider.oauth.status.missing') }}
            </template>
          </div>
          <div
            v-if="oauthStatus?.callback_url"
            class="text-xs text-muted-foreground"
          >
            {{ $t('provider.oauth.callback') }}: {{ oauthStatus.callback_url }}
          </div>
        </div>
        <div
          v-if="form.values.client_type === 'github-copilot'
            && oauthStatus?.device?.pending
            && !oauthStatus?.has_token
            && oauthStatus?.device?.user_code
            && oauthStatus?.device?.verification_uri"
          class="rounded-md bg-muted/40 p-3 space-y-2"
        >
          <div class="text-muted-foreground">
            {{ $t('provider.oauth.githubDeviceHint') }}
          </div>
          <div class="space-y-1">
            <div class="font-medium">
              {{ $t('provider.oauth.deviceVerificationUri') }}
            </div>
            <code class="block break-all rounded bg-background px-2 py-1 select-all">{{ oauthStatus?.device?.verification_uri }}</code>
          </div>
          <div class="space-y-1">
            <div class="font-medium">
              {{ $t('provider.oauth.deviceUserCode') }}
            </div>
            <div class="flex items-center gap-2">
              <code class="block flex-1 rounded bg-background px-2 py-1 text-sm tracking-[0.3em] select-all">{{ oauthStatus?.device?.user_code }}</code>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                @click="handleCopyDeviceCode"
              >
                <Copy />
                {{ $t('common.copy') }}
              </Button>
            </div>
          </div>
          <div
            v-if="oauthStatus?.device?.expires_at"
            class="text-muted-foreground"
          >
            {{ $t('provider.oauth.deviceExpiresAt') }}: {{ oauthStatus.device.expires_at }}
          </div>
          <div class="flex items-center gap-2 text-foreground">
            <Spinner class="size-4" />
            <span>{{ $t('provider.oauth.status.oauthing') }}</span>
          </div>
        </div>
        <div
          v-if="form.values.client_type === 'github-copilot' && oauthStatus?.has_token && !oauthExpired"
          class="rounded-md bg-muted/40 p-3 space-y-1"
        >
          <div class="font-medium">
            {{ $t('provider.oauth.connectedAccount') }}
          </div>
          <div class="text-sm font-medium">
            {{ oauthStatus?.account?.email || oauthStatus?.account?.label || oauthStatus?.account?.name || oauthStatus?.account?.login || $t('provider.oauth.status.authorizedCurrent') }}
          </div>
          <div
            v-if="[oauthStatus?.account?.login?.trim() ? `@${oauthStatus.account.login.trim()}` : '', oauthStatus?.account?.email?.trim() ?? ''].filter(Boolean).join(' · ')"
            class="text-xs text-muted-foreground"
          >
            {{ [oauthStatus?.account?.login?.trim() ? `@${oauthStatus.account.login.trim()}` : '', oauthStatus?.account?.email?.trim() ?? ''].filter(Boolean).join(' · ') }}
          </div>
        </div>
        <div class="flex gap-2">
          <LoadingButton
            v-if="props.provider?.id
              && ['openai-codex', 'github-copilot'].includes(form.values.client_type)
              && !(
                form.values.client_type === 'github-copilot'
                && oauthStatus?.device?.pending
                && !oauthStatus?.has_token
                && oauthStatus?.device?.user_code
                && oauthStatus?.device?.verification_uri
              )
              && (!oauthStatus?.has_token || oauthExpired)"
            type="button"
            variant="outline"
            :disabled="!props.provider?.id || !['openai-codex', 'github-copilot'].includes(form.values.client_type) || oauthStatusLoading"
            :loading="authorizeLoading"
            @click="handleAuthorize"
          >
            <KeyRound />
            {{ $t(form.values.client_type === 'github-copilot' ? 'provider.oauth.deviceAuthorize' : 'provider.oauth.authorize') }}
          </LoadingButton>
          <LoadingButton
            v-if="oauthStatus?.has_token"
            type="button"
            variant="ghost"
            :loading="revokeLoading"
            @click="handleRevoke"
          >
            {{ $t('provider.oauth.revoke') }}
          </LoadingButton>
        </div>
      </section>
    </div>

    <section class="flex justify-between items-center mt-4">
      <LoadingButton
        type="button"
        variant="outline"
        :loading="testLoading"
        :disabled="!props.provider?.id"
        @click="runTest"
      >
        <RefreshCw
          v-if="!testLoading"
        />
        {{ $t('provider.testConnection') }}
      </LoadingButton>

      <div class="flex gap-4">
        <ConfirmPopover
          :message="$t('provider.deleteConfirm')"
          :loading="deleteLoading"
          @confirm="$emit('delete')"
        >
          <template #trigger>
            <Button
              type="button"
              variant="outline"
              :aria-label="$t('common.delete')"
            >
              <Trash2 />
            </Button>
          </template>
        </ConfirmPopover>

        <LoadingButton
          type="submit"
          :loading="editLoading"
          :disabled="!hasChanges || !form.meta.value.valid"
        >
          {{ $t('provider.saveChanges') }}
        </LoadingButton>
      </div>
    </section>

    <section
      v-if="testResult"
      class="mt-4 rounded-lg border p-4 space-y-3 text-xs"
    >
      <div class="flex items-center gap-2">
        <StatusDot :status="testResult.reachable ? 'success' : 'error'" />
        <span class="font-medium">
          {{ testResult.reachable ? $t('provider.reachable') : $t('provider.unreachable') }}
        </span>
        <span
          v-if="testResult.latency_ms"
          class="text-muted-foreground"
        >
          {{ testResult.latency_ms }}ms
        </span>
      </div>

      <div
        v-if="testResult.message"
        class="text-muted-foreground text-xs"
      >
        {{ testResult.message }}
      </div>

      <div
        v-if="testError"
        class="text-destructive text-xs"
      >
        {{ testError }}
      </div>
    </section>
  </form>
</template>

<script setup lang="ts">
import {
  Input,
  Button,
  FormControl,
  FormField,
  FormLabel,
  FormItem,
  Spinner,
} from '@memohai/ui'
import { Copy, KeyRound, RefreshCw, Trash2 } from 'lucide-vue-next'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import StatusDot from '@/components/status-dot/index.vue'
import LoadingButton from '@/components/loading-button/index.vue'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import { useClipboard } from '@/composables/useClipboard'
import { CLIENT_TYPE_LIST, CLIENT_TYPE_META } from '@/constants/client-types'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { postProvidersByIdTest } from '@memohai/sdk'
import type { ProvidersGetResponse, ProvidersTestResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'

const { t } = useI18n()
const { copyText } = useClipboard()

type ProviderWithAuth = Partial<ProvidersGetResponse>

type ProviderOAuthStatus = {
  configured: boolean
  mode?: string
  has_token: boolean
  expired: boolean
  callback_url?: string
  expires_at?: string
  account?: {
    label?: string
    login?: string
    name?: string
    email?: string
    avatar_url?: string
    profile_url?: string
  }
  device?: {
    pending: boolean
    user_code?: string
    verification_uri?: string
    expires_at?: string
    interval_seconds?: number
  }
}

type ProviderOAuthAuthorizeResponse = {
  mode?: string
  auth_url?: string
  device?: ProviderOAuthStatus['device']
}

function getStoredSecret(config: Record<string, unknown> | undefined) {
  if (!config) return ''
  const apiKey = config.api_key
  return typeof apiKey === 'string' ? apiKey : ''
}

const props = defineProps<{
  provider: ProviderWithAuth | undefined
  editLoading: boolean
  deleteLoading: boolean
}>()

const emit = defineEmits<{
  submit: [values: Record<string, unknown>]
  delete: []
}>()

const testLoading = ref(false)
const testResult = ref<ProvidersTestResponse | null>(null)
const testError = ref('')
const oauthStatus = ref<ProviderOAuthStatus | null>(null)
const oauthStatusLoading = ref(false)
const authorizeLoading = ref(false)
const revokeLoading = ref(false)
const pollTimer = ref<number | null>(null)
const apiBase = import.meta.env.VITE_API_URL?.trim() || '/api'

async function runTest() {
  if (!props.provider?.id) return
  testLoading.value = true
  testResult.value = null
  testError.value = ''
  try {
    const { data } = await postProvidersByIdTest({
      path: { id: props.provider.id },
      throwOnError: true,
    })
    testResult.value = data ?? null
  } catch (err: unknown) {
    testError.value = err instanceof Error ? err.message : t('provider.testFailed')
  } finally {
    testLoading.value = false
  }
}

watch(() => props.provider?.id, () => {
  testResult.value = null
  testError.value = ''
})

const clientTypeOptions = computed(() =>
  CLIENT_TYPE_LIST.map((ct) => ({
    value: ct.value,
    label: ct.label,
    description: ct.hint,
    keywords: [ct.label, ct.hint, CLIENT_TYPE_META[ct.value]?.value ?? ct.value],
  })),
)

const providerSchema = toTypedSchema(z.object({
  enable: z.boolean(),
  name: z.string().min(1),
  base_url: z.string().optional(),
  api_key: z.string().optional(),
  client_type: z.string().min(1),
}).superRefine((value, ctx) => {
  const existingSecret = getStoredSecret(
    props.provider?.config as Record<string, unknown> | undefined,
  )
  if (!['openai-codex', 'github-copilot'].includes(value.client_type) && !value.api_key?.trim() && !existingSecret.trim()) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['api_key'],
      message: 'API key is required',
    })
  }
  if (value.client_type !== 'github-copilot' && !value.base_url?.trim()) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['base_url'],
      message: 'Base URL is required',
    })
  }
}))

const form = useForm({
  validationSchema: providerSchema,
})

watch(() => props.provider, (newVal) => {
  if (newVal) {
    const cfg = newVal.config as Record<string, unknown> | undefined
    form.setValues({
      enable: newVal.enable ?? true,
      name: newVal.name,
      base_url: (cfg?.base_url as string) ?? '',
      api_key: '',
      client_type: newVal.client_type || 'openai-completions',
    })
  }
}, { immediate: true })

watch(() => form.values.client_type, (clientType) => {
  if (!['openai-codex', 'github-copilot'].includes(clientType)) {
    oauthStatus.value = null
  }
  if (clientType === 'openai-codex' && !form.values.base_url) {
    form.setFieldValue('base_url', 'https://chatgpt.com/backend-api')
  }
  if (clientType === 'github-copilot') {
    form.setFieldValue('base_url', '')
  }
})

watch(() => [props.provider?.id, form.values.client_type] as const, async ([id, clientType]) => {
  if (!id || (clientType !== 'openai-codex' && clientType !== 'github-copilot')) {
    oauthStatus.value = null
    return
  }
  await fetchOAuthStatus()
}, { immediate: true })

const hasChanges = computed(() => {
  const raw = props.provider
  const cfg = raw?.config as Record<string, unknown> | undefined
  const baseChanged = JSON.stringify({
    enable: form.values.enable,
    name: form.values.name,
    base_url: form.values.base_url,
    client_type: form.values.client_type,
  }) !== JSON.stringify({
    enable: raw?.enable ?? true,
    name: raw?.name,
    base_url: (cfg?.base_url as string) ?? '',
    client_type: raw?.client_type || 'openai-completions',
  })

  const apiKeyChanged = Boolean(form.values.api_key && form.values.api_key.trim() !== '')
  return baseChanged || apiKeyChanged
})

const editProvider = form.handleSubmit(async (value) => {
  const config: Record<string, unknown> = {}
  if (value.base_url && value.base_url.trim() !== '') {
    config.base_url = value.base_url
  }
  if (value.api_key && value.api_key.trim() !== '') {
    if (value.client_type !== 'github-copilot') {
      config.api_key = value.api_key.trim()
    }
  }
  const metadata = {
    ...((props.provider?.metadata as Record<string, unknown> | undefined) ?? {}),
  }
  if (value.client_type === 'github-copilot') {
    delete metadata.oauth_client_id
  }
  const payload: Record<string, unknown> = {
    enable: value.enable,
    name: value.name,
    config,
    client_type: value.client_type,
  }
  if (Object.keys(metadata).length > 0 || value.client_type === 'github-copilot') {
    payload.metadata = metadata
  }
  emit('submit', payload)
})

const oauthExpired = computed(() => Boolean(oauthStatus.value?.has_token && oauthStatus.value?.expired))

function authHeaders(): Record<string, string> {
  const token = localStorage.getItem('token')
  return token ? { Authorization: `Bearer ${token}` } : {}
}

function clearPollTimer() {
  if (pollTimer.value !== null) {
    window.clearTimeout(pollTimer.value)
    pollTimer.value = null
  }
}

async function fetchOAuthStatus() {
  if (!props.provider?.id) return
  oauthStatusLoading.value = true
  try {
    const response = await fetch(`${apiBase}/providers/${props.provider.id}/oauth/status`, {
      headers: authHeaders(),
    })
    if (!response.ok) throw new Error(t('provider.oauth.statusFailed'))
    oauthStatus.value = await response.json() as ProviderOAuthStatus
  } catch (error) {
    oauthStatus.value = null
    console.error('failed to load provider oauth status', error)
  } finally {
    oauthStatusLoading.value = false
  }
}

async function pollOAuthAuthorization(notifyOnSuccess = false) {
  if (!props.provider?.id || form.values.client_type !== 'github-copilot') return
  try {
    const response = await fetch(`${apiBase}/providers/${props.provider.id}/oauth/poll`, {
      method: 'POST',
      headers: authHeaders(),
    })
    if (!response.ok) throw new Error(t('provider.oauth.authorizeFailed'))
    const nextStatus = await response.json() as ProviderOAuthStatus
    const becameAuthorized = !oauthStatus.value?.has_token && Boolean(nextStatus.has_token)
    oauthStatus.value = nextStatus
    if (notifyOnSuccess && becameAuthorized) {
      toast.success(t('provider.oauth.authorizeSuccess'))
    }
  } catch (error) {
    clearPollTimer()
    toast.error(error instanceof Error ? error.message : t('provider.oauth.authorizeFailed'))
  }
}

watch(oauthStatus, (status) => {
  clearPollTimer()
  if (form.values.client_type !== 'github-copilot') {
    return
  }
  if (!status?.device?.pending || status.has_token) {
    return
  }
  const intervalSeconds = Math.max(status.device.interval_seconds ?? 5, 1)
  pollTimer.value = window.setTimeout(() => {
    void pollOAuthAuthorization(true)
  }, intervalSeconds * 1000)
})

onBeforeUnmount(() => {
  clearPollTimer()
})

async function handleAuthorize() {
  if (!props.provider?.id) return
  authorizeLoading.value = true
  try {
    const response = await fetch(`${apiBase}/providers/${props.provider.id}/oauth/authorize`, {
      headers: authHeaders(),
    })
    if (!response.ok) throw new Error(t('provider.oauth.authorizeFailed'))
    const data = await response.json() as ProviderOAuthAuthorizeResponse
    if (data.mode === 'device') {
      oauthStatus.value = {
        configured: true,
        mode: 'device',
        has_token: false,
        expired: false,
        callback_url: '',
        device: data.device,
      }
      return
    }
    if (!data.auth_url) throw new Error(t('provider.oauth.authorizeFailed'))
    const popup = window.open(data.auth_url, 'provider-oauth', 'width=600,height=720')
    const listener = async (event: MessageEvent) => {
      if (event.data?.type !== 'memoh-provider-oauth-success') return
      window.removeEventListener('message', listener)
      popup?.close()
      toast.success(t('provider.oauth.authorizeSuccess'))
      await fetchOAuthStatus()
    }
    window.addEventListener('message', listener)
  } catch (error) {
    toast.error(error instanceof Error ? error.message : t('provider.oauth.authorizeFailed'))
  } finally {
    authorizeLoading.value = false
  }
}

async function handleCopyDeviceCode() {
  const userCode = oauthStatus.value?.device?.user_code?.trim()
  const verificationUri = oauthStatus.value?.device?.verification_uri?.trim()
  if (!userCode || !verificationUri) return

  const popup = window.open('', 'provider-device-oauth', 'width=960,height=720')
  const copied = await copyText(userCode)

  if (!copied) {
    popup?.close()
    toast.error(t('provider.oauth.copyFailed'))
    return
  }

  toast.success(t('common.copied'))

  if (popup) {
    popup.location.href = verificationUri
    popup.focus()
    return
  }

  window.open(verificationUri, '_blank', 'width=960,height=720')
}

async function handleRevoke() {
  if (!props.provider?.id) return
  clearPollTimer()
  revokeLoading.value = true
  try {
    const response = await fetch(`${apiBase}/providers/${props.provider.id}/oauth/token`, {
      method: 'DELETE',
      headers: authHeaders(),
    })
    if (!response.ok) throw new Error(t('provider.oauth.revokeFailed'))
    toast.success(t('provider.oauth.revokeSuccess'))
    await fetchOAuthStatus()
  } catch (error) {
    toast.error(error instanceof Error ? error.message : t('provider.oauth.revokeFailed'))
  } finally {
    revokeLoading.value = false
  }
}
</script>
