<template>
  <div
    class="flex gap-3 items-start"
    :class="message.role === 'user' && isSelf && !isSpecialUserMessage ? 'justify-end' : ''"
  >
    <!-- Assistant avatar
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
    </div> -->

    <!-- User avatar (other sender, left-aligned; hidden for special session types) -->
    <div
      v-if="message.role === 'user' && !isSelf && !isSpecialUserMessage"
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
      data-chat-content
    >
      <!-- Sender name for non-self user messages
      <p
        v-if="message.role === 'user' && !isSelf"
        class="text-xs text-muted-foreground mb-1"
      >
        {{ message.senderDisplayName || senderFallbackName }}
      </p> -->

      <!-- Heartbeat trigger (replaces user message) -->
      <div
        v-if="message.role === 'user' && sessionType === 'heartbeat'"
        class="space-y-2"
      >
        <HeartbeatTriggerBlock
          v-if="message.text"
          :content="message.text"
          :bot-id="botId"
        />
        <AttachmentBlock
          v-if="userAttachmentBlock"
          :block="userAttachmentBlock"
          :on-open-media="onOpenMedia"
        />
        <p
          class="text-xs text-muted-foreground/80 mt-1"
          :title="fullTimestamp"
        >
          {{ relativeTimestamp }}
        </p>
      </div>

      <!-- Schedule trigger (replaces user message) -->
      <div
        v-else-if="message.role === 'user' && sessionType === 'schedule'"
        class="space-y-2"
      >
        <ScheduleTriggerBlock
          v-if="message.text"
          :content="message.text"
          :bot-id="botId"
        />
        <AttachmentBlock
          v-if="userAttachmentBlock"
          :block="userAttachmentBlock"
          :on-open-media="onOpenMedia"
        />
        <p
          class="text-xs text-muted-foreground/80 mt-1"
          :title="fullTimestamp"
        >
          {{ relativeTimestamp }}
        </p>
      </div>

      <!-- Subagent user message (full-width markdown box) -->
      <div
        v-else-if="message.role === 'user' && sessionType === 'subagent'"
        class="space-y-2"
      >
        <div
          v-if="message.text"
          class="w-full rounded-lg border border-violet-200 dark:border-violet-400/20 bg-violet-50/50 dark:bg-violet-950/20 px-4 py-3"
        >
          <div class="prose prose-sm dark:prose-invert max-w-none *:first:mt-0">
            <MarkdownRender
              :content="message.text"
              :is-dark="isDark"
              :typewriter="message.streaming"
              custom-id="chat-msg"
            />
          </div>
        </div>
        <AttachmentBlock
          v-if="userAttachmentBlock"
          :block="userAttachmentBlock"
          :on-open-media="onOpenMedia"
        />
        <p
          class="text-xs text-muted-foreground/80 mt-1"
          :title="fullTimestamp"
        >
          {{ relativeTimestamp }}
        </p>
      </div>

      <!-- Default user message (chat bubble) -->
      <div
        v-else-if="message.role === 'user'"
        class="space-y-2"
      >
        <div
          v-if="cleanUserText(message.text)"
          class="rounded-2xl px-3 py-2 text-xs whitespace-pre-wrap break-all"
          :class="isSelf
            ? 'rounded-tr-sm bg-foreground text-background'
            : 'rounded-tl-sm bg-accent text-foreground'"
        >
          {{ cleanUserText(message.text) }}
        </div>
        <AttachmentBlock
          v-if="userAttachmentBlock"
          :block="userAttachmentBlock"
          :on-open-media="onOpenMedia"
        />
        <p
          class="text-xs text-muted-foreground/80 mt-1"
          :title="fullTimestamp"
        >
          {{ relativeTimestamp }}
        </p>
      </div>

      <!-- Assistant message blocks -->
      <div
        v-else
        class="space-y-3"
      >
        <!-- Bot name label -->
        <!-- <p
          v-if="botName"
          class="text-xs text-muted-foreground"
        >
          {{ botName }}
        </p> -->

        <template
          v-for="(block, i) in message.messages"
          :key="i"
        >
          <!-- Thinking block -->
          <ThinkingBlock
            v-if="block.type === 'reasoning'"
            :block="(block as ThinkingBlockType)"
            :streaming="isAssistantBlockStreaming(i)"
          />

          <!-- Tool call block -->
          <ToolCallBlock
            v-else-if="block.type === 'tool' && !isHiddenToolMessage(block)"
            :block="(block as ToolCallBlockType)"
          />

          <!-- Text block -->
          <div
            v-else-if="block.type === 'text' && block.content"
            class="prose prose-sm dark:prose-invert max-w-none *:first:mt-0"
          >
            <MarkdownRender
              :content="block.content"
              :is-dark="isDark"
              :typewriter="isAssistantBlockStreaming(i)"
              custom-id="chat-msg"
            />
          </div>

          <!-- Attachment block -->
          <AttachmentBlock
            v-else-if="block.type === 'attachments'"
            :block="(block as AttachmentBlockType)"
            :on-open-media="onOpenMedia"
          />
        </template>

        <!-- Streaming indicator -->
        <div
          v-if="message.streaming && !hasVisibleAssistantBlocks"
          class="flex items-center gap-2 text-xs text-muted-foreground h-6"
        >
          <LoaderCircle
            class="size-3.5 animate-spin"
          />
          {{ $t('chat.thinking') }}
        </div>
        <p
          class="text-xs text-muted-foreground/80 mt-1"
          :title="fullTimestamp"
        >
          {{ relativeTimestamp }}
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { LoaderCircle } from 'lucide-vue-next'
import { formatRelativeTime, formatDateTime } from '@/utils/date-time'
import { Avatar, AvatarImage, AvatarFallback } from '@memohai/ui'
import MarkdownRender, { enableKatex, enableMermaid } from 'markstream-vue'
import { useSettingsStore } from '@/store/settings'
import ThinkingBlock from './thinking-block.vue'
import ToolCallBlock from './tool-call-block.vue'
import AttachmentBlock from './attachment-block.vue'
import HeartbeatTriggerBlock from './heartbeat-trigger-block.vue'
import ScheduleTriggerBlock from './schedule-trigger-block.vue'
import ChannelBadge from '@/components/chat-list/channel-badge/index.vue'
// import { useUserStore } from '@/store/user'
// import { useChatStore } from '@/store/chat-list'
// import { storeToRefs } from 'pinia'
// import { useI18n } from 'vue-i18n'
import type {
  ChatMessage,
  ContentBlock,
  ThinkingBlock as ThinkingBlockType,
  ToolCallBlock as ToolCallBlockType,
  AttachmentBlock as AttachmentBlockType,
} from '@/store/chat-list'

