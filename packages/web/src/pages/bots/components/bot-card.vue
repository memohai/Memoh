<template>
  <Card
    class="group relative transition-shadow hover:shadow-md cursor-pointer"
    @click="router.push({ name: 'bot-detail', params: { botId: bot.id } })"
  >
    <CardHeader class="flex flex-row items-start gap-4 space-y-0">
      <Avatar class="size-12 shrink-0">
        <AvatarImage
          v-if="bot.avatar_url"
          :src="bot.avatar_url"
          :alt="bot.display_name"
        />
        <AvatarFallback class="text-lg">
          {{ avatarFallback }}
        </AvatarFallback>
      </Avatar>
      <div class="flex-1 min-w-0">
        <CardTitle class="text-base truncate">
          {{ bot.display_name || bot.id }}
        </CardTitle>
        <CardDescription class="mt-1 flex items-center gap-2">
          <Badge
            :variant="bot.is_active ? 'default' : 'secondary'"
            class="text-xs"
          >
            {{ bot.is_active ? $t('bots.active') : $t('bots.inactive') }}
          </Badge>
          <span
            v-if="bot.type"
            class="text-xs text-muted-foreground"
          >
            {{ bot.type }}
          </span>
        </CardDescription>
      </div>
    </CardHeader>
    <CardFooter class="pt-0 flex items-center justify-between text-xs text-muted-foreground">
      <span>{{ $t('bots.createdAt') }} {{ formattedDate }}</span>
      <div class="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
        <Button
          variant="ghost"
          size="sm"
          @click.stop="$emit('edit', bot)"
        >
          <FontAwesomeIcon :icon="['fas', 'pen-to-square']" />
        </Button>
        <ConfirmPopover
          :message="$t('bots.deleteConfirm')"
          :loading="deleteLoading"
          @confirm="$emit('delete', bot.id)"
        >
          <template #trigger>
            <Button
              variant="ghost"
              size="sm"
              @click.stop
            >
              <FontAwesomeIcon :icon="['far', 'trash-can']" />
            </Button>
          </template>
        </ConfirmPopover>
      </div>
    </CardFooter>
  </Card>
</template>

<script setup lang="ts">
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardFooter,
  Avatar,
  AvatarImage,
  AvatarFallback,
  Badge,
  Button,
} from '@memoh/ui'
import ConfirmPopover from '@/components/confirm-popover/index.vue'
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import type { BotInfo } from '@/composables/api/useBots'

const router = useRouter()

const props = defineProps<{
  bot: BotInfo
  deleteLoading: boolean
}>()

defineEmits<{
  edit: [bot: BotInfo]
  delete: [id: string]
}>()

const avatarFallback = computed(() => {
  const name = props.bot.display_name || props.bot.id
  return name.slice(0, 2).toUpperCase()
})

const formattedDate = computed(() => {
  if (!props.bot.created_at) return ''
  return new Date(props.bot.created_at).toLocaleDateString()
})
</script>
