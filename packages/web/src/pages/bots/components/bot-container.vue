<script setup lang="ts">
import { computed, ref,watch } from 'vue'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { useCapabilitiesStore } from '@/store/capabilities'
import {
  getBotsByBotIdContainer,
  type HandlersGetContainerResponse,
  type HandlersListSnapshotsResponse,
  getBotsByBotIdContainerSnapshots,
  getBotsById,
  postBotsByBotIdContainer,
  postBotsByBotIdContainerStop,
  postBotsByBotIdContainerStart,
  deleteBotsByBotIdContainer,
  postBotsByBotIdContainerSnapshots
} from '@memoh/sdk'
import { useRoute } from 'vue-router'
import { useBotStatusMeta } from '@/composables/useBotStatusMeta'
import { useQuery } from '@pinia/colada'
import { formatDateTime } from '@/utils/date-time' 
import { Spinner, Button, Separator,Input } from '@memoh/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'


const route=useRoute()

const { t } = useI18n()

const containerLoading = ref(false)
const containerAction = ref<'refresh' | 'create' | 'start' | 'stop' | 'delete' | 'snapshot' | ''>('')

const containerBusy = computed(() => containerLoading.value || containerAction.value !== '')

function resolveErrorMessage(error: unknown, fallback: string): string {
  return resolveApiErrorMessage(error, fallback)
}

async function runContainerAction(
  action: 'refresh' | 'create' | 'start' | 'stop' | 'delete' | 'snapshot',
  operation: () => Promise<void>,
  successMessage: string,
) {
  containerAction.value = action
  try {
    await operation()
    if (successMessage) {
      toast.success(successMessage)
    }
  } catch (error) {
    toast.error(resolveErrorMessage(error, t('bots.container.actionFailed')))
  } finally {
    containerAction.value = ''
  }
}


const capabilitiesStore = useCapabilitiesStore()
const botId = computed(() => route.params.botId as string)

type BotContainerInfo = HandlersGetContainerResponse
const containerInfo = ref<BotContainerInfo | null>(null)
const containerMissing = ref(false)

type BotContainerSnapshot = HandlersListSnapshotsResponse extends { snapshots?: (infer T)[] } ? T : never
const snapshots = ref<BotContainerSnapshot[]>([])
async function loadContainerData(showLoadingToast: boolean) {
  await capabilitiesStore.load()
  containerLoading.value = true
  try {
    const result = await getBotsByBotIdContainer({ path: { bot_id: botId.value } })
    if (result.error !== undefined) {
      if (result.response.status === 404) {
        containerInfo.value = null
        containerMissing.value = true
        snapshots.value = []
        return
      }
      throw result.error
    }
    containerInfo.value = result.data
    containerMissing.value = false
    if (capabilitiesStore.snapshotSupported) {
      await loadSnapshots()
    }
  } catch (error) {
    if (showLoadingToast) {
      toast.error(resolveErrorMessage(error, t('bots.container.loadFailed')))
    }
  } finally {
    containerLoading.value = false
  }
}

const snapshotsLoading = ref(false)

async function loadSnapshots() {
  if (!containerInfo.value || !capabilitiesStore.snapshotSupported) {
    snapshots.value = []
    return
  }
  snapshotsLoading.value = true
  try {
    const { data } = await getBotsByBotIdContainerSnapshots({ path: { bot_id: botId.value }, throwOnError: true })
    snapshots.value = data.snapshots ?? []
  } catch (error) {
    snapshots.value = []
    toast.error(resolveErrorMessage(error, t('bots.container.snapshotLoadFailed')))
  } finally {
    snapshotsLoading.value = false
  }
}


async function handleRefreshContainer() {
  await runContainerAction(
    'refresh',
    async () => {
      await loadContainerData(false)
    },
    '',
  )
}

const { data: bot } = useQuery({
  key: () => ['bot', botId.value],
  query: async () => {
    const { data } = await getBotsById({ path: { id: botId.value }, throwOnError: true })
    return data
  },
  enabled: () => !!botId.value,
})


const {
  isPending: botLifecyclePending,
} = useBotStatusMeta(bot, t)

async function handleCreateContainer() {
  if (botLifecyclePending.value) return
  await runContainerAction(
    'create',
    async () => {
      await postBotsByBotIdContainer({ path: { bot_id: botId.value }, body: {}, throwOnError: true })
      await loadContainerData(false)
    },
    t('bots.container.createSuccess'),
  )
}

const isContainerTaskRunning = computed(() => {
  const info = containerInfo.value
  if (!info) return false
  const status = (info.status ?? '').trim().toLowerCase()
  if (status === 'stopped' || status === 'exited') return false
  return info.task_running
})

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

