<template>
  <!-- Overview is the bot's "lobby", modeled on a real product dashboard (à la
       Cursor): where it's reachable (platforms), the couple of settings worth
       surfacing (model + memory), then a usage visualization built from actual
       data (token stat row + a daily bar chart). No filler rows that just
       mirror the left nav. Health checks stay demoted to an issue banner +
       dialog so a healthy bot reads calm. Outer shell mirrors the Appearance
       page (max-w-3xl, h1.text-lg.mb-6.px-2, space-y-8); the bot tab container
       adds px-6 pt-4 pb-4, so pt-6/pb-8 here match its pt-10/pb-12. -->
  <div class="mx-auto max-w-3xl pt-6 pb-8">
    <h1 class="mb-6 px-2 text-lg font-semibold">
      {{ $t('bots.tabs.overview') }}
    </h1>

    <div class="space-y-8">
      <!-- Issue banner: only when the bot needs attention; opens diagnostics. -->
      <button
        v-if="hasIssue"
        type="button"
        class="flex w-full items-center gap-3 rounded-[var(--radius-menu-shell)] border border-destructive/30 bg-destructive/5 px-4 py-3 text-left transition-colors hover:bg-destructive/10"
        @click="checksOpen = true"
      >
        <AlertCircle class="size-4 shrink-0 text-destructive" />
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-foreground">
            {{ issueTitle }}
          </p>
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.overview.issueHint') }}
          </p>
        </div>
        <ChevronRight class="size-4 shrink-0 text-muted-foreground" />
      </button>

      <!-- Platforms: where the bot is reachable (this product's "source control"
           block). Every state holds the same min-height so a cold load doesn't
           make the block jump. -->
      <SettingsSection :title="$t('bots.overview.platformsTitle')">
        <div
          v-if="channelsLoading && configuredChannels.length === 0"
          class="mx-4 flex min-h-[3.75rem] items-center gap-3 py-3"
        >
          <Skeleton class="size-7 shrink-0 rounded-md" />
          <div class="flex-1 space-y-1.5">
            <Skeleton class="h-3.5 w-40" />
            <Skeleton class="h-3 w-56" />
          </div>
        </div>

        <div
          v-else-if="configuredChannels.length === 0"
          class="mx-4 flex min-h-[3.75rem] items-center justify-between gap-4 py-3"
        >
          <div class="min-w-0">
            <div class="text-sm font-medium text-foreground">
              {{ $t('bots.overview.platformsEmpty') }}
            </div>
            <p class="mt-0.5 text-xs text-muted-foreground">
              {{ $t('bots.overview.platformsEmptyHint') }}
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            class="shrink-0"
            @click="go('channels')"
          >
            {{ $t('bots.overview.connectAction') }}
          </Button>
        </div>

        <template v-else>
          <div
            v-for="item in configuredChannels"
            :key="item.meta.type"
            class="mx-4 flex min-h-[3.75rem] items-center gap-3 border-b border-border py-3 last:border-b-0"
          >
            <span class="flex size-7 shrink-0 items-center justify-center">
              <ChannelIcon
                :channel="item.meta.type as string"
                size="1.25em"
              />
            </span>
            <span class="flex-1 truncate text-sm font-medium text-foreground">
              {{ channelTitle(item.meta) }}
            </span>
            <span class="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
              <span
                class="size-1.5 rounded-full"
                :class="item.config?.disabled ? 'bg-muted-foreground/40' : 'bg-success'"
              />
              {{ item.config?.disabled ? $t('bots.channels.configured') : $t('bots.channels.statusActive') }}
            </span>
          </div>
        </template>
      </SettingsSection>

      <!-- Core setup: only the two settings worth surfacing here — the model it
           thinks with, and whether memory is on. Everything else lives in its
           own tab, so this never becomes a mirror of the left nav. -->
      <SettingsSection :title="$t('bots.overview.configTitle')">
        <SettingsRow
          :label="$t('bots.overview.modelLabel')"
          :description="modelName"
        >
          <span
            v-if="reasoningOn"
            class="rounded bg-accent px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground"
          >{{ reasoningLabel }}</span>
        </SettingsRow>

        <SettingsRow
          :label="$t('bots.overview.memoryLabel')"
          :description="memoryDesc"
        />
      </SettingsSection>

      <!-- Usage: a real data visualization (stat row + daily token bar chart)
           from the token-usage feed — the dashboard's "numbers", kept last so
           identity, reachability and setup read first. Same echarts recipe as
           the dedicated Usage page. -->
      <SettingsSection :title="$t('bots.overview.usageTitle')">
        <div class="space-y-4 p-4">
          <div class="grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-4">
            <div
              v-for="stat in usageStats"
              :key="stat.key"
            >
              <p class="text-xs text-muted-foreground">
                {{ stat.label }}
              </p>
              <p class="mt-0.5 text-xl font-semibold tabular-nums text-foreground">
                {{ usageLoading ? '—' : stat.value }}
              </p>
            </div>
          </div>

          <div
            v-if="usageLoading"
            class="h-[200px]"
          >
            <Skeleton class="size-full rounded-md" />
          </div>
          <!-- Inline style, not a Tailwind height class: vue-echarts puts an
               inline height on its root, which beats a class and collapses the
               canvas to 0. Mirrors the Usage page's `style="height:..."`. -->
          <VChart
            v-else-if="hasUsage"
            :option="dailyOption"
            autoresize
            style="height: 200px; width: 100%"
          />
          <div
            v-else
            class="flex h-[200px] items-center justify-center text-sm text-muted-foreground"
          >
            {{ $t('bots.overview.usageNone') }}
          </div>
        </div>
      </SettingsSection>

      <BotChecksPanel
        v-model:open="checksOpen"
        :bot-id="botId"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useQuery } from '@pinia/colada'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import VChart from 'vue-echarts'
