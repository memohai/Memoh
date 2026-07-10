<template>
  <form @submit="editProvider">
    <SettingsSection :title="$t('provider.configurationTitle')">
      <!-- Field rows are grouped so the LAST one keeps its `last:border-b-0`
           (no trailing inset hairline) — the footer below owns the only divider,
           and it spans full width. -->
      <div>
        <FormField
          v-slot="{ componentField, errorMessage }"
          name="name"
        >
          <SettingsRow :label="$t('common.name')">
            <FormItem class="w-80">
              <FormControl>
                <Input
                  type="text"
                  :placeholder="$t('common.namePlaceholder')"
                  :aria-label="$t('common.name')"
                  :aria-invalid="!!errorMessage"
                  v-bind="componentField"
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          </SettingsRow>
        </FormField>

        <FormField
          v-slot="{ value, handleChange, errorMessage }"
          name="client_type"
        >
          <SettingsRow :label="$t('provider.clientType')">
            <FormItem class="w-80">
              <FormControl>
                <Select
                  :model-value="value"
                  :aria-invalid="!!errorMessage"
                  @update:model-value="handleChange"
                >
                  <SelectTrigger class="w-full">
                    <SelectValue :placeholder="$t('models.clientTypePlaceholder')" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem
                      v-for="option in clientTypeOptions"
                      :key="option.value"
                      :value="option.value"
                    >
                      {{ option.label }}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </FormControl>
              <FormMessage />
            </FormItem>
          </SettingsRow>
        </FormField>

        <FormField
          v-if="form.values.client_type !== 'github-copilot'"
          v-slot="{ componentField, errorMessage }"
          name="base_url"
        >
          <SettingsRow :label="$t('provider.url')">
            <FormItem class="w-80">
              <FormControl>
                <Input
                  type="text"
                  :placeholder="$t('provider.urlPlaceholder')"
                  :aria-label="$t('provider.url')"
                  :aria-invalid="!!errorMessage"
                  v-bind="componentField"
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          </SettingsRow>
        </FormField>

        <FormField
          v-if="!isProviderOAuthClientType(form.values.client_type)"
          v-slot="{ componentField, errorMessage }"
          name="api_key"
        >
          <SettingsRow :label="$t('provider.apiKey')">
            <FormItem class="w-80">
              <FormControl>
                <Input
                  type="password"
                  :placeholder="getStoredSecret(props.provider?.config as Record<string, unknown> | undefined) || $t('provider.apiKeyPlaceholder')"
                  :aria-label="$t('provider.apiKey')"
                  :aria-invalid="!!errorMessage"
                  v-bind="componentField"
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          </SettingsRow>
        </FormField>

        <FormField
          v-if="supportsPromptCache(form.values.client_type)"
          v-slot="{ value, handleChange, errorMessage }"
          name="prompt_cache_ttl"
        >
          <SettingsRow
            :label="$t('provider.promptCache.label')"
            :description="cacheDescription"
          >
            <FormItem>
              <FormControl>
                <Select
                  :model-value="value || '5m'"
                  :aria-invalid="!!errorMessage"
                  @update:model-value="handleChange"
                >
                  <SelectTrigger
                    size="sm"
                    class="min-w-36"
                  >
                    <SelectValue :placeholder="$t('provider.promptCache.label')" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="5m">
                      {{ $t('provider.promptCache.option5m') }}
                    </SelectItem>
                    <SelectItem value="1h">
                      {{ $t('provider.promptCache.option1h') }}
                    </SelectItem>
                    <SelectItem value="off">
                      {{ $t('provider.promptCache.optionOff') }}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </FormControl>
              <FormMessage />
            </FormItem>
          </SettingsRow>
        </FormField>
      </div>

      <!-- Actions close the card via the section's footer band (its top hairline
           spans the card and its inset padding matches the field rows above). -->
      <template #footer>
        <HoverCard :open-delay="120">
          <HoverCardTrigger as-child>
            <Button
              type="button"
              variant="outline"
              size="sm"
              loading-mode="manual"
              :loading="testLoading"
              :disabled="!props.provider?.id"
              @click="runTest"
            >
              <Spinner
                v-if="testLoading"
                class="size-4"
              />
              <svg
                v-else-if="testStatus === 'ok'"
                class="check-draw size-4 text-success"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
                aria-hidden="true"
              >
                <path
                  d="M4 12.5 9 17.5 20 6.5"
                  pathLength="1"
                />
              </svg>
              <AlertCircle
                v-else-if="testStatus === 'error'"
                class="size-4 text-destructive"
              />
              <RefreshCw
                v-else
                class="size-4"
              />
              {{ $t('provider.testConnection') }}
            </Button>
          </HoverCardTrigger>
          <HoverCardContent
            v-if="testError"
            class="w-80 text-xs text-destructive whitespace-pre-wrap break-words"
          >
            {{ testError }}
          </HoverCardContent>
        </HoverCard>

        <LoadingButton
          type="submit"
          size="sm"
          :loading="editLoading"
          :disabled="!hasChanges || !form.meta.value.valid"
        >
          {{ $t('provider.saveChanges') }}
        </LoadingButton>
      </template>
    </SettingsSection>

    <!-- OAuth -->
    <SettingsSection
      v-if="isProviderOAuthClientType(form.values.client_type)"
      class="mt-6"
    >
      <div class="p-4 space-y-3 text-xs">
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
          class="rounded-md bg-muted-soft p-3 space-y-2"
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
          class="rounded-md bg-muted-soft p-3 space-y-1"
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
              && isProviderOAuthClientType(form.values.client_type)
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
            size="sm"
            :disabled="!props.provider?.id || !isProviderOAuthClientType(form.values.client_type) || oauthStatusLoading"
            :loading="authorizeLoading"
            @click="handleAuthorize"
          >
            <KeyRound />
            {{ $t(form.values.client_type === 'github-copilot' ? 'provider.oauth.deviceAuthorize' : 'provider.oauth.authorize') }}
          </LoadingButton>
          <Button
            v-if="webOAuthFlow"
            type="button"
            variant="ghost"
            size="sm"
            @click="cancelWebOAuthAuthorization"
          >
            {{ $t('common.cancel') }}
          </Button>
          <LoadingButton
            v-if="oauthStatus?.has_token"
            type="button"
            variant="ghost"
            size="sm"
            :loading="revokeLoading"
            @click="handleRevoke"
          >
            {{ $t('provider.oauth.revoke') }}
          </LoadingButton>
        </div>
      </div>
    </SettingsSection>
  </form>