async function handleDeleteContainer() {
  if (botLifecyclePending.value || !containerInfo.value) return
  await runContainerAction(
    'delete',
    async () => {
      await deleteBotsByBotIdContainer({ path: { bot_id: botId.value }, throwOnError: true })
      containerInfo.value = null
      containerMissing.value = true
      snapshots.value = []
    },
    t('bots.container.deleteSuccess'),
  )
}

const statusKeyMap: Record<string, string> = {
  created: 'statusCreated',
  running: 'statusRunning',
  stopped: 'statusStopped',
  exited: 'statusExited',
}

const containerStatusText = computed(() => {
  const s = (containerInfo.value?.status ?? '').trim().toLowerCase()
  const key = statusKeyMap[s] ?? 'statusUnknown'
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

const newSnapshotName = ref('')

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
  const copied = [...snapshots.value]
  copied.sort((a, b) => {
    const left = Date.parse(a.created_at ?? '')
    const right = Date.parse(b.created_at ?? '')
    if (Number.isNaN(left) && Number.isNaN(right)) {
      return (a.name ?? '').localeCompare(b.name ?? '')
    }
    if (Number.isNaN(left)) return 1
    if (Number.isNaN(right)) return -1
    return right - left
  })
  return copied
})


const activeTab = useSyncedQueryParam('tab', 'overview')

watch([activeTab, botId], ([tab]) => {
  if (!botId.value) {
    return
  }
  if (tab === 'container') {
    void loadContainerData(true)
    return
  }
 
}, { immediate: true })
</script>

