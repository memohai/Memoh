<template>
  <div class="mx-auto space-y-5">
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0 space-y-1">
        <h3 class="text-sm font-semibold">
          {{ $t('bots.settings.networkPageTitle') }}
        </h3>
        <p class="text-xs text-muted-foreground">
          {{ $t('bots.settings.networkPageSubtitle') }}
        </p>
      </div>
      <Button
        variant="outline"
        size="sm"
        class="shrink-0"
        :disabled="!props.botId || isNetworkStatusFetching"
        @click="handleRefreshNetworkStatus"
      >
        <Spinner
          v-if="isNetworkStatusFetching"
          class="mr-1.5"
        />
        {{ $t('common.refresh') }}
      </Button>
    </div>

    <!-- Status card -->
    <div
      v-if="props.botId"
      class="rounded-md border p-4 space-y-4"
    >
      <div
        v-if="isNetworkStatusLoading && !networkStatusCard"
        class="flex items-center gap-2 text-xs text-muted-foreground"
      >
        <Spinner />
        <span>{{ $t('common.loading') }}</span>
      </div>

      <template v-else-if="networkStatusCard">
        <template v-if="isNetworkStatusPendingSave">
          <div class="space-y-1">
            <p class="text-xs font-medium text-foreground">
              {{ networkStatusCard.title || networkStatusCard.state }}
            </p>
            <p
              v-if="networkStatusCard.description"
              class="text-xs text-muted-foreground"
            >
              {{ networkStatusCard.description }}
            </p>
          </div>
        </template>

        <template v-else>
          <dl
            v-if="workspaceStatusFields.length"
            class="grid grid-cols-1 gap-3 text-xs sm:grid-cols-2"
          >
            <div
              v-for="(item, idx) in workspaceStatusFields"
              :key="`ws-${idx}-${item.label}`"
              class="space-y-1"
            >
              <dt class="text-muted-foreground">
                {{ item.label }}
              </dt>
              <dd class="break-all font-mono text-xs">
                {{ item.value }}
              </dd>
            </div>
          </dl>
          <p
            v-else
            class="text-xs text-muted-foreground"
          >
            {{ $t('bots.settings.networkStatusEmpty') }}
          </p>

          <div
            v-if="showOverlayStatusInNetworkCard"
            class="space-y-3 border-t border-border pt-3"
          >
            <h4 class="text-xs font-medium">
              {{ $t('bots.settings.networkSDWANSectionTitle') }}
            </h4>

            <!-- needs_login: prominent login banner -->
            <div
              v-if="overlayState === 'needs_login'"
              class="space-y-3"
            >
              <p class="text-xs text-muted-foreground">
                {{ $t('bots.settings.networkNeedsLoginDescription') }}
              </p>
              <Button
                v-if="overlayAuthURL"
                size="sm"
                @click="openAuthURL"
              >
                {{ $t('bots.settings.networkOpenLoginPage') }}
              </Button>
              <p class="text-xs text-muted-foreground">
                {{ $t('bots.settings.networkNeedsLoginHint') }}
              </p>
            </div>

            <template v-else>
              <div class="space-y-1">
                <p class="text-xs font-medium text-foreground">
                  {{ networkStatusCard.title || networkStatusCard.state }}
                </p>
                <p
                  v-if="networkStatusCard.description"
                  class="text-xs text-muted-foreground"
                >
                  {{ networkStatusCard.description }}
                </p>
              </div>

              <dl
                v-if="overlayNetworkStatusFields.length"
                class="grid grid-cols-1 gap-3 text-xs sm:grid-cols-2"
              >
                <div
                  v-for="(item, idx) in overlayNetworkStatusFields"
                  :key="`ov-${idx}-${item.label}`"
                  class="space-y-1"
                >
                  <dt class="text-muted-foreground">
                    {{ item.label }}
                  </dt>
                  <dd class="break-all font-mono text-xs">
                    {{ item.value }}
                  </dd>
                </div>
              </dl>
            </template>

            <!-- Logout button — shown whenever sidecar exists (connected or waiting login) -->
            <div
              v-if="showLogoutButton"
              class="pt-1"
            >
              <Button
                variant="outline"
                size="sm"
                :disabled="isLoggingOut"
                @click="handleLogout"
              >
                <Spinner
                  v-if="isLoggingOut"
                  class="mr-1.5"
                />
                {{ $t('bots.settings.networkLogout') }}
              </Button>
            </div>
          </div>
        </template>
      </template>
    </div>

    <!-- Configuration card -->
    <div
      v-if="props.botId"
      class="rounded-md border p-4 space-y-4"
    >
      <div class="space-y-1">
        <div class="flex items-center justify-between gap-3">
          <h4 class="text-xs font-medium">
            {{ $t('bots.settings.networkSDWANSectionTitle') }}
          </h4>
          <Switch
            class="shrink-0"
            :model-value="form.overlay_enabled"
            @update:model-value="(value) => form.overlay_enabled = !!value"
          />
        </div>
        <p class="text-xs text-muted-foreground">
          {{ $t('bots.settings.networkSDWANSectionHint') }}
        </p>
      </div>

      <div
        v-if="form.overlay_enabled"
        class="space-y-4"
      >
        <div class="space-y-2">
          <Label>{{ $t('bots.settings.overlayProviderFieldLabel') }}</Label>
          <OverlayProviderSelect
            v-model="form.overlay_provider"
            :providers="overlayProviderMeta"
            :placeholder="$t('bots.settings.overlayProviderPlaceholder')"
          />
        </div>

        <!-- Unified config form.
             When connected, auth fields (auth_key, control_url, setup_key, management_url)
             are rendered as readonly to prevent accidental identity changes. -->
        <ConfigSchemaForm
          v-if="showOverlayConfig"
          v-model="form.overlay_config"
          :schema="selectedOverlayProviderSchema"
          id-prefix="bot-network-config"
        />
      </div>
    </div>

    <!-- Exit Node card — independent from config form -->
    <div
      v-if="showExitNodeSelector"
      class="rounded-md border p-4 space-y-4"
    >
      <div class="space-y-1">
        <div class="flex items-center justify-between gap-3">
          <h4 class="text-xs font-medium">
            {{ $t('bots.settings.networkExitNode') }}
          </h4>
          <Button
            variant="outline"
            size="sm"
            class="shrink-0"
            :disabled="!shouldLoadNodeOptions || isNodeListLoading"
            @click="handleRefreshNodes"
          >
            <Spinner
              v-if="isNodeListLoading"
              class="mr-1.5"
            />
            {{ $t('common.refresh') }}
          </Button>
        </div>
        <p class="text-xs text-muted-foreground">
          {{ $t('bots.settings.networkExitNodeSectionHint') }}
        </p>
      </div>

      <NetworkNodeSelect
        v-model="exitNodeValue"
        :nodes="exitNodeOptions"
        :placeholder="$t('bots.settings.networkExitNodePlaceholder')"
      />

      <p class="text-xs text-muted-foreground">
        {{ nodeListHint }}
      </p>

      <div
        v-if="selectedExitNodeMeta"
        class="grid gap-3 md:grid-cols-2"
      >
        <div class="rounded-md border border-border bg-background/60 px-3 py-2">
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.settings.networkExitNodeStatus') }}
          </p>
          <p class="mt-1 text-xs font-medium text-foreground">
            {{ selectedExitNodeMeta.online ? $t('bots.settings.networkExitNodeOnline') : $t('bots.settings.networkExitNodeOffline') }}
          </p>
        </div>
        <div class="rounded-md border border-border bg-background/60 px-3 py-2">
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.settings.networkExitNodeAddresses') }}
          </p>
          <p class="mt-1 text-xs font-medium text-foreground break-all">
            {{ (selectedExitNodeMeta.addresses ?? []).join(', ') || '-' }}
          </p>
        </div>
      </div>
    </div>

    <div class="flex justify-end">
      <Button
        :disabled="!hasChanges || isSaving"
        @click="handleSave"
      >
        <Spinner v-if="isSaving" />
        {{ $t('bots.settings.save') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Label,
  Button,
  Spinner,
  Switch,
} from '@memohai/ui'
import { reactive, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import { getBotsByBotIdSettings, putBotsByBotIdSettings } from '@memohai/sdk'
import type { SettingsSettings } from '@memohai/sdk'
import ConfigSchemaForm from '@/components/config-schema-form/index.vue'
import { cloneConfig } from '@/components/config-schema-form/utils'
import type { ConfigSchema } from '@/components/config-schema-form/types'
import { resolveApiErrorMessage } from '@/utils/api-error'
import OverlayProviderSelect from './network-provider-select.vue'
import NetworkNodeSelect from './network-node-select.vue'
import {
  getBotNetworkStatus,
  executeBotNetworkAction,
  listBotNetworkNodes,
  listOverlayProviderMeta,
  type NetworkBotStatus,
  type OverlayProviderMeta,
} from '@/pages/network/api'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const queryCache = useQueryCache()

const { data: settings } = useQuery({
  key: () => ['bot-settings', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdSettings({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId,
})

const { data: overlayProviderMetaData } = useQuery({
  key: ['network-providers-meta'],
  query: () => listOverlayProviderMeta(),
})

const overlayProviderMeta = computed(() => overlayProviderMetaData.value ?? [])

const form = reactive({
  overlay_enabled: false,
  overlay_provider: '',
  overlay_config: {} as Record<string, unknown>,
})

const selectedOverlayProviderMeta = computed(() =>
  overlayProviderMeta.value.find((meta: OverlayProviderMeta) => meta.kind === form.overlay_provider),
)
const selectedNetworkCapabilities = computed(() =>
  selectedOverlayProviderMeta.value?.capabilities ?? null,
)
// Config schema excludes exit_node (managed by the dedicated Exit Node card).
const selectedOverlayProviderSchema = computed<ConfigSchema | undefined>(() => {
  const schema = selectedOverlayProviderMeta.value?.config_schema as ConfigSchema | undefined
  if (!schema) return undefined
  return {
    ...schema,
    fields: (schema.fields ?? []).filter(field => field.key !== 'exit_node'),
  }
})
const showOverlayConfig = computed(() =>
  !!form.overlay_enabled
  && !!form.overlay_provider
  && !!selectedOverlayProviderSchema.value?.fields?.length,
)
// Exit node selection only makes sense after the sidecar is authenticated and connected.
const showExitNodeSelector = computed(() =>
  !!form.overlay_enabled
  && !!form.overlay_provider
  && !!selectedNetworkCapabilities.value?.exit_node
  && isConnected.value,
)

const persistedOverlayProvider = computed(() => settings.value?.overlay_provider ?? '')
const persistedOverlayEnabled = computed(() => settings.value?.overlay_enabled ?? false)
const persistedOverlayConfig = computed(() =>
  JSON.stringify((settings.value?.overlay_config as Record<string, unknown> | undefined) ?? {}),
)
const isSelectedNetworkPersisted = computed(() =>
  form.overlay_enabled === persistedOverlayEnabled.value
  && form.overlay_provider === persistedOverlayProvider.value
  && JSON.stringify(form.overlay_config ?? {}) === persistedOverlayConfig.value,
)
const shouldLoadNetworkStatus = computed(() =>
  !!props.botId
  && persistedOverlayEnabled.value
  && !!persistedOverlayProvider.value
  && isSelectedNetworkPersisted.value,
)
const shouldLoadNodeOptions = computed(() =>
  !!props.botId
  && shouldLoadNetworkStatus.value
  && !!selectedNetworkCapabilities.value?.exit_node,
)

// Transient states that should trigger automatic polling until resolved.
const TRANSIENT_STATES = ['starting', 'needs_login', 'needslogin', 'stopped']

const isTransientState = computed(() =>
  TRANSIENT_STATES.includes(overlayState.value),
)

const {
  data: networkStatusData,
  refetch: refetchNetworkStatus,
  isFetching: isNetworkStatusFetching,
  isLoading: isNetworkStatusLoading,
} = useQuery({
  key: () => ['bot-network-status', props.botId],
  query: () => getBotNetworkStatus(props.botId),
  enabled: () => !!props.botId,
  refetchOnWindowFocus: true,
})

const {
  data: nodeListData,
  isLoading: isNodeListLoading,
  refetch: refetchNodeList,
} = useQuery({
  key: () => ['bot-network-nodes', props.botId, persistedOverlayProvider.value],
  query: () => listBotNetworkNodes(props.botId),
  enabled: () => shouldLoadNodeOptions.value,
})

const { mutateAsync: updateSettings, isLoading: isSaving } = useMutation({
  mutation: async (body: Partial<SettingsSettings>) => {
    const { data } = await putBotsByBotIdSettings({
      path: { bot_id: props.botId },
      body,
      throwOnError: true,
    })
    return data
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['bot-settings', props.botId] })
    queryCache.invalidateQueries({ key: ['bot-network-status', props.botId] })
    queryCache.invalidateQueries({ key: ['bot-network-nodes', props.botId] })
  },
})

const { mutateAsync: runNetworkAction, isLoading: isLoggingOut } = useMutation({
  mutation: (actionID: string) =>
    executeBotNetworkAction(props.botId, actionID, {}),
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['bot-network-status', props.botId] })
  },
})

