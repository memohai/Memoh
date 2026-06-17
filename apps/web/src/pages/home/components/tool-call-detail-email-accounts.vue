<template>
  <div class="space-y-1.5">
    <div
      v-if="accounts.length"
      class="space-y-1"
    >
      <div
        v-for="(item, i) in accounts"
        :key="item.id ?? item.email ?? i"
        class="flex items-center gap-2 text-xs"
      >
        <span class="text-foreground truncate flex-1">{{ item.email || item.id }}</span>
        <span
          v-if="item.provider"
          class="text-caption text-muted-foreground font-mono shrink-0 rounded bg-muted/30 px-1 py-0.5"
        >{{ item.provider }}</span>
      </div>
    </div>
    <p
      v-else
      class="text-xs text-muted-foreground italic"
    >
      {{ t('chat.tools.detail.noAccounts') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ToolCallBlock } from '@/store/chat-list'

interface AccountItem {
  id?: string
  email?: string
  provider?: string
}

const props = defineProps<{ block: ToolCallBlock }>()
const { t } = useI18n()

function resolveResult(): Record<string, unknown> | null {
  if (!props.block.result) return null
  const result = props.block.result as Record<string, unknown>
  return (result.structuredContent as Record<string, unknown>) ?? result
}

const accounts = computed<AccountItem[]>(() => {
  if (!props.block.done) return []
  const r = resolveResult()
  if (!r) return []
  const items = r.accounts as AccountItem[] | undefined
  return Array.isArray(items) ? items : []
})
</script>
