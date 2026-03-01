<template>
  <div class="p-6 space-y-6 max-w-7xl mx-auto">
    <div class="flex items-center justify-between">
      <h1 class="text-2xl font-semibold tracking-tight">
        {{ $t('usage.title') }}
      </h1>
      <Button
        variant="outline"
        size="sm"
        :disabled="isLoading || !selectedBotId"
        @click="refetch()"
      >
        <Spinner
          v-if="isLoading"
          class="mr-2 size-4"
        />
        {{ $t('common.refresh') }}
      </Button>
    </div>

    <!-- Filters -->
    <div class="flex flex-wrap items-end gap-4">
      <div class="space-y-1.5">
        <Label>{{ $t('usage.selectBot') }}</Label>
        <Select v-model="selectedBotId">
          <SelectTrigger class="w-56">
            <SelectValue :placeholder="$t('usage.selectBotPlaceholder')" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem
              v-for="bot in botList"
              :key="bot.id"
              :value="bot.id!"
            >
              {{ bot.display_name || bot.id }}
            </SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div class="space-y-1.5">
        <Label>{{ $t('usage.timeRange') }}</Label>
        <Select v-model="timeRange">
          <SelectTrigger class="w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="7">
              {{ $t('usage.last7Days') }}
            </SelectItem>
            <SelectItem value="30">
              {{ $t('usage.last30Days') }}
            </SelectItem>
            <SelectItem value="90">
              {{ $t('usage.last90Days') }}
            </SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div class="space-y-1.5">
        <Label>{{ $t('usage.dateFrom') }}</Label>
        <Input
          v-model="dateFrom"
          type="date"
          class="w-40"
        />
      </div>
      <div class="space-y-1.5">
        <Label>{{ $t('usage.dateTo') }}</Label>
        <Input
          v-model="dateTo"
          type="date"
          class="w-40"
        />
      </div>

      <div
        v-if="modelOptions.length > 0"
        class="space-y-1.5"
      >
        <Label>{{ $t('usage.filterByModel') }}</Label>
        <Select v-model="selectedModelId">
          <SelectTrigger class="w-56">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">
              {{ $t('usage.allModels') }}
            </SelectItem>
            <SelectItem
              v-for="m in modelOptions"
              :key="m.model_id"
              :value="m.model_id!"
            >
              {{ m.model_name || m.model_slug }} ({{ m.provider_name }})
            </SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>

    <template v-if="!selectedBotId">
      <div class="text-muted-foreground text-center py-20">
        {{ $t('usage.selectBotPlaceholder') }}
      </div>
    </template>

    <template v-else-if="isLoading">
      <div class="flex justify-center py-20">
        <Spinner class="size-8" />
      </div>
    </template>

    <template v-else>
      <!-- Summary cards -->
      <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <Card>
          <CardHeader class="pb-2">
            <CardDescription>{{ $t('usage.totalInputTokens') }}</CardDescription>
          </CardHeader>
          <CardContent>
            <p class="text-2xl font-bold tabular-nums">
              {{ formatNumber(summary.totalInputTokens) }}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader class="pb-2">
            <CardDescription>{{ $t('usage.totalOutputTokens') }}</CardDescription>
          </CardHeader>
          <CardContent>
            <p class="text-2xl font-bold tabular-nums">
              {{ formatNumber(summary.totalOutputTokens) }}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader class="pb-2">
            <CardDescription>{{ $t('usage.avgCacheHitRate') }}</CardDescription>
          </CardHeader>
          <CardContent>
            <p class="text-2xl font-bold tabular-nums">
              {{ summary.avgCacheHitRate }}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader class="pb-2">
            <CardDescription>{{ $t('usage.totalReasoningTokens') }}</CardDescription>
          </CardHeader>
          <CardContent>
            <p class="text-2xl font-bold tabular-nums">
              {{ formatNumber(summary.totalReasoningTokens) }}
            </p>
          </CardContent>
        </Card>
      </div>

      <div
        v-if="hasData"
        class="grid grid-cols-1 lg:grid-cols-2 gap-6"
      >
        <!-- Chart: Model distribution -->
        <Card v-if="byModelData.length > 0">
          <CardHeader class="pb-2 flex flex-row items-center justify-between">
            <CardTitle class="text-base">
              {{ $t('usage.modelDistribution') }}
            </CardTitle>
            <Select
              v-model="modelChartType"
              class="w-auto"
            >
              <SelectTrigger class="h-7 w-24 text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="pie">
                  {{ $t('usage.chartPie') }}
                </SelectItem>
                <SelectItem value="bar">
                  {{ $t('usage.chartBar') }}
                </SelectItem>
              </SelectContent>
            </Select>
          </CardHeader>
          <CardContent>
            <VChart
              :key="modelChartType"
              style="height: 300px; width: 100%"
              :option="modelChartOption"
              autoresize
            />
          </CardContent>
        </Card>

        <!-- Chart 1: Daily token usage stacked area -->
        <Card>
          <CardHeader class="pb-2">
            <CardTitle class="text-base">
              {{ $t('usage.dailyTokens') }}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <VChart
              style="height: 300px; width: 100%"
              :option="dailyTokensOption"
              autoresize
            />
          </CardContent>
        </Card>

        <!-- Chart 2: Cache breakdown stacked bar -->
        <Card>
          <CardHeader class="pb-2">
            <CardTitle class="text-base">
              {{ $t('usage.cacheBreakdown') }}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <VChart
              style="height: 300px; width: 100%"
              :option="cacheBreakdownOption"
              autoresize
            />
          </CardContent>
        </Card>

        <!-- Chart 3: Cache hit rate line -->
        <Card>
          <CardHeader class="pb-2">
            <CardTitle class="text-base">
              {{ $t('usage.cacheHitRate') }}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <VChart
              style="height: 300px; width: 100%"
              :option="cacheHitRateOption"
              autoresize
            />
          </CardContent>
        </Card>
      </div>

      <div
        v-else
        class="text-muted-foreground text-center py-12"
      >
        {{ $t('usage.noData') }}
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useQuery } from '@pinia/colada'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, BarChart, PieChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
} from 'echarts/components'
import VChart from 'vue-echarts'
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Spinner,
} from '@memoh/ui'
import { getBotsQuery } from '@memoh/sdk/colada'
import { getBotsByBotIdTokenUsage } from '@memoh/sdk'
import type { HandlersDailyTokenUsage, HandlersModelTokenUsage } from '@memoh/sdk'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'

