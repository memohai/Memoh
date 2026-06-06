<template>
  <div class="max-w-3xl mx-auto pb-6 space-y-5">
    <header class="pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-30 pt-4 -mt-4 flex items-center justify-between">
      <div class="space-y-1">
        <h2 class="text-sm font-semibold text-foreground">
          {{ $t('bots.plugins.title') }}
        </h2>
        <p class="text-[11px] leading-snug text-muted-foreground max-w-md">
          {{ $t('bots.plugins.intro') }}
        </p>
      </div>
      <Button
        variant="outline"
        size="sm"
        class="h-8 text-xs"
        @click="router.push({ name: 'supermarket' })"
      >
        <Store class="size-3.5" />
        {{ $t('sidebar.supermarket') }}
      </Button>
    </header>

    <div
      v-if="loading && !plugins.length"
      class="flex items-center justify-center py-12 text-xs text-muted-foreground"
    >
      <Spinner class="mr-2 size-4" />
      {{ $t('common.loading') }}
    </div>

    <div
      v-else-if="!plugins.length"
      class="flex flex-col items-center justify-center py-16 text-center border border-dashed border-border/50 bg-muted/5 rounded-lg"
    >
      <div class="rounded-md bg-background border border-border/50 p-2.5 mb-4 shadow-sm">
        <PackageOpen class="size-5 text-muted-foreground" />
      </div>
      <h3 class="text-sm font-medium text-foreground">
        {{ $t('bots.plugins.emptyTitle') }}
      </h3>
      <p class="text-[11px] text-muted-foreground mt-1.5 max-w-sm">
        {{ $t('bots.plugins.emptyDescription') }}
      </p>
    </div>

    <div
      v-else
      class="space-y-3"
    >
      <section
        v-for="plugin in plugins"
        :key="plugin.id"
        class="rounded-md border border-border/60 bg-background p-4 shadow-sm"
      >
        <div class="flex items-start gap-3">
          <div class="size-10 shrink-0 rounded-md border border-border bg-muted/20 flex items-center justify-center overflow-hidden">
            <ProviderIcon
              v-if="pluginIcon(plugin)"
              :icon="pluginIcon(plugin)!"
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
          <div class="min-w-0 flex-1 space-y-2">
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="flex items-center gap-2 min-w-0">
                  <h3
                    class="text-sm font-semibold text-foreground truncate"
                    :title="plugin.plugin_name || plugin.manifest?.name"
                  >
                    {{ plugin.plugin_name || plugin.manifest?.name || plugin.plugin_id }}
                  </h3>
                  <Badge
                    variant="outline"
                    size="sm"
                    :class="statusClass(plugin.status)"
                  >
                    {{ statusLabel(plugin.status) }}
                  </Badge>
                </div>
                <p class="text-[11px] text-muted-foreground line-clamp-2 mt-1">
                  {{ plugin.manifest?.description || '-' }}
                </p>
              </div>
              <div class="flex items-center gap-1 shrink-0">
                <Button
                  v-if="plugin.status === 'needs_auth'"
                  variant="outline"
                  size="sm"
                  class="h-7 text-xs"
                  :disabled="isPending(plugin, 'oauth')"
                  @click="startOAuth(plugin)"
                >
                  <ExternalLink class="size-3.5" />
                  {{ $t('bots.plugins.authorizeAction') }}
                </Button>
                <Button
                  v-if="plugin.status === 'needs_auth'"
                  variant="ghost"
                  size="icon-sm"
                  class="size-7 text-muted-foreground"
                  :title="$t('bots.plugins.refreshAuthAction')"
                  :disabled="isPending(plugin, 'status')"
                  @click="refreshOAuth(plugin)"
                >
                  <RefreshCw
                    class="size-3.5"
                    :class="{ 'animate-spin': isPending(plugin, 'status') }"
                  />
                </Button>
                <Button
                  v-if="plugin.enabled"
                  variant="ghost"
                  size="icon-sm"
                  class="size-7 text-muted-foreground"
                  :title="$t('bots.plugins.disableAction')"
                  :disabled="isPending(plugin, 'disable')"
                  @click="disablePlugin(plugin)"
                >
                  <PowerOff class="size-3.5" />
                </Button>
                <Button
                  v-else-if="plugin.status === 'ready' || plugin.status === 'disabled'"
                  variant="ghost"
                  size="icon-sm"
                  class="size-7 text-muted-foreground"
                  :title="$t('bots.plugins.enableAction')"
                  :disabled="isPending(plugin, 'enable')"
                  @click="enablePlugin(plugin)"
                >
                  <Power class="size-3.5" />
                </Button>
                <ConfirmPopover
                  :message="$t('bots.plugins.uninstallConfirm')"
                  @confirm="uninstallPlugin(plugin)"
                >
                  <template #trigger>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      class="size-7 text-muted-foreground hover:text-destructive"
                      :title="$t('bots.plugins.uninstallAction')"
                      :disabled="isPending(plugin, 'uninstall')"
                    >
                      <Archive class="size-3.5" />
                    </Button>
                  </template>
                </ConfirmPopover>
                <ConfirmPopover
                  v-if="plugin.status === 'uninstalled'"
                  :message="$t('bots.plugins.purgeConfirm')"
                  @confirm="purgePlugin(plugin)"
                >
                  <template #trigger>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      class="size-7 text-muted-foreground hover:text-destructive"
                      :title="$t('bots.plugins.purgeAction')"
                      :disabled="isPending(plugin, 'purge')"
                    >
                      <Trash2 class="size-3.5" />
                    </Button>
                  </template>
                </ConfirmPopover>
              </div>
            </div>

            <div class="flex flex-wrap items-center gap-1.5">
              <Badge
                v-for="tag in plugin.manifest?.tags?.slice(0, 4)"
                :key="tag"
                variant="secondary"
                size="sm"
              >
                {{ tag }}
              </Badge>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-3 gap-2 pt-1">
              <div class="rounded-md bg-muted/30 border border-border/50 px-2.5 py-2">
                <p class="text-[10px] uppercase text-muted-foreground">
                  {{ $t('bots.plugins.resourcesLabel') }}
                </p>
                <p class="text-xs font-medium text-foreground mt-1">
                  {{ mcpResourceCount(plugin) }} MCP / {{ skillResourceCount(plugin) }} Skill
                </p>
              </div>
              <div class="rounded-md bg-muted/30 border border-border/50 px-2.5 py-2">
                <p class="text-[10px] uppercase text-muted-foreground">
                  {{ $t('bots.plugins.authLabel') }}
                </p>
                <p class="text-xs font-medium text-foreground mt-1 truncate">
                  {{ authSummary(plugin) }}
                </p>
              </div>
              <div class="rounded-md bg-muted/30 border border-border/50 px-2.5 py-2">
                <p class="text-[10px] uppercase text-muted-foreground">
                  {{ $t('bots.plugins.toolPrefixLabel') }}
                </p>
                <p
                  class="text-xs font-mono text-foreground mt-1 truncate"
                  :title="toolPrefix(plugin)"
                >
                  {{ toolPrefix(plugin) || '-' }}
                </p>
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { Archive, ExternalLink, PackageOpen, Power, PowerOff, RefreshCw, Store, Trash2 } from 'lucide-vue-next'
import { Badge, Button, Spinner } from '@memohai/ui'
import {
  deleteBotsByBotIdPluginsById,
  getBotsByBotIdPlugins,
  getBotsByBotIdPluginsByIdOauthStatus,
  postBotsByBotIdPluginsByIdDisable,
  postBotsByBotIdPluginsByIdEnable,
  postBotsByBotIdPluginsByIdOauthAuthorize,
  postBotsByBotIdPluginsByIdUninstall,
  type PluginsInstallation,
} from '@memohai/sdk'
import { client } from '@memohai/sdk/client'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ProviderIcon from '@/components/provider-icon/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const router = useRouter()
const plugins = ref<PluginsInstallation[]>([])
const loading = ref(false)
const pendingKey = ref('')