</template>

<script setup lang="ts">
import {
  Input,
  Button,
  FormControl,
  FormField,
  FormItem,
  FormMessage,
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Spinner,
} from '@felinic/ui'
import { AlertCircle, Copy, KeyRound, RefreshCw } from 'lucide-vue-next'
import LoadingButton from '@/components/loading-button/index.vue'
import SettingsRow from '@/components/settings/row.vue'
import SettingsSection from '@/components/settings/section.vue'
import { useClipboard } from '@/composables/useClipboard'
import { LLM_CLIENT_TYPE_LIST } from '@/constants/client-types'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import {
  deleteProvidersByIdOauthToken,
  getProvidersByIdOauthAuthorize,
  getProvidersByIdOauthStatus,
  postProvidersByIdOauthPoll,
  postProvidersByIdTest,
} from '@memohai/sdk'
import type {
  ProvidersGetResponse,
  ProvidersOAuthAuthorizeResponse,
  ProvidersOAuthStatus,
  ProvidersTestResponse,
} from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import { toast } from '@felinic/ui'
import { startOAuthPopupFlow, type OAuthPopupFlowController } from '@/utils/oauth/popup-flow'

const { t } = useI18n()
const { copyText } = useClipboard()

type ProviderWithAuth = Partial<ProvidersGetResponse>

function getStoredSecret(config: Record<string, unknown> | undefined) {
  if (!config) return ''
  const apiKey = config.api_key
  return typeof apiKey === 'string' ? apiKey : ''
}

type PromptCacheTtl = '5m' | '1h' | 'off'

function normalizeCacheTtl(value: string | undefined): PromptCacheTtl {
  return value === '1h' || value === 'off' ? value : '5m'
}

// Vendors that expose configurable prompt cache TTL. Currently only
// Anthropic Messages; expand this list as other providers gain support.
const PROMPT_CACHE_CLIENT_TYPES = new Set(['anthropic-messages'])

function supportsPromptCache(clientType: string | undefined): boolean {
  return !!clientType && PROMPT_CACHE_CLIENT_TYPES.has(clientType)
}

function isProviderOAuthClientType(clientType: string | undefined): boolean {
  return clientType === 'openai-codex' || clientType === 'github-copilot'
}

const props = defineProps<{
  provider: ProviderWithAuth | undefined
  editLoading: boolean
}>()

const emit = defineEmits<{
  submit: [values: Record<string, unknown>]
}>()

const testLoading = ref(false)
const testResult = ref<ProvidersTestResponse | null>(null)
const testError = ref('')
const oauthStatus = ref<ProvidersOAuthStatus | null>(null)
const oauthStatusLoading = ref(false)
const authorizeLoading = ref(false)
const revokeLoading = ref(false)
const devicePollTimer = ref<number | null>(null)
const webOAuthFlow = ref<OAuthPopupFlowController | null>(null)
const webOAuthPollIntervalMs = 2000
const webOAuthPollTimeoutMs = 5 * 60 * 1000

