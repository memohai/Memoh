<template>
  <div class="flex gap-6 min-h-[500px] h-[calc(100vh-300px)] max-w-6xl mx-auto">
    <!-- Left: File list -->
    <div class="w-64 shrink-0 flex flex-col border rounded-lg overflow-hidden bg-card max-h-full">
      <div class="p-3 border-b space-y-3 shrink-0">
        <div class="flex items-center justify-between">
          <h4 class="text-sm font-medium">
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
              <FontAwesomeIcon
                :icon="['fas', 'brain']"
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
              <FontAwesomeIcon
                :icon="['fas', 'rotate']"
                :class="{ 'animate-spin': loading }"
                class="size-3.5"
              />
            </Button>
          </div>
        </div>
        <div class="relative">
          <FontAwesomeIcon
            :icon="['fas', 'magnifying-glass']"
            class="absolute left-2.5 top-1/2 -translate-y-1/2 size-3 text-muted-foreground"
          />
          <Input
            v-model="searchQuery"
            :placeholder="$t('bots.memory.searchPlaceholder')"
            class="pl-8 h-8 text-xs"
          />
        </div>
      </div>

      <ScrollArea class="flex-1 min-h-0">
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
              <FontAwesomeIcon
                :icon="['fas', 'file-lines']"
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
          <FontAwesomeIcon
            :icon="['fas', 'plus']"
            class="mr-2 size-3"
          />
          {{ $t('bots.memory.newMemory') }}
        </Button>
      </div>
    </div>

    <!-- Right: Editor/Preview -->
    <div class="flex-1 flex flex-col border rounded-lg overflow-hidden bg-card">
      <template v-if="selectedMemory">
        <div class="flex-1 flex flex-col min-h-0">
          <div class="p-3 border-b flex items-center justify-between bg-muted/30 shrink-0">
            <div class="flex items-center gap-3 min-w-0">
              <FontAwesomeIcon
                :icon="['fas', 'file-lines']"
                class="size-4 text-muted-foreground shrink-0"
              />
              <div class="min-w-0">
                <h4 class="text-sm font-medium truncate">
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
                    <FontAwesomeIcon
                      :icon="['fas', 'copy']"
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
                    <FontAwesomeIcon
                      :icon="['far', 'trash-can']"
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
              class="absolute inset-0 resize-none border-0 rounded-none focus-visible:ring-0 font-mono text-sm p-4 h-full"
              placeholder="Write your memory content here (Markdown)..."
            />
          </div>
        </div>

        <!-- Charts Section -->
        <div class="h-[240px] border-t flex flex-col bg-muted/5 shrink-0">
          <div class="px-3 py-1.5 border-b bg-muted/10 flex items-center justify-between shrink-0">
            <h5 class="text-[10px] font-bold uppercase tracking-wider text-muted-foreground/70">
              Vector Manifold
            </h5>
          </div>
          <div class="flex-1 flex min-h-0 divide-x overflow-hidden">
            <!-- Top K Buckets (Bar Chart) -->
            <div class="flex-1 flex flex-col p-3 min-w-0">
              <p class="text-[9px] font-semibold text-muted-foreground/60 mb-2 uppercase shrink-0">
                Top-K Bucket
              </p>
              <div class="flex-1 flex items-end gap-0.5 relative group min-h-0 pt-2 pb-4">
                <div
                  v-for="(bucket, idx) in selectedTopKBuckets"
                  :key="idx"
                  class="flex-1 bg-primary/25 hover:bg-primary/50 transition-colors relative group/bar"
                  :style="{ height: `${topKBarHeights[idx]}%` }"
                >
                  <!-- Tooltip for Bar -->
                  <div class="absolute z-20 bottom-full left-1/2 -translate-x-1/2 mb-1 bg-popover border text-popover-foreground px-2 py-1 rounded shadow-lg text-[10px] hidden group-hover/bar:block whitespace-nowrap pointer-events-none">
                    <p class="font-bold text-primary">
                      Index: {{ bucket.index }}
                    </p>
                    <p>Value: {{ bucket.value.toFixed(6) }}</p>
                  </div>
                </div>
                <!-- Axis labels (showing actual range) -->
                <div class="absolute left-[-2px] top-2 bottom-4 border-l border-muted-foreground/10 flex flex-col justify-between text-[8px] font-mono text-muted-foreground/40 pr-1">
                  <span>{{ topKMaxValue.toFixed(4) }}</span>
                  <span>{{ topKMinValue.toFixed(4) }}</span>
                </div>
              </div>
            </div>

            <!-- CDF Curve (Line Chart) -->
            <div class="flex-1 flex flex-col p-3 min-w-0">
              <p class="text-[9px] font-semibold text-muted-foreground/60 mb-2 uppercase shrink-0">
                Energy Gradient (CDF)
              </p>
              <div class="flex-1 relative min-h-0 pt-2 pb-4 group/cdf">
                <svg
                  class="w-full h-full overflow-visible"
                  viewBox="0 0 100 100"
                  preserveAspectRatio="none"
                >
                  <!-- Grid lines -->
                  <line
                    x1="0"
                    y1="50"
                    x2="100"
                    y2="50"
                    stroke="currentColor"
                    class="text-muted-foreground/5"
                    stroke-width="0.5"
                  />
                  
                  <!-- Area under curve -->
                  <path
                    :d="generateSmoothPath(selectedMemory.cdf_curve, true)"
                    fill="currentColor"
                    class="text-primary/10"
                  />
                  <!-- Curve -->
                  <path
                    :d="generateSmoothPath(selectedMemory.cdf_curve)"
                    fill="none"
                    stroke="currentColor"
                    class="text-primary"
                    stroke-width="1.5"
                    stroke-linecap="round"
                  />
                  <!-- Interaction vertical line -->
                  <line
                    v-if="hoveredCdfPoint"
                    :x1="hoveredCdfX"
                    y1="0"
                    :x2="hoveredCdfX"
                    y2="100"
                    stroke="currentColor"
                    class="text-primary/20"
                    stroke-width="0.5"
                    stroke-dasharray="2,2"
                  />
                  <!-- Interaction vertical area (Hit area) -->
                  <rect
                    v-for="(point, idx) in selectedMemory.cdf_curve"
                    :key="'hit-' + idx"
                    :x="(idx / (selectedMemory.cdf_curve.length - 1)) * 100 - 2"
                    y="0"
                    width="4"
                    height="100"
                    fill="transparent"
                    class="cursor-crosshair pointer-events-auto"
                    @mouseenter="hoveredCdfPoint = point; hoveredCdfIdx = idx"
                    @mouseleave="hoveredCdfPoint = null"
                  />
                </svg>
                
                <!-- Fixed aspect ratio markers overlay -->
                <div class="absolute inset-0 pointer-events-none pt-2 pb-4">
                  <div
                    v-for="(point, idx) in selectedMemory.cdf_curve"
                    :key="'dot-' + idx"
                    class="absolute size-2 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-background transition-transform"
                    :class="hoveredCdfIdx === idx && hoveredCdfPoint ? 'bg-primary scale-125 z-10' : 'bg-primary/60 scale-100'"
                    :style="{ 
                      left: `${(idx / (selectedMemory.cdf_curve.length - 1)) * 100}%`,
                      top: `${(100 - 5) - (point.cumulative * 90)}%`
                    }"
                  />
                </div>
                
                <!-- Tooltip -->
                <div
                  v-if="hoveredCdfPoint"
                  class="absolute z-30 bg-popover border text-popover-foreground px-2 py-1 rounded shadow-xl text-[10px] pointer-events-none whitespace-nowrap"
                  :style="{ 
                    left: `${Math.min(Math.max(hoveredCdfX, 15), 85)}%`, 
                    top: `${Math.min(Math.max(hoveredCdfY, 15), 85)}%`,
                    transform: 'translate(-50%, -140%)'
                  }"
                >
                  <p class="font-bold text-primary">
                    K: {{ hoveredCdfPoint.k }}
                  </p>
                  <p class="font-mono">
                    P: {{ hoveredCdfPoint.cumulative.toFixed(6) }}
                  </p>
                </div>
                
                <!-- Axis labels -->
                <div class="absolute left-[-2px] top-2 bottom-4 border-l border-muted-foreground/10 flex flex-col justify-between text-[8px] font-mono text-muted-foreground/40 pr-1">
                  <span>1.0</span>
                  <span>0.0</span>
                </div>
                <div class="absolute bottom-0 left-0 right-0 flex justify-between text-[8px] font-mono text-muted-foreground/40 px-1">
                  <span>k=1</span>
                  <span>k={{ selectedMemory.cdf_curve.length }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>
      <div
        v-else
        class="flex-1 flex flex-col items-center justify-center text-muted-foreground p-8 text-center"
      >
        <div class="size-12 rounded-full bg-muted flex items-center justify-center mb-4">
          <FontAwesomeIcon
            :icon="['fas', 'brain']"
            class="size-6 opacity-20"
          />
        </div>
        <h3 class="text-sm font-medium text-foreground">
          {{ $t('bots.memory.title') }}
        </h3>
        <p class="text-xs mt-1 max-w-[240px]">
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
              <FontAwesomeIcon
                :icon="['fas', 'rotate']"
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
                  <FontAwesomeIcon
                    v-if="selectedHistoryMessages.includes(msg)"
                    :icon="['fas', 'check']"
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
                  <p class="text-xs text-foreground break-words line-clamp-3">
                    {{ msg.content?.text || (typeof msg.content === 'string' ? msg.content : JSON.stringify(msg.content)) }}
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
          <p class="text-sm text-muted-foreground">
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
                  <p class="text-sm font-medium">{{ $t('bots.memory.compactRatioLight') }}</p>
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
                  <p class="text-sm font-medium">{{ $t('bots.memory.compactRatioMedium') }}</p>
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
                  <p class="text-sm font-medium">{{ $t('bots.memory.compactRatioAggressive') }}</p>
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
import { computed, ref, onMounted, watch } from 'vue'
import {
  Button,
  Input,
  ScrollArea,
  Separator,
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
} from '@memoh/ui'
import {
  getBotsByBotIdMemory,
  postBotsByBotIdMemory,
  deleteBotsByBotIdMemoryById,
  postBotsByBotIdMemoryCompact,
  getBotsByBotIdMessages,
} from '@memoh/sdk'
import { toast } from 'vue-sonner'
import { useI18n } from 'vue-i18n'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import { useClipboard } from '@/composables/useClipboard'
import { formatDateTimeSeconds } from '@/utils/date-time'

interface MemoryItem {
  id: string
  memory: string
  created_at?: string
  updated_at?: string
  hash?: string
  score?: number
}

interface Message {
  role: string
  content: any
  created_at?: string
}

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()
const { copyText } = useClipboard()
const loading = ref(false)
const actionLoading = ref(false)
const compactLoading = ref(false)
const memories = ref<MemoryItem[]>([])
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

// Hover state for CDF chart
const hoveredCdfPoint = ref<any>(null)
const hoveredCdfIdx = ref<number>(-1)
const hoveredCdfX = computed(() => {
  if (!hoveredCdfPoint.value || !selectedMemory.value) return 0
  const len = selectedMemory.value.cdf_curve?.length || 1
  return (hoveredCdfIdx.value / (len - 1)) * 100
})
const hoveredCdfY = computed(() => {
  if (!hoveredCdfPoint.value) return 0
  return (100 - 5) - (hoveredCdfPoint.value.cumulative * 90)
})

const selectedTopKBuckets = computed(() => selectedMemory.value?.top_k_buckets ?? [])
const topKBucketValues = computed(() => selectedTopKBuckets.value.map((bucket: any) => bucket.value))
const topKMinValue = computed(() => Math.min(...topKBucketValues.value))
const topKMaxValue = computed(() => Math.max(...topKBucketValues.value))
const topKRange = computed(() => (topKMaxValue.value - topKMinValue.value) || 1)
const topKBarHeights = computed(() =>
  selectedTopKBuckets.value.map(
    (bucket: any) => (((bucket.value - topKMinValue.value) / topKRange.value) * 80) + 20,
  ),
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
    memories.value = data.results ?? []
  } catch (error) {
    console.error('Failed to load memories:', error)
    toast.error(t('common.loadFailed'))
  } finally {
    loading.value = false
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
    historyMessages.value = data.items ?? []
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
      const text = m.content?.text || (typeof m.content === 'string' ? m.content : JSON.stringify(m.content))
      return `[${m.role.toUpperCase()}]: ${text}`
    })
    .join('\n\n')
}

