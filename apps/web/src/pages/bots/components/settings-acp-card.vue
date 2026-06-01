<!-- eslint-disable vue/no-mutating-props -->
<template>
  <div class="space-y-4 rounded-md border border-border bg-background p-4 shadow-none">
    <div class="space-y-0.5">
      <h4 class="text-xs font-medium text-foreground">
        {{ $t('bots.settings.blocks.acp') }}
      </h4>
      <p class="text-[11px] text-muted-foreground">
        {{ $t('bots.settings.blocks.acpDescription') }}
      </p>
    </div>

    <div
      v-if="loading"
      class="flex items-center gap-2 rounded-md border border-border p-3 text-xs text-muted-foreground"
    >
      <LoaderCircle class="size-3 animate-spin" />
      {{ $t('common.loading') }}
    </div>

    <div
      v-else-if="profiles.length === 0"
      class="rounded-md border border-border p-3 text-xs text-muted-foreground"
    >
      {{ $t('common.noData') }}
    </div>

    <template v-else>
      <div
        v-for="profile in profiles"
        :key="profile.id"
        class="space-y-4 rounded-md border border-border p-3"
      >
        <div class="flex items-start justify-between gap-4">
          <div class="min-w-0 space-y-0.5">
            <div class="flex min-w-0 items-center gap-2">
              <component
                :is="acpAgentIcon(profile.id, true)"
                class="size-4 shrink-0"
              />
              <Label class="truncate text-xs font-medium text-foreground">
                {{ profile.display_name || profile.id }}
              </Label>
            </div>
            <p
              v-if="profile.description"
              class="text-[10px] text-muted-foreground"
            >
              {{ profile.description }}
            </p>
          </div>
          <Switch
            :model-value="agentForm(profile).enabled"
            class="origin-right scale-[0.8] shadow-none"
            @update:model-value="(val) => agentForm(profile).enabled = !!val"
          />
        </div>

        <div
          v-if="agentForm(profile).enabled"
          class="space-y-3 border-t border-border/70 pt-3"
        >
          <div
            v-if="isLocalWorkspace"
            class="rounded-md border border-border/70 bg-muted/30 px-3 py-2 text-[11px] text-muted-foreground"
          >
            {{ $t('bots.settings.acpLocalModeHint') }}
          </div>

          <template v-else>
            <div class="space-y-1.5">
              <Label class="text-xs font-medium text-foreground">
                {{ $t('bots.settings.acpSetupMode') }}
              </Label>
              <div class="grid grid-cols-3 gap-2">
                <button
                  v-for="mode in setupModes(profile)"
                  :key="mode"
                  type="button"
                  class="min-h-8 rounded-md border px-2 py-1 text-[11px] font-medium leading-tight transition-colors"
                  :class="agentForm(profile).setup_mode === mode ? 'border-foreground bg-foreground text-background' : 'border-border bg-background text-foreground hover:bg-muted'"
                  @click="setSetupMode(profile, mode)"
                >
                  {{ setupModeLabel(mode, profile) }}
                </button>
              </div>
            </div>

            <div
              v-if="agentForm(profile).setup_mode !== 'self'"
              class="space-y-3"
            >
              <div
                v-if="isCodexProfile(profile) && agentForm(profile).setup_mode === 'oauth'"
                class="space-y-2 rounded-md border border-border/70 bg-muted/20 p-3"
              >
                <div class="flex items-center justify-between gap-3">
                  <div
                    class="min-w-0 text-[10px]"
                    :class="codexOAuthStatus?.has_token ? 'text-muted-foreground' : 'text-destructive'"
                  >
                    {{ codexOAuthStatusText() }}
                  </div>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    class="h-7 shrink-0 text-xs shadow-none"
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
                class="space-y-2 rounded-md border border-border/70 bg-muted/20 p-3"
              >
                <div class="flex items-center justify-between gap-3">
                  <div
                    class="min-w-0 text-[10px]"
                    :class="claudeOAuthStatus?.has_token ? 'text-muted-foreground' : 'text-destructive'"
                  >
                    {{ claudeOAuthStatusText() }}
                  </div>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    class="h-7 shrink-0 text-xs shadow-none"
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
                  <p class="text-[10px] text-muted-foreground">
                    {{ $t('bots.settings.acpClaudeOAuthCodeHint') }}
                  </p>
                  <div class="flex flex-col gap-2 sm:flex-row">
                    <Input
                      v-model="claudeOAuthCode"
                      :placeholder="$t('bots.settings.acpClaudeOAuthCodePlaceholder')"
                      class="h-8 min-w-0 flex-1 text-xs shadow-none"
                    />
                    <Button
                      type="button"
                      size="sm"
                      class="h-8 shrink-0 text-xs shadow-none"
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
                  class="h-8 text-xs shadow-none"
                  @update:model-value="(val) => setManagedField(profile, field.id, String(val ?? ''))"
                />
                <p
                  v-if="field.help"
                  class="text-[10px] text-muted-foreground"
                >
                  {{ field.help }}
                </p>
              </div>
            </div>

            <div
              v-else
              class="break-words rounded-md border border-border/70 bg-muted/30 px-3 py-2 text-[11px] text-muted-foreground"
            >
              {{ $t('bots.settings.acpSelfModeHint') }}
            </div>
          </template>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { useQueryCache } from '@pinia/colada'
