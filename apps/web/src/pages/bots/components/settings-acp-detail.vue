<!-- eslint-disable vue/no-mutating-props -->
<template>
  <div class="space-y-8">
    <!-- Identity card: which agent this is. The brand mark is signal (which
         agent), not decoration. Mirrors the provider detail header so every
         backend reads the same way. -->
    <section class="flex items-center gap-3 rounded-[var(--radius-menu-shell)] border border-border bg-card px-4 py-3">
      <span class="flex size-9 shrink-0 items-center justify-center">
        <component
          :is="acpAgentIcon(profile.id, true)"
          class="size-5"
        />
      </span>
      <h2 class="truncate text-sm font-semibold">
        {{ profile.display_name || profile.id }}
      </h2>
    </section>

    <SettingsSection>
      <div class="space-y-5 p-4">
        <SegmentedControl
          :model-value="agent.setup_mode"
          :items="setupModeItems"
          :aria-label="$t('bots.settings.acpSetupMode')"
          class="w-full sm:w-fit"
          @update:model-value="(mode) => setSetupMode(String(mode))"
        />

        <template v-if="agent.setup_mode !== 'self'">
          <div
            v-if="isCodex && agent.setup_mode === 'oauth'"
            class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"
          >
            <p
              class="min-w-0 flex-1 text-sm"
              :class="codexOAuthTextClass()"
            >
              {{ codexOAuthStatusText() }}
            </p>
            <div class="flex shrink-0 items-center gap-2">
              <Button
                v-if="codexOAuthFlow"
                type="button"
                variant="ghost"
                @click="cancelCodexOAuthAuthorization"
              >
                {{ $t('common.cancel') }}
              </Button>
              <Button
                type="button"
                variant="outline"
                :disabled="authorizingCodexOAuth"
                @click="handleAuthorize"
              >
                <LoaderCircle
                  v-if="authorizingCodexOAuth"
                  class="size-4 animate-spin"
                />
                {{ $t('bots.settings.acpOAuthAuthorizeCodex') }}
              </Button>
            </div>
          </div>

          <div
            v-if="isClaude && agent.setup_mode === 'oauth'"
            class="space-y-4"
          >
            <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <p
                class="min-w-0 flex-1 text-sm"
                :class="claudeOAuthTextClass()"
              >
                {{ claudeOAuthStatusText() }}
              </p>
              <Button
                type="button"
                variant="outline"
                class="shrink-0"
                :disabled="authorizingClaudeOAuth"
                @click="handleAuthorizeClaude"
              >
                <LoaderCircle
                  v-if="authorizingClaudeOAuth"
                  class="size-4 animate-spin"
                />
                {{ $t('bots.settings.acpOAuthAuthorizeClaudeCode') }}
              </Button>
            </div>

            <div
              v-if="claudeOAuthSessionId && !claudeOAuthStatus?.has_token"
              class="space-y-2"
            >
              <p class="text-sm text-muted-foreground">
                {{ $t('bots.settings.acpClaudeOAuthCodeHint') }}
              </p>
              <div class="flex flex-col gap-2 sm:flex-row">
                <Input
                  v-model="claudeOAuthCode"
                  :placeholder="$t('bots.settings.acpClaudeOAuthCodePlaceholder')"
                  class="min-w-0 flex-1"
                />
                <Button
                  type="button"
                  class="shrink-0"
                  :disabled="exchangingClaudeOAuth"
                  @click="handleExchangeClaudeOAuth"
                >
                  <LoaderCircle
                    v-if="exchangingClaudeOAuth"
                    class="size-4 animate-spin"
                  />
                  {{ $t('bots.settings.acpClaudeOAuthExchange') }}
                </Button>
              </div>
            </div>
          </div>

          <div
            v-for="field in visibleManagedFields"
            :key="field.id"
            class="space-y-1.5"
          >
            <Label class="text-sm font-medium text-foreground">
              {{ field.label || field.id }}
            </Label>
            <Select
              v-if="isHermesProviderField(field)"
              :model-value="hermesProvider"
              @update:model-value="(value) => setHermesProvider(String(value))"
            >
              <SelectTrigger class="w-full">
                <SelectValue :placeholder="$t('bots.settings.acpHermesProviderPlaceholder')" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem
                  v-for="provider in HERMES_PROVIDER_PRESETS"
                  :key="provider.value"
                  :value="provider.value"
                >
                  {{ $t(provider.labelKey) }}
                </SelectItem>
              </SelectContent>
            </Select>
            <template v-else-if="isHermesModelField(field)">
              <Select
                :model-value="hermesModelSelect"
                @update:model-value="(value) => setHermesModel(String(value))"
              >
                <SelectTrigger class="w-full">
                  <SelectValue :placeholder="$t('bots.settings.acpHermesModelPlaceholder')" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem
                    v-for="model in hermesModelOptions"
                    :key="model.value"
                    :value="model.value"
                  >
                    {{ model.label }}
                  </SelectItem>
                  <SelectItem :value="HERMES_CUSTOM_MODEL_VALUE">
                    {{ $t('bots.settings.acpHermesCustomModel') }}
                  </SelectItem>
                </SelectContent>
              </Select>
              <Input
                v-if="hermesUsingCustomModel"
                class="mt-2"
                :model-value="agent.managed.model || ''"
                :name="managedFieldName(field)"
                autocomplete="off"
                autocapitalize="off"
                autocorrect="off"
                spellcheck="false"
                :placeholder="$t('bots.settings.acpHermesCustomModelPlaceholder')"
                @update:model-value="(val) => setManagedField(field.id, String(val ?? ''))"
                @change="commitForm"
              />
            </template>
            <Input
              v-else
              :model-value="agent.managed[field.id || ''] || ''"
              :type="inputType(field.type)"
              :name="managedFieldName(field)"
              :autocomplete="managedFieldAutocomplete(field)"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
              :placeholder="managedFieldPlaceholder(field)"
              @update:model-value="(val) => setManagedField(field.id, String(val ?? ''))"
              @change="commitForm"
            />
            <p
              v-if="field.help && !isHermesProviderField(field) && !isHermesModelField(field)"
              class="text-sm text-muted-foreground"
            >
              {{ field.help }}
            </p>
          </div>
        </template>

        <p
          v-else
          class="break-words text-sm text-muted-foreground"
        >
          {{ selfModeHint }}
        </p>
        <Button
          v-if="isHermesSelfConfirmVisible"
          size="sm"
          class="mt-3"
          @click="confirmSelfMode"
        >
          {{ $t('bots.settings.acpHermesSelfModeConfirm') }}
        </Button>
      </div>
    </SettingsSection>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from '@memohai/ui'
