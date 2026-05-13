<template>
  <div class="px-4 pt-2 pb-10 md:px-6 md:pt-4 md:pb-12 max-w-7xl mx-auto space-y-6">
    <div class="flex items-center justify-end">
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
        <BotSelect
          v-model="selectedBotId"
          trigger-class="w-56"
          :placeholder="$t('usage.selectBotPlaceholder')"
        />
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

      <div class="space-y-1.5">
        <Label>{{ $t('usage.sessionType') }}</Label>
        <Select v-model="selectedSessionType">
          <SelectTrigger class="w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">
              {{ $t('usage.allTypes') }}
            </SelectItem>
            <SelectItem value="chat">
              {{ $t('usage.chat') }}
            </SelectItem>
            <SelectItem value="heartbeat">
              {{ $t('usage.heartbeat') }}
            </SelectItem>
            <SelectItem value="schedule">
              {{ $t('usage.schedule') }}
            </SelectItem>
          </SelectContent>
        </Select>
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
      <div class="text-muted-foreground flex items-center justify-center min-h-[60vh]">
        {{ $t('usage.selectBotPlaceholder') }}
      </div>
    </template>

    <template v-else-if="isLoading">
      <div class="flex items-center justify-center min-h-[60vh]">
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
            <CardTitle class="text-sm">
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

        <!-- Chart: Daily token usage -->
        <Card>
          <CardHeader class="pb-2">
            <CardTitle class="text-sm">
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

        <!-- Chart: Cache breakdown -->
        <Card>
          <CardHeader class="pb-2">
            <CardTitle class="text-sm">
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

        <!-- Chart: Cache hit rate -->
        <Card>
          <CardHeader class="pb-2">
            <CardTitle class="text-sm">
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

      <!-- Call records -->
      <Card>
        <CardHeader class="pb-2 flex flex-row items-center justify-between">
          <CardTitle class="text-sm">
            {{ $t('usage.records') }}
          </CardTitle>
          <span
            v-if="recordsPaginationSummary"
            class="text-xs text-muted-foreground tabular-nums"
          >
            {{ recordsPaginationSummary }}
          </span>
        </CardHeader>
        <CardContent class="space-y-3">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{{ $t('usage.colTime') }}</TableHead>
                <TableHead>{{ $t('usage.colBot') }}</TableHead>
                <TableHead>{{ $t('usage.colSessionType') }}</TableHead>
                <TableHead>{{ $t('usage.colModel') }}</TableHead>
                <TableHead>{{ $t('usage.colProvider') }}</TableHead>
                <TableHead class="text-right">
                  {{ $t('usage.colInputTokens') }}
                </TableHead>
                <TableHead class="text-right">
                  {{ $t('usage.colOutputTokens') }}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-if="isRecordsInitialLoading">
                <TableCell
                  :colspan="7"
                  class="p-0"
                >
                  <div class="flex items-center justify-center h-[480px]">
                    <Spinner class="size-6" />
                  </div>
                </TableCell>
              </TableRow>
              <TableRow v-else-if="recordsList.length === 0">
                <TableCell
                  :colspan="7"
                  class="p-0"
                >
                  <div class="flex items-center justify-center h-[480px] text-muted-foreground">
                    {{ $t('usage.noRecords') }}
                  </div>
                </TableCell>
              </TableRow>
              <template v-else>
                <TableRow
                  v-for="r in recordsList"
                  :key="r.id"
                  :class="isRecordsFetching ? 'opacity-60 transition-opacity' : 'transition-opacity'"
                >
                  <TableCell class="text-muted-foreground tabular-nums">
                    {{ formatDateTimeSeconds(r.created_at) }}
                  </TableCell>
                  <TableCell>{{ selectedBotName }}</TableCell>
                  <TableCell>{{ sessionTypeLabel(r.session_type) }}</TableCell>
                  <TableCell>{{ recordModelLabel(r) }}</TableCell>
                  <TableCell class="text-muted-foreground">
                    {{ r.provider_name || '-' }}
                  </TableCell>
                  <TableCell class="text-right tabular-nums">
                    {{ formatNumber(r.input_tokens ?? 0) }}
                  </TableCell>
                  <TableCell class="text-right tabular-nums">
                    {{ formatNumber(r.output_tokens ?? 0) }}
                  </TableCell>
                </TableRow>
              </template>
            </TableBody>
          </Table>

          <div
            v-if="recordsTotalPages > 1"
            class="flex justify-end"
          >
            <Pagination
              :total="recordsTotal"
              :items-per-page="RECORDS_PAGE_SIZE"
              :sibling-count="1"
              :page="recordsPageNumber"
              show-edges
              @update:page="setRecordsPage"
            >
              <PaginationContent v-slot="{ items }">
                <PaginationFirst />
                <PaginationPrevious />
                <template
                  v-for="(item, index) in items"
                  :key="index"
                >
                  <PaginationEllipsis
                    v-if="item.type === 'ellipsis'"
                    :index="index"
                  />
                  <PaginationItem
                    v-else
                    :value="item.value"
                    :is-active="item.value === recordsPageNumber"
                  />
                </template>
                <PaginationNext />
                <PaginationLast />
              </PaginationContent>
            </Pagination>
          </div>
        </CardContent>
      </Card>
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
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationFirst,
  PaginationItem,
  PaginationLast,
  PaginationNext,
  PaginationPrevious,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Spinner,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@memohai/ui'