import { Button, Input, Label, Switch } from '@memohai/ui'
import { LoaderCircle } from 'lucide-vue-next'
import { client } from '@memohai/sdk/client'
import {
  type AcpprofileManagedField,
  type AcpprofilePublicProfile,
} from '@memohai/sdk'
import { acpAgentIcon, ensureACPAgentForm, normalizeACPAgentID, type ACPAgentForm, type ACPForm } from '@/utils/acp'

const props = defineProps<{
  botId: string
  profiles: AcpprofilePublicProfile[]
  form: ACPForm
  loading?: boolean
  isLocalWorkspace?: boolean
}>()

const { t } = useI18n()
const queryCache = useQueryCache()
const codexOAuthStatus = ref<ACPCodexOAuthStatus | null>(null)
const codexOAuthStatusLoading = ref(false)
const authorizingCodexOAuth = ref(false)
const claudeOAuthStatus = ref<ACPClaudeCodeOAuthStatus | null>(null)
const claudeOAuthStatusLoading = ref(false)
const authorizingClaudeOAuth = ref(false)
const exchangingClaudeOAuth = ref(false)
const claudeOAuthSessionId = ref('')
const claudeOAuthCode = ref('')

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

function setSetupMode(profile: AcpprofilePublicProfile, mode: string) {
  const form = agentForm(profile)
  form.setup_mode = mode
  if (isCodexProfile(profile) && mode === 'oauth') {
    void loadOAuthStatus()
  }
  if (isClaudeCodeProfile(profile) && mode === 'oauth') {
    void loadClaudeOAuthStatus()
  }
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
  if (!profile || props.isLocalWorkspace) return false
  const form = agentForm(profile)
  return !!form.enabled && form.setup_mode === 'oauth'
})

const claudeOAuthActive = computed(() => {
  const profile = props.profiles.find(isClaudeCodeProfile)
  if (!profile || props.isLocalWorkspace) return false
  const form = agentForm(profile)
  return !!form.enabled && form.setup_mode === 'oauth'
})

watch([() => props.botId, codexOAuthActive], () => {
  if (codexOAuthActive.value) void loadOAuthStatus()
}, { immediate: true })

watch([() => props.botId, claudeOAuthActive], () => {
  if (claudeOAuthActive.value) void loadClaudeOAuthStatus()
}, { immediate: true })

function codexOAuthStatusText(): string {
  return oauthStatusText(codexOAuthStatusLoading.value, codexOAuthStatus.value, 'bots.settings.acpOAuthUnavailable')
}

function claudeOAuthStatusText(): string {
  return oauthStatusText(claudeOAuthStatusLoading.value, claudeOAuthStatus.value, 'bots.settings.acpClaudeOAuthUnavailable')
}

function oauthStatusText(loading: boolean, status: { configured: boolean, has_token: boolean } | null, unavailableKey: string): string {
  if (loading) return t('provider.oauth.status.checking')
  if (!status?.configured) return t(unavailableKey)
  if (status.has_token) return t('provider.oauth.status.authorized')
  return t('provider.oauth.status.missing')
}

async function loadOAuthStatus(): Promise<ACPCodexOAuthStatus | null> {
  if (!props.botId) return null
  codexOAuthStatusLoading.value = true
  try {
    const { data } = await client.get<{ 200: ACPCodexOAuthStatus }, unknown, true>({
      url: '/bots/{bot_id}/acp/codex/oauth/status',
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    codexOAuthStatus.value = data ?? null
    return codexOAuthStatus.value
  } catch {
    codexOAuthStatus.value = null
    return null
  } finally {
    codexOAuthStatusLoading.value = false
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

async function handleAuthorize(profile: AcpprofilePublicProfile) {
  try {
    agentForm(profile).setup_mode = 'oauth'
    authorizingCodexOAuth.value = true
    const { data } = await client.get<{ 200: ACPCodexOAuthAuthorizeResponse }, unknown, true>({
      url: '/bots/{bot_id}/acp/codex/oauth/authorize',
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    if (!data?.auth_url) throw new Error(t('provider.oauth.authorizeFailed'))
    const popup = window.open(data.auth_url, 'acp-codex-oauth', 'width=600,height=720')
    const startedAt = Date.now()
    let completed = false
    const finish = async () => {
      if (completed) return
      completed = true
      window.removeEventListener('message', listener)
      popup?.close()
      await loadOAuthStatus()
      toast.success(t('provider.oauth.authorizeSuccess'))
      authorizingCodexOAuth.value = false
    }
    const poll = () => {
      window.setTimeout(() => {
        void (async () => {
          const status = await loadOAuthStatus()
          if (status?.has_token) {
            await finish()
            return
          }
          if (Date.now() - startedAt < 120_000 && !completed) poll()
          else authorizingCodexOAuth.value = false
        })()
      }, 1_500)
    }
    const listener = (event: MessageEvent) => {
      if (event.data?.type === 'memoh-acp-codex-oauth-success' && event.data?.botId === props.botId) {
        void finish()
      }
    }
    window.addEventListener('message', listener)
    poll()
  } catch (error) {
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
