<template>
  <div class="w-56 shrink-0 border-r flex flex-col h-full">
    <div class="p-3 border-b">
      <p class="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
        {{ $t('sidebar.bots') }}
      </p>
    </div>

    <ScrollArea class="flex-1">
      <div class="p-1">
        <!-- Loading -->
        <div
          v-if="isLoading"
          class="flex justify-center py-4"
        >
          <FontAwesomeIcon
            :icon="['fas', 'spinner']"
            class="size-4 animate-spin text-muted-foreground"
          />
        </div>

        <!-- Bot list -->
        <button
          v-for="bot in bots"
          :key="bot.id"
          type="button"
          :aria-pressed="currentBotId === bot.id"
          class="flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-sm transition-colors hover:bg-accent"
          :class="{ 'bg-accent': currentBotId === bot.id }"
          @click="handleSelect(bot)"
        >
          <Avatar class="size-8 shrink-0">
            <AvatarImage
              v-if="bot.avatar_url"
              :src="bot.avatar_url"
              :alt="bot.display_name"
            />
            <AvatarFallback class="text-xs">
              {{ (bot.display_name || bot.id).slice(0, 2).toUpperCase() }}
            </AvatarFallback>
          </Avatar>
          <div class="flex-1 text-left min-w-0">
            <div class="font-medium truncate">
              {{ bot.display_name || bot.id }}
            </div>
            <div
              v-if="bot.type"
              class="text-xs text-muted-foreground truncate"
            >
              {{ botTypeLabel(bot.type) }}
            </div>
          </div>
        </button>

        <!-- Empty -->
        <div
          v-if="!isLoading && bots.length === 0"
          class="px-3 py-6 text-center text-sm text-muted-foreground"
        >
          {{ $t('bots.emptyTitle') }}
        </div>
      </div>
    </ScrollArea>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Avatar, AvatarImage, AvatarFallback, ScrollArea } from '@memoh/ui'
import { useQuery } from '@pinia/colada'
import { getBotsQuery } from '@memoh/sdk/colada'
import type { BotsBot } from '@memoh/sdk'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'

const { t } = useI18n()
const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)

const { data: botData, isLoading } = useQuery(getBotsQuery())
const bots = computed<BotsBot[]>(() => botData.value?.items ?? [])

function botTypeLabel(type: string): string {
  if (!type) return ''
  const key = `bots.types.${type}`
  const out = t(key)
  return out !== key ? out : type
}

function handleSelect(bot: BotsBot) {
  chatStore.selectBot(bot.id)
}
</script>
