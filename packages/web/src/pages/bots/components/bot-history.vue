<template>
  <div class="space-y-4">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-3">
        <h3 class="text-lg font-medium">
          {{ $t('bots.history.title') }}
        </h3>
        <Badge
          v-if="messages.length"
          variant="secondary"
          class="text-xs"
        >
          {{ $t('bots.history.messageCount', { count: messages.length }) }}
        </Badge>
      </div>
      <div class="flex items-center gap-2">
        <NativeSelect
          v-model="roleFilter"
          class="h-9 w-32 text-sm"
        >
          <option value="">
            {{ $t('bots.history.filterAll') }}
          </option>
          <option value="user">
            {{ $t('bots.history.role.user') }}
          </option>
          <option value="assistant">
            {{ $t('bots.history.role.assistant') }}
          </option>
          <option value="tool">
            {{ $t('bots.history.role.tool') }}
          </option>
          <option value="system">
            {{ $t('bots.history.role.system') }}
          </option>
        </NativeSelect>
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
      v-if="isLoading && messages.length === 0"
      class="flex items-center justify-center py-8 text-sm text-muted-foreground"
    >
      <Spinner class="mr-2" />
      {{ $t('common.loading') }}
    </div>

    <!-- Empty -->
    <div
      v-else-if="!isLoading && messages.length === 0"
      class="flex flex-col items-center justify-center py-12 text-center"
    >
      <div class="rounded-full bg-muted p-3 mb-4">
        <FontAwesomeIcon
          :icon="['fas', 'message']"
          class="size-6 text-muted-foreground"
        />
      </div>
      <p class="text-sm text-muted-foreground">
        {{ $t('bots.history.empty') }}
      </p>
    </div>

    <!-- Messages -->
    <template v-else>
      <MessageList :messages="pagedMessages" />

      <!-- Pagination -->
      <div
        v-if="totalPages > 1"
        class="flex items-center justify-between gap-3 pt-4"
      >
        <span class="shrink-0 whitespace-nowrap text-sm text-muted-foreground">
          {{ paginationSummary }}
        </span>
        <Pagination
          :total="filteredMessages.length"
          :items-per-page="PAGE_SIZE"
          :sibling-count="1"
          :page="currentPage"
          class="w-auto shrink-0"
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
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import {
  Button, Badge, Spinner, NativeSelect,
  Pagination, PaginationContent, PaginationEllipsis,
  PaginationFirst, PaginationItem, PaginationLast,
  PaginationNext, PaginationPrevious,
} from '@memoh/ui'
import MessageList from './message-list.vue'
import {
  getBotsByBotIdMessages,
} from '@memoh/sdk'
import type { MessageMessage } from '@memoh/sdk'
import { resolveApiErrorMessage } from '@/utils/api-error'

const props = defineProps<{
  botId: string
}>()

const { t } = useI18n()

const isLoading = ref(false)
const messages = ref<MessageMessage[]>([])
const roleFilter = ref('')
const currentPage = ref(1)

const PAGE_SIZE = 20

const filteredMessages = computed(() => {
  if (!roleFilter.value) return messages.value
  return messages.value.filter(m => m.role === roleFilter.value)
})

const totalPages = computed(() => Math.ceil(filteredMessages.value.length / PAGE_SIZE))

const pagedMessages = computed(() => {
  const start = (currentPage.value - 1) * PAGE_SIZE
  return filteredMessages.value.slice(start, start + PAGE_SIZE)
})

const paginationSummary = computed(() => {
  const total = filteredMessages.value.length
  if (total === 0) return ''
  const start = (currentPage.value - 1) * PAGE_SIZE + 1
  const end = Math.min(currentPage.value * PAGE_SIZE, total)
  return `${start}-${end} / ${total}`
})

watch(roleFilter, () => {
  currentPage.value = 1
})

async function fetchAllHistory() {
  if (!props.botId) return
  isLoading.value = true
  messages.value = []

  try {
    let before: string | undefined
    let hasMore = true

    while (hasMore) {
      const { data } = await getBotsByBotIdMessages({
        path: { bot_id: props.botId },
        query: { limit: 100, before },
        throwOnError: true,
      })
      const items = data?.items || []
      if (items.length === 0) {
        hasMore = false
      } else {
        messages.value.push(...items)
        before = items[items.length - 1]?.created_at
        hasMore = items.length >= 100
      }
    }
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t('bots.history.loadFailed')))
  } finally {
    isLoading.value = false
  }
}

async function handleRefresh() {
  currentPage.value = 1
  await fetchAllHistory()
}

onMounted(() => {
  fetchAllHistory()
})
</script>
