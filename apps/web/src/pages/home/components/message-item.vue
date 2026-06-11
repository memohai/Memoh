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
          v-for="segment in renderSegments"
          :key="segment.key"
        >
          <!-- Active rail (streaming): prior steps as a chip + the current
               phase live — one rolling command, or the thinking peek. -->
          <ProcessRail v-if="segment.kind === 'rail-active'">
            <RailSummary
              v-if="segment.prior.length"
              :blocks="segment.prior"
            />
            <ThinkingBlock
              v-if="segment.current && segment.current.kind === 'think'"
              :block="(segment.current.blocks[0] as ThinkingBlockType)"
              :streaming="true"
            />
            <RollingToolSlot
              v-else-if="segment.current && segment.current.kind === 'tools'"
              :tools="(segment.current.blocks as ToolCallBlockType[])"
            />
          </ProcessRail>

          <!-- Process rail: recessed lane for thinking + tool calls. Settled
               segments collapse to a single summary line; otherwise the rows
               (with consecutive done tools clustered) render in the lane. -->
          <ProcessRail v-else-if="segment.kind === 'summary' || segment.kind === 'rail'">
            <RailSummary
              v-if="segment.kind === 'summary'"
              :blocks="segment.blocks"
            />
            <RailItems
              v-else
              :items="segment.items"
              :streaming-block-id="streamingBlockId"
            />
          </ProcessRail>

          <!-- Flow segment: the answer / error / attachments break out full-width -->
          <template v-else>
            <!-- Text block -->
            <div
              v-if="segment.block.type === 'text' && segment.block.content"
              class="prose prose-sm dark:prose-invert max-w-none *:first:mt-0"
            >
              <MarkdownRender
                :content="segment.block.content"
                :is-dark="isDark"
                :smooth-streaming="isBlockStreaming(segment.block)"
                :typewriter="isBlockStreaming(segment.block)"
                :fade="isBlockStreaming(segment.block)"
                custom-id="chat-msg"
              />
            </div>

            <!-- Error block -->
            <div
              v-else-if="segment.block.type === 'error' && segment.block.content"
              class="flex items-start gap-2 rounded-md border border-destructive/25 bg-destructive/10 px-3 py-2 text-xs text-destructive"
            >
              <CircleAlert class="mt-0.5 size-3.5 shrink-0" />
              <span class="min-w-0 whitespace-pre-wrap break-words">{{ segment.block.content }}</span>
            </div>

            <!-- Attachment block -->
            <AttachmentBlock
              v-else-if="segment.block.type === 'attachments'"
              :block="(segment.block as AttachmentBlockType)"
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

<script setup lang="ts">
import { computed, onBeforeUnmount, useTemplateRef, watch } from 'vue'
import { CircleAlert, LoaderCircle } from 'lucide-vue-next'
import { formatRelativeTime, formatDateTime } from '@/utils/date-time'
import { Avatar, AvatarImage, AvatarFallback } from '@memohai/ui'
import MarkdownRender, { enableKatex, enableMermaid } from 'markstream-vue'
import { useSettingsStore } from '@/store/settings'
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
  ThinkingBlock as ThinkingBlockType,
  ToolCallBlock as ToolCallBlockType,
  AttachmentBlock as AttachmentBlockType,
} from '@/store/chat-list'
import type { RailGroup, RailItem, TurnSegment } from '@/store/chat-list.utils'
import { canSummarizeRailSegment, clusterRailBlocks, latestOutputLine, segmentHasLiveBg, segmentTurnBlocks, splitActiveRail } from '@/store/chat-list.utils'
import { useBgTaskBeacon } from '../composables/useBgTaskBeacons'
import ProcessRail from './process-rail.vue'
import RailItems from './rail-items.vue'
import RailSummary from './rail-summary.vue'
import RollingToolSlot from './rolling-tool-slot.vue'
import ThinkingBlock from './thinking-block.vue'

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
}>()

const isVisible = useElementVisibility(messageEl, {
  threshold: 0.1
})

