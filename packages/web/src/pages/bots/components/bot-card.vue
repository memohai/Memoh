<template>
  <Card
    class="group relative transition-shadow"
    :class="isPending ? 'opacity-80 cursor-not-allowed' : 'hover:shadow-md cursor-pointer'"
    @click="onOpenDetail"
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

const router = useRouter()
const { t } = useI18n()

const props = defineProps<{
  bot: BotsBot
}>()

const avatarFallback = computed(() => {
  const name = props.bot.display_name || props.bot.id
  return name.slice(0, 2).toUpperCase()
})

const formattedDate = computed(() => {
  if (!props.bot.created_at) return ''
  return new Date(props.bot.created_at).toLocaleDateString()
})

const isCreating = computed(() => props.bot.status === 'creating')
const isDeleting = computed(() => props.bot.status === 'deleting')
const isPending = computed(() => isCreating.value || isDeleting.value)
const hasIssue = computed(() => props.bot.check_state === 'issue')
const issueTitle = computed(() => {
  const count = Number(props.bot.check_issue_count ?? 0)
  if (count <= 0) return t('bots.checks.hasIssue')
  return t('bots.checks.issueCount', { count })
})

const statusVariant = computed<'default' | 'secondary' | 'destructive'>(() => {
  if (isPending.value) return 'secondary'
  if (hasIssue.value) return 'destructive'
  return props.bot.is_active ? 'default' : 'secondary'
})

const statusLabel = computed(() => {
  if (isCreating.value) return t('bots.lifecycle.creating')
  if (isDeleting.value) return t('bots.lifecycle.deleting')
  if (hasIssue.value) return issueTitle.value
  return props.bot.is_active ? t('bots.active') : t('bots.inactive')
})

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
