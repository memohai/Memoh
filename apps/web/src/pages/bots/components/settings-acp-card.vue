<!-- eslint-disable vue/no-mutating-props -->
<template>
  <SettingsSection :title="$t('bots.settings.blocks.acp')">
    <div
      v-if="loading"
      class="mx-4 flex min-h-[3.75rem] items-center gap-3 py-3 text-sm text-muted-foreground"
    >
      <LoaderCircle class="size-4 animate-spin" />
      {{ $t('common.loading') }}
    </div>

    <div
      v-else-if="profiles.length === 0"
      class="p-4"
    >
      <Empty class="rounded-[var(--radius-menu-shell)] border border-dashed border-border py-12">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <BotIcon />
          </EmptyMedia>
          <EmptyTitle>{{ $t('bots.settings.acpEmptyTitle') }}</EmptyTitle>
          <EmptyDescription>{{ $t('bots.settings.acpEmptyDescription') }}</EmptyDescription>
        </EmptyHeader>
      </Empty>
    </div>

    <template v-else>
      <div
        v-for="profile in profiles"
        :key="profile.id"
        class="mx-4 border-b border-border py-4 last:border-b-0"
      >
        <div class="flex min-h-[3.75rem] items-center justify-between gap-4">
          <div class="flex min-w-0 items-center gap-3">
            <span class="flex size-8 shrink-0 items-center justify-center">
              <component
                :is="acpAgentIcon(profile.id, true)"
                class="size-5"
              />
            </span>
            <div class="min-w-0">
              <Label class="truncate text-sm font-medium text-foreground">
                {{ profile.display_name || profile.id }}
              </Label>
              <p
                v-if="profile.description"
                class="mt-0.5 text-xs text-muted-foreground"
              >
                {{ profile.description }}
              </p>
            </div>
          </div>

          <div class="flex shrink-0 items-center gap-3">
            <span class="hidden text-xs text-muted-foreground sm:inline">
              {{ agentForm(profile).enabled ? $t('common.enabled') : $t('common.disabled') }}
            </span>
            <Switch
              :model-value="agentForm(profile).enabled"
              @update:model-value="(val) => setAgentEnabled(profile, !!val)"
            />
          </div>
        </div>

        <div
          v-if="agentForm(profile).enabled"
          class="mt-4 space-y-4 pl-0 sm:pl-11"
        >
          <div class="space-y-2">
            <Label class="text-sm font-medium text-foreground">
              {{ $t('bots.settings.acpSetupMode') }}
            </Label>
            <SegmentedControl
              :model-value="agentForm(profile).setup_mode"
              :items="setupModeItems(profile)"
              :aria-label="$t('bots.settings.acpSetupMode')"
              class="w-full sm:w-fit"
              @update:model-value="(mode) => setSetupMode(profile, String(mode))"
            />
          </div>

          <div
            v-if="agentForm(profile).setup_mode !== 'self'"
            class="space-y-4"
          >
            <div
              v-if="isCodexProfile(profile) && agentForm(profile).setup_mode === 'oauth'"
              class="flex flex-col gap-3 border-t border-border pt-4 sm:flex-row sm:items-center sm:justify-between"
            >
              <p
                class="min-w-0 flex-1 text-xs"
                :class="codexOAuthTextClass()"
              >
                {{ codexOAuthStatusText() }}
              </p>
              <div class="flex shrink-0 items-center gap-2">
                <Button
                  v-if="codexOAuthFlow"
                  type="button"
                  size="sm"
                  variant="ghost"
                  @click="cancelCodexOAuthAuthorization"
                >
                  {{ $t('common.cancel') }}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  :disabled="authorizingCodexOAuth"
                  @click="handleAuthorize(profile)"
                >
                  <LoaderCircle
                    v-if="authorizingCodexOAuth"
                    class="size-3 animate-spin"
                  />
                  {{ $t('bots.settings.acpOAuthAuthorizeCodex') }}
                </Button>
              </div>
            </div>

            <div
              v-if="isClaudeCodeProfile(profile) && agentForm(profile).setup_mode === 'oauth'"
              class="space-y-4 border-t border-border pt-4"
            >
              <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <p
                  class="min-w-0 flex-1 text-xs"
                  :class="claudeOAuthTextClass()"
                >
                  {{ claudeOAuthStatusText() }}
                </p>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  class="shrink-0"
                  :disabled="authorizingClaudeOAuth"
                  @click="handleAuthorizeClaude(profile)"
                >
                  <LoaderCircle
                    v-if="authorizingClaudeOAuth"
                    class="size-3 animate-spin"
                  />
                  {{ $t('bots.settings.acpOAuthAuthorizeClaudeCode') }}
                </Button>
              </div>

              <div
                v-if="claudeOAuthSessionId && !claudeOAuthStatus?.has_token"
                class="space-y-2"
              >
                <p class="text-xs text-muted-foreground">
                  {{ $t('bots.settings.acpClaudeOAuthCodeHint') }}
                </p>
                <div class="flex flex-col gap-2 sm:flex-row">
                  <Input
                    v-model="claudeOAuthCode"
                    :placeholder="$t('bots.settings.acpClaudeOAuthCodePlaceholder')"
                    class="h-8 min-w-0 flex-1"
                  />
                  <Button
                    type="button"
                    size="sm"
                    class="shrink-0"
                    :disabled="exchangingClaudeOAuth"
                    @click="handleExchangeClaudeOAuth(profile)"
                  >
                    <LoaderCircle
                      v-if="exchangingClaudeOAuth"
                      class="size-3 animate-spin"
                    />
                    {{ $t('bots.settings.acpClaudeOAuthExchange') }}
                  </Button>
                </div>
              </div>
            </div>

            <div
              v-for="field in visibleManagedFields(profile)"
              :key="field.id"
              class="space-y-1.5"
            >
              <Label class="text-xs font-medium text-foreground">
                {{ field.label || field.id }}
              </Label>
              <Input
                :model-value="agentForm(profile).managed[field.id || ''] || ''"
                :type="inputType(field.type)"
                :name="managedFieldName(profile, field)"
                :autocomplete="managedFieldAutocomplete(field)"
                autocapitalize="off"
                autocorrect="off"
                spellcheck="false"
                :placeholder="field.placeholder"
                class="h-8"
                @update:model-value="(val) => setManagedField(profile, field.id, String(val ?? ''))"
                @change="commitForm"
                @blur="commitForm"
                @keydown.enter="commitForm"
              />
              <p
                v-if="field.help"
                class="text-xs text-muted-foreground"
              >
                {{ field.help }}
              </p>
            </div>
          </div>

          <div
            v-else
            class="break-words border-t border-border pt-4 text-xs text-muted-foreground"
          >
            {{ $t('bots.settings.acpSelfModeHint') }}
          </div>
        </div>
      </div>
    </template>
  </SettingsSection>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from '@memohai/ui'
