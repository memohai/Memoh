<template>
  <div class="space-y-1.5">
    <div
      v-if="contacts.length"
      class="space-y-1"
    >
      <div
        v-for="(item, i) in contacts"
        :key="item.route_id ?? i"
        class="flex items-center gap-2 text-xs"
      >
        <span class="text-foreground truncate flex-1">
          {{ item.display_name || item.username || item.target }}
        </span>
        <Badge
          v-if="item.platform"
          variant="secondary"
          size="sm"
          font="mono"
          class="shrink-0"
        >
          {{ item.platform }}
        </Badge>
      </div>
    </div>
    <EmptyRow v-else>
      {{ t('chat.tools.detail.noContacts') }}
    </EmptyRow>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Badge } from '@memohai/ui'
import type { ToolCallBlock } from '@/store/chat-list'
import EmptyRow from './tool-detail/empty-row.vue'

interface Contact {
  route_id?: string
  platform?: string
  conversation_type?: string
  target?: string
  display_name?: string
  username?: string
}

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

const contacts = computed<Contact[]>(() => {
  if (!props.block.done || !props.block.result) return []
  const result = props.block.result as Record<string, unknown>
  const sc = result.structuredContent as Record<string, unknown> | undefined
  const items = (sc ?? result).contacts as Contact[] | undefined
  return Array.isArray(items) ? items : []
})
</script>