import { useDark } from '@vueuse/core'
import { Button, Skeleton } from '@memohai/ui'
import { AlertCircle, ChevronRight } from 'lucide-vue-next'
import {
  getBotsById,
  getBotsByBotIdSettings,
  getBotsByBotIdMemoryStatus,
  getBotsByBotIdTokenUsage,
  getModels,
  getChannels,
  getBotsByIdChannelByPlatform,
  type HandlersChannelMeta,
  type ChannelChannelConfig,
  type HandlersDailyTokenUsage,
} from '@memohai/sdk'
import BotChecksPanel from './bot-checks-panel.vue'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import ChannelIcon from '@/components/channel-icon/index.vue'
import { channelTypeDisplayName } from '@/utils/channel-type-label'
import { useBotStatusMeta } from '@/composables/useBotStatusMeta'
import { useSyncedQueryParam } from '@/composables/useSyncedQueryParam'

use([CanvasRenderer, BarChart, GridComponent, TooltipComponent, LegendComponent])

interface BotChannelItem {
  meta: HandlersChannelMeta
  config: ChannelChannelConfig | null
  configured: boolean
}

const route = useRoute()
const { t } = useI18n()

const routeIdentifier = computed(() => route.params.botName as string)
const activeTab = useSyncedQueryParam('tab', 'overview')
const checksOpen = ref(false)

const { data: bot } = useQuery({
  key: () => ['bot', routeIdentifier.value],
  query: async () => {
    const { data } = await getBotsById({ path: { id: routeIdentifier.value }, throwOnError: true })
    return data
  },
  enabled: () => !!routeIdentifier.value,
})
const botId = computed(() => bot.value?.id ?? '')

const { hasIssue, issueTitle } = useBotStatusMeta(bot, t)

const { data: settings } = useQuery({
  key: () => ['bot-settings', botId.value],
  query: async () => {
    const { data } = await getBotsByBotIdSettings({ path: { bot_id: botId.value }, throwOnError: true })
    return data
  },
  enabled: () => !!botId.value,
})

const { data: models } = useQuery({
  key: () => ['models'],
  query: async () => {
    const { data } = await getModels({ throwOnError: true })
    return data
  },
})

