<template>
  <div class="flex gap-6 h-full absolute inset-0 px-4 pt-4 pb-6 w-full max-w-4xl mx-auto">
    <!-- Left: File list -->
    <div class="w-60 shrink-0 flex flex-col border rounded-lg overflow-hidden max-h-full bg-background shadow-sm">
      <div class="p-3 pb-2 border-b space-y-3 shrink-0">
        <div class="flex items-center justify-between">
          <h4 class="text-xs font-medium">
            {{ $t('bots.memory.files') }}
          </h4>
          <div class="flex items-center gap-1">
            <Popover v-model:open="compactPopoverOpen">
              <PopoverAnchor as-child>
                <div class="inline-block">
                  <TooltipProvider>
                    <Tooltip
                      :delay-duration="300"
                      :open="compactPopoverOpen ? false : undefined"
                    >
                      <TooltipTrigger as-child>
                        <Button
                          variant="ghost"
                          size="sm"
                          type="button"
                          class="size-8 p-0 hover:bg-muted/50 group"
                          :disabled="loading || compactLoading || memories.length === 0"
                          :aria-label="$t('bots.memory.compact')"
                          @click="openCompactDialog"
                        >
                          <Brain class="size-3.5 text-foreground/70 group-hover:text-foreground transition-colors" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent
                        side="bottom"
                        align="center"
                      >
                        <p class="text-[11px]">
                          {{ $t('bots.memory.compact') }}
                        </p>
                      </TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                </div>
              </PopoverAnchor>

              <PopoverContent
                side="bottom"
                align="start"
                class="w-72 p-3 flex flex-col gap-3 shadow-md"
                :side-offset="4"
              >
                <div class="space-y-1">
                  <h4 class="text-xs font-semibold text-foreground leading-none">
                    {{ $t('bots.memory.compact') }}
                  </h4>
                  <p class="text-[10px] text-muted-foreground leading-snug">
                    {{ $t('bots.memory.compactConfirm') }}
                  </p>
                </div>

                <div class="space-y-1.5">
                  <Label class="text-[9px] font-semibold text-muted-foreground uppercase tracking-wider">{{ $t('bots.memory.compactRatio') }}</Label>
                  <RadioGroup
                    v-model="compactRatio"
                    class="grid grid-cols-1 gap-1"
                  >
                    <Label
                      class="flex items-center gap-2.5 p-1.5 rounded-md border cursor-pointer hover:bg-muted/40 transition-colors"
                      :class="{ 'bg-muted/50 border-foreground/40': compactRatio === '0.8' }"
                    >
                      <RadioGroupItem
                        value="0.8"
                        class="size-3"
                      />
                      <Zap class="size-3.5 text-muted-foreground shrink-0" />
                      <div class="min-w-0">
                        <p class="text-[10px] font-medium text-foreground leading-none">{{ $t('bots.memory.compactRatioLight') }}</p>
                      </div>
                    </Label>
                    <Label
                      class="flex items-center gap-2.5 p-1.5 rounded-md border cursor-pointer hover:bg-muted/40 transition-colors"
                      :class="{ 'bg-muted/50 border-foreground/40': compactRatio === '0.5' }"
                    >
                      <RadioGroupItem
                        value="0.5"
                        class="size-3"
                      />
                      <BrainCircuit class="size-3.5 text-muted-foreground shrink-0" />
                      <div class="min-w-0">
                        <p class="text-[10px] font-medium text-foreground leading-none">{{ $t('bots.memory.compactRatioMedium') }}</p>
                      </div>
                    </Label>
                    <Label
                      class="flex items-center gap-2.5 p-1.5 rounded-md border cursor-pointer hover:bg-muted/40 transition-colors"
                      :class="{ 'bg-muted/50 border-foreground/40': compactRatio === '0.3' }"
                    >
                      <RadioGroupItem
                        value="0.3"
                        class="size-3"
                      />
                      <Brain class="size-3.5 text-muted-foreground shrink-0" />
                      <div class="min-w-0">
                        <p class="text-[10px] font-medium text-foreground leading-none">{{ $t('bots.memory.compactRatioAggressive') }}</p>
                      </div>
                    </Label>
                  </RadioGroup>
                </div>

                <div class="space-y-1.5">
                  <Label class="text-[9px] font-semibold text-muted-foreground uppercase tracking-wider">
                    {{ $t('bots.memory.compactDecayDate') }}
                    <span class="text-muted-foreground/60 normal-case tracking-normal">({{ $t('common.optional') }})</span>
                  </Label>
                  <Input
                    v-model="compactDecayDate"
                    type="date"
                    class="w-full h-7 text-[10px] px-2 shadow-none border-border"
                  />
                  <p
                    v-if="compactDecayDays > 0"
                    class="text-[9px] text-muted-foreground"
                  >
                    Calculated: {{ compactDecayDays }} days old
                  </p>
                </div>

                <div class="flex items-center justify-end gap-2 pt-2 mt-1 border-t">
                  <Button
                    variant="ghost"
                    size="sm"
                    class="h-6 text-[10px] font-medium px-2 shadow-none"
                    @click="compactPopoverOpen = false"
                  >
                    {{ $t('common.cancel') }}
                  </Button>
                  <Button
                    size="sm"
                    class="h-6 text-[10px] font-medium px-3 shadow-none"
                    :disabled="compactLoading"
                    @click="handleCompact"
                  >
                    <Spinner
                      v-if="compactLoading"
                      class="mr-1 size-2.5"
                    />
                    {{ $t('common.confirm') }}
                  </Button>
                </div>
              </PopoverContent>
            </Popover>

            <TooltipProvider>
              <Tooltip :delay-duration="300">
                <TooltipTrigger as-child>
                  <Button
                    variant="ghost"
                    size="sm"
                    type="button"
                    class="size-8 p-0 hover:bg-muted/50 group"
                    :disabled="loading"
                    :aria-label="$t('common.refresh')"
                    @click="loadMemories"
                  >
                    <RefreshCw
                      :class="{ 'animate-spin': loading }"
                      class="size-3.5 text-foreground/70 group-hover:text-foreground transition-colors"
                    />
                  </Button>
                </TooltipTrigger>
                <TooltipContent
                  side="bottom"
                  align="center"
                >
                  <p class="text-[11px]">
                    {{ $t('common.refresh') }}
                  </p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </div>
        </div>
        <div class="relative">
          <Search class="absolute left-2.5 top-1/2 -translate-y-1/2 size-3 text-muted-foreground" />
          <Input
            v-model="searchQuery"
            :placeholder="$t('bots.memory.searchPlaceholder')"
            class="pl-8 h-8 text-xs bg-transparent shadow-none"
          />
        </div>
      </div>

      <ScrollArea class="flex-1 min-h-0">
        <div class="p-2 space-y-0.5">
          <template v-if="loading && memories.length === 0">
            <Skeleton class="h-10 w-full rounded-md" />
            <Skeleton class="h-10 w-full rounded-md" />
            <Skeleton class="h-10 w-full rounded-md" />
          </template>
          <div
            v-else-if="filteredMemories.length === 0"
            class="p-4 text-center text-[11px] text-muted-foreground"
          >
            {{ $t('bots.memory.empty') }}
          </div>
          <button
            v-for="item in filteredMemories"
            :key="item.id"
            type="button"
            class="w-full text-left px-3 py-1.5 rounded-md text-xs transition-colors hover:bg-accent/40 group relative"
            :class="{ 'bg-accent/40 font-medium text-foreground': selectedId === item.id, 'text-muted-foreground': selectedId !== item.id }"
            :aria-label="`Open memory ${formatDate(item.created_at)}`"
            @click="selectMemory(item)"
          >
            <div class="flex items-center gap-2">
              <FileText class="size-3 shrink-0 opacity-70" />
              <span class="truncate pr-4 flex-1">{{ formatDate(item.created_at) }}</span>
              <span
                v-if="isDirty && selectedId === item.id"
                class="text-foreground shrink-0 text-xs font-bold leading-none select-none"
              >*</span>
            </div>
            <div
              class="mt-0.5 text-[10px] truncate opacity-70 group-hover:opacity-100"
              :class="{ 'text-muted-foreground': selectedId !== item.id, 'text-foreground/70': selectedId === item.id }"
            >
              {{ item.memory.length > 60 ? item.memory.slice(0, 60) + '...' : item.memory }}
            </div>
          </button>
        </div>
      </ScrollArea>

      <div class="border-t p-2 bg-background shrink-0">
        <button
          class="inline-flex items-center justify-center whitespace-nowrap font-medium transition-all disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none outline-none focus-visible:ring-2 focus-visible:ring-ring/30 cursor-pointer hover:bg-accent bg-transparent rounded-lg gap-1.5 px-3 w-full h-8 text-xs text-muted-foreground hover:text-foreground"
          type="button"
          @click="openNewMemoryDialog"
        >
          <Plus class="mr-2 size-3" />
          {{ $t('bots.memory.newMemory') }}
        </button>
      </div>
    </div>

    <!-- Right: Editor/Preview -->
    <div class="flex-1 flex flex-col border rounded-lg overflow-hidden bg-background shadow-sm">
      <template v-if="selectedMemory">
        <!-- L4 Header -->
        <div class="pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-10 p-4 shrink-0 flex items-start justify-between">
          <div class="flex items-start gap-3 min-w-0">
            <FileText class="size-4 text-muted-foreground shrink-0 mt-0.5" />
            <div class="min-w-0 space-y-0.5">
              <h4 class="text-xs font-medium text-foreground truncate">
                {{ formatDate(selectedMemory.created_at) }}
              </h4>
              <TooltipProvider>
                <Tooltip :delay-duration="300">
                  <TooltipTrigger as-child>
                    <button
                      type="button"
                      class="flex items-center gap-1.5 text-[10px] text-muted-foreground hover:text-foreground font-mono transition-colors outline-none focus-visible:ring-1 focus-visible:ring-ring rounded-sm -ml-1 px-1"
                      @click="copyToClipboard(selectedMemory.id)"
                    >
                      <span class="truncate">ID: {{ selectedMemory.id }}</span>
                      <Copy class="size-2.5 shrink-0" />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent
                    side="bottom"
                    align="start"
                  >
                    <p class="text-[11px]">
                      Click to copy
                    </p>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </div>
          </div>
          <div class="flex items-center shrink-0 gap-3">
            <Transition name="fade">
              <div
                v-if="isDirty"
                class="flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-muted/40 border border-border/50"
              >
                <div class="size-1 rounded-full bg-muted-foreground/40" />
                <span class="text-[10px] text-muted-foreground font-medium whitespace-nowrap">
                  Unsaved
                </span>
              </div>
            </Transition>

            <Button
              size="sm"
              class="h-8 px-4 text-xs font-medium shadow-none min-w-24"
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

        <div class="flex-1 flex flex-col min-h-0 overflow-y-auto">
          <!-- Box-in-Box Editor -->
          <div class="p-4 flex-1 flex flex-col min-h-0">
            <Card class="flex-1 overflow-hidden focus-within:border-foreground/50 transition-colors shadow-none bg-background border min-h-[300px]">
              <CardContent class="p-0 h-full relative">
                <Textarea
                  v-model="editContent"
                  class="absolute inset-0 w-full h-full resize-none border-0 rounded-none focus-visible:ring-0 font-mono text-xs p-4 bg-transparent"
                  placeholder="Write your memory content here (Markdown)..."
                />
              </CardContent>
            </Card>

            <!-- Danger Zone -->
            <div class="pt-8 mt-auto">
              <div class="space-y-4 rounded-md border border-border bg-background p-4 shadow-none">
                <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                  <div class="space-y-0.5">
                    <h4 class="text-xs font-medium text-destructive">
                      Danger Zone
                    </h4>
                    <p class="text-[11px] text-muted-foreground">
                      Deleting this memory cannot be undone. Proceed with caution.
                    </p>
                  </div>
                  <div class="flex justify-end shrink-0">
                    <ConfirmPopover
                      :message="$t('bots.memory.deleteConfirm')"
                      @confirm="handleDelete"
                    >
                      <template #trigger>
                        <Button
                          variant="destructive"
                          class="h-8 text-xs font-medium shadow-none min-w-28"
                          :disabled="actionLoading"
                        >
                          {{ $t('common.delete') }}
                        </Button>
                      </template>
                    </ConfirmPopover>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Charts Section -->
        <div
          v-if="showChartSection"
          class="h-60 border-t flex flex-col bg-muted/5 shrink-0"
        >
          <div class="px-4 py-2 border-b bg-muted/10 flex items-center justify-between shrink-0">
            <h5 class="text-[10px] font-bold uppercase tracking-wider text-muted-foreground/70">
              Vector Manifold
            </h5>
          </div>
          <div class="flex-1 flex min-h-0 overflow-hidden">
            <!-- Sparse: Top K Buckets -->
            <div class="flex-1 flex flex-col p-4 min-w-0">
              <p class="text-[9px] font-semibold text-muted-foreground/60 mb-2 uppercase shrink-0">
                {{ chartLeftTitle }}
              </p>
              <VChart
                class="h-full w-full min-h-0"
                :option="chartLeftOption"
                autoresize
              />
            </div>

            <Separator
              orientation="vertical"
              class="opacity-30 h-auto"
            />

            <!-- Sparse/Dense secondary chart -->
            <div class="flex-1 flex flex-col p-4 min-w-0">
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

      <!-- Empty State -->
      <Empty
        v-else
        class="flex-1 flex flex-col items-center justify-center p-8 bg-muted/5"
      >
        <EmptyMedia>
          <Brain class="size-6 text-muted-foreground opacity-50" />
        </EmptyMedia>
        <EmptyTitle class="text-xs font-medium mt-4">
          {{ $t('bots.memory.title') }}
        </EmptyTitle>
        <EmptyDescription class="text-[11px] mt-1.5 max-w-xs text-center text-muted-foreground">
          Select a file from the sidebar to view or edit, or create a new one to persist long-term information for your bot.
        </EmptyDescription>
        <Button
          variant="outline"
          size="sm"
          class="mt-6 h-8 text-xs font-medium shadow-none"
          @click="openNewMemoryDialog"
        >
          {{ $t('bots.memory.newMemory') }}
        </Button>
      </Empty>
    </div>

    <!-- New Memory Dialog -->
    <Dialog v-model:open="newMemoryDialogOpen">
      <DialogContent class="sm:max-w-4xl max-h-[85vh] p-0 flex flex-col gap-0 overflow-hidden bg-background">
        <div class="px-5 py-4 border-b shrink-0 flex items-center justify-between bg-muted/10">
          <DialogTitle class="text-sm font-medium">
            {{ $t('bots.memory.newMemory') }}
          </DialogTitle>
        </div>

        <!-- Side-by-Side Container -->
        <div class="flex-1 flex min-h-0 overflow-hidden">
          <!-- LEFT: History Selection -->
          <div class="w-1/2 flex flex-col border-r min-w-0 bg-background">
            <div class="px-4 h-10 border-b flex items-center justify-between shrink-0">
              <Label class="text-xs font-medium text-foreground">{{ $t('bots.memory.fromConversation') }}</Label>
              <Button
                variant="ghost"
                size="sm"
                class="h-7 px-2 -mr-2 text-xs text-muted-foreground hover:text-foreground shadow-none"
                @click="loadHistory"
              >
                <RefreshCw
                  :class="{ 'animate-spin': historyLoading }"
                  class="mr-1.5 size-3"
                />
                {{ $t('common.refresh') }}
              </Button>
            </div>

            <div
              v-if="historyLoading"
              class="flex-1 flex items-center justify-center bg-muted/5"
            >
              <Spinner />
            </div>
            <div
              v-else-if="historyMessages.length === 0"
              class="flex-1 flex flex-col items-center justify-center p-8 bg-muted/5"
            >
              <div class="border-2 border-dashed border-border/50 rounded-md p-6 flex flex-col items-center justify-center text-center max-w-xs w-full">
                <p class="text-xs text-muted-foreground">
                  {{ $t('bots.memory.emptyHistory') }}
                </p>
              </div>
            </div>
            <ScrollArea
              v-else
              class="flex-1 min-h-0 bg-muted/5"
            >
              <div class="p-3 space-y-2">
                <button
                  v-for="(msg, idx) in historyMessages"
                  :key="idx"
                  type="button"
                  class="w-full text-left flex items-start gap-3 p-3 rounded-md border transition-colors group cursor-pointer"
                  :class="selectedHistoryMessages.includes(msg) ? 'bg-primary/5 border-primary/30' : 'bg-background border-border hover:border-foreground/30'"
                  @click="toggleMessageSelection(msg)"
                >
                  <div
                    class="mt-0.5 size-4 shrink-0 rounded-sm border flex items-center justify-center transition-colors"
                    :class="selectedHistoryMessages.includes(msg) ? 'bg-primary border-primary text-primary-foreground' : 'border-input bg-background group-hover:border-foreground/30'"
                  >
                    <Check
                      v-if="selectedHistoryMessages.includes(msg)"
                      class="size-3"
                    />
                  </div>
                  <div class="min-w-0 space-y-1.5 flex-1">
                    <Badge
                      variant="outline"
                      class="text-[9px] font-medium uppercase px-1.5 py-0 h-4 border-foreground/10 shadow-none"
                      :class="msg.role === 'user' ? 'bg-muted text-muted-foreground' : 'bg-primary/10 text-primary border-primary/20'"
                    >
                      {{ msg.role }}
                    </Badge>
                    <p class="text-[11px] text-foreground/90 wrap-break-word line-clamp-4 leading-relaxed font-mono">
                      {{ extractMessageText(msg.content) }}
                    </p>
                  </div>
                </button>
              </div>
            </ScrollArea>
          </div>

          <!-- RIGHT: Memory Content Editing -->
          <div class="w-1/2 flex flex-col min-w-0 bg-background">
            <div class="px-4 h-10 border-b flex items-center justify-between shrink-0">
              <Label class="flex items-center gap-1 text-xs font-medium text-foreground">
                Memory Content
                <span
                  v-if="newMemoryContent.trim()"
                  class="text-foreground shrink-0 text-xs font-bold leading-none select-none"
                >*</span>
              </Label>
            </div>
            <div class="flex-1 flex flex-col min-h-0 p-3 bg-muted/5">
              <Card class="flex-1 overflow-hidden rounded-md shadow-none bg-background border focus-within:border-foreground/50 transition-colors">
                <CardContent class="p-0 h-full relative">
                  <Textarea
                    v-model="newMemoryContent"
                    class="absolute inset-0 w-full h-full resize-none border-0 rounded-none focus-visible:ring-0 font-mono text-xs p-3 bg-transparent leading-relaxed"
                    placeholder="Select from history on the left, or manually write memory content here..."
                  />
                </CardContent>
              </Card>
            </div>
          </div>
        </div>

        <DialogFooter class="p-4 border-t bg-muted/10 shrink-0 flex items-center justify-end gap-2">
          <Button
            variant="outline"
            size="sm"
            class="h-8 text-xs font-medium shadow-none min-w-20"
            @click="newMemoryDialogOpen = false"
          >
            {{ $t('common.cancel') }}
          </Button>
          <Button
            size="sm"
            class="h-8 text-xs font-medium shadow-none min-w-24"
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
  </div>
