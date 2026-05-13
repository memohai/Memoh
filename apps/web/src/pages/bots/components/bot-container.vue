<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { useQuery } from '@pinia/colada'
import { RefreshCw, Play, Square, Box, Database, Settings, History } from 'lucide-vue-next'
import {
  deleteBotsByBotIdContainer,
  getBotsByBotIdContainer,
  getBotsByBotIdContainerMetrics,
  getBotsByBotIdContainerSnapshots,
  getBotsById,
  postBotsByBotIdContainerDataExport,
  postBotsByBotIdContainerDataImport,
  postBotsByBotIdContainerDataRestore,
  postBotsByBotIdContainerSnapshots,
  postBotsByBotIdContainerSnapshotsRollback,
  postBotsByBotIdContainerStart,
  postBotsByBotIdContainerStop,
  type HandlersCreateContainerRequest,
  type HandlersGetContainerMetricsResponse,
  type HandlersGetContainerResponse,
  type HandlersListSnapshotsResponse,
} from '@memohai/sdk'
import {
  postBotsByBotIdContainerStream,
  type ContainerCreateLayerStatus,
  type ContainerCreateStreamEvent,
} from '@/composables/api/useContainerStream'
import { Button, Input, Label, Separator, Spinner, Switch, Textarea } from '@memohai/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import ContainerCreateProgress from './container-create-progress.vue'
import ContainerMetricsPanel from './container-metrics-panel.vue'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'
import { useBotStatusMeta } from '@/composables/useBotStatusMeta'
import { useCapabilitiesStore } from '@/store/capabilities'
import { formatDateTime } from '@/utils/date-time'
import { shortenImageRef } from '@/utils/image-ref'
import { resolveApiErrorMessage } from '@/utils/api-error'

const route = useRoute()
const { t } = useI18n()

type ContainerAction =
  | 'refresh'
  | 'create'
  | 'start'
  | 'stop'
  | 'delete'
  | 'delete-preserve'
  | 'snapshot'
  | 'export'
  | 'import'
  | 'restore'
  | 'rollback'
  | 'recreate'
  | ''

const containerLoading = ref(false)
const containerAction = ref<ContainerAction>('')
const rollbackVersion = ref<number | null>(null)
const createRestoreData = ref(false)
const createImage = ref('')
const createImagePrefilled = ref(false)
const createGPUEnabled = ref(false)
const createGPUDevices = ref('')
const createGPUPrefilled = ref(false)
const newSnapshotName = ref('')
const importInputRef = ref<HTMLInputElement | null>(null)

interface CreateProgress {
  phase: 'preserving' | 'pulling' | 'creating' | 'restoring' | 'complete' | 'error'
  layers?: ContainerCreateLayerStatus[]
  image?: string
  error?: string
}
const createProgress = ref<CreateProgress | null>(null)

const createProgressPercent = computed(() => {
  const layers = createProgress.value?.layers
  if (!layers || layers.length === 0) return 0
  let totalOffset = 0
  let totalSize = 0
  for (const l of layers) {
    totalOffset += l.offset
    totalSize += l.total
  }
  return totalSize > 0 ? Math.round((totalOffset / totalSize) * 100) : 0
})

const capabilitiesStore = useCapabilitiesStore()
const botId = computed(() => route.params.botId as string)
const containerBusy = computed(() => containerLoading.value || containerAction.value !== '')

type BotContainerInfo = HandlersGetContainerResponse
type BotContainerMetrics = HandlersGetContainerMetricsResponse
type BotContainerSnapshot = HandlersListSnapshotsResponse extends { snapshots?: (infer T)[] } ? T : never

const containerInfo = ref<BotContainerInfo | null>(null)
const containerMetrics = ref<BotContainerMetrics | null>(null)
const containerMissing = ref(false)
const snapshots = ref<BotContainerSnapshot[]>([])
const metricsLoading = ref(false)
const snapshotsLoading = ref(false)

function resolveErrorMessage(error: unknown, fallback: string): string {
  return resolveApiErrorMessage(error, fallback)
}

async function runContainerAction<T>(
  action: ContainerAction,
  operation: () => Promise<T>,
  successMessage?: string | ((result: T) => string),
) {
  containerAction.value = action
  try {
    const result = await operation()
    const message = typeof successMessage === 'function'
      ? successMessage(result)
      : successMessage
    if (message) {
      toast.success(message)
    }
    return result
  } catch (error) {
    toast.error(resolveErrorMessage(error, t('bots.container.actionFailed')))
    return undefined
  } finally {
    containerAction.value = ''
  }
}

async function loadContainerData(showLoadingToast: boolean) {
  await capabilitiesStore.load()
  containerLoading.value = true
  try {
    const result = await getBotsByBotIdContainer({ path: { bot_id: botId.value } })
    if (result.error !== undefined) {
      if (result.response.status === 404) {
        containerInfo.value = null
        containerMetrics.value = null
        containerMissing.value = true
        snapshots.value = []
        return
      }
      throw result.error
    }

    containerInfo.value = result.data
    containerMissing.value = false

    const metricsPromise = loadContainerMetrics(showLoadingToast)

    if (capabilitiesStore.snapshotSupported) {
      await Promise.all([metricsPromise, loadSnapshots()])
    } else {
      snapshots.value = []
      await metricsPromise
    }
  } catch (error) {
    if (showLoadingToast) {
      toast.error(resolveErrorMessage(error, t('bots.container.loadFailed')))
    }
  } finally {
    containerLoading.value = false
  }
}

