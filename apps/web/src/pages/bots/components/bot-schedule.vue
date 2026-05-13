<template>
  <div class="max-w-4xl mx-auto pb-6 space-y-5">
    <!-- Sovereign Header -->
    <header class="pb-4 border-b border-border/50 sticky top-0 bg-background/95 backdrop-blur z-30 pt-4 -mt-4 flex items-center justify-between gap-4">
      <div class="space-y-1">
        <h2 class="text-sm font-semibold text-foreground flex items-center gap-2">
          {{ $t('bots.schedule.title') }}
          <Badge
            v-if="schedules.length"
            variant="secondary"
            class="text-[10px] h-5 px-1.5 font-mono"
          >
            {{ schedules.length }}
          </Badge>
        </h2>
        <p class="text-[11px] leading-snug text-muted-foreground max-w-md">
          {{ $t('bots.schedule.subtitle') }}
        </p>
      </div>
      <div class="flex shrink-0 flex-wrap justify-end gap-2">
        <Button
          variant="outline"
          size="sm"
          :disabled="isLoading || isRefreshing"
          class="shadow-none"
          @click="handleRefresh"
        >
          <Spinner
            v-if="isLoading || isRefreshing"
            class="mr-1.5"
          />
          <RefreshCw
            v-else
            class="mr-1.5 size-3.5 text-muted-foreground"
          />
          {{ $t('common.refresh') }}
        </Button>
        <Button
          v-if="!formVisible"
          variant="secondary"
          size="sm"
          class="shadow-none"
          @click="handleNew"
        >
          <Plus class="mr-1.5 size-3.5" />
          {{ $t('bots.schedule.create') }}
        </Button>
      </div>
    </header>

    <div class="space-y-6">
      <!-- Loading State -->
      <div
        v-if="isLoading && schedules.length === 0"
        class="flex items-center gap-2 text-xs text-muted-foreground"
      >
        <Spinner />
        <span>{{ $t('common.loading') }}</span>
      </div>

      <!-- Empty State -->
      <div
        v-else-if="!isLoading && schedules.length === 0 && !formVisible"
        class="space-y-6"
      >
        <div class="flex flex-col items-center justify-center py-10 border border-border/40 border-dashed rounded-lg bg-muted/5">
          <div class="size-10 rounded-full bg-muted/20 flex items-center justify-center mb-4">
            <Calendar class="size-5 text-muted-foreground" />
          </div>
          <p class="text-sm font-medium text-foreground mb-1">
            {{ $t('bots.schedule.empty') }}
          </p>
          <Button
            size="sm"
            variant="outline"
            class="shadow-none h-8 text-xs mt-3 bg-background border-border/40"
            @click="handleNew"
          >
            <Plus class="mr-1.5 size-3.5" />
            {{ $t('bots.schedule.create') }}
          </Button>
        </div>
      </div>

      <template v-else>
        <!-- Top Pinned Form -->
        <div
          v-if="formVisible"
          class="bg-background border border-border/60 rounded-md flex flex-col shadow-none"
        >
          <div class="p-4 border-b border-border/40 flex items-center justify-between bg-muted/10">
            <div class="space-y-0.5">
              <h4 class="text-sm font-semibold text-foreground">
                {{ formMode === 'create' ? $t('bots.schedule.create') : $t('bots.schedule.edit') }}
              </h4>
              <div
                v-if="editingSchedule"
                class="text-[10px] text-muted-foreground/80 font-mono flex items-center gap-1.5"
              >
                <span>ID: {{ editingSchedule.id }}</span>
              </div>
            </div>
            <Button
              variant="ghost"
              size="icon"
              class="size-7 text-muted-foreground hover:bg-accent/40"
              @click="handleFormCancel"
            >
              <X class="size-4" />
            </Button>
          </div>
          
          <form
            class="p-4 space-y-6"
            @submit.prevent="handleFormSubmit"
          >
            <div class="space-y-4">
              <!-- Name & Enabled -->
              <div class="flex items-end gap-4">
                <div class="space-y-1.5 flex-1 min-w-0">
                  <Label
                    for="schedule-name"
                    class="text-xs font-medium"
                  >{{ $t('bots.schedule.form.name') }}</Label>
                  <Input
                    id="schedule-name"
                    v-model="form.name"
                    :placeholder="$t('bots.schedule.form.namePlaceholder')"
                    class="h-8 text-xs shadow-none border-border/60 bg-transparent"
                  />
                </div>
                <div class="flex items-center gap-2 h-8 shrink-0 bg-muted/20 px-3 rounded-md border border-border/40">
                  <Label
                    class="cursor-pointer text-[11px] text-muted-foreground"
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
                <Label
                  for="schedule-description"
                  class="text-xs font-medium flex items-center gap-1.5"
                >
                  {{ $t('bots.schedule.form.description') }}
                  <span class="text-[10px] text-muted-foreground font-normal">({{ $t('common.optional') }})</span>
                </Label>
                <Input
                  id="schedule-description"
                  v-model="form.description"
                  :placeholder="$t('bots.schedule.form.descriptionPlaceholder')"
                  class="h-8 text-xs shadow-none border-border/60 bg-transparent"
                />
              </div>
              
              <!-- Command -->
              <div class="space-y-1.5">
                <Label
                  for="schedule-command"
                  class="text-xs font-medium"
                >{{ $t('bots.schedule.form.command') }}</Label>
                <div class="relative">
                  <Textarea
                    id="schedule-command"
                    v-model="form.command"
                    class="text-xs shadow-none border-border/60 min-h-[60px] bg-transparent font-mono pr-8"
                    :placeholder="$t('bots.schedule.form.commandPlaceholder')"
                    rows="2"
                  />
                </div>
                <p class="text-[10px] text-muted-foreground">
                  {{ $t('bots.schedule.form.commandHint') }}
                </p>
              </div>

              <!-- Pattern Section -->
              <div class="space-y-3 relative">
                <!-- Sticky Header Container -->
                <div class="sticky top-[56px] z-20 bg-background/95 backdrop-blur-md pt-2 pb-1 -mt-2">
                  <Label class="text-xs font-medium block mb-2">{{ $t('bots.schedule.form.pattern') }}</Label>
                  
                  <!-- Cron Expression Card -->
                  <div class="bg-muted/10 border border-border/40 rounded-lg p-4 space-y-3">
                    <div class="flex items-center justify-between">
                      <Label class="text-[11px] font-medium text-muted-foreground">{{ $t('bots.schedule.form.cronCode') }}</Label>
                      <div class="flex items-center gap-3">
                        <p
                          v-if="!isValidCron(manualCron)"
                          class="text-[11px] text-destructive tracking-tight"
                        >
                          {{ $t('bots.schedule.form.invalidPattern') }}
                        </p>
                        <p class="text-[10px] text-muted-foreground">
                          {{ $t('bots.schedule.form.manualEditHint') }}
                        </p>
                      </div>
                    </div>
                    <Input
                      v-model="manualCron"
                      class="w-full bg-background border border-border/40 rounded-md px-3 py-2 font-mono text-[11px] text-foreground focus:outline-none shadow-none h-8"
                    />
                  </div>
                </div>

                <!-- Visual Builder Card -->
                <div class="bg-muted/10 rounded-lg border border-border/40 p-4 space-y-4">
                  <Label class="text-[11px] font-medium text-muted-foreground">{{ $t('bots.schedule.form.visualBuilder') }}</Label>
                  <SchedulePatternBuilder
                    :state="patternState"
                    :timezone="botTimezone"
                    @update:state="(next) => patternState = next"
                  />
                </div>
              </div>

              <!-- Max Calls -->
              <div class="bg-muted/10 border border-border/40 rounded-lg p-4 flex items-center justify-between gap-4">
                <Label class="text-xs font-medium shrink-0">{{ $t('bots.schedule.form.maxCalls') }}</Label>
                <div class="flex items-center gap-2">
                  <Input 
                    v-if="!maxCallsUnlimited" 
                    :model-value="form.maxCalls ?? 1" 
                    type="number" 
                    :min="1" 
                    placeholder="1" 
                    class="h-8 text-xs shadow-none border-border/60 bg-transparent w-20 px-2" 
                    @update:model-value="(v) => form.maxCalls = Math.max(1, Math.floor(Number(v) || 1))" 
                  />
                  <div class="flex items-center gap-2 h-8 shrink-0 bg-muted/20 px-3 rounded-md border border-border/40">
                    <Label
                      class="cursor-pointer text-[11px] text-muted-foreground"
                      @click="handleMaxCallsUnlimited(!maxCallsUnlimited)"
                    >
                      {{ $t('bots.schedule.form.maxCallsUnlimited') }}
                    </Label>
                    <Switch
                      :model-value="maxCallsUnlimited"
                      @update:model-value="(v: boolean) => handleMaxCallsUnlimited(!!v)"
                    />
                  </div>
                </div>
              </div>

              <p
                v-if="submitError"
                class="text-[11px] text-destructive"
              >
                {{ submitError }}
              </p>
            </div>

            <!-- Form Actions -->
            <div class="flex justify-end gap-2 pt-4 border-t border-border/40">
              <Button
                type="button"
                variant="outline"
                size="sm"
                class="shadow-none h-8 text-xs font-medium"
                @click="handleFormCancel"
              >
                {{ $t('common.cancel') }}
              </Button>
              <Button
                type="submit"
                size="sm"
                :disabled="!canSubmit || isSaving"
                class="shadow-none h-8 text-xs font-medium"
              >
                <Spinner
                  v-if="isSaving"
                  class="mr-1.5 size-3.5"
                />
                {{ formMode === 'create' ? $t('common.create') : $t('common.save') }}
              </Button>
            </div>
          </form>

          <!-- Danger Zone (Only for Edit) -->
          <div
            v-if="formMode === 'edit' && editingSchedule"
            class="p-4 pt-0"
          >
            <div class="pt-4">
              <div class="space-y-4 rounded-md border border-border bg-background p-4 shadow-none">
                <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                  <div class="space-y-0.5">
                    <h4 class="text-xs font-medium text-destructive">
                      {{ $t('common.dangerZone') }}
                    </h4>
                    <p class="text-[11px] text-muted-foreground">
                      {{ $t('bots.schedule.dangerZoneDesc') }}
                    </p>
                  </div>
                  <div class="flex justify-end shrink-0">
                    <ConfirmPopover
                      :message="$t('bots.schedule.deleteConfirm', { name: editingSchedule.name })"
                      :confirm-text="$t('bots.schedule.delete')"
                      :loading="busyIds.has(editingSchedule.id || '')"
                      @confirm="handleDelete(editingSchedule); formVisible = false"
                    >
                      <template #trigger>
                        <button
                          data-slot="popover-trigger"
                          class="[&_svg]:pointer-events-none [&_svg:not([class*='size-'])]:size-4 shrink-0 [&_svg]:shrink-0 has-[>svg]:px-2.5 inline-flex items-center justify-center whitespace-nowrap transition-all disabled:pointer-events-none disabled:opacity-50 outline-none focus-visible:ring-2 focus-visible:ring-ring/30 cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90 rounded-lg gap-1.5 px-3 min-w-28 h-8 text-xs font-medium shadow-none"
                          type="button"
                          aria-haspopup="dialog"
                          aria-expanded="false"
                        >
                          <Spinner
                            v-if="busyIds.has(editingSchedule.id || '')"
                            class="mr-1.5 size-3.5"
                          />
                          {{ $t('common.delete') }}
                        </button>
                      </template>
                    </ConfirmPopover>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- High Density List (Table Mode) -->
        <div
          v-if="schedules.length > 0"
          class="bg-background border border-border/60 rounded-md shadow-none flex flex-col overflow-hidden"
        >
          <div class="overflow-x-auto">
            <table class="w-full text-xs border-collapse min-w-[800px]">
              <thead>
                <tr class="border-b border-border/50 bg-muted/40">
                  <th class="px-4 py-2.5 text-left font-semibold text-foreground/80 whitespace-nowrap">
                    {{ $t('common.name') }}
                  </th>
                  <th class="px-4 py-2.5 text-left font-semibold text-foreground/80 whitespace-nowrap">
                    {{ $t('bots.schedule.form.pattern') }}
                  </th>
                  <th class="px-4 py-2.5 text-left font-semibold text-foreground/80 whitespace-nowrap">
                    {{ $t('bots.schedule.form.enabled') }}
                  </th>
                  <th class="px-4 py-2.5 text-left font-semibold text-foreground/80 whitespace-nowrap">
                    {{ $t('bots.schedule.form.maxCalls') }}
                  </th>
                  <th class="px-4 py-2.5 text-left font-semibold text-foreground/80 whitespace-nowrap">
                    {{ $t('common.updatedAt') }}
                  </th>
                  <th class="px-4 py-2.5 text-right font-semibold text-foreground/80 w-[1%] whitespace-nowrap">
                    {{ $t('common.actions') }}
                  </th>
                </tr>
              </thead>
              <tbody class="divide-y divide-border/50">
                <tr 
                  v-for="item in pagedSchedules" 
                  :key="item.id" 
                  class="hover:bg-muted/30 transition-colors group cursor-pointer" 
                  @click="handleEdit(item)"
                >
                  <!-- Identity -->
                  <td class="px-4 py-3 align-middle min-w-[200px]">
                    <div class="font-medium text-foreground">
                      {{ item.name }}
                    </div>
                    <div
                      v-if="item.description"
                      class="text-[11px] text-muted-foreground mt-0.5 line-clamp-1 leading-relaxed"
                    >
                      {{ item.description }}
                    </div>
                    <div class="text-[9px] font-mono text-muted-foreground/80 mt-2 uppercase tracking-tight">
                      ID: {{ item.id }}
                    </div>
                  </td>

                  <!-- Pattern -->
                  <td class="px-4 py-3 align-middle whitespace-nowrap">
                    <div class="font-mono text-[10px] text-foreground font-medium">
                      {{ item.pattern }}
                    </div>
                    <div class="text-[11px] text-muted-foreground mt-1 line-clamp-1">
                      {{ describeItem(item.pattern) || '-' }}
                    </div>
                  </td>

                  <!-- Enabled -->
                  <td
                    class="px-4 py-3 align-middle whitespace-nowrap"
                    @click.stop
                  >
                    <div class="flex items-center gap-2.5">
                      <Switch 
                        :model-value="!!item.enabled" 
                        :disabled="busyIds.has(item.id || '')" 
                        class="scale-90 origin-left" 
                        @update:model-value="(val: boolean) => handleToggleEnabled(item, !!val)"
                      />
                      <span
                        class="text-[11px] font-semibold min-w-[42px] transition-colors"
                        :class="item.enabled ? 'text-emerald-600 dark:text-emerald-400' : 'text-foreground/70'"
                      >
                        {{ item.enabled ? $t('common.enabled') : $t('common.disabled') }}
                      </span>
                    </div>
                  </td>

                  <!-- Telemetry -->
                  <td class="px-4 py-3 align-middle whitespace-nowrap text-[11px]">
                    <div class="flex items-center gap-1.5">
                      <span class="text-foreground font-semibold">{{ item.current_calls ?? 0 }}</span>
                      <span class="text-muted-foreground/30 text-[10px]">/</span>
                      <span class="text-muted-foreground font-medium">{{ formatMaxCalls(item) }}</span>
                    </div>
                  </td>

                  <!-- Timestamp -->
                  <td class="px-4 py-3 align-middle text-[11px] text-muted-foreground/90 whitespace-nowrap font-medium">
                    {{ formatDateTime(item.updated_at) }}
                  </td>

                  <!-- Actions -->
                  <td
                    class="px-4 py-3 align-middle text-right whitespace-nowrap"
                    @click.stop
                  >
                    <div class="flex items-center justify-end gap-1">
                      <Button 
                        variant="ghost" 
                        size="icon" 
                        class="size-7 text-foreground/70 hover:text-foreground hover:bg-accent/40 shadow-none transition-colors"
                        @click="handleEdit(item)"
                      >
                        <Pencil class="size-3.5" />
                      </Button>
                      <ConfirmPopover
                        :message="$t('bots.schedule.deleteConfirm', { name: item.name })"
                        :confirm-text="$t('bots.schedule.delete')"
                        :loading="busyIds.has(item.id || '')"
                        @confirm="handleDelete(item)"
                      >
                        <template #trigger>
                          <Button 
                            variant="ghost" 
                            size="icon" 
                            class="size-7 text-foreground/70 hover:text-destructive hover:bg-destructive/10 shadow-none transition-colors"
                          >
                            <Trash2 class="size-3.5" />
                          </Button>
                        </template>
                      </ConfirmPopover>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- Pagination -->
          <div
            v-if="totalPages > 1"
            class="flex items-center justify-between p-4 border-t border-border/40 bg-muted/5"
          >
            <span class="text-[11px] text-muted-foreground whitespace-nowrap">{{ paginationSummary }}</span>
            <Pagination
              :total="schedules.length"
              :items-per-page="PAGE_SIZE"
              :sibling-count="1"
              :page="currentPage"
              show-edges
              @update:page="currentPage = $event"
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
                    :is-active="item.value === currentPage"
                  />
                </template>
                <PaginationNext />
                <PaginationLast />
              </PaginationContent>
            </Pagination>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Calendar, Pencil, Plus, Trash2, X, RefreshCw } from 'lucide-vue-next'