// ---------------------------------------------------------------------------
// Overlay state helpers
// ---------------------------------------------------------------------------

const overlayState = computed(() => {
  const status = networkStatusData.value as NetworkBotStatus | null
  return status?.state ?? ''
})

const overlayAuthURL = computed(() => {
  const status = networkStatusData.value as NetworkBotStatus | null
  return (status?.details?.auth_url as string | undefined) ?? ''
})

// "Connected" means sidecar is fully running and authenticated.
const isConnected = computed(() =>
  ['ready', 'running', 'degraded'].includes(overlayState.value),
)

// Show logout when the sidecar is alive (connected or waiting for login).
const showLogoutButton = computed(() =>
  shouldLoadNetworkStatus.value
  && !isNetworkStatusPendingSave.value
  && ['ready', 'running', 'degraded', 'needs_login', 'starting', 'stopped'].includes(overlayState.value),
)

// ---------------------------------------------------------------------------

const exitNodeOptions = computed(() =>
  (nodeListData.value?.items ?? []).filter(node => node.can_exit_node !== false),
)
const nodeListHint = computed(() => {
  if (!isSelectedNetworkPersisted.value) return t('bots.settings.networkNodesPendingSave')
  if (nodeListData.value?.message) return nodeListData.value.message
  if (!exitNodeOptions.value.length) return t('bots.settings.networkNodesEmpty')
  return t('bots.settings.networkExitNodeDescription')
})
const exitNodeValue = computed({
  get: () => String(form.overlay_config.exit_node ?? ''),
  set: (value: string) => {
    form.overlay_config = {
      ...form.overlay_config,
      exit_node: value || undefined,
    }
  },
})
const selectedExitNodeMeta = computed(() =>
  exitNodeOptions.value.find(node => node.value === exitNodeValue.value),
)