async function loadContainerMetrics(showLoadingToast: boolean) {
  metricsLoading.value = true
  try {
    const { data } = await getBotsByBotIdContainerMetrics({
      path: { bot_id: botId.value },
      throwOnError: true,
    })
    containerMetrics.value = data
  } catch (error) {
    containerMetrics.value = null
    if (showLoadingToast) {
      toast.error(resolveErrorMessage(error, t('bots.container.metricsLoadFailed')))
    }
  } finally {
    metricsLoading.value = false
  }
}

async function loadSnapshots() {
  if (!containerInfo.value || !capabilitiesStore.snapshotSupported) {
    snapshots.value = []
    return
  }

  snapshotsLoading.value = true
  try {
    const { data } = await getBotsByBotIdContainerSnapshots({
      path: { bot_id: botId.value },
      throwOnError: true,
    })
    snapshots.value = data.snapshots ?? []
  } catch (error) {
    snapshots.value = []
    toast.error(resolveErrorMessage(error, t('bots.container.snapshotLoadFailed')))
  } finally {
    snapshotsLoading.value = false
  }
}

async function handleRefreshContainer() {
  await runContainerAction('refresh', () => loadContainerData(false))
}

const { data: bot, refetch: refetchBot } = useQuery({
  key: () => ['bot', botId.value],
  query: async () => {
    const { data } = await getBotsById({ path: { id: botId.value }, throwOnError: true })
    return data
  },
  enabled: () => !!botId.value,
})

function rememberedWorkspaceImage(metadata: Record<string, unknown> | undefined): string {
  const workspace = metadata?.workspace
  if (!workspace || typeof workspace !== 'object' || Array.isArray(workspace)) return ''
  const image = (workspace as Record<string, unknown>).image
  return typeof image === 'string' ? shortenImageRef(image) : ''
}

type RememberedWorkspaceGPU = {
  exists: boolean
  devices: string[]
}

function rememberedWorkspaceGPU(metadata: Record<string, unknown> | undefined): RememberedWorkspaceGPU {
  const workspace = metadata?.workspace
  if (!workspace || typeof workspace !== 'object' || Array.isArray(workspace)) {
    return { exists: false, devices: [] }
  }

  const workspaceRecord = workspace as Record<string, unknown>
  if (!Object.prototype.hasOwnProperty.call(workspaceRecord, 'gpu')) {
    return { exists: false, devices: [] }
  }

  const gpu = workspaceRecord.gpu
  if (!gpu || typeof gpu !== 'object' || Array.isArray(gpu)) {
    return { exists: true, devices: [] }
  }

  const rawDevices = (gpu as Record<string, unknown>).devices
  const devices = Array.isArray(rawDevices)
    ? rawDevices.filter((value): value is string => typeof value === 'string').map(value => value.trim()).filter(Boolean)
    : []

  return { exists: true, devices: [...new Set(devices)] }
}

function parseCDIDevices(value: string): string[] {
  return [...new Set(
    value
      .split(/[\n,]/)
      .map(item => item.trim())
      .filter(Boolean),
  )]
}

const rememberedCreateImage = computed(() => rememberedWorkspaceImage(bot.value?.metadata as Record<string, unknown> | undefined))
const rememberedCreateGPU = computed(() => rememberedWorkspaceGPU(bot.value?.metadata as Record<string, unknown> | undefined))
const displayedContainerImage = computed(() => shortenImageRef(containerInfo.value?.image))
const displayedCDIDevices = computed(() => containerInfo.value?.cdi_devices ?? [])

const { isPending: botLifecyclePending } = useBotStatusMeta(bot, t)

const containerStatusColorClass = computed(() => {
  const status = (containerInfo.value?.status ?? '').trim().toLowerCase()
  if (status === 'running') return 'bg-success'
  if (status === 'created') return 'bg-primary'
  if (status === 'stopped' || status === 'exited') return 'bg-muted-foreground'
  return 'bg-muted-foreground'
})

function applyCreateContainerEvent(event: ContainerCreateStreamEvent): boolean {
  switch (event.type) {
    case 'pulling':
      createProgress.value = { phase: 'pulling', image: event.image }
      return false
    case 'pull_progress':
      createProgress.value = {
        phase: 'pulling',
        image: createProgress.value?.image,
        layers: event.layers,
      }
      return false
    case 'pull_skipped':
    case 'pull_delegated':
      createProgress.value = { phase: 'pulling', image: event.image }
      return false
    case 'creating':
      createProgress.value = { phase: 'creating' }
      return false
    case 'restoring':
      createProgress.value = { phase: 'restoring' }
      return false
    case 'complete':
      // Keep the last visible progress state until the container detail view loads.
      // Rendering a separate "complete" phase here looks like the bar jumped back to 0.
      return !!event.container.data_restored
    case 'error':
      createProgress.value = { phase: 'error', error: event.message }
      throw new Error(event.message || 'Unknown error')
  }
}

