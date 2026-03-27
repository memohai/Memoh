<template>
  <form @submit="editProvider">
    <div class="space-y-4">
      <FormField
        v-slot="{ value, handleChange }"
        name="enable"
      >
        <FormItem class="flex flex-row items-center justify-between rounded-lg border p-3 shadow-sm">
          <div class="space-y-0.5">
            <Label class="text-base">
              {{ $t('provider.enable') }}
            </Label>
            <p class="text-[0.8rem] text-muted-foreground">
              {{ $t('provider.enableHint') }}
            </p>
          </div>
          <FormControl>
            <Switch
              :model-value="value"
              @update:model-value="handleChange"
            />
          </FormControl>
        </FormItem>
      </FormField>

      <section class="space-y-2">
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          {{ $t('common.name') }}
        </h4>
        <FormField
          v-slot="{ componentField }"
          name="name"
        >
          <FormItem>
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
        v-if="form.values.client_type !== 'openai-codex'"
        class="space-y-2"
      >
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          {{ $t('provider.apiKey') }}
        </h4>
        <FormField
          v-slot="{ componentField }"
          name="api_key"
        >
          <FormItem>
            <FormControl>
              <Input
                type="password"
                :placeholder="providerWithAuth?.api_key || $t('provider.apiKeyPlaceholder')"
                :aria-label="$t('provider.apiKey')"
                v-bind="componentField"
              />
            </FormControl>
          </FormItem>
        </FormField>
      </section>

      <section class="space-y-2">
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          {{ $t('provider.url') }}
        </h4>
        <FormField
          v-slot="{ componentField }"
          name="base_url"
        >
          <FormItem>
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
        <h4 class="scroll-m-20 font-semibold tracking-tight">
          {{ $t('provider.clientType') }}
        </h4>
        <FormField
          v-slot="{ value, handleChange }"
          name="client_type"
        >
          <FormItem>
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
        v-if="form.values.client_type === 'openai-codex'"
        class="rounded-lg border p-4 space-y-3 text-sm"
      >
        <div class="space-y-1">
          <div class="font-medium">
            {{ $t('provider.oauth.title') }}
          </div>
          <div class="text-muted-foreground">
            {{ $t('provider.oauth.description') }}
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
              {{ $t('provider.oauth.status.authorized') }}
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
        <div class="flex gap-2">
          <LoadingButton
            type="button"
            variant="outline"
            :disabled="!canAuthorizeOAuth"
            :loading="authorizeLoading"
            @click="handleAuthorize"
          >
            <FontAwesomeIcon :icon="['fas', 'key']" />
            {{ $t('provider.oauth.authorize') }}
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
        <FontAwesomeIcon
          v-if="!testLoading"
          :icon="['fas', 'rotate']"
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
              <FontAwesomeIcon :icon="['far', 'trash-can']" />
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
      class="mt-4 rounded-lg border p-4 space-y-3 text-sm"
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
  FormItem,
  Label,
  Switch,
} from '@memohai/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import StatusDot from '@/components/status-dot/index.vue'
import LoadingButton from '@/components/loading-button/index.vue'
import SearchableSelectPopover from '@/components/searchable-select-popover/index.vue'
import { CLIENT_TYPE_LIST, CLIENT_TYPE_META } from '@/constants/client-types'
import { computed, ref, watch } from 'vue'
import { toTypedSchema } from '@vee-validate/zod'
import z from 'zod'
import { useForm } from 'vee-validate'
import { postProvidersByIdTest } from '@memohai/sdk'
import type { ProvidersGetResponse, ProvidersTestResponse } from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'

const { t } = useI18n()

type ProviderWithAuth = Partial<ProvidersGetResponse>

type ProviderOAuthStatus = {
  configured: boolean
  has_token: boolean
  expired: boolean
  callback_url?: string
  expires_at?: string
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
const apiBase = import.meta.env.VITE_API_URL?.trim() || '/api'

const providerWithAuth = computed(() => props.provider as ProviderWithAuth | undefined)

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
  base_url: z.string().min(1),
  api_key: z.string().optional(),
  client_type: z.string().min(1),
  metadata: z.object({
    additionalProp1: z.object({}),
  }),
}).superRefine((value, ctx) => {
  if (value.client_type !== 'openai-codex' && !value.api_key?.trim() && !providerWithAuth.value?.api_key) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['api_key'],
      message: 'API key is required',
    })
  }
}))

const form = useForm({
  validationSchema: providerSchema,
})

watch(() => props.provider, (newVal) => {
  if (newVal) {
    form.setValues({
      enable: newVal.enable ?? true,
      name: newVal.name,
      base_url: newVal.base_url,
      api_key: '',
      client_type: newVal.client_type || 'openai-completions',
    })
  }
}, { immediate: true })

watch(() => form.values.client_type, (clientType) => {
  if (clientType !== 'openai-codex') {
    oauthStatus.value = null
    return
  }
  if (!form.values.base_url) {
    form.setFieldValue('base_url', 'https://chatgpt.com/backend-api')
  }
})

watch(() => [props.provider?.id, form.values.client_type] as const, async ([id, clientType]) => {
  if (!id || clientType !== 'openai-codex') {
    oauthStatus.value = null
    return
  }
  await fetchOAuthStatus()
}, { immediate: true })

const hasChanges = computed(() => {
  const raw = props.provider
  const baseChanged = JSON.stringify({
    enable: form.values.enable,
    name: form.values.name,
    base_url: form.values.base_url,
    client_type: form.values.client_type,
    metadata: form.values.metadata,
  }) !== JSON.stringify({
    enable: raw?.enable ?? true,
    name: raw?.name,
    base_url: raw?.base_url,
    client_type: raw?.client_type || 'openai-completions',
    metadata: { additionalProp1: {} },
  })

  const apiKeyChanged = Boolean(form.values.api_key && form.values.api_key.trim() !== '')
  return baseChanged || apiKeyChanged
})

const editProvider = form.handleSubmit(async (value) => {
  const payload: Record<string, unknown> = {
    enable: value.enable,
    name: value.name,
    base_url: value.base_url,
    client_type: value.client_type,
    metadata: value.metadata,
  }
  if (value.api_key && value.api_key.trim() !== '') {
    payload.api_key = value.api_key
  }
  emit('submit', payload)
})

const oauthExpired = computed(() => Boolean(oauthStatus.value?.has_token && oauthStatus.value?.expired))
const canAuthorizeOAuth = computed(() =>
  Boolean(
    props.provider?.id
    && form.values.client_type === 'openai-codex',
  ) && !oauthStatusLoading.value,
)

function authHeaders(): Record<string, string> {
  const token = localStorage.getItem('token')
  return token ? { Authorization: `Bearer ${token}` } : {}
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

async function handleAuthorize() {
  if (!props.provider?.id) return
  authorizeLoading.value = true
  try {
    const response = await fetch(`${apiBase}/providers/${props.provider.id}/oauth/authorize`, {
      headers: authHeaders(),
    })
    if (!response.ok) throw new Error(t('provider.oauth.authorizeFailed'))
    const data = await response.json() as { auth_url?: string }
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

async function handleRevoke() {
  if (!props.provider?.id) return
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