import { useQueryCache } from '@pinia/colada'
import {
  Button,
  Input,
  Label,
  SegmentedControl,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  type SegmentedItem,
} from '@memohai/ui'
import { LoaderCircle } from 'lucide-vue-next'
import { client } from '@memohai/sdk/client'
import {
  type AcpprofileManagedField,
  type AcpprofilePublicProfile,
} from '@memohai/sdk'
import {
  HERMES_CUSTOM_MODEL_VALUE,
  HERMES_PROVIDER_PRESETS,
  acpAgentIcon,
  ensureACPAgentForm,
  ensureHermesManagedDefaults,
  findMissingRequiredManagedField,
  hermesAPIKeyPlaceholder,
  hermesDefaultModel,
  hermesModelPresets,
  hermesModelSelectValue,
  hermesProviderValue,
  isClaudeCodeAgent,
  isCodexAgent,
  isHermesCustomProvider,
  isHermesPresetModel,
  normalizeACPAgentID,
  type ACPAgentForm,
  type ACPForm,
} from '@/utils/acp'
import { startOAuthPopupFlow, type OAuthPopupFlowController } from '@/utils/oauth/popup-flow'
import { oauthStatusTextKey } from '@/utils/oauth/status-text'
import SettingsSection from '@/components/settings/section.vue'

const props = defineProps<{
  botId: string
  profile: AcpprofilePublicProfile
  form: ACPForm
  pendingSelfConfirm?: boolean
}>()

interface ACPDetailCommitOptions {
  confirmSelf?: boolean
}

const emit = defineEmits<{
  commit: [options?: ACPDetailCommitOptions]
}>()

const { t } = useI18n()
const queryCache = useQueryCache()
const codexOAuthStatus = ref<ACPCodexOAuthStatus | null>(null)
const codexOAuthStatusLoading = ref(false)
const authorizingCodexOAuth = ref(false)
const codexOAuthFlow = ref<OAuthPopupFlowController | null>(null)
const claudeOAuthStatus = ref<ACPClaudeCodeOAuthStatus | null>(null)
const claudeOAuthStatusLoading = ref(false)
const authorizingClaudeOAuth = ref(false)
const exchangingClaudeOAuth = ref(false)
const claudeOAuthSessionId = ref('')
const claudeOAuthCode = ref('')
const codexOAuthPollIntervalMs = 1500
const codexOAuthPollTimeoutMs = 5 * 60 * 1000

