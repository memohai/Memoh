<template>
  <div
    v-if="shouldRenderMessage"
    ref="messageItem"
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

      <!-- Background task status -->
      <div
        v-if="message.role === 'system' && message.kind === 'background_task'"
        class="space-y-1"
      >
        <BackgroundTaskBlock :task="message.backgroundTask" />
        <p
          class="text-xs text-muted-foreground/80 mt-1"
          :title="fullTimestamp"
        >
          {{ relativeTimestamp }}
        </p>
      </div>

      <!-- Heartbeat trigger (replaces user message) -->
      <div
        v-else-if="message.role === 'user' && sessionType === 'heartbeat'"
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
          class="w-full rounded-lg border border-event-subagent-border bg-event-subagent-soft px-4 py-3"
        >
          <div class="prose prose-sm dark:prose-invert max-w-none *:first:mt-0">
            <MarkdownRender
              :content="message.text"
              :is-dark="isDark"
              :smooth-streaming="message.streaming"
              :typewriter="message.streaming"
              :fade="message.streaming"
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
          v-if="cleanUserText(message.text) || message.forward || message.reply"
          class="rounded-2xl px-3 py-2 text-xs whitespace-pre-wrap break-all"
          :class="isSelf
            ? 'rounded-tr-sm bg-accent text-foreground'
            : 'rounded-tl-sm bg-muted text-foreground'"
        >
          <div
            v-if="message.forward"
            class="mb-1 text-[11px] font-medium leading-snug text-muted-foreground"
          >
            {{ t('chat.forwardedFrom', { sender: forwardSenderLabel }) }}
          </div>
          <button
            v-if="message.reply"
            type="button"
            class="relative mb-1 min-w-0 overflow-hidden rounded-sm py-1 pl-3 pr-2 leading-snug break-normal"
            :class="[
              'bg-background/55 dark:bg-background/20',
              canJumpReply ? 'block w-full text-left cursor-pointer hover:bg-background/70 dark:hover:bg-background/30 focus:outline-none focus:ring-1 focus:ring-primary/40' : 'block w-full text-left cursor-default',
            ]"
            :disabled="!canJumpReply"
            @click.stop="handleReplyClick"
          >
            <span
              class="absolute inset-y-0 left-0 w-[3px]"
              :class="isSelf ? 'bg-border' : 'bg-primary/70'"
            />
            <div class="flex min-w-0 items-start gap-2">
              <div class="min-w-0 flex-1">
                <div
                  class="truncate text-[11px] font-semibold"
                  :class="isSelf ? 'text-foreground' : 'text-primary'"
                >
                  {{ replySenderLabel }}
                </div>
                <div
                  v-if="replyPreviewLabel"
                  class="mt-0.5 line-clamp-2 text-[11px] whitespace-pre-wrap break-words text-muted-foreground"
                >
                  {{ replyPreviewLabel }}
                </div>
              </div>
              <img
                v-if="replyThumbnailSrc"
                :src="replyThumbnailSrc"
                :alt="replyPreviewLabel || replySenderLabel"
                class="size-9 shrink-0 rounded-sm object-cover"
                loading="lazy"
              >
            </div>
          </button>
          <div v-if="cleanUserText(message.text)">
            {{ cleanUserText(message.text) }}
          </div>
        </div>
        <AttachmentBlock
          v-if="userAttachmentBlock"
          :block="userAttachmentBlock"
          :on-open-media="onOpenMedia"
        />
        <p
          class="text-xs text-muted-foreground/80 mt-1 text-right"
          :title="fullTimestamp"
        >
          {{ relativeTimestamp }}
        </p>
      </div>

      <!-- Assistant message blocks -->
      <div
        v-else
        class="space-y-1.5"
      >
        <!-- Bot name label -->
        <!-- <p
          v-if="botName"
          class="text-xs text-muted-foreground"
        >
          {{ botName }}
        </p> -->

        <template
          v-for="node in renderNodes"
          :key="node.key"
        >
          <!-- Process segment: consecutive tool + reasoning blocks. A single
               item renders as a bare row; multiple collapse into one group. -->
          <ToolCallGroup
            v-if="node.kind === 'process'"
            :items="node.items"
            :active="message.streaming && node.lastIndex === message.messages.length - 1"
          />

          <template v-else>
            <!-- Text block -->
            <div
              v-if="node.block.type === 'text' && node.block.content"
              class="prose prose-sm dark:prose-invert max-w-none [&_p]:my-0! [&_p+p]:mt-2! [&_ul]:my-1.5! [&_ol]:my-1.5! [&_li]:my-0.5! [&_:is(h1,h2,h3,h4,h5,h6)]:mt-2.5! [&_:is(h1,h2,h3,h4,h5,h6)]:mb-1! [&>*:first-child]:mt-0! [&>*:last-child]:mb-0!"
            >
              <MarkdownRender
                :content="node.block.content"
                :is-dark="isDark"
                :smooth-streaming="isAssistantBlockStreaming(node.index)"
                :typewriter="isAssistantBlockStreaming(node.index)"
                :fade="isAssistantBlockStreaming(node.index)"
                custom-id="chat-msg"
              />
            </div>

            <!-- Error block -->
            <div
              v-else-if="node.block.type === 'error' && node.block.content"
              class="flex items-start gap-2 rounded-md border border-destructive/25 bg-destructive/10 px-3 py-2 text-xs text-destructive"
            >
              <CircleAlert class="mt-0.5 size-3.5 shrink-0" />
              <span class="min-w-0 whitespace-pre-wrap break-words">{{ node.block.content }}</span>
            </div>

            <!-- Attachment block -->
            <AttachmentBlock
              v-else-if="node.block.type === 'attachments'"
              :block="(node.block as AttachmentBlockType)"
              :on-open-media="onOpenMedia"
            />
          </template>
        </template>

        <!-- Streaming indicator -->
        <div
          v-if="message.streaming && !hasVisibleAssistantBlocks"
          class="flex items-center gap-2 text-xs text-muted-foreground h-6"
        >
          <LoaderCircle class="size-3.5 animate-spin" />
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

