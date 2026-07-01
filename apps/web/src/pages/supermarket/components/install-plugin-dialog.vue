<template>
  <Dialog
    :open="open"
    @update:open="$emit('update:open', $event)"
  >
    <DialogContent class="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>{{ $t('supermarket.pluginInstallTitle') }}</DialogTitle>
      </DialogHeader>
      <div class="space-y-4 py-2">
        <FieldStack :label="$t('supermarket.selectBot')">
          <BotSelect
            v-model="selectedBotId"
            trigger-class="w-full"
          />
        </FieldStack>

        <div
          v-if="plugin"
          class="rounded-md border border-border p-3 space-y-1"
        >
          <div class="flex min-w-0 flex-wrap items-center gap-2">
            <p
              class="min-w-0 truncate text-xs font-medium"
              :title="plugin.name"
            >
              {{ plugin.name }}
            </p>
            <Badge
              v-if="plugin.mcps?.length"
              variant="outline"
              size="sm"
            >
              {{ plugin.mcps.length }} MCPs
            </Badge>
            <Badge
              v-if="pluginSkills.length"
              variant="outline"
              size="sm"
            >
              {{ pluginSkills.length }} Skills
            </Badge>
          </div>
          <p class="text-caption text-muted-foreground line-clamp-3">
            {{ plugin.description }}
          </p>
          <div
            v-if="pluginSkills.length"
            class="mt-3 grid gap-1.5"
          >
            <div
              v-for="skill in pluginSkills"
              :key="skillKey(skill)"
              class="flex min-w-0 items-start gap-2 rounded border border-border/60 bg-muted/20 px-2 py-1.5"
            >
              <Boxes class="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />
              <div class="min-w-0 flex-1">
                <p
                  class="truncate text-caption font-medium"
                  :title="skillName(skill)"
                >
                  {{ skillName(skill) }}
                </p>
                <p
                  class="truncate text-[10px] text-muted-foreground"
                  :title="skillDescription(skill)"
                >
                  {{ skillDescription(skill) }}
                </p>
              </div>
            </div>
          </div>
        </div>

        <div
          v-if="variables.length"
          class="space-y-3"
        >
          <FieldStack
            v-for="item in variables"
            :key="item.key"
          >
            <template #label>
              <Label>
                {{ item.key }}
                <span
                  v-if="item.required"
                  class="text-destructive"
                >*</span>
              </Label>
            </template>
            <Select
              v-if="configOptions(item).length"
              v-model="variableValues[item.key || '']"
            >
              <SelectTrigger
                size="sm"
                class="w-full"
              >
                <SelectValue :placeholder="item.description || item.key" />
              </SelectTrigger>
              <SelectContent
                size="sm"
                class="w-[--reka-select-trigger-width]"
              >
                <SelectItem
                  v-for="option in configOptions(item)"
                  :key="option.value"
                  :value="option.value"
                >
                  {{ option.label || option.value }}
                </SelectItem>
              </SelectContent>
            </Select>
            <Input
              v-else
              v-model="variableValues[item.key || '']"
              :type="item.secret ? 'password' : 'text'"
              class="h-8 text-xs"
              :placeholder="item.description || item.key"
            />
          </FieldStack>
        </div>
      </div>
      <DialogFooter>
        <DialogClose as-child>
          <Button
            variant="outline"
            :disabled="installing"
          >
            {{ $t('common.cancel') }}
          </Button>
        </DialogClose>
        <Button
          :disabled="!canInstall"
          :loading="installing"
          @click="handleInstall"
        >
          {{ installing ? $t('supermarket.installing') : $t('supermarket.install') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Boxes } from 'lucide-vue-next'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogClose,
  Button, Badge, Input, Label, Select, SelectContent, SelectItem, SelectTrigger, SelectValue, toast,
} from '@memohai/ui'
import {
  getBotsByBotIdPluginsByIdOauthStatus,
  postBotsByBotIdPluginsByIdOauthAuthorize,
  postBotsByBotIdSupermarketInstallPlugin,
  type PluginsConfigVar,
  type PluginsInstallation,
  type PluginsManifest,
  type PluginsSkillEntry,
  type PluginsSkillResource,
} from '@memohai/sdk'
import { client } from '@memohai/sdk/client'
import { emitBotPluginsUpdated } from '@/utils/bot-plugin-events'
import { resolvePluginActionErrorMessage } from '@/utils/mcp-error-message'
import { isPluginInstallationNotFoundError, isPluginOAuthAuthorized, openPluginOAuthURL, waitForPluginOAuth } from '@/utils/plugin-oauth-flow'
import BotSelect from '@/components/bot-select/index.vue'
import FieldStack from '@/components/settings/field-stack.vue'
import { pluginConfigRows, templateVariableKeys } from '../plugin-config-rows'