interface ACPCodexOAuthStatus {
  configured: boolean
  has_token: boolean
  callback_url: string
  account_id?: string
}

interface ACPCodexOAuthAuthorizeResponse {
  auth_url: string
}

interface ACPClaudeCodeOAuthStatus {
  configured: boolean
  has_token: boolean
}

interface ACPClaudeCodeOAuthAuthorizeResponse {
  auth_url: string
  session_id: string
}

interface OAuthStatusLoadOptions {
  silent?: boolean
}

const agent = computed<ACPAgentForm>(() => ensureACPAgentForm(props.form, props.profile))
const isCodex = computed(() => isCodexAgent(props.profile.id))
const isClaude = computed(() => isClaudeCodeAgent(props.profile.id))
const isHermes = computed(() => normalizeACPAgentID(props.profile.id) === 'hermes')
const hermesProvider = computed(() => hermesProviderValue(agent.value.managed.provider))
const hermesModelOptions = computed(() => hermesModelPresets(hermesProvider.value))
const hermesModelSelect = computed(() => hermesModelSelectValue(hermesProvider.value, agent.value.managed.model))
const hermesUsingCustomModel = computed(() => hermesModelSelect.value === HERMES_CUSTOM_MODEL_VALUE)
const isHermesSelfConfirmVisible = computed(() =>
  isHermes.value && props.pendingSelfConfirm === true && agent.value.enabled && agent.value.setup_mode === 'self',
)
const selfModeHint = computed(() => isHermes.value
  ? t('bots.settings.acpHermesSelfModeHint')
  : t('bots.settings.acpSelfModeHint'))

function setupModes(): string[] {
  const modes = props.profile.setup_modes?.filter(Boolean) ?? []
  return modes.length > 0 ? modes : ['api_key']
}

function setupModeLabel(mode: string): string {
  if (mode === 'api_key') return t('bots.settings.acpSetupApiKey')
  if (mode === 'oauth') {
    if (isCodex.value) return t('bots.settings.acpSetupChatGPT')
    if (isClaude.value) return t('bots.settings.acpSetupClaude')
    return t('bots.settings.acpSetupOAuth')
  }
  if (mode === 'self') return t('bots.settings.acpSetupSelf')
  return mode
}

const setupModeItems = computed<SegmentedItem<string>[]>(() =>
  setupModes().map(mode => ({
    value: mode,
    label: setupModeLabel(mode),
  })),
)

function commitForm() {
  if (agent.value.enabled && findMissingRequiredManagedField(props.profile, agent.value.managed, agent.value.setup_mode)) {
    return
  }
  emit('commit')
}

function confirmSelfMode() {
  if (!isHermesSelfConfirmVisible.value) return
  emit('commit', { confirmSelf: true })
}

function setSetupMode(mode: string) {
  agent.value.setup_mode = mode
  if (isHermes.value && mode === 'api_key') ensureHermesManagedDefaults(agent.value.managed)
  if (isCodex.value && mode === 'oauth') void loadOAuthStatus()
  if (isClaude.value && mode === 'oauth') void loadClaudeOAuthStatus()
  commitForm()
}

function inputType(type: string | undefined): string {
  if (type === 'password') return 'password'
  if (type === 'url') return 'url'
  return 'text'
}

function managedFieldName(field: AcpprofileManagedField): string {
  return `acp-${normalizeACPAgentID(props.profile.id) || 'agent'}-${normalizeACPAgentID(field.id) || 'field'}`
}

function managedFieldAutocomplete(field: AcpprofileManagedField): string {
  return field.type === 'password' ? 'new-password' : 'off'
}

function managedFieldPlaceholder(field: AcpprofileManagedField): string | undefined {
  if (isHermes.value && normalizeACPAgentID(field.id) === 'api_key') {
    return hermesAPIKeyPlaceholder(hermesProvider.value, field.placeholder)
  }
  return field.placeholder
}

function setManagedField(fieldID: string | undefined, value: string) {
  const id = normalizeACPAgentID(fieldID)
  if (!id) return
  agent.value.managed[id] = value
}

function isHermesProviderField(field: AcpprofileManagedField): boolean {
  return isHermes.value && normalizeACPAgentID(field.id) === 'provider'
}

function isHermesModelField(field: AcpprofileManagedField): boolean {
  return isHermes.value && normalizeACPAgentID(field.id) === 'model'
}

function setHermesProvider(value: string) {
  const provider = hermesProviderValue(value)
  agent.value.managed.provider = provider
  agent.value.managed.model = hermesDefaultModel(provider)
  if (!isHermesCustomProvider(provider)) agent.value.managed.base_url = ''
  commitForm()
}