onMounted(() => {
  void loadPlugins()
})

watch(() => props.botId, () => {
  void loadPlugins()
})

async function loadPlugins() {
  if (!props.botId) return
  loading.value = true
  try {
    const { data } = await getBotsByBotIdPlugins({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    plugins.value = data.items ?? []
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.plugins.loadFailed')))
  } finally {
    loading.value = false
  }
}

function pluginIcon(plugin: PluginsInstallation): string {
  const icon = plugin.manifest?.icon
  if (!icon) return ''
  if (icon.kind === 'external_url') return icon.url || ''
  if (icon.kind === 'builtin') return icon.name || ''
  return ''
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

function statusClass(status?: string): string {
  switch (status) {
    case 'ready': return 'text-success border-success/30'
    case 'needs_auth':
    case 'needs_config': return 'text-warning border-warning/30'
    case 'admin_required':
    case 'uninstalled': return 'text-destructive border-destructive/30'
    case 'disabled': return 'text-muted-foreground'
    default: return 'text-muted-foreground'
  }
}

function mcpResourceCount(plugin: PluginsInstallation): number {
  return plugin.resources?.filter(item => item.resource_type === 'mcp').length ?? 0
}

function skillResourceCount(plugin: PluginsInstallation): number {
  return plugin.resources?.filter(item => item.resource_type === 'skill').length ?? 0
}

function authSummary(plugin: PluginsInstallation): string {
  const types = new Set((plugin.manifest?.auth_requirements ?? []).map(item => item.type || 'none'))
  if (types.size === 0) return t('bots.plugins.auth.none')
  return Array.from(types).map(authTypeLabel).join(', ')
}

function authTypeLabel(type: string): string {
  switch (type) {
    case 'managed_oauth': return t('bots.plugins.auth.managed_oauth')
    case 'user_secret': return t('bots.plugins.auth.user_secret')
    case 'none': return t('bots.plugins.auth.none')
    default: return type
  }
}

function toolPrefix(plugin: PluginsInstallation): string {
  const resource = plugin.resources?.find(item => item.resource_type === 'mcp')
  const rawPrefix = resource?.metadata?.tool_prefix
  return typeof rawPrefix === 'string' ? rawPrefix : ''
}

function isPending(plugin: PluginsInstallation, action: string): boolean {
  return pendingKey.value === `${plugin.id}:${action}`
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
  pendingKey.value = `${plugin.id}:${action}`
  try {
    await task()
  } finally {
    pendingKey.value = ''
  }
}

async function enablePlugin(plugin: PluginsInstallation) {
  await withPending(plugin, 'enable', async () => {
    await postBotsByBotIdPluginsByIdEnable({
      path: { bot_id: props.botId, id: plugin.id! },
      throwOnError: true,
    })
    toast.success(t('bots.plugins.enableSuccess'))
    await loadPlugins()
  })
}

async function disablePlugin(plugin: PluginsInstallation) {
  await withPending(plugin, 'disable', async () => {
    await postBotsByBotIdPluginsByIdDisable({
      path: { bot_id: props.botId, id: plugin.id! },
      throwOnError: true,
    })
    toast.success(t('bots.plugins.disableSuccess'))
    await loadPlugins()
  })
}

async function startOAuth(plugin: PluginsInstallation) {
  await withPending(plugin, 'oauth', async () => {
    try {
      const { data } = await postBotsByBotIdPluginsByIdOauthAuthorize({
        path: { bot_id: props.botId, id: plugin.id! },
        body: { callback_url: mcpOAuthCallbackUrl() },
        throwOnError: true,
      })
      if (!data.authorization_url) throw new Error(t('mcp.oauth.flowInitFailed'))

      const popup = window.open(data.authorization_url, 'mcp-oauth', 'width=600,height=700')
      await waitForMCPOAuth(plugin, popup)
      const synced = await syncOAuthStatus(plugin)
      if (synced.status !== 'ready' && !synced.enabled) throw new Error(t('mcp.oauth.authFailed'))
      toast.success(t('mcp.oauth.authSuccess'))
      await loadPlugins()
    } catch (error) {
      const synced = await syncOAuthStatus(plugin).catch(() => null)
      if (synced?.status === 'ready' || synced?.enabled) {
        toast.success(t('mcp.oauth.authSuccess'))
        await loadPlugins()
        return
      }
      toast.error(resolveApiErrorMessage(error, t('mcp.oauth.flowInitFailed')))
    }
  })
}

function waitForMCPOAuth(plugin: PluginsInstallation, popup: Window | null): Promise<void> {
  return new Promise((resolve, reject) => {
    let completed = false
    const startedAt = Date.now()
    let pollTimer: ReturnType<typeof setInterval> | undefined

    const finish = (status: 'success' | 'error', error?: string) => {
      if (completed) return
      completed = true
      if (pollTimer) clearInterval(pollTimer)
      window.removeEventListener('message', onMessage)
      if (status === 'success') {
        resolve()
      } else {
        reject(new Error(error || t('mcp.oauth.authFailed')))
      }
    }

    const onMessage = (event: MessageEvent) => {
      if (event.data?.type !== 'mcp-oauth-callback') return
      finish(event.data.status === 'success' ? 'success' : 'error', event.data.error)
    }

    window.addEventListener('message', onMessage)
    pollTimer = setInterval(() => {
      getBotsByBotIdPluginsByIdOauthStatus({
        path: { bot_id: props.botId, id: plugin.id! },
        throwOnError: true,
      }).then(({ data }) => {
        if (completed) return
        if (data.status === 'ready' || data.enabled) {
          finish('success')
          return
        }
        if (popup?.closed || Date.now() - startedAt > 120_000) {
          finish('error')
        }
      }).catch(() => {
        if (!completed && (popup?.closed || Date.now() - startedAt > 120_000)) {
          finish('error')
        }
      })
    }, 2000)
  })
}

async function refreshOAuth(plugin: PluginsInstallation) {
  await withPending(plugin, 'status', async () => {
    await syncOAuthStatus(plugin)
  })
}

async function syncOAuthStatus(plugin: PluginsInstallation): Promise<PluginsInstallation> {
  const { data } = await getBotsByBotIdPluginsByIdOauthStatus({
    path: { bot_id: props.botId, id: plugin.id! },
    throwOnError: true,
  })
  plugins.value = plugins.value.map(item => item.id === data.id ? data : item)
  return data
}

async function uninstallPlugin(plugin: PluginsInstallation) {
  await withPending(plugin, 'uninstall', async () => {
    await postBotsByBotIdPluginsByIdUninstall({
      path: { bot_id: props.botId, id: plugin.id! },
      throwOnError: true,
    })
    toast.success(t('bots.plugins.uninstallSuccess'))
    await loadPlugins()
  })
}

async function purgePlugin(plugin: PluginsInstallation) {
  await withPending(plugin, 'purge', async () => {
    await deleteBotsByBotIdPluginsById({
      path: { bot_id: props.botId, id: plugin.id! },
      throwOnError: true,
    })
    toast.success(t('bots.plugins.purgeSuccess'))
    await loadPlugins()
  })
}
</script>