<script lang="ts">
import { setCustomComponents } from 'markstream-vue'
import ChatCodeBlock from './chat-code-block.vue'

// Replace markstream's heavy Monaco code block (and its font-size/expand/preview
// toolbar that surfaced raw i18n keys) with a clean integral code block, scoped
// to the chat renderer id ("chat-msg"). Both `code_block` and `shell` are mapped
// to the same component so shell/bash blocks render identically (no separate
// terminal "run" toolbar / language chrome). Runs once at module load.
setCustomComponents('chat-msg', { code_block: ChatCodeBlock, shell: ChatCodeBlock })
</script>

<script setup lang="ts">
import { computed, toRef, useTemplateRef, watch } from 'vue'
import { CircleAlert, LoaderCircle } from 'lucide-vue-next'
import { formatRelativeTime, formatDateTime } from '@/utils/date-time'
import { Avatar, AvatarImage, AvatarFallback } from '@memohai/ui'
import MarkdownRender, { enableKatex, enableMermaid } from 'markstream-vue'
import { useSettingsStore } from '@/store/settings'
import ToolCallGroup from './tool-call-group.vue'
import { isReadOnlyTool } from './tool-call-registry'
import { finalizeReasoning, markReasoningSeen } from './reasoning-timing'
import AttachmentBlock from './attachment-block.vue'
import BackgroundTaskBlock from './background-task-block.vue'
import HeartbeatTriggerBlock from './heartbeat-trigger-block.vue'
import ScheduleTriggerBlock from './schedule-trigger-block.vue'
import ChannelBadge from '@/components/chat-list/channel-badge/index.vue'
// import { useUserStore } from '@/store/user'
// import { useChatStore } from '@/store/chat-list'
// import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import type {
  AttachmentItem,
  ChatMessage,
  ContentBlock,
  ToolCallBlock as ToolCallBlockType,
  ThinkingBlock as ThinkingBlockType,
  AttachmentBlock as AttachmentBlockType,
} from '@/store/chat-list'

import { resolveUrl } from '../composables/useMediaGallery'
import { useElementVisibility } from '@vueuse/core'


enableKatex()
enableMermaid()


const settingsStore = useSettingsStore()
const isDark = computed(() => settingsStore.theme === 'dark')

const messageEl = useTemplateRef('messageItem')
const emit = defineEmits<{
  active: [isActive: boolean, { id: string, top: number,  }]
}>()

const props = defineProps<{
  message: ChatMessage
  sessionType?: string
  botId?: string
  onOpenMedia?: (src: string) => void
  onReplyClick?: (messageId: string) => void
  isScrolling: boolean
}>()

const isVisible = useElementVisibility(messageEl, {
  threshold: 0.1
})

watch([isVisible, toRef(props, 'isScrolling')], () => { 
  emit('active', isVisible.value, { id: props.message.id, top: ((messageEl.value?.getBoundingClientRect().top ?? 0) - 48) })
}, {
  immediate: true,
  deep:true
})

const isSelf = computed(() =>
  props.message.role !== 'user' || props.message.isSelf !== false,
)


const { t, locale } = useI18n()


const senderFallback = computed(() => {
  const name = props.message.role === 'user' ? (props.message.senderDisplayName ?? '') : ''
  return name.slice(0, 2).toUpperCase() || '?'
})

const replySenderLabel = computed(() => {
  if (props.message.role !== 'user') return ''
  return props.message.reply?.sender || props.message.reply?.message_id || t('chat.unknownMessage')
})

const forwardSenderLabel = computed(() => {
  if (props.message.role !== 'user') return ''
  return props.message.forward?.sender
    || props.message.forward?.from_conversation_id
    || props.message.forward?.from_user_id
    || t('chat.unknownMessage')
})

const canJumpReply = computed(() =>
  props.message.role === 'user'
  && !!props.message.reply?.message_id?.trim()
  && typeof props.onReplyClick === 'function',
)

const replyThumbnail = computed<AttachmentItem | null>(() => {
  if (props.message.role !== 'user') return null
  return (props.message.reply?.attachments ?? []).find((att) => isImageAttachment(att) && resolveUrl(att)) ?? null
})

