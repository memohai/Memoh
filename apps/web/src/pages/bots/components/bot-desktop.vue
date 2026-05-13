<template>
  <div class="max-w-4xl mx-auto pb-6 space-y-5">
    <!-- Sovereign Header -->
    <header class="pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-30 pt-4 -mt-4 flex items-center justify-between gap-4">
      <div class="space-y-1">
        <h2 class="text-sm font-semibold text-foreground flex items-center gap-2">
          <span class="relative flex items-center justify-center size-2.5">
            <span
              class="absolute inline-flex h-full w-full rounded-full opacity-20"
              :class="statusColorClass"
            />
            <span
              class="relative inline-flex rounded-full size-2"
              :class="statusColorClass"
            />
          </span>
          {{ $t('bots.desktop.title', 'Desktop Environment') }}
        </h2>
        <p class="text-[11px] leading-snug text-muted-foreground max-w-md">
          {{ $t('bots.desktop.subtitle', 'Manage virtual display and interaction sessions.') }}
        </p>
      </div>
      <div class="flex shrink-0 flex-wrap justify-end gap-2">
        <Button
          variant="outline"
          size="sm"
          class="shadow-none"
          :disabled="isRefreshing || isSessionsFetching || isDisplayFetching"
          @click="refetchAll"
        >
          <Spinner
            v-if="isRefreshing || isSessionsFetching || isDisplayFetching"
            class="mr-1.5 size-3.5"
          />
          <RefreshCw
            v-else
            class="mr-1.5 size-3.5 text-muted-foreground"
          />
          {{ $t('common.refresh') }}
        </Button>
      </div>
    </header>

    <!-- Telemetry Panel (Diagnostics) -->
    <section class="rounded-md border border-border/60 bg-muted/5 overflow-hidden shadow-none flex flex-col">
      <header class="px-4 py-3 border-b border-border/40 flex items-center justify-between bg-muted/10">
        <h3 class="text-xs font-medium text-foreground flex items-center gap-2">
          {{ $t('bots.desktop.runtimeTitle') }}
        </h3>
        <span
          class="text-[10px]"
          :class="isPreparing ? 'text-foreground' : 'text-muted-foreground/60'"
        >
          {{ runtimeSummary }}
        </span>
      </header>
      
      <div class="p-4 space-y-6">
        <div
          class="grid grid-cols-1 sm:grid-cols-3 gap-4 text-[11px] tabular-nums transition-all duration-200 ease-in-out"
          :class="{ 'opacity-40 grayscale-[0.4]': !info.enabled }"
        >
          <div
            v-for="group in runtimeGroups"
            :key="group.title"
            class="space-y-4 p-4 rounded-md border border-border/40 bg-background/50 flex flex-col justify-between"
          >
            <div class="space-y-4">
              <div class="text-[11px] font-semibold text-foreground/80 flex items-center gap-2 px-0.5">
                <component
                  :is="group.icon"
                  class="size-3.5 text-muted-foreground/80"
                />
                {{ group.title }}
              </div>
              <div class="space-y-2.5">
                <div
                  v-for="item in group.items"
                  :key="item.key"
                  class="flex justify-between items-center border-b border-border/40 pb-2 last:border-0 last:pb-0"
                >
                  <span class="text-muted-foreground/70">{{ item.label }}</span>
                  <div class="flex items-center gap-1.5">
                    <span
                      v-if="item.isBoolean"
                      class="size-2 rounded-full shrink-0 transition-all duration-200 ease-in-out"
                      :class="!info.enabled ? 'bg-muted-foreground/20' : (item.ok ? 'bg-success' : 'bg-destructive')"
                    />
                    <span
                      v-if="!item.isBoolean"
                      class="text-foreground font-medium text-right truncate max-w-[120px] sm:max-w-[80px]"
                      :title="item.value"
                    >{{ item.value }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- VNC Viewport -->
    <section class="relative group border border-border/60 bg-black rounded-md overflow-hidden aspect-[4/3] w-full flex flex-col shadow-none">
      <!-- Hover Overlay Control -->
      <div class="absolute top-0 inset-x-0 px-4 py-3 opacity-0 group-hover:opacity-100 z-20 backdrop-blur bg-background/95 border-b border-border/60 flex items-center justify-between">
        <div class="space-y-0.5">
          <Label class="text-xs font-medium text-foreground">{{ $t('bots.settings.desktopEnabled') }}</Label>
          <p class="text-[10px] text-muted-foreground">
            {{ $t('bots.settings.desktopEnabledDescription') }}
          </p>
        </div>
        <Switch
          :model-value="settingsForm.display_enabled"
          :disabled="isSaving"
          class="scale-90"
          @update:model-value="(val) => handleToggleDisplay(!!val)"
        />
      </div>

      <!-- Empty State / Preparing State -->
      <div
        v-if="!info.enabled"
        class="absolute inset-0 z-10 flex flex-col items-center justify-center text-muted-foreground"
      >
        <MonitorOff class="size-8 mb-3 opacity-20" />
        <span class="text-xs">{{ $t('bots.desktop.summaryDisabled') }}</span>
      </div>
      <div
        v-else-if="!info.running"
        class="absolute inset-0 z-10 flex flex-col items-center justify-center text-muted-foreground"
      >
        <Monitor class="size-8 mb-3 opacity-20" />
        <span class="text-xs">{{ runtimeSummary }}</span>
      </div>

      <!-- Display Pane -->
      <DisplayPane
        v-if="props.botId && info.running"
        :bot-id="props.botId"
        tab-id="settings-desktop"
        :title="$t('bots.desktop.liveTitle')"
        active
        :closable="false"
        class="flex-1 w-full h-full z-0"
        @snapshot="handleSnapshot"
      />
    </section>

    <!-- Session Archive -->
    <section class="space-y-4 pt-2">
      <div class="flex items-center justify-between">
        <h3 class="text-xs font-medium text-foreground flex items-center gap-2">
          {{ $t('bots.desktop.previewTitle') }}
        </h3>
      </div>

      <div
        v-if="isSessionsFetching && !previewItems.length"
        class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4"
      >
        <div
          v-for="i in 4"
          :key="i"
          class="aspect-[4/3] bg-muted/10 rounded-md border border-border/60 animate-pulse"
        />
      </div>
      
      <div
        v-else-if="previewItems.length"
        class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4"
      >
        <div
          v-for="item in previewItems"
          :key="item.key"
          class="group flex flex-col rounded-md border border-border/60 bg-muted/5 overflow-hidden shadow-none hover:bg-muted/10 transition-colors"
        >
          <!-- Snapshot Image -->
          <div class="aspect-video bg-black overflow-hidden border-b border-border/60 relative">
            <img
              v-if="item.snapshot"
              :src="item.snapshot"
              :alt="item.title"
              class="size-full object-cover grayscale opacity-70 group-hover:grayscale-0 group-hover:opacity-100 transition-all duration-300"
            >
            <div
              v-else
              class="flex size-full items-center justify-center text-[10px] text-muted-foreground bg-muted/10"
            >
              {{ $t('bots.desktop.previewEmpty') }}
            </div>
            <!-- Session State Dot -->
            <div class="absolute top-2 right-2 flex items-center justify-center size-2.5">
              <span
                class="absolute inline-flex h-full w-full rounded-full opacity-20"
                :class="item.state === 'connected' ? 'bg-success' : 'bg-muted-foreground'"
              />
              <span
                class="relative inline-flex size-2 rounded-full border border-black/10"
                :class="item.state === 'connected' ? 'bg-success' : 'bg-muted-foreground'"
              />
            </div>
          </div>
          
          <!-- Metadata & Danger Zone -->
          <div class="p-3 space-y-3 flex-1 flex flex-col justify-between">
            <div class="text-[10px] leading-tight text-foreground/80 break-all bg-muted/20 px-1.5 py-1 rounded border border-border/40">
              <span class="text-muted-foreground/60 mr-1">ID:</span>{{ item.sessionId || item.key }}
            </div>
            
            <div class="pt-2 border-t border-border/40">
              <ConfirmPopover
                v-if="item.sessionId"
                :message="$t('bots.desktop.closeSessionConfirm', 'Terminate session?')"
                :loading="closingSessionId === item.sessionId"
                @confirm="handleCloseSession(item.sessionId)"
              >
                <template #trigger>
                  <Button
                    variant="ghost"
                    size="sm"
                    class="h-7 px-2 text-[10px] text-muted-foreground hover:text-destructive hover:bg-destructive/10 shadow-none justify-center w-full font-medium"
                  >
                    <Spinner
                      v-if="closingSessionId === item.sessionId"
                      class="mr-1.5 size-3"
                    />
                    {{ $t('bots.desktop.closeSession', 'Terminate') }}
                  </Button>
                </template>
              </ConfirmPopover>
              <div
                v-else
                class="h-7 flex items-center justify-center text-[10px] text-muted-foreground/40 w-full uppercase tracking-wider"
              >
                Snapshot
              </div>
            </div>
          </div>
        </div>
      </div>
      
      <div
        v-else
        class="rounded-md border border-dashed border-border/60 bg-muted/5 py-12 text-center text-[11px] text-muted-foreground/60 shadow-none"
      >
        {{ $t('bots.desktop.noSessions') }}
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { storeToRefs } from 'pinia'
import { useMutation, useQuery, useQueryCache } from '@pinia/colada'
import {
  Button,
  Label,
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

const isPreparing = computed(() => {
  return info.value.enabled && (!info.value.available || !info.value.running)
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
