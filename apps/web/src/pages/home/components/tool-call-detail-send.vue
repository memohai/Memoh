<template>
  <div class="space-y-1.5">
    <div
      v-if="text"
      class="text-xs text-foreground/90 whitespace-pre-wrap break-all max-h-96 overflow-y-auto rounded-sm bg-muted/30 px-2 py-1.5"
    >
      {{ text }}
    </div>

    <div
      v-if="attachments.length"
      class="space-y-0.5"
    >
      <div class="text-caption uppercase tracking-wide text-muted-foreground/70 mb-0.5">
        {{ t('chat.tools.detail.attachments') }}
      </div>
      <ul class="space-y-0.5">
        <li
          v-for="(att, i) in attachments"
          :key="i"
          class="flex items-center gap-1.5 text-xs text-muted-foreground"
        >
          <Paperclip class="size-3 shrink-0" />
          <span
            class="font-mono truncate"
            :title="att"
          >{{ att }}</span>
        </li>
      </ul>
    </div>

    <div
      v-if="replyTo"
      class="flex items-center gap-1.5 text-xs text-muted-foreground"
    >
      <CornerDownRight class="size-3 shrink-0" />
      <span class="text-caption uppercase tracking-wide text-muted-foreground/70">
        {{ t('chat.tools.detail.replyTo') }}
      </span>
      <span
        class="font-mono truncate"
        :title="replyTo"
      >{{ replyTo }}</span>
    </div>

    <p
      v-if="!text && !attachments.length && !replyTo"
      class="text-xs text-muted-foreground italic"
    >
      {{ t('chat.tools.detail.noContent') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CornerDownRight, Paperclip } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

function asObject(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : {}
}

function pickString(obj: Record<string, unknown>, ...keys: string[]): string {
  for (const k of keys) {
    const v = obj[k]
    if (typeof v === 'string' && v.length > 0) return v
  }
  return ''
}

function extractPartsText(parts: unknown): string {
  if (!Array.isArray(parts)) return ''
  return parts
    .map((part) => {
      const obj = asObject(part)
      const type = String(obj.type ?? '').toLowerCase()
      if (type === 'text' && typeof obj.text === 'string') return obj.text
      if (typeof obj.text === 'string') return obj.text
      return ''
    })
    .filter(Boolean)
    .join('\n')
}

const text = computed(() => {
  const input = asObject(props.block.input)
  const direct = pickString(input, 'text')
  if (direct) return direct
  const message = asObject(input.message)
  if (Object.keys(message).length === 0) return ''
  const messageText = pickString(message, 'text')
  if (messageText) return messageText
  return extractPartsText(message.parts)
})

const attachments = computed<string[]>(() => {
  const input = asObject(props.block.input)
  const list = Array.isArray(input.attachments) ? input.attachments : []
  return list
    .map((item) => {
      if (typeof item === 'string') return item
      const obj = asObject(item)
      return pickString(obj, 'path', 'url', 'name')
    })
    .filter((s): s is string => Boolean(s))
})

const replyTo = computed(() => {
  const input = asObject(props.block.input)
  return pickString(input, 'reply_to')
})
</script>