const { data: memoryStatus } = useQuery({
  key: () => ['bot-memory-status', botId.value],
  query: async () => {
    const { data } = await getBotsByBotIdMemoryStatus({ path: { bot_id: botId.value }, throwOnError: true })
    return data
  },
  enabled: () => !!botId.value,
})

// Shares the colada key with bot-channels.vue, so visiting Platforms after
// Overview (or vice versa) reuses the cached probe instead of refetching.
const { data: channels, isLoading: channelsLoading } = useQuery({
  key: () => ['bot-channels', botId.value],
  query: async (): Promise<BotChannelItem[]> => {
    const { data: metas } = await getChannels({ throwOnError: true })
    if (!metas) return []
    const configurableTypes = metas.filter((m) => !m.configless)
    return Promise.all(
      configurableTypes.map(async (meta) => {
        try {
          const { data: config } = await getBotsByIdChannelByPlatform({ path: { id: botId.value, platform: meta.type ?? '' }, throwOnError: true })
          return { meta, config: config ?? null, configured: true }
        } catch {
          return { meta, config: null, configured: false }
        }
      }),
    )
  },
  enabled: () => !!botId.value,
})

const configuredChannels = computed(() => (channels.value ?? []).filter((c) => c.configured))

function channelTitle(meta: HandlersChannelMeta) {
  return channelTypeDisplayName(t, meta.type, meta.display_name)
}

const modelName = computed(() => {
  const id = settings.value?.chat_model_id
  if (!id) return t('bots.overview.modelNone')
  const model = (models.value ?? []).find((m) => (m.id || m.model_id) === id)
  return model?.name || model?.model_id || id
})

const reasoningOn = computed(() => !!settings.value?.reasoning_enabled)
const reasoningLabel = computed(() => {
  const effort = settings.value?.reasoning_effort
  return effort
    ? `${t('bots.overview.reasoningBadge')} · ${effort}`
    : t('bots.overview.reasoningBadge')
})

const memoryDesc = computed(() => {
  const n = memoryStatus.value?.indexed_count
  if (n == null) return t('bots.overview.memoryNone')
  return t('bots.overview.memoryCount', { count: n })
})

// --- Usage: last 30 days of token usage, drawn as a stat row + a daily bar
// chart (same data shape + echarts recipe as the dedicated Usage page). ---

