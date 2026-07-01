<template>
  <SwapTransition :direction="direction">
    <PageShell
      v-if="view === 'list'"
      variant="tab"
      :title="$t('bots.plugins.title')"
      :description="$t('bots.plugins.intro')"
    >
      <template #actions>
        <Button
          variant="outline"
          size="sm"
          class="shrink-0"
          @click="router.push({ name: 'supermarket' })"
        >
          <Store class="size-4" />
          {{ $t('sidebar.supermarket') }}
        </Button>
      </template>

      <div class="space-y-8">
        <div
          v-if="loading && !plugins.length"
          class="space-y-3"
        >
          <Skeleton
            v-for="n in 3"
            :key="n"
            class="h-[4.25rem] w-full rounded-[var(--radius-menu-shell)]"
          />
        </div>

        <div
          v-else-if="!plugins.length"
          class="rounded-[var(--radius-menu-shell)] border border-border bg-card"
        >
          <Empty
            class="py-12"
          >
            <EmptyHeader>
              <EmptyTitle>{{ $t('bots.plugins.emptyTitle') }}</EmptyTitle>
              <EmptyDescription>{{ $t('bots.plugins.emptyDescription') }}</EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <Button
                variant="outline"
                size="sm"
                @click="router.push({ name: 'supermarket' })"
              >
                <Store class="size-4" />
                {{ $t('sidebar.supermarket') }}
              </Button>
            </EmptyContent>
          </Empty>
        </div>

        <template v-else>
          <section
            v-for="group in pluginGroups"
            :key="group.key"
            class="space-y-2.5"
          >
            <h2 class="px-2 text-label font-medium text-muted-foreground">
              {{ $t(group.titleKey) }}
            </h2>

            <div class="space-y-3">
              <div
                v-for="plugin in group.items"
                :key="plugin.id"
                class="flex items-center overflow-hidden rounded-[var(--radius-menu-shell)] border border-border bg-card transition-colors hover:bg-[color:var(--ui-hover)]"
              >
                <button
                  type="button"
                  class="flex min-w-0 flex-1 items-center gap-3 p-3.5 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  :aria-label="$t('bots.plugins.detailsActionFor', { name: pluginManifest(plugin).name })"
                  @click="openPluginDetails(plugin)"
                >
                  <span class="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-md bg-accent">
                    <ProviderIcon
                      v-if="pluginIcon(plugin)"
                      :icon="pluginIcon(plugin)"
                      size="20"
                      class="size-5 object-contain"
                    >
                      <PackageOpen class="size-4 text-muted-foreground" />
                    </ProviderIcon>
                    <PackageOpen
                      v-else
                      class="size-4 text-muted-foreground"
                    />
                  </span>
                  <span class="min-w-0 flex-1">
                    <span
                      class="block truncate text-sm font-medium text-foreground"
                      :title="pluginManifest(plugin).name"
                    >
                      {{ pluginManifest(plugin).name }}
                    </span>
                  </span>
                  <Badge
                    v-if="pluginListStatus(plugin)"
                    :variant="resourceBadgeVariant(plugin.status || '')"
                    size="sm"
                    class="shrink-0"
                  >
                    {{ pluginListStatus(plugin) }}
                  </Badge>
                  <ChevronRight class="size-4 text-muted-foreground" />
                </button>

                <div class="flex shrink-0 items-center px-3.5">
                  <Switch
                    :model-value="!!plugin.enabled"
                    :disabled="!canTogglePlugin(plugin)"
                    :aria-label="toggleActionLabel(plugin)"
                    @update:model-value="enabled => togglePlugin(plugin, !!enabled)"
                  />
                </div>
              </div>
            </div>
          </section>
        </template>
      </div>
    </PageShell>

    <!-- Detail mirrors the Bot Settings Agents/Channels in-page setup pattern. -->
    <section
      v-else
      class="mx-auto max-w-3xl pt-6 pb-8"
    >
      <Button
        variant="ghost"
        class="mb-6"
        @click="closePluginDetails"
      >
        <ChevronLeft class="size-4" />
        {{ $t('bots.plugins.title') }}
      </Button>

      <div
        v-if="selectedPlugin"
        class="space-y-8"
      >
        <section class="flex items-center gap-3 rounded-[var(--radius-menu-shell)] border border-border bg-card px-4 py-3">
          <div class="flex size-9 shrink-0 items-center justify-center overflow-hidden rounded-md bg-accent">
            <ProviderIcon
              v-if="pluginIcon(selectedPlugin)"
              :icon="pluginIcon(selectedPlugin)"
              size="20"
              class="size-5 object-contain"
            >
              <PackageOpen class="size-4 text-muted-foreground" />
            </ProviderIcon>
            <PackageOpen
              v-else
              class="size-4 text-muted-foreground"
            />
          </div>
          <div class="min-w-0 flex-1">
            <h2 class="truncate text-sm font-semibold text-foreground">
              {{ pluginManifest(selectedPlugin).name }}
            </h2>
          </div>

          <div class="ml-auto flex shrink-0 items-center gap-2">
            <ConfirmPopover
              v-if="selectedPlugin.status !== 'uninstalled'"
              :message="$t('bots.plugins.uninstallConfirm')"
              :confirm-text="$t('bots.plugins.uninstallAction')"
              :loading="isPending(selectedPlugin, 'uninstall')"
              variant="destructive"
              @confirm="uninstallPlugin(selectedPlugin)"
            >
              <template #trigger>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  :disabled="isPluginPending(selectedPlugin)"
                  :aria-label="$t('bots.plugins.uninstallAction')"
                >
                  <Spinner
                    v-if="isPending(selectedPlugin, 'uninstall')"
                    class="size-4"
                  />
                  <Trash2
                    v-else
                    class="size-4"
                  />
                </Button>
              </template>
            </ConfirmPopover>
            <Switch
              :model-value="!!selectedPlugin.enabled"
              :disabled="!canTogglePlugin(selectedPlugin)"
              :aria-label="toggleActionLabel(selectedPlugin)"
              @update:model-value="enabled => selectedPlugin && togglePlugin(selectedPlugin, !!enabled)"
            />
          </div>
        </section>

        <form
          v-if="selectedPluginConfigRows.length"
          @submit.prevent
        >
          <SettingsSection :title="$t('bots.plugins.detailsConfigTitle')">
            <div>
              <SettingsRow
                v-for="item in selectedPluginConfigRows"
                :key="item.key"
                :label="item.key"
              >
                <FormItem class="w-48 sm:w-80">
                  <Select
                    v-if="item.options.length"
                    :model-value="configInputValue(item)"
                    @update:model-value="value => updateSelectConfig(item.key, value)"
                  >
                    <SelectTrigger
                      size="sm"
                      class="w-full"
                    >
                      <SelectValue :placeholder="item.placeholder || item.key" />
                    </SelectTrigger>
                    <SelectContent
                      size="sm"
                      class="w-[--reka-select-trigger-width]"
                    >
                      <SelectItem
                        v-for="option in item.options"
                        :key="option.value"
                        :value="option.value"
                      >
                        {{ option.label || option.value }}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <Input
                    v-else
                    :id="configInputId(item.key)"
                    class="w-full"
                    :name="item.key"
                    :model-value="configInputValue(item)"
                    :type="item.secret ? 'password' : 'text'"
                    autocomplete="off"
                    :placeholder="item.placeholder"
                    :aria-label="item.key"
                    @update:model-value="value => setConfigDraft(item.key, value)"
                    @blur="flushConfigSave"
                  />
                </FormItem>
              </SettingsRow>
            </div>
          </SettingsSection>
        </form>

        <SettingsSection
          v-if="selectedPluginMCPRows.length"
          :title="$t('bots.plugins.detailsMCPTitle')"
        >
          <div
            v-for="item in selectedPluginMCPRows"
            :key="item.key"
            class="mx-4 border-b border-border py-3 last:border-b-0"
          >
            <div class="flex min-w-0 flex-wrap items-center gap-2">
              <p class="min-w-0 truncate text-sm font-medium text-foreground">
                {{ item.name }}
              </p>
              <Badge
                :variant="resourceBadgeVariant(item.status)"
                size="sm"
              >
                {{ resourceStatusLabel(item.status) }}
              </Badge>
            </div>
            <p
              v-if="item.description"
              class="mt-1 text-xs text-muted-foreground"
            >
              {{ item.description }}
            </p>
            <p
              v-if="item.error"
              class="mt-2 break-words text-xs text-destructive"
            >
              {{ item.error }}
            </p>
          </div>
        </SettingsSection>

        <SettingsSection
          v-if="selectedPluginSkillRows.length"
          :title="$t('bots.plugins.detailsSkillsTitle')"
        >
          <div
            v-for="item in selectedPluginSkillRows"
            :key="item.key"
            class="mx-4 border-b border-border py-3 last:border-b-0"
          >
            <div class="min-w-0">
              <p class="truncate text-sm font-medium text-foreground">
                {{ item.name }}
              </p>
              <p
                v-if="item.description"
                class="mt-1 text-xs text-muted-foreground"
              >
                {{ item.description }}
              </p>
            </div>
          </div>
        </SettingsSection>

        <SettingsSection :title="$t('bots.plugins.detailsTitle')">
          <SettingsRow
            :label="$t('common.status')"
            :description="pluginManifest(selectedPlugin).description || $t('supermarket.noDescription')"
          >
            <Badge
              :variant="resourceBadgeVariant(selectedPlugin.status || '')"
              size="sm"
            >
              {{ statusLabel(selectedPlugin.status) }}
            </Badge>
          </SettingsRow>
          <SettingsRow
            v-if="selectedPluginVersion(selectedPlugin)"
            :label="$t('supermarket.version')"
          >
            <span class="text-sm text-muted-foreground">
              {{ selectedPluginVersion(selectedPlugin) }}
            </span>
          </SettingsRow>
          <SettingsRow
            :label="$t('bots.plugins.pluginIdLabel')"
          >
            <span
              class="max-w-80 truncate font-mono text-xs text-muted-foreground"
              :title="pluginManifest(selectedPlugin).id"
            >
              {{ pluginManifest(selectedPlugin).id }}
            </span>
          </SettingsRow>
          <SettingsRow
            v-if="pluginManifest(selectedPlugin).homepage"
            :label="$t('supermarket.homepage')"
          >
            <TooltipProvider
              :delay-duration="0"
              :disable-hoverable-content="true"
            >
              <Tooltip>
                <TooltipTrigger as-child>
                  <Button
                    as="a"
                    variant="outline"
                    size="icon-sm"
                    :href="pluginManifest(selectedPlugin).homepage"
                    target="_blank"
                    rel="noopener noreferrer"
                    :aria-label="$t('supermarket.homepage')"
                  >
                    <ExternalLink />
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="bottom">
                  {{ $t('supermarket.homepage') }}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </SettingsRow>
          <SettingsRow
            v-if="selectedPlugin.status === 'needs_auth'"
            :label="$t('bots.plugins.authLabel')"
          >
            <Button
              type="button"
              variant="outline"
              size="sm"
              :disabled="isPluginPending(selectedPlugin)"
              @click="startOAuth(selectedPlugin)"
            >
              <Spinner
                v-if="isPending(selectedPlugin, 'oauth')"
                class="size-4"
              />
              <ExternalLink
                v-else
                class="size-4"
              />
              {{ $t('bots.plugins.authorizeAction') }}
            </Button>
          </SettingsRow>
        </SettingsSection>
      </div>
    </section>
  </SwapTransition>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ChevronLeft, ChevronRight, ExternalLink, PackageOpen, Store, Trash2 } from 'lucide-vue-next'