use([CanvasRenderer, LineChart, BarChart, PieChart, GridComponent, TooltipComponent, LegendComponent])

const { t } = useI18n()

const selectedBotId = useSyncedQueryParam('bot', '')
const timeRange = useSyncedQueryParam('range', '30')
const selectedModelId = useSyncedQueryParam('model', 'all')
const modelChartType = ref('pie')

function daysAgo(days: number): string {
  const d = new Date()
  d.setDate(d.getDate() - days + 1)
  return formatDate(d)
}

function tomorrow(): string {
  const d = new Date()
  d.setDate(d.getDate() + 1)
  return formatDate(d)
}

const initDays = parseInt(timeRange.value, 10) || 30
const dateFrom = useSyncedQueryParam('from', daysAgo(initDays))
const dateTo = useSyncedQueryParam('to', tomorrow())

watch(timeRange, (val) => {
  const days = parseInt(val, 10)
  if (days > 0) {
    dateFrom.value = daysAgo(days)
    dateTo.value = tomorrow()
  }
})

const { data: botData } = useQuery(getBotsQuery())
const botList = computed(() => botData.value?.items ?? [])

watch(botList, (list) => {
  if (!selectedBotId.value && list.length > 0 && list[0].id) {
    selectedBotId.value = list[0].id
  }
}, { immediate: true })

const modelIdFilter = computed(() =>
  selectedModelId.value === 'all' ? undefined : selectedModelId.value,
)