const replyThumbnailSrc = computed(() => replyThumbnail.value ? resolveUrl(replyThumbnail.value) : '')

const replyPreviewLabel = computed(() => {
  if (props.message.role !== 'user') return ''
  const preview = props.message.reply?.preview?.trim()
  if (preview) return preview
  return replyThumbnailSrc.value ? t('chat.replyPhoto') : ''
})

function isImageAttachment(att: AttachmentItem): boolean {
  const type = String(att.type ?? '').toLowerCase()
  if (type === 'image' || type === 'gif') return true
  const mime = String(att.mime ?? '').toLowerCase()
  return mime.startsWith('image/')
}

function handleReplyClick() {
  if (props.message.role !== 'user') return
  const messageId = props.message.reply?.message_id?.trim()
  if (!messageId || !props.onReplyClick) return
  props.onReplyClick(messageId)
}

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

function hasLaterAssistantMessage(index: number): boolean {
  return props.message.role === 'assistant' && props.message.messages.slice(index + 1).length > 0
}

function isAssistantBlockStreaming(index: number): boolean {
  return props.message.role === 'assistant' && props.message.streaming && !hasLaterAssistantMessage(index)
}

const hasVisibleAssistantBlocks = computed(() =>
  props.message.role === 'assistant'
  && props.message.messages.some(isVisibleAssistantBlock),
)

const shouldRenderMessage = computed(() =>
  props.message.role !== 'assistant' || hasVisibleAssistantBlocks.value || props.message.streaming,
)

function isVisibleAssistantBlock(block: ContentBlock): boolean {
  if (block.type === 'tool') return true
  if (block.type === 'text' || block.type === 'error') return Boolean(block.content)
  if (block.type === 'attachments') return block.attachments.length > 0
  return true
}

// Project the flat assistant block list into render nodes.
//  - A "process" node is a run of consecutive tool + reasoning blocks. It splits
//    by tool category (read-only "explore" vs side-effecting "action") so reads
//    and edits don't merge into one bucket; reasoning rides along with whichever
//    segment it sits next to (it is never rendered standalone).
//  - Every other block type (text / error / attachments) keeps its place.
// Keyed by stable block id.
type ProcessNode = { kind: 'process'; key: string; items: ContentBlock[]; cat: 'explore' | 'action' | null; lastIndex: number }
type BlockNode = { kind: 'block'; key: string; block: ContentBlock; index: number }
type RenderNode = ProcessNode | BlockNode

const renderNodes = computed<RenderNode[]>(() => {
  if (props.message.role !== 'assistant') return []
  const nodes: RenderNode[] = []
  let run: ProcessNode | null = null
  props.message.messages.forEach((block, index) => {
    if (!isVisibleAssistantBlock(block)) return
    if (block.type === 'tool' || block.type === 'reasoning') {
      const cat = block.type === 'tool'
        ? (isReadOnlyTool((block as ToolCallBlockType).toolName) ? 'explore' : 'action')
        : null
      if (!run) {
        run = { kind: 'process', key: `p${block.id}`, items: [block], cat, lastIndex: index }
        nodes.push(run)
      } else if (cat !== null && run.cat !== null && cat !== run.cat) {
        // Category switch (e.g. finished reading, now editing) → new segment.
        run = { kind: 'process', key: `p${block.id}`, items: [block], cat, lastIndex: index }
        nodes.push(run)
      } else {
        run.items.push(block)
        run.lastIndex = index
        if (run.cat === null && cat !== null) run.cat = cat
      }
    } else {
      run = null
      nodes.push({ kind: 'block', key: `b${block.type}-${block.id}`, block, index })
    }
  })
  return nodes
})

// Centralized reasoning timing. The stream carries no duration, so we measure
// it client-side: stamp a reasoning block the first time it appears mid-stream,
// and finalize it once a later block supersedes it (or the turn ends). This
// covers every reasoning step — including ones immediately followed by a tool
// call — so they show a real "Thought for Ns" instead of a bare "Thought".
watch(
  () => (props.message.role === 'assistant' && props.message.streaming
    ? props.message.messages.map(block => `${block.type}:${block.id}`).join('|')
    : ''),
  () => {
    if (props.message.role !== 'assistant' || !props.message.streaming) return
    const blocks = props.message.messages
    blocks.forEach((block, index) => {
      if (block.type !== 'reasoning') return
      const content = (block as ThinkingBlockType).content ?? ''
      markReasoningSeen(content)
      if (index < blocks.length - 1) finalizeReasoning(content)
    })
  },
  { immediate: true },
)

watch(
  () => props.message.role === 'assistant' && props.message.streaming,
  (streaming, was) => {
    if (!was || streaming || props.message.role !== 'assistant') return
    props.message.messages.forEach((block) => {
      if (block.type === 'reasoning') finalizeReasoning((block as ThinkingBlockType).content ?? '')
    })
  },
)

const relativeTimestamp = computed(() =>
  formatRelativeTime(props.message.timestamp, { locale: locale.value }),
)
const fullTimestamp = computed(() =>
  formatDateTime(props.message.timestamp, { locale: locale.value }),
)
</script>