</template>

<script setup lang="ts">
import { Brain, RefreshCw, Search, FileText, Plus, Copy, Check, Zap, BrainCircuit } from 'lucide-vue-next'
import { computed, ref, onMounted, watch } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, BarChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
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
  DialogTitle,
  DialogFooter,
  Popover,
  PopoverAnchor,
  PopoverContent,
  Badge,
  Label,
  RadioGroup,
  RadioGroupItem,
  Card,
  CardContent,
  Tooltip,
  TooltipProvider,
  TooltipTrigger,
  TooltipContent,
  Empty,
  EmptyTitle,
  EmptyDescription,
  EmptyMedia,
  Separator,
  Skeleton
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

// State: New Memory Modal configuration
const newMemoryDialogOpen = ref(false)
const newMemoryContent = ref('')
const historyLoading = ref(false)
const historyMessages = ref<Message[]>([])
const selectedHistoryMessages = ref<Message[]>([])

// State: Compact Memory Popover configuration
const compactPopoverOpen = ref(false)
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
  if (loading.value || compactLoading.value || memories.value.length === 0) return

  if (!compactPopoverOpen.value) {
    compactRatio.value = '0.5'
    compactDecayDate.value = ''
  }
  compactPopoverOpen.value = !compactPopoverOpen.value
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
    compactPopoverOpen.value = false
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