async function createContainerSSE(body: HandlersCreateContainerRequest): Promise<{ dataRestored: boolean }> {
  const { stream } = await postBotsByBotIdContainerStream({
    path: { bot_id: botId.value },
    body,
    throwOnError: true,
  })

  let dataRestored = false
  for await (const event of stream) {
    dataRestored = applyCreateContainerEvent(event) || dataRestored
  }

  return { dataRestored }
}

async function handleCreateContainer() {
  if (botLifecyclePending.value) return

  containerAction.value = 'create'
  createProgress.value = { phase: 'pulling' }
  try {
    const gpuDevices = parseCDIDevices(createGPUDevices.value)
    if (createGPUEnabled.value && gpuDevices.length === 0) {
      throw new Error(t('bots.container.gpuDevicesRequired'))
    }

    const body: HandlersCreateContainerRequest = {
      restore_data: createRestoreData.value,
    }
    const trimmedImage = createImage.value.trim()
    if (trimmedImage) body.image = trimmedImage
    if (createGPUEnabled.value || rememberedCreateGPU.value.exists) {
      body.gpu = {
        devices: createGPUEnabled.value ? gpuDevices : [],
      }
    }

    const { dataRestored } = await createContainerSSE(body)
    createRestoreData.value = false
    createImage.value = ''
    createGPUEnabled.value = false
    createGPUDevices.value = ''
    await loadContainerData(false)
    await refetchBot()
    toast.success(dataRestored
      ? t('bots.container.createRestoreSuccess')
      : t('bots.container.createSuccess'))
  }
  catch (error) {
    toast.error(resolveErrorMessage(error, t('bots.container.actionFailed')))
  }
  finally {
    containerAction.value = ''
    createProgress.value = null
  }
}

const isContainerTaskRunning = computed(() => {
  const info = containerInfo.value
  if (!info) return false

  if (info.task_running) return true
  const status = (info.status ?? '').trim().toLowerCase()
  if (status === 'stopped' || status === 'exited') return false
  return false
})

const hasPreservedData = computed(() => !!containerInfo.value?.has_preserved_data)
const isLegacy = computed(() => !!containerInfo.value?.legacy)

async function handleRecreateContainer() {
  if (botLifecyclePending.value || !containerInfo.value) return

  containerAction.value = 'recreate'
  try {
    createProgress.value = { phase: 'preserving' }
    await deleteBotsByBotIdContainer({
      path: { bot_id: botId.value },
      query: { preserve_data: true },
      throwOnError: true,
    })

    createProgress.value = { phase: 'pulling' }
    await createContainerSSE({ restore_data: true })
    await loadContainerData(false)
    toast.success(t('bots.container.legacyRecreateSuccess'))
  }
  catch (error) {
    toast.error(resolveErrorMessage(error, t('bots.container.actionFailed')))
  }
  finally {
    containerAction.value = ''
    createProgress.value = null
  }
}

async function handleStopContainer() {
  if (botLifecyclePending.value || !containerInfo.value) return

  await runContainerAction(
    'stop',
    async () => {
      await postBotsByBotIdContainerStop({ path: { bot_id: botId.value }, throwOnError: true })
      await loadContainerData(false)
    },
    t('bots.container.stopSuccess'),
  )
}

async function handleStartContainer() {
  if (botLifecyclePending.value || !containerInfo.value) return

  await runContainerAction(
    'start',
    async () => {
      await postBotsByBotIdContainerStart({ path: { bot_id: botId.value }, throwOnError: true })
      await loadContainerData(false)
    },
    t('bots.container.startSuccess'),
  )
}

async function handleDeleteContainer(preserveData: boolean) {
  if (botLifecyclePending.value || !containerInfo.value) return

  const action: ContainerAction = preserveData ? 'delete-preserve' : 'delete'
  const successMessage = preserveData
    ? t('bots.container.deletePreserveSuccess')
    : t('bots.container.deleteSuccess')
  const lastImage = shortenImageRef(containerInfo.value.image)

  await runContainerAction(
    action,
    async () => {
      await deleteBotsByBotIdContainer({
        path: { bot_id: botId.value },
        query: preserveData ? { preserve_data: true } : undefined,
        throwOnError: true,
      })
      containerInfo.value = null
      containerMetrics.value = null
      containerMissing.value = true
      snapshots.value = []
      createRestoreData.value = preserveData
      createImage.value = lastImage
      createImagePrefilled.value = !!lastImage
    },
    successMessage,
  )
}

function buildExportFilename() {
  const timestamp = new Date().toISOString().replaceAll(':', '-')
  return `bot-${botId.value}-data-${timestamp}.tar.gz`
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  window.setTimeout(() => URL.revokeObjectURL(url), 0)
}

async function handleExportData() {
  if (botLifecyclePending.value || !containerInfo.value) return

  await runContainerAction(
    'export',
    async () => {
      const response = await postBotsByBotIdContainerDataExport({
        path: { bot_id: botId.value },
        parseAs: 'blob',
        throwOnError: true,
      })
      downloadBlob(response.data as unknown as Blob, buildExportFilename())
    },
    t('bots.container.exportSuccess'),
  )
}

function triggerImportData() {
  importInputRef.value?.click()
}

async function handleImportData(event: Event) {
  if (botLifecyclePending.value || !containerInfo.value) return

  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return

  await runContainerAction(
    'import',
    async () => {
      await postBotsByBotIdContainerDataImport({
        path: { bot_id: botId.value },
        body: { file },
        throwOnError: true,
      })
      await loadContainerData(false)
    },
    t('bots.container.importSuccess'),
  )

  input.value = ''
}

