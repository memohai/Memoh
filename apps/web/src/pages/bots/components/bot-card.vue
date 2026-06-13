<template>
  <button
    type="button"
    class="group/card relative flex w-52 flex-col items-center rounded-[var(--radius-menu-shell)] border border-border bg-card p-5 text-center transition-colors hover:bg-accent/30 dark:hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60 disabled:hover:bg-card"
    :disabled="isPending"
    :aria-label="`${bot.display_name || bot.id}`"
    @click="onOpenDetail"
  >
    <!-- Corner status: present only when the bot needs attention. A healthy,
         active bot shows nothing here; hover is just the gray-ladder fill. -->
    <div class="absolute right-3 top-3 flex h-5 items-center">
      <LoaderCircle
        v-if="isPending"
        class="size-4 animate-spin text-muted-foreground"
      />
      <span
        v-else-if="hasIssue"
        class="flex items-center text-destructive"
        :title="issueTitle"
      >
        <AlertTriangle class="size-4" />
      </span>
      <span
        v-else-if="!bot.is_active"
        class="size-2 rounded-full bg-muted-foreground/40"
        :title="$t('bots.inactive')"
      />
    </div>

    <Avatar
      class="size-14 shrink-0"
      :class="{ 'opacity-60': !bot.is_active && !isPending }"
    >
      <AvatarImage
        v-if="bot.avatar_url"
        :src="bot.avatar_url"
        :alt="bot.display_name"
      />
      <AvatarFallback class="text-base">
        {{ avatarFallback }}
      </AvatarFallback>
    </Avatar>

    <div class="mt-3 w-full min-w-0">
      <div class="truncate text-sm font-medium text-foreground">
        {{ bot.display_name || bot.id }}
      </div>
    </div>
  </button>
</template>

<script setup lang="ts">
import {
  Avatar,
  AvatarImage,
  AvatarFallback,
} from '@memohai/ui'
import { AlertTriangle, LoaderCircle } from 'lucide-vue-next'
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { BotsBot } from '@memohai/sdk'
import { useAvatarInitials } from '@/composables/useAvatarInitials'
import { useBotStatusMeta } from '@/composables/useBotStatusMeta'

const router = useRouter()
const { t } = useI18n()

const props = defineProps<{
  bot: BotsBot
}>()

const botRef = computed(() => props.bot)

const avatarFallback = useAvatarInitials(() => props.bot.display_name || props.bot.id)

const { hasIssue, isPending, issueTitle } = useBotStatusMeta(botRef, t)

function onOpenDetail() {
  if (isPending.value) return
  router.push({ name: 'bot-detail', params: { botName: props.bot.name ?? props.bot.id } })
}
</script>
