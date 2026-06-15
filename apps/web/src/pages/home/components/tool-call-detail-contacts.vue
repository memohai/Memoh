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
        <span
          v-if="item.platform"
          class="text-caption text-muted-foreground font-mono shrink-0 rounded bg-muted/30 px-1 py-0.5"
        >
          {{ item.platform }}
        </span>
      </div>
    </div>
    <p
      v-else
      class="text-xs text-muted-foreground italic"
    >
      {{ t('chat.tools.detail.noContacts') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'

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
