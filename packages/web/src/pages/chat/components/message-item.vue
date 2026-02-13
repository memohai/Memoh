<template>
  <div
    class="flex gap-3 items-start"
    :class="message.role === 'user' && isSelf ? 'justify-end' : ''"
  >
    <!-- Assistant avatar -->
    <div
      v-if="message.role === 'assistant'"
      class="relative shrink-0"
    >
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

    <!-- User avatar (other sender, left-aligned) -->
    <div
      v-if="message.role === 'user' && !isSelf"
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
    <div
      class="min-w-0"
      :class="contentClass"
    >
      <!-- Sender name for non-self user messages -->
      <p
        v-if="message.role === 'user' && !isSelf"
        class="text-xs text-muted-foreground mb-1"
      >
        {{ message.senderDisplayName || senderFallbackName }}
      </p>

      <!-- User message -->
      <div
        v-if="message.role === 'user'"
        class="rounded-2xl px-4 py-2.5 text-sm whitespace-pre-wrap"
        :class="isSelf
          ? 'rounded-tr-sm bg-primary text-primary-foreground'
          : 'rounded-tl-sm bg-accent/60 text-foreground'"
      >
        {{ (message.blocks[0] as TextBlock)?.content }}
      </div>

      <!-- Assistant message blocks -->
      <div
        v-else
        class="space-y-3"
      >
        <!-- Bot name label -->
        <p
          v-if="botName"
          class="text-xs text-muted-foreground"
        >
          {{ botName }}
        </p>

        <template
          v-for="(block, i) in message.blocks"
          :key="i"
        >
          <!-- Thinking block -->
          <ThinkingBlock
            v-if="block.type === 'thinking'"
            :block="(block as ThinkingBlockType)"
            :streaming="message.streaming && !block.done"
          />

          <!-- Tool call block -->
          <ToolCallBlock
            v-else-if="block.type === 'tool_call'"
            :block="(block as ToolCallBlockType)"
          />

          <!-- Text block -->
          <div
            v-else-if="block.type === 'text' && block.content"
            class="prose prose-sm dark:prose-invert max-w-none *:first:mt-0"
          >
            <MarkdownRender
              :content="block.content"
              custom-id="chat-msg"
            />
          </div>
        </template>

        <!-- Streaming indicator -->
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
      </div>
    </div>

    <!-- Self user avatar (right side) -->
    <div
      v-if="message.role === 'user' && isSelf"
      class="relative shrink-0"
    >
      <Avatar class="size-8">
        <AvatarImage
          v-if="selfAvatarUrl"
          :src="selfAvatarUrl"
          alt=""
        />
        <AvatarFallback class="text-xs">
          {{ selfFallback }}
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
import MarkdownRender, { enableKatex, enableMermaid } from 'markstream-vue'
import ThinkingBlock from './thinking-block.vue'
import ToolCallBlock from './tool-call-block.vue'
import ChannelBadge from '@/components/chat-list/channel-badge/index.vue'
import { useUserStore } from '@/store/user'
import { useChatStore } from '@/store/chat-list'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import type {
  ChatMessage,
  TextBlock,
  ThinkingBlock as ThinkingBlockType,
  ToolCallBlock as ToolCallBlockType,
} from '@/store/chat-list'

enableKatex()
enableMermaid()

const props = defineProps<{
  message: ChatMessage
}>()

const userStore = useUserStore()
const chatStore = useChatStore()
const { currentBotId, bots } = storeToRefs(chatStore)

const isSelf = computed(() => props.message.isSelf !== false)

const currentBot = computed(() =>
  bots.value.find((b) => b.id === currentBotId.value) ?? null,
)

const botAvatarUrl = computed(() => currentBot.value?.avatar_url ?? '')
const botName = computed(() => currentBot.value?.display_name ?? '')

// For isSelf messages: prefer channel avatar/name over web platform avatar
const selfAvatarUrl = computed(() =>
  props.message.senderAvatarUrl || userStore.userInfo.avatarUrl || '',
)
const selfFallback = computed(() => {
  const name = props.message.senderDisplayName
    || userStore.userInfo.displayName
    || userStore.userInfo.username
    || ''
  return name.slice(0, 2).toUpperCase() || 'U'
})

const { t } = useI18n()

const senderFallbackName = computed(() => {
  const p = (props.message.platform ?? '').trim()
  const platformLabel = p
    ? t(`bots.channels.types.${p}`, p.charAt(0).toUpperCase() + p.slice(1))
    : ''
  return t('chat.unknownUser', { platform: platformLabel })
})

const senderFallback = computed(() => {
  const name = props.message.senderDisplayName ?? ''
  return name.slice(0, 2).toUpperCase() || '?'
})

const contentClass = computed(() => {
  if (props.message.role === 'user') {
    return isSelf.value ? 'max-w-[80%]' : 'max-w-[80%]'
  }
  return 'flex-1 max-w-full'
})
</script>
