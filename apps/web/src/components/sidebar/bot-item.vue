<template>
  <SidebarMenuButton
    :tooltip="bot.display_name || bot.id"
    as-child
  >
    <button
      :class="[
        'flex items-center gap-2.5 w-full h-[38px] px-2.5 rounded-lg transition-colors',
        isActive
          ? 'bg-background'
          : bot.status === 'error'
            ? 'opacity-50 cursor-not-allowed'
            : 'hover:bg-background/60',
      ]"
      :disabled="bot.status === 'error'"
      @click="handleSelect"
    >
      <div class="size-[26px] shrink-0 rounded-full border border-border bg-accent overflow-hidden p-px">
        <img
          v-if="bot.avatar_url"
          :src="bot.avatar_url"
          :alt="bot.display_name || bot.id"
          class="size-full rounded-full object-cover"
        >
        <span
          v-else
          class="size-full flex items-center justify-center text-[8px] font-medium text-muted-foreground"
        >
          {{ avatarFallback }}
        </span>
      </div>
      <span class="truncate text-xs font-medium text-foreground leading-[18px]">
        {{ bot.display_name || bot.id }}
      </span>
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
import { SidebarMenuButton } from '@memohai/ui'

const props = defineProps<{ bot: BotsBot }>()

const router = useRouter()
const chatStore = useChatStore()
const { currentBotId } = storeToRefs(chatStore)

const displayName = computed(() => props.bot.display_name || props.bot.id || '')
const avatarFallback = useAvatarInitials(() => displayName.value, 'B')

const isActive = computed(() => currentBotId.value === props.bot.id)

function handleSelect() {
  if (props.bot.status === 'error') return
  chatStore.selectBot(props.bot.id ?? '')
  router.push({ name: 'chat', params: { botId: props.bot.id } })
}
</script>
