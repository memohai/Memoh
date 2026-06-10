<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child>
      <Button
        v-if="variant === 'rail'"
        variant="ghost"
        size="icon-sm"
        class="shrink-0"
        :aria-label="t('sidebar.switchBot')"
        :title="currentLabel"
      >
        <Avatar class="size-6 shrink-0 border border-border bg-accent">
          <AvatarImage
            v-if="currentBot?.avatar_url"
            :src="currentBot.avatar_url"
            :alt="currentLabel"
          />
          <AvatarFallback class="text-[9px] bg-accent text-muted-foreground">
            {{ avatarFallback }}
          </AvatarFallback>
        </Avatar>
      </Button>
      <Button
        v-else
        variant="ghost"
        class="h-7 w-full min-w-0 max-w-full justify-start gap-2 px-1.5 text-xs"
        :aria-label="t('sidebar.switchBot')"
      >
        <Avatar class="size-5 shrink-0 border border-border bg-accent">
          <AvatarImage
            v-if="currentBot?.avatar_url"
            :src="currentBot.avatar_url"
            :alt="currentLabel"
          />
          <AvatarFallback class="text-[8px] bg-accent text-muted-foreground">
            {{ avatarFallback }}
          </AvatarFallback>
        </Avatar>
        <span class="truncate">
          {{ currentLabel }}
        </span>
        <ChevronsUpDown class="size-3 shrink-0 text-muted-foreground" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent
      align="start"
      :side="variant === 'rail' ? 'right' : 'bottom'"
      class="w-60"
    >
      <DropdownMenuLabel class="text-xs font-[475] text-muted-foreground">
        {{ t('sidebar.bots') }}
      </DropdownMenuLabel>
      <DropdownMenuItem
        v-for="bot in bots"
        :key="bot.id"
        class="gap-2"
        :disabled="bot.status === 'error'"
        @select="handleSelect(bot)"
      >
        <Avatar class="size-5 shrink-0 border border-border bg-accent">
          <AvatarImage
            v-if="bot.avatar_url"
            :src="bot.avatar_url"
            :alt="bot.display_name || bot.id"
          />
          <AvatarFallback class="text-[8px] bg-accent text-muted-foreground">
            {{ initialsOf(bot) }}
          </AvatarFallback>
        </Avatar>
        <span class="min-w-0 flex-1 truncate text-xs">
          {{ bot.display_name || bot.id }}
        </span>
        <Check
          v-if="bot.id === currentBotId"
          class="size-3 shrink-0 text-muted-foreground"
        />
      </DropdownMenuItem>
      <div
        v-if="isLoading"
        class="flex justify-center py-3"
      >
        <LoaderCircle class="size-3.5 animate-spin text-muted-foreground" />
      </div>
      <div
        v-if="!isLoading && bots.length === 0"
        class="px-2 py-3 text-center text-xs text-muted-foreground"
      >
        {{ t('bots.emptyTitle') }}
      </div>
      <DropdownMenuSeparator />
      <DropdownMenuItem
        class="gap-2"
        @select="router.push({ name: 'bot-new' })"
      >
        <Plus class="size-3.5 text-muted-foreground" />
        <span class="text-xs">{{ t('bots.createBot') }}</span>
      </DropdownMenuItem>
      <DropdownMenuItem
        class="gap-2"
        @select="router.push({ name: 'bots' })"
      >
        <Settings2 class="size-3.5 text-muted-foreground" />
        <span class="text-xs">{{ t('sidebar.manageBots') }}</span>
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useQuery } from '@pinia/colada'
import { getBotsQuery } from '@memohai/sdk/colada'
import type { BotsBot } from '@memohai/sdk'
import {
  Avatar,
  AvatarImage,
  AvatarFallback,
  Button,
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from '@memohai/ui'
import { Check, ChevronsUpDown, LoaderCircle, Plus, Settings2 } from 'lucide-vue-next'
import { useChatStore } from '@/store/chat-list'
import { usePinnedBots } from '@/composables/usePinnedBots'

withDefaults(defineProps<{
  variant?: 'row' | 'rail'
}>(), {
  variant: 'row',
})

const router = useRouter()
const { t } = useI18n()
const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)
const { sortBots } = usePinnedBots()

const { data: botData, isLoading } = useQuery(getBotsQuery())
const bots = computed<BotsBot[]>(() => sortBots(botData.value?.items ?? []))

const currentBot = computed(() =>
  bots.value.find((bot) => bot.id === currentBotId.value) ?? null,
)
const currentLabel = computed(() =>
  currentBot.value
    ? currentBot.value.display_name || currentBot.value.id || ''
    : t('chat.selectBot'),
)

function initialsOf(bot: BotsBot): string {
  const name = (bot.display_name || bot.id || '').trim()
  return name ? name.charAt(0).toUpperCase() : 'B'
}

const avatarFallback = computed(() =>
  currentBot.value ? initialsOf(currentBot.value) : 'B',
)

function handleSelect(bot: BotsBot) {
  if (bot.status === 'error') return
  const id = bot.id ?? ''
  if (!id || id === currentBotId.value) return
  void chatStore.selectBot(id)
  void router.push({ name: 'bot', params: { botName: bot.name ?? id } })
}
</script>
