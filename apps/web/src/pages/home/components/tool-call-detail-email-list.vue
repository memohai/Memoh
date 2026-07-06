<template>
  <div class="space-y-1.5">
    <div
      v-if="emails.length"
      class="space-y-1.5"
    >
      <div
        v-for="(item, i) in emails"
        :key="item.uid ?? i"
        class="flex flex-col gap-0.5"
      >
        <div class="flex items-center gap-2">
          <span class="text-xs font-medium text-foreground truncate flex-1">{{ item.subject || t('chat.tools.detail.noSubject') }}</span>
          <span
            v-if="item.received_at"
            class="text-caption text-muted-foreground shrink-0"
          >{{ item.received_at }}</span>
        </div>
        <span
          v-if="item.from"
          class="text-caption text-muted-foreground truncate"
        >{{ item.from }}</span>
      </div>
    </div>
    <EmptyRow v-else>
      {{ t('chat.tools.detail.noEmails') }}
    </EmptyRow>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'
import EmptyRow from './tool-detail/empty-row.vue'

interface EmailItem {
  uid?: number
  from?: string
  subject?: string
  received_at?: string
}

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

function resolveResult(): Record<string, unknown> | null {
  if (!props.block.result) return null
  const result = props.block.result as Record<string, unknown>
  return (result.structuredContent as Record<string, unknown>) ?? result
}

const emails = computed<EmailItem[]>(() => {
  if (!props.block.done) return []
  const r = resolveResult()
  if (!r) return []
  const items = r.emails as EmailItem[] | undefined
  return Array.isArray(items) ? items : []
})
</script>