async function handleCreateMemory() {
  if (!newMemoryContent.value.trim()) return

  actionLoading.value = true
  try {
    const { data } = await postBotsByBotIdMemory({
      path: { bot_id: props.botId },
      body: {
        message: newMemoryContent.value,
      },
      throwOnError: true,
    })

    toast.success(t('common.add'))
    newMemoryDialogOpen.value = false
    await loadMemories()

    if (data.results && data.results.length > 0) {
      selectMemory(data.results[0])
    }
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
    const { data } = await postBotsByBotIdMemory({
      path: { bot_id: props.botId },
      body: {
        message: editContent.value,
      },
      throwOnError: true,
    })

    toast.success(t('common.save'))
    await loadMemories()

    if (data.results && data.results.length > 0) {
      selectMemory(data.results[0])
    }
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
      } as any,
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
})

watch(() => props.botId, () => {
  memories.value = []
  selectedId.value = null
  loadMemories()
})

// Chart Helper: Generate smooth SVG path
function generateSmoothPath(data: any[], closePath: boolean = false) {
  if (!data || data.length < 2) return ''
  
  // Use a small margin (2%) to prevent clipping at boundaries
  const margin = 2
  const height = 100 - (margin * 2)
  const points = data.map((p, idx) => ({
    x: (idx / (data.length - 1)) * 100,
    y: (100 - margin) - (p.cumulative * height)
  }))

  let d = `M ${points[0].x},${points[0].y}`
  
  for (let i = 0; i < points.length - 1; i++) {
    const p0 = points[i === 0 ? i : i - 1]
    const p1 = points[i]
    const p2 = points[i + 1]
    const p3 = points[i + 2] || p2

    // Catmull-Rom to Bezier conversion factors
    const cp1x = p1.x + (p2.x - p0.x) / 6
    const cp1y = p1.y + (p2.y - p0.y) / 6
    const cp2x = p2.x - (p3.x - p1.x) / 6
    const cp2y = p2.y - (p3.y - p1.y) / 6

    d += ` C ${cp1x},${cp1y} ${cp2x},${cp2y} ${p2.x},${p2.y}`
  }

  if (closePath) {
    d += ' L 100,100 L 0,100 Z'
  }

  return d
}
</script>
