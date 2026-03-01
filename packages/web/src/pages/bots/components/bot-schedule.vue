<template>
  <div class="space-y-4">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-3">
        <h3 class="text-lg font-medium">
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
      </div>
    </div>

    <!-- Loading -->
    <div
      v-if="isLoading && schedules.length === 0"
      class="flex items-center justify-center py-8 text-sm text-muted-foreground"
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
        <FontAwesomeIcon
          :icon="['fas', 'calendar-alt']"
          class="size-6 text-muted-foreground"
        />
      </div>
      <p class="text-sm text-muted-foreground">
        {{ $t('bots.schedule.empty') }}
      </p>
    </div>

    <!-- Table -->
    <template v-else>
      <div class="rounded-md border overflow-hidden">
        <table class="w-full text-sm">
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
                {{ $t('bots.schedule.createdAt') }}
              </th>
              <th class="px-4 py-2 text-left font-medium">
                {{ $t('bots.schedule.updatedAt') }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="item in pagedSchedules"
              :key="item.id"
              class="border-b last:border-0 hover:bg-muted/30"
            >
              <td class="px-4 py-2 font-medium">
                <div>{{ item.name }}</div>
                <div class="text-xs text-muted-foreground line-clamp-1">
                  {{ item.description }}
                </div>
              </td>
              <td class="px-4 py-2">
                <code class="text-xs bg-muted px-1.5 py-0.5 rounded">
                  {{ item.pattern }}
                </code>
              </td>
              <td class="px-4 py-2">
                <Badge :variant="item.enabled ? 'secondary' : 'outline'">
                  {{ item.enabled ? $t('bots.schedule.statusEnabled') : $t('bots.schedule.statusDisabled') }}
                </Badge>
              </td>
              <td class="px-4 py-2 text-muted-foreground">
                {{ item.current_calls ?? 0 }} / {{ item.max_calls || $t('bots.schedule.unlimited') }}
              </td>
              <td class="px-4 py-2 text-muted-foreground">
                {{ formatDateTime(item.created_at) }}
              </td>
              <td class="px-4 py-2 text-muted-foreground">
                {{ formatDateTime(item.updated_at) }}
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
        <span class="text-sm text-muted-foreground">
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
              />
            </template>
            <PaginationNext />
            <PaginationLast />
          </PaginationContent>
        </Pagination>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import {
  Button, Badge, Spinner,
  Pagination, PaginationContent, PaginationEllipsis,
  PaginationFirst, PaginationItem, PaginationLast,
  PaginationNext, PaginationPrevious,
} from '@memoh/ui'
import { getBotsByBotIdSchedule } from '@memoh/sdk'
import type { ScheduleSchedule } from '@memoh/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'
import { formatDateTime } from '@/utils/date-time'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()

const isLoading = ref(false)
const schedules = ref<ScheduleSchedule[]>([])
const currentPage = ref(1)
const PAGE_SIZE = 10

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

async function handleRefresh() {
  currentPage.value = 1
  await fetchSchedules()
}

onMounted(() => {
  fetchSchedules()
})
</script>