function setHermesModel(value: string) {
  if (value === HERMES_CUSTOM_MODEL_VALUE) {
    if (isHermesPresetModel(hermesProvider.value, agent.value.managed.model)) {
      agent.value.managed.model = ''
    }
  } else {
    agent.value.managed.model = value
  }
  commitForm()
}

const visibleManagedFields = computed<AcpprofileManagedField[]>(() => {
  const mode = agent.value.setup_mode
  return (props.profile.managed_fields ?? []).filter((field) => {
    const id = normalizeACPAgentID(field.id)
    if (id === 'provider_id') return false
    if (isHermes.value && id === 'base_url') return isHermesCustomProvider(hermesProvider.value)
    if (isCodex.value && mode === 'oauth') return false
    if (isClaude.value) {
      if (id === 'api_key') return mode === 'api_key'
      if (id === 'oauth_token') return false
    }
    return true
  })
})

const codexOAuthActive = computed(() => isCodex.value && !!agent.value.enabled && agent.value.setup_mode === 'oauth')
const claudeOAuthActive = computed(() => isClaude.value && !!agent.value.enabled && agent.value.setup_mode === 'oauth')

const codexOAuthPending = computed(() => authorizingCodexOAuth.value || Boolean(codexOAuthFlow.value))
const claudeOAuthPending = computed(() => {
  if (authorizingClaudeOAuth.value || exchangingClaudeOAuth.value) return true
  return Boolean(claudeOAuthSessionId.value && !claudeOAuthStatus.value?.has_token)
})

// Codex/Claude in OAuth mode need their live token status the moment the detail
// opens (or the mode flips), so the status line and authorize button reflect reality.
watch([() => props.botId, () => props.profile.id, () => agent.value.setup_mode], () => {
  if (isHermes.value && agent.value.setup_mode === 'api_key') ensureHermesManagedDefaults(agent.value.managed)
  if (codexOAuthActive.value || (isCodex.value && agent.value.setup_mode === 'oauth')) void loadOAuthStatus()
  if (claudeOAuthActive.value || (isClaude.value && agent.value.setup_mode === 'oauth')) void loadClaudeOAuthStatus()
}, { immediate: true })

onBeforeUnmount(() => {
  cancelCodexOAuthAuthorization()
})

function codexOAuthStatusText(): string {
  return t(oauthStatusTextKey({
    loading: codexOAuthStatusLoading.value,
    authorizing: codexOAuthPending.value,
    status: codexOAuthStatus.value,
    unavailableKey: 'bots.settings.acpOAuthUnavailable',
  }))
}

function codexOAuthTextClass(): string {
  return codexOAuthStatusLoading.value || codexOAuthPending.value || codexOAuthStatus.value?.has_token
    ? 'text-muted-foreground'
    : 'text-destructive'
}

function claudeOAuthStatusText(): string {
  return t(oauthStatusTextKey({
    loading: claudeOAuthStatusLoading.value,
    authorizing: claudeOAuthPending.value,
    status: claudeOAuthStatus.value,
    unavailableKey: 'bots.settings.acpClaudeOAuthUnavailable',
  }))
}

function claudeOAuthTextClass(): string {
  return claudeOAuthStatusLoading.value || claudeOAuthPending.value || claudeOAuthStatus.value?.has_token
    ? 'text-muted-foreground'
    : 'text-destructive'
}