import { getBotsQuery } from '@memohai/sdk/colada'
import { getBotsByBotIdTokenUsage, getBotsByBotIdTokenUsageRecords } from '@memohai/sdk'
import BotSelect from '@/components/bot-select/index.vue'
import type { HandlersDailyTokenUsage, HandlersModelTokenUsage, HandlersTokenUsageRecord } from '@memohai/sdk'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'
import { formatDateTimeSeconds } from '@/utils/date-time'

use([CanvasRenderer, LineChart, BarChart, PieChart, GridComponent, TooltipComponent, LegendComponent])

const { t } = useI18n()

const selectedBotId = useSyncedQueryParam('bot', '')
const timeRange = useSyncedQueryParam('range', '7')
const selectedModelId = useSyncedQueryParam('model', 'all')
const selectedSessionType = useSyncedQueryParam('type', 'all')
const recordsPage = useSyncedQueryParam('rpage', '1')
const modelChartType = ref('pie')

const RECORDS_PAGE_SIZE = 20

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
  if (!selectedBotId.value && list.length > 0 && list[0]!.id) {
    selectedBotId.value = list[0]!.id
  }
}, { immediate: true })

const modelIdFilter = computed(() =>
  selectedModelId.value === 'all' ? undefined : selectedModelId.value,
)