const props = defineProps<{
  open: boolean
  plugin: PluginsManifest | null
}>()

const emit = defineEmits<{
  'update:open': [open: boolean]
  'installed': []
}>()

const { t } = useI18n()
const router = useRouter()

const selectedBotId = ref('')
const installing = ref(false)
const variableValues = reactive<Record<string, string>>({})

const variables = computed<PluginsConfigVar[]>(() => props.plugin ? pluginConfigRows(props.plugin) : [])
type PluginSkill = PluginsSkillEntry | PluginsSkillResource

const pluginSkills = computed<PluginSkill[]>(() => [
  ...(props.plugin?.bundled_skills ?? []),
  ...(props.plugin?.skills ?? []),
])

const requiresManagedOAuth = computed(() => {
  return (props.plugin?.auth_requirements ?? []).some(item => item.type === 'managed_oauth')
})
const canInstall = computed(() => {
  if (!selectedBotId.value || !props.plugin?.id) return false
  return variables.value.every(item => !item.required || !!variableValues[item.key || '']?.trim())
})

function skillKey(skill: PluginSkill): string {
  if ('id' in skill && skill.id) return skill.id
  if ('key' in skill && skill.key) return skill.key
  return skillName(skill)
}

function skillName(skill: PluginSkill): string {
  if ('name' in skill && skill.name) return skill.name
  if ('id' in skill && skill.id) return skill.id
  if ('key' in skill && skill.key) return skill.key
  return t('supermarket.unnamedSkill')
}

function skillDescription(skill: PluginSkill): string {
  if ('description' in skill && skill.description) return skill.description
  if ('path' in skill && skill.path) return skill.path
  return t('supermarket.noDescription')
}

watch(() => props.open, (open) => {
  if (!open) {
    selectedBotId.value = ''
    installing.value = false
    for (const key of Object.keys(variableValues)) delete variableValues[key]
    return
  }
  for (const item of variables.value) {
    if (item.key) variableValues[item.key] = initialVariableValue(item)
  }
})

watch(variables, (items) => {
  for (const key of Object.keys(variableValues)) {
    if (!items.some(item => item.key === key)) delete variableValues[key]
  }
  for (const item of items) {
    const key = (item.key || '').trim()
    if (!key || Object.hasOwn(variableValues, key)) continue
    variableValues[key] = initialVariableValue(item)
  }
})

function initialVariableValue(item: PluginsConfigVar): string {
  const value = String(item.defaultValue || '').trim()
  return templateVariableKeys(value).length ? '' : value
}

function configOptions(item: PluginsConfigVar): Array<{ label?: string, value: string }> {
  return (item.options ?? [])
    .flatMap((option) => {
      const value = option.value
      if (!value) return []
      return [{ label: option.label, value }]
    })
}