const testStatus = computed(() => {
  if (testResult.value?.status === 'ok') return 'ok'
  if (testError.value) return 'error'
  if (testResult.value && testResult.value.status !== 'ok') return 'error'
  return 'idle'
})
const cacheDescription = computed(() =>
  form.values.prompt_cache_ttl === 'off'
    ? t('provider.promptCache.descriptionOff')
    : t('provider.promptCache.description'),
)

function truncateError(text: string): string {
  const max = 220
  return text.length > max ? `${text.slice(0, max).trimEnd()}…` : text
}

// The probe detail can embed the raw upstream response inside `[body: …]`. When
// a Base URL points at a website instead of an API the body is a full HTML
// page, so strip the markup down to its visible text (often near-empty) and
// keep only a short, actionable hint instead of dumping the document.
function formatTestError(raw: string | undefined): string {
  const text = (raw ?? '').trim()
  if (!text) return t('provider.unreachable')
  const bodyStart = text.indexOf('[body:')
  if (bodyStart === -1) return truncateError(text)
  const head = text.slice(0, bodyStart).trim()
  let body = text.slice(bodyStart + '[body:'.length).replace(/\]\s*$/, '').trim()
  if (/<!doctype|<\/?[a-z][^>]*>/i.test(body)) {
    body = body
      .replace(/<(script|style)[^>]*>[\s\S]*?(<\/\1>|$)/gi, ' ')
      .replace(/<[^>]*>/g, ' ')
  }
  body = body.replace(/\s+/g, ' ').trim()
  return truncateError(body ? `${head} · ${body}` : head)
}

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
    if (testResult.value?.status !== 'ok') {
      testError.value = formatTestError(testResult.value?.message)
    }
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : ''
    testError.value = message ? formatTestError(message) : t('provider.testFailed')
  } finally {
    testLoading.value = false
  }
}

watch(() => props.provider?.id, () => {
  testResult.value = null
  testError.value = ''
})

const clientTypeOptions = computed(() =>
  LLM_CLIENT_TYPE_LIST.map((ct) => ({
    value: ct.value,
    label: ct.label,
  })),
)

const providerSchema = toTypedSchema(z.object({
  enable: z.boolean(),
  name: z.string().min(1),
  base_url: z.string().optional(),
  api_key: z.string().optional(),
  client_type: z.string().min(1),
  prompt_cache_ttl: z.enum(['5m', '1h', 'off']).optional(),
}).superRefine((value, ctx) => {
  const existingSecret = getStoredSecret(
    props.provider?.config as Record<string, unknown> | undefined,
  )
  if (!isProviderOAuthClientType(value.client_type) && !value.api_key?.trim() && !existingSecret.trim()) {
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
      prompt_cache_ttl: normalizeCacheTtl(cfg?.prompt_cache_ttl as string | undefined),
    })
  }
}, { immediate: true })

