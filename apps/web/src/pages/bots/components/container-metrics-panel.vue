<template>
  <div class="space-y-4 rounded-md border p-4">
    <div class="space-y-1">
      <h4 class="text-xs font-medium">
        {{ t('bots.container.metricsTitle') }}
      </h4>
      <p class="text-xs text-muted-foreground">
        {{ t('bots.container.metricsSubtitle') }}
      </p>
    </div>

    <div
      v-if="loading && !metrics"
      class="flex items-center gap-2 text-xs text-muted-foreground"
    >
      <Spinner />
      <span>{{ t('common.loading') }}</span>
    </div>

    <div
      v-else-if="backendUnsupported"
      class="rounded-md border border-dashed px-3 py-2 text-xs text-muted-foreground"
    >
      {{ t('bots.container.metricsUnsupported') }}
    </div>

    <div
      v-else-if="!hasAnyMetric"
      class="rounded-md border border-dashed px-3 py-2 text-xs text-muted-foreground"
    >
      {{ taskRunning === false ? t('bots.container.metricsStopped') : t('bots.container.metricsUnavailable') }}
    </div>

    <template v-else>
      <div
        v-if="taskRunning === false"
        class="rounded-md border border-primary/20 bg-primary/5 px-3 py-2 text-xs"
      >
        {{ t('bots.container.metricsStopped') }}
      </div>

      <div class="grid gap-3 md:grid-cols-3">
        <div class="rounded-md border bg-background/70 p-3">
          <p class="text-xs text-muted-foreground">
            {{ t('bots.container.metricsLabels.cpu') }}
          </p>
          <p class="mt-2 text-2xl font-semibold">
            {{ cpuValueText }}
          </p>
          <p class="mt-2 text-[11px] text-muted-foreground">
            {{ t('bots.container.currentSample') }}
          </p>
        </div>

        <div class="rounded-md border bg-background/70 p-3">
          <p class="text-xs text-muted-foreground">
            {{ t('bots.container.metricsLabels.memory') }}
          </p>
          <p class="mt-2 text-2xl font-semibold">
            {{ memoryValueText }}
          </p>
          <p class="mt-2 text-[11px] text-muted-foreground">
            {{ memoryHintText }}
          </p>
        </div>

        <div class="rounded-md border bg-background/70 p-3">
          <p class="text-xs text-muted-foreground">
            {{ t('bots.container.metricsLabels.storage') }}
          </p>
          <p class="mt-2 text-2xl font-semibold">
            {{ storageValueText }}
          </p>
          <p class="mt-2 text-[11px] text-muted-foreground break-all">
            {{ t('bots.container.metricsPath') }}: {{ storagePathText }}
          </p>
        </div>
      </div>

      <p
        v-if="sampledAtText !== '-'"
        class="text-[11px] text-muted-foreground"
      >
        {{ t('bots.container.sampledAt') }}: {{ sampledAtText }}
      </p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Spinner } from '@memohai/ui'
import type { HandlersGetContainerMetricsResponse } from '@memohai/sdk'
import { formatDateTime } from '@/utils/date-time'

const props = defineProps<{
  backend: string
  loading: boolean
  metrics: HandlersGetContainerMetricsResponse | null
}>()

const { t } = useI18n()

const status = computed(() => props.metrics?.status)
const cpuMetrics = computed(() => props.metrics?.metrics?.cpu)
const memoryMetrics = computed(() => props.metrics?.metrics?.memory)
const storageMetrics = computed(() => props.metrics?.metrics?.storage)

const backendUnsupported = computed(() =>
  props.metrics?.supported === false,
)
const taskRunning = computed(() => status.value?.task_running)
const hasAnyMetric = computed(() =>
  !!cpuMetrics.value || !!memoryMetrics.value || !!storageMetrics.value,
)

const cpuValueText = computed(() => formatPercent(cpuMetrics.value?.usage_percent))
const memoryValueText = computed(() => formatBytes(memoryMetrics.value?.usage_bytes))
const storageValueText = computed(() => formatBytes(storageMetrics.value?.used_bytes))
const storagePathText = computed(() => storageMetrics.value?.path || '-')
const sampledAtText = computed(() =>
  formatDateTime(props.metrics?.sampled_at, { fallback: '-' }),
)
const memoryHintText = computed(() => {
  const limit = memoryMetrics.value?.limit_bytes
  if (limit && limit > 0) {
    const usagePercent = formatPercent(memoryMetrics.value?.usage_percent)
    return `${formatBytes(memoryMetrics.value?.usage_bytes)} / ${formatBytes(limit)}${usagePercent === '--' ? '' : ` (${usagePercent})`}`
  }
  if (memoryMetrics.value) {
    return t('bots.container.metricsUnlimited')
  }
  return t('bots.container.metricsUnavailable')
})

function formatBytes(value?: number) {
  if (typeof value !== 'number' || Number.isNaN(value) || value < 0) return '--'
  if (value === 0) return '0 B'

  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let size = value
  let unitIndex = 0

  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex += 1
  }

  const fractionDigits = size >= 100 || unitIndex === 0 ? 0 : 1
  return `${size.toFixed(fractionDigits)} ${units[unitIndex]}`
}

function formatPercent(value?: number) {
  if (typeof value !== 'number' || Number.isNaN(value) || value < 0) return '--'
  const fractionDigits = value >= 100 ? 0 : 1
  return `${value.toFixed(fractionDigits)}%`
}
</script>
