<template>
  <SettingsSection :title="panelTitle">
    <template #actions>
      <span
        v-if="checkedAtText"
        class="text-caption tabular-nums text-muted-foreground"
      >
        {{ checkedAtText }}
      </span>
    </template>

    <SettingsRow
      :label="$t('bots.runtimeDiagnostics.summaryLabel')"
      :description="summaryText"
    >
      <Badge
        :variant="stateBadgeVariant(diagnostics?.overall_state)"
        size="sm"
      >
        {{ stateText(diagnostics?.overall_state) }}
      </Badge>
    </SettingsRow>

    <SettingsRow
      v-if="loadErrorText"
      :label="$t('bots.runtimeDiagnostics.errorLabel')"
      :description="loadErrorText"
    >
      <Button
        variant="outline"
        size="sm"
        :disabled="isRefreshing"
        @click="refreshDiagnostics"
      >
        <RefreshCw
          class="size-4"
          :class="isRefreshing ? 'animate-spin' : ''"
        />
        {{ $t('common.retry') }}
      </Button>
    </SettingsRow>

    <SettingsRow
      v-for="row in readoutRows"
      :key="row.label"
      :label="row.label"
      :description="row.detail"
    >
      <Badge
        :variant="stateBadgeVariant(row.state)"
        size="sm"
      >
        {{ row.value }}
      </Badge>
    </SettingsRow>

    <SettingsRow
      :label="$t('bots.runtimeDiagnostics.evidenceLabel')"
      :description="$t('bots.runtimeDiagnostics.evidenceDescription')"
    >
      <div class="grid w-full grid-cols-3 gap-2 sm:flex sm:w-auto sm:flex-wrap sm:items-center sm:justify-end">
        <Button
          variant="outline"
          size="sm"
          :disabled="isRefreshing"
          @click="refreshDiagnostics"
        >
          <RefreshCw
            class="size-4"
            :class="isRefreshing ? 'animate-spin' : ''"
          />
          {{ $t('common.refresh') }}
        </Button>
        <Button
          variant="outline"
          size="sm"
          :disabled="!diagnostics"
          @click="copyDiagnostics"
        >
          <CopyIcon class="size-4" />
          {{ $t('common.copy') }}
        </Button>
        <Button
          variant="outline"
          size="sm"
          :disabled="!diagnostics"
          @click="openDetails"
        >
          <PanelRightOpen class="size-4" />
          {{ $t('common.details') }}
        </Button>
      </div>
    </SettingsRow>

    <Sheet
      :open="detailsOpen"
      @update:open="setDetailsOpen"
    >
      <SheetContent class="w-[min(100vw,42rem)] p-0 sm:max-w-[42rem]">
        <SheetHeader class="border-b border-border pr-12">
          <SheetTitle>{{ panelTitle }}</SheetTitle>
          <SheetDescription>
            {{ $t('bots.runtimeDiagnostics.drawerDescription') }}
          </SheetDescription>
        </SheetHeader>

        <ScrollArea class="min-h-0 flex-1">
          <div class="space-y-6 p-4">
            <section
              v-for="section in sections"
              :key="section.id"
              class="space-y-2.5"
            >
              <h3 class="px-2 text-label font-medium text-muted-foreground">
                {{ sectionTitle(section.id, section.title) }}
              </h3>
              <div class="overflow-hidden rounded-[var(--radius-menu-shell)] border border-border bg-card">
                <div
                  v-for="row in section.rows"
                  :key="`${section.id}-${row.label}`"
                  class="mx-4 grid gap-2 border-b border-border py-3 last:border-b-0 sm:grid-cols-[minmax(0,11rem)_1fr]"
                >
                  <div class="min-w-0">
                    <p class="truncate text-sm font-medium text-foreground">
                      {{ row.label }}
                    </p>
                    <Badge
                      v-if="row.state || row.code"
                      :variant="stateBadgeVariant(row.state)"
                      size="sm"
                      class="mt-1"
                    >
                      {{ row.code || stateText(row.state) }}
                    </Badge>
                  </div>
                  <div class="min-w-0 text-sm text-foreground">
                    <p
                      class="break-words"
                      :class="row.mono ? 'font-mono text-xs' : ''"
                    >
                      {{ row.value }}
                    </p>
                    <Button
                      v-if="row.copyValue"
                      variant="ghost"
                      size="sm"
                      class="mt-2"
                      @click="copyValue(row.copyValue)"
                    >
                      <CopyIcon class="size-4" />
                      {{ $t('common.copy') }}
                    </Button>
                    <pre
                      v-if="row.detail && (row.mono || row.detail.includes('\n'))"
                      class="mt-2 max-h-64 overflow-auto rounded-md bg-muted p-3 text-xs text-muted-foreground"
                    >{{ row.detail }}</pre>
                    <p
                      v-else-if="row.detail"
                      class="mt-1 break-words text-xs text-muted-foreground"
                    >
                      {{ row.detail }}
                    </p>
                  </div>
                </div>
              </div>
            </section>
          </div>
        </ScrollArea>

        <SheetFooter
          v-if="jumpTargets.length > 0"
          class="border-t border-border p-4"
        >
          <Button
            v-for="target in jumpTargets"
            :key="target.target"
            variant="outline"
            size="sm"
            @click="jumpTo(target.target)"
          >
            <ArrowRight class="size-4" />
            {{ target.label }}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  </SettingsSection>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useQuery } from '@pinia/colada'