function ymd(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

// The 30 calendar days ending today; doubles as the chart x-axis so empty days
// still render a gap instead of collapsing the timeline.
const usageDays = computed(() => {
  const days: string[] = []
  const cursor = new Date()
  cursor.setHours(0, 0, 0, 0)
  cursor.setDate(cursor.getDate() - 29)
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  while (cursor <= today) {
    days.push(ymd(cursor))
    cursor.setDate(cursor.getDate() + 1)
  }
  return days
})

const { data: tokenUsage, isLoading: usageLoading } = useQuery({
  key: () => ['bot-token-usage-overview', botId.value],
  query: async () => {
    const from = usageDays.value[0] ?? ymd(new Date())
    const end = new Date()
    end.setDate(end.getDate() + 1) // `to` is exclusive
    const { data } = await getBotsByBotIdTokenUsage({
      path: { bot_id: botId.value },
      query: { from, to: ymd(end) },
      throwOnError: true,
    })
    return data
  },
  enabled: () => !!botId.value,
})

function buildDayMap(rows: HandlersDailyTokenUsage[] | undefined) {
  const map = new Map<string, HandlersDailyTokenUsage>()
  for (const r of rows ?? []) {
    if (r.day) map.set(r.day, r)
  }
  return map
}

const dayMaps = computed(() => ({
  chat: buildDayMap(tokenUsage.value?.chat),
  heartbeat: buildDayMap(tokenUsage.value?.heartbeat),
  schedule: buildDayMap(tokenUsage.value?.schedule),
}))

const usageTotals = computed(() => {
  const maps = dayMaps.value
  let input = 0
  let output = 0
  let cacheRead = 0
  for (const day of usageDays.value) {
    for (const tp of ['chat', 'heartbeat', 'schedule'] as const) {
      const r = maps[tp].get(day)
      if (!r) continue
      input += r.input_tokens ?? 0
      output += r.output_tokens ?? 0
      cacheRead += r.cache_read_tokens ?? 0
    }
  }
  return { input, output, total: input + output, cacheRead }
})

const hasUsage = computed(() => usageTotals.value.total > 0)

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

const usageStats = computed(() => {
  const u = usageTotals.value
  const rate = u.input > 0 ? `${Math.round((u.cacheRead / u.input) * 100)}%` : '—'
  return [
    { key: 'total', label: t('bots.overview.usageTotal'), value: formatNumber(u.total) },
    { key: 'input', label: t('bots.overview.usageInput'), value: formatNumber(u.input) },
    { key: 'output', label: t('bots.overview.usageOutput'), value: formatNumber(u.output) },
    { key: 'cache', label: t('bots.overview.usageCacheHit'), value: rate },
  ]
})

const isDark = useDark()

// echarts paints on a <canvas> and can't read our CSS custom properties (the
// tokens are oklch + nested vars), so resolve each design token to a concrete
// color through a probe element, then rasterize it to a single pixel and read
// the bytes back as rgb/rgba. The pixel round-trip matters: echarts' default
// hover (emphasis) state runs the bar's fill through zrender's `liftColor`,
// which only parses #hex/rgb/rgba/hsl — NOT oklch/color(). On Electron 34's
// Chromium 132, `getComputedStyle(...).color` (and a canvas `fillStyle`
// round-trip) keep CSS Color 4 values as `oklch(...)`, so liftColor returns
// undefined and the hovered bar paints transparent — i.e. "the bar vanishes on
// hover". Painting a pixel collapses any renderable color to concrete sRGB
// bytes, so the value zrender sees is always parseable. `void isDark.value`
// re-runs this when the theme flips so the chart tracks light/dark.
const colorCanvas = typeof document !== 'undefined'
  ? document.createElement('canvas').getContext('2d', { willReadFrequently: true })
  : null

function readColor(token: string, fallback: string): string {
  if (typeof document === 'undefined') return fallback
  const probe = document.createElement('span')
  probe.style.color = `var(${token})`
  probe.style.display = 'none'
  document.body.appendChild(probe)
  const resolved = getComputedStyle(probe).color
  probe.remove()
  if (!resolved) return fallback
  if (!colorCanvas) return resolved
  try {
    colorCanvas.clearRect(0, 0, 1, 1)
    colorCanvas.fillStyle = '#000'
    colorCanvas.fillStyle = resolved
    colorCanvas.fillRect(0, 0, 1, 1)
    const [r, g, b, a] = colorCanvas.getImageData(0, 0, 1, 1).data
    return a === 255 ? `rgb(${r}, ${g}, ${b})` : `rgba(${r}, ${g}, ${b}, ${(a / 255).toFixed(3)})`
  }
  catch {
    return fallback
  }
}

const chartTheme = computed(() => {
  void isDark.value
  const fontFamily = typeof document !== 'undefined'
    ? getComputedStyle(document.body).fontFamily
    : 'inherit'
  return {
    // Black / white / grey only. `--primary` is a violet in this theme, so the
    // bars use the neutral foreground (input) + muted-foreground (output); no
    // brand or accent color anywhere in the chart.
    bar: readColor('--foreground', '#18181b'),
    barMuted: readColor('--muted-foreground', '#a1a1aa'),
    text: readColor('--muted-foreground', '#a1a1aa'),
    line: readColor('--border', '#e4e4e7'),
    fontFamily,
  }
})

const dailyOption = computed(() => {
  const days = usageDays.value
  const maps = dayMaps.value
  const c = chartTheme.value
  const inputLabel = t('bots.overview.usageInput')
  const outputLabel = t('bots.overview.usageOutput')
  const sumDay = (day: string, field: 'input_tokens' | 'output_tokens') => {
    let sum = 0
    for (const tp of ['chat', 'heartbeat', 'schedule'] as const) {
      sum += maps[tp].get(day)?.[field] ?? 0
    }
    return sum
  }
  return {
    textStyle: { fontFamily: c.fontFamily },
    // The tooltip is real DOM (not canvas), so its CSS references the SAME
    // tokens as Popover/HoverCard directly — shell radius, the --border-menu
    // hairline and --shadow-dropdown — for a pixel-identical surface. echarts'
    // own background/border/padding are zeroed so they don't fight the token
    // CSS. Body copy uses --text-body (the popover's own size/leading) + the
    // page font. axisPointer is a soft solid hairline, not echarts' dashed line.
    tooltip: {
      trigger: 'axis' as const,
      backgroundColor: 'transparent',
      borderWidth: 0,
      padding: 0,
      extraCssText: [
        'background: var(--popover)',
        'color: var(--popover-foreground)',
        'border: 1px solid var(--border-menu)',
        'border-radius: var(--radius-menu-shell)',
        'box-shadow: var(--shadow-dropdown)',
        'padding: 12px 14px',
        `font-family: ${c.fontFamily}`,
      ].join('; '),
      // `shadow`, NOT `line`: a 1px line pointer paints ON TOP of the bar, and a
      // 30-day bar is only a few px wide, so the line fully covers the hovered
      // bar — which reads as "the bar vanishes when I hover it". A shadow band
      // paints a faint wash BEHIND the bars, so the hovered bar stays visible.
      axisPointer: {
        type: 'shadow' as const,
        shadowStyle: { color: 'rgba(128, 128, 128, 0.12)' },
      },
      formatter: (params: { seriesName?: string, value?: number, color?: string, axisValueLabel?: string }[]) => {
        const list = Array.isArray(params) ? params : [params]
        const head = list[0]?.axisValueLabel ?? ''
        const rows = list.map((p) => {
          const val = formatNumber(typeof p.value === 'number' ? p.value : 0)
          const dot = `<span style="display:inline-block;width:6px;height:6px;border-radius:9999px;margin-right:7px;background:${p.color ?? 'var(--muted-foreground)'};"></span>`
          return '<div style="display:flex;align-items:center;justify-content:space-between;gap:24px;line-height:1.7;">'
            + `<span style="color:var(--muted-foreground);">${dot}${p.seriesName ?? ''}</span>`
            + `<span style="color:var(--popover-foreground);font-weight:500;font-variant-numeric:tabular-nums;">${val}</span></div>`
        }).join('')
        return '<div style="font-size:var(--text-body);line-height:var(--text-body--line-height);letter-spacing:var(--text-body--letter-spacing);min-width:132px;">'
          + `<div style="color:var(--muted-foreground);margin-bottom:3px;">${head}</div>${rows}</div>`
      },
    },
    legend: {
      data: [inputLabel, outputLabel],
      bottom: 0,
      itemGap: 16,
      icon: 'roundRect',
      itemWidth: 8,
      itemHeight: 8,
      textStyle: { color: c.text, fontFamily: c.fontFamily, fontSize: 11 },
    },
    grid: { left: 8, right: 8, top: 14, bottom: 40, containLabel: true },
    xAxis: {
      type: 'category' as const,
      data: days,
      axisTick: { show: false },
      axisLine: { lineStyle: { color: c.line } },
      axisLabel: { color: c.text, fontFamily: c.fontFamily, fontSize: 10, formatter: (v: string) => v.slice(5) },
    },
    yAxis: {
      type: 'value' as const,
      axisLine: { show: false },
      splitLine: { lineStyle: { color: c.line } },
      axisLabel: { color: c.text, fontFamily: c.fontFamily, fontSize: 10, formatter: (v: number) => formatNumber(v) },
    },
    series: [
      {
        name: inputLabel,
        type: 'bar' as const,
        stack: 'tokens',
        itemStyle: { color: c.bar },
        data: days.map((d) => sumDay(d, 'input_tokens')),
      },
      {
        name: outputLabel,
        type: 'bar' as const,
        stack: 'tokens',
        itemStyle: { color: c.barMuted, borderRadius: [3, 3, 0, 0] as [number, number, number, number] },
        data: days.map((d) => sumDay(d, 'output_tokens')),
      },
    ],
  }
})

function go(tab: string) {
  activeTab.value = tab
}
</script>