import { ref, computed, onMounted, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { useQueryCache } from '@pinia/colada'
import {
  Button, Badge, Input, Label, Spinner, Switch, Textarea,
  Pagination, PaginationContent, PaginationEllipsis,
  PaginationFirst, PaginationItem, PaginationLast,
  PaginationNext, PaginationPrevious,
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
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { formatDateTime } from '@/utils/date-time'
import {
  describeCron,
  defaultScheduleFormState,
  fromCron,
  isValidCron,
  toCron,
  type ScheduleFormState,
} from '@/utils/cron-pattern'
import SchedulePatternBuilder from './schedule-pattern-builder.vue'

const props = defineProps<{
  botId: string
}>()

const { t, locale } = useI18n()

const isLoading = ref(false)
const isRefreshing = ref(false)
const schedules = ref<ScheduleSchedule[]>([])
const currentPage = ref(1)
const PAGE_SIZE = 10
const botTimezone = ref<string | undefined>(undefined)
const busyIds = reactive(new Set<string>())

// Inline form state
const formVisible = ref(false)
const formMode = ref<'create' | 'edit'>('create')
const editingSchedule = ref<ScheduleSchedule | null>(null)
const isSaving = ref(false)
const submitError = ref<string | null>(null)

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

const maxCallsUnlimited = computed(() => form.maxCalls === null)

function handleMaxCallsUnlimited(v: boolean) {
  form.maxCalls = v ? null : 1
}

// Sync Visual Builder -> Cron Code
watch(
  () => patternState.value,
  (next) => {
    const canonical = toCron(next)
    // Only update if the current manual input doesn't already represent this state
    if (toCron(fromCron(manualCron.value)) !== canonical) {
      manualCron.value = canonical
    }
  },
  { deep: true },
)

// Sync Cron Code -> Visual Builder
watch(manualCron, (next) => {
  const nextState = fromCron(next)
  // Only update state if it's different to avoid loops
  if (JSON.stringify(patternState.value) !== JSON.stringify(nextState)) {
    patternState.value = nextState
  }
})

const canSubmit = computed(() => {
  if (isSaving.value) return false
  if (!form.name.trim()) return false
  if (!form.command.trim()) return false
  if (!manualCron.value) return false
  if (!isValidCron(manualCron.value)) return false
  if (!maxCallsUnlimited.value && (form.maxCalls === null || form.maxCalls < 1)) return false
  return true
})

function resetForm() {
  form.name = ''
  form.description = ''
  form.command = ''
  form.maxCalls = null
  form.enabled = true
  patternState.value = defaultScheduleFormState()
  manualCron.value = toCron(patternState.value)
  submitError.value = null
}

function hydrateForm(s: ScheduleSchedule) {
  form.name = s.name ?? ''
  form.description = s.description ?? ''
  form.command = s.command ?? ''
  const maxCallsRaw = s.max_calls as unknown
  form.maxCalls = (typeof maxCallsRaw === 'number' && maxCallsRaw > 0) ? maxCallsRaw : null
  form.enabled = s.enabled ?? true
  patternState.value = fromCron(s.pattern ?? '')
  manualCron.value = s.pattern ?? ''
  submitError.value = null
}

// List computeds
const totalPages = computed(() => Math.ceil(schedules.value.length / PAGE_SIZE))

const pagedSchedules = computed(() => {
  const start = (currentPage.value - 1) * PAGE_SIZE
  return schedules.value.slice(start, start + PAGE_SIZE)
})

const paginationSummary = computed(() => {
  const total = schedules.value.length
  if (total === 0) return ''
  const start = (currentPage.value - 1) * PAGE_SIZE + 1
  const end = Math.min(currentPage.value * PAGE_SIZE, total)
  return `${start}-${end} / ${total}`
})

const cronLocale = computed<'en' | 'zh'>(() => (locale.value.startsWith('zh') ? 'zh' : 'en'))

function describeItem(pattern: string | undefined): string | undefined {
  if (!pattern) return undefined
  return describeCron(pattern, cronLocale.value)
}

function formatMaxCalls(item: ScheduleSchedule): string {
  const raw = item.max_calls as unknown
  if (typeof raw === 'number' && raw > 0) return String(raw)
  return '∞'
}

const queryCache = useQueryCache()

function invalidateSidebarSchedule() {
  queryCache.invalidateQueries({ key: ['bot-schedule', props.botId] })
}

async function fetchSchedules() {
  if (!props.botId) return
  isLoading.value = true
  try {
    const { data } = await getBotsByBotIdSchedule({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
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
    const { data } = await getBotsByBotIdSettings({
      path: { bot_id: props.botId },
      throwOnError: true,
    })
    const tz = (data as { timezone?: string } | undefined)?.timezone
    botTimezone.value = tz && tz.trim() !== '' ? tz : undefined
  } catch {
    botTimezone.value = undefined
  }
}

async function handleRefresh() {
  isRefreshing.value = true
  currentPage.value = 1
  try {
    await fetchSchedules()
  } finally {
    isRefreshing.value = false
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
    const maxCallsWire = form.maxCalls ?? null
    if (formMode.value === 'create') {
      const body = {
        name: form.name.trim(),
        description: form.description.trim(),
        command: form.command.trim(),
        pattern,
        enabled: form.enabled,
        max_calls: maxCallsWire,
      } as unknown as ScheduleCreateRequest
      await postBotsByBotIdSchedule({
        path: { bot_id: props.botId },
        body,
        throwOnError: true,
      })
      toast.success(t('bots.schedule.saveSuccess'))
    } else {
      const id = editingSchedule.value?.id
      if (!id) throw new Error('schedule id missing')
      const body = {
        name: form.name.trim(),
        description: form.description.trim(),
        command: form.command.trim(),
        pattern,
        enabled: form.enabled,
        max_calls: maxCallsWire,
      } as unknown as ScheduleUpdateRequest
      await putBotsByBotIdScheduleById({
        path: { bot_id: props.botId, id },
        body,
        throwOnError: true,
      })
      toast.success(t('bots.schedule.saveSuccess'))
    }
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
    await putBotsByBotIdScheduleById({
      path: { bot_id: props.botId, id },
      body: { enabled },
      throwOnError: true,
    })
    await fetchSchedules()
    invalidateSidebarSchedule()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.schedule.saveFailed')))
  } finally {
    busyIds.delete(id)
  }
}

async function handleDelete(item: ScheduleSchedule) {
  const id = item.id
  if (!id) return
  busyIds.add(id)
  try {
    await deleteBotsByBotIdScheduleById({
      path: { bot_id: props.botId, id },
      throwOnError: true,
    })
    toast.success(t('bots.schedule.deleteSuccess'))
    await fetchSchedules()
    invalidateSidebarSchedule()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.schedule.deleteFailed')))
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
    if (!props.botId) return
    if (next === prev) return
    void fetchSchedules()
  },
)
</script>
