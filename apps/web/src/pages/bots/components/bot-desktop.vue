<template>
  <PageShell
    variant="tab"
    :title="$t('bots.desktop.title', 'Desktop Environment')"
    :description="$t('bots.desktop.subtitle', 'Manage virtual display and interaction sessions.')"
  >
    <template #actions>
      <Button
        variant="outline"
        size="sm"
        class="shrink-0"
        :disabled="isRefreshing || isSessionsFetching || isDisplayFetching"
        @click="refetchAll"
      >
        <Spinner
          v-if="isRefreshing || isSessionsFetching || isDisplayFetching"
          class="size-3"
        />
        <RefreshCw
          v-else
          class="size-4"
        />
        {{ $t('common.refresh') }}
      </Button>
    </template>

    <div class="space-y-8">
      <SettingsSection :title="$t('bots.settings.desktopEnabled')">
        <SettingsRow
          :label="$t('bots.settings.desktopEnabled')"
          :description="$t('bots.settings.desktopEnabledDescription')"
        >
          <Switch
            :model-value="settingsForm.display_enabled"
            :disabled="isSaving"
            @update:model-value="(val) => handleToggleDisplay(!!val)"
          />
        </SettingsRow>
      </SettingsSection>

      <section class="space-y-2.5">
        <div class="flex items-center gap-2 px-2">
          <h2 class="text-[13px] font-medium text-muted-foreground">
            {{ $t('bots.desktop.runtimeTitle') }}
          </h2>
          <span
            class="size-1.5 rounded-sm"
            :class="statusColorClass"
          />
          <span class="text-xs text-muted-foreground">
            {{ runtimeSummary }}
          </span>
        </div>

        <div class="grid gap-px overflow-hidden rounded-[var(--radius-menu-shell)] border border-border bg-border sm:grid-cols-3">
          <div
            v-for="group in runtimeGroups"
            :key="group.title"
            class="bg-card p-4"
          >
            <div class="mb-3 flex items-center gap-2 text-sm font-medium text-foreground">
              <component
                :is="group.icon"
                class="size-4 text-muted-foreground"
              />
              {{ group.title }}
            </div>
            <div class="space-y-2">
              <div
                v-for="item in group.items"
                :key="item.key"
                class="flex items-center justify-between gap-3 text-xs"
              >
                <span class="min-w-0 text-muted-foreground">{{ item.label }}</span>
                <span
                  v-if="item.isBoolean"
                  class="size-1.5 shrink-0 rounded-sm"
                  :class="!info.enabled ? 'bg-muted-foreground/40' : (item.ok ? 'bg-success' : 'bg-destructive')"
                />
                <span
                  v-else
                  class="truncate text-right font-medium text-foreground"
                  :title="item.value"
                >
                  {{ item.value }}
                </span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section class="space-y-2.5">
        <h2 class="px-2 text-[13px] font-medium text-muted-foreground">
          {{ $t('bots.desktop.liveTitle') }}
        </h2>
        <div class="relative flex aspect-[4/3] w-full overflow-hidden rounded-[var(--radius-menu-shell)] border border-border bg-foreground">
          <Empty
            v-if="!info.enabled"
            class="absolute inset-0 border-0 text-muted-foreground"
          >
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <MonitorOff />
              </EmptyMedia>
              <EmptyTitle>{{ $t('bots.desktop.summaryDisabled') }}</EmptyTitle>
            </EmptyHeader>
          </Empty>
          <Empty
            v-else-if="!info.running"
            class="absolute inset-0 border-0 text-muted-foreground"
          >
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Monitor />
              </EmptyMedia>
              <EmptyTitle>{{ runtimeSummary }}</EmptyTitle>
            </EmptyHeader>
          </Empty>

          <DisplayPane
            v-if="props.botId && info.running"
            :bot-id="props.botId"
            tab-id="settings-desktop"
            :title="$t('bots.desktop.liveTitle')"
            active
            :closable="false"
            class="z-0 size-full"
            @snapshot="handleSnapshot"
          />
        </div>
      </section>

      <SettingsSection :title="$t('bots.desktop.previewTitle')">
        <div
          v-if="isSessionsFetching && !previewItems.length"
          class="grid gap-px bg-border sm:grid-cols-2"
        >
          <Skeleton
            v-for="i in 4"
            :key="i"
            class="aspect-video rounded-none bg-card"
          />
        </div>

        <div
          v-else-if="previewItems.length"
          class="grid gap-px bg-border sm:grid-cols-2"
        >
          <div
            v-for="item in previewItems"
            :key="item.key"
            class="bg-card p-4"
          >
            <div class="overflow-hidden rounded-md border border-border bg-foreground">
              <img
                v-if="item.snapshot"
                :src="item.snapshot"
                :alt="item.title"
                class="aspect-video w-full object-cover"
              >
              <div
                v-else
                class="flex aspect-video items-center justify-center text-xs text-muted-foreground"
              >
                {{ $t('bots.desktop.previewEmpty') }}
              </div>
            </div>
            <div class="mt-3 flex items-center justify-between gap-3">
              <div class="min-w-0">
                <p class="truncate text-xs font-medium text-foreground">
                  {{ item.sessionId || item.key }}
                </p>
                <p class="mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground">
                  <span
                    class="size-1.5 rounded-sm"
                    :class="item.state === 'connected' ? 'bg-success' : 'bg-muted-foreground/40'"
                  />
                  {{ item.state || $t('bots.desktop.previewEmpty') }}
                </p>
              </div>
              <ConfirmPopover
                v-if="item.sessionId"
                :message="$t('bots.desktop.closeSessionConfirm', 'Terminate session?')"
                :loading="closingSessionId === item.sessionId"
                @confirm="handleCloseSession(item.sessionId)"
              >
                <template #trigger>
                  <Button
                    variant="destructive"
                    size="sm"
                    :disabled="closingSessionId === item.sessionId"
                  >
                    <Spinner
                      v-if="closingSessionId === item.sessionId"
                      class="size-3"
                    />
                    {{ $t('bots.desktop.closeSession', 'Terminate') }}
                  </Button>
                </template>
              </ConfirmPopover>
            </div>
          </div>
        </div>

        <Empty
          v-else
          class="m-4 rounded-[var(--radius-menu-shell)] border border-dashed border-border py-12"
        >
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Monitor />
            </EmptyMedia>
            <EmptyTitle>{{ $t('bots.desktop.noSessions') }}</EmptyTitle>
          </EmptyHeader>
        </Empty>
      </SettingsSection>
    </div>
  </PageShell>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from '@memohai/ui'