const networkStatusCard = computed(() => {
  if (form.overlay_enabled && form.overlay_provider && !isSelectedNetworkPersisted.value) {
    return {
      state: 'pending_save',
      title: t('bots.settings.networkStatusPendingSaveTitle'),
      description: t('bots.settings.networkStatusPendingSave'),
    }
  }
  if (networkStatusData.value) {
    return networkStatusData.value
  }
  return null
})
const isNetworkStatusPendingSave = computed(() =>
  networkStatusCard.value?.state === 'pending_save',
)

const showOverlayStatusInNetworkCard = computed(() =>
  shouldLoadNetworkStatus.value
  && !isNetworkStatusPendingSave.value
  && !!networkStatusData.value,
)

async function handleRefreshNetworkStatus() {
  await refetchNetworkStatus()
}

function workspaceStateDisplay(state: string) {
  const key = `bots.settings.networkWorkspaceState.${state}`
  const translated = t(key)
  return translated === key ? t('bots.settings.networkWorkspaceState.unknown') : translated
}

const workspaceStatusFields = computed(() => {
  const status = networkStatusCard.value as NetworkBotStatus | { state: string; title?: string; description?: string } | null
  if (!status || status.state === 'pending_save') return []
  if (!('workspace' in status) || !status.workspace) return []
  const ws = status.workspace
  const items: { label: string; value: string }[] = [
    { label: t('bots.settings.networkWorkspaceStateLabel'), value: workspaceStateDisplay(ws.state) },
  ]
  if (ws.container_id) items.push({ label: t('bots.settings.networkWorkspaceContainerID'), value: ws.container_id })
  if (ws.task_status) items.push({ label: t('bots.settings.networkWorkspaceTaskStatus'), value: ws.task_status })
  if (ws.pid != null && ws.pid > 0) {
    items.push({ label: t('bots.settings.networkWorkspaceTaskPID'), value: String(ws.pid) })
  }
  if (ws.network_target) items.push({ label: t('bots.settings.networkWorkspaceTarget'), value: ws.network_target })
  if (ws.message) items.push({ label: t('bots.settings.networkWorkspaceMessage'), value: ws.message })
  return items.filter(item => item.value)
})

