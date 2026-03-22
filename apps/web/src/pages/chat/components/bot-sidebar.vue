<template>
  <div class="flex flex-col gap-1 px-1.5">
    <button
      v-for="bot in bots"
      :key="bot.id"
      class="flex items-center gap-2.5 h-[58px] w-full rounded-lg px-2.5 text-left transition-colors"
      :class="currentBotId === bot.id
        ? 'bg-card border border-border'
        : 'hover:bg-card/60'"
      @click="handleSelect(bot)"
    >
      <Avatar class="size-[26px] shrink-0 border border-border">
        <AvatarImage
          v-if="bot.avatar_url"
          :src="bot.avatar_url"
          :alt="bot.display_name"
        />
        <AvatarFallback class="text-[9px] bg-secondary text-muted-foreground">
          {{ (bot.display_name || bot.id || '').slice(0, 2).toUpperCase() }}
        </AvatarFallback>
      </Avatar>
      <div class="flex-1 min-w-0">
        <div class="text-xs font-medium text-foreground truncate">
          {{ bot.display_name || bot.id }}
        </div>
      </div>
    </button>

    <div
      v-if="isLoading"
      class="flex justify-center py-4"
    >
      <FontAwesomeIcon
        :icon="['fas', 'spinner']"
        class="size-4 animate-spin text-muted-foreground"
      />
    </div>

    <div
      v-if="!isLoading && bots.length === 0"
      class="px-3 py-6 text-center text-xs text-muted-foreground"
    >
      {{ $t('bots.emptyTitle') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Avatar, AvatarImage, AvatarFallback } from '@memohai/ui'
import { useQuery } from '@pinia/colada'
import { getBotsQuery } from '@memohai/sdk/colada'
import type { BotsBot } from '@memohai/sdk'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'

const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)

const { data: botData, isLoading } = useQuery(getBotsQuery())
const bots = computed<BotsBot[]>(() => botData.value?.items ?? [])

function handleSelect(bot: BotsBot) {
  chatStore.selectBot(bot.id!)
}
</script>