import { getBotsByBotIdRuntimeDiagnostics } from '@memohai/sdk'
import type { RuntimediagnosticsState } from '@memohai/sdk'
import {
  Badge,
  Button,
  ScrollArea,
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  toast,
} from '@memohai/ui'
import { ArrowRight, Copy as CopyIcon, PanelRightOpen, RefreshCw } from 'lucide-vue-next'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import { formatRelativeTime } from '@/utils/date-time'
import {
  buildRuntimeDiagnosticReadouts,
  buildRuntimeDiagnosticSections,
  runtimeDiagnosticSummaryText,
  stateBadgeVariant,
  stateLabel,
  type RuntimeDiagnosticTextResolver,
  type RuntimeDiagnosticsScope,
} from './runtime-diagnostics'
import { resolveApiErrorMessage } from '@/utils/api-error'

interface JumpTarget {
  label: string
  target: string
}

const props = withDefaults(defineProps<{
  botId: string
  scope: RuntimeDiagnosticsScope
  agentId?: string
  jumpTargets?: JumpTarget[]
}>(), {
  agentId: '',
  jumpTargets: () => [],
})

const emit = defineEmits<{
  jump: [target: string]
}>()

const { t, te, locale } = useI18n()
const detailsOpen = ref(false)
const isRefreshing = ref(false)

const { data: diagnostics, error: loadError, refetch } = useQuery({
  key: () => ['bot-runtime-diagnostics', props.botId],
  query: async () => {
    const { data } = await getBotsByBotIdRuntimeDiagnostics({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!props.botId,
  refetchOnWindowFocus: false,
})

const panelTitle = computed(() =>
  props.scope === 'acp'
    ? t('bots.runtimeDiagnostics.acpTitle')
    : t('bots.runtimeDiagnostics.workspaceTitle'),
)

const summaryText = computed(() =>
  runtimeDiagnosticSummaryText(diagnostics.value, runtimeDiagnosticText),
)

const loadErrorText = computed(() =>
  loadError.value ? resolveApiErrorMessage(loadError.value, t('bots.runtimeDiagnostics.loadFailed')) : '',
)

const checkedAtText = computed(() => {
  const value = diagnostics.value?.checked_at
  if (!value) return ''
  const relative = formatRelativeTime(value, { locale: locale.value, fallback: '' })
  return relative ? t('bots.runtimeDiagnostics.checkedAt', { time: relative }) : ''
})

const readoutRows = computed(() =>
  buildRuntimeDiagnosticReadouts(diagnostics.value, props.scope, props.agentId, runtimeDiagnosticText),
)

const sections = computed(() =>
  buildRuntimeDiagnosticSections(diagnostics.value, {
    scope: props.scope,
    agentId: props.agentId,
    text: runtimeDiagnosticText,
  }),
)

const runtimeDiagnosticText: RuntimeDiagnosticTextResolver = (key, fallback, params) => {
  if (!te(key)) return formatFallback(fallback, params)
  return params ? t(key, params) : t(key)
}

function stateText(state: RuntimediagnosticsState | undefined): string {
  const key = state || 'unknown'
  return t(`bots.runtimeDiagnostics.states.${key}`, stateLabel(state))
}

function sectionTitle(id: string, fallback: string): string {
  return t(`bots.runtimeDiagnostics.sections.${id}`, fallback)
}

function formatFallback(fallback: string, params?: Record<string, number | string>): string {
  if (!params) return fallback
  return Object.entries(params).reduce(
    (value, [key, replacement]) => value.replaceAll(`{${key}}`, String(replacement)),
    fallback,
  )
}

async function refreshDiagnostics() {
  if (!props.botId || isRefreshing.value) return
  isRefreshing.value = true
  try {
    await refetch()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.runtimeDiagnostics.loadFailed')))
  } finally {
    isRefreshing.value = false
  }
}

function openDetails() {
  detailsOpen.value = true
}

function setDetailsOpen(value: boolean) {
  detailsOpen.value = value
}

async function copyDiagnostics() {
  if (!diagnostics.value) return
  await copyValue(JSON.stringify(diagnostics.value, null, 2))
}

async function copyValue(value: string) {
  try {
    await navigator.clipboard.writeText(value)
    toast.success(t('common.copied'))
  } catch {
    toast.error(t('common.copyFailed'))
  }
}

function jumpTo(target: string) {
  detailsOpen.value = false
  emit('jump', target)
}

const POLL_INTERVAL_MS = 30_000
let pollTimer: ReturnType<typeof setInterval> | null = null

function pageVisible() {
  return typeof document === 'undefined' || document.visibilityState === 'visible'
}

function pollNow() {
  if (!pageVisible() || !props.botId) return
  void refreshDiagnostics()
}

function startPoll() {
  stopPoll()
  pollTimer = setInterval(pollNow, POLL_INTERVAL_MS)
}

function stopPoll() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

function handleVisibilityChange() {
  if (pageVisible()) {
    pollNow()
    startPoll()
  } else {
    stopPoll()
  }
}

watch(detailsOpen, (open) => {
  if (open) pollNow()
})

onMounted(() => {
  document.addEventListener('visibilitychange', handleVisibilityChange)
  startPoll()
})

onBeforeUnmount(() => {
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  stopPoll()
})
</script>