const overlayNetworkStatusFields = computed(() => {
  const status = networkStatusCard.value as NetworkBotStatus | null
  if (!status || status.state === 'pending_save') return []
  const details = status.details ?? {}
  const items = [
    { label: t('bots.settings.networkStatusState'), value: status.state },
    { label: t('bots.settings.networkStatusIP'), value: status.network_ip },
    { label: t('bots.settings.networkStatusProxy'), value: status.proxy_address },
    { label: t('bots.settings.networkStatusPID'), value: details.pid == null ? undefined : String(details.pid) },
    { label: t('bots.settings.networkStatusDNSName'), value: details.dns_name as string | undefined },
    { label: t('bots.settings.networkStatusBackendState'), value: details.backend_state as string | undefined },
    { label: t('bots.settings.networkStatusHealth'), value: Array.isArray(details.health) ? details.health.join('; ') : undefined },
    { label: t('bots.settings.networkStatusSocket'), value: details.localapi_socket_host_path as string | undefined },
    { label: t('bots.settings.networkStatusExitNode'), value: details.configured_exit_node as string | undefined },
  ]
  return items.filter(item => item.value)
})

const hasChanges = computed(() => {
  if (!settings.value) return true
  const s = settings.value
  return form.overlay_enabled !== (s.overlay_enabled ?? false)
    || form.overlay_provider !== (s.overlay_provider ?? '')
    || JSON.stringify(form.overlay_config ?? {}) !== JSON.stringify((s.overlay_config as Record<string, unknown> | undefined) ?? {})
})

