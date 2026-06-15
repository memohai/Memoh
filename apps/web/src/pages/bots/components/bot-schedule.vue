<template>
  <div class="mx-auto max-w-3xl pt-6 pb-8">
    <header class="mb-6 flex items-center justify-between gap-4 px-2">
      <h1 class="text-lg font-semibold">
        {{ $t('bots.schedule.title') }}
      </h1>
      <div class="flex items-center gap-2">
        <DropdownMenu v-if="schedules.length > 1">
          <DropdownMenuTrigger as-child>
            <Button
              variant="ghost"
              class="text-muted-foreground"
            >
              <ArrowUpDown class="size-3.5" />
              {{ currentSortLabel }}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              v-for="opt in SORT_OPTIONS"
              :key="opt.key"
              class="justify-between gap-4"
              @select="sortKey = opt.key"
            >
              {{ $t(opt.labelKey) }}
              <Check
                class="size-3.5 shrink-0"
                :class="sortKey === opt.key ? 'opacity-100' : 'opacity-0'"
              />
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <Button @click="handleNew">
          <Plus class="size-4" />
          {{ $t('bots.schedule.create') }}
        </Button>
      </div>
    </header>

    <!-- Loading -->
    <div
      v-if="isLoading && schedules.length === 0"
      class="flex items-center gap-2 px-2 text-xs text-muted-foreground"
    >
      <Spinner class="size-3.5" />
      <span>{{ $t('common.loading') }}</span>
    </div>

    <!-- Empty -->
    <div
      v-else-if="schedules.length === 0"
      class="flex flex-col items-center justify-center rounded-[var(--radius-menu-shell)] border border-dashed border-border py-16 text-center"
    >
      <Calendar class="mb-3 size-8 text-muted-foreground/40" />
      <p class="text-sm font-medium text-foreground">
        {{ $t('bots.schedule.empty') }}
      </p>
      <Button
        variant="outline"
        size="sm"
        class="mt-4"
        @click="handleNew"
      >
        <Plus class="size-4" />
        {{ $t('bots.schedule.create') }}
      </Button>
    </div>

    <!-- Card Grid -->
    <div
      v-else
      class="grid grid-cols-1 gap-3 sm:grid-cols-2"
    >
      <div
        v-for="item in sortedSchedules"
        :key="item.id"
        class="group/card flex cursor-pointer items-center gap-3 rounded-[var(--radius-menu-shell)] border border-border bg-card px-4 py-3.5 transition-colors hover:bg-accent/30 dark:hover:bg-accent focus-visible:outline-none"
        role="button"
        tabindex="0"
        @click="handleEdit(item)"
        @keydown.enter="handleEdit(item)"
        @keydown.space.prevent="handleEdit(item)"
      >
        <!-- Name + description -->
        <div class="min-w-0 flex-1">
          <p class="truncate text-sm font-medium text-foreground">
            {{ item.name }}
          </p>
          <p class="truncate text-xs text-muted-foreground">
            {{ item.description?.trim() || describeItem(item.pattern) || item.pattern }}
          </p>
        </div>

        <!-- Hover actions + switch -->
        <div
          class="flex shrink-0 items-center gap-2"
          @click.stop
        >
          <!-- Three-dot menu: visible on hover OR when dropdown is open -->
          <DropdownMenu @update:open="(open: boolean) => { if (open) openMenuIds.add(item.id ?? ''); else openMenuIds.delete(item.id ?? '') }">
            <DropdownMenuTrigger as-child>
              <Button
                variant="ghost"
                size="icon"
                class="size-7 transition-opacity"
                :class="openMenuIds.has(item.id ?? '') ? 'opacity-100' : 'opacity-0 group-hover/card:opacity-100'"
                :aria-label="$t('common.actions')"
              >
                <MoreHorizontal class="size-3.5" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                class="gap-2"
                @select="handleEdit(item)"
              >
                <Pencil class="size-3.5" />
                {{ $t('bots.schedule.edit') }}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                class="gap-2 text-destructive focus:text-destructive"
                @select="deleteTarget = item"
              >
                <Trash2 class="size-3.5" />
                {{ $t('bots.schedule.delete') }}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>

          <Switch
            :model-value="!!item.enabled"
            :disabled="busyIds.has(item.id || '')"
            @update:model-value="(val: boolean) => handleToggleEnabled(item, !!val)"
          />
        </div>
      </div>
    </div>

    <!-- Create / Edit Dialog -->
    <Dialog v-model:open="formVisible">
      <DialogScrollContent class="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {{ formMode === 'create' ? $t('bots.schedule.create') : $t('bots.schedule.edit') }}
          </DialogTitle>
        </DialogHeader>

        <form
          class="space-y-4"
          @submit.prevent="handleFormSubmit"
        >
          <!-- Name + Enabled -->
          <div class="flex items-end gap-3">
            <div class="min-w-0 flex-1 space-y-1.5">
              <Label for="sched-name">
                {{ $t('bots.schedule.form.name') }}
              </Label>
              <Input
                id="sched-name"
                v-model="form.name"
                :placeholder="$t('bots.schedule.form.namePlaceholder')"
              />
            </div>
            <div class="flex h-9 shrink-0 items-center gap-2">
              <Label
                class="cursor-pointer text-muted-foreground"
                @click="form.enabled = !form.enabled"
              >
                {{ $t('bots.schedule.form.enabled') }}
              </Label>
              <Switch
                :model-value="form.enabled"
                @update:model-value="(v: boolean) => form.enabled = !!v"
              />
            </div>
          </div>

          <!-- Description -->
          <div class="space-y-1.5">
            <Label for="sched-desc">
              {{ $t('bots.schedule.form.description') }}
              <span class="ml-1 text-[11px] text-muted-foreground font-normal">({{ $t('common.optional') }})</span>
            </Label>
            <Input
              id="sched-desc"
              v-model="form.description"
              :placeholder="$t('bots.schedule.form.descriptionPlaceholder')"
            />
          </div>

          <!-- Command -->
          <div class="space-y-1.5">
            <Label for="sched-command">
              {{ $t('bots.schedule.form.command') }}
            </Label>
            <Textarea
              id="sched-command"
              v-model="form.command"
              class="min-h-[4.5rem] resize-none font-mono"
              :placeholder="$t('bots.schedule.form.commandPlaceholder')"
              rows="3"
            />
          </div>

          <!-- Schedule picker -->
          <div class="space-y-3">
            <Label>
              {{ $t('bots.schedule.form.pattern') }}
            </Label>

            <!-- Primary row: mode select + inline value/time -->
            <div class="flex items-center gap-2 flex-wrap">
              <!-- Mode selector -->
              <Select v-model="schedModeModel">
                <SelectTrigger class="w-36 shrink-0">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem
                    v-for="mode in SCHEDULE_MODES"
                    :key="mode.value"
                    :value="mode.value"
                  >
                    {{ $t(mode.labelKey) }}
                  </SelectItem>
                </SelectContent>
              </Select>

              <!-- Every N minutes: inline -->
              <template v-if="patternState.mode === 'minutes'">
                <Input
                  type="number"
                  :min="1"
                  :max="59"
                  :model-value="patternState.intervalMinutes"
                  class="w-20 text-center"
                  @update:model-value="v => patchState({ intervalMinutes: clampInt(v, 1, 59, 1) })"
                />
                <span class="text-sm text-muted-foreground">{{ $t('bots.schedule.picker.minutes') }}</span>
              </template>

              <!-- Hourly: inline minute -->
              <template v-else-if="patternState.mode === 'hourly'">
                <span class="text-sm text-muted-foreground">{{ $t('bots.schedule.picker.atMinute') }}</span>
                <Input
                  type="number"
                  :min="0"
                  :max="59"
                  :model-value="patternState.minute"
                  class="w-20 text-center"
                  @update:model-value="v => patchState({ minute: clampInt(v, 0, 59, 0) })"
                />
              </template>

              <!-- Daily: TimeInput inline -->
              <TimeInput
                v-else-if="patternState.mode === 'daily'"
                :hour="patternState.hours[0] ?? 9"
                :minute="patternState.minute"
                @update:hour="v => patchState({ hours: [v] })"
                @update:minute="v => patchState({ minute: v })"
              />

              <!-- Weekly: TimeInput inline (days below) -->
              <TimeInput
                v-else-if="patternState.mode === 'weekly'"
                :hour="patternState.hours[0] ?? 9"
                :minute="patternState.minute"
                @update:hour="v => patchState({ hours: [v] })"
                @update:minute="v => patchState({ minute: v })"
              />

              <!-- Monthly: day input + TimeInput inline -->
              <template v-else-if="patternState.mode === 'monthly'">
                <span class="text-sm text-muted-foreground">{{ $t('bots.schedule.picker.day') }}</span>
                <Input
                  type="number"
                  :min="1"
                  :max="31"
                  :model-value="patternState.monthDays[0] ?? 1"
                  class="w-16 text-center"
                  @update:model-value="v => patchState({ monthDays: [clampInt(v, 1, 31, 1)] })"
                />
                <TimeInput
                  :hour="patternState.hours[0] ?? 9"
                  :minute="patternState.minute"
                  @update:hour="v => patchState({ hours: [v] })"
                  @update:minute="v => patchState({ minute: v })"
                />
              </template>
            </div>

            <!-- Weekly: day buttons (below the row) -->
            <div
              v-if="patternState.mode === 'weekly'"
              class="grid grid-cols-7 gap-1"
            >
              <button
                v-for="(key, idx) in WEEKDAY_KEYS"
                :key="key"
                type="button"
                class="h-9 rounded-md border text-sm transition-colors"
                :class="patternState.weekdays.includes(idx)
                  ? 'bg-primary text-primary-foreground border-primary'
                  : 'bg-background hover:bg-accent'"
                @click="toggleWeekday(idx)"
              >
                {{ $t(`bots.schedule.weekday.${key}`) }}
              </button>
            </div>

            <!-- Advanced: cron input (below the row) -->
            <div
              v-if="patternState.mode === 'advanced'"
              class="space-y-1.5"
            >
              <Input
                :model-value="patternState.advancedPattern"
                class="font-mono"
                placeholder="0 9 * * *"
                @update:model-value="v => patchState({ advancedPattern: String(v) })"
              />
              <p
                v-if="patternState.advancedPattern && !isValidCron(patternState.advancedPattern)"
                class="text-[11px] text-destructive"
              >
                {{ $t('bots.schedule.form.invalidPattern') }}
              </p>
            </div>

            <!-- Preview: only for modes where it adds real information -->
            <p
              v-if="schedulePreviewText && ['weekly', 'monthly', 'advanced'].includes(patternState.mode)"
              class="text-[11px] text-muted-foreground"
            >
              {{ schedulePreviewText }}
            </p>
          </div>

          <!-- More options: run limit only -->
          <div>
            <button
              type="button"
              class="flex items-center gap-1.5 text-xs text-muted-foreground transition-colors hover:text-foreground"
              @click="showMore = !showMore"
            >
              <ChevronRight
                class="size-3.5"
                :class="showMore ? 'rotate-90' : ''"
              />
              {{ $t('bots.schedule.moreOptions') }}
            </button>

            <!-- CSS grid-rows collapse trick: animates height without JS -->
            <div
              class="grid overflow-hidden transition-[grid-template-rows] duration-200 ease-out"
              :class="showMore ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'"
            >
              <div class="min-h-0">
                <div class="mt-3">
                  <!-- Run limit: single input, ∞ placeholder = unlimited -->
                  <div class="flex items-center justify-between gap-3">
                    <Label class="text-muted-foreground">
                      {{ $t('bots.schedule.form.maxCalls') }}
                    </Label>
                    <Input
                      v-model="runLimitModel"
                      type="text"
                      inputmode="numeric"
                      size="sm"
                      :placeholder="'∞'"
                      class="w-24 text-center"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>

          <p
            v-if="submitError"
            class="text-[11px] text-destructive"
          >
            {{ submitError }}
          </p>

          <DialogFooter class="gap-2 sm:justify-between">
            <div
              v-if="formMode === 'edit' && editingSchedule"
              class="flex-1"
            >
              <Button
                type="button"
                variant="ghost"
                class="text-destructive hover:bg-destructive/10 hover:text-destructive"
                @click="deleteTarget = editingSchedule; formVisible = false"
              >
                <Trash2 class="size-4" />
                {{ $t('common.delete') }}
              </Button>
            </div>
            <div
              v-else
              class="flex-1"
            />

            <div class="flex gap-2">
              <DialogClose as-child>
                <Button
                  type="button"
                  variant="ghost"
                  @click="handleFormCancel"
                >
                  {{ $t('common.cancel') }}
                </Button>
              </DialogClose>
              <Button
                type="submit"
                :disabled="!canSubmit || isSaving"
              >
                <Spinner
                  v-if="isSaving"
                  class="mr-1.5 size-4"
                />
                {{ formMode === 'create' ? $t('common.create') : $t('common.confirm') }}
              </Button>
            </div>
          </DialogFooter>
        </form>
      </DialogScrollContent>
    </Dialog>

    <!-- Delete confirmation dialog -->
    <Dialog
      :open="!!deleteTarget"
      @update:open="(v) => { if (!v) deleteTarget = null }"
    >
      <DialogContent class="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{{ $t('bots.schedule.deleteTitle') }}</DialogTitle>
        </DialogHeader>
        <p class="text-sm text-muted-foreground">
          {{ $t('bots.schedule.deleteConfirm', { name: deleteTarget?.name ?? '' }) }}
        </p>
        <DialogFooter class="gap-2">
          <Button
            variant="outline"
            @click="deleteTarget = null"
          >
            {{ $t('common.cancel') }}
          </Button>
          <Button
            variant="destructive"
            :disabled="isDeleting"
            @click="confirmDelete"
          >
            <Spinner
              v-if="isDeleting"
              class="mr-1.5 size-4"
            />
            {{ $t('bots.schedule.delete') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import {
  ArrowUpDown, Calendar, Check, ChevronRight,
  MoreHorizontal, Pencil, Plus, Trash2,
} from 'lucide-vue-next'
import { ref, computed, onMounted, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from '@memohai/ui'
import { useQueryCache } from '@pinia/colada'
import {
  Button, Input, Label, Spinner, Switch, Textarea, TimeInput,
  Dialog, DialogContent, DialogScrollContent, DialogHeader, DialogTitle, DialogFooter, DialogClose,
  DropdownMenu, DropdownMenuContent, DropdownMenuTrigger, DropdownMenuItem, DropdownMenuSeparator,
  Select, SelectTrigger, SelectValue, SelectContent, SelectItem,
} from '@memohai/ui'
import {
  deleteBotsByBotIdScheduleById,
  getBotsByBotIdSchedule,
  getBotsByBotIdSettings,
  postBotsByBotIdSchedule,
  putBotsByBotIdScheduleById,
} from '@memohai/sdk'
import type {
  ScheduleSchedule,
  ScheduleCreateRequest,
  ScheduleUpdateRequest,
} from '@memohai/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import {
  describeCron,
  defaultScheduleFormState,
  fromCron,
  isValidCron,
  nextRuns,
  toCron,
  WEEKDAY_KEYS,
  type ScheduleFormState,
  type ScheduleMode,
} from '@/utils/cron-pattern'

const props = defineProps<{
  botId: string
}>()

const { t, locale } = useI18n()

const isLoading = ref(false)
const schedules = ref<ScheduleSchedule[]>([])
const botTimezone = ref<string | undefined>(undefined)
const busyIds = reactive(new Set<string>())

// Track which card menus are open (keeps the ⋯ button visible while dropdown is open)
const openMenuIds = reactive(new Set<string>())

// --- Sort ---
type SortKey = 'name' | 'status' | 'next-run'
const sortKey = ref<SortKey>('name')

const SORT_OPTIONS: { key: SortKey; labelKey: string }[] = [
  { key: 'name', labelKey: 'bots.schedule.sortName' },
  { key: 'status', labelKey: 'bots.schedule.sortStatus' },
  { key: 'next-run', labelKey: 'bots.schedule.sortNextRun' },
]

const SCHEDULE_MODES: { value: ScheduleMode; labelKey: string }[] = [
  { value: 'minutes', labelKey: 'bots.schedule.mode.minutes' },
  { value: 'hourly', labelKey: 'bots.schedule.mode.hourly' },
  { value: 'daily', labelKey: 'bots.schedule.mode.daily' },
  { value: 'weekly', labelKey: 'bots.schedule.mode.weekly' },
  { value: 'monthly', labelKey: 'bots.schedule.mode.monthly' },
  { value: 'advanced', labelKey: 'bots.schedule.mode.advanced' },
]

const currentSortLabel = computed(
  () => t(SORT_OPTIONS.find((o) => o.key === sortKey.value)?.labelKey ?? 'bots.schedule.sortName'),
)

const effectiveTimezone = computed(() => {
  const tz = botTimezone.value?.trim()
  if (tz) return tz
  try { return Intl.DateTimeFormat().resolvedOptions().timeZone } catch { return 'UTC' }
})

const sortedSchedules = computed(() => {
  const list = [...schedules.value]
  if (sortKey.value === 'name') {
    return list.sort((a, b) => (a.name ?? '').localeCompare(b.name ?? ''))
  }
  if (sortKey.value === 'status') {
    return list.sort((a, b) => Number(b.enabled ?? false) - Number(a.enabled ?? false))
  }
  if (sortKey.value === 'next-run') {
    return list.sort((a, b) => {
      const aTime = a.pattern ? (nextRuns(a.pattern, effectiveTimezone.value, 1)[0]?.getTime() ?? Infinity) : Infinity
      const bTime = b.pattern ? (nextRuns(b.pattern, effectiveTimezone.value, 1)[0]?.getTime() ?? Infinity) : Infinity
      return aTime - bTime
    })
  }
  return list
})

// --- Delete via card menu ---
const deleteTarget = ref<ScheduleSchedule | null>(null)
const isDeleting = ref(false)

async function confirmDelete() {
  const item = deleteTarget.value
  if (!item?.id) return
  isDeleting.value = true
  const id = item.id
  busyIds.add(id)
  try {
    await deleteBotsByBotIdScheduleById({ path: { bot_id: props.botId, id }, throwOnError: true })
    toast.success(t('bots.schedule.deleteSuccess'))
    deleteTarget.value = null
    await fetchSchedules()
    invalidateSidebarSchedule()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.schedule.deleteFailed')))
  } finally {
    isDeleting.value = false
    busyIds.delete(id)
  }
}

// --- Dialog state ---
const formVisible = ref(false)
const formMode = ref<'create' | 'edit'>('create')
const editingSchedule = ref<ScheduleSchedule | null>(null)
const isSaving = ref(false)
const submitError = ref<string | null>(null)
const showMore = ref(false)

interface SchedulePlainForm {
  name: string
  description: string
  command: string
  maxCalls: number | null
  enabled: boolean
}

const form = reactive<SchedulePlainForm>({
  name: '',
  description: '',
  command: '',
  maxCalls: null,
  enabled: true,
})

const patternState = ref<ScheduleFormState>(defaultScheduleFormState())
const manualCron = ref('')

// --- Compact schedule picker ---

function clampInt(value: unknown, min: number, max: number, fallback: number): number {
  const n = Number(value)
  if (!Number.isFinite(n)) return fallback
  return Math.max(min, Math.min(max, Math.round(n)))
}

function patchState(patch: Partial<ScheduleFormState>) {
  patternState.value = { ...patternState.value, ...patch }
}

const schedModeModel = computed({
  get: (): string => patternState.value.mode,
  set: (val: unknown) => {
    const next = String(val) as ScheduleMode
    const allowed: ScheduleMode[] = ['minutes', 'hourly', 'daily', 'weekly', 'monthly', 'advanced']
    if (!allowed.includes(next)) return
    const patch: Partial<ScheduleFormState> = { mode: next }
    if (next === 'weekly' || next === 'monthly' || next === 'hourly') {
      patch.hours = [patternState.value.hours[0] ?? 9]
    }
    if (next === 'advanced' && !patternState.value.advancedPattern.trim()) {
      try { patch.advancedPattern = toCron(patternState.value) } catch { patch.advancedPattern = '' }
    }
    patternState.value = { ...patternState.value, ...patch }
  },
})

function toggleWeekday(d: number) {
  const set = new Set(patternState.value.weekdays)
  if (set.has(d)) set.delete(d)
  else set.add(d)
  const next = Array.from(set).sort((a, b) => a - b)
  patchState({ weekdays: next.length ? next : [d] })
}

const cronLocale = computed<'en' | 'zh'>(() => (locale.value.startsWith('zh') ? 'zh' : 'en'))

const schedulePreviewText = computed(() => {
  if (!manualCron.value || !isValidCron(manualCron.value)) return ''
  return describeCron(manualCron.value, cronLocale.value) || ''
})

// --- Sync patternState ↔ manualCron ---
watch(
  () => patternState.value,
  (next) => {
    try {
      const canonical = toCron(next)
      if (toCron(fromCron(manualCron.value)) !== canonical) manualCron.value = canonical
    } catch { /* invalid intermediate state */ }
  },
  { deep: true },
)
watch(manualCron, (next) => {
  const nextState = fromCron(next)
  if (JSON.stringify(patternState.value) !== JSON.stringify(nextState)) patternState.value = nextState
})

// --- Max calls: single input, ∞ placeholder = unlimited ---
const runLimitModel = computed({
  get(): string {
    return form.maxCalls === null ? '' : String(form.maxCalls)
  },
  set(val: string | number) {
    const str = String(val).replace(/\D/g, '')
    if (!str) {
      form.maxCalls = null
    } else {
      const n = parseInt(str, 10)
      form.maxCalls = Number.isFinite(n) && n > 0 ? n : null
    }
  },
})

const maxCallsUnlimited = computed(() => form.maxCalls === null)

// --- Validation ---
const canSubmit = computed(() => {
  if (isSaving.value) return false
  if (!form.name.trim()) return false
  if (!form.command.trim()) return false
  if (!manualCron.value || !isValidCron(manualCron.value)) return false
  if (!maxCallsUnlimited.value && (form.maxCalls === null || form.maxCalls < 1)) return false
  return true
})

// --- Form lifecycle ---
function resetForm() {
  form.name = ''
  form.description = ''
  form.command = ''
  form.maxCalls = null
  form.enabled = true
  patternState.value = defaultScheduleFormState()
  manualCron.value = toCron(patternState.value)
  submitError.value = null
  showMore.value = false
}

function hydrateForm(s: ScheduleSchedule) {
  form.name = s.name ?? ''
  form.description = s.description ?? ''
  form.command = s.command ?? ''
  const raw = s.max_calls as unknown
  form.maxCalls = (typeof raw === 'number' && raw > 0) ? raw : null
  form.enabled = s.enabled ?? true
  patternState.value = fromCron(s.pattern ?? '')
  manualCron.value = s.pattern ?? ''
  submitError.value = null
  showMore.value = (typeof raw === 'number' && (raw as number) > 0)
}

// --- Card helpers ---
function describeItem(pattern: string | undefined): string | undefined {
  if (!pattern) return undefined
  return describeCron(pattern, cronLocale.value)
}

// --- API ---
const queryCache = useQueryCache()
function invalidateSidebarSchedule() {
  queryCache.invalidateQueries({ key: ['bot-schedule', props.botId] })
}

async function fetchSchedules() {
  if (!props.botId) return
  isLoading.value = true
  try {
    const { data } = await getBotsByBotIdSchedule({ path: { bot_id: props.botId }, throwOnError: true })
    schedules.value = data?.items || []
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.schedule.loadFailed')))
  } finally {
    isLoading.value = false
  }
}

async function fetchBotSettings() {
  if (!props.botId) return
  try {
    const { data } = await getBotsByBotIdSettings({ path: { bot_id: props.botId }, throwOnError: true })
    const tz = (data as { timezone?: string } | undefined)?.timezone
    botTimezone.value = tz?.trim() || undefined
  } catch {
    botTimezone.value = undefined
  }
}

function handleNew() {
  formMode.value = 'create'
  editingSchedule.value = null
  resetForm()
  formVisible.value = true
}

function handleEdit(item: ScheduleSchedule) {
  formMode.value = 'edit'
  editingSchedule.value = item
  hydrateForm(item)
  formVisible.value = true
}

function handleFormCancel() {
  formVisible.value = false
  editingSchedule.value = null
  submitError.value = null
}

async function handleFormSubmit() {
  if (!canSubmit.value) return
  submitError.value = null
  isSaving.value = true
  try {
    const pattern = manualCron.value.trim()
    const max_calls = form.maxCalls ?? null
    const base = {
      name: form.name.trim(),
      description: form.description.trim(),
      command: form.command.trim(),
      pattern,
      enabled: form.enabled,
      max_calls,
    }
    if (formMode.value === 'create') {
      await postBotsByBotIdSchedule({ path: { bot_id: props.botId }, body: base as unknown as ScheduleCreateRequest, throwOnError: true })
    } else {
      const id = editingSchedule.value?.id
      if (!id) throw new Error('schedule id missing')
      await putBotsByBotIdScheduleById({ path: { bot_id: props.botId, id }, body: base as unknown as ScheduleUpdateRequest, throwOnError: true })
    }
    toast.success(t('bots.schedule.saveSuccess'))
    formVisible.value = false
    editingSchedule.value = null
    await fetchSchedules()
    invalidateSidebarSchedule()
  } catch (err) {
    submitError.value = resolveApiErrorMessage(err, t('bots.schedule.saveFailed'))
  } finally {
    isSaving.value = false
  }
}

async function handleToggleEnabled(item: ScheduleSchedule, enabled: boolean) {
  const id = item.id
  if (!id) return
  busyIds.add(id)
  try {
    await putBotsByBotIdScheduleById({ path: { bot_id: props.botId, id }, body: { enabled }, throwOnError: true })
    await fetchSchedules()
    invalidateSidebarSchedule()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.schedule.saveFailed')))
  } finally {
    busyIds.delete(id)
  }
}

onMounted(() => {
  fetchSchedules()
  fetchBotSettings()
})

watch(
  () => {
    const entries = queryCache.getEntries({ key: ['bot-schedule', props.botId] })
    return entries[0]?.state.value.data
  },
  (next, prev) => {
    if (!props.botId || next === prev) return
    void fetchSchedules()
  },
)
</script>