async function handleRestorePreservedData() {
  if (botLifecyclePending.value || !containerInfo.value || !hasPreservedData.value) return

  await runContainerAction(
    'restore',
    async () => {
      await postBotsByBotIdContainerDataRestore({
        path: { bot_id: botId.value },
        throwOnError: true,
      })
      await loadContainerData(false)
    },
    t('bots.container.restoreSuccess'),
  )
}

const statusKeyMap: Record<string, string> = {
  created: 'statusCreated',
  running: 'statusRunning',
  stopped: 'statusStopped',
  exited: 'statusExited',
}

const containerStatusText = computed(() => {
  const status = (containerInfo.value?.status ?? '').trim().toLowerCase()
  const key = statusKeyMap[status] ?? 'statusUnknown'
  return t(`bots.container.${key}`)
})

const containerTaskText = computed(() => {
  const info = containerInfo.value
  if (!info) return '-'

  const status = (info.status ?? '').trim().toLowerCase()
  if (status === 'exited') return t('bots.container.taskCompleted')
  return info.task_running ? t('bots.container.taskRunning') : t('bots.container.taskStopped')
})

function formatDate(value: string | undefined): string {
  return formatDateTime(value, { fallback: '-' })
}

function snapshotCreatedAt(value: BotContainerSnapshot) {
  const timestamp = Date.parse(value.created_at ?? '')
  return Number.isNaN(timestamp) ? Number.NEGATIVE_INFINITY : timestamp
}

function snapshotDisplayName(value: BotContainerSnapshot) {
  return (value.display_name ?? value.name ?? value.runtime_snapshot_name ?? '').trim() || '-'
}

function snapshotRuntimeName(value: BotContainerSnapshot) {
  const runtimeName = (value.runtime_snapshot_name ?? '').trim()
  return runtimeName && runtimeName !== snapshotDisplayName(value) ? runtimeName : ''
}

function snapshotVersionText(value: BotContainerSnapshot) {
  return value.version !== undefined ? `v${value.version}` : '-'
}

function snapshotSourceText(value: BotContainerSnapshot) {
  const source = (value.source ?? '').trim().toLowerCase()
  if (!source) return '-'

  const sourceKeyMap: Record<string, string> = {
    manual: 'sourceManual',
    pre_exec: 'sourcePreExec',
    rollback: 'sourceRollback',
  }
  const sourceKey = sourceKeyMap[source]
  return sourceKey ? t(`bots.container.${sourceKey}`) : source
}

function canRollbackSnapshot(value: BotContainerSnapshot) {
  return !!value.managed && typeof value.version === 'number' && value.version > 0
}

async function handleRollbackSnapshot(snapshot: BotContainerSnapshot) {
  if (
    botLifecyclePending.value
    || !containerInfo.value
    || !canRollbackSnapshot(snapshot)
    || snapshot.version === undefined
  ) {
    return
  }

  rollbackVersion.value = snapshot.version
  await runContainerAction(
    'rollback',
    async () => {
      await postBotsByBotIdContainerSnapshotsRollback({
        path: { bot_id: botId.value },
        body: { version: snapshot.version },
        throwOnError: true,
      })
      await loadContainerData(false)
    },
    t('bots.container.rollbackSuccess'),
  )
  rollbackVersion.value = null
}

async function handleCreateSnapshot() {
  if (botLifecyclePending.value || !containerInfo.value || !capabilitiesStore.snapshotSupported) return

  await runContainerAction(
    'snapshot',
    async () => {
      await postBotsByBotIdContainerSnapshots({
        path: { bot_id: botId.value },
        body: { snapshot_name: newSnapshotName.value.trim() },
        throwOnError: true,
      })
      newSnapshotName.value = ''
      await loadSnapshots()
    },
    t('bots.container.snapshotSuccess'),
  )
}

const sortedSnapshots = computed(() => {
  return [...snapshots.value].sort((left, right) => {
    const managedDiff = Number(!!right.managed) - Number(!!left.managed)
    if (managedDiff !== 0) return managedDiff

    const leftVersion = left.version ?? Number.NEGATIVE_INFINITY
    const rightVersion = right.version ?? Number.NEGATIVE_INFINITY
    if (leftVersion !== rightVersion) return rightVersion - leftVersion

    const createdDiff = snapshotCreatedAt(right) - snapshotCreatedAt(left)
    if (createdDiff !== 0) return createdDiff

    return snapshotDisplayName(left).localeCompare(snapshotDisplayName(right))
  })
})

const activeTab = useSyncedQueryParam('tab', 'overview')

watch(containerMissing, (missing) => {
  if (!missing) {
    createImagePrefilled.value = false
    createGPUPrefilled.value = false
  }
})

watch([containerMissing, rememberedCreateImage], ([missing, remembered]) => {
  if (!missing || createImagePrefilled.value) return
  if (!remembered || createImage.value.trim()) return
  createImage.value = remembered
  createImagePrefilled.value = true
}, { immediate: true })