import {
  Badge, Button, Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyTitle,
  FormItem, Input, Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
  Skeleton, Spinner, Switch, Tooltip, TooltipContent, TooltipProvider, TooltipTrigger, toast,
} from '@memohai/ui'
import {
  getBotsByBotIdPlugins,
  getBotsByBotIdPluginsByIdOauthStatus,
  deleteBotsByBotIdPluginsById,
  postBotsByBotIdPluginsByIdDisable,
  postBotsByBotIdPluginsByIdEnable,
  postBotsByBotIdPluginsByIdOauthAuthorize,
  putBotsByBotIdPluginsByIdConfig,
  type PluginsConfigVar,
  type PluginsInstallation,
  type PluginsMcpResource,
  type PluginsManifest,
  type PluginsResource,
} from '@memohai/sdk'
import { client } from '@memohai/sdk/client'
import PageShell from '@/components/page-shell/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { mcpConnectionErrorMessage, resolvePluginActionErrorMessage } from '@/utils/mcp-error-message'
import { BOT_PLUGINS_UPDATED_EVENT, emitBotPluginsUpdated, isBotPluginsUpdatedEvent } from '@/utils/bot-plugin-events'
import { isPluginInstallationNotFoundError, isPluginOAuthAuthorized, openPluginOAuthURL, waitForPluginOAuth } from '@/utils/plugin-oauth-flow'
import type { OAuthPopupFlowController } from '@/utils/oauth/popup-flow'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import ProviderIcon from '@/components/provider-icon/index.vue'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import SwapTransition from '@/components/settings/swap-transition.vue'
import { useViewSwap } from '@/composables/useViewSwap'
import { pluginConfigRows as manifestPluginConfigRows } from '@/pages/supermarket/plugin-config-rows'