async function loadOAuthStatus(options: OAuthStatusLoadOptions = {}): Promise<ACPCodexOAuthStatus | null> {
  if (!props.botId) return null
  if (!options.silent) codexOAuthStatusLoading.value = true
  try {
    const { data } = await client.get<{ 200: ACPCodexOAuthStatus }, unknown, true>({
      url: '/bots/{bot_id}/acp/codex/oauth/status',
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    codexOAuthStatus.value = data ?? null
    return codexOAuthStatus.value
  } catch {
    if (!options.silent) codexOAuthStatus.value = null
    return null
  } finally {
    if (!options.silent) codexOAuthStatusLoading.value = false
  }
}

async function loadClaudeOAuthStatus(): Promise<ACPClaudeCodeOAuthStatus | null> {
  if (!props.botId) return null
  claudeOAuthStatusLoading.value = true
  try {
    const { data } = await client.get<{ 200: ACPClaudeCodeOAuthStatus }, unknown, true>({
      url: '/bots/{bot_id}/acp/claude-code/oauth/status',
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    claudeOAuthStatus.value = data ?? null
    if (data?.has_token) {
      agent.value.managed.oauth_token = agent.value.managed.oauth_token || '***'
    }
    return claudeOAuthStatus.value
  } catch {
    claudeOAuthStatus.value = null
    return null
  } finally {
    claudeOAuthStatusLoading.value = false
  }
}

function cancelCodexOAuthAuthorization() {
  codexOAuthFlow.value?.cancel()
}

async function handleAuthorize() {
  try {
    if (!props.botId) return
    // Supersede any in-flight popup silently: dispose() (not cancel()) so the
    // previous flow's onAborted doesn't fight the new attempt's loading state.
    codexOAuthFlow.value?.dispose()
    agent.value.setup_mode = 'oauth'
    authorizingCodexOAuth.value = true
    const { data } = await client.get<{ 200: ACPCodexOAuthAuthorizeResponse }, unknown, true>({
      url: '/bots/{bot_id}/acp/codex/oauth/authorize',
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    if (!data?.auth_url) throw new Error(t('provider.oauth.authorizeFailed'))
    const popup = window.open(data.auth_url, 'acp-codex-oauth', 'width=600,height=720')
    if (!popup) throw new Error(t('provider.oauth.authorizeFailed'))
    codexOAuthFlow.value = startOAuthPopupFlow<ACPCodexOAuthStatus>({
      popup,
      target: window,
      expectedSource: popup,
      messageType: 'memoh-acp-codex-oauth-success',
      messageMatches: event => event.data?.botId === props.botId,
      pollIntervalMs: codexOAuthPollIntervalMs,
      timeoutMs: codexOAuthPollTimeoutMs,
      pollStatus: () => loadOAuthStatus({ silent: true }),
      isAuthorized: status => Boolean(status?.has_token),
      onAuthorized: async () => {
        codexOAuthFlow.value = null
        await loadOAuthStatus({ silent: true })
        toast.success(t('provider.oauth.authorizeSuccess'))
        authorizingCodexOAuth.value = false
      },
      onAborted: (reason) => {
        codexOAuthFlow.value = null
        authorizingCodexOAuth.value = false
        if (reason === 'timeout') {
          toast.error(t('provider.oauth.authorizeTimedOut'))
        }
      },
    })
  } catch (error) {
    cancelCodexOAuthAuthorization()
    authorizingCodexOAuth.value = false
    toast.error(error instanceof Error ? error.message : t('provider.oauth.authorizeFailed'))
  }
}

async function handleAuthorizeClaude() {
  try {
    agent.value.setup_mode = 'oauth'
    authorizingClaudeOAuth.value = true
    const { data } = await client.get<{ 200: ACPClaudeCodeOAuthAuthorizeResponse }, unknown, true>({
      url: '/bots/{bot_id}/acp/claude-code/oauth/authorize',
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    if (!data?.auth_url || !data.session_id) throw new Error(t('provider.oauth.authorizeFailed'))
    claudeOAuthSessionId.value = data.session_id
    claudeOAuthCode.value = ''
    window.open(data.auth_url, 'acp-claude-code-oauth', 'width=600,height=720')
  } catch (error) {
    toast.error(error instanceof Error ? error.message : t('provider.oauth.authorizeFailed'))
  } finally {
    authorizingClaudeOAuth.value = false
  }
}

async function handleExchangeClaudeOAuth() {
  const code = claudeOAuthCode.value.trim()
  if (!code) {
    toast.error(t('bots.settings.acpClaudeOAuthCodeRequired'))
    return
  }
  try {
    exchangingClaudeOAuth.value = true
    const { data } = await client.post<{ 200: ACPClaudeCodeOAuthStatus }, unknown, true>({
      url: '/bots/{bot_id}/acp/claude-code/oauth/exchange',
      path: { bot_id: props.botId },
      body: {
        session_id: claudeOAuthSessionId.value,
        code,
      },
      throwOnError: true,
    })
    claudeOAuthStatus.value = data ?? { configured: true, has_token: true }
    agent.value.enabled = true
    agent.value.setup_mode = 'oauth'
    agent.value.managed.oauth_token = '***'
    claudeOAuthSessionId.value = ''
    claudeOAuthCode.value = ''
    commitForm()
    void queryCache.invalidateQueries({ key: ['bot', props.botId] })
    void queryCache.invalidateQueries({ key: ['bots'] })
    toast.success(t('provider.oauth.authorizeSuccess'))
  } catch (error) {
    toast.error(error instanceof Error ? error.message : t('bots.settings.acpClaudeOAuthExchangeFailed'))
  } finally {
    exchangingClaudeOAuth.value = false
  }
}
</script>