<template>
  <div class=" mx-auto space-y-5">
    <div class="flex items-start justify-between gap-3">
      <div class="space-y-1 min-w-0">
        <h3 class="text-lg font-semibold">
          {{ $t('bots.container.title') }}
        </h3>
        <p class="text-sm text-muted-foreground">
          {{ $t('bots.container.subtitle') }}
        </p>
      </div>
      <div class="flex flex-wrap gap-2 shrink-0 justify-end">
        <Button
          variant="outline"
          size="sm"
          :disabled="containerBusy"
          @click="handleRefreshContainer"
        >
          <Spinner
            v-if="containerLoading || containerAction === 'refresh'"
            class="mr-1.5"
          />
          {{ $t('common.refresh') }}
        </Button>
        <Button
          v-if="containerMissing"
          :disabled="containerBusy || botLifecyclePending"
          @click="handleCreateContainer"
        >
          <Spinner
            v-if="containerAction === 'create'"
            class="mr-1.5"
          />
          {{ $t('bots.container.actions.create') }}
        </Button>
        <template v-if="containerInfo">
          <Button
            variant="secondary"
            size="sm"
            :disabled="containerBusy || botLifecyclePending"
            @click="isContainerTaskRunning ? handleStopContainer() : handleStartContainer()"
          >
            <Spinner
              v-if="containerAction === 'start' || containerAction === 'stop'"
              class="mr-1.5"
            />
            {{ isContainerTaskRunning ? $t('bots.container.actions.stop') : $t('bots.container.actions.start') }}
          </Button>
          <ConfirmPopover
            :message="$t('bots.container.deleteConfirm')"
            :loading="containerAction === 'delete'"
            @confirm="handleDeleteContainer"
          >
            <template #trigger>
              <Button
                variant="destructive"
                size="sm"
                :disabled="containerBusy || botLifecyclePending"
              >
                <Spinner
                  v-if="containerAction === 'delete'"
                  class="mr-1.5"
                />
                {{ $t('bots.container.actions.delete') }}
              </Button>
            </template>
          </ConfirmPopover>
        </template>
      </div>
    </div>

    <div
      v-if="botLifecyclePending"
      class="rounded-md border border-yellow-300/50 bg-yellow-50/70 p-3 text-sm text-yellow-800 dark:border-yellow-800/50 dark:bg-yellow-900/10 dark:text-yellow-200"
    >
      {{ $t('bots.container.botNotReady') }}
    </div>

    <div
      v-if="containerLoading && !containerInfo && !containerMissing"
      class="flex items-center gap-2 text-sm text-muted-foreground"
    >
      <Spinner />
      <span>{{ $t('common.loading') }}</span>
    </div>

    <div
      v-else-if="containerMissing"
      class="rounded-md border p-4"
    >
      <p class="text-sm text-muted-foreground">
        {{ $t('bots.container.empty') }}
      </p>
    </div>

    <div
      v-else-if="containerInfo"
      class="space-y-5"
    >
      <div class="rounded-md border p-4">
        <dl class="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
          <div class="space-y-1">
            <dt class="text-muted-foreground">
              {{ $t('bots.container.fields.id') }}
            </dt>
            <dd class="font-mono break-all">
              {{ containerInfo.container_id }}
            </dd>
          </div>
          <div class="space-y-1">
            <dt class="text-muted-foreground">
              {{ $t('bots.container.fields.status') }}
            </dt>
            <dd>{{ containerStatusText }}</dd>
          </div>
          <div class="space-y-1">
            <dt class="text-muted-foreground">
              {{ $t('bots.container.fields.task') }}
            </dt>
            <dd>{{ containerTaskText }}</dd>
          </div>
          <div class="space-y-1">
            <dt class="text-muted-foreground">
              {{ $t('bots.container.fields.namespace') }}
            </dt>
            <dd>{{ containerInfo.namespace }}</dd>
          </div>
          <div class="space-y-1 sm:col-span-2">
            <dt class="text-muted-foreground">
              {{ $t('bots.container.fields.image') }}
            </dt>
            <dd class="break-all">
              {{ containerInfo.image }}
            </dd>
          </div>
          <div class="space-y-1 sm:col-span-2">
            <dt class="text-muted-foreground">
              {{ $t('bots.container.fields.hostPath') }}
            </dt>
            <dd class="break-all">
              {{ containerInfo.host_path || '-' }}
            </dd>
          </div>
          <div class="space-y-1 sm:col-span-2">
            <dt class="text-muted-foreground">
              {{ $t('bots.container.fields.containerPath') }}
            </dt>
            <dd class="break-all">
              {{ containerInfo.container_path }}
            </dd>
          </div>
          <div class="space-y-1">
            <dt class="text-muted-foreground">
              {{ $t('bots.container.fields.createdAt') }}
            </dt>
            <dd>{{ formatDate(containerInfo.created_at) }}</dd>
          </div>
          <div class="space-y-1">
            <dt class="text-muted-foreground">
              {{ $t('bots.container.fields.updatedAt') }}
            </dt>
            <dd>{{ formatDate(containerInfo.updated_at) }}</dd>
          </div>
        </dl>
      </div>

      <Separator v-if="capabilitiesStore.snapshotSupported" />

      <div
        v-if="capabilitiesStore.snapshotSupported"
        class="space-y-3"
      >
        <div class="flex flex-col gap-2 sm:flex-row">
          <Input
            v-model="newSnapshotName"
            :placeholder="$t('bots.container.snapshotNamePlaceholder')"
            :disabled="containerBusy || snapshotsLoading || botLifecyclePending"
            class="max-w-50"
          />
          <Button
            :disabled="containerBusy || snapshotsLoading || botLifecyclePending"
            @click="handleCreateSnapshot"
          >
            <Spinner
              v-if="containerAction === 'snapshot'"
              class="mr-1.5"
            />
            {{ $t('bots.container.actions.snapshot') }}
          </Button>
        </div>

        <div
          v-if="snapshotsLoading"
          class="flex items-center gap-2 text-sm text-muted-foreground"
        >
          <Spinner />
          <span>{{ $t('common.loading') }}</span>
        </div>
        <div
          v-else-if="sortedSnapshots.length === 0"
          class="text-sm text-muted-foreground"
        >
          {{ $t('bots.container.snapshotEmpty') }}
        </div>
        <div
          v-else
          class="overflow-x-auto rounded-md border"
        >
          <table class="w-full text-sm">
            <thead class="bg-muted/50 text-left">
              <tr>
                <th class="px-3 py-2 font-medium">
                  {{ $t('bots.container.snapshotColumns.name') }}
                </th>
                <th class="px-3 py-2 font-medium">
                  {{ $t('bots.container.snapshotColumns.kind') }}
                </th>
                <th class="px-3 py-2 font-medium">
                  {{ $t('bots.container.snapshotColumns.parent') }}
                </th>
                <th class="px-3 py-2 font-medium">
                  {{ $t('bots.container.snapshotColumns.createdAt') }}
                </th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="item in sortedSnapshots"
                :key="`${item.snapshotter}:${item.name}`"
                class="border-t"
              >
                <td class="px-3 py-2 font-mono text-xs break-all">
                  {{ item.name }}
                </td>
                <td class="px-3 py-2">
                  {{ item.kind }}
                </td>
                <td class="px-3 py-2 break-all">
                  {{ item.parent || '-' }}
                </td>
                <td class="px-3 py-2">
                  {{ formatDate(item.created_at) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
</template>