const { data: usageData, status, refetch } = useQuery({
  key: () => ['token-usage', selectedBotId.value, dateFrom.value, dateTo.value, modelIdFilter.value ?? ''],
  query: async () => {
    const { data } = await getBotsByBotIdTokenUsage({
      path: { bot_id: selectedBotId.value },
      query: {
        from: dateFrom.value,
        to: dateTo.value,
        model_id: modelIdFilter.value,
      },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!selectedBotId.value,
})

const isLoading = computed(() => status.value === 'loading')

onMounted(() => {
  if (selectedBotId.value) refetch()
})

const byModelData = computed<HandlersModelTokenUsage[]>(() => usageData.value?.by_model ?? [])

const modelOptions = computed(() =>
  byModelData.value.filter(m => m.model_id),
)

const allDays = computed(() => {
  const daySet = new Set<string>()
  for (const row of usageData.value?.chat ?? []) {
    if (row.day) daySet.add(row.day)
  }
  for (const row of usageData.value?.heartbeat ?? []) {
    if (row.day) daySet.add(row.day)
  }
  return [...daySet].sort()
})

const hasData = computed(() => allDays.value.length > 0 || byModelData.value.length > 0)

function buildDayMap(rows: HandlersDailyTokenUsage[] | undefined) {
  const map = new Map<string, HandlersDailyTokenUsage>()
  for (const r of rows ?? []) {
    if (r.day) map.set(r.day, r)
  }
  return map
}

const summary = computed(() => {
  const chatMap = buildDayMap(usageData.value?.chat)
  const hbMap = buildDayMap(usageData.value?.heartbeat)
  let totalInput = 0
  let totalOutput = 0
  let totalCacheRead = 0
  let totalReasoning = 0
  for (const m of [chatMap, hbMap]) {
    for (const r of m.values()) {
      totalInput += r.input_tokens ?? 0
      totalOutput += r.output_tokens ?? 0
      totalCacheRead += r.cache_read_tokens ?? 0
      totalReasoning += r.reasoning_tokens ?? 0
    }
  }
  const rate = totalInput > 0 ? ((totalCacheRead / totalInput) * 100).toFixed(1) + '%' : '-'
  return {
    totalInputTokens: totalInput,
    totalOutputTokens: totalOutput,
    avgCacheHitRate: rate,
    totalReasoningTokens: totalReasoning,
  }
})

function modelLabel(m: HandlersModelTokenUsage) {
  return `${m.model_name || m.model_slug} (${m.provider_name})`
}

const modelPieOption = computed(() => {
  const data = byModelData.value.map(m => ({
    name: modelLabel(m),
    value: (m.input_tokens ?? 0) + (m.output_tokens ?? 0),
  }))
  return {
    tooltip: {
      trigger: 'item' as const,
      formatter: (params: { name: string; value: number; percent: number }) =>
        `${params.name}<br/>${t('usage.tokens')}: ${formatNumber(params.value)} (${params.percent}%)`,
    },
    legend: {
      orient: 'vertical' as const,
      right: 10,
      top: 'center',
    },
    series: [
      {
        type: 'pie' as const,
        radius: ['40%', '70%'],
        center: ['40%', '50%'],
        avoidLabelOverlap: true,
        itemStyle: {
          borderRadius: 6,
          borderColor: 'var(--background)',
          borderWidth: 2,
        },
        label: { show: false },
        emphasis: {
          label: { show: true, fontWeight: 'bold' as const },
        },
        data,
      },
    ],
  }
})

const modelBarOption = computed(() => {
  const models = byModelData.value
  const names = models.map(m => modelLabel(m))
  return {
    tooltip: { trigger: 'axis' as const },
    legend: { data: [t('usage.chatInput'), t('usage.chatOutput')] },
    grid: { left: 60, right: 20, bottom: 60, top: 40 },
    xAxis: {
      type: 'category' as const,
      data: names,
      axisLabel: { rotate: 30, fontSize: 10 },
    },
    yAxis: { type: 'value' as const },
    series: [
      {
        name: t('usage.chatInput'),
        type: 'bar' as const,
        stack: 'tokens',
        data: models.map(m => m.input_tokens ?? 0),
      },
      {
        name: t('usage.chatOutput'),
        type: 'bar' as const,
        stack: 'tokens',
        data: models.map(m => m.output_tokens ?? 0),
      },
    ],
  }
})

const modelChartOption = computed(() =>
  modelChartType.value === 'bar' ? modelBarOption.value : modelPieOption.value,
)

const dailyTokensOption = computed(() => {
  const days = allDays.value
  const chatMap = buildDayMap(usageData.value?.chat)
  const hbMap = buildDayMap(usageData.value?.heartbeat)
  return {
    tooltip: { trigger: 'axis' as const },
    legend: { data: [t('usage.chatInput'), t('usage.chatOutput'), t('usage.heartbeatInput'), t('usage.heartbeatOutput')] },
    grid: { left: 60, right: 20, bottom: 30, top: 40 },
    xAxis: { type: 'category' as const, data: days },
    yAxis: { type: 'value' as const },
    series: [
      {
        name: t('usage.chatInput'),
        type: 'line' as const,
        stack: 'input',
        areaStyle: {},
        data: days.map(d => chatMap.get(d)?.input_tokens ?? 0),
      },
      {
        name: t('usage.heartbeatInput'),
        type: 'line' as const,
        stack: 'input',
        areaStyle: {},
        data: days.map(d => hbMap.get(d)?.input_tokens ?? 0),
      },
      {
        name: t('usage.chatOutput'),
        type: 'line' as const,
        stack: 'output',
        areaStyle: {},
        data: days.map(d => chatMap.get(d)?.output_tokens ?? 0),
      },
      {
        name: t('usage.heartbeatOutput'),
        type: 'line' as const,
        stack: 'output',
        areaStyle: {},
        data: days.map(d => hbMap.get(d)?.output_tokens ?? 0),
      },
    ],
  }
})

const cacheBreakdownOption = computed(() => {
  const days = allDays.value
  const chatMap = buildDayMap(usageData.value?.chat)
  const hbMap = buildDayMap(usageData.value?.heartbeat)
  return {
    tooltip: { trigger: 'axis' as const },
    legend: { data: [t('usage.cacheRead'), t('usage.cacheWrite'), t('usage.noCache')] },
    grid: { left: 60, right: 20, bottom: 30, top: 40 },
    xAxis: { type: 'category' as const, data: days },
    yAxis: { type: 'value' as const },
    series: [
      {
        name: t('usage.cacheRead'),
        type: 'bar' as const,
        stack: 'cache',
        data: days.map(d => {
          const c = chatMap.get(d)
          const h = hbMap.get(d)
          return (c?.cache_read_tokens ?? 0) + (h?.cache_read_tokens ?? 0)
        }),
      },
      {
        name: t('usage.cacheWrite'),
        type: 'bar' as const,
        stack: 'cache',
        data: days.map(d => {
          const c = chatMap.get(d)
          const h = hbMap.get(d)
          return (c?.cache_write_tokens ?? 0) + (h?.cache_write_tokens ?? 0)
        }),
      },
      {
        name: t('usage.noCache'),
        type: 'bar' as const,
        stack: 'cache',
        data: days.map(d => {
          const c = chatMap.get(d)
          const h = hbMap.get(d)
          const totalInput = (c?.input_tokens ?? 0) + (h?.input_tokens ?? 0)
          const cacheRead = (c?.cache_read_tokens ?? 0) + (h?.cache_read_tokens ?? 0)
          const cacheWrite = (c?.cache_write_tokens ?? 0) + (h?.cache_write_tokens ?? 0)
          return Math.max(0, totalInput - cacheRead - cacheWrite)
        }),
      },
    ],
  }
})

const cacheHitRateOption = computed(() => {
  const days = allDays.value
  const chatMap = buildDayMap(usageData.value?.chat)
  const hbMap = buildDayMap(usageData.value?.heartbeat)
  return {
    tooltip: {
      trigger: 'axis' as const,
      formatter: (params: { name: string; value: number }[]) => {
        const p = Array.isArray(params) ? params[0] : params
        return `${p.name}<br/>${t('usage.cacheHitRate')}: ${p.value.toFixed(1)}%`
      },
    },
    grid: { left: 60, right: 20, bottom: 30, top: 20 },
    xAxis: { type: 'category' as const, data: days },
    yAxis: { type: 'value' as const, axisLabel: { formatter: '{value}%' }, max: 100 },
    series: [
      {
        name: t('usage.cacheHitRate'),
        type: 'line' as const,
        smooth: true,
        data: days.map(d => {
          const c = chatMap.get(d)
          const h = hbMap.get(d)
          const totalInput = (c?.input_tokens ?? 0) + (h?.input_tokens ?? 0)
          const cacheRead = (c?.cache_read_tokens ?? 0) + (h?.cache_read_tokens ?? 0)
          return totalInput > 0 ? parseFloat(((cacheRead / totalInput) * 100).toFixed(1)) : 0
        }),
      },
    ],
  }
})

function formatDate(d: Date): string {
  return d.toISOString().slice(0, 10)
}

function formatNumber(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K'
  return String(n)
}
</script>