watch(isVisible, () => {
  emit('active', isVisible.value, { id: props.message.id, top: ((messageEl.value?.getBoundingClientRect().top ?? 0) - 48) })
}, {
  immediate: true,
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

const turnSegments = computed<TurnSegment<ContentBlock>[]>(() =>
  props.message.role === 'assistant' ? segmentTurnBlocks(props.message.messages) : [],
)

type RenderSegment =
  | { kind: 'rail-active'; key: string; prior: ContentBlock[]; current: RailGroup<ContentBlock> | null }
  | { kind: 'summary'; key: string; blocks: ContentBlock[] }
  | { kind: 'rail'; key: string; items: RailItem<ContentBlock>[] }
  | { kind: 'flow'; key: string; block: ContentBlock }

// The active (last) rail of a streaming turn rolls its current phase live: prior
// steps collapse to a summary chip, the current tool run shows one rolling
// command, current thinking shows its peek. Settled segments collapse to a
// summary; remaining settled tool runs cluster. Folding here only touches
// settled prior rows (a deliberate collapse motion) — the streaming answer
// markdown never reparents, so the answer itself never stalls.
const renderSegments = computed<RenderSegment[]>(() => {
  const streaming = props.message.role === 'assistant' && props.message.streaming
  const segments = turnSegments.value
  const lastIndex = segments.length - 1
  return segments.map((segment, index) => {
    if (segment.kind === 'flow') return segment
    if (streaming && index === lastIndex) {
      // A live background-task row must stay visible — never roll/collapse it
      // into the prior chip. Render the whole active rail as solo rows instead.
      if (segmentHasLiveBg(segment.blocks)) {
        return { kind: 'rail', key: segment.key, items: clusterRailBlocks(segment.blocks, true) }
      }
      const { prior, current } = splitActiveRail(segment.blocks)
      return { kind: 'rail-active', key: segment.key, prior, current }
    }
    if (canSummarizeRailSegment(segment.blocks)) {
      return { kind: 'summary', key: segment.key, blocks: segment.blocks }
    }
    return {
      kind: 'rail',
      key: segment.key,
      items: clusterRailBlocks(segment.blocks, false),
    }
  })
})

// Mirror this turn's background tasks into the pane-level beacon so a floating
// pill can surface a running task once the turn scrolls off screen — regardless
// of whether the task's row is expanded or folded into a cluster. Visibility is
// the turn's, which is robust to that collapse state (the row may not be in the
// DOM when clustered).
const beacon = useBgTaskBeacon()

const bgTasks = computed(() => {
  if (props.message.role !== 'assistant') return []
  const tasks: { taskId: string, phase: 'active' | 'done', latestLine: string }[] = []
  for (const block of props.message.messages) {
    if (block.type !== 'tool') continue
    const task = (block as ToolCallBlockType).backgroundTask
    if (!task) continue
    const status = (task.status || '').trim().toLowerCase()
    const active = status === 'running' || status === 'stalled'
    const done = status === 'completed' || status === 'failed' || status === 'killed'
    if (!active && !done) continue
    tasks.push({
      taskId: task.taskId,
      phase: active ? 'active' : 'done',
      latestLine: latestOutputLine(task.outputTail) || task.command || '',
    })
  }
  return tasks
})

const registeredTaskIds = new Set<string>()

watch(
  [bgTasks, isVisible],
  ([tasks, visible]) => {
    if (!beacon) return
    const current = new Set<string>()
    for (const task of tasks) {
      current.add(task.taskId)
      beacon.upsert({
        taskId: task.taskId,
        phase: task.phase,
        visible,
        latestLine: task.latestLine,
        scrollIntoView: () => messageEl.value?.scrollIntoView({ block: 'center', behavior: 'smooth' }),
      })
    }
    for (const id of registeredTaskIds) {
      if (!current.has(id)) beacon.remove(id)
    }
    registeredTaskIds.clear()
    for (const id of current) registeredTaskIds.add(id)
  },
  { immediate: true, deep: true },
)

onBeforeUnmount(() => {
  if (!beacon) return
  for (const id of registeredTaskIds) beacon.remove(id)
})

// Only the final block of a streaming turn is "live" — earlier blocks have
// settled. We compare by stable block id (never array index) so segmentation
// and streaming state agree without depending on position.
const streamingBlockId = computed<number | null>(() => {
  if (props.message.role !== 'assistant' || !props.message.streaming) return null
  const blocks = props.message.messages
  const last = blocks[blocks.length - 1]
  return last ? last.id : null
})

function isBlockStreaming(block: ContentBlock): boolean {
  return streamingBlockId.value !== null && block.id === streamingBlockId.value
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

const relativeTimestamp = computed(() =>
  formatRelativeTime(props.message.timestamp, { locale: locale.value }),
)
const fullTimestamp = computed(() =>
  formatDateTime(props.message.timestamp, { locale: locale.value }),
)
</script>
