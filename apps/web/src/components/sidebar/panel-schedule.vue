<template>
  <div class="flex flex-col h-full min-w-0">
    <!-- No bot -->
    <p
      v-if="!currentBotId"
      class="px-[23px] pt-3 text-caption text-muted-foreground/60"
    >
      {{ t('chat.noBotSelected') }}
    </p>

    <!-- Loading (initial) -->
    <div
      v-else-if="isLoading && schedules.length === 0"
      class="flex items-center gap-2 px-[23px] pt-3 text-caption text-muted-foreground/60"
    >
      <Spinner class="size-3" />
    </div>

    <!-- Grouped list -->
    <div
      v-else
      class="flex-1 min-h-0 overflow-y-auto sidebar-scroll"
    >
      <!-- Empty -->
      <div
        v-if="groupedSchedules.length === 0"
        class="px-2 pt-2 space-y-1.5"
      >
        <p class="pl-[11px] text-caption text-muted-foreground/60">
          {{ t('bots.schedule.empty') }}
        </p>
        <button
          type="button"
          class="pl-[11px] text-left text-caption text-muted-foreground transition-colors hover:text-foreground"
          @click="handleManage"
        >
          + {{ t('bots.schedule.create') }}
        </button>
      </div>

      <!-- Groups -->
      <template v-else>
        <div
          v-for="(group, groupIdx) in groupedSchedules"
          :key="group.key"
        >
          <!-- Separator between groups -->
          <div
            v-if="groupIdx > 0"
            class="mx-3 my-1.5 h-px bg-border/50"
          />

          <!-- Group header: time label + gear (first group only) -->
          <div class="flex h-8 items-center px-2 mt-2">
            <span class="flex-1 pl-[11px] text-xs font-[550] tracking-[-0.02em] text-muted-foreground/80">
              {{ group.label }}
            </span>
            <Button
              v-if="groupIdx === 0"
              variant="ghost"
              size="icon-sm"
              class="shrink-0 rounded-full text-muted-foreground hover:text-foreground"
              :title="t('chat.manageSchedule')"
              :aria-label="t('chat.manageSchedule')"
              @click="handleManage"
            >
              <Settings2
                :stroke-width="1.75"
                class="size-[15px]"
              />
            </Button>
          </div>

          <!-- Cards in group -->
          <div class="px-2 pb-1 space-y-1.5">
            <div
              v-for="item in group.tasks"
              :key="item.id"
              class="group/card relative flex items-center rounded-[var(--radius-menu-shell)] border border-border bg-card transition-colors hover:bg-accent/30 dark:hover:bg-accent"
            >
              <button
                type="button"
                class="min-w-0 flex-1 px-3 py-2.5 text-left"
                @click="handleOpenTask(item)"
              >
                <p class="truncate text-control font-normal text-foreground leading-snug">
                  {{ item.name }}
                </p>
                <p class="truncate text-caption text-muted-foreground mt-0.5 leading-snug">
                  {{ item.description?.trim() || describeItem(item.pattern) || item.pattern }}
                </p>
              </button>

              <!-- Hover actions -->
              <div
                class="flex shrink-0 items-center pr-1.5 opacity-0 transition-opacity group-hover/card:opacity-100"
                :class="openMenuIds.has(item.id ?? '') ? 'opacity-100' : ''"
                @click.stop
              >
                <DropdownMenu @update:open="(open: boolean) => { if (open) openMenuIds.add(item.id ?? ''); else openMenuIds.delete(item.id ?? '') }">
                  <DropdownMenuTrigger as-child>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      class="size-6"
                      :aria-label="t('common.actions')"
                    >
                      <MoreHorizontal class="size-3.5" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem
                      class="gap-2"
                      @select="handleOpenTask(item)"
                    >
                      <Pencil class="size-3.5" />
                      {{ t('bots.schedule.edit') }}
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      class="gap-2 text-destructive focus:text-destructive"
                      @select="deleteTarget = item"
                    >
                      <Trash2 class="size-3.5" />
                      {{ t('bots.schedule.delete') }}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>

    <!-- Delete confirmation -->
    <Dialog
      :open="!!deleteTarget"
      @update:open="(v) => { if (!v) deleteTarget = null }"
    >
      <DialogContent class="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{{ t('bots.schedule.deleteTitle') }}</DialogTitle>
        </DialogHeader>
        <p class="text-sm text-muted-foreground">
          {{ t('bots.schedule.deleteConfirm', { name: deleteTarget?.name ?? '' }) }}
        </p>
        <DialogFooter class="gap-2">
          <Button
            variant="outline"
            @click="deleteTarget = null"
          >
            {{ t('common.cancel') }}
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
            {{ t('bots.schedule.delete') }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed, onMounted, reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import { MoreHorizontal, Pencil, Settings2, Trash2 } from 'lucide-vue-next'
import { toast } from '@memohai/ui'
import { useQueryCache } from '@pinia/colada'
import {
  Button, Spinner,
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
  DropdownMenu, DropdownMenuContent, DropdownMenuTrigger,
  DropdownMenuItem, DropdownMenuSeparator,
} from '@memohai/ui'
import { getBotsByBotIdSchedule, deleteBotsByBotIdScheduleById } from '@memohai/sdk'
import type { ScheduleSchedule } from '@memohai/sdk'
import { useChatStore } from '@/store/chat-list'
import { useWorkspaceTabsStore } from '@/store/workspace-tabs'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { describeCron, nextRuns } from '@/utils/cron-pattern'

const { t, locale } = useI18n()
const router = useRouter()
const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)
const workspaceTabs = useWorkspaceTabsStore()
const { sidebarView } = storeToRefs(workspaceTabs)
const queryCache = useQueryCache()

