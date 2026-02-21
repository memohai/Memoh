<template>
  <Card
    class="group relative transition-shadow"
    :class="isPending ? 'opacity-80 cursor-not-allowed' : 'hover:shadow-md cursor-pointer'"
    role="button"
    :tabindex="isPending ? -1 : 0"
    :aria-disabled="isPending"
    :aria-label="`Open bot ${(bot.display_name || bot.id)}`"
    @click="onOpenDetail"
    @keydown.enter.prevent="onOpenDetail"
    @keydown.space.prevent="onOpenDetail"
  >
    <CardHeader class="flex flex-row items-start gap-3 space-y-0 pb-2">
      <Avatar class="size-11 shrink-0">
        <AvatarImage
          v-if="bot.avatar_url"
          :src="bot.avatar_url"
          :alt="bot.display_name"
        />
        <AvatarFallback class="text-base">
          {{ avatarFallback }}
        </AvatarFallback>
      </Avatar>
      <div class="flex-1 min-w-0 flex flex-col gap-1.5">
        <div class="flex items-center justify-between gap-2">
          <CardTitle class="text-base truncate">
            {{ bot.display_name || bot.id }}
          </CardTitle>
          <Badge
            :variant="statusVariant"
            class="shrink-0 text-xs"
            :title="hasIssue ? issueTitle : undefined"
          >
            <FontAwesomeIcon
              v-if="isPending"
              :icon="['fas', 'spinner']"
              class="mr-1 size-3 animate-spin"
            />
            {{ statusLabel }}
          </Badge>
        </div>
        <div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
          <span
            v-if="bot.type"
            class="truncate"
          >
            {{ botTypeLabel }}
          </span>
          <span
            v-if="bot.type && formattedDate"
            class="text-muted-foreground/60"
          >·</span>
          <span v-if="formattedDate">
            {{ $t('common.createdAt') }} {{ formattedDate }}
          </span>
        </div>
      </div>
    </CardHeader>
  </Card>
</template>

<script setup lang="ts">
import {
  Card,
  CardHeader,
  CardTitle,
  Avatar,
  AvatarImage,
  AvatarFallback,
  Badge,
} from '@memoh/ui'
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { BotsBot } from '@memoh/sdk'
import { formatDate } from '@/utils/date-time'
import { useAvatarInitials } from '@/composables/useAvatarInitials'
import { useBotStatusMeta } from '@/composables/useBotStatusMeta'

const router = useRouter()
const { t } = useI18n()

const props = defineProps<{
  bot: BotsBot
}>()

const botRef = computed(() => props.bot)

const avatarFallback = useAvatarInitials(() => props.bot.display_name || props.bot.id)

const formattedDate = computed(() => {
  if (!props.bot.created_at) return ''
  return formatDate(props.bot.created_at)
})

const { hasIssue, isPending, issueTitle, statusLabel, statusVariant } = useBotStatusMeta(botRef, t)

const botTypeLabel = computed(() => {
  const type = props.bot.type
  if (type === 'personal' || type === 'public') return t('bots.types.' + type)
  return type ?? ''
})

function onOpenDetail() {
  if (isPending.value) return
  router.push({ name: 'bot-detail', params: { botId: props.bot.id } })
}
</script>
