<template>
  <section class="p-4 max-w-7xl mx-auto">
    <!-- Header -->
    <div class="flex items-center gap-4 mb-8">
      <div
        class="group/avatar relative size-16 shrink-0 rounded-full overflow-hidden"
      >
        <Avatar class="size-16 rounded-full">
          <AvatarImage
            v-if="bot?.avatar_url"
            :src="bot.avatar_url"
            :alt="bot.display_name"
          />
          <AvatarFallback class="text-xl">
            {{ avatarFallback }}
          </AvatarFallback>
        </Avatar>
        <button
          type="button"
          class="absolute inset-0 flex items-center justify-center rounded-full bg-black/40 opacity-0 transition-opacity group-hover/avatar:opacity-100"
          :title="$t('common.edit')"
          :aria-label="$t('common.edit')"
          :disabled="!bot || botLifecyclePending"
          @click="handleEditAvatar"
        >
          <FontAwesomeIcon
            :icon="['fas', 'pen-to-square']"
            class="size-6 text-white"
          />
        </button>
      </div>
      <div class="min-w-0">
        <div class="flex items-center gap-2">
          <template v-if="isEditingBotName && bot">
            <Input
              v-model="botNameDraft"
              class="h-9 max-w-xl"
              :placeholder="$t('bots.displayNamePlaceholder')"
              :disabled="isSavingBotName"
              @keydown.enter.prevent="handleConfirmBotName"
              @keydown.esc.prevent="handleCancelBotName"
            />
            <LoadingButton
              size="sm"
              :loading="isSavingBotName"
              :disabled="!canConfirmBotName"
              @click="handleConfirmBotName"
            >
              {{ $t('common.confirm') }}
            </LoadingButton>
            <Button
              variant="ghost"
              size="sm"
              :disabled="isSavingBotName"
              @click="handleCancelBotName"
            >
              {{ $t('common.cancel') }}
            </Button>
          </template>
          <template v-else>
            <h2 class="truncate text-2xl font-semibold tracking-tight">
              {{ botNameDraft.trim() || bot?.display_name || botId }}
            </h2>
            <Button
              v-if="bot"
              type="button"
              variant="ghost"
              size="sm"
              class="size-7 p-0"
              :disabled="botLifecyclePending"
              :title="$t('common.edit')"
              :aria-label="$t('common.edit')"
              @click="handleStartEditBotName"
            >
              <FontAwesomeIcon
                :icon="['fas', 'pen-to-square']"
                class="size-3.5"
              />
            </Button>
          </template>
        </div>
        <div class="mt-1 flex items-center gap-2 text-sm text-muted-foreground">
          <Badge
            v-if="bot"
            :variant="statusVariant"
            class="text-xs"
            :title="hasIssue ? issueTitle : undefined"
          >
            <FontAwesomeIcon
              v-if="bot.status === 'creating' || bot.status === 'deleting'"
              :icon="['fas', 'spinner']"
              class="mr-1 size-3 animate-spin"
            />
            {{ statusLabel }}
          </Badge>
          <span v-if="bot?.type">{{ botTypeLabel }}</span>
        </div>
      </div>
    </div>

    <!-- Tabs -->
    <Tabs
      v-model="activeTab"
      class="w-full"
    >
      <TabsList class="w-full justify-start">
        <TabsTrigger value="overview">
          {{ $t('bots.tabs.overview') }}
        </TabsTrigger>
        <TabsTrigger value="memory">
          {{ $t('bots.tabs.memory') }}
        </TabsTrigger>
        <TabsTrigger value="channels">
          {{ $t('bots.tabs.channels') }}
        </TabsTrigger>
        <TabsTrigger value="container">
          {{ $t('bots.tabs.container') }}
        </TabsTrigger>
        <TabsTrigger value="mcp">
          {{ $t('bots.tabs.mcp') }}
        </TabsTrigger>
        <TabsTrigger value="subagents">
          {{ $t('bots.tabs.subagents') }}
        </TabsTrigger>
        <TabsTrigger value="heartbeat">
          {{ $t('bots.tabs.heartbeat') }}
        </TabsTrigger>
        <TabsTrigger value="history">
          {{ $t('bots.tabs.history') }}
        </TabsTrigger>
        <TabsTrigger value="skills">
          {{ $t('bots.tabs.skills') }}
        </TabsTrigger>
        <TabsTrigger value="settings">
          {{ $t('bots.tabs.settings') }}
        </TabsTrigger>
      </TabsList>

      <TabsContent
        value="overview"
        class="mt-6"
      >
        <div class="max-w-4xl mx-auto">
          <div class="rounded-md border p-4">
            <div class="flex items-center justify-between gap-2">
              <div>
                <p class="text-sm font-medium">
                  {{ $t('bots.checks.title') }}
                </p>
                <p class="text-sm text-muted-foreground">
                  {{ $t('bots.checks.subtitle') }}
                </p>
              </div>
              <LoadingButton
                variant="outline"
                size="sm"
                :loading="checksLoading"
                @click="handleRefreshChecks"
              >
                {{ $t('common.refresh') }}
              </LoadingButton>
            </div>
            <div class="mt-3 flex items-center gap-2 text-sm">
              <Badge
                :variant="hasIssue ? 'destructive' : 'default'"
                class="text-xs"
              >
                {{ checksSummaryText }}
              </Badge>
            </div>

            <div
              v-if="checksLoading && checks.length === 0"
              class="mt-4 flex items-center gap-2 text-sm text-muted-foreground"
            >
              <Spinner />
              <span>{{ $t('common.loading') }}</span>
            </div>

            <p
              v-else-if="checks.length === 0"
              class="mt-4 text-sm text-muted-foreground"
            >
              {{ $t('bots.checks.empty') }}
            </p>

            <ul
              v-else
              class="mt-4 divide-y"
            >
              <li
                v-for="item in checks"
                :key="item.id"
                class="py-3 first:pt-0 last:pb-0"
              >
                <div class="flex items-center justify-between gap-2">
                  <div class="min-w-0">
                    <p class="font-mono text-xs">
                      {{ checkTitleLabel(item) }}
                    </p>
                    <p
                      v-if="item.subtitle"
                      class="mt-0.5 text-xs text-muted-foreground"
                    >
                      {{ item.subtitle }}
                    </p>
                  </div>
                  <Badge
                    :variant="checkStatusVariant(item.status)"
                    class="text-[10px]"
                  >
                    {{ checkStatusLabel(item.status) }}
                  </Badge>
                </div>
                <p class="mt-2 text-sm">
                  {{ item.summary }}
                </p>
                <p
                  v-if="item.detail"
                  class="mt-1 text-xs text-muted-foreground break-all"
                >
                  {{ item.detail }}
                </p>
              </li>
            </ul>
          </div>
        </div>
      </TabsContent>
      <TabsContent
        value="memory"
        class="mt-6"
      >
        <BotMemory :bot-id="botId" />
      </TabsContent>
      <TabsContent
        value="channels"
        class="mt-6"
      >
        <BotChannels :bot-id="botId" />
      </TabsContent>
      <TabsContent
        value="container"
        class="mt-6"
      >
        <div class="max-w-4xl mx-auto space-y-5">
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

          <WarningBanner v-if="botLifecyclePending">
            {{ $t('bots.container.botNotReady') }}
          </WarningBanner>

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
      </TabsContent>
      <TabsContent
        value="mcp"
        class="mt-6"
      >
        <BotMcp :bot-id="botId" />
      </TabsContent>
      <TabsContent
        value="subagents"
        class="mt-6"
      >
        <BotSubagents :bot-id="botId" />
      </TabsContent>
      <TabsContent
        value="history"
        class="mt-6"
      >
        <BotHistory :bot-id="botId" />
      </TabsContent>
      <TabsContent
        value="heartbeat"
        class="mt-6"
      >
        <BotHeartbeat :bot-id="botId" />
      </TabsContent>
      <TabsContent
        value="skills"
        class="mt-6"
      >
        <BotSkills :bot-id="botId" />
      </TabsContent>
      <TabsContent
        value="settings"
        class="mt-6"
      >
        <BotSettings
          :bot-id="botId"
          :bot-type="bot?.type"
        />
      </TabsContent>
    </Tabs>

    <!-- Edit avatar dialog -->
    <Dialog v-model:open="avatarDialogOpen">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ $t('bots.editAvatar') }}</DialogTitle>
          <DialogDescription>
            {{ $t('bots.editAvatarDescription') }}
          </DialogDescription>
        </DialogHeader>
        <div class="mt-4 flex flex-col items-center gap-4">
          <Avatar class="size-20 shrink-0 rounded-full">
            <AvatarImage
              v-if="avatarUrlDraft.trim()"
              :src="avatarUrlDraft.trim()"
              :alt="bot?.display_name"
            />
            <AvatarFallback class="text-xl">
              {{ avatarFallback }}
            </AvatarFallback>
          </Avatar>
          <Input
            v-model="avatarUrlDraft"
            type="url"
            class="w-full"
            :placeholder="$t('bots.avatarUrlPlaceholder')"
            :disabled="avatarSaving"
          />
        </div>
        <DialogFooter class="mt-6">
          <DialogClose as-child>
            <Button
              variant="outline"
              :disabled="avatarSaving"
            >
              {{ $t('common.cancel') }}
            </Button>
          </DialogClose>
          <Button
            :disabled="avatarSaving || !canConfirmAvatar"
            @click="handleConfirmAvatar"
          >
            <Spinner
              v-if="avatarSaving"
              class="mr-1.5"
            />
            {{ $t('common.confirm') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </section>
</template>

<script setup lang="ts">
import {
  Avatar,
  AvatarImage,
  AvatarFallback,
  Badge,
  Button,
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Separator,
  Spinner,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
} from '@memoh/ui'
import { computed, ref, watch, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import { useQuery, useMutation, useQueryCache } from '@pinia/colada'
import {
  getBotsById, putBotsById,
  getBotsByIdChecks,
  getBotsByBotIdContainer, postBotsByBotIdContainer, deleteBotsByBotIdContainer,
  postBotsByBotIdContainerStart, postBotsByBotIdContainerStop,
  getBotsByBotIdContainerSnapshots, postBotsByBotIdContainerSnapshots,
} from '@memoh/sdk'
import type {
  BotsBotCheck, HandlersGetContainerResponse,
  HandlersListSnapshotsResponse,
} from '@memoh/sdk'
import { useCapabilitiesStore } from '@/store/capabilities'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import WarningBanner from '@/components/warning-banner/index.vue'
import LoadingButton from '@/components/loading-button/index.vue'
import BotSettings from './components/bot-settings.vue'
import BotChannels from './components/bot-channels.vue'
import BotMcp from './components/bot-mcp.vue'
import BotMemory from './components/bot-memory.vue'
import BotSkills from './components/bot-skills.vue'
import BotHistory from './components/bot-history.vue'
import BotHeartbeat from './components/bot-heartbeat.vue'
import BotSubagents from './components/bot-subagents.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { formatDateTime } from '@/utils/date-time'
import { useAvatarInitials } from '@/composables/useAvatarInitials'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'
import { useBotStatusMeta } from '@/composables/useBotStatusMeta'

type BotCheck = BotsBotCheck
type BotContainerInfo = HandlersGetContainerResponse
type BotContainerSnapshot = HandlersListSnapshotsResponse extends { snapshots?: (infer T)[] } ? T : never

const route = useRoute()
const { t } = useI18n()
const botId = computed(() => route.params.botId as string)

const capabilitiesStore = useCapabilitiesStore()
onMounted(() => {
  void capabilitiesStore.load()
})

const { data: bot } = useQuery({
  key: () => ['bot', botId.value],
  query: async () => {
    const { data } = await getBotsById({ path: { id: botId.value }, throwOnError: true })
    return data
  },
  enabled: () => !!botId.value,
})

const queryCache = useQueryCache()
const { mutateAsync: updateBot, isLoading: updateBotLoading } = useMutation({
  mutation: async ({ id, ...body }: Record<string, unknown> & { id: string }) => {
    const { data } = await putBotsById({ path: { id }, body: body as any, throwOnError: true })
    return data
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ['bots'] })
    queryCache.invalidateQueries({ key: ['bot'] })
  },
})

async function fetchChecks(id: string): Promise<BotCheck[]> {
  const { data } = await getBotsByIdChecks({ path: { id }, throwOnError: true })
  return data?.items ?? []
}

const isEditingBotName = ref(false)
const botNameDraft = ref('')

// Replace breadcrumb bot id with display name when available.
watch(bot, (val) => {
  if (!val) return
  const currentName = (val.display_name || '').trim()
  if (currentName) {
    route.meta.breadcrumb = () => currentName
  }
  if (!isEditingBotName.value) {
    botNameDraft.value = val.display_name || ''
  }
}, { immediate: true })

const activeTab = useSyncedQueryParam('tab', 'overview')
const avatarDialogOpen = ref(false)
const avatarUrlDraft = ref('')

const avatarFallback = useAvatarInitials(() => bot.value?.display_name || botId.value || '')
const isSavingBotName = computed(() => updateBotLoading.value)
const avatarSaving = computed(() => updateBotLoading.value)
const canConfirmAvatar = computed(() => {
  if (!bot.value) return false
  const next = avatarUrlDraft.value.trim()
  const current = (bot.value.avatar_url || '').trim()
  return next !== current
})
const canConfirmBotName = computed(() => {
  if (!bot.value) return false
  const nextName = botNameDraft.value.trim()
  if (!nextName) return false
  return nextName !== (bot.value.display_name || '').trim()
})
const {
  hasIssue,
  isPending: botLifecyclePending,
  issueTitle,
  statusLabel,
  statusVariant,
} = useBotStatusMeta(bot, t)

const botTypeLabel = computed(() => {
  const type = bot.value?.type
  if (type === 'personal' || type === 'public') return t('bots.types.' + type)
  return type ?? ''
})

const checks = ref<BotCheck[]>([])
const checksLoading = ref(false)
const checksSummaryText = computed(() => {
  const issueCount = checks.value.filter((item) => item.status === 'warn' || item.status === 'error').length
  if (issueCount > 0) {
    return t('bots.checks.issueCount', { count: issueCount })
  }
  if (checks.value.length === 0) {
    return t('bots.checks.empty')
  }
  return t('bots.checks.ok')
})

const containerInfo = ref<BotContainerInfo | null>(null)
const containerMissing = ref(false)
const containerLoading = ref(false)
const snapshotsLoading = ref(false)
const containerAction = ref<'refresh' | 'create' | 'start' | 'stop' | 'delete' | 'snapshot' | ''>('')
const newSnapshotName = ref('')
const snapshots = ref<BotContainerSnapshot[]>([])

const containerBusy = computed(() => containerLoading.value || containerAction.value !== '')
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
const isContainerTaskRunning = computed(() => {
  const info = containerInfo.value
  if (!info) return false
  const status = (info.status ?? '').trim().toLowerCase()
  if (status === 'stopped' || status === 'exited') return false
  return info.task_running
})
const containerTaskText = computed(() => {
  const info = containerInfo.value
  if (!info) return '-'
  const status = (info.status ?? '').trim().toLowerCase()
  if (status === 'exited') return t('bots.container.taskCompleted')
  return info.task_running ? t('bots.container.taskRunning') : t('bots.container.taskStopped')
})

watch(botId, () => {
  isEditingBotName.value = false
  botNameDraft.value = ''
})

watch([activeTab, botId], ([tab]) => {
  if (!botId.value) {
    return
  }
  if (tab === 'container') {
    void loadContainerData(true)
    return
  }
  if (tab === 'overview') {
    void loadChecks(true)
  }
}, { immediate: true })

function formatDate(value: string | undefined): string {
  return formatDateTime(value, { fallback: '-' })
}

function resolveErrorMessage(error: unknown, fallback: string): string {
  return resolveApiErrorMessage(error, fallback)
}

function handleEditAvatar() {
  if (!bot.value || botLifecyclePending.value) return
  avatarUrlDraft.value = bot.value.avatar_url || ''
  avatarDialogOpen.value = true
}

async function handleConfirmAvatar() {
  if (!bot.value || !canConfirmAvatar.value || avatarSaving.value) return
  const nextUrl = avatarUrlDraft.value.trim()
  try {
    await updateBot({
      id: bot.value.id,
      avatar_url: nextUrl || undefined,
    })
    avatarDialogOpen.value = false
    toast.success(t('bots.avatarUpdateSuccess'))
  } catch (error) {
    toast.error(resolveErrorMessage(error, t('bots.avatarUpdateFailed')))
  }
}

function handleStartEditBotName() {
  if (!bot.value) return
  isEditingBotName.value = true
  botNameDraft.value = bot.value.display_name || ''
}

function handleCancelBotName() {
  isEditingBotName.value = false
  botNameDraft.value = bot.value?.display_name || ''
}

async function handleConfirmBotName() {
  if (!bot.value || !canConfirmBotName.value) {
    handleCancelBotName()
    return
  }
  const nextName = botNameDraft.value.trim()
  try {
    await updateBot({
      id: bot.value.id,
      display_name: nextName,
    })
    route.meta.breadcrumb = () => nextName
    isEditingBotName.value = false
    toast.success(t('bots.renameSuccess'))
  } catch (error) {
    toast.error(resolveErrorMessage(error, t('bots.renameFailed')))
  }
}

function checkStatusVariant(status: BotCheck['status']): 'default' | 'secondary' | 'destructive' {
  if (status === 'error') return 'destructive'
  if (status === 'warn') return 'secondary'
  if (status === 'unknown') return 'secondary'
  return 'default'
}

function checkStatusLabel(status: BotCheck['status']): string {
  if (status === 'error') return t('bots.checks.status.error')
  if (status === 'warn') return t('bots.checks.status.warn')
  if (status === 'unknown') return t('bots.checks.status.unknown')
  return t('bots.checks.status.ok')
}

function checkTitleLabel(item: BotCheck): string {
  const titleKey = (item.title_key ?? '').trim()
  if (titleKey) {
    const translated = t(titleKey)
    if (translated !== titleKey) {
      return translated
    }
  }
  return (item.type ?? '').trim() || (item.id ?? '').trim() || '-'
}

async function loadChecks(showToast: boolean) {
  checksLoading.value = true
  checks.value = []
  try {
    checks.value = await fetchChecks(botId.value)
  } catch (error) {
    if (showToast) {
      toast.error(resolveErrorMessage(error, t('bots.checks.loadFailed')))
    }
  } finally {
    checksLoading.value = false
  }
}

async function handleRefreshChecks() {
  await loadChecks(true)
}

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

async function handleRefreshContainer() {
  await runContainerAction(
    'refresh',
    async () => {
      await loadContainerData(false)
    },
    '',
  )
}

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
</script>