const isLoading = ref(false)
const schedules = ref<ScheduleSchedule[]>([])
const openMenuIds = reactive(new Set<string>())
const deleteTarget = ref<ScheduleSchedule | null>(null)
const isDeleting = ref(false)

const cronLocale = computed<'en' | 'zh'>(() => (locale.value.startsWith('zh') ? 'zh' : 'en'))
const uiLocale = computed(() => locale.value.startsWith('zh') ? 'zh-CN' : 'en-US')

const effectiveTimezone = computed(() => {
  try { return Intl.DateTimeFormat().resolvedOptions().timeZone } catch { return 'UTC' }
})

// --- Grouping by exact (day + HH:MM) ---

interface ScheduleGroup {
  key: string
  label: string
  tasks: ScheduleSchedule[]
}

function isSameDay(a: Date, b: Date) {
  return a.getFullYear() === b.getFullYear()
    && a.getMonth() === b.getMonth()
    && a.getDate() === b.getDate()
}

function groupLabel(date: Date, now: Date): string {
  const tomorrow = new Date(now)
  tomorrow.setDate(now.getDate() + 1)
  const timeStr = date.toLocaleTimeString(uiLocale.value, { hour: '2-digit', minute: '2-digit', hour12: false })
  const diff = date.getTime() - now.getTime()
  const week = 7 * 86_400_000

  if (isSameDay(date, now)) {
    const todayWord = locale.value.startsWith('zh') ? '今天' : 'Today'
    return `${todayWord} · ${timeStr}`
  }
  if (isSameDay(date, tomorrow)) {
    const tomorrowWord = locale.value.startsWith('zh') ? '明天' : 'Tomorrow'
    return `${tomorrowWord} · ${timeStr}`
  }
  if (diff < week) {
    const dayAbbr = date.toLocaleDateString(uiLocale.value, { weekday: 'short' })
    return `${dayAbbr} · ${timeStr}`
  }
  const dateStr = date.toLocaleDateString(uiLocale.value, { month: 'short', day: 'numeric' })
  return `${dateStr} · ${timeStr}`
}

const groupedSchedules = computed<ScheduleGroup[]>(() => {
  const now = new Date()
  const tz = effectiveTimezone.value

  const pairs = schedules.value.flatMap(task => {
    if (!task.pattern) return []
    const runs = nextRuns(task.pattern, tz, 1)
    const next = runs[0]
    if (!next) return []
    return [{ task, next }]
  })

  // Group by (year, month, day, hour, minute) — exact time
  const map = new Map<string, { date: Date; tasks: ScheduleSchedule[] }>()
  for (const { task, next } of pairs) {
    const key = `${next.getFullYear()}-${next.getMonth()}-${next.getDate()}-${next.getHours()}-${next.getMinutes()}`
    if (!map.has(key)) map.set(key, { date: next, tasks: [] })
    map.get(key)!.tasks.push(task)
  }

  return Array.from(map.entries())
    .sort(([, a], [, b]) => a.date.getTime() - b.date.getTime())
    .map(([key, { date, tasks }]) => ({
      key,
      label: groupLabel(date, now),
      tasks,
    }))
})

// ---

function describeItem(pattern: string | undefined): string | undefined {
  if (!pattern) return undefined
  return describeCron(pattern, cronLocale.value)
}

async function fetchSchedules() {
  const botId = currentBotId.value
  if (!botId) { schedules.value = []; return }
  isLoading.value = true
  try {
    const { data } = await getBotsByBotIdSchedule({ path: { bot_id: botId } })
    schedules.value = data?.items || []
  } catch {
    schedules.value = []
  } finally {
    isLoading.value = false
  }
}

// Refresh when user switches TO the schedule tab
watch(sidebarView, (view) => {
  if (view === 'schedule') void fetchSchedules()
})

// Refresh when bot-schedule.vue invalidates the cache (after create/edit/delete)
watch(
  () => {
    const entries = queryCache.getEntries({ key: ['bot-schedule', currentBotId.value ?? ''] })
    return entries[0]?.state.value.data
  },
  (next, prev) => {
    if (!currentBotId.value || next === prev) return
    void fetchSchedules()
  },
)

// Refresh when active bot changes
watch(currentBotId, () => void fetchSchedules())

async function confirmDelete() {
  const item = deleteTarget.value
  if (!item?.id || !currentBotId.value) return
  isDeleting.value = true
  try {
    await deleteBotsByBotIdScheduleById({
      path: { bot_id: currentBotId.value, id: item.id },
      throwOnError: true,
    })
    toast.success(t('bots.schedule.deleteSuccess'))
    deleteTarget.value = null
    await fetchSchedules()
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.schedule.deleteFailed')))
  } finally {
    isDeleting.value = false
  }
}

function handleManage() {
  const botId = currentBotId.value
  if (!botId) return
  void router.push({ name: 'bot-detail', params: { botName: botId }, query: { tab: 'schedule' } })
}

function handleOpenTask(_item: ScheduleSchedule) {
  const botId = currentBotId.value
  if (!botId) return
  void router.push({ name: 'bot-detail', params: { botName: botId }, query: { tab: 'schedule' } })
}

onMounted(() => void fetchSchedules())
</script>