watch([containerMissing, rememberedCreateGPU], ([missing, remembered]) => {
  if (!missing || createGPUPrefilled.value) return
  if (!remembered.exists) return
  if (createGPUEnabled.value || createGPUDevices.value.trim()) return
  createGPUEnabled.value = remembered.devices.length > 0
  createGPUDevices.value = remembered.devices.join('\n')
  createGPUPrefilled.value = true
}, { immediate: true })

watch([activeTab, botId], ([tab]) => {
  if (!botId.value) return
  if (tab === 'container') {
    void loadContainerData(true)
  }
}, { immediate: true })
</script>

<template>
  <div class="max-w-2xl mx-auto pb-6 space-y-5">
    <!-- Sovereign Header -->
    <header class="pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-30 pt-4 -mt-4 flex items-center justify-between gap-4">
      <div class="space-y-1">
        <h2 class="text-sm font-semibold text-foreground flex items-center gap-2">
          <span
            v-if="containerInfo"
            class="relative flex items-center justify-center size-2.5"
          >
            <span
              class="absolute inline-flex h-full w-full rounded-full opacity-20"
              :class="containerStatusColorClass"
            />
            <span
              class="relative inline-flex rounded-full size-2"
              :class="containerStatusColorClass"
            />
          </span>
          {{ $t('bots.container.title') }}
        </h2>
        <p class="text-[11px] leading-snug text-muted-foreground max-w-md">
          {{ $t('bots.container.subtitle') }}
        </p>
      </div>
      <div class="flex shrink-0 flex-wrap justify-end gap-2">
        <Button
          variant="outline"
          size="sm"
          :disabled="containerBusy"
          class="shadow-none"
          @click="handleRefreshContainer"
        >
          <Spinner
            v-if="containerLoading || containerAction === 'refresh'"
            class="mr-1.5 size-3.5"
          />
          <RefreshCw
            v-else
            class="mr-1.5 size-3.5 text-muted-foreground"
          />
          {{ $t('common.refresh') }}
        </Button>
        <Button
          v-if="containerInfo"
          variant="secondary"
          size="sm"
          :disabled="containerBusy || botLifecyclePending"
          class="shadow-none"
          @click="isContainerTaskRunning ? handleStopContainer() : handleStartContainer()"
        >
          <Spinner
            v-if="containerAction === 'start' || containerAction === 'stop'"
            class="mr-1.5"
          />
          <Square
            v-else-if="isContainerTaskRunning"
            class="mr-1.5 size-3.5"
          />
          <Play
            v-else
            class="mr-1.5 size-3.5"
          />
          {{ isContainerTaskRunning ? $t('bots.container.actions.stop') : $t('bots.container.actions.start') }}
        </Button>
      </div>
    </header>

    <!-- Bot Not Ready -->
    <div
      v-if="botLifecyclePending"
      class="rounded-md border border-warning-border bg-warning-soft p-3 text-xs text-warning-foreground shadow-none"
    >
      {{ $t('bots.container.botNotReady') }}
    </div>

    <!-- Loading -->
    <div
      v-if="containerLoading && !containerInfo && !containerMissing"
      class="flex items-center gap-2 text-xs text-muted-foreground"
    >
      <Spinner />
      <span>{{ $t('common.loading') }}</span>
    </div>

    <!-- Empty State (Create) -->
    <div
      v-else-if="containerMissing"
      class="space-y-6"
    >
      <div class="flex flex-col items-center justify-center py-10 border border-border/40 border-dashed rounded-lg bg-muted/5">
        <div class="size-10 rounded-full bg-muted/20 flex items-center justify-center mb-4">
          <Box class="size-5 text-muted-foreground" />
        </div>
        <p class="text-sm font-medium text-foreground mb-1">
          {{ $t('bots.container.empty') }}
        </p>
        <p class="text-[11px] text-muted-foreground text-center max-w-sm">
          {{ $t('bots.container.createHint') }}
        </p>
      </div>

      <div class="space-y-4">
        <div class="grid gap-4 sm:grid-cols-2">
          <!-- Restore Data Switch -->
          <div class="flex items-start justify-between gap-4 rounded-md border border-border/60 bg-background p-3 shadow-none">
            <div class="space-y-1">
              <Label class="text-xs font-medium">{{ $t('bots.container.createRestoreDataLabel') }}</Label>
              <p class="text-[11px] text-muted-foreground">
                {{ $t('bots.container.createRestoreDataDescription') }}
              </p>
            </div>
            <Switch
              :model-value="createRestoreData"
              :disabled="containerBusy || botLifecyclePending"
              @update:model-value="(value) => createRestoreData = !!value"
            />
          </div>

          <!-- GPU Switch -->
          <div class="flex items-start justify-between gap-4 rounded-md border border-border/60 bg-background p-3 shadow-none">
            <div class="space-y-1">
              <Label class="text-xs font-medium">{{ $t('bots.container.createGpuLabel') }}</Label>
              <p class="text-[11px] text-muted-foreground">
                {{ $t('bots.container.createGpuDescription') }}
              </p>
            </div>
            <Switch
              :model-value="createGPUEnabled"
              :disabled="containerBusy || botLifecyclePending"
              @update:model-value="(value) => createGPUEnabled = !!value"
            />
          </div>
        </div>

        <!-- Image Input -->
        <div class="space-y-1.5">
          <Label class="text-xs font-medium">{{ $t('bots.container.createImageLabel') }}</Label>
          <Input
            v-model="createImage"
            placeholder="debian:bookworm-slim"
            :disabled="containerBusy || botLifecyclePending"
            class="font-mono text-xs h-8 shadow-none bg-background border-border/60"
          />
          <p class="text-[11px] text-muted-foreground">
            {{ $t('bots.container.createImageDescription') }}
          </p>
        </div>

        <!-- GPU Devices Input -->
        <div
          v-if="createGPUEnabled"
          class="space-y-1.5 animate-in fade-in slide-in-from-top-2 duration-200"
        >
          <Label class="text-xs font-medium">{{ $t('bots.container.createGpuDevicesLabel') }}</Label>
          <Textarea
            v-model="createGPUDevices"
            :placeholder="$t('bots.container.createGpuDevicesPlaceholder')"
            :disabled="containerBusy || botLifecyclePending"
            class="min-h-20 font-mono text-xs shadow-none bg-background border-border/60"
          />
          <p class="text-[11px] text-muted-foreground">
            {{ $t('bots.container.createGpuDevicesDescription') }}
          </p>
        </div>

        <div class="flex justify-end pt-2">
          <Button
            :disabled="containerBusy || botLifecyclePending"
            size="sm"
            class="h-8 text-xs font-medium shadow-none"
            @click="handleCreateContainer"
          >
            <Spinner
              v-if="containerAction === 'create'"
              class="mr-1.5 size-3.5"
            />
            <Play
              v-else
              class="mr-1.5 size-3.5"
            />
            {{ $t('bots.container.actions.create') }}
          </Button>
        </div>

        <div
          v-if="createProgress && (containerAction === 'create')"
          class="space-y-2 mt-4"
        >
          <ContainerCreateProgress
            :phase="createProgress.phase"
            :percent="createProgressPercent"
            :error="createProgress.error"
          />
        </div>
      </div>
    </div>

    <!-- Active Container -->
    <div
      v-else-if="containerInfo"
      class="space-y-6"
    >
      <!-- Legacy Warning -->
      <div
        v-if="isLegacy"
        class="flex items-center justify-between gap-3 rounded-md border border-warning-border bg-warning-soft p-3 shadow-none"
      >
        <p class="text-[11px] text-warning-foreground">
          {{ $t('bots.container.legacyWarning') }}
        </p>
        <Button
          variant="outline"
          size="sm"
          class="shrink-0 h-7 text-[10px] shadow-none"
          :disabled="containerBusy || botLifecyclePending"
          @click="handleRecreateContainer"
        >
          <Spinner
            v-if="containerAction === 'recreate'"
            class="mr-1.5"
          />
          {{ $t('bots.container.legacyRecreate') }}
        </Button>
      </div>
      <div
        v-if="createProgress && containerAction === 'recreate'"
        class="space-y-2 rounded-md border border-border/60 p-3 shadow-none"
      >
        <ContainerCreateProgress
          :phase="createProgress.phase"
          :percent="createProgressPercent"
          :error="createProgress.error"
        />
      </div>

      <!-- Identity Badges (L3 Meta) -->
      <div class="flex flex-wrap items-center gap-2">
        <span class="inline-flex items-center rounded bg-muted/20 px-2 py-0.5 text-[10px] font-mono font-medium text-muted-foreground border border-border/40">
          ID: {{ containerInfo.container_id }}
        </span>
        <span class="inline-flex items-center rounded bg-muted/20 px-2 py-0.5 text-[10px] font-mono font-medium text-muted-foreground border border-border/40">
          IMG: {{ displayedContainerImage }}
        </span>
        <span class="inline-flex items-center rounded bg-muted/20 px-2 py-0.5 text-[10px] font-medium text-muted-foreground border border-border/40">
          TASK: {{ containerTaskText }}
        </span>
      </div>

      <div class="grid grid-cols-12 gap-4">
        <!-- Static Metadata Bento -->
        <div class="col-span-12 rounded-md border border-border/60 bg-muted/5 p-4 shadow-none">
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-4 text-[11px]">
            <div class="space-y-1">
              <span class="text-muted-foreground block">{{ $t('bots.container.fields.status') }}</span>
              <span class="font-medium text-foreground">{{ containerStatusText }}</span>
            </div>
            <div class="space-y-1">
              <span class="text-muted-foreground block">{{ $t('bots.container.fields.namespace') }}</span>
              <span class="font-mono text-foreground">{{ containerInfo.namespace }}</span>
            </div>
            <div class="space-y-1">
              <span class="text-muted-foreground block">{{ $t('bots.container.fields.createdAt') }}</span>
              <span class="text-foreground">{{ formatDate(containerInfo.created_at) }}</span>
            </div>
            <div class="space-y-1">
              <span class="text-muted-foreground block">{{ $t('bots.container.fields.updatedAt') }}</span>
              <span class="text-foreground">{{ formatDate(containerInfo.updated_at) }}</span>
            </div>
            <div class="space-y-1 sm:col-span-2">
              <span class="text-muted-foreground block">{{ $t('bots.container.fields.containerPath') }}</span>
              <span class="font-mono text-foreground break-all">{{ containerInfo.container_path }}</span>
            </div>
            <div class="space-y-1 sm:col-span-2">
              <span class="text-muted-foreground block">{{ $t('bots.container.fields.cdiDevices') }}</span>
              <div
                v-if="displayedCDIDevices.length === 0"
                class="text-muted-foreground"
              >
                {{ $t('bots.container.cdiDevicesEmpty') }}
              </div>
              <div
                v-else
                class="space-y-0.5 font-mono"
              >
                <div
                  v-for="device in displayedCDIDevices"
                  :key="device"
                  class="break-all text-foreground"
                >
                  {{ device }}
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <ContainerMetricsPanel
        :backend="capabilitiesStore.containerBackend"
        :loading="metricsLoading"
        :metrics="containerMetrics"
      />
      <div class="rounded-md border border-border/50 bg-background px-3 py-2 text-[11px] text-muted-foreground shadow-none">
        {{ $t('bots.container.gpuRecreateHint') }}
      </div>

      <!-- Data Operations -->
      <div class="space-y-4">
        <div class="rounded-md border border-border/60 bg-background overflow-hidden shadow-none">
          <div class="p-4 space-y-4">
            <!-- Data Pipeline Group -->
            <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div class="space-y-0.5">
                <h4 class="text-xs font-medium text-foreground flex items-center gap-2">
                  <Database class="size-3.5 text-muted-foreground" />
                  {{ $t('bots.container.dataTitle') }}
                </h4>
                <p class="text-[11px] text-muted-foreground leading-snug">
                  {{ $t('bots.container.dataSubtitle') }}
                </p>
                <div
                  v-if="hasPreservedData"
                  class="mt-2 inline-flex items-center rounded bg-primary/10 px-2 py-0.5 text-[10px] text-primary"
                >
                  {{ $t('bots.container.preservedDataAvailable') }}
                </div>
              </div>
              <div class="flex items-center gap-2 shrink-0 sm:justify-end">
                <Button
                  variant="secondary"
                  size="sm"
                  :disabled="containerBusy || botLifecyclePending"
                  class="h-8 text-xs shadow-none font-medium border border-border"
                  @click="handleExportData"
                >
                  {{ $t('bots.container.actions.exportData') }}
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  :disabled="containerBusy || botLifecyclePending"
                  class="h-8 text-xs shadow-none font-medium border border-border"
                  @click="triggerImportData"
                >
                  {{ $t('bots.container.actions.importData') }}
                </Button>
                <ConfirmPopover
                  :message="$t('bots.container.restoreConfirm')"
                  :loading="containerAction === 'restore'"
                  @confirm="handleRestorePreservedData"
                >
                  <template #trigger>
                    <Button
                      variant="secondary"
                      size="sm"
                      :disabled="containerBusy || botLifecyclePending || !hasPreservedData"
                      class="h-8 text-xs shadow-none font-medium border border-border"
                    >
                      <Spinner
                        v-if="containerAction === 'restore'"
                        class="mr-1.5"
                      />
                      {{ $t('bots.container.actions.restoreData') }}
                    </Button>
                  </template>
                </ConfirmPopover>
              </div>
            </div>

            <Separator class="bg-border/40" />

            <!-- Lifecycle Group -->
            <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div class="space-y-0.5">
                <h4 class="text-xs font-medium text-foreground flex items-center gap-2">
                  <Settings class="size-3.5 text-muted-foreground" />
                  {{ $t('bots.container.lifecycleTitle') }}
                </h4>
                <p class="text-[11px] text-muted-foreground leading-snug">
                  {{ $t('bots.container.deleteSubtitle') }}
                </p>
              </div>
              <div class="flex justify-end shrink-0">
                <ConfirmPopover
                  :message="$t('bots.container.deletePreserveConfirm')"
                  :loading="containerAction === 'delete-preserve'"
                  @confirm="handleDeleteContainer(true)"
                >
                  <template #trigger>
                    <Button
                      variant="secondary"
                      size="sm"
                      :disabled="containerBusy || botLifecyclePending"
                      class="h-8 text-xs shadow-none font-medium border border-border"
                    >
                      <Spinner
                        v-if="containerAction === 'delete-preserve'"
                        class="mr-1.5"
                      />
                      {{ $t('bots.container.actions.deletePreserve') }}
                    </Button>
                  </template>
                </ConfirmPopover>
              </div>
            </div>
          </div>
        </div>
        <input
          ref="importInputRef"
          type="file"
          accept=".tar.gz,.tgz,application/gzip,application/x-gzip,application/x-tar"
          class="hidden"
          @change="handleImportData"
        >
      </div>

      <!-- Danger Zone - Exact replica from ?tab=channels -->
      <div class="pt-4">
        <div class="space-y-4 rounded-md border border-border bg-background p-4 shadow-none">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div class="space-y-0.5">
              <h4 class="text-xs font-medium text-destructive">
                {{ $t('common.dangerZone') }}
              </h4>
              <p class="text-[11px] text-muted-foreground">
                {{ $t('bots.container.deleteZoneDesc') }}
              </p>
            </div>
            <div class="flex justify-end shrink-0">
              <ConfirmPopover
                :message="$t('bots.container.deleteConfirm')"
                :loading="containerAction === 'delete'"
                @confirm="handleDeleteContainer(false)"
              >
                <template #trigger>
                  <Button
                    variant="destructive"
                    size="sm"
                    :disabled="containerBusy || botLifecyclePending"
                    class="inline-flex items-center justify-center whitespace-nowrap transition-all disabled:pointer-events-none disabled:opacity-50 outline-none focus-visible:ring-2 focus-visible:ring-ring/30 cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90 rounded-lg gap-1.5 px-3 min-w-28 h-8 text-xs font-medium shadow-none"
                  >
                    <Spinner
                      v-if="containerAction === 'delete'"
                      class="mr-1.5"
                    />
                    {{ $t('bots.container.actions.delete') }}
                  </Button>
                </template>
              </ConfirmPopover>
            </div>
          </div>
        </div>
      </div>

      <Separator
        v-if="capabilitiesStore.snapshotSupported"
        class="bg-border/50"
      />

      <!-- Snapshots -->
      <div
        v-if="capabilitiesStore.snapshotSupported"
        class="space-y-4"
      >
        <div class="rounded-md border border-border/60 bg-background overflow-hidden shadow-none">
          <!-- Snapshot Management Row -->
          <div class="p-4 space-y-4">
            <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div class="space-y-0.5">
                <h4 class="text-xs font-medium text-foreground flex items-center gap-2">
                  <History class="size-3.5 text-muted-foreground" />
                  {{ $t('bots.container.snapshotTitle') }}
                </h4>
                <p class="text-[11px] text-muted-foreground leading-snug">
                  {{ $t('bots.container.snapshotSubtitle') }}
                </p>
              </div>
              <div class="flex items-center gap-2 shrink-0 sm:justify-end flex-1 max-w-md">
                <Input
                  v-model="newSnapshotName"
                  :placeholder="$t('bots.container.snapshotNamePlaceholder')"
                  :disabled="containerBusy || snapshotsLoading || botLifecyclePending"
                  class="flex-1 h-8 text-xs shadow-none border-border/60 bg-transparent"
                />
                <Button
                  size="sm"
                  :disabled="containerBusy || snapshotsLoading || botLifecyclePending"
                  class="min-w-28 h-8 text-xs shadow-none font-medium border border-border bg-accent text-foreground hover:bg-accent/80 transition-colors"
                  @click="handleCreateSnapshot"
                >
                  <Spinner
                    v-if="containerAction === 'snapshot'"
                    class="mr-1.5"
                  />
                  {{ $t('bots.container.actions.snapshot') }}
                </Button>
              </div>
            </div>
            <p class="text-[10px] text-muted-foreground/60 leading-none">
              {{ $t('bots.container.snapshotNameHint') }}
            </p>
          </div>

          <Separator class="bg-border/40" />

          <!-- List Section -->
          <div
            v-if="snapshotsLoading"
            class="flex items-center gap-2 text-xs text-muted-foreground p-4"
          >
            <Spinner /> <span>{{ $t('common.loading') }}</span>
          </div>
          <div
            v-else-if="sortedSnapshots.length === 0"
            class="text-[11px] text-muted-foreground py-8 text-center border-dashed border-t border-border/20"
          >
            {{ $t('bots.container.snapshotEmpty') }}
          </div>
          <div
            v-else
            class="divide-y divide-border/40"
          >
            <div
              v-for="item in sortedSnapshots"
              :key="`${item.snapshotter}:${item.runtime_snapshot_name || item.name}`"
              class="flex flex-col sm:flex-row sm:items-center justify-between p-3 gap-3 transition-colors hover:bg-muted/20 group"
            >
              <div class="flex-1 min-w-0 grid grid-cols-1 sm:grid-cols-12 gap-2 sm:gap-4 items-center">
                <div class="sm:col-span-4 min-w-0">
                  <div class="truncate text-xs font-medium text-foreground">
                    {{ snapshotDisplayName(item) }}
                  </div>
                  <div
                    v-if="snapshotRuntimeName(item)"
                    class="truncate text-[10px] font-mono text-muted-foreground"
                  >
                    {{ snapshotRuntimeName(item) }}
                  </div>
                </div>
                <div class="sm:col-span-2 text-[11px] text-muted-foreground">
                  <span class="inline-flex px-1.5 py-0.5 rounded bg-muted/40 border border-border/40">{{ snapshotVersionText(item) }}</span>
                </div>
                <div class="sm:col-span-2 text-[11px] text-muted-foreground">
                  {{ snapshotSourceText(item) }}
                </div>
                <div class="sm:col-span-4 text-[11px] text-muted-foreground truncate">
                  {{ formatDate(item.created_at) }}
                </div>
              </div>

              <div class="shrink-0 flex items-center justify-end">
                <ConfirmPopover
                  v-if="canRollbackSnapshot(item)"
                  :message="$t('bots.container.rollbackConfirm')"
                  :loading="containerAction === 'rollback' && rollbackVersion === item.version"
                  @confirm="handleRollbackSnapshot(item)"
                >
                  <template #trigger>
                    <Button
                      variant="ghost"
                      size="sm"
                      :disabled="containerBusy || botLifecyclePending"
                      class="h-6 px-2 text-[11px] text-muted-foreground hover:text-primary hover:bg-primary/10 shadow-none transition-colors"
                    >
                      <Spinner
                        v-if="containerAction === 'rollback' && rollbackVersion === item.version"
                        class="mr-1.5"
                      />
                      {{ $t('bots.container.actions.rollback') }}
                    </Button>
                  </template>
                </ConfirmPopover>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