import { storeToRefs } from 'pinia'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import {
  Button,
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  Skeleton,
  Spinner,
  Switch,
} from '@memohai/ui'
import { RefreshCw, Monitor, MonitorOff, Layers, Box, Puzzle } from 'lucide-vue-next'
import {
  deleteBotsByBotIdContainerDisplaySessionsBySessionId,
  getBotsByBotIdContainerDisplay,
  getBotsByBotIdContainerDisplaySessions,
  getBotsByBotIdSettings,
  putBotsByBotIdSettings,
  type DisplaySessionInfo,
  type HandlersDisplayInfoResponse,
  type SettingsSettings,
} from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import DisplayPane from '@/pages/home/components/display-pane.vue'
import { useDisplaySnapshotsStore } from '@/store/display-snapshots'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import PageShell from '@/components/page-shell/index.vue'

const props = defineProps<{
  botId: string
}>()

interface PreviewItem {
  key: string
  sessionId?: string
  title: string
  state?: string
  snapshot?: string
}

const { t } = useI18n()
const queryCache = useQueryCache()
const displaySnapshots = useDisplaySnapshotsStore()
const { items: snapshotItems } = storeToRefs(displaySnapshots)

const settingsForm = reactive({
  display_enabled: false,
})
const isRefreshing = ref(false)
const closingSessionId = ref('')
const liveSessionId = ref('')

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

