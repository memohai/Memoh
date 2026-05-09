<template>
  <div class="flex gap-6 h-full absolute inset-0 p-4 mx-auto">
    <!-- Left: File list -->
    <div class="w-64 shrink-0 flex flex-col border rounded-lg overflow-hidden max-h-full">
      <div class="p-3 border-b space-y-3 shrink-0">
        <div class="flex items-center justify-between">
          <h4 class="text-xs font-medium">
            {{ $t('bots.memory.files') }}
          </h4>
          <div class="flex items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              type="button"
              class="size-8 p-0"
              :disabled="loading || compactLoading || memories.length === 0"
              :title="$t('bots.memory.compact')"
              :aria-label="$t('bots.memory.compact')"
              @click="openCompactDialog"
            >
              <Brain
                class="size-3.5 text-primary"
              />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              type="button"
              class="size-8 p-0"
              :disabled="loading"
              :aria-label="$t('common.refresh')"
              @click="loadMemories"
            >
              <RefreshCw
                :class="{ 'animate-spin': loading }"
                class="size-3.5"
              />
            </Button>
          </div>
        </div>
        <div class="relative">
          <Search
            class="absolute left-2.5 top-1/2 -translate-y-1/2 size-3 text-muted-foreground"
          />
          <Input
            v-model="searchQuery"
            :placeholder="$t('bots.memory.searchPlaceholder')"
            class="pl-8 h-8 text-xs"
          />
        </div>
      </div>

      <ScrollArea class="flex-1 min-h-0 ">
        <div class="p-2 space-y-1">
          <div
            v-if="loading && memories.length === 0"
            class="p-4 text-center"
          >
            <Spinner class="mx-auto" />
          </div>
          <div
            v-else-if="filteredMemories.length === 0"
            class="p-4 text-center text-xs text-muted-foreground"
          >
            {{ $t('bots.memory.empty') }}
          </div>
          <button
            v-for="item in filteredMemories"
            :key="item.id"
            type="button"
            class="w-full text-left px-3 py-2 rounded-md text-xs transition-colors hover:bg-accent group relative"
            :class="{ 'bg-accent font-medium text-primary': selectedId === item.id }"
            :aria-label="`Open memory ${formatDate(item.created_at)}`"
            @click="selectMemory(item)"
          >
            <div class="flex items-center gap-2">
              <FileText
                class="size-3 shrink-0 opacity-70"
              />
              <span class="truncate pr-4">{{ formatDate(item.created_at) }}</span>
            </div>
            <div class="mt-1 text-[10px] text-muted-foreground truncate opacity-70 group-hover:opacity-100">
              {{ item.memory.length > 60 ? item.memory.slice(0, 60) + '...' : item.memory }}
            </div>
          </button>
        </div>
      </ScrollArea>

      <div class="p-2 border-t mt-auto">
        <Button
          variant="outline"
          size="sm"
          class="w-full h-8 text-xs"
          @click="openNewMemoryDialog"
        >
          <Plus
            class="mr-2 size-3"
          />
          {{ $t('bots.memory.newMemory') }}
        </Button>
      </div>
    </div>

    <!-- Right: Editor/Preview -->
    <div class="flex-1 flex flex-col border rounded-lg overflow-hidden ">
      <template v-if="selectedMemory">
        <div class="flex-1 flex flex-col min-h-0">
          <div class="p-3 border-b flex items-center justify-between bg-muted/30 shrink-0">
            <div class="flex items-center gap-3 min-w-0">
              <FileText
                class="size-4 text-muted-foreground shrink-0"
              />
              <div class="min-w-0">
                <h4 class="text-xs font-medium truncate">
                  {{ formatDate(selectedMemory.created_at) }}
                </h4>
                <div class="flex items-center gap-1.5 text-[10px] text-muted-foreground mt-0.5">
                  <span class="font-mono">ID: {{ selectedMemory.id }}</span>
                  <button
                    type="button"
                    class="hover:text-foreground transition-colors"
                    :title="$t('common.copy')"
                    :aria-label="$t('common.copy')"
                    @click="copyToClipboard(selectedMemory.id)"
                  >
                    <Copy
                      class="size-2.5"
                    />
                  </button>
                </div>
              </div>
            </div>
            <div class="flex items-center gap-2 shrink-0">
              <ConfirmPopover
                :message="$t('bots.memory.deleteConfirm')"
                @confirm="handleDelete"
              >
                <template #trigger>
                  <Button
                    variant="ghost"
                    size="sm"
                    type="button"
                    class="size-8 p-0 text-destructive hover:text-destructive hover:bg-destructive/10"
                    :disabled="actionLoading"
                    :aria-label="$t('common.delete')"
                  >
                    <Trash2
                      class="size-3.5"
                    />
                  </Button>
                </template>
              </ConfirmPopover>
              <Button
                size="sm"
                class="h-8 px-3 text-xs"
                :disabled="actionLoading || !isDirty"
                @click="handleSave"
              >
                <Spinner
                  v-if="actionLoading"
                  class="mr-1.5 size-3"
                />
                {{ $t('common.save') }}
              </Button>
            </div>
          </div>
          <div class="flex-1 relative">
            <Textarea
              v-model="editContent"
              class="absolute inset-0 resize-none border-0 rounded-none focus-visible:ring-0 font-mono text-xs p-4 h-full"
              placeholder="Write your memory content here (Markdown)..."
            />
          </div>
        </div>

        <!-- Charts Section -->
        <div
          v-if="showChartSection"
          class="h-60 border-t flex flex-col bg-muted/5 shrink-0"
        >
          <div class="px-3 py-1.5 border-b bg-muted/10 flex items-center justify-between shrink-0">
            <h5 class="text-[10px] font-bold uppercase tracking-wider text-muted-foreground/70">
              Vector Manifold
            </h5>
          </div>
          <div class="flex-1 flex min-h-0 divide-x overflow-hidden">
            <!-- Sparse: Top K Buckets -->
            <div class="flex-1 flex flex-col p-3 min-w-0">
              <p class="text-[9px] font-semibold text-muted-foreground/60 mb-2 uppercase shrink-0">
                {{ chartLeftTitle }}
              </p>
              <VChart
                class="h-full w-full min-h-0"
                :option="chartLeftOption"
                autoresize
              />
            </div>

            <!-- Sparse/Dense secondary chart -->
            <div class="flex-1 flex flex-col p-3 min-w-0">
              <p class="text-[9px] font-semibold text-muted-foreground/60 mb-2 uppercase shrink-0">
                {{ chartRightTitle }}
              </p>
              <VChart
                class="h-full w-full min-h-0"
                :option="chartRightOption"
                autoresize
              />
            </div>
          </div>
        </div>
      </template>
      <div
        v-else
        class="flex-1 flex flex-col items-center justify-center text-muted-foreground p-8 text-center"
      >
        <div class="size-12 rounded-full bg-muted flex items-center justify-center mb-4">
          <Brain
            class="size-6 opacity-20"
          />
        </div>
        <h3 class="text-xs font-medium text-foreground">
          {{ $t('bots.memory.title') }}
        </h3>
        <p class="text-xs mt-1 max-w-60">
          Select a file from the sidebar to view or edit, or create a new one to persist long-term information for your bot.
        </p>
        <Button
          variant="outline"
          size="sm"
          class="mt-6"
          @click="openNewMemoryDialog"
        >
          {{ $t('bots.memory.newMemory') }}
        </Button>
      </div>
    </div>

    <!-- New Memory Dialog -->
    <Dialog v-model:open="newMemoryDialogOpen">
      <DialogContent class="sm:max-w-2xl max-h-[80vh] flex flex-col">
        <DialogHeader>
          <DialogTitle>{{ $t('bots.memory.newMemory') }}</DialogTitle>
        </DialogHeader>

        <div class="flex-1 min-h-0 overflow-hidden flex flex-col gap-4 py-4">
          <div class="flex items-center gap-4 shrink-0">
            <Button
              variant="outline"
              size="sm"
              class="text-xs h-8"
              @click="loadHistory"
            >
              <RefreshCw
                :class="{ 'animate-spin': historyLoading }"
                class="mr-1.5 size-3"
              />
              {{ $t('bots.memory.fromConversation') }}
            </Button>
          </div>

          <div
            v-if="historyLoading"
            class="h-40 flex items-center justify-center border rounded-md bg-muted/10 shrink-0"
          >
            <Spinner />
          </div>
          <ScrollArea
            v-else-if="historyMessages.length > 0"
            class="h-48 border rounded-md p-2 bg-muted/10 shrink-0"
          >
            <div class="space-y-2">
              <button
                v-for="(msg, idx) in historyMessages"
                :key="idx"
                type="button"
                class="w-full text-left flex items-start gap-2 p-2 rounded hover:bg-muted/50 transition-colors group cursor-pointer"
                :aria-pressed="selectedHistoryMessages.includes(msg)"
                @click="toggleMessageSelection(msg)"
              >
                <div
                  class="mt-1 size-4 shrink-0 rounded border border-primary flex items-center justify-center transition-colors"
                  :class="selectedHistoryMessages.includes(msg) ? 'bg-primary text-primary-foreground' : 'bg-background'"
                >
                  <Check
                    v-if="selectedHistoryMessages.includes(msg)"
                    class="size-2.5"
                  />
                </div>
                <div class="min-w-0">
                  <Badge
                    variant="outline"
                    class="text-[9px] uppercase px-1 py-0 h-3.5 mb-1"
                  >
                    {{ msg.role }}
                  </Badge>
                  <p class="text-xs text-foreground wrap-break-word line-clamp-3">
                    {{ extractMessageText(msg.content) }}
                  </p>
                </div>
              </button>
            </div>
          </ScrollArea>

          <div class="space-y-2 flex-1 min-h-0 flex flex-col">
            <Label class="text-xs font-medium shrink-0">Memory Content</Label>
            <Textarea
              v-model="newMemoryContent"
              class="flex-1 font-mono text-xs resize-none min-h-0"
              placeholder="Paste content or select from history above..."
            />
          </div>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            @click="newMemoryDialogOpen = false"
          >
            {{ $t('common.cancel') }}
          </Button>
          <Button
            :disabled="actionLoading || !newMemoryContent.trim()"
            @click="handleCreateMemory"
          >
            <Spinner
              v-if="actionLoading"
              class="mr-1.5 size-3"
            />
            {{ $t('common.confirm') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Compact Memory Dialog -->
    <Dialog v-model:open="compactDialogOpen">
      <DialogContent class="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{{ $t('bots.memory.compact') }}</DialogTitle>
        </DialogHeader>

        <div class="py-4 space-y-6">
          <p class="text-xs text-muted-foreground">
            {{ $t('bots.memory.compactConfirm') }}
          </p>

          <div class="space-y-3">
            <Label>{{ $t('bots.memory.compactRatio') }}</Label>
            <RadioGroup
              v-model="compactRatio"
              class="grid grid-cols-1 gap-3"
            >
              <Label
                class="flex items-start gap-3 p-3 rounded-md border cursor-pointer hover:bg-muted/50 transition-colors"
                :class="{ 'bg-muted border-primary': compactRatio === '0.8' }"
              >
                <RadioGroupItem
                  value="0.8"
                  class="mt-1"
                />
                <div class="min-w-0">
                  <p class="text-xs font-medium">{{ $t('bots.memory.compactRatioLight') }}</p>
                  <p class="text-xs text-muted-foreground">{{ $t('bots.memory.compactRatioLightDesc') }}</p>
                </div>
              </Label>
              <Label
                class="flex items-start gap-3 p-3 rounded-md border cursor-pointer hover:bg-muted/50 transition-colors"
                :class="{ 'bg-muted border-primary': compactRatio === '0.5' }"
              >
                <RadioGroupItem
                  value="0.5"
                  class="mt-1"
                />
                <div class="min-w-0">
                  <p class="text-xs font-medium">{{ $t('bots.memory.compactRatioMedium') }}</p>
                  <p class="text-xs text-muted-foreground">{{ $t('bots.memory.compactRatioMediumDesc') }}</p>
                </div>
              </Label>
              <Label
                class="flex items-start gap-3 p-3 rounded-md border cursor-pointer hover:bg-muted/50 transition-colors"
                :class="{ 'bg-muted border-primary': compactRatio === '0.3' }"
              >
                <RadioGroupItem
                  value="0.3"
                  class="mt-1"
                />
                <div class="min-w-0">
                  <p class="text-xs font-medium">{{ $t('bots.memory.compactRatioAggressive') }}</p>
                  <p class="text-xs text-muted-foreground">{{ $t('bots.memory.compactRatioAggressiveDesc') }}</p>
                </div>
              </Label>
            </RadioGroup>
          </div>

          <div class="space-y-3">
            <Label>{{ $t('bots.memory.compactDecayDate') }} ({{ $t('common.optional') }})</Label>
            <Input
              v-model="compactDecayDate"
              type="date"
              class="w-full"
            />
            <p
              v-if="compactDecayDays > 0"
              class="text-[10px] text-muted-foreground"
            >
              Calculated: {{ compactDecayDays }} days old
            </p>
          </div>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            @click="compactDialogOpen = false"
          >
            {{ $t('common.cancel') }}
          </Button>
          <Button
            :disabled="compactLoading"
            @click="handleCompact"
          >
            <Spinner
              v-if="compactLoading"
              class="mr-1.5 size-3"
            />
            {{ $t('common.confirm') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { Brain, RefreshCw, Search, FileText, Plus, Copy, Trash2, Check } from 'lucide-vue-next'
import { computed, ref, onMounted, watch } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, BarChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
} from 'echarts/components'
import VChart from 'vue-echarts'
import { useColorMode } from '@vueuse/core'
import {
  Button,
  Input,
  ScrollArea,
  Spinner,
  Textarea,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Badge,
  Label,
  RadioGroup,
  RadioGroupItem,
} from '@memohai/ui'
import {
  getBotsByBotIdMemory,
  getBotsByBotIdMemoryStatus,
  postBotsByBotIdMemory,
  deleteBotsByBotIdMemoryById,
  postBotsByBotIdMemoryCompact,
  getBotsByBotIdMessages,
  postBotsByBotIdMemorySearch,
} from '@memohai/sdk'
import type {
  AdaptersCdfPoint as MemoryCdfPoint,
  AdaptersMemoryItem,
  AdaptersMemoryStatusResponse,
  AdaptersTopKBucket as MemoryTopKBucket,
  MessageMessage,
} from '@memohai/sdk'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import { useClipboard } from '@/composables/useClipboard'
import { formatDateTimeSeconds } from '@/utils/date-time'
import { useSettingsStore } from '@/store/settings'

use([CanvasRenderer, LineChart, BarChart, GridComponent, TooltipComponent])

interface MemoryItem {
  id: string
  memory: string
  created_at?: string
  updated_at?: string
  hash?: string
  score?: number
  cdf_curve?: MemoryCdfPoint[]
  top_k_buckets?: MemoryTopKBucket[]
}

type MessageContentBlock = { type: string; text?: string }
type MessageContent = string | MessageContentBlock[] | unknown

interface Message {
  role: string
  content: MessageContent
  created_at?: string
}

function extractMessageText(content: MessageContent): string {
  if (typeof content === 'string') return content
  if (Array.isArray(content)) {
    return content
      .filter((b): b is MessageContentBlock => typeof b === 'object' && b !== null)
      .map(b => b.text ?? '')
      .join('')
  }
  return JSON.stringify(content)
}

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const colorMode = useColorMode()
const settingsStore = useSettingsStore()
const { copyText } = useClipboard()
const loading = ref(false)
const actionLoading = ref(false)
const compactLoading = ref(false)
const denseSearchLoading = ref(false)
const memories = ref<MemoryItem[]>([])
const memoryStatus = ref<AdaptersMemoryStatusResponse | null>(null)
const denseSearchResults = ref<Array<{ id: string; memory: string; score: number }>>([])
const searchQuery = ref('')
const selectedId = ref<string | null>(null)
const editContent = ref('')
const originalContent = ref('')

// New memory dialog
const newMemoryDialogOpen = ref(false)
const newMemoryContent = ref('')
const historyLoading = ref(false)
const historyMessages = ref<Message[]>([])
const selectedHistoryMessages = ref<Message[]>([])

// Compact memory dialog
const compactDialogOpen = ref(false)
const compactRatio = ref('0.5')
const compactDecayDate = ref('')

const selectedTopKBuckets = computed(() => selectedMemory.value?.top_k_buckets ?? [])
const selectedCdfCurve = computed(() => selectedMemory.value?.cdf_curve ?? [])
const hasSparseExplain = computed(() =>
  selectedTopKBuckets.value.length > 0 && selectedCdfCurve.value.length > 0,
)
const memoryMode = computed(() => memoryStatus.value?.memory_mode ?? 'off')
const isDenseMode = computed(() => memoryMode.value === 'dense')
const hasDenseExplain = computed(() => denseSearchResults.value.length > 0)
const showChartSection = computed(() =>
  (isDenseMode.value && hasDenseExplain.value) || (!isDenseMode.value && hasSparseExplain.value),
)
const selectedCdfMaxK = computed(() => {
  const lastPoint = selectedCdfCurve.value[selectedCdfCurve.value.length - 1]
  return Math.max(1, lastPoint?.k ?? selectedCdfCurve.value.length)
})
const selectedDisplayCdfCurve = computed(() =>
  buildDisplayCdfCurve(selectedCdfCurve.value, 48),
)
const topKBucketValues = computed(() => selectedTopKBuckets.value.map((bucket: MemoryTopKBucket) => bucket.value ?? 0))
const topKMinValue = computed(() => topKBucketValues.value.length > 0 ? Math.min(...topKBucketValues.value) : 0)
const topKMaxValue = computed(() => topKBucketValues.value.length > 0 ? Math.max(...topKBucketValues.value) : 0)
const denseScores = computed(() => denseSearchResults.value.map((item) => item.score))
const denseScoreMax = computed(() => denseScores.value.length > 0 ? Math.max(...denseScores.value) : 1)
const denseCumulativeSeries = computed(() => {
  if (denseScores.value.length === 0) return []
  const total = denseScores.value.reduce((sum, score) => sum + score, 0)
  if (total <= 0) {
    return denseScores.value.map((_, idx) => [idx + 1, 0])
  }
  let running = 0
  return denseScores.value.map((score, idx) => {
    running += score
    return [idx + 1, running / total]
  })
})

const chartPalette = computed(() => {
  // Depend on theme and palette so echarts colors recalculate on appearance changes.
  void colorMode.value
  void settingsStore.colorScheme
  return {
    tooltipBackground: resolveCssColor('var(--popover)', 'white'),
    tooltipBorder: resolveCssColor('var(--border)', 'rgba(0,0,0,0.12)'),
    tooltipText: resolveCssColor('var(--popover-foreground)', 'CanvasText'),
    axisText: resolveCssColor('var(--muted-foreground)', 'rgba(107,114,128,0.9)'),
    splitLine: resolveCssColor('color-mix(in oklab, var(--muted-foreground) 8%, transparent)', 'rgba(107,114,128,0.12)'),
    topKBar: resolveCssColor('color-mix(in oklab, var(--primary) 16%, transparent)', 'rgba(99,102,241,0.18)'),
    topKBarHover: resolveCssColor('color-mix(in oklab, var(--primary) 26%, transparent)', 'rgba(99,102,241,0.26)'),
    cdfLine: resolveCssColor('color-mix(in oklab, var(--primary) 34%, var(--foreground) 12%)', 'rgba(99,102,241,0.46)'),
    cdfArea: resolveCssColor('color-mix(in oklab, var(--primary) 8%, transparent)', 'rgba(99,102,241,0.09)'),
    cdfPointer: resolveCssColor('color-mix(in oklab, var(--primary) 30%, transparent)', 'rgba(99,102,241,0.24)'),
  }
})

const topKChartOption = computed(() => ({
  animation: false,
  grid: {
    left: 34,
    right: 8,
    top: 8,
    bottom: 18,
  },
  tooltip: {
    trigger: 'axis',
    axisPointer: { type: 'shadow' },
    backgroundColor: chartPalette.value.tooltipBackground,
    borderColor: chartPalette.value.tooltipBorder,
    textStyle: { color: chartPalette.value.tooltipText, fontSize: 10 },
    formatter: (params: Array<{ data?: number; dataIndex?: number }>) => {
      const first = params[0]
      const dataIndex = first?.dataIndex ?? -1
      const bucket = selectedTopKBuckets.value[dataIndex]
      if (!bucket) return ''
      const value = bucket.value ?? 0
      return `Index: ${bucket.index ?? dataIndex}<br/>Value: ${Number(value ?? 0).toFixed(6)}`
    },
  },
  xAxis: {
    type: 'category',
    axisLabel: { show: false },
    axisTick: { show: false },
    axisLine: { show: false },
    data: selectedTopKBuckets.value.map((bucket) => String(bucket.index ?? '')),
  },
  yAxis: {
    type: 'value',
    min: topKMinValue.value,
    max: topKMaxValue.value,
    splitNumber: 2,
    axisLabel: {
      color: chartPalette.value.axisText,
      fontSize: 8,
      formatter: (value: number) => Number(value).toFixed(4),
    },
    splitLine: {
      lineStyle: {
        color: chartPalette.value.splitLine,
      },
    },
  },
  series: [
    {
      type: 'bar',
      data: selectedTopKBuckets.value.map((bucket) => bucket.value ?? 0),
      barGap: '10%',
      barCategoryGap: '20%',
      itemStyle: {
        color: chartPalette.value.topKBar,
        borderRadius: [2, 2, 0, 0],
      },
      emphasis: {
        itemStyle: {
          color: chartPalette.value.topKBarHover,
        },
      },
    },
  ],
}))

const denseSimilarityChartOption = computed(() => ({
  animation: false,
  grid: {
    left: 34,
    right: 8,
    top: 8,
    bottom: 18,
  },
  tooltip: {
    trigger: 'axis',
    axisPointer: { type: 'shadow' },
    backgroundColor: chartPalette.value.tooltipBackground,
    borderColor: chartPalette.value.tooltipBorder,
    textStyle: { color: chartPalette.value.tooltipText, fontSize: 10 },
    formatter: (params: Array<{ data?: number; dataIndex?: number }>) => {
      const first = params[0]
      const dataIndex = first?.dataIndex ?? -1
      const item = denseSearchResults.value[dataIndex]
      if (!item) return ''
      return `Rank: ${dataIndex + 1}<br/>Score: ${Number(item.score ?? 0).toFixed(6)}`
    },
  },
  xAxis: {
    type: 'category',
    axisLabel: {
      color: chartPalette.value.axisText,
      fontSize: 8,
      formatter: (value: string) => value,
    },
    axisTick: { show: false },
    axisLine: { show: false },
    data: denseSearchResults.value.map((_, idx) => `#${idx + 1}`),
  },
  yAxis: {
    type: 'value',
    min: 0,
    max: denseScoreMax.value,
    splitNumber: 3,
    axisLabel: {
      color: chartPalette.value.axisText,
      fontSize: 8,
      formatter: (value: number) => Number(value).toFixed(2),
    },
    splitLine: {
      lineStyle: {
        color: chartPalette.value.splitLine,
      },
    },
  },
  series: [
    {
      type: 'bar',
      data: denseSearchResults.value.map((item) => item.score),
      itemStyle: {
        color: chartPalette.value.topKBarHover,
        borderRadius: [2, 2, 0, 0],
      },
      emphasis: {
        itemStyle: {
          color: chartPalette.value.cdfLine,
        },
      },
    },
  ],
}))

const cdfChartOption = computed(() => ({
  animation: false,
  grid: {
    left: 32,
    right: 8,
    top: 8,
    bottom: 18,
  },
  tooltip: {
    trigger: 'axis',
    axisPointer: {
      type: 'line',
      lineStyle: {
        color: chartPalette.value.cdfPointer,
        type: 'dashed',
      },
    },
    backgroundColor: chartPalette.value.tooltipBackground,
    borderColor: chartPalette.value.tooltipBorder,
    textStyle: { color: chartPalette.value.tooltipText, fontSize: 10 },
    formatter: (params: Array<{ data?: [number, number] }>) => {
      const first = params[0]
      const point = first?.data
      if (!point) return ''
      const [k, cumulative] = point
      return `K: ${k}<br/>P: ${Number(cumulative ?? 0).toFixed(6)}`
    },
  },
  xAxis: {
    type: 'value',
    min: 0,
    max: selectedCdfMaxK.value,
    axisLabel: {
      color: chartPalette.value.axisText,
      fontSize: 8,
      formatter: (value: number) => {
        if (value === 0) return 'k=0'
        if (value === selectedCdfMaxK.value) return `k=${selectedCdfMaxK.value}`
        return ''
      },
    },
    splitLine: { show: false },
  },
  yAxis: {
    type: 'value',
    min: 0,
    max: 1,
    splitNumber: 2,
    axisLabel: {
      color: chartPalette.value.axisText,
      fontSize: 8,
      formatter: (value: number) => Number(value).toFixed(1),
    },
    splitLine: {
      lineStyle: {
        color: chartPalette.value.splitLine,
      },
    },
  },
  series: [
    {
      type: 'line',
      smooth: 0.2,
      smoothMonotone: 'x',
      connectNulls: true,
      showSymbol: false,
      hoverAnimation: false,
      symbol: 'circle',
      symbolSize: 6,
      sampling: 'lttb',
      data: selectedDisplayCdfCurve.value.map((point) => [point.k ?? 0, point.cumulative ?? 0]),
      lineStyle: {
        width: 1.25,
        color: chartPalette.value.cdfLine,
      },
      areaStyle: {
        color: chartPalette.value.cdfArea,
      },
      emphasis: {
        disabled: true,
      },
    },
  ],
}))

const denseCumulativeChartOption = computed(() => ({
  animation: false,
  grid: {
    left: 32,
    right: 8,
    top: 8,
    bottom: 18,
  },
  tooltip: {
    trigger: 'axis',
    axisPointer: {
      type: 'line',
      lineStyle: {
        color: chartPalette.value.cdfPointer,
        type: 'dashed',
      },
    },
    backgroundColor: chartPalette.value.tooltipBackground,
    borderColor: chartPalette.value.tooltipBorder,
    textStyle: { color: chartPalette.value.tooltipText, fontSize: 10 },
    formatter: (params: Array<{ data?: [number, number] }>) => {
      const first = params[0]
      const point = first?.data
      if (!point) return ''
      const [rank, cumulative] = point
      return `Rank: ${rank}<br/>Cumulative: ${Number(cumulative ?? 0).toFixed(6)}`
    },
  },
  xAxis: {
    type: 'value',
    min: 1,
    max: Math.max(1, denseSearchResults.value.length),
    axisLabel: {
      color: chartPalette.value.axisText,
      fontSize: 8,
      formatter: (value: number) => {
        if (value === 1) return '#1'
        if (value === denseSearchResults.value.length) return `#${denseSearchResults.value.length}`
        return ''
      },
    },
    splitLine: { show: false },
  },
  yAxis: {
    type: 'value',
    min: 0,
    max: 1,
    splitNumber: 2,
    axisLabel: {
      color: chartPalette.value.axisText,
      fontSize: 8,
      formatter: (value: number) => Number(value).toFixed(1),
    },
    splitLine: {
      lineStyle: {
        color: chartPalette.value.splitLine,
      },
    },
  },
  series: [
    {
      type: 'line',
      smooth: 0.15,
      smoothMonotone: 'x',
      showSymbol: false,
      hoverAnimation: false,
      data: denseCumulativeSeries.value,
      lineStyle: {
        width: 1.25,
        color: chartPalette.value.cdfLine,
      },
      areaStyle: {
        color: chartPalette.value.cdfArea,
      },
      emphasis: {
        disabled: true,
      },
    },
  ],
}))

const chartLeftTitle = computed(() =>
  isDenseMode.value ? 'Top-K Similarity' : 'Top-K Bucket',
)
const chartRightTitle = computed(() =>
  isDenseMode.value ? 'Cumulative Similarity' : 'Energy Gradient (CDF)',
)
const chartLeftOption = computed(() =>
  isDenseMode.value ? denseSimilarityChartOption.value : topKChartOption.value,
)
const chartRightOption = computed(() =>
  isDenseMode.value ? denseCumulativeChartOption.value : cdfChartOption.value,
)

const compactDecayDays = computed(() => {
  if (!compactDecayDate.value) return 0
  const selected = new Date(compactDecayDate.value)
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  selected.setHours(0, 0, 0, 0)
  const diffTime = today.getTime() - selected.getTime()
  const diffDays = Math.floor(diffTime / (1000 * 60 * 60 * 24))
  return diffDays > 0 ? diffDays : 0
})

const filteredMemories = computed(() => {
  const query = searchQuery.value.toLowerCase().trim()
  let list = [...memories.value]

  // Sort by created_at descending
  list.sort((a, b) => {
    const timeA = a.created_at ? new Date(a.created_at).getTime() : 0
    const timeB = b.created_at ? new Date(b.created_at).getTime() : 0
    return timeB - timeA
  })

  if (!query) return list
  return list.filter(
    (m) => m.id.toLowerCase().includes(query) || m.memory.toLowerCase().includes(query),
  )
})

const selectedMemory = computed(() =>
  memories.value.find((m) => m.id === selectedId.value) ?? null,
)

const isDirty = computed(() => editContent.value !== originalContent.value)

async function loadMemories() {
  loading.value = true
  try {
    const { data } = await getBotsByBotIdMemory({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    memories.value = (data.results ?? [])
      .filter((item): item is AdaptersMemoryItem & { id: string; memory: string } =>
        typeof item?.id === 'string' && item.id.length > 0 && typeof item.memory === 'string',
      )
      .map((item) => ({
        id: item.id,
        memory: item.memory,
        created_at: item.created_at,
        updated_at: item.updated_at,
        hash: item.hash,
        score: item.score,
        cdf_curve: item.cdf_curve ?? [],
        top_k_buckets: item.top_k_buckets ?? [],
      }))
  } catch (error) {
    console.error('Failed to load memories:', error)
    toast.error(t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function loadMemoryStatus() {
  try {
    const { data } = await getBotsByBotIdMemoryStatus({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    memoryStatus.value = data ?? null
  } catch (error) {
    console.error('Failed to load memory status:', error)
    memoryStatus.value = null
  }
}

async function loadDenseSearchDiagnostics(memory: MemoryItem | null) {
  if (!memory || !isDenseMode.value) {
    denseSearchResults.value = []
    return
  }
  denseSearchLoading.value = true
  try {
    const { data } = await postBotsByBotIdMemorySearch({
      path: { bot_id: props.botId },
      body: {
        query: memory.memory,
        limit: 8,
      },
      throwOnError: true,
    })
    denseSearchResults.value = (data.results ?? [])
      .filter((item): item is AdaptersMemoryItem & { id: string; memory: string; score: number } =>
        typeof item?.id === 'string'
        && typeof item.memory === 'string'
        && typeof item.score === 'number',
      )
      .map((item) => ({
        id: item.id,
        memory: item.memory,
        score: item.score,
      }))
  } catch (error) {
    console.error('Failed to load dense diagnostics:', error)
    denseSearchResults.value = []
  } finally {
    denseSearchLoading.value = false
  }
}

function selectMemory(item: MemoryItem) {
  selectedId.value = item.id
  editContent.value = item.memory
  originalContent.value = item.memory
}

function openNewMemoryDialog() {
  newMemoryContent.value = ''
  selectedHistoryMessages.value = []
  historyMessages.value = []
  newMemoryDialogOpen.value = true
  loadHistory()
}

async function loadHistory() {
  historyLoading.value = true
  try {
    const { data } = await getBotsByBotIdMessages({
      path: { bot_id: props.botId },
      query: { limit: 50 },
      throwOnError: true,
    })
    historyMessages.value = (data.items ?? []).map((item: MessageMessage) => ({
      role: item.role ?? 'assistant',
      content: item.content,
      created_at: item.created_at,
    }))
  } catch (error) {
    console.error('Failed to load history:', error)
    toast.error('Failed to load history')
  } finally {
    historyLoading.value = false
  }
}

function toggleMessageSelection(msg: Message) {
  const idx = selectedHistoryMessages.value.indexOf(msg)
  if (idx > -1) {
    selectedHistoryMessages.value.splice(idx, 1)
  } else {
    selectedHistoryMessages.value.push(msg)
  }

  // Update content
  newMemoryContent.value = selectedHistoryMessages.value
    .map(m => {
      const text = extractMessageText(m.content)
      return `[${m.role.toUpperCase()}]: ${text}`
    })
    .join('\n\n')
}

async function handleCreateMemory() {
  if (!newMemoryContent.value.trim()) return

  actionLoading.value = true
  try {
    await postBotsByBotIdMemory({
      path: { bot_id: props.botId },
      body: {
        message: newMemoryContent.value,
      },
      throwOnError: true,
    })

    toast.success(t('common.add'))
    newMemoryDialogOpen.value = false
    await loadMemories()

    const first = memories.value[0]
    if (first) selectMemory(first)
  } catch (error) {
    console.error('Failed to create memory:', error)
    toast.error(t('common.saveFailed'))
  } finally {
    actionLoading.value = false
  }
}

async function handleSave() {
  if (!editContent.value.trim() || !selectedId.value) return

  actionLoading.value = true
  try {
    // Delete old
    await deleteBotsByBotIdMemoryById({
      path: { bot_id: props.botId, id: selectedId.value },
      throwOnError: true,
    })

    // Add new
    await postBotsByBotIdMemory({
      path: { bot_id: props.botId },
      body: {
        message: editContent.value,
      },
      throwOnError: true,
    })

    toast.success(t('common.save'))
    await loadMemories()

    const first = memories.value[0]
    if (first) selectMemory(first)
  } catch (error) {
    console.error('Failed to save memory:', error)
    toast.error(t('common.saveFailed'))
  } finally {
    actionLoading.value = false
  }
}

async function handleDelete() {
  if (!selectedId.value) return

  actionLoading.value = true
  try {
    await deleteBotsByBotIdMemoryById({
      path: { bot_id: props.botId, id: selectedId.value },
      throwOnError: true,
    })
    toast.success(t('common.delete'))
    selectedId.value = null
    editContent.value = ''
    originalContent.value = ''
    await loadMemories()
  } catch (error) {
    console.error('Failed to delete memory:', error)
    toast.error(t('common.delete'))
  } finally {
    actionLoading.value = false
  }
}

function openCompactDialog() {
  compactRatio.value = '0.5'
  compactDecayDate.value = ''
  compactDialogOpen.value = true
}

async function handleCompact() {
  compactLoading.value = true
  try {
    await postBotsByBotIdMemoryCompact({
      path: { bot_id: props.botId },
      body: {
        ratio: parseFloat(compactRatio.value),
        decay_days: compactDecayDays.value || undefined,
      },
      throwOnError: true,
    })
    toast.success(t('bots.memory.compactSuccess'))
    compactDialogOpen.value = false
    await loadMemories()
    selectedId.value = null
  } catch (error) {
    console.error('Failed to compact memory:', error)
    toast.error(t('bots.memory.compactFailed'))
  } finally {
    compactLoading.value = false
  }
}

function formatDate(dateStr?: string) {
  return formatDateTimeSeconds(dateStr, { fallback: 'Unknown' })
}

async function copyToClipboard(text: string) {
  try {
    const copied = await copyText(text)
    if (!copied) throw new Error('copy failed')
    toast.success(t('bots.memory.idCopied'))
  } catch (err) {
    console.error('Failed to copy:', err)
    toast.error('Failed to copy')
  }
}

onMounted(() => {
  loadMemories()
  loadMemoryStatus()
})

watch(() => props.botId, () => {
  memories.value = []
  selectedId.value = null
  denseSearchResults.value = []
  loadMemories()
  loadMemoryStatus()
})

watch([selectedMemory, isDenseMode], ([memory, dense]) => {
  if (!dense) {
    denseSearchResults.value = []
    return
  }
  loadDenseSearchDiagnostics(memory)
})

function buildDisplayCdfCurve(data: MemoryCdfPoint[], maxPoints: number) {
  if (!data || data.length === 0) return []
  const withOrigin: MemoryCdfPoint[] = [{ k: 0, cumulative: 0 }, ...data]
  if (withOrigin.length <= maxPoints) return withOrigin

  const firstPoint = withOrigin[0]
  const lastPoint = withOrigin[withOrigin.length - 1]
  if (!firstPoint || !lastPoint) return []
  const targets = buildCdfSamplingTargets(maxPoints)
  const sampled: MemoryCdfPoint[] = []

  let sourceIdx = 0
  for (const target of targets) {
    while (sourceIdx < withOrigin.length - 1 && (withOrigin[sourceIdx]?.cumulative ?? 0) < target) {
      sourceIdx++
    }
    const point = withOrigin[sourceIdx]
    if (!point) continue
    if (sampled[sampled.length - 1]?.k !== point.k) {
      sampled.push(point)
    }
  }

  if (sampled[sampled.length - 1]?.k !== lastPoint.k) {
    sampled.push(lastPoint)
  }
  return sampled
}

function buildCdfSamplingTargets(maxPoints: number) {
  const clamped = Math.max(8, maxPoints)
  const targets: number[] = [0]
  const fineCount = Math.max(4, Math.floor(clamped * 0.45))
  const mediumCount = Math.max(3, Math.floor(clamped * 0.35))
  const tailCount = Math.max(2, clamped - fineCount - mediumCount - 1)

  for (let i = 1; i <= fineCount; i++) {
    targets.push((0.5 * i) / fineCount)
  }
  for (let i = 1; i <= mediumCount; i++) {
    targets.push(0.5 + (0.4 * i) / mediumCount)
  }
  for (let i = 1; i <= tailCount; i++) {
    targets.push(0.9 + (0.1 * i) / tailCount)
  }

  return Array.from(new Set(targets.map(target => Number(target.toFixed(6))))).sort((a, b) => a - b)
}

function resolveCssColor(input: string, fallback: string) {
  if (typeof document === 'undefined') return fallback
  const el = document.createElement('span')
  el.style.color = input
  el.style.display = 'none'
  document.body.appendChild(el)
  const resolved = window.getComputedStyle(el).color
  el.remove()
  return resolved && resolved !== input ? resolved : fallback
}
</script>
