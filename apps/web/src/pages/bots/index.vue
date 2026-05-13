<template>
  <section class="px-4 pt-2 pb-10 md:px-6 md:pt-4 md:pb-12 mx-auto">
    <!-- Header: search + create -->
    <div class="flex items-center justify-end mb-6 flex-wrap">
      <div class="flex items-center gap-3">
        <div class="relative">
          <Search
            class="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground size-3.5"
          />
          <Input
            v-model="searchText"
            :placeholder="$t('bots.searchPlaceholder')"
            class="pl-9 w-64"
          />
        </div>
        <Button
          variant="default"
          @click="router.push({ name: 'bot-new' })"
        >
          <Plus class="mr-1.5" />
          {{ $t('bots.createBot') }}
        </Button>
      </div>
    </div>

    <!-- Bot grid -->
    <div
      v-if="filteredBots.length > 0"
      class="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
    >
      <BotCard
        v-for="bot in filteredBots"
        :key="bot.id"
        :bot="bot"
      />
    </div>

    <!-- Empty state -->
    <Empty
      v-else-if="!isLoading"
      class="mt-20 flex flex-col items-center justify-center"
    >
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Bot />
        </EmptyMedia>
      </EmptyHeader>
      <EmptyTitle>{{ $t('bots.emptyTitle') }}</EmptyTitle>
      <EmptyDescription>{{ $t('bots.emptyDescription') }}</EmptyDescription>
      <EmptyContent />
    </Empty>
  </section>
</template>

<script setup lang="ts">
import {
  Button,
  Input,
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@memohai/ui'
import { Search, Bot, Plus } from 'lucide-vue-next'
import { ref, computed, watch, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import BotCard from './components/bot-card.vue'
import { useQuery, useQueryCache } from '@pinia/colada'
import { getBotsQuery, getBotsQueryKey } from '@memohai/sdk/colada'

const router = useRouter()
const searchText = ref('')
const queryCache = useQueryCache()

const { data: botData, status } = useQuery(getBotsQuery())

const isLoading = computed(() => status.value === 'loading')

const allBots = computed(() => botData.value?.items ?? [])

const filteredBots = computed(() => {
  const keyword = searchText.value.trim().toLowerCase()
  if (!keyword) return allBots.value
  return allBots.value.filter(bot =>
    bot.display_name?.toLowerCase().includes(keyword)
    || bot.id?.toLowerCase().includes(keyword),
  )
})

const hasPendingBots = computed(() =>
  allBots.value.some(bot => bot.status === 'creating' || bot.status === 'deleting'),
)

let pollTimer: ReturnType<typeof setInterval> | null = null

watch(hasPendingBots, (pending) => {
  if (pending) {
    if (pollTimer == null) {
      pollTimer = setInterval(() => {
        queryCache.invalidateQueries({ key: getBotsQueryKey() })
      }, 2000)
    }
    return
  }
  if (pollTimer != null) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}, { immediate: true })

onUnmounted(() => {
  if (pollTimer != null) {
    clearInterval(pollTimer)
    pollTimer = null
  }
})
</script>