async function handleInstall() {
  if (!selectedBotId.value || !props.plugin?.id) return
  const botId = selectedBotId.value
  const oauthPopup = requiresManagedOAuth.value && !canOpenOAuthExternally()
    ? window.open('', 'mcp-oauth', 'width=600,height=700')
    : null
  if (requiresManagedOAuth.value && !canOpenOAuthExternally() && !oauthPopup) {
    toast.error(t('mcp.oauth.flowInitFailed'))
    return
  }
  installing.value = true
  try {
    const { data } = await postBotsByBotIdSupermarketInstallPlugin({
      path: { bot_id: botId },
      body: {
        plugin_id: props.plugin.id,
        variables: Object.fromEntries(
          Object.entries(variableValues).filter(([, value]) => value.trim() !== ''),
        ),
      },
      throwOnError: true,
    })
    toast.success(t('supermarket.installSuccess'))
    emitBotPluginsUpdated(botId)
    emit('update:open', false)
    emit('installed')
    void router.push({
      name: 'bot-detail',
      params: { botName: botId },
      query: { tab: 'plugins' },
    }).catch(() => {})
    if (data.status === 'needs_auth' && data.id) {
      await startOAuthAfterInstall(botId, data, oauthPopup)
    } else {
      oauthPopup?.close()
    }
  } catch (error) {
    oauthPopup?.close()
    emitBotPluginsUpdated(botId)
    toast.error(resolvePluginActionErrorMessage(error, t('supermarket.installFailed'), t))
  } finally {
    installing.value = false
  }
}

function mcpOAuthCallbackUrl() {
  const rawBase = String(client.getConfig().baseUrl || '/api')
  const base = new URL(rawBase, window.location.origin)
  const basePath = base.pathname.replace(/\/+$/, '')
  base.pathname = `${basePath}/oauth/mcp/callback`
  base.search = ''
  base.hash = ''
  return base.toString()
}

async function startOAuthAfterInstall(botId: string, installation: PluginsInstallation, popup: Window | null) {
  try {
    const { data } = await postBotsByBotIdPluginsByIdOauthAuthorize({
      path: { bot_id: botId, id: installation.id! },
      body: { callback_url: mcpOAuthCallbackUrl() },
      throwOnError: true,
    })
    if (!data.authorization_url) throw new Error(t('mcp.oauth.flowInitFailed'))

    const opened = await openPluginOAuthURL(data.authorization_url, popup)
    popup = opened.popup
    const waitResult = await waitForPluginOAuth({
      botId,
      installationId: installation.id!,
      popup: opened.popup,
      external: opened.external,
      fetchStatus: fetchOAuthStatus,
      t,
    })
    if (waitResult === 'timeout') {
      throw new Error(t('mcp.oauth.authFailed'))
    }
    if (waitResult === 'needs_config' || waitResult === 'admin_required') {
      throw new Error(`plugin is not ready: ${waitResult}`)
    }
    if (waitResult !== 'authorized') {
      emitBotPluginsUpdated(botId)
      popup?.close()
      return
    }
    const synced = await syncOAuthStatus(botId, installation.id!)
    if (synced.status === 'needs_config' || synced.status === 'admin_required') {
      throw new Error(`plugin is not ready: ${synced.status}`)
    }
    if (!isPluginOAuthAuthorized(synced)) throw new Error(t('mcp.oauth.authFailed'))
    emitBotPluginsUpdated(botId)
    toast.success(t('mcp.oauth.authSuccess'))
  } catch (error) {
    if (isPluginInstallationNotFoundError(error)) {
      emitBotPluginsUpdated(botId)
      popup?.close()
      return
    }
    let synced: PluginsInstallation | null = null
    try {
      synced = await syncOAuthStatus(botId, installation.id!)
    } catch (syncError) {
      if (isPluginInstallationNotFoundError(syncError)) {
        emitBotPluginsUpdated(botId)
        popup?.close()
        return
      }
    }
    if (synced) emitBotPluginsUpdated(botId)
    if (isPluginOAuthAuthorized(synced)) {
      toast.success(t('mcp.oauth.authSuccess'))
      return
    }
    emitBotPluginsUpdated(botId)
    popup?.close()
    toast.error(resolvePluginActionErrorMessage(error, t('mcp.oauth.flowInitFailed'), t))
  }
}

function canOpenOAuthExternally(): boolean {
  return Boolean(window.api?.desktop?.openExternalUrl)
}

async function fetchOAuthStatus(botId: string, installationId: string): Promise<PluginsInstallation> {
  const { data } = await getBotsByBotIdPluginsByIdOauthStatus({
    path: { bot_id: botId, id: installationId },
    throwOnError: true,
  })
  return data
}

async function syncOAuthStatus(botId: string, installationId: string): Promise<PluginsInstallation> {
  return fetchOAuthStatus(botId, installationId)
}
</script>
