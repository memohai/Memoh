<template>
  <div
    class="flex gap-3 items-start"
    :class="isSelf ? 'justify-end' : ''"
  >
    <!-- Other sender avatar (left) -->
    <div
      v-if="!isSelf"
      class="relative shrink-0"
    >
      <Avatar class="size-8">
        <AvatarImage
          v-if="message.senderAvatarUrl"
          :src="message.senderAvatarUrl"
          :alt="message.senderDisplayName"
        />
        <AvatarFallback class="text-xs">
          {{ senderFallback }}
        </AvatarFallback>
      </Avatar>
      <ChannelBadge
        v-if="message.platform"
        :platform="message.platform"
      />
    </div>

    <!-- Content -->
    <div class="max-w-[80%] min-w-0">
      <p
        v-if="!isSelf && message.senderDisplayName"
        class="text-xs text-muted-foreground mb-1"
      >
        {{ message.senderDisplayName }}
      </p>
      <div
        class="rounded-2xl px-4 py-2.5 text-sm whitespace-pre-wrap shadow-sm"
        :class="isSelf
          ? 'rounded-tr-none bg-muted/90 border border-border/80'
          : 'rounded-tl-none bg-accent/60'"
      >
        {{ textContent }}
      </div>
    </div>

    <!-- Self avatar (right) -->
    <div
      v-if="isSelf"
      class="relative shrink-0"
    >
      <Avatar class="size-8">
        <AvatarImage
          v-if="userAvatarUrl"
          :src="userAvatarUrl"
          alt=""
        />
        <AvatarFallback class="text-xs">
          {{ userFallback }}
        </AvatarFallback>
      </Avatar>
      <ChannelBadge
        v-if="message.platform"
        :platform="message.platform"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Avatar, AvatarImage, AvatarFallback } from '@memoh/ui'
import ChannelBadge from '@/components/chat-list/channel-badge/index.vue'
import { useUserStore } from '@/store/user'
import type { ChatMessage, TextBlock } from '@/store/chat-list'

const props = defineProps<{
  message: ChatMessage
}>()

const userStore = useUserStore()

const isSelf = computed(() => props.message.isSelf !== false)

const textContent = computed(() => {
  const block = props.message.blocks[0]
  return block?.type === 'text' ? (block as TextBlock).content : ''
})

const userAvatarUrl = computed(() => userStore.userInfo.avatarUrl ?? '')
const userFallback = computed(() => {
  const name = userStore.userInfo.displayName || userStore.userInfo.username || ''
  return name.slice(0, 2).toUpperCase() || 'U'
})

const senderFallback = computed(() => {
  const name = props.message.senderDisplayName ?? ''
  return name.slice(0, 2).toUpperCase() || '?'
})
</script>
