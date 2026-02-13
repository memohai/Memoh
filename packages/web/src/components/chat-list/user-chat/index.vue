<template>
  <div class="flex gap-3 items-start justify-end not-first:mt-6">
    <p
      class="leading-7 max-w-[85%] text-foreground bg-muted/90 py-3 px-4 rounded-xl rounded-tr-none break-all border border-border/80 shadow-sm"
    >
      {{ userSay.description }}
    </p>
    <div class="relative shrink-0 flex items-end">
      <Avatar class="size-8">
        <AvatarImage
          v-if="userInfo.avatarUrl"
          :src="userInfo.avatarUrl"
          :alt="userFallback"
        />
        <AvatarFallback class="text-xs">
          {{ userFallback }}
        </AvatarFallback>
      </Avatar>
      <ChannelBadge :platform="platformKey" />
    </div>
  </div>
</template>

<script setup lang="ts">
import type { user } from '@memoh/shared'
import { Avatar, AvatarFallback, AvatarImage } from '@memoh/ui'
import { computed } from 'vue'
import { useUserStore } from '@/store/user'
import ChannelBadge from '../channel-badge/index.vue'

const { userSay } = defineProps<{
  userSay: user
}>()

const { userInfo } = useUserStore()
const userFallback = computed(() => {
  const name = userInfo.displayName || userInfo.username || userInfo.id || ''
  return name.slice(0, 2).toUpperCase() || 'U'
})

const platformKey = computed(() => (userSay.platform ?? '').trim().toLowerCase())
</script>