type PluginGroup = {
  key: 'enabled' | 'disabled'
  titleKey: string
  items: PluginsInstallation[]
}

type PluginConfigRow = {
  key: string
  secret: boolean
  value: string
  placeholder: string
  options: Array<{ label?: string, value: string }>
}

type PluginMCPDetailRow = {
  key: string
  name: string
  description: string
  status: string
  error: string
}

type PluginSkillRow = {
  key: string
  name: string
  description: string
}

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const router = useRouter()
const plugins = ref<PluginsInstallation[]>([])
const loading = ref(false)
const pendingKey = ref('')
const selectedPlugin = ref<PluginsInstallation | null>(null)
const configSaving = ref(false)
const configDirty = ref(false)
const configSavePromise = ref<Promise<void> | null>(null)
const configDraft = reactive<Record<string, string>>({})
const locallyConfiguredKeys = reactive<Record<string, boolean>>({})
const { view, direction, openDetail, backToList } = useViewSwap()
const oauthFlows = new Map<string, OAuthPopupFlowController>()

const pluginGroups = computed<PluginGroup[]>(() => [
  {
    key: 'enabled' as const,
    titleKey: 'bots.plugins.enabledTitle',
    items: plugins.value.filter(plugin => !!plugin.enabled),
  },
  {
    key: 'disabled' as const,
    titleKey: 'bots.plugins.disabledTitle',
    items: plugins.value.filter(plugin => !plugin.enabled),
  },
].filter(group => group.items.length > 0))