const { data: usageData, asyncStatus, refetch } = useQuery({
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

const isFetching = computed(() => asyncStatus.value === 'loading')
const isLoading = computed(() => isFetching.value && !usageData.value)

onMounted(() => {
  if (selectedBotId.value) {
    refetch()
  }
})

const byModelData = computed<HandlersModelTokenUsage[]>(() => usageData.value?.by_model ?? [])

const modelOptions = computed(() =>
  byModelData.value.filter(m => m.model_id),
)

type SessionType = 'chat' | 'heartbeat' | 'schedule'

const sessionTypeFilter = computed(() =>
  selectedSessionType.value === 'all' ? null : selectedSessionType.value as SessionType,
)

const recordsPageNumber = computed(() => {
  const parsed = parseInt(recordsPage.value, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 1
})

const { data: recordsData, asyncStatus: recordsAsyncStatus, refetch: refetchRecords } = useQuery({
  key: () => [
    'token-usage-records',
    selectedBotId.value,
    dateFrom.value,
    dateTo.value,
    modelIdFilter.value ?? '',
    sessionTypeFilter.value ?? '',
    recordsPageNumber.value,
  ],
  query: async () => {
    const { data } = await getBotsByBotIdTokenUsageRecords({
      path: { bot_id: selectedBotId.value },
      query: {
        from: dateFrom.value,
        to: dateTo.value,
        model_id: modelIdFilter.value,
        session_type: sessionTypeFilter.value ?? undefined,
        limit: RECORDS_PAGE_SIZE,
        offset: (recordsPageNumber.value - 1) * RECORDS_PAGE_SIZE,
      },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!selectedBotId.value,
})

const recordsList = computed<HandlersTokenUsageRecord[]>(() => recordsData.value?.items ?? [])
const isRecordsFetching = computed(() => recordsAsyncStatus.value === 'loading')
const isRecordsInitialLoading = computed(() => isRecordsFetching.value && !recordsData.value)
const recordsTotal = computed(() => recordsData.value?.total ?? 0)
const recordsTotalPages = computed(() =>
  Math.max(1, Math.ceil(recordsTotal.value / RECORDS_PAGE_SIZE)),
)

const recordsPaginationSummary = computed(() => {
  const total = recordsTotal.value
  if (total === 0) return ''
  const start = (recordsPageNumber.value - 1) * RECORDS_PAGE_SIZE + 1
  const end = Math.min(recordsPageNumber.value * RECORDS_PAGE_SIZE, total)
  return `${start}-${end} / ${total}`
})

const selectedBotName = computed(() => {
  const bot = botList.value.find(b => b.id === selectedBotId.value)
  return bot?.display_name || bot?.id || ''
})

function resetRecordsPage() {
  if (recordsPage.value !== '1') {
    recordsPage.value = '1'
  }
}

watch(
  () => [
    selectedBotId.value,
    dateFrom.value,
    dateTo.value,
    modelIdFilter.value,
    sessionTypeFilter.value,
  ],
  resetRecordsPage,
)

function setRecordsPage(page: number) {
  const clamped = Math.max(1, Math.min(page, recordsTotalPages.value))
  recordsPage.value = String(clamped)
}

function sessionTypeLabel(type: string | undefined): string {
  switch (type) {
    case 'chat': return t('usage.chat')
    case 'heartbeat': return t('usage.heartbeat')
    case 'schedule': return t('usage.schedule')
    default: return type || '-'
  }
}

function recordModelLabel(r: HandlersTokenUsageRecord): string {
  return r.model_name || r.model_slug || '-'
}

onMounted(() => {
  if (selectedBotId.value) {
    refetchRecords()
  }
})

interface TypedDayMaps {
  chat: Map<string, HandlersDailyTokenUsage>
  heartbeat: Map<string, HandlersDailyTokenUsage>
  schedule: Map<string, HandlersDailyTokenUsage>
}

function buildDayMap(rows: HandlersDailyTokenUsage[] | undefined) {
  const map = new Map<string, HandlersDailyTokenUsage>()
  for (const r of rows ?? []) {
    if (r.day) map.set(r.day, r)
  }
  return map
}

const dayMaps = computed<TypedDayMaps>(() => ({
  chat: buildDayMap(usageData.value?.chat),
  heartbeat: buildDayMap(usageData.value?.heartbeat),
  schedule: buildDayMap(usageData.value?.schedule),
}))

const activeTypes = computed<SessionType[]>(() => {
  const filter = sessionTypeFilter.value
  if (filter) return [filter]
  return ['chat', 'heartbeat', 'schedule']
})

const allDays = computed(() => {
  const from = new Date(dateFrom.value + 'T00:00:00')
  const toExclusive = new Date(dateTo.value + 'T00:00:00')
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const end = new Date(Math.min(toExclusive.getTime(), today.getTime() + 86400000))
  const days: string[] = []
  const cursor = new Date(from)
  while (cursor < end) {
    const y = cursor.getFullYear()
    const m = String(cursor.getMonth() + 1).padStart(2, '0')
    const d = String(cursor.getDate()).padStart(2, '0')
    days.push(`${y}-${m}-${d}`)
    cursor.setDate(cursor.getDate() + 1)
  }
  return days
})

const hasData = computed(() => {
  const chat = usageData.value?.chat ?? []
  const heartbeat = usageData.value?.heartbeat ?? []
  const schedule = usageData.value?.schedule ?? []
  return chat.length > 0 || heartbeat.length > 0 || schedule.length > 0 || byModelData.value.length > 0
})

const summary = computed(() => {
  const days = allDays.value
  const types = activeTypes.value
  const maps = dayMaps.value
  let totalInput = 0
  let totalOutput = 0
  let totalCacheRead = 0
  let totalReasoning = 0
  for (const day of days) {
    for (const tp of types) {
      const r = maps[tp].get(day)
      if (!r) continue
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
      top: 0,
      fontSize: 10,     
      textStyle: {
        overflow: 'truncate',
        width: 150,        
      },
      tooltip: {
        show:true
      }
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
    legend: { data: [t('usage.inputTokens'), t('usage.outputTokens')],top: 0, },
    grid: { left: 60, right: 20, bottom: 60, top: 40 },
    xAxis: {
      type: 'category' as const,
      data: names,
      axisLabel: { rotate: 30, fontSize: 10 },
    },
    yAxis: { type: 'value' as const },
    series: [
      {
        name: t('usage.inputTokens'),
        type: 'bar' as const,
        stack: 'tokens',
        data: models.map(m => m.input_tokens ?? 0),
      },
      {
        name: t('usage.outputTokens'),
        type: 'bar' as const,
        stack: 'tokens',
        data: models.map(m => m.output_tokens ?? 0),
      },
    ],
  }
})

const modelChartOption = computed(() => ({
  ...(modelChartType.value === 'bar' ? modelBarOption.value : modelPieOption.value),
  // legend: {
  //   fontSize: 10,
  // }
}),
)

const dailyTokensOption = computed(() => {
  const days = allDays.value
  const types = activeTypes.value
  const maps = dayMaps.value

  const totalInputLabel = t('usage.totalInput')
  const totalOutputLabel = t('usage.totalOutput')

  return {
    tooltip: { trigger: 'axis' as const },
    legend: {
      data: [totalInputLabel, totalOutputLabel],
      bottom: 0,
      left: 'center',
      itemGap: 12,
    },
    grid: { left: 60, right: 20, bottom: 40, top: 20 },
    xAxis: { type: 'category' as const, data: days },
    yAxis: { type: 'value' as const },
    series: [
      {
        name: totalInputLabel,
        type: 'bar' as const,
        stack: 'tokens',
        data: days.map(d => {
          let sum = 0
          for (const tp of types) sum += maps[tp].get(d)?.input_tokens ?? 0
          return sum
        }),
      },
      {
        name: totalOutputLabel,
        type: 'bar' as const,
        stack: 'tokens',
        data: days.map(d => {
          let sum = 0
          for (const tp of types) sum += maps[tp].get(d)?.output_tokens ?? 0
          return sum
        }),
      },
    ],
  }
})

const cacheBreakdownOption = computed(() => {
  const days = allDays.value
  const types = activeTypes.value
  const maps = dayMaps.value

  function sumField(day: string, field: 'cache_read_tokens' | 'input_tokens') {
    let total = 0
    for (const tp of types) {
      total += (maps[tp].get(day)?.[field] ?? 0) as number
    }
    return total
  }

  return {
    tooltip: { trigger: 'axis' as const },
    legend: {
      data: [t('usage.cacheRead'), t('usage.noCache')],
      bottom: 0,
      left: 'center',
      itemGap: 16,
    },
    grid: { left: 60, right: 20, bottom: 50, top: 20 },
    xAxis: { type: 'category' as const, data: days },
    yAxis: { type: 'value' as const },
    series: [
      {
        name: t('usage.cacheRead'),
        type: 'bar' as const,
        stack: 'cache',
        data: days.map(d => sumField(d, 'cache_read_tokens')),
      },
      {
        name: t('usage.noCache'),
        type: 'bar' as const,
        stack: 'cache',
        data: days.map(d => {
          const totalInput = sumField(d, 'input_tokens')
          const cacheRead = sumField(d, 'cache_read_tokens')
          return Math.max(0, totalInput - cacheRead)
        }),
      },
    ],
  }
})

const cacheHitRateOption = computed(() => {
  const days = allDays.value
  const types = activeTypes.value
  const maps = dayMaps.value

  function sumField(day: string, field: 'cache_read_tokens' | 'input_tokens') {
    let total = 0
    for (const tp of types) {
      total += (maps[tp].get(day)?.[field] ?? 0) as number
    }
    return total
  }

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
          const totalInput = sumField(d, 'input_tokens')
          const cacheRead = sumField(d, 'cache_read_tokens')
          return totalInput > 0 ? parseFloat(((cacheRead / totalInput) * 100).toFixed(1)) : 0
        }),
      },
    ],
  }
})

function formatDate(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function formatNumber(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K'
  return String(n)
}
</script>
