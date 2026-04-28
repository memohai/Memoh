<template>
  <div class="space-y-4">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-3">
        <h3 class="text-sm font-medium">
          {{ $t('bots.schedule.title') }}
        </h3>
        <Badge
          v-if="schedules.length"
          variant="secondary"
          class="text-xs"
        >
          {{ schedules.length }}
        </Badge>
      </div>
      <div class="flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          :disabled="isLoading"
          @click="handleRefresh"
        >
          <Spinner
            v-if="isLoading"
            class="mr-2 size-4"
          />
          {{ $t('common.refresh') }}
        </Button>
        <Button
          size="sm"
          @click="handleNew"
        >
          <Plus class="mr-1 size-4" />
          {{ $t('bots.schedule.create') }}
        </Button>
      </div>
    </div>

    <!-- Loading -->
    <div
      v-if="isLoading && schedules.length === 0"
      class="flex items-center justify-center py-8 text-xs text-muted-foreground"
    >
      <Spinner class="mr-2" />
      {{ $t('common.loading') }}
    </div>

    <!-- Empty -->
    <div
      v-else-if="!isLoading && schedules.length === 0"
      class="flex flex-col items-center justify-center py-12 text-center"
    >
      <div class="rounded-full bg-muted p-3 mb-4">
        <Calendar
          class="size-6 text-muted-foreground"
        />
      </div>
      <p class="text-xs text-muted-foreground mb-3">
        {{ $t('bots.schedule.empty') }}
      </p>
      <Button
        size="sm"
        variant="outline"
        @click="handleNew"
      >
        <Plus class="mr-1 size-4" />
        {{ $t('bots.schedule.create') }}
      </Button>
    </div>

    <!-- Table -->
    <template v-else>
      <div class="rounded-md border overflow-hidden">
        <table class="w-full text-xs">
          <thead>
            <tr class="border-b bg-muted/50">
              <th class="px-4 py-2 text-left font-medium">
                {{ $t('bots.schedule.name') }}
              </th>
              <th class="px-4 py-2 text-left font-medium">
                {{ $t('bots.schedule.pattern') }}
              </th>
              <th class="px-4 py-2 text-left font-medium">
                {{ $t('bots.schedule.enabled') }}
              </th>
              <th class="px-4 py-2 text-left font-medium">
                {{ $t('bots.schedule.calls') }}
              </th>
              <th class="px-4 py-2 text-left font-medium">
                {{ $t('bots.schedule.updatedAt') }}
              </th>
              <th class="px-4 py-2 text-right font-medium w-[1%]">
                {{ $t('bots.schedule.actions') }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="item in pagedSchedules"
              :key="item.id"
              class="border-b last:border-0 hover:bg-muted/30"
            >
              <td class="px-4 py-2 font-medium align-top">
                <div>{{ item.name }}</div>
                <div class="text-xs text-muted-foreground line-clamp-1">
                  {{ item.description }}
                </div>
              </td>
              <td class="px-4 py-2 align-top">
                <code class="text-xs bg-muted px-1.5 py-0.5 rounded font-mono">
                  {{ item.pattern }}
                </code>
                <div
                  v-if="describeItem(item.pattern)"
                  class="text-[11px] text-muted-foreground mt-1"
                >
                  {{ describeItem(item.pattern) }}
                </div>
              </td>
              <td class="px-4 py-2 align-top">
                <div class="flex items-center gap-2">
                  <Switch
                    :model-value="!!item.enabled"
                    :disabled="busyIds.has(item.id || '')"
                    @update:model-value="(val: boolean) => handleToggleEnabled(item, !!val)"
                  />
                  <span class="text-xs text-muted-foreground">
                    {{ item.enabled ? $t('bots.schedule.statusEnabled') : $t('bots.schedule.statusDisabled') }}
                  </span>
                </div>
              </td>
              <td class="px-4 py-2 text-muted-foreground align-top">
                {{ item.current_calls ?? 0 }} / {{ formatMaxCalls(item) }}
              </td>
              <td class="px-4 py-2 text-muted-foreground align-top">
                {{ formatDateTime(item.updated_at) }}
              </td>
              <td class="px-4 py-2 align-top text-right whitespace-nowrap">
                <div class="flex items-center justify-end gap-1">
                  <Button
                    size="icon"
                    variant="ghost"
                    class="size-7"
                    :aria-label="$t('bots.schedule.edit')"
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
                        size="icon"
                        variant="ghost"
                        class="size-7 text-destructive hover:text-destructive"
                        :aria-label="$t('bots.schedule.delete')"
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
        class="flex items-center justify-between pt-4"
      >
        <span class="text-xs text-muted-foreground whitespace-nowrap">
          {{ paginationSummary }}
        </span>
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
    </template>

    <ScheduleFormDialog
      v-model:open="dialogOpen"
      :bot-id="botId"
      :mode="dialogMode"
      :schedule="dialogSchedule"
      :timezone="botTimezone"
      @saved="handleSaved"
    />
  </div>
</template>

<script setup lang="ts">
import { Calendar, Pencil, Plus, Trash2 } from 'lucide-vue-next'
import { ref, computed, onMounted, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import {
  Button, Badge, Spinner, Switch,
  Pagination, PaginationContent, PaginationEllipsis,
  PaginationFirst, PaginationItem, PaginationLast,
  PaginationNext, PaginationPrevious,
} from '@memohai/ui'
import {
  deleteBotsByBotIdScheduleById,
  getBotsByBotIdSchedule,
  getBotsByBotIdSettings,
  putBotsByBotIdScheduleById,
} from '@memohai/sdk'
import type { ScheduleSchedule } from '@memohai/sdk'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { formatDateTime } from '@/utils/date-time'
import { describeCron } from '@/utils/cron-pattern'
import ScheduleFormDialog from './schedule-form-dialog.vue'

const props = defineProps<{
  botId: string
}>()

const { t, locale } = useI18n()

const isLoading = ref(false)
const schedules = ref<ScheduleSchedule[]>([])
const currentPage = ref(1)
const PAGE_SIZE = 10
const botTimezone = ref<string | undefined>(undefined)
const busyIds = reactive(new Set<string>())

const dialogOpen = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const dialogSchedule = ref<ScheduleSchedule | null>(null)

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
  // Backend emits max_calls as a plain number or omits it (typing in the SDK
  // is a known mismatch). Treat any non-positive/absent value as unlimited.
  const raw = item.max_calls as unknown
  if (typeof raw === 'number' && raw > 0) return String(raw)
  return t('bots.schedule.unlimited')
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
    // Fallback to browser timezone — non-fatal.
    botTimezone.value = undefined
  }
}

async function handleRefresh() {
  currentPage.value = 1
  await fetchSchedules()
}

function handleNew() {
  dialogMode.value = 'create'
  dialogSchedule.value = null
  dialogOpen.value = true
}

function handleEdit(item: ScheduleSchedule) {
  dialogMode.value = 'edit'
  dialogSchedule.value = item
  dialogOpen.value = true
}

async function handleSaved() {
  await fetchSchedules()
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
</script>