const {
  data: displayInfo,
  refetch: refetchDisplay,
  isFetching: isDisplayFetching,
} = useQuery({
  key: () => ['bot-display-info', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdContainerDisplay({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId,
  refetchOnWindowFocus: true,
})

const {
  data: sessionData,
  refetch: refetchSessions,
  isFetching: isSessionsFetching,
} = useQuery({
  key: () => ['bot-display-sessions', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdContainerDisplaySessions({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId,
  refetchOnWindowFocus: true,
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
  onSettled: () => queryCache.invalidateQueries({ key: ['bot-settings', props.botId] }),
})

watch(settings, (value) => {
  settingsForm.display_enabled = value?.display_enabled ?? false
}, { immediate: true })

async function handleToggleDisplay(enabled: boolean) {
  const previous = settingsForm.display_enabled
  settingsForm.display_enabled = enabled
  try {
    await updateSettings({ display_enabled: enabled })
    await refetchDisplay()
    toast.success(enabled ? t('bots.desktop.desktopEnabledSuccess') : t('bots.desktop.desktopDisabledSuccess'))
  } catch (error) {
    settingsForm.display_enabled = previous
    toast.error(resolveApiErrorMessage(error, t('common.saveFailed')))
  }
}

const sessions = computed<DisplaySessionInfo[]>(() => sessionData.value?.items ?? [])
const info = computed<HandlersDisplayInfoResponse>(() => displayInfo.value ?? {})

const runtimeSummary = computed(() => {
  if (!info.value.enabled) return t('bots.desktop.summaryDisabled')
  if (info.value.available && info.value.running) return t('bots.desktop.summaryReady')
  if (info.value.unavailable_reason) return info.value.unavailable_reason
  return t('bots.desktop.summaryPreparing')
})

const statusColorClass = computed(() => {
  if (info.value.running) return 'bg-success'
  if (info.value.enabled) return 'bg-warning'
  return 'bg-muted-foreground'
})

const runtimeGroups = computed(() => [
  {
    title: t('bots.desktop.groupInfrastructure'),
    icon: Layers,
    items: [
      { key: 'enabled', label: t('bots.desktop.enabled'), isBoolean: true, ok: info.value.enabled === true, value: info.value.enabled ? t('common.yes') : t('common.no') },
      { key: 'runtime', label: t('bots.desktop.runtime'), isBoolean: true, ok: info.value.available === true, value: info.value.available ? t('bots.desktop.statusReady') : t('bots.desktop.statusNotReady') },
      { key: 'system', label: t('bots.desktop.system'), isBoolean: false, ok: info.value.prepare_supported !== false, value: info.value.prepare_system || '-' },
    ]
  },
  {
    title: t('bots.desktop.groupEnvironment'),
    icon: Box,
    items: [
      { key: 'desktop', label: t('bots.desktop.desktop'), isBoolean: true, ok: info.value.desktop_available === true, value: info.value.desktop_available ? t('bots.desktop.statusReady') : t('bots.desktop.statusNotReady') },
      { key: 'vnc', label: t('bots.desktop.vnc'), isBoolean: true, ok: info.value.running === true, value: info.value.running ? t('bots.desktop.statusRunning') : t('bots.desktop.statusStopped') },
    ]
  },
  {
    title: t('bots.desktop.groupApplication'),
    icon: Puzzle,
    items: [
      { key: 'browser', label: t('bots.desktop.browser'), isBoolean: true, ok: info.value.browser_available === true, value: info.value.browser_available ? t('bots.desktop.statusReady') : t('bots.desktop.statusNotReady') },
      { key: 'toolkit', label: t('bots.desktop.toolkit'), isBoolean: true, ok: info.value.toolkit_available === true, value: info.value.toolkit_available ? t('bots.desktop.statusReady') : t('bots.desktop.statusNotReady') },
    ]
  }
])

const previewItems = computed<PreviewItem[]>(() => {
  const seen = new Set<string>()
  const items: PreviewItem[] = []
  for (const session of sessions.value) {
    if (!session.id || session.id === liveSessionId.value) continue
    seen.add(session.id)
    items.push({
      key: session.id,
      sessionId: session.id,
      title: session.id,
      state: session.state,
      snapshot: displaySnapshots.find(props.botId, session.id)?.dataUrl,
    })
  }
  for (const snapshot of snapshotItems.value) {
    if (snapshot.botId !== props.botId) continue
    const id = snapshot.sessionId || snapshot.tabId
    if (!id || id === liveSessionId.value || snapshot.tabId === 'settings-desktop' || seen.has(id)) continue
    seen.add(id)
    items.push({
      key: id,
      sessionId: snapshot.sessionId,
      title: id,
      snapshot: snapshot.dataUrl,
    })
  }
  return items
})

async function refetchAll() {
  isRefreshing.value = true
  try {
    await Promise.all([refetchDisplay(), refetchSessions()])
  } finally {
    isRefreshing.value = false
  }
}

async function handleCloseSession(sessionId: string | undefined) {
  if (!sessionId) return
  closingSessionId.value = sessionId
  try {
    await deleteBotsByBotIdContainerDisplaySessionsBySessionId({
      path: {
        bot_id: props.botId,
        session_id: sessionId,
      },
      throwOnError: true,
    })
    await refetchSessions()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.desktop.closeFailed')))
  } finally {
    closingSessionId.value = ''
  }
}

function handleSnapshot(payload: { tabId: string; sessionId?: string; dataUrl: string }) {
  if (payload.sessionId) {
    liveSessionId.value = payload.sessionId
  }
  displaySnapshots.upsert(props.botId, payload)
}
</script>
