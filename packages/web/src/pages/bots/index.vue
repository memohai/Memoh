<template>
  <section class="p-4  mx-auto">
    <!-- Header: search + create -->
    <div class="flex items-center justify-between mb-6">
      <h2 class="text-xl font-semibold tracking-tight">
        {{ $t('bots.title') }}
      </h2>
      <div class="flex items-center gap-3">
        <div class="relative">
          <FontAwesomeIcon
            :icon="['fas', 'magnifying-glass']"
            class="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground size-3.5"
          />
          <Input
            v-model="searchText"
            :placeholder="$t('bots.searchPlaceholder')"
            class="pl-9 w-64"
          />
        </div>
        <CreateBot v-model:open="dialogOpen" />
      </div>
    </div>

    <!-- Bot grid -->
    <div
      v-if="filteredBots.length > 0"
      class="grid gap-4"
      style="grid-template-columns:repeat(auto-fill,minmax(300px,calc(25% - 1em)))!important"
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
          <FontAwesomeIcon :icon="['fas', 'robot']" />
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
  Input,
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@memoh/ui'
import { ref, computed, watch, onUnmounted } from 'vue'
import BotCard from './components/bot-card.vue'
import CreateBot from './components/create-bot.vue'
import { useQuery, useQueryCache } from '@pinia/colada'
import { getBotsQuery, getBotsQueryKey } from '@memoh/sdk/colada'

const searchText = ref('')
const dialogOpen = ref(false)
const queryCache = useQueryCache()

const { data: botData, status } = useQuery(getBotsQuery())

const isLoading = computed(() => status.value === 'loading')

const allBots = computed(() => botData.value?.items ?? [])

const filteredBots = computed(() => {
  const keyword = searchText.value.trim().toLowerCase()
  if (!keyword) return allBots.value
  return allBots.value.filter((bot) =>
    bot.display_name?.toLowerCase().includes(keyword)
    || bot.id?.toLowerCase().includes(keyword)
    || bot.type?.toLowerCase().includes(keyword),
  )
})

const hasPendingBots = computed(() =>
  allBots.value.some((bot) => bot.status === 'creating' || bot.status === 'deleting'),
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