const selectedPluginConfigRows = computed(() => selectedPlugin.value ? pluginConfigRows(selectedPlugin.value) : [])
const selectedPluginMCPRows = computed(() => selectedPlugin.value ? pluginMCPDetailRows(selectedPlugin.value) : [])
const selectedPluginSkillRows = computed(() => selectedPlugin.value ? pluginSkillRows(selectedPlugin.value) : [])

onMounted(() => {
  void loadPlugins()
  window.addEventListener(BOT_PLUGINS_UPDATED_EVENT, handleBotPluginsUpdated)
})

onUnmounted(() => {
  cancelAllPluginOAuth()
  void flushConfigSave()
  window.removeEventListener(BOT_PLUGINS_UPDATED_EVENT, handleBotPluginsUpdated)
})

watch(() => props.botId, async () => {
  cancelAllPluginOAuth()
  await flushConfigSave()
  backToList()
  resetConfigDraft()
  void loadPlugins()
})

watch(() => selectedPlugin.value?.id, async () => {
  await flushConfigSave()
  resetConfigDraft()
})

function handleBotPluginsUpdated(event: Event) {
  if (!isBotPluginsUpdatedEvent(event)) return
  if (event.detail.botId !== props.botId) return
  void loadPlugins()
}

async function loadPlugins() {
  if (!props.botId) return
  loading.value = true
  try {
    const { data } = await getBotsByBotIdPlugins({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    plugins.value = (data.items ?? []).filter(item => item.status !== 'uninstalled')
    syncSelectedPlugin()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.plugins.loadFailed')))
  } finally {
    loading.value = false
  }
}

function pluginManifest(plugin: PluginsInstallation): PluginsManifest {
  return {
    ...(plugin.manifest ?? {}),
    id: plugin.plugin_id || plugin.manifest?.id || plugin.id,
    name: plugin.plugin_name || plugin.manifest?.name || plugin.plugin_id || plugin.id,
    description: plugin.manifest?.description || '',
  }
}

function pluginIcon(plugin: PluginsInstallation): string {
  const icon = pluginManifest(plugin).icon
  if (!icon) return ''
  if (icon.kind === 'external_url') return icon.url || ''
  if (icon.kind === 'builtin') return icon.name || ''
  return ''
}

function openPluginDetails(plugin: PluginsInstallation) {
  resetConfigDraft()
  selectedPlugin.value = plugin
  openDetail()
}

async function closePluginDetails() {
  await flushConfigSave()
  backToList()
}

function syncSelectedPlugin() {
  const current = selectedPlugin.value
  if (!current) return
  const updated = plugins.value.find(item => item.id === current.id)
  if (!updated) {
    selectedPlugin.value = null
    resetConfigDraft()
    backToList()
    return
  }
  selectedPlugin.value = updated
}

function resetConfigDraft() {
  for (const key of Object.keys(configDraft)) delete configDraft[key]
  if (!configSaving.value) configDirty.value = false
}

function configInputId(key: string): string {
  return `plugin-config-${selectedPlugin.value?.id || 'plugin'}-${key}`
}

function setConfigDraft(key: string, value: unknown) {
  configDraft[key] = value === undefined || value === null ? '' : String(value)
  configDirty.value = true
}

function updateSelectConfig(key: string, value: unknown) {
  setConfigDraft(key, value)
  void flushConfigSave()
}

function configInputValue(item: PluginConfigRow): string {
  return Object.hasOwn(configDraft, item.key) ? (configDraft[item.key] ?? '') : item.value
}

function configPayload(): Record<string, string> {
  const out: Record<string, string> = {}
  const rowByKey = new Map(selectedPluginConfigRows.value.map(row => [row.key, row]))
  for (const [key, value] of Object.entries(configDraft)) {
    const trimmedKey = key.trim()
    if (!trimmedKey) continue
    const row = rowByKey.get(trimmedKey)
    out[trimmedKey] = row?.options.length ? value : value.trim()
  }
  return out
}

function clearSavedDraft(saved: Record<string, string>) {
  const currentPayload = configPayload()
  for (const [key, value] of Object.entries(saved)) {
    if (currentPayload[key] === value) delete configDraft[key]
  }
  configDirty.value = Object.keys(configDraft).length > 0
}

function rememberSavedConfig(pluginId: string, saved: Record<string, string>) {
  for (const [key, value] of Object.entries(saved)) {
    const stateKey = pluginConfigStateKey(pluginId, key)
    if (value.trim()) {
      locallyConfiguredKeys[stateKey] = true
    } else {
      delete locallyConfiguredKeys[stateKey]
    }
  }
}

function flushConfigSave(): Promise<void> {
  if (configSaving.value) return configSavePromise.value ?? Promise.resolve()
  if (!configDirty.value) return Promise.resolve()
  return savePluginConfig(props.botId, selectedPlugin.value?.id || '', configPayload())
}

function statusLabel(status?: string): string {
  switch (status) {
    case 'ready': return t('bots.plugins.status.ready')
    case 'needs_config': return t('bots.plugins.status.needsConfig')
    case 'needs_auth': return t('bots.plugins.status.needsAuth')
    case 'admin_required': return t('bots.plugins.status.adminRequired')
    case 'disabled': return t('bots.plugins.status.disabled')
    case 'uninstalled': return t('bots.plugins.status.uninstalled')
    default: return status || t('mcp.statusUnknown')
  }
}

function pluginListStatus(plugin: PluginsInstallation): string {
  switch (plugin.status) {
    case 'needs_config':
    case 'needs_auth':
    case 'admin_required':
      return statusLabel(plugin.status)
    default:
      return ''
  }
}

function pluginConfigRows(plugin: PluginsInstallation): PluginConfigRow[] {
  const manifest = pluginManifest(plugin)
  const configured = configuredVariables(plugin)
  const rows: PluginConfigRow[] = []
  const rowByKey = new Map<string, PluginConfigRow>()

  const addRow = (item: PluginsConfigVar) => {
    const key = (item.key || '').trim()
    if (!key) return
    const existing = rowByKey.get(key)
    if (existing) {
      existing.secret ||= !!item.secret
      if (!existing.value) existing.value = variableInputValue(item, configured)
      if (!existing.placeholder) existing.placeholder = variablePlaceholder(plugin, item, configured)
      if (!existing.options.length) existing.options = configOptions(item)
      return
    }
    const row = {
      key,
      secret: !!item.secret,
      value: variableInputValue(item, configured),
      placeholder: variablePlaceholder(plugin, item, configured),
      options: configOptions(item),
    }
    rows.push(row)
    rowByKey.set(key, row)
  }

  for (const item of manifestPluginConfigRows(manifest)) addRow(item)

  return rows
}

function configuredVariables(plugin: PluginsInstallation): Record<string, { configured?: boolean; value?: unknown }> {
  const raw = asRecord(plugin.config?.variables)
  const out: Record<string, { configured?: boolean; value?: unknown }> = {}
  for (const [key, value] of Object.entries(raw)) {
    if (isRecord(value)) {
      out[key] = {
        configured: typeof value.configured === 'boolean' ? value.configured : undefined,
        value: value.value,
      }
      continue
    }
    out[key] = { value, configured: value !== undefined && value !== null && String(value).trim() !== '' }
  }
  return out
}

function variableInputValue(item: PluginsConfigVar, configured: Record<string, { configured?: boolean; value?: unknown }>): string {
  if (item.secret) return ''
  const state = configured[item.key || '']
  if (state?.value !== undefined && state.value !== null && String(state.value).trim() !== '') {
    return String(state.value)
  }
  const defaultValue = String(item.defaultValue || '').trim()
  return templateVariableKeys(defaultValue).length ? '' : defaultValue
}

function variablePlaceholder(plugin: PluginsInstallation, item: PluginsConfigVar, configured: Record<string, { configured?: boolean; value?: unknown }>): string {
  const key = item.key || ''
  const state = configured[key]
  const isConfigured = locallyConfiguredKeys[pluginConfigStateKey(plugin.id || '', key)]
    || state?.configured
    || (state?.value !== undefined && state.value !== null && String(state.value).trim() !== '')
  if (item.secret) {
    return isConfigured ? t('bots.plugins.configuredValue') : ''
  }
  return ''
}

function configOptions(item: PluginsConfigVar): Array<{ label?: string, value: string }> {
  return (item.options ?? [])
    .flatMap((option) => {
      const value = option.value
      if (!value) return []
      return [{ label: option.label, value }]
    })
}

function pluginConfigStateKey(pluginId: string, key: string): string {
  return `${pluginId}:${key}`
}

function templateVariableKeys(value: string): string[] {
  return [...value.matchAll(/\$\{([A-Za-z_][A-Za-z0-9_]*)\}/g)]
    .map(match => match[1])
    .filter((key): key is string => !!key)
}

function pluginMCPDetailRows(plugin: PluginsInstallation): PluginMCPDetailRow[] {
  const manifest = pluginManifest(plugin)
  const resources = plugin.resources ?? []
  const resourceByKey = new Map(resources
    .filter(resource => resource.resource_type === 'mcp')
    .map(resource => [resource.resource_key || '', resource]))

  const rows = (manifest.mcps ?? []).map((mcp) => {
    const resource = resourceByKey.get(mcp.key || '')
    return pluginMCPDetailRow(plugin, mcp, resource)
  })

  for (const resource of resources) {
    if (resource.resource_type !== 'mcp') continue
    if (rows.some(row => row.key === resource.resource_key)) continue
    rows.push(pluginMCPDetailRow(plugin, undefined, resource))
  }
  return rows
}

function pluginMCPDetailRow(plugin: PluginsInstallation, mcp?: PluginsMcpResource, resource?: PluginsResource): PluginMCPDetailRow {
  const metadata = asRecord(resource?.metadata)
  const key = mcp?.key || resource?.resource_key || resource?.resource_id || 'mcp'
  let status = plugin.status || 'disabled'
  if (plugin.enabled && plugin.status === 'ready') {
    status = stringFrom(metadata.probe_status) || resource?.status || 'ready'
  } else if (!plugin.enabled && (plugin.status === 'ready' || plugin.status === 'disabled')) {
    status = 'disabled'
  }
  return {
    key,
    name: mcp?.display_name || mcp?.name || stringFrom(metadata.display_name) || key,
    description: mcp?.description || '',
    status,
    error: status === 'error' ? resourceErrorLabel(stringFrom(metadata.probe_error)) : '',
  }
}

function resourceErrorLabel(raw: string): string {
  return mcpConnectionErrorMessage(raw, t)
}

function selectedPluginVersion(plugin: PluginsInstallation): string {
  const version = String(plugin.version || pluginManifest(plugin).version || '').trim()
  return version ? t('bots.plugins.versionValue', { version }) : ''
}

function resourceStatusLabel(status: string): string {
  switch (status) {
    case 'connected':
      return t('mcp.statusConnected')
    case 'error':
      return t('mcp.statusError')
    case 'ready':
      return t('bots.plugins.status.ready')
    case 'needs_auth':
      return t('bots.plugins.status.needsAuth')
    case 'needs_config':
      return t('bots.plugins.status.needsConfig')
    case 'admin_required':
      return t('bots.plugins.status.adminRequired')
    case 'disabled':
      return t('bots.plugins.status.disabled')
    case 'uninstalled':
      return t('bots.plugins.status.uninstalled')
    default:
      return status || t('mcp.statusUnknown')
  }
}

function resourceBadgeVariant(status: string): 'default' | 'secondary' | 'destructive' | 'success' | 'warning' | 'info' | 'outline' {
  switch (status) {
    case 'connected':
    case 'ready':
      return 'success'
    case 'error':
      return 'destructive'
    case 'needs_auth':
    case 'needs_config':
    case 'admin_required':
      return 'warning'
    default:
      return 'secondary'
  }
}

function pluginSkillRows(plugin: PluginsInstallation): PluginSkillRow[] {
  const manifest = pluginManifest(plugin)
  const rows: PluginSkillRow[] = []
  for (const skill of manifest.skills ?? []) {
    rows.push({
      key: `skill:${skill.key || skill.path || skill.name}`,
      name: skill.name || skill.key || skill.path || t('bots.plugins.unnamedSkill'),
      description: skill.path || '',
    })
  }
  for (const skill of manifest.bundled_skills ?? []) {
    rows.push({
      key: `bundled:${skill.id || skill.name}`,
      name: skill.name || skill.id || t('bots.plugins.unnamedSkill'),
      description: skill.description || '',
    })
  }
  return rows
}

function asRecord(value: unknown): Record<string, unknown> {
  return isRecord(value) ? value : {}
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function stringFrom(value: unknown): string {
  if (value === undefined || value === null) return ''
  return String(value).trim()
}

function isPending(plugin: PluginsInstallation, action: string): boolean {
  return pendingKey.value === `${plugin.id}:${action}`
}

function isPluginPending(plugin: PluginsInstallation): boolean {
  return !!plugin.id && pendingKey.value.startsWith(`${plugin.id}:`)
}

function canTogglePlugin(plugin: PluginsInstallation): boolean {
  if (isPluginPending(plugin)) return false
  if (configSaving.value) return false
  if (plugin.status === 'uninstalled') return false
  if (plugin.enabled) return true
  return plugin.status === 'ready' || plugin.status === 'disabled'
}

function toggleActionLabel(plugin: PluginsInstallation): string {
  return plugin.enabled ? t('bots.plugins.disableAction') : t('bots.plugins.enableAction')
}

function togglePlugin(plugin: PluginsInstallation, enabled: boolean) {
  if (enabled === !!plugin.enabled) return
  if (enabled) {
    void enablePlugin(plugin)
  } else {
    void disablePlugin(plugin)
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

async function withPending(plugin: PluginsInstallation, action: string, task: () => Promise<void>) {
  if (!plugin.id) return
  const key = `${plugin.id}:${action}`
  pendingKey.value = key
  try {
    await task()
  } finally {
    if (pendingKey.value === key) pendingKey.value = ''
  }
}

async function enablePlugin(plugin: PluginsInstallation) {
  await withPending(plugin, 'enable', async () => {
    try {
      await flushConfigSave()
      await postBotsByBotIdPluginsByIdEnable({
        path: { bot_id: props.botId, id: plugin.id! },
        throwOnError: true,
      })
      toast.success(t('bots.plugins.enableSuccess'))
      await loadPlugins()
    } catch (error) {
      toast.error(resolvePluginActionErrorMessage(error, t('bots.plugins.enableFailed'), t))
      await loadPlugins()
    }
  })
}

async function disablePlugin(plugin: PluginsInstallation) {
  await withPending(plugin, 'disable', async () => {
    try {
      await flushConfigSave()
      await postBotsByBotIdPluginsByIdDisable({
        path: { bot_id: props.botId, id: plugin.id! },
        throwOnError: true,
      })
      toast.success(t('bots.plugins.disableSuccess'))
      await loadPlugins()
    } catch (error) {
      toast.error(resolvePluginActionErrorMessage(error, t('bots.plugins.disableFailed'), t))
      await loadPlugins()
    }
  })
}

async function uninstallPlugin(plugin: PluginsInstallation) {
  await flushConfigSave()
  await withPending(plugin, 'uninstall', async () => {
    try {
      if (plugin.id) cancelPluginOAuth(plugin.id)
      await deleteBotsByBotIdPluginsById({
        path: { bot_id: props.botId, id: plugin.id! },
        throwOnError: true,
      })
      emitBotPluginsUpdated(props.botId)
      toast.success(t('bots.plugins.uninstallSuccess'))
      await loadPlugins()
      backToList()
    } catch (error) {
      toast.error(resolvePluginActionErrorMessage(error, t('bots.plugins.uninstallFailed'), t))
      await loadPlugins()
    }
  })
}

async function savePluginConfig(botId = props.botId, pluginId = selectedPlugin.value?.id || '', variables = configPayload()) {
  if (!botId || !pluginId) return
  if (configSaving.value) {
    await (configSavePromise.value ?? Promise.resolve())
    if (configDirty.value) {
      await savePluginConfig(botId, pluginId, configPayload())
    }
    return
  }
  if (!Object.keys(variables).length) return
  configSaving.value = true
  pendingKey.value = `${pluginId}:config`
  const run = (async () => {
    let nextVariables = variables
    while (Object.keys(nextVariables).length) {
      const { data } = await putBotsByBotIdPluginsByIdConfig({
        path: { bot_id: botId, id: pluginId },
        body: { variables: nextVariables },
        throwOnError: true,
      })
      rememberSavedConfig(pluginId, nextVariables)
      clearSavedDraft(nextVariables)
      plugins.value = plugins.value.map(item => item.id === data.id ? data : item)
      if (selectedPlugin.value?.id === pluginId) {
        selectedPlugin.value = data
      }
      await loadPlugins()
      if (!configDirty.value) break
      nextVariables = configPayload()
    }
  })()
  configSavePromise.value = run
  try {
    await run
  } catch (error) {
    toast.error(resolvePluginActionErrorMessage(error, t('bots.plugins.configSaveFailed'), t))
    await loadPlugins()
  } finally {
    if (pendingKey.value === `${pluginId}:config`) pendingKey.value = ''
    configSaving.value = false
    if (configSavePromise.value === run) configSavePromise.value = null
  }
}

async function startOAuth(plugin: PluginsInstallation) {
  await withPending(plugin, 'oauth', async () => {
    const pluginId = plugin.id
    if (!pluginId) return
    cancelAllPluginOAuth(false)
    const popup = !window.api?.desktop?.openExternalUrl
      ? window.open('', 'mcp-oauth', 'width=600,height=700')
      : null
    if (!window.api?.desktop?.openExternalUrl && !popup) {
      toast.error(t('mcp.oauth.flowInitFailed'))
      return
    }
    try {
      const { data } = await postBotsByBotIdPluginsByIdOauthAuthorize({
        path: { bot_id: props.botId, id: plugin.id! },
        body: { callback_url: mcpOAuthCallbackUrl() },
        throwOnError: true,
      })
      if (!data.authorization_url) throw new Error(t('mcp.oauth.flowInitFailed'))

      const opened = await openPluginOAuthURL(data.authorization_url, popup)
      const waitResult = await waitForPluginOAuth({
        botId: props.botId,
        installationId: pluginId,
        popup: opened.popup,
        external: opened.external,
        fetchStatus: fetchOAuthStatus,
        t,
        onController: flow => {
          cancelAllPluginOAuth(false)
          oauthFlows.set(pluginId, flow)
        },
        onCleanup: () => oauthFlows.delete(pluginId),
      })
      if (waitResult === 'timeout') {
        throw new Error(t('mcp.oauth.authFailed'))
      }
      if (waitResult === 'needs_config' || waitResult === 'admin_required') {
        throw new Error(`plugin is not ready: ${waitResult}`)
      }
      if (waitResult !== 'authorized') {
        await loadPlugins()
        return
      }
      const synced = await syncOAuthStatus(plugin)
      if (synced.status === 'needs_config' || synced.status === 'admin_required') {
        throw new Error(`plugin is not ready: ${synced.status}`)
      }
      if (!isPluginOAuthAuthorized(synced)) throw new Error(t('mcp.oauth.authFailed'))
      toast.success(t('mcp.oauth.authSuccess'))
      await loadPlugins()
    } catch (error) {
      popup?.close()
      if (isPluginInstallationNotFoundError(error)) {
        await loadPlugins()
        return
      }
      let synced: PluginsInstallation | null = null
      try {
        synced = await syncOAuthStatus(plugin)
      } catch (syncError) {
        if (isPluginInstallationNotFoundError(syncError)) {
          await loadPlugins()
          return
        }
      }
      if (isPluginOAuthAuthorized(synced)) {
        toast.success(t('mcp.oauth.authSuccess'))
        await loadPlugins()
        return
      }
      await loadPlugins()
      toast.error(resolvePluginActionErrorMessage(error, t('mcp.oauth.flowInitFailed'), t))
    }
  })
}

function cancelPluginOAuth(pluginId: string, notify = true) {
  const flow = oauthFlows.get(pluginId)
  if (!flow) return
  oauthFlows.delete(pluginId)
  if (notify) {
    flow.cancel()
    return
  }
  flow.dispose()
}

function cancelAllPluginOAuth(notify = true) {
  for (const pluginId of oauthFlows.keys()) cancelPluginOAuth(pluginId, notify)
}

async function syncOAuthStatus(plugin: PluginsInstallation): Promise<PluginsInstallation> {
  if (!plugin.id) throw new Error(t('mcp.oauth.authFailed'))
  return fetchOAuthStatus(props.botId, plugin.id)
}

async function fetchOAuthStatus(botId: string, installationId: string): Promise<PluginsInstallation> {
  const { data } = await getBotsByBotIdPluginsByIdOauthStatus({
    path: { bot_id: botId, id: installationId },
    throwOnError: true,
  })
  plugins.value = plugins.value.map(item => item.id === data.id ? data : item)
  return data
}
</script>