import { useQueryCache } from '@pinia/colada'
import {
  Button,
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  Input,
  Label,
  SegmentedControl,
  Switch,
  type SegmentedItem,
} from '@memohai/ui'
import { Bot as BotIcon, LoaderCircle } from 'lucide-vue-next'
import { client } from '@memohai/sdk/client'
import {
  type AcpprofileManagedField,
  type AcpprofilePublicProfile,
} from '@memohai/sdk'
import { acpAgentIcon, ensureACPAgentForm, normalizeACPAgentID, type ACPAgentForm, type ACPForm } from '@/utils/acp'
import { startOAuthPopupFlow, type OAuthPopupFlowController } from '@/utils/oauth/popup-flow'
import { oauthStatusTextKey } from '@/utils/oauth/status-text'
import SettingsSection from '@/components/settings/section.vue'

const props = defineProps<{
  botId: string
  profiles: AcpprofilePublicProfile[]
  form: ACPForm
  loading?: boolean
}>()

const emit = defineEmits<{
  commit: []
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

function agentForm(profile: AcpprofilePublicProfile): ACPAgentForm {
  return ensureACPAgentForm(props.form, profile)
}

function setupModes(profile: AcpprofilePublicProfile): string[] {
  const modes = profile.setup_modes?.filter(Boolean) ?? []
  return modes.length > 0 ? modes : ['api_key']
}

function setupModeLabel(mode: string, profile: AcpprofilePublicProfile): string {
  if (mode === 'api_key') return t('bots.settings.acpSetupApiKey')
  if (mode === 'oauth') {
    if (isCodexProfile(profile)) return t('bots.settings.acpSetupChatGPT')
    if (isClaudeCodeProfile(profile)) return t('bots.settings.acpSetupClaude')
    return t('bots.settings.acpSetupOAuth')
  }
  if (mode === 'self') return t('bots.settings.acpSetupSelf')
  return mode
}

function setupModeItems(profile: AcpprofilePublicProfile): SegmentedItem<string>[] {
  return setupModes(profile).map(mode => ({
    value: mode,
    label: setupModeLabel(mode, profile),
  }))
}

function commitForm() {
  emit('commit')
}

function setAgentEnabled(profile: AcpprofilePublicProfile, enabled: boolean) {
  agentForm(profile).enabled = enabled
  commitForm()
}

function setSetupMode(profile: AcpprofilePublicProfile, mode: string) {
  const form = agentForm(profile)
  form.setup_mode = mode
  if (isCodexProfile(profile) && mode === 'oauth') {
    void loadOAuthStatus()
  }
  if (isClaudeCodeProfile(profile) && mode === 'oauth') {
    void loadClaudeOAuthStatus()
  }
  commitForm()
}

function inputType(type: string | undefined): string {
  if (type === 'password') return 'password'
  if (type === 'url') return 'url'
  return 'text'
}

function managedFieldName(profile: AcpprofilePublicProfile, field: AcpprofileManagedField): string {
  return `acp-${normalizeACPAgentID(profile.id) || 'agent'}-${normalizeACPAgentID(field.id) || 'field'}`
}

function managedFieldAutocomplete(field: AcpprofileManagedField): string {
  return field.type === 'password' ? 'new-password' : 'off'
}

function setManagedField(profile: AcpprofilePublicProfile, fieldID: string | undefined, value: string) {
  const id = normalizeACPAgentID(fieldID)
  if (!id) return
  agentForm(profile).managed[id] = value
}

function isCodexProfile(profile: AcpprofilePublicProfile): boolean {
  return normalizeACPAgentID(profile.id) === 'codex'
}

function isClaudeCodeProfile(profile: AcpprofilePublicProfile): boolean {
  return normalizeACPAgentID(profile.id) === 'claude-code'
}

function visibleManagedFields(profile: AcpprofilePublicProfile): AcpprofileManagedField[] {
  const mode = agentForm(profile).setup_mode
  return (profile.managed_fields ?? []).filter((field) => {
    const id = normalizeACPAgentID(field.id)
    if (id === 'provider_id') return false
    if (isCodexProfile(profile)) {
      if (mode === 'oauth') return false
    }
    if (isClaudeCodeProfile(profile)) {
      if (id === 'api_key') return mode === 'api_key'
      if (id === 'oauth_token') return false
    }
    return true
  })
}

const codexOAuthActive = computed(() => {
  const profile = props.profiles.find(isCodexProfile)
  if (!profile) return false
  const form = agentForm(profile)
  return !!form.enabled && form.setup_mode === 'oauth'
})

const claudeOAuthActive = computed(() => {
  const profile = props.profiles.find(isClaudeCodeProfile)
  if (!profile) return false
  const form = agentForm(profile)
  return !!form.enabled && form.setup_mode === 'oauth'
})

const codexOAuthPending = computed(() => authorizingCodexOAuth.value || Boolean(codexOAuthFlow.value))
const claudeOAuthPending = computed(() => {
  if (authorizingClaudeOAuth.value || exchangingClaudeOAuth.value) return true
  return Boolean(claudeOAuthSessionId.value && !claudeOAuthStatus.value?.has_token)
})

watch([() => props.botId, codexOAuthActive], () => {
  if (codexOAuthActive.value) void loadOAuthStatus()
}, { immediate: true })

watch([() => props.botId, claudeOAuthActive], () => {
  if (claudeOAuthActive.value) void loadClaudeOAuthStatus()
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
      const profile = props.profiles.find(isClaudeCodeProfile)
      if (profile) agentForm(profile).managed.oauth_token = agentForm(profile).managed.oauth_token || '***'
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

async function handleAuthorize(profile: AcpprofilePublicProfile) {
  try {
    if (!props.botId) return
    // Supersede any in-flight popup silently: dispose() (not cancel()) so the
    // previous flow's onAborted doesn't fight the new attempt's loading state.
    codexOAuthFlow.value?.dispose()
    agentForm(profile).setup_mode = 'oauth'
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

async function handleAuthorizeClaude(profile: AcpprofilePublicProfile) {
  try {
    agentForm(profile).setup_mode = 'oauth'
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

async function handleExchangeClaudeOAuth(profile: AcpprofilePublicProfile) {
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
    agentForm(profile).enabled = true
    agentForm(profile).setup_mode = 'oauth'
    agentForm(profile).managed.oauth_token = '***'
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
