<template>
  <SidebarMenuButton
    :tooltip="bot.display_name || bot.id"
    as-child
  >
    <button
      :class="[
        'group/bot flex items-center gap-2.5 w-full h-9.5 px-2.5 rounded-lg transition-colors',
        'group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0',
        isActive
          ? 'bg-sidebar-accent'
          : bot.status === 'error'
            ? 'opacity-50 cursor-not-allowed'
            : 'hover:bg-sidebar-accent/60',
      ]"
      :disabled="bot.status === 'error'"
      @click="handleSelect"
    >
      <div class="size-6.5 shrink-0 rounded-full border border-border bg-accent overflow-hidden p-px group-data-[collapsible=icon]:mx-auto">
        <img
          v-if="bot.avatar_url&&!isError"
          :src="bot.avatar_url"
          :alt="bot.display_name || bot.id"
          class="size-full rounded-full object-cover "
          @error="() => {
            isError = true;
          }"
          @load="() => { 
            isError = false
          }"
        >
        <span
          v-else
          class="size-full flex items-center justify-center text-[8px] font-medium text-muted-foreground"
        >
          {{ avatarFallback }}
        </span>
      </div>
      <span class="truncate text-xs font-medium text-foreground leading-4.5 flex-1 text-left group-data-[collapsible=icon]:hidden">
        {{ bot.display_name || bot.id }}
      </span>

      <div class="group-data-[collapsible=icon]:hidden">
        <DropdownMenu>
          <DropdownMenuTrigger
            as-child
            @click.stop
          >
            <span
              class="shrink-0 size-6 flex items-center justify-center rounded text-muted-foreground opacity-0 group-hover/bot:opacity-100 hover:text-foreground hover:bg-accent transition-opacity"
            >
              <Ellipsis
                class="size-3"
              />
            </span>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            align="start"
            side="bottom"
            @click.stop
          >
            <DropdownMenuItem @click.stop="handleTogglePin">
              <Pin
                class="size-3 mr-2"
              />
              {{ pinned ? $t('common.unpin') : $t('common.pin') }}
            </DropdownMenuItem>
            <DropdownMenuItem @click.stop="handleDetails">
              <Settings
                class="size-3 mr-2"
              />
              {{ $t('common.details') }}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </button>
  </SidebarMenuButton>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import type { BotsBot } from '@memohai/sdk'
import { useChatStore } from '@/store/chat-list'
import { useAvatarInitials } from '@/composables/useAvatarInitials'
import { usePinnedBots } from '@/composables/usePinnedBots'
import { Ellipsis, Pin, Settings } from 'lucide-vue-next'
import {
  SidebarMenuButton,
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@memohai/ui'
import { ref } from 'vue'

const props = defineProps<{ bot: BotsBot }>()

const router = useRouter()
const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)
const { isPinned, togglePin } = usePinnedBots()

const displayName = computed(() => props.bot.display_name || props.bot.id || '')
const avatarFallback = useAvatarInitials(() => displayName.value, 'B')

const isActive = computed(() => currentBotId.value === props.bot.id)
const pinned = computed(() => isPinned(props.bot.id ?? ''))
const isError=ref(false)

function handleSelect() {
  if (props.bot.status === 'error') return
  chatStore.selectBot(props.bot.id ?? '')
  router.push({ name: 'chat', params: { botId: props.bot.id } })
}

function handleDetails() {
  router.push({ name: 'bot-detail', params: { botId: props.bot.id } })
}

function handleTogglePin() {
  togglePin(props.bot.id ?? '')
}
</script>