// When settings load from API, overlay_provider goes from '' to the saved value in the
// same flush as configs are written. A separate watcher on overlay_provider must not
// wipe those configs (it would leave the UI empty after refresh).
let skipProviderChangeReset = false

watch(() => form.overlay_provider, (next, prev) => {
  if (next === prev || skipProviderChangeReset) return
  form.overlay_config = {}
})

watch(settings, (val) => {
  if (!val) return
  skipProviderChangeReset = true
  form.overlay_enabled = val.overlay_enabled ?? false
  form.overlay_provider = val.overlay_provider ?? ''
  form.overlay_config = cloneConfig((val.overlay_config as Record<string, unknown> | undefined) ?? {})
  void nextTick(() => {
    skipProviderChangeReset = false
  })
}, { immediate: true })

// Poll network status every 5s while in a transient state (starting, needs_login, etc.)
let pollTimer: ReturnType<typeof setInterval> | null = null

watch(isTransientState, (shouldPoll) => {
  if (shouldPoll && !pollTimer) {
    pollTimer = setInterval(() => {
      if (isTransientState.value && !isNetworkStatusFetching.value) {
        refetchNetworkStatus()
      }
    }, 5000)
  } else if (!shouldPoll && pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}, { immediate: true })

onBeforeUnmount(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
})

async function handleSave() {
  if (form.overlay_enabled && !form.overlay_provider) {
    toast.error(t('bots.settings.overlayProviderRequired'))
    return
  }
  try {
    await updateSettings({
      overlay_enabled: form.overlay_enabled,
      overlay_provider: form.overlay_provider,
      overlay_config: form.overlay_config,
    })
    toast.success(t('bots.settings.saveSuccess'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.settings.networkActionFailed')))
  }
}

async function handleRefreshNodes() {
  try {
    await refetchNodeList()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.settings.networkNodesRefreshFailed')))
  }
}

function openAuthURL() {
  if (overlayAuthURL.value) {
    window.open(overlayAuthURL.value, '_blank', 'noopener,noreferrer')
  }
}

async function handleLogout() {
  try {
    await runNetworkAction('logout')
    toast.success(t('bots.settings.networkLogoutSuccess'))
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.settings.networkLogoutFailed')))
  }
}
</script>
