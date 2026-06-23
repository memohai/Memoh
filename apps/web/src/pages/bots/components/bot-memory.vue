<template>
  <PageShell
    variant="tab"
    :title="$t('bots.memory.title')"
  >
    <template #actions>
      <!-- Compact: low-frequency maintenance action, kept as a quiet icon
           button so it doesn't compete with the primary CTA. -->
      <Popover
        v-if="canSemanticCompact && memories.length > 0"
        v-model:open="compactPopoverOpen"
      >
        <PopoverAnchor as-child>
          <div class="inline-flex">
            <TooltipProvider>
              <Tooltip :delay-duration="300">
                <TooltipTrigger as-child>
                  <Button
                    variant="outline"
                    type="button"
                    :disabled="compactLoading"
                    @click="compactPopoverOpen = !compactPopoverOpen"
                  >
                    <Brain class="size-4" />
                    {{ $t('bots.memory.compact') }}
                  </Button>
                </TooltipTrigger>
                <TooltipContent
                  side="bottom"
                  align="center"
                >
                  <p class="text-caption">
                    {{ $t('bots.memory.compact') }}
                  </p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </div>
        </PopoverAnchor>

        <PopoverContent
          side="bottom"
          align="end"
          class="w-72 p-3 flex flex-col gap-3"
          :side-offset="4"
        >
          <div class="space-y-1">
            <h4 class="text-label font-medium text-foreground leading-none">
              {{ $t('bots.memory.compact') }}
            </h4>
            <p class="text-body text-muted-foreground leading-snug">
              {{ $t('bots.memory.compactConfirm') }}
            </p>
          </div>

          <div class="space-y-1.5">
            <Label class="text-caption font-semibold text-muted-foreground uppercase tracking-wider">{{ $t('bots.memory.compactRatio') }}</Label>
            <Select v-model="compactRatio">
              <SelectTrigger
                size="sm"
                class="w-full"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="0.8">
                  {{ $t('bots.memory.compactRatioLight') }}
                </SelectItem>
                <SelectItem value="0.5">
                  {{ $t('bots.memory.compactRatioMedium') }}
                </SelectItem>
                <SelectItem value="0.3">
                  {{ $t('bots.memory.compactRatioAggressive') }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div class="space-y-1.5">
            <Label class="text-caption font-semibold text-muted-foreground uppercase tracking-wider">
              {{ $t('bots.memory.compactDecayDate') }}
              <span class="text-muted-foreground/60 normal-case tracking-normal">({{ $t('common.optional') }})</span>
            </Label>
            <Popover>
              <PopoverTrigger
                type="button"
                data-slot="select-trigger"
                data-size="sm"
                :data-placeholder="compactDecayRange ? undefined : ''"
                :class="[selectTriggerClass, 'w-full']"
              >
                <span class="flex min-w-0 items-center gap-2">
                  <CalendarDays class="size-4" />
                  <span class="truncate">{{ compactDecayLabel }}</span>
                </span>
              </PopoverTrigger>
              <PopoverContent
                class="w-auto p-0"
                align="start"
              >
                <div class="p-3">
                  <RangeCalendar v-model="compactDecayRange" />
                </div>
              </PopoverContent>
            </Popover>
          </div>

          <div class="flex items-center justify-end gap-2 pt-2 mt-1 border-t">
            <Button
              variant="ghost"
              @click="compactPopoverOpen = false"
            >
              {{ $t('common.cancel') }}
            </Button>
            <Button
              :loading="compactLoading"
              @click="handleCompact"
            >
              {{ $t('common.confirm') }}
            </Button>
          </div>
        </PopoverContent>
      </Popover>

      <Button @click="openNewMemoryDialog">
        <Plus class="size-4" />
        {{ $t('bots.memory.newMemory') }}
      </Button>
    </template>

    <!-- Stat tiles: two framed metric readouts, same row. The caller owns the
         grid; each tile carries its own border so there's no divider-line
         visual-weight bias. `—` while a cold load has no data yet. -->
    <section class="mb-8 grid grid-cols-2 gap-3">
      <MetricReadout
        :label="$t('bots.memory.totalCount')"
        :value="String(loading && memories.length === 0 ? '—' : stats.totalCount)"
      />
      <MetricReadout :label="$t('bots.memory.lastUpdated')">
        <template #value>
          <template v-if="stats.lastUpdatedAt">
            {{ formatRelativeTime(stats.lastUpdatedAt, { locale }) }}
          </template>
          <template v-else>
            —
          </template>
        </template>
      </MetricReadout>
    </section>

    <!-- Memory graph view: see the shape of the wiki — hubs, clusters, orphans.
         Mirrors the LLM Wiki pattern where cross-references are compiled and
         browsable, not re-derived on every query. -->
    <section class="mb-8">
      <MemoryGraph :bot-id="props.botId" />
    </section>

    <!-- Loading skeleton: matches the card shape so the swap does not jump. -->
    <div
      v-if="loading && memories.length === 0"
      class="space-y-3"
    >
      <Skeleton
        v-for="n in 3"
        :key="n"
        class="h-[4.5rem] w-full rounded-[var(--radius-menu-shell)]"
      />
    </div>

    <!-- Empty: the section card is the frame, so the Empty is borderless,
         no icon-tile (skill: in-card Empty rule). -->
    <SettingsSection v-else-if="memories.length === 0">
      <Empty class="py-12">
        <EmptyHeader>
          <EmptyTitle>{{ $t('bots.memory.emptyTitle') }}</EmptyTitle>
          <EmptyDescription>{{ $t('bots.memory.empty') }}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button
            variant="outline"
            @click="openNewMemoryDialog"
          >
            {{ $t('bots.memory.newMemory') }}
          </Button>
        </EmptyContent>
      </Empty>
    </SettingsSection>

    <!-- Memory stream: dated groups, newest first. Each group is a
         SettingsSection card; each memory is an inset row with a trailing
         edit button. -->
    <div
      v-else
      class="space-y-8"
    >
      <SettingsSection
        v-for="group in groups"
        :key="group.date"
        :title="group.label"
      >
        <SettingsRow
          v-for="item in group.items"
          :key="item.id"
        >
          <template #content>
            <p class="truncate text-sm text-foreground">
              {{ item.memory }}
            </p>
            <p class="mt-0.5 truncate text-caption text-muted-foreground">
              {{ formatRelativeTime(item.created_at, { locale }) }}
            </p>
          </template>
          <Button
            variant="ghost"
            size="icon-sm"
            class="text-muted-foreground hover:text-foreground"
            :aria-label="$t('bots.memory.editMemory')"
            @click="openEditDialog(item)"
          >
            <Pencil class="size-4" />
          </Button>
        </SettingsRow>
      </SettingsSection>
    </div>

    <!-- Unified create / edit dialog. House form standard: DialogTitle, single
         Textarea, default-height footer buttons. Edit mode adds a destructive
         delete (ConfirmPopover) on the far left of the footer. -->
    <Dialog v-model:open="memoryDialogOpen">
      <DialogScrollContent class="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {{ editingId ? $t('bots.memory.editMemory') : $t('bots.memory.newMemory') }}
          </DialogTitle>
        </DialogHeader>

        <form
          class="space-y-4"
          @submit.prevent="handleSave"
        >
          <div class="space-y-1.5">
            <Label for="memory-content">
              {{ $t('bots.memory.contentLabel') }}
            </Label>
            <Textarea
              id="memory-content"
              v-model="dialogContent"
              :placeholder="$t('bots.memory.contentPlaceholder')"
              rows="3"
              class="min-h-[4.5rem] resize-y"
            />
          </div>

          <DialogFooter class="gap-2 sm:justify-between">
            <ConfirmPopover
              v-if="editingId"
              :message="$t('bots.memory.deleteConfirm')"
              variant="destructive"
              :confirm-text="$t('common.delete')"
              @confirm="handleDelete"
            >
              <template #trigger>
                <Button
                  variant="ghost"
                  size="icon"
                  type="button"
                  class="text-destructive hover:bg-destructive/10 hover:text-destructive"
                  :aria-label="$t('common.delete')"
                >
                  <Trash2 class="size-4" />
                </Button>
              </template>
            </ConfirmPopover>
            <div
              v-else
              class="flex-1"
            />

            <div class="flex gap-2">
              <DialogClose as-child>
                <Button
                  type="button"
                  variant="outline"
                >
                  {{ $t('common.cancel') }}
                </Button>
              </DialogClose>
              <Button
                type="submit"
                :disabled="!dialogContent.trim()"
                :loading="actionLoading"
              >
                {{ editingId ? $t('common.save') : $t('common.confirm') }}
              </Button>
            </div>
          </DialogFooter>
        </form>
      </DialogScrollContent>
    </Dialog>
  </PageShell>
</template>

<script setup lang="ts">
import { Brain, CalendarDays, Pencil, Plus, Trash2 } from 'lucide-vue-next'
import type { DateRange } from 'reka-ui'
import { DateFormatter, getLocalTimeZone, today } from '@internationalized/date'
import { computed, ref, watch } from 'vue'
import {
  Button,
  Textarea,
  Dialog,
  DialogScrollContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
  Popover,
  PopoverAnchor,
  PopoverTrigger,
  PopoverContent,
  Label,
  RangeCalendar,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  Tooltip,
  TooltipProvider,
  TooltipTrigger,
  TooltipContent,
  Empty,
  EmptyTitle,
  EmptyDescription,
  EmptyHeader,
  EmptyContent,
  Skeleton,
  selectTriggerClass,
  toast,
} from '@memohai/ui'
import {
  getBotsByBotIdMemory,
  getBotsByBotIdMemoryStatus,
  postBotsByBotIdMemory,
  deleteBotsByBotIdMemoryById,
  postBotsByBotIdMemoryCompact,
} from '@memohai/sdk'
import type {
  AdaptersMemoryItem,
  AdaptersMemoryStatusResponse,
} from '@memohai/sdk'
import { useI18n } from 'vue-i18n'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import { formatRelativeTime } from '@/utils/date-time'
import { resolveApiErrorMessage } from '@/utils/api-error'
import PageShell from '@/components/page-shell/index.vue'
import SettingsSection from '@/components/settings/section.vue'
import SettingsRow from '@/components/settings/row.vue'
import MetricReadout from '@/components/settings/metric-readout.vue'
import MemoryGraph from './memory-graph.vue'
import { useMemoryGroups } from './use-memory-groups'

interface MemoryItem {
  id: string
  memory: string
  created_at?: string
  updated_at?: string
  hash?: string
  score?: number
}

const props = defineProps<{
  botId: string
}>()

const { t, locale } = useI18n()
const loading = ref(false)
const actionLoading = ref(false)
const compactLoading = ref(false)
const memories = ref<MemoryItem[]>([])
const memoryStatus = ref<AdaptersMemoryStatusResponse | null>(null)
const memoryStatusError = ref('')

const compactPopoverOpen = ref(false)
const compactRatio = ref('0.5')
const compactDecayRange = ref<DateRange | undefined>({ start: today(getLocalTimeZone()), end: today(getLocalTimeZone()) })
const compactDecayFormatter = new DateFormatter(locale.value, { dateStyle: 'medium' })
const compactDecayLabel = computed(() => {
  if (!compactDecayRange.value?.start) return ''
  return compactDecayFormatter.format(compactDecayRange.value.start.toDate(getLocalTimeZone()))
})

// Dialog state: shared between create and edit modes.
const memoryDialogOpen = ref(false)
const editingId = ref<string | null>(null)
const dialogContent = ref('')

const memoriesComputed = computed(() => memories.value)
const { groups, stats } = useMemoryGroups(memoriesComputed, computed(() => locale.value))

const compactCapability = computed(() => memoryStatus.value?.compact)
const canSemanticCompact = computed(() => compactCapability.value?.semantic === true)

const compactDecayDays = computed(() => {
  if (!compactDecayRange.value?.start) return 0
  const selectedDate = compactDecayRange.value.start.toDate(getLocalTimeZone())
  const todayDate = today(getLocalTimeZone()).toDate(getLocalTimeZone())
  selectedDate.setHours(0, 0, 0, 0)
  todayDate.setHours(0, 0, 0, 0)
  const diffTime = todayDate.getTime() - selectedDate.getTime()
  const diffDays = Math.floor(diffTime / (1000 * 60 * 60 * 24))
  return diffDays > 0 ? diffDays : 0
})

async function loadMemories() {
  const botId = props.botId.trim()
  if (!botId) {
    memories.value = []
    return
  }

  loading.value = true
  try {
    const { data } = await getBotsByBotIdMemory({
      path: { bot_id: botId },
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
      }))
  } catch (error) {
    console.error('Failed to load memories:', error)
    toast.error(t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function loadMemoryStatus() {
  const botId = props.botId.trim()
  if (!botId) {
    memoryStatus.value = null
    memoryStatusError.value = ''
    return
  }

  try {
    const { data } = await getBotsByBotIdMemoryStatus({
      path: { bot_id: botId },
      throwOnError: true,
    })
    memoryStatus.value = data ?? null
    memoryStatusError.value = ''
  } catch (error) {
    console.error('Failed to load memory status:', error)
    memoryStatus.value = null
    memoryStatusError.value = resolveApiErrorMessage(error, t('bots.memory.compactStatusUnavailable'))
  }
}

function openNewMemoryDialog() {
  editingId.value = null
  dialogContent.value = ''
  memoryDialogOpen.value = true
}

function openEditDialog(item: MemoryItem) {
  editingId.value = item.id
  dialogContent.value = item.memory
  memoryDialogOpen.value = true
}

async function handleSave() {
  const text = dialogContent.value.trim()
  if (!text) return

  actionLoading.value = true
  try {
    if (editingId.value) {
      // Edit = delete old + add new (the API has no update-by-id for chat scope).
      await deleteBotsByBotIdMemoryById({
        path: { bot_id: props.botId, id: editingId.value },
        throwOnError: true,
      })
    }
    await postBotsByBotIdMemory({
      path: { bot_id: props.botId },
      body: { message: text },
      throwOnError: true,
    })

    toast.success(t('bots.memory.saveSuccess'))
    memoryDialogOpen.value = false
    await loadMemories()
  } catch (error) {
    console.error('Failed to save memory:', error)
    toast.error(t('common.saveFailed'))
  } finally {
    actionLoading.value = false
  }
}

async function handleDelete() {
  if (!editingId.value) return

  actionLoading.value = true
  try {
    await deleteBotsByBotIdMemoryById({
      path: { bot_id: props.botId, id: editingId.value },
      throwOnError: true,
    })
    toast.success(t('bots.memory.deleteSuccess'))
    memoryDialogOpen.value = false
    await loadMemories()
  } catch (error) {
    console.error('Failed to delete memory:', error)
    toast.error(t('bots.memory.deleteFailed'))
  } finally {
    actionLoading.value = false
  }
}

async function handleCompact() {
  if (!canSemanticCompact.value) return
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
  } catch (error) {
    console.error('Failed to compact memory:', error)
    toast.error(resolveApiErrorMessage(error, t('bots.memory.compactFailed')))
  } finally {
    compactLoading.value = false
  }
}

watch(() => props.botId, () => {
  memories.value = []
  void loadMemories()
  void loadMemoryStatus()
}, { immediate: true })
</script>