watch(() => form.values.client_type, (clientType) => {
  if (!isProviderOAuthClientType(clientType)) {
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
  const cacheChanged = supportsPromptCache(form.values.client_type)
    && normalizeCacheTtl(form.values.prompt_cache_ttl)
      !== normalizeCacheTtl(cfg?.prompt_cache_ttl as string | undefined)
  return baseChanged || apiKeyChanged || cacheChanged
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
  if (supportsPromptCache(value.client_type)) {
    config.prompt_cache_ttl = normalizeCacheTtl(value.prompt_cache_ttl)
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

function clearDevicePollTimer() {
  if (devicePollTimer.value !== null) {
    window.clearTimeout(devicePollTimer.value)
    devicePollTimer.value = null
  }
}

function clearWebPollTimer() {
  webOAuthFlow.value?.cancel()
  webOAuthFlow.value = null
}

function clearPollTimers() {
  clearDevicePollTimer()
  clearWebPollTimer()
}

async function fetchOAuthStatus(): Promise<ProvidersOAuthStatus | null> {
  if (!props.provider?.id) return null
  oauthStatusLoading.value = true
  try {
    const { data } = await getProvidersByIdOauthStatus({
      path: { id: props.provider.id },
      throwOnError: true,
    })
    const nextStatus = data ?? null
    oauthStatus.value = nextStatus
    return nextStatus
  } catch (error) {
    oauthStatus.value = null
    console.error('failed to load provider oauth status', error)
    return null
  } finally {
    oauthStatusLoading.value = false
  }
}

async function pollOAuthAuthorization(notifyOnSuccess = false) {
  if (!props.provider?.id || form.values.client_type !== 'github-copilot') return
  try {
    const { data } = await postProvidersByIdOauthPoll({
      path: { id: props.provider.id },
      throwOnError: true,
    })
    if (!data) throw new Error(t('provider.oauth.authorizeFailed'))
    const nextStatus = data
    const becameAuthorized = !oauthStatus.value?.has_token && Boolean(nextStatus.has_token)
    oauthStatus.value = nextStatus
    if (notifyOnSuccess && becameAuthorized) {
      toast.success(t('provider.oauth.authorizeSuccess'))
    }
  } catch (error) {
    clearDevicePollTimer()
    toast.error(error instanceof Error ? error.message : t('provider.oauth.authorizeFailed'))
  }
}

watch(oauthStatus, (status) => {
  clearDevicePollTimer()
  if (form.values.client_type !== 'github-copilot') {
    return
  }
  if (!status?.device?.pending || status.has_token) {
    return
  }
  const intervalSeconds = Math.max(status.device.interval_seconds ?? 5, 1)
  devicePollTimer.value = window.setTimeout(() => {
    void pollOAuthAuthorization(true)
  }, intervalSeconds * 1000)
})

onBeforeUnmount(() => {
  clearPollTimers()
})

function cancelWebOAuthAuthorization() {
  webOAuthFlow.value?.cancel()
}

async function handleAuthorize() {
  if (!props.provider?.id) return
  authorizeLoading.value = true
  try {
    const { data } = await getProvidersByIdOauthAuthorize({
      path: { id: props.provider.id },
      throwOnError: true,
    })
    if (!data) throw new Error(t('provider.oauth.authorizeFailed'))
    const result = data as ProvidersOAuthAuthorizeResponse
    if (result.mode === 'device') {
      oauthStatus.value = {
        configured: true,
        mode: 'device',
        has_token: false,
        expired: false,
        callback_url: '',
        device: result.device,
      }
      authorizeLoading.value = false
      return
    }
    if (!result.auth_url) throw new Error(t('provider.oauth.authorizeFailed'))
    const popup = window.open(result.auth_url, 'provider-oauth', 'width=600,height=720')
    if (!popup) throw new Error(t('provider.oauth.authorizeFailed'))
    // Supersede any in-flight popup silently: dispose() (not cancel()) so the
    // previous flow's onAborted doesn't clear this attempt's loading state or
    // close the window we just reused.
    webOAuthFlow.value?.dispose()
    webOAuthFlow.value = startOAuthPopupFlow<ProvidersOAuthStatus>({
      popup,
      target: window,
      expectedSource: popup,
      messageType: 'memoh-provider-oauth-success',
      pollIntervalMs: webOAuthPollIntervalMs,
      timeoutMs: webOAuthPollTimeoutMs,
      pollStatus: fetchOAuthStatus,
      isAuthorized: status => Boolean(status?.has_token && !status.expired),
      onAuthorized: async () => {
        webOAuthFlow.value = null
        toast.success(t('provider.oauth.authorizeSuccess'))
        await fetchOAuthStatus()
        authorizeLoading.value = false
      },
      onAborted: (reason) => {
        webOAuthFlow.value = null
        authorizeLoading.value = false
        if (reason === 'timeout') {
          toast.error(t('provider.oauth.authorizeTimedOut'))
        }
      },
    })
  } catch (error) {
    clearWebPollTimer()
    toast.error(error instanceof Error ? error.message : t('provider.oauth.authorizeFailed'))
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
  clearPollTimers()
  revokeLoading.value = true
  try {
    await deleteProvidersByIdOauthToken({
      path: { id: props.provider.id },
      throwOnError: true,
    })
    toast.success(t('provider.oauth.revokeSuccess'))
    await fetchOAuthStatus()
  } catch (error) {
    toast.error(error instanceof Error ? error.message : t('provider.oauth.revokeFailed'))
  } finally {
    revokeLoading.value = false
  }
}
</script>

<style scoped>
/* pathLength="1" normalizes the stroke so the dash math is length-agnostic: the
   check is fully hidden (offset 1) then drawn on (offset 0). Stroke only — no
   scale — so the glyph appears by being drawn, not by popping in. */
.check-draw path {
  stroke-dasharray: 1;
  stroke-dashoffset: 1;
  animation: check-draw 0.3s cubic-bezier(0.65, 0, 0.35, 1) forwards;
}

@keyframes check-draw {
  to {
    stroke-dashoffset: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .check-draw path {
    animation: none;
    stroke-dashoffset: 0;
  }
}
</style>
