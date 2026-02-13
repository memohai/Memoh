<template>
  <div class="flex gap-3 items-start">
    <!-- Bot avatar -->
    <div class="relative shrink-0">
      <Avatar class="size-8">
        <AvatarImage
          v-if="botAvatarUrl"
          :src="botAvatarUrl"
          :alt="botName"
        />
        <AvatarFallback class="text-xs bg-primary/10 text-primary">
          <FontAwesomeIcon
            :icon="['fas', 'robot']"
            class="size-4"
          />
        </AvatarFallback>
      </Avatar>
      <ChannelBadge
        v-if="message.platform"
        :platform="message.platform"
      />
    </div>

    <!-- Content -->
    <div class="flex-1 min-w-0 max-w-full space-y-2">
      <p
        v-if="botName"
        class="text-xs text-muted-foreground"
      >
        {{ botName }}
      </p>

      <!-- Streaming with no content yet -->
      <div
        v-if="message.streaming && message.blocks.length === 0"
        class="flex items-center gap-2 text-sm text-muted-foreground h-8"
      >
        <FontAwesomeIcon
          :icon="['fas', 'spinner']"
          class="size-3.5 animate-spin"
        />
        {{ $t('chat.thinking') }}
      </div>

      <!-- Blocks -->
      <template
        v-for="(block, i) in message.blocks"
        :key="i"
      >
        <div
          v-if="block.type === 'text' && block.content"
          class="prose prose-sm dark:prose-invert max-w-none *:first:mt-0"
        >
          <MarkdownRender
            :content="block.content"
            custom-id="chat-msg"
          />
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Avatar, AvatarImage, AvatarFallback } from '@memoh/ui'
import MarkdownRender from 'markstream-vue'
import ChannelBadge from '@/components/chat-list/channel-badge/index.vue'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import type { ChatMessage } from '@/store/chat-list'

defineProps<{
  message: ChatMessage
}>()

const chatStore = useChatStore()
const { currentBotId, bots } = storeToRefs(chatStore)

const currentBot = computed(() =>
  bots.value.find((b) => b.id === currentBotId.value) ?? null,
)

const botAvatarUrl = computed(() => currentBot.value?.avatar_url ?? '')
const botName = computed(() => currentBot.value?.display_name ?? '')
</script>
