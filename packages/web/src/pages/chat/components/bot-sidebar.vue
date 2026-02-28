<template>
  <SidebarMenu
    v-for="bot in bots"
    :key="bot.id"
  >
    <SidebarMenuItem>
      <SidebarMenuButton
        as-child
        class="justify-start py-5! px-4"
      >
        <Toggle
          :class="`p-2! border border-transparent h-[initial]! ${currentBotId === bot.id ? 'border-inherit' : ''}`"
          :model-value="isActive(bot.id as string).value"
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
        </Toggle>
      </SidebarMenuButton>
    </SidebarMenuItem>
  </SidebarMenu>
  <SidebarMenu>
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
      class="px-3 py-6 text-center text-sm text-muted-foreground"
    >
      {{ $t('bots.emptyTitle') }}
    </div>
  </SidebarMenu>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Avatar, AvatarImage, AvatarFallback } from '@memoh/ui'
import { useQuery } from '@pinia/colada'
import { getBotsQuery } from '@memoh/sdk/colada'
import type { BotsBot } from '@memoh/sdk'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import {
  Toggle,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarHeader
 } from '@memoh/ui'


const { t } = useI18n()
const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)

const { data: botData, isLoading } = useQuery(getBotsQuery())
const bots = computed<BotsBot[]>(() => botData.value?.items ?? [])

const isActive = (id: string) => computed(() => {
  return currentBotId.value === id
})

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