enableKatex()
enableMermaid()

const settingsStore = useSettingsStore()
const isDark = computed(() => settingsStore.theme === 'dark')

const props = defineProps<{
  message: ChatMessage
  sessionType?: string
  botId?: string
  onOpenMedia?: (src: string) => void
}>()

// const chatStore = useChatStore()
// const { currentBotId, bots } = storeToRefs(chatStore)

const isSelf = computed(() =>
  props.message.role !== 'user' || props.message.isSelf !== false,
)

// const currentBot = computed(() =>
//   bots.value.find((b) => b.id === currentBotId.value) ?? null,
// )

// const botAvatarUrl = computed(() => currentBot.value?.avatar_url ?? '')
// const botName = computed(() => currentBot.value?.display_name ?? '')

// const { t } = useI18n()

// const senderFallbackName = computed(() => {
//   const p = (props.message.platform ?? '').trim()
//   const platformLabel = p
//     ? t(`bots.channels.types.${p}`, p.charAt(0).toUpperCase() + p.slice(1))
//     : ''
//   return t('chat.unknownUser', { platform: platformLabel })
// })

const senderFallback = computed(() => {
  const name = props.message.role === 'user' ? (props.message.senderDisplayName ?? '') : ''
  return name.slice(0, 2).toUpperCase() || '?'
})

function cleanUserText(content?: string): string {
  if (!content) return ''
  return content
    .split('\n')
    .filter((line) => !/^\[attachment:\w+\]\s/.test(line.trim()))
    .join('\n')
    .trim()
}

const isSpecialUserMessage = computed(() =>
  props.message.role === 'user'
  && (props.sessionType === 'heartbeat' || props.sessionType === 'schedule' || props.sessionType === 'subagent'),
)

const contentClass = computed(() => {
  if (isSpecialUserMessage.value) return 'flex-1 max-w-full'
  if (props.message.role === 'user') return 'max-w-[80%]'
  return 'flex-1 max-w-full'
})

const userAttachmentBlock = computed<AttachmentBlockType | null>(() => {
  if (props.message.role !== 'user' || props.message.attachments.length === 0) return null
  return {
    id: -1,
    type: 'attachments',
    attachments: props.message.attachments,
  }
})

function isHiddenToolMessage(block: ContentBlock): boolean {
  if (block.type !== 'tool') return false
  const result = block.result as Record<string, unknown> | null
  return !!result && typeof result === 'object' && result.delivered === 'current_conversation'
}

function hasLaterAssistantMessage(index: number): boolean {
  return props.message.role === 'assistant' && props.message.messages.slice(index + 1).length > 0
}

function isAssistantBlockStreaming(index: number): boolean {
  return props.message.role === 'assistant' && props.message.streaming && !hasLaterAssistantMessage(index)
}

const hasVisibleAssistantBlocks = computed(() =>
  props.message.role === 'assistant'
    && props.message.messages.some(block => block.type !== 'tool' || !isHiddenToolMessage(block)),
)

const relativeTimestamp = computed(() =>
  formatRelativeTime(props.message.timestamp),
)
const fullTimestamp = computed(() =>
  formatDateTime(props.message.timestamp),
)
</script>
