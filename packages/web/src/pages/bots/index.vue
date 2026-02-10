<template>
  <section class="p-6 max-w-7xl mx-auto">
    <!-- Header: search + create -->
    <div class="flex items-center justify-between mb-6">
      <h3 class="text-xl font-semibold tracking-tight">
        {{ $t('bots.title') }}
      </h3>
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
        <CreateBot
          v-model:open="dialogOpen"
          v-model:edit-bot="editingBot"
        />
      </div>
    </div>

    <!-- Bot grid -->
    <div
      v-if="filteredBots.length > 0"
      class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4"
    >
      <BotCard
        v-for="bot in filteredBots"
        :key="bot.id"
        :bot="bot"
        :delete-loading="deleteLoading"
        @edit="handleEdit"
        @delete="handleDelete"
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
import { ref, computed } from 'vue'
import BotCard from './components/bot-card.vue'
import CreateBot from './components/create-bot.vue'
import { useBotList, useDeleteBot, type BotInfo } from '@/composables/api/useBots'

const searchText = ref('')
const dialogOpen = ref(false)
const editingBot = ref<BotInfo | null>(null)

const { data: botData, status } = useBotList()
const { mutate: deleteBot, isLoading: deleteLoading } = useDeleteBot()

const isLoading = computed(() => status.value === 'loading')

const filteredBots = computed(() => {
  const list = botData.value ?? []
  const keyword = searchText.value.trim().toLowerCase()
  if (!keyword) return list
  return list.filter((bot) =>
    bot.display_name?.toLowerCase().includes(keyword)
    || bot.id.toLowerCase().includes(keyword)
    || bot.type?.toLowerCase().includes(keyword),
  )
})

function handleEdit(bot: BotInfo) {
  editingBot.value = bot
  dialogOpen.value = true
}

async function handleDelete(id: string) {
  try {
    await deleteBot(id)
  } catch {
    return
  }
}
</